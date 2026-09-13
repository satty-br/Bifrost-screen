//go:build windows

package pawnio

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Interface do driver (PawnIO/include/pawnio_um.h): tipo de dispositivo 41394,
// funções 0x821 (carregar módulo) e 0x841 (executar função), ambas
// METHOD_BUFFERED/FILE_ANY_ACCESS.
const (
	devicePath      = `\\?\GLOBALROOT\Device\PawnIO`
	ioctlLoadBinary = 0xA1B22084
	ioctlExecuteFn  = 0xA1B22104
	fnNameLen       = 32 // tamanho fixo do nome da função no buffer de entrada
)

var (
	ErrNaoInstalado = errors.New("o driver PawnIO não está instalado")
	ErrSemPermissao = errors.New("o driver PawnIO só responde a um processo executado como administrador")
	ErrCPU          = errors.New("esta CPU não é suportada pelos módulos do PawnIO (só Intel x64 e AMD famílias 17h-1Ah)")
)

var (
	kernel32                 = windows.NewLazySystemDLL("kernel32.dll")
	procSetThreadAffinityMsk = kernel32.NewProc("SetThreadAffinityMask")
	procGetCurrentThread     = kernel32.NewProc("GetCurrentThread")
)

type dispositivo struct{ h windows.Handle }

// Cada handle do driver carrega no máximo um módulo, então abrimos um por módulo.
func abrir() (*dispositivo, error) {
	p, err := windows.UTF16PtrFromString(devicePath)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		switch {
		case errors.Is(err, windows.ERROR_FILE_NOT_FOUND), errors.Is(err, windows.ERROR_PATH_NOT_FOUND):
			return nil, ErrNaoInstalado
		case errors.Is(err, windows.ERROR_ACCESS_DENIED):
			return nil, ErrSemPermissao
		}
		return nil, fmt.Errorf("abrindo o driver PawnIO: %w", err)
	}
	return &dispositivo{h: h}, nil
}

func (d *dispositivo) fechar() {
	if d != nil && d.h != 0 {
		windows.CloseHandle(d.h)
		d.h = 0
	}
}

func (d *dispositivo) carregar(blob []byte) error {
	if len(blob) == 0 {
		return errors.New("módulo vazio")
	}
	var ret uint32
	return windows.DeviceIoControl(d.h, ioctlLoadBinary,
		&blob[0], uint32(len(blob)), nil, 0, &ret, nil)
}

// executar chama uma função do módulo carregado. Entrada e saída são vetores
// de inteiros de 64 bits; o módulo exige exatamente os tamanhos que declarou.
func (d *dispositivo) executar(fn string, in []uint64, saidas int) ([]uint64, error) {
	if len(fn) >= fnNameLen {
		return nil, fmt.Errorf("nome de função longo demais: %s", fn)
	}
	buf := make([]byte, fnNameLen+8*len(in))
	copy(buf, fn)
	for i, v := range in {
		binary.LittleEndian.PutUint64(buf[fnNameLen+8*i:], v)
	}
	out := make([]uint64, saidas)
	var outPtr *byte
	if saidas > 0 {
		outPtr = (*byte)(unsafe.Pointer(&out[0]))
	}
	var ret uint32
	err := windows.DeviceIoControl(d.h, ioctlExecuteFn,
		&buf[0], uint32(len(buf)), outPtr, uint32(saidas*8), &ret, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn, err)
	}
	return out, nil
}

// Leitor mantém o módulo certo para a CPU desta máquina carregado.
type Leitor struct {
	mu    sync.Mutex
	dev   *dispositivo
	amd   bool
	tjmax uint64 // IA32_TEMPERATURE_TARGET, lido uma vez
}

