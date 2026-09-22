// Package media lê o que está tocando no Controle de Mídia do Windows (SMTC).
package media

import (
	"image"
	"sync"
	"time"
)

// Info é o estado atual da mídia.
type Info struct {
	HasSession bool          `json:"tem_sessao"`
	Playing    bool          `json:"tocando"`
	Title      string        `json:"titulo"`
	Artist     string        `json:"artista"`
	Album      string        `json:"album"`
	App        string        `json:"app"`
	Position   time.Duration `json:"-"`
	Duration   time.Duration `json:"-"`
	PositionS  float64       `json:"posicao_s"`
	DurationS  float64       `json:"duracao_s"`
	// UpdatedAt é quando a posição foi lida, para estimar o avanço entre leituras.
	UpdatedAt time.Time   `json:"-"`
	Cover     image.Image `json:"-"`
	HasCover  bool        `json:"tem_capa"`
}

// EstimatedPosition avança a posição com o relógio enquanto a música toca.
func (i Info) EstimatedPosition(now time.Time) time.Duration {
	p := i.Position
	if i.Playing && !i.UpdatedAt.IsZero() {
		p += now.Sub(i.UpdatedAt)
	}
	if i.Duration > 0 && p > i.Duration {
		p = i.Duration
	}
	return p
}

// Reader consulta a mídia periodicamente em segundo plano.
type Reader struct {
	mu      sync.RWMutex
	info    Info
	err     string
	backend string
	// rawPos/rawAt guardam a última posição realmente diferente que o player
	// reportou (e quando), pra detectar quando a leitura crua ficou "grudada".
	rawPos time.Duration
	rawAt  time.Time
}

func (r *Reader) Get() Info {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.info
}

func (r *Reader) LastError() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.err
}

func (r *Reader) set(info Info, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err != nil {
		r.err = err.Error()
		r.info = Info{}
		r.rawAt = time.Time{}
		return
	}
	r.err = ""

	if !info.HasSession {
		r.rawAt = time.Time{}
	} else {
		sameTrack := r.info.HasSession &&
			r.info.Title == info.Title && r.info.Artist == info.Artist &&
			r.info.Album == info.Album && r.info.App == info.App

		if !sameTrack || r.rawAt.IsZero() || info.Position != r.rawPos {
			// Posição realmente nova (faixa trocou, sessão nova ou o player
			// reportou um valor diferente): vira a referência daqui pra frente.
			r.rawPos, r.rawAt = info.Position, info.UpdatedAt
		} else if info.Playing {
			// Alguns players (sobretudo vídeo em navegador) só empurram a
			// posição pro SO de vez em quando, então a leitura crua fica
			// parada por vários polls seguidos mesmo com a mídia tocando.
			// Extrapola a partir da última leitura que de fato mudou, senão
			// o contador na tela trava.
			info.Position = r.rawPos + info.UpdatedAt.Sub(r.rawAt)
			if info.Duration > 0 && info.Position > info.Duration {
				info.Position = info.Duration
			}
		}
	}

	info.PositionS = info.Position.Seconds()
	info.DurationS = info.Duration.Seconds()
	info.HasCover = info.Cover != nil
	r.info = info
}
