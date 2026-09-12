package i18n

import (
	"testing"
	"time"
)

func TestIsSupportedAndName(t *testing.T) {
	for _, l := range Supported {
		if !IsSupported(l) {
			t.Errorf("%s deveria ser suportado", l)
		}
		if Name(l) == "" {
			t.Errorf("Name(%s) vazio", l)
		}
	}
	if IsSupported("xx") {
		t.Error("xx não deveria ser suportado")
	}
	if got := Name("xx"); got != "English" {
		t.Errorf("Name(xx) = %q, esperava fallback para English", got)
	}
}

func TestResolve(t *testing.T) {
	for _, l := range Supported {
		if got := Resolve(string(l)); got != l {
			t.Errorf("Resolve(%s) = %s", l, got)
		}
	}
	if got := Resolve("nao-suportado"); got != EN && !IsSupported(got) {
		t.Errorf("Resolve(idioma desconhecido) = %s, esperava um idioma suportado", got)
	}
	// "auto" ou vazio cai no Detect(), que sempre devolve um idioma suportado.
	if got := Resolve("auto"); !IsSupported(got) {
		t.Errorf("Resolve(auto) = %s, não é suportado", got)
	}
	if got := Resolve(""); !IsSupported(got) {
		t.Errorf("Resolve(\"\") = %s, não é suportado", got)
	}
}

func TestDetectReturnsSupported(t *testing.T) {
	if got := Detect(); !IsSupported(got) {
		t.Errorf("Detect() = %s, não é suportado", got)
	}
}

func TestT(t *testing.T) {
	if got := T(EN, "music.now_playing"); got != "NOW PLAYING" {
		t.Errorf("T(EN, music.now_playing) = %q", got)
	}
	if got := T(PT, "music.now_playing"); got != "TOCANDO AGORA" {
		t.Errorf("T(PT, music.now_playing) = %q", got)
	}
	// idioma sem suporte cai no inglês.
	if got := T(Lang("xx"), "music.now_playing"); got != "NOW PLAYING" {
		t.Errorf("T(xx, ...) deveria cair no inglês, veio %q", got)
	}
	// key desconhecida devolve a própria key.
	if got := T(EN, "chave.inexistente"); got != "chave.inexistente" {
		t.Errorf("T com key desconhecida = %q", got)
	}
	if got := T(EN, "app.connected", "COM3"); got != "Connected on COM3" {
		t.Errorf("T com args = %q", got)
	}
}

func TestPlaytime(t *testing.T) {
	cases := []struct {
		lang Lang
		min  int
		want string
	}{
		{EN, 0, "0min"},
		{EN, 45, "45min"},
		{EN, 60, "1h"},
		{EN, 125, "2h 5min"},
		{JA, 125, "2時間5分"},
		{ZH, 125, "2小时5分钟"},
	}
	for _, c := range cases {
		if got := Playtime(c.lang, c.min); got != c.want {
			t.Errorf("Playtime(%s, %d) = %q, esperava %q", c.lang, c.min, got, c.want)
		}
	}
}

func TestSessionText(t *testing.T) {
	got := SessionText(EN, 95*time.Minute)
	want := "1h 35min · this session"
	if got != want {
		t.Errorf("SessionText = %q, esperava %q", got, want)
	}
}

func TestUptime(t *testing.T) {
	if got := Uptime(EN, 26*time.Hour+14*time.Minute); got != "1d 2h" {
		t.Errorf("Uptime(1d2h) = %q", got)
	}
	if got := Uptime(EN, 6*time.Hour+3*time.Minute); got != "6h 03min" {
		t.Errorf("Uptime(6h3min) = %q", got)
	}
}

func TestAMPM(t *testing.T) {
	if got := AMPM(EN, false); got != "AM" {
		t.Errorf("AMPM(EN,false) = %q", got)
	}
	if got := AMPM(EN, true); got != "PM" {
		t.Errorf("AMPM(EN,true) = %q", got)
	}
	if got := AMPM(JA, true); got != "午後" {
		t.Errorf("AMPM(JA,true) = %q", got)
	}
	if got := AMPM(ZH, false); got != "上午" {
		t.Errorf("AMPM(ZH,false) = %q", got)
	}
}

func TestDateLine(t *testing.T) {
	d := time.Date(2026, time.September, 11, 0, 0, 0, 0, time.UTC) // sexta-feira
	cases := map[Lang]string{
		EN: "Friday, September 11",
		PT: "sexta, 11 de setembro",
		ES: "viernes, 11 de septiembre",
		ZH: "9月11日 星期五",
	}
	for lang, want := range cases {
		if got := DateLine(lang, d); got != want {
			t.Errorf("DateLine(%s) = %q, esperava %q", lang, got, want)
		}
	}
	// idioma desconhecido não deve entrar em pânico (cai para inglês).
	if got := DateLine("xx", d); got == "" {
		t.Error("DateLine com idioma desconhecido não deveria ficar vazio")
	}
}
