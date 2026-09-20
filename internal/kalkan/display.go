package kalkan

import (
	"fmt"
	"image"
	"sync"

	"github.com/satty-br/Bifrost-screen/internal/lcd"
)

// Display adapta o painel Kalkan/GAMDIAS à interface lcd.Display, pra ele
// entrar no mesmo maquinário das telas USB de 3,5": mesma rotação de telas,
// mesma prévia no painel, mesmo status na aba Conexão.
//
// Duas diferenças em relação à tela serial:
//
//   - não existe atualização parcial. O protocolo manda um JPEG inteiro por
//     quadro, então o retângulo sujo é ignorado (mas continua valendo a
//     verificação de "nada mudou" feita por quem chama, que evita o envio).
//   - a geometria vem do modelo detectado, não da configuração.
//
// EXPERIMENTAL: nada disso foi testado em hardware. Veja docs/kalkan-aura-lcd.md.
type Display struct {
	mu   sync.Mutex
	c    *Client
	info DeviceInfo
}

// NewDisplay cria o driver; a detecção acontece no Open.
func NewDisplay() *Display { return &Display{} }

// Present diz se existe um painel conhecido conectado agora. É uma varredura
// HID barata, usada pra decidir se o dispositivo aparece no painel.
func Present() bool {
	_, ok := Detect()
	return ok
}

// Detect devolve o modelo do primeiro painel conhecido plugado.
func Detect() (Product, bool) {
	devs, err := Devices()
	if err != nil {
		return Product{}, false
	}
	for _, d := range devs {
		if d.KnownProduct {
			return d.Product, true
		}
	}
	return Product{}, false
}

// Open acha o primeiro painel conhecido, abre e faz o handshake.
func (d *Display) Open() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.c != nil {
		return nil
	}
	devs, err := Devices()
	if err != nil {
		return fmt.Errorf("%w: %v", lcd.ErrNotFound, err)
	}
	var escolhido *DeviceInfo
	for i := range devs {
		if devs[i].KnownProduct {
			escolhido = &devs[i]
			break
		}
	}
	if escolhido == nil {
		return lcd.ErrNotFound
	}
	t, err := OpenPath(escolhido.Path)
	if err != nil {
		return err
	}
	c := NewClient(t, escolhido.Product)
	if err := c.Conn(); err != nil {
		t.Close()
		return err
	}
	d.c, d.info = c, *escolhido
	return nil
}

func (d *Display) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.c == nil {
		return nil
	}
	err := d.c.Close()
	d.c = nil
	return err
}

// PortName identifica o painel do jeito que o usuário vê no painel de
// controle — aqui não há porta COM, então mostramos o VID/PID.
func (d *Display) PortName() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.c == nil {
		return "USB HID"
	}
	return fmt.Sprintf("USB HID VID_%04X&PID_%04X", d.info.VendorID, d.info.ProductID)
}

// Size devolve a geometria nativa do modelo detectado. Antes do Open, o
// padrão é o do modelo mais comum da família.
func (d *Display) Size() (int, int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.c == nil {
		return Products[0].Width, Products[0].Height
	}
	p := d.c.Product()
	return p.Width, p.Height
}

func (d *Display) SetBrightness(percent int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.c == nil {
		return lcd.ErrNotFound
	}
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	return d.c.Brightness(percent)
}

// SetOrientation traduz a orientação do Bifrost pros graus que o painel espera.
func (d *Display) SetOrientation(o lcd.Orientation) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.c == nil {
		return lcd.ErrNotFound
	}
	return d.c.Rotate(grausDaOrientacao(o))
}

func grausDaOrientacao(o lcd.Orientation) int {
	switch o {
	case lcd.ReversePortrait:
		return 180
	case lcd.Landscape:
		return 90
	case lcd.ReverseLandscape:
		return 270
	}
	return 0
}

// Draw manda o quadro inteiro. O retângulo é ignorado: o protocolo não tem
// atualização parcial.
func (d *Display) Draw(img *image.RGBA, _ image.Rectangle) error {
	d.mu.Lock()
	c := d.c
	d.mu.Unlock()
	if c == nil {
		return lcd.ErrNotFound
	}
	return c.SendImage(img, jpegQuality)
}

// jpegQuality equilibra nitidez e tamanho do quadro: a 320x240 a diferença
// entre 85 e 95 é de poucos KB, e o gargalo aqui é o USB HID.
const jpegQuality = 88
