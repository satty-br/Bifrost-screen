package pawnio

import (
	"math"
	"testing"
	"time"
)

func TestIntelTemp(t *testing.T) {
	// TjMax 100 °C (byte 2 do IA32_TEMPERATURE_TARGET).
	target := uint64(100) << 16
	casos := []struct {
		nome   string
		therm  uint64
		target uint64
		quer   float64
		ok     bool
	}{
		{"38 graus abaixo do limite", 1<<31 | 38<<16, target, 62, true},
		{"no limite", 1<<31 | 0<<16, target, 100, true},
		{"bit de validade desligado", 38 << 16, target, 0, false},
		{"sem TjMax usa 100", 1<<31 | 45<<16, 0, 55, true},
		{"TjMax absurdo cai para 100", 1<<31 | 45<<16, uint64(250) << 16, 55, true},
		{"delta maior que o limite", 1<<31 | 127<<16, uint64(60) << 16, 0, false},
		{"outros bits não atrapalham", 1<<31 | 1<<10 | 40<<16 | 0xFF, target, 60, true},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			got, ok := intelTemp(c.therm, c.target)
			if ok != c.ok {
				t.Fatalf("validade = %v, queria %v", ok, c.ok)
			}
			if ok && math.Abs(got-c.quer) > 0.01 {
				t.Fatalf("temperatura = %.2f, queria %.2f", got, c.quer)
			}
		})
	}
}

func TestZenTemp(t *testing.T) {
	// 480 passos de 0,125 °C = 60 °C, sem faixa estendida.
	raw := uint32(480) << 21
	if got, ok := zenTemp(raw); !ok || math.Abs(got-60) > 0.01 {
		t.Fatalf("temperatura = %.2f (%v), queria 60", got, ok)
	}
	// Mesma leitura com a faixa estendida: 49 °C a menos.
	if got, ok := zenTemp(raw | 0x80000); !ok || math.Abs(got-11) > 0.01 {
		t.Fatalf("faixa estendida = %.2f (%v), queria 11", got, ok)
	}
	// Os dois bits de seleção ligados também valem como faixa estendida.
	if got, ok := zenTemp(raw | 0x30000); !ok || math.Abs(got-11) > 0.01 {
		t.Fatalf("bits de seleção = %.2f (%v), queria 11", got, ok)
	}
	// Leitura zerada (registrador não respondeu) não vira temperatura.
	if _, ok := zenTemp(0); ok {
		t.Fatal("leitura zerada devia ser inválida")
	}
	// Valor fora da faixa física não vira temperatura.
	if _, ok := zenTemp(uint32(2000) << 21); ok {
		t.Fatal("250 °C devia ser inválido")
	}
}

func TestLeituraFresca(t *testing.T) {
	agora := Leitura{CPU: 55, Atualizado: time.Now()}
	if !agora.Fresca() {
		t.Fatal("leitura recente devia valer")
	}
	velha := Leitura{CPU: 55, Atualizado: time.Now().Add(-2 * ValidadeLeitura)}
	if velha.Fresca() {
		t.Fatal("leitura velha não devia valer (o agente pode ter parado)")
	}
	semValor := Leitura{Atualizado: time.Now()}
	if semValor.Fresca() {
		t.Fatal("leitura sem temperatura não devia valer")
	}
}

func TestModulosEmbutidos(t *testing.T) {
	// Formato do blob: 4 bytes com o tamanho da assinatura (512), a assinatura
	// RSA e o corpo AMX. Se o arquivo vier truncado, o driver recusa.
	for nome, blob := range map[string][]byte{
		"IntelMSR":    moduleIntelMSR,
		"AMDFamily17": moduleAMDFamily17,
	} {
		if len(blob) < 600 {
			t.Fatalf("%s: módulo pequeno demais (%d bytes)", nome, len(blob))
		}
		tam := uint32(blob[0]) | uint32(blob[1])<<8 | uint32(blob[2])<<16 | uint32(blob[3])<<24
		if tam != 512 {
			t.Fatalf("%s: tamanho de assinatura inesperado: %d", nome, tam)
		}
	}
	if len(setupEXE) < 1<<20 || string(setupEXE[:2]) != "MZ" {
		t.Fatalf("instalador embutido inválido (%d bytes)", len(setupEXE))
	}
}
