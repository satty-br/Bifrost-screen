package sysinfo

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Nomes das fontes de temperatura (aparecem no painel e no diagnóstico).
const (
	SourceACPI        = "ACPI (Windows)"
	SourcePawnIOAgent = "PawnIO (agente do Bifrost)"
	SourceHWiNFO      = "HWiNFO"
	SourceLHMWeb      = "LibreHardwareMonitor (web)"
	SourceLHMWMI      = "LibreHardwareMonitor (WMI)"
	SourceAfterburner = "MSI Afterburner"
	SourceAIDA64      = "AIDA64"
	SourceNvidiaSMI   = "nvidia-smi"
	SourceNVML        = "NVIDIA (nvml.dll)"
	SourceADL         = "AMD (atiadlxx.dll)"
	SourceProc        = "/sys (Linux)"
	SourceSMC         = "SMC (macOS)"
)

// Faixa aceitável para uma leitura de temperatura fazer sentido.
const (
	tempMin = 5.0
	tempMax = 125.0
)

func validTemp(v float64) bool { return v >= tempMin && v <= tempMax }

// PawnIOStatus resume, para o painel, em que pé está a leitura por driver
// (o agente e o driver só existem no Windows; ver TempDriverStatus).
type PawnIOStatus struct {
	Installed bool    `json:"driver_instalado"`
	Version   string  `json:"driver_versao,omitempty"`
	Agent     bool    `json:"agente_ativo"`
	CPU       float64 `json:"cpu_temperatura"`
	Error     string  `json:"erro,omitempty"`
}

// parseTempNumber entende "45", "45.5", "45,5" e "45,5 °C".
func parseTempNumber(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	// corta a unidade e qualquer coisa depois dela
	if i := strings.IndexAny(s, "°CcFf"); i > 0 {
		s = strings.TrimSpace(s[:i])
	}
	s = strings.ReplaceAll(s, ",", ".")
	s = strings.TrimRight(s, ". ")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// scoreCPUName dá nota para o nome de um sensor de CPU: quanto maior, mais
// representativo da temperatura "do processador" (cada fabricante usa um nome).
func scoreCPUName(name string) int {
	n := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.Contains(n, "tctl/tdie"), n == "cpu package", n == "package", n == "cpu temperature":
		return 5
	case strings.Contains(n, "die (average)"), strings.Contains(n, "cpu die"):
		return 4
	case strings.Contains(n, "ccd"), strings.Contains(n, "core max"), strings.Contains(n, "core average"):
		return 3
	case strings.HasPrefix(n, "core "), n == "cpu", strings.Contains(n, "cpu socket"):
		return 2
	case strings.Contains(n, "cpu"):
		return 1
	}
	return 0
}

func scoreGPUName(name string) int {
	n := strings.ToLower(strings.TrimSpace(name))
	switch {
	case n == "gpu core", n == "gpu temperature", n == "gpu":
		return 5
	case strings.Contains(n, "edge"):
		return 4
	case strings.Contains(n, "hot spot"), strings.Contains(n, "hotspot"), strings.Contains(n, "junction"):
		return 3
	case strings.Contains(n, "gpu"):
		return 1
	}
	return 0
}

// picker escolhe a melhor leitura de CPU e de GPU entre vários candidatos.
type picker struct {
	cpu, gpu           float64
	cpuScore, gpuScore int
}

func newPicker() *picker { return &picker{cpu: -1, gpu: -1} }

func (p *picker) offerCPU(name string, v float64) {
	if !validTemp(v) {
		return
	}
	if s := scoreCPUName(name); s > p.cpuScore {
		p.cpu, p.cpuScore = v, s
	}
}

func (p *picker) offerGPU(name string, v float64) {
	if !validTemp(v) {
		return
	}
	if s := scoreGPUName(name); s > p.gpuScore {
		p.gpu, p.gpuScore = v, s
	}
}

func (p *picker) result() (cpu, gpu float64) { return p.cpu, p.gpu }

// isCPUContext/isGPUContext olham o "caminho" do sensor (o aparelho a que ele
// pertence) para saber se a leitura é de CPU ou de GPU.
func isCPUContext(path string) bool {
	p := strings.ToLower(path)
	for _, k := range []string{"cpu", "ryzen", "intel core", "core i", "threadripper", "athlon", "xeon", "amdcpu", "intelcpu", "processor"} {
		if strings.Contains(p, k) {
			return true
		}
	}
	return false
}

func isGPUContext(path string) bool {
	p := strings.ToLower(path)
	for _, k := range []string{"gpu", "radeon", "geforce", "nvidia", "rtx", "gtx", "arc a"} {
		if strings.Contains(p, k) {
			return true
		}
	}
	return false
}

// --- HWiNFO (valores publicados no registro do Windows) -------------------

// hwinfoEntry é uma entrada do "Gadget" do HWiNFO: sensor, rótulo e valor.
type hwinfoEntry struct {
	Sensor   string
	Label    string
	Value    string // com unidade, ex: "45,0 °C"
	ValueRaw string // só o número
}

func parseHWiNFO(entries []hwinfoEntry) (cpu, gpu float64) {
	p := newPicker()
	for _, e := range entries {
		// Só interessam as entradas em graus: o Gadget publica também clocks,
		// tensões e uso, que viriam com outras unidades.
		if !strings.Contains(e.Value, "°") && !strings.Contains(strings.ToLower(e.Value), "c") {
			continue
		}
		raw := e.ValueRaw
		if raw == "" {
			raw = e.Value
		}
		v, ok := parseTempNumber(raw)
		if !ok {
			continue
		}
		ctx := e.Sensor + " " + e.Label
		if isGPUContext(ctx) {
			p.offerGPU(e.Label, v)
			continue
		}
		if isCPUContext(ctx) {
			p.offerCPU(e.Label, v)
		}
	}
	return p.result()
}

