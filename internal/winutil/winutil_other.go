//go:build !windows

package winutil

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func ListProcesses() ([]Process, error) { return nil, nil }
func KillElevated([]uint32) error       { return errors.New("só no Windows") }
func RunSelfElevated(args string) error { return errors.New("só no Windows") }

// OpenURL abre um endereço no navegador padrão: "open" no macOS, "xdg-open" no Linux/BSD.
func OpenURL(url string) error {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", url).Start()
	}
	return exec.Command("xdg-open", url).Start()
}

// SetAutostart liga/desliga a inicialização junto com o sistema (só para este usuário):
// arquivo .desktop em ~/.config/autostart no Linux, LaunchAgent no macOS.
func SetAutostart(enabled bool) error {
	path, err := autostartPath()
	if err != nil {
		return err
	}
	if !enabled {
		if runtime.GOOS == "darwin" {
			_ = exec.Command("launchctl", "unload", "-w", path).Run()
		}
		err := os.Remove(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(autostartContents(exe)), 0o644); err != nil {
		return err
	}
	if runtime.GOOS == "darwin" {
		_ = exec.Command("launchctl", "load", "-w", path).Run()
	}
	return nil
}

// AutostartEnabled diz se o Bifrost está configurado para iniciar com o sistema.
func AutostartEnabled() bool {
	path, err := autostartPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

func autostartPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "LaunchAgents", "com.bifrost.screen.plist"), nil
	}
	return filepath.Join(home, ".config", "autostart", "bifrost.desktop"), nil
}

func autostartContents(exe string) string {
	if runtime.GOOS == "darwin" {
		return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.bifrost.screen</string>
	<key>ProgramArguments</key>
	<array>
		<string>` + exe + `</string>
		<string>--segundo-plano</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
</dict>
</plist>
`
	}
	return `[Desktop Entry]
Type=Application
Name=Bifrost Screen
Exec="` + exe + `" --segundo-plano
X-GNOME-Autostart-enabled=true
`
}

func SingleInstance() bool    { return true }
func Alert(title, msg string) { fmt.Fprintln(os.Stderr, title+": "+msg) }
func FocusWindow(string) bool { return false }
func NamedMutex(string) bool  { return true }
