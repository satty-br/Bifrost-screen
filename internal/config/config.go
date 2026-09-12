// Package config lê e grava a configuração do Bifrost (config.json).
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/satty-br/Bifrost-screen/internal/i18n"
)

// Screen identifica uma das telas que o Bifrost sabe desenhar.
type Screen string

const (
	ScreenMusic  Screen = "musica"
	ScreenGame   Screen = "jogo"
	ScreenSystem Screen = "sistema"
	ScreenClock  Screen = "relogio"
)

// AllScreens lista as telas na ordem padrão de prioridade.
var AllScreens = []Screen{ScreenGame, ScreenMusic, ScreenSystem, ScreenClock}

// Modos de exibição.
const (
	ModeAuto   = "automatico" // primeira tela ativa na ordem de prioridade
	ModeRotate = "rotacao"    // alterna entre as telas ativas
	ModeFixed  = "fixo"       // sempre a mesma tela
)

// Orientações suportadas pela tela.
const (
	OrientPortrait         = "retrato"
	OrientPortraitReverse  = "retrato_invertido"
	OrientLandscape        = "paisagem"
	OrientLandscapeReverse = "paisagem_invertida"
)

type Config struct {
	Devices []DeviceConfig `json:"dispositivos"`
	Screens ScreensConfig `json:"telas"`
	Theme   ThemeConfig   `json:"tema"`
	Steam   SteamConfig   `json:"steam"`
	General GeneralConfig `json:"geral"`
	Mancer  MancerConfig  `json:"mancer"`
}

// DeviceConfig é uma tela USB configurada. O ID é interno e estável (não muda
// ao reconectar); a Porta pode ser "AUTO" (pega qualquer tela livre detectada)
// ou uma porta específica ("COM3", "/dev/ttyUSB0"...). Como os clones
// Turing/UsbMonitor baratos costumam repetir o mesmo VID/PID/número de série
// de fábrica, não dá pra identificar cada unidade física com certeza — só a
// porta é garantidamente estável (no mesmo PC/mesma entrada USB).
type DeviceConfig struct {
	ID          string     `json:"id"`
	Name        string     `json:"nome"`       // rótulo escolhido pelo usuário ("Tela esquerda"...)
	Port        string     `json:"porta"`      // "AUTO" ou "COM3"
	Revision    string     `json:"revisao"`    // "A" ou "SIMULADO"
	Orientation string     `json:"orientacao"` // ver constantes Orient*
	Brightness  int        `json:"brilho"`     // 0-100
	Mode        ModeConfig `json:"modo"`
}

type ScreensConfig struct {
	Order  []Screen     `json:"ordem"`
	Music  MusicScreen  `json:"musica"`
	Game   GameScreen   `json:"jogo"`
	System SystemScreen `json:"sistema"`
	Clock  ClockScreen  `json:"relogio"`
}

type MusicScreen struct {
	Enabled        bool `json:"ativa"`
	ShowCover      bool `json:"mostrar_capa"`
	ShowAlbum      bool `json:"mostrar_album"`
	ShowProgress   bool `json:"mostrar_progresso"`
	ShowWhenPaused bool `json:"mostrar_pausada"`
}

type GameScreen struct {
	Enabled      bool `json:"ativa"`
	ShowCover    bool `json:"mostrar_capa"`
	ShowTotal    bool `json:"mostrar_total"`
	ShowTwoWeeks bool `json:"mostrar_duas_semanas"`
	ShowSession  bool `json:"mostrar_sessao"`
}

type SystemScreen struct {
	Enabled    bool   `json:"ativa"`
	ShowCPU    bool   `json:"mostrar_cpu"`
	ShowGPU    bool   `json:"mostrar_gpu"`
	ShowRAM    bool   `json:"mostrar_ram"`
	ShowNet    bool   `json:"mostrar_rede"`
	ShowDisk   bool   `json:"mostrar_disco"`
	ShowUptime bool   `json:"mostrar_tempo_ligado"`
	Disk       string `json:"disco"` // ex: "C:"
}

