//go:build !windows

package steam

import "errors"

type noSteamEnv struct{}

func defaultLocalEnv() localEnv { return noSteamEnv{} }

func (noSteamEnv) SteamPath() (string, error)  { return "", errors.New("só no Windows") }
func (noSteamEnv) RunningAppID() (int, error)  { return 0, nil }
func (noSteamEnv) ActiveUser() (uint32, error) { return 0, nil }
func (noSteamEnv) RegistryAppName(int) string  { return "" }
