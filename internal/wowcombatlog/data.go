package wowcombatlog

import "fmt"

// difficultyNamesByID vem da tabela oficial de DifficultyID (retail) —
// só os valores mais comuns em masmorras/raides; o resto cai no fallback.
var difficultyNamesByID = map[string]string{
	"1":  "Normal",
	"2":  "Heroica",
	"3":  "10 jogadores",
	"4":  "25 jogadores",
	"5":  "10 jogadores (Heroica)",
	"6":  "25 jogadores (Heroica)",
	"7":  "Busca por Raide",
	"8":  "Mítica+",
	"9":  "40 jogadores",
	"11": "Cenário Heroico",
	"12": "Cenário Normal",
	"14": "Normal",
	"15": "Heroica",
	"16": "Mítica",
	"17": "Busca por Raide",
	"23": "Mítica",
	"24": "Viagem no Tempo",
}

func difficultyName(id string) string {
	if name, ok := difficultyNamesByID[id]; ok {
		return name
	}
	return fmt.Sprintf("Dificuldade %s", id)
}
