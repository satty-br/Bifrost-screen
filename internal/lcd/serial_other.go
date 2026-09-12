//go:build !windows

package lcd

import "errors"

type PortInfo struct {
	Name     string `json:"nome"`
	VIDPID   string `json:"vid_pid"`
	Serial   string `json:"serial"`
	IsScreen bool   `json:"e_a_tela"`
}

var errOnlyWindows = errors.New("a tela USB só é suportada no Windows nesta versão")

func OpenSerial(string) (Port, error) { return nil, errOnlyWindows }
func ListPorts() ([]PortInfo, error)  { return nil, nil }
func DetectRevA() (string, error)     { return "", ErrNotFound }
