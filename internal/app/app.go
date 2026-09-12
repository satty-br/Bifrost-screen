// Package app junta tudo: escolhe a tela, desenha e envia para o display.
package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/satty-br/Bifrost-screen/internal/config"
	"github.com/satty-br/Bifrost-screen/internal/i18n"
	"github.com/satty-br/Bifrost-screen/internal/lcd"
	"github.com/satty-br/Bifrost-screen/internal/mancer"
	"github.com/satty-br/Bifrost-screen/internal/media"
	"github.com/satty-br/Bifrost-screen/internal/render"
	"github.com/satty-br/Bifrost-screen/internal/steam"
	"github.com/satty-br/Bifrost-screen/internal/sysinfo"
	"github.com/satty-br/Bifrost-screen/internal/update"
	"github.com/satty-br/Bifrost-screen/internal/winutil"
)

// Estados da conexão com a tela.
const (
	StatusConnecting = "conectando"
	StatusConnected  = "conectada"
	StatusBusy       = "porta_ocupada"
	StatusNotFound   = "nao_encontrada"
	StatusError      = "erro"
	StatusSimulated  = "simulada"
)

// MancerState é o resumo do mostrador Mancer Mystic G1 (watercooler).
type MancerState struct {
	Enabled   bool   `json:"ativado"`
	Connected bool   `json:"conectado"`
	Error     string `json:"erro"`
}

// State é o resumo mostrado no painel.
type State struct {
	Status     string             `json:"status"`
	StatusText string             `json:"status_texto"`
	Port       string             `json:"porta"`
	Screen     config.Screen      `json:"tela_atual"`
	Paused     bool               `json:"pausado"`
	Pinned     bool               `json:"fixada_manual"`
	Conflicts  []winutil.Conflict `json:"conflitos"`
	Media      media.Info         `json:"musica"`
	MediaError string             `json:"erro_musica"`
	Steam      steam.Status       `json:"steam"`
	SteamError string             `json:"erro_steam"`
	System     sysinfo.Stats      `json:"sistema"`
	FrameID    uint64             `json:"frame"`
	Active     []config.Screen    `json:"telas_ativas"`
	Version    string             `json:"versao"`
	Update     *update.Release    `json:"atualizacao,omitempty"`
	Mancer     MancerState        `json:"mancer"`
}

type App struct {
	Version string
	store   *config.Store
	media   *media.Reader
	steam   *steam.Client
	sys     *sysinfo.Sampler
	updater *update.Checker
	mancer  *mancer.Monitor

	mu           sync.Mutex
	display      lcd.Display
	status       string
	statusText   string
	conflicts    []winutil.Conflict
	frame        *image.RGBA
	sent         *image.RGBA
	frameID      uint64
	pngCache     []byte
	pngID        uint64
	screen       config.Screen
	paused       bool
	pin          config.Screen
	pinUntil     time.Time
	rotateIdx    int
	rotateAt     time.Time
	applied      config.DisplayConfig
	reconnect    chan struct{}
	forceRedraw  bool
	mancerOn     bool
	mancerCancel context.CancelFunc
	rootCtx      context.Context
}

func New(store *config.Store, cacheDir, version string) *App {
	a := &App{
		Version:   version,
		store:     store,
		media:     &media.Reader{},
		steam:     steam.New(cacheDir),
		sys:       &sysinfo.Sampler{},
		updater:   update.NewChecker(version),
		mancer:    mancer.NewMonitor(),
		status:    StatusConnecting,
		reconnect: make(chan struct{}, 1),
	}
	store.OnChange(a.onConfig)
	return a
}

// Media/Steam/System/Updater dão acesso às fontes de dados (usados nos testes e no painel).
func (a *App) Media() *media.Reader     { return a.media }
func (a *App) Steam() *steam.Client     { return a.steam }
func (a *App) System() *sysinfo.Sampler { return a.sys }
func (a *App) Updater() *update.Checker { return a.updater }

func steamSettings(c config.Config) steam.Settings {
	return steam.Settings{
		Enabled: c.Steam.Enabled, Source: c.Steam.Source, APIKey: c.Steam.APIKey, SteamID64: c.Steam.SteamID64,
		StatusEvery:  time.Duration(c.Steam.StatusSeconds) * time.Second,
		LibraryEvery: time.Duration(c.Steam.LibrarySeconds) * time.Second,
	}
}

