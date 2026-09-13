// Package valorantapi lê a partida de Valorant ao vivo através da API local
// (não oficial) que o Riot Client expõe — diferente do League of Legends, a
// Riot NÃO documenta oficialmente uma "Live Client Data API" para o Valorant.
//
// O que existe é uma API interna reverse-engineered pela comunidade (ver
// https://valapidocs.techchrism.me/, mantida por techchrism e colaboradores):
//  1. O Riot Client grava um "lockfile" em
//     %LocalAppData%\Riot Games\Riot Client\Config\lockfile com o formato
//     "nome:pid:porta:senha:protocolo" enquanto está aberto.
//  2. Com a porta e a senha, dá pra pedir um token de autenticação local em
//     https://127.0.0.1:{porta}/entitlements/v1/token (Basic Auth
//     "riot:{senha}") — a resposta já traz o PUUID do jogador (campo
//     "subject") e o token usado nas chamadas seguintes.
//  3. https://127.0.0.1:{porta}/riotclient/region-locale dá a região da
//     conta; o "shard" (servidor) é derivado da região por uma tabela fixa.
//  4. Com token + PUUID + região/shard, dá pra perguntar pros servidores de
//     verdade da Riot (glz-{regiao}-1.{shard}.a.pvp.net) se o jogador está
//     numa partida (pregame ou em andamento) e pegar mapa/modo/agentes.
//
// Limitações importantes (documentar pro usuário, não é força de expressão):
//   - Não é oficial: a Riot não documenta nem dá suporte a isso, embora seja
//     uso comum e tolerado por ferramentas de overlay (o próprio FAQ do
//     valapidocs diz "as long as you use common sense... you won't get
//     banned").
//   - Os dados "ao vivo" disponíveis NÃO incluem abates/mortes/assistências/
//     dinheiro em tempo real como a API do LoL dá — só mapa, modo e agente
//     escolhido/selecionado. As partidas da Riot (pregame/coregame) só
//     expõem metadados da partida, não um placar ao vivo.
package valorantapi

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Validade: sem partida detectada por esse tempo, considera encerrada.
const Validade = 15 * time.Second

// intervaloAuth: token/região/PUUID não mudam com frequência — só busca de
// novo depois desse tempo (ou se uma chamada falhar por credencial inválida).
const intervaloAuth = 10 * time.Minute

// Phase indica em que fase da partida o jogador está.
type Phase string

const (
	PhasePreGame Phase = "pregame" // seleção de agente, antes do round 1
	PhaseInGame  Phase = "ingame"  // partida em andamento
)

// Match é o resumo da partida de Valorant em andamento.
type Match struct {
	UpdatedAt time.Time
	Phase     Phase
	Map       string // nome amigável ("Ascent"), ou o ID cru se desconhecido
	Mode      string // nome amigável ("Competitiva"), ou o ID cru se desconhecido
	Agent     string // nome amigável do agente escolhido/selecionado ("Jett"), vazio se ainda não escolheu
}

// Ativa diz se a leitura é de uma partida realmente em andamento (recente).
func (m Match) Ativa() bool {
	return m.Map != "" && time.Since(m.UpdatedAt) < Validade
}

// shardPorRegiao: o servidor (shard) é determinado pela região da conta —
// tabela documentada em https://valapidocs.techchrism.me/endpoint/pre-game-match.
var shardPorRegiao = map[string]string{
	"na": "na", "latam": "na", "br": "na",
	"eu": "eu", "ap": "ap", "kr": "kr",
}

// clientPlatform é o valor documentado que funciona pra qualquer PC Windows
// (base64 de um JSON fixo — não muda por instalação).
const clientPlatform = "ew0KCSJwbGF0Zm9ybVR5cGUiOiAiUEMiLA0KCSJwbGF0Zm9ybU9TIjogIldpbmRvd3MiLA0KCSJwbGF0Zm9ybU9TVmVyc2lvbiI6ICIxMC4wLjE5MDQyLjEuMjU2LjY0Yml0IiwNCgkicGxhdGZvcm1DaGlwc2V0IjogIlVua25vd24iDQp9"

type authState struct {
	accessToken string
	entitlement string
	puuid       string
	region      string
	shard       string
	clientVer   string
	at          time.Time
}

func (a authState) valid() bool {
	return a.accessToken != "" && a.puuid != "" && a.shard != "" && time.Since(a.at) < intervaloAuth
}

