package rap

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ParseText parses plain-text config.cpp/hpp syntax into an AST root class.
// Preprocessor directives (#include, #define ...) are skipped with a warning.
func ParseText(src string) (*Class, []string, error) {
	lx := &lexer{src: src}
	lx.run()
	if lx.err != nil {
		return nil, lx.warnings, lx.err
	}
	p := &parser{toks: lx.toks, warnings: lx.warnings}
	root := &Class{}
	if err := p.parseBody(root, true); err != nil {
		return nil, p.warnings, err
	}
	return root, p.warnings, nil
}

type tokKind int

const (
	tIdent tokKind = iota
	tString
	tNumber
	tPunct // { } ; : , = [ ] + (as "+=" pair handled in parser)
	tEOF
)

type token struct {
	kind tokKind
	text string
	f    float64
	i    int64
	isInt bool
	line int
}

type lexer struct {
	src      string
	pos      int
	line     int
	toks     []token
	warnings []string
	err      error
}

func (l *lexer) peek() byte {
	if l.pos < len(l.src) {
		return l.src[l.pos]
	}
	return 0
}

func (l *lexer) at(off int) byte {
	if l.pos+off < len(l.src) {
		return l.src[l.pos+off]
	}
	return 0
}

func isIdentChar(c byte) bool {
	return c == '_' || c == '.' || c == '\\' || c == '/' ||
		(c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}

func (l *lexer) run() {
	l.line = 1
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		switch {
		case c == '\n':
			l.line++
			l.pos++
		case c == ' ' || c == '\t' || c == '\r':
			l.pos++
		case c == '/' && l.at(1) == '/':
			for l.pos < len(l.src) && l.src[l.pos] != '\n' {
				l.pos++
			}
		case c == '/' && l.at(1) == '*':
			l.pos += 2
			for l.pos < len(l.src) && !(l.src[l.pos] == '*' && l.at(1) == '/') {
				if l.src[l.pos] == '\n' {
					l.line++
				}
				l.pos++
			}
			l.pos += 2
		case c == '#':
			start := l.pos
			for l.pos < len(l.src) && l.src[l.pos] != '\n' {
				// handle line continuation in macros
				if l.src[l.pos] == '\\' && l.at(1) == '\n' {
					l.pos += 2
					l.line++
					continue
				}
				l.pos++
			}
			directive := strings.TrimSpace(l.src[start:min(l.pos, start+60)])
			l.warnings = append(l.warnings, fmt.Sprintf("строка %d: директива препроцессора пропущена: %s", l.line, directive))
		case c == '"':
			l.lexString()
		case c == '-' || c == '+' || (c >= '0' && c <= '9') ||
			(c == '.' && l.at(1) >= '0' && l.at(1) <= '9'):
			// try number; if it fails, fall back to ident
			if !l.lexNumber() {
				l.lexIdent()
			}
		case isIdentChar(c) || c == '$':
			l.lexIdent()
		case strings.IndexByte("{};:,=[]+", c) >= 0:
			l.toks = append(l.toks, token{kind: tPunct, text: string(c), line: l.line})
			l.pos++
		default:
			// unknown char — skip with warning
			l.warnings = append(l.warnings, fmt.Sprintf("строка %d: пропущен неожиданный символ %q", l.line, string(c)))
			l.pos++
		}
	}
	l.toks = append(l.toks, token{kind: tEOF, line: l.line})
}

func (l *lexer) lexString() {
	l.pos++ // opening quote
	var sb strings.Builder
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		if c == '"' {
			if l.at(1) == '"' { // doubled quote escape
				sb.WriteByte('"')
				l.pos += 2
				continue
			}
			l.pos++
			l.toks = append(l.toks, token{kind: tString, text: sb.String(), line: l.line})
			return
		}
		if c == '\n' {
			l.line++
		}
		sb.WriteByte(c)
		l.pos++
	}
	l.err = fmt.Errorf("строка %d: незакрытая строка", l.line)
}

