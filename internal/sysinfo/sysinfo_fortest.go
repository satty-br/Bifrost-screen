//go:build !windows

package sysinfo

// SetForTest injeta uma leitura (usado no preview/testes fora do Windows).
func (s *Sampler) SetForTest(st Stats) { s.store(st) }
