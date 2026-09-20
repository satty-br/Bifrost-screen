package kalkan

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
	"testing"
	"time"
)

// falsoTransport grava o que foi escrito e devolve respostas enfileiradas,
// pra testar o protocolo inteiro sem nenhum hardware.
type falsoTransport struct {
	escrito    bytes.Buffer
	relatorios int
	respostas  [][]byte
	tamanho    int
	fechado    bool
}

func novoFalso(respostas ...string) *falsoTransport {
	f := &falsoTransport{tamanho: 64}
	for _, r := range respostas {
		b := make([]byte, 64)
		copy(b, r)
		f.respostas = append(f.respostas, b)
	}
	return f
}

func (f *falsoTransport) WriteReport(p []byte) error {
	if len(p) != f.tamanho {
		panic("relatório com tamanho errado")
	}
	f.escrito.Write(p)
	f.relatorios++
	return nil
}

func (f *falsoTransport) ReadReport(time.Duration) ([]byte, error) {
	if len(f.respostas) == 0 {
		return nil, ErrTimeout
	}
	r := f.respostas[0]
	f.respostas = f.respostas[1:]
	return r, nil
}

func (f *falsoTransport) PayloadSize() int { return f.tamanho }
func (f *falsoTransport) Close() error     { f.fechado = true; return nil }

// texto devolve tudo que foi escrito, sem o preenchimento de zeros.
func (f *falsoTransport) texto() string {
	return string(bytes.ReplaceAll(f.escrito.Bytes(), []byte{0}, nil))
}

func TestEncodeHandshake(t *testing.T) {
	got := string(Request{Verb: VerbPost, Field: FieldConn}.Encode())
	want := "POST conn\r\n\r\n"
	if got != want {
		t.Fatalf("handshake = %q, esperava %q", got, want)
	}
}

func TestEncodeComCorpo(t *testing.T) {
	r := Request{Verb: VerbPost, Field: FieldBrightness, Value: 60, Seq: 7, Body: []byte(`{"v":60}`)}
	got := string(r.Encode())
	want := "POST brightness 60\r\n" +
		"SeqNumber=7\r\n" +
		"ContentType=json\r\n" +
		"ContentLength=8\r\n" +
		"\r\n" +
		`{"v":60}`
	if got != want {
		t.Fatalf("encode =\n%q\nesperava\n%q", got, want)
	}
}

func TestEncodeSemCorpoTemContentLengthZero(t *testing.T) {
	got := string(Request{Verb: VerbState, Field: FieldHeartbeat, Value: 1, Seq: 2}.Encode())
	if !strings.Contains(got, "ContentLength=0\r\n\r\n") {
		t.Fatalf("faltou ContentLength=0: %q", got)
	}
	if !strings.HasPrefix(got, "STATE heartbeat 1\r\n") {
		t.Fatalf("linha de requisição errada: %q", got)
	}
}

func TestParseStatus(t *testing.T) {
	casos := []struct {
		nome string
		in   []byte
		want Status
		erro bool
	}{
		{"ok", []byte("200\r\n"), StatusOK, false},
		{"falha", []byte("400\r\n"), StatusFail, false},
		{"com preenchimento", append([]byte("200\r\n"), make([]byte, 59)...), StatusOK, false},
		{"vazio", make([]byte, 64), 0, true},
		{"lixo", []byte("nada\r\n"), 0, true},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			got, err := ParseStatus(c.in)
			if c.erro {
				if err == nil {
					t.Fatalf("esperava erro, veio %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if got != c.want {
				t.Fatalf("status = %v, esperava %v", got, c.want)
			}
		})
	}
}

func TestChunk(t *testing.T) {
	casos := []struct {
		nome    string
		msg     []byte
		tamanho int
		partes  int
	}{
		{"mensagem vazia vira um relatório zerado", nil, 64, 1},
		{"cabe num relatório", make([]byte, 10), 64, 1},
		{"exatamente um relatório", make([]byte, 64), 64, 1},
		{"um byte a mais", make([]byte, 65), 64, 2},
		{"imagem", make([]byte, 4096), 64, 64},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			partes := Chunk(c.msg, c.tamanho)
			if len(partes) != c.partes {
				t.Fatalf("partes = %d, esperava %d", len(partes), c.partes)
			}
			for i, p := range partes {
				if len(p) != c.tamanho {
					t.Fatalf("parte %d tem %d bytes, esperava %d", i, len(p), c.tamanho)
				}
			}
		})
	}
}

func TestChunkPreencheComZeros(t *testing.T) {
	partes := Chunk([]byte("oi"), 8)
	if len(partes) != 1 {
		t.Fatalf("partes = %d", len(partes))
	}
	if !bytes.Equal(partes[0], []byte{'o', 'i', 0, 0, 0, 0, 0, 0}) {
		t.Fatalf("preenchimento errado: %v", partes[0])
	}
}

func TestClientConn(t *testing.T) {
	f := novoFalso("200\r\n")
	c := NewClient(f, Products[0])
	if err := c.Conn(); err != nil {
		t.Fatalf("Conn: %v", err)
	}
	if got := f.texto(); got != "POST conn\r\n\r\n" {
		t.Fatalf("mandou %q", got)
	}
}

