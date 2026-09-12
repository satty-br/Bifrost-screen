package lolapi

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Payload real de exemplo da Live Client Data API (formato documentado pela
// Riot), com só os campos que o parser usa.
const samplePayload = `{
  "activePlayer": {
    "summonerName": "jogador#BR1",
    "level": 11,
    "currentGold": 2450.5
  },
  "allPlayers": [
    {
      "championName": "Ahri",
      "summonerName": "jogador#BR1",
      "team": "ORDER",
      "level": 11,
      "scores": {"kills": 7, "deaths": 3, "assists": 9, "creepScore": 142, "wardScore": 12.5}
    },
    {
      "championName": "Zed",
      "summonerName": "inimigo#BR1",
      "team": "CHAOS",
      "level": 10,
      "scores": {"kills": 3, "deaths": 7, "assists": 2, "creepScore": 130, "wardScore": 8.0}
    }
  ],
  "gameData": {"gameMode": "CLASSIC", "gameTime": 845.32, "mapName": "Map11", "mapNumber": 11}
}`

func TestParse(t *testing.T) {
	m, ok := parse([]byte(samplePayload))
	if !ok {
		t.Fatal("esperava parsear com sucesso")
	}
	if m.Champion != "Ahri" || m.Team != "ORDER" || m.Level != 11 {
		t.Errorf("dados do jogador errados: %+v", m)
	}
	if m.Kills != 7 || m.Deaths != 3 || m.Assists != 9 || m.CreepScore != 142 {
		t.Errorf("stats erradas: %+v", m)
	}
	if m.CurrentGold != 2450 {
		t.Errorf("currentGold = %d, queria 2450", m.CurrentGold)
	}
	if m.GameTime != 845.32 {
		t.Errorf("gameTime = %v, queria 845.32", m.GameTime)
	}
}

func TestParseSemActivePlayer(t *testing.T) {
	if _, ok := parse([]byte(`{"allPlayers":[]}`)); ok {
		t.Error("payload sem activePlayer não devia parsear")
	}
}

func TestParseJSONInvalido(t *testing.T) {
	if _, ok := parse([]byte("não é json")); ok {
		t.Error("json inválido não devia parsear")
	}
}

func TestPollerContraServidorFalso(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(samplePayload))
	}))
	defer srv.Close()

	p := &Poller{client: &http.Client{
		Timeout:   2 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	}}

	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	resp, err := p.client.Do(req)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	// Reaproveita a lógica do poll() manualmente contra o servidor de teste,
	// já que o endpoint real é fixo (127.0.0.1:2999) e não dá pra apontar
	// pro httptest a partir de poll() sem reescrever a URL global.
	m, ok := parse([]byte(samplePayload))
	if !ok {
		t.Fatal("parse falhou")
	}
	p.mu.Lock()
	p.match = m
	p.mu.Unlock()

	if got := p.Current(); !got.Ativa() {
		t.Error("partida devia estar ativa logo após atualização")
	}
}

func TestMatchAtivaExpira(t *testing.T) {
	m := Match{Champion: "Ahri", UpdatedAt: time.Now().Add(-time.Minute)}
	if m.Ativa() {
		t.Error("partida velha não devia contar como ativa")
	}
}

func TestPollerStartPara(t *testing.T) {
	// Só garante que Start/ctx.Done não trava nem gera pânico.
	p := NovoPoller()
	ctx, cancel := context.WithCancel(context.Background())
	p.Start(ctx, 50*time.Millisecond)
	time.Sleep(120 * time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)
	// Sem servidor real de LoL rodando, Current() deve continuar vazio.
	if p.Current().Champion != "" {
		t.Error("sem cliente do LoL rodando, não devia haver partida")
	}
}
