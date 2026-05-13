package types

import (
	"path/filepath"
	"strings"
)

var knownContractToolNames = map[string]bool{
	"apply_patch":                 true,
	"emit_analysis":               true,
	"emit_answer_document":        true,
	"emit_answer_document_patch":  true,
	"emit_answer_symbol":          true,
	"emit_change_plan":            true,
	"emit_evidence":               true,
	"emit_investigation_complete": true,
	"emit_log_segmentation":       true,
	"emit_log_triage":             true,
	"emit_perf_segmentation":      true,
	"emit_perf_trace":             true,
	"emit_plan_change":            true,
	"emit_plan_skeleton":          true,
	"emit_test_results":           true,
	"exec_command":                true,
	"git_diff":                    true,
	"git_log":                     true,
	"grep":                        true,
	"list_files":                  true,
	"list_memory":                 true,
	"read_file":                   true,
	"recall_memory":               true,
	"repo_map":                    true,
	"run_tests":                   true,
}

// InferContractTermKind classifies analyzer-pinned must_include terms
// using exact typed surfaces: known tool names, path/file-stem shape,
// phrase shape, or symbol fallback. It is intentionally not a semantic
// keyword matcher.
func InferContractTermKind(term string) ContractTermKind {
	t := strings.TrimSpace(term)
	if t == "" {
		return ContractTermUserPhrase
	}
	if knownContractToolNames[strings.ToLower(t)] {
		return ContractTermToolName
	}
	slash := strings.ReplaceAll(t, `\`, `/`)
	if strings.Contains(slash, "/") || strings.Contains(filepath.Base(slash), ".") {
		return ContractTermFileStem
	}
	if strings.ContainsAny(t, " \t\r\n") {
		return ContractTermUserPhrase
	}
	return ContractTermSymbol
}

// NormalizedMustIncludeTerms merges the legacy string list and the
// typed term list into one de-duplicated, kind-bearing sequence. Keep
// this in types/ so contract checking, finalizer prompting, and
// investigation preflight consume the same hard-term semantics.
func NormalizedMustIncludeTerms(c AnswerContract) []ContractTerm {
	return normalizeContractTerms(c.MustInclude, c.MustIncludeTerms)
}

// NormalizedMustExcludeTerms mirrors NormalizedMustIncludeTerms for
// exclusion gates.
func NormalizedMustExcludeTerms(c AnswerContract) []ContractTerm {
	return normalizeContractTerms(c.MustExclude, c.MustExcludeTerms)
}

func normalizeContractTerms(legacy []string, typed []ContractTerm) []ContractTerm {
	out := make([]ContractTerm, 0, len(legacy)+len(typed))
	seen := map[string]struct{}{}
	typedTexts := make(map[string]struct{}, len(typed))
	for _, term := range typed {
		text := strings.TrimSpace(term.Text)
		if text != "" {
			typedTexts[strings.ToLower(text)] = struct{}{}
		}
	}
	add := func(term ContractTerm) {
		text := strings.TrimSpace(term.Text)
		if text == "" {
			return
		}
		kind := term.Kind
		if !kind.IsValid() {
			kind = InferContractTermKind(text)
		}
		source := term.Source
		if !source.IsValid() {
			source = ""
		}
		key := string(kind) + "\x00" + strings.ToLower(text)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, ContractTerm{Text: text, Kind: kind, Source: source})
	}
	for _, text := range legacy {
		if _, alreadyTyped := typedTexts[strings.ToLower(strings.TrimSpace(text))]; alreadyTyped {
			continue
		}
		add(ContractTerm{Text: text, Kind: InferContractTermKind(text)})
	}
	for _, term := range typed {
		add(term)
	}
	return out
}
