package steam

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func newResp(status int, body []byte) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 60), uint8(y * 60), 100, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func newTestClient(t *testing.T, fn roundTripFunc) *Client {
	t.Helper()
	c := New(t.TempDir())
	c.http = &http.Client{Transport: fn}
	return c
}

func TestGetJSONStatusCodes(t *testing.T) {
	cases := []struct {
		status  int
		wantErr string
	}{
		{http.StatusUnauthorized, "API key da Steam inválida"},
		{http.StatusForbidden, "API key da Steam inválida"},
		{http.StatusTooManyRequests, "limite de consultas"},
		{http.StatusInternalServerError, "Steam respondeu"},
	}
	for _, tc := range cases {
		c := newTestClient(t, func(r *http.Request) (*http.Response, error) {
			return newResp(tc.status, nil), nil
		})
		var out any
		err := c.getJSON(context.Background(), "https://example.invalid/x", &out)
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("status %d: err = %v, esperava conter %q", tc.status, err, tc.wantErr)
		}
	}
}

func TestGetJSONNetworkError(t *testing.T) {
	c := newTestClient(t, func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("conexão recusada")
	})
	var out any
	err := c.getJSON(context.Background(), "https://example.invalid/x", &out)
	if err == nil || !strings.Contains(err.Error(), "sem conexão com a Steam") {
		t.Errorf("err = %v", err)
	}
}

func TestTestSuccessAndNotFound(t *testing.T) {
	body := []byte(`{"response":{"players":[{"personaname":"satty","gameid":"570","gameextrainfo":"Dota 2"}]}}`)
	c := newTestClient(t, func(r *http.Request) (*http.Response, error) { return newResp(200, body), nil })
	name, err := c.Test(context.Background(), "key", "id")
	if err != nil || name != "satty" {
		t.Fatalf("Test() = %q, %v", name, err)
	}

	empty := newTestClient(t, func(r *http.Request) (*http.Response, error) {
		return newResp(200, []byte(`{"response":{"players":[]}}`)), nil
	})
	if _, err := empty.Test(context.Background(), "key", "id"); err == nil {
		t.Error("Test() com lista vazia deveria falhar")
	}
}

func TestRefreshEndToEnd(t *testing.T) {
	summary := []byte(`{"response":{"players":[{"personaname":"satty","gameid":"730","gameextrainfo":"Counter-Strike 2"}]}}`)
	owned := []byte(`{"response":{"games":[{"appid":730,"name":"Counter-Strike 2","playtime_forever":600,"playtime_2weeks":30}]}}`)
	cover := pngBytes(t)
	var coverCalls int
	c := newTestClient(t, func(r *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(r.URL.Host, "steamcommunity") || strings.Contains(r.URL.Path, "GetPlayerSummaries"):
			return newResp(200, summary), nil
		case strings.Contains(r.URL.Path, "GetOwnedGames"):
			return newResp(200, owned), nil
		case strings.Contains(r.URL.Host, "akamai"):
			coverCalls++
			return newResp(200, cover), nil
		}
		return newResp(404, nil), nil
	})

	s := Settings{Enabled: true, Source: SourceWeb, APIKey: "key", SteamID64: "id", LibraryEvery: time.Minute}
	c.refresh(context.Background(), s)

	got := c.Get()
	if !got.Playing || got.Name != "Counter-Strike 2" || got.AppID != 730 {
		t.Fatalf("status inesperado: %+v", got)
	}
	if got.TotalMinutes != 600 || got.TwoWeeksMinutes != 30 {
		t.Errorf("minutos não vieram da biblioteca: %+v", got)
	}
	if !got.HasCover {
		t.Error("deveria ter baixado a capa")
	}
	if got.SessionStart.IsZero() {
		t.Error("SessionStart deveria estar preenchido")
	}
	if c.LastError() != "" {
		t.Errorf("não deveria ter erro, veio %q", c.LastError())
	}

	// uma segunda chamada não deveria baixar a capa de novo (cache em memória).
	c.refresh(context.Background(), s)
	if coverCalls != 1 {
		t.Errorf("capa deveria ser baixada só uma vez, foi baixada %d", coverCalls)
	}
}

