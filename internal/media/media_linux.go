//go:build linux

package media

import (
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	mprisPrefix = "org.mpris.MediaPlayer2."
	mprisPath   = dbus.ObjectPath("/org/mpris/MediaPlayer2")
	mprisIface  = "org.mpris.MediaPlayer2.Player"
)

// Start lê o estado via MPRIS (D-Bus), o padrão usado pelos players no Linux
// (Spotify, VLC, Rhythmbox, Chrome/Firefox, etc). Exige um barramento de
// sessão D-Bus, ou seja, só funciona dentro de uma sessão gráfica normal.
func (r *Reader) Start(interval time.Duration) {
	r.backend = "mpris"
	go func() {
		var conn *dbus.Conn
		var lastCoverURL string
		var lastCover image.Image
		for {
			if conn == nil {
				c, err := dbus.SessionBus()
				if err != nil {
					r.set(Info{}, err)
					time.Sleep(interval)
					continue
				}
				conn = c
			}
			info, err := pollMPRIS(conn, lastCoverURL, lastCover)
			if err != nil {
				// barramento pode ter caído; força reconexão na próxima volta.
				conn = nil
			} else if info.Cover != nil {
				lastCover = info.Cover
			}
			r.set(info, err)
			time.Sleep(interval)
		}
	}()
}

func pollMPRIS(conn *dbus.Conn, lastCoverURL string, lastCover image.Image) (Info, error) {
	names, err := listMPRISPlayers(conn)
	if err != nil {
		return Info{}, err
	}
	if len(names) == 0 {
		return Info{}, nil
	}

	// Prefere um player que esteja tocando; senão usa o primeiro encontrado.
	var chosen, status string
	for _, n := range names {
		v, err := conn.Object(n, mprisPath).GetProperty(mprisIface + ".PlaybackStatus")
		if err != nil {
			continue
		}
		s, _ := v.Value().(string)
		if chosen == "" {
			chosen, status = n, s
		}
		if s == "Playing" {
			chosen, status = n, s
			break
		}
	}
	if chosen == "" {
		return Info{}, nil
	}

	obj := conn.Object(chosen, mprisPath)
	info := Info{
		HasSession: true,
		Playing:    status == "Playing",
		App:        strings.TrimSuffix(strings.TrimPrefix(chosen, mprisPrefix), ".instance0"),
		UpdatedAt:  time.Now(),
	}

	if v, err := obj.GetProperty(mprisIface + ".Metadata"); err == nil {
		if m, ok := v.Value().(map[string]dbus.Variant); ok {
			if t, ok := m["xesam:title"].Value().(string); ok {
				info.Title = t
			}
			if artists, ok := m["xesam:artist"].Value().([]string); ok && len(artists) > 0 {
				info.Artist = strings.Join(artists, ", ")
			}
			if al, ok := m["xesam:album"].Value().(string); ok {
				info.Album = al
			}
			if length, ok := m["mpris:length"].Value().(int64); ok {
				info.Duration = time.Duration(length) * time.Microsecond
			}
			if artURL, ok := m["mpris:artUrl"].Value().(string); ok && artURL != "" {
				if artURL == lastCoverURL {
					info.Cover = lastCover
				} else if img, err := loadLocalCover(artURL); err == nil {
					info.Cover = img
				}
			}
		}
	}

	if v, err := obj.GetProperty(mprisIface + ".Position"); err == nil {
		if pos, ok := v.Value().(int64); ok {
			info.Position = time.Duration(pos) * time.Microsecond
		}
	}

	return info, nil
}

func listMPRISPlayers(conn *dbus.Conn) ([]string, error) {
	var names []string
	if err := conn.BusObject().Call("org.freedesktop.DBus.ListNames", 0).Store(&names); err != nil {
		return nil, err
	}
	var out []string
	for _, n := range names {
		if strings.HasPrefix(n, mprisPrefix) {
			out = append(out, n)
		}
	}
	return out, nil
}

var errNotLocalCover = errors.New("capa não é um arquivo local")

// loadLocalCover só carrega capas via file:// (a maioria dos players cacheia
// localmente); URLs http/https são ignoradas para não gerar tráfego de rede.
func loadLocalCover(rawURL string) (image.Image, error) {
	path := strings.TrimPrefix(rawURL, "file://")
	if path == rawURL {
		return nil, errNotLocalCover
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}
