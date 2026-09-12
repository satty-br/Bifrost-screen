// Package steam consulta a Steam Web API: jogo atual e tempo jogado.
package steam

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

const (
	summariesURL = "https://api.steampowered.com/ISteamUser/GetPlayerSummaries/v0002/"
	ownedURL     = "https://api.steampowered.com/IPlayerService/GetOwnedGames/v0001/"
	headerURL    = "https://cdn.akamai.steamstatic.com/steam/apps/%d/header.jpg"
)

// Status é o que o Bifrost sabe sobre o jogo atual.
type Status struct {
	Source          string      `json:"fonte"`
	SteamPath       string      `json:"pasta_steam,omitempty"`
	Playing         bool        `json:"jogando"`
	AppID           int         `json:"appid"`
	Name            string      `json:"nome"`
	PersonaName     string      `json:"perfil"`
	TotalMinutes    int         `json:"minutos_total"`
	TwoWeeksMinutes int         `json:"minutos_duas_semanas"`
	SessionStart    time.Time   `json:"inicio_sessao"`
	Cover           image.Image `json:"-"`
	HasCover        bool        `json:"tem_capa"`
}

// Settings controla a consulta.
// Fontes de dados.
const (
	SourceLocal = "local" // lê a Steam instalada no PC (registro + arquivos)
	SourceWeb   = "web"   // Steam Web API (precisa de API key)
)

// Settings controla a consulta.
type Settings struct {
	Enabled      bool
	Source       string
	APIKey       string
	SteamID64    string
	StatusEvery  time.Duration
	LibraryEvery time.Duration
}

type Client struct {
	http     *http.Client
	cacheDir string

	mu       sync.RWMutex
	settings Settings
	status   Status
	lastErr  string
	owned    map[int]ownedGame
	covers   map[int]image.Image

	sessionApp   int
	sessionStart time.Time
	lastLibrary  time.Time
	wake         chan struct{}

	local       *localReader
	localCovers map[string]image.Image
	ready       bool
}

type ownedGame struct {
	AppID           int    `json:"appid"`
	Name            string `json:"name"`
	PlaytimeForever int    `json:"playtime_forever"`
	Playtime2Weeks  int    `json:"playtime_2weeks"`
}

func New(cacheDir string) *Client {
	_ = os.MkdirAll(cacheDir, 0o755)
	return &Client{
		http:     &http.Client{Timeout: 10 * time.Second},
		cacheDir: cacheDir,
		owned:    map[int]ownedGame{},
		covers:   map[int]image.Image{},
		wake:     make(chan struct{}, 1),

		local:       newLocalReader(defaultLocalEnv()),
		localCovers: map[string]image.Image{},
	}
}

// Ready diz se a fonte escolhida está pronta (Steam achada, ou key preenchida).
func (c *Client) Ready() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ready
}

// Configure troca as credenciais/intervalos e força uma consulta imediata.
func (c *Client) Configure(s Settings) {
	c.mu.Lock()
	changed := s.APIKey != c.settings.APIKey || s.SteamID64 != c.settings.SteamID64 || s.Source != c.settings.Source
	c.settings = s
	if changed {
		c.owned = map[int]ownedGame{}
		c.lastLibrary = time.Time{}
		c.status = Status{}
		c.lastErr = ""
	}
	c.mu.Unlock()
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *Client) Get() Status {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.status
}

func (c *Client) LastError() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastErr
}

// Run consulta a Steam em segundo plano até ctx terminar.
func (c *Client) Run(ctx context.Context) {
	for {
		c.mu.RLock()
		s := c.settings
		c.mu.RUnlock()
		wait := s.StatusEvery
		if wait <= 0 {
			wait = 15 * time.Second
		}
		switch {
		case s.Enabled && s.Source == SourceLocal:
			c.refreshLocal(ctx)
			wait = 2 * time.Second
		case s.Enabled && s.APIKey != "" && s.SteamID64 != "":
			c.mu.Lock()
			c.ready = true
			c.mu.Unlock()
			c.refresh(ctx, s)
		default:
			c.mu.Lock()
			c.status = Status{}
			c.ready = false
			c.mu.Unlock()
			wait = time.Minute
		}
		select {
		case <-ctx.Done():
			return
		case <-c.wake:
		case <-time.After(wait):
		}
	}
}

// refreshLocal lê a Steam instalada no PC.
func (c *Client) refreshLocal(ctx context.Context) {
	info, err := c.local.Status()
	c.mu.Lock()
	if err != nil {
		c.lastErr = err.Error()
		c.status = Status{Source: SourceLocal}
		c.ready = false
		c.sessionApp = 0
		c.mu.Unlock()
		return
	}
	c.lastErr = ""
	c.ready = true
	st := Status{Source: SourceLocal, SteamPath: info.SteamPath, PersonaName: info.Persona}
	if info.AppID > 0 {
		st.Playing, st.AppID, st.Name = true, info.AppID, info.Name
		st.TotalMinutes, st.TwoWeeksMinutes = info.TotalMinutes, info.TwoWeeksMinutes
		if c.sessionApp != info.AppID {
			c.sessionApp, c.sessionStart = info.AppID, time.Now()
		}
		st.SessionStart = c.sessionStart
	} else {
		c.sessionApp = 0
	}
	c.mu.Unlock()

	if st.Playing {
		var cover image.Image
		if info.CoverPath != "" {
			cover = c.fileCover(info.CoverPath)
		}
		if cover == nil {
			cover = c.cover(ctx, info.AppID) // baixa do CDN público da Steam
		}
		st.Cover, st.HasCover = cover, cover != nil
	}
	c.mu.Lock()
	c.status = st
	c.mu.Unlock()
}

