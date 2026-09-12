package lcd

import (
	"image"
	"image/color"
	"testing"
)

func TestParseOrientation(t *testing.T) {
	cases := map[string]Orientation{
		"retrato":            Portrait,
		"retrato_invertido":  ReversePortrait,
		"paisagem":           Landscape,
		"paisagem_invertida": ReverseLandscape,
		"":                   Portrait,
		"lixo":               Portrait,
	}
	for in, want := range cases {
		if got := ParseOrientation(in); got != want {
			t.Errorf("ParseOrientation(%q) = %v, esperava %v", in, got, want)
		}
	}
}

func TestIsLandscape(t *testing.T) {
	if Portrait.IsLandscape() || ReversePortrait.IsLandscape() {
		t.Error("retrato não é paisagem")
	}
	if !Landscape.IsLandscape() || !ReverseLandscape.IsLandscape() {
		t.Error("paisagem deveria ser paisagem")
	}
}

func TestSimulated(t *testing.T) {
	s := NewSimulated()
	if err := s.Open(); err != nil {
		t.Fatalf("Open() = %v", err)
	}
	defer s.Close()
	if s.PortName() != "simulada" {
		t.Errorf("PortName() = %q", s.PortName())
	}
	w, h := s.Size()
	if w != 320 || h != 480 {
		t.Errorf("Size() = %d,%d, esperava 320,480", w, h)
	}
	if err := s.SetOrientation(Landscape); err != nil {
		t.Fatal(err)
	}
	w, h = s.Size()
	if w != 480 || h != 320 {
		t.Errorf("Size() em paisagem = %d,%d, esperava 480,320", w, h)
	}
	if err := s.SetBrightness(50); err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	if err := s.Draw(img, img.Bounds()); err != nil {
		t.Errorf("Draw() não deveria falhar: %v", err)
	}
}

func TestDirtyRectEdgeCases(t *testing.T) {
	if got := DirtyRect(nil, nil); !got.Empty() {
		t.Errorf("nil,nil deveria ser vazio, veio %v", got)
	}
	a := image.NewRGBA(image.Rect(0, 0, 4, 4))
	c := image.NewRGBA(image.Rect(0, 0, 8, 8))
	if got := DirtyRect(a, c); got != c.Bounds() {
		t.Errorf("tamanhos diferentes deveriam devolver os bounds inteiros de b, veio %v", got)
	}
}

func TestRGB565LE(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.Set(0, 0, color.RGBA{255, 255, 255, 255})
	img.Set(1, 0, color.RGBA{0, 0, 0, 255})
	out := RGB565LE(img, img.Bounds())
	if len(out) != 4 {
		t.Fatalf("esperava 4 bytes (2 pixels x 2 bytes), veio %d", len(out))
	}
	white := uint16(out[0]) | uint16(out[1])<<8
	if white != 0xFFFF {
		t.Errorf("branco deveria virar 0xFFFF, veio %#04x", white)
	}
	black := uint16(out[2]) | uint16(out[3])<<8
	if black != 0x0000 {
		t.Errorf("preto deveria virar 0x0000, veio %#04x", black)
	}
}

func TestListPortsAndDetectRevADoNotPanic(t *testing.T) {
	// Resultado depende do hardware da máquina; só confere que roda sem travar.
	if _, err := ListPorts(); err != nil {
		t.Logf("ListPorts() erro (aceitável neste ambiente): %v", err)
	}
	if _, err := DetectRevA(); err != nil {
		t.Logf("DetectRevA() erro (aceitável neste ambiente): %v", err)
	}
}
