package markdownext

import (
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// QualifiedIdentifierLiteral preserves angle-delimited namespace-qualified
// type arguments as text. CommonMark otherwise reads <Namespace::Type> as an
// autolink with scheme "Namespace:", losing the literal angle brackets and
// inventing navigation authority. This bounded type grammar is shared by
// HTML and terminal rendering; it does not rewrite Markdown or fenced code.
// Normal scheme:URI/email autolinks and explicit [label](destination) links
// retain their own parsers, including an explicitly linked Namespace::Type.
// A bare <custom::opaque> is syntactically ambiguous: it can also be a URI.
// Prefer the lossless literal for that overlap; authors can express link intent
// explicitly with [label](custom::opaque), without any scheme/name allowlist.
var QualifiedIdentifierLiteral goldmark.Extender = &qualifiedIdentifierExtension{}

type qualifiedIdentifierExtension struct{}

func (*qualifiedIdentifierExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(parser.WithInlineParsers(
		// After code spans (100) and explicit links (200), before the
		// stock angle autolink parser (300) can claim a scoped identifier.
		util.Prioritized(&qualifiedIdentifierParser{}, 250),
	))
}

type qualifiedIdentifierParser struct{}

func (*qualifiedIdentifierParser) Trigger() []byte { return []byte{'<'} }

func (*qualifiedIdentifierParser) Parse(_ ast.Node, reader text.Reader, _ parser.Context) ast.Node {
	line, segment := reader.PeekLine()
	length := qualifiedIdentifierLength(line)
	if length == 0 {
		return nil
	}
	reader.Advance(length)
	// Raw text preserves the original identity bytes; HTML's text renderer
	// still escapes markup. It is neither a RawHTML node nor a new code span.
	return ast.NewRawTextSegment(segment.WithStop(segment.Start + length))
}

func (*qualifiedIdentifierParser) CloseBlock(ast.Node, parser.Context) {}

func qualifiedIdentifierLength(line []byte) int {
	if len(line) < 2 || line[0] != '<' {
		return 0
	}
	i, first := 1, true
	for {
		var ok bool
		i, ok = qualifiedTypeArgumentEnd(line, i, first)
		if !ok || i >= len(line) {
			return 0
		}
		if line[i] == '>' {
			return i + 1
		}
		if line[i] != ',' {
			return 0
		}
		i, first = i+1, false
	}
}

// This is deliberately a finite type-argument grammar, not a C++ parser:
// a name, language cv qualifiers, pointer/reference/array decorations, and
// comma-separated arguments. The first name must be namespace-qualified.
// Array extents are empty, decimal, or names, never evaluated expressions.
func qualifiedTypeArgumentEnd(line []byte, i int, requireQualified bool) (int, bool) {
	i = qualifiedTypeSpace(line, i)
	for {
		end := qualifiedTypeCV(line, i)
		if end == i || qualifiedTypeSpace(line, end) == end {
			break
		}
		i = qualifiedTypeSpace(line, end)
	}
	var parts int
	i, parts = qualifiedTypeName(line, i)
	if parts == 0 || requireQualified && parts < 2 {
		return 0, false
	}
	for {
		i = qualifiedTypeSpace(line, i)
		if end := qualifiedTypeCV(line, i); end != i {
			i = end
			continue
		}
		if i >= len(line) || line[i] != '*' {
			break
		}
		i++
	}
	if i < len(line) && line[i] == '&' {
		i++
		if i < len(line) && line[i] == '&' {
			i++
		}
	}
	for {
		i = qualifiedTypeSpace(line, i)
		if i >= len(line) || line[i] != '[' {
			return i, true
		}
		i = qualifiedTypeSpace(line, i+1)
		if i < len(line) && line[i] >= '0' && line[i] <= '9' {
			for i < len(line) && line[i] >= '0' && line[i] <= '9' {
				i++
			}
		} else if i < len(line) && line[i] != ']' {
			i, parts = qualifiedTypeName(line, i)
			if parts == 0 {
				return 0, false
			}
		}
		i = qualifiedTypeSpace(line, i)
		if i >= len(line) || line[i] != ']' {
			return 0, false
		}
		i++
	}
}

func qualifiedTypeSpace(line []byte, i int) int {
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	return i
}

func qualifiedTypeCV(line []byte, i int) int {
	for _, qualifier := range []string{"const", "volatile"} {
		end := i + len(qualifier)
		if end > len(line) || string(line[i:end]) != qualifier {
			continue
		}
		if end == len(line) {
			return end
		}
		switch line[end] {
		case ' ', '\t', '*', '&', '[', ',', '>':
			return end
		}
	}
	return i
}

func qualifiedTypeName(line []byte, i int) (int, int) {
	parts := 0
	for {
		start := i
		for i < len(line) {
			r, size := utf8.DecodeRune(line[i:])
			if r == utf8.RuneError && size == 1 {
				return 0, 0
			}
			if r != '_' && !unicode.IsLetter(r) && !(i > start && (unicode.IsDigit(r) || unicode.IsMark(r))) {
				break
			}
			i += size
		}
		if i == start {
			return 0, 0
		}
		parts++
		if i+1 >= len(line) || line[i] != ':' || line[i+1] != ':' {
			return i, parts
		}
		i += 2
	}
}
