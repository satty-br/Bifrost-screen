package sysinfo

import "testing"

func TestParseTempNumber(t *testing.T) {
	cases := map[string]float64{
		"45":       45,
		"45.5":     45.5,
		"45,5":     45.5,
		"45,0 °C":  45,
		" 61.2 °C": 61.2,
		"78 C":     78,
	}
	for in, want := range cases {
		got, ok := parseTempNumber(in)
		if !ok || got != want {
			t.Errorf("parseTempNumber(%q) = %v,%v; queria %v", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "n/a", "°C", "abc"} {
		if v, ok := parseTempNumber(in); ok {
			t.Errorf("parseTempNumber(%q) devia falhar, deu %v", in, v)
		}
	}
}

// Formato do "Gadget" do HWiNFO publicado no registro.
func TestParseHWiNFO(t *testing.T) {
	entries := []hwinfoEntry{
		{Sensor: "CPU [#0]: AMD Ryzen 7 5800X", Label: "CPU (Tctl/Tdie)", Value: "62,4 °C", ValueRaw: "62,375"},
		{Sensor: "CPU [#0]: AMD Ryzen 7 5800X", Label: "Core 0", Value: "58,0 °C", ValueRaw: "58,0"},
		{Sensor: "CPU [#0]: AMD Ryzen 7 5800X", Label: "CPU Package Power", Value: "88,0 W", ValueRaw: "88,0"},
		{Sensor: "GPU [#0]: AMD Radeon RX 6700 XT", Label: "GPU Temperature", Value: "54,0 °C", ValueRaw: "54,0"},
		{Sensor: "GPU [#0]: AMD Radeon RX 6700 XT", Label: "GPU Hot Spot", Value: "71,0 °C", ValueRaw: "71,0"},
	}
	cpu, gpu := parseHWiNFO(entries)
	if cpu != 62.375 {
		t.Errorf("cpu = %v, queria 62.375 (Tctl/Tdie tem prioridade)", cpu)
	}
	if gpu != 54 {
		t.Errorf("gpu = %v, queria 54 (temperatura principal, não o hot spot)", gpu)
	}
	if c, g := parseHWiNFO(nil); c != -1 || g != -1 {
		t.Errorf("sem entradas deveria dar -1/-1, deu %v/%v", c, g)
	}
	// Potência não pode virar temperatura.
	only := []hwinfoEntry{{Sensor: "CPU [#0]", Label: "CPU Package Power", Value: "88,0 W", ValueRaw: "88,0"}}
	if c, _ := parseHWiNFO(only); c != -1 {
		t.Errorf("watts não é temperatura: %v", c)
	}
}

// Árvore /data.json do LibreHardwareMonitor / OpenHardwareMonitor.
const lhmJSON = `{"id":0,"Text":"Sensor","Children":[
 {"id":1,"Text":"DESKTOP","Children":[
  {"id":2,"Text":"AMD Ryzen 7 5800X","Children":[
    {"id":3,"Text":"Temperatures","Children":[
      {"id":4,"Text":"Core (Tctl/Tdie)","Value":"63,5 °C","Children":[]},
      {"id":5,"Text":"Core (Tdie)","Value":"61,0 °C","Children":[]}]},
    {"id":6,"Text":"Load","Children":[{"id":7,"Text":"CPU Total","Value":"12,5 %","Children":[]}]}]},
  {"id":8,"Text":"AMD Radeon RX 6700 XT","Children":[
    {"id":9,"Text":"Temperatures","Children":[
      {"id":10,"Text":"GPU Core","Value":"49,0 °C","Children":[]},
      {"id":11,"Text":"GPU Hot Spot","Value":"66,0 °C","Children":[]}]}]},
  {"id":12,"Text":"Samsung SSD 980","Children":[
    {"id":13,"Text":"Temperatures","Children":[{"id":14,"Text":"Temperature","Value":"41,0 °C","Children":[]}]}]}]}]}`

func TestParseLHMTree(t *testing.T) {
	cpu, gpu := parseLHMTree([]byte(lhmJSON))
	if cpu != 63.5 {
		t.Errorf("cpu = %v, queria 63.5", cpu)
	}
	if gpu != 49 {
		t.Errorf("gpu = %v, queria 49", gpu)
	}
	if c, g := parseLHMTree([]byte("não é json")); c != -1 || g != -1 {
		t.Errorf("json inválido deveria dar -1/-1, deu %v/%v", c, g)
	}
	// O SSD tem "Temperature" mas não é CPU nem GPU: não pode ser escolhido.
	const ssdOnly = `{"Text":"Sensor","Children":[{"Text":"Samsung SSD 980","Children":[
		{"Text":"Temperatures","Children":[{"Text":"Temperature","Value":"41,0 °C","Children":[]}]}]}]}`
	if c, g := parseLHMTree([]byte(ssdOnly)); c != -1 || g != -1 {
		t.Errorf("só SSD deveria dar -1/-1, deu %v/%v", c, g)
	}
}

func TestParseAIDA64(t *testing.T) {
	const shm = `<sys><id>SCPUCLK</id><label>CPU Clock</label><value>4200</value></sys>` +
		`<temp><id>TCPU</id><label>CPU</label><value>57</value></temp>` +
		`<temp><id>TCPUPKG</id><label>CPU Package</label><value>59</value></temp>` +
		`<temp><id>TGPU1</id><label>GPU Diode</label><value>48</value></temp>` +
		`<temp><id>THDD1</id><label>WDC WD10</label><value>36</value></temp>`
	cpu, gpu := parseAIDA64(shm)
	if cpu != 59 {
		t.Errorf("cpu = %v, queria 59 (CPU Package)", cpu)
	}
	if gpu != 48 {
		t.Errorf("gpu = %v, queria 48", gpu)
	}
	if c, g := parseAIDA64(""); c != -1 || g != -1 {
		t.Errorf("vazio deveria dar -1/-1, deu %v/%v", c, g)
	}
}

func TestParseAfterburnerEntries(t *testing.T) {
	names := []string{"GPU temperature", "GPU usage", "CPU temperature", "CPU usage", "RAM usage"}
	values := []float64{52, 97, 64, 31, 12000}
	cpu, gpu := parseAfterburnerEntries(names, values)
	if cpu != 64 || gpu != 52 {
		t.Errorf("cpu/gpu = %v/%v, queria 64/52", cpu, gpu)
	}
	// Leituras fora de faixa (sensor ausente costuma vir 0 ou negativo) são ignoradas.
	if c, g := parseAfterburnerEntries([]string{"CPU temperature", "GPU temperature"}, []float64{0, -1}); c != -1 || g != -1 {
		t.Errorf("valores inválidos deveriam dar -1/-1, deu %v/%v", c, g)
	}
	// Menos valores que nomes não pode causar pânico.
	parseAfterburnerEntries([]string{"CPU temperature", "GPU temperature"}, []float64{55})
}

func TestContextos(t *testing.T) {
	if !isCPUContext("CPU [#0]: AMD Ryzen 7 5800X") || !isGPUContext("AMD Radeon RX 6700 XT") {
		t.Error("classificação de contexto falhou")
	}
	if isCPUContext("Samsung SSD 980") || isGPUContext("Samsung SSD 980") {
		t.Error("SSD não é CPU nem GPU")
	}
}
