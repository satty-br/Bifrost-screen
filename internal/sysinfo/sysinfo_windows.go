//go:build windows

package sysinfo

import (
	"context"
	"encoding/json"
	"log"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	pdh                      = windows.NewLazySystemDLL("pdh.dll")
	procPdhOpenQuery         = pdh.NewProc("PdhOpenQueryW")
	procPdhAddEnglishCounter = pdh.NewProc("PdhAddEnglishCounterW")
	procPdhCollectQueryData  = pdh.NewProc("PdhCollectQueryData")
	procPdhGetFormattedValue = pdh.NewProc("PdhGetFormattedCounterValue")
	procPdhGetFormattedArray = pdh.NewProc("PdhGetFormattedCounterArrayW")
	kernel32                 = windows.NewLazySystemDLL("kernel32.dll")
	procGlobalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
	procGetTickCount64       = kernel32.NewProc("GetTickCount64")
)

const (
	pdhFmtDouble   = 0x00000200
	pdhFmtNoCap100 = 0x00008000
	pdhMoreData    = 0x800007D2
)

type pdhValue struct {
	CStatus uint32
	_       uint32
	Double  float64
}

type pdhItem struct {
	Name  *uint16
	Value pdhValue
}

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

type pdhQuery struct {
	h       uintptr
	cpu     uintptr
	gpu     uintptr
	temp    uintptr
	netDown uintptr
	netUp   uintptr
}

func addCounter(q uintptr, path string) uintptr {
	p, _ := windows.UTF16PtrFromString(path)
	var c uintptr
	r, _, _ := procPdhAddEnglishCounter.Call(q, uintptr(unsafe.Pointer(p)), 0, uintptr(unsafe.Pointer(&c)))
	if r != 0 {
		return 0
	}
	return c
}

func newPDH() *pdhQuery {
	if procPdhOpenQuery.Find() != nil {
		return nil
	}
	q := &pdhQuery{}
	if r, _, _ := procPdhOpenQuery.Call(0, 0, uintptr(unsafe.Pointer(&q.h))); r != 0 {
		log.Printf("sistema: PdhOpenQuery falhou: 0x%x", r)
		return nil
	}
	q.cpu = addCounter(q.h, `\Processor Information(_Total)\% Processor Utility`)
	if q.cpu == 0 {
		q.cpu = addCounter(q.h, `\Processor(_Total)\% Processor Time`)
	}
	q.gpu = addCounter(q.h, `\GPU Engine(*engtype_3D)\Utilization Percentage`)
	q.temp = addCounter(q.h, `\Thermal Zone Information(*)\Temperature`)
	q.netDown = addCounter(q.h, `\Network Interface(*)\Bytes Received/sec`)
	q.netUp = addCounter(q.h, `\Network Interface(*)\Bytes Sent/sec`)
	procPdhCollectQueryData.Call(q.h)
	return q
}

func single(c uintptr) float64 {
	if c == 0 {
		return -1
	}
	var v pdhValue
	r, _, _ := procPdhGetFormattedValue.Call(c, pdhFmtDouble|pdhFmtNoCap100, 0, uintptr(unsafe.Pointer(&v)))
	if r != 0 || v.CStatus != 0 {
		return -1
	}
	return v.Double
}

// sum soma todas as instâncias de um contador com curinga.
func sum(c uintptr, skip func(string) bool) float64 {
	if c == 0 {
		return -1
	}
	var size, count uint32
	r, _, _ := procPdhGetFormattedArray.Call(c, pdhFmtDouble|pdhFmtNoCap100, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), 0)
	if uint32(r) != pdhMoreData || size == 0 {
		if count == 0 {
			return 0
		}
		return -1
	}
	buf := make([]byte, size)
	r, _, _ = procPdhGetFormattedArray.Call(c, pdhFmtDouble|pdhFmtNoCap100, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&buf[0])))
	if r != 0 {
		return -1
	}
	items := unsafe.Slice((*pdhItem)(unsafe.Pointer(&buf[0])), count)
	total := 0.0
	for _, it := range items {
		if it.Value.CStatus != 0 {
			continue
		}
		if skip != nil && skip(windows.UTF16PtrToString(it.Name)) {
			continue
		}
		total += it.Value.Double
	}
	return total
}

