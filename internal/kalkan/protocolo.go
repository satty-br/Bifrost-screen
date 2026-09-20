package kalkan

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Verbos e campos aceitos pelo painel, lidos um a um da classe
// ProtocolCommands do software do fabricante.
const (
	VerbPost  = "POST"
	VerbState = "STATE"
)

// Campos do verbo POST.
const (
	FieldConn            = "conn"
	FieldPower           = "power"
	FieldBrightness      = "brightness"
	FieldRotate          = "rotate"
	FieldMode            = "mode"
	FieldLogo            = "logo"
	FieldPresetThemeID   = "presetThemeId"
	FieldOSDState        = "osdState"
	FieldRealtimeDisplay = "realtimeDisplay"
	FieldSysinfoDisplay  = "sysinfoDisplay"
	FieldTimeout         = "timeout"
	FieldAlarm           = "alarm"
	FieldLog             = "log"
	FieldReboot          = "reboot"
	FieldRecovery        = "recovery"
	FieldTransport       = "transport"
	FieldTransported     = "transported"
)

// Campos do verbo STATE.
const (
	FieldHeartbeat = "heartbeat"
	FieldTimestamp = "timestamp"
)

// Request é uma mensagem pro painel. O formato na linha é:
//
//	POST <campo> <valor>\r\n
//	SeqNumber=<n>\r\n
//	ContentType=json\r\n
//	ContentLength=<len>\r\n
//	\r\n
//	<corpo>
//
// O handshake é o único caso especial: "POST conn\r\n\r\n", sem valor, sem
// cabeçalhos e sem corpo.
type Request struct {
	Verb  string
	Field string
	Value int
	Seq   uint32
	Body  []byte // JSON; vazio significa sem corpo
}

// Encode devolve a mensagem inteira em bytes, pronta pra ser fatiada em
// relatórios HID.
func (r Request) Encode() []byte {
	var b bytes.Buffer
	verb := r.Verb
	if verb == "" {
		verb = VerbPost
	}
	if r.Field == FieldConn && len(r.Body) == 0 {
		// handshake: sem valor, sem cabeçalhos
		b.WriteString(verb + " " + FieldConn + "\r\n\r\n")
		return b.Bytes()
	}
	b.WriteString(verb + " " + r.Field + " " + strconv.Itoa(r.Value) + "\r\n")
	if r.Seq != 0 {
		b.WriteString("SeqNumber=" + strconv.FormatUint(uint64(r.Seq), 10) + "\r\n")
	}
	b.WriteString("ContentType=json\r\n")
	b.WriteString("ContentLength=" + strconv.Itoa(len(r.Body)) + "\r\n")
	b.WriteString("\r\n")
	b.Write(r.Body)
	return b.Bytes()
}

// Status é o código devolvido pelo painel.
type Status int

const (
	StatusOK   Status = 200
	StatusFail Status = 400
)

// ErrNoStatus indica que a resposta lida não continha um código conhecido.
var ErrNoStatus = errors.New("resposta sem código de status")

// ParseStatus lê "200\r\n" ou "400\r\n" de uma resposta, ignorando bytes de
// preenchimento (o relatório HID vem sempre com o tamanho cheio, zerado no
// fim) e espaços em branco.
func ParseStatus(resp []byte) (Status, error) {
	s := strings.TrimSpace(string(bytes.TrimRight(resp, "\x00")))
	if s == "" {
		return 0, ErrNoStatus
	}
	// pega a primeira linha não vazia
	for _, linha := range strings.Split(s, "\n") {
		linha = strings.TrimSpace(linha)
		if linha == "" {
			continue
		}
		n, err := strconv.Atoi(linha)
		if err != nil {
			return 0, fmt.Errorf("%w: %q", ErrNoStatus, linha)
		}
		return Status(n), nil
	}
	return 0, ErrNoStatus
}

// Chunk fatia uma mensagem em relatórios de saída de tamanho fixo. Cada
// relatório é preenchido com zeros até o tamanho exato — o Windows exige o
// tamanho declarado pelo descritor HID, nem um byte a menos.
func Chunk(msg []byte, size int) [][]byte {
	if size <= 0 {
		return nil
	}
	if len(msg) == 0 {
		return [][]byte{make([]byte, size)}
	}
	var out [][]byte
	for i := 0; i < len(msg); i += size {
		parte := make([]byte, size)
		copy(parte, msg[i:min(i+size, len(msg))])
		out = append(out, parte)
	}
	return out
}
