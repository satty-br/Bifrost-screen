package gsi

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/satty-br/Bifrost-screen/internal/steam"
)

const cfgFileName = "gamestate_integration_bifrost.cfg"

// cs2CfgTemplate segue o formato documentado pela Valve: um bloco raiz com
// nome livre, "uri" apontando pro nosso endpoint, e "data" dizendo quais
// seções o jogo deve mandar (só as usadas por parseCS2, pra não gerar
// tráfego/CPU à toa com o que não vamos ler).
const cs2CfgTemplate = `"Bifrost GSI"
{
 "uri" "http://127.0.0.1:%d/gsi"
 "timeout" "5.0"
 "buffer"  "0.1"
 "throttle" "0.1"
 "heartbeat" "30.0"
 "auth"
 {
  "token" "%s"
 }
 "data"
 {
  "provider"           "1"
  "map"                "1"
  "round"              "1"
  "player_id"          "1"
  "player_state"       "1"
  "player_match_stats" "1"
 }
}
`

const dota2CfgTemplate = `"Bifrost GSI Dota2"
{
 "uri" "http://127.0.0.1:%d/gsi"
 "timeout" "5.0"
 "buffer"  "0.1"
 "throttle" "0.1"
 "heartbeat" "30.0"
 "auth"
 {
  "token" "%s"
 }
 "data"
 {
  "provider" "1"
  "map"      "1"
  "player"   "1"
  "hero"     "1"
 }
}
`

// EnsureCS2Config escreve o .cfg de GSI na pasta do CS2 (se ainda não estiver
// lá com o conteúdo certo). O jogo só lê esse arquivo quando abre — se ele já
// estava aberto, precisa reiniciar pra valer.
func EnsureCS2Config(port int, token string) (bool, error) {
	return ensureConfig(730, filepath.Join("game", "csgo", "cfg", cfgFileName), fmt.Sprintf(cs2CfgTemplate, port, token))
}

// EnsureDota2Config faz o mesmo que EnsureCS2Config, pro Dota 2. Diferente do
// CS2, o Dota 2 só lê o .cfg de GSI dentro de uma subpasta "gamestate_integration"
// (não direto em "cfg/") — confirmado na documentação da Valve e em bibliotecas
// de referência como github.com/antonpup/Dota2GSI.
func EnsureDota2Config(port int, token string) (bool, error) {
	return ensureConfig(570, filepath.Join("game", "dota", "cfg", "gamestate_integration", cfgFileName), fmt.Sprintf(dota2CfgTemplate, port, token))
}

// ensureConfig devolve (escreveu, erro): "escreveu" é true só quando o
// arquivo foi criado ou alterado agora (útil pra avisar que precisa
// reiniciar o jogo).
func ensureConfig(appID int, relPath, content string) (bool, error) {
	dir, err := steam.InstallDir(appID)
	if err != nil {
		return false, errAppNaoInstalado
	}
	path := filepath.Join(dir, relPath)
	if existing, err := os.ReadFile(path); err == nil && string(existing) == content {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// RemoveCS2Config e RemoveDota2Config tiram o .cfg (usado quando o recurso é desligado).
func RemoveCS2Config() error {
	return removeConfig(730, filepath.Join("game", "csgo", "cfg", cfgFileName))
}
func RemoveDota2Config() error {
	return removeConfig(570, filepath.Join("game", "dota", "cfg", "gamestate_integration", cfgFileName))
}

func removeConfig(appID int, relPath string) error {
	dir, err := steam.InstallDir(appID)
	if err != nil {
		return nil // jogo não está instalado: não tem o que remover
	}
	err = os.Remove(filepath.Join(dir, relPath))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
