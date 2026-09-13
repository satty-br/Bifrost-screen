package render

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"time"

	"github.com/fogleman/gg"
	"github.com/satty-br/Bifrost-screen/internal/config"
	"github.com/satty-br/Bifrost-screen/internal/i18n"
	"github.com/satty-br/Bifrost-screen/internal/media"
	"github.com/satty-br/Bifrost-screen/internal/steam"
	"github.com/satty-br/Bifrost-screen/internal/sysinfo"
	"golang.org/x/image/font"
)

// Input reúne tudo que as telas podem precisar.
type Input struct {
	Now    time.Time
	Cfg    config.Config
	Media  media.Info
	Steam  steam.Status
	System sysinfo.Stats
	Lang   i18n.Lang
	// SteamReady indica se a Steam está configurada (para a mensagem da tela vazia).
	SteamReady bool
	// GameLive é a partida ao vivo (CS2/Dota2/LoL) detectada agora, se houver.
	GameLive LiveMatch
	// FPS é a taxa de quadros lida do RTSS (RivaTuner), ou 0 se não disponível.
	FPS float64
	// CustomBgImage é a imagem de fundo da tela personalizada, já decodificada
	// (nil se não houver uma configurada, ou se Screens.Custom.Background não for "imagem").
	CustomBgImage image.Image
}

// LiveMatch descreve uma partida ao vivo (CS2, Dota 2 ou League of Legends),
// mostrada na tela do jogo no lugar do resumo padrão da Steam enquanto durar.
type LiveMatch struct {
	Active bool
	Game   string // "CS2", "Dota 2", "League of Legends"
	Title  string // mapa (CS2/Dota2) ou campeão (LoL)
	Sub    string // fase da rodada, modo, nível, tempo de jogo...
	Score  string // placar do time, mostrado no cabeçalho
	Alert  string // aviso destacado, ex: "Bomba plantada" (vazio = nenhum)
	Stats  []LiveStat
}

// LiveStat é um par rótulo/valor mostrado num cartão da tela de partida ao vivo.
type LiveStat struct{ Label, Value string }

// CanvasLang ajusta o idioma para o que a fonte embutida (Roboto) consegue desenhar:
// ela não tem glifos de CJK, então japonês/mandarim caem para o inglês só na tela física.
func CanvasLang(l i18n.Lang) i18n.Lang {
	if l == i18n.JA || l == i18n.ZH {
		return i18n.EN
	}
	return l
}

func canvasLang(l i18n.Lang) i18n.Lang { return CanvasLang(l) }

// Draw desenha a tela pedida no tamanho w×h.
func Draw(s config.Screen, w, h int, in Input) *image.RGBA {
	// as fontes embutidas (Roboto) não têm glifos de CJK: para japonês/mandarim,
	// a tela física cai para o inglês (o painel web e a bandeja continuam no idioma escolhido).
	in.Lang = canvasLang(in.Lang)
	th := themeFor(s, in.Cfg.Theme)
	var dc *gg.Context
	switch s {
	case config.ScreenMusic:
		dc = drawMusic(w, h, in, th)
	case config.ScreenGame:
		dc = drawGame(w, h, in, th)
	case config.ScreenSystem:
		dc = drawSystem(w, h, in, th)
	case config.ScreenCustom:
		dc = drawCustom(w, h, in, th)
	default:
		dc = drawClock(w, h, in, th)
	}
	return toRGBA(dc.Image())
}

// Message desenha uma tela simples com um aviso (ex: "conectando...").
func Message(w, h int, title, detail string, t config.ThemeConfig) *image.RGBA {
	th := Theme{Accent: ParseHex(t.AccentClock, color.RGBA{165, 180, 252, 255}), Bg: ParseHex(t.BgColor, color.RGBA{16, 17, 22, 255}), Gradient: t.Background != "solido"}
	dc := newCanvas(w, h, th)
	fw := float64(w)
	textCenter(dc, "BIFROST", fw/2, float64(h)*0.36, face(fontBold, 26), th.Accent)
	textCenter(dc, title, fw/2, float64(h)*0.36+46, face(fontMedium, 16), colFg)
	for i, l := range wrap(dc, detail, face(fontRegular, 13), fw-48, 3) {
		textCenter(dc, l, fw/2, float64(h)*0.36+76+float64(i)*18, face(fontRegular, 13), colDim)
	}
	return toRGBA(dc.Image())
}

