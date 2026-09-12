//go:build windows

package media

import "testing"

func TestFriendlyApp(t *testing.T) {
	cases := map[string]string{
		"Spotify.exe":                           "Spotify",
		"Chrome.exe":                            "Chrome",
		"msedge.exe":                            "Edge",
		"firefox.exe":                           "Firefox",
		"Microsoft.ZuneMusic_8wekyb3d8bbwe!App": "Media Player",
		"vlc.exe":                    "VLC",
		"deezer.exe":                 "Deezer",
		"tidal.exe":                  "TIDAL",
		"discord.exe":                "Discord",
		`C:\Some\Path\Unknown.exe`:   "Unknown",
		"Unknown":                    "Unknown",
	}
	for id, want := range cases {
		if got := friendlyApp(id); got != want {
			t.Errorf("friendlyApp(%q) = %q, esperava %q", id, got, want)
		}
	}
}
