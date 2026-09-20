// kalkanprobe é a ferramenta de diagnóstico da tela LCD dos water coolers
// GAMDIAS/Kalkan. Ela não depende do resto do Bifrost: é um executável só,
// feito pra quem tem o cooler rodar e mandar a saída de volta.
//
// Uso:
//
//	kalkan-probe                 lista as telas encontradas
//	kalkan-probe -todos          lista TODOS os dispositivos HID do PC
//	kalkan-probe -conectar       tenta o handshake e mostra a resposta crua
//	kalkan-probe -imagem         manda um quadro de teste pra tela
//	kalkan-probe -brilho 40      ajusta o brilho
//	kalkan-probe -log saida.txt  grava tudo num arquivo
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/satty-br/Bifrost-screen/internal/kalkan"
)

var saida io.Writer = os.Stdout

func linha(f string, a ...any) { fmt.Fprintf(saida, f+"\n", a...) }

func main() {
	todos := flag.Bool("todos", false, "lista todos os dispositivos HID, não só os da família GAMDIAS")
	conectar := flag.Bool("conectar", false, "abre a tela e faz o handshake (POST conn)")
	imagem := flag.Bool("imagem", false, "manda um quadro de teste pra tela (implica -conectar)")
	brilho := flag.Int("brilho", -1, "ajusta o brilho, 0-100 (implica -conectar)")
	caminho := flag.String("caminho", "", "usa este caminho de dispositivo em vez de escolher sozinho")
	arquivoLog := flag.String("log", "", "também grava a saída neste arquivo")
	flag.Parse()

	if *arquivoLog != "" {
		f, err := os.Create(*arquivoLog)
		if err != nil {
			fmt.Fprintln(os.Stderr, "não consegui criar o log:", err)
			os.Exit(1)
		}
		defer f.Close()
		saida = io.MultiWriter(os.Stdout, f)
	}

	linha("kalkan-probe — diagnóstico da tela LCD GAMDIAS/Kalkan")
	linha("sistema: %s/%s   data: %s", runtime.GOOS, runtime.GOARCH, time.Now().Format(time.RFC3339))
	linha("procurando VID_%04X", kalkan.VendorID)
	linha("")

	if err := listar(*todos); err != nil {
		linha("ERRO ao listar dispositivos: %v", err)
		os.Exit(1)
	}

	if *imagem || *brilho >= 0 {
		*conectar = true
	}
	if !*conectar {
		linha("")
		linha("Nada mais a fazer. Para tentar falar com a tela: kalkan-probe -conectar")
		return
	}
	if err := sessao(*caminho, *brilho, *imagem); err != nil {
		linha("")
		linha("ERRO: %v", err)
		linha("")
		linha("Se o erro for de acesso negado, feche o app da Kalkan (KK.exe) pelo")
		linha("Gerenciador de Tarefas e rode de novo — ele segura o dispositivo aberto.")
		os.Exit(1)
	}
}

func listar(todos bool) error {
	devs, err := kalkan.Devices()
	if err != nil {
		return err
	}
	if len(devs) == 0 {
		linha("Nenhum dispositivo com VID_%04X encontrado.", kalkan.VendorID)
		if !todos {
			linha("")
			linha("Se a tela está ligada por USB, rode com -todos: pode ser que ela use")
			linha("outro VID, e aí a saída completa é exatamente o que precisamos ver.")
			return nil
		}
	}
	for i, d := range devs {
		linha("[%d] %s", i, d)
		linha("     %s", d.Path)
		if d.KnownProduct {
			linha("     modelo conhecido: %s, %dx%d a %d fps",
				d.Product.Code, d.Product.Width, d.Product.Height, d.Product.FPS)
		} else {
			linha("     PID %04X ainda não está na tabela — mande esta linha na issue", d.ProductID)
		}
	}
	if todos {
		linha("")
		linha("--- todos os dispositivos HID ---")
		tudo, err := kalkan.AllDevices()
		if err != nil {
			return err
		}
		for _, d := range tudo {
			marca := "  "
			if d.VendorID == kalkan.VendorID {
				marca = "->"
			}
			linha("%s VID_%04X&PID_%04X mi_%-2s usage=%04X:%04X in=%-3d out=%-3d feat=%-3d",
				marca, d.VendorID, d.ProductID, ouTraco(d.Interface),
				d.UsagePage, d.Usage, d.InputReportLen, d.OutputReportLen, d.FeatureReportLen)
		}
		linha("total: %d dispositivos HID", len(tudo))
	}
	return nil
}

