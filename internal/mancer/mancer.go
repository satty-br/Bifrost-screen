// Package mancer envia a temperatura da CPU para o mostrador LCD embutido no
// bloco d'água Mancer Mystic G1 (e clones que usem o mesmo controlador HID
// genérico, VID 0xAA88 / PID 0x8666).
//
// Protocolo (documentado por engenharia reversa em
// https://github.com/dsmlucas/mancer-g1-cpu-temp-display): o mostrador é um
// dispositivo HID que aceita um relatório de saída de 1 byte com a
// temperatura em graus Celsius, como inteiro decimal simples (ex.: 45°C vira
// o byte 45/0x2D). Ele só mostra os 2 dígitos — não é uma tela de imagem, por
// isso não usa o mesmo driver/protocolo da tela Turing/UsbMonitor. Se o valor
// não for reenviado por mais de ~1s o mostrador volta a exibir "88".
package mancer

import (
	"context"
	"errors"
	"sync"
	"time"
)

// VendorID e ProductID identificam o controlador HID genérico usado pelo
// Mancer Mystic G1 (MCR-MTC360-BK01).
const (
	VendorID  uint16 = 0xAA88
	ProductID uint16 = 0x8666
)

// ErrNotFound é devolvido quando nenhum dispositivo com o VID/PID esperado é encontrado.
var ErrNotFound = errors.New("mostrador Mancer não encontrado")

// clampTemp limita a temperatura à faixa que o mostrador consegue exibir (0-99°C).
func clampTemp(celsius float64) byte {
	v := int(celsius + 0.5) // arredonda pro inteiro mais próximo
	if v < 0 {
		v = 0
	}
	if v > 99 {
		v = 99
	}
	return byte(v)
}

// device é implementado por cada plataforma: abre o HID pelo VID/PID e manda 1 byte.
type device interface {
	Write(temp byte) error
	Close() error
}

// openDevice acha e abre o mostrador pelo VID/PID; implementado por
// plataforma em hid_windows.go, hid_linux.go e hid_darwin.go.

// Monitor manda a temperatura da CPU pro mostrador periodicamente, reconectando sozinho.
type Monitor struct {
	mu        sync.RWMutex
	connected bool
	err       string
}

// NewMonitor cria um monitor parado; chame Start para ligar.
func NewMonitor() *Monitor { return &Monitor{} }

// Start manda temp() pro mostrador a cada interval (recomendado: 500ms, o
// próprio protocolo exige atualização frequente pra não voltar a exibir "88").
// Reconecta sozinho se o dispositivo for desconectado/reconectado.
func (m *Monitor) Start(ctx context.Context, interval time.Duration, temp func() float64) {
	go func() {
		var dev device
		defer func() {
			if dev != nil {
				dev.Close()
			}
		}()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			if dev == nil {
				d, err := openDevice()
				if err != nil {
					m.setStatus(false, err.Error())
					continue
				}
				dev = d
			}
			if err := dev.Write(clampTemp(temp())); err != nil {
				dev.Close()
				dev = nil
				m.setStatus(false, err.Error())
				continue
			}
			m.setStatus(true, "")
		}
	}()
}

func (m *Monitor) setStatus(connected bool, errMsg string) {
	m.mu.Lock()
	m.connected, m.err = connected, errMsg
	m.mu.Unlock()
}

// Connected diz se o último envio deu certo.
func (m *Monitor) Connected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connected
}

// LastError devolve o erro do último envio, se algum.
func (m *Monitor) LastError() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.err
}
