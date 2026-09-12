package winutil

import "testing"

func TestFindConflicts(t *testing.T) {
	// Só verifica que roda sem erro e devolve um formato usável; não há como
	// garantir presença/ausência dos processos conhecidos na máquina de teste.
	conf := FindConflicts()
	for _, c := range conf {
		if c.Name == "" {
			t.Error("conflito sem nome de processo")
		}
		if c.About == "" {
			t.Error("conflito sem descrição")
		}
	}
}

func TestListProcesses(t *testing.T) {
	procs, err := ListProcesses()
	if err != nil {
		t.Fatalf("ListProcesses() falhou: %v", err)
	}
	if len(procs) == 0 {
		t.Error("deveria haver pelo menos um processo rodando (o próprio teste)")
	}
	// PID 0 é legítimo (System Idle Process no Windows), então só confere os nomes.
	for _, p := range procs {
		if p.Name == "" {
			t.Error("processo sem nome")
		}
	}
}

func TestKillElevatedNoPids(t *testing.T) {
	// Sem PIDs não deve tentar elevar nem pedir UAC.
	if err := KillElevated(nil); err != nil {
		t.Errorf("KillElevated(nil) não deveria falhar: %v", err)
	}
	if err := KillElevated([]uint32{}); err != nil {
		t.Errorf("KillElevated([]) não deveria falhar: %v", err)
	}
}

func TestNamedMutex(t *testing.T) {
	name := `Local\BifrostTestMutex`
	if !NamedMutex(name) {
		t.Error("primeira criação do mutex deveria devolver true")
	}
	if NamedMutex(name) {
		t.Error("segunda criação do mesmo mutex deveria devolver false (já existe)")
	}
}

func TestFocusWindowNotFound(t *testing.T) {
	if FocusWindow("Título que certamente não existe em nenhuma janela 123456") {
		t.Error("não deveria encontrar uma janela com esse título")
	}
}

func TestAutostartEnabledDoesNotPanic(t *testing.T) {
	_ = AutostartEnabled()
}

func TestSingleInstance(t *testing.T) {
	// Só confere que a chamada não trava/erra; o valor depende do ambiente.
	_ = SingleInstance()
}
