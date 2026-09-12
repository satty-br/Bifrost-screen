// Package render desenha as telas do Bifrost como imagens.
package render

import (
	"embed"
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"sync"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
)

//go:embed fonts/*.ttf
var fontFS embed.FS

const (
	fontRegular = "Roboto-Regular.ttf"
	fontMedium  = "Roboto-Medium.ttf"
	fontBold    = "Roboto-Bold.ttf"
	fontMono    = "RobotoMono-Medium.ttf"
	fontMonoB   = "RobotoMono-Bold.ttf"
)

var (
	fontMu    sync.Mutex
	fontCache = map[string]*truetype.Font{}
	faceCache = map[string]font.Face{}
)

func face(name string, size float64) font.Face {
	fontMu.Lock()
	defer fontMu.Unlock()
	key := fmt.Sprintf("%s@%.1f", name, size)
	if f, ok := faceCache[key]; ok {
		return f
	}
	ft, ok := fontCache[name]
	if !ok {
		data, err := fontFS.ReadFile("fonts/" + name)
		if err != nil {
			panic(err)
		}
		ft, err = truetype.Parse(data)
		if err != nil {
			panic(err)
		}
		fontCache[name] = ft
	}
	f := truetype.NewFace(ft, &truetype.Options{Size: size, DPI: 72, Hinting: font.HintingFull})
	faceCache[key] = f
	return f
}

// Theme são as cores usadas no desenho.
type Theme struct {
	Accent   color.RGBA
	Bg       color.RGBA
	Gradient bool
}

var (
	colFg     = color.RGBA{235, 236, 240, 255}
	colDim    = color.RGBA{150, 153, 165, 255}
	colFaint  = color.RGBA{95, 99, 112, 255}
	colCard   = color.NRGBA{255, 255, 255, 16}
	colTrack  = color.NRGBA{255, 255, 255, 30}
	colPlaceh = color.RGBA{36, 38, 46, 255}
)

// ParseHex converte "#rrggbb" em cor.
func ParseHex(s string, def color.RGBA) color.RGBA {
	var r, g, b uint8
	if _, err := fmt.Sscanf(strings.TrimPrefix(s, "#"), "%02x%02x%02x", &r, &g, &b); err != nil {
		return def
	}
	return color.RGBA{r, g, b, 255}
}

func lighten(c color.RGBA, amt float64) color.RGBA {
	f := func(v uint8) uint8 { return uint8(math.Min(255, float64(v)+(255-float64(v))*amt)) }
	return color.RGBA{f(c.R), f(c.G), f(c.B), 255}
}

func withAlpha(c color.RGBA, a uint8) color.RGBA {
	// gg espera cor pré-multiplicada
	return color.RGBA{uint8(uint16(c.R) * uint16(a) / 255), uint8(uint16(c.G) * uint16(a) / 255), uint8(uint16(c.B) * uint16(a) / 255), a}
}

func newCanvas(w, h int, t Theme) *gg.Context {
	dc := gg.NewContext(w, h)
	if t.Gradient {
		grad := gg.NewLinearGradient(0, 0, 0, float64(h))
		grad.AddColorStop(0, t.Bg)
		grad.AddColorStop(1, lighten(t.Bg, 0.06))
		dc.SetFillStyle(grad)
	} else {
		dc.SetColor(t.Bg)
	}
	dc.DrawRectangle(0, 0, float64(w), float64(h))
	dc.Fill()
	// brilho sutil da cor de destaque no topo
	glow := gg.NewRadialGradient(float64(w)*0.15, 0, 0, float64(w)*0.15, 0, float64(w)*0.9)
	glow.AddColorStop(0, withAlpha(t.Accent, 34))
	glow.AddColorStop(1, color.RGBA{})
	dc.SetFillStyle(glow)
	dc.DrawRectangle(0, 0, float64(w), float64(h))
	dc.Fill()
	return dc
}

// text desenha s com o topo em y (e não na linha de base, como o gg faz).
func text(dc *gg.Context, s string, x, y float64, f font.Face, c color.Color) {
	dc.SetFontFace(f)
	dc.SetColor(c)
	dc.DrawString(s, x, y+ascent(f))
}

// textCenter desenha s centralizado em cx com o topo em y.
func textCenter(dc *gg.Context, s string, cx, y float64, f font.Face, c color.Color) {
	dc.SetFontFace(f)
	w, _ := dc.MeasureString(s)
	text(dc, s, cx-w/2, y, f, c)
}

func textRight(dc *gg.Context, s string, rx, y float64, f font.Face, c color.Color) {
	dc.SetFontFace(f)
	w, _ := dc.MeasureString(s)
	text(dc, s, rx-w, y, f, c)
}

