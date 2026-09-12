// Package gsi recebe os dados de partida ao vivo do CS2 e do Dota 2 pelo
// recurso oficial da Valve, "Game State Integration": o jogo lê um arquivo
// .cfg na própria pasta dele e passa a mandar, sozinho, um JSON por HTTP toda
// vez que algo muda (kills, placar, vida, bomba etc.). Não precisa de
// injeção, leitura de memória nem burlar nada — é o mesmo mecanismo que
// overlays como o HLTV/GosuGamers usam.
//
// Documentação oficial:
//   - CS2/CS:GO: https://developer.valvesoftware.com/wiki/Counter-Strike:_Global_Offensive_Game_State_Integration
//   - Dota 2:    https://developer.valvesoftware.com/wiki/Dota_2_Workshop_Tools/Game_State_Integration
package gsi

import "time"

// DefaultPort é a porta fixa (só em 127.0.0.1) em que o Bifrost escuta os
// POSTs de GSI. Fixa de propósito: o .cfg escrito na pasta do jogo aponta
// pra essa porta, e ela precisa continuar igual entre reinícios do Bifrost
// pra não obrigar a reiniciar o jogo toda vez.
const DefaultPort = 47018

// Validade: se o jogo parar de mandar atualização por esse tempo (fechou,
// saiu da partida...), a leitura é considerada velha e o Bifrost volta a
// mostrar as estatísticas da conta.
const Validade = 15 * time.Second

// Match é o resumo da partida ao vivo mostrado no painel/tela. Só um dos
// blocos (CS2 ou Dota2) vem preenchido, conforme qual jogo mandou por último.
type Match struct {
	Game      string // "cs2", "dota2" ou "" (nenhuma partida ao vivo)
	UpdatedAt time.Time
	CS2       *CS2State
	Dota2     *Dota2State
}

// Ativa diz se a leitura é de uma partida realmente em andamento (recente).
func (m Match) Ativa() bool {
	return m.Game != "" && time.Since(m.UpdatedAt) < Validade
}

// CS2State é o que dá pra saber da partida de CS2/CS:GO em andamento.
type CS2State struct {
	Map       string
	Mode      string
	Phase     string // "warmup", "live", "gameover"...
	Round     int
	ScoreCT   int
	ScoreT    int
	Team      string // "CT" ou "T"
	Kills     int
	Deaths    int
	Assists   int
	MVPs      int
	Health    int
	Armor     int
	Money     int
	BombState string // "planted", "defused", "exploded" (vazio = nada acontecendo)
}

// Dota2State é o que dá pra saber da partida de Dota 2 em andamento.
type Dota2State struct {
	GameTime     int // segundos desde o início da partida (pode ser negativo, pré-jogo)
	RadiantScore int
	DireScore    int
	Hero         string
	Level        int
	Kills        int
	Deaths       int
	Assists      int
	GPM          int
	XPM          int
	LastHits     int
	Denies       int
	Alive        bool
}
