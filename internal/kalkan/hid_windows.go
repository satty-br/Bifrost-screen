//go:build windows

package kalkan

import (
	"fmt"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Acesso HID sem CGO, pela mesma API que o Gerenciador de Dispositivos usa
// (SetupAPI + HidD_*/HidP_*). Diferente do mostrador Mancer, aqui também
// precisamos LER relatórios de entrada (o painel responde 200/400), então o
// arquivo é aberto com FILE_FLAG_OVERLAPPED e a leitura tem tempo limite.

var (
	modSetupapi = windows.NewLazySystemDLL("setupapi.dll")
	modHid      = windows.NewLazySystemDLL("hid.dll")
	modKernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procSetupDiGetClassDevsW             = modSetupapi.NewProc("SetupDiGetClassDevsW")
	procSetupDiEnumDeviceInterfaces      = modSetupapi.NewProc("SetupDiEnumDeviceInterfaces")
	procSetupDiGetDeviceInterfaceDetailW = modSetupapi.NewProc("SetupDiGetDeviceInterfaceDetailW")
	procSetupDiDestroyDeviceInfoList     = modSetupapi.NewProc("SetupDiDestroyDeviceInfoList")
	procHidDGetHidGuid                   = modHid.NewProc("HidD_GetHidGuid")
	procHidDGetAttributes                = modHid.NewProc("HidD_GetAttributes")
	procHidDGetPreparsedData             = modHid.NewProc("HidD_GetPreparsedData")
	procHidDFreePreparsedData            = modHid.NewProc("HidD_FreePreparsedData")
	procHidPGetCaps                      = modHid.NewProc("HidP_GetCaps")
	procCancelIoEx                       = modKernel32.NewProc("CancelIoEx")
)

const (
	digcfPresent         = 0x00000002
	digcfDeviceInterface = 0x00000010
	invalidHandleValue   = ^uintptr(0)
	// Peculiaridade documentada da API: cbSize de SP_DEVICE_INTERFACE_DETAIL_DATA
	// é sempre 8 no Windows 64-bit, independente do sizeof real da struct.
	deviceInterfaceDetailSize = 8
)

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

type spDeviceInterfaceData struct {
	cbSize             uint32
	interfaceClassGUID guid
	flags              uint32
	reserved           uintptr
}

type hidAttributes struct {
	size          uint32
	vendorID      uint16
	productID     uint16
	versionNumber uint16
}

type hidpCaps struct {
	usage                     uint16
	usagePage                 uint16
	inputReportByteLength     uint16
	outputReportByteLength    uint16
	featureReportByteLength   uint16
	reserved                  [17]uint16
	numberLinkCollectionNodes uint16
	numberInputButtonCaps     uint16
	numberInputValueCaps      uint16
	numberInputDataIndices    uint16
	numberOutputButtonCaps    uint16
	numberOutputValueCaps     uint16
	numberOutputDataIndices   uint16
	numberFeatureButtonCaps   uint16
	numberFeatureValueCaps    uint16
	numberFeatureDataIndices  uint16
}

func hidGUID() guid {
	var g guid
	procHidDGetHidGuid.Call(uintptr(unsafe.Pointer(&g)))
	return g
}

// Devices lista as interfaces HID do VID da família, com o que dá pra saber
// sem abrir uma sessão com o painel.
func Devices() ([]DeviceInfo, error) { return enumerar(VendorID) }

// AllDevices lista TODOS os dispositivos HID do PC. Serve pro diagnóstico:
// se o painel não usar o VID que esperamos, é aqui que ele aparece.
func AllDevices() ([]DeviceInfo, error) { return enumerar(0) }

// enumerar varre os dispositivos HID; vid 0 quer dizer "todos".
func enumerar(vid uint16) ([]DeviceInfo, error) {
	g := hidGUID()
	h, _, _ := procSetupDiGetClassDevsW.Call(uintptr(unsafe.Pointer(&g)), 0, 0,
		uintptr(digcfPresent|digcfDeviceInterface))
	if h == 0 || h == invalidHandleValue {
		return nil, fmt.Errorf("SetupDiGetClassDevs falhou")
	}
	defer procSetupDiDestroyDeviceInfoList.Call(h)

	var achados []DeviceInfo
	for idx := uint32(0); ; idx++ {
		var ifData spDeviceInterfaceData
		ifData.cbSize = uint32(unsafe.Sizeof(ifData))
		ret, _, _ := procSetupDiEnumDeviceInterfaces.Call(h, 0, uintptr(unsafe.Pointer(&g)),
			uintptr(idx), uintptr(unsafe.Pointer(&ifData)))
		if ret == 0 {
			break
		}
		var needed uint32
		procSetupDiGetDeviceInterfaceDetailW.Call(h, uintptr(unsafe.Pointer(&ifData)), 0, 0,
			uintptr(unsafe.Pointer(&needed)), 0)
		if needed == 0 {
			continue
		}
		buf := make([]byte, needed)
		*(*uint32)(unsafe.Pointer(&buf[0])) = deviceInterfaceDetailSize
		ret, _, _ = procSetupDiGetDeviceInterfaceDetailW.Call(h, uintptr(unsafe.Pointer(&ifData)),
			uintptr(unsafe.Pointer(&buf[0])), uintptr(needed), 0, 0)
		if ret == 0 {
			continue
		}
		path := windows.UTF16ToString(unsafe.Slice((*uint16)(unsafe.Pointer(&buf[4])), (len(buf)-4)/2))

		info, err := inspecionar(path)
		if err != nil {
			continue
		}
		if vid != 0 && info.VendorID != vid {
			continue
		}
		achados = append(achados, info)
	}
	return achados, nil
}

// inspecionar abre o caminho HID só pra ler atributos e capacidades, e fecha.
func inspecionar(path string) (DeviceInfo, error) {
	info := DeviceInfo{Path: path, Interface: interfaceDoCaminho(path)}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return info, err
	}
	// Só o suficiente pra consultar: alguns painéis recusam GENERIC_WRITE
	// enquanto o app do fabricante está aberto, mas deixam consultar.
	h, err := windows.CreateFile(p, 0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return info, err
	}
	defer windows.CloseHandle(h)

	var attrs hidAttributes
	attrs.size = uint32(unsafe.Sizeof(attrs))
	procHidDGetAttributes.Call(uintptr(h), uintptr(unsafe.Pointer(&attrs)))
	info.VendorID, info.ProductID, info.Version = attrs.vendorID, attrs.productID, attrs.versionNumber

	var preparsed uintptr
	if ret, _, _ := procHidDGetPreparsedData.Call(uintptr(h), uintptr(unsafe.Pointer(&preparsed))); ret != 0 && preparsed != 0 {
		defer procHidDFreePreparsedData.Call(preparsed)
		var caps hidpCaps
		procHidPGetCaps.Call(preparsed, uintptr(unsafe.Pointer(&caps)))
		info.InputReportLen = int(caps.inputReportByteLength)
		info.OutputReportLen = int(caps.outputReportByteLength)
		info.FeatureReportLen = int(caps.featureReportByteLength)
		info.UsagePage, info.Usage = caps.usagePage, caps.usage
	}
	info.Product, info.KnownProduct = Lookup(info.ProductID)
	return info, nil
}

