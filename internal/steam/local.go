package steam

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"
)

// localEnv é o que o leitor local precisa do sistema (registro do Windows).
// Separado para dar para testar sem a Steam instalada.
type localEnv interface {
	SteamPath() (string, error)
	RunningAppID() (int, error)
	ActiveUser() (uint32, error)
	RegistryAppName(appID int) string
}

// LocalInfo é o que dá para saber lendo a Steam instalada no PC.
type LocalInfo struct {
	SteamPath       string
	AppID           int
	Name            string
	Persona         string
	TotalMinutes    int
	TwoWeeksMinutes int
	CoverPath       string
}

// ErrSteamNotFound indica que a Steam não está instalada (ou não foi achada).
var ErrSteamNotFound = errors.New("Steam não encontrada neste PC")

type localReader struct {
	env localEnv

	mu        sync.Mutex
	cfgPath   string
	cfgMod    time.Time
	cfgRoot   *VDF
	names     map[int]string
	libraries []string
	libsRead  time.Time
}

func newLocalReader(env localEnv) *localReader {
	return &localReader{env: env, names: map[int]string{}}
}

// Status lê o jogo aberto e os dados dele. AppID 0 = nenhum jogo.
func (l *localReader) Status() (LocalInfo, error) {
	root, err := l.env.SteamPath()
	if err != nil || root == "" {
		return LocalInfo{}, ErrSteamNotFound
	}
	root = filepath.Clean(root)
	if _, err := os.Stat(root); err != nil {
		return LocalInfo{}, ErrSteamNotFound
	}
	info := LocalInfo{SteamPath: root}

	l.mu.Lock()
	defer l.mu.Unlock()

	cfg := l.localConfig(root)
	if cfg != nil {
		info.Persona = cfg.String("UserLocalConfigStore", "friends", "PersonaName")
	}

	id, _ := l.env.RunningAppID()
	if id <= 0 {
		return info, nil
	}
	info.AppID = id
	info.Name = l.appName(root, id)
	if cfg != nil {
		app := cfg.Get("UserLocalConfigStore", "Software", "Valve", "Steam", "apps", strconv.Itoa(id))
		if app != nil {
			info.TotalMinutes, _ = strconv.Atoi(app.String("Playtime"))
			info.TwoWeeksMinutes, _ = strconv.Atoi(app.String("Playtime2wks"))
		}
	}
	info.CoverPath = findCover(root, id)
	return info, nil
}

// localConfig lê userdata\<conta>\config\localconfig.vdf (com cache pela data do arquivo).
func (l *localReader) localConfig(root string) *VDF {
	path := ""
	if user, err := l.env.ActiveUser(); err == nil && user != 0 {
		p := filepath.Join(root, "userdata", strconv.FormatUint(uint64(user), 10), "config", "localconfig.vdf")
		if _, err := os.Stat(p); err == nil {
			path = p
		}
	}
	if path == "" {
		// Steam fechada: usa a conta usada mais recentemente.
		matches, _ := filepath.Glob(filepath.Join(root, "userdata", "*", "config", "localconfig.vdf"))
		var newest time.Time
		for _, m := range matches {
			if fi, err := os.Stat(m); err == nil && fi.ModTime().After(newest) {
				newest, path = fi.ModTime(), m
			}
		}
	}
	if path == "" {
		return nil
	}
	fi, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if path == l.cfgPath && fi.ModTime().Equal(l.cfgMod) && l.cfgRoot != nil {
		return l.cfgRoot
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return l.cfgRoot
	}
	v, err := ParseVDF(string(data))
	if err != nil {
		return l.cfgRoot
	}
	l.cfgPath, l.cfgMod, l.cfgRoot = path, fi.ModTime(), v
	return v
}

func (l *localReader) libraryPaths(root string) []string {
	if time.Since(l.libsRead) < time.Minute && len(l.libraries) > 0 {
		return l.libraries
	}
	libs := []string{root}
	if data, err := os.ReadFile(filepath.Join(root, "steamapps", "libraryfolders.vdf")); err == nil {
		if v, err := ParseVDF(string(data)); err == nil {
			if lf := v.Get("libraryfolders"); lf != nil {
				keys := make([]string, 0, len(lf.Children))
				for k := range lf.Children {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					if p := lf.Children[k].String("path"); p != "" && !samePath(p, root) {
						libs = append(libs, filepath.Clean(p))
					}
				}
			}
		}
	}
	l.libraries, l.libsRead = libs, time.Now()
	return libs
}

func samePath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b) || stringsEqualFold(filepath.Clean(a), filepath.Clean(b))
}

func (l *localReader) appName(root string, id int) string {
	if n, ok := l.names[id]; ok && n != "" {
		return n
	}
	name := ""
	for _, lib := range l.libraryPaths(root) {
		data, err := os.ReadFile(filepath.Join(lib, "steamapps", fmt.Sprintf("appmanifest_%d.acf", id)))
		if err != nil {
			continue
		}
		if v, err := ParseVDF(string(data)); err == nil {
			name = v.String("AppState", "name")
		}
		if name != "" {
			break
		}
	}
	if name == "" {
		name = l.env.RegistryAppName(id)
	}
	if name == "" {
		name = fmt.Sprintf("Jogo %d", id)
	} else {
		l.names[id] = name
	}
	return name
}

// findCover procura a capa no cache de imagens da biblioteca da Steam.
func findCover(root string, id int) string {
	cache := filepath.Join(root, "appcache", "librarycache")
	sid := strconv.Itoa(id)
	candidates := []string{
		filepath.Join(cache, sid+"_header.jpg"),
		filepath.Join(cache, sid, "header.jpg"),
	}
	if m, _ := filepath.Glob(filepath.Join(cache, sid, "*", "header.jpg")); len(m) > 0 {
		candidates = append(candidates, m...)
	}
	candidates = append(candidates,
		filepath.Join(cache, sid+"_library_hero.jpg"),
		filepath.Join(cache, sid, "library_hero.jpg"),
	)
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && fi.Size() > 0 {
			return c
		}
	}
	return ""
}
