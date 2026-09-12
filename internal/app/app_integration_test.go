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
// nunca é acionado durante os testes.
func newTestApp(t *testing.T) (*App, *config.Store) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Display.Revision = "SIMULADO"
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
	return a, store
}

func TestNewAppAccessors(t *testing.T) {
	a, _ := newTestApp(t)
	if a.Media() == nil || a.Steam() == nil || a.System() == nil {
		t.Fatal("acessores não deveriam devolver nil")
	}
	if a.Version != "test" {
		t.Errorf("Version = %q", a.Version)
	}
	st := a.State()
	if st.Status != StatusConnecting {
		t.Errorf("status inicial = %q, esperava %q", st.Status, StatusConnecting)
	}
}

func TestPreviewPNGBeforeRun(t *testing.T) {
	a, _ := newTestApp(t)
	data, id := a.PreviewPNG()
	if len(data) == 0 {
		t.Fatal("PreviewPNG() deveria devolver bytes mesmo sem Run()")
	}
	if len(data) < 8 || string(data[1:4]) != "PNG" {
		t.Error("PreviewPNG() não parece um PNG válido")
	}
	// segunda chamada usa o cache (mesmo id de frame).
	data2, id2 := a.PreviewPNG()
	if id != id2 || string(data) != string(data2) {
		t.Error("segunda chamada deveria devolver o mesmo frame em cache")
	}
}

func TestSetPausedAndPaused(t *testing.T) {
	a, _ := newTestApp(t)
	if a.Paused() {
		t.Error("não deveria começar pausado")
	}
	a.SetPaused(true)
	if !a.Paused() {
		t.Error("deveria estar pausado após SetPaused(true)")
	}
	a.SetPaused(false)
	if a.Paused() {
		t.Error("deveria despausar")
	}
}

func TestKillConflictsNoPids(t *testing.T) {
	a, _ := newTestApp(t)
	if err := a.KillConflicts(nil); err != nil {
		t.Errorf("KillConflicts(nil) sem conflitos não deveria falhar: %v", err)
	}
}

func TestNextWithoutActiveScreens(t *testing.T) {
	a, _ := newTestApp(t)
	// não deve entrar em pânico mesmo sem telas ativas ainda calculadas.
	a.Next(1)
	a.Next(-1)
}

func TestReconnectDoesNotBlock(t *testing.T) {
	a, _ := newTestApp(t)
	a.Reconnect()
	a.Reconnect() // canal com buffer 1: a segunda chamada não pode travar.
}

func TestRunConnectsSimulated(t *testing.T) {
	a, _ := newTestApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	a.Run(ctx)

	st := a.State()
	if st.Status != StatusSimulated {
		t.Errorf("status final = %q, esperava %q", st.Status, StatusSimulated)
	}
	if st.FrameID == 0 {
		t.Error("deveria ter desenhado ao menos um frame")
	}
	if st.Version != "test" {
		t.Errorf("versão no estado = %q", st.Version)
	}
	switch st.Screen {
	case config.ScreenMusic, config.ScreenGame, config.ScreenSystem, config.ScreenClock:
	default:
		t.Errorf("tela atual inesperada: %q", st.Screen)
	}
}

func TestTestSteamLocalDoesNotPanic(t *testing.T) {
	a, _ := newTestApp(t)
	// só confere que a chamada (leitura local, sem rede) não trava; o resultado
	// depende de haver ou não uma Steam instalada na máquina de teste.
	_, _, _ = a.TestSteamLocal()
}
