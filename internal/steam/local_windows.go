//go:build windows

package steam

import (
	"errors"
	"strconv"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const steamKey = `Software\Valve\Steam`

type winSteamEnv struct{}

func defaultLocalEnv() localEnv { return winSteamEnv{} }

func (winSteamEnv) SteamPath() (string, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, steamKey, registry.QUERY_VALUE)
	if err != nil {
		return "", err
	}
	defer k.Close()
	p, _, err := k.GetStringValue("SteamPath")
	if err != nil || p == "" {
		return "", errors.New("SteamPath não definido")
	}
	return strings.ReplaceAll(p, "/", `\`), nil
}

// RunningAppID é atualizado pela própria Steam quando um jogo abre e fecha.
func (winSteamEnv) RunningAppID() (int, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, steamKey, registry.QUERY_VALUE)
	if err != nil {
		return 0, err
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("RunningAppID")
	if err != nil {
		return 0, err
	}
	return int(v), nil
}

func (winSteamEnv) ActiveUser() (uint32, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, steamKey+`\ActiveProcess`, registry.QUERY_VALUE)
	if err != nil {
		return 0, err
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("ActiveUser")
	return uint32(v), err
}

func (winSteamEnv) RegistryAppName(appID int) string {
	k, err := registry.OpenKey(registry.CURRENT_USER, steamKey+`\Apps\`+strconv.Itoa(appID), registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	n, _, _ := k.GetStringValue("Name")
	return n
}