func toRGBA(img image.Image) *image.RGBA {
	if r, ok := img.(*image.RGBA); ok {
		return r
	}
	b := img.Bounds()
	out := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			out.Set(x, y, img.At(x, y))
		}
	}
	return out
}

func themeFor(s config.Screen, t config.ThemeConfig) Theme {
	accent := t.AccentClock
	switch s {
	case config.ScreenMusic:
		accent = t.AccentMusic
	case config.ScreenGame:
		accent = t.AccentGame
	case config.ScreenSystem:
		accent = t.AccentSystem
	case config.ScreenCustom:
		accent = t.AccentCustom
	}
	return Theme{
		Accent:   ParseHex(accent, color.RGBA{45, 212, 191, 255}),
		Bg:       ParseHex(t.BgColor, color.RGBA{16, 17, 22, 255}),
		Gradient: t.Background != "solido",
	}
}

// ---------------------------------------------------------------- música

func drawMusic(w, h int, in Input, th Theme) *gg.Context {
	dc := newCanvas(w, h, th)
	fw, fh := float64(w), float64(h)
	m, opt := in.Media, in.Cfg.Screens.Music

	label := i18n.T(in.Lang, "music.now_playing")
	if !m.HasSession {
		label = i18n.T(in.Lang, "music.idle")
	} else if !m.Playing {
		label = i18n.T(in.Lang, "music.paused")
	}
	header(dc, fw, label, th.Accent, m.App)

	if !m.HasSession {
		roundedImage(dc, nil, fw/2-60, fh/2-90, 120, 120, 16, noteIcon(th.Accent))
		textCenter(dc, i18n.T(in.Lang, "music.nothing"), fw/2, fh/2+46, face(fontMedium, 18), colFg)
		textCenter(dc, i18n.T(in.Lang, "music.hint"), fw/2, fh/2+72, face(fontRegular, 13), colDim)
		return dc
	}

	title := m.Title
	if title == "" {
		title = "Sem título"
	}
	sub := m.Artist
	pos := m.EstimatedPosition(in.Now)
	showBar := opt.ShowProgress && m.Duration > 0

	if w > h { // paisagem: capa à esquerda, texto à direita
		art := 0.0
		if opt.ShowCover {
			art = fh - 60 - 64
			roundedImage(dc, m.Cover, 18, 56, art, art, 14, noteIcon(th.Accent))
			art += 18
		}
		x := 18 + art + 4
		tw := fw - x - 18
		y := 64.0
		ft := face(fontBold, 22)
		for _, l := range wrap(dc, title, ft, tw, 3) {
			text(dc, l, x, y, ft, colFg)
			y += lineH(ft)
		}
		y += 6
		if sub != "" {
			fa := face(fontMedium, 16)
			text(dc, ellipsize(dc, sub, fa, tw), x, y, fa, th.Accent)
			y += lineH(fa) + 2
		}
		if opt.ShowAlbum && m.Album != "" {
			fl := face(fontRegular, 14)
			text(dc, ellipsize(dc, m.Album, fl, tw), x, y, fl, colDim)
		}
		if showBar {
			drawProgress(dc, 18, fh-40, fw-36, pos, m.Duration, m.Playing, th.Accent)
		}
		return dc
	}

	// retrato
	y := 58.0
	if opt.ShowCover {
		art := math.Min(fw-64, fh*0.46)
		roundedImage(dc, m.Cover, (fw-art)/2, y, art, art, 18, noteIcon(th.Accent))
		y += art + 22
	} else {
		y = fh * 0.28
	}
	ft := face(fontBold, 23)
	maxLines := 2
	if !opt.ShowCover {
		ft, maxLines = face(fontBold, 30), 3
	}
	for _, l := range wrap(dc, title, ft, fw-36, maxLines) {
		textCenter(dc, l, fw/2, y, ft, colFg)
		y += lineH(ft)
	}
	y += 4
	if sub != "" {
		fa := face(fontMedium, 16)
		textCenter(dc, ellipsize(dc, sub, fa, fw-40), fw/2, y, fa, th.Accent)
		y += lineH(fa) + 2
	}
	if opt.ShowAlbum && m.Album != "" {
		fl := face(fontRegular, 14)
		textCenter(dc, ellipsize(dc, m.Album, fl, fw-40), fw/2, y, fl, colDim)
	}
	if showBar {
		drawProgress(dc, 22, fh-46, fw-44, pos, m.Duration, m.Playing, th.Accent)
	}
	return dc
}

