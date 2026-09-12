//go:build linux

package mancer

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// linuxDevice escreve direto no nó /dev/hidrawN do mostrador.
type linuxDevice struct {
	f *os.File
}

func (d *linuxDevice) Write(temp byte) error {
	_, err := d.f.Write([]byte{temp})
	return err
}

func (d *linuxDevice) Close() error { return d.f.Close() }

// openDevice varre /sys/class/hidraw procurando o nó cujo dispositivo USB
// tenha o VID/PID do Mancer Mystic G1, e abre /dev/hidrawN correspondente.
func openDevice() (device, error) {
	entries, err := os.ReadDir("/sys/class/hidraw")
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		name := e.Name() // "hidraw0", "hidraw1", ...
		vid, pid, err := hidrawUEvent(name)
		if err != nil {
			continue
		}
		if vid == VendorID && pid == ProductID {
			f, err := os.OpenFile(filepath.Join("/dev", name), os.O_RDWR, 0)
			if err != nil {
				return nil, fmt.Errorf("abrindo /dev/%s: %w", name, err)
			}
			return &linuxDevice{f: f}, nil
		}
	}
	return nil, ErrNotFound
}

// hidrawUEvent lê o VID/PID do dispositivo USB por trás de /sys/class/hidraw/<name>,
// a partir do arquivo "uevent" (contém uma linha "HID_ID=<bus>:<vid>:<pid>" em hexadecimal).
func hidrawUEvent(name string) (vid, pid uint16, err error) {
	data, err := os.ReadFile(filepath.Join("/sys/class/hidraw", name, "device", "uevent"))
	if err != nil {
		return 0, 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "HID_ID=") {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(line, "HID_ID="), ":")
		if len(parts) != 3 {
			continue
		}
		v, err1 := strconv.ParseUint(parts[1], 16, 16)
		p, err2 := strconv.ParseUint(parts[2], 16, 16)
		if err1 != nil || err2 != nil {
			continue
		}
		return uint16(v), uint16(p), nil
	}
	return 0, 0, fmt.Errorf("HID_ID não encontrado em %s/uevent", name)
}
