package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/satty-br/Bifrost-screen/internal/config"
	"github.com/satty-br/Bifrost-screen/internal/lcd"
	"github.com/satty-br/Bifrost-screen/internal/media"
	"github.com/satty-br/Bifrost-screen/internal/steam"
	"github.com/satty-br/Bifrost-screen/internal/sysinfo"
	"github.com/satty-br/Bifrost-screen/internal/winutil"
)

// runDiagnostics grava um relatório em <dados>\diagnostico.txt e o abre.
func runDiagnostics(store *config.Store, dir string) {
	var b strings.Builder
	p := func(f string, a ...any) { fmt.Fprintf(&b, f+"\n", a...) }
	cfg := store.Get()

	p("=== Diagnóstico Bifrost %s ===", version)
	p("Data: %s", time.Now().Format("02/01/2006 15:04:05"))
	p("Sistema: %s/%s", runtime.GOOS, runtime.GOARCH)
	p("Pasta de dados: %s", dir)
	p("")

	p("--- Portas COM ---")
	ports, err := lcd.ListPorts()
	if err != nil {
		p("ERRO listando portas: %v", err)
	}
	if len(ports) == 0 {
		p("nenhuma porta encontrada")
	}
	for _, pt := range ports {
		mark := ""
		if pt.IsScreen {
			mark = "  <== TELA"
		}
		p("%-6s  %-10s  serial=%s%s", pt.Name, pt.VIDPID, pt.Serial, mark)
	}
	if name, err := lcd.DetectRevA(); err == nil {
		p("Auto-detecção: %s", name)
		if port, err := lcd.OpenSerial(name); err != nil {
			p("Abrir %s: FALHOU — %v", name, err)
		} else {
			port.Close()
			p("Abrir %s: OK (a porta está livre)", name)
		}
	} else {
		p("Auto-detecção: %v", err)
	}
	p("")

	p("--- Programas que podem prender a porta ---")
	conf := winutil.FindConflicts()
	if len(conf) == 0 {
		p("nenhum encontrado")
	}
	for _, c := range conf {
		p("%s (PID %d) — %s", c.Name, c.PID, c.About)
	}
	p("")

	m := &media.Reader{}
	m.Start(500 * time.Millisecond)
	s := &sysinfo.Sampler{}
	s.SetDisk(cfg.Screens.System.Disk)
	s.Start(time.Second)
	time.Sleep(3 * time.Second)

	p("--- Música (Controle de Mídia do Windows) ---")
	mi := m.Get()
	if e := m.LastError(); e != "" {
		p("ERRO: %s", e)
	}
	if !mi.HasSession {
		p("nenhuma sessão de mídia (dê play em algo para testar)")
	} else {
		p("App: %s", mi.App)
		p("Título: %s", mi.Title)
		p("Artista: %s", mi.Artist)
		p("Álbum: %s", mi.Album)
		p("Tocando: %v   posição %s / %s", mi.Playing, mi.Position.Round(time.Second), mi.Duration.Round(time.Second))
		if mi.Cover != nil {
			p("Capa: %dx%d", mi.Cover.Bounds().Dx(), mi.Cover.Bounds().Dy())
		} else {
			p("Capa: nenhuma")
		}
	}
	p("")

	p("--- Sistema ---")
	st := s.Get()
	p("CPU: %.1f%%   GPU: %.1f%%", st.CPU, st.GPU)
	p("RAM: %.1f / %.1f GB", float64(st.RAMUsed)/(1<<30), float64(st.RAMTotal)/(1<<30))
	p("Disco %s: %.0f / %.0f GB", cfg.Screens.System.Disk, float64(st.DiskUsed)/(1<<30), float64(st.DiskTotal)/(1<<30))
	p("Rede: ↓ %.0f B/s  ↑ %.0f B/s", st.NetDown, st.NetUp)
	p("Ligado há: %s", st.Uptime.Round(time.Minute))
	p("")

	p("--- Steam ---")
	p("Fonte configurada: %s (ligada: %v)", cfg.Steam.Source, cfg.Steam.Enabled)
	sc := steam.New(filepath.Join(dir, "cache"))
	if persona, path, err := sc.TestLocal(); err != nil {
		p("Steam deste PC: %v", err)
	} else {
		p("Steam deste PC: %s (conta: %s)", path, persona)
		sc.Configure(steam.Settings{Enabled: true, Source: steam.SourceLocal})
		ctx, cancel := context.WithCancel(context.Background())
		go sc.Run(ctx)
		time.Sleep(1500 * time.Millisecond)
		cancel()
		if g := sc.Get(); g.Playing {
			p("Jogo aberto: %s (appid %d) — total %d min, 2 semanas %d min, capa: %v", g.Name, g.AppID, g.TotalMinutes, g.TwoWeeksMinutes, g.HasCover)
		} else {
			p("Jogo aberto: nenhum")
		}
	}
	if cfg.Steam.APIKey != "" && cfg.Steam.SteamID64 != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		name, err := sc.Test(ctx, cfg.Steam.APIKey, cfg.Steam.SteamID64)
		cancel()
		if err != nil {
			p("Web API: ERRO %v", err)
		} else {
			p("Web API: OK, perfil %s", name)
		}
	}
	p("")
	p("=== fim ===")

	path := filepath.Join(dir, "diagnostico.txt")
	_ = os.WriteFile(path, append([]byte{0xEF, 0xBB, 0xBF}, b.String()...), 0o644)
	fmt.Print(b.String())
	_ = winutil.OpenURL(path)
}
