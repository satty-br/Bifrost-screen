//go:build windows

package main

import (
	_ "embed"

	"fyne.io/systray"
	"github.com/satty-br/Bifrost-screen/internal/app"
	"github.com/satty-br/Bifrost-screen/internal/config"
	"github.com/satty-br/Bifrost-screen/internal/i18n"
)

//go:embed icon.ico
var iconICO []byte

func runTray(a *app.App, store *config.Store, onQuit func()) {
	systray.Run(func() {
		systray.SetIcon(iconICO)
		systray.SetTitle("Bifrost")

		cfg := store.Get()
		l := cfg.ResolvedLanguage()
		systray.SetTooltip(i18n.T(l, "tray.tooltip"))
		systray.SetOnTapped(openPanel)

		mOpen := systray.AddMenuItem(i18n.T(l, "tray.open_panel"), i18n.T(l, "tray.open_panel_desc"))
		systray.AddSeparator()
		mNext := systray.AddMenuItem(i18n.T(l, "tray.next"), "")
		mPause := systray.AddMenuItemCheckbox(i18n.T(l, "tray.pause"), i18n.T(l, "tray.pause_desc"), false)
		mReconnect := systray.AddMenuItem(i18n.T(l, "tray.reconnect"), i18n.T(l, "tray.reconnect_desc"))
		systray.AddSeparator()
		mQuit := systray.AddMenuItem(i18n.T(l, "tray.quit"), i18n.T(l, "tray.quit_desc"))

		store.OnChange(func(c config.Config) {
			l := c.ResolvedLanguage()
			systray.SetTooltip(i18n.T(l, "tray.tooltip"))
			mOpen.SetTitle(i18n.T(l, "tray.open_panel"))
			mOpen.SetTooltip(i18n.T(l, "tray.open_panel_desc"))
			mNext.SetTitle(i18n.T(l, "tray.next"))
			mPause.SetTitle(i18n.T(l, "tray.pause"))
			mPause.SetTooltip(i18n.T(l, "tray.pause_desc"))
			mReconnect.SetTitle(i18n.T(l, "tray.reconnect"))
			mReconnect.SetTooltip(i18n.T(l, "tray.reconnect_desc"))
			mQuit.SetTitle(i18n.T(l, "tray.quit"))
			mQuit.SetTooltip(i18n.T(l, "tray.quit_desc"))
		})

		go func() {
			for {
				select {
				case <-mOpen.ClickedCh:
					openPanel()
				case <-mNext.ClickedCh:
					a.Next("", 1)
				case <-mPause.ClickedCh:
					if mPause.Checked() {
						mPause.Uncheck()
						a.SetPaused("", false)
					} else {
						mPause.Check()
						a.SetPaused("", true)
					}
				case <-mReconnect.ClickedCh:
					a.Reconnect("")
				case <-mQuit.ClickedCh:
					systray.Quit()
					return
				}
			}
		}()
	}, onQuit)
}
