package skill

import (
	"strings"
	"testing"
)

// TestNoInternalTermsInPrompts scans every LLM-facing string inside
// every registered skill config for implementation-jargon tokens from
// InternalTermsBlocklist.
//
// Batch 1 (report-only): violations are reported via t.Log but do not
// fail the build. The collected violation list becomes the work queue
// for batches 2/3/4. Each later batch purges its category, then flips
// the corresponding tokens to t.Errorf so a regression cannot be
// re-introduced.
//
// Scope:
//   - skill.BuildAnalysisSkill() — the analyzer's declarative contract
//   - every entry the default registry carries (explore / extract /
//     answer-document / log-triage / log-segmentation)
//
// Each skill contributes five corpora (Goal, OutputFormat, and the
// Workflow / Prohibitions / ToolSuggestions slices). ToolSuggestions
// is scanned for completeness but is very unlikely to contain jargon
// because its entries are tool names; including it keeps the lint
// surface uniform with the rest of the Config.
func TestNoInternalTermsInPrompts(t *testing.T) {
	reg := NewRegistry()
	RegisterDefaults(reg)

	// BuildAnalysisSkill is called by RegisterDefaults, so iterating
	// the registry covers every skill including analysis-skill.
	names := reg.List()
	if len(names) == 0 {
		t.Fatalf("empty skill registry — RegisterDefaults did not register any skill")
	}

	var hits []string
	for _, name := range names {
		sk, err := reg.Get(name)
		if err != nil {
			t.Fatalf("Registry.Get(%q): %v", name, err)
		}
		hits = append(hits, scanConfig(sk)...)
	}

	if len(hits) == 0 {
		return
	}

	// Batches 2A/2B purged the categories currently covered here; the
	// gate is now t.Errorf so any regression fails CI. Adding a new
	// blocklist token without first cleaning the existing surfaces
	// will also fail — by design, so the one-sided cleanup cannot land.
	t.Errorf("TestNoInternalTermsInPrompts found %d violation(s); see docs/prompt_glossary.md for replacements:", len(hits))
	for _, h := range hits {
		t.Errorf("  %s", h)
	}
}

// scanConfig walks every LLM-facing string in a Config and returns
// one "<skill>.<field>[idx]: token=<term>" line per hit.
func scanConfig(sk *Config) []string {
	var out []string
	prefix := sk.Name + "."

	if h := scanString(prefix+"Goal", sk.Goal); len(h) > 0 {
		out = append(out, h...)
	}
	for i, w := range sk.Workflow {
		label := prefix + "Workflow[" + itoa(i) + "]"
		if h := scanString(label, w); len(h) > 0 {
			out = append(out, h...)
		}
	}
	if h := scanString(prefix+"OutputFormat", sk.OutputFormat); len(h) > 0 {
		out = append(out, h...)
	}
	for i, p := range sk.Prohibitions {
		label := prefix + "Prohibitions[" + itoa(i) + "]"
		if h := scanString(label, p); len(h) > 0 {
			out = append(out, h...)
		}
	}
	// ToolSuggestions are tool names and unlikely to contain jargon,
	// but scan for completeness so a future addition can't hide here.
	for i, ts := range sk.ToolSuggestions {
		label := prefix + "ToolSuggestions[" + itoa(i) + "]"
		if h := scanString(label, ts); len(h) > 0 {
			out = append(out, h...)
		}
	}
	return out
}

// scanString returns one hit line per blocklist token found in s.
// Matches are reported with a short preview of the surrounding text
// so reviewers can see the context without opening the file.
//
// Short uppercase acronyms (e.g. "CGEC", "ERM") require word-boundary
// matches so they do not false-positive inside unrelated words
// ("TERMINAL" contains "ERM", "EmitAnalysisSchemaCache" contains
// "ERM"-like triplets, etc.). Longer / mixed-case tokens use plain
// substring matching.
func scanString(label, s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, term := range InternalTermsBlocklist {
		if term == "" {
			continue
		}
		idx := findTermMatch(s, term)
		if idx < 0 {
			continue
		}
		out = append(out, label+": token="+term+" preview="+preview(s, idx, len(term)))
	}
	return out
}

func findTermMatch(body, term string) int {
	if isShortUpperAcronym(term) {
		return findWholeWord(body, term)
	}
	return strings.Index(body, term)
}

func isShortUpperAcronym(s string) bool {
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

func findWholeWord(body, term string) int {
	from := 0
	for {
		i := strings.Index(body[from:], term)
		if i < 0 {
			return -1
		}
		pos := from + i
		left := pos == 0 || !isWordChar(body[pos-1])
		end := pos + len(term)
		right := end == len(body) || !isWordChar(body[end])
		if left && right {
			return pos
		}
		from = pos + 1
		if from >= len(body) {
			return -1
		}
	}
}

func isWordChar(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '_'
}

// preview returns a short snippet of s centered on the match, escaped
// for single-line log output (newlines → " / ").
func preview(s string, start, length int) string {
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

// itoa is a tiny strconv.Itoa replacement so this test file needs no
// strconv import (fits alongside the Go-stdlib-lean style elsewhere
// in the skill package).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
