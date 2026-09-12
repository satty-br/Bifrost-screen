//go:build windows

package rtss

import (
	"testing"
	"unsafe"
)

// O tamanho tem que bater com RTSS_SHARED_MEMORY (9 DWORDs = 36 bytes) —
// documentado no cabeçalho oficial do SDK do RTSS.
func TestHeaderSize(t *testing.T) {
	if got := unsafe.Sizeof(header{}); got != 36 {
		t.Errorf("sizeof(header) = %d, queria 36", got)
	}
}

// Sem o RTSS rodando (o normal nesta máquina de teste), Ler tem que falhar de
// forma limpa, sem travar nem entrar em pânico.
func TestLerSemRTSS(t *testing.T) {
	if _, err := Ler(); err == nil {
		t.Skip("RTSS está rodando nesta máquina; nada a conferir aqui")
	}
}

func TestCString(t *testing.T) {
	casos := map[string]string{
		"cs2.exe\x00\x00\x00": "cs2.exe",
		"":                    "",
		"semnulo":             "semnulo",
	}
	for in, quer := range casos {
		if got := cString([]byte(in)); got != quer {
			t.Errorf("cString(%q) = %q, queria %q", in, got, quer)
		}
	}
}
