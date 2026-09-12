package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	d := Default()
	if d.Display.Port != "AUTO" || d.Display.Orientation != OrientPortrait {
		t.Errorf("display padrão inesperado: %+v", d.Display)
	}
	if len(d.Screens.Order) != len(AllScreens) {
		t.Errorf("ordem padrão deveria ter %d telas, tem %d", len(AllScreens), len(d.Screens.Order))
	}
	if d.General.Language != "auto" {
		t.Errorf("idioma padrão deveria ser auto, veio %q", d.General.Language)
	}
	if d.Steam.Source != "local" {
		t.Errorf("fonte da steam padrão deveria ser local")
	}
}

func TestNormalizeDefaults(t *testing.T) {
	var c Config
	c.Normalize()
	d := Default()
	if c.Display.Port != d.Display.Port {
		t.Errorf("porta vazia deveria virar AUTO, veio %q", c.Display.Port)
	}
	if c.Display.Revision != d.Display.Revision {
		t.Errorf("revisão inválida deveria cair no padrão")
	}
	if c.Display.Orientation != d.Display.Orientation {
		t.Errorf("orientação inválida deveria cair no padrão")
	}
	if c.Mode.Type != ModeAuto {
		t.Errorf("modo inválido deveria virar automático, veio %q", c.Mode.Type)
	}
	if c.Steam.Source != "local" {
		t.Errorf("fonte da steam inválida deveria virar local")
	}
	if c.General.Language != "auto" {
		t.Errorf("idioma vazio deveria virar auto, veio %q", c.General.Language)
	}
	if c.Theme.Background != "gradiente" {
		t.Errorf("fundo inválido deveria virar gradiente")
	}
	if c.Screens.System.Disk != "C:" {
		t.Errorf("disco vazio deveria virar C:, veio %q", c.Screens.System.Disk)
	}
}

func TestNormalizeClampsAndFixes(t *testing.T) {
	c := Config{
		Display: DisplayConfig{Port: " com3 ", Revision: "simulado", Orientation: OrientLandscape, Brightness: 500},
		Screens: ScreensConfig{Order: []Screen{"relogio", "xyz", "relogio", "jogo"}, System: SystemScreen{Disk: "  d  "}},
		Mode:    ModeConfig{Type: "rotacao", RotateSeconds: -5, Fixed: "invalido"},
		Theme:   ThemeConfig{AccentMusic: "vermelho", AccentGame: "#ABCDEF", Background: "outro", BgColor: "#123"},
		Steam:   SteamConfig{Source: "outra-coisa", APIKey: "  chave  ", SteamID64: " 123 ", StatusSeconds: 1, LibrarySeconds: 1},
		General: GeneralConfig{RefreshMillis: 10, WebPort: 80, Language: "fr"},
	}
	c.Normalize()

	if c.Display.Port != "COM3" {
		t.Errorf("porta deveria virar COM3 maiúsculo e sem espaços, veio %q", c.Display.Port)
	}
	if c.Display.Revision != "SIMULADO" {
		t.Errorf("revisão deveria virar SIMULADO, veio %q", c.Display.Revision)
	}
	if c.Display.Brightness != 100 {
		t.Errorf("brilho deveria ser limitado a 100, veio %d", c.Display.Brightness)
	}
	wantOrder := []Screen{ScreenClock, ScreenGame, ScreenMusic, ScreenSystem}
	if len(c.Screens.Order) != len(wantOrder) {
		t.Fatalf("ordem deveria ter %d telas (sem duplicatas/desconhecidas + completada), veio %v", len(wantOrder), c.Screens.Order)
	}
	if c.Screens.Order[0] != ScreenClock || c.Screens.Order[1] != ScreenGame {
		t.Errorf("ordem deveria manter relogio,jogo primeiro e completar o resto, veio %v", c.Screens.Order)
	}
	if c.Screens.System.Disk != "D:" {
		t.Errorf("disco deveria virar D: (maiúsculo, com dois pontos), veio %q", c.Screens.System.Disk)
	}
	if c.Mode.RotateSeconds != 3 {
		t.Errorf("rotação deveria ser limitada a 3s no mínimo, veio %d", c.Mode.RotateSeconds)
	}
	if c.Mode.Fixed != ScreenClock {
		t.Errorf("tela fixa inválida deveria cair no relógio, veio %q", c.Mode.Fixed)
	}
	if c.Theme.AccentMusic != Default().Theme.AccentMusic {
		t.Errorf("cor inválida deveria cair no padrão")
	}
	if c.Theme.AccentGame != "#abcdef" {
		t.Errorf("cor válida deveria virar minúscula, veio %q", c.Theme.AccentGame)
	}
	if c.Theme.Background != "gradiente" {
		t.Errorf("fundo inválido deveria virar gradiente")
	}
	if c.Theme.BgColor != Default().Theme.BgColor {
		t.Errorf("cor de fundo curta demais deveria cair no padrão")
	}
	if c.Steam.Source != "local" {
		t.Errorf("fonte inválida deveria virar local")
	}
	if c.Steam.APIKey != "chave" || c.Steam.SteamID64 != "123" {
		t.Errorf("api key/steamid deveriam ser aparados, veio %q/%q", c.Steam.APIKey, c.Steam.SteamID64)
	}
	if c.Steam.StatusSeconds != 10 || c.Steam.LibrarySeconds != 60 {
		t.Errorf("intervalos da steam deveriam ser limitados ao mínimo, veio %d/%d", c.Steam.StatusSeconds, c.Steam.LibrarySeconds)
	}
	if c.General.RefreshMillis != 250 {
		t.Errorf("atualização deveria ser limitada a 250ms, veio %d", c.General.RefreshMillis)
	}
	if c.General.WebPort != Default().General.WebPort {
		t.Errorf("porta do painel fora da faixa deveria cair no padrão")
	}
	if c.General.Language != "auto" {
		t.Errorf("idioma não suportado deveria virar auto, veio %q", c.General.Language)
	}
}