func (c *Client) fileCover(path string) image.Image {
	c.mu.RLock()
	img, ok := c.localCovers[path]
	c.mu.RUnlock()
	if ok {
		return img
	}
	if f, err := os.Open(path); err == nil {
		img, _, _ = image.Decode(f)
		f.Close()
	}
	c.mu.Lock()
	c.localCovers[path] = img
	c.mu.Unlock()
	return img
}

// TestLocal confere se a Steam instalada foi encontrada (botão "Testar" do painel).
func (c *Client) TestLocal() (persona, path string, err error) {
	info, err := c.local.Status()
	if err != nil {
		return "", "", err
	}
	return info.Persona, info.SteamPath, nil
}

// Test faz uma consulta única e devolve o nome do perfil (usado pelo botão "Testar").
func (c *Client) Test(ctx context.Context, apiKey, steamID string) (string, error) {
	p, err := c.summary(ctx, apiKey, steamID)
	if err != nil {
		return "", err
	}
	return p.PersonaName, nil
}

type player struct {
	PersonaName   string `json:"personaname"`
	GameID        string `json:"gameid"`
	GameExtraInfo string `json:"gameextrainfo"`
}

func (c *Client) summary(ctx context.Context, apiKey, steamID string) (player, error) {
	q := url.Values{"key": {apiKey}, "steamids": {steamID}}
	var resp struct {
		Response struct {
			Players []player `json:"players"`
		} `json:"response"`
	}
	if err := c.getJSON(ctx, summariesURL+"?"+q.Encode(), &resp); err != nil {
		return player{}, err
	}
	if len(resp.Response.Players) == 0 {
		return player{}, errors.New("SteamID64 não encontrado")
	}
	return resp.Response.Players[0], nil
}

func (c *Client) refresh(ctx context.Context, s Settings) {
	p, err := c.summary(ctx, s.APIKey, s.SteamID64)
	if err != nil {
		c.mu.Lock()
		c.lastErr = err.Error()
		c.mu.Unlock()
		return
	}
	st := Status{Source: SourceWeb, PersonaName: p.PersonaName}
	appID, _ := strconv.Atoi(p.GameID)
	if appID > 0 && p.GameExtraInfo != "" {
		st.Playing, st.AppID, st.Name = true, appID, p.GameExtraInfo
	}

	c.mu.Lock()
	needLibrary := st.Playing && (time.Since(c.lastLibrary) > s.LibraryEvery || c.owned[appID].AppID == 0)
	c.mu.Unlock()
	if needLibrary {
		if owned, err := c.library(ctx, s); err == nil {
			c.mu.Lock()
			c.owned = owned
			c.lastLibrary = time.Now()
			c.mu.Unlock()
		}
	}

	var cover image.Image
	if st.Playing {
		cover = c.cover(ctx, appID)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastErr = ""
	if st.Playing {
		if g, ok := c.owned[appID]; ok {
			st.TotalMinutes, st.TwoWeeksMinutes = g.PlaytimeForever, g.Playtime2Weeks
		}
		if c.sessionApp != appID {
			c.sessionApp, c.sessionStart = appID, time.Now()
		}
		st.SessionStart = c.sessionStart
		st.Cover = cover
		st.HasCover = cover != nil
	} else {
		c.sessionApp = 0
	}
	c.status = st
}

func (c *Client) library(ctx context.Context, s Settings) (map[int]ownedGame, error) {
	q := url.Values{
		"key": {s.APIKey}, "steamid": {s.SteamID64},
		"include_appinfo": {"1"}, "include_played_free_games": {"1"}, "format": {"json"},
	}
	var resp struct {
		Response struct {
			Games []ownedGame `json:"games"`
		} `json:"response"`
	}
	if err := c.getJSON(ctx, ownedURL+"?"+q.Encode(), &resp); err != nil {
		return nil, err
	}
	out := make(map[int]ownedGame, len(resp.Response.Games))
	for _, g := range resp.Response.Games {
		out[g.AppID] = g
	}
	return out, nil
}

func (c *Client) cover(ctx context.Context, appID int) image.Image {
	c.mu.RLock()
	img, ok := c.covers[appID]
	c.mu.RUnlock()
	if ok {
		return img
	}
	path := filepath.Join(c.cacheDir, fmt.Sprintf("steam_%d.jpg", appID))
	if _, err := os.Stat(path); err != nil {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(headerURL, appID), nil)
		if resp, err := c.http.Do(req); err == nil {
			if resp.StatusCode == http.StatusOK {
				if f, err := os.Create(path); err == nil {
					_, _ = f.ReadFrom(resp.Body)
					f.Close()
				}
			}
			resp.Body.Close()
		}
	}
	if f, err := os.Open(path); err == nil {
		img, _, _ = image.Decode(f)
		f.Close()
	}
	c.mu.Lock()
	c.covers[appID] = img
	c.mu.Unlock()
	return img
}

func (c *Client) getJSON(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("sem conexão com a Steam: %w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return errors.New("API key da Steam inválida")
	case http.StatusTooManyRequests:
		return errors.New("limite de consultas da Steam atingido, tentando mais tarde")
	default:
		return fmt.Errorf("Steam respondeu %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
