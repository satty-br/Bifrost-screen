// previa gera PNGs das telas para conferir o visual sem a tela física ligada.
//
// Uso: go run ./tools/previa <pasta-de-saida>
package main

import (
	"image/png"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/satty-br/Bifrost-screen/internal/config"
	"github.com/satty-br/Bifrost-screen/internal/i18n"
	"github.com/satty-br/Bifrost-screen/internal/render"
	"github.com/satty-br/Bifrost-screen/internal/steam"
	"github.com/satty-br/Bifrost-screen/internal/sysinfo"
)

func main() {
	out := "previa"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		log.Fatal(err)
	}

	cfg := config.Default()
	sys := sysinfo.Stats{
		CPU: 51, GPU: 28, CPUTemp: 52, GPUTemp: 64,
		RAMUsed: 19 << 30, RAMTotal: 32 << 30,
		DiskUsed: 700 << 30, DiskTotal: 1000 << 30,
		NetDown: 1_500_000, NetUp: 220_000,
		Uptime:     5*time.Hour + 12*time.Minute,
		CPUHistory: []float64{20, 35, 28, 44, 51, 60, 48, 51},
	}
	semTemp := sys
	semTemp.CPUTemp, semTemp.GPUTemp = -1, -1

	in := render.Input{
		Now:   time.Now(),
		Cfg:   cfg,
		Lang:  i18n.PT,
		Steam: steam.Status{Playing: true, Name: "Helldivers 2", TotalMinutes: 4210, TwoWeeksMinutes: 380, SessionStart: time.Now().Add(-95 * time.Minute)},
	}

	casos := []struct {
		nome string
		tela config.Screen
		st   sysinfo.Stats
		w, h int
	}{
		{"sistema_retrato", config.ScreenSystem, sys, 320, 480},
		{"sistema_paisagem", config.ScreenSystem, sys, 480, 320},
		{"sistema_sem_temp", config.ScreenSystem, semTemp, 320, 480},
		{"jogo_retrato", config.ScreenGame, sys, 320, 480},
		{"jogo_paisagem", config.ScreenGame, sys, 480, 320},
	}
	for _, c := range casos {
		in.System = c.st
		img := render.Draw(c.tela, c.w, c.h, in)
		f, err := os.Create(filepath.Join(out, c.nome+".png"))
		if err != nil {
			log.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			log.Fatal(err)
		}
		f.Close()
	}

	// telas de partida ao vivo (GSI/LoL) — não dependem do Steam.Playing.
	in.FPS = 143
	in.System = sys
	cs2Stats := []render.LiveStat{{Label: "K/D/A", Value: "14/9/3"}, {Label: "Vida", Value: "72"}, {Label: "Armadura", Value: "100"}, {Label: "Dinheiro", Value: "$3400"}}
	dota2Stats := []render.LiveStat{{Label: "K/D/A", Value: "6/2/9"}, {Label: "CS", Value: "140/8"}, {Label: "Ouro/min", Value: "510"}, {Label: "XP/min", Value: "602"}}
	lolStats := []render.LiveStat{{Label: "K/D/A", Value: "7/3/9"}, {Label: "CS", Value: "142"}, {Label: "Ouro", Value: "2450"}}
	vivos := []struct {
		nome string
		lm   render.LiveMatch
		w, h int
	}{
		{"jogo_cs2_retrato", render.LiveMatch{Active: true, Game: "CS2", Title: "de_mirage", Sub: "Round 18 · live · CT", Score: "9 - 8", Alert: "Bomba plantada", Stats: cs2Stats}, 320, 480},
		{"jogo_cs2_paisagem", render.LiveMatch{Active: true, Game: "CS2", Title: "de_mirage", Sub: "Round 18 · live · CT", Score: "9 - 8", Alert: "Bomba plantada", Stats: cs2Stats}, 480, 320},
		{"jogo_dota2_retrato", render.LiveMatch{Active: true, Game: "Dota 2", Title: "Axe", Sub: "Nível 14 · 20:45", Score: "12 - 7", Stats: dota2Stats}, 320, 480},
		{"jogo_lol_retrato", render.LiveMatch{Active: true, Game: "League of Legends", Title: "Ahri", Sub: "Nível 11 · 14:05", Stats: lolStats}, 320, 480},
	}
	for _, c := range vivos {
		in.GameLive = c.lm
		img := render.Draw(config.ScreenGame, c.w, c.h, in)
		f, err := os.Create(filepath.Join(out, c.nome+".png"))
		if err != nil {
			log.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			log.Fatal(err)
		}
		f.Close()
	}
}
