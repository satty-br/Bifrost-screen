package render

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/fogleman/gg"
	"github.com/satty-br/Bifrost-screen/internal/config"
	"github.com/satty-br/Bifrost-screen/internal/i18n"
	"golang.org/x/image/font"
)

// wStyle junta as opções de estilo que o usuário escolhe por widget no
// editor (cor, negrito, tamanho da fonte), pra não empilhar parâmetros
// soltos em toda função de desenho.
type wStyle struct {
	Accent color.RGBA
	Bold   bool
	Scale  float64 // multiplicador de tamanho de fonte (1 = automático)
}

// drawCustom desenha a tela personalizada: cada widget é convertido da
// posição relativa (0..1, guardada na config) pra pixels desta tela em
// específico, e recortado pra não vazar por cima dos vizinhos. Cor, negrito,
// tamanho de fonte, fundo e tipo de gráfico são escolhidos por widget no
// editor arrasta-e-solta do painel.
func drawCustom(w, h int, in Input, th Theme) *gg.Context {
	cs := in.Cfg.Screens.Custom
	var dc *gg.Context
	if cs.Background == "imagem" && in.CustomBgImage != nil {
		dc = newImageCanvas(w, h, th, in.CustomBgImage)
	} else {
		if cs.BgColor != "" {
			th.Bg = ParseHex(cs.BgColor, th.Bg)
		}
		if cs.Background != "" {
			th.Gradient = cs.Background == "gradiente"
		}
		dc = newCanvas(w, h, th)
	}
	fw, fh := float64(w), float64(h)
	widgets := cs.Widgets
	if len(widgets) == 0 {
		header(dc, fw, i18n.T(in.Lang, "custom.header"), th.Accent, "")
		textCenter(dc, i18n.T(in.Lang, "custom.empty_title"), fw/2, fh/2-8, face(fontMedium, 16), colFg)
		for i, l := range wrap(dc, i18n.T(in.Lang, "custom.empty_detail"), face(fontRegular, 12), fw-48, 3) {
			textCenter(dc, l, fw/2, fh/2+18+float64(i)*16, face(fontRegular, 12), colDim)
		}
		return dc
	}
	for _, wd := range widgets {
		x, y := wd.X*fw, wd.Y*fh
		bw, bh := wd.W*fw, wd.H*fh
		if bw < 4 || bh < 4 {
			continue
		}
		dc.Push()
		dc.DrawRectangle(x, y, bw, bh)
		dc.Clip()
		st := wStyle{Accent: th.Accent, Bold: wd.Bold, Scale: wd.FontSize}
		if wd.Color != "" {
			st.Accent = ParseHex(wd.Color, th.Accent)
		}
		if st.Scale <= 0 {
			st.Scale = 1
		}
		if wd.Bg {
			var col color.Color = colCard
			if wd.BgColor != "" {
				col = withAlpha(ParseHex(wd.BgColor, th.Bg), 60)
			}
			dc.SetColor(col)
			dc.DrawRoundedRectangle(x, y, bw, bh, math.Min(bw, bh)*0.12)
			dc.Fill()
		}
		drawCustomWidget(dc, wd, x, y, bw, bh, in, st)
		dc.ResetClip()
		dc.Pop()
	}
	return dc
}

// newImageCanvas usa bg (já redimensionada/recortada pra cobrir a tela) como
// fundo, com um véu escuro por cima pra manter o texto legível sobre
// qualquer foto, e o mesmo brilho de destaque sutil das outras telas.
func newImageCanvas(w, h int, th Theme, bg image.Image) *gg.Context {
	dc := gg.NewContext(w, h)
	dc.DrawImage(fit(bg, w, h), 0, 0)
	dc.SetColor(color.NRGBA{0, 0, 0, 90})
	dc.DrawRectangle(0, 0, float64(w), float64(h))
	dc.Fill()
	glow := gg.NewRadialGradient(float64(w)*0.15, 0, 0, float64(w)*0.15, 0, float64(w)*0.9)
	glow.AddColorStop(0, withAlpha(th.Accent, 34))
	glow.AddColorStop(1, color.RGBA{})
	dc.SetFillStyle(glow)
	dc.DrawRectangle(0, 0, float64(w), float64(h))
	dc.Fill()
	return dc
}

