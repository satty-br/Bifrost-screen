// Package lcd conversa com a tela USB Turing/UsbMonitor 3.5".
//
// O protocolo da revisão A foi portado da biblioteca turing-smart-screen-python
// (GPL-3.0, Copyright (C) 2021 Matthieu Houdebine) — ver THIRD_PARTY_NOTICES.md.
package lcd

import (
	"errors"
	"image"
)

// Orientation segue a numeração usada pelo firmware da tela.
type Orientation int

const (
	Portrait         Orientation = 0
	ReversePortrait  Orientation = 1
	Landscape        Orientation = 2
	ReverseLandscape Orientation = 3
)

// ParseOrientation converte o texto da configuração.
func ParseOrientation(s string) Orientation {
	switch s {
	case "retrato_invertido":
		return ReversePortrait
	case "paisagem":
		return Landscape
	case "paisagem_invertida":
		return ReverseLandscape
	}
	return Portrait
}

func (o Orientation) IsLandscape() bool { return o == Landscape || o == ReverseLandscape }

// ErrPortBusy indica que outro programa está com a porta COM aberta.
var ErrPortBusy = errors.New("porta em uso por outro programa")

// ErrNotFound indica que a tela não foi encontrada nas portas COM.
var ErrNotFound = errors.New("tela não encontrada")

// Display é qualquer coisa capaz de mostrar os frames (tela real ou simulada).
type Display interface {
	Open() error
	Close() error
	PortName() string
	Size() (w, h int)
	SetBrightness(percent int) error
	SetOrientation(o Orientation) error
	// Draw envia a região r da imagem img (coordenadas da própria tela).
	Draw(img *image.RGBA, r image.Rectangle) error
}

// Port é a porta serial usada pelo driver.
type Port interface {
	Read(p []byte) (int, error)
	Write(p []byte) (int, error)
	Close() error
	Flush() error
}

// Simulated é uma tela "de mentira" para testar sem hardware.
type Simulated struct {
	w, h   int
	orient Orientation
}

func NewSimulated() *Simulated { return &Simulated{w: 320, h: 480} }

// NewSimulatedSize cria uma tela de mentira num tamanho arbitrário, usado
// pra testar o layout de telas em resoluções que o Bifrost ainda não sabe
// falar com hardware de verdade (ex.: painéis mais largos tipo 1280x800 ou
// 1920x480). Valores <= 0 caem no tamanho padrão (320x480).
func NewSimulatedSize(w, h int) *Simulated {
	if w <= 0 || h <= 0 {
		return NewSimulated()
	}
	return &Simulated{w: w, h: h}
}

func (s *Simulated) Open() error      { return nil }
func (s *Simulated) Close() error     { return nil }
func (s *Simulated) PortName() string { return "simulada" }
func (s *Simulated) Size() (int, int) {
	if s.orient.IsLandscape() {
		return s.h, s.w
	}
	return s.w, s.h
}
func (s *Simulated) SetBrightness(int) error { return nil }
func (s *Simulated) SetOrientation(o Orientation) error {
	s.orient = o
	return nil
}
func (s *Simulated) Draw(*image.RGBA, image.Rectangle) error { return nil }

// DirtyRect devolve o menor retângulo com pixels diferentes entre a e b
// (ou vazio se forem iguais). As duas imagens precisam ter o mesmo tamanho.
func DirtyRect(a, b *image.RGBA) image.Rectangle {
	if a == nil || b == nil || a.Bounds() != b.Bounds() {
		if b == nil {
			return image.Rectangle{}
		}
		return b.Bounds()
	}
	bounds := b.Bounds()
	minX, minY, maxX, maxY := bounds.Max.X, bounds.Max.Y, -1, -1
	w := bounds.Dx()
	for y := 0; y < bounds.Dy(); y++ {
		ra := a.Pix[y*a.Stride : y*a.Stride+w*4]
		rb := b.Pix[y*b.Stride : y*b.Stride+w*4]
		if string(ra) == string(rb) {
			continue
		}
		if y < minY {
			minY = y
		}
		maxY = y
		for x := 0; x < w; x++ {
			i := x * 4
			if ra[i] != rb[i] || ra[i+1] != rb[i+1] || ra[i+2] != rb[i+2] {
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
			}
		}
	}
	if maxY < 0 {
		return image.Rectangle{}
	}
	return image.Rect(minX, minY, maxX+1, maxY+1).Add(bounds.Min)
}

// RGB565LE converte a região r de img em RGB565 little-endian (formato da tela).
func RGB565LE(img *image.RGBA, r image.Rectangle) []byte {
	out := make([]byte, 0, r.Dx()*r.Dy()*2)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		off := img.PixOffset(r.Min.X, y)
		row := img.Pix[off : off+r.Dx()*4]
		for x := 0; x < len(row); x += 4 {
			v := uint16(row[x]>>3)<<11 | uint16(row[x+1]>>2)<<5 | uint16(row[x+2]>>3)
			out = append(out, byte(v), byte(v>>8))
		}
	}
	return out
}