// NovoLeitor descobre a CPU pela própria carga do módulo: o IntelMSR recusa
// carregar fora de uma Intel x64, e o AMDFamily17 fora de um Ryzen 17h-1Ah.
func NovoLeitor() (*Leitor, error) {
	if runtime.GOARCH != "amd64" {
		return nil, ErrCPU
	}
	var primeiroErro error
	for _, m := range []struct {
		blob []byte
		amd  bool
	}{
		{moduleIntelMSR, false},
		{moduleAMDFamily17, true},
	} {
		d, err := abrir()
		if err != nil {
			return nil, err // driver ausente ou sem permissão: não adianta tentar o outro
		}
		if err := d.carregar(m.blob); err != nil {
			if primeiroErro == nil {
				primeiroErro = err
			}
			d.fechar()
			continue
		}
		return &Leitor{dev: d, amd: m.amd}, nil
	}
	if primeiroErro != nil {
		return nil, fmt.Errorf("%w (%v)", ErrCPU, primeiroErro)
	}
	return nil, ErrCPU
}

func (l *Leitor) Fechar() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.dev.fechar()
}

// Fonte identifica de onde veio a leitura, para mostrar no painel.
func (l *Leitor) Fonte() string {
	if l != nil && l.amd {
		return "PawnIO (AMD)"
	}
	return "PawnIO (Intel)"
}

// TemperaturaCPU devolve a temperatura em °C.
func (l *Leitor) TemperaturaCPU() (float64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.amd {
		return l.tempAMD()
	}
	return l.tempIntel()
}

func (l *Leitor) lerMSR(msr uint64) (uint64, error) {
	out, err := l.dev.executar("ioctl_read_msr", []uint64{msr}, 1)
	if err != nil {
		return 0, err
	}
	return out[0], nil
}

func (l *Leitor) tempIntel() (float64, error) {
	if l.tjmax == 0 {
		if v, err := l.lerMSR(msrTemperatureTarget); err == nil {
			l.tjmax = v
		}
	}
	// Primeiro a temperatura do pacote; se não vier válida, a do núcleo 0.
	if v, err := l.lerMSR(msrPackageThermStatus); err == nil {
		if t, ok := intelTemp(v, l.tjmax); ok {
			return t, nil
		}
	}
	var v uint64
	var err error
	comAfinidade(0, func() { v, err = l.lerMSR(msrThermStatus) })
	if err != nil {
		return 0, err
	}
	t, ok := intelTemp(v, l.tjmax)
	if !ok {
		return 0, errors.New("a CPU respondeu uma leitura de temperatura inválida")
	}
	return t, nil
}

func (l *Leitor) tempAMD() (float64, error) {
	// O módulo pede que o mutex de barramento PCI seja tomado antes de mexer
	// no par de registradores índice/dados do SMN — é a convenção que as
	// ferramentas de monitoramento usam entre si.
	liberar := travarPCI()
	out, err := l.dev.executar("ioctl_read_smn", []uint64{smnZenCurTmp}, 1)
	liberar()
	if err != nil {
		return 0, err
	}
	t, ok := zenTemp(uint32(out[0]))
	if !ok {
		return 0, errors.New("a CPU respondeu uma leitura de temperatura inválida")
	}
	return t, nil
}

// comAfinidade roda f preso a um núcleo, porque os MSR de temperatura são por
// núcleo e o Windows pode mudar a thread de lugar no meio da leitura.
func comAfinidade(nucleo uint, f func()) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	th, _, _ := procGetCurrentThread.Call()
	anterior, _, _ := procSetThreadAffinityMsk.Call(th, uintptr(1)<<nucleo)
	defer func() {
		if anterior != 0 {
			procSetThreadAffinityMsk.Call(th, anterior)
		}
	}()
	f()
}

func travarPCI() func() {
	nome, err := windows.UTF16PtrFromString(`Global\Access_PCI`)
	if err != nil {
		return func() {}
	}
	h, err := windows.CreateMutex(nil, false, nome)
	if err != nil && h == 0 {
		return func() {}
	}
	if _, err := windows.WaitForSingleObject(h, 20); err != nil {
		windows.CloseHandle(h)
		return func() {}
	}
	return func() {
		windows.ReleaseMutex(h)
		windows.CloseHandle(h)
	}
}

// --- instalação -----------------------------------------------------------

const chaveDesinstalar = `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\PawnIO`