func (l *lexer) lexNumber() bool {
	start := l.pos
	p := l.pos
	if l.src[p] == '-' || l.src[p] == '+' {
		p++
	}
	// hex
	if p+1 < len(l.src) && l.src[p] == '0' && (l.src[p+1] == 'x' || l.src[p+1] == 'X') {
		p += 2
		h := p
		for p < len(l.src) && isHex(l.src[p]) {
			p++
		}
		if p > h && (p >= len(l.src) || !isIdentChar(l.src[p])) {
			v, err := strconv.ParseInt(l.src[start:p], 0, 64)
			if err == nil {
				l.toks = append(l.toks, token{kind: tNumber, i: v, isInt: true, line: l.line})
				l.pos = p
				return true
			}
		}
		return false
	}
	digits := 0
	for p < len(l.src) && l.src[p] >= '0' && l.src[p] <= '9' {
		p++
		digits++
	}
	isFloat := false
	if p < len(l.src) && l.src[p] == '.' {
		isFloat = true
		p++
		for p < len(l.src) && l.src[p] >= '0' && l.src[p] <= '9' {
			p++
			digits++
		}
	}
	if digits == 0 {
		return false
	}
	if p < len(l.src) && (l.src[p] == 'e' || l.src[p] == 'E') {
		q := p + 1
		if q < len(l.src) && (l.src[q] == '-' || l.src[q] == '+') {
			q++
		}
		ed := 0
		for q < len(l.src) && l.src[q] >= '0' && l.src[q] <= '9' {
			q++
			ed++
		}
		if ed > 0 {
			isFloat = true
			p = q
		}
	}
	if p < len(l.src) && isIdentChar(l.src[p]) {
		return false // e.g. 7Rnd_... identifier
	}
	text := l.src[start:p]
	if isFloat {
		f, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return false
		}
		l.toks = append(l.toks, token{kind: tNumber, f: f, line: l.line})
	} else {
		v, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			f, ferr := strconv.ParseFloat(text, 64)
			if ferr != nil {
				return false
			}
			l.toks = append(l.toks, token{kind: tNumber, f: f, line: l.line})
		} else {
			l.toks = append(l.toks, token{kind: tNumber, i: v, isInt: true, line: l.line})
		}
	}
	l.pos = p
	return true
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func (l *lexer) lexIdent() {
	start := l.pos
	for l.pos < len(l.src) && (isIdentChar(l.src[l.pos]) || l.src[l.pos] == '$' || l.src[l.pos] == '-') {
		l.pos++
	}
	if l.pos == start {
		l.pos++ // safety
	}
	l.toks = append(l.toks, token{kind: tIdent, text: l.src[start:l.pos], line: l.line})
}

// ---------- parser ----------

type parser struct {
	toks     []token
	pos      int
	warnings []string
}

func (p *parser) cur() token  { return p.toks[p.pos] }
func (p *parser) next() token { t := p.toks[p.pos]; p.pos++; return t }

func (p *parser) accept(text string) bool {
	if p.cur().kind == tPunct && p.cur().text == text {
		p.pos++
		return true
	}
	return false
}

func (p *parser) expect(text string) error {
	if !p.accept(text) {
		return fmt.Errorf("строка %d: ожидался %q, найден %q", p.cur().line, text, p.cur().text)
	}
	return nil
}

func (p *parser) parseBody(cls *Class, topLevel bool) error {
	for {
		t := p.cur()
		if t.kind == tEOF {
			if !topLevel {
				return errors.New("неожиданный конец файла внутри класса")
			}
			return nil
		}
		if t.kind == tPunct && t.text == "}" {
			if topLevel {
				p.warnings = append(p.warnings, fmt.Sprintf("строка %d: лишняя закрывающая скобка", t.line))
				p.pos++
				continue
			}
			return nil
		}
		if t.kind == tPunct && t.text == ";" {
			p.pos++
			continue
		}
		if t.kind == tIdent && strings.EqualFold(t.text, "class") {
			if err := p.parseClass(cls); err != nil {
				return err
			}
			continue
		}
		if t.kind == tIdent && strings.EqualFold(t.text, "delete") {
			p.pos++
			nameTok := p.next()
			if nameTok.kind != tIdent {
				return fmt.Errorf("строка %d: ожидалось имя после delete", nameTok.line)
			}
			p.accept(";")
			cls.Entries = append(cls.Entries, &Entry{Kind: KindDelete, Name: nameTok.text})
			continue
		}
		if t.kind == tIdent && strings.EqualFold(t.text, "enum") {
			// enum { ... }; — skip block
			p.pos++
			if p.accept("{") {
				depth := 1
				for depth > 0 && p.cur().kind != tEOF {
					tt := p.next()
					if tt.kind == tPunct && tt.text == "{" {
						depth++
					}
					if tt.kind == tPunct && tt.text == "}" {
						depth--
					}
				}
			}
			p.accept(";")
			continue
		}
		if t.kind == tIdent {
			if err := p.parseAssignment(cls); err != nil {
				return err
			}
			continue
		}
		return fmt.Errorf("строка %d: неожиданный токен %q", t.line, t.text)
	}
}

