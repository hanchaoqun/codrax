package agent

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// erm_completeness.go — Phase 2 of the commercial-grade
// remediation (docs/design/commercial_grade_3_pattern_remediation.md).
//
// Two-tier ERM design:
//
//   - Tier 1 (breadth) — existing explorer_erm.go: scalar count
//     thresholds (≥2 for moderate, ≥3 for complex) drive the
//     soft-stop signal that lets the explorer LLM decide it has
//     read enough to answer. Used as the LLM-facing "you can stop
//     reading" hint.
//
//   - Tier 2 (depth) — this file: per-question-family completeness
//     validators that gate the finalize stage on hard structural
//     coverage requirements. Even when Tier 1 fired "satisfied"
//     after 2-3 evidence items, Tier 2 may demand the call-chain's
//     entry+exit functions, the count question's exec_command-
//     derived literal, the config-precedence's three layers, etc.
//     Failing Tier 2 reroutes back to the explorer with an
//     LLM-natural "you missed X" hint.
//
// CompletionDimension enumerates the structural axes Tier 2
// validators check. Each question family declares which dimensions
// apply — a count question doesn't need path-depth; a call-chain
// question doesn't need scalar-count.
//
// Tonight's eval surfaced two real Tier 2 gaps:
//   - s7b — "count Criterion Kind constants": LLM read full file,
//     visual-counted 23 (correct: 25). Fix: require count answers
//     to derive from a deterministic tool (grep -c / wc -l), not
//     from a visual scan of read_file output.
//   - s8a — "buildAnalysisIR → gate.Run call chain": LLM read the
//     reconcile* helpers, declared satisfied at 2-3 items, never
//     reached normalizer.Normalize / compiler.Compile / hdp.Plan /
//     binder.BindByRelevance. Fix: require call-chain answers to
//     evidence both entry and exit functions plus ≥3 mid-segment.

// CompletionDimension enumerates the structural-coverage axes a
// Tier 2 validator may check. Each value names ONE dimension; a
// validator may target one dimension, multiple, or none (when the
// validator doesn't apply to the family).
type CompletionDimension string

const (
	// DimensionScalarCount applies to count questions (IsCountQuestion=
	// true). The answer must surface a number derived from a
	// deterministic counting tool (exec_command running grep -c / wc
	// -l / find | wc), not from the LLM's visual count of read_file
	// output. Visual-count is error-prone (s7b regressed off-by-2).
	DimensionScalarCount CompletionDimension = "scalar_count"

	// DimensionCardinality applies to enumeration questions with an
	// explicit declared count (EnumerationBoundary.DeclaredCount > 0).
	// The answer's enumerated item slice must contain ≥ DeclaredCount
	// items.
	DimensionCardinality CompletionDimension = "cardinality"

	// DimensionPathDepth applies to call-chain questions. The
	// evidence must include the chain's entry function AND exit
	// function AND at least 3 mid-segment functions. Without the
	// closure check Tier 1's breadth heuristic stops at any 2 found
	// functions and the LLM produces a partial chain (s8a regressed
	// missing normalizer/compiler/hdp/binder).
	DimensionPathDepth CompletionDimension = "path_depth"

	// DimensionLayerDepth applies to config-precedence questions.
	// The evidence must include all three precedence layers (default
	// struct / config file / CLI override). Missing any layer
	// suggests the answer is half-formed.
	DimensionLayerDepth CompletionDimension = "layer_depth"

	// DimensionEntityParity applies to comparison questions
	// (buckets ≥ 2). The per-bucket evidence count must balance
	// within 50% of the largest bucket — heavily lopsided sampling
	// produces a comparison where one side is well-grounded and
	// the other is sketched.
	DimensionEntityParity CompletionDimension = "entity_parity"
)

// CompletenessValidator is the interface each per-dimension Tier 2
// check implements. A nil validator returned by FamilyValidator means
// "no Tier 2 check applies to this family"; the finalize gate
// proceeds to render.
type CompletenessValidator interface {
	// Dimension returns the structural axis this validator covers.
	Dimension() CompletionDimension

	// Validate inspects the AnalysisIR + accumulated evidence /
	// answer-symbol slate and returns nil when the family's
	// completeness requirements are met. On failure it returns a
	// CompletenessFailure with an LLM-natural FixHint that the
	// re-explore prompt surfaces.
	Validate(input ValidatorInput) *CompletenessFailure
}

