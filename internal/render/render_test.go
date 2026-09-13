package render

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/satty-br/Bifrost-screen/internal/config"
	"github.com/satty-br/Bifrost-screen/internal/i18n"
	"github.com/satty-br/Bifrost-screen/internal/media"
	"github.com/satty-br/Bifrost-screen/internal/steam"
	"github.com/satty-br/Bifrost-screen/internal/sysinfo"
)

func fakeImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(255 * x / w), 60, uint8(255 * y / h), 255})
		}
	}
	return img
}

func sampleInput() Input {
	now := time.Date(2026, 9, 11, 21, 37, 42, 0, time.Local)
	cfg := config.Default()
	cfg.Steam.Enabled = true
	hist := make([]float64, 60)
	for i := range hist {
		hist[i] = 20 + float64((i*37)%50)
	}
	return Input{
		Now: now, Cfg: cfg, SteamReady: true, Lang: i18n.EN,
		Media: media.Info{HasSession: true, Playing: true, Title: "Bohemian Rhapsody (Remastered 2011)", Artist: "Queen",
			Album: "A Night at the Opera", App: "Spotify", Position: 123 * time.Second, Duration: 355 * time.Second,
			UpdatedAt: now, Cover: fakeImage(300, 300)},
		Steam: steam.Status{Playing: true, AppID: 730, Name: "Counter-Strike 2", PersonaName: "satty", TotalMinutes: 5423,
			TwoWeeksMinutes: 340, SessionStart: now.Add(-95 * time.Minute), Cover: fakeImage(460, 215)},
		System: sysinfo.Stats{CPU: 37, GPU: 82, CPUTemp: 58, GPUTemp: 64, RAMUsed: 11 << 30, RAMTotal: 32 << 30, NetDown: 3.4 * (1 << 20), NetUp: 220 * (1 << 10),
			DiskUsed: 612 << 30, DiskTotal: 931 << 30, Uptime: 26*time.Hour + 14*time.Minute, CPUHistory: hist},
	}
}

func TestRenderAll(t *testing.T) {
	out := os.Getenv("BIFROST_PREVIEW_DIR")
	in := sampleInput()
	cases := map[string]Input{"": in}
	empty := in
	empty.Media = media.Info{}
	empty.Steam = steam.Status{}
	cases["_vazio"] = empty
	noCover := in
	noCover.Media.Cover = nil
	noCover.Steam.Cover = nil
	cases["_semcapa"] = noCover
	for suffix, input := range cases {
		for _, s := range config.AllScreens {
			for _, dims := range [][2]int{{320, 480}, {480, 320}} {
				img := Draw(s, dims[0], dims[1], input)
				if img.Bounds().Dx() != dims[0] || img.Bounds().Dy() != dims[1] {
					t.Fatalf("%s: tamanho errado %v", s, img.Bounds())
				}
				if out == "" {
					continue
				}
				name := filepath.Join(out, string(s)+suffix+map[bool]string{true: "_paisagem", false: ""}[dims[0] > dims[1]]+".png")
				f, _ := os.Create(name)
				png.Encode(f, img)
				f.Close()
			}
		}
	}
	if out != "" {
		f, _ := os.Create(filepath.Join(out, "mensagem.png"))
		png.Encode(f, Message(320, 480, "Porta em uso", "Feche o app oficial da tela (UsbMonitor) para o Bifrost conectar.", in.Cfg.Theme))
		f.Close()
	}
}

// TestRenderCustomWidgets exercita cada tipo de widget da tela personalizada
// (um por vez, e todos juntos numa grade), em caixas bem pequenas e bem
// grandes, pra pegar qualquer pânico de divisão por zero/medida negativa.
func TestRenderCustomWidgets(t *testing.T) {
	in := sampleInput()
	for _, kind := range config.CustomWidgetKinds {
		in.Cfg.Screens.Custom.Widgets = []config.CustomWidget{{Type: kind, X: 0.1, Y: 0.1, W: 0.3, H: 0.15}}
		for _, dims := range [][2]int{{320, 480}, {480, 320}} {
			img := Draw(config.ScreenCustom, dims[0], dims[1], in)
			if img == nil {
				t.Fatalf("widget %s: imagem nula", kind)
			}
		}
	}
	var grid []config.CustomWidget
	for i, kind := range config.CustomWidgetKinds {
		grid = append(grid, config.CustomWidget{Type: kind, X: float64(i%4) * 0.25, Y: float64(i/4) * 0.25, W: 0.25, H: 0.25})
	}
	in.Cfg.Screens.Custom.Widgets = grid
	Draw(config.ScreenCustom, 320, 480, in)
}

// TestRenderWideAspectRatios verifica que nenhuma tela quebra (pânico, medida
// negativa, tamanho de imagem errado) em proporções bem diferentes de
// 320x480/480x320 — telas mais novas/largas como 1280x800 (5.2") e
// principalmente 1920x480 (8.8", quase 4:1) ainda não têm um driver de
// verdade no Bifrost, mas o layout das telas embutidas já precisa aguentar
// esses tamanhos pro dia em que o protocolo delas for suportado.
func TestRenderWideAspectRatios(t *testing.T) {
	out := os.Getenv("BIFROST_PREVIEW_DIR")
	in := sampleInput()
	dims := [][2]int{{1280, 800}, {800, 1280}, {1920, 480}, {480, 1920}}
	for _, s := range config.AllScreens {
		for _, d := range dims {
			img := Draw(s, d[0], d[1], in)
			if img == nil {
				t.Fatalf("%s %dx%d: imagem nula", s, d[0], d[1])
			}
			if img.Bounds().Dx() != d[0] || img.Bounds().Dy() != d[1] {
				t.Fatalf("%s %dx%d: tamanho errado %v", s, d[0], d[1], img.Bounds())
			}
			if out == "" {
				continue
			}
			name := filepath.Join(out, fmt.Sprintf("%s_%dx%d.png", s, d[0], d[1]))
			f, _ := os.Create(name)
			png.Encode(f, img)
			f.Close()
		}
	}
}

