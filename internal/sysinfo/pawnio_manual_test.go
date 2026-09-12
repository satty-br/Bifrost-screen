//go:build windows

package sysinfo

import (
	"os"
	"testing"
)

// Verificação manual, não roda no CI: baixa de verdade o pacote oficial do
// PawnIO.Modules e confere que a descoberta via API do GitHub + checksum +
// extração do zip funcionam. Rode com: go test -run TestManualFetchPawnIOModule -v
func TestManualFetchPawnIOModule(t *testing.T) {
	if os.Getenv("BIFROST_MANUAL_NET_TEST") == "" {
		t.Skip("defina BIFROST_MANUAL_NET_TEST=1 para rodar este teste (usa a rede de verdade)")
	}
	dir, _ := pawnioModuleCacheDir()
	os.RemoveAll(dir)

	for _, name := range []string{"IntelMSR.bin", "AMDFamily17.bin"} {
		data, err := fetchPawnIOModule(name)
		if err != nil {
			t.Fatalf("fetchPawnIOModule(%q): %v", name, err)
		}
		if len(data) == 0 {
			t.Fatalf("fetchPawnIOModule(%q) devolveu vazio", name)
		}
		t.Logf("%s: %d bytes", name, len(data))
	}
	// segunda chamada tem que vir do cache local, sem rede
	if _, err := fetchPawnIOModule("IntelMSR.bin"); err != nil {
		t.Fatalf("leitura do cache falhou: %v", err)
	}
}
