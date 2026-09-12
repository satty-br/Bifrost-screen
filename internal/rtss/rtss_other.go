//go:build !windows

package rtss

import "errors"

// Ler: o RTSS só existe no Windows.
func Ler() (Leitura, error) { return Leitura{}, errors.New("RTSS só existe no Windows") }
