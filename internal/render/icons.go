package render

import (
	"image/color"

	"github.com/fogleman/gg"
)

func noteIcon(accent color.RGBA) func(dc *gg.Context, cx, cy, size float64) {
	return func(dc *gg.Context, cx, cy, size float64) {
		s := size * 0.42
		dc.SetColor(accent)
		stemX := cx + s*0.18
		top, bottom := cy-s/2, cy+s/2
		dc.DrawRectangle(stemX, top, s*0.08, s)
		dc.Fill()
		dc.DrawEllipse(stemX-s*0.14, bottom, s*0.22, s*0.16)
		dc.Fill()
		dc.MoveTo(stemX+s*0.08, top)
		dc.LineTo(stemX+s*0.42, top+s*0.12)
		dc.LineTo(stemX+s*0.42, top+s*0.32)
		dc.LineTo(stemX+s*0.08, top+s*0.2)
		dc.ClosePath()
		dc.Fill()
	}
}

func padIcon(accent color.RGBA) func(dc *gg.Context, cx, cy, size float64) {
	return func(dc *gg.Context, cx, cy, size float64) {
		w := size * 0.62
		h := w * 0.55
		dc.SetColor(accent)
		dc.DrawRoundedRectangle(cx-w/2, cy-h/2, w, h, h/2)
		dc.Fill()
		dark := color.RGBA{20, 21, 27, 255}
		dc.SetColor(dark)
		arm, th := h*0.17, h*0.11
		dx := cx - w*0.24
		dc.DrawRectangle(dx-arm, cy-th/2, arm*2, th)
		dc.DrawRectangle(dx-th/2, cy-arm, th, arm*2)
		dc.Fill()
		bx := cx + w*0.24
		r := h * 0.09
		for _, o := range [][2]float64{{-1.7, 0}, {1.7, 0}, {0, -1.7}, {0, 1.7}} {
			dc.DrawCircle(bx+o[0]*r, cy+o[1]*r, r)
		}
		dc.Fill()
	}
}

func pauseIcon(dc *gg.Context, x, y, h float64, c color.RGBA) {
	dc.SetColor(c)
	w := h * 0.32
	dc.DrawRoundedRectangle(x, y, w, h, 2)
	dc.DrawRoundedRectangle(x+w*1.9, y, w, h, 2)
	dc.Fill()
}

func playIcon(dc *gg.Context, x, y, h float64, c color.RGBA) {
	dc.SetColor(c)
	dc.MoveTo(x, y)
	dc.LineTo(x+h*0.85, y+h/2)
	dc.LineTo(x, y+h)
	dc.ClosePath()
	dc.Fill()
}
