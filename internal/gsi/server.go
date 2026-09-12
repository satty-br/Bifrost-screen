package gsi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

// Server recebe os POSTs de Game State Integration do CS2 e do Dota 2 num
// único endpoint local — cada payload já diz de qual jogo é (provider.appid).
type Server struct {
	token string

	mu    sync.RWMutex
	match Match

	httpSrv *http.Server
	port    int
}

// NovoServer cria um servidor parado, com o token de autenticação dado (o
// mesmo token vai no .cfg escrito na pasta do jogo — assim só o próprio
// CS2/Dota2 desta máquina consegue mandar dados pra cá). Se token vier vazio,
// um novo é gerado na hora.
func NovoServer(token string) *Server {
	if token == "" {
		token = randomToken()
	}
	return &Server{token: token}
}

func randomToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "bifrost" // extremamente improvável, mas não trava o app por isso
	}
	return hex.EncodeToString(b)
}

// Token devolve o token atual (pro .cfg escrito na pasta do jogo).
func (s *Server) Token() string { return s.token }

// Porta devolve a porta em que o servidor está escutando (0 se parado).
func (s *Server) Porta() int { return s.port }

// Start liga o servidor HTTP em 127.0.0.1:port (0 = o sistema escolhe uma porta livre).
func (s *Server) Start(port int) error {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/gsi", s.handle)
	s.httpSrv = &http.Server{Handler: mux}
	s.port = ln.Addr().(*net.TCPAddr).Port
	go s.httpSrv.Serve(ln)
	return nil
}

// Stop desliga o servidor.
func (s *Server) Stop() {
	if s.httpSrv != nil {
		_ = s.httpSrv.Close()
	}
}

// Current devolve a última partida recebida, ou Match{} se estiver velha
// demais (o jogo fechou ou saiu da partida — ver Match.Ativa).
func (s *Server) Current() Match {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.match
}

type gsiEnvelope struct {
	Provider struct {
		AppID int `json:"appid"`
	} `json:"provider"`
	Auth struct {
		Token string `json:"token"`
	} `json:"auth"`
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var env gsiEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if env.Auth.Token != "" && env.Auth.Token != s.token {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	if cs2, ok := parseCS2(env.Provider.AppID, body); ok {
		s.mu.Lock()
		s.match = Match{Game: "cs2", UpdatedAt: time.Now(), CS2: cs2}
		s.mu.Unlock()
	} else if dota, ok := parseDota2(env.Provider.AppID, body); ok {
		s.mu.Lock()
		s.match = Match{Game: "dota2", UpdatedAt: time.Now(), Dota2: dota}
		s.mu.Unlock()
	}
	w.WriteHeader(http.StatusOK)
}

var errAppNaoInstalado = errors.New("jogo não encontrado nas bibliotecas da Steam")
