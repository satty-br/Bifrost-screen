package lcd

import (
	"fmt"
	"image"
	"log"
	"sync"
	"time"
)

// Comandos da revisão A (Turing Smart Screen 3.5" / UsbMonitor).
const (
	cmdReset          = 101
	cmdClear          = 102
	cmdScreenOff      = 108
	cmdScreenOn       = 109
	cmdSetBrightness  = 110
	cmdSetOrientation = 121
	cmdDisplayBitmap  = 197
	cmdHello          = 69
)

// Opener abre a porta serial com o nome dado ("COM3").
type Opener func(name string) (Port, error)

// Detector encontra a porta da tela quando a configuração está em "AUTO".
type Detector func() (string, error)

// RevA implementa o protocolo da revisão A.
type RevA struct {
	mu       sync.Mutex
	portCfg  string
	open     Opener
	detect   Detector
	port     Port
	portName string
	width    int // largura em retrato
	height   int // altura em retrato
	orient   Orientation
	subRev   string
}

func NewRevA(portCfg string, open Opener, detect Detector) *RevA {
	return &RevA{portCfg: portCfg, open: open, detect: detect, width: 320, height: 480}
}

func (d *RevA) PortName() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.portName
}

func (d *RevA) SubRevision() string { return d.subRev }

func (d *RevA) Size() (int, int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.sizeLocked()
}

func (d *RevA) sizeLocked() (int, int) {
	if d.orient.IsLandscape() {
		return d.height, d.width
	}
	return d.width, d.height
}

func (d *RevA) resolvePort() (string, error) {
	if d.portCfg != "" && d.portCfg != "AUTO" {
		return d.portCfg, nil
	}
	return d.detect()
}

func (d *RevA) openPort() error {
	name, err := d.resolvePort()
	if err != nil {
		return err
	}
	p, err := d.open(name)
	if err != nil {
		return err
	}
	d.port = p
	d.portName = name
	return nil
}

// Open conecta, reinicia a tela (como o app original faz) e identifica o modelo.
func (d *RevA) Open() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.openPort(); err != nil {
		return err
	}
	// Reset: a tela reinicia e pode reaparecer com outro nome de porta.
	_ = d.sendCommand(cmdReset, 0, 0, 0, 0)
	_ = d.port.Close()
	d.port = nil
	time.Sleep(5 * time.Second)
	var err error
	for attempt := 0; attempt < 10; attempt++ {
		if err = d.openPort(); err == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if err != nil {
		return fmt.Errorf("reabrindo a porta depois do reset: %w", err)
	}
	d.hello()
	return nil
}

func (d *RevA) hello() {
	b := []byte{cmdHello, cmdHello, cmdHello, cmdHello, cmdHello, cmdHello}
	if _, err := d.port.Write(b); err != nil {
		return
	}
	resp := make([]byte, 6)
	n, _ := readFull(d.port, resp, time.Second)
	_ = d.port.Flush()
	d.subRev = "Turing 3.5\""
	if n == 6 {
		switch resp[0] {
		case 0x01:
			d.subRev = "UsbMonitor 3.5\""
		case 0x02:
			d.subRev, d.width, d.height = "UsbMonitor 5\"", 480, 800
		case 0x03:
			d.subRev, d.width, d.height = "UsbMonitor 7\"", 600, 1024
		}
	}
	log.Printf("tela identificada: %s (%dx%d) em %s", d.subRev, d.width, d.height, d.portName)
}

func readFull(p Port, buf []byte, timeout time.Duration) (int, error) {
	deadline := time.Now().Add(timeout)
	got := 0
	for got < len(buf) && time.Now().Before(deadline) {
		n, err := p.Read(buf[got:])
		got += n
		if err != nil {
			return got, err
		}
		if n == 0 {
			break // a leitura já espera até o timeout configurado na porta
		}
	}
	return got, nil
}

func (d *RevA) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.port == nil {
		return nil
	}
	err := d.port.Close()
	d.port = nil
	return err
}

func (d *RevA) sendCommand(cmd byte, x, y, ex, ey int) error {
	b := [6]byte{
		byte(x >> 2),
		byte(((x & 3) << 6) + (y >> 4)),
		byte(((y & 15) << 4) + (ex >> 6)),
		byte(((ex & 63) << 2) + (ey >> 8)),
		byte(ey & 255),
		cmd,
	}
	_, err := d.port.Write(b[:])
	return err
}

func (d *RevA) SetBrightness(percent int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.port == nil {
		return ErrNotFound
	}
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	// A tela vai de 0 (mais claro) a 255 (mais escuro).
	level := 255 - percent*255/100
	return d.sendCommand(cmdSetBrightness, level, 0, 0, 0)
}

func (d *RevA) SetOrientation(o Orientation) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.port == nil {
		return ErrNotFound
	}
	d.orient = o
	w, h := d.sizeLocked()
	b := make([]byte, 16)
	b[5] = cmdSetOrientation
	b[6] = byte(int(o) + 100)
	b[7] = byte(w >> 8)
	b[8] = byte(w & 255)
	b[9] = byte(h >> 8)
	b[10] = byte(h & 255)
	_, err := d.port.Write(b)
	return err
}

// Draw envia a região r (em coordenadas da tela) da imagem.
func (d *RevA) Draw(img *image.RGBA, r image.Rectangle) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.port == nil {
		return ErrNotFound
	}
	w, h := d.sizeLocked()
	r = r.Intersect(image.Rect(0, 0, w, h))
	if r.Empty() {
		return nil
	}
	data := RGB565LE(img, r)
	if err := d.sendCommand(cmdDisplayBitmap, r.Min.X, r.Min.Y, r.Max.X-1, r.Max.Y-1); err != nil {
		return err
	}
	chunk := w * 8
	for i := 0; i < len(data); i += chunk {
		end := i + chunk
		if end > len(data) {
			end = len(data)
		}
		if _, err := d.port.Write(data[i:end]); err != nil {
			return err
		}
	}
	return nil
}
