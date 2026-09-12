//go:build darwin

package lcd

import (
	"strings"

	"go.bug.st/serial"
)

// ListPorts lista as portas serial no macOS. Sem CGO não dá pra ler VID/PID
// (a enumeração detalhada da biblioteca usa IOKit via CGO), então a detecção
// da tela usa o nome do driver da CH340 (o chip usado pela Turing/UsbMonitor),
// que no macOS aparece como "/dev/cu.wchusbserial...".
func ListPorts() ([]PortInfo, error) {
	names, err := serial.GetPortsList()
	if err != nil {
		return nil, err
	}
	out := make([]PortInfo, 0, len(names))
	for _, name := range names {
		out = append(out, PortInfo{
			Name:     name,
			IsScreen: strings.Contains(strings.ToLower(name), "wchusbserial"),
		})
	}
	return out, nil
}
