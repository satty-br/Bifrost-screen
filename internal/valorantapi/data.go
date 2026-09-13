package valorantapi

import "strings"

// agentNamesByID mapeia o UUID do agente (campo CharacterID nas respostas de
// pregame/coregame) pro nome exibido no jogo. Não muda com frequência (a
// Riot raramente reaproveita UUIDs), mas agentes novos exigem atualizar essa
// tabela — se um UUID não estiver aqui, agentName devolve o próprio UUID.
// Fonte: https://valorant-api.com/v1/agents (API pública de dados estáticos,
// diferente da API local/remota usada pra ler a partida ao vivo).
var agentNamesByID = map[string]string{
	"e370fa57-4757-3604-3648-499e1f642d3f": "Gekko",
	"dade69b4-4f5a-8528-247b-219e5a1facd6": "Fade",
	"5f8d3a7f-467b-97f3-062c-13acf203c006": "Breach",
	"cc8b64c8-4b25-4ff9-6e7f-37b4da43d235": "Deadlock",
	"b444168c-4e35-8076-db47-ef9bf368f384": "Tejo",
	"f94c3b30-42be-e959-889c-5aa313dba261": "Raze",
	"22697a3d-45bf-8dd7-4fec-84a9e28c69d7": "Chamber",
	"601dbbe7-43ce-be57-2a40-4abd24953621": "KAY/O",
	"6f2a04ca-43e0-be17-7f36-b3908627744d": "Skye",
	"117ed9e3-49f3-6512-3ccf-0cada7e3823b": "Cypher",
	"320b2a48-4d9b-a075-30f1-1f93a9b638fa": "Sova",
	"7c8a4701-4de6-9355-b254-e09bc2a34b72": "Miks",
	"1e58de9c-4950-5125-93e9-a0aee9f98746": "Killjoy",
	"95b78ed7-4637-86d9-7e41-71ba8c293152": "Harbor",
	"efba5359-4016-a1e5-7626-b1ae76895940": "Vyse",
	"707eab51-4836-f488-046a-cda6bf494859": "Viper",
	"eb93336a-449b-9c1b-0a54-a891f7921d69": "Phoenix",
	"92eeef5d-43b5-1d4a-8d03-b3927a09034b": "Veto",
	"41fb69c1-4189-7b37-f117-bcaf1e96f1bf": "Astra",
	"9f0d8ba9-4140-b941-57d3-a7ad57c6b417": "Brimstone",
	"0e38b510-41a8-5780-5e8f-568b2a4f2d6c": "Iso",
	"1dbf2edd-4729-0984-3115-daa5eed44993": "Clove",
	"bb2a4828-46eb-8cd1-e765-15848195d751": "Neon",
	"7f94d92c-4234-0a36-9646-3a87eb8b5c89": "Yoru",
	"df1cb487-4902-002e-5c17-d28e83e78588": "Waylay",
	"569fdd95-4d10-43ab-ca70-79becc718b46": "Sage",
	"a3bfb853-43b2-7238-a4f1-ad90e9e46bcc": "Reyna",
	"8e253930-4c05-31dd-1b6c-968525494517": "Omen",
	"add6443a-41bd-e414-f6ad-e58d267f4e95": "Jett",
}

// mapNamesByURL mapeia o "mapUrl" interno (campo MapID nas respostas de
// pregame/coregame, ex.: "/Game/Maps/Ascent/Ascent") pro nome exibido do
// mapa. Fonte: https://valorant-api.com/v1/maps (campo mapUrl).
var mapNamesByURL = map[string]string{
	"/Game/Maps/Ascent/Ascent":      "Ascent",
	"/Game/Maps/Bonsai/Bonsai":      "Split",
	"/Game/Maps/Canyon/Canyon":      "Fracture",
	"/Game/Maps/Duality/Duality":    "Bind",
	"/Game/Maps/Foxtrot/Foxtrot":    "Breeze",
	"/Game/Maps/HURM/HURM_Alley":    "District",
	"/Game/Maps/HURM/HURM_Bowl":     "Kasbah",
	"/Game/Maps/HURM/HURM_Helix":    "Drift",
	"/Game/Maps/HURM/HURM_HighTide": "Glitch",
	"/Game/Maps/HURM/HURM_Yard":     "Piazza",
	"/Game/Maps/Infinity/Infinity":  "Abyss",
	"/Game/Maps/Jam/Jam":            "Lotus",
	"/Game/Maps/Juliett/Juliett":    "Sunset",
	"/Game/Maps/Pitt/Pitt":          "Pearl",
	"/Game/Maps/Plummet/Plummet":    "Summit",
	"/Game/Maps/Port/Port":          "Icebox",
	"/Game/Maps/Rook/Rook":          "Corrode",
	"/Game/Maps/Triad/Triad":        "Haven",
}

