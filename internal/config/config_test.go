package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	d := Default()
	if len(d.Devices) != 1 || d.Devices[0].Port != "AUTO" || d.Devices[0].Orientation != OrientPortrait {
		t.Errorf("dispositivo padrão inesperado: %+v", d.Devices)
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
	if len(c.Devices) != 1 {
		t.Fatalf("config sem dispositivos deveria ganhar 1 padrão, veio %d", len(c.Devices))
	}
	if c.Devices[0].Port != d.Devices[0].Port {
		t.Errorf("porta vazia deveria virar AUTO, veio %q", c.Devices[0].Port)
	}
	if c.Devices[0].ID == "" {
		t.Errorf("dispositivo deveria ganhar um ID")
	}
	if c.Devices[0].Revision != d.Devices[0].Revision {
		t.Errorf("revisão inválida deveria cair no padrão")
	}
	if c.Devices[0].Orientation != d.Devices[0].Orientation {
		t.Errorf("orientação inválida deveria cair no padrão")
	}
	if c.Devices[0].Mode.Type != ModeAuto {
		t.Errorf("modo inválido deveria virar automático, veio %q", c.Devices[0].Mode.Type)
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

func TestNormalizeDevices(t *testing.T) {
	c := Config{Devices: []DeviceConfig{
		{ID: "a", Port: " com3 ", Revision: "simulado", Orientation: OrientLandscape, Brightness: 500, Mode: ModeConfig{Type: "rotacao", RotateSeconds: -5, Fixed: "invalido"}},
		{ID: "a", Port: "AUTO"}, // ID duplicado, deve ganhar um novo
		{ID: "", Port: "AUTO"},  // ID vazio, deve ganhar um novo
	}}
	c.Normalize()
	if len(c.Devices) != 3 {
		t.Fatalf("deveria manter os 3 dispositivos, veio %d", len(c.Devices))
	}
	if c.Devices[0].Port != "COM3" {
		t.Errorf("porta deveria virar COM3 maiúsculo e sem espaços, veio %q", c.Devices[0].Port)
	}
	if c.Devices[0].Revision != "SIMULADO" {
		t.Errorf("revisão deveria virar SIMULADO, veio %q", c.Devices[0].Revision)
	}
	if c.Devices[0].Brightness != 100 {
		t.Errorf("brilho deveria ser limitado a 100, veio %d", c.Devices[0].Brightness)
	}
	if c.Devices[0].Mode.RotateSeconds != 3 {
		t.Errorf("rotação deveria ser limitada a 3s no mínimo, veio %d", c.Devices[0].Mode.RotateSeconds)
	}
	if c.Devices[0].Mode.Fixed != ScreenClock {
		t.Errorf("tela fixa inválida deveria cair no relógio, veio %q", c.Devices[0].Mode.Fixed)
	}
	if c.Devices[0].SimWidth != 320 || c.Devices[0].SimHeight != 480 {
		t.Errorf("resolução simulada vazia deveria virar 320x480, veio %dx%d", c.Devices[0].SimWidth, c.Devices[0].SimHeight)
	}
	ids := map[string]bool{}
	for _, d := range c.Devices {
		if ids[d.ID] {
			t.Fatalf("IDs duplicados após normalizar: %v", c.Devices)
		}
		ids[d.ID] = true
	}
}

func TestNormalizeCapsMaxDevices(t *testing.T) {
	var c Config
	for i := 0; i < maxDevices+5; i++ {
		c.Devices = append(c.Devices, DeviceConfig{})
	}
	c.Normalize()
	if len(c.Devices) != maxDevices {
		t.Errorf("deveria limitar a %d dispositivos, veio %d", maxDevices, len(c.Devices))
	}
}

func TestNormalizeClampsAndFixes(t *testing.T) {
	c := Config{
		Screens: ScreensConfig{Order: []Screen{"relogio", "xyz", "relogio", "jogo"}, System: SystemScreen{Disk: "  d  "}},
		Theme:   ThemeConfig{AccentMusic: "vermelho", AccentGame: "#ABCDEF", Background: "outro", BgColor: "#123"},
		Steam:   SteamConfig{Source: "outra-coisa", APIKey: "  chave  ", SteamID64: " 123 ", StatusSeconds: 1, LibrarySeconds: 1},
		General: GeneralConfig{RefreshMillis: 10, WebPort: 80, Language: "fr"},
	}
	c.Normalize()

	wantOrder := []Screen{ScreenClock, ScreenGame, ScreenMusic, ScreenSystem, ScreenCustom}
	if len(c.Screens.Order) != len(wantOrder) {
		t.Fatalf("ordem deveria ter %d telas (sem duplicatas/desconhecidas + completada), veio %v", len(wantOrder), c.Screens.Order)
	}
	if c.Screens.Order[0] != ScreenClock || c.Screens.Order[1] != ScreenGame {
		t.Errorf("ordem deveria manter relogio,jogo primeiro e completar o resto, veio %v", c.Screens.Order)
	}
	if c.Screens.System.Disk != "D:" {
		t.Errorf("disco deveria virar D: (maiúsculo, com dois pontos), veio %q", c.Screens.System.Disk)
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
	if c.Enabled(ScreenCustom) {
		t.Error("personalizada não deveria estar ligada sem nenhum widget")
	}
	c.Screens.Custom.Enabled = true
	c.Screens.Custom.Widgets = []CustomWidget{{Type: "relogio", X: 0, Y: 0, W: 0.5, H: 0.5}}
	if !c.Enabled(ScreenCustom) {
		t.Error("personalizada deveria estar ligada com ao menos um widget")
	}
}

func TestNormalizeCustomWidgets(t *testing.T) {
	c := Default()
	c.Screens.Custom.Widgets = []CustomWidget{
		{Type: "relogio", X: 0.5, Y: 0.5, W: 0.8, H: 0.8}, // W/H estourariam os limites da tela
		{Type: "widget-que-nao-existe", X: 0, Y: 0, W: 0.3, H: 0.3},
		{Type: "cpu_medidor", X: -1, Y: -1, W: 2, H: 2},
		{Type: "gpu_medidor", X: 0, Y: 0, W: 0, H: 0.3}, // largura zero: some pro mínimo (0.05)
	}
	c.Normalize()

	if len(c.Screens.Custom.Widgets) != 3 {
		t.Fatalf("deveria sobrar 3 widgets válidos (o tipo desconhecido descartado), veio %d: %+v", len(c.Screens.Custom.Widgets), c.Screens.Custom.Widgets)
	}
	first := c.Screens.Custom.Widgets[0]
	if first.X+first.W > 1.0001 || first.Y+first.H > 1.0001 {
		t.Errorf("widget não deveria passar dos limites da tela: %+v", first)
	}
	second := c.Screens.Custom.Widgets[1]
	if second.X != 0 || second.Y != 0 || second.W != 1 || second.H != 1 {
		t.Errorf("coordenadas fora de 0..1 deveriam ser fixadas nos limites, veio %+v", second)
	}
	third := c.Screens.Custom.Widgets[2]
	if third.W < 0.05 {
		t.Errorf("largura zero deveria virar o mínimo (0.05), veio %+v", third)
	}

	for i := 0; i < maxCustomWidgets+5; i++ {
		c.Screens.Custom.Widgets = append(c.Screens.Custom.Widgets, CustomWidget{Type: "relogio", X: 0, Y: 0, W: 0.1, H: 0.1})
	}
	c.Normalize()
	if len(c.Screens.Custom.Widgets) != maxCustomWidgets {
		t.Errorf("deveria limitar a %d widgets, veio %d", maxCustomWidgets, len(c.Screens.Custom.Widgets))
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
	if len(got.Devices) != 1 || got.Devices[0].Port != "AUTO" {
		t.Errorf("config recém-criada deveria ser o padrão")
	}
}

func TestStoreOpenExisting(t *testing.T) {
	dir := t.TempDir()
	c := Default()
	c.Devices[0].Port = "COM7"
	data, _ := json.MarshalIndent(c, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Get().Devices[0].Port; got != "COM7" {
		t.Errorf("deveria ter carregado a porta salva, veio %q", got)
	}
}

func TestStoreOpenMigratesLegacySingleDevice(t *testing.T) {
	dir := t.TempDir()
	// formato antigo: "tela"/"modo" no nível raiz, sem "dispositivos".
	legacy := `{"tela":{"porta":"COM9","revisao":"A","orientacao":"paisagem","brilho":42},"modo":{"tipo":"fixo","tela_fixa":"sistema"}}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := s.Get()
	if len(got.Devices) != 1 {
		t.Fatalf("deveria migrar para 1 dispositivo, veio %d", len(got.Devices))
	}
	dev := got.Devices[0]
	if dev.Port != "COM9" || dev.Orientation != OrientLandscape || dev.Brightness != 42 {
		t.Errorf("dispositivo migrado incorreto: %+v", dev)
	}
	if dev.Mode.Type != ModeFixed || dev.Mode.Fixed != ScreenSystem {
		t.Errorf("modo migrado incorreto: %+v", dev.Mode)
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
	if got := s.Get().Devices[0].Port; got != "AUTO" {
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
	c.Devices[0].Brightness = 55
	saved, err := s.Set(c)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Devices[0].Brightness != 55 {
		t.Errorf("Set() deveria devolver o valor salvo, veio %d", saved.Devices[0].Brightness)
	}
	if calls != 1 {
		t.Errorf("OnChange deveria ter sido chamado 1 vez, veio %d", calls)
	}
	if got.Devices[0].Brightness != 55 {
		t.Errorf("callback deveria receber a config nova")
	}
	if reread := s.Get().Devices[0].Brightness; reread != 55 {
		t.Errorf("Get() deveria refletir o valor salvo, veio %d", reread)
	}

	// mexer na config devolvida pelo Get() não deve afetar o Store (clone).
	before := s.Get()
	before.Screens.Order[0] = "mutated"
	before.Devices[0].Port = "MUTATED"
	if after := s.Get(); after.Screens.Order[0] == Screen("mutated") || after.Devices[0].Port == "MUTATED" {
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
