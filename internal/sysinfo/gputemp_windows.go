//go:build windows

package sysinfo

// Temperatura da GPU sem nenhum programa extra: as próprias bibliotecas que o
// driver de vídeo instala.
//
//   - NVIDIA: nvml.dll (NVIDIA Management Library), a mesma que o nvidia-smi
//     usa por baixo. Chamar a DLL direto evita abrir um processo a cada
//     amostra — o nvidia-smi demora de 1 a 3 segundos na primeira chamada e
//     nem sempre está no PATH, que é o motivo de a temperatura não aparecer.
//   - AMD: atiadlxx.dll (ADL, AMD Display Library), instalada pelo Adrenalin.
//     Tentamos as três gerações de interface de temperatura (OverdriveN, 6 e 5),
//     porque cada família de placa expõe uma delas.
//
// Nada disso pede privilégio de administrador nem instala coisa alguma.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// soGPU adapta uma fonte que só lê GPU para o formato (cpu, gpu) usado pela
// lista de fontes do diagnóstico.
func soGPU(f func() (float64, bool)) func() (float64, float64) {
	return func() (float64, float64) {
		if v, ok := f(); ok {
			return -1, v
		}
		return -1, -1
	}
}

// --- NVIDIA (NVML) --------------------------------------------------------

const nvmlTemperatureGPU = 0 // NVML_TEMPERATURE_GPU

type nvmlAPI struct {
	init        *windows.LazyProc
	handleByIdx *windows.LazyProc
	temperature *windows.LazyProc
	ok          bool
}

var (
	nvmlOnce sync.Once
	nvml     nvmlAPI
	nvmlDev  uintptr // handle da primeira GPU
)

func loadNVML() {
	dll := windows.NewLazySystemDLL("nvml.dll")
	if err := dll.Load(); err != nil {
		// Instalações antigas do driver deixam a DLL só na pasta do NVSMI.
		alt := filepath.Join(os.Getenv("ProgramFiles"), "NVIDIA Corporation", "NVSMI", "nvml.dll")
		if _, err := os.Stat(alt); err != nil {
			return
		}
		dll = windows.NewLazyDLL(alt)
		if err := dll.Load(); err != nil {
			return
		}
	}
	nvml = nvmlAPI{
		init:        dll.NewProc("nvmlInit_v2"),
		handleByIdx: dll.NewProc("nvmlDeviceGetHandleByIndex_v2"),
		temperature: dll.NewProc("nvmlDeviceGetTemperature"),
	}
	for _, p := range []*windows.LazyProc{nvml.init, nvml.handleByIdx, nvml.temperature} {
		if err := p.Find(); err != nil {
			return
		}
	}
	if r, _, _ := nvml.init.Call(); r != 0 { // NVML_SUCCESS == 0
		return
	}
	var dev uintptr
	if r, _, _ := nvml.handleByIdx.Call(0, uintptr(unsafe.Pointer(&dev))); r != 0 || dev == 0 {
		return
	}
	nvmlDev, nvml.ok = dev, true
}

// nvmlTemp devolve a temperatura do núcleo da GPU NVIDIA.
func nvmlTemp() (float64, bool) {
	nvmlOnce.Do(loadNVML)
	if !nvml.ok {
		return 0, false
	}
	var t uint32
	r, _, _ := nvml.temperature.Call(nvmlDev, nvmlTemperatureGPU, uintptr(unsafe.Pointer(&t)))
	if r != 0 {
		return 0, false
	}
	v := float64(t)
	if !validTemp(v) {
		return 0, false
	}
	return v, true
}

// --- AMD (ADL) ------------------------------------------------------------

const (
	adlOK              = 0
	adlOKWarning       = 1
	adlTempTypeCore    = 1 // ODN_TEMPERATURE_TYPE_CORE
	adlMaxAdaptadores  = 8
	adlMaxPath         = 256
	adlPMLogMaxSensors = 256
)

// placas RDNA2/RDNA3 (ex.: RX 6000/7000) não respondem mais à Overdrive5/6/N
// (voltam ADL_ERR_NOT_SUPPORTED) — a AMD trocou pra essa API de "PM Log" a
// partir do Vega. Índices de github.com/GPUOpen-LibrariesAndSDKs/display-library.
const (
	pmlogTemperatureEdge    = 8
	pmlogTemperatureMem     = 9
	pmlogTemperatureHotspot = 27
)

// adlAdapterInfo espelha a struct AdapterInfo do SDK da ADL (adl_structures.h)
// byte a byte — os índices de adaptador não são necessariamente 0..N-1
// sequenciais, então precisa ler os índices reais daqui antes de consultar
// temperatura de cada um.
type adlAdapterInfo struct {
	Size           int32
	AdapterIndex   int32
	UDID           [adlMaxPath]byte
	BusNumber      int32
	DeviceNumber   int32
	FunctionNumber int32
	VendorID       int32
	AdapterName    [adlMaxPath]byte
	DisplayName    [adlMaxPath]byte
	Present        int32
	Exist          int32
	DriverPath     [adlMaxPath]byte
	DriverPathExt  [adlMaxPath]byte
	PNPString      [adlMaxPath]byte
	OSDisplayIndex int32
}

