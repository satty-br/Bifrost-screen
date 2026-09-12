//go:build windows

package i18n

import "golang.org/x/sys/windows"

var (
	kernel32                     = windows.NewLazySystemDLL("kernel32.dll")
	procGetUserDefaultUILanguage = kernel32.NewProc("GetUserDefaultUILanguage")
)

// Detect lê o idioma da interface configurado no Windows.
func Detect() Lang {
	r, _, _ := procGetUserDefaultUILanguage.Call()
	// LANGID: os 10 bits menos significativos são o "primary language".
	switch r & 0x3ff {
	case 0x09:
		return EN
	case 0x0a:
		return ES
	case 0x11:
		return JA
	case 0x04:
		return ZH
	case 0x16:
		return PT
	}
	return EN
}
