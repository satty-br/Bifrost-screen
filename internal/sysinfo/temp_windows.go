//go:build windows

package sysinfo

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/satty-br/Bifrost-screen/internal/pawnio"
)

// OpenFileMappingW não vem no pacote windows, então é chamada direto.
var procOpenFileMappingW = kernel32.NewProc("OpenFileMappingW")

// Este arquivo reúne as formas de obter temperatura de CPU/GPU no Windows sem
// driver próprio nem privilégio de administrador. O Windows não expõe a
// temperatura do processador numa API pública: quem lê isso são programas de
// monitoramento com driver assinado (HWiNFO, LibreHardwareMonitor, MSI
// Afterburner, AIDA64). O Bifrost aproveita o que já estiver instalado.

const tempCacheFor = 5 * time.Second

// readPawnIOAgente pega a temperatura publicada pelo agente do Bifrost. O
// driver PawnIO só responde a processos administradores, e o Bifrost roda como
// usuário comum; então quem fala com o driver é o agente (o próprio
// bifrost.exe em modo --sensores, iniciado por uma tarefa agendada com
// privilégio), que escreve a leitura num arquivo em %ProgramData%\Bifrost.
// É a única fonte que não depende de nenhum outro programa instalado.
func readPawnIOAgente() (cpu, gpu float64) {
	l, err := pawnio.LerLeitura()
	if err != nil || !l.Fresca() || !validTemp(l.CPU) {
		pawnio.GarantirAgenteRodando()
		return -1, -1
	}
	return l.CPU, -1
}

// PawnIOStatus resume, para o painel, em que pé está a leitura por driver.
// TempDriverStatus diz ao painel se o driver está instalado e se o agente está
// publicando leituras, para ele oferecer ativar ou desativar o recurso.
func TempDriverStatus() PawnIOStatus {
	instalado, versao := pawnio.Instalado()
	st := PawnIOStatus{Installed: instalado, Version: versao, CPU: -1}
	l, err := pawnio.LerLeitura()
	if err != nil {
		return st
	}
	st.Error = l.Erro
	if l.Fresca() {
		st.Agent, st.CPU = true, l.CPU
	}
	return st
}

// externalTemps tenta as fontes externas em ordem de custo e devolve também o
// nome da fonte que respondeu (para mostrar no painel e no diagnóstico).
func (s *Sampler) externalTemps() (cpu, gpu float64, source string) {
	s.mu.Lock()
	if time.Since(s.hwTempAt) < tempCacheFor {
		cpu, gpu, source = s.hwCPUTemp, s.hwGPUTemp, s.hwTempSource
		s.mu.Unlock()
		return cpu, gpu, source
	}
	s.mu.Unlock()

	cpu, gpu, source = -1, -1, ""
	fontes := []struct {
		nome string
		ler  func() (float64, float64)
	}{
		{SourcePawnIOAgent, readPawnIOAgente},
		{SourceHWiNFO, readHWiNFORegistry},
		{SourceLHMWeb, readLHMWeb},
		{SourceAfterburner, readAfterburner},
		{SourceAIDA64, readAIDA64},
		{SourceLHMWMI, s.hwSensorTemps}, // último: abre um PowerShell, é lento
	}
	for _, f := range fontes {
		c, g := f.ler()
		if c >= 0 && cpu < 0 {
			cpu, source = c, f.nome
		}
		if g >= 0 && gpu < 0 {
			gpu = g
			if source == "" {
				source = f.nome
			}
		}
		if cpu >= 0 && gpu >= 0 {
			break
		}
	}

	s.mu.Lock()
	s.hwCPUTemp, s.hwGPUTemp, s.hwTempSource, s.hwTempAt = cpu, gpu, source, time.Now()
	s.mu.Unlock()
	return cpu, gpu, source
}

// --- HWiNFO ---------------------------------------------------------------
// O HWiNFO publica no registro os sensores marcados com "Report value in
// Gadget" (menu Sensor Settings). É a via mais barata: leitura direta do
// registro, sem processo extra.

func readHWiNFORegistry() (cpu, gpu float64) {
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		k, err := registry.OpenKey(root, `Software\HWiNFO64\VSB`, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		var entries []hwinfoEntry
		for i := 0; i < 256; i++ {
			idx := itoa(i)
			sensor, _, err := k.GetStringValue("Sensor" + idx)
			if err != nil {
				// pode haver buracos na numeração; segue tentando um pouco
				if i > 48 {
					break
				}
				continue
			}
			label, _, _ := k.GetStringValue("Label" + idx)
			value, _, _ := k.GetStringValue("Value" + idx)
			raw, _, _ := k.GetStringValue("ValueRaw" + idx)
			entries = append(entries, hwinfoEntry{Sensor: sensor, Label: label, Value: value, ValueRaw: raw})
		}
		k.Close()
		if len(entries) == 0 {
			continue
		}
		if c, g := parseHWiNFO(entries); c >= 0 || g >= 0 {
			return c, g
		}
	}
	return -1, -1
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [4]byte
	n := 0
	for i > 0 {
		b[n] = byte('0' + i%10)
		i /= 10
		n++
	}
	out := make([]byte, n)
	for j := 0; j < n; j++ {
		out[j] = b[n-1-j]
	}
	return string(out)
}

// --- LibreHardwareMonitor / OpenHardwareMonitor (servidor web) ------------
// Ambos têm um servidor embutido (Options > Remote Web Server), que serve a
// árvore de sensores em /data.json. Não precisa de PowerShell nem de WMI.

var lhmClient = &http.Client{Timeout: 900 * time.Millisecond}