type adlSingleSensorData struct {
	Supported int32
	Value     int32
}

// adlPMLogDataOutput espelha ADLPMLogDataOutput (adl_structures.h).
type adlPMLogDataOutput struct {
	Size    int32
	Sensors [adlPMLogMaxSensors]adlSingleSensorData
}

type adlAPI struct {
	create      *windows.LazyProc
	destroy     *windows.LazyProc
	numAdapters *windows.LazyProc
	adapterInfo *windows.LazyProc
	pmLogQuery  *windows.LazyProc
	odnTemp     *windows.LazyProc
	od6Temp     *windows.LazyProc
	od5Temp     *windows.LazyProc
	ctx         uintptr
	ok          bool
}

var (
	adlOnce sync.Once
	adl     adlAPI
	adlMu   sync.Mutex
)

// adlMalloc é o alocador que a ADL exige receber na inicialização. Usamos o
// heap do Windows (LocalAlloc) para a memória não ficar sob o coletor do Go.
var adlMalloc = syscall.NewCallback(func(size uintptr) uintptr {
	const lptr = 0x0040 // LMEM_FIXED | LMEM_ZEROINIT
	r, _, _ := procLocalAlloc.Call(lptr, size)
	return r
})

var procLocalAlloc = windows.NewLazySystemDLL("kernel32.dll").NewProc("LocalAlloc")

func loadADL() {
	dll := windows.NewLazySystemDLL("atiadlxx.dll")
	if err := dll.Load(); err != nil {
		dll = windows.NewLazySystemDLL("atiadlxy.dll") // variante 32 bits
		if err := dll.Load(); err != nil {
			return
		}
	}
	a := adlAPI{
		create:      dll.NewProc("ADL2_Main_Control_Create"),
		destroy:     dll.NewProc("ADL2_Main_Control_Destroy"),
		numAdapters: dll.NewProc("ADL2_Adapter_NumberOfAdapters_Get"),
		adapterInfo: dll.NewProc("ADL2_Adapter_AdapterInfo_Get"),
		pmLogQuery:  dll.NewProc("ADL2_New_QueryPMLogData_Get"),
		odnTemp:     dll.NewProc("ADL2_OverdriveN_Temperature_Get"),
		od6Temp:     dll.NewProc("ADL2_Overdrive6_Temperature_Get"),
		od5Temp:     dll.NewProc("ADL2_Overdrive5_Temperature_Get"),
	}
	if a.create.Find() != nil {
		return
	}
	var ctx uintptr
	// 1 = só adaptadores conectados.
	if r, _, _ := a.create.Call(adlMalloc, 1, uintptr(unsafe.Pointer(&ctx))); int32(r) != adlOK || ctx == 0 {
		return
	}
	a.ctx, a.ok = ctx, true
	adl = a
}

// adlTemp lê a temperatura da GPU AMD: primeiro tenta a API de PM Log
// (RDNA/RDNA2/RDNA3 e Vega em diante), e só cai pras interfaces antigas
// (OverdriveN/6/5) se a placa não responder à PM Log (GCN mais velhas).
func adlTemp() (float64, bool) {
	adlOnce.Do(loadADL)
	if !adl.ok {
		return 0, false
	}
	adlMu.Lock()
	defer adlMu.Unlock()

	n := adlMaxAdaptadores
	if adl.numAdapters.Find() == nil {
		var num int32
		if r, _, _ := adl.numAdapters.Call(adl.ctx, uintptr(unsafe.Pointer(&num))); int32(r) == adlOK && num > 0 {
			n = int(num)
		}
	}
	if n <= 0 {
		return 0, false
	}

	indices := adlAdapterIndices(n)
	if len(indices) == 0 {
		// não deu pra enumerar os adaptadores reais: tenta 0..n-1 do jeito antigo
		for i := 0; i < n && i < adlMaxAdaptadores; i++ {
			indices = append(indices, i)
		}
	}
	for _, idx := range indices {
		if v, ok := adlPMLogTemp(idx); ok {
			return v, true
		}
	}
	for _, idx := range indices {
		if v, ok := adlTempAdaptador(idx); ok {
			return v, true
		}
	}
	return 0, false
}