// Run inicia tudo e bloqueia até ctx terminar.
func (a *App) Run(ctx context.Context) {
	a.mu.Lock()
	a.rootCtx = ctx
	a.mu.Unlock()

	cfg := a.store.Get()
	a.media.Start(time.Second)
	a.sys.SetDisk(cfg.Screens.System.Disk)
	a.sys.Start(time.Second)
	a.steam.Configure(steamSettings(cfg))
	go a.steam.Run(ctx)
	go a.connectionLoop(ctx)
	if cfg.General.AutoUpdate {
		a.updater.Start(ctx, 6*time.Hour)
	}
	a.syncMancer(cfg)

	ticker := time.NewTicker(time.Duration(cfg.General.RefreshMillis) * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			a.mu.Lock()
			if a.display != nil {
				a.display.Close()
			}
			a.mu.Unlock()
			return
		case <-ticker.C:
			a.tick()
			ticker.Reset(time.Duration(a.store.Get().General.RefreshMillis) * time.Millisecond)
		}
	}
}

// onConfig aplica mudanças de configuração que afetam a tela.
func (a *App) onConfig(c config.Config) {
	a.steam.Configure(steamSettings(c))
	a.sys.SetDisk(c.Screens.System.Disk)
	if err := winutil.SetAutostart(c.General.Autostart); err != nil {
		log.Printf("não consegui ajustar a inicialização com o Windows: %v", err)
	}
	a.syncMancer(c)
	a.mu.Lock()
	prev := a.applied
	disp := a.display
	status := a.status
	a.forceRedraw = true
	a.mu.Unlock()
	// re-traduz o texto de status atual (ex: "Conectada em X") se o idioma mudou.
	lang := c.ResolvedLanguage()
	switch status {
	case StatusConnected:
		if disp != nil {
			a.setStatus(StatusConnected, i18n.T(lang, "app.connected", disp.PortName()))
		}
	case StatusSimulated:
		a.setStatus(StatusSimulated, i18n.T(lang, "app.simulated"))
	}
	if prev.Port != c.Display.Port || prev.Revision != c.Display.Revision {
		a.Reconnect()
		return
	}
	if disp == nil {
		return
	}
	if prev.Brightness != c.Display.Brightness {
		_ = disp.SetBrightness(c.Display.Brightness)
	}
	if prev.Orientation != c.Display.Orientation {
		_ = disp.SetOrientation(lcd.ParseOrientation(c.Display.Orientation))
		a.mu.Lock()
		a.sent = nil
		a.mu.Unlock()
	}
	a.mu.Lock()
	a.applied = c.Display
	a.mu.Unlock()
}

// syncMancer liga/desliga o envio de temperatura pro mostrador Mancer Mystic
// G1 conforme a configuração (chamado no início do Run e a cada mudança de config).
func (a *App) syncMancer(c config.Config) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if c.Mancer.Enabled == a.mancerOn {
		return
	}
	a.mancerOn = c.Mancer.Enabled
	if a.mancerCancel != nil {
		a.mancerCancel()
		a.mancerCancel = nil
	}
	if !c.Mancer.Enabled || a.rootCtx == nil {
		return
	}
	ctx, cancel := context.WithCancel(a.rootCtx)
	a.mancerCancel = cancel
	a.mancer.Start(ctx, 500*time.Millisecond, func() float64 { return a.sys.Get().CPUTemp })
}

// Reconnect derruba a conexão atual e tenta de novo.
func (a *App) Reconnect() {
	select {
	case a.reconnect <- struct{}{}:
	default:
	}
}

func (a *App) setStatus(s, text string) {
	a.mu.Lock()
	changed := a.status != s || a.statusText != text
	a.status, a.statusText = s, text
	a.mu.Unlock()
	if changed {
		log.Printf("tela: %s — %s", s, text)
	}
}

func (a *App) connectionLoop(ctx context.Context) {
	for ctx.Err() == nil {
		cfg := a.store.Get()
		lang := cfg.ResolvedLanguage()
		var d lcd.Display
		if cfg.Display.Revision == "SIMULADO" {
			d = lcd.NewSimulated()
		} else {
			d = lcd.NewRevA(cfg.Display.Port, lcd.OpenSerial, lcd.DetectRevA)
		}
		a.setStatus(StatusConnecting, i18n.T(lang, "app.connecting"))
		err := d.Open()
		if err == nil {
			err = d.SetBrightness(cfg.Display.Brightness)
		}
		if err == nil {
			err = d.SetOrientation(lcd.ParseOrientation(cfg.Display.Orientation))
		}
		if err != nil {
			d.Close()
			a.handleConnectError(err, lang)
			if a.wait(ctx, 5*time.Second) {
				continue
			}
			return
		}
		a.mu.Lock()
		a.display = d
		a.sent = nil
		a.applied = cfg.Display
		a.conflicts = nil
		a.mu.Unlock()
		if cfg.Display.Revision == "SIMULADO" {
			a.setStatus(StatusSimulated, i18n.T(lang, "app.simulated"))
		} else {
			a.setStatus(StatusConnected, i18n.T(lang, "app.connected", d.PortName()))
		}

		// Fica conectado até pedirem reconexão ou dar erro de escrita.
		select {
		case <-ctx.Done():
		case <-a.reconnect:
		}
		a.mu.Lock()
		a.display = nil
		a.mu.Unlock()
		d.Close()
	}
}

