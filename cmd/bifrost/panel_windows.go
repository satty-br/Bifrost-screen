//go:build windows

package main

import (
	"path/filepath"

	webview2 "github.com/jchv/go-webview2"
	"github.com/satty-br/Bifrost-screen/internal/winutil"
)

// runPanel mostra o painel numa janela própria (WebView2, que já vem no Windows 11).
// Se o WebView2 não estiver disponível, abre no navegador padrão.
func runPanel(url, dir string) {
	if !winutil.NamedMutex(`Local\BifrostPanel`) {
		winutil.FocusWindow(panelTitle)
		return
	}
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		AutoFocus: true,
		DataPath:  filepath.Join(dir, "webview"),
		WindowOptions: webview2.WindowOptions{
			Title:  panelTitle,
			Width:  1180,
			Height: 800,
			IconId: 1,
			Center: true,
		},
	})
	if w == nil {
		_ = winutil.OpenURL(url)
		return
	}
	defer w.Destroy()
	w.SetSize(1180, 800, webview2.HintNone)
	w.Navigate(url)
	w.Run()
}
