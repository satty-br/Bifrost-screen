// Package kalkan fala com as telas LCD de water cooler da família GAMDIAS
// ZeusCast — vendidas no Brasil pela Kalkan como "Aura LCD", e por outras
// marcas sob nomes diferentes.
//
// EXPERIMENTAL: este pacote foi escrito a partir de engenharia reversa
// estática do software do fabricante (KK.exe / ZeusCast, .NET), SEM acesso ao
// hardware. A identificação do aparelho e o formato das mensagens estão
// documentados em docs/kalkan-aura-lcd.md. Os pontos ainda não confirmados
// estão marcados com "NÃO CONFIRMADO" ao longo do código — são exatamente os
// que a ferramenta tools/kalkanprobe serve pra descobrir.
//
// Resumo do que se sabe:
//
//   - O painel é um dispositivo HID (interface de um aparelho USB composto),
//     VID 0x1B80. O PID identifica o modelo.
//   - Os relatórios são de 64 bytes ("HidTransport64X" no software original).
//   - O protocolo é texto no estilo HTTP: uma linha de requisição, cabeçalhos
//     Key=Valor, linha em branco e um corpo JSON. O aparelho responde
//     "200\r\n" ou "400\r\n".
//   - As imagens vão em JPEG, em blocos, entre um "POST transport" e um
//     "POST transported".
package kalkan

import "errors"

// VendorID é o fabricante do controlador usado por toda a família (GAMDIAS).
const VendorID uint16 = 0x1B80

// reportSize é quantos bytes de dado cabem num relatório HID desta família.
// O software do fabricante chama seu transporte de "HidTransport64X", e os
// aparelhos declaram 64 bytes de payload (65 com o byte do Report ID).
const reportSize = 64

// ErrNotFound é devolvido quando nenhum painel conhecido está conectado.
var ErrNotFound = errors.New("nenhuma tela Kalkan/GAMDIAS encontrada")

// Product descreve um modelo de painel: como o software do fabricante o
// identifica e qual a geometria nativa da tela.
type Product struct {
	PID    uint16 // Product ID USB
	Code   string // código de 4 dígitos hex usado pelo firmware e pelo site do fabricante
	Name   string // nome do modelo na tabela do fabricante
	OEM    string // nome com que a marca revende o mesmo painel
	Width  int
	Height int
	FPS    int
}

// Products lista os modelos cuja geometria foi lida da LcdProductTable do
// software do fabricante. Outros PIDs da mesma família aparecem no binário
// (B522 B526 B533 B534 B538 B53A B53B B53C B53D B540 B541 B542 B548 B54B B54C
// B54D B54F) mas sem resolução confirmada, então ficam de fora até alguém com
// o aparelho confirmar.
var Products = []Product{
	// O Kalkan Aura LCD 240 (KLK00102) e 360 (KLK00103) são, quase com certeza,
	// este: é o único PID cujo nome OEM no binário é literalmente
	// "KALKAN LCD Display", e 320x240 bate com o painel de 2,4" anunciado.
	// NÃO CONFIRMADO em hardware.
	{PID: 0xB550, Code: "B550", Name: "AURA Lite II LCD Display", OEM: "KALKAN LCD Display", Width: 320, Height: 240, FPS: 30},
	{PID: 0xB54E, Code: "B54E", Name: "CHIONE LCD 4 Display", OEM: "AURA LITE LCD", Width: 320, Height: 240, FPS: 30},
	{PID: 0xB547, Code: "B547", Name: "HeliosP2", OEM: "AURA LCD Display", Width: 480, Height: 480, FPS: 30},
	{PID: 0xB551, Code: "B551", Name: "AI LCD Display", OEM: "AURA KK LCD Display", Width: 480, Height: 480, FPS: 30},
}

// Lookup acha o modelo pelo Product ID.
func Lookup(pid uint16) (Product, bool) {
	for _, p := range Products {
		if p.PID == pid {
			return p, true
		}
	}
	return Product{}, false
}

// Known diz se o PID é de um modelo com geometria conhecida.
func Known(pid uint16) bool {
	_, ok := Lookup(pid)
	return ok
}