// ValidatorInput aggregates the typed signals every validator
// reads. Built once at finalize-gate entry; readers MUST treat
// every field as read-only.
type ValidatorInput struct {
	IR             *types.AnalysisIR
	EvidenceItems  []types.EvidenceItem
	AnswerSymbols  []types.AnswerSymbol
	ToolResults    []types.ToolResult
	RepoFacts      []types.RepoFact
}

// CompletenessFailure describes one Tier 2 violation. FixHint is
// the LLM-natural prose surfaced in the re-explore continuation
// prompt — R6-clean (no internal pipeline terms).
type CompletenessFailure struct {
	Dimension     CompletionDimension
	Family        types.QuestionFamily
	Reason        string // brief structural description
	FixHint       string // LLM-natural prose for re-explore
	MissingTokens []string // optional: specific symbols/files the validator detected missing
}

// FamilyValidators returns the slice of CompletenessValidator
// instances that apply to a given question family. An empty result
// means Tier 2 is a no-op for the family — the finalize gate
// proceeds to render without further checks.
//
// Order matters: validators run in sequence and the first non-nil
// failure surfaces first. The order below sorts by typical
// likelihood-of-fire (count → path → enumeration → comparison →
// config) so the LLM gets the most actionable hint first when
// multiple dimensions fail.
func FamilyValidators(family types.QuestionFamily, ir *types.AnalysisIR) []CompletenessValidator {
	if ir == nil {
		return nil
	}
	rm := ir.RequestModel
	var validators []CompletenessValidator

	// Count-question dimension fires regardless of family — the
	// IsCountQuestion typed signal is the trigger, not Family.
	if rm.Predicates.IsCountQuestion {
		validators = append(validators, ScalarCountValidator{})
	}

	switch family {
	case types.QFEnumeration:
		// Cardinality applies only when the analyzer detected an
		// explicit declared count (e.g., "the 7 stages of X").
		if rm.EnumerationBoundary != nil && rm.EnumerationBoundary.DeclaredCount > 0 {
			validators = append(validators, CardinalityValidator{})
		}
	case types.QFCallChain:
		validators = append(validators, PathDepthValidator{})
	case types.QFConfigPrecedence:
		validators = append(validators, LayerDepthValidator{})
	case types.QFComparison:
		// Comparison validators only fire when the analyzer emitted
		// ≥2 buckets — the structural minimum for a comparison.
		if len(rm.Buckets) >= 2 {
			validators = append(validators, EntityParityValidator{})
		}
	}

	return validators
}

// RunFamilyValidators executes every applicable validator for the
// given family and returns the first non-nil failure encountered.
// nil result means all Tier 2 checks passed — finalize may proceed.
func RunFamilyValidators(family types.QuestionFamily, input ValidatorInput) *CompletenessFailure {
	for _, v := range FamilyValidators(family, input.IR) {
		if fail := v.Validate(input); fail != nil {
			return fail
		}
	}
	return nil
}

// --- ScalarCountValidator ---------------------------------------

// ScalarCountValidator (DimensionScalarCount) requires a count-
// question answer to be backed by deterministic-tool output
// (exec_command running grep -c / wc -l / find | wc), not the
// LLM's visual count of read_file content. s7b's off-by-2 result
// would have been caught here: the failing run had no exec_command
// tool result at all in the trace, only read_file.
//
// The validator inspects ToolResults for an exec_command invocation
// whose output looks like a count (a leading integer literal). Any
// match satisfies the dimension. Future enhancement could check
// specifically that the count cited in the answer matches the
// tool's output.
type ScalarCountValidator struct{}

func (ScalarCountValidator) Dimension() CompletionDimension { return DimensionScalarCount }

