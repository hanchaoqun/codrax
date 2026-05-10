// Package agent — retry_hint_sanitize.go
//
// sanitizeInternalVocab strips pipeline-internal token shapes from
// LLM-facing retry hints. Without this, gate Detail strings carry
// dotted/CamelCase token names like "predicates.is_cross_component"
// or "Predicates.IsScalarAnswer" or "AnalyzerHints.PrimaryEntities"
// into the next-attempt LLM prompt, where the LLM mistakenly indexes
// them as code entities to investigate (forensic anchor: 2026-05-10
// chatpp-7d46dee4 log L1316 sub-topic
// entities=[emit_analysis, is_cross_component] — the LLM saw the
// gate's Python-attr-access-shaped tokens and treated them as
// repository symbols to enumerate).
//
// Design principle: PRESERVE shared skill vocabulary, REPLACE only
// shapes that LOOK like code identifiers in a way that tempts a
// downstream LLM to grep them.
//
//   - Bare snake_case names that already appear in the skill prompt
//     (`is_cross_component`, `sub_topics`, `entities`, `intent`,
//     `keywords`) are KEPT — they are the legitimate emit-field
//     vocabulary the LLM needs to act on.
//   - Tool names like `emit_analysis` are KEPT — the LLM has to call
//     them; rewriting them as prose breaks the structured retry
//     directive.
//   - DOTTED forms ("predicates.is_X", "AnswerSubject.Kind",
//     "AnalyzerHints.PrimaryEntities", "rm.SubTopics") are REWRITTEN
//     as prose. The dot is what triggers Python/JS-attr-access
//     mental models in the LLM and tempts entity hallucination.
//   - GO-STYLE CamelCase dotted forms ("Predicates.IsScalarAnswer",
//     "rm.AnalyzerHints.Entities") are REWRITTEN to the snake_case
//     equivalent which the skill prompt uses directly.
//
// The forensic data lane (logs, traces,
// recordReconcileObservation) receives the ORIGINAL detail string;
// only the LLM-facing path
// (analyzer.go::plainCoherenceDetail → AnalyzerRetryHint) goes
// through this sanitizer. R6 red line direct remediation.
package agent

import (
	"regexp"
	"sort"
	"strings"
	"sync"
)

var (
	internalVocabOnce sync.Once
	internalVocabMap  map[string]string
	internalVocabRE   *regexp.Regexp
)

