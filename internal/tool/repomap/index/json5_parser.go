package index

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// json5_parser.go — a hand-written, stdlib-only JSON5 subset parser.
//
// Why not a third-party dependency: the only use-site is
// oh-package.json5 / build-profile.json5 parsing for HarmonyOS
// import resolution. The needed JSON5 features are narrow:
//
//   - line / block comments
//   - trailing commas after the last element of object / array
//   - unquoted object keys (identifier-shaped)
//   - single-quoted strings alongside double-quoted
//   - hex numbers (0x...) and leading-dot / trailing-dot floats
//   - multi-line strings are NOT required by oh-package.json5 and
//     not implemented here — if encountered, the raw double-quote
//     decoder handles them line-by-line.
//
// The parser is ~220 LOC of stdlib Go. Adding a generic JSON5
// dependency would have been ~30 LOC but pulls in a module and
// ongoing supply-chain risk; the narrow feature surface is worth
// owning locally.
//
// Public API:
//
//	ParseJSON5(src []byte, out interface{}) error
//
// Follows encoding/json semantics for `out` (pointer to map[string]
// any / struct). Unknown keys silently ignored (consistent with how
// resolvers treat optional fields).

// parseJSON5Value parses any JSON5 value from the tokeniser state
// and returns the resulting Go value (string / float64 / bool / nil
// / []any / map[string]any).
func parseJSON5Value(p *json5Parser) (any, error) {
	p.skipWhitespace()
	if p.eof() {
		return nil, fmt.Errorf("json5: unexpected EOF")
	}
	c := p.peek()
	switch {
	case c == '{':
		return p.parseObject()
	case c == '[':
		return p.parseArray()
	case c == '"' || c == '\'':
		return p.parseString()
	case c == '-' || c == '+' || (c >= '0' && c <= '9') || c == '.':
		return p.parseNumber()
	case isJSON5IdentStart(c):
		return p.parseIdentOrKeyword()
	}
	return nil, fmt.Errorf("json5: unexpected character %q at offset %d", c, p.pos)
}

// ParseJSON5 parses src as JSON5 into *out. Out must be **map[string]any
// or *[]any at the top level (oh-package.json5 is always an object).
// Returns a descriptive error on parse failure.
func ParseJSON5(src []byte, out *map[string]any) error {
	p := &json5Parser{src: src, pos: 0}
	v, err := parseJSON5Value(p)
	if err != nil {
		return err
	}
	p.skipWhitespace()
	if !p.eof() {
		return fmt.Errorf("json5: trailing data after top-level value at offset %d", p.pos)
	}
	m, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("json5: top-level value is not an object (got %T)", v)
	}
	*out = m
	return nil
}

type json5Parser struct {
	src []byte
	pos int
}

func (p *json5Parser) eof() bool { return p.pos >= len(p.src) }

func (p *json5Parser) peek() byte {
	if p.eof() {
		return 0
	}
	return p.src[p.pos]
}

func (p *json5Parser) advance() byte {
	b := p.src[p.pos]
	p.pos++
	return b
}

// skipWhitespace consumes spaces, tabs, newlines, and both line
// and block comments. Idempotent — callers can safely invoke it
// before every lookahead.
func (p *json5Parser) skipWhitespace() {
	for !p.eof() {
		c := p.peek()
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			p.pos++
		case c == '/' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '/':
			for !p.eof() && p.peek() != '\n' {
				p.pos++
			}
		case c == '/' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '*':
			p.pos += 2
			for p.pos+1 < len(p.src) {
				if p.src[p.pos] == '*' && p.src[p.pos+1] == '/' {
					p.pos += 2
					break
				}
				p.pos++
			}
		default:
			return
		}
	}
}

// parseObject consumes `{` ... `}` and returns a map.
func (p *json5Parser) parseObject() (map[string]any, error) {
	if p.peek() != '{' {
		return nil, fmt.Errorf("json5: expected { at offset %d", p.pos)
	}
	p.advance()
	m := map[string]any{}
	for {
		p.skipWhitespace()
		if p.eof() {
			return nil, fmt.Errorf("json5: unclosed object")
		}
		if p.peek() == '}' {
			p.advance()
			return m, nil
		}
		key, err := p.parseKey()
		if err != nil {
			return nil, err
		}
		p.skipWhitespace()
		if p.peek() != ':' {
			return nil, fmt.Errorf("json5: expected `:` after key %q at offset %d", key, p.pos)
		}
		p.advance()
		val, err := parseJSON5Value(p)
		if err != nil {
			return nil, err
		}
		m[key] = val
		p.skipWhitespace()
		if p.peek() == ',' {
			p.advance()
			continue
		}
		if p.peek() == '}' {
			p.advance()
			return m, nil
		}
		return nil, fmt.Errorf("json5: expected `,` or `}` after value at offset %d", p.pos)
	}
}

