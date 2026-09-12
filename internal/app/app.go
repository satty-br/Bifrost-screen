// Package app junta tudo: escolhe a tela, desenha e envia para cada display.
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
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/satty-br/Bifrost-screen/internal/config"
	"github.com/satty-br/Bifrost-screen/internal/gsi"
	"github.com/satty-br/Bifrost-screen/internal/i18n"
	"github.com/satty-br/Bifrost-screen/internal/lcd"
	"github.com/satty-br/Bifrost-screen/internal/lolapi"
	"github.com/satty-br/Bifrost-screen/internal/mancer"
	"github.com/satty-br/Bifrost-screen/internal/media"
	"github.com/satty-br/Bifrost-screen/internal/render"
	"github.com/satty-br/Bifrost-screen/internal/rtss"
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

// GameDataState é o resumo do acompanhamento ao vivo de partidas (painel).
type GameDataState struct {
	Enabled            bool    `json:"ativado"`
	Active             bool    `json:"partida_ativa"`
	Game               string  `json:"jogo"`
	FPS                float64 `json:"fps"`
	AguardandoReinicio bool    `json:"aguardando_reinicio"`
}

// DeviceState é o resumo de UM dispositivo (uma tela USB), mostrado no painel.
type DeviceState struct {
	ID         string             `json:"id"`
	Name       string             `json:"nome"`
	Status     string             `json:"status"`
	StatusText string             `json:"status_texto"`
	Port       string             `json:"porta"`
	Screen     config.Screen      `json:"tela_atual"`
	Paused     bool               `json:"pausado"`
	Pinned     bool               `json:"fixada_manual"`
	Conflicts  []winutil.Conflict `json:"conflitos"`
	FrameID    uint64             `json:"frame"`
	Active     []config.Screen    `json:"telas_ativas"`
}

// State é o resumo mostrado no painel.
type State struct {
	Devices    []DeviceState   `json:"dispositivos"`
	Media      media.Info      `json:"musica"`
	MediaError string          `json:"erro_musica"`
	Steam      steam.Status    `json:"steam"`
	SteamError string          `json:"erro_steam"`
	System     sysinfo.Stats   `json:"sistema"`
	Version    string          `json:"versao"`
	Update     *update.Release `json:"atualizacao,omitempty"`
	Mancer     MancerState     `json:"mancer"`
	GameData   GameDataState   `json:"dados_de_jogo"`
}

// device é o estado ao vivo de um dispositivo configurado (uma tela USB).
type device struct {
	id string

	mu          sync.Mutex
	display     lcd.Display
	status      string
	statusText  string
	conflicts   []winutil.Conflict
	frame       *image.RGBA
	sent        *image.RGBA
	frameID     uint64
	pngCache    []byte
	pngID       uint64
	screen      config.Screen
	paused      bool
	pin         config.Screen
	pinUntil    time.Time
	rotateIdx   int
	rotateAt    time.Time
	applied     config.DeviceConfig
	reconnect   chan struct{}
	forceRedraw bool
	cancel      context.CancelFunc
}

func newDevice(id string) *device {
	return &device{id: id, status: StatusConnecting, reconnect: make(chan struct{}, 1)}
}

// portClaims coordena qual dispositivo em modo "AUTO" fica com qual porta,
// pra dois dispositivos não brigarem pela mesma tela detectada. Como os
// clones baratos costumam repetir o mesmo VID/PID/número de série de
// fábrica, não dá pra ter certeza de qual unidade física é qual — só que
// cada porta só é usada por um dispositivo de cada vez.
type portClaims struct {
	mu      sync.Mutex
	claimed map[string]string // nome da porta -> ID do dispositivo que está usando
}

func newPortClaims() *portClaims { return &portClaims{claimed: map[string]string{}} }

func (p *portClaims) claim(deviceID string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	ports, err := lcd.ListPorts()
	if err != nil {
		return "", err
	}
	sort.Slice(ports, func(i, j int) bool { return ports[i].Name < ports[j].Name })
	for _, port := range ports {
		if !port.IsScreen {
			continue
		}
		if holder, ok := p.claimed[port.Name]; ok && holder != deviceID {
			continue
		}
		p.claimed[port.Name] = deviceID
		return port.Name, nil
	}
	return "", lcd.ErrNotFound
}

