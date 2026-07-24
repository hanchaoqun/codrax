package markdownext

import (
	"bytes"
	"strings"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// ArtifactLiteral runs immediately before Goldmark Linkify. A Harmony trace
// capture name such as Other_trace_...@69326-2310.sys.systrace is a valid
// email-shaped token to Linkify, but semantically it is an artifact filename.
// Recognized artifact tokens become CodeSpan nodes, which both the HTML and
// terminal renderers display literally and which Linkify cannot turn into a
// mailto link. Ordinary email and URL tokens are left untouched for Linkify.
var ArtifactLiteral goldmark.Extender = &artifactLiteralExtension{}

type artifactLiteralExtension struct{}

func (e *artifactLiteralExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(parser.WithInlineParsers(
		// Goldmark's stock Linkify parser is priority 999. Run one step before
		// it so only the narrow artifact-shaped token is claimed here.
		util.Prioritized(&artifactLiteralParser{}, 998),
	))
}

type artifactLiteralParser struct{}

func (p *artifactLiteralParser) Trigger() []byte {
	// Same trigger surface as Goldmark Linkify. The special space trigger also
	// invokes the parser at a line head.
	return []byte{' ', '*', '_', '~', '('}
}

func (p *artifactLiteralParser) Parse(parent ast.Node, reader text.Reader, _ parser.Context) ast.Node {
	line, segment := reader.PeekLine()
	if len(line) == 0 {
		return nil
	}
	consumedPrefix := 0
	start := segment.Start
	switch line[0] {
	case ' ', '*', '_', '~', '(':
		consumedPrefix = 1
		start++
		line = line[1:]
	}
	tokenLen := artifactLiteralTokenLength(line)
	if tokenLen == 0 {
		return nil
	}
	if consumedPrefix != 0 {
		ast.MergeOrAppendTextSegment(parent, segment.WithStop(segment.Start+1))
	}
	reader.Advance(consumedPrefix + tokenLen)
	code := ast.NewCodeSpan()
	code.AppendChild(code, ast.NewRawTextSegment(text.NewSegment(start, start+tokenLen)))
	return code
}

func (p *artifactLiteralParser) CloseBlock(ast.Node, parser.Context) {}

func artifactLiteralTokenLength(line []byte) int {
	end := len(line)
	if i := bytes.IndexFunc(line, artifactLiteralTokenSeparator); i >= 0 {
		end = i
	}
	if end == 0 {
		return 0
	}
	raw := string(line[:end])
	token := strings.TrimRightFunc(raw, artifactLiteralTrailingPunctuation)
	if token == "" || !artifactLiteralToken(token) {
		return 0
	}
	return len(token)
}

func artifactLiteralTokenSeparator(r rune) bool {
	if unicode.IsSpace(r) {
		return true
	}
	switch r {
	case ',', ';', '!', '?', ')', ']', '}',
		'。', '，', '；', '：', '！', '？', '、':
		return true
	default:
		return false
	}
}

func artifactLiteralTrailingPunctuation(r rune) bool {
	switch r {
	case '.', ',', ';', ':', '!', '?', ')', ']', '}',
		'。', '，', '；', '：', '！', '？', '、':
		return true
	default:
		return false
	}
}

func artifactLiteralToken(token string) bool {
	at := strings.LastIndexByte(token, '@')
	if at <= 0 || at >= len(token)-1 {
		return false
	}
	lower := strings.ToLower(token)
	if !artifactLiteralSuffix(lower) {
		return false
	}
	local := lower[:at]
	afterAt := lower[at+1:]
	pathShaped := strings.ContainsAny(token, `/\`)
	traceNamed := strings.Contains(local, "trace") &&
		strings.ContainsAny(local, "_-")
	timestampNamed := afterAt[0] >= '0' && afterAt[0] <= '9'
	return pathShaped || traceNamed || timestampNamed
}

func artifactLiteralSuffix(lower string) bool {
	for _, suffix := range []string{
		".tracebundle.json",
		".systrace",
		".perftrace",
		".ftrace",
		".htrace",
		".atrace",
		".trace",
		".sys",
	} {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}
