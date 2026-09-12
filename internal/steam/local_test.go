package steam

import (
	"os"
	"path/filepath"
	"testing"
)

type fakeEnv struct {
	root    string
	running int
	user    uint32
}

func (f fakeEnv) SteamPath() (string, error)  { return f.root, nil }
func (f fakeEnv) RunningAppID() (int, error)  { return f.running, nil }
func (f fakeEnv) ActiveUser() (uint32, error) { return f.user, nil }
func (f fakeEnv) RegistryAppName(int) string  { return "" }

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLocalReader(t *testing.T) {
	root := t.TempDir()
	lib2 := t.TempDir()
	write(t, filepath.Join(root, "steamapps", "libraryfolders.vdf"),
		`"libraryfolders" { "0" { "path" "`+filepath.ToSlash(root)+`" } "1" { "path" "`+filepath.ToSlash(lib2)+`" } }`)
	write(t, filepath.Join(lib2, "steamapps", "appmanifest_730.acf"), `"AppState" { "appid" "730" "name" "Counter-Strike 2" }`)
	write(t, filepath.Join(root, "userdata", "12345", "config", "localconfig.vdf"), sampleLocalConfig)
	write(t, filepath.Join(root, "appcache", "librarycache", "730", "abc123", "header.jpg"), "jpg")

	r := newLocalReader(fakeEnv{root: root, running: 0, user: 12345})
	info, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if info.AppID != 0 || info.Persona != "satty" {
		t.Fatalf("sem jogo: %+v", info)
	}

	r = newLocalReader(fakeEnv{root: root, running: 730, user: 12345})
	info, err = r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "Counter-Strike 2" || info.TotalMinutes != 5423 || info.TwoWeeksMinutes != 340 {
		t.Errorf("dados errados: %+v", info)
	}
	if filepath.Base(info.CoverPath) != "header.jpg" {
		t.Errorf("capa não encontrada: %q", info.CoverPath)
	}

	// jogo sem manifesto: nome genérico, e usuário desconhecido cai no localconfig mais recente
	r = newLocalReader(fakeEnv{root: root, running: 999, user: 0})
	info, _ = r.Status()
	if info.Name != "Jogo 999" || info.Persona != "satty" {
		t.Errorf("fallback errado: %+v", info)
	}

	if _, err := newLocalReader(fakeEnv{root: filepath.Join(root, "nao-existe")}).Status(); err != ErrSteamNotFound {
		t.Errorf("esperava ErrSteamNotFound, veio %v", err)
	}
}