func drawCustomWidget(dc *gg.Context, wd config.CustomWidget, x, y, w, h float64, in Input, st wStyle) {
	switch wd.Type {
	case "relogio":
		widgetClock(dc, x, y, w, h, in, st)
	case "data":
		widgetDate(dc, x, y, w, h, in, st)
	case "texto":
		widgetText(dc, x, y, w, h, wd.Text, st)
	case "cpu_medidor":
		widgetGaugeStyled(dc, x, y, w, h, "CPU", in.System.CPU, in.System.CPUTemp, wd.Style, st)
	case "gpu_medidor":
		widgetGaugeStyled(dc, x, y, w, h, "GPU", in.System.GPU, in.System.GPUTemp, wd.Style, st)
	case "cpu_temperatura":
		widgetTemp(dc, x, y, w, h, "CPU", in.System.CPUTemp, st)
	case "gpu_temperatura":
		widgetTemp(dc, x, y, w, h, "GPU", in.System.GPUTemp, st)
	case "ram_barra":
		widgetUsageStyled(dc, x, y, w, h, "RAM", gb(in.System.RAMUsed)+" / "+gb(in.System.RAMTotal), in.System.RAMPercent(), wd.Style, st)
	case "disco_barra":
		label := i18n.T(in.Lang, "system.disk") + in.Cfg.Screens.System.Disk
		widgetUsageStyled(dc, x, y, w, h, label, gb(in.System.DiskUsed)+" / "+gb(in.System.DiskTotal), in.System.DiskPercent(), wd.Style, st)
	case "rede":
		widgetNet(dc, x, y, w, h, in, st)
	case "tempo_ligado":
		widgetUptime(dc, x, y, w, h, in, st)
	case "musica_titulo":
		widgetMusicTitle(dc, x, y, w, h, in, st)
	case "musica_capa":
		roundedImage(dc, in.Media.Cover, x, y, w, h, math.Min(w, h)*0.12, noteIcon(st.Accent))
	case "musica_progresso":
		widgetMusicProgress(dc, x, y, w, h, in, st)
	case "jogo_nome":
		widgetGameName(dc, x, y, w, h, in, st)
	case "jogo_capa":
		roundedImage(dc, in.Steam.Cover, x, y, w, h, math.Min(w, h)*0.08, padIcon(st.Accent))
	case "jogo_tempo":
		widgetGamePlaytime(dc, x, y, w, h, in, st)
	}
}