func (p *portClaims) release(deviceID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for port, holder := range p.claimed {
		if holder == deviceID {
			delete(p.claimed, port)
		}
	}
}

type App struct {
	Version string
	store   *config.Store
	media   *media.Reader
	steam   *steam.Client
	sys     *sysinfo.Sampler
	updater *update.Checker
	claims  *portClaims
	mancer  *mancer.Monitor
	gsi     *gsi.Server
	lol     *lolapi.Poller

	devMu          sync.RWMutex
	rootCtx        context.Context
	devices        map[string]*device
	order          []string // IDs na ordem da configuração, pra State() sair estável
	mancerOn       bool
	mancerCancel   context.CancelFunc
	gameDataOn     bool
	gameDataCancel context.CancelFunc
}

func New(store *config.Store, cacheDir, version string) *App {
	cfg := store.Get()
	a := &App{
		Version: version,
		store:   store,
		media:   &media.Reader{},
		steam:   steam.New(cacheDir),
		sys:     &sysinfo.Sampler{},
		updater: update.NewChecker(version),
		claims:  newPortClaims(),
		mancer:  mancer.NewMonitor(),
		gsi:     gsi.NovoServer(cfg.GameData.Token),
		lol:     lolapi.NovoPoller(),
		devices: map[string]*device{},
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
	a.devMu.Lock()
	a.rootCtx = ctx
	a.devMu.Unlock()

	cfg := a.store.Get()
	a.media.Start(time.Second)
	a.sys.SetDisk(cfg.Screens.System.Disk)
	a.sys.Start(time.Second)
	a.steam.Configure(steamSettings(cfg))
	go a.steam.Run(ctx)
	a.syncDevices(ctx, cfg.Devices)
	if cfg.General.AutoUpdate {
		a.updater.Start(ctx, 6*time.Hour)
	}
	a.syncMancer(cfg)
	a.syncGameData(cfg)

	ticker := time.NewTicker(time.Duration(cfg.General.RefreshMillis) * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			a.devMu.RLock()
			for _, d := range a.devices {
				d.mu.Lock()
				if d.display != nil {
					d.display.Close()
				}
				d.mu.Unlock()
			}
			a.devMu.RUnlock()
			return
		case <-ticker.C:
			a.tick()
			ticker.Reset(time.Duration(a.store.Get().General.RefreshMillis) * time.Millisecond)
		}
	}
}

// syncDevices cria/encerra as goroutines de conexão pra acompanhar a lista
// configurada (chamado no início do Run e sempre que a lista de dispositivos muda).
func (a *App) syncDevices(ctx context.Context, cfgs []config.DeviceConfig) {
	a.devMu.Lock()
	defer a.devMu.Unlock()

	want := map[string]bool{}
	order := make([]string, 0, len(cfgs))
	for _, dc := range cfgs {
		want[dc.ID] = true
		order = append(order, dc.ID)
	}
	a.order = order

	for id, d := range a.devices {
		if !want[id] {
			d.cancel()
			a.claims.release(id)
			delete(a.devices, id)
		}
	}
	for _, dc := range cfgs {
		if _, ok := a.devices[dc.ID]; ok {
			continue
		}
		d := newDevice(dc.ID)
		devCtx, cancel := context.WithCancel(ctx)
		d.cancel = cancel
		a.devices[dc.ID] = d
		go a.connectionLoop(devCtx, d)
	}
}

// deviceByID devolve o estado ao vivo de um dispositivo (ou nil se não existir).
func (a *App) deviceByID(id string) *device {
	a.devMu.RLock()
	defer a.devMu.RUnlock()
	return a.devices[id]
}

func deviceConfig(cfgs []config.DeviceConfig, id string) (config.DeviceConfig, bool) {
	for _, dc := range cfgs {
		if dc.ID == id {
			return dc, true
		}
	}
	return config.DeviceConfig{}, false
}