type ClockScreen struct {
	Enabled     bool `json:"ativa"`
	Use24h      bool `json:"formato_24h"`
	ShowSeconds bool `json:"mostrar_segundos"`
	ShowDate    bool `json:"mostrar_data"`
}

type ModeConfig struct {
	Type          string `json:"tipo"`
	RotateSeconds int    `json:"rotacao_segundos"`
	Fixed         Screen `json:"tela_fixa"`
}

type ThemeConfig struct {
	AccentMusic  string `json:"cor_musica"`
	AccentGame   string `json:"cor_jogo"`
	AccentSystem string `json:"cor_sistema"`
	AccentClock  string `json:"cor_relogio"`
	Background   string `json:"fundo"` // "gradiente" ou "solido"
	BgColor      string `json:"cor_fundo"`
}

type SteamConfig struct {
	Enabled        bool   `json:"ativa"`
	Source         string `json:"fonte"` // "local" (Steam instalada no PC) ou "web" (Web API)
	APIKey         string `json:"api_key"`
	SteamID64      string `json:"steam_id64"`
	StatusSeconds  int    `json:"intervalo_status_segundos"`
	LibrarySeconds int    `json:"intervalo_biblioteca_segundos"`
}

type GeneralConfig struct {
	Autostart     bool   `json:"iniciar_com_windows"`
	OpenPanel     bool   `json:"abrir_painel_ao_iniciar"`
	RefreshMillis int    `json:"atualizacao_ms"`
	WebPort       int    `json:"porta_painel"`
	Language      string `json:"idioma"` // "auto" ou um código ("en", "pt", "es", "ja", "zh")
	AutoUpdate    bool   `json:"atualizar_automaticamente"`
}

// MancerConfig liga/desliga o envio da temperatura da CPU para o mostrador
// embutido no bloco d'água Mancer Mystic G1 (detectado sozinho pelo VID/PID
// do HID, sem precisar escolher porta). Ligado por padrão: se o dispositivo
// não estiver conectado, a detecção simplesmente não encontra nada.
type MancerConfig struct {
	Enabled bool `json:"ativado"`
}

// Default devolve a configuração de fábrica.
func Default() Config {
	return Config{
		Devices: []DeviceConfig{defaultDevice("principal")},
		Screens: ScreensConfig{
			Order:  append([]Screen(nil), AllScreens...),
			Music:  MusicScreen{Enabled: true, ShowCover: true, ShowAlbum: true, ShowProgress: true, ShowWhenPaused: true},
			Game:   GameScreen{Enabled: true, ShowCover: true, ShowTotal: true, ShowTwoWeeks: true, ShowSession: true},
			System: SystemScreen{Enabled: true, ShowCPU: true, ShowGPU: true, ShowRAM: true, ShowNet: true, ShowDisk: true, ShowUptime: true, Disk: "C:"},
			Clock:  ClockScreen{Enabled: true, Use24h: true, ShowSeconds: false, ShowDate: true},
		},
		Theme: ThemeConfig{
			AccentMusic: "#2dd4bf", AccentGame: "#66c0f4", AccentSystem: "#f59e0b", AccentClock: "#a5b4fc",
			Background: "gradiente", BgColor: "#101116",
		},
		Steam:   SteamConfig{Enabled: true, Source: "local", StatusSeconds: 15, LibrarySeconds: 300},
		General: GeneralConfig{Autostart: false, OpenPanel: true, RefreshMillis: 1000, WebPort: 47017, Language: "auto", AutoUpdate: true},
		Mancer:  MancerConfig{Enabled: true},
	}
}

// defaultDevice devolve um dispositivo com as configurações de fábrica, com o ID dado.
func defaultDevice(id string) DeviceConfig {
	return DeviceConfig{
		ID: id, Port: "AUTO", Revision: "A", Orientation: OrientPortrait, Brightness: 20,
		Mode: ModeConfig{Type: ModeAuto, RotateSeconds: 10, Fixed: ScreenClock},
	}
}