func (a *App) handleConnectError(err error, lang i18n.Lang) {
	switch {
	case errors.Is(err, lcd.ErrPortBusy):
		conf := winutil.FindConflicts()
		a.mu.Lock()
		a.conflicts = conf
		a.mu.Unlock()
		a.setStatus(StatusBusy, i18n.T(lang, "app.port_busy"))
	case errors.Is(err, lcd.ErrNotFound):
		a.setStatus(StatusNotFound, i18n.T(lang, "app.not_found"))
	default:
		a.setStatus(StatusError, err.Error())
	}
}

// wait espera d ou um pedido de reconexão; devolve false se ctx acabou.
func (a *App) wait(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-a.reconnect:
		return true
	case <-time.After(d):
		return true
	}
}

// isActive diz se a tela tem algo para mostrar agora.
func isActive(s config.Screen, c config.Config, m media.Info, st steam.Status) bool {
	if !c.Enabled(s) {
		return false
	}
	switch s {
	case config.ScreenMusic:
		return m.HasSession && (m.Playing || c.Screens.Music.ShowWhenPaused)
	case config.ScreenGame:
		return st.Playing
	}
	return true
}

func (a *App) activeScreens(c config.Config, m media.Info, st steam.Status) []config.Screen {
	var out []config.Screen
	for _, s := range c.Screens.Order {
		if isActive(s, c, m, st) {
			out = append(out, s)
		}
	}
	return out
}

// choose decide a tela da vez. Precisa ser chamado com a.mu travado.
func (a *App) choose(now time.Time, c config.Config, active []config.Screen) config.Screen {
	if a.pin != "" && now.Before(a.pinUntil) {
		return a.pin
	}
	a.pin = ""
	switch c.Mode.Type {
	case config.ModeFixed:
		return c.Mode.Fixed
	case config.ModeRotate:
		if len(active) == 0 {
			return config.ScreenClock
		}
		if now.After(a.rotateAt) {
			a.rotateIdx++
			a.rotateAt = now.Add(time.Duration(c.Mode.RotateSeconds) * time.Second)
		}
		return active[a.rotateIdx%len(active)]
	}
	if len(active) == 0 {
		return config.ScreenClock
	}
	return active[0]
}

func (a *App) tick() {
	now := time.Now()
	cfg := a.store.Get()
	m := a.media.Get()
	st := a.steam.Get()
	sys := a.sys.Get()
	active := a.activeScreens(cfg, m, st)

	a.mu.Lock()
	if a.paused && a.frame != nil {
		a.mu.Unlock()
		return
	}
	screen := a.choose(now, cfg, active)
	a.screen = screen
	disp := a.display
	a.mu.Unlock()

	w, h := 320, 480
	if lcd.ParseOrientation(cfg.Display.Orientation).IsLandscape() {
		w, h = 480, 320
	}
	if disp != nil {
		w, h = disp.Size()
	}

	in := render.Input{Now: now, Cfg: cfg, Media: m, Steam: st, System: sys,
		Lang: cfg.ResolvedLanguage(), SteamReady: cfg.Steam.Enabled && a.steam.Ready()}
	img := render.Draw(screen, w, h, in)

	a.mu.Lock()
	a.frame = img
	a.frameID++
	prev := a.sent
	if a.forceRedraw {
		prev = nil
		a.forceRedraw = false
	}
	a.mu.Unlock()

	if disp == nil {
		return
	}
	r := lcd.DirtyRect(prev, img)
	if r.Empty() {
		return
	}
	if err := disp.Draw(img, r); err != nil {
		log.Printf("erro enviando para a tela: %v", err)
		a.setStatus(StatusError, i18n.T(cfg.ResolvedLanguage(), "app.lost_connection"))
		a.Reconnect()
		return
	}
	a.mu.Lock()
	a.sent = img
	a.mu.Unlock()
}

// --- controles usados pelo painel e pela bandeja

