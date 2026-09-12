//go:build windows

package sysinfo

import "testing"

func TestIsVirtualNIC(t *testing.T) {
	cases := map[string]bool{
		"Ethernet":                          false,
		"Wi-Fi":                             false,
		"Software Loopback Interface 1":     true,
		"Microsoft ISATAP Adapter":          true,
		"Teredo Tunneling Pseudo-Interface": true,
		"Hyper-V Virtual Ethernet Adapter":  true,
		"vEthernet (Default Switch)":        true,
		"VirtualBox Host-Only Network":      true,
		"WAN Miniport (IP)":                 true,
		"Cisco AnyConnect VPN Adapter":      true,
	}
	for name, want := range cases {
		if got := isVirtualNIC(name); got != want {
			t.Errorf("isVirtualNIC(%q) = %v, esperava %v", name, got, want)
		}
	}
}

func TestClamp100(t *testing.T) {
	cases := []struct{ in, want float64 }{
		{-1, -1},
		{0, 0},
		{50, 50},
		{100, 100},
		{150, 100},
	}
	for _, c := range cases {
		if got := clamp100(c.in); got != c.want {
			t.Errorf("clamp100(%v) = %v, esperava %v", c.in, got, c.want)
		}
	}
}

func TestParseHWSensors(t *testing.T) {
	data := []byte(`[
		{"Name":"CPU Total","Parent":"/amdcpu/0/load","Value":42.0},
		{"Name":"Core (Tctl/Tdie)","Parent":"/amdcpu/0","Value":58.5},
		{"Name":"Package","Parent":"/amdcpu/0","Value":55.0},
		{"Name":"GPU Core","Parent":"/gpu-amd/5","Value":64.2},
		{"Name":"GPU Hot Spot","Parent":"/gpu-amd/5","Value":72.0},
		{"Name":"Temperature","Parent":"/nvme/0","Value":40.0},
		{"Name":"Stale Sensor","Parent":"/amdcpu/0","Value":0}
	]`)
	cpu, gpu := parseHWSensors(data)
	if cpu != 58.5 {
		t.Errorf("cpu = %v, esperava o Tctl/Tdie (58.5)", cpu)
	}
	if gpu != 64.2 {
		t.Errorf("gpu = %v, esperava o GPU Core (64.2)", gpu)
	}
}

func TestParseHWSensorsSingleObject(t *testing.T) {
	// ConvertTo-Json devolve um objeto solto (não array) quando só há 1 sensor.
	data := []byte(`{"Name":"Package","Parent":"/intelcpu/0","Value":47.0}`)
	cpu, gpu := parseHWSensors(data)
	if cpu != 47.0 {
		t.Errorf("cpu = %v, esperava 47.0", cpu)
	}
	if gpu != -1 {
		t.Errorf("gpu = %v, esperava -1 (sem sensor de GPU)", gpu)
	}
}

func TestParseHWSensorsEmptyOrInvalid(t *testing.T) {
	cpu, gpu := parseHWSensors([]byte(``))
	if cpu != -1 || gpu != -1 {
		t.Errorf("entrada vazia deveria devolver -1,-1, veio %v,%v", cpu, gpu)
	}
	cpu, gpu = parseHWSensors([]byte(`não é json`))
	if cpu != -1 || gpu != -1 {
		t.Errorf("json inválido deveria devolver -1,-1, veio %v,%v", cpu, gpu)
	}
}