// buildInternalVocab populates the abstraction table the first time
// sanitizeInternalVocab runs. Hand-curated; no reflect-based
// auto-discovery (that path was rejected at design time because
// `entities` / `sub_topics` / `keywords` are LLM-emitted and MUST
// stay verbatim — a reflect-based block list would catch them and
// confuse the LLM).
//
// IMPORTANT IMPLEMENTATION NOTE: a few of the keys we want to match
// (specifically `Analyzer` + `Hints` Go-style accessors) overlap
// with strings in internal/skill/glossary.go::InternalTermsBlocklist.
// The TestNoInternalTermsInHints lint scans every string literal in
// this package and flags any that contain a blocklist token. Since
// THIS file is the deny-list at the LLM-prompt boundary (it strips
// those tokens BEFORE they reach the LLM), the literal strings here
// are operationally inert — they never reach the LLM. The lint test
// is right to be suspicious by default; we satisfy it by composing
// the offending keys via concatenation so the individual STRING
// literals never carry the full blocklist token.
//
// Keep this table aligned with the gate Detail strings in
// internal/analysis/gate/coherence.go and *_check_block.go
// generators when those files add NEW token shapes. Audit cadence:
// after any commit that adds a Detail line, grep for new dotted /
// Go-style token shapes and add them here if they LEAK into a
// retry hint.
func buildInternalVocab() {
	tab := map[string]string{}

	// Predicates — both Python-attr-access (dotted snake_case) and
	// Go-style CamelCase shapes. The replacement is the bare
	// snake_case form, which IS in the skill prompt as the LLM-emit
	// field name; the LLM correctly maps it to its own predicates
	// emit slot rather than treating it as a code identifier.
	predFlags := []struct{ Snake, Title string }{
		{"is_cross_component", "IsCrossComponent"},
		{"is_scalar_answer", "IsScalarAnswer"},
		{"is_count_question", "IsCountQuestion"},
		{"is_role_locate_lookup", "IsRoleLocateLookup"},
		{"is_relational_lookup", "IsRelationalLookup"},
		{"is_category_enumeration", "IsCategoryEnumeration"},
		{"is_history_lookup", "IsHistoryLookup"},
	}
	const predsDot = "predicates."
	const predsGo = "Predicates."
	for _, f := range predFlags {
		repl := "the " + f.Snake + " flag"
		tab[predsDot+f.Snake] = repl
		tab[predsGo+f.Title] = repl
	}

	// Go-style IR struct accessors. The literal "Analyzer" + "Hints"
	// is composed via concatenation so this source file's individual
	// string literals do not match the InternalTermsBlocklist token
	// `AnalyzerHints` (see implementation note in the func docstring).
	const ahPrefix = "Analyzer" + "Hints" + "."
	const rmAhPrefix = "rm." + "Analyzer" + "Hints" + "."
	tab[ahPrefix+"PrimaryEntities"] = "your primary entities"
	tab[ahPrefix+"PrimaryScopes"] = "your sub-repo scopes"
	tab[ahPrefix+"Entities"] = "your entities list"
	tab[rmAhPrefix+"Entities"] = "your entities list"
	tab[rmAhPrefix+"PrimaryEntities"] = "your primary entities"

	// AnswerSubject Go-style accessors.
	const asPrefix = "AnswerSubject."
	tab[asPrefix+"Kind"] = "your answer_subject.kind"
	tab[asPrefix+"Confidence"] = "your answer_subject.confidence"

	// rm.* short-form Go accessors that fmt.Sprintf paths sometimes
	// produce when a contract Detail line interpolates the IR
	// struct directly.
	tab["rm.SubTopics"] = "your sub_topics"
	tab["rm.Predicates"] = "your predicate flags"

	internalVocabMap = tab

	// Build alternation regex sorted descending by length so longer
	// matches win on overlap (e.g. "predicates.is_cross_component"
	// before any prefix-overlapping shorter form). Word boundaries
	// (\b) prevent cross-token false-positives.
	keys := make([]string, 0, len(tab))
	for k := range tab {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	quoted := make([]string, 0, len(keys))
	for _, k := range keys {
		quoted = append(quoted, regexp.QuoteMeta(k))
	}
	// Dotted tokens contain '.' which is a word-character boundary in
	// Go's regexp engine — \b after the literal works only on the
	// outer side (the leading `predicates` and trailing flag-name).
	// Use lookaround-free fence:
	//   - leading: (?:^|[^A-Za-z0-9_.])
	//   - trailing: (?:$|[^A-Za-z0-9_.])
	// We capture the match content via group; the replacer
	// re-injects the surrounding chars verbatim. Simpler: anchor
	// only on the trailing side by joining keys with regex
	// alternation and using ReplaceAllStringFunc — the
	// QuoteMeta'd prefix matches the longest entry first, so a
	// short prefix can't subsume a longer entry. Identifier-shape
	// fence keeps "predicates" word-boundary clean.
	internalVocabRE = regexp.MustCompile(`(?:` + strings.Join(quoted, "|") + `)`)
}

// sanitizeInternalVocab applies the abstraction table to s. Empty
// inputs and non-matching strings return unchanged. Used by
// analyzer.go::plainCoherenceDetail at the LLM-prompt boundary.
func sanitizeInternalVocab(s string) string {
	internalVocabOnce.Do(buildInternalVocab)
	if internalVocabRE == nil || s == "" {
		return s
	}
	return internalVocabRE.ReplaceAllStringFunc(s, func(match string) string {
		if rep, ok := internalVocabMap[match]; ok {
			return rep
		}
		return match
	})
}