// onConfig aplica mudanças de configuração que afetam as telas.
func (a *App) onConfig(c config.Config) {
	a.steam.Configure(steamSettings(c))
	a.sys.SetDisk(c.Screens.System.Disk)
	if err := winutil.SetAutostart(c.General.Autostart); err != nil {
		log.Printf("não consegui ajustar a inicialização com o Windows: %v", err)
	}
	a.syncMancer(c)
	a.syncGameData(c)

	a.devMu.RLock()
	root := a.rootCtx
	a.devMu.RUnlock()
	if root != nil {
		a.syncDevices(root, c.Devices)
	}

	lang := c.ResolvedLanguage()
	for _, dc := range c.Devices {
		d := a.deviceByID(dc.ID)
		if d == nil {
			continue
		}
		d.mu.Lock()
		prev := d.applied
		disp := d.display
		status := d.status
		d.forceRedraw = true
		d.mu.Unlock()
		// re-traduz o texto de status atual (ex: "Conectada em X") se o idioma mudou.
		switch status {
		case StatusConnected:
			if disp != nil {
				a.setStatus(d, StatusConnected, i18n.T(lang, "app.connected", disp.PortName()))
			}
		case StatusSimulated:
			a.setStatus(d, StatusSimulated, i18n.T(lang, "app.simulated"))
		}

		if prev.Port != dc.Port || prev.Revision != dc.Revision {
			a.Reconnect(dc.ID)
			continue
		}
		if disp == nil {
			d.mu.Lock()
			d.applied = dc
			d.mu.Unlock()
			continue
		}
		if prev.Brightness != dc.Brightness {
			_ = disp.SetBrightness(dc.Brightness)
		}
		if prev.Orientation != dc.Orientation {
			_ = disp.SetOrientation(lcd.ParseOrientation(dc.Orientation))
			d.mu.Lock()
			d.sent = nil
			d.mu.Unlock()
		}
		d.mu.Lock()
		d.applied = dc
		d.mu.Unlock()
	}
}

