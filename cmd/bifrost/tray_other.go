//go:build !windows

package main

import (
	"log"
	"os"
	"os/signal"

	"github.com/satty-br/Bifrost-screen/internal/app"
	"github.com/satty-br/Bifrost-screen/internal/config"
)

// Fora do Windows não há bandeja: roda até receber Ctrl+C.
func runTray(a *app.App, store *config.Store, onQuit func()) {
	log.Printf("rodando sem bandeja (Ctrl+C para sair)")
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
	onQuit()
}
