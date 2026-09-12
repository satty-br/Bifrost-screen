// Bifrost: painel de música, jogo e sistema para a tela USB Turing/UsbMonitor 3.5".
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/satty-br/Bifrost-screen/internal/app"
	"github.com/satty-br/Bifrost-screen/internal/config"
	"github.com/satty-br/Bifrost-screen/internal/web"
	"github.com/satty-br/Bifrost-screen/internal/winutil"
)

// version é preenchida no build (-ldflags "-X main.version=...").
var version = "dev"

const panelTitle = "Bifrost"

func main() {
	panelOnly := flag.Bool("painel", false, "abre só a janela do painel (usado internamente)")
	background := flag.Bool("segundo-plano", false, "inicia sem abrir o painel (usado na inicialização do Windows)")
	diagnostics := flag.Bool("diagnostico", false, "gera um relatório (portas, música, sistema) e sai")
	sensorAgent := flag.Bool("sensores", false, "modo agente: publica a temperatura da CPU (usado pela tarefa agendada)")
	sensorSetup := flag.Bool("instalar-sensores", false, "instala o driver de temperatura e liga o agente (pede administrador)")
	sensorRemove := flag.Bool("remover-sensores", false, "desliga o agente de temperatura e remove o driver (pede administrador)")
	flag.Parse()

	if *sensorAgent {
		runSensorAgent()
		return
	}
	if *sensorSetup || *sensorRemove {
		runSensorSetup(*sensorRemove)
		return
	}

	dir := config.Dir()
	store, err := config.Open(dir)
	if err != nil {
		winutil.Alert("Bifrost", "Não consegui ler a configuração:\n"+err.Error())
		os.Exit(1)
	}
	cfg := store.Get()
	url := fmt.Sprintf("http://127.0.0.1:%d/", cfg.General.WebPort)

	if *panelOnly {
		runPanel(url, dir)
		return
	}
	if *diagnostics {
		runDiagnostics(store, dir)
		return
	}

	// Já tem um Bifrost rodando? Só abre o painel dele.
	if !winutil.SingleInstance() {
		openPanel()
		return
	}

	logPath := filepath.Join(dir, "bifrost.log")
	if w, err := web.OpenLog(logPath); err == nil {
		log.SetOutput(w)
	}
	log.Printf("Bifrost %s iniciando (config em %s)", version, store.Path())

	a := app.New(store, filepath.Join(dir, "cache"), version)
	srv := &web.Server{App: a, Store: store, LogPath: logPath, Port: cfg.General.WebPort}
	ln, err := srv.Listen()
	if err != nil {
		winutil.Alert("Bifrost", fmt.Sprintf("A porta %d do painel já está em uso por outro programa.\n\n%v", cfg.General.WebPort, err))
		os.Exit(1)
	}
	if err := winutil.SetAutostart(cfg.General.Autostart); err != nil {
		log.Printf("inicialização com o Windows: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	quit := func() {
		log.Printf("encerrando")
		cancel()
		sctx, c := context.WithTimeout(context.Background(), 2*time.Second)
		defer c()
		srv.Shutdown(sctx)
		time.Sleep(300 * time.Millisecond) // deixa o loop fechar a porta serial
	}
	srv.OnRestart = func() {
		quit()
		os.Exit(0)
	}
	go func() {
		if err := srv.Serve(ln); err != nil {
			log.Printf("painel: %v", err)
		}
	}()
	go a.Run(ctx)
	startDemo(a)

	if cfg.General.OpenPanel && !*background {
		go func() {
			time.Sleep(300 * time.Millisecond)
			openPanel()
		}()
	}

	runTray(a, store, quit)
}

// openPanel abre (ou traz para a frente) a janela do painel num processo separado,
// porque a janela e o ícone da bandeja precisam cada um da sua thread principal.
func openPanel() {
	if winutil.FocusWindow(panelTitle) {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(exe, "--painel")
	if err := cmd.Start(); err != nil {
		log.Printf("abrindo painel: %v", err)
		return
	}
	go cmd.Wait()
}