func readLHMWeb() (cpu, gpu float64) {
	for _, url := range []string{"http://127.0.0.1:8085/data.json", "http://127.0.0.1:8086/data.json"} {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			cancel()
			continue
		}
		resp, err := lhmClient.Do(req)
		if err != nil {
			cancel()
			continue
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		cancel()
		if err != nil || resp.StatusCode != http.StatusOK {
			continue
		}
		if c, g := parseLHMTree(data); c >= 0 || g >= 0 {
			return c, g
		}
	}
	return -1, -1
}

// --- memória compartilhada (MSI Afterburner e AIDA64) ---------------------

// readSharedMemory devolve uma cópia da região de memória compartilhada `name`.
// A cópia evita mexer com ponteiros para memória fora do Go (o conteúdo muda
// enquanto o outro programa escreve, e a cópia é pequena: algumas centenas de KB).
func readSharedMemory(name string) ([]byte, error) {
	n, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	r, _, callErr := procOpenFileMappingW.Call(uintptr(windows.FILE_MAP_READ), 0, uintptr(unsafe.Pointer(n)))
	if r == 0 {
		return nil, callErr
	}
	h := windows.Handle(r)
	defer windows.CloseHandle(h)

	addr, err := windows.MapViewOfFile(h, windows.FILE_MAP_READ, 0, 0, 0)
	if err != nil {
		return nil, err
	}
	defer windows.UnmapViewOfFile(addr)

	var info windows.MemoryBasicInformation
	if err := windows.VirtualQuery(addr, &info, unsafe.Sizeof(info)); err != nil {
		return nil, err
	}
	size := int(info.RegionSize)
	if size <= 0 || size > 16<<20 {
		return nil, errors.New("região de memória compartilhada inválida")
	}
	buf := make([]byte, size)
	var read uintptr
	if err := windows.ReadProcessMemory(windows.CurrentProcess(), addr, &buf[0], uintptr(size), &read); err != nil {
		return nil, err
	}
	return buf[:read], nil
}

// Estrutura da memória compartilhada do MSI Afterburner (MAHM): cabeçalho com
// o número/tamanho das entradas, e entradas com 5 textos de 260 bytes seguidos
// do valor em float.
const (
	mahmSignature   = 0x4D48414D // 'MAHM'
	mahmHeaderSize  = 28
	mahmNameLen     = 260
	mahmDataOffset  = 5 * mahmNameLen
	mahmMinEntrySiz = mahmDataOffset + 4
)

func readAfterburner() (cpu, gpu float64) {
	buf, err := readSharedMemory("MAHMSharedMemory")
	if err != nil {
		return -1, -1
	}
	if len(buf) < mahmHeaderSize || binary.LittleEndian.Uint32(buf[0:4]) != mahmSignature {
		return -1, -1
	}
	headerSize := int(binary.LittleEndian.Uint32(buf[8:12]))
	numEntries := int(binary.LittleEndian.Uint32(buf[12:16]))
	entrySize := int(binary.LittleEndian.Uint32(buf[16:20]))
	if headerSize <= 0 || entrySize < mahmMinEntrySiz || numEntries <= 0 || numEntries > 4096 {
		return -1, -1
	}
	var names []string
	var values []float64
	for i := 0; i < numEntries; i++ {
		off := headerSize + i*entrySize
		if off+entrySize > len(buf) {
			break
		}
		entry := buf[off : off+entrySize]
		names = append(names, cString(entry[:mahmNameLen]))
		values = append(values, float64(math.Float32frombits(binary.LittleEndian.Uint32(entry[mahmDataOffset:mahmDataOffset+4]))))
	}
	return parseAfterburnerEntries(names, values)
}

func readAIDA64() (cpu, gpu float64) {
	buf, err := readSharedMemory("AIDA64_SensorValues")
	if err != nil {
		return -1, -1
	}
	return parseAIDA64(cString(buf))
}

func cString(b []byte) string {
	if i := indexZero(b); i >= 0 {
		b = b[:i]
	}
	return strings.TrimSpace(string(b))
}

func indexZero(b []byte) int {
	for i, c := range b {
		if c == 0 {
			return i
		}
	}
	return -1
}

// TempDiagnostics consulta todas as fontes e diz o que cada uma respondeu.
// Usado pelo relatório do `bifrost.exe --diagnostico`.
func (s *Sampler) TempDiagnostics() []TempSourceStatus {
	fontes := []struct {
		nome string
		hint string
		ler  func() (float64, float64)
	}{
		{SourceHWiNFO, hintHWiNFO, readHWiNFORegistry},
		{SourcePawnIOAgent, hintPawnIOAgent, readPawnIOAgente},
		{SourceNVML, hintNVML, soGPU(nvmlTemp)},
		{SourceADL, hintADL, soGPU(adlTemp)},
		{SourceNvidiaSMI, hintNvidiaSMI, soGPU(nvidiaSMITemp)},
		{SourceLHMWeb, hintLHMWeb, readLHMWeb},
		{SourceAfterburner, hintAfterburner, readAfterburner},
		{SourceAIDA64, hintAIDA64, readAIDA64},
		{SourceLHMWMI, hintLHMWMI, s.hwSensorTemps},
	}
	out := make([]TempSourceStatus, 0, len(fontes)+1)
	st := s.Get()
	out = append(out, TempSourceStatus{
		Name: SourceACPI, Available: st.CPUTempSource == SourceACPI,
		CPU: acpiOnly(st), GPU: -1, Hint: hintACPI,
	})
	for _, f := range fontes {
		cpu, gpu := f.ler()
		out = append(out, TempSourceStatus{
			Name: f.nome, Available: cpu >= 0 || gpu >= 0,
			CPU: cpu, GPU: gpu, Hint: f.hint,
		})
	}
	return out
}

func acpiOnly(st Stats) float64 {
	if st.CPUTempSource == SourceACPI {
		return st.CPUTemp
	}
	return -1
}
