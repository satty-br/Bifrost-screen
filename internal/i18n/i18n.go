// Package i18n traduz os textos do Bifrost (tela LCD, painel web, bandeja).
package i18n

import "fmt"

// Lang é um idioma suportado pelo Bifrost.
type Lang string

const (
	PT Lang = "pt" // padrão histórico (Brasil)
	EN Lang = "en" // idioma padrão do projeto e fallback
	ES Lang = "es"
	JA Lang = "ja"
	ZH Lang = "zh" // mandarim (chinês simplificado)
)

// Supported lista os idiomas, na ordem mostrada no painel.
var Supported = []Lang{EN, PT, ES, JA, ZH}

// IsSupported diz se l é um dos idiomas que o Bifrost sabe mostrar.
func IsSupported(l Lang) bool {
	for _, s := range Supported {
		if s == l {
			return true
		}
	}
	return false
}

// Name devolve o nome do idioma, no próprio idioma (para o seletor do painel).
func Name(l Lang) string {
	switch l {
	case PT:
		return "Português"
	case ES:
		return "Español"
	case JA:
		return "日本語"
	case ZH:
		return "中文"
	default:
		return "English"
	}
}

// Resolve escolhe o idioma efetivo a partir da preferência salva ("auto", "" ou um código).
// "auto" (ou vazio) detecta o idioma do Windows; se não for suportado, cai para o inglês.
func Resolve(pref string) Lang {
	if l := Lang(pref); pref != "auto" && IsSupported(l) {
		return l
	}
	if d := Detect(); IsSupported(d) {
		return d
	}
	return EN
}

// T traduz key para l, com fallback para inglês e, por último, a própria key.
func T(l Lang, key string, args ...any) string {
	row, ok := catalog[key]
	if !ok {
		return key
	}
	s, ok := row[l]
	if !ok {
		s, ok = row[EN]
	}
	if !ok {
		return key
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}
