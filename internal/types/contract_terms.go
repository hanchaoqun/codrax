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

// RelaxAnalyzerEntityMustIncludeWithAggregateMemberSet demotes analyzer-derived
// must_include pins once exploration has emitted an authoritative member_set.
//
// AnalyzerHints.Entities are useful early search hints, but exhaustive
// enumerations can narrow or exclude candidates only after reading code and
// emitting aggregate_facts.member_set. At that point the member_set becomes the
// hard principal-member contract; analyzer_entity terms must not keep pulling
// stale candidates back into finalizer retries. Explicit user/template terms
// remain untouched because they are not marked source=analyzer_entity.
func RelaxAnalyzerEntityMustIncludeWithAggregateMemberSet(c AnswerContract, facts []AnswerAggregateFact) AnswerContract {
	if !answerAggregateFactsHaveMemberSet(facts) {
		return c
	}
	drop := map[string]bool{}
	preserve := map[string]bool{}
	var filteredTerms []ContractTerm
	for _, term := range c.MustIncludeTerms {
		text := strings.TrimSpace(term.Text)
		if text == "" {
			continue
		}
		key := strings.ToLower(text)
		if term.Source == ContractTermSourceAnalyzerEntity {
			drop[key] = true
			continue
		}
		preserve[key] = true
		filteredTerms = append(filteredTerms, term)
	}
	if len(drop) == 0 {
		return c
	}
	var filteredLegacy []string
	for _, text := range c.MustInclude {
		key := strings.ToLower(strings.TrimSpace(text))
		if key != "" && drop[key] && !preserve[key] {
			continue
		}
		filteredLegacy = append(filteredLegacy, text)
	}
	c.MustInclude = filteredLegacy
	c.MustIncludeTerms = filteredTerms
	return c
}

func answerAggregateFactsHaveMemberSet(facts []AnswerAggregateFact) bool {
	for _, fact := range facts {
		if AnswerAggregateFactCarriesCompleteMemberSet(fact) {
			return true
		}
	}
	return false
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
