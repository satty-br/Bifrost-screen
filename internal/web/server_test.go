package web

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/satty-br/Bifrost-screen/internal/app"
	"github.com/satty-br/Bifrost-screen/internal/config"
)

// freePort pega uma porta livre em 127.0.0.1 para o teste usar.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

// startServer sobe um Server real num listener local e devolve uma função de limpeza.
func startServer(t *testing.T, srv *Server) {
	t.Helper()
	srv.Port = freePort(t)
	ln, err := srv.Listen()
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve(ln)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	})
	// espera o servidor aceitar conexões.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if c, err := net.Dial("tcp", srv.URL()[len("http://"):len(srv.URL())-1]); err == nil {
			c.Close()
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("servidor não começou a aceitar conexões")
}

func do(t *testing.T, method, url string, body io.Reader, withHeader bool) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	if withHeader {
		req.Header.Set("X-Bifrost", "1")
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// serverWithApp monta um Server com uma App real (revisão SIMULADO, sem Run()
// chamado — então nenhum acesso a hardware/rede acontece).
func serverWithApp(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	store, err := config.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := app.New(store, filepath.Join(dir, "cache"), "test")
	srv := &Server{App: a, Store: store, LogPath: filepath.Join(dir, "bifrost.log")}
	startServer(t, srv)
	return srv
}

// serverBare monta um Server só com Store (sem App), usado para testar os
// endpoints de configuração sem nunca chamar Store.Set() num Store ligado a
// uma App (o que gravaria a inicialização automática no registro real).
func serverBare(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	store, err := config.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	srv := &Server{Store: store}
	startServer(t, srv)
	return srv
}

func TestIndexAndPreview(t *testing.T) {
	srv := serverWithApp(t)
	resp := do(t, "GET", srv.URL(), nil, false)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET / = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "<html") {
		t.Error("página inicial deveria conter html")
	}

	resp2 := do(t, "GET", srv.URL()+"api/previa.png", nil, false)
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 || resp2.Header.Get("Content-Type") != "image/png" {
		t.Errorf("GET /api/previa.png = %d, content-type %q", resp2.StatusCode, resp2.Header.Get("Content-Type"))
	}
}

func TestStateEndpoint(t *testing.T) {
	srv := serverWithApp(t)
	resp := do(t, "GET", srv.URL()+"api/estado", nil, false)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET /api/estado = %d", resp.StatusCode)
	}
	var st app.State
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	if st.Version != "test" {
		t.Errorf("versão = %q, esperava test", st.Version)
	}
}

func TestPortsEndpoint(t *testing.T) {
	srv := serverWithApp(t)
	resp := do(t, "GET", srv.URL()+"api/portas", nil, false)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET /api/portas = %d", resp.StatusCode)
	}
	var ports []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&ports); err != nil {
		t.Fatal(err)
	}
}

func TestActionEndpoint(t *testing.T) {
	srv := serverWithApp(t)
	for _, acao := range []string{"proxima", "anterior", "pausar", "retomar", "reconectar", "encerrar_conflitos"} {
		body := strings.NewReader(`{"acao":"` + acao + `"}`)
		resp := do(t, "POST", srv.URL()+"api/acao", body, true)
		if resp.StatusCode != 200 {
			b, _ := io.ReadAll(resp.Body)
			t.Errorf("ação %q = %d: %s", acao, resp.StatusCode, b)
		}
		resp.Body.Close()
	}

	// ação desconhecida.
	resp := do(t, "POST", srv.URL()+"api/acao", strings.NewReader(`{"acao":"xablau"}`), true)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("ação desconhecida deveria dar 400, veio %d", resp.StatusCode)
	}

	// sem o cabeçalho X-Bifrost, deve recusar.
	resp2 := do(t, "POST", srv.URL()+"api/acao", strings.NewReader(`{"acao":"pausar"}`), false)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusForbidden {
		t.Errorf("sem X-Bifrost deveria dar 403, veio %d", resp2.StatusCode)
	}

	// corpo inválido.
	resp3 := do(t, "POST", srv.URL()+"api/acao", strings.NewReader(`{isso não é json`), true)
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusBadRequest {
		t.Errorf("corpo inválido deveria dar 400, veio %d", resp3.StatusCode)
	}
}

