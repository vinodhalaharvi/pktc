package sexp

import "strings"

// Read parses every top-level form in src. file is used only in
// diagnostics and may be empty.
func Read(file, src string) ([]Value, error) {
	r := &reader{src: src, file: file, line: 1, col: 1}
	var out []Value
	for {
		r.skipSpace()
		if r.eof() {
			return out, nil
		}
		v, err := r.readValue()
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
}

type reader struct {
	src  string
	file string
	i    int
	line int
	col  int
}

func (r *reader) eof() bool { return r.i >= len(r.src) }
func (r *reader) pos() Pos  { return Pos{File: r.file, Line: r.line, Col: r.col} }

func (r *reader) peek() byte {
	if r.eof() {
		return 0
	}
	return r.src[r.i]
}

func (r *reader) advance() byte {
	c := r.src[r.i]
	r.i++
	if c == '\n' {
		r.line++
		r.col = 1
	} else {
		r.col++
	}
	return c
}

func (r *reader) skipSpace() {
	for !r.eof() {
		switch c := r.peek(); {
		case c == ' ', c == '\t', c == '\r', c == '\n':
			r.advance()
		case c == ';':
			for !r.eof() && r.peek() != '\n' {
				r.advance()
			}
		default:
			return
		}
	}
}

func (r *reader) readValue() (Value, error) {
	switch c := r.peek(); {
	case c == '(':
		return r.readList()
	case c == ')':
		return Value{}, Errorf(r.pos(), "unexpected ')'")
	case c == '"':
		return r.readString()
	default:
		return r.readAtom()
	}
}

func (r *reader) readList() (Value, error) {
	pos := r.pos()
	r.advance() // consume '('
	items := []Value{}
	for {
		r.skipSpace()
		if r.eof() {
			return Value{}, Errorf(pos, "unclosed '(' — missing ')'")
		}
		if r.peek() == ')' {
			r.advance()
			return Value{Kind: KList, Items: items, Pos: pos}, nil
		}
		v, err := r.readValue()
		if err != nil {
			return Value{}, err
		}
		items = append(items, v)
	}
}

func (r *reader) readString() (Value, error) {
	pos := r.pos()
	r.advance() // consume opening quote
	var sb strings.Builder
	for {
		if r.eof() {
			return Value{}, Errorf(pos, "unterminated string")
		}
		c := r.advance()
		if c == '"' {
			return Value{Kind: KString, Text: sb.String(), Pos: pos}, nil
		}
		if c != '\\' {
			sb.WriteByte(c)
			continue
		}
		if r.eof() {
			return Value{}, Errorf(pos, "unterminated string")
		}
		esc := r.pos()
		switch e := r.advance(); e {
		case 'n':
			sb.WriteByte('\n')
		case 't':
			sb.WriteByte('\t')
		case '\\':
			sb.WriteByte('\\')
		case '"':
			sb.WriteByte('"')
		default:
			return Value{}, Errorf(esc, "unknown escape \\%c", e)
		}
	}
}

func isDelim(c byte) bool {
	switch c {
	case '(', ')', '"', ';', ' ', '\t', '\r', '\n':
		return true
	}
	return false
}

func (r *reader) readAtom() (Value, error) {
	pos := r.pos()
	start := r.i
	for !r.eof() && !isDelim(r.peek()) {
		r.advance()
	}
	text := r.src[start:r.i]
	if text == "" {
		return Value{}, Errorf(pos, "expected a value")
	}
	if text == ":" {
		return Value{}, Errorf(pos, "property name is empty")
	}
	return Value{Kind: classify(text), Text: text, Pos: pos}, nil
}

func classify(text string) Kind {
	switch {
	case text == "*":
		return KStar
	case strings.HasPrefix(text, ":"):
		return KKeyword
	case isNumber(text):
		return KNumber
	default:
		return KSymbol
	}
}

// isNumber reports whether text is a decimal or 0x-prefixed integer.
// Dotted forms such as 10.0.0.1 are deliberately not numbers; addresses
// are written as strings.
func isNumber(s string) bool {
	t := strings.TrimPrefix(s, "-")
	if t == "" {
		return false
	}
	if strings.HasPrefix(t, "0x") || strings.HasPrefix(t, "0X") {
		t = t[2:]
		if t == "" {
			return false
		}
		for i := 0; i < len(t); i++ {
			c := t[i]
			if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
				return false
			}
		}
		return true
	}
	for i := 0; i < len(t); i++ {
		if t[i] < '0' || t[i] > '9' {
			return false
		}
	}
	return true
}
