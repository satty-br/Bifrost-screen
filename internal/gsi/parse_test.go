package gsi

import "testing"

// Payload real de exemplo do CS2 (formato documentado pela Valve), com o
// necessário pra conferir o parser: mapa em andamento, placar, stats e bomba.
const cs2SamplePayload = `{
  "provider": {"name":"Counter-Strike 2","appid":730,"version":13800,"steamid":"76561198000000000","timestamp":1700000000},
  "player": {
    "steamid":"76561198000000000","name":"jogador","team":"CT",
    "activity":"playing",
    "state": {"health":72,"armor":100,"helmet":true,"flashed":0,"smoked":0,"burning":0,"money":3400,"round_kills":1,"round_killhs":0,"equip_value":4750},
    "match_stats": {"kills":14,"assists":3,"deaths":9,"mvps":2,"score":31}
  },
  "map": {
    "mode":"competitive","name":"de_mirage","phase":"live","round":18,
    "team_ct": {"score":9,"consecutive_round_losses":1,"timeouts_remaining":1,"matches_won_this_series":0},
    "team_t": {"score":8,"consecutive_round_losses":0,"timeouts_remaining":1,"matches_won_this_series":0}
  },
  "round": {"phase":"live","bomb":"planted"}
}`

func TestParseCS2(t *testing.T) {
	st, ok := parseCS2(730, []byte(cs2SamplePayload))
	if !ok {
		t.Fatal("esperava parsear com sucesso")
	}
	want := CS2State{
		Map: "de_mirage", Mode: "competitive", Phase: "live", Round: 18,
		ScoreCT: 9, ScoreT: 8, Team: "CT",
		Kills: 14, Deaths: 9, Assists: 3, MVPs: 2,
		Health: 72, Armor: 100, Money: 3400,
		BombState: "planted",
	}
	if *st != want {
		t.Errorf("parseCS2() = %+v, queria %+v", *st, want)
	}
}

func TestParseCS2AppIDErrado(t *testing.T) {
	if _, ok := parseCS2(570, []byte(cs2SamplePayload)); ok {
		t.Error("não devia parsear payload de CS2 com appid do Dota2")
	}
}

func TestParseCS2SemMapa(t *testing.T) {
	// Heartbeat fora de partida: só o provider, sem map/player de verdade.
	const heartbeat = `{"provider":{"name":"Counter-Strike 2","appid":730,"version":13800,"steamid":"1","timestamp":1}}`
	if _, ok := parseCS2(730, []byte(heartbeat)); ok {
		t.Error("heartbeat sem dados de mapa não devia virar partida ativa")
	}
}

// Payload real de exemplo do Dota 2 (formato documentado pela Valve).
const dota2SamplePayload = `{
  "provider": {"name":"Dota 2","appid":570,"version":47,"timestamp":1700000000},
  "map": {"name":"start","matchid":"7000000000","game_time":1245,"clock_time":1240,"daytime":true,"game_state":"DOTA_GAMERULES_STATE_GAME_IN_PROGRESS","radiant_score":12,"dire_score":7,"paused":false},
  "player": {"steamid":"76561198000000000","name":"jogador","activity":"playing","kills":6,"deaths":2,"assists":9,"last_hits":140,"denies":8,"kill_streak":0,"gold":1850,"gold_reliable":600,"gold_unreliable":1250,"gpm":510,"xpm":602},
  "hero": {"id":2,"name":"npc_dota_hero_axe","level":14,"alive":true,"respawn_seconds":0,"health_percent":88,"mana_percent":40}
}`

func TestParseDota2(t *testing.T) {
	st, ok := parseDota2(570, []byte(dota2SamplePayload))
	if !ok {
		t.Fatal("esperava parsear com sucesso")
	}
	want := Dota2State{
		GameTime: 1245, RadiantScore: 12, DireScore: 7,
		Hero: "axe", Level: 14,
		Kills: 6, Deaths: 2, Assists: 9, GPM: 510, XPM: 602,
		LastHits: 140, Denies: 8,
		Alive: true,
	}
	if *st != want {
		t.Errorf("parseDota2() = %+v, queria %+v", *st, want)
	}
}

func TestParseDota2AppIDErrado(t *testing.T) {
	if _, ok := parseDota2(730, []byte(dota2SamplePayload)); ok {
		t.Error("não devia parsear payload de Dota2 com appid do CS2")
	}
}

func TestHeroDisplayName(t *testing.T) {
	casos := map[string]string{
		"npc_dota_hero_axe":          "axe",
		"npc_dota_hero_antimage":     "antimage",
		"npc_dota_hero_shadow_fiend": "shadow_fiend",
		"":                           "",
		"sem_prefixo":                "sem_prefixo",
	}
	for in, want := range casos {
		if got := heroDisplayName(in); got != want {
			t.Errorf("heroDisplayName(%q) = %q, queria %q", in, got, want)
		}
	}
}
