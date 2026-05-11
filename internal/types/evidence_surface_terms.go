package types

import (
	"path/filepath"
	"strings"
)

// SurfaceTermShouldBeRequiredForEvidence reports whether a
// model-authored surface term is relevant enough to become a hard
// preservation requirement for this evidence anchor. Non-path labels
// (routes, config keys, UI labels, single-file names like Index.ets)
// remain relevant by default. Source-path-like terms are relevant only
// when their basename/stem identifies the item itself; this keeps
// nearby doc-comment filenames from being promoted into principal
// answer prose.
func SurfaceTermShouldBeRequiredForEvidence(term string, ev EvidenceItem, extraCandidates ...string) bool {
	if !SurfaceTermLooksLikeSourcePathReference(term) {
		return true
	}
	candidates := append([]string{}, extraCandidates...)
	candidates = append(candidates,
		ev.Subject,
		ev.AnchorSymbol,
		ev.Object,
		ev.OwnerSymbol,
	)
	return SurfaceTermIdentifiesAnyCandidate(term, candidates...)
}

func SurfaceTermLooksLikeSourcePathReference(term string) bool {
	term = strings.TrimSpace(strings.ReplaceAll(term, `\`, `/`))
	if term == "" || !strings.Contains(term, "/") {
		return false
	}
	return IsCodeOrConfigPathExtension(filepath.Ext(term))
}

func SurfaceTermIdentifiesAnyCandidate(term string, candidates ...string) bool {
	stem := surfaceTermPathStem(term)
	if stem == "" {
		return false
	}
	for _, candidate := range candidates {
		if surfaceTermPathStem(candidate) == stem {
			return true
		}
		for _, part := range ExactResolutionIdentifierTerms(candidate) {
			if part != "" && part == stem {
				return true
			}
		}
	}
	return false
}

func surfaceTermPathStem(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	raw = strings.Trim(raw, "`'\"")
	raw = strings.ReplaceAll(raw, `\`, `/`)
	if idx := strings.LastIndex(raw, "/"); idx >= 0 {
		raw = raw[idx+1:]
	}
	if idx := strings.LastIndex(raw, "."); idx > 0 {
		raw = raw[:idx]
	}
	raw = strings.Trim(raw, "`'\"")
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-' || r == '.':
			b.WriteRune(r)
		default:
			return ""
		}
	}
	return b.String()
}
