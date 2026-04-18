// Package subject provides the AnswerSubject taxonomy and the per-
// kind judge functions the chain ranker / shape reconciler / retry-
// hint renderer all dispatch on. Given a candidate answer token (the
// terminal of a Resolution Chain, the literal in a value field, the
// key in a config_value field) and an expected AnswerSubjectKind,
// Score returns a confidence in [0, 1] that the token plausibly
// answers a question of that kind.
//
// The judges are deliberately simple: they consult the repomap.Graph
// for symbol existence + Kind match and apply a small set of token-
// shape heuristics (substring patterns in file paths, naming
// conventions, surrounding-context hints). They do NOT try to
// fully understand the code. The chain ranker uses the score
// multiplicatively against the existing chain rank, so a wrong
// judge softly demotes a chain rather than removes it; over-eager
// scoring would distort the ranker without producing a clear win.
//
// To add a new kind: declare it in internal/types/analysis_ir.go
// (AnswerSubjectKind), add a case to Score, write a judge, add a
// test. Do not pre-populate speculative kinds.
package subject

import (
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Score returns a confidence in [0, 1] that token is the kind of
// answer-literal a question with kind=expectedKind would resolve to.
// graph is optional; nil is fine and degrades the judges to pure
// token-shape heuristics.
//
// Score is the central entrypoint — every consumer should go through
// this rather than calling individual judges, because the dispatch
// here is the contract that the chain ranker's α-weighting depends
// on.
func Score(token string, expectedKind types.AnswerSubjectKind, graph *repomap.Graph) float64 {
	clean := normalizeToken(token)
	if clean == "" {
		return 0
	}
	switch expectedKind {
	case types.SubjectFunctionName:
		return judgeFunctionName(clean, graph)
	case types.SubjectTypeName:
		return judgeTypeName(clean, graph)
	case types.SubjectInterface:
		return judgeInterface(clean, graph)
	case types.SubjectHandlerRoute:
		return judgeHandlerRoute(clean, graph)
	case types.SubjectConfigKey:
		return judgeConfigKey(clean, graph)
	case types.SubjectReturnValue:
		return judgeReturnValue(clean, graph)
	case types.SubjectFilePath:
		return judgeFilePath(clean)
	case types.SubjectStringLiteral:
		return judgeStringLiteral(token)
	case types.SubjectNumeric:
		return judgeNumeric(clean)
	case types.SubjectEnumValue:
		return judgeEnumValue(clean, graph)
	case types.SubjectStructField:
		return judgeStructField(clean, graph)
	case types.SubjectGeneric:
		return 0.5 // generic answers neither score high nor zero
	case types.SubjectUnknown:
		return 0
	default:
		return 0
	}
}

// IsAnswerOfKind is a convenience wrapper around Score with a
// hard-coded threshold (0.4). Callers that need fine-grained
// behaviour should use Score directly. Threshold chosen so a token
// that fails every judge (score 0) returns false but a token that
// passes one weak heuristic (~0.3) is treated as a maybe.
func IsAnswerOfKind(token string, kind types.AnswerSubjectKind, graph *repomap.Graph) bool {
	return Score(token, kind, graph) >= 0.4
}

// HasJudge reports whether the taxonomy has a kind-specific
// scoring function for kind. Callers that want to distinguish
// "token does not look like kind" from "taxonomy cannot judge
// this kind" (so they can fail open on the latter) use this.
// Session 11 G7 C5 literal-form check leans on it — a literal
// whose kind has no judge passes unconditionally.
func HasJudge(kind types.AnswerSubjectKind) bool {
	switch kind {
	case types.SubjectFunctionName, types.SubjectTypeName,
		types.SubjectInterface, types.SubjectHandlerRoute,
		types.SubjectConfigKey, types.SubjectReturnValue,
		types.SubjectFilePath, types.SubjectStringLiteral,
		types.SubjectNumeric, types.SubjectEnumValue,
		types.SubjectStructField:
		return true
	}
	return false
}

// ChainTerminalToken extracts the terminal answer token from a
// Resolution Chain summary. Chains have the shape
//
//	`A.foo()` <verb> X → `B.bar()` <verb> Y → `C.baz()` returns "literal"
//
// and the terminal answer is the right-most quoted literal (or the
// right-most backticked identifier when no literal is present). The
// chain ranker calls this once per chain and feeds the result into
// Score.
//
// Returns the empty string when no terminal can be extracted (the
// caller treats this as score 0).
func ChainTerminalToken(chainSummary string) string {
	// Take the segment after the last "→" (or the whole string if
	// no arrow is present — single-step chains).
	tail := chainSummary
	if i := strings.LastIndex(tail, "→"); i >= 0 {
		tail = tail[i+len("→"):]
	}
	tail = strings.TrimSpace(tail)
	// Prefer a double-quoted literal — that is the chain's actual
	// answer terminal. Fall back to the last backticked identifier.
	if t := lastBetween(tail, `"`, `"`); t != "" {
		return t
	}
	if t := lastBetween(tail, "`", "`"); t != "" {
		// Strip trailing "()" which the chain producer often
		// appends to method-name backticks.
		t = strings.TrimSuffix(t, "()")
		return t
	}
	// No quoted token: fall back to the right-most space-separated
	// word (best-effort).
	parts := strings.Fields(tail)
	if len(parts) == 0 {
		return ""
	}
	return strings.Trim(parts[len(parts)-1], `.,;:`)
}

// normalizeToken strips quoting + backticks + leading/trailing
// punctuation so the judges can compare bare identifiers.
func normalizeToken(t string) string {
	t = strings.TrimSpace(t)
	t = strings.Trim(t, "`\"'")
	t = strings.TrimSuffix(t, "()")
	t = strings.TrimSpace(t)
	return t
}

// lastBetween returns the contents of the LAST complete delimiter
// pair in s. Handles both distinct delimiters ("(", ")") and
// identical delimiters (`"`, `"`); for the identical case the
// positions are scanned left-to-right and paired sequentially so
// the LAST pair is the right-most quoted region.
func lastBetween(s, open, close string) string {
	if open == close {
		positions := indexAll(s, open)
		if len(positions) < 2 {
			return ""
		}
		// Pair (0,1), (2,3), ... — return contents of the last
		// complete pair.
		lastPair := (len(positions) / 2) * 2
		i := positions[lastPair-2]
		j := positions[lastPair-1]
		return s[i+len(open) : j]
	}
	// Distinct delimiters: find the rightmost open, then the next
	// close after it.
	i := strings.LastIndex(s, open)
	if i < 0 {
		return ""
	}
	rest := s[i+len(open):]
	j := strings.Index(rest, close)
	if j < 0 {
		return ""
	}
	return rest[:j]
}

// indexAll returns every starting index of substr in s. Returns nil
// when substr does not occur.
func indexAll(s, substr string) []int {
	if substr == "" {
		return nil
	}
	var out []int
	for i := 0; i+len(substr) <= len(s); {
		j := strings.Index(s[i:], substr)
		if j < 0 {
			break
		}
		out = append(out, i+j)
		i += j + len(substr)
	}
	return out
}

// ── Per-kind judges ─────────────────────────────────────────────────

// (judgeAgentName removed — see Session 11 over-fitting audit.
// The "agent" convention, even as an industry-wide suffix, was
// still a nudge toward the project's own pipeline nomenclature.
// "Agent"-shaped answers are now judged by the generic identifier
// kinds: SubjectTypeName / SubjectFunctionName / SubjectStringLiteral.)

// judgeFunctionName: token must be a function/method symbol in the
// graph. Score is high (0.9) when matched, zero otherwise — a
// function-name answer that isn't in the symbol table is almost
// always wrong.
func judgeFunctionName(token string, graph *repomap.Graph) float64 {
	if token == "" || graph == nil {
		// Without a graph fall back to a weak naming-convention
		// heuristic so the judge still distinguishes obvious
		// function-shaped tokens from plain words.
		if looksLikeIdentifier(token) {
			return 0.3
		}
		return 0
	}
	for _, def := range graph.SymbolDefs[token] {
		switch def.Kind {
		case "function", "method":
			return 0.9
		}
	}
	return 0
}

// judgeTypeName: token must be a type/struct/class/enum symbol.
func judgeTypeName(token string, graph *repomap.Graph) float64 {
	if token == "" || graph == nil {
		if isPascalCase(token) {
			return 0.4
		}
		return 0
	}
	for _, def := range graph.SymbolDefs[token] {
		switch def.Kind {
		case "type", "struct", "class", "enum":
			return 0.9
		}
	}
	return 0
}

// judgeInterface: token must be an interface symbol.
func judgeInterface(token string, graph *repomap.Graph) float64 {
	if token == "" || graph == nil {
		return 0
	}
	for _, def := range graph.SymbolDefs[token] {
		if def.Kind == "interface" || def.Kind == "trait" {
			return 0.9
		}
	}
	return 0
}

// judgeHandlerRoute: routes look like "/users", "/api/v1/foo", or
// "GET /endpoint". Slashes are the dominant signal; HTTP-verb prefixes
// boost confidence.
func judgeHandlerRoute(token string, _ *repomap.Graph) float64 {
	score := 0.0
	if strings.Contains(token, "/") {
		score += 0.5
	}
	for _, verb := range []string{"GET ", "POST ", "PUT ", "DELETE ", "PATCH "} {
		if strings.HasPrefix(token, verb) {
			score += 0.4
			break
		}
	}
	if score > 1 {
		score = 1
	}
	return score
}

// judgeConfigKey: config keys live in *.yaml/*.json/*.toml or as
// snake_case YAML-mapped struct fields. Token-shape: snake_case with
// dots is a strong signal; bare PascalCase is a weak signal (might
// be a Go field name with a yaml tag elsewhere).
func judgeConfigKey(token string, graph *repomap.Graph) float64 {
	if token == "" {
		return 0
	}
	score := 0.0
	if strings.Contains(token, "_") || strings.Contains(token, ".") {
		score += 0.4
	}
	if graph != nil {
		for _, def := range graph.SymbolDefs[token] {
			ext := strings.ToLower(filepath.Ext(def.File))
			switch ext {
			case ".yaml", ".yml", ".json", ".toml":
				score += 0.5
				break
			}
		}
	}
	if score > 1 {
		score = 1
	}
	return score
}

// judgeReturnValue: a return-value answer is essentially "what does
// function X return". The token is usually a literal or another
// identifier; the judge accepts both with similar confidence and
// leaves the disambiguation to the chain ranker.
func judgeReturnValue(token string, graph *repomap.Graph) float64 {
	// Quoted literals get the highest weight: most "what does X
	// return" answers are string/numeric constants.
	if isQuotedLikeToken(token) {
		return 0.8
	}
	// Identifiers in the graph (the function returns ANOTHER
	// callable / type) score 0.6 — still legitimate but less
	// definitive than a literal.
	if graph != nil {
		if _, ok := graph.SymbolDefs[token]; ok {
			return 0.6
		}
	}
	if looksLikeIdentifier(token) {
		return 0.3
	}
	return 0
}

// judgeFilePath: looks like a path with at least one slash and an
// extension we recognize.
func judgeFilePath(token string) float64 {
	if !strings.Contains(token, "/") {
		return 0
	}
	switch strings.ToLower(filepath.Ext(token)) {
	case ".go", ".py", ".js", ".ts", ".java", ".rs", ".rb",
		".yaml", ".yml", ".json", ".toml", ".md":
		return 0.9
	}
	if strings.HasSuffix(token, "/") {
		return 0.5 // directory
	}
	return 0.3
}

// judgeStringLiteral: token is wrapped in quotes (the un-stripped
// surface form, hence the original token is passed in).
func judgeStringLiteral(originalToken string) float64 {
	t := strings.TrimSpace(originalToken)
	if (strings.HasPrefix(t, `"`) && strings.HasSuffix(t, `"`)) ||
		(strings.HasPrefix(t, "'") && strings.HasSuffix(t, "'")) {
		return 0.9
	}
	return 0
}

// judgeNumeric: token is a bare integer or float.
func judgeNumeric(token string) float64 {
	if token == "" {
		return 0
	}
	for i, r := range token {
		switch {
		case r >= '0' && r <= '9':
			continue
		case (r == '.' || r == '-' || r == '+') && i == 0:
			continue
		case r == '.':
			continue
		default:
			return 0
		}
	}
	return 0.9
}

// judgeEnumValue: ALL_CAPS_SNAKE_CASE or PascalCase const-style.
func judgeEnumValue(token string, graph *repomap.Graph) float64 {
	if token == "" {
		return 0
	}
	if isAllCapsSnake(token) {
		return 0.7
	}
	if graph != nil {
		for _, def := range graph.SymbolDefs[token] {
			if def.Kind == "const" {
				return 0.8
			}
		}
	}
	if isPascalCase(token) {
		return 0.4
	}
	return 0
}

// judgeStructField: dotted form Type.Field strongly signals a field.
// Fall back to graph lookup for bare names.
func judgeStructField(token string, graph *repomap.Graph) float64 {
	if strings.Count(token, ".") == 1 {
		return 0.8
	}
	if graph != nil {
		for _, def := range graph.SymbolDefs[token] {
			if def.Kind == "field" {
				return 0.9
			}
		}
	}
	return 0
}

// ── Token-shape helpers ─────────────────────────────────────────────

func looksLikeIdentifier(token string) bool {
	if token == "" {
		return false
	}
	for i, r := range token {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r == '_':
			continue
		case (r >= '0' && r <= '9') && i > 0:
			continue
		default:
			return false
		}
	}
	return true
}

func isPascalCase(token string) bool {
	if !looksLikeIdentifier(token) {
		return false
	}
	if len(token) == 0 {
		return false
	}
	first := rune(token[0])
	return first >= 'A' && first <= 'Z'
}

func isAllCapsSnake(token string) bool {
	if token == "" {
		return false
	}
	hasLetter := false
	for _, r := range token {
		switch {
		case r >= 'A' && r <= 'Z':
			hasLetter = true
		case r >= '0' && r <= '9':
			// digits ok
		case r == '_':
			// underscore ok
		default:
			return false
		}
	}
	return hasLetter
}

func isQuotedLikeToken(token string) bool {
	t := strings.TrimSpace(token)
	if (strings.HasPrefix(t, `"`) && strings.HasSuffix(t, `"`)) ||
		(strings.HasPrefix(t, "'") && strings.HasSuffix(t, "'")) {
		return true
	}
	return false
}
