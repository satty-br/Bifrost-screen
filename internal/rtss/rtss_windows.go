//go:build windows

package rtss

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Layout de RTSS_SHARED_MEMORY (RTSSSharedMemoryV2), documentado no SDK do
// RivaTuner Statistics Server — é o mesmo formato que MSI Afterburner, Special
// K, RTSS Free e vários overlays de terceiros leem pra mostrar o FPS de outro
// programa sem precisar de nenhuma API do jogo.
type header struct {
	Signature    uint32 // 'RTSS' (0x53535452 em little-endian)
	Version      uint32
	AppEntrySize uint32
	AppArrOffset uint32
	AppArrSize   uint32
	OSDEntrySize uint32
	OSDArrOffset uint32
	OSDArrSize   uint32
	OSDFrame     uint32
}

const maxPath = 260

// appEntry é o começo de RTSS_SHARED_MEMORY_APP_ENTRY — só os campos usados
// aqui (o struct completo tem mais estatísticas em versões novas do RTSS,
// mas AppEntrySize diz o tamanho real de cada item, então pulamos por ele).
type appEntry struct {
	ProcessID uint32
	Name      [maxPath]byte
	Flags     uint32
	Time0     uint32
	Time1     uint32
	Frames    uint32
	FrameTime uint32 // média móvel do tempo de quadro, em décimos de milissegundo
}

var kernel32 = windows.NewLazySystemDLL("kernel32.dll")
var procOpenFileMappingW = kernel32.NewProc("OpenFileMappingW")

func openSharedMemory(name string) ([]byte, error) {
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
		return nil, errors.New("região de memória compartilhada do RTSS com tamanho inválido")
	}
	buf := make([]byte, size)
	var read uintptr
	if err := windows.ReadProcessMemory(windows.CurrentProcess(), addr, &buf[0], uintptr(size), &read); err != nil {
		return nil, err
	}
	return buf[:read], nil
}

var errNaoEncontrado = errors.New("RTSS não está rodando (ou nenhum jogo sendo monitorado por ele)")

// Ler devolve o FPS do jogo mais ativo que o RTSS está monitorando — o que
// tiver mais quadros contados, já que normalmente só tem um jogo rodando por
// vez. Precisa só do RTSS aberto (não precisa do Afterburner).
func Ler() (Leitura, error) {
	buf, err := openSharedMemory("RTSSSharedMemoryV2")
	if err != nil {
		return Leitura{}, errNaoEncontrado
	}
	if len(buf) < int(unsafe.Sizeof(header{})) {
		return Leitura{}, errNaoEncontrado
	}
	h := (*header)(unsafe.Pointer(&buf[0]))
	if h.Signature != 0x53535452 { // 'RTSS' em little-endian ("RTSS" ASCII lido como uint32 LE)
		return Leitura{}, errNaoEncontrado
	}
	entrySize := int(h.AppEntrySize)
	if entrySize <= 0 {
		return Leitura{}, errNaoEncontrado
	}
	best := Leitura{}
	var bestFrames uint32
	for i := uint32(0); i < h.AppArrSize; i++ {
		off := int(h.AppArrOffset) + int(i)*entrySize
		if off+entrySize > len(buf) || off+int(unsafe.Sizeof(appEntry{})) > len(buf) {
			break
		}
		e := (*appEntry)(unsafe.Pointer(&buf[off]))
		if e.ProcessID == 0 || e.Frames == 0 {
			continue
		}
		if e.Frames <= bestFrames {
			continue
		}
		fps := 0.0
		if e.FrameTime > 0 {
			fps = 10000.0 / float64(e.FrameTime)
		}
		if fps <= 0 {
			continue
		}
		bestFrames = e.Frames
		best = Leitura{Processo: cString(e.Name[:]), FPS: fps}
	}
	if bestFrames == 0 {
		return Leitura{}, errNaoEncontrado
	}
	return best, nil
}

func cString(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}
