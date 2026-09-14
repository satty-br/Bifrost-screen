//go:build windows

package acctelemetry

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// windows/x/sys não expõe OpenFileMapping (só CreateFileMapping), então
// chamamos a função do kernel32 diretamente — mesmo padrão usado em
// internal/rtss/rtss_windows.go e internal/ets2telemetry/reader_windows.go.
var (
	kernel32            = windows.NewLazySystemDLL("kernel32.dll")
	procOpenFileMapping = kernel32.NewProc("OpenFileMappingW")
)

type mmfReader struct{}

func openReader() (reader, error) { return mmfReader{}, nil }

func (mmfReader) read(name string, size int) ([]byte, error) {
	n, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	r, _, callErr := procOpenFileMapping.Call(uintptr(windows.FILE_MAP_READ), 0, uintptr(unsafe.Pointer(n)))
	if r == 0 {
		return nil, fmt.Errorf("mapeamento de memória %q não encontrado (jogo fechado?): %w", name, callErr)
	}
	h := windows.Handle(r)
	defer windows.CloseHandle(h)

	addr, err := windows.MapViewOfFile(h, windows.FILE_MAP_READ, 0, 0, uintptr(size))
	if err != nil {
		return nil, err
	}
	defer windows.UnmapViewOfFile(addr)

	buf := make([]byte, size)
	var read uintptr
	if err := windows.ReadProcessMemory(windows.CurrentProcess(), addr, &buf[0], uintptr(size), &read); err != nil {
		return nil, err
	}
	return buf, nil
}

func (mmfReader) close() {}
