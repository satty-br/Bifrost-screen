package app

import (
	"testing"
	"time"

	"github.com/satty-br/Bifrost-screen/internal/config"
	"github.com/satty-br/Bifrost-screen/internal/media"
	"github.com/satty-br/Bifrost-screen/internal/steam"
)

func TestAutoModePriority(t *testing.T) {
	c := config.Default()
	c.Steam.Enabled = true
	a := &App{}
	d := newDevice("t")
	mode := c.Devices[0].Mode
	now := time.Now()
	playing := media.Info{HasSession: true, Playing: true}
	game := steam.Status{Playing: true}

	cases := []struct {
		name string
		m    media.Info
		g    steam.Status
		want config.Screen
	}{
		{"jogo tem prioridade", playing, game, config.ScreenGame},
		{"só música", playing, steam.Status{}, config.ScreenMusic},
		{"nada: cai no sistema", media.Info{}, steam.Status{}, config.ScreenSystem},
	}
	for _, tc := range cases {
		got := a.choose(d, now, mode, a.activeScreens(c, tc.m, tc.g))
		if got != tc.want {
			t.Errorf("%s: esperava %s, veio %s", tc.name, tc.want, got)
		}
	}

	// música pausada some quando "mostrar pausada" está desligado
	c.Screens.Music.ShowWhenPaused = false
	if got := a.choose(d, now, mode, a.activeScreens(c, media.Info{HasSession: true}, steam.Status{})); got != config.ScreenSystem {
		t.Errorf("pausada escondida: veio %s", got)
	}
	// Steam desligada não mostra jogo mesmo com status
	c.Steam.Enabled = false
	if got := a.choose(d, now, mode, a.activeScreens(c, playing, game)); got != config.ScreenMusic {
		t.Errorf("steam desligada: veio %s", got)
	}
	// tudo desligado: relógio
	c.Screens.Music.Enabled, c.Screens.System.Enabled, c.Screens.Clock.Enabled = false, false, false
	if got := a.choose(d, now, mode, a.activeScreens(c, playing, game)); got != config.ScreenClock {
		t.Errorf("tudo desligado: veio %s", got)
	}
}

func TestRotateAndFixed(t *testing.T) {
	c := config.Default()
	mode := c.Devices[0].Mode
	mode.Type = config.ModeRotate
	mode.RotateSeconds = 5
	a := &App{}
	d := newDevice("t")
	active := []config.Screen{config.ScreenMusic, config.ScreenSystem, config.ScreenClock}
	start := time.Now()
	seen := map[config.Screen]bool{}
	for i := 0; i < 6; i++ {
		seen[a.choose(d, start.Add(time.Duration(i)*6*time.Second), mode, active)] = true
	}
	if len(seen) != 3 {
		t.Errorf("rotação deveria passar pelas 3 telas, passou por %v", seen)
	}
	mode.Type = config.ModeFixed
	mode.Fixed = config.ScreenGame
	if got := a.choose(d, start, mode, active); got != config.ScreenGame {
		t.Errorf("fixo: veio %s", got)
	}
}

func TestNormalize(t *testing.T) {
	c := config.Config{}
	c.Screens.Order = []config.Screen{"relogio", "xyz", "relogio"}
	c.Devices = []config.DeviceConfig{{Brightness: 500}}
	c.Theme.AccentMusic = "vermelho"
	c.Normalize()
	if len(c.Screens.Order) != 5 || c.Screens.Order[0] != config.ScreenClock {
		t.Errorf("ordem: %v", c.Screens.Order)
	}
	if c.Devices[0].Brightness != 100 || c.Theme.AccentMusic != "#2dd4bf" || c.Devices[0].Port != "AUTO" {
		t.Errorf("normalização falhou: %+v", c.Devices[0])
	}
}

