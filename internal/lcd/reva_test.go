package lcd

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"os"
	"testing"
)

// recPort grava tudo que o driver escreve.
type recPort struct{ buf bytes.Buffer }

func (p *recPort) Read(b []byte) (int, error)  { return 0, nil }
func (p *recPort) Write(b []byte) (int, error) { return p.buf.Write(b) }
func (p *recPort) Close() error                { return nil }
func (p *recPort) Flush() error                { return nil }

func newTestDev() (*RevA, *recPort) {
	p := &recPort{}
	d := NewRevA("COM9", nil, nil)
	d.port, d.portName = p, "COM9"
	return d, p
}

// Compara byte a byte com a biblioteca Python original (testdata/python_reva.json,
// gerado com a turing-smart-screen-python usando as mesmas entradas).
func TestMatchesPythonDriver(t *testing.T) {
	raw, err := os.ReadFile("testdata/python_reva.json")
	if err != nil {
		t.Fatal(err)
	}
	var ref map[string]string
	if err := json.Unmarshal(raw, &ref); err != nil {
		t.Fatal(err)
	}
	check := func(name string, got []byte) {
		t.Helper()
		if want := ref[name]; hex.EncodeToString(got) != want {
			t.Errorf("%s diferente do Python:\n go: %x\n py: %s", name, got[:min(len(got), 32)], want[:min(len(want), 64)])
		}
	}

	d, p := newTestDev()
	d.SetBrightness(20)
	check("brightness20", p.buf.Bytes())

	d, p = newTestDev()
	d.SetBrightness(100)
	check("brightness100", p.buf.Bytes())

	d, p = newTestDev()
	d.SetOrientation(Landscape)
	check("orient_landscape", p.buf.Bytes())

	d, p = newTestDev()
	d.SetOrientation(ReversePortrait)
	check("orient_rportrait", p.buf.Bytes())

	// mesma imagem 100x60 do script Python, em (37,211)
	frame := image.NewRGBA(image.Rect(0, 0, 320, 480))
	for y := 0; y < 60; y++ {
		for x := 0; x < 100; x++ {
			frame.Set(37+x, 211+y, color.RGBA{uint8((x * 7) % 256), uint8((y * 11) % 256), uint8(((x + y) * 5) % 256), 255})
		}
	}
	d, p = newTestDev()
	if err := d.Draw(frame, image.Rect(37, 211, 137, 271)); err != nil {
		t.Fatal(err)
	}
	check("bitmap", p.buf.Bytes())
}

func TestDirtyRect(t *testing.T) {
	a := image.NewRGBA(image.Rect(0, 0, 320, 480))
	b := image.NewRGBA(image.Rect(0, 0, 320, 480))
	if r := DirtyRect(a, b); !r.Empty() {
		t.Fatalf("iguais deveriam dar vazio, deu %v", r)
	}
	b.Set(10, 20, color.RGBA{255, 0, 0, 255})
	b.Set(200, 300, color.RGBA{0, 255, 0, 255})
	if r := DirtyRect(a, b); r != image.Rect(10, 20, 201, 301) {
		t.Fatalf("retângulo errado: %v", r)
	}
	if r := DirtyRect(nil, b); r != b.Bounds() {
		t.Fatalf("sem frame anterior deveria mandar tudo: %v", r)
	}
}
