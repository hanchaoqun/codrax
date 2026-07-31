package tool

import (
	"strings"
	"unicode"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func sourceInventoryConstructKindSurface(sym *repotypes.Symbol) string {
	if sym == nil {
		return ""
	}
	return sourceInventoryConstructKindKey(sym.Kind)
}

func sourceInventoryConstructKindKey(raw string) string {
	return strings.Join(strings.Fields(strings.NewReplacer("-", " ", "_", " ").Replace(strings.ToLower(strings.TrimSpace(raw)))), " ")
}

// sourceInventoryParserMarkerForConstructKind keeps parser-declared syntax
// forms distinct when a marker name equals a keyword construct kind. A parser row carrying kind=extend and
// Doc=@Extend(Text) belongs to the @extend marker family, not to the bare
// "extend" keyword family. The decision is row-local and language-neutral:
// it consumes only the parser's Kind and leading marker tokens.
func sourceInventoryParserMarkerForConstructKind(doc, kind string) string {
	doc = strings.TrimSpace(doc)
	if !strings.HasPrefix(doc, "@") || kind == "" {
		return ""
	}
	for _, raw := range strings.Fields(doc) {
		marker := strings.Trim(raw, " \t\r\n,;")
		if !strings.HasPrefix(marker, "@") {
			continue
		}
		base := marker
		if idx := strings.IndexAny(base, "([{"); idx > 0 {
			base = base[:idx]
		}
		if sourceInventoryConstructKindKey(strings.TrimPrefix(base, "@")) == kind {
			return base
		}
	}
	return ""
}

// sourceInventoryNormalizeSurfaceQuote differs intentionally from generic
// query-token normalization: syntax sigils are part of a construct's identity.
// Dropping '@' would collapse @Marker forms into same-spelled keyword forms.
func sourceInventoryNormalizeSurfaceQuote(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}
	var b strings.Builder
	lastSpace := true
	for _, r := range raw {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '@':
			b.WriteRune(r)
			lastSpace = false
		default:
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}