func (ScalarCountValidator) Validate(input ValidatorInput) *CompletenessFailure {
	if input.IR == nil || !input.IR.RequestModel.Predicates.IsCountQuestion {
		return nil
	}
	for _, tr := range input.ToolResults {
		if tr.ToolName != "exec_command" {
			continue
		}
		// Success=true indicates the tool finished without error;
		// failed calls produce no usable count output.
		if !tr.Success {
			continue
		}
		summary := strings.TrimSpace(tr.Summary)
		if summary == "" {
			continue
		}
		// Heuristic: any digit run anywhere in the output indicates
		// a number was produced. This matches grep -c (single
		// integer line), wc -l (number padded by whitespace), find
		// | wc (similar). False-positive rate is acceptable —
		// the goal is to refuse the case where ZERO exec_command
		// ran, not to perfectly parse output.
		if hasIntegerLiteral(summary) {
			return nil
		}
	}
	// No exec_command output found — the answer is being assembled
	// from visual count.
	return &CompletenessFailure{
		Dimension: DimensionScalarCount,
		Family:    "",
		Reason:    "count question lacks deterministic-tool-derived count (no exec_command result with integer output)",
		FixHint: "This is a counting question. Run `exec_command` with a deterministic counting tool (e.g. `grep -c <pattern> <path>` or `find <path> | wc -l`) and use the tool's exact integer output as the answer. Do NOT count by reading the file and tallying matches yourself — visual counts go off-by-one easily.",
	}
}

// --- CardinalityValidator ---------------------------------------

// CardinalityValidator (DimensionCardinality) requires the answer's
// enumerated item count to satisfy the analyzer's
// EnumerationBoundary.DeclaredCount when the boundary is non-zero.
// "The 7 stages" → answer must list 7 items.
type CardinalityValidator struct{}

func (CardinalityValidator) Dimension() CompletionDimension { return DimensionCardinality }

func (CardinalityValidator) Validate(input ValidatorInput) *CompletenessFailure {
	if input.IR == nil {
		return nil
	}
	rm := input.IR.RequestModel
	if rm.EnumerationBoundary == nil || rm.EnumerationBoundary.DeclaredCount <= 0 {
		return nil
	}
	declared := rm.EnumerationBoundary.DeclaredCount
	got := len(input.AnswerSymbols)
	if got >= declared {
		return nil
	}
	return &CompletenessFailure{
		Dimension: DimensionCardinality,
		Family:    types.QFEnumeration,
		Reason:    "enumerated item count below declared boundary",
		FixHint: "The question stated an explicit count, but the answer surfaces fewer items than that count requires. Continue investigation to find the missing items, or, if no further items can be located, explicitly acknowledge the count mismatch in the answer (e.g., 'the question mentions N but only M are present in the codebase').",
	}
}

// --- PathDepthValidator -----------------------------------------

// PathDepthValidator (DimensionPathDepth) requires call-chain
// answers to evidence both entry and exit functions PLUS at least
// 3 distinct mid-segment functions. s8a's failure mode was the
// explorer reaching Tier 1 satisfied at 2 reconcile* functions
// and stopping; the answer missed normalizer.Normalize /
// compiler.Compile / hdp.Plan / binder.BindByRelevance — all
// load-bearing mid-segment steps in buildAnalysisIR.
//
// The validator extracts entry and exit candidates from the
// question's entities (the user's named subjects) and counts
// distinct function-symbol mentions across the evidence pool.
type PathDepthValidator struct{}

func (PathDepthValidator) Dimension() CompletionDimension { return DimensionPathDepth }

func (PathDepthValidator) Validate(input ValidatorInput) *CompletenessFailure {
	if input.IR == nil {
		return nil
	}
	rm := input.IR.RequestModel
	entities := rm.AnalyzerHints.Entities
	if len(entities) < 2 {
		// Without ≥2 named entities the analyzer didn't classify
		// this as a path question; defer to other validators.
		return nil
	}

	// Count distinct function-symbol mentions across the evidence
	// pool. We only count symbols that look like Go-style function
	// or method names (uppercase first letter for exported, or
	// receiver-method form like `*Type.Method` / `pkg.Func`).
	mentions := make(map[string]struct{})
	for _, ev := range input.EvidenceItems {
		anchor := strings.TrimSpace(ev.AnchorSymbol)
		if anchor == "" {
			continue
		}
		if !looksLikeFunctionSymbol(anchor) {
			continue
		}
		mentions[anchor] = struct{}{}
	}

	// Path closure heuristic: at least len(entities) + 3 distinct
	// function symbols. The +3 covers expected mid-segment functions
	// beyond the two named entities (entry + exit). This generalises
	// to longer questions ("X → A → B → C → Y" needs 5 entities + 3
	// = 8 mentions), preventing over-easy satisfaction on nominal
	// 2-entity questions.
	required := len(entities) + 3
	if len(mentions) >= required {
		return nil
	}

	missingHint := ""
	if len(mentions) < required {
		missingHint = " The current evidence covers " +
			itoa(len(mentions)) + " distinct functions; this kind of question needs at least " +
			itoa(required) + "."
	}

	return &CompletenessFailure{
		Dimension: DimensionPathDepth,
		Family:    types.QFCallChain,
		Reason:    "call-chain evidence does not cover entry → mid-segment → exit",
		FixHint: "This is a call-chain or processing-pipeline question. The answer must trace from the entry function through the mid-segment processing all the way to the exit function — list every load-bearing intermediate function the data passes through, not just the first two or three." + missingHint + " If the entry function is large (>500 lines), read the full body before stopping; load-bearing pipeline steps are often scattered across the function.",
	}
}

