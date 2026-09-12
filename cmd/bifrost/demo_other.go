//go:build !windows

package main

import (
	"image"
	"image/color"
	"os"
	"time"

	"github.com/satty-br/Bifrost-screen/internal/app"
	"github.com/satty-br/Bifrost-screen/internal/media"
)

// startDemo injeta uma música de exemplo (BIFROST_DEMO=1), para testar o visual fora do Windows.
func startDemo(a *app.App) {
	if os.Getenv("BIFROST_DEMO") != "1" {
		return
	}
	cover := image.NewRGBA(image.Rect(0, 0, 300, 300))
	for y := 0; y < 300; y++ {
		for x := 0; x < 300; x++ {
			cover.Set(x, y, color.RGBA{uint8(40 + x/2), uint8(90 + y/3), 200, 255})
		}
	}
	start := time.Now()
	go func() {
		for {
			a.Media().SetForTest(media.Info{HasSession: true, Playing: true, Title: "Bohemian Rhapsody", Artist: "Queen",
				Album: "A Night at the Opera", App: "Spotify", Position: 83*time.Second + time.Since(start),
				Duration: 355 * time.Second, UpdatedAt: time.Now(), Cover: cover})
			time.Sleep(time.Second)
		}
	}()
}
