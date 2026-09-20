package kalkan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"time"
)

// Transport é o canal HID já aberto com o painel. Cada Write manda um
// relatório de saída inteiro (sem o byte de Report ID — quem cuida disso é a
// implementação por plataforma).
type Transport interface {
	// WriteReport manda um relatório de saída. O slice tem exatamente
	// PayloadSize() bytes.
	WriteReport(p []byte) error
	// ReadReport lê um relatório de entrada, esperando no máximo timeout.
	ReadReport(timeout time.Duration) ([]byte, error)
	// PayloadSize é quantos bytes de dado cabem num relatório (64 nesta família).
	PayloadSize() int
	Close() error
}

// DefaultTimeout é quanto esperamos por uma resposta do painel.
const DefaultTimeout = 2 * time.Second

// Client fala o protocolo do painel por cima de um Transport.
type Client struct {
	t       Transport
	produto Product
	seq     uint32
	Timeout time.Duration
}

// NewClient embrulha um Transport já aberto.
func NewClient(t Transport, p Product) *Client {
	return &Client{t: t, produto: p, Timeout: DefaultTimeout}
}

// Product devolve o modelo com que este cliente foi aberto.
func (c *Client) Product() Product { return c.produto }

// Close fecha o canal.
func (c *Client) Close() error { return c.t.Close() }

func (c *Client) proximoSeq() uint32 {
	c.seq++
	return c.seq
}

// Send manda uma requisição e espera o código de status.
func (c *Client) Send(r Request) (Status, error) {
	if r.Seq == 0 && r.Field != FieldConn {
		r.Seq = c.proximoSeq()
	}
	if err := c.sendRaw(r.Encode()); err != nil {
		return 0, err
	}
	return c.readStatus()
}

// sendRaw fatia e manda uma mensagem já codificada.
func (c *Client) sendRaw(msg []byte) error {
	for _, parte := range Chunk(msg, c.t.PayloadSize()) {
		if err := c.t.WriteReport(parte); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) readStatus() (Status, error) {
	to := c.Timeout
	if to <= 0 {
		to = DefaultTimeout
	}
	resp, err := c.t.ReadReport(to)
	if err != nil {
		return 0, err
	}
	return ParseStatus(resp)
}

// Conn faz o handshake inicial. É a primeira coisa a mandar depois de abrir
// o aparelho.
func (c *Client) Conn() error {
	st, err := c.Send(Request{Verb: VerbPost, Field: FieldConn})
	return conferir(FieldConn, st, err)
}

// Post manda um POST simples com corpo JSON opcional.
func (c *Client) Post(field string, value int, body any) error {
	var corpo []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		corpo = b
	}
	st, err := c.Send(Request{Verb: VerbPost, Field: field, Value: value, Body: corpo})
	return conferir(field, st, err)
}

// Brightness ajusta o brilho do painel (0-100).
func (c *Client) Brightness(pct int) error { return c.Post(FieldBrightness, pct, nil) }

// Rotate gira a imagem (0, 90, 180, 270 graus).
func (c *Client) Rotate(graus int) error { return c.Post(FieldRotate, graus, nil) }

// Power liga (1) ou desliga (0) o painel.
func (c *Client) Power(on bool) error {
	v := 0
	if on {
		v = 1
	}
	return c.Post(FieldPower, v, nil)
}

// Heartbeat mantém a conexão viva. O software do fabricante manda isso
// periodicamente; sem ele o painel pode voltar ao tema interno.
func (c *Client) Heartbeat() error {
	st, err := c.Send(Request{Verb: VerbState, Field: FieldHeartbeat, Value: 1})
	return conferir(FieldHeartbeat, st, err)
}

// blockMaxSize é o tamanho de bloco usado na transferência da imagem enquanto
// o painel não informa o dele.
//
// NÃO CONFIRMADO: o software do fabricante manda "blockMaxSize" no cabeçalho
// da transferência e o firmware responde com o valor que aceita. Até termos um
// aparelho pra ler essa resposta, usamos 4096, múltiplo do relatório de 64.
const blockMaxSize = 4096

// transportHeader é o corpo JSON do "POST transport".
//
// NÃO CONFIRMADO: os nomes dos campos vêm das chaves JSON encontradas no
// binário do fabricante (file, format, width, height, data, blockMaxSize),
// mas a forma exata do objeto não foi observada num aparelho real.
type transportHeader struct {
	File         string `json:"file"`
	Format       string `json:"format"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Length       int    `json:"length"`
	BlockMaxSize int    `json:"blockMaxSize"`
}

// SendImage manda um quadro pro painel: cabeçalho, blocos e fim de
// transferência. A imagem é redimensionada por quem chama — aqui ela precisa
// já estar na geometria nativa do modelo.
func (c *Client) SendImage(img image.Image, quality int) error {
	b := img.Bounds()
	if b.Dx() != c.produto.Width || b.Dy() != c.produto.Height {
		return fmt.Errorf("imagem %dx%d não bate com a tela %dx%d do %s",
			b.Dx(), b.Dy(), c.produto.Width, c.produto.Height, c.produto.Code)
	}
	var buf bytes.Buffer
	if quality <= 0 || quality > 100 {
		quality = 85
	}
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return err
	}
	dados := buf.Bytes()

	cab, err := json.Marshal(transportHeader{
		File:         "frame.jpg",
		Format:       "jpeg",
		Width:        b.Dx(),
		Height:       b.Dy(),
		Length:       len(dados),
		BlockMaxSize: blockMaxSize,
	})
	if err != nil {
		return err
	}
	st, err := c.Send(Request{Verb: VerbPost, Field: FieldTransport, Value: 1, Body: cab})
	if err := conferir(FieldTransport, st, err); err != nil {
		return err
	}
	// Os blocos vão crus, sem linha de requisição: o painel já sabe quantos
	// bytes esperar pelo ContentLength do cabeçalho.
	for i := 0; i < len(dados); i += blockMaxSize {
		if err := c.sendRaw(dados[i:min(i+blockMaxSize, len(dados))]); err != nil {
			return err
		}
	}
	st, err = c.Send(Request{Verb: VerbPost, Field: FieldTransported, Value: 1})
	return conferir(FieldTransported, st, err)
}

func conferir(campo string, st Status, err error) error {
	if err != nil {
		return fmt.Errorf("%s: %w", campo, err)
	}
	if st != StatusOK {
		return fmt.Errorf("%s: painel respondeu %d", campo, int(st))
	}
	return nil
}
