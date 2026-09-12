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
}
