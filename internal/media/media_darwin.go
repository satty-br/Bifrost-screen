//go:build darwin

package media

import "time"

// Start no macOS: não há um equivalente público e sem CGO ao SMTC do Windows
// nem ao MPRIS do Linux (o framework MediaRemote é privado da Apple).
func (r *Reader) Start(interval time.Duration) { r.backend = "indisponivel" }