func TestRefreshSummaryError(t *testing.T) {
	c := newTestClient(t, func(r *http.Request) (*http.Response, error) { return newResp(401, nil), nil })
	c.refresh(context.Background(), Settings{Source: SourceWeb, APIKey: "x", SteamID64: "y"})
	if c.LastError() == "" {
		t.Error("deveria ter registrado o erro")
	}
	if c.Get().Playing {
		t.Error("não deveria estar jogando após erro")
	}
}

func TestConfigureAndReady(t *testing.T) {
	c := New(t.TempDir())
	if c.Ready() {
		t.Error("cliente novo não deveria estar pronto")
	}
	c.Configure(Settings{Enabled: true, Source: SourceWeb, APIKey: "a", SteamID64: "b"})
	// Configure só troca as credenciais; não marca ready sozinho (isso é papel do Run()).
	if c.LastError() != "" {
		t.Errorf("Configure não deveria gerar erro, veio %q", c.LastError())
	}
}

func TestRefreshLocalAndTestLocal(t *testing.T) {
	dir := t.TempDir()
	writeVDF(t, filepath.Join(dir, "steamapps", "libraryfolders.vdf"), `"libraryfolders" { "0" { "path" "`+filepath.ToSlash(dir)+`" } }`)
	writeVDF(t, filepath.Join(dir, "steamapps", "appmanifest_570.acf"), `"AppState" { "appid" "570" "name" "Dota 2" }`)
	writeVDF(t, filepath.Join(dir, "userdata", "1", "config", "localconfig.vdf"), sampleLocalConfig)

	c := New(t.TempDir())
	c.local = newLocalReader(fakeEnv{root: dir, running: 570, user: 1})

	persona, path, err := c.TestLocal()
	if err != nil || persona != "satty" || path != dir {
		t.Fatalf("TestLocal() = %q, %q, %v", persona, path, err)
	}

	c.refreshLocal(context.Background())
	got := c.Get()
	if !got.Playing || got.Name != "Dota 2" || got.AppID != 570 {
		t.Fatalf("refreshLocal não preencheu o status: %+v", got)
	}
	if !c.Ready() {
		t.Error("deveria estar pronto após refreshLocal com sucesso")
	}

	// Steam não encontrada: erro e não-pronto.
	broken := New(t.TempDir())
	broken.local = newLocalReader(fakeEnv{root: filepath.Join(dir, "nao-existe")})
	broken.refreshLocal(context.Background())
	if broken.LastError() == "" || broken.Ready() {
		t.Error("Steam não encontrada deveria gerar erro e ready=false")
	}
}

func TestFileCover(t *testing.T) {
	c := New(t.TempDir())
	dir := t.TempDir()
	path := filepath.Join(dir, "capa.png")
	if err := os.WriteFile(path, pngBytes(t), 0o644); err != nil {
		t.Fatal(err)
	}
	img := c.fileCover(path)
	if img == nil {
		t.Fatal("deveria ter decodificado a capa")
	}
	// segunda chamada usa o cache em memória (mesmo resultado).
	if c.fileCover(path) == nil {
		t.Error("cache da capa deveria devolver a mesma imagem")
	}
	if c.fileCover(filepath.Join(dir, "nao-existe.png")) != nil {
		t.Error("arquivo inexistente deveria devolver nil")
	}
}

func TestRunLocalAndDisabled(t *testing.T) {
	dir := t.TempDir()
	writeVDF(t, filepath.Join(dir, "userdata", "1", "config", "localconfig.vdf"), sampleLocalConfig)

	c := New(t.TempDir())
	c.local = newLocalReader(fakeEnv{root: dir, running: 0, user: 1})
	c.Configure(Settings{Enabled: true, Source: SourceLocal, StatusEvery: 10 * time.Millisecond})

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	c.Run(ctx)

	if c.Get().Source != SourceLocal {
		t.Errorf("Run() deveria ter consultado a fonte local, veio %+v", c.Get())
	}

	// desligado: status some e ready volta a false.
	c2 := New(t.TempDir())
	c2.Configure(Settings{Enabled: false})
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel2()
	c2.Run(ctx2)
	if c2.Ready() {
		t.Error("desligado não deveria ficar pronto")
	}
}

func writeVDF(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