// --- LayerDepthValidator ----------------------------------------

// LayerDepthValidator (DimensionLayerDepth) requires config-
// precedence answers to surface evidence from all three layers:
// default struct (Go default constants), config file (yaml), and
// CLI override (cmd flag binding). Missing any layer suggests the
// answer is half-formed.
type LayerDepthValidator struct{}

func (LayerDepthValidator) Dimension() CompletionDimension { return DimensionLayerDepth }

func (LayerDepthValidator) Validate(input ValidatorInput) *CompletenessFailure {
	if input.IR == nil {
		return nil
	}
	hasDefault, hasConfig, hasCLI := false, false, false
	for _, ev := range input.EvidenceItems {
		// DiagramRole is the typed precedence-layer marker the
		// authority projection produces. Three values map to the
		// three layers we want covered.
		switch ev.DiagramRole {
		case types.EvidenceDiagramRoleDefault:
			hasDefault = true
		case types.EvidenceDiagramRoleConfig:
			hasConfig = true
		case types.EvidenceDiagramRoleOverride, types.EvidenceDiagramRoleRuntime:
			hasCLI = true
		}
		// Source path heuristic fallback — language-agnostic only.
		// Config-file layer detection uses universal config-file
		// extensions that appear in EVERY language ecosystem
		// (yaml/yml/toml/json/ini/properties/conf/cfg/env).
		// Default-struct and CLI-override layers do NOT have a
		// universal path convention (Go uses cmd/root.go, Java uses
		// src/main/.../Cli.java, Python uses cli/main.py, Rust uses
		// src/bin/, etc.) so we rely entirely on the typed DiagramRole
		// signal for those two layers. Falsely under-firing is the
		// safer error mode — better to miss a layer-coverage gap than
		// to falsely accept evidence routed via a Go-specific path
		// pattern from a non-Go repo.
		path := strings.ToLower(strings.TrimSpace(ev.Source))
		if hasConfigFileExtension(path) {
			hasConfig = true
		}
	}
	if hasDefault && hasConfig && hasCLI {
		return nil
	}

	missing := []string{}
	if !hasDefault {
		missing = append(missing, "default-struct layer (Go code defining the default value)")
	}
	if !hasConfig {
		missing = append(missing, "config-file layer (yaml / toml / json)")
	}
	if !hasCLI {
		missing = append(missing, "CLI-override layer (cmd flag or env var)")
	}
	return &CompletenessFailure{
		Dimension:     DimensionLayerDepth,
		Family:        types.QFConfigPrecedence,
		Reason:        "config precedence answer missing one or more layers",
		FixHint:       "Configuration precedence questions need evidence from all three layers: the default-struct layer (Go code), the config-file layer (yaml / toml), and the CLI-override layer (command-line flag or env var). Continue investigation to surface the missing layer(s).",
		MissingTokens: missing,
	}
}

// --- EntityParityValidator --------------------------------------

// EntityParityValidator (DimensionEntityParity) checks that
// per-bucket evidence sampling is balanced across comparison
// buckets. Heavily lopsided sampling means one side of the
// comparison is well-grounded and the other is sketched.
type EntityParityValidator struct{}

func (EntityParityValidator) Dimension() CompletionDimension { return DimensionEntityParity }

