// Package pawnio lê a temperatura da CPU no Windows através do PawnIO, um
// driver de kernel open-source e assinado (https://github.com/namazso/PawnIO).
//
// Por que um driver: no Windows a temperatura do processador só existe em
// registradores do próprio chip — MSR 0x19C/0x1B1 na Intel, registrador SMN
// 0x59800 nos Ryzen. Ler isso exige modo kernel; não há API, WMI nem contador
// de desempenho que entregue o valor. Todo programa de monitoramento instala
// um driver para isso.
//
// O PawnIO é o driver que o LibreHardwareMonitor passou a usar em 2025 no
// lugar do WinRing0 (bloqueado pelo Defender por dar acesso irrestrito ao
// hardware). Ele só carrega módulos assinados, e cada módulo expõe uma lista
// branca de registradores — o IntelMSR, por exemplo, só deixa ler os MSRs de
// temperatura, energia e frequência.
//
// Os módulos embutidos aqui (IntelMSR.bin, AMDFamily17.bin) vêm sem
// modificação de https://github.com/namazso/PawnIO.Modules (LGPL-2.1-or-later,
// licença em blobs/COPYING-PawnIO-Modules.txt), e o instalador
// PawnIO_setup.exe de https://github.com/namazso/PawnIO.Setup.
package pawnio

import (
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

//go:embed blobs/IntelMSR.bin
var moduleIntelMSR []byte

//go:embed blobs/AMDFamily17.bin
var moduleAMDFamily17 []byte

//go:embed blobs/PawnIO_setup.exe
var setupEXE []byte

// ModuloEmbutido devolve, pelo nome de arquivo usado no projeto PawnIO.Modules,
// o módulo assinado que vem junto com o Bifrost — assim nada precisa ser
// baixado em tempo de execução.
func ModuloEmbutido(nome string) []byte {
	switch nome {
	case "IntelMSR.bin":
		return moduleIntelMSR
	case "AMDFamily17.bin":
		return moduleAMDFamily17
	}
	return nil
}

// Registradores lidos (nomes conforme os manuais Intel/AMD).
const (
	msrThermStatus        = 0x19C // IA32_THERM_STATUS (núcleo)
	msrTemperatureTarget  = 0x1A2 // IA32_TEMPERATURE_TARGET (TjMax)
	msrPackageThermStatus = 0x1B1 // IA32_PACKAGE_THERM_STATUS (pacote)

	smnZenCurTmp = 0x00059800 // THM_TCON_CURTMP (Zen, famílias 17h-1Ah)
)

// intelTemp converte IA32_THERM_STATUS + IA32_TEMPERATURE_TARGET em °C.
// A CPU não informa a temperatura direta: informa quantos graus faltam para o
// limite térmico (TjMax), no campo de 7 bits a partir do bit 16. O bit 31 diz
// se a leitura é válida.
func intelTemp(therm, target uint64) (float64, bool) {
	if therm&(1<<31) == 0 {
		return 0, false
	}
	delta := float64((therm >> 16) & 0x7F)
	tjmax := float64((target >> 16) & 0xFF)
	if tjmax < 50 || tjmax > 130 {
		tjmax = 100 // valor de fábrica quando o MSR não traz o limite
	}
	t := tjmax - delta
	if t < 0 || t > 130 {
		return 0, false
	}
	return t, true
}

// zenTemp converte o registrador SMN de temperatura dos Ryzen em °C (Tctl).
// O valor tem 11 bits a partir do bit 21, em passos de 0,125 °C; quando a
// faixa estendida está ligada, subtrai-se o deslocamento de 49 °C.
func zenTemp(raw uint32) (float64, bool) {
	t := float64(raw>>21) * 0.125
	if raw&0x80000 != 0 || raw&0x30000 == 0x30000 {
		t -= 49
	}
	if t <= 0 || t > 130 {
		return 0, false
	}
	return t, true
}

// Leitura é o que o agente de sensores publica para o Bifrost ler sem
// privilégio de administrador.
type Leitura struct {
	CPU        float64   `json:"cpu"`
	Fonte      string    `json:"fonte"`
	Erro       string    `json:"erro,omitempty"`
	Atualizado time.Time `json:"atualizado"`
}

// Válida enquanto for recente: se o agente parar, o painel volta a dizer que
// não tem temperatura em vez de mostrar um número congelado.
const ValidadeLeitura = 30 * time.Second

func (l Leitura) Fresca() bool {
	return l.CPU > 0 && time.Since(l.Atualizado) < ValidadeLeitura
}

// PastaDados é onde o agente (que roda como administrador) deixa a leitura
// para o Bifrost do usuário ler.
func PastaDados() string {
	base := os.Getenv("ProgramData")
	if base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "Bifrost")
}

func ArquivoLeitura() string { return filepath.Join(PastaDados(), "sensores.json") }

func GravarLeitura(l Leitura) error {
	if err := os.MkdirAll(PastaDados(), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(l)
	if err != nil {
		return err
	}
	tmp := ArquivoLeitura() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, ArquivoLeitura())
}

func LerLeitura() (Leitura, error) {
	var l Leitura
	data, err := os.ReadFile(ArquivoLeitura())
	if err != nil {
		return l, err
	}
	err = json.Unmarshal(data, &l)
	return l, err
}
