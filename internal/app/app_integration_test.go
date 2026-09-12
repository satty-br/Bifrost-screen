package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/satty-br/Bifrost-screen/internal/config"
)

// newTestApp cria uma App de verdade, com a tela em modo SIMULADO (sem hardware)
// e a Steam desligada, gravando o config.json direto no disco *antes* de abrir o
// Store — assim nunca chamamos Store.Set() numa App já registrada, e o
// onConfig() (que grava a inicialização automática no registro do Windows)
// nunca é acionado durante os testes. Devolve também o ID do dispositivo padrão.
func newTestApp(t *testing.T) (*App, *config.Store, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Devices[0].Revision = "SIMULADO"
	cfg.Steam.Enabled = false
	cfg.General.Autostart = false
	cfg.General.RefreshMillis = 250 // será clampado para o mínimo (250ms)
	cfg.Normalize()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := config.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := New(store, filepath.Join(dir, "cache"), "test")
	return a, store, store.Get().Devices[0].ID
}

// waitForDevice espera até a goroutine de conexão do dispositivo id existir
// (Run cria os dispositivos de forma assíncrona em relação a quem o chamou).
func waitForDevice(t *testing.T, a *App, id string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if a.deviceByID(id) != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("dispositivo nunca apareceu")
}

func TestNewAppAccessors(t *testing.T) {
	a, _, _ := newTestApp(t)
	if a.Media() == nil || a.Steam() == nil || a.System() == nil {
		t.Fatal("acessores não deveriam devolver nil")
	}
	if a.Version != "test" {
		t.Errorf("Version = %q", a.Version)
	}
	// antes do Run(), nenhum dispositivo foi instanciado ainda.
	if st := a.State(); len(st.Devices) != 0 {
		t.Errorf("Devices antes do Run() = %v, esperava vazio", st.Devices)
	}
}

func TestPreviewPNGBeforeRun(t *testing.T) {
	a, _, id := newTestApp(t)
	data, frameID := a.PreviewPNG(id)
	if len(data) == 0 {
		t.Fatal("PreviewPNG() deveria devolver bytes mesmo sem Run()")
	}
	if len(data) < 8 || string(data[1:4]) != "PNG" {
		t.Error("PreviewPNG() não parece um PNG válido")
	}
	// segunda chamada devolve o mesmo frame "iniciando" (ainda sem dispositivo real).
	data2, frameID2 := a.PreviewPNG(id)
	if frameID != frameID2 || string(data) != string(data2) {
		t.Error("segunda chamada deveria devolver o mesmo frame")
	}
}