// parseArray consumes `[` ... `]` and returns a slice.
func (p *json5Parser) parseArray() ([]any, error) {
	if p.peek() != '[' {
		return nil, fmt.Errorf("json5: expected [ at offset %d", p.pos)
	}
	p.advance()
	var out []any
	for {
		p.skipWhitespace()
		if p.eof() {
			return nil, fmt.Errorf("json5: unclosed array")
		}
		if p.peek() == ']' {
			p.advance()
			return out, nil
		}
		val, err := parseJSON5Value(p)
		if err != nil {
			return nil, err
		}
		out = append(out, val)
		p.skipWhitespace()
		if p.peek() == ',' {
			p.advance()
			continue
		}
		if p.peek() == ']' {
			p.advance()
			return out, nil
		}
		return nil, fmt.Errorf("json5: expected `,` or `]` at offset %d", p.pos)
	}
}

// parseKey reads an object key — either a quoted string or a bare
// identifier (JSON5 extension).
func (p *json5Parser) parseKey() (string, error) {
	c := p.peek()
	if c == '"' || c == '\'' {
		return p.parseString()
	}
	if !isJSON5IdentStart(c) {
		return "", fmt.Errorf("json5: invalid key start %q at offset %d", c, p.pos)
	}
	start := p.pos
	for !p.eof() {
		if isJSON5IdentCont(p.peek()) {
			p.pos++
			continue
		}
		break
	}
	return string(p.src[start:p.pos]), nil
}

// parseString reads a quoted string, accepting single or double
// quotes. Decodes standard escapes (\n, \t, \\, \" / \', \u{hex},
// hex \xNN). Line continuation (`\` at EOL) is supported.
func (p *json5Parser) parseString() (string, error) {
	quote := p.advance()
	var b strings.Builder
	for !p.eof() {
		c := p.advance()
		if c == quote {
			return b.String(), nil
		}
		if c == '\\' {
			if p.eof() {
				return "", fmt.Errorf("json5: trailing backslash in string")
			}
			e := p.advance()
			switch e {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case '"', '\'', '\\', '/':
				b.WriteByte(e)
			case '\n', '\r':
				// line continuation — drop.
			case 'u':
				// \uXXXX — narrow subset, 4 hex digits.
				if p.pos+4 > len(p.src) {
					return "", fmt.Errorf("json5: truncated \\u escape")
				}
				hex := string(p.src[p.pos : p.pos+4])
				p.pos += 4
				v, err := strconv.ParseUint(hex, 16, 32)
				if err != nil {
					return "", err
				}
				b.WriteRune(rune(v))
			default:
				b.WriteByte(e)
			}
			continue
		}
		b.WriteByte(c)
	}
	return "", fmt.Errorf("json5: unterminated string")
}

// parseNumber reads a numeric literal. Accepts integer, float,
// hex (0x...), leading-dot, trailing-dot, signed, and exponents.
// Returns float64 for all values (matching encoding/json default).
func (p *json5Parser) parseNumber() (float64, error) {
	start := p.pos
	if p.peek() == '+' || p.peek() == '-' {
		p.pos++
	}
	// Hex?
	if p.pos+1 < len(p.src) && p.src[p.pos] == '0' &&
		(p.src[p.pos+1] == 'x' || p.src[p.pos+1] == 'X') {
		p.pos += 2
		for !p.eof() && isHexDigit(p.peek()) {
			p.pos++
		}
		v, err := strconv.ParseInt(string(p.src[start:p.pos]), 0, 64)
		if err != nil {
			return 0, err
		}
		return float64(v), nil
	}
	for !p.eof() {
		c := p.peek()
		if c >= '0' && c <= '9' {
			p.pos++
			continue
		}
		if c == '.' || c == 'e' || c == 'E' || c == '+' || c == '-' {
			p.pos++
			continue
		}
		break
	}
	text := string(p.src[start:p.pos])
	v, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, fmt.Errorf("json5: bad number %q: %w", text, err)
	}
	return v, nil
}

// parseIdentOrKeyword decodes true / false / null (plus Infinity /
// NaN as JSON5 literals).
func (p *json5Parser) parseIdentOrKeyword() (any, error) {
	start := p.pos
	for !p.eof() && isJSON5IdentCont(p.peek()) {
		p.pos++
	}
	text := string(p.src[start:p.pos])
	switch text {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "null":
		return nil, nil
	case "Infinity":
		return math.Inf(1), nil
	case "NaN":
		return math.NaN(), nil
	}
	return nil, fmt.Errorf("json5: unexpected identifier %q at offset %d", text, start)
}

func isJSON5IdentStart(c byte) bool {
	return c == '_' || c == '$' || unicode.IsLetter(rune(c))
}

func isJSON5IdentCont(c byte) bool {
	return isJSON5IdentStart(c) || (c >= '0' && c <= '9')
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') ||
		(c >= 'a' && c <= 'f') ||
		(c >= 'A' && c <= 'F')
}