func clampF(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

// titleFace alterna entre negrito e a variação normal (média) do texto
// principal de um widget, no tamanho pedido já multiplicado pela escala
// escolhida pelo usuário — são os controles de "fonte" expostos no editor.
func titleFace(st wStyle, size float64) font.Face {
	if st.Bold {
		return face(fontBold, size*st.Scale)
	}
	return face(fontMedium, size*st.Scale)
}

// valueFace é o valor numérico monoespaçado em negrito (o padrão dos
// medidores/cartões), também escalado.
func valueFace(st wStyle, size float64) font.Face { return face(fontMonoB, size*st.Scale) }

// monoValueFace idem, mas a própria alternância negrito/normal do widget
// também vale pro valor (usado quando não há um rótulo separado).
func monoValueFace(st wStyle, size float64) font.Face {
	if st.Bold {
		return face(fontMonoB, size*st.Scale)
	}
	return face(fontMono, size*st.Scale)
}

func widgetClock(dc *gg.Context, x, y, w, h float64, in Input, st wStyle) {
	opt := in.Cfg.Screens.Clock
	hh := in.Now.Hour()
	suffix := ""
	if !opt.Use24h {
		suffix = i18n.AMPM(in.Lang, hh >= 12)
		hh %= 12
		if hh == 0 {
			hh = 12
		}
	}
	main := fmt.Sprintf("%02d:%02d", hh, in.Now.Minute())
	size := clampF(math.Min(h*0.55, w/2.6), 12, 96)
	ft := titleFace(st, math.Round(size))
	cy := y + h/2
	if suffix != "" {
		cy -= lineH(face(fontMedium, size*0.28*st.Scale)) / 2
	}
	drawTabular(dc, main, x+w/2, cy-capH(ft)/2, ft, st.Accent)
	if suffix != "" {
		fs := face(fontMedium, clampF(size*0.28, 9, 20)*st.Scale)
		textCenter(dc, suffix, x+w/2, cy+capH(ft)+2, fs, st.Accent)
	}
}

func widgetDate(dc *gg.Context, x, y, w, h float64, in Input, st wStyle) {
	date := i18n.DateLine(in.Lang, in.Now)
	size := clampF(h*0.32, 10, 22)
	f := titleFace(st, size)
	textCenter(dc, ellipsize(dc, date, f, w-8), x+w/2, y+h/2-capH(f)/2, f, st.Accent)
}

// widgetText desenha um rótulo de texto livre (widget "texto"), definido
// pelo usuário no editor — não tem dado dinâmico, só o que foi digitado.
func widgetText(dc *gg.Context, x, y, w, h float64, txt string, st wStyle) {
	if txt == "" {
		return
	}
	ft := titleFace(st, clampF(math.Min(w, h)*0.3, 12, 30))
	lines := wrap(dc, txt, ft, w-12, 3)
	ty := y + h/2 - float64(len(lines))*lineH(ft)/2
	for _, l := range lines {
		textCenter(dc, l, x+w/2, ty, ft, st.Accent)
		ty += lineH(ft)
	}
}

// widgetGaugeStyled é o dispatcher de "tipo de gráfico" dos medidores de
// CPU/GPU (arco com temperatura, barra com temperatura, ou só o número).
func widgetGaugeStyled(dc *gg.Context, x, y, w, h float64, label string, value, tempC float64, style string, st wStyle) {
	switch style {
	case "barra":
		widgetGaugeBar(dc, x, y, w, h, label, value, tempC, st)
	case "numero":
		widgetGaugeNumber(dc, x, y, w, h, label, value, st)
	default:
		widgetGauge(dc, x, y, w, h, label, value, tempC, st.Accent)
	}
}

// widgetUsageStyled é o mesmo dispatcher, só que pras barras de RAM/disco
// (sem temperatura, com o texto de GB usado/total no lugar da barra padrão).
func widgetUsageStyled(dc *gg.Context, x, y, w, h float64, label, valueText string, pct float64, style string, st wStyle) {
	switch style {
	case "medidor":
		widgetGauge(dc, x, y, w, h, label, pct, -1, st.Accent)
	case "numero":
		widgetGaugeNumber(dc, x, y, w, h, label, pct, st)
	default:
		widgetUsageBar(dc, x, y, w, h, label, valueText, pct/100, st)
	}
}

func widgetGauge(dc *gg.Context, x, y, w, h float64, label string, value, tempC float64, accent color.RGBA) {
	r := math.Min(w, h) / 2 * 0.88
	drawGauge(dc, x+w/2, y+h/2, r, value, tempC, label, accent)
}

// widgetGaugeBar é a variação "barra" do medidor: uma barra linear com o
// valor e a temperatura em texto, em vez do arco.
func widgetGaugeBar(dc *gg.Context, x, y, w, h float64, label string, value, tempC float64, st wStyle) {
	if h < 30 {
		widgetGaugeNumber(dc, x, y, w, h, label, value, st)
		return
	}
	fl := titleFace(st, clampF(h*0.2, 9, 12))
	text(dc, label, x+10, y+8, fl, colDim)
	val := "--"
	if value >= 0 {
		val = fmt.Sprintf("%.0f%%", value)
	}
	fv := valueFace(st, clampF(h*0.24, 11, 16))
	text(dc, val, x+10, y+h*0.42, fv, colFg)
	if tempC >= 0 {
		ft := face(fontMono, clampF(h*0.18, 9, 12)*st.Scale)
		textRight(dc, fmt.Sprintf("%.0f°C", tempC), x+w-10, y+h*0.42, ft, colDim)
	}
	if value >= 0 {
		bar(dc, x+10, y+h-16, w-20, 6, value/100, st.Accent)
	}
}

// widgetGaugeNumber é a variação "número": só o valor grande, sem arco nem
// barra — útil em caixas pequenas.
func widgetGaugeNumber(dc *gg.Context, x, y, w, h float64, label string, value float64, st wStyle) {
	val := "--"
	if value >= 0 {
		val = fmt.Sprintf("%.0f%%", value)
	}
	fv := valueFace(st, clampF(math.Min(w, h)*0.4, 14, 40))
	textCenter(dc, val, x+w/2, y+h/2-capH(fv)/2-4, fv, st.Accent)
	fl := titleFace(st, clampF(h*0.16, 9, 13))
	textCenter(dc, label, x+w/2, y+h/2+capH(fv)/2+2, fl, colDim)
}

func widgetTemp(dc *gg.Context, x, y, w, h float64, label string, tempC float64, st wStyle) {
	txt := "--"
	if tempC >= 0 {
		txt = fmt.Sprintf("%.0f°C", tempC)
	}
	fl := titleFace(st, clampF(h*0.18, 9, 13))
	text(dc, label, x+10, y+8, fl, colDim)
	fv := valueFace(st, clampF(h*0.4, 14, 30))
	textCenter(dc, txt, x+w/2, y+h/2-capH(fv)/2+6, fv, st.Accent)
}

func widgetUsageBar(dc *gg.Context, x, y, w, h float64, label, value string, frac float64, st wStyle) {
	fl := titleFace(st, clampF(h*0.2, 9, 12))
	text(dc, label, x+10, y+8, fl, colDim)
	fv := valueFace(st, clampF(h*0.24, 11, 15))
	text(dc, ellipsize(dc, value, fv, w-20), x+10, y+h*0.42, fv, st.Accent)
	if h > 30 {
		bar(dc, x+10, y+h-16, w-20, 6, frac, st.Accent)
	}
}

func widgetNet(dc *gg.Context, x, y, w, h float64, in Input, st wStyle) {
	fl := titleFace(st, clampF(h*0.2, 9, 11))
	text(dc, i18n.T(in.Lang, "system.net"), x+10, y+8, fl, colDim)
	down, up := rate(in.System.NetDown), rate(in.System.NetUp)
	fv := valueFace(st, clampF(h*0.22, 10, 14))
	arrow(dc, x+10, y+h*0.55, 8, true, colFg)
	text(dc, ellipsize(dc, down, fv, w-30), x+24, y+h*0.55-8, fv, colFg)
	if h > 46 {
		arrow(dc, x+10, y+h-16, 8, false, st.Accent)
		text(dc, ellipsize(dc, up, fv, w-30), x+24, y+h-24, fv, st.Accent)
	}
}

func widgetUptime(dc *gg.Context, x, y, w, h float64, in Input, st wStyle) {
	label := i18n.T(in.Lang, "system.uptime_prefix") + i18n.Uptime(in.Lang, in.System.Uptime)
	fv := monoValueFace(st, clampF(h*0.22, 10, 16))
	textCenter(dc, ellipsize(dc, label, fv, w-16), x+w/2, y+h/2-capH(fv)/2, fv, st.Accent)
}

func widgetMusicTitle(dc *gg.Context, x, y, w, h float64, in Input, st wStyle) {
	m := in.Media
	if !m.HasSession {
		textCenter(dc, i18n.T(in.Lang, "music.nothing"), x+w/2, y+h/2-8, face(fontMedium, clampF(h*0.16, 10, 14)), colDim)
		return
	}
	title := m.Title
	if title == "" {
		title = "—"
	}
	ft := titleFace(st, clampF(h*0.24, 12, 20))
	ty := y + 6
	for _, l := range wrap(dc, title, ft, w-16, 2) {
		text(dc, l, x+8, ty, ft, st.Accent)
		ty += lineH(ft)
	}
	if m.Artist != "" && ty+lineH(face(fontMedium, 12)) <= y+h-4 {
		fa := face(fontMedium, clampF(h*0.16, 10, 14)*st.Scale)
		text(dc, ellipsize(dc, m.Artist, fa, w-16), x+8, ty+2, fa, colDim)
	}
}

func widgetMusicProgress(dc *gg.Context, x, y, w, h float64, in Input, st wStyle) {
	m := in.Media
	if !m.HasSession || m.Duration <= 0 {
		return
	}
	pos := m.EstimatedPosition(in.Now)
	drawProgress(dc, x+8, y+h/2-3, w-16, pos, m.Duration, m.Playing, st.Accent)
}

func widgetGameName(dc *gg.Context, x, y, w, h float64, in Input, st wStyle) {
	g := in.Steam
	if !g.Playing {
		textCenter(dc, i18n.T(in.Lang, "game.empty_title"), x+w/2, y+h/2-8, face(fontMedium, clampF(h*0.16, 10, 14)), colDim)
		return
	}
	ft := titleFace(st, clampF(h*0.26, 12, 22))
	lines := wrap(dc, g.Name, ft, w-16, 2)
	ty := y + h/2 - float64(len(lines))*lineH(ft)/2
	for _, l := range lines {
		textCenter(dc, l, x+w/2, ty, ft, st.Accent)
		ty += lineH(ft)
	}
}

func widgetGamePlaytime(dc *gg.Context, x, y, w, h float64, in Input, st wStyle) {
	fl := titleFace(st, clampF(h*0.18, 9, 12))
	text(dc, i18n.T(in.Lang, "game.total"), x+10, y+8, fl, colDim)
	fv := valueFace(st, clampF(h*0.3, 13, 20))
	value := i18n.Playtime(in.Lang, in.Steam.TotalMinutes)
	textCenter(dc, ellipsize(dc, value, fv, w-16), x+w/2, y+h*0.55, fv, st.Accent)
}
