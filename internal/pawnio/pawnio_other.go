//go:build !windows

package pawnio

import "errors"

// Fora do Windows não existe PawnIO: Linux e macOS entregam a temperatura pelo
// próprio sistema (hwmon / SMC), tratados em internal/sysinfo.

var (
	ErrNaoInstalado = errors.New("o driver PawnIO só existe no Windows")
	ErrSemPermissao = ErrNaoInstalado
	ErrCPU          = ErrNaoInstalado
)

type Leitor struct{}

func NovoLeitor() (*Leitor, error)                 { return nil, ErrNaoInstalado }
func (l *Leitor) TemperaturaCPU() (float64, error) { return 0, ErrNaoInstalado }
func (l *Leitor) Fechar()                          {}
func (l *Leitor) Fonte() string                    { return "PawnIO" }

func Instalado() (bool, string) { return false, "" }
func Elevado() bool             { return false }
func Instalar() error           { return ErrNaoInstalado }
func Desinstalar() error        { return ErrNaoInstalado }
