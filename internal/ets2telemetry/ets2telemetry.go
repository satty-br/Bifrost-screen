// Package ets2telemetry lê a telemetria ao vivo do Euro Truck Simulator 2 e do
// American Truck Simulator (SCS Software) pelo SDK oficial de telemetria: o
// jogo, com o plugin "SCS Telemetry" instalado na pasta plugins/, escreve os
// dados num bloco de memória compartilhada do Windows (nome fixo
// "Local\SCSTelemetry"). O Bifrost só lê essa memória — não injeta nada no
// jogo nem lê o processo dele diretamente.
//
// Plugin de referência (open source, distribuído pela própria comunidade SCS
// e usado por dashboards como o SCS Telemetry Web Server):
// https://github.com/RenCloud/scs-sdk-plugin — os offsets abaixo seguem o
// layout de "zonas" documentado em scs-telemetry-common.hpp daquele projeto.
package ets2telemetry

import (
	"context"
	"encoding/binary"
	"math"
	"sync"
	"time"
)

// MMFName é o nome do mapeamento de memória que o plugin do SCS cria.
const MMFName = `Local\SCSTelemetry`

// MMFSize é o tamanho reservado pelo plugin pra essa memória.
const MMFSize = 32 * 1024

// Validade: sem leitura nova por esse tempo (jogo fechado, plugin não
// instalado...), a leitura é considerada velha.
const Validade = 3 * time.Second

// Offsets (em bytes) dentro da struct de memória compartilhada, conforme as
// "zonas" documentadas no scs-telemetry-common.hpp do RenCloud/scs-sdk-plugin.
const (
	offSDKActive  = 0   // zona 1: bool
	offGameID     = 52  // zona 2: scs_values.game (u32) — 0=desconhecido, 1=ETS2, 2=ATS
	offGearDash   = 508 // zona 3: truck_i.gearDashboard (int32)
	offFuelCapCfg = 704 // zona 4: config_f.fuelCapacity (float, litros)
	offSpeed      = 948 // zona 4: truck_f.speed (float, m/s)
	offEngineRPM  = 952 // zona 4: truck_f.engineRpm (float)
	offFuel       = 1000 // zona 4: truck_f.fuel (float, litros)
	offFuelRange  = 1008 // zona 4: truck_f.fuelRange (float, km)
	offSpeedLimit = 1068 // zona 4: truck_f.speedLimit (float, m/s)
)

// Snapshot é o resumo da viagem em andamento, mostrado no painel/tela.
type Snapshot struct {
	UpdatedAt time.Time

	Game       string // "Euro Truck Simulator 2" ou "American Truck Simulator"
	SpeedKMH   float64
	Gear       int
	EngineRPM  int
	FuelLiters float64
	FuelPct    float64 // 0-100, -1 se a capacidade não vier preenchida
	FuelRangeKM float64
	SpeedLimitKMH float64
}

// Ativa diz se a leitura é recente o bastante pra ser considerada "ao vivo".
func (s Snapshot) Ativa() bool {
	return !s.UpdatedAt.IsZero() && time.Since(s.UpdatedAt) < Validade
}

func f32(buf []byte, off int) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(buf[off : off+4]))
}

// parseSnapshot devolve (Snapshot, false) se o buffer for curto demais ou se
// o SDK não estiver ativo (jogo fechado, ou plugin não carregado ainda).
func parseSnapshot(buf []byte) (Snapshot, bool) {
	if len(buf) < offSpeedLimit+4 {
		return Snapshot{}, false
	}
	if buf[offSDKActive] == 0 {
		return Snapshot{}, false
	}
	game := "Euro Truck Simulator 2"
	if binary.LittleEndian.Uint32(buf[offGameID:offGameID+4]) == 2 {
		game = "American Truck Simulator"
	}
	fuelCap := f32(buf, offFuelCapCfg)
	fuel := f32(buf, offFuel)
	fuelPct := -1.0
	if fuelCap > 0 {
		fuelPct = float64(fuel/fuelCap) * 100
	}
	return Snapshot{
		UpdatedAt:     time.Now(),
		Game:          game,
		SpeedKMH:      float64(f32(buf, offSpeed)) * 3.6,
		Gear:          int(int32(binary.LittleEndian.Uint32(buf[offGearDash : offGearDash+4]))),
		EngineRPM:     int(f32(buf, offEngineRPM)),
		FuelLiters:    float64(fuel),
		FuelPct:       fuelPct,
		FuelRangeKM:   float64(f32(buf, offFuelRange)),
		SpeedLimitKMH: float64(f32(buf, offSpeedLimit)) * 3.6,
	}, true
}

// reader abstrai o acesso à memória compartilhada (implementado só no
// Windows; em outros sistemas openReader devolve erro).
type reader interface {
	read() ([]byte, error)
	close()
}

// Poller lê a memória compartilhada periodicamente enquanto estiver ligado.
type Poller struct {
	mu   sync.RWMutex
	snap Snapshot
}

// NovoPoller cria um poller parado; chame Start para ligar.
func NovoPoller() *Poller { return &Poller{} }

// Start liga a leitura periódica; para sozinho quando ctx é cancelado. Se a
// plataforma não suportar (não-Windows) ou o plugin não estiver instalado,
// simplesmente não encontra dados — não é um erro fatal pro resto do app.
func (p *Poller) Start(ctx context.Context, interval time.Duration) {
	r, err := openReader()
	if err != nil {
		return
	}
	go func() {
		defer r.close()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				buf, err := r.read()
				if err != nil {
					continue
				}
				snap, ok := parseSnapshot(buf)
				p.mu.Lock()
				if ok {
					p.snap = snap
				} else {
					p.snap = Snapshot{}
				}
				p.mu.Unlock()
			}
		}
	}()
}

// Current devolve a última leitura, ou Snapshot{} se estiver velha demais
// (ver Snapshot.Ativa).
func (p *Poller) Current() Snapshot {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.snap
}
