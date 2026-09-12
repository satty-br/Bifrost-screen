//go:build !windows

package media

// SetForTest permite simular uma música tocando (usado nos testes e no preview).
func (r *Reader) SetForTest(i Info) { r.set(i, nil) }
