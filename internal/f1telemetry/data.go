package f1telemetry

import "fmt"

// trackNamesByID vem do apêndice "Track IDs" da especificação oficial de UDP
// Telemetry (mesmo ID em todos os jogos F1 22 em diante, novas pistas só
// somam no final da lista).
var trackNamesByID = map[int]string{
	0:  "Melbourne",
	1:  "Paul Ricard",
	2:  "Xangai",
	3:  "Sakhir (Bahrein)",
	4:  "Catalunya",
	5:  "Mônaco",
	6:  "Montreal",
	7:  "Silverstone",
	8:  "Hockenheim",
	9:  "Hungaroring",
	10: "Spa",
	11: "Monza",
	12: "Singapura",
	13: "Suzuka",
	14: "Abu Dhabi",
	15: "Texas (COTA)",
	16: "Brasil (Interlagos)",
	17: "Áustria",
	18: "Sochi",
	19: "México",
	20: "Baku (Azerbaijão)",
	21: "Sakhir Curto",
	22: "Silverstone Curto",
	23: "Texas Curto",
	24: "Suzuka Curto",
	25: "Hanói",
	26: "Zandvoort",
	27: "Ímola",
	28: "Portimão",
	29: "Jeddah",
	30: "Miami",
	31: "Las Vegas",
	32: "Losail (Catar)",
}

// sessionTypeNamesByID vem do apêndice "Session Types" da especificação.
var sessionTypeNamesByID = map[int]string{
	0:  "",
	1:  "Treino Livre 1",
	2:  "Treino Livre 2",
	3:  "Treino Livre 3",
	4:  "Treino Curto",
	5:  "Classificação 1",
	6:  "Classificação 2",
	7:  "Classificação 3",
	8:  "Classificação Curta",
	9:  "Classificação Única",
	10: "Sprint Shootout 1",
	11: "Sprint Shootout 2",
	12: "Sprint Shootout 3",
	13: "Sprint Shootout Curto",
	14: "Sprint Shootout Único",
	15: "Corrida",
	16: "Corrida 2",
	17: "Corrida 3",
	18: "Contrarrelógio",
}

func trackName(id int) string {
	if name, ok := trackNamesByID[id]; ok {
		return name
	}
	return fmt.Sprintf("Pista %d", id)
}

func sessionTypeName(id int) string {
	if name, ok := sessionTypeNamesByID[id]; ok {
		return name
	}
	return ""
}