// modeNamesByQueue mapeia o QueueID (fila de matchmaking) pro nome exibido
// do modo — mais confiável que tentar decodificar o ModeID interno (que é o
// mesmo "Bomb" tanto pra Competitiva quanto pra Não-classificatória). Usado
// pra partida em pré-jogo (tem QueueID/Mode explícito na resposta).
var modeNamesByQueue = map[string]string{
	"competitive": "Competitiva",
	"unrated":     "Não-classificatória",
	"spikerush":   "Spike Rush",
	"deathmatch":  "Deathmatch",
	"ggteam":      "Escalation",
	"onefa":       "Replication",
	"snowball":    "Guerra de Bolas de Neve",
	"swiftplay":   "Swiftplay",
	"":            "Personalizada",
}

// modeNamesByPath cobre o caso da partida em andamento, onde só vem o
// ModeID interno (ex.: "/Game/GameModes/Bomb/BombGameMode.BombGameMode_C") —
// nesse caso não dá pra distinguir Competitiva de Não-classificatória, só o
// tipo de modo de jogo.
var modeNamesByPath = map[string]string{
	"bomb":       "Bomba (Competitiva/Não-classificatória)",
	"quickbomb":  "Spike Rush",
	"deathmatch": "Deathmatch",
	"ggteam":     "Escalation",
	"onefa":      "Replication",
	"snowball":   "Guerra de Bolas de Neve",
	"swiftplay":  "Swiftplay",
}

// agentName devolve o nome amigável do agente, ou o próprio ID se for
// desconhecido (agente novo que ainda não está na tabela).
func agentName(id string) string {
	if name, ok := agentNamesByID[strings.ToLower(id)]; ok {
		return name
	}
	return id
}

// mapName devolve o nome amigável do mapa a partir do MapID interno
// ("/Game/Maps/Ascent/Ascent"), ou o próprio ID se for desconhecido.
func mapName(id string) string {
	// O MapID às vezes vem com o nome do arquivo repetido no final
	// ("/Game/Maps/Ascent/Ascent"); casos como "/Game/Maps/HURM/HURM_Alley/HURM_Alley"
	// têm um nível a mais — comparamos pelo prefixo até o penúltimo segmento.
	if name, ok := mapNamesByURL[id]; ok {
		return name
	}
	if idx := strings.LastIndex(id, "/"); idx > 0 {
		if name, ok := mapNamesByURL[id[:idx]]; ok {
			return name
		}
	}
	return id
}

// modeName devolve o nome amigável do modo a partir do QueueID (partida em
// pré-jogo) ou do ModeID interno (partida em andamento, ex.:
// "/Game/GameModes/Bomb/BombGameMode.BombGameMode_C"), ou o próprio valor se
// for desconhecido.
func modeName(raw string) string {
	if name, ok := modeNamesByQueue[strings.ToLower(raw)]; ok {
		return name
	}
	if !strings.Contains(raw, "/") {
		return raw
	}
	// ModeID interno: pega o penúltimo segmento do caminho
	// ("Bomb" em ".../Bomb/BombGameMode.BombGameMode_C").
	parts := strings.Split(raw, "/")
	if len(parts) < 2 {
		return raw
	}
	key := strings.ToLower(parts[len(parts)-2])
	if name, ok := modeNamesByPath[key]; ok {
		return name
	}
	return raw
}