func ouTraco(s string) string {
	if s == "" {
		return "--"
	}
	return s
}

func sessao(caminho string, brilho int, comImagem bool) error {
	var c *kalkan.Client
	var err error
	if caminho != "" {
		devs, e := kalkan.Devices()
		if e != nil {
			return e
		}
		var escolhido *kalkan.DeviceInfo
		for i := range devs {
			if strings.EqualFold(devs[i].Path, caminho) {
				escolhido = &devs[i]
			}
		}
		if escolhido == nil {
			return fmt.Errorf("caminho %q não está entre os dispositivos da família", caminho)
		}
		t, e := kalkan.OpenPath(escolhido.Path)
		if e != nil {
			return e
		}
		c = kalkan.NewClient(t, escolhido.Product)
		linha("")
		linha("abrindo %s ...", escolhido.Path)
		if e := c.Conn(); e != nil {
			t.Close()
			return e
		}
	} else {
		linha("")
		linha("abrindo a primeira tela conhecida e mandando POST conn ...")
		c, err = kalkan.Open()
		if err != nil {
			return err
		}
	}
	defer c.Close()

	p := c.Product()
	linha("HANDSHAKE OK — %s (%s), %dx%d", p.Name, p.OEM, p.Width, p.Height)
	linha("")
	linha("Se você chegou até aqui, o protocolo está certo. Mande esta saída na issue.")

	if brilho >= 0 {
		linha("")
		linha("mandando POST brightness %d ...", brilho)
		if err := c.Brightness(brilho); err != nil {
			return err
		}
		linha("brilho OK")
	}

	if comImagem {
		linha("")
		linha("mandando um quadro de teste %dx%d ...", p.Width, p.Height)
		if err := c.SendImage(quadroTeste(p.Width, p.Height), 85); err != nil {
			return err
		}
		linha("quadro enviado. Olhe a tela do cooler: se apareceram faixas coloridas")
		linha("com uma cruz branca no meio, a transferência de imagem funciona.")
	}
	return nil
}

// quadroTeste desenha faixas de cor e uma cruz central — fácil de reconhecer
// e revela na hora se a ordem dos canais ou a orientação estão erradas.
func quadroTeste(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	faixas := []color.RGBA{
		{255, 0, 0, 255}, {0, 255, 0, 255}, {0, 0, 255, 255},
		{255, 255, 0, 255}, {0, 255, 255, 255}, {255, 0, 255, 255},
		{255, 255, 255, 255}, {20, 20, 20, 255},
	}
	largura := w / len(faixas)
	if largura < 1 {
		largura = 1
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := x / largura
			if i >= len(faixas) {
				i = len(faixas) - 1
			}
			img.Set(x, y, faixas[i])
		}
	}
	// cruz branca no centro: mostra a orientação
	for x := 0; x < w; x++ {
		img.Set(x, h/2, color.RGBA{255, 255, 255, 255})
	}
	for y := 0; y < h; y++ {
		img.Set(w/2, y, color.RGBA{255, 255, 255, 255})
	}
	// quadrado vermelho no canto superior esquerdo: mostra se está espelhado
	for y := 0; y < h/10; y++ {
		for x := 0; x < w/10; x++ {
			img.Set(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	return img
}
