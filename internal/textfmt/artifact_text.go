package textfmt

import "strings"

// NormalizeAttachedArtifactText returns the canonical text surface shown to
// models for attached logs and traces. Provenance checks that compare a
// structured verbatim field with the held attachment must use this same
// surface; otherwise platform newline encoding can make an exact excerpt look
// fabricated after prompt rendering normalized it.
func NormalizeAttachedArtifactText(raw string) string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	return raw
}
