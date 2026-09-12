//go:build !windows

package sysinfo

// TempDiagnostics só tem fontes alternativas no Windows; nos outros sistemas a
// temperatura vem direto do próprio sistema operacional (ver sysinfo_linux.go
// e sysinfo_darwin.go).
func (s *Sampler) TempDiagnostics() []TempSourceStatus { return nil }

// TempDriverStatus: o agente PawnIO só existe no Windows.
func TempDriverStatus() PawnIOStatus { return PawnIOStatus{CPU: -1} }

// GPUAdapters: a identificação da placa pelo registro só existe no Windows.
func GPUAdapters() []string { return nil }
