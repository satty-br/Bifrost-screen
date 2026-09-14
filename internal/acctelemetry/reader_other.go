//go:build !windows

package acctelemetry

import "errors"

type mmfReader struct{}

func openReader() (reader, error) {
	return nil, errors.New("telemetria do Assetto Corsa/ACC só está disponível no Windows (memória compartilhada do jogo)")
}

func (mmfReader) read(name string, size int) ([]byte, error) { return nil, errors.New("não suportado") }
func (mmfReader) close()                                      {}
