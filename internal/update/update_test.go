package update

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestIsNewer(t *testing.T) {
	cases := []struct {
		current, candidate string
		want               bool
	}{
		{"1.0.0", "1.0.1", true},
		{"1.0.0", "1.1.0", true},
		{"1.0.0", "2.0.0", true},
		{"1.2.3", "1.2.3", false},
		{"1.2.3", "1.2.2", false},
		{"v1.0.0", "v1.0.1", true},
		{"1.0.0", "1.0.0-beta", false},
		{"1.0.0-beta", "1.0.0", false}, // sufixo é ignorado, então "iguais"
	}
	for _, c := range cases {
		if got := IsNewer(c.current, c.candidate); got != c.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", c.current, c.candidate, got, c.want)
		}
	}
}

func TestVerifyChecksum(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bifrost-linux-amd64")
	content := []byte("conteudo de teste")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	real := hex.EncodeToString(sum[:])

	if err := VerifyChecksum(path, "0000000000000000000000000000000000000000000000000000000000000  bifrost-linux-amd64\n", "bifrost-linux-amd64"); err == nil {
		t.Fatal("esperava erro com checksum errado, mas passou")
	}

	checksums := real + "  bifrost-linux-amd64\nabc123  outro-arquivo\n"
	if err := VerifyChecksum(path, checksums, "bifrost-linux-amd64"); err != nil {
		t.Fatalf("checksum correto foi rejeitado: %v", err)
	}
	if err := VerifyChecksum(path, checksums, "nome-nao-listado"); err == nil {
		t.Fatal("esperava erro pra nome de asset não listado")
	}
}

func TestAssetName(t *testing.T) {
	if AssetName() == "" {
		t.Fatal("AssetName não pode ser vazio")
	}
}
