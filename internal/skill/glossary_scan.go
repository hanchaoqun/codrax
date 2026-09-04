package skill

import "strings"

// glossary_scan.go is the ONE matcher for the glossary vocabulary in
// glossary.go. It lives next to the vocabulary (not in a test file and
// not in glossarylint) so that this package's own internal tests and
// every other package's tests — via internal/skill/glossarylint, which
// adds the Go-source scanning and the renderer census — match tokens by
// exactly the same rule (§40.52 retired four hand-copied matchers).
// Pure string code: no go/ast, no testing import.

// GlossaryHit is one glossary token found in one scanned string.
type GlossaryHit struct {
	// Label identifies the surface: "<file>:<line>" for source
	// literals, a logical "<owner>.<field>" path for rendered text.
	Label   string
	Term    string
	Preview string
}

// String renders the hit in the one-line form every lint reports.
func (h GlossaryHit) String() string {
	return h.Label + ": token=" + h.Term + " preview=" + h.Preview
}

// GlossaryTerms returns the full glossary vocabulary (both blocklists)
// as one fresh slice: internal terms first, project identifiers second.
func GlossaryTerms() []string {
	out := make([]string, 0, len(InternalTermsBlocklist)+len(ProjectSpecificIdentifierBlocklist))
	out = append(out, InternalTermsBlocklist...)
	out = append(out, ProjectSpecificIdentifierBlocklist...)
	return out
}

// MatchGlossaryTerm locates term inside body and returns its byte
// offset, or -1.
//
// Short uppercase acronyms (2–4 letters, all A–Z, e.g. "CGEC", "ERM")
// require word boundaries on both sides so "ERM" does not match inside
// "TERMINAL". Every other token — longer phrases, mixed-case Go names,
// hyphenated codenames such as "LOW-MIND" — uses plain case-sensitive
// substring matching, so "LOW-MIND" matches inside "LOW-MIND RULE:".
func MatchGlossaryTerm(body, term string) int {
	if term == "" {
		return -1
	}
	if isShortUpperAcronym(term) {
		return findWholeWord(body, term)
	}
	return strings.Index(body, term)
}

// ScanText returns one hit per glossary token found in s.
func ScanText(label, s string) []GlossaryHit {
	return scanWithTerms(label, s, GlossaryTerms())
}

// ScanTextWith scans s against the glossary vocabulary PLUS the extra
// per-surface tokens (matched by the same rule). Per-surface oracles
// that need a stricter local vocabulary (bare stage names, Go-shape
// fragments) keep only their extras and delegate the shared vocabulary
// here, so no test re-lists glossary entries (pinned by
// glossarylint.TestNoGlossaryForksInTests).
func ScanTextWith(label, s string, extra ...string) []GlossaryHit {
	terms := GlossaryTerms()
	terms = append(terms, extra...)
	return scanWithTerms(label, s, terms)
}

func scanWithTerms(label, s string, terms []string) []GlossaryHit {
	if s == "" {
		return nil
	}
	var out []GlossaryHit
	for _, term := range terms {
		idx := MatchGlossaryTerm(s, term)
		if idx < 0 {
			continue
		}
		out = append(out, GlossaryHit{Label: label, Term: term, Preview: glossaryPreview(s, idx, len(term))})
	}
	return out
}

func isShortUpperAcronym(s string) bool {
	if len(s) > 4 || len(s) < 2 {
		return false
	}
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

func findWholeWord(body, term string) int {
	from := 0
	for {
		i := strings.Index(body[from:], term)
		if i < 0 {
			return -1
		}
		pos := from + i
		left := pos == 0 || !isGlossaryWordChar(body[pos-1])
		end := pos + len(term)
		right := end == len(body) || !isGlossaryWordChar(body[end])
		if left && right {
			return pos
		}
		from = pos + 1
		if from >= len(body) {
			return -1
		}
	}
}

func isGlossaryWordChar(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '_'
}

// glossaryPreview returns a short single-line snippet of s centred on
// the match.
func glossaryPreview(s string, start, length int) string {
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
