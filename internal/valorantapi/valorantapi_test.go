package valorantapi

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAgentMapModeNames(t *testing.T) {
	if got := agentName("add6443a-41bd-e414-f6ad-e58d267f4e95"); got != "Jett" {
		t.Errorf("agentName(Jett) = %q", got)
	}
	if got := agentName("00000000-0000-0000-0000-000000000000"); got != "00000000-0000-0000-0000-000000000000" {
		t.Errorf("agente desconhecido deveria devolver o próprio ID, veio %q", got)
	}
	if got := mapName("/Game/Maps/Ascent/Ascent"); got != "Ascent" {
		t.Errorf("mapName(Ascent) = %q", got)
	}
	if got := mapName("/Game/Maps/HURM/HURM_Alley/HURM_Alley"); got != "District" {
		t.Errorf("mapName(District, caminho com nível extra) = %q", got)
	}
	if got := mapName("/Game/Maps/Unknown/Unknown"); got != "/Game/Maps/Unknown/Unknown" {
		t.Errorf("mapa desconhecido deveria devolver o próprio ID, veio %q", got)
	}
	if got := modeName("competitive"); got != "Competitiva" {
		t.Errorf("modeName(competitive) = %q", got)
	}
	if got := modeName("/Game/GameModes/Bomb/BombGameMode.BombGameMode_C"); got != "Bomba (Competitiva/Não-classificatória)" {
		t.Errorf("modeName(ModeID Bomb) = %q", got)
	}
}

func TestMatchAtiva(t *testing.T) {
	m := Match{Map: "Ascent", UpdatedAt: time.Now()}
	if !m.Ativa() {
		t.Error("partida recente com mapa preenchido deveria estar ativa")
	}
	if (Match{}).Ativa() {
		t.Error("partida vazia não deveria estar ativa")
	}
	old := Match{Map: "Ascent", UpdatedAt: time.Now().Add(-time.Hour)}
	if old.Ativa() {
		t.Error("partida velha não deveria estar ativa")
	}
}

func TestReadLockfile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", dir)
	cfgDir := filepath.Join(dir, "Riot Games", "Riot Client", "Config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "lockfile"), []byte("Riot Client:12345:54321:senha-secreta:https"), 0o644); err != nil {
		t.Fatal(err)
	}
	lock, err := readLockfile()
	if err != nil {
		t.Fatalf("readLockfile: %v", err)
	}
	if lock.port != "54321" || lock.password != "senha-secreta" {
		t.Errorf("lockfile parseado errado: %+v", lock)
	}
}

func TestReadLockfileAusente(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	if _, err := readLockfile(); err == nil {
		t.Error("lockfile ausente (Riot Client fechado) deveria dar erro")
	}
}

// TestPollerFluxoCompleto simula o Riot Client (token/região) e os
// servidores glz da Riot (pregame/coregame) com servidores de teste, e
// confere que o poller monta a Match corretamente em cada fase.
func TestPollerFluxoCompleto(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", dir)
	cfgDir := filepath.Join(dir, "Riot Games", "Riot Client", "Config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}

	const puuid = "11111111-1111-1111-1111-111111111111"

	local := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "riot" || pass != "senha-secreta" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/entitlements/v1/token":
			fmt.Fprintf(w, `{"accessToken":"tok-123","token":"ent-456","subject":%q}`, puuid)
		case "/riotclient/region-locale":
			_, _ = w.Write([]byte(`{"region":"br","locale":"pt-BR"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer local.Close()
	_, port, err := net.SplitHostPort(local.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "lockfile"), []byte(fmt.Sprintf("Riot Client:1:%s:senha-secreta:https", port)), 0o644); err != nil {
		t.Fatal(err)
	}

	var coreGameActive, preGameActive bool
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok-123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if got := r.Header.Get("X-Riot-Entitlements-JWT"); got != "ent-456" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/core-game/v1/players/"+puuid:
			if !coreGameActive {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			fmt.Fprint(w, `{"MatchID":"match-1"}`)
		case r.URL.Path == "/core-game/v1/matches/match-1":
			fmt.Fprintf(w, `{"MapID":"/Game/Maps/Ascent/Ascent","ModeID":"/Game/GameModes/Bomb/BombGameMode.BombGameMode_C","Players":[{"Subject":%q,"CharacterID":"add6443a-41bd-e414-f6ad-e58d267f4e95"}]}`, puuid)
		case r.URL.Path == "/pregame/v1/players/"+puuid:
			if !preGameActive {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			fmt.Fprint(w, `{"MatchID":"pre-1"}`)
		case r.URL.Path == "/pregame/v1/matches/pre-1":
			fmt.Fprintf(w, `{"MapID":"/Game/Maps/Bonsai/Bonsai","Mode":"unrated","Teams":[{"Players":[{"Subject":%q,"CharacterID":"eb93336a-449b-9c1b-0a54-a891f7921d69","CharacterSelectionState":"selected"}]}]}`, puuid)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer remote.Close()

	p := NovoPoller()
	p.localClient = &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}} //nolint:gosec // servidor de teste local
	p.remoteBase = remote.URL

	// Fora de partida: nem pregame nem coregame respondem 200.
	p.poll()
	if p.Current().Ativa() {
		t.Fatalf("não deveria ter partida ativa ainda, veio %+v", p.Current())
	}

	// Pré-jogo (seleção de agente): coregame ainda 404, pregame já responde.
	preGameActive = true
	p.poll()
	m := p.Current()
	if !m.Ativa() || m.Phase != PhasePreGame || m.Map != "Split" || m.Mode != "Não-classificatória" || m.Agent != "Phoenix" {
		t.Fatalf("pré-jogo errado: %+v", m)
	}

	// Partida em andamento.
	coreGameActive = true
	p.poll()
	m = p.Current()
	if !m.Ativa() || m.Phase != PhaseInGame || m.Map != "Ascent" || m.Agent != "Jett" {
		t.Fatalf("partida em andamento errada: %+v", m)
	}

	// Riot Client fecha: sem lockfile, limpa a partida.
	if err := os.Remove(filepath.Join(cfgDir, "lockfile")); err != nil {
		t.Fatal(err)
	}
	p.poll()
	if p.Current().Ativa() {
		t.Error("sem lockfile (Riot Client fechado) deveria limpar a partida")
	}
}
