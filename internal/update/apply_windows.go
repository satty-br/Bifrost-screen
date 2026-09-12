//go:build windows

package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// Apply instala o binário baixado em newPath no lugar do executável atual e
// relança o app. O Windows não deixa sobrescrever um .exe rodando, então isso
// é feito por um .bat auxiliar: espera este processo terminar, troca os
// arquivos e reabre o app. O chamador deve encerrar o processo atual logo
// depois (Apply não bloqueia esperando o processo morrer).
func Apply(newPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	pid := os.Getpid()
	bat := filepath.Join(os.TempDir(), fmt.Sprintf("bifrost_update_%d.bat", pid))
	script := "@echo off\r\n" +
		":espera\r\n" +
		fmt.Sprintf("tasklist /fi \"PID eq %d\" | find \"%d\" >nul\r\n", pid, pid) +
		"if not errorlevel 1 (\r\n" +
		"  timeout /t 1 /nobreak >nul\r\n" +
		"  goto espera\r\n" +
		")\r\n" +
		fmt.Sprintf("move /y \"%s\" \"%s\" >nul\r\n", newPath, exe) +
		fmt.Sprintf("start \"\" \"%s\"\r\n", exe) +
		"del \"%~f0\"\r\n"
	if err := os.WriteFile(bat, []byte(script), 0o644); err != nil {
		return err
	}
	cmd := exec.Command("cmd", "/C", bat)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}