// interfaceDoCaminho tira o "mi_XX" do caminho do dispositivo: o painel é uma
// interface de um aparelho composto, e saber qual é ajuda no diagnóstico.
func interfaceDoCaminho(path string) string {
	baixo := strings.ToLower(path)
	i := strings.Index(baixo, "mi_")
	if i < 0 {
		return ""
	}
	resto := baixo[i+3:]
	fim := 0
	for fim < len(resto) && isHex(resto[fim]) {
		fim++
	}
	return resto[:fim]
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')
}

// windowsTransport é um canal HID aberto com leitura e escrita sobrepostas.
type windowsTransport struct {
	h      windows.Handle
	outLen int // tamanho exato exigido pelo WriteFile (inclui o byte do Report ID)
	inLen  int
}

func (t *windowsTransport) PayloadSize() int {
	if t.outLen <= 1 {
		return 64
	}
	return t.outLen - 1 // desconta o byte do Report ID
}

func (t *windowsTransport) WriteReport(p []byte) error {
	buf := make([]byte, t.outLen)
	buf[0] = 0 // Report ID
	copy(buf[1:], p)
	return t.overlapped(func(ov *windows.Overlapped) error {
		return windows.WriteFile(t.h, buf, nil, ov)
	}, 2*time.Second, nil)
}

func (t *windowsTransport) ReadReport(timeout time.Duration) ([]byte, error) {
	buf := make([]byte, t.inLen)
	var lidos uint32
	err := t.overlapped(func(ov *windows.Overlapped) error {
		return windows.ReadFile(t.h, buf, nil, ov)
	}, timeout, &lidos)
	if err != nil {
		return nil, err
	}
	if int(lidos) > len(buf) {
		lidos = uint32(len(buf))
	}
	dados := buf[:lidos]
	// O primeiro byte é o Report ID; o conteúdo vem depois.
	if len(dados) > 1 {
		dados = dados[1:]
	}
	return dados, nil
}

// overlapped roda uma operação assíncrona com tempo limite e cancela se estourar.
func (t *windowsTransport) overlapped(op func(*windows.Overlapped) error, timeout time.Duration, n *uint32) error {
	ev, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(ev)
	ov := &windows.Overlapped{HEvent: ev}

	err = op(ov)
	if err != nil && err != windows.ERROR_IO_PENDING {
		return err
	}
	if err == windows.ERROR_IO_PENDING {
		ms := uint32(timeout / time.Millisecond)
		w, err := windows.WaitForSingleObject(ev, ms)
		if err != nil {
			return err
		}
		if w == uint32(windows.WAIT_TIMEOUT) {
			procCancelIoEx.Call(uintptr(t.h), uintptr(unsafe.Pointer(ov)))
			return ErrTimeout
		}
	}
	var lidos uint32
	if err := windows.GetOverlappedResult(t.h, ov, &lidos, false); err != nil {
		return err
	}
	if n != nil {
		*n = lidos
	}
	return nil
}

func (t *windowsTransport) Close() error { return windows.CloseHandle(t.h) }

// OpenPath abre um caminho de dispositivo HID específico.
func OpenPath(path string) (Transport, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		return nil, fmt.Errorf("abrindo %s: %w (o app do fabricante está aberto?)", path, err)
	}
	t := &windowsTransport{h: h, outLen: 65, inLen: 65}
	var preparsed uintptr
	if ret, _, _ := procHidDGetPreparsedData.Call(uintptr(h), uintptr(unsafe.Pointer(&preparsed))); ret != 0 && preparsed != 0 {
		defer procHidDFreePreparsedData.Call(preparsed)
		var caps hidpCaps
		procHidPGetCaps.Call(preparsed, uintptr(unsafe.Pointer(&caps)))
		if caps.outputReportByteLength > 0 {
			t.outLen = int(caps.outputReportByteLength)
		}
		if caps.inputReportByteLength > 0 {
			t.inLen = int(caps.inputReportByteLength)
		}
	}
	return t, nil
}