func (p *parser) parseClass(cls *Class) error {
	p.pos++ // 'class'
	nameTok := p.next()
	if nameTok.kind != tIdent {
		return fmt.Errorf("строка %d: ожидалось имя класса", nameTok.line)
	}
	child := &Class{Name: nameTok.text}
	if p.accept(":") {
		parTok := p.next()
		if parTok.kind != tIdent {
			return fmt.Errorf("строка %d: ожидалось имя родителя", parTok.line)
		}
		child.Parent = parTok.text
	}
	if p.accept("{") {
		if err := p.parseBody(child, false); err != nil {
			return err
		}
		if err := p.expect("}"); err != nil {
			return err
		}
		p.accept(";")
		cls.Entries = append(cls.Entries, &Entry{Kind: KindClass, Name: child.Name, Class: child})
		return nil
	}
	// extern declaration: class Name;
	p.accept(";")
	if child.Parent != "" {
		// "class A : B;" — treat as empty class def
		cls.Entries = append(cls.Entries, &Entry{Kind: KindClass, Name: child.Name, Class: child})
	} else {
		cls.Entries = append(cls.Entries, &Entry{Kind: KindExtern, Name: child.Name})
	}
	return nil
}

func (p *parser) parseAssignment(cls *Class) error {
	nameTok := p.next()
	name := nameTok.text
	isArray := false
	if p.accept("[") {
		if err := p.expect("]"); err != nil {
			return err
		}
		isArray = true
	}
	var flags uint32
	if p.accept("+") {
		if err := p.expect("="); err != nil {
			return err
		}
		flags = 1
	} else if err := p.expect("="); err != nil {
		return err
	}
	if isArray {
		arr, err := p.parseArrayLit()
		if err != nil {
			return err
		}
		p.accept(";")
		cls.Entries = append(cls.Entries, &Entry{Kind: KindArray, Name: name, Array: arr, ArrayFlags: flags})
		return nil
	}
	v, err := p.parseScalar()
	if err != nil {
		return err
	}
	p.accept(";")
	cls.Entries = append(cls.Entries, &Entry{Kind: KindValue, Name: name, Value: v})
	return nil
}

func (p *parser) parseScalar() (*Value, error) {
	t := p.next()
	switch t.kind {
	case tString:
		// concatenated strings: "a" "b"? rare — merge
		s := t.text
		for p.cur().kind == tString {
			s += p.next().text
		}
		return &Value{Kind: VString, Str: s}, nil
	case tNumber:
		if t.isInt {
			return &Value{Kind: VInt, I: t.i}, nil
		}
		return &Value{Kind: VFloat, F: t.f}, nil
	case tIdent:
		return &Value{Kind: VVariable, Str: t.text}, nil
	}
	return nil, fmt.Errorf("строка %d: ожидалось значение, найден %q", t.line, t.text)
}

func (p *parser) parseArrayLit() ([]*Value, error) {
	if err := p.expect("{"); err != nil {
		return nil, err
	}
	var out []*Value
	for {
		if p.accept("}") {
			return out, nil
		}
		if p.cur().kind == tPunct && p.cur().text == "{" {
			sub, err := p.parseArrayLit()
			if err != nil {
				return nil, err
			}
			out = append(out, &Value{Kind: VArray, Arr: sub})
		} else {
			v, err := p.parseScalar()
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		if p.accept(",") {
			continue
		}
		if p.accept("}") {
			return out, nil
		}
		if p.cur().kind == tEOF {
			return nil, errors.New("незакрытый массив")
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