func (a *App) Next(delta int) {
	cfg := a.store.Get()
	active := a.activeScreens(cfg, a.media.Get(), a.steam.Get())
	if len(active) == 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	cur := 0
	for i, s := range active {
		if s == a.screen {
			cur = i
		}
	}
	next := active[((cur+delta)%len(active)+len(active))%len(active)]
	a.pin = next
	a.pinUntil = time.Now().Add(time.Minute)
	a.rotateIdx = cur + delta
	a.rotateAt = time.Now().Add(time.Duration(cfg.Mode.RotateSeconds) * time.Second)
	a.paused = false
}

func (a *App) SetPaused(p bool) {
	a.mu.Lock()
	a.paused = p
	a.mu.Unlock()
}

func (a *App) Paused() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.paused
}

// KillConflicts encerra os programas que prendem a porta e reconecta.
func (a *App) KillConflicts(pids []uint32) error {
	if len(pids) == 0 {
		a.mu.Lock()
		for _, c := range a.conflicts {
			pids = append(pids, c.PID)
		}
		a.mu.Unlock()
	}
	if err := winutil.KillElevated(pids); err != nil {
		return err
	}
	go func() {
		time.Sleep(2 * time.Second)
		a.Reconnect()
	}()
	return nil
}

func (a *App) State() State {
	cfg := a.store.Get()
	m, st := a.media.Get(), a.steam.Get()
	a.mu.Lock()
	defer a.mu.Unlock()
	port := ""
	if a.display != nil {
		port = a.display.PortName()
	}
	return State{
		Status: a.status, StatusText: a.statusText, Port: port, Screen: a.screen, Paused: a.paused,
		Pinned: a.pin != "" && time.Now().Before(a.pinUntil), Conflicts: a.conflicts,
		Media: m, MediaError: a.media.LastError(), Steam: st, SteamError: a.steam.LastError(),
		System: a.sys.Get(), FrameID: a.frameID, Active: a.activeScreens(cfg, m, st), Version: a.Version,
		Update: a.updater.Available(),
		Mancer: MancerState{Enabled: cfg.Mancer.Enabled, Connected: a.mancer.Connected(), Error: a.mancer.LastError()},
	}
}

// InstallUpdate baixa a versão mais nova (se houver), confere o checksum
// quando o release publica um checksums.txt, e substitui o executável atual.
// Quem chamou deve encerrar o processo logo em seguida: Apply já deixa o
// novo binário pronto e reaberto (ou preparado pra reabrir, no Windows).
func (a *App) InstallUpdate(ctx context.Context) error {
	rel := a.updater.Available()
	if rel == nil {
		return errors.New("nenhuma atualização disponível")
	}
	asset, checksums := rel.FindAsset()
	if asset == nil {
		return fmt.Errorf("a versão %s não tem um binário para esta plataforma (%s)", rel.Version, update.AssetName())
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	path, err := update.Download(ctx, asset.URL, filepath.Dir(exe))
	if err != nil {
		return fmt.Errorf("baixando a atualização: %w", err)
	}
	if checksums != nil {
		text, err := update.DownloadText(ctx, checksums.URL)
		if err != nil {
			os.Remove(path)
			return fmt.Errorf("baixando o checksums.txt: %w", err)
		}
		if err := update.VerifyChecksum(path, text, asset.Name); err != nil {
			os.Remove(path)
			return fmt.Errorf("verificação de integridade falhou: %w", err)
		}
	}
	if err := update.Apply(path); err != nil {
		os.Remove(path)
		return fmt.Errorf("instalando a atualização: %w", err)
	}
	return nil
}

// PreviewPNG devolve o frame atual em PNG (com cache por frame).
func (a *App) PreviewPNG() ([]byte, uint64) {
	a.mu.Lock()
	img, id := a.frame, a.frameID
	if a.pngID == id && a.pngCache != nil {
		b := a.pngCache
		a.mu.Unlock()
		return b, id
	}
	a.mu.Unlock()
	if img == nil {
		cfg := a.store.Get()
		img = render.Message(320, 480, i18n.T(render.CanvasLang(cfg.ResolvedLanguage()), "app.starting"), "", cfg.Theme)
	}
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	_ = enc.Encode(&buf, img)
	a.mu.Lock()
	a.pngCache, a.pngID = buf.Bytes(), id
	a.mu.Unlock()
	return buf.Bytes(), id
}

func (a *App) TestSteam(ctx context.Context, key, id string) (string, error) {
	return a.steam.Test(ctx, key, id)
}

func (a *App) TestSteamLocal() (string, string, error) { return a.steam.TestLocal() }
