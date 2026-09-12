// genres gera cmd/bifrost/rsrc_windows_amd64.syso (ícone, manifesto e versão do .exe).
//
// Uso: go run ./tools/genres 1.0.0
package main

import (
	"image"
	_ "image/png"
	"log"
	"os"

	"github.com/tc-hib/winres"
	"github.com/tc-hib/winres/version"
)

func main() {
	ver := "0.0.0"
	if len(os.Args) > 1 {
		ver = os.Args[1]
	}
	f, err := os.Open("assets/icon.png")
	if err != nil {
		log.Fatal(err)
	}
	img, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		log.Fatal(err)
	}
	icon, err := winres.NewIconFromResizedImage(img, []int{256, 128, 64, 48, 32, 24, 16})
	if err != nil {
		log.Fatal(err)
	}
	rs := winres.ResourceSet{}
	// ID 1: usado pela janela do painel (WindowOptions.IconId) e pelo Explorer.
	if err := rs.SetIcon(winres.ID(1), icon); err != nil {
		log.Fatal(err)
	}
	vi := version.Info{}
	vi.SetFileVersion(ver)
	vi.SetProductVersion(ver)
	vi.Set(0, version.ProductName, "Bifrost")
	vi.Set(0, version.FileDescription, "Bifrost - painel para a tela USB Turing/UsbMonitor")
	vi.Set(0, version.CompanyName, "satty-br")
	vi.Set(0, version.LegalCopyright, "GPL-3.0-or-later")
	vi.Set(0, version.OriginalFilename, "bifrost.exe")
	rs.SetVersionInfo(vi)
	rs.SetManifest(winres.AppManifest{
		Description:         "Bifrost",
		ExecutionLevel:      winres.AsInvoker,
		DPIAwareness:        winres.DPIPerMonitorV2,
		UseCommonControlsV6: true,
	})
	out, err := os.Create("cmd/bifrost/rsrc_windows_amd64.syso")
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()
	if err := rs.WriteObject(out, winres.ArchAMD64); err != nil {
		log.Fatal(err)
	}
	log.Printf("recursos gerados (versão %s)", ver)
}