func ascent(f font.Face) float64 { return float64(f.Metrics().Ascent.Round()) }
func lineH(f font.Face) float64  { return float64(f.Metrics().Height.Round()) }

// capH mede a altura das letras maiúsculas/dígitos (a fonte não informa isso).
func capH(f font.Face) float64 {
	b, _ := font.BoundString(f, "H0")
	return float64((-b.Min.Y).Round())
}

func measure(dc *gg.Context, s string, f font.Face) float64 {
	dc.SetFontFace(f)
	w, _ := dc.MeasureString(s)
	return w
}

// ellipsize corta s com "…" para caber em maxW.
func ellipsize(dc *gg.Context, s string, f font.Face, maxW float64) string {
	if measure(dc, s, f) <= maxW {
		return s
	}
	r := []rune(s)
	lo, hi := 0, len(r)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if measure(dc, strings.TrimSpace(string(r[:mid]))+"…", f) <= maxW {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return strings.TrimSpace(string(r[:lo])) + "…"
}

// wrap quebra s em até maxLines linhas, terminando em "…" se sobrar texto.
func wrap(dc *gg.Context, s string, f font.Face, maxW float64, maxLines int) []string {
	words := strings.Fields(s)
	if len(words) == 0 || maxLines < 1 {
		return []string{""}
	}
	join := func(ws []string) string { return strings.Join(ws, " ") }
	var lines []string
	lineStart := 0
	for i := 0; i < len(words); i++ {
		if measure(dc, join(words[lineStart:i+1]), f) <= maxW {
			continue
		}
		if len(lines) == maxLines-1 {
			return append(lines, ellipsize(dc, join(words[lineStart:]), f, maxW))
		}
		if i == lineStart { // uma palavra sozinha maior que a linha
			lines = append(lines, ellipsize(dc, words[i], f, maxW))
			lineStart = i + 1
			continue
		}
		lines = append(lines, join(words[lineStart:i]))
		lineStart = i
		i-- // reavalia a palavra atual como começo da próxima linha
	}
	if lineStart < len(words) {
		lines = append(lines, join(words[lineStart:]))
	}
	return lines
}

// header desenha o rótulo da tela com a barrinha colorida, e um texto opcional à direita.
func header(dc *gg.Context, w float64, label string, accent color.RGBA, right string) {
	dc.SetColor(accent)
	dc.DrawRoundedRectangle(16, 18, 6, 20, 3)
	dc.Fill()
	f := face(fontBold, 16)
	text(dc, label, 30, 28-capH(f)/2-1, f, accent)
	if right != "" {
		fr := face(fontMedium, 13)
		right = ellipsize(dc, right, fr, w/2-20)
		textRight(dc, right, w-16, 28-capH(fr)/2-1, fr, colDim)
	}
}

func card(dc *gg.Context, x, y, w, h, r float64) {
	dc.SetColor(colCard)
	dc.DrawRoundedRectangle(x, y, w, h, r)
	dc.Fill()
}

func bar(dc *gg.Context, x, y, w, h, frac float64, accent color.RGBA) {
	frac = math.Max(0, math.Min(1, frac))
	dc.SetColor(colTrack)
	dc.DrawRoundedRectangle(x, y, w, h, h/2)
	dc.Fill()
	if frac > 0 {
		dc.SetColor(accent)
		dc.DrawRoundedRectangle(x, y, math.Max(h, w*frac), h, h/2)
		dc.Fill()
	}
}

// fit redimensiona img para cobrir w×h (cortando as sobras, como "object-fit: cover").
func fit(img image.Image, w, h int) image.Image {
	b := img.Bounds()
	sw, sh := float64(b.Dx()), float64(b.Dy())
	scale := math.Max(float64(w)/sw, float64(h)/sh)
	cw, ch := float64(w)/scale, float64(h)/scale
	src := image.Rect(
		b.Min.X+int((sw-cw)/2), b.Min.Y+int((sh-ch)/2),
		b.Min.X+int((sw+cw)/2), b.Min.Y+int((sh+ch)/2),
	)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, src, xdraw.Over, nil)
	return dst
}

// roundedImage desenha img (ou um ícone, se img for nil) num quadro arredondado.
func roundedImage(dc *gg.Context, img image.Image, x, y, w, h, r float64, placeholder func(dc *gg.Context, cx, cy, size float64)) {
	dc.Push()
	dc.DrawRoundedRectangle(x, y, w, h, r)
	dc.Clip()
	if img != nil {
		dc.DrawImage(fit(img, int(w), int(h)), int(x), int(y))
	} else {
		dc.SetColor(colPlaceh)
		dc.DrawRectangle(x, y, w, h)
		dc.Fill()
		if placeholder != nil {
			placeholder(dc, x+w/2, y+h/2, math.Min(w, h))
		}
	}
	dc.ResetClip()
	dc.Pop()
}
