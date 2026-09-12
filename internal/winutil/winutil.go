// Package winutil reúne integrações com o Windows (processos, registro, atalhos).
package winutil

import "strings"

// Process é um processo em execução.
type Process struct {
	Name string `json:"nome"`
	PID  uint32 `json:"pid"`
}

// Conflict é um programa que costuma prender a porta da tela.
type Conflict struct {
	Process
	Sure  bool   `json:"certeza"`
	About string `json:"descricao"`
}

// knownConflicts são os programas conhecidos por usar a tela Turing/UsbMonitor.
var knownConflicts = []struct {
	name  string
	sure  bool
	about string
}{
	{"usbmonitor.exe", true, "App oficial da tela (UsbMonitor)"},
	{"sendtemp.exe", false, "Serviço que acompanha o UsbMonitor"},
	{"cpu server.exe", false, "Servidor de sensores usado pelo UsbMonitor"},
	{"turing.exe", true, "App oficial Turing Smart Screen"},
	{"turzx.exe", true, "App oficial TURZX"},
	{"extendscreen.exe", true, "App oficial da tela"},
}

// FindConflicts procura, nos processos em execução, programas que prendem a porta.
func FindConflicts() []Conflict {
	procs, err := ListProcesses()
	if err != nil {
		return nil
	}
	var out []Conflict
	for _, p := range procs {
		n := strings.ToLower(p.Name)
		for _, k := range knownConflicts {
			if n == k.name {
				out = append(out, Conflict{Process: p, Sure: k.sure, About: k.about})
			}
		}
	}
	return out
}
