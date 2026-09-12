//go:build windows

package sysinfo

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Os tamanhos têm que bater exatamente com as structs nativas do Windows
// (UNICODE_STRING, OBJECT_ATTRIBUTES, IO_STATUS_BLOCK) — um tamanho errado
// faz o NtOpenFile/NtDeviceIoControlFile ler memória fora da struct.
func TestPawnIOStructSizes(t *testing.T) {
	if got := unsafe.Sizeof(unicodeString{}); got != 16 {
		t.Errorf("sizeof(unicodeString) = %d, queria 16", got)
	}
	if got := unsafe.Sizeof(objectAttributes{}); got != 48 {
		t.Errorf("sizeof(objectAttributes) = %d, queria 48", got)
	}
	if got := unsafe.Sizeof(ioStatusBlock{}); got != 16 {
		t.Errorf("sizeof(ioStatusBlock) = %d, queria 16", got)
	}
}

// Confere a aritmética de CTL_CODE contra os valores calculados à mão pela
// fórmula do Windows: (DeviceType << 16) | (Access << 14) | (Function << 2) | Method.
func TestPawnIOCtlCode(t *testing.T) {
	want := uint32(41394)<<16 | 0x821<<2
	if ioctlPioLoadBinary != want {
		t.Errorf("IOCTL_PIO_LOAD_BINARY = 0x%08X, queria 0x%08X", ioctlPioLoadBinary, want)
	}
	want = uint32(41394)<<16 | 0x841<<2
	if ioctlPioExecuteFn != want {
		t.Errorf("IOCTL_PIO_EXECUTE_FN = 0x%08X, queria 0x%08X", ioctlPioExecuteFn, want)
	}
}

// Sem o driver instalado (o normal nesta máquina de teste), abrir o
// dispositivo tem que falhar de forma limpa, sem travar nem pausar o programa.
func TestOpenPawnIODeviceGracefulFailure(t *testing.T) {
	h, err := openPawnIODevice()
	if err == nil {
		windows.CloseHandle(h)
		t.Skip("driver PawnIO está instalado nesta máquina; nada a conferir aqui")
	}
}

// readPawnIO nunca deve travar nem entrar em pane mesmo sem o driver.
func TestReadPawnIONoDriver(t *testing.T) {
	cpu, gpu := readPawnIO()
	if cpu < 0 && gpu != -1 {
		t.Errorf("sem CPU válida, gpu deveria ser -1, veio %v", gpu)
	}
}

// Fórmula AMD (k10temp): raw>>21 em unidades de 0.125°C, bit 19 marca -49°C.
func TestAMDTctlFormula(t *testing.T) {
	cases := []struct {
		raw  uint64
		want float64
	}{
		{400 << 21, 50.0},
		{(400 << 21) | (1 << 19), 1.0}, // 50 - 49
	}
	for _, c := range cases {
		milliC := float64(c.raw>>21) * 125.0
		if c.raw&amdSMNTempRangeSelBit != 0 {
			milliC -= 49000
		}
		got := milliC / 1000.0
		if got != c.want {
			t.Errorf("Tctl(0x%X) = %v, queria %v", c.raw, got, c.want)
		}
	}
}

// Fórmula Intel: TjMax (bits 23:16 de MSR_IA32_TEMPERATURE_TARGET) menos o
// "digital readout" (bits 22:16 de MSR_IA32_(PACKAGE_)THERM_STATUS).
func TestIntelThermFormula(t *testing.T) {
	target := uint64(100) << 16 // TjMax = 100
	status := uint64(1)<<31 | uint64(25)<<16 // válido, 25° abaixo do TjMax
	tjmax := (target >> 16) & 0xFF
	readout := (status >> 16) & 0x7F
	valid := status&(1<<31) != 0
	if !valid {
		t.Fatal("deveria ser válido")
	}
	if got := float64(tjmax) - float64(readout); got != 75 {
		t.Errorf("temp = %v, queria 75", got)
	}
}
