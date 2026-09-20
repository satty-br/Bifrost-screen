package kalkan

import (
	"errors"
	"fmt"
)

// ErrTimeout indica que o painel não respondeu no tempo esperado.
var ErrTimeout = errors.New("o painel não respondeu")

// ErrUnsupported é devolvido nos sistemas onde o acesso HID ainda não foi
// implementado neste pacote.
var ErrUnsupported = errors.New("acesso à tela Kalkan/GAMDIAS não implementado neste sistema")

// DeviceInfo é tudo que dá pra descobrir sobre uma interface HID da família
// sem abrir uma sessão com ela.
type DeviceInfo struct {
	Path             string // caminho do dispositivo no sistema
	Interface        string // índice "mi_XX" do aparelho composto, quando há
	VendorID         uint16
	ProductID        uint16
	Version          uint16
	UsagePage        uint16
	Usage            uint16
	InputReportLen   int // inclui o byte do Report ID
	OutputReportLen  int
	FeatureReportLen int
	Product          Product // preenchido quando o PID é conhecido
	KnownProduct     bool
}

// String resume o dispositivo numa linha.
func (d DeviceInfo) String() string {
	nome := "modelo desconhecido"
	if d.KnownProduct {
		nome = fmt.Sprintf("%s (%s) %dx%d", d.Product.Name, d.Product.OEM, d.Product.Width, d.Product.Height)
	}
	mi := d.Interface
	if mi == "" {
		mi = "--"
	}
	return fmt.Sprintf("VID_%04X&PID_%04X mi_%s  in=%d out=%d feat=%d  %s",
		d.VendorID, d.ProductID, mi, d.InputReportLen, d.OutputReportLen, d.FeatureReportLen, nome)
}

// Open acha o primeiro painel conhecido conectado, abre e faz o handshake.
func Open() (*Client, error) {
	devs, err := Devices()
	if err != nil {
		return nil, err
	}
	var escolhido *DeviceInfo
	for i := range devs {
		if devs[i].KnownProduct {
			escolhido = &devs[i]
			break
		}
	}
	if escolhido == nil {
		return nil, ErrNotFound
	}
	t, err := OpenPath(escolhido.Path)
	if err != nil {
		return nil, err
	}
	c := NewClient(t, escolhido.Product)
	if err := c.Conn(); err != nil {
		t.Close()
		return nil, err
	}
	return c, nil
}
