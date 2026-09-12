//go:build windows

package winutil

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	shell32                 = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteW       = shell32.NewProc("ShellExecuteW")
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procMessageBoxW         = user32.NewProc("MessageBoxW")
	procFindWindowW         = user32.NewProc("FindWindowW")
	procShowWindow          = user32.NewProc("ShowWindow")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
)

// FocusWindow traz para a frente uma janela pelo título. Devolve false se não achar.
func FocusWindow(title string) bool {
	t, _ := windows.UTF16PtrFromString(title)
	h, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(t)))
	if h == 0 {
		return false
	}
	procShowWindow.Call(h, 9) // SW_RESTORE
	procSetForegroundWindow.Call(h)
	return true
}

// NamedMutex cria um mutex com nome; devolve false se já existir.
func NamedMutex(name string) bool {
	n, _ := windows.UTF16PtrFromString(name)
	_, err := windows.CreateMutex(nil, true, n)
	return !errors.Is(err, windows.ERROR_ALREADY_EXISTS)
}

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const runValue = "Bifrost"

func ListProcesses() ([]Process, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	if err := windows.Process32First(snap, &e); err != nil {
		return nil, err
	}
	var out []Process
	for {
		out = append(out, Process{Name: windows.UTF16ToString(e.ExeFile[:]), PID: e.ProcessID})
		if err := windows.Process32Next(snap, &e); err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
				break
			}
			return out, err
		}
	}
	return out, nil
}

func shellExecute(verb, file, args string, show int32) error {
	v, _ := windows.UTF16PtrFromString(verb)
	f, _ := windows.UTF16PtrFromString(file)
	var a *uint16
	if args != "" {
		a, _ = windows.UTF16PtrFromString(args)
	}
	r, _, _ := procShellExecuteW.Call(0, uintptr(unsafe.Pointer(v)), uintptr(unsafe.Pointer(f)), uintptr(unsafe.Pointer(a)), 0, uintptr(show))
	if r <= 32 {
		if r == 5 { // SE_ERR_ACCESSDENIED: o usuário recusou o UAC
			return errors.New("permissão de administrador recusada")
		}
		return fmt.Errorf("ShellExecute falhou (%d)", r)
	}
	return nil
}

// KillElevated encerra os PIDs pedindo permissão de administrador (janela do UAC),
// necessário porque o app oficial da tela roda como administrador.
func KillElevated(pids []uint32) error {
	if len(pids) == 0 {
		return nil
	}
	var args []string
	for _, p := range pids {
		args = append(args, fmt.Sprintf("/PID %d", p))
	}
	return shellExecute("runas", "taskkill.exe", "/F /T "+strings.Join(args, " "), 0)
}

// OpenURL abre um endereço no navegador padrão.
func OpenURL(url string) error { return shellExecute("open", url, "", 1) }

// RunSelfElevated roda este mesmo executável como administrador (janela do
// UAC). Usado para instalar o driver de temperatura, que exige privilégio.
func RunSelfElevated(args string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return shellExecute("runas", exe, args, 0)
}

// SetAutostart liga/desliga a inicialização junto com o Windows (só para este usuário).
func SetAutostart(enabled bool) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if !enabled {
		err := k.DeleteValue(runValue)
		if errors.Is(err, registry.ErrNotExist) || errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
			return nil
		}
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return k.SetStringValue(runValue, fmt.Sprintf(`"%s" --segundo-plano`, exe))
}

// AutostartEnabled diz se o Bifrost está no registro de inicialização.
func AutostartEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(runValue)
	return err == nil
}

// SingleInstance devolve false se já existir outro Bifrost rodando.
func SingleInstance() bool { return NamedMutex(`Local\BifrostScreenApp`) }

// Alert mostra uma caixa de mensagem (para erros fatais, já que não há console).
func Alert(title, msg string) {
	t, _ := windows.UTF16PtrFromString(title)
	m, _ := windows.UTF16PtrFromString(msg)
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0x10)
}
