// Package lolapi lê a partida de League of Legends ao vivo pela API local
// oficial da Riot ("Live Client Data API"): o próprio cliente do jogo expõe
// https://127.0.0.1:2999/liveclientdata/allgamedata enquanto uma partida está
// rolando — sem precisar configurar nada, ao contrário do GSI da Valve.
//
// Documentação oficial: https://developer.riotgames.com/docs/lol#game-client-api
//
// O certificado dessa API é autoassinado (é só localhost, nunca sai da
// máquina), então o cliente HTTP precisa ignorar a verificação — é assim que
// a própria Riot documenta o uso, e todo overlay de LoL faz a mesma coisa.
package lolapi

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"
)

const url = "https://127.0.0.1:2999/liveclientdata/allgamedata"

// Validade: sem resposta da API por esse tempo, a partida é considerada
// encerrada (o cliente do LoL fecha essa API assim que sai da partida).
const Validade = 10 * time.Second

// Match é o resumo da partida de LoL em andamento.
type Match struct {
	UpdatedAt   time.Time
	GameTime    float64 // segundos desde o início
	Champion    string
	Level       int
	Kills       int
	Deaths      int
	Assists     int
	CreepScore  int
	CurrentGold int
	Team        string // "ORDER" ou "CHAOS"
}

// Ativa diz se a leitura é de uma partida realmente em andamento (recente).
func (m Match) Ativa() bool {
	return m.Champion != "" && time.Since(m.UpdatedAt) < Validade
}

// Poller consulta a API periodicamente enquanto estiver ligado.
type Poller struct {
	client *http.Client

	mu    sync.RWMutex
	match Match
}

// NovoPoller cria um poller parado; chame Start para ligar.
func NovoPoller() *Poller {
	return &Poller{
		client: &http.Client{
			Timeout:   2 * time.Second,
			Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, //nolint:gosec // API local da Riot, certificado autoassinado documentado oficialmente
		},
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

// Current devolve a última partida vista, ou Match{} se não tem nenhuma
// (LoL fechado, ou fora de partida).
func (p *Poller) Current() Match {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.match
}

func (p *Poller) poll() {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return
	}
	resp, err := p.client.Do(req)
	if err != nil {
		// LoL fechado ou fora de partida: a API nem responde.
		p.mu.Lock()
		p.match = Match{}
		p.mu.Unlock()
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		p.mu.Lock()
		p.match = Match{}
		p.mu.Unlock()
		return
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return
	}
	m, ok := parse(body)
	if !ok {
		return
	}
	p.mu.Lock()
	p.match = m
	p.mu.Unlock()
}

type liveClientPayload struct {
	ActivePlayer struct {
		SummonerName string  `json:"summonerName"`
		CurrentGold  float64 `json:"currentGold"`
	} `json:"activePlayer"`
	AllPlayers []struct {
		ChampionName string `json:"championName"`
		SummonerName string `json:"summonerName"`
		Team         string `json:"team"`
		Level        int    `json:"level"`
		Scores       struct {
			Kills      int `json:"kills"`
			Deaths     int `json:"deaths"`
			Assists    int `json:"assists"`
			CreepScore int `json:"creepScore"`
		} `json:"scores"`
	} `json:"allPlayers"`
	GameData struct {
		GameTime float64 `json:"gameTime"`
	} `json:"gameData"`
}

// parse acha, entre allPlayers, o jogador que corresponde ao activePlayer
// (é o único jeito de saber qual é "eu": o bloco activePlayer não traz
// kills/deaths/assists, só allPlayers[].scores traz).
func parse(body []byte) (Match, bool) {
	var p liveClientPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return Match{}, false
	}
	me := p.ActivePlayer.SummonerName
	if me == "" {
		return Match{}, false
	}
	for _, pl := range p.AllPlayers {
		if pl.SummonerName != me {
			continue
		}
		return Match{
			UpdatedAt:   time.Now(),
			GameTime:    p.GameData.GameTime,
			Champion:    pl.ChampionName,
			Level:       pl.Level,
			Kills:       pl.Scores.Kills,
			Deaths:      pl.Scores.Deaths,
			Assists:     pl.Scores.Assists,
			CreepScore:  pl.Scores.CreepScore,
			CurrentGold: int(p.ActivePlayer.CurrentGold),
			Team:        pl.Team,
		}, true
	}
	return Match{}, false
}
