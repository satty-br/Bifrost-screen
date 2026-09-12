//go:build !windows

package i18n

import (
	"os"
	"strings"
)

// Detect lê o idioma do ambiente (LC_ALL/LANG/LANGUAGE), para testar fora do Windows.
func Detect() Lang {
	v := os.Getenv("LC_ALL")
	if v == "" {
		v = os.Getenv("LANG")
	}
	if v == "" {
		v = os.Getenv("LANGUAGE")
	}
	v = strings.ToLower(v)
	switch {
	case strings.HasPrefix(v, "pt"):
		return PT
	case strings.HasPrefix(v, "es"):
		return ES
	case strings.HasPrefix(v, "ja"):
		return JA
	case strings.HasPrefix(v, "zh"):
		return ZH
	case strings.HasPrefix(v, "en"):
		return EN
	}
	return EN
}