// Poller consulta a API local periodicamente enquanto estiver ligado.
type Poller struct {
	// localClient fala com o Riot Client em 127.0.0.1 (certificado
	// autoassinado, só localhost).
	localClient *http.Client
	// remoteClient fala com os servidores de verdade da Riot
	// (glz-*.a.pvp.net) — esses têm certificado válido de verdade, NÃO
	// deve pular verificação de TLS.
	remoteClient *http.Client

	// localBase/remoteBase, quando preenchidos (só nos testes), substituem
	// a porta do lockfile e o host glz-*.a.pvp.net por um servidor de teste.
	localBase  string
	remoteBase string

	authMu sync.Mutex
	auth   authState

	mu    sync.RWMutex
	match Match
}

// NovoPoller cria um poller parado; chame Start para ligar.
func NovoPoller() *Poller {
	return &Poller{
		localClient: &http.Client{
			Timeout: 3 * time.Second,
			// codeql[go/disabled-certificate-check]: API local do Riot Client em 127.0.0.1, certificado autoassinado (sem alternativa) — mesmo padrão usado pela API do LoL.
			Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, //nolint:gosec // API local do Riot Client, certificado autoassinado
		},
		remoteClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// Start consulta a cada interval, até ctx terminar.
func (p *Poller) Start(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.poll()
			}
		}
	}()
}

// Current devolve a última partida vista, ou Match{} se não tem nenhuma.
func (p *Poller) Current() Match {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.match
}

func (p *Poller) clear() {
	p.mu.Lock()
	p.match = Match{}
	p.mu.Unlock()
}

func (p *Poller) set(m Match) {
	m.UpdatedAt = time.Now()
	p.mu.Lock()
	p.match = m
	p.mu.Unlock()
}

func (p *Poller) poll() {
	lock, err := readLockfile()
	if err != nil {
		p.clear() // Riot Client fechado
		return
	}
	auth, err := p.ensureAuth(lock)
	if err != nil {
		p.clear()
		return
	}
	if m, ok := p.fetchCoreGame(auth); ok {
		p.set(m)
		return
	}
	if m, ok := p.fetchPreGame(auth); ok {
		p.set(m)
		return
	}
	p.clear() // Valorant fechado, ou fora de partida
}

// lockfile é o arquivo que o Riot Client grava com a porta/senha da API
// local enquanto está aberto.
type lockfile struct {
	port     string
	password string
}

func lockfilePath() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = os.Getenv("APPDATA") // fallback improvável, mas evita path vazio
	}
	return filepath.Join(base, "Riot Games", "Riot Client", "Config", "lockfile")
}

func readLockfile() (lockfile, error) {
	data, err := os.ReadFile(lockfilePath())
	if err != nil {
		return lockfile{}, err // Riot Client fechado (arquivo some quando fecha)
	}
	parts := strings.Split(strings.TrimSpace(string(data)), ":")
	if len(parts) < 4 {
		return lockfile{}, errors.New("lockfile em formato inesperado")
	}
	return lockfile{port: parts[2], password: parts[3]}, nil
}

func (p *Poller) localGet(lock lockfile, path string) ([]byte, int, error) {
	base := p.localBase
	if base == "" {
		base = fmt.Sprintf("https://127.0.0.1:%s", lock.port)
	}
	req, err := http.NewRequest(http.MethodGet, base+path, nil)
	if err != nil {
		return nil, 0, err
	}
	req.SetBasicAuth("riot", lock.password)
	resp, err := p.localClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return body, resp.StatusCode, err
}

// ensureAuth busca (ou reaproveita, se ainda válido) o token local, o PUUID
// e a região/shard do jogador.
func (p *Poller) ensureAuth(lock lockfile) (authState, error) {
	p.authMu.Lock()
	defer p.authMu.Unlock()
	if p.auth.valid() {
		return p.auth, nil
	}

	body, status, err := p.localGet(lock, "/entitlements/v1/token")
	if err != nil || status != http.StatusOK {
		return authState{}, fmt.Errorf("token de entitlements: status %d, err %v", status, err)
	}
	var tok struct {
		AccessToken string `json:"accessToken"`
		Token       string `json:"token"`
		Subject     string `json:"subject"`
	}
	if err := json.Unmarshal(body, &tok); err != nil || tok.AccessToken == "" || tok.Subject == "" {
		return authState{}, errors.New("resposta de entitlements inválida")
	}

	body, status, err = p.localGet(lock, "/riotclient/region-locale")
	if err != nil || status != http.StatusOK {
		return authState{}, fmt.Errorf("região do cliente: status %d, err %v", status, err)
	}
	var reg struct {
		Region string `json:"region"`
	}
	if err := json.Unmarshal(body, &reg); err != nil || reg.Region == "" {
		return authState{}, errors.New("resposta de região inválida")
	}
	shard, ok := shardPorRegiao[strings.ToLower(reg.Region)]
	if !ok {
		return authState{}, fmt.Errorf("região desconhecida: %q", reg.Region)
	}

	auth := authState{
		accessToken: tok.AccessToken,
		entitlement: tok.Token,
		puuid:       tok.Subject,
		region:      strings.ToLower(reg.Region),
		shard:       shard,
		clientVer:   clientVersion(),
		at:          time.Now(),
	}
	p.auth = auth
	return auth, nil
}

