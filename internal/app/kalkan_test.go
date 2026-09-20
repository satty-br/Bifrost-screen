package app

import (
	"strings"
	"testing"

	"github.com/satty-br/Bifrost-screen/internal/config"
	"github.com/satty-br/Bifrost-screen/internal/kalkan"
)

// A tela do water cooler é detectada sozinha: ela entra e sai da lista de
// dispositivos conforme o painel aparece na varredura HID, sem nada disso ir
// parar no config.json.
func TestDeviceListDetectaKalkan(t *testing.T) {
	base := config.Default()
	base.Devices = []config.DeviceConfig{{ID: "principal", Revision: config.RevA}}
	base.Kalkan.Brightness = 55
	base.Kalkan.Orientation = config.OrientLandscape

	casos := []struct {
		nome       string
		presente   bool
		habilitado bool
		quer       bool
	}{
		{"plugado e habilitado entra na lista", true, true, true},
		{"plugado mas desabilitado fica de fora", true, false, false},
		{"habilitado mas sem hardware fica de fora", false, true, false},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			a := &App{kalkanScan: func() (kalkan.Product, bool) { return kalkan.Products[0], c.presente }, devices: map[string]*device{}}
			cfg := base
			cfg.Kalkan.Enabled = c.habilitado
			lista := a.deviceList(cfg)

			var achado *config.DeviceConfig
			for i := range lista {
				if lista[i].ID == kalkanDeviceID {
					achado = &lista[i]
				}
			}
			if c.quer && achado == nil {
				t.Fatalf("esperava o dispositivo %q na lista, veio %d dispositivos", kalkanDeviceID, len(lista))
			}
			if !c.quer {
				if achado != nil {
					t.Fatal("não esperava o dispositivo do water cooler na lista")
				}
				return
			}
			if achado.Revision != config.RevKalkan {
				t.Fatalf("revisão = %q, esperava %q", achado.Revision, config.RevKalkan)
			}
			if achado.Brightness != 55 || achado.Orientation != config.OrientLandscape {
				t.Fatalf("o dispositivo sintético não herdou a configuração: %+v", *achado)
			}
			if !strings.Contains(achado.Name, kalkan.Products[0].Name) {
				t.Fatalf("o nome %q não traz o modelo detectado", achado.Name)
			}
			if len(lista) != len(cfg.Devices)+1 {
				t.Fatalf("lista tem %d, esperava %d", len(lista), len(cfg.Devices)+1)
			}
		})
	}
}

// Se o usuário já tiver um dispositivo com esse ID, não duplicamos nem
// sobrescrevemos o dele.
func TestDeviceListNaoDuplicaIDDoUsuario(t *testing.T) {
	cfg := config.Default()
	cfg.Kalkan.Enabled = true
	cfg.Devices = []config.DeviceConfig{{ID: kalkanDeviceID, Revision: config.RevA, Brightness: 7}}
	a := &App{kalkanScan: func() (kalkan.Product, bool) { return kalkan.Products[0], true }, devices: map[string]*device{}}

	lista := a.deviceList(cfg)
	if len(lista) != 1 {
		t.Fatalf("lista tem %d dispositivos, esperava 1", len(lista))
	}
	if lista[0].Revision != config.RevA || lista[0].Brightness != 7 {
		t.Fatalf("o dispositivo do usuário foi alterado: %+v", lista[0])
	}
}

// A lista original do usuário não pode ser modificada ao acrescentarmos o
// dispositivo sintético (senão o append vazaria pro config em memória).
func TestDeviceListNaoMexeNaFatiaOriginal(t *testing.T) {
	cfg := config.Default()
	cfg.Kalkan.Enabled = true
	cfg.Devices = []config.DeviceConfig{{ID: "principal", Revision: config.RevA}}
	a := &App{kalkanScan: func() (kalkan.Product, bool) { return kalkan.Products[0], true }, devices: map[string]*device{}}

	_ = a.deviceList(cfg)
	if len(cfg.Devices) != 1 {
		t.Fatalf("cfg.Devices mudou: %d dispositivos", len(cfg.Devices))
	}
}