func (EntityParityValidator) Validate(input ValidatorInput) *CompletenessFailure {
	if input.IR == nil {
		return nil
	}
	buckets := input.IR.RequestModel.Buckets
	if len(buckets) < 2 {
		return nil
	}
	// Count evidence per bucket via anchor entity overlap.
	counts := make([]int, len(buckets))
	for _, ev := range input.EvidenceItems {
		anchor := strings.ToLower(strings.TrimSpace(ev.AnchorSymbol))
		if anchor == "" {
			continue
		}
		for i, b := range buckets {
			for _, e := range b.Anchors {
				if strings.EqualFold(strings.TrimSpace(e), anchor) {
					counts[i]++
					break
				}
			}
		}
	}
	maxCount := 0
	minCount := -1
	for _, c := range counts {
		if c > maxCount {
			maxCount = c
		}
		if minCount < 0 || c < minCount {
			minCount = c
		}
	}
	if maxCount == 0 {
		// No bucket-anchored evidence yet; defer to Tier 1.
		return nil
	}
	// Reject when smallest bucket is < 50% of largest.
	if minCount*2 < maxCount {
		return &CompletenessFailure{
			Dimension: DimensionEntityParity,
			Family:    types.QFComparison,
			Reason:    "comparison buckets sampled with imbalanced evidence",
			FixHint:   "This is a comparison question. The evidence is heavily skewed to one bucket; continue investigation to gather comparable evidence for the under-sampled bucket(s) so the comparison sides are equally well-grounded.",
		}
	}
	return nil
}

// --- helpers ----------------------------------------------------

// hasIntegerLiteral returns true when s contains a contiguous
// digit run of length ≥ 1 surrounded by non-digit boundaries (so
// "errno=22" matches "22" but a code path like "func256(" is
// matched correctly too — we only need to detect "the output
// contains a counted number"). Used by ScalarCountValidator.
func hasIntegerLiteral(s string) bool {
	for _, r := range s {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

// looksLikeFunctionSymbol returns true when s is a non-path
// identifier-like token. Cross-language: Go / Java / Kotlin use
// CamelCase / camelCase, Python / Rust / Lua use snake_case,
// JS / TS use camelCase / PascalCase, C uses snake_case or
// CamelCase, etc. We accept ANY token that:
//   - is non-empty after trim
//   - does not contain a path separator '/' (filters file paths)
//   - does not look like a file extension (no leading "." that
//     dominates the token)
//   - has at least one alphabetic character (filters numeric-only)
//
// This is intentionally permissive — a false positive (counting a
// non-function symbol toward path depth) is less harmful than a
// false negative (missing an snake_case function name on a non-Go
// repo). The depth threshold (entities + 3) gives breathing room.
func looksLikeFunctionSymbol(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	// Strip leading "*" / "&" for receiver / pointer prefixes.
	for len(s) > 0 && (s[0] == '*' || s[0] == '&') {
		s = s[1:]
	}
	if s == "" {
		return false
	}
	if strings.Contains(s, "/") {
		return false
	}
	// Reject when the token is dominated by a file extension at the
	// end — e.g. "main.go", "helper.py", "lib.rs". The "." may
	// legitimately separate package from symbol ("pkg.Func"), so
	// only reject when what's after the LAST dot looks like a
	// 1-4-char extension and what's before is short.
	if dotAt := strings.LastIndexByte(s, '.'); dotAt > 0 && dotAt < len(s)-1 {
		ext := s[dotAt+1:]
		if len(ext) <= 4 && allLowerLetters(ext) {
			// Looks like ".go" / ".py" / ".rs" / ".java" / ".lua"
			// — file basename, not a symbol.
			return false
		}
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return true
		}
	}
	return false
}

// allLowerLetters returns true when every rune in s is a lowercase
// ASCII letter. Used by looksLikeFunctionSymbol to identify file
// extensions.
func allLowerLetters(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return true
}

// hasConfigFileExtension returns true when path contains a
// universal config-file extension. Cross-language: every ecosystem
// uses one of these for declarative configuration. Substring match
// rather than HasSuffix so paths like "codrax.yaml.example" or
// "config.yml.j2" (template form) still resolve.
func hasConfigFileExtension(path string) bool {
	if path == "" {
		return false
	}
	return strings.Contains(path, ".yaml") ||
		strings.Contains(path, ".yml") ||
		strings.Contains(path, ".toml") ||
		strings.Contains(path, ".json") ||
		strings.Contains(path, ".ini") ||
		strings.Contains(path, ".properties") ||
		strings.Contains(path, ".conf") ||
		strings.Contains(path, ".cfg") ||
		strings.Contains(path, ".env")
}

// itoa is a small ASCII int → string helper local to this file so
// the validator failure messages don't pull in fmt for one Sprintf.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	if neg {
		return "-" + digits
	}
	return digits
}
