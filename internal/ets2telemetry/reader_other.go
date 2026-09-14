//go:build !windows

package ets2telemetry

import "errors"

func openReader() (reader, error) {
	return nil, errors.New("telemetria do ETS2/ATS só está disponível no Windows (memória compartilhada do plugin SCS)")
}
