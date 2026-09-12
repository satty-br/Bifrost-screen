//go:build darwin

package media

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// usep é um separador improvável de aparecer em título/artista/álbum (ASCII 31).
const usep = "\x1f"

// Start no macOS: não existe um "now playing" universal sem o framework privado
// da Apple (MediaRemote, não documentado, exigiria CGO). Em vez disso, consulta
// via AppleScript os dois players com dicionário público mais usados: Music.app
// e Spotify. Não busca capa (evita ler artwork binária via AppleScript).
func (r *Reader) Start(interval time.Duration) {
	r.backend = "applescript"
	go func() {
		for {
			info, err := pollAppleScript()
			r.set(info, err)
			time.Sleep(interval)
		}
	}()
}

func pollAppleScript() (Info, error) {
	running, err := runningApps()
	if err != nil {
		return Info{}, err
	}
	if running["Music"] {
		if info, ok := queryMusicApp(); ok {
			return info, nil
		}
	}
	if running["Spotify"] {
		if info, ok := querySpotify(); ok {
			return info, nil
		}
	}
	return Info{}, nil
}

func runningApps() (map[string]bool, error) {
	out, err := runOsascript(`tell application "System Events" to name of every process`)
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, name := range strings.Split(out, ",") {
		set[strings.TrimSpace(name)] = true
	}
	return set, nil
}

func queryMusicApp() (Info, bool) {
	return parsePlayerScript(`tell application "Music"
	if player state is playing or player state is paused then
		return (player state as string) & "`+usep+`" & (name of current track) & "`+usep+`" & (artist of current track) & "`+usep+`" & (album of current track) & "`+usep+`" & (duration of current track as string) & "`+usep+`" & (player position as string)
	end if
	return "none"
end tell`, "Music", time.Second)
}

func querySpotify() (Info, bool) {
	// Diferente do Music.app, o "duration" do Spotify vem em milissegundos.
	return parsePlayerScript(`tell application "Spotify"
	if player state is playing or player state is paused then
		return (player state as string) & "`+usep+`" & (name of current track) & "`+usep+`" & (artist of current track) & "`+usep+`" & (album of current track) & "`+usep+`" & (duration of current track as string) & "`+usep+`" & (player position as string)
	end if
	return "none"
end tell`, "Spotify", time.Millisecond)
}

func parsePlayerScript(script, appName string, durationUnit time.Duration) (Info, bool) {
	out, err := runOsascript(script)
	if err != nil || out == "" || out == "none" {
		return Info{}, false
	}
	parts := strings.Split(out, usep)
	if len(parts) != 6 {
		return Info{}, false
	}
	state, title, artist, album, durStr, posStr := parts[0], parts[1], parts[2], parts[3], parts[4], parts[5]
	dur, _ := strconv.ParseFloat(durStr, 64)
	pos, _ := strconv.ParseFloat(posStr, 64)
	return Info{
		HasSession: true,
		Playing:    state == "playing",
		Title:      title,
		Artist:     artist,
		Album:      album,
		App:        appName,
		Duration:   time.Duration(dur * float64(durationUnit)),
		Position:   time.Duration(pos * float64(time.Second)),
		UpdatedAt:  time.Now(),
	}, true
}

func runOsascript(script string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "osascript", "-e", script).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
}