func TestTestSteamEndpoint(t *testing.T) {
	srv := serverWithApp(t)

	// fonte local: só lê o registro/arquivos (sem rede). Pode falhar se a Steam
	// não estiver instalada na máquina de teste — em ambos os casos o handler
	// deve responder com um status coerente, sem travar.
	resp := do(t, "POST", srv.URL()+"api/steam/testar", strings.NewReader(`{"fonte":"local"}`), true)
	resp.Body.Close()
	if resp.StatusCode != 200 && resp.StatusCode != http.StatusNotFound {
		t.Errorf("fonte local = %d, esperava 200 ou 404", resp.StatusCode)
	}

	// fonte web sem credenciais: erro de validação, sem chamada de rede.
	resp2 := do(t, "POST", srv.URL()+"api/steam/testar", strings.NewReader(`{"fonte":"web","api_key":"","steam_id64":""}`), true)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusBadRequest {
		t.Errorf("sem credenciais deveria dar 400, veio %d", resp2.StatusCode)
	}

	// corpo inválido.
	resp3 := do(t, "POST", srv.URL()+"api/steam/testar", strings.NewReader(`{invalido`), true)
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusBadRequest {
		t.Errorf("corpo inválido deveria dar 400, veio %d", resp3.StatusCode)
	}
}

func TestLogsAndInfoEndpoints(t *testing.T) {
	srv := serverWithApp(t)

	resp := do(t, "GET", srv.URL()+"api/logs", nil, false)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET /api/logs = %d", resp.StatusCode)
	}
	var logs struct {
		Lines []string `json:"linhas"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&logs); err != nil {
		t.Fatal(err)
	}

	resp2 := do(t, "GET", srv.URL()+"api/info", nil, false)
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("GET /api/info = %d", resp2.StatusCode)
	}
	var info map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&info); err != nil {
		t.Fatal(err)
	}
	if info["versao"] != "test" {
		t.Errorf("info.versao = %v", info["versao"])
	}
}

func TestConfigEndpoints(t *testing.T) {
	srv := serverBare(t)

	resp := do(t, "GET", srv.URL()+"api/config", nil, false)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET /api/config = %d", resp.StatusCode)
	}
	var cfg config.Config
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		t.Fatal(err)
	}

	cfg.Devices[0].Brightness = 77
	data, _ := json.Marshal(cfg)
	resp2 := do(t, "PUT", srv.URL()+"api/config", strings.NewReader(string(data)), true)
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		b, _ := io.ReadAll(resp2.Body)
		t.Fatalf("PUT /api/config = %d: %s", resp2.StatusCode, b)
	}
	var saved config.Config
	if err := json.NewDecoder(resp2.Body).Decode(&saved); err != nil {
		t.Fatal(err)
	}
	if saved.Devices[0].Brightness != 77 {
		t.Errorf("brilho não foi salvo, veio %d", saved.Devices[0].Brightness)
	}

	// corpo inválido.
	resp3 := do(t, "PUT", srv.URL()+"api/config", strings.NewReader(`{invalido`), true)
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusBadRequest {
		t.Errorf("config inválida deveria dar 400, veio %d", resp3.StatusCode)
	}

	// sem X-Bifrost.
	resp4 := do(t, "PUT", srv.URL()+"api/config", strings.NewReader(string(data)), false)
	defer resp4.Body.Close()
	if resp4.StatusCode != http.StatusForbidden {
		t.Errorf("PUT sem cabeçalho deveria dar 403, veio %d", resp4.StatusCode)
	}

	resp5 := do(t, "POST", srv.URL()+"api/config/padrao", strings.NewReader(`{}`), true)
	defer resp5.Body.Close()
	if resp5.StatusCode != 200 {
		t.Fatalf("POST /api/config/padrao = %d", resp5.StatusCode)
	}
	var reset config.Config
	if err := json.NewDecoder(resp5.Body).Decode(&reset); err != nil {
		t.Fatal(err)
	}
	if reset.Devices[0].Brightness != config.Default().Devices[0].Brightness {
		t.Errorf("restaurar padrão deveria zerar o brilho customizado")
	}
}

func TestGuardHostAndMethod(t *testing.T) {
	srv := serverBare(t)

	req, _ := http.NewRequest("GET", srv.URL()+"api/config", nil)
	req.Host = "evil.example.com"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("host não permitido deveria dar 403, veio %d", resp.StatusCode)
	}
}

func TestDirOf(t *testing.T) {
	if got := dirOf(`C:\a\b\config.json`); got != `C:\a\b` {
		t.Errorf("dirOf(barra invertida) = %q", got)
	}
	if got := dirOf("/a/b/config.json"); got != "/a/b" {
		t.Errorf("dirOf(barra normal) = %q", got)
	}
	if got := dirOf("config.json"); got != "config.json" {
		t.Errorf("dirOf(sem pasta) = %q", got)
	}
}
