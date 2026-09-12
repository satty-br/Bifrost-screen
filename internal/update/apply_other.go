//go:build !windows

package update

import (
	"os"
	"os/exec"
)

// Apply instala o binário baixado em newPath no lugar do executável atual e
// relança o app. No Linux/macOS um processo pode substituir (rename) o
// arquivo do binário que já está rodando: o processo atual continua usando o
// inode antigo até sair, então isso é seguro sem precisar de um script
// auxiliar como no Windows. O chamador deve encerrar o processo atual logo
// depois (Apply não bloqueia esperando o processo morrer).
func Apply(newPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.Chmod(newPath, 0o755); err != nil {
		return err
	}
	if err := os.Rename(newPath, exe); err != nil {
		return err
	}
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Start()
}
