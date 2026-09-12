// Package sysinfo mede uso de CPU, GPU, memória, rede e disco.
package sysinfo

import (
	"sync"
	"time"
)

// Stats é uma leitura do sistema. Valores negativos significam "indisponível".
type Stats struct {
	CPU     float64 `json:"cpu"`
	GPU     float64 `json:"gpu"`
	CPUTemp float64 `json:"cpu_temperatura"` // graus Celsius
	GPUTemp float64 `json:"gpu_temperatura"` // graus Celsius
	// De onde veio cada temperatura ("HWiNFO", "LibreHardwareMonitor (web)"…).
	CPUTempSource string        `json:"cpu_temperatura_fonte,omitempty"`
	GPUTempSource string        `json:"gpu_temperatura_fonte,omitempty"`
	RAMUsed       uint64        `json:"ram_usada"`
	RAMTotal      uint64        `json:"ram_total"`
	NetDown       float64       `json:"rede_down"` // bytes/s
	NetUp         float64       `json:"rede_up"`   // bytes/s
	DiskUsed      uint64        `json:"disco_usado"`
	DiskTotal     uint64        `json:"disco_total"`
	Uptime        time.Duration `json:"-"`
	UptimeS       float64       `json:"tempo_ligado_s"`
	CPUHistory    []float64     `json:"-"`
}

// RAMPercent devolve o uso de memória em %.
func (s Stats) RAMPercent() float64 {
	if s.RAMTotal == 0 {
		return -1
	}
	return float64(s.RAMUsed) * 100 / float64(s.RAMTotal)
}

// DiskPercent devolve o uso do disco em %.
func (s Stats) DiskPercent() float64 {
	if s.DiskTotal == 0 {
		return -1
	}
	return float64(s.DiskUsed) * 100 / float64(s.DiskTotal)
}

// Sampler lê o sistema periodicamente.
type Sampler struct {
	mu      sync.RWMutex
	stats   Stats
	disk    string
	history []float64

	gpuTemp   float64 // cache da temperatura da GPU (nvidia-smi é caro para chamar toda hora)
	gpuTempAt time.Time

	hwCPUTemp, hwGPUTemp float64 // cache dos sensores externos (HWiNFO, LHM, Afterburner…)
	hwTempSource         string
	hwTempAt             time.Time
}

func (s *Sampler) Get() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st := s.stats
	st.CPUHistory = append([]float64(nil), s.history...)
	return st
}

// SetDisk escolhe o disco medido (ex: "C:").
func (s *Sampler) SetDisk(d string) {
	s.mu.Lock()
	s.disk = d
	s.mu.Unlock()
}

func (s *Sampler) currentDisk() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.disk == "" {
		return "C:"
	}
	return s.disk
}

func (s *Sampler) store(st Stats) {
	st.UptimeS = st.Uptime.Seconds()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stats = st
	if st.CPU >= 0 {
		s.history = append(s.history, st.CPU)
		if len(s.history) > 60 {
			s.history = s.history[len(s.history)-60:]
		}
	}
}
