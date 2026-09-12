package gsi

import "encoding/json"

// cs2Payload é o formato do JSON que o CS2 manda (só os campos usados aqui;
// o payload real tem bem mais coisa, mas o encoding/json ignora o resto).
type cs2Payload struct {
	Provider struct {
		AppID int `json:"appid"`
	} `json:"provider"`
	Player struct {
		Team  string `json:"team"`
		State struct {
			Health int `json:"health"`
			Armor  int `json:"armor"`
			Money  int `json:"money"`
		} `json:"state"`
		MatchStats struct {
			Kills   int `json:"kills"`
			Assists int `json:"assists"`
			Deaths  int `json:"deaths"`
			MVPs    int `json:"mvps"`
		} `json:"match_stats"`
	} `json:"player"`
	Map struct {
		Mode   string `json:"mode"`
		Name   string `json:"name"`
		Phase  string `json:"phase"`
		Round  int    `json:"round"`
		TeamCT struct {
			Score int `json:"score"`
		} `json:"team_ct"`
		TeamT struct {
			Score int `json:"score"`
		} `json:"team_t"`
	} `json:"map"`
	Round struct {
		Bomb string `json:"bomb"`
	} `json:"round"`
}

// parseCS2 devolve (nil, false) quando o payload não é do CS2 (appid errado)
// ou não traz dados de mapa (ex.: só um heartbeat fora de partida).
func parseCS2(appid int, body []byte) (*CS2State, bool) {
	if appid != 730 {
		return nil, false
	}
	var p cs2Payload
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, false
	}
	if p.Map.Name == "" {
		return nil, false
	}
	return &CS2State{
		Map: p.Map.Name, Mode: p.Map.Mode, Phase: p.Map.Phase, Round: p.Map.Round,
		ScoreCT: p.Map.TeamCT.Score, ScoreT: p.Map.TeamT.Score,
		Team:      p.Player.Team,
		Kills:     p.Player.MatchStats.Kills,
		Deaths:    p.Player.MatchStats.Deaths,
		Assists:   p.Player.MatchStats.Assists,
		MVPs:      p.Player.MatchStats.MVPs,
		Health:    p.Player.State.Health,
		Armor:     p.Player.State.Armor,
		Money:     p.Player.State.Money,
		BombState: p.Round.Bomb,
	}, true
}
