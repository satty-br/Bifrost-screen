//go:build windows

package ets2telemetry

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// windows/x/sys não expõe OpenFileMapping (só CreateFileMapping), então
// chamamos a função do kernel32 diretamente — mesmo padrão usado em
// internal/rtss/rtss_windows.go pra ler a memória compartilhada do RTSS.
var (
	kernel32            = windows.NewLazySystemDLL("kernel32.dll")
	procOpenFileMapping = kernel32.NewProc("OpenFileMappingW")
)

// mmfReader lê a memória compartilhada exposta pelo plugin SCS Telemetry
// (é só leitura — o Bifrost nunca escreve nessa área).
type mmfReader struct {
	handle windows.Handle
	addr   uintptr
}

func openReader() (reader, error) {
	name, err := windows.UTF16PtrFromString(MMFName)
	if err != nil {
		return nil, err
	}
	r, _, callErr := procOpenFileMapping.Call(uintptr(windows.FILE_MAP_READ), 0, uintptr(unsafe.Pointer(name)))
	if r == 0 {
		return nil, fmt.Errorf("memória do SCS Telemetry não encontrada (plugin não instalado, ou ETS2/ATS fechado): %w", callErr)
	}
	h := windows.Handle(r)
	addr, err := windows.MapViewOfFile(h, windows.FILE_MAP_READ, 0, 0, MMFSize)
	if err != nil {
		_ = windows.CloseHandle(h)
		return nil, err
	}
	return &mmfReader{handle: h, addr: addr}, nil
}

func (r *mmfReader) read() ([]byte, error) {
	buf := make([]byte, MMFSize)
	var read uintptr
	if err := windows.ReadProcessMemory(windows.CurrentProcess(), r.addr, &buf[0], MMFSize, &read); err != nil {
		return nil, err
	}
	return buf, nil
}

func (r *mmfReader) close() {
	_ = windows.UnmapViewOfFile(r.addr)
	_ = windows.CloseHandle(r.handle)
}