// Instalado diz se o driver já está na máquina (e em qual versão).
func Instalado() (bool, string) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, chaveDesinstalar, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return false, ""
	}
	defer k.Close()
	v, _, err := k.GetStringValue("DisplayVersion")
	if err != nil {
		return true, ""
	}
	return true, v
}

// Elevado diz se este processo tem privilégio de administrador — sem ele o
// driver recusa abrir (a permissão do dispositivo é só SYSTEM + Administradores).
func Elevado() bool {
	var t windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &t); err != nil {
		return false
	}
	defer t.Close()
	return t.IsElevated()
}

// Instalar roda o instalador oficial embutido em modo silencioso. Precisa de
// privilégio de administrador (o processo que chama já deve estar elevado).
func Instalar() error {
	if ok, _ := Instalado(); ok {
		return nil
	}
	if !Elevado() {
		return ErrSemPermissao
	}
	dir, err := os.MkdirTemp("", "bifrost-pawnio")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	exe := filepath.Join(dir, "PawnIO_setup.exe")
	if err := os.WriteFile(exe, setupEXE, 0o700); err != nil {
		return err
	}
	cmd := exec.Command(exe, "-install", "-silent")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	saida, err := cmd.CombinedOutput()
	if err != nil {
		var ee *exec.ExitError
		// 3010 = instalado, pede reinício. Para um driver sob demanda como
		// este não é necessário reiniciar para usar.
		if errors.As(err, &ee) && ee.ExitCode() == 3010 {
			return nil
		}
		msg := strings.TrimSpace(string(saida))
		if msg == "" {
			return fmt.Errorf("o instalador do PawnIO falhou: %w", err)
		}
		return fmt.Errorf("o instalador do PawnIO falhou: %w (%s)", err, msg)
	}
	// O dispositivo aparece logo depois do instalador sair; damos um tempo.
	for i := 0; i < 20; i++ {
		if d, err := abrir(); err == nil {
			d.fechar()
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	if ok, _ := Instalado(); ok {
		return nil
	}
	return errors.New("o driver PawnIO foi instalado mas não respondeu")
}

// Desinstalar remove o driver (usado quando o usuário desliga o recurso).
func Desinstalar() error {
	if ok, _ := Instalado(); !ok {
		return nil
	}
	if !Elevado() {
		return ErrSemPermissao
	}
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, chaveDesinstalar, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return err
	}
	local, _, err := k.GetStringValue("InstallLocation")
	k.Close()
	if err != nil {
		return err
	}
	cmd := exec.Command(filepath.Join(local, "PawnIO_setup.exe"), "-uninstall", "-silent")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}

// --- agente elevado ---------------------------------------------------------

// TarefaAgente é o nome da tarefa agendada que roda o agente elevado (o
// próprio bifrost.exe em modo --sensores; veja cmd/bifrost/sensores_windows.go).
// Fica aqui pra este pacote conseguir religá-la sozinho sem duplicar o nome.
const TarefaAgente = "Bifrost Sensores"

var (
	reiniciarMu sync.Mutex
	reiniciarEm time.Time
)

// GarantirAgenteRodando religa o agente se o driver estiver instalado mas a
// última leitura estiver velha — o processo do agente morreu por algum
// motivo (ex.: alguém encerra todo processo "bifrost.exe" por engano, o que
// também mata o agente por terem o mesmo nome executável). Não precisa de
// elevação: a tarefa agendada já guarda seu próprio nível de privilégio
// (/RL HIGHEST), então o Windows eleva sozinho ao rodá-la, mesmo chamada por
// um processo comum — não pede UAC de novo. Limitado a uma tentativa a cada
// 20s pra não martelar o schtasks.exe a cada amostra de temperatura.
func GarantirAgenteRodando() {
	if ok, _ := Instalado(); !ok {
		return
	}
	if l, err := LerLeitura(); err == nil && l.Fresca() {
		return
	}
	reiniciarMu.Lock()
	if time.Since(reiniciarEm) < 20*time.Second {
		reiniciarMu.Unlock()
		return
	}
	reiniciarEm = time.Now()
	reiniciarMu.Unlock()
	cmd := exec.Command("schtasks.exe", "/Run", "/TN", TarefaAgente)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Run()
}
