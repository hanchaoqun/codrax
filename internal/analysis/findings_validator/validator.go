// Package findings_validator scrubs the analyzer's free-form prose
// (auto-captured into StageOutput.StageReport and rendered downstream
// as the "Prior Stage Findings" section) so any file path or
// backticked symbol name the LLM dropped into the narrative is
// cross-checked against repo state. Misses get visibly annotated
// with a strikethrough plus a localized "未验证" / "unverified" tag,
// so the extractor and finalizer cannot silently bake hallucinated
// artefacts into the final answer.
//
// The validator is the CGEC P4 enforcer (Prior-stage findings must
// be verifiable). It runs at analyzer.ParseOutput time, before the
// StageReport is emitted, so every downstream stage that reads the
// findings sees the annotated copy. Successfully verified tokens are
// passed through untouched.
package findings_validator

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Result bundles the annotated text and the structured findings the
// validator wrote so the caller can also push them into
// MutableState.EvidenceClosure.unverifiedFinds for cross-stage
// awareness.
type Result struct {
	// Annotated is the input text with miss-tokens decorated as
	// `~~original~~ ·[未验证: …]`. Hits are unchanged.
	Annotated string
	// Unverified records every miss for the closure tracker.
	Unverified []types.UnverifiedFinding
}

// pathRegex matches repo-relative source-tree paths the analyzer is
// likely to mention. Covers internal/, cmd/, pkg/, src/, lib/,
// docs/, eval/ roots and the most common source extensions. Must
// match `internal/agent/explorer.go` but NOT `please/help`.
var pathRegex = regexp.MustCompile(`(?i)\b(?:internal|cmd|pkg|src|lib|docs|eval|test|tests)/[\w./-]+\.(?:go|py|js|ts|jsx|tsx|java|rs|rb|c|cpp|h|hpp|md|yaml|yml|json|toml)\b`)

// symbolRegex matches `backticked-identifier` tokens. Anchors on
// ASCII letters/digits/underscore so plain English words in
// backticks ("`important`") do not flood the validator with noise.
// We use a positive lookahead-style boundary by requiring that the
// content is at least 3 chars and contains either an upper-case
// letter or a digit / underscore — typical Go / Python identifier
// shape — so words like "agent" / "config" stay below the bar.
var symbolRegex = regexp.MustCompile("`([A-Za-z_][A-Za-z0-9_]*)`")

// Validate scans text for path / backticked-symbol references and
// annotates each miss. repoRoot is the absolute path used to resolve
// path references; graph (optional) is consulted to verify backticked
// symbols. Either may be empty / nil — that disables the
// corresponding check (validate path checks only when repoRoot is
// non-empty; symbol checks only when graph != nil).
//
// The annotation format is intentionally visible:
//
//	~~OriginalText~~ ·[未验证: <reason>]
//	~~OriginalText~~ ·[unverified: <reason>]
//
// Language is auto-detected: any CJK rune in the input switches the
// label to Chinese; otherwise English. Single annotation per miss
// per render so a token mentioned twice gets two annotations.
func Validate(text, repoRoot string, graph *repomap.Graph) Result {
	if strings.TrimSpace(text) == "" {
		return Result{Annotated: text}
	}
	useChinese := containsCJK(text)
	annotated := text
	var unverified []types.UnverifiedFinding
	seen := make(map[string]bool)

	// PATH CHECK
	//
	// Batch-6 C3: pathRegex anchors on well-known tree roots
	// (internal/, src/, ...), so a multi-repo reference like
	// `repo-stub-rust/src/lib.rs` matches starting at `src/` — the
	// sub-repo prefix is silently truncated and the stat against the
	// parent root fails on a CORRECT path. Scan by index and expand
	// each match leftward over path-token bytes to recover the full
	// slash-joined reference; a stat hit on either the full token or
	// the anchored match verifies the path.
	if repoRoot != "" {
		var b strings.Builder
		last := 0
		for _, loc := range pathRegex.FindAllStringIndex(annotated, -1) {
			start, end := loc[0], loc[1]
			full := expandPathTokenLeft(annotated, start, end)
			match := annotated[start:end]
			hit := false
			for _, candidate := range []string{full, match} {
				if candidate == "" {
					continue
				}
				if _, err := os.Stat(filepath.Join(repoRoot, candidate)); err == nil {
					hit = true
					break
				}
			}
			b.WriteString(annotated[last:start])
			if hit {
				b.WriteString(match)
			} else {
				key := "path:" + match
				if !seen[key] {
					seen[key] = true
					unverified = append(unverified, types.UnverifiedFinding{
						Token:  match,
						Kind:   "path",
						Reason: "file does not exist in repo",
					})
				}
				b.WriteString(annotateMiss(match, "path", useChinese))
			}
			last = end
		}
		b.WriteString(annotated[last:])
		annotated = b.String()
	}

	// SYMBOL CHECK
	if graph != nil {
		annotated = symbolRegex.ReplaceAllStringFunc(annotated, func(match string) string {
			// Strip backticks to get the identifier.
			ident := strings.Trim(match, "`")
			// Skip very short / non-identifier-shaped tokens to avoid
			// flagging plain English words wrapped in backticks.
			if !looksLikeCodeIdentifier(ident) {
				return match
			}
			if isStructuredProtocolVocabulary(ident) {
				return match
			}
			if _, ok := graph.SymbolDefs[ident]; ok {
				return match // hit
			}
			key := "symbol:" + ident
			if !seen[key] {
				seen[key] = true
				unverified = append(unverified, types.UnverifiedFinding{
					Token:  ident,
					Kind:   "symbol",
					Reason: "symbol not found in repo graph",
				})
			}
			return annotateMiss(match, "symbol", useChinese)
		})
	}

	return Result{Annotated: annotated, Unverified: unverified}
}

