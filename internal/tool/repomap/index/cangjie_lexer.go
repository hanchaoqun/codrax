package index

import (
	"unicode"
)

// cangjie_lexer.go — a hand-written tokeniser for Cangjie 1.0.0 LTS
// source. Emits a flat stream of tokens with comment / string
// content already consumed so the parser does not need to be
// lexically aware.
//
// Why a lexer rather than regex: the original extract_cangjie.go
// used line-anchored regex which (1) could not track brace depth
// for parent-class association, (2) mis-fired on generic `<T, U>`
// containing commas nested across lines, (3) mis-matched `init` /
// `main` tokens appearing inside string interpolation. The lexer
// eliminates all three classes of bug.
//
// This file is scoped to what the Cangjie top-level extractor
// actually needs: it emits keyword / identifier / punctuation
// tokens at source positions. Expression-level tokens (numbers,
// operator symbols) collapse into a single `tokOther` kind because
// the parser only needs to skip them; it never interprets them.

type cangjieTokKind int

const (
	cjTokIdent cangjieTokKind = iota
	cjTokKeyword
	cjTokLBrace
	cjTokRBrace
	cjTokLParen
	cjTokRParen
	cjTokLBracket
	cjTokRBracket
	cjTokLAngle // `<` when in a generic-parameter context
	cjTokRAngle // `>` when closing a generic-parameter context
	cjTokComma
	cjTokColon
	cjTokSemicolon
	cjTokDot
	cjTokArrow // `->`
	cjTokAt    // `@` decorator lead
	cjTokEq    // `=`
	cjTokAssignInit
	cjTokOther
	cjTokEOF
)

type cangjieToken struct {
	Kind   cangjieTokKind
	Text   string // exact source slice
	Offset int    // byte offset into source
	Line   int    // 1-based line number
}

// cangjieKeywords is the small set of tokens the parser actually
// discriminates on. Other Cangjie reserved words (`if`, `else`,
// `return`, …) fall through as `cjTokIdent` — we never need to
// tell them apart at top-level parse.
var cangjieKeywords = map[string]bool{
	"package":   true,
	"import":    true,
	"class":     true,
	"struct":    true,
	"interface": true,
	"enum":      true,
	"extend":    true,
	"func":      true,
	"init":      true,
	"main":      true,
	"operator":  true,
	"foreign":   true,
	"public":    true,
	"private":   true,
	"protected": true,
	"internal":  true,
	"open":      true,
	"static":    true,
	"sealed":    true,
	"abstract":  true,
	"override":  true,
	"redef":     true,
	"mut":       true,
	"const":     true,
	"unsafe":    true,
	"let":       true,
	"var":       true,
	"as":        true,
	"where":     true,
}

// lexCangjie tokenises `src` into a slice of tokens. Never returns
// errors — unrecognisable runes collapse to `cjTokOther` and the
// parser continues. This matches the fail-loud-at-extraction-step
// invariant of the rest of the repomap: partial extraction is
// always better than a hard failure.
func lexCangjie(src []byte) []cangjieToken {
	toks := make([]cangjieToken, 0, len(src)/8)
	n := len(src)
	i := 0
	line := 1

	emit := func(kind cangjieTokKind, off int, text string) {
		toks = append(toks, cangjieToken{Kind: kind, Text: text, Offset: off, Line: line})
	}

	for i < n {
		c := src[i]
		switch {
		case c == '\n':
			line++
			i++
		case c == ' ' || c == '\t' || c == '\r':
			i++
		case c == '/' && i+1 < n && src[i+1] == '/':
			for i < n && src[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < n && src[i+1] == '*':
			i += 2
			closed := false
			for i < n-1 {
				if src[i] == '\n' {
					line++
				}
				if src[i] == '*' && src[i+1] == '/' {
					i += 2
					closed = true
					break
				}
				i++
			}
			if !closed {
				i = n
			}
		case c == '"':
			// Skip string body; track newlines but do not emit a
			// token — the parser never looks at string content.
			i++
			for i < n && src[i] != '"' {
				if src[i] == '\\' && i+1 < n {
					i += 2
					continue
				}
				if src[i] == '\n' {
					line++
				}
				i++
			}
			if i < n {
				i++
			}
		case c == '\'':
			// Rune literal.
			i++
			for i < n && src[i] != '\'' {
				if src[i] == '\\' && i+1 < n {
					i += 2
					continue
				}
				i++
			}
			if i < n {
				i++
			}
		case c == '{':
			emit(cjTokLBrace, i, "{")
			i++
		case c == '}':
			emit(cjTokRBrace, i, "}")
			i++
		case c == '(':
			emit(cjTokLParen, i, "(")
			i++
		case c == ')':
			emit(cjTokRParen, i, ")")
			i++
		case c == '[':
			emit(cjTokLBracket, i, "[")
			i++
		case c == ']':
			emit(cjTokRBracket, i, "]")
			i++
		case c == ',':
			emit(cjTokComma, i, ",")
			i++
		case c == ':':
			emit(cjTokColon, i, ":")
			i++
		case c == ';':
			emit(cjTokSemicolon, i, ";")
			i++
		case c == '.':
			emit(cjTokDot, i, ".")
			i++
		case c == '@':
			emit(cjTokAt, i, "@")
			i++
		case c == '<':
			emit(cjTokLAngle, i, "<")
			i++
		case c == '>':
			emit(cjTokRAngle, i, ">")
			i++
		case c == '-' && i+1 < n && src[i+1] == '>':
			emit(cjTokArrow, i, "->")
			i += 2
		case c == '=':
			// `==` is operator; `=` is assignment. Lumped as cjTokEq
			// which the parser uses only to skip initialisers.
			emit(cjTokEq, i, "=")
			i++
		case isCangjieIdentStart(rune(c)):
			start := i
			for i < n && isCangjieIdentCont(rune(src[i])) {
				i++
			}
			text := string(src[start:i])
			if cangjieKeywords[text] {
				emit(cjTokKeyword, start, text)
			} else {
				emit(cjTokIdent, start, text)
			}
		default:
			emit(cjTokOther, i, string(c))
			i++
		}
	}
	emit(cjTokEOF, n, "")
	return toks
}

func isCangjieIdentStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r)
}

func isCangjieIdentCont(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