func TestClientPropagaFalha(t *testing.T) {
	f := novoFalso("400\r\n")
	c := NewClient(f, Products[0])
	err := c.Brightness(50)
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Fatalf("esperava erro com 400, veio %v", err)
	}
}

func TestClientSemRespostaDaTimeout(t *testing.T) {
	c := NewClient(novoFalso(), Products[0])
	if err := c.Heartbeat(); err == nil {
		t.Fatal("esperava erro de timeout")
	}
}

func TestSeqIncrementa(t *testing.T) {
	f := novoFalso("200\r\n", "200\r\n")
	c := NewClient(f, Products[0])
	if err := c.Brightness(10); err != nil {
		t.Fatal(err)
	}
	if err := c.Brightness(20); err != nil {
		t.Fatal(err)
	}
	texto := f.texto()
	if !strings.Contains(texto, "SeqNumber=1\r\n") || !strings.Contains(texto, "SeqNumber=2\r\n") {
		t.Fatalf("sequência não incrementou: %q", texto)
	}
}

func TestSendImageGeometriaErrada(t *testing.T) {
	c := NewClient(novoFalso("200\r\n"), Products[0]) // 320x240
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	err := c.SendImage(img, 85)
	if err == nil || !strings.Contains(err.Error(), "não bate") {
		t.Fatalf("esperava erro de geometria, veio %v", err)
	}
}

func TestSendImageFluxo(t *testing.T) {
	p := Products[0] // 320x240
	f := novoFalso("200\r\n", "200\r\n")
	c := NewClient(f, p)

	img := image.NewRGBA(image.Rect(0, 0, p.Width, p.Height))
	for y := 0; y < p.Height; y++ {
		for x := 0; x < p.Width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 128, A: 255})
		}
	}
	if err := c.SendImage(img, 85); err != nil {
		t.Fatalf("SendImage: %v", err)
	}

	texto := f.texto()
	if !strings.Contains(texto, "POST transport 1\r\n") {
		t.Fatal("faltou o POST transport")
	}
	if !strings.Contains(texto, "POST transported 1\r\n") {
		t.Fatal("faltou o POST transported")
	}
	// o cabeçalho tem que descrever a imagem de verdade
	ini := strings.Index(texto, `{"file"`)
	if ini < 0 {
		t.Fatalf("cabeçalho JSON não encontrado em %q", texto[:min(200, len(texto))])
	}
	fim := strings.Index(texto[ini:], "}") + ini + 1
	var cab transportHeader
	if err := json.Unmarshal([]byte(texto[ini:fim]), &cab); err != nil {
		t.Fatalf("cabeçalho ilegível: %v (%q)", err, texto[ini:fim])
	}
	if cab.Width != p.Width || cab.Height != p.Height || cab.Format != "jpeg" {
		t.Fatalf("cabeçalho errado: %+v", cab)
	}
	if cab.Length <= 0 {
		t.Fatalf("ContentLength da imagem inválido: %d", cab.Length)
	}
	if cab.BlockMaxSize%64 != 0 {
		t.Fatalf("blockMaxSize %d não é múltiplo do relatório de 64", cab.BlockMaxSize)
	}
	// os bytes da imagem têm que ser um JPEG decodificável na geometria certa.
	// Aqui olhamos o fluxo cru (com o preenchimento), senão tirar os zeros
	// estragaria o próprio JPEG.
	cru := f.escrito.Bytes()
	jini := bytes.Index(cru, []byte{0xff, 0xd8, 0xff})
	if jini < 0 {
		t.Fatal("não achei a marca de início de JPEG no que foi enviado")
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(cru[jini:]))
	if err != nil {
		t.Fatalf("JPEG inválido: %v", err)
	}
	if cfg.Width != p.Width || cfg.Height != p.Height {
		t.Fatalf("JPEG %dx%d, esperava %dx%d", cfg.Width, cfg.Height, p.Width, p.Height)
	}
}

func TestLookup(t *testing.T) {
	p, ok := Lookup(0xB550)
	if !ok {
		t.Fatal("B550 devia estar na tabela")
	}
	if p.Width != 320 || p.Height != 240 {
		t.Fatalf("B550 = %dx%d, esperava 320x240", p.Width, p.Height)
	}
	if strings.ToUpper(p.OEM) != "KALKAN LCD DISPLAY" {
		t.Fatalf("nome OEM do B550 = %q", p.OEM)
	}
	if Known(0x0000) {
		t.Fatal("PID inexistente não devia ser conhecido")
	}
}

func TestTabelaDeProdutosCoerente(t *testing.T) {
	vistos := map[uint16]bool{}
	for _, p := range Products {
		if vistos[p.PID] {
			t.Fatalf("PID %04X duplicado", p.PID)
		}
		vistos[p.PID] = true
		if p.Width <= 0 || p.Height <= 0 || p.FPS <= 0 {
			t.Fatalf("%s tem geometria inválida: %+v", p.Code, p)
		}
		if strings.ToUpper(p.Code) != strings.ToUpper(hex4(p.PID)) {
			t.Fatalf("código %q não bate com o PID %04X", p.Code, p.PID)
		}
	}
}

func hex4(v uint16) string {
	const d = "0123456789ABCDEF"
	return string([]byte{d[v>>12&0xf], d[v>>8&0xf], d[v>>4&0xf], d[v&0xf]})
}