func TestEnabled(t *testing.T) {
	c := Default()
	if !c.Enabled(ScreenMusic) || !c.Enabled(ScreenSystem) || !c.Enabled(ScreenClock) {
		t.Error("telas padrão deveriam estar ligadas")
	}
	if !c.Enabled(ScreenGame) {
		t.Error("jogo deveria estar ligado (Steam e tela ligadas por padrão)")
	}
	c.Steam.Enabled = false
	if c.Enabled(ScreenGame) {
		t.Error("jogo não deveria estar ligado com Steam desligada")
	}
	if c.Enabled(Screen("desconhecida")) {
		t.Error("tela desconhecida nunca deveria estar ligada")
	}
}

func TestResolvedLanguage(t *testing.T) {
	c := Default()
	c.General.Language = "es"
	if got := c.ResolvedLanguage(); got != "es" {
		t.Errorf("ResolvedLanguage() = %s, esperava es", got)
	}
}

func TestDirDoesNotPanic(t *testing.T) {
	if Dir() == "" {
		t.Error("Dir() não deveria devolver vazio")
	}
}

func TestStoreOpenCreatesDefault(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json")); err != nil {
		t.Errorf("config.json deveria ter sido criado: %v", err)
	}
	if s.Path() != filepath.Join(dir, "config.json") {
		t.Errorf("Path() = %q", s.Path())
	}
	got := s.Get()
	if got.Display.Port != "AUTO" {
		t.Errorf("config recém-criada deveria ser o padrão")
	}
}

func TestStoreOpenExisting(t *testing.T) {
	dir := t.TempDir()
	c := Default()
	c.Display.Port = "COM7"
	data, _ := json.MarshalIndent(c, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Get().Display.Port; got != "COM7" {
		t.Errorf("deveria ter carregado a porta salva, veio %q", got)
	}
}

func TestStoreOpenCorruptedFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{ isso não é json"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Get().Display.Port; got != "AUTO" {
		t.Errorf("config corrompida deveria virar o padrão, veio %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json.invalido")); err != nil {
		t.Errorf("deveria ter guardado uma cópia do arquivo corrompido: %v", err)
	}
}

func TestStoreSetAndOnChange(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got Config
	calls := 0
	s.OnChange(func(c Config) { calls++; got = c })

	c := s.Get()
	c.Display.Brightness = 55
	saved, err := s.Set(c)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Display.Brightness != 55 {
		t.Errorf("Set() deveria devolver o valor salvo, veio %d", saved.Display.Brightness)
	}
	if calls != 1 {
		t.Errorf("OnChange deveria ter sido chamado 1 vez, veio %d", calls)
	}
	if got.Display.Brightness != 55 {
		t.Errorf("callback deveria receber a config nova")
	}
	if reread := s.Get().Display.Brightness; reread != 55 {
		t.Errorf("Get() deveria refletir o valor salvo, veio %d", reread)
	}

	// mexer na config devolvida pelo Get() não deve afetar o Store (clone).
	before := s.Get()
	before.Screens.Order[0] = "mutated"
	if after := s.Get(); after.Screens.Order[0] == Screen("mutated") {
		t.Error("Get() deveria devolver uma cópia independente (clone)")
	}
}

func TestStoreSetInvalidPath(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	// aponta o caminho para um diretório inexistente, forçando save() a falhar.
	s.path = filepath.Join(dir, "nao-existe", "config.json")
	if _, err := s.Set(Default()); err == nil {
		t.Error("Set() deveria falhar quando não consegue gravar")
	}
}
