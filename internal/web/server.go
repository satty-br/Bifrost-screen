// Package web serve o painel de controle do Bifrost em http://127.0.0.1:<porta>.
package web

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/satty-br/Bifrost-screen/internal/app"
	"github.com/satty-br/Bifrost-screen/internal/config"
	"github.com/satty-br/Bifrost-screen/internal/lcd"
	"github.com/satty-br/Bifrost-screen/internal/sysinfo"
	"github.com/satty-br/Bifrost-screen/internal/winutil"
)

//go:embed ui/index.html
var uiFS embed.FS

type Server struct {
	App     *app.App
	Store   *config.Store
	LogPath string
	Port    int
	// OnRestart é chamado depois de uma atualização instalada com sucesso,
	// pra encerrar o processo (o binário novo já foi deixado pronto/reaberto).
	OnRestart func()
	srv       *http.Server
}

// URL do painel.
func (s *Server) URL() string { return fmt.Sprintf("http://127.0.0.1:%d/", s.Port) }

// Listen abre a porta (só no loopback). Devolve erro se já estiver em uso.
func (s *Server) Listen() (net.Listener, error) {
	return net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", s.Port))
}

func (s *Server) Serve(ln net.Listener) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /api/estado", s.state)
	mux.HandleFunc("GET /api/config", s.getConfig)
	mux.HandleFunc("PUT /api/config", s.putConfig)
	mux.HandleFunc("POST /api/config/padrao", s.resetConfig)
	mux.HandleFunc("GET /api/previa.png", s.preview)
	mux.HandleFunc("GET /api/portas", s.ports)
	mux.HandleFunc("POST /api/acao", s.action)
	mux.HandleFunc("POST /api/steam/testar", s.testSteam)
	mux.HandleFunc("GET /api/logs", s.logs)
	mux.HandleFunc("GET /api/info", s.info)
	mux.HandleFunc("GET /api/atualizacao", s.updateStatus)
	mux.HandleFunc("POST /api/atualizacao/verificar", s.checkUpdate)
	mux.HandleFunc("POST /api/atualizacao/instalar", s.installUpdate)
	s.srv = &http.Server{Handler: s.guard(mux), ReadHeaderTimeout: 5 * time.Second}
	err := s.srv.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) Shutdown(ctx context.Context) {
	if s.srv != nil {
		_ = s.srv.Shutdown(ctx)
	}
}

// guard só aceita requisições feitas para o próprio PC, e exige um cabeçalho
// próprio nas alterações (páginas de outros sites não conseguem enviá-lo).
func (s *Server) guard(next http.Handler) http.Handler {
	allowed := map[string]bool{
		fmt.Sprintf("127.0.0.1:%d", s.Port): true,
		fmt.Sprintf("localhost:%d", s.Port): true,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowed[r.Host] {
			http.Error(w, "host não permitido", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Header.Get("X-Bifrost") != "1" {
			http.Error(w, "requisição recusada", http.StatusForbidden)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"erro": msg})
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	data, _ := uiFS.ReadFile("ui/index.html")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'")
	_, _ = w.Write(data)
}

func (s *Server) state(w http.ResponseWriter, r *http.Request) { writeJSON(w, s.App.State()) }

func (s *Server) getConfig(w http.ResponseWriter, r *http.Request) { writeJSON(w, s.Store.Get()) }

func (s *Server) putConfig(w http.ResponseWriter, r *http.Request) {
	var c config.Config
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(&c); err != nil {
		writeErr(w, http.StatusBadRequest, "configuração inválida: "+err.Error())
		return
	}
	saved, err := s.Store.Set(c)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, saved)
}

func (s *Server) resetConfig(w http.ResponseWriter, r *http.Request) {
	cur := s.Store.Get()
	def := config.Default()
	// mantém o que é pessoal: credenciais da Steam e preferências gerais.
	def.Steam = cur.Steam
	def.General = cur.General
	// mantém os dispositivos (ID/nome/porta escolhidos), com o resto de fábrica.
	factory := def.Devices[0]
	devices := make([]config.DeviceConfig, len(cur.Devices))
	for i, d := range cur.Devices {
		nd := factory
		nd.ID, nd.Name, nd.Port = d.ID, d.Name, d.Port
		devices[i] = nd
	}
	def.Devices = devices
	saved, err := s.Store.Set(def)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, saved)
}

func (s *Server) preview(w http.ResponseWriter, r *http.Request) {
	data, id := s.App.PreviewPNG(r.URL.Query().Get("dispositivo"))
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("X-Frame", strconv.FormatUint(id, 10))
	_, _ = w.Write(data)
}

func (s *Server) ports(w http.ResponseWriter, r *http.Request) {
	ports, err := lcd.ListPorts()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ports == nil {
		ports = []lcd.PortInfo{}
	}
	writeJSON(w, ports)
}

