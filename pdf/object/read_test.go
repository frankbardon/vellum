package object_test

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// A minimal PDF object reader, for tests only.
//
// It exists because the first version of these tests matched the written bytes
// with regular expressions, and that "parser" produced wrong answers about
// correct output — which is the same closed loop the tests were meant to break,
// with an extra bug in it. A real reader, however small, either understands the
// bytes or fails; it does not quietly return something plausible.
//
// It reads the shapes this package emits and nothing else. It is not a PDF
// reader and must not grow into one: the moment it needs streams, filters or
// cross-reference recovery, the right answer is a validator, not more code
// here.

// pval is a parsed PDF value.
type pval any

type (
	pdict map[string]pval
	parr  []pval
	pname string
	pref  struct{ Num, Gen int }
	pnum  string // kept as text, because these tests compare rather than compute
	pstr  string
	pbool bool
	pnull struct{}
)

// objectRe finds each indirect object's header and body.
//
// Anchored at line starts rather than requiring a preceding newline, because
// consecutive objects share that byte: "endobj\n" ends one and "1 0 obj" begins
// the next, so a pattern consuming the newline can only match every other
// object. That was the first version's bug.
var objectRe = regexp.MustCompile(`(?ms)^(\d+) 0 obj$\n(.*?)\n^endobj$`)

// readObjects parses a written document into object number to value.
func readObjects(t *testing.T, raw []byte) map[int]pval {
	t.Helper()

	out := map[int]pval{}
	for _, m := range objectRe.FindAllSubmatch(raw, -1) {
		n, err := strconv.Atoi(string(m[1]))
		if err != nil {
			t.Fatalf("object header %q does not parse", m[1])
		}
		p := &parser{src: string(m[2])}
		v, err := p.value()
		if err != nil {
			t.Fatalf("object %d does not parse: %v\n%s", n, err, m[2])
		}
		out[n] = v
	}
	if len(out) == 0 {
		t.Fatal("no objects were parsed; every assertion over them would pass vacuously")
	}
	return out
}

// parser is a recursive-descent reader over one object body.
type parser struct {
	src string
	at  int
}

func (p *parser) skip() {
	for p.at < len(p.src) {
		switch c := p.src[p.at]; {
		case c == ' ' || c == '\n' || c == '\r' || c == '\t' || c == '\f' || c == 0:
			p.at++
		case c == '%':
			for p.at < len(p.src) && p.src[p.at] != '\n' {
				p.at++
			}
		default:
			return
		}
	}
}

func (p *parser) value() (pval, error) {
	p.skip()
	if p.at >= len(p.src) {
		return nil, errAt(p, "unexpected end of object")
	}

	switch {
	case strings.HasPrefix(p.src[p.at:], "<<"):
		return p.dict()
	case p.src[p.at] == '[':
		return p.array()
	case p.src[p.at] == '/':
		return p.name()
	case p.src[p.at] == '(':
		return p.literalString()
	case p.src[p.at] == '<':
		return p.hexString()
	case strings.HasPrefix(p.src[p.at:], "true"):
		p.at += 4
		return pbool(true), nil
	case strings.HasPrefix(p.src[p.at:], "false"):
		p.at += 5
		return pbool(false), nil
	case strings.HasPrefix(p.src[p.at:], "null"):
		p.at += 4
		return pnull{}, nil
	default:
		return p.number()
	}
}

func (p *parser) dict() (pval, error) {
	p.at += 2 // <<
	out := pdict{}
	for {
		p.skip()
		if strings.HasPrefix(p.src[p.at:], ">>") {
			p.at += 2
			return out, nil
		}
		if p.at >= len(p.src) || p.src[p.at] != '/' {
			return nil, errAt(p, "expected a name as a dictionary key")
		}
		key, err := p.name()
		if err != nil {
			return nil, err
		}
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		out[string(key.(pname))] = v
	}
}

func (p *parser) array() (pval, error) {
	p.at++ // [
	var out parr
	for {
		p.skip()
		if p.at >= len(p.src) {
			return nil, errAt(p, "unterminated array")
		}
		if p.src[p.at] == ']' {
			p.at++
			return out, nil
		}
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
}

func (p *parser) name() (pval, error) {
	p.at++ // /
	start := p.at
	for p.at < len(p.src) && isRegular(p.src[p.at]) {
		p.at++
	}
	return pname(p.src[start:p.at]), nil
}

func (p *parser) literalString() (pval, error) {
	p.at++ // (
	var b strings.Builder
	depth := 1
	for p.at < len(p.src) {
		c := p.src[p.at]
		p.at++
		switch c {
		case '\\':
			if p.at < len(p.src) {
				b.WriteByte(p.src[p.at])
				p.at++
			}
		case '(':
			depth++
			b.WriteByte(c)
		case ')':
			depth--
			if depth == 0 {
				return pstr(b.String()), nil
			}
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	return nil, errAt(p, "unterminated string")
}

func (p *parser) hexString() (pval, error) {
	p.at++ // <
	start := p.at
	for p.at < len(p.src) && p.src[p.at] != '>' {
		p.at++
	}
	if p.at >= len(p.src) {
		return nil, errAt(p, "unterminated hex string")
	}
	s := p.src[start:p.at]
	p.at++
	return pstr(s), nil
}

// number reads a number, or an indirect reference when the next two tokens make
// one.
//
// The lookahead is what makes a PDF reader awkward: "3 0 R" is a reference and
// "3 0" is two numbers, and nothing distinguishes them until the R.
func (p *parser) number() (pval, error) {
	start := p.at
	for p.at < len(p.src) && isNumeric(p.src[p.at]) {
		p.at++
	}
	if p.at == start {
		return nil, errAt(p, "expected a number")
	}
	first := p.src[start:p.at]

	save := p.at
	p.skip()
	genStart := p.at
	for p.at < len(p.src) && p.src[p.at] >= '0' && p.src[p.at] <= '9' {
		p.at++
	}
	if p.at > genStart {
		gen := p.src[genStart:p.at]
		p.skip()
		if p.at < len(p.src) && p.src[p.at] == 'R' &&
			(p.at+1 == len(p.src) || !isRegular(p.src[p.at+1])) {
			p.at++
			n, _ := strconv.Atoi(first)
			g, _ := strconv.Atoi(gen)
			return pref{Num: n, Gen: g}, nil
		}
	}
	p.at = save
	return pnum(first), nil
}

func isRegular(c byte) bool {
	if c <= 32 || c >= 127 {
		return false
	}
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return false
	}
	return true
}

func isNumeric(c byte) bool {
	return (c >= '0' && c <= '9') || c == '+' || c == '-' || c == '.'
}

func errAt(p *parser, msg string) error {
	return &parseError{msg: msg, at: p.at, src: p.src}
}

type parseError struct {
	msg string
	at  int
	src string
}

func (e *parseError) Error() string {
	return e.msg + " at offset " + strconv.Itoa(e.at) + ": " +
		e.src[max(0, e.at-20):min(len(e.src), e.at+20)]
}
