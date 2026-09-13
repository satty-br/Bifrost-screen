//go:build windows

package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/satty-br/Bifrost-screen/internal/pawnio"
	"github.com/satty-br/Bifrost-screen/internal/winutil"
)

// A leitura de temperatura precisa falar com o driver PawnIO, e o driver só
// aceita processos administradores. Em vez de exigir UAC toda vez que o
// Bifrost abre, instalamos uma tarefa agendada que roda este mesmo executável
// em modo agente (--sensores) com privilégio máximo. O agente publica a
// temperatura num arquivo em %ProgramData%\Bifrost, que o Bifrost do usuário
// lê sem precisar de permissão nenhuma.
const tarefaSensores = pawnio.TarefaAgente

const intervaloAgente = 2 * time.Second

// runSensorAgent é o modo --sensores: lê a temperatura e publica, até ser encerrado.
func runSensorAgent() {
	if !winutil.NamedMutex(`Local\BifrostSensores`) {
		return // já tem um agente rodando
	}
	leitor, err := pawnio.NovoLeitor()
	if err != nil {
		_ = pawnio.GravarLeitura(pawnio.Leitura{Erro: err.Error(), Atualizado: time.Now()})
		return
	}
	defer leitor.Fechar()

	parar := make(chan os.Signal, 1)
	signal.Notify(parar, os.Interrupt, syscall.SIGTERM)
	fonte := leitor.Fonte()
	for {
		l := pawnio.Leitura{Fonte: fonte, Atualizado: time.Now()}
		if t, err := leitor.TemperaturaCPU(); err != nil {
			l.Erro = err.Error()
		} else {
			l.CPU = t
		}
		_ = pawnio.GravarLeitura(l)
		select {
		case <-parar:
			return
		case <-time.After(intervaloAgente):
		}
	}
}

// installSensors é o modo --instalar-sensores, sempre executado elevado: põe o
// driver na máquina e deixa o agente ligado agora e nos próximos logons.
func installSensors() error {
	if !pawnio.Elevado() {
		return errors.New("esta etapa precisa de permissão de administrador")
	}
	if err := pawnio.Instalar(); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	// /RL HIGHEST: roda com o privilégio máximo do usuário, sem pedir UAC.
	// /SC ONLOGON: começa junto com a sessão. /F: substitui se já existir.
	if out, err := schtasks("/Create", "/TN", tarefaSensores,
		"/TR", `"`+exe+`" --sensores`, "/SC", "ONLOGON", "/RL", "HIGHEST", "/F"); err != nil {
		return fmt.Errorf("criando a tarefa do agente: %w (%s)", err, out)
	}
	if out, err := schtasks("/Run", "/TN", tarefaSensores); err != nil {
		return fmt.Errorf("iniciando o agente: %w (%s)", err, out)
	}
	// Espera a primeira leitura aparecer para poder confirmar no painel.
	for i := 0; i < 40; i++ {
		if l, err := pawnio.LerLeitura(); err == nil && l.Fresca() {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	if l, err := pawnio.LerLeitura(); err == nil && l.Erro != "" {
		return errors.New(l.Erro)
	}
	return errors.New("o agente foi instalado mas ainda não publicou uma leitura")
}

// removeSensors é o modo --remover-sensores: desliga o agente e tira o driver.
func removeSensors() error {
	if !pawnio.Elevado() {
		return errors.New("esta etapa precisa de permissão de administrador")
	}
	_, _ = schtasks("/End", "/TN", tarefaSensores)
	_, _ = schtasks("/Delete", "/TN", tarefaSensores, "/F")
	_ = os.Remove(pawnio.ArquivoLeitura())
	return pawnio.Desinstalar()
}

func schtasks(args ...string) (string, error) {
	cmd := exec.Command("schtasks.exe", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// runSensorSetup executa a instalação/remoção e guarda o resultado num arquivo,
// porque este processo roda separado (elevado) e não tem como responder ao painel.
func runSensorSetup(remover bool) {
	var err error
	if remover {
		err = removeSensors()
	} else {
		err = installSensors()
	}
	msg := "ok"
	if err != nil {
		msg = err.Error()
	}
	_ = os.MkdirAll(pawnio.PastaDados(), 0o755)
	_ = os.WriteFile(pawnio.PastaDados()+`\instalacao.txt`,
		[]byte(time.Now().Format(time.RFC3339)+" "+msg+"\n"), 0o644)
	if err != nil {
		log.Printf("sensores: %v", err)
		winutil.Alert("Bifrost", "Não consegui ativar a leitura de temperatura:\n\n"+err.Error())
	}
}