func (s *Server) action(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string   `json:"acao"`
		Device string   `json:"dispositivo"`
		PIDs   []uint32 `json:"pids"`
		Link   string   `json:"link"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "pedido inválido")
		return
	}
	switch req.Action {
	case "proxima":
		s.App.Next(req.Device, 1)
	case "anterior":
		s.App.Next(req.Device, -1)
	case "pausar":
		s.App.SetPaused(req.Device, true)
	case "retomar":
		s.App.SetPaused(req.Device, false)
	case "reconectar":
		s.App.Reconnect(req.Device)
	case "encerrar_conflitos":
		if err := s.App.KillConflicts(req.PIDs); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	case "instalar_sensores", "remover_sensores":
		// A leitura por driver precisa de administrador uma única vez: o
		// processo elevado instala o driver e deixa o agente ligado.
		arg := "--instalar-sensores"
		if req.Action == "remover_sensores" {
			arg = "--remover-sensores"
		}
		if err := winutil.RunSelfElevated(arg); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	case "abrir_pasta_config":
		_ = winutil.OpenURL(dirOf(s.Store.Path()))
	case "abrir_log":
		_ = winutil.OpenURL(s.LogPath)
	case "abrir_link":
		u, ok := links[req.Link]
		if !ok {
			writeErr(w, http.StatusBadRequest, "link desconhecido")
			return
		}
		_ = winutil.OpenURL(u)
	default:
		writeErr(w, http.StatusBadRequest, "ação desconhecida")
		return
	}
	writeJSON(w, s.App.State())
}

// links que o painel pode abrir no navegador (lista fechada, por segurança).
var links = map[string]string{
	"steam_apikey":      "https://steamcommunity.com/dev/apikey",
	"steam_id":          "https://steamid.io/",
	"steam_privacidade": "https://steamcommunity.com/my/edit/settings",
	"repositorio":       "https://github.com/satty-br/Bifrost-screen",
	// Programas de monitoramento que o Bifrost também aproveita se já estiverem instalados.
	"lhm":    "https://github.com/LibreHardwareMonitor/LibreHardwareMonitor/releases",
	"hwinfo": "https://www.hwinfo.com/download/",
}

func dirOf(p string) string {
	if i := strings.LastIndexAny(p, `\/`); i > 0 {
		return p[:i]
	}
	return p
}

func (s *Server) testSteam(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Source string `json:"fonte"`
		Key    string `json:"api_key"`
		ID     string `json:"steam_id64"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "pedido inválido")
		return
	}
	if req.Source != "web" {
		persona, path, err := s.App.TestSteamLocal()
		if err != nil {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, map[string]string{"perfil": persona, "pasta": path})
		return
	}
	req.Key, req.ID = strings.TrimSpace(req.Key), strings.TrimSpace(req.ID)
	if req.Key == "" || req.ID == "" {
		writeErr(w, http.StatusBadRequest, "Preencha a API key e o SteamID64")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	name, err := s.App.TestSteam(ctx, req.Key, req.ID)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, map[string]string{"perfil": name})
}

func (s *Server) logs(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(s.LogPath)
	if err != nil {
		writeJSON(w, map[string]any{"linhas": []string{}})
		return
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > 200 {
		lines = lines[len(lines)-200:]
	}
	writeJSON(w, map[string]any{"linhas": lines})
}

func (s *Server) info(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"versao":              s.App.Version,
		"arquivo_config":      s.Store.Path(),
		"arquivo_log":         s.LogPath,
		"inicia_com_windows":  winutil.AutostartEnabled(),
		"sistema_operacional": runtime.GOOS,
		"sensores":            sysinfo.TempDriverStatus(),
	})
}

func (s *Server) updateStatus(w http.ResponseWriter, r *http.Request) {
	rel := s.App.Updater().Available()
	writeJSON(w, map[string]any{"disponivel": rel != nil, "release": rel, "erro": s.App.Updater().LastError()})
}

func (s *Server) checkUpdate(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	rel := s.App.Updater().CheckNow(ctx)
	writeJSON(w, map[string]any{"disponivel": rel != nil, "release": rel, "erro": s.App.Updater().LastError()})
}

func (s *Server) installUpdate(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	if err := s.App.InstallUpdate(ctx); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "instalando"})
	if s.OnRestart != nil {
		go func() {
			time.Sleep(500 * time.Millisecond)
			s.OnRestart()
		}()
	}
}

// LogWriter manda o log para arquivo (e o limita a ~1 MB).
func OpenLog(path string) (io.Writer, error) {
	if fi, err := os.Stat(path); err == nil && fi.Size() > 1<<20 {
		_ = os.Rename(path, path+".anterior")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	log.SetFlags(log.Ldate | log.Ltime)
	return f, nil
}