// clientVersion tenta ler a versão do cliente do log do jogo; se não achar,
// devolve uma versão genérica — o servidor aceita mesmo se estiver
// desatualizada, só usa isso pra telemetria.
func clientVersion() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return "release-01.00-shipping"
	}
	path := filepath.Join(base, "VALORANT", "Saved", "Logs", "ShooterGame.log")
	data, err := os.ReadFile(path)
	if err != nil {
		return "release-01.00-shipping"
	}
	// Procura uma linha "Build version: <versão>" perto do início do log.
	const marker = "Build version: "
	text := string(data)
	if len(text) > 8192 {
		text = text[:8192]
	}
	if i := strings.Index(text, marker); i >= 0 {
		rest := text[i+len(marker):]
		if end := strings.IndexAny(rest, "\r\n"); end >= 0 {
			return strings.TrimSpace(rest[:end])
		}
	}
	return "release-01.00-shipping"
}

func (p *Poller) remoteGet(auth authState, path string) ([]byte, int, error) {
	base := p.remoteBase
	if base == "" {
		base = fmt.Sprintf("https://glz-%s-1.%s.a.pvp.net", auth.region, auth.shard)
	}
	req, err := http.NewRequest(http.MethodGet, base+path, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+auth.accessToken)
	req.Header.Set("X-Riot-Entitlements-JWT", auth.entitlement)
	req.Header.Set("X-Riot-ClientPlatform", clientPlatform)
	req.Header.Set("X-Riot-ClientVersion", auth.clientVer)
	resp, err := p.remoteClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return body, resp.StatusCode, err
}

func (p *Poller) fetchCoreGame(auth authState) (Match, bool) {
	body, status, err := p.remoteGet(auth, "/core-game/v1/players/"+auth.puuid)
	if err != nil || status != http.StatusOK {
		return Match{}, false
	}
	var ref struct {
		MatchID string `json:"MatchID"`
	}
	if err := json.Unmarshal(body, &ref); err != nil || ref.MatchID == "" {
		return Match{}, false
	}
	body, status, err = p.remoteGet(auth, "/core-game/v1/matches/"+ref.MatchID)
	if err != nil || status != http.StatusOK {
		return Match{}, false
	}
	var m struct {
		MapID   string `json:"MapID"`
		ModeID  string `json:"ModeID"`
		Players []struct {
			Subject     string `json:"Subject"`
			CharacterID string `json:"CharacterID"`
		} `json:"Players"`
	}
	if err := json.Unmarshal(body, &m); err != nil {
		return Match{}, false
	}
	agent := ""
	for _, pl := range m.Players {
		if pl.Subject == auth.puuid {
			agent = agentName(pl.CharacterID)
			break
		}
	}
	return Match{Phase: PhaseInGame, Map: mapName(m.MapID), Mode: modeName(m.ModeID), Agent: agent}, true
}

func (p *Poller) fetchPreGame(auth authState) (Match, bool) {
	body, status, err := p.remoteGet(auth, "/pregame/v1/players/"+auth.puuid)
	if err != nil || status != http.StatusOK {
		return Match{}, false
	}
	var ref struct {
		MatchID string `json:"MatchID"`
	}
	if err := json.Unmarshal(body, &ref); err != nil || ref.MatchID == "" {
		return Match{}, false
	}
	body, status, err = p.remoteGet(auth, "/pregame/v1/matches/"+ref.MatchID)
	if err != nil || status != http.StatusOK {
		return Match{}, false
	}
	var m struct {
		MapID string `json:"MapID"`
		Mode  string `json:"Mode"`
		Teams []struct {
			Players []struct {
				Subject                 string `json:"Subject"`
				CharacterID             string `json:"CharacterID"`
				CharacterSelectionState string `json:"CharacterSelectionState"`
			} `json:"Players"`
		} `json:"Teams"`
	}
	if err := json.Unmarshal(body, &m); err != nil {
		return Match{}, false
	}
	agent := ""
	for _, team := range m.Teams {
		for _, pl := range team.Players {
			if pl.Subject == auth.puuid && pl.CharacterSelectionState != "" {
				agent = agentName(pl.CharacterID)
			}
		}
	}
	return Match{Phase: PhasePreGame, Map: mapName(m.MapID), Mode: modeName(m.Mode), Agent: agent}, true
}
