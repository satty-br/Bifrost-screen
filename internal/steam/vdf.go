package steam

import (
	"errors"
	"strings"
)

// VDF é um nó do formato KeyValues da Valve (usado nos .vdf/.acf da Steam).
// As chaves são guardadas em minúsculas, porque a Steam não é consistente nas maiúsculas.
type VDF struct {
	Value    string
	Children map[string]*VDF
}

// Get navega pelos filhos: v.Get("UserLocalConfigStore", "friends", "PersonaName").
func (v *VDF) Get(path ...string) *VDF {
	cur := v
	for _, p := range path {
		if cur == nil || cur.Children == nil {
			return nil
		}
		cur = cur.Children[strings.ToLower(p)]
	}
	return cur
}

// String devolve o valor do caminho (ou "").
func (v *VDF) String(path ...string) string {
	if n := v.Get(path...); n != nil {
		return n.Value
	}
	return ""
}

// ParseVDF lê o texto de um arquivo .vdf/.acf.
func ParseVDF(data string) (*VDF, error) {
	p := &vdfParser{s: data}
	root := &VDF{Children: map[string]*VDF{}}
	if err := p.parseBody(root, false); err != nil {
		return nil, err
	}
	return root, nil
}

type vdfParser struct {
	s   string
	pos int
}

func (p *vdfParser) skip() {
	for p.pos < len(p.s) {
		c := p.s[p.pos]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			p.pos++
		case c == '/' && p.pos+1 < len(p.s) && p.s[p.pos+1] == '/':
			for p.pos < len(p.s) && p.s[p.pos] != '\n' {
				p.pos++
			}
		case c == '[': // condicionais tipo [$WIN32] são ignoradas
			for p.pos < len(p.s) && p.s[p.pos] != ']' {
				p.pos++
			}
			p.pos++
		default:
			return
		}
	}
}

func (p *vdfParser) token() (string, bool, error) {
	p.skip()
	if p.pos >= len(p.s) {
		return "", false, nil
	}
	c := p.s[p.pos]
	if c == '{' || c == '}' {
		p.pos++
		return string(c), true, nil
	}
	if c == '"' {
		p.pos++
		var b strings.Builder
		for p.pos < len(p.s) {
			c := p.s[p.pos]
			if c == '\\' && p.pos+1 < len(p.s) {
				n := p.s[p.pos+1]
				switch n {
				case 'n':
					b.WriteByte('\n')
				case 't':
					b.WriteByte('\t')
				default:
					b.WriteByte(n)
				}
				p.pos += 2
				continue
			}
			if c == '"' {
				p.pos++
				return b.String(), false, nil
			}
			b.WriteByte(c)
			p.pos++
		}
		return "", false, errors.New("vdf: aspas sem fechar")
	}
	start := p.pos
	for p.pos < len(p.s) && !strings.ContainsRune(" \t\r\n{}\"", rune(p.s[p.pos])) {
		p.pos++
	}
	return p.s[start:p.pos], false, nil
}

func (p *vdfParser) parseBody(node *VDF, nested bool) error {
	for {
		key, brace, err := p.token()
		if err != nil {
			return err
		}
		if brace && key == "}" {
			if !nested {
				return errors.New("vdf: '}' sobrando")
			}
			return nil
		}
		if key == "" && !brace {
			if nested {
				return errors.New("vdf: fim inesperado")
			}
			return nil
		}
		val, brace, err := p.token()
		if err != nil {
			return err
		}
		child := &VDF{}
		if brace && val == "{" {
			child.Children = map[string]*VDF{}
			if err := p.parseBody(child, true); err != nil {
				return err
			}
		} else {
			child.Value = val
		}
		node.Children[strings.ToLower(key)] = child
	}
}
