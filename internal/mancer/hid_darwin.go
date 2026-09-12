//go:build darwin

package mancer

import "errors"

// No macOS, acesso a HID bruto exige o IOKit HID Manager (via CGO) — não há
// um jeito documentado de abrir e escrever num dispositivo HID sem CGO, ao
// contrário do Linux (/dev/hidraw) e do Windows (SetupAPI/HidD_*). Por isso o
// mostrador Mancer Mystic G1 fica indisponível no macOS por enquanto.
func openDevice() (device, error) {
	return nil, errors.New("mostrador Mancer: não suportado no macOS (precisaria de CGO/IOKit)")
}
