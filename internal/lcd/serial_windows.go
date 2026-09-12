//go:build windows

package lcd

import (
	"errors"
	"fmt"
	"strings"
	"unsafe"

	"go.bug.st/serial/enumerator"
	"golang.org/x/sys/windows"
)

// Identificação USB das telas revisão A.
const (
	revAVID    = "1A86"
	revAPID    = "5722"
	revASerial = "USB35INCHIPSV2"
)

type winPort struct {
	h windows.Handle
}

// Bits do campo Flags da estrutura DCB.
const (
	dcbBinary       = 0x0001
	dcbOutxCtsFlow  = 0x0004
	dcbDtrControlOn = 0x0010
	dcbRtsHandshake = 0x2000
	purgeRxClear    = 0x0008
	purgeTxClear    = 0x0004
)

// OpenSerial abre a porta com as mesmas opções que o pyserial usa na biblioteca
// original: 115200 8N1, RTS/CTS, timeout de leitura de 1s.
func OpenSerial(name string) (Port, error) {
	path := `\\.\` + name
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
			return nil, fmt.Errorf("%s: %w", name, ErrPortBusy)
		}
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
			return nil, fmt.Errorf("%s: %w", name, ErrNotFound)
		}
		return nil, fmt.Errorf("abrindo %s: %w", name, err)
	}
	_ = windows.SetupComm(h, 65536, 65536)

	var dcb windows.DCB
	dcb.DCBlength = uint32(unsafe.Sizeof(dcb))
	if err := windows.GetCommState(h, &dcb); err != nil {
		windows.CloseHandle(h)
		return nil, fmt.Errorf("lendo estado de %s: %w", name, err)
	}
	dcb.BaudRate = 115200
	dcb.ByteSize = 8
	dcb.Parity = windows.NOPARITY
	dcb.StopBits = windows.ONESTOPBIT
	dcb.Flags = dcbBinary | dcbOutxCtsFlow | dcbDtrControlOn | dcbRtsHandshake
	if err := windows.SetCommState(h, &dcb); err != nil {
		windows.CloseHandle(h)
		return nil, fmt.Errorf("configurando %s: %w", name, err)
	}
	timeouts := windows.CommTimeouts{
		ReadIntervalTimeout:        0,
		ReadTotalTimeoutMultiplier: 0,
		ReadTotalTimeoutConstant:   1000,
		WriteTotalTimeoutConstant:  5000,
	}
	if err := windows.SetCommTimeouts(h, &timeouts); err != nil {
		windows.CloseHandle(h)
		return nil, fmt.Errorf("timeouts de %s: %w", name, err)
	}
	_ = windows.PurgeComm(h, purgeRxClear|purgeTxClear)
	return &winPort{h: h}, nil
}

func (p *winPort) Read(b []byte) (int, error) {
	var n uint32
	err := windows.ReadFile(p.h, b, &n, nil)
	return int(n), err
}

func (p *winPort) Write(b []byte) (int, error) {
	written := 0
	for written < len(b) {
		var n uint32
		if err := windows.WriteFile(p.h, b[written:], &n, nil); err != nil {
			return written, err
		}
		if n == 0 {
			return written, errors.New("tempo esgotado escrevendo na tela")
		}
		written += int(n)
	}
	return written, nil
}

func (p *winPort) Flush() error { return windows.PurgeComm(p.h, purgeRxClear) }

func (p *winPort) Close() error { return windows.CloseHandle(p.h) }

// PortInfo descreve uma porta COM encontrada no sistema.
type PortInfo struct {
	Name     string `json:"nome"`
	VIDPID   string `json:"vid_pid"`
	Serial   string `json:"serial"`
	IsScreen bool   `json:"e_a_tela"`
}

// ListPorts lista as portas COM e marca a que parece ser a tela.
func ListPorts() ([]PortInfo, error) {
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		return nil, err
	}
	var out []PortInfo
	for _, p := range ports {
		info := PortInfo{Name: p.Name, Serial: p.SerialNumber}
		if p.IsUSB {
			info.VIDPID = strings.ToUpper(p.VID + ":" + p.PID)
			info.IsScreen = strings.EqualFold(p.SerialNumber, revASerial) ||
				(strings.EqualFold(p.VID, revAVID) && strings.EqualFold(p.PID, revAPID))
		}
		out = append(out, info)
	}
	return out, nil
}

// DetectRevA encontra a porta da tela revisão A.
func DetectRevA() (string, error) {
	ports, err := ListPorts()
	if err != nil {
		return "", err
	}
	for _, p := range ports {
		if p.IsScreen {
			return p.Name, nil
		}
	}
	return "", ErrNotFound
}
