package acctelemetry

// sessionTypeNamesByID vem do enum ACC_SESSION_TYPE (também usado, com o
// mesmo layout, pelo Assetto Corsa original — lá chamado AcSharedSessionType).
var sessionTypeNamesByID = map[int]string{
	-1: "",
	0:  "Treino",
	1:  "Classificação",
	2:  "Corrida",
	3:  "Volta rápida",
	4:  "Contrarrelógio",
	5:  "Drift",
	6:  "Arrancada",
	7:  "Hotstint",
	8:  "Superpole",
}

func sessionTypeName(id int) string {
	if name, ok := sessionTypeNamesByID[id]; ok {
		return name
	}
	return ""
}
