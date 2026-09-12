//go:build windows || linux

package lcd

import (
	"strings"

	"go.bug.st/serial/enumerator"
)

// ListPorts lista as portas serial e marca a que parece ser a tela.
// No Windows e no Linux dá pra pegar VID/PID/número de série sem CGO.
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