// syncMancer liga/desliga o envio de temperatura pro mostrador Mancer Mystic
// G1 conforme a configuração (chamado no início do Run e a cada mudança de config).
func (a *App) syncMancer(c config.Config) {
	a.devMu.Lock()
	defer a.devMu.Unlock()
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

// syncGameData liga/desliga o acompanhamento ao vivo de partidas (CS2/Dota2
// via GSI + League of Legends via a API local da Riot), conforme a
// configuração (chamado no início do Run e a cada mudança de config).
func (a *App) syncGameData(c config.Config) {
	a.devMu.Lock()
	defer a.devMu.Unlock()
	if c.GameData.Enabled == a.gameDataOn {
		return
	}
	a.gameDataOn = c.GameData.Enabled
	if a.gameDataCancel != nil {
		a.gameDataCancel()
		a.gameDataCancel = nil
		a.gsi.Stop()
		if err := gsi.RemoveCS2Config(); err != nil {
			log.Printf("não consegui remover o .cfg de GSI do CS2: %v", err)
		}
		if err := gsi.RemoveDota2Config(); err != nil {
			log.Printf("não consegui remover o .cfg de GSI do Dota 2: %v", err)
		}
	}
	if !c.GameData.Enabled || a.rootCtx == nil {
		return
	}
	if err := a.gsi.Start(gsi.DefaultPort); err != nil {
		log.Printf("não consegui ligar o servidor de GSI (CS2/Dota2): %v", err)
	} else {
		if wrote, err := gsi.EnsureCS2Config(a.gsi.Porta(), a.gsi.Token()); err != nil {
			log.Printf("CS2 não está instalado, ou não consegui configurar o GSI: %v", err)
		} else if wrote {
			log.Printf("GSI do CS2 configurado — reinicie o jogo se ele já estava aberto")
		}
		if wrote, err := gsi.EnsureDota2Config(a.gsi.Porta(), a.gsi.Token()); err != nil {
			log.Printf("Dota 2 não está instalado, ou não consegui configurar o GSI: %v", err)
		} else if wrote {
			log.Printf("GSI do Dota 2 configurado — reinicie o jogo se ele já estava aberto")
		}
	}
	ctx, cancel := context.WithCancel(a.rootCtx)
	a.gameDataCancel = cancel
	a.lol.Start(ctx, time.Second)
}

// currentLiveMatch junta a partida de GSI (CS2/Dota2) com a do LoL — CS2/Dota2
// tem prioridade se as duas por acaso estiverem ativas (não deveria acontecer
// na prática, já que são jogos diferentes rodando ao mesmo tempo).
func (a *App) currentLiveMatch() render.LiveMatch {
	if m := a.gsi.Current(); m.Ativa() {
		return liveMatchFromGSI(m)
	}
	if m := a.lol.Current(); m.Ativa() {
		return liveMatchFromLoL(m)
	}
	return render.LiveMatch{}
}

func liveMatchFromGSI(m gsi.Match) render.LiveMatch {
	if c := m.CS2; c != nil {
		alert := ""
		switch c.BombState {
		case "planted":
			alert = "Bomba plantada"
		case "defused":
			alert = "Bomba desarmada"
		case "exploded":
			alert = "Bomba explodiu"
		}
		sub := fmt.Sprintf("Round %d · %s", c.Round, c.Phase)
		if c.Team != "" {
			sub += " · " + c.Team
		}
		return render.LiveMatch{
			Active: true, Game: "CS2",
			Title: c.Map, Sub: sub,
			Score: fmt.Sprintf("%d - %d", c.ScoreCT, c.ScoreT),
			Alert: alert,
			Stats: []render.LiveStat{
				{Label: "K/D/A", Value: fmt.Sprintf("%d/%d/%d", c.Kills, c.Deaths, c.Assists)},
				{Label: "Vida", Value: fmt.Sprintf("%d", c.Health)},
				{Label: "Armadura", Value: fmt.Sprintf("%d", c.Armor)},
				{Label: "Dinheiro", Value: fmt.Sprintf("$%d", c.Money)},
			},
		}
	}
	if d := m.Dota2; d != nil {
		return render.LiveMatch{
			Active: true, Game: "Dota 2",
			Title: prettyName(d.Hero), Sub: fmt.Sprintf("Nível %d · %s", d.Level, fmtClock(d.GameTime)),
			Score: fmt.Sprintf("%d - %d", d.RadiantScore, d.DireScore),
			Stats: []render.LiveStat{
				{Label: "K/D/A", Value: fmt.Sprintf("%d/%d/%d", d.Kills, d.Deaths, d.Assists)},
				{Label: "CS", Value: fmt.Sprintf("%d/%d", d.LastHits, d.Denies)},
				{Label: "Ouro/min", Value: fmt.Sprintf("%d", d.GPM)},
				{Label: "XP/min", Value: fmt.Sprintf("%d", d.XPM)},
			},
		}
	}
	return render.LiveMatch{}
}

func liveMatchFromLoL(m lolapi.Match) render.LiveMatch {
	return render.LiveMatch{
		Active: true, Game: "League of Legends",
		Title: m.Champion, Sub: fmt.Sprintf("Nível %d · %s", m.Level, fmtClock(int(m.GameTime))),
		Stats: []render.LiveStat{
			{Label: "K/D/A", Value: fmt.Sprintf("%d/%d/%d", m.Kills, m.Deaths, m.Assists)},
			{Label: "CS", Value: fmt.Sprintf("%d", m.CreepScore)},
			{Label: "Ouro", Value: fmt.Sprintf("%d", m.CurrentGold)},
		},
	}
}

// prettyName transforma um nome interno tipo "shadow_fiend" em "Shadow Fiend".
func prettyName(internal string) string {
	parts := strings.Split(internal, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

// fmtClock formata segundos como "m:ss" (o tempo de jogo do Dota2 pode ser
// negativo durante a preparação antes do horn).
func fmtClock(seconds int) string {
	neg := seconds < 0
	if neg {
		seconds = -seconds
	}
	s := fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
	if neg {
		return "-" + s
	}
	return s
}

// currentFPS lê a taxa de quadros do RTSS (RivaTuner), se estiver rodando.
func (a *App) currentFPS() float64 {
	l, err := rtss.Ler()
	if err != nil {
		return 0
	}
	return l.FPS
}

// Reconnect derruba a conexão de um dispositivo e tenta de novo.
// id vazio ("") reconecta todos (usado depois de encerrar conflitos).
func (a *App) Reconnect(id string) {
	a.devMu.RLock()
	defer a.devMu.RUnlock()
	if id == "" {
		for _, d := range a.devices {
			select {
			case d.reconnect <- struct{}{}:
			default:
			}
		}
		return
	}
	if d, ok := a.devices[id]; ok {
		select {
		case d.reconnect <- struct{}{}:
		default:
		}
	}
}

func (a *App) setStatus(d *device, s, text string) {
	d.mu.Lock()
	changed := d.status != s || d.statusText != text
	d.status, d.statusText = s, text
	d.mu.Unlock()
	if changed {
		log.Printf("tela %s: %s — %s", d.id, s, text)
	}
}

func (a *App) connectionLoop(ctx context.Context, d *device) {
	defer a.claims.release(d.id)
	for ctx.Err() == nil {
		cfg := a.store.Get()
		dc, ok := deviceConfig(cfg.Devices, d.id)
		if !ok {
			return // dispositivo foi removido da configuração
		}
		lang := cfg.ResolvedLanguage()
		var disp lcd.Display
		if dc.Revision == "SIMULADO" {
			disp = lcd.NewSimulated()
		} else {
			detect := func() (string, error) { return a.claims.claim(d.id) }
			disp = lcd.NewRevA(dc.Port, lcd.OpenSerial, detect)
		}
		a.setStatus(d, StatusConnecting, i18n.T(lang, "app.connecting"))
		err := disp.Open()
		if err == nil {
			err = disp.SetBrightness(dc.Brightness)
		}
		if err == nil {
			err = disp.SetOrientation(lcd.ParseOrientation(dc.Orientation))
		}
		if err != nil {
			disp.Close()
			a.claims.release(d.id)
			a.handleConnectError(d, err, lang)
			if a.wait(ctx, d, 5*time.Second) {
				continue
			}
			return
		}
		d.mu.Lock()
		d.display = disp
		d.sent = nil
		d.applied = dc
		d.conflicts = nil
		d.mu.Unlock()
		if dc.Revision == "SIMULADO" {
			a.setStatus(d, StatusSimulated, i18n.T(lang, "app.simulated"))
		} else {
			a.setStatus(d, StatusConnected, i18n.T(lang, "app.connected", disp.PortName()))
		}

		// Fica conectado até pedirem reconexão ou o dispositivo ser removido.
		select {
		case <-ctx.Done():
		case <-d.reconnect:
		}
		d.mu.Lock()
		d.display = nil
		d.mu.Unlock()
		disp.Close()
		a.claims.release(d.id)
	}
}

func (a *App) handleConnectError(d *device, err error, lang i18n.Lang) {
	switch {
	case errors.Is(err, lcd.ErrPortBusy):
		conf := winutil.FindConflicts()
		d.mu.Lock()
		d.conflicts = conf
		d.mu.Unlock()
		a.setStatus(d, StatusBusy, i18n.T(lang, "app.port_busy"))
	case errors.Is(err, lcd.ErrNotFound):
		a.setStatus(d, StatusNotFound, i18n.T(lang, "app.not_found"))
	default:
		a.setStatus(d, StatusError, err.Error())
	}
}

// wait espera d ou um pedido de reconexão; devolve false se ctx acabou.
func (a *App) wait(ctx context.Context, d *device, dur time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-d.reconnect:
		return true
	case <-time.After(dur):
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

// choose decide a tela da vez pro dispositivo d. Precisa ser chamado com d.mu travado.
func (a *App) choose(d *device, now time.Time, mode config.ModeConfig, active []config.Screen) config.Screen {
	if d.pin != "" && now.Before(d.pinUntil) {
		return d.pin
	}
	d.pin = ""
	switch mode.Type {
	case config.ModeFixed:
		return mode.Fixed
	case config.ModeRotate:
		if len(active) == 0 {
			return config.ScreenClock
		}
		if now.After(d.rotateAt) {
			d.rotateIdx++
			d.rotateAt = now.Add(time.Duration(mode.RotateSeconds) * time.Second)
		}
		return active[d.rotateIdx%len(active)]
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
	in := render.Input{Now: now, Cfg: cfg, Media: m, Steam: st, System: sys,
		Lang: cfg.ResolvedLanguage(), SteamReady: cfg.Steam.Enabled && a.steam.Ready()}
	if cfg.GameData.Enabled {
		in.GameLive = a.currentLiveMatch()
		in.FPS = a.currentFPS()
	}

	a.devMu.RLock()
	devices := make([]*device, 0, len(a.order))
	for _, id := range a.order {
		if d, ok := a.devices[id]; ok {
			devices = append(devices, d)
		}
	}
	a.devMu.RUnlock()

	for _, d := range devices {
		dc, ok := deviceConfig(cfg.Devices, d.id)
		if !ok {
			continue
		}
		a.tickDevice(d, dc, now, cfg, in, active)
	}
}

func (a *App) tickDevice(d *device, dc config.DeviceConfig, now time.Time, cfg config.Config, in render.Input, active []config.Screen) {
	d.mu.Lock()
	if d.paused && d.frame != nil {
		d.mu.Unlock()
		return
	}
	screen := a.choose(d, now, dc.Mode, active)
	d.screen = screen
	disp := d.display
	d.mu.Unlock()

	w, h := 320, 480
	if lcd.ParseOrientation(dc.Orientation).IsLandscape() {
		w, h = 480, 320
	}
	if disp != nil {
		w, h = disp.Size()
	}

	img := render.Draw(screen, w, h, in)

	d.mu.Lock()
	d.frame = img
	d.frameID++
	prev := d.sent
	if d.forceRedraw {
		prev = nil
		d.forceRedraw = false
	}
	d.mu.Unlock()

	if disp == nil {
		return
	}
	r := lcd.DirtyRect(prev, img)
	if r.Empty() {
		return
	}
	if err := disp.Draw(img, r); err != nil {
		log.Printf("erro enviando para a tela %s: %v", d.id, err)
		a.setStatus(d, StatusError, i18n.T(cfg.ResolvedLanguage(), "app.lost_connection"))
		a.Reconnect(d.id)
		return
	}
	d.mu.Lock()
	d.sent = img
	d.mu.Unlock()
}

// --- controles usados pelo painel e pela bandeja

// Next avança (ou volta, com delta negativo) a tela do dispositivo id.
// id vazio ("") aplica em todos os dispositivos.
func (a *App) Next(id string, delta int) {
	if id == "" {
		a.devMu.RLock()
		ids := make([]string, 0, len(a.devices))
		for devID := range a.devices {
			ids = append(ids, devID)
		}
		a.devMu.RUnlock()
		for _, devID := range ids {
			a.Next(devID, delta)
		}
		return
	}
	d := a.deviceByID(id)
	if d == nil {
		return
	}
	cfg := a.store.Get()
	dc, ok := deviceConfig(cfg.Devices, id)
	if !ok {
		return
	}
	active := a.activeScreens(cfg, a.media.Get(), a.steam.Get())
	if len(active) == 0 {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	cur := 0
	for i, s := range active {
		if s == d.screen {
			cur = i
		}
	}
	next := active[((cur+delta)%len(active)+len(active))%len(active)]
	d.pin = next
	d.pinUntil = time.Now().Add(time.Minute)
	d.rotateIdx = cur + delta
	d.rotateAt = time.Now().Add(time.Duration(dc.Mode.RotateSeconds) * time.Second)
	d.paused = false
}

// SetPaused pausa/retoma o dispositivo id. id vazio ("") aplica em todos.
func (a *App) SetPaused(id string, p bool) {
	if id == "" {
		a.devMu.RLock()
		defer a.devMu.RUnlock()
		for _, d := range a.devices {
			d.mu.Lock()
			d.paused = p
			d.mu.Unlock()
		}
		return
	}
	if d := a.deviceByID(id); d != nil {
		d.mu.Lock()
		d.paused = p
		d.mu.Unlock()
	}
}

func (a *App) Paused(id string) bool {
	d := a.deviceByID(id)
	if d == nil {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.paused
}

// KillConflicts encerra os programas que prendem a porta e reconecta todos os dispositivos.
func (a *App) KillConflicts(pids []uint32) error {
	if len(pids) == 0 {
		a.devMu.RLock()
		for _, d := range a.devices {
			d.mu.Lock()
			for _, c := range d.conflicts {
				pids = append(pids, c.PID)
			}
			d.mu.Unlock()
		}
		a.devMu.RUnlock()
	}
	if err := winutil.KillElevated(pids); err != nil {
		return err
	}
	go func() {
		time.Sleep(2 * time.Second)
		a.Reconnect("")
	}()
	return nil
}

func (a *App) State() State {
	cfg := a.store.Get()
	m, st := a.media.Get(), a.steam.Get()
	active := a.activeScreens(cfg, m, st)

	a.devMu.RLock()
	order := append([]string(nil), a.order...)
	devices := make(map[string]*device, len(a.devices))
	for id, d := range a.devices {
		devices[id] = d
	}
	a.devMu.RUnlock()

	names := map[string]string{}
	for _, dc := range cfg.Devices {
		names[dc.ID] = dc.Name
	}

	out := make([]DeviceState, 0, len(order))
	for _, id := range order {
		d, ok := devices[id]
		if !ok {
			continue
		}
		d.mu.Lock()
		port := ""
		if d.display != nil {
			port = d.display.PortName()
		}
		out = append(out, DeviceState{
			ID: id, Name: names[id], Status: d.status, StatusText: d.statusText, Port: port,
			Screen: d.screen, Paused: d.paused, Pinned: d.pin != "" && time.Now().Before(d.pinUntil),
			Conflicts: d.conflicts, FrameID: d.frameID, Active: active,
		})
		d.mu.Unlock()
	}
	lm := a.currentLiveMatch()
	// CS2/Dota2 só leem o .cfg de GSI ao abrir: se a Steam mostra o jogo
	// aberto mas nada chegou ainda, é sinal de que falta reiniciar o jogo.
	aguardandoReinicio := cfg.GameData.Enabled && !lm.Active && st.Playing && (st.AppID == 730 || st.AppID == 570)
	return State{
		Devices: out, Media: m, MediaError: a.media.LastError(), Steam: st, SteamError: a.steam.LastError(),
		System: a.sys.Get(), Version: a.Version, Update: a.updater.Available(),
		Mancer: MancerState{Enabled: cfg.Mancer.Enabled, Connected: a.mancer.Connected(), Error: a.mancer.LastError()},
		GameData: GameDataState{
			Enabled: cfg.GameData.Enabled, Active: lm.Active, Game: lm.Game, FPS: a.currentFPS(),
			AguardandoReinicio: aguardandoReinicio,
		},
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

// PreviewPNG devolve o frame atual do dispositivo id em PNG (com cache por frame).
func (a *App) PreviewPNG(id string) ([]byte, uint64) {
	cfg := a.store.Get()
	d := a.deviceByID(id)
	if d == nil {
		img := render.Message(320, 480, i18n.T(render.CanvasLang(cfg.ResolvedLanguage()), "app.starting"), "", cfg.Theme)
		var buf bytes.Buffer
		_ = (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&buf, img)
		return buf.Bytes(), 0
	}
	d.mu.Lock()
	img, frameID := d.frame, d.frameID
	if d.pngID == frameID && d.pngCache != nil {
		b := d.pngCache
		d.mu.Unlock()
		return b, frameID
	}
	d.mu.Unlock()
	if img == nil {
		img = render.Message(320, 480, i18n.T(render.CanvasLang(cfg.ResolvedLanguage()), "app.starting"), "", cfg.Theme)
	}
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	_ = enc.Encode(&buf, img)
	d.mu.Lock()
	d.pngCache, d.pngID = buf.Bytes(), frameID
	d.mu.Unlock()
	return buf.Bytes(), frameID
}

func (a *App) TestSteam(ctx context.Context, key, id string) (string, error) {
	return a.steam.Test(ctx, key, id)
}

func (a *App) TestSteamLocal() (string, string, error) { return a.steam.TestLocal() }
