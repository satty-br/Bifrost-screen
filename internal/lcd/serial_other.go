//go:build !windows

package lcd

import (
	"fmt"
	"strings"
	"time"

	"go.bug.st/serial"
)

// unixPort adapta go.bug.st/serial (que já é multiplataforma) para a interface
// Port. Diferente do driver do Windows, esta biblioteca não expõe controle de
// fluxo por hardware (RTS/CTS); o protocolo já manda os dados em pedaços
// pequenos, então funciona sem isso, só um pouco mais devagar em portas lentas.
type unixPort struct {
	serial.Port
}

func (p unixPort) Flush() error { return p.ResetInputBuffer() }

// OpenSerial abre a porta com as mesmas opções da revisão A: 115200 8N1,
// com timeout de leitura de 1s.
func OpenSerial(name string) (Port, error) {
	mode := &serial.Mode{BaudRate: 115200, DataBits: 8, Parity: serial.NoParity, StopBits: serial.OneStopBit}
	p, err := serial.Open(name, mode)
	if err != nil {
		msg := strings.ToLower(err.Error())
		switch {
		case strings.Contains(msg, "busy") || strings.Contains(msg, "in use") || strings.Contains(msg, "permission"):
			return nil, fmt.Errorf("%s: %w", name, ErrPortBusy)
		case strings.Contains(msg, "no such file") || strings.Contains(msg, "not found") || strings.Contains(msg, "cannot find"):
			return nil, fmt.Errorf("%s: %w", name, ErrNotFound)
		}
		return nil, fmt.Errorf("abrindo %s: %w", name, err)
	}
	if err := p.SetReadTimeout(time.Second); err != nil {
		p.Close()
		return nil, fmt.Errorf("configurando %s: %w", name, err)
	}
	_ = p.ResetInputBuffer()
	_ = p.ResetOutputBuffer()
	return unixPort{p}, nil
}
