// Package rtss lê o FPS em tempo real do jogo em foco pela memória
// compartilhada do RivaTuner Statistics Server (RTSS) — o componente de FPS
// do MSI Afterburner, e também usado sozinho por muita gente. Não precisa de
// nenhuma configuração: o RTSS já publica isso pra qualquer overlay conseguir
// ler (é assim que Discord, Steam overlay etc. mostram o FPS de outros).
package rtss

import "time"

// Leitura é o FPS do jogo que o RTSS está monitorando no momento.
type Leitura struct {
	Processo string // nome do executável (ex.: "cs2.exe")
	FPS      float64
}

// ValidadeLeitura: se o RTSS não estiver rodando, Ler devolve erro; isso aqui
// só existe pra quem quiser decidir quando parar de confiar num valor em cache.
const ValidadeLeitura = 3 * time.Second