// maxDevices evita que a API deixe configurar uma quantidade absurda de telas.
const maxDevices = 8

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// Normalize corrige valores fora do intervalo e preenche o que estiver faltando.
func (c *Config) Normalize() {
	d := Default()

	if len(c.Devices) > maxDevices {
		c.Devices = c.Devices[:maxDevices]
	}
	seenID := map[string]bool{}
	nextAuto := 1
	for i := range c.Devices {
		dev := &c.Devices[i]
		dev.ID = strings.TrimSpace(dev.ID)
		for dev.ID == "" || seenID[dev.ID] {
			dev.ID = fmt.Sprintf("dispositivo-%d", nextAuto)
			nextAuto++
		}
		seenID[dev.ID] = true
		dev.Name = strings.TrimSpace(dev.Name)

		dev.Port = strings.ToUpper(strings.TrimSpace(dev.Port))
		if dev.Port == "" {
			dev.Port = "AUTO"
		}
		switch strings.ToUpper(dev.Revision) {
		case "A", "SIMULADO":
			dev.Revision = strings.ToUpper(dev.Revision)
		default:
			dev.Revision = d.Devices[0].Revision
		}
		switch dev.Orientation {
		case OrientPortrait, OrientPortraitReverse, OrientLandscape, OrientLandscapeReverse:
		default:
			dev.Orientation = d.Devices[0].Orientation
		}
		dev.Brightness = clamp(dev.Brightness, 0, 100)

		switch dev.Mode.Type {
		case ModeAuto, ModeRotate, ModeFixed:
		default:
			dev.Mode.Type = ModeAuto
		}
		dev.Mode.RotateSeconds = clamp(dev.Mode.RotateSeconds, 3, 600)
		if !isKnown(dev.Mode.Fixed) {
			dev.Mode.Fixed = ScreenClock
		}
	}
	if len(c.Devices) == 0 {
		c.Devices = []DeviceConfig{defaultDevice("principal")}
	}

	// Ordem: mantém as conhecidas, sem repetição, e completa com as que faltarem.
	seen := map[Screen]bool{}
	var order []Screen
	for _, s := range c.Screens.Order {
		if isKnown(s) && !seen[s] {
			order = append(order, s)
			seen[s] = true
		}
	}
	for _, s := range AllScreens {
		if !seen[s] {
			order = append(order, s)
		}
	}
	c.Screens.Order = order
	if strings.TrimSpace(c.Screens.System.Disk) == "" {
		c.Screens.System.Disk = "C:"
	}
	c.Screens.System.Disk = strings.ToUpper(strings.TrimRight(strings.TrimSpace(c.Screens.System.Disk), `\/`))
	if !strings.HasSuffix(c.Screens.System.Disk, ":") {
		c.Screens.System.Disk += ":"
	}

	fixColor(&c.Theme.AccentMusic, d.Theme.AccentMusic)
	fixColor(&c.Theme.AccentGame, d.Theme.AccentGame)
	fixColor(&c.Theme.AccentSystem, d.Theme.AccentSystem)
	fixColor(&c.Theme.AccentClock, d.Theme.AccentClock)
	fixColor(&c.Theme.BgColor, d.Theme.BgColor)
	if c.Theme.Background != "solido" {
		c.Theme.Background = "gradiente"
	}

	if c.Steam.Source != "web" {
		c.Steam.Source = "local"
	}
	c.Steam.APIKey = strings.TrimSpace(c.Steam.APIKey)
	c.Steam.SteamID64 = strings.TrimSpace(c.Steam.SteamID64)
	c.Steam.StatusSeconds = clamp(c.Steam.StatusSeconds, 10, 600)
	c.Steam.LibrarySeconds = clamp(c.Steam.LibrarySeconds, 60, 3600)

	c.General.RefreshMillis = clamp(c.General.RefreshMillis, 250, 5000)
	if c.General.WebPort < 1024 || c.General.WebPort > 65535 {
		c.General.WebPort = d.General.WebPort
	}
	if c.General.Language != "auto" && !i18n.IsSupported(i18n.Lang(c.General.Language)) {
		c.General.Language = "auto"
	}
}

// ResolvedLanguage devolve o idioma efetivo: o escolhido, ou o do Windows se for "auto".
func (c *Config) ResolvedLanguage() i18n.Lang {
	return i18n.Resolve(c.General.Language)
}

