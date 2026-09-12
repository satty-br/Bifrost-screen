//go:build !windows

package media

import "time"

// Start não faz nada fora do Windows (não existe Controle de Mídia do Windows).
func (r *Reader) Start(interval time.Duration) { r.backend = "indisponivel" }

// SetForTest permite simular uma música tocando (usado nos testes e no preview).
func (r *Reader) SetForTest(i Info) { r.set(i, nil) }