// pdhMax devolve o maior valor entre as instâncias de um contador com curinga
// (usado na temperatura: cada zona térmica é uma instância separada).
func pdhMax(c uintptr) float64 {
	if c == 0 {
		return -1
	}
	var size, count uint32
	r, _, _ := procPdhGetFormattedArray.Call(c, pdhFmtDouble|pdhFmtNoCap100, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), 0)
	if uint32(r) != pdhMoreData || size == 0 {
		return -1
	}
	buf := make([]byte, size)
	r, _, _ = procPdhGetFormattedArray.Call(c, pdhFmtDouble|pdhFmtNoCap100, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&buf[0])))
	if r != 0 {
		return -1
	}
	items := unsafe.Slice((*pdhItem)(unsafe.Pointer(&buf[0])), count)
	best := -1.0
	for _, it := range items {
		if it.Value.CStatus != 0 {
			continue
		}
		if it.Value.Double > best {
			best = it.Value.Double
		}
	}
	return best
}

func isVirtualNIC(name string) bool {
	l := strings.ToLower(name)
	for _, k := range []string{"loopback", "isatap", "teredo", "hyper-v", "vethernet", "virtual", "vpn", "wan miniport"} {
		if strings.Contains(l, k) {
			return true
		}
	}
	return false
}

// Start mede o sistema a cada intervalo.
func (s *Sampler) Start(interval time.Duration) {
	go func() {
		q := newPDH()
		for {
			time.Sleep(interval)
			st := Stats{CPU: -1, GPU: -1, CPUTemp: -1, GPUTemp: -1, NetDown: -1, NetUp: -1}
			if q != nil {
				procPdhCollectQueryData.Call(q.h)
				st.CPU = clamp100(single(q.cpu))
				if g := sum(q.gpu, nil); g >= 0 {
					st.GPU = clamp100(g)
				}
				// contador vem em décimos de Kelvin.
				if raw := pdhMax(q.temp); raw >= 0 {
					st.CPUTemp = raw/10 - 273.15
				}
				st.NetDown = sum(q.netDown, isVirtualNIC)
				st.NetUp = sum(q.netUp, isVirtualNIC)
			}
			if st.CPUTemp >= 0 {
				st.CPUTempSource = SourceACPI
			}
			st.GPUTemp, st.GPUTempSource = s.gpuTemperature()
			if st.CPUTemp < 0 || st.GPUTemp < 0 {
				// A maioria das placas-mãe não expõe a temperatura da CPU por API
				// pública do Windows, e as bibliotecas de vídeo só respondem pela
				// GPU do próprio fabricante. O que faltar vem do agente do PawnIO
				// ou de um programa de monitoramento já instalado.
				exCPU, exGPU, src := s.externalTemps()
				if st.CPUTemp < 0 && exCPU >= 0 {
					st.CPUTemp, st.CPUTempSource = exCPU, src
				}
				if st.GPUTemp < 0 && exGPU >= 0 {
					st.GPUTemp, st.GPUTempSource = exGPU, src
				}
			}
			var mem memoryStatusEx
			mem.Length = uint32(unsafe.Sizeof(mem))
			if r, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&mem))); r != 0 {
				st.RAMTotal = mem.TotalPhys
				st.RAMUsed = mem.TotalPhys - mem.AvailPhys
			}
			root, _ := windows.UTF16PtrFromString(s.currentDisk() + `\`)
			var free, total, totalFree uint64
			if err := windows.GetDiskFreeSpaceEx(root, &free, &total, &totalFree); err == nil {
				st.DiskTotal = total
				st.DiskUsed = total - totalFree
			}
			if r, _, _ := procGetTickCount64.Call(); r != 0 {
				st.Uptime = time.Duration(r) * time.Millisecond
			}
			s.store(st)
		}
	}()
}

func clamp100(v float64) float64 {
	if v < 0 {
		return v
	}
	if v > 100 {
		return 100
	}
	return v
}

// gpuTemperature lê a temperatura da GPU nas bibliotecas do próprio driver de
// vídeo (ver gputemp_windows.go) e devolve também o nome da fonte que
// respondeu. O resultado fica em cache por alguns segundos.
func (s *Sampler) gpuTemperature() (float64, string) {
	s.mu.Lock()
	if time.Since(s.gpuTempAt) < 5*time.Second {
		v, src := s.gpuTemp, s.gpuTempSrc
		s.mu.Unlock()
		return v, src
	}
	s.mu.Unlock()

	v, src := -1.0, ""
	if t, ok := nvmlTemp(); ok {
		v, src = t, SourceNVML
	} else if t, ok := adlTemp(); ok {
		v, src = t, SourceADL
	} else if t, ok := nvidiaSMITemp(); ok {
		v, src = t, SourceNvidiaSMI
	}

	s.mu.Lock()
	s.gpuTemp, s.gpuTempSrc, s.gpuTempAt = v, src, time.Now()
	s.mu.Unlock()
	return v, src
}

type hwSensor struct {
	Name   string
	Parent string
	Value  float64
}

// hwSensorTemps lê a temperatura de CPU/GPU do LibreHardwareMonitor (ou
// OpenHardwareMonitor) via WMI, se o usuário tiver o programa instalado e
// aberto (ele expõe root\LibreHardwareMonitor enquanto está rodando). É a
// única fonte que funciona em qualquer placa-mãe/GPU sem precisar de driver
// próprio nosso. Sem ele rodando, os sensores voltam zerados e o resultado
// fica -1 (indisponível), como as outras fontes.
func (s *Sampler) hwSensorTemps() (cpu, gpu float64) {
	s.mu.Lock()
	if time.Since(s.hwTempAt) < 5*time.Second {
		cpu, gpu = s.hwCPUTemp, s.hwGPUTemp
		s.mu.Unlock()
		return cpu, gpu
	}
	s.mu.Unlock()

	cpu, gpu = -1, -1
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	const script = `Get-CimInstance -Namespace root\LibreHardwareMonitor -ClassName Sensor -ErrorAction SilentlyContinue | ` +
		`Where-Object { $_.SensorType -eq 'Temperature' } | Select-Object Name,Parent,Value | ConvertTo-Json -Compress`
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if out, err := cmd.Output(); err == nil {
		cpu, gpu = parseHWSensors(out)
	}
	s.mu.Lock()
	s.hwCPUTemp, s.hwGPUTemp, s.hwTempAt = cpu, gpu, time.Now()
	s.mu.Unlock()
	return cpu, gpu
}

// parseHWSensors escolhe, entre os sensores de temperatura do LibreHardwareMonitor,
// o mais representativo da CPU e da GPU (a lista de nomes muda por fabricante).
func parseHWSensors(data []byte) (cpu, gpu float64) {
	cpu, gpu = -1, -1
	var list []hwSensor
	if err := json.Unmarshal(data, &list); err != nil {
		// ConvertTo-Json devolve um objeto solto (não uma lista) quando só há 1 item.
		var one hwSensor
		if json.Unmarshal(data, &one) != nil {
			return
		}
		list = []hwSensor{one}
	}
	cpuNames := map[string]int{"core (tctl/tdie)": 3, "cpu package": 3, "package": 2}
	gpuNames := map[string]int{"gpu core": 3, "gpu hot spot": 2}
	bestCPU, bestCPUScore := -1.0, 0
	bestGPU, bestGPUScore := -1.0, 0
	for _, sn := range list {
		if sn.Value <= 0 {
			continue
		}
		name := strings.ToLower(sn.Name)
		switch {
		case strings.HasPrefix(sn.Parent, "/amdcpu/") || strings.HasPrefix(sn.Parent, "/intelcpu/"):
			score := cpuNames[name]
			if score == 0 {
				score = 1
			}
			if score > bestCPUScore {
				bestCPUScore, bestCPU = score, sn.Value
			}
		case strings.HasPrefix(sn.Parent, "/gpu-amd/") || strings.HasPrefix(sn.Parent, "/gpu-nvidia/") || strings.HasPrefix(sn.Parent, "/gpu-intel/"):
			score := gpuNames[name]
			if score == 0 {
				score = 1
			}
			if score > bestGPUScore {
				bestGPUScore, bestGPU = score, sn.Value
			}
		}
	}
	if bestCPUScore > 0 {
		cpu = bestCPU
	}
	if bestGPUScore > 0 {
		gpu = bestGPU
	}
	return
}