// --- LibreHardwareMonitor / OpenHardwareMonitor (servidor web) ------------

// lhmNode é um nó da árvore devolvida em /data.json.
type lhmNode struct {
	Text     string    `json:"Text"`
	Value    string    `json:"Value"`
	Children []lhmNode `json:"Children"`
}

func parseLHMTree(data []byte) (cpu, gpu float64) {
	var root lhmNode
	if err := json.Unmarshal(data, &root); err != nil {
		return -1, -1
	}
	p := newPicker()
	var walk func(n lhmNode, path string)
	walk = func(n lhmNode, path string) {
		full := path + " / " + n.Text
		if len(n.Children) == 0 {
			if v, ok := parseTempNumber(n.Value); ok && strings.Contains(n.Value, "°") {
				switch {
				case isGPUContext(path):
					p.offerGPU(n.Text, v)
				case isCPUContext(path):
					p.offerCPU(n.Text, v)
				}
			}
			return
		}
		for _, c := range n.Children {
			walk(c, full)
		}
	}
	walk(root, "")
	return p.result()
}

// --- AIDA64 (memória compartilhada, formato tipo XML) ---------------------

func parseAIDA64(text string) (cpu, gpu float64) {
	p := newPicker()
	for _, block := range strings.Split(text, "</temp>") {
		id := between(block, "<id>", "</id>")
		label := between(block, "<label>", "</label>")
		value := between(block, "<value>", "</value>")
		if id == "" || value == "" {
			continue
		}
		v, ok := parseTempNumber(value)
		if !ok {
			continue
		}
		name := label
		if name == "" {
			name = id
		}
		switch {
		case strings.HasPrefix(id, "TGPU"), isGPUContext(id + " " + label):
			p.offerGPU(name, v)
		case strings.HasPrefix(id, "TCPU"), strings.HasPrefix(id, "TCC"), isCPUContext(id + " " + label):
			p.offerCPU(name, v)
		}
	}
	return p.result()
}

func between(s, open, close string) string {
	i := strings.Index(s, open)
	if i < 0 {
		return ""
	}
	s = s[i+len(open):]
	j := strings.Index(s, close)
	if j < 0 {
		return ""
	}
	return strings.TrimSpace(s[:j])
}

// --- MSI Afterburner (memória compartilhada) ------------------------------

// parseAfterburnerEntry recebe o nome e o valor de uma entrada da memória
// compartilhada do Afterburner e classifica a leitura.
func parseAfterburnerEntries(names []string, values []float64) (cpu, gpu float64) {
	p := newPicker()
	for i, name := range names {
		if i >= len(values) {
			break
		}
		n := strings.ToLower(name)
		if !strings.Contains(n, "temperature") && !strings.Contains(n, "temp") {
			continue
		}
		switch {
		case strings.HasPrefix(n, "gpu"):
			p.offerGPU("gpu core", values[i])
		case strings.HasPrefix(n, "cpu"):
			p.offerCPU("cpu package", values[i])
		}
	}
	return p.result()
}

// TempSourceStatus descreve uma fonte de temperatura e o que ela respondeu.
type TempSourceStatus struct {
	Name      string  `json:"fonte"`
	Available bool    `json:"disponivel"`
	CPU       float64 `json:"cpu"`
	GPU       float64 `json:"gpu"`
	Hint      string  `json:"dica,omitempty"`
}

// Dicas mostradas quando nenhuma fonte respondeu.
const (
	hintHWiNFO      = "instale o HWiNFO e marque \"Report value in Gadget\" na temperatura da CPU"
	hintLHMWeb      = "no LibreHardwareMonitor, ligue Options > Remote Web Server > Run"
	hintLHMWMI      = "abra o LibreHardwareMonitor como administrador (ele publica os sensores no WMI)"
	hintAfterburner = "abra o MSI Afterburner (ele publica os sensores em memória compartilhada)"
	hintAIDA64      = "no AIDA64, ligue Preferences > External Applications > Shared Memory"
	hintACPI        = "depende da placa-mãe expor zona térmica ACPI; a maioria dos desktops não expõe"
	hintPawnIOAgent = "ative a leitura de temperatura no painel: o Bifrost instala o driver PawnIO (assinado, open-source) e deixa um agente publicando a leitura"
	hintNVML        = "vem junto com o driver NVIDIA (nvml.dll); só responde em máquina com placa NVIDIA"
	hintADL         = "vem junto com o driver AMD Adrenalin (atiadlxx.dll); só responde em máquina com placa AMD"
	hintNvidiaSMI   = "reserva para placas NVIDIA quando a nvml.dll não está no caminho padrão"
)

// parseNvidiaSMI lê a saída de `nvidia-smi --query-gpu=temperature.gpu
// --format=csv,noheader,nounits`: um número por linha, uma linha por GPU.
// Placas sem sensor respondem "N/A" — nesse caso seguimos para a próxima linha.
func parseNvidiaSMI(out string) (float64, bool) {
	for _, linha := range strings.Split(out, "\n") {
		if v, ok := parseTempNumber(strings.TrimSpace(linha)); ok && validTemp(v) {
			return v, true
		}
	}
	return 0, false
}