func TestSetPausedAndPaused(t *testing.T) {
	a, _, id := newTestApp(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go a.Run(ctx)
	waitForDevice(t, a, id)

	if a.Paused(id) {
		t.Error("não deveria começar pausado")
	}
	a.SetPaused(id, true)
	if !a.Paused(id) {
		t.Error("deveria estar pausado após SetPaused(id, true)")
	}
	a.SetPaused(id, false)
	if a.Paused(id) {
		t.Error("deveria despausar")
	}
}

func TestKillConflictsNoPids(t *testing.T) {
	a, _, _ := newTestApp(t)
	if err := a.KillConflicts(nil); err != nil {
		t.Errorf("KillConflicts(nil) sem conflitos não deveria falhar: %v", err)
	}
}

func TestNextWithoutActiveScreens(t *testing.T) {
	a, _, _ := newTestApp(t)
	// não deve entrar em pânico mesmo sem dispositivos/telas ativas ainda.
	a.Next("", 1)
	a.Next("", -1)
}

func TestReconnectDoesNotBlock(t *testing.T) {
	a, _, _ := newTestApp(t)
	a.Reconnect("")
	a.Reconnect("") // canal com buffer 1: a segunda chamada não pode travar.
}

func TestRunConnectsSimulated(t *testing.T) {
	a, _, id := newTestApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	a.Run(ctx)

	st := a.State()
	if len(st.Devices) != 1 {
		t.Fatalf("esperava 1 dispositivo no estado, veio %d", len(st.Devices))
	}
	ds := st.Devices[0]
	if ds.ID != id {
		t.Errorf("ID do dispositivo = %q, esperava %q", ds.ID, id)
	}
	if ds.Status != StatusSimulated {
		t.Errorf("status final = %q, esperava %q", ds.Status, StatusSimulated)
	}
	if ds.FrameID == 0 {
		t.Error("deveria ter desenhado ao menos um frame")
	}
	if st.Version != "test" {
		t.Errorf("versão no estado = %q", st.Version)
	}
	switch ds.Screen {
	case config.ScreenMusic, config.ScreenGame, config.ScreenSystem, config.ScreenClock:
	default:
		t.Errorf("tela atual inesperada: %q", ds.Screen)
	}
}


func TestTestSteamLocalDoesNotPanic(t *testing.T) {
	a, _, _ := newTestApp(t)
	// só confere que a chamada (leitura local, sem rede) não trava; o resultado
	// depende de haver ou não uma Steam instalada na máquina de teste.
	_, _, _ = a.TestSteamLocal()
}

// TestMultipleSimulatedDevices confere o cerne do suporte a múltiplas telas:
// dois dispositivos simulados, cada um fixo numa tela diferente, rodando ao
// mesmo tempo de forma independente.
func TestMultipleSimulatedDevices(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Devices[0].Revision = "SIMULADO"
	cfg.Devices[0].ID = "d1"
	cfg.Devices[0].Mode = config.ModeConfig{Type: config.ModeFixed, Fixed: config.ScreenClock}
	second := cfg.Devices[0]
	second.ID = "d2"
	second.Mode = config.ModeConfig{Type: config.ModeFixed, Fixed: config.ScreenSystem}
	cfg.Devices = append(cfg.Devices, second)
	cfg.Steam.Enabled = false
	cfg.General.RefreshMillis = 250
	cfg.Normalize()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := config.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := New(store, filepath.Join(dir, "cache"), "test")
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	a.Run(ctx)

	st := a.State()
	if len(st.Devices) != 2 {
		t.Fatalf("esperava 2 dispositivos no estado, veio %d", len(st.Devices))
	}
	byID := map[string]DeviceState{}
	for _, d := range st.Devices {
		byID[d.ID] = d
	}
	if byID["d1"].Screen != config.ScreenClock {
		t.Errorf("d1 deveria estar fixo no relógio, veio %q", byID["d1"].Screen)
	}
	if byID["d2"].Screen != config.ScreenSystem {
		t.Errorf("d2 deveria estar fixo no sistema, veio %q", byID["d2"].Screen)
	}
	if byID["d1"].Status != StatusSimulated || byID["d2"].Status != StatusSimulated {
		t.Errorf("os dois dispositivos deveriam estar conectados (simulados): %+v", st.Devices)
	}
	if byID["d1"].FrameID == 0 || byID["d2"].FrameID == 0 {
		t.Error("os dois dispositivos deveriam ter desenhado ao menos um frame")
	}
}

// TestDeviceRemovedFromConfigStopsSession confere que remover um dispositivo
// da configuração encerra a goroutine dele, sem afetar os outros.
func TestDeviceRemovedFromConfigStopsSession(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Devices[0].Revision = "SIMULADO"
	cfg.Devices[0].ID = "d1"
	second := cfg.Devices[0]
	second.ID = "d2"
	cfg.Devices = append(cfg.Devices, second)
	cfg.Steam.Enabled = false
	cfg.General.RefreshMillis = 250
	cfg.Normalize()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := config.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := New(store, filepath.Join(dir, "cache"), "test")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go a.Run(ctx)
	waitForDevice(t, a, "d1")
	waitForDevice(t, a, "d2")

	newCfg := store.Get()
	newCfg.Devices = newCfg.Devices[:1] // mantém só d1
	if _, err := store.Set(newCfg); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && a.deviceByID("d2") != nil {
		time.Sleep(10 * time.Millisecond)
	}
	if a.deviceByID("d2") != nil {
		t.Error("d2 deveria ter sido removido depois de sair da configuração")
	}
	if a.deviceByID("d1") == nil {
		t.Error("d1 não deveria ter sido afetado pela remoção de d2")
	}
}
