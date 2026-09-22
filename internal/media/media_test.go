package media

import (
	"errors"
	"image"
	"testing"
	"time"
)

func TestEstimatedPosition(t *testing.T) {
	now := time.Now()
	i := Info{Playing: true, Position: 10 * time.Second, Duration: 60 * time.Second, UpdatedAt: now.Add(-5 * time.Second)}
	got := i.EstimatedPosition(now)
	if got < 14*time.Second || got > 16*time.Second {
		t.Errorf("posição estimada = %v, esperava ~15s", got)
	}
}

func TestEstimatedPositionPaused(t *testing.T) {
	now := time.Now()
	i := Info{Playing: false, Position: 10 * time.Second, UpdatedAt: now.Add(-5 * time.Second)}
	if got := i.EstimatedPosition(now); got != 10*time.Second {
		t.Errorf("pausado não deveria avançar, veio %v", got)
	}
}

func TestEstimatedPositionZeroUpdatedAt(t *testing.T) {
	i := Info{Playing: true, Position: 10 * time.Second}
	if got := i.EstimatedPosition(time.Now()); got != 10*time.Second {
		t.Errorf("sem UpdatedAt não deveria avançar, veio %v", got)
	}
}

func TestEstimatedPositionClampsToDuration(t *testing.T) {
	now := time.Now()
	i := Info{Playing: true, Position: 55 * time.Second, Duration: 60 * time.Second, UpdatedAt: now.Add(-30 * time.Second)}
	if got := i.EstimatedPosition(now); got != 60*time.Second {
		t.Errorf("posição não deveria passar da duração, veio %v", got)
	}
}

func TestReaderGetSetLastError(t *testing.T) {
	var r Reader
	if r.Get().HasSession {
		t.Error("leitor novo não deveria ter sessão")
	}
	if r.LastError() != "" {
		t.Error("leitor novo não deveria ter erro")
	}

	cover := image.NewRGBA(image.Rect(0, 0, 4, 4))
	r.set(Info{HasSession: true, Title: "Título", Position: 30 * time.Second, Duration: 200 * time.Second, Cover: cover}, nil)
	got := r.Get()
	if !got.HasSession || got.Title != "Título" {
		t.Errorf("info não foi salva corretamente: %+v", got)
	}
	if !got.HasCover {
		t.Error("HasCover deveria ser true quando há capa")
	}
	if got.PositionS != 30 || got.DurationS != 200 {
		t.Errorf("PositionS/DurationS não foram calculados: %+v", got)
	}

	r.set(Info{}, errors.New("falha de teste"))
	if r.LastError() != "falha de teste" {
		t.Errorf("LastError() = %q", r.LastError())
	}
	if r.Get().HasSession {
		t.Error("erro deveria limpar a sessão")
	}
}

// Reproduz o "contador não atualiza": alguns players (vídeo em navegador,
// por ex.) só reportam a posição pro SO de vez em quando, então a leitura
// crua chega igualzinha em vários polls seguidos mesmo com a mídia tocando.
func TestReaderSetExtrapolatesPosicaoTravada(t *testing.T) {
	var r Reader
	base := time.Now()
	track := Info{HasSession: true, Playing: true, Title: "Vídeo", Position: 10 * time.Second, Duration: 300 * time.Second, UpdatedAt: base}
	r.set(track, nil)
	if got := r.Get().Position; got != 10*time.Second {
		t.Fatalf("posição inicial = %v, esperava 10s", got)
	}

	// O player não atualizou a posição crua, mas continua "tocando" 4s depois.
	track.UpdatedAt = base.Add(4 * time.Second)
	r.set(track, nil)
	if got := r.Get().Position; got != 14*time.Second {
		t.Errorf("posição travada deveria ser extrapolada, veio %v (esperava 14s)", got)
	}

	// Um novo poll, ainda sem a posição crua mudar.
	track.UpdatedAt = base.Add(7 * time.Second)
	r.set(track, nil)
	if got := r.Get().Position; got != 17*time.Second {
		t.Errorf("posição travada deveria continuar avançando, veio %v (esperava 17s)", got)
	}

	// Quando o player finalmente reporta um valor novo (seek ou atualização
	// de verdade), essa passa a ser a referência.
	track.Position = 50 * time.Second
	track.UpdatedAt = base.Add(8 * time.Second)
	r.set(track, nil)
	if got := r.Get().Position; got != 50*time.Second {
		t.Errorf("posição nova reportada deveria ser respeitada, veio %v (esperava 50s)", got)
	}
}
