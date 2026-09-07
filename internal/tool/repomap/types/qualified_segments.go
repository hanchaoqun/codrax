package types

import "strings"

// SplitQualifiedSegments is the shared, case-preserving qualified-name
// grammar. It only decomposes '.', '::', '#' and Go receiver spellings;
// callers own identity/existence decisions. In particular, '->' remains a
// value-member expression, not a declared scope alias.
// Moved unchanged from repomap's qualified oracle so tool consumers can use
// the same grammar without importing repomap's tool-registration layer.
func SplitQualifiedSegments(name string) []string {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	canon := strings.ReplaceAll(name, "::", ".")
	canon = strings.ReplaceAll(canon, "#", ".")
	raw := strings.Split(canon, ".")
	segments := make([]string, 0, len(raw))
	for _, s := range raw {
		s = normalizeReceiverSegment(strings.TrimSpace(s))
		if s != "" {
			segments = append(segments, s)
		}
	}
	if len(segments) == 0 {
		return nil
	}
	return segments
}

// normalizeReceiverSegment reduces a Go receiver-shaped segment to its
// bare type name by grammar, not guesswork:
//
//	"(*Gate)"   → "Gate"   (pointer method expression)
//	"(Gate)"    → "Gate"   (value method expression)
//	"(g *Gate)" → "Gate"   (receiver declaration: last field is the type)
//	"*Gate"     → "Gate"   (unparenthesised pointer spelling)
//	"Gate"      → "Gate"   (already bare)
//
// Segments that are not receiver-shaped pass through unchanged; a
// segment that reduces to nothing returns "" and is dropped by the
// caller (honest decomposition failure, never a guess).
func normalizeReceiverSegment(seg string) string {
	if strings.HasPrefix(seg, "(") && strings.HasSuffix(seg, ")") && len(seg) >= 2 {
		seg = strings.TrimSpace(seg[1 : len(seg)-1])
		// Receiver declaration form "(g *Gate)": the TYPE is the last
		// whitespace-separated field per Go grammar.
		if fields := strings.Fields(seg); len(fields) > 1 {
			seg = fields[len(fields)-1]
		}
	}
	seg = strings.TrimLeft(seg, "*&")
	return strings.TrimSpace(seg)
}