// Enabled diz se a tela está ligada nas opções.
func (c *Config) Enabled(s Screen) bool {
	switch s {
	case ScreenMusic:
		return c.Screens.Music.Enabled
	case ScreenGame:
		return c.Screens.Game.Enabled && c.Steam.Enabled
	case ScreenSystem:
		return c.Screens.System.Enabled
	case ScreenClock:
		return c.Screens.Clock.Enabled
	}
	return false
}

func isKnown(s Screen) bool {
	for _, k := range AllScreens {
		if k == s {
			return true
		}
	}
	return false
}

func fixColor(v *string, def string) {
	if !hexColor.MatchString(*v) {
		*v = def
	}
	*v = strings.ToLower(*v)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Store guarda a configuração em memória e no disco, com acesso concorrente seguro.
type Store struct {
	mu   sync.RWMutex
	path string
	cfg  Config
	subs []func(Config)
}

// Dir devolve a pasta de dados do Bifrost: a pasta "dados" ao lado do .exe, se
// existir (modo portátil), senão %APPDATA%\Bifrost no Windows.
func Dir() string {
	if exe, err := os.Executable(); err == nil {
		portable := filepath.Join(filepath.Dir(exe), "dados")
		if fi, err := os.Stat(portable); err == nil && fi.IsDir() {
			return portable
		}
	}
	if runtime.GOOS == "windows" {
		if app := os.Getenv("APPDATA"); app != "" {
			return filepath.Join(app, "Bifrost")
		}
	}
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "bifrost")
	}
	return "."
}

// Open carrega o config.json de dir (criando com os valores padrão se não existir).
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{path: filepath.Join(dir, "config.json"), cfg: Default()}
	data, err := os.ReadFile(s.path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		s.cfg.Normalize()
		return s, s.save(s.cfg)
	case err != nil:
		return nil, err
	}
	cfg := Default()
	if err := json.Unmarshal(data, &cfg); err != nil {
		// Arquivo corrompido: guarda uma cópia e recomeça do padrão.
		_ = os.WriteFile(s.path+".invalido", data, 0o644)
		cfg = Default()
	}
	migrateSingleDevice(data, &cfg)
	cfg.Normalize()
	s.cfg = cfg
	return s, nil
}

// migrateSingleDevice converte um config.json de antes do suporte a múltiplas
// telas (chaves "tela"/"modo" no nível raiz) para o novo formato com "dispositivos".
func migrateSingleDevice(data []byte, cfg *Config) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return
	}
	if _, hasNewFormat := raw["dispositivos"]; hasNewFormat {
		return
	}
	var legacy struct {
		Display *DeviceConfig `json:"tela"`
		Mode    *ModeConfig   `json:"modo"`
	}
	if err := json.Unmarshal(data, &legacy); err != nil || legacy.Display == nil {
		return
	}
	dev := *legacy.Display
	dev.ID = "principal"
	if legacy.Mode != nil {
		dev.Mode = *legacy.Mode
	}
	cfg.Devices = []DeviceConfig{dev}
}

func (s *Store) Path() string { return s.path }

func (s *Store) Get() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return clone(s.cfg)
}

// Set valida, grava e avisa quem estiver inscrito.
func (s *Store) Set(c Config) (Config, error) {
	c.Normalize()
	s.mu.Lock()
	if err := s.save(c); err != nil {
		s.mu.Unlock()
		return s.cfg, err
	}
	s.cfg = clone(c)
	subs := append([]func(Config){}, s.subs...)
	s.mu.Unlock()
	for _, fn := range subs {
		fn(clone(c))
	}
	return c, nil
}

// OnChange registra uma função chamada sempre que a configuração muda.
func (s *Store) OnChange(fn func(Config)) {
	s.mu.Lock()
	s.subs = append(s.subs, fn)
	s.mu.Unlock()
}

func (s *Store) save(c Config) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("gravando configuração: %w", err)
	}
	return os.Rename(tmp, s.path)
}

func clone(c Config) Config {
	c.Screens.Order = append([]Screen(nil), c.Screens.Order...)
	c.Devices = append([]DeviceConfig(nil), c.Devices...)
	return c
}
