// Package update verifica, baixa e instala novas versões do Bifrost a partir
// dos releases públicos do GitHub (sem servidor próprio, sem telemetria).
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	repoOwner    = "satty-br"
	repoName     = "Bifrost-screen"
	apiLatestURL = "https://api.github.com/repos/" + repoOwner + "/" + repoName + "/releases/latest"
	userAgent    = "Bifrost-updater"
)

// Asset é um arquivo anexado a um release do GitHub.
type Asset struct {
	Name string
	URL  string
	Size int64
}

// Release é a versão mais nova encontrada.
type Release struct {
	Version string  `json:"versao"` // sem o prefixo "v" (ex.: "1.1.0")
	Notes   string  `json:"notas"`
	URL     string  `json:"url"` // link da release no GitHub
	Assets  []Asset `json:"-"`   // detalhe interno, não exposto no painel
}

// FindAsset acha, entre os anexos do release, o binário desta plataforma e o
// checksums.txt (se existir). checksums pode vir nil em releases antigos.
func (r *Release) FindAsset() (bin *Asset, checksums *Asset) {
	want := AssetName()
	for i := range r.Assets {
		switch r.Assets[i].Name {
		case want:
			bin = &r.Assets[i]
		case "checksums.txt":
			checksums = &r.Assets[i]
		}
	}
	return
}

// AssetName é o nome esperado do binário desta plataforma nos releases,
// seguindo a convenção usada em build.sh e no workflow de release:
// bifrost-{GOOS}-{GOARCH}[.exe].
func AssetName() string {
	name := fmt.Sprintf("bifrost-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

// IsNewer diz se candidate é uma versão mais nova que current. Aceita "x.y.z"
// com ou sem o prefixo "v"; sufixos como "-beta" são ignorados na comparação.
func IsNewer(current, candidate string) bool {
	cur, cand := parseVersion(current), parseVersion(candidate)
	for i := 0; i < 3; i++ {
		if cand[i] != cur[i] {
			return cand[i] > cur[i]
		}
	}
	return false
}

func parseVersion(v string) [3]int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	var out [3]int
	for i, part := range strings.SplitN(v, ".", 3) {
		if i >= 3 {
			break
		}
		out[i], _ = strconv.Atoi(part)
	}
	return out
}

func httpClient(timeout time.Duration) *http.Client { return &http.Client{Timeout: timeout} }

// FetchLatest consulta a API pública do GitHub (sem autenticação; sujeita ao
// limite de 60 requisições/hora por IP, bem acima do necessário aqui).
func FetchLatest(ctx context.Context) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiLatestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient(10 * time.Second).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // repositório ainda sem nenhum release
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github: status %d", resp.StatusCode)
	}
	var payload struct {
		TagName string `json:"tag_name"`
		Body    string `json:"body"`
		HTMLURL string `json:"html_url"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return nil, err
	}
	rel := &Release{Version: strings.TrimPrefix(payload.TagName, "v"), Notes: payload.Body, URL: payload.HTMLURL}
	for _, a := range payload.Assets {
		rel.Assets = append(rel.Assets, Asset{Name: a.Name, URL: a.BrowserDownloadURL, Size: a.Size})
	}
	return rel, nil
}

// DownloadText baixa um arquivo de texto pequeno (o checksums.txt) para a memória.
func DownloadText(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient(30 * time.Second).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download: status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return string(data), err
}

const maxBinaryBytes = 200 << 20 // bem acima do tamanho real (~10-20 MB)

// Download baixa o binário para um arquivo temporário na mesma pasta do
// executável atual (necessário para o rename atômico funcionar em Apply,
// já que rename não cruza sistemas de arquivo/discos diferentes).
func Download(ctx context.Context, url, sameDirAs string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient(5 * time.Minute).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download: status %d", resp.StatusCode)
	}
	f, err := os.CreateTemp(sameDirAs, "bifrost-update-*.tmp")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, io.LimitReader(resp.Body, maxBinaryBytes)); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// VerifyChecksum confere o SHA-256 do arquivo baixado contra o conteúdo de um
// checksums.txt (formato "sha256sum": "<hex>  <nome-do-arquivo>" por linha).
func VerifyChecksum(path, checksumsText, assetName string) error {
	want := ""
	for _, line := range strings.Split(checksumsText, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == assetName {
			want = strings.ToLower(fields[0])
			break
		}
	}
	if want == "" {
		return fmt.Errorf("checksum de %s não encontrado em checksums.txt", assetName)
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("checksum não confere: esperado %s, obtido %s", want, got)
	}
	return nil
}

// Checker consulta o GitHub periodicamente e guarda o release mais novo
// (se houver um mais novo que a versão atual), pronto pra o painel mostrar.
type Checker struct {
	current string

	mu     sync.RWMutex
	latest *Release
	err    string
}

// NewChecker cria um Checker para a versão atualmente rodando.
func NewChecker(currentVersion string) *Checker { return &Checker{current: currentVersion} }

// Start dispara uma checagem imediata e depois repete a cada interval, até ctx acabar.
func (c *Checker) Start(ctx context.Context, interval time.Duration) {
	go func() {
		c.checkOnce(ctx)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				c.checkOnce(ctx)
			}
		}
	}()
}

// CheckNow força uma checagem síncrona (usado pelo botão "verificar agora" do painel).
func (c *Checker) CheckNow(ctx context.Context) *Release {
	c.checkOnce(ctx)
	return c.Available()
}

func (c *Checker) checkOnce(ctx context.Context) {
	rel, err := FetchLatest(ctx)
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		c.err = err.Error()
		return
	}
	c.err = ""
	if rel != nil && IsNewer(c.current, rel.Version) {
		c.latest = rel
	} else {
		c.latest = nil
	}
}

// Available devolve o release mais novo, se houver um mais novo que a versão atual (senão nil).
func (c *Checker) Available() *Release {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.latest
}

// LastError devolve o erro da última tentativa de checagem, se houve algum.
func (c *Checker) LastError() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.err
}
