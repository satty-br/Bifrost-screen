package gsi

import "encoding/json"

// dota2Payload é o formato do JSON que o Dota 2 manda (só os campos usados
// aqui, do jogador que está de fato jogando — não dos espectados).
type dota2Payload struct {
	Provider struct {
		AppID int `json:"appid"`
	} `json:"provider"`
	Map struct {
		GameTime     int    `json:"game_time"`
		RadiantScore int    `json:"radiant_score"`
		DireScore    int    `json:"dire_score"`
		GameState    string `json:"game_state"`
	} `json:"map"`
	Player struct {
		Kills    int `json:"kills"`
		Deaths   int `json:"deaths"`
		Assists  int `json:"assists"`
		LastHits int `json:"last_hits"`
		Denies   int `json:"denies"`
		GPM      int `json:"gpm"`
		XPM      int `json:"xpm"`
	} `json:"player"`
	Hero struct {
		Name  string `json:"name"`
		Level int    `json:"level"`
		Alive bool   `json:"alive"`
	} `json:"hero"`
}

// parseDota2 devolve (nil, false) quando o payload não é do Dota 2 (appid
// errado) ou não traz dados de partida (ex.: só um heartbeat no menu).
func parseDota2(appid int, body []byte) (*Dota2State, bool) {
	if appid != 570 {
		return nil, false
	}
	var p dota2Payload
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, false
	}
	if p.Hero.Name == "" && p.Map.GameState == "" {
		return nil, false
	}
	return &Dota2State{
		GameTime:     p.Map.GameTime,
		RadiantScore: p.Map.RadiantScore,
		DireScore:    p.Map.DireScore,
		Hero:         heroDisplayName(p.Hero.Name),
		Level:        p.Hero.Level,
		Kills:        p.Player.Kills,
		Deaths:       p.Player.Deaths,
		Assists:      p.Player.Assists,
		GPM:          p.Player.GPM,
		XPM:          p.Player.XPM,
		LastHits:     p.Player.LastHits,
		Denies:       p.Player.Denies,
		Alive:        p.Hero.Alive,
	}, true
}

// heroDisplayName tira o prefixo interno do jogo ("npc_dota_hero_axe" -> "axe"),
// já que o nome bonito de cada herói não vem no payload de GSI.
func heroDisplayName(internal string) string {
	const prefix = "npc_dota_hero_"
	if len(internal) > len(prefix) && internal[:len(prefix)] == prefix {
		return internal[len(prefix):]
	}
	return internal
}
