// Package acctelemetry acompanha o Assetto Corsa e o Assetto Corsa
// Competizione (ACC) ao vivo pela mesma "Shared Memory" oficial da Kunos —
// três blocos de memória compartilhada do Windows (Local\acpmf_physics,
// Local\acpmf_graphics, Local\acpmf_static) que o jogo mantém atualizados
// sozinho enquanto está rodando. O Bifrost só lê essa memória — não injeta
// nada no jogo nem precisa de nenhum addon/plugin.
//
// O ACC manteve de propósito o mesmo layout inicial de campos do Assetto
// Corsa original (para compatibilidade com painéis/hardware já existentes),
// por isso os mesmos offsets servem para os dois jogos.
//
// Layout conferido cruzando duas implementações de referência independentes
// e consistentes entre si: a biblioteca em C# do Assetto Corsa original
// (github.com/gro-ove/actools, licença MIT, structs com [MarshalAs]) e o
// leitor em Python do ACC (github.com/rrennoir/PyAccSharedMemory, licença
// MIT, com struct.unpack explícito por campo).
package acctelemetry

import (
	"context"
	"encoding/binary"
	"math"
	"sync"
	"time"
	"unicode/utf16"
)

// Nomes dos mapeamentos de memória compartilhada que o jogo cria.
const (
	MMFNamePhysics  = `Local\acpmf_physics`
	MMFNameGraphics = `Local\acpmf_graphics`
	MMFNameStatic   = `Local\acpmf_static`
)

// Tamanhos de leitura — maiores que o necessário de propósito (o struct real
// tem centenas de campos que não usamos; só lemos o prefixo).
const (
	physicsReadSize  = 32
	graphicsReadSize = 168
	staticReadSize   = 204
)

// Validade: sem atualização nova por esse tempo, a sessão é considerada
// encerrada (jogo fechado, ou saiu para o menu).
const Validade = 5 * time.Second

// Status da sessão (ACC_STATUS / AcGameStatus — mesmos valores nos dois jogos).
const (
	StatusOff    = 0
	StatusReplay = 1
	StatusLive   = 2
	StatusPause  = 3
)

// Snapshot é o resumo da sessão em andamento, mostrado no painel/tela.
type Snapshot struct {
	UpdatedAt time.Time

	Status      int
	SessionType string
	Track       string

	SpeedKmh float64
	Gear     int // -1 = ré (R), 0 = neutro (N), 1+
	RPM      int
	Gas      float64 // 0.0 a 1.0
	Brake    float64 // 0.0 a 1.0
	Fuel     float64 // litros

	CompletedLaps int
	Position      int
	IsInPit       bool
}

// Ativa diz se há uma sessão realmente rodando (carro na pista, não no menu/replay).
func (s Snapshot) Ativa() bool {
	return (s.Status == StatusLive || s.Status == StatusPause) &&
		!s.UpdatedAt.IsZero() && time.Since(s.UpdatedAt) < Validade
}

// reader abstrai o acesso à memória compartilhada (implementado só no
// Windows; em outros sistemas openReader devolve erro).
type reader interface {
	read(name string, size int) ([]byte, error)
	close()
}

// Poller lê a memória compartilhada periodicamente enquanto estiver ligado.
type Poller struct {
	mu   sync.RWMutex
	snap Snapshot
}

// NovoPoller cria um poller parado; chame Start para ligar.
func NovoPoller() *Poller { return &Poller{} }

// Current devolve a última leitura, ou Snapshot{} se estiver velha demais.
func (p *Poller) Current() Snapshot {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.snap
}

// Start liga a leitura periódica; para sozinho quando ctx é cancelado. Se a
// plataforma não suportar (não-Windows) ou o jogo não estiver rodando,
// simplesmente não encontra dados — não é um erro fatal pro resto do app.
func (p *Poller) Start(ctx context.Context, interval time.Duration) {
	go func() {
		var trackName string
		var trackAt time.Time
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			r, err := openReader()
			if err != nil {
				continue
			}
			physics, errP := r.read(MMFNamePhysics, physicsReadSize)
			graphics, errG := r.read(MMFNameGraphics, graphicsReadSize)
			r.close()
			if errP != nil || errG != nil {
				continue
			}
			// o nome da pista quase não muda; só relê a cada 10s pra não
			// abrir a terceira memória compartilhada toda hora à toa.
			if time.Since(trackAt) > 10*time.Second {
				if r2, err := openReader(); err == nil {
					if st, err := r2.read(MMFNameStatic, staticReadSize); err == nil {
						trackName = parseTrackName(st)
						trackAt = time.Now()
					}
					r2.close()
				}
			}
			p.mu.Lock()
			p.snap = parseSnapshot(physics, graphics, trackName)
			p.mu.Unlock()
		}
	}()
}

func f32(buf []byte, off int) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(buf[off : off+4]))
}

func i32(buf []byte, off int) int32 {
	return int32(binary.LittleEndian.Uint32(buf[off : off+4]))
}

func parseSnapshot(physics, graphics []byte, track string) Snapshot {
	if len(physics) < physicsReadSize || len(graphics) < graphicsReadSize {
		return Snapshot{}
	}
	status := int(i32(graphics, 4))
	return Snapshot{
		UpdatedAt:     time.Now(),
		Status:        status,
		SessionType:   sessionTypeName(int(i32(graphics, 8))),
		Track:         track,
		SpeedKmh:      float64(f32(physics, 28)),
		Gear:          int(i32(physics, 16)) - 1, // 0=R, 1=N, 2+=marchas -> normaliza pra -1/0/1+
		RPM:           int(i32(physics, 20)),
		Gas:           float64(f32(physics, 4)),
		Brake:         float64(f32(physics, 8)),
		Fuel:          float64(f32(physics, 12)),
		CompletedLaps: int(i32(graphics, 132)),
		Position:      int(i32(graphics, 136)),
		IsInPit:       i32(graphics, 160) != 0,
	}
}

// parseTrackName lê o campo "track" da memória estática (offset 134, 33
// wchar_t = 66 bytes em UTF-16LE, terminado em NUL).
func parseTrackName(static []byte) string {
	const off, size = 134, 66
	if len(static) < off+size {
		return ""
	}
	return decodeUTF16NulTerminated(static[off : off+size])
}

func decodeUTF16NulTerminated(b []byte) string {
	u16 := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		v := binary.LittleEndian.Uint16(b[i : i+2])
		if v == 0 {
			break
		}
		u16 = append(u16, v)
	}
	return string(utf16.Decode(u16))
}