func drawProgress(dc *gg.Context, x, y, w float64, pos, dur time.Duration, playing bool, accent color.RGBA) {
	bar(dc, x, y, w, 6, pos.Seconds()/dur.Seconds(), accent)
	f := face(fontMono, 13)
	text(dc, mmss(pos), x, y+13, f, colDim)
	textRight(dc, mmss(dur), x+w, y+13, f, colDim)
	// ícone de play/pause no meio
	if playing {
		pauseIcon(dc, x+w/2-6, y+14, 12, colDim)
	} else {
		playIcon(dc, x+w/2-4, y+14, 12, colDim)
	}
}

func mmss(d time.Duration) string {
	s := int(d.Seconds())
	if s < 0 {
		s = 0
	}
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, (s%3600)/60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// ---------------------------------------------------------------- jogo

func drawGame(w, h int, in Input, th Theme) *gg.Context {
	if in.GameLive.Active {
		return drawLiveMatch(w, h, in, th)
	}
	dc := newCanvas(w, h, th)
	fw, fh := float64(w), float64(h)
	g, opt := in.Steam, in.Cfg.Screens.Game

	right := ""
	if g.PersonaName != "" {
		right = g.PersonaName
	}
	header(dc, fw, i18n.T(in.Lang, "game.header"), th.Accent, right)

	if !g.Playing {
		roundedImage(dc, nil, fw/2-70, fh/2-90, 140, 110, 16, padIcon(th.Accent))
		msg, det := i18n.T(in.Lang, "game.empty_title"), i18n.T(in.Lang, "game.empty_detail")
		if !in.SteamReady {
			msg, det = i18n.T(in.Lang, "game.notfound_title"), i18n.T(in.Lang, "game.notfound_detail")
			if in.Cfg.Steam.Source == "web" {
				msg, det = i18n.T(in.Lang, "game.notconfigured_title"), i18n.T(in.Lang, "game.notconfigured_detail")
			}
		}
		textCenter(dc, msg, fw/2, fh/2+36, face(fontMedium, 18), colFg)
		textCenter(dc, det, fw/2, fh/2+62, face(fontRegular, 13), colDim)
		return dc
	}

	type stat struct{ label, value string }
	var stats []stat
	if opt.ShowTotal {
		stats = append(stats, stat{i18n.T(in.Lang, "game.total"), i18n.Playtime(in.Lang, g.TotalMinutes)})
	}
	if opt.ShowTwoWeeks {
		stats = append(stats, stat{i18n.T(in.Lang, "game.twoweeks"), i18n.Playtime(in.Lang, g.TwoWeeksMinutes)})
	}
	session := ""
	if opt.ShowSession && !g.SessionStart.IsZero() {
		session = i18n.SessionText(in.Lang, in.Now.Sub(g.SessionStart))
	}
	// CPU/GPU (uso + temperatura), pra aproveitar o espaço sobrando na tela do jogo.
	perf := []stat{
		{"CPU", formatPerf(in.System.CPU, in.System.CPUTemp)},
		{"GPU", formatPerf(in.System.GPU, in.System.GPUTemp)},
	}
	if in.FPS > 0 {
		perf = append(perf, stat{"FPS", fmt.Sprintf("%.0f", in.FPS)})
	}

	drawStats := func(list []stat, x, y, width float64) float64 {
		if len(list) == 0 {
			return y
		}
		gap := 10.0
		cw := (width - gap*float64(len(list)-1)) / float64(len(list))
		for i, s := range list {
			cx := x + float64(i)*(cw+gap)
			card(dc, cx, y, cw, 58, 10)
			text(dc, s.label, cx+12, y+10, face(fontMedium, 11), colDim)
			fv := face(fontMonoB, 18)
			for size := 18.0; measure(dc, s.value, fv) > cw-20 && size > 11; size-- {
				fv = face(fontMonoB, size-1)
			}
			text(dc, s.value, cx+12, y+30, fv, colFg)
		}
		return y + 58
	}

	if w > h { // paisagem
		y := 56.0
		if opt.ShowCover {
			aw := fw * 0.48
			ah := aw * 215 / 460
			roundedImage(dc, g.Cover, 16, y, aw, ah, 12, padIcon(th.Accent))
			x := 16 + aw + 14
			tw := fw - x - 16
			ft := face(fontBold, 20)
			ty := y + 2
			for _, l := range wrap(dc, g.Name, ft, tw, 3) {
				text(dc, l, x, ty, ft, colFg)
				ty += lineH(ft)
			}
			if session != "" {
				text(dc, session, x, ty+6, face(fontMedium, 13), th.Accent)
			}
			y += ah + 14
		} else {
			ft := face(fontBold, 26)
			for _, l := range wrap(dc, g.Name, ft, fw-32, 2) {
				text(dc, l, 16, y, ft, colFg)
				y += lineH(ft)
			}
			if session != "" {
				text(dc, session, 16, y+4, face(fontMedium, 14), th.Accent)
				y += 26
			}
			y += 10
		}
		statsY := drawStats(stats, 16, math.Min(y, fh-70), fw-32)
		if perfY := statsY + 10; perfY+58 <= fh-8 {
			drawStats(perf, 16, perfY, fw-32)
		}
		return dc
	}

	// retrato
	y := 56.0
	if opt.ShowCover {
		aw := fw - 32
		ah := aw * 215 / 460
		roundedImage(dc, g.Cover, 16, y, aw, ah, 14, padIcon(th.Accent))
		y += ah + 20
	} else {
		y = fh * 0.24
	}
	ft := face(fontBold, 23)
	if !opt.ShowCover {
		ft = face(fontBold, 30)
	}
	for _, l := range wrap(dc, g.Name, ft, fw-36, 2) {
		textCenter(dc, l, fw/2, y, ft, colFg)
		y += lineH(ft)
	}
	y += 14
	y = drawStats(stats, 16, y, fw-32)
	if session != "" {
		textCenter(dc, session, fw/2, y+14, face(fontMedium, 14), th.Accent)
		y += 28
	}
	if perfY := y + 10; perfY+58 <= fh-8 {
		drawStats(perf, 16, perfY, fw-32)
	}
	return dc
}

// formatPerf formata uso (%) + temperatura (°C) de CPU/GPU num só texto,
// pulando o que não tiver leitura disponível.
func formatPerf(pct, tempC float64) string {
	v := "--"
	if pct >= 0 {
		v = fmt.Sprintf("%.0f%%", pct)
	}
	if tempC >= 0 {
		if v == "--" {
			v = fmt.Sprintf("%.0f°C", tempC)
		} else {
			v += fmt.Sprintf(" · %.0f°C", tempC)
		}
	}
	return v
}

// drawLiveMatch desenha o resumo de uma partida ao vivo (CS2, Dota 2 ou LoL),
// no lugar do resumo padrão da Steam enquanto a partida durar.
func drawLiveMatch(w, h int, in Input, th Theme) *gg.Context {
	dc := newCanvas(w, h, th)
	fw, fh := float64(w), float64(h)
	lm := in.GameLive

	drawGameWordmark(dc, fw, fh, lm.Game)

	header(dc, fw, strings.ToUpper(lm.Game), th.Accent, lm.Score)

	y := 58.0
	ft := face(fontBold, 22)
	for _, l := range wrap(dc, lm.Title, ft, fw-32, 2) {
		textCenter(dc, l, fw/2, y, ft, colFg)
		y += lineH(ft)
	}
	if lm.Sub != "" {
		y += 4
		textCenter(dc, lm.Sub, fw/2, y, face(fontMedium, 13), th.Accent)
		y += 20
	}
	if lm.Alert != "" {
		y += 8
		card(dc, 16, y, fw-32, 30, 8)
		textCenter(dc, strings.ToUpper(lm.Alert), fw/2, y+20, face(fontBold, 12), th.Accent)
		y += 40
	}
	y += 12

	stats := append([]LiveStat(nil), lm.Stats...)
	stats = append(stats,
		LiveStat{Label: "CPU", Value: formatPerf(in.System.CPU, in.System.CPUTemp)},
		LiveStat{Label: "GPU", Value: formatPerf(in.System.GPU, in.System.GPUTemp)},
	)
	if in.FPS > 0 {
		stats = append(stats, LiveStat{Label: "FPS", Value: fmt.Sprintf("%.0f", in.FPS)})
	}

	gap := 10.0
	cw := (fw - 32 - gap) / 2
	rowH := 58.0
	for i := 0; i < len(stats); i += 2 {
		if y+rowH > fh-8 {
			break
		}
		row := stats[i:minInt(i+2, len(stats))]
		for j, s := range row {
			cx := 16 + float64(j)*(cw+gap)
			cardW := cw
			if len(row) == 1 {
				cardW = fw - 32
			}
			card(dc, cx, y, cardW, rowH, 10)
			text(dc, s.Label, cx+12, y+10, face(fontMedium, 11), colDim)
			fv := face(fontMonoB, 18)
			for size := 18.0; measure(dc, s.Value, fv) > cardW-20 && size > 11; size-- {
				fv = face(fontMonoB, size-1)
			}
			text(dc, s.Value, cx+12, y+30, fv, colFg)
		}
		y += rowH + gap
	}

	return dc
}

// drawGameWordmark dá um fundo mais escuro (parecido com o HUD do jogo) e
// desenha o nome bem grande, apagado e encostado na borda direita — um
// floreio visual atrás dos cartões de estatística (desenhados por cima).
// É só o nome em texto: nenhum logo/arte oficial é reproduzido.
func drawGameWordmark(dc *gg.Context, fw, fh float64, game string) {
	label, tint := "", color.RGBA{}
	switch game {
	case "CS2":
		label, tint = "CS2", color.RGBA{50, 95, 140, 255} // frio, lembra o HUD azulado do CS2
	case "Dota 2":
		label, tint = "DOTA 2", color.RGBA{130, 45, 40, 255} // vermelho, cara do Dota
	case "League of Legends":
		label, tint = "LOL", color.RGBA{40, 80, 150, 255}
	default:
		return
	}

	// vinheta mais forte perto do canto onde o nome fica, apagando pro resto da tela
	glow := gg.NewRadialGradient(fw, fh*0.5, 0, fw*0.25, fh*0.5, fw*1.15)
	glow.AddColorStop(0, withAlpha(tint, 190))
	glow.AddColorStop(1, withAlpha(tint, 0))
	dc.SetFillStyle(glow)
	dc.DrawRectangle(0, 0, fw, fh)
	dc.Fill()

	// tamanho começa relativo à altura, mas encolhe até o texto inteiro caber
	// na largura — nomes maiores como "DOTA 2" senão vazavam pra fora da tela.
	maxW := fw * 0.92
	size := fh * 0.6
	f := face(fontBold, size)
	for size > 16 && measure(dc, label, f) > maxW {
		size -= 2
		f = face(fontBold, size)
	}
	dc.SetFontFace(f)
	dc.SetColor(withAlpha(colFg, 60))
	dc.DrawStringAnchored(label, fw-14, fh*0.54, 1, 0.5)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ---------------------------------------------------------------- sistema

func drawSystem(w, h int, in Input, th Theme) *gg.Context {
	dc := newCanvas(w, h, th)
	fw, fh := float64(w), float64(h)
	s, opt := in.System, in.Cfg.Screens.System

	right := ""
	if opt.ShowUptime && s.Uptime > 0 {
		right = i18n.T(in.Lang, "system.uptime_prefix") + i18n.Uptime(in.Lang, s.Uptime)
	}
	header(dc, fw, i18n.T(in.Lang, "system.header"), th.Accent, right)

	type gauge struct {
		label string
		value float64
		temp  float64 // °C, ou -1 quando não há leitura
	}
	var gauges []gauge
	if opt.ShowCPU {
		gauges = append(gauges, gauge{"CPU", s.CPU, s.CPUTemp})
	}
	if opt.ShowGPU {
		gauges = append(gauges, gauge{"GPU", s.GPU, s.GPUTemp})
	}
	landscape := w > h
	type row struct {
		label, value string
		frac         float64
	}
	pair := func(used, total uint64) string {
		if landscape {
			return gbPair(used, total)
		}
		return fmt.Sprintf("%s / %s", gb(used), gb(total))
	}
	var rows []row
	if opt.ShowRAM {
		rows = append(rows, row{"RAM", pair(s.RAMUsed, s.RAMTotal), s.RAMPercent() / 100})
	}
	if opt.ShowDisk {
		rows = append(rows, row{i18n.T(in.Lang, "system.disk") + opt.Disk, pair(s.DiskUsed, s.DiskTotal), s.DiskPercent() / 100})
	}

	y := 56.0
	if len(gauges) > 0 {
		size := math.Min((fw-32-16*float64(len(gauges)-1))/float64(len(gauges)), 128)
		if landscape {
			size = math.Min(size, fh*0.42)
		}
		total := size*float64(len(gauges)) + 16*float64(len(gauges)-1)
		x := (fw - total) / 2
		for _, g := range gauges {
			drawGauge(dc, x+size/2, y+size/2, size/2, g.value, g.temp, g.label, th.Accent)
			x += size + 16
		}
		y += size + 12
	}
	if opt.ShowCPU && !landscape && len(s.CPUHistory) > 1 {
		drawSpark(dc, 16, y, fw-32, 34, s.CPUHistory, th.Accent)
		y += 46
	}

	if landscape {
		// linhas lado a lado na parte de baixo
		n := len(rows)
		if opt.ShowNet {
			n++
		}
		if n == 0 {
			return dc
		}
		gap := 10.0
		cw := (fw - 32 - gap*float64(n-1)) / float64(n)
		x := 16.0
		cy := math.Max(y, fh-66) // cartões presos embaixo
		for _, r := range rows {
			drawRowCard(dc, x, cy, cw, r.label, r.value, r.frac, th.Accent, true)
			x += cw + gap
		}
		if opt.ShowNet {
			drawNetCard(dc, x, cy, cw, s, i18n.T(in.Lang, "system.net"), th.Accent, true)
		}
		return dc
	}

	for _, r := range rows {
		if y+54 > fh-8 {
			break
		}
		drawRowCard(dc, 16, y, fw-32, r.label, r.value, r.frac, th.Accent, false)
		y += 62
	}
	if opt.ShowNet && y+54 <= fh-8 {
		drawNetCard(dc, 16, y, fw-32, s, i18n.T(in.Lang, "system.net"), th.Accent, false)
	}
	return dc
}

// drawGauge desenha o arco de uso e, quando há leitura, a temperatura logo
// abaixo do número (tempC < 0 significa "sem sensor disponível").
func drawGauge(dc *gg.Context, cx, cy, r, value, tempC float64, label string, accent color.RGBA) {
	start, sweep := math.Pi*0.75, math.Pi*1.5
	lw := math.Max(8, r*0.16)
	dc.SetLineCapRound()
	dc.SetLineWidth(lw)
	dc.SetColor(colTrack)
	dc.DrawArc(cx, cy, r-lw/2, start, start+sweep)
	dc.Stroke()
	txt := "--"
	if value >= 0 {
		txt = fmt.Sprintf("%.0f%%", value)
		if value > 0.5 {
			dc.SetColor(accent)
			dc.DrawArc(cx, cy, r-lw/2, start, start+sweep*math.Min(value, 100)/100)
			dc.Stroke()
		}
	}
	fv := face(fontMonoB, math.Round(r*0.42))
	dc.SetFontFace(fv)
	tw, _ := dc.MeasureString(txt)
	text(dc, txt, cx-tw/2, cy-capH(fv)/2-6, fv, colFg)
	if tempC >= 0 {
		ft := face(fontMonoB, math.Max(10, math.Round(r*0.2)))
		textCenter(dc, fmt.Sprintf("%.0f°C", tempC), cx, cy+r*0.16, ft, colDim)
	}
	fl := face(fontMedium, math.Max(11, math.Round(r*0.2)))
	textCenter(dc, label, cx, cy+r*0.5, fl, colDim)
}

func drawSpark(dc *gg.Context, x, y, w, h float64, vals []float64, accent color.RGBA) {
	card(dc, x, y, w, h, 8)
	n := len(vals)
	step := (w - 16) / float64(max(n-1, 1))
	dc.SetLineWidth(2)
	dc.SetLineCapRound()
	dc.SetLineJoinRound()
	for i, v := range vals {
		px := x + 8 + float64(i)*step
		py := y + h - 6 - (h-12)*math.Max(0, math.Min(v, 100))/100
		if i == 0 {
			dc.MoveTo(px, py)
		} else {
			dc.LineTo(px, py)
		}
	}
	dc.SetColor(accent)
	dc.Stroke()
}

func drawRowCard(dc *gg.Context, x, y, w float64, label, value string, frac float64, accent color.RGBA, compact bool) {
	card(dc, x, y, w, 54, 10)
	fl := face(fontMedium, 11)
	text(dc, label, x+12, y+9, fl, colDim)
	fv := face(fontMonoB, 14)
	if compact {
		text(dc, ellipsize(dc, value, fv, w-24), x+12, y+24, fv, colFg)
	} else {
		textRight(dc, value, x+w-12, y+8, fv, colFg)
	}
	if frac >= 0 {
		bar(dc, x+12, y+(map[bool]float64{true: 43, false: 34}[compact]), w-24, 6, frac, accent)
	}
}

func drawNetCard(dc *gg.Context, x, y, w float64, s sysinfo.Stats, label string, accent color.RGBA, compact bool) {
	card(dc, x, y, w, 54, 10)
	text(dc, label, x+12, y+9, face(fontMedium, 11), colDim)
	fv := face(fontMonoB, 14)
	down, up := rate(s.NetDown), rate(s.NetUp)
	if compact {
		fc := face(fontMonoB, 13)
		arrow(dc, x+12, y+24, 9, true, colFg)
		text(dc, ellipsize(dc, down, fc, w-38), x+26, y+22, fc, colFg)
		fu := face(fontMono, 12)
		arrow(dc, x+12, y+40, 9, false, accent)
		text(dc, ellipsize(dc, up, fu, w-38), x+26, y+38, fu, colDim)
		return
	}
	arrow(dc, x+12, y+28, 11, true, colFg)
	text(dc, down, x+28, y+27, fv, colFg)
	uw := measure(dc, up, fv)
	arrow(dc, x+w-12-uw-16, y+28, 11, false, accent)
	textRight(dc, up, x+w-12, y+27, fv, accent)
}

// arrow desenha uma seta para baixo (download) ou para cima (upload).
func arrow(dc *gg.Context, x, y, size float64, down bool, c color.RGBA) {
	dc.SetColor(c)
	dc.SetLineWidth(2)
	dc.SetLineCapRound()
	cx := x + size/2
	if down {
		dc.DrawLine(cx, y, cx, y+size)
		dc.Stroke()
		dc.MoveTo(x, y+size*0.55)
		dc.LineTo(cx, y+size+1)
		dc.LineTo(x+size, y+size*0.55)
	} else {
		dc.DrawLine(cx, y, cx, y+size)
		dc.Stroke()
		dc.MoveTo(x, y+size*0.45)
		dc.LineTo(cx, y-1)
		dc.LineTo(x+size, y+size*0.45)
	}
	dc.SetLineJoinRound()
	dc.Stroke()
}

func gb(b uint64) string {
	if b == 0 {
		return "--"
	}
	v := float64(b) / (1 << 30)
	if v >= 100 {
		return fmt.Sprintf("%.0f GB", v)
	}
	return strings.Replace(fmt.Sprintf("%.1f GB", v), ".", ",", 1)
}

// gbPair mostra "11/32 GB" (versão curta para os cartões estreitos).
func gbPair(used, total uint64) string {
	if total == 0 {
		return "--"
	}
	g := func(b uint64) string { return fmt.Sprintf("%.0f", float64(b)/(1<<30)) }
	if total >= 1<<40 {
		t := func(b uint64) string { return strings.Replace(fmt.Sprintf("%.1f", float64(b)/(1<<40)), ".", ",", 1) }
		return t(used) + "/" + t(total) + " TB"
	}
	return g(used) + "/" + g(total) + " GB"
}

func rate(bps float64) string {
	switch {
	case bps < 0:
		return "--"
	case bps >= 1<<20:
		return strings.Replace(fmt.Sprintf("%.1f MB/s", bps/(1<<20)), ".", ",", 1)
	case bps >= 1<<10:
		return fmt.Sprintf("%.0f KB/s", bps/(1<<10))
	}
	return fmt.Sprintf("%.0f B/s", bps)
}

// ---------------------------------------------------------------- relógio

func drawClock(w, h int, in Input, th Theme) *gg.Context {
	dc := newCanvas(w, h, th)
	fw, fh := float64(w), float64(h)
	opt := in.Cfg.Screens.Clock
	now := in.Now

	hh := now.Hour()
	suffix := ""
	if !opt.Use24h {
		suffix = i18n.AMPM(in.Lang, hh >= 12)
		hh %= 12
		if hh == 0 {
			hh = 12
		}
	}
	main := fmt.Sprintf("%02d:%02d", hh, now.Minute())
	size := math.Min(fw/2.9, 116)
	if w > h {
		size = math.Min(fh/3.1, 116)
	}
	ft := face(fontBold, math.Round(size))
	extra := ""
	if opt.ShowSeconds {
		extra = strings.TrimSpace(fmt.Sprintf("%02d %s", now.Second(), suffix))
	} else {
		extra = suffix
	}
	fs := face(fontMono, math.Round(size*0.28))
	fd := face(fontMedium, 18)

	// altura total do bloco: dígitos + (segundos/AM-PM) + barrinha + data
	block := capH(ft) + 14
	if extra != "" {
		block += lineH(fs) + 4
	}
	block += 18
	if opt.ShowDate {
		block += lineH(fd)
	}
	top := (fh-block)/2 - 6

	drawTabular(dc, main, fw/2, top-(ascent(ft)-capH(ft)), ft, colFg)
	ty := top + capH(ft) + 14
	if extra != "" {
		textCenter(dc, extra, fw/2, ty, fs, th.Accent)
		ty += lineH(fs) + 4
	}
	dc.SetColor(th.Accent)
	dc.DrawRoundedRectangle(fw/2-18, ty, 36, 4, 2)
	dc.Fill()
	ty += 18
	if opt.ShowDate {
		date := i18n.DateLine(in.Lang, now)
		textCenter(dc, ellipsize(dc, date, fd, fw-32), fw/2, ty, fd, colDim)
	}
	return dc
}

// drawTabular desenha números com todos os dígitos na mesma largura
// (o relógio não "pula" quando os dígitos mudam), centralizado em cx.
func drawTabular(dc *gg.Context, s string, cx, y float64, f font.Face, c color.Color) {
	digitW := 0.0
	for _, d := range "0123456789" {
		digitW = math.Max(digitW, measure(dc, string(d), f))
	}
	cell := func(r rune) float64 {
		if r == ':' {
			return digitW * 0.42
		}
		return digitW
	}
	total := 0.0
	for _, r := range s {
		total += cell(r)
	}
	x := cx - total/2
	for _, r := range s {
		cw := cell(r)
		gw := measure(dc, string(r), f)
		dy := 0.0
		if r == ':' {
			dy = -capH(f) * 0.06
		}
		text(dc, string(r), x+(cw-gw)/2, y+dy, f, c)
		x += cw
	}
}