// adlAdapterIndices devolve os índices REAIS dos adaptadores presentes, lidos
// de ADL2_Adapter_AdapterInfo_Get — não dá pra supor que são 0..n-1
// sequenciais (o Windows numera com buracos, ainda mais com GPU integrada e
// dedicada ao mesmo tempo).
func adlAdapterIndices(n int) []int {
	if adl.adapterInfo.Find() != nil {
		return nil
	}
	size := int(unsafe.Sizeof(adlAdapterInfo{}))
	buf := make([]byte, size*n)
	r, _, _ := adl.adapterInfo.Call(adl.ctx, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if int32(r) != adlOK {
		return nil
	}
	var out []int
	for i := 0; i < n; i++ {
		info := (*adlAdapterInfo)(unsafe.Pointer(&buf[i*size]))
		if info.Present != 0 {
			out = append(out, int(info.AdapterIndex))
		}
	}
	return out
}

// adlPMLogTemp lê a temperatura pela API de PM Log (a única que placas
// RDNA/RDNA2/RDNA3 respondem) — prefere a temperatura "edge" (a que os
// programas de monitoramento chamam de "temperatura da GPU"), com "hotspot"
// e memória como reserva.
func adlPMLogTemp(idx int) (float64, bool) {
	if adl.pmLogQuery.Find() != nil {
		return 0, false
	}
	var out adlPMLogDataOutput
	out.Size = int32(unsafe.Sizeof(out))
	r, _, _ := adl.pmLogQuery.Call(adl.ctx, uintptr(idx), uintptr(unsafe.Pointer(&out)))
	if int32(r) != adlOK {
		return 0, false
	}
	for _, sensor := range []int{pmlogTemperatureEdge, pmlogTemperatureHotspot, pmlogTemperatureMem} {
		s := out.Sensors[sensor]
		if s.Supported != 0 && validTemp(float64(s.Value)) {
			return float64(s.Value), true
		}
	}
	return 0, false
}

func adlTempAdaptador(idx int) (float64, bool) {
	// OverdriveN e Overdrive6 devolvem milésimos de grau.
	if adl.odnTemp.Find() == nil {
		var t int32
		r, _, _ := adl.odnTemp.Call(adl.ctx, uintptr(idx), adlTempTypeCore, uintptr(unsafe.Pointer(&t)))
		if ok := int32(r); (ok == adlOK || ok == adlOKWarning) && t > 0 {
			if v := float64(t) / 1000; validTemp(v) {
				return v, true
			}
		}
	}
	if adl.od6Temp.Find() == nil {
		var t int32
		r, _, _ := adl.od6Temp.Call(adl.ctx, uintptr(idx), uintptr(unsafe.Pointer(&t)))
		if ok := int32(r); (ok == adlOK || ok == adlOKWarning) && t > 0 {
			if v := float64(t) / 1000; validTemp(v) {
				return v, true
			}
		}
	}
	if adl.od5Temp.Find() == nil {
		// ADLTemperature: { int iSize; int iTemperature } (milésimos de grau).
		var st struct {
			Size int32
			Temp int32
		}
		st.Size = int32(unsafe.Sizeof(st))
		r, _, _ := adl.od5Temp.Call(adl.ctx, uintptr(idx), 0, uintptr(unsafe.Pointer(&st)))
		if ok := int32(r); (ok == adlOK || ok == adlOKWarning) && st.Temp > 0 {
			if v := float64(st.Temp) / 1000; validTemp(v) {
				return v, true
			}
		}
	}
	return 0, false
}

// --- nvidia-smi (reserva) -------------------------------------------------

// nvidiaSMITemp é a última tentativa para placas NVIDIA, caso a nvml.dll não
// esteja onde o Windows procura. O tempo é generoso de propósito: a primeira
// chamada do nvidia-smi costuma levar mais de um segundo.
func nvidiaSMITemp() (float64, bool) {
	caminhos := []string{"nvidia-smi"}
	for _, base := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramW6432"), os.Getenv("SystemRoot")} {
		if base == "" {
			continue
		}
		caminhos = append(caminhos,
			filepath.Join(base, "NVIDIA Corporation", "NVSMI", "nvidia-smi.exe"),
			filepath.Join(base, "System32", "nvidia-smi.exe"),
		)
	}
	for _, c := range caminhos {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		cmd := exec.CommandContext(ctx, c, "--query-gpu=temperature.gpu", "--format=csv,noheader,nounits")
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		out, err := cmd.Output()
		cancel()
		if err != nil {
			continue
		}
		if v, ok := parseNvidiaSMI(string(out)); ok {
			return v, true
		}
	}
	return 0, false
}

// --- identificação da placa de vídeo --------------------------------------

// GPUAdapters devolve o nome das placas de vídeo instaladas, lido do registro
// (a mesma informação que o Gerenciador de Dispositivos mostra). Serve para o
// diagnóstico dizer por que a temperatura não apareceu: cada fabricante só
// entrega a leitura pela sua própria biblioteca, e a Intel não entrega nenhuma.
func GPUAdapters() []string {
	const classe = `SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}`
	var out []string
	vistos := map[string]bool{}
	for i := 0; i < 12; i++ {
		// As subchaves são numeradas com quatro dígitos: 0000, 0001, ...
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, fmt.Sprintf(`%s\%04d`, classe, i), registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		nome, _, err := k.GetStringValue("DriverDesc")
		k.Close()
		if err != nil || nome == "" || vistos[nome] {
			continue
		}
		vistos[nome] = true
		out = append(out, nome)
	}
	return out
}
