package sysinfo

import "testing"

func TestRAMPercent(t *testing.T) {
	s := Stats{RAMUsed: 8 << 30, RAMTotal: 32 << 30}
	if got := s.RAMPercent(); got != 25 {
		t.Errorf("RAMPercent() = %v, esperava 25", got)
	}
	if got := (Stats{}).RAMPercent(); got != -1 {
		t.Errorf("RAMPercent() sem total deveria ser -1, veio %v", got)
	}
}

func TestDiskPercent(t *testing.T) {
	s := Stats{DiskUsed: 300 << 30, DiskTotal: 1000 << 30}
	if got := s.DiskPercent(); got != 30 {
		t.Errorf("DiskPercent() = %v, esperava 30", got)
	}
	if got := (Stats{}).DiskPercent(); got != -1 {
		t.Errorf("DiskPercent() sem total deveria ser -1, veio %v", got)
	}
}

func TestSamplerGetAndDisk(t *testing.T) {
	var s Sampler
	if s.currentDisk() != "C:" {
		t.Errorf("disco padrão deveria ser C:, veio %q", s.currentDisk())
	}
	s.SetDisk("D:")
	if s.currentDisk() != "D:" {
		t.Errorf("SetDisk não funcionou, veio %q", s.currentDisk())
	}

	s.store(Stats{CPU: 42, GPU: 10})
	got := s.Get()
	if got.CPU != 42 || got.GPU != 10 {
		t.Errorf("store/Get não bateram: %+v", got)
	}
	if len(got.CPUHistory) != 1 || got.CPUHistory[0] != 42 {
		t.Errorf("histórico de CPU deveria ter 1 item = 42, veio %v", got.CPUHistory)
	}
}

func TestSamplerHistoryTrimsTo60(t *testing.T) {
	var s Sampler
	for i := 0; i < 70; i++ {
		s.store(Stats{CPU: float64(i)})
	}
	hist := s.Get().CPUHistory
	if len(hist) != 60 {
		t.Fatalf("histórico deveria ter no máximo 60 itens, tem %d", len(hist))
	}
	if hist[len(hist)-1] != 69 {
		t.Errorf("último item do histórico deveria ser 69, veio %v", hist[len(hist)-1])
	}
}

func TestSamplerHistoryIgnoresNegativeCPU(t *testing.T) {
	var s Sampler
	s.store(Stats{CPU: -1})
	if len(s.Get().CPUHistory) != 0 {
		t.Error("CPU negativa (indisponível) não deveria entrar no histórico")
	}
}

func TestSamplerGetReturnsIndependentHistory(t *testing.T) {
	var s Sampler
	s.store(Stats{CPU: 1})
	got := s.Get()
	got.CPUHistory[0] = 999
	if s.Get().CPUHistory[0] == 999 {
		t.Error("Get() deveria devolver uma cópia independente do histórico")
	}
}
