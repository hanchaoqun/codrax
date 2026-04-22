package tool

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
)

// TestNoInternalTermsInToolSchemas scans every emit_* tool's
// Description() return and every "description" string in its
// Parameters() JSON schema for implementation-jargon tokens from
// skill.InternalTermsBlocklist.
//
// Unlike skill configs which have a fixed struct shape, tool schemas
// are free-form JSON. This test recursively walks the decoded
// map[string]any tree so descriptions nested inside properties /
// items / anyOf / oneOf are all covered.
//
// Batch 1 policy: report-only (t.Log). Batch 2B cleaned the tool-schema
// surfaces and batch 4B promoted the gate to t.Fatal.
func TestNoInternalTermsInToolSchemas(t *testing.T) {
	// The list of emit_* tools the LLM sees. Kept explicit rather
	// than pulling from the Registry so this test does not depend on
	// cmd/root.go's registration path (which would pull in the
	// repomap package just to discard it). Adding a new emit_* tool
	// requires one line here — the compiler flags the mismatch.
	tools := []Tool{
		&EmitAnalysis{},
		&EmitEvidence{},
		&EmitInvestigationComplete{},
		&EmitAnswerSymbol{},
		&EmitHypothesisVerdict{},
		&EmitAnswerDocument{},
		&EmitLogTriage{},
		&EmitLogSegmentation{},
	}

	var hits []string
	for _, tl := range tools {
		name := tl.Name()

		if h := scanLLMText(name+".Description", tl.Description()); len(h) > 0 {
			hits = append(hits, h...)
		}

		params := tl.Parameters()
		if len(params) == 0 {
			continue
		}
		var decoded any
		if err := json.Unmarshal(params, &decoded); err != nil {
			t.Errorf("%s: Parameters() is not valid JSON: %v", name, err)
			continue
		}
		hits = append(hits, scanJSONDescriptions(name+".Parameters", "", decoded)...)
	}

	if len(hits) == 0 {
		return
	}

	// Per-violation lines via t.Errorf first, summary via t.Fatalf
	// last so the full list is visible in a single test run.
	for _, h := range hits {
		t.Errorf("  %s", h)
	}
	t.Fatalf("TestNoInternalTermsInToolSchemas found %d violation(s); see docs/prompt_glossary.md for replacements", len(hits))
}

// scanJSONDescriptions walks a decoded JSON schema tree and scans
// every value keyed under the literal property name "description"
// against the blocklist. Non-string "description" values (unlikely
// but defensively handled) are skipped silently.
func scanJSONDescriptions(root, path string, node any) []string {
	var out []string
	switch v := node.(type) {
	case map[string]any:
		for k, child := range v {
			childPath := path
			if childPath == "" {
				childPath = k
			} else {
				childPath = childPath + "." + k
			}
			if k == "description" {
				if s, ok := child.(string); ok {
					if h := scanLLMText(root+"."+childPath, s); len(h) > 0 {
						out = append(out, h...)
					}
				}
				continue
			}
			out = append(out, scanJSONDescriptions(root, childPath, child)...)
		}
	case []any:
		for i, child := range v {
			childPath := path + "[" + strconv.Itoa(i) + "]"
			out = append(out, scanJSONDescriptions(root, childPath, child)...)
		}
	}
	return out
}

// scanLLMText runs the blocklist across a single string and returns
// one hit per match with a short context preview. Short uppercase
// acronyms require word-boundary matches (so "ERM" does not match
// inside "TERMINAL"); longer tokens use plain substring matching.
func scanLLMText(label, s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, term := range skill.InternalTermsBlocklist {
		if term == "" {
			continue
		}
		idx := findTermMatchLocal(s, term)
		if idx < 0 {
			continue
		}
		out = append(out, label+": token="+term+" preview="+previewLLMText(s, idx, len(term)))
	}
	return out
}

func findTermMatchLocal(body, term string) int {
	if isShortUpperAcronymLocal(term) {
		return findWholeWordLocal(body, term)
	}
	return strings.Index(body, term)
}

func isShortUpperAcronymLocal(s string) bool {
	if len(s) > 4 {
		return false
	}
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return len(s) >= 2
}

func findWholeWordLocal(body, term string) int {
	from := 0
	for {
		i := strings.Index(body[from:], term)
		if i < 0 {
			return -1
		}
		pos := from + i
		left := pos == 0 || !isWordCharLocal(body[pos-1])
		end := pos + len(term)
		right := end == len(body) || !isWordCharLocal(body[end])
		if left && right {
			return pos
		}
		from = pos + 1
		if from >= len(body) {
			return -1
		}
	}
}

func isWordCharLocal(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '_'
}

func previewLLMText(s string, start, length int) string {
	const window = 40
	from := start - window
	if from < 0 {
		from = 0
	}
	to := start + length + window
	if to > len(s) {
		to = len(s)
	}
	snippet := s[from:to]
	snippet = strings.ReplaceAll(snippet, "\n", " / ")
	snippet = strings.ReplaceAll(snippet, "\t", " ")
	return "…" + snippet + "…"
}

