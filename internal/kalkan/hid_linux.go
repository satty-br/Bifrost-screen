//go:build linux

package kalkan

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// No Linux o painel aparece como um nó /dev/hidrawN. Se der erro de
// permissão, é preciso uma regra de udev liberando o dispositivo pro usuário
// (VID 1b80), igual ao que o mostrador Mancer pede.

// Devices varre /sys/class/hidraw procurando interfaces do VID da família.
func Devices() ([]DeviceInfo, error) { return enumerar(VendorID) }

// AllDevices lista todos os nós hidraw. Serve pro diagnóstico.
func AllDevices() ([]DeviceInfo, error) { return enumerar(0) }

func enumerar(vid uint16) ([]DeviceInfo, error) {
	entradas, err := os.ReadDir("/sys/class/hidraw")
	if err != nil {
		return nil, err
	}
	var achados []DeviceInfo
	for _, e := range entradas {
		nome := e.Name() // "hidraw0", "hidraw1", ...
		v, pid, err := lerUEvent(nome)
		if err != nil {
			continue
		}
		if vid != 0 && v != vid {
			continue
		}
		info := DeviceInfo{
			Path:      filepath.Join("/dev", nome),
			VendorID:  v,
			ProductID: pid,
			// hidraw não expõe os tamanhos de relatório sem parsear o
			// descritor; 64 é o tamanho desta família.
			InputReportLen:  65,
			OutputReportLen: 65,
		}
		info.Product, info.KnownProduct = Lookup(pid)
		achados = append(achados, info)
	}
	return achados, nil
}

// lerUEvent tira VID e PID do uevent do nó hidraw. A linha tem o formato
// "HID_ID=0003:00001B80:0000B550".
func lerUEvent(nome string) (uint16, uint16, error) {
	b, err := os.ReadFile(filepath.Join("/sys/class/hidraw", nome, "device", "uevent"))
	if err != nil {
		return 0, 0, err
	}
	for _, linha := range strings.Split(string(b), "\n") {
		if !strings.HasPrefix(linha, "HID_ID=") {
			continue
		}
		partes := strings.Split(strings.TrimPrefix(linha, "HID_ID="), ":")
		if len(partes) != 3 {
			return 0, 0, fmt.Errorf("HID_ID inesperado: %q", linha)
		}
		vid, err1 := strconv.ParseUint(partes[1], 16, 16)
		pid, err2 := strconv.ParseUint(partes[2], 16, 16)
		if err1 != nil || err2 != nil {
			return 0, 0, fmt.Errorf("HID_ID ilegível: %q", linha)
		}
		return uint16(vid), uint16(pid), nil
	}
	return 0, 0, fmt.Errorf("sem HID_ID em %s", nome)
}

type linuxTransport struct {
	f *os.File
}

func (t *linuxTransport) PayloadSize() int { return 64 }

func (t *linuxTransport) WriteReport(p []byte) error {
	// hidraw espera o Report ID como primeiro byte; 0 quando o dispositivo
	// não usa relatórios numerados.
	buf := make([]byte, len(p)+1)
	copy(buf[1:], p)
	_, err := t.f.Write(buf)
	return err
}

func (t *linuxTransport) ReadReport(timeout time.Duration) ([]byte, error) {
	fds := []unix.PollFd{{Fd: int32(t.f.Fd()), Events: unix.POLLIN}}
	ms := int(timeout / time.Millisecond)
	n, err := unix.Poll(fds, ms)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, ErrTimeout
	}
	buf := make([]byte, 65)
	lidos, err := t.f.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:lidos], nil
}

func (t *linuxTransport) Close() error { return t.f.Close() }

// OpenPath abre um nó /dev/hidrawN.
func OpenPath(path string) (Transport, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("abrindo %s: %w (falta uma regra de udev?)", path, err)
	}
	return &linuxTransport{f: f}, nil
}
