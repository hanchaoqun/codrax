package types

import "strings"

// ArtifactLineRef is one artifact-local line span the user's question
// references. Source distinguishes which attached artifact the span
// addresses.
type ArtifactLineRef struct {
	// Source is "log" or "trace".
	Source string `json:"source"`
	// StartLine is the 1-based artifact-local line. EndLine extends the
	// span inclusively; zero means single line.
	StartLine int `json:"start_line"`
	EndLine   int `json:"end_line,omitempty"`
}

// NormalizeArtifactLineRefs bounds the refs: source enum, positive
// lines, end >= start (defaulting to start). Invalid entries drop.
func NormalizeArtifactLineRefs(in []ArtifactLineRef) []ArtifactLineRef {
	var out []ArtifactLineRef
	for _, r := range in {
		src := strings.ToLower(strings.TrimSpace(r.Source))
		if src != "log" && src != "trace" {
			continue
		}
		if r.StartLine < 1 {
			continue
		}
		if r.EndLine < r.StartLine {
			r.EndLine = r.StartLine
		}
		out = append(out, ArtifactLineRef{Source: src, StartLine: r.StartLine, EndLine: r.EndLine})
	}
	return out
}