func isStructuredProtocolVocabulary(ident string) bool {
	token := strings.ToLower(strings.TrimSpace(ident))
	if token == "" {
		return false
	}
	if types.AnswerEvidenceOriginFromStructuredToken(token) != types.AnswerEvidenceOriginUnknown {
		return true
	}
	if _, ok := findingsValidatorProtocolVocabulary[token]; ok {
		return true
	}
	return false
}

var findingsValidatorProtocolVocabulary = map[string]struct{}{
	"emit_analysis":               {},
	"emit_answer_document":        {},
	"emit_answer_document_patch":  {},
	"emit_answer_symbol":          {},
	"emit_evidence":               {},
	"emit_hypothesis_verdict":     {},
	"emit_investigation_complete": {},
	"exec_command":                {},
	"git_diff":                    {},
	"git_history_search":          {},
	"git_log":                     {},
	"git_show":                    {},
	"grep":                        {},
	"list_files":                  {},
	"list_memory":                 {},
	"read_file":                   {},
	"recall_memory":               {},
	"repo_map":                    {},
	"answer_document":             {},
	"answer_document_patch":       {},
	"answer_count":                {},
	"answer_axis":                 {},
	"aggregate_facts":             {},
	"citation_ref":                {},
	"citations":                   {},
	"claim_uses":                  {},
	"command_measurement":         {},
	"context_lines":               {},
	"current_source":              {},
	"evidence_origin":             {},
	"exact_resolution":            {},
	"files_only":                  {},
	"first_byte_timeout":          {},
	"is_count_question":           {},
	"is_scalar_answer":            {},
	"measurement_origin":          {},
	"requested_answer_dimensions": {},
	"result_kind":                 {},
	"return_value":                {},
	"source_inventory":            {},
	"source_inventory_profile":    {},
	"support_refs":                {},
	"tool_choice":                 {},
	"tool_result":                 {},
}

// annotateMiss formats the strikethrough + warning tag.
func annotateMiss(orig, kind string, useChinese bool) string {
	if useChinese {
		var label string
		switch kind {
		case "path":
			label = "未验证: repo 中不存在"
		case "symbol":
			label = "未验证: 未在 repo 中找到"
		default:
			label = "未验证"
		}
		return "~~" + orig + "~~ ·[" + label + "]"
	}
	var label string
	switch kind {
	case "path":
		label = "unverified: file does not exist in repo"
	case "symbol":
		label = "unverified: symbol not in repo graph"
	default:
		label = "unverified"
	}
	return "~~" + orig + "~~ ·[" + label + "]"
}

// containsCJK returns true when text contains any rune in the
// Hiragana / Katakana / Han / Hangul ranges. Used as the language
// switch for the annotation label.
func containsCJK(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) ||
			unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r) {
			return true
		}
	}
	return false
}

// looksLikeCodeIdentifier filters plain English words wrapped in
// backticks (which the analyzer sometimes emits for emphasis) from
// real code identifiers. Heuristic: must be ≥3 chars AND must
// contain at least one upper-case letter, digit, or underscore.
// "explorerSkill" qualifies (camelCase); "important" doesn't.
func looksLikeCodeIdentifier(s string) bool {
	if len(s) < 3 {
		return false
	}
	for _, r := range s {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			return true
		}
	}
	return false
}

// expandPathTokenLeft widens a pathRegex match leftward over
// path-token bytes (letters, digits, '_', '-', '.', '/') so a
// sub-repo-prefixed reference like repo-stub-rust/src/lib.rs is
// recovered in full when the regex anchored mid-token at src/.
// Returns "" when no expansion happened (caller then checks only the
// original match). Expansion stops at whitespace, backticks, CJK, or
// any byte outside the token set, and is only meaningful when the
// extra prefix ends with '/'.
func expandPathTokenLeft(text string, start, end int) string {
	i := start
	for i > 0 {
		c := text[i-1]
		if c == '/' || c == '.' || c == '-' || c == '_' ||
			(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			i--
			continue
		}
		break
	}
	if i == start || text[start-1] != '/' {
		return ""
	}
	return text[i:end]
}
