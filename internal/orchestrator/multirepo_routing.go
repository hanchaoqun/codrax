package orchestrator

import (
	"path/filepath"
	"strings"

	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// inferQueryLanguages is a cheap heuristic that scans the user's
// request string for filename extensions ("foo.rs", ".go test", "src/
// main.cj") and maps them to language constants. Used as channel D
// input (noisy rank) for the multi-repo routing fold so a "fix the
// rust panic" question naturally biases toward the rust sub-repo.
//
// Returns the deduplicated language list (rmtypes.Lang* values). An
// empty result disables channel D — fold falls through to E
// (biggest-first fallback).
//
// Heuristic only — multi-language repos that legitimately mention
// many extensions get all of them as candidates and the cap-trimming
// will still pick the largest-FileCount among matches. False
// positives (a comment mentioning ".rs" in a non-rust question) are
// noisy-by-construction (R3 — channel D drives RANK not hard
// correctness; partial_typed_lane still discloses anything missed).
func inferQueryLanguages(request string) []string {
	if request == "" {
		return nil
	}
	tokens := tokenizeRequest(request)
	seen := make(map[string]bool)
	out := make([]string, 0, 4)
	for _, tok := range tokens {
		ext := strings.ToLower(filepath.Ext(tok))
		if ext == "" {
			continue
		}
		lang := rmtypes.DetectLanguage("x" + ext)
		if lang == "" || seen[lang] {
			continue
		}
		seen[lang] = true
		out = append(out, lang)
	}
	return out
}

// tokenizeRequest splits the request into whitespace + punctuation
// boundaries so "fix foo.go panic" yields ["fix", "foo.go", "panic"].
// Keeps strings with embedded "." intact (extension detection needs
// the suffix). Drops empty tokens.
func tokenizeRequest(s string) []string {
	if s == "" {
		return nil
	}
	out := make([]string, 0, 8)
	var b strings.Builder
	for _, r := range s {
		if r == '.' || r == '/' || r == '_' || r == '-' || (r >= '0' && r <= '9') ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			b.WriteRune(r)
		} else {
			if b.Len() > 0 {
				out = append(out, b.String())
				b.Reset()
			}
		}
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}
