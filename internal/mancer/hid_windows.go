//go:build windows

package mancer

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Implementação sem CGO: acha o mostrador HID pelo VID/PID usando a mesma
// API que o Gerenciador de Dispositivos usa por baixo dos panos (SetupAPI +
// HidD_*, em setupapi.dll/hid.dll), e manda o byte com WriteFile.

var (
	modSetupapi = windows.NewLazySystemDLL("setupapi.dll")
	modHid      = windows.NewLazySystemDLL("hid.dll")

	procSetupDiGetClassDevsW             = modSetupapi.NewProc("SetupDiGetClassDevsW")
	procSetupDiEnumDeviceInterfaces      = modSetupapi.NewProc("SetupDiEnumDeviceInterfaces")
	procSetupDiGetDeviceInterfaceDetailW = modSetupapi.NewProc("SetupDiGetDeviceInterfaceDetailW")
	procSetupDiDestroyDeviceInfoList     = modSetupapi.NewProc("SetupDiDestroyDeviceInfoList")
	procHidDGetHidGuid                   = modHid.NewProc("HidD_GetHidGuid")
	procHidDGetAttributes                = modHid.NewProc("HidD_GetAttributes")
	procHidDGetPreparsedData             = modHid.NewProc("HidD_GetPreparsedData")
	procHidDFreePreparsedData            = modHid.NewProc("HidD_FreePreparsedData")
	procHidPGetCaps                      = modHid.NewProc("HidP_GetCaps")
)

const (
	digcfPresent         = 0x00000002
	digcfDeviceInterface = 0x00000010
	invalidHandleValue   = ^uintptr(0)
	// SP_DEVICE_INTERFACE_DETAIL_DATA.cbSize é sempre 8 em Windows 64-bit,
	// independente do sizeof real da struct em Go — peculiaridade documentada
	// da própria API do Windows (o valor certo pra 32-bit seria 6).
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

// hidpCaps espelha a struct HIDP_CAPS do Windows (hidpi.h). Só o campo
// OutputReportByteLength importa aqui: é o tamanho exato que o WriteFile
// exige pro relatório de saída (o primeiro byte é sempre o Report ID, 0
// quando o dispositivo não usa IDs de relatório).
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

// windowsDevice escreve no arquivo HID já aberto do mostrador.
type windowsDevice struct {
	h      windows.Handle
	outLen int // tamanho exato exigido pro relatório de saída (inclui o byte do Report ID)
}

func (d *windowsDevice) Write(temp byte) error {
	n := d.outLen
	if n < 1 {
		n = 1
	}
	buf := make([]byte, n)
	if n >= 2 {
		buf[0] = 0 // Report ID (o Mancer G1 não usa IDs de relatório)
		buf[1] = temp
	} else {
		buf[0] = temp
	}
	var written uint32
	return windows.WriteFile(d.h, buf, &written, nil)
}

func (d *windowsDevice) Close() error { return windows.CloseHandle(d.h) }

// outputReportLength descobre o tamanho exato que o WriteFile espera pro
// relatório de saída desse dispositivo HID (via HidD_GetPreparsedData +
// HidP_GetCaps). O Windows exige esse tamanho exato — diferente do Linux
// (hidraw) e do hidapi, que aceitam escrever só os bytes de dado puros.
func outputReportLength(h windows.Handle) int {
	var preparsed uintptr
	ret, _, _ := procHidDGetPreparsedData.Call(uintptr(h), uintptr(unsafe.Pointer(&preparsed)))
	if ret == 0 || preparsed == 0 {
		return 0
	}
	defer procHidDFreePreparsedData.Call(preparsed)

	var caps hidpCaps
	procHidPGetCaps.Call(preparsed, uintptr(unsafe.Pointer(&caps)))
	return int(caps.outputReportByteLength)
}

func hidGUID() guid {
	var g guid
	procHidDGetHidGuid.Call(uintptr(unsafe.Pointer(&g)))
	return g
}

// openDevice varre os dispositivos HID conectados procurando o VID/PID do
// Mancer Mystic G1, e abre o arquivo correspondente.
func openDevice() (device, error) {
	path, err := findDevicePath(VendorID, ProductID)
	if err != nil {
		return nil, err
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("abrindo %s: %w", path, err)
	}
	return &windowsDevice{h: h, outLen: outputReportLength(h)}, nil
}

func findDevicePath(vid, pid uint16) (string, error) {
	g := hidGUID()
	h, _, _ := procSetupDiGetClassDevsW.Call(uintptr(unsafe.Pointer(&g)), 0, 0, uintptr(digcfPresent|digcfDeviceInterface))
	if h == 0 || h == invalidHandleValue {
		return "", fmt.Errorf("SetupDiGetClassDevs falhou")
	}
	defer procSetupDiDestroyDeviceInfoList.Call(h)

	for idx := uint32(0); ; idx++ {
		var ifData spDeviceInterfaceData
		ifData.cbSize = uint32(unsafe.Sizeof(ifData))
		ret, _, _ := procSetupDiEnumDeviceInterfaces.Call(h, 0, uintptr(unsafe.Pointer(&g)), uintptr(idx), uintptr(unsafe.Pointer(&ifData)))
		if ret == 0 {
			break // sem mais dispositivos
		}

		var neededSize uint32
		procSetupDiGetDeviceInterfaceDetailW.Call(h, uintptr(unsafe.Pointer(&ifData)), 0, 0, uintptr(unsafe.Pointer(&neededSize)), 0)
		if neededSize == 0 {
			continue
		}
		buf := make([]byte, neededSize)
		*(*uint32)(unsafe.Pointer(&buf[0])) = deviceInterfaceDetailSize
		ret, _, _ = procSetupDiGetDeviceInterfaceDetailW.Call(h, uintptr(unsafe.Pointer(&ifData)),
			uintptr(unsafe.Pointer(&buf[0])), uintptr(neededSize), 0, 0)
		if ret == 0 {
			continue
		}
		// DevicePath (WCHAR[]) começa logo após o campo cbSize (4 bytes).
		path := windows.UTF16ToString(unsafe.Slice((*uint16)(unsafe.Pointer(&buf[4])), (len(buf)-4)/2))

		devPath, err := windows.UTF16PtrFromString(path)
		if err != nil {
			continue
		}
		hFile, err := windows.CreateFile(devPath, windows.GENERIC_READ|windows.GENERIC_WRITE,
			windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
		if err != nil {
			continue
		}
		var attrs hidAttributes
		attrs.size = uint32(unsafe.Sizeof(attrs))
		procHidDGetAttributes.Call(uintptr(hFile), uintptr(unsafe.Pointer(&attrs)))
		windows.CloseHandle(hFile)
		if attrs.vendorID == vid && attrs.productID == pid {
			return path, nil
		}
	}
	return "", ErrNotFound
}
