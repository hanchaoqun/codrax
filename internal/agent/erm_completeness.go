package agent

import (
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// erm_completeness.go — Phase 2 (advisory) + Phase 2.B (post-finalize
// answer-aware hard gate) of the commercial-grade remediation
// (docs/design/commercial_grade_3_pattern_remediation.md).
//
// Two-tier ERM design:
//
//   - Tier 1 (breadth) — explorer_erm.go: scalar count thresholds
//     (≥2/3 evidence items per question family) drive the soft-stop
//     signal that lets the explorer LLM decide it has read enough.
//
//   - Tier 2 (depth) — this file: per-question-family completeness
//     validators that gate the finalize stage on hard structural
//     coverage requirements. Each validator runs answer-aware AFTER
//     finalize composes the AnswerDocumentV2, reading typed blocks
//     first (Tier 1 input precedence), prose fallback second, and
//     evidence-pool tertiary.
//
// Tonight's eval surfaced two real Tier 2 gaps:
//   - s7b — "count Criterion Kind constants": LLM read full file,
//     visual-counted 23; ground truth was 25.
//   - s8a — "buildAnalysisIR → gate.Run call chain": LLM read
//     reconcile* helpers, declared satisfied at 2-3 items, never
//     reached normalizer.Normalize / compiler.Compile / hdp.Plan /
//     binder.BindByRelevance.
//
// Phase 2.B (2026-05-09) replaced the original advisory wire-up with
// post-finalize hard-gate enforcement: each validator now emits a
// typed Violation (one of four ViolKinds — see violation_registry.go)
// that flows through the existing retry / FallbackTarget / caveat
// machinery. Bounded retry budget is the existing per-kind
// `state.retryUsedForKind` machinery — no new bookkeeping.
//
// Cross-project portability (2026-05-09 audit, after user feedback):
//   - The original LayerDepthValidator was DROPPED — it hard-coded a
//     codrax-specific 3-layer config-precedence assumption (default /
//     config-file / CLI override). Real projects vary widely (Spring
//     Boot profiles × N + env + cmdline; Kubernetes ConfigMap +
//     Secret + env + volumeMount + cluster default; remote config
//     stores adding more layers; static frontends with zero config).
//     The LLM's typed `MissingRequestedRoles` slot already discloses
//     missing layers per the user's question shape — no system-side
//     enforcement needed.
//   - Function-symbol detection (looksLikeFunctionSymbol) accepts
//     CamelCase / camelCase / PascalCase / snake_case / qualified
//     forms — language-neutral.
//   - ScalarCountValidator consumes ToolResult.CommandMeasurement
//     rather than rendered tool summaries, so shell/UI wording changes
//     cannot affect this post-finalize gate.
//   - Caveat templates registered in violation_registry.go are
//     project-portable (no Unix tool names, no fixed layer counts).

// CompletionDimension enumerates the structural-coverage axes a
// Tier 2 validator may check.
type CompletionDimension string

const (
	DimensionScalarCount  CompletionDimension = "scalar_count"
	DimensionCardinality  CompletionDimension = "cardinality"
	DimensionPathDepth    CompletionDimension = "path_depth"
	DimensionEntityParity CompletionDimension = "entity_parity"
)

// CompletenessValidator is the interface each per-dimension Tier 2
// check implements. Returning nil means "no failure"; the finalize
// gate proceeds.
type CompletenessValidator interface {
	Dimension() CompletionDimension
	ViolationKind() types.ViolationKind
	Validate(input ValidatorInput) *CompletenessFailure
}

// ValidatorInput aggregates the typed signals every validator reads.
// Built once at post-finalize gate entry; readers MUST treat every
// field as read-only.
//
// Phase 2.B added AnswerDocumentV2 so validators can do answer-aware
// validation (read the actual composed answer, not just the evidence
// pool). The 3-tier input precedence per validator:
//
//	Tier 1 (typed blocks): inspect AnswerDocumentV2.Blocks[] typed
//	    structure (BlockKind / Items / Citations / EdgeAnchors /
//	    ClaimUses) — the highest-precision signal, never confuses
//	    legitimate prose with structural gaps.
//
//	Tier 2 (prose):  scan block.Text / block.Items[].Text for
//	    semantic markers (digits, list-item regex, identifier-shaped
//	    tokens) — fallback when typed signal is incomplete.
//
//	Tier 3 (evidence pool): the original Phase 2 advisory mode —
//	    inspect EvidenceItems / ToolResults directly. Used as a
//	    fallback only.
//
// Each validator's Validate method walks the tiers in order; a Tier 1
// hit short-circuits without consulting Tier 2/3. This makes false
// positives bounded — an answer that already covers the dimension
// via typed structure NEVER trips the validator.
type ValidatorInput struct {
	IR               *types.AnalysisIR
	EvidenceItems    []types.EvidenceItem
	AnswerSymbols    []types.AnswerSymbol
	ToolResults      []types.ToolResult
	RepoFacts        []types.RepoFact
	AggregateFacts   []types.AnswerAggregateFact
	AnswerDocumentV2 *types.AnswerDocumentV2
}

// CompletenessFailure describes one Tier 2 violation. FixHint is
// the LLM-natural prose surfaced in the re-explore continuation
// prompt — R6-clean (no internal pipeline terms).
type CompletenessFailure struct {
	Dimension     CompletionDimension
	ViolationKind types.ViolationKind
	Family        types.QuestionFamily
	Reason        string   // brief structural description
	FixHint       string   // LLM-natural prose for re-explore
	MissingTokens []string // optional: specific symbols/files the validator detected missing
}

// FamilyValidators returns the slice of CompletenessValidator
// instances that apply to a given question family. An empty result
// means Tier 2 is a no-op for the family.
//
// LayerDepthValidator was removed from this dispatch in Phase 2.B
// (2026-05-09) — codrax-specific 3-layer assumption. QFConfigPrecedence
// now relies on the LLM's MissingRequestedRoles typed slot and the
// CONFIG PRECEDENCE skill prompt rule for layer coverage.
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
		if rm.EnumerationBoundary != nil && rm.EnumerationBoundary.DeclaredCount > 0 {
			validators = append(validators, CardinalityValidator{})
		}
	case types.QFCallChain:
		validators = append(validators, PathDepthValidator{})
	case types.QFComparison:
		if len(rm.Buckets) >= 2 {
			validators = append(validators, EntityParityValidator{})
		}
	}

	return validators
}

// RunFamilyValidators executes every applicable validator for the
// given family and returns the first non-nil failure encountered.
// nil result means all Tier 2 checks passed.
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
// question answer to be backed by deterministic-tool output.
//
// Phase 2.B answer-aware 3-tier input:
//
//	Tier 1: AnswerDocumentV2.Blocks where Kind=BlockScalar AND its
//	    Items[].CitationRef points to a Citation whose evidence
//	    chain ultimately came from a successful deterministic count
//	    measurement.
//
//	Tier 2: any block.Text in the answer contains an integer
//	    literal AND ≥1 successful ToolResult carries a typed
//	    count measurement.
//
//	Tier 3: rejected (no typed evidence at all).
type ScalarCountValidator struct{}

func (ScalarCountValidator) Dimension() CompletionDimension { return DimensionScalarCount }
func (ScalarCountValidator) ViolationKind() types.ViolationKind {
	return types.ViolScalarCountUnsourced
}

func (ScalarCountValidator) Validate(input ValidatorInput) *CompletenessFailure {
	if input.IR == nil || !input.IR.RequestModel.Predicates.IsCountQuestion {
		return nil
	}
	// Syntactic vs semantic count carve-out (Phase 2.C audit,
	// 2026-05-09): a count question that is ALSO a relational lookup
	// is a SEMANTIC count — the answer is the size of a set whose
	// members are filtered by a property the LLM has to READ code to
	// determine ("how many agents can call sub-agents", "how many
	// endpoints accept JSON", "几个 config key 被废弃"). For these
	// shapes the count cannot come from a single deterministic tool
	// call; it comes from enumerating the set, then counting. The
	// enumeration path (path c/d in expandEntitiesWithImplementers
	// + EXHAUSTIVE ENUMERATION skill rule) covers the semantic case.
	// Skipping ScalarCountValidator here prevents the validator from
	// over-firing on semantically-derived counts that have no
	// `grep -c`-shaped tool output.
	//
	// Syntactic counts (file count / LOC / regex match count) keep
	// IsRelationalLookup=false and continue to require deterministic
	// count measurement output — the validator's original target.
	if input.IR.RequestModel.Predicates.IsRelationalLookup {
		return nil
	}

	// Tier 0: model-authored aggregate handoff. For semantic counts
	// whose exact value is the size of a verified member set, forcing
	// an additional counting command is both wasteful and sometimes the
	// wrong abstraction: the explorer already emitted the exact
	// member_set/total_count through structured aggregate_facts after
	// reading and classifying candidates. This still does not let the
	// system invent an answer: the value must come from accepted
	// aggregate_facts, and the composed answer must visibly contain
	// the same integer.
	if aggregateFactsProvideVisibleCount(input.AggregateFacts, input.AnswerDocumentV2) {
		return nil
	}

	// Tier 1: answer-aware via typed BlockScalar + citation chain.
	// If the answer surfaces a count via a BlockScalar block AND has
	// at least one successful typed count measurement, the count has
	// deterministic-tool backing.
	if input.AnswerDocumentV2 != nil {
		hasScalarBlock := false
		hasIntegerInAnswer := false
		for _, blk := range input.AnswerDocumentV2.Blocks {
			if blk.Kind == types.BlockScalar {
				hasScalarBlock = true
				if hasIntegerLiteral(blk.Text) {
					hasIntegerInAnswer = true
				}
				for _, item := range blk.Items {
					if hasIntegerLiteral(item.Label) || hasIntegerLiteral(item.Text) {
						hasIntegerInAnswer = true
					}
				}
			}
		}
		// Tier 1 PASS condition: BlockScalar present AND there's at
		// least one successful typed count measurement. The strict
		// citation-chain link (CitationRef → ToolResult lineage) is not currently
		// tracked in V2; we approximate by combining the BlockScalar
		// presence with the typed measurement presence.
		if hasScalarBlock && hasIntegerInAnswer && hasDeterministicCountMeasurementResult(input.ToolResults) {
			return nil
		}
	}

	// Tier 2: prose-fallback — answer contains an integer somewhere
	// AND ToolResults has a typed count measurement.
	if input.AnswerDocumentV2 != nil && hasDeterministicCountMeasurementResult(input.ToolResults) {
		// Walk all blocks for any integer literal in prose. If
		// found, accept — the count likely came from the tool.
		for _, blk := range input.AnswerDocumentV2.Blocks {
			if hasIntegerLiteral(blk.Text) {
				return nil
			}
			for _, item := range blk.Items {
				if hasIntegerLiteral(item.Label) || hasIntegerLiteral(item.Text) {
					return nil
				}
			}
		}
	}

	// Tier 3 fallback: original Phase 2 evidence-pool check (kept
	// for backwards compat when AnswerDocumentV2 is nil — e.g., in
	// unit tests).
	if input.AnswerDocumentV2 == nil {
		if hasDeterministicCountMeasurementResult(input.ToolResults) {
			return nil
		}
	}

	return &CompletenessFailure{
		Dimension:     DimensionScalarCount,
		ViolationKind: types.ViolScalarCountUnsourced,
		Reason:        "count question answer not backed by deterministic-tool output",
		FixHint:       "This is a counting question. Run a deterministic counting command (such as `grep -c <pattern> <path>` / `find <path> | wc -l` / `wc -l <files>` on Unix-like systems, or your platform's equivalent) and use that exact integer as the answer. Do NOT count by reading a file and tallying matches yourself — visual counts go off-by-one easily on long files, on multi-block declarations spread across blank lines, and on patterns that overlap.",
	}
}

// hasDeterministicCountMeasurementResult reports whether ToolResults contains
// at least one successful typed count measurement. This helper is used by a
// post-finalize hard gate, so it must not inspect rendered ToolResult.Summary.
func hasDeterministicCountMeasurementResult(results []types.ToolResult) bool {
	for _, tr := range results {
		if !tr.Success {
			continue
		}
		measurement := tr.CommandMeasurement
		if measurement == nil ||
			measurement.Kind != types.ToolCommandMeasurementKindCount ||
			measurement.Value < 0 {
			continue
		}
		switch measurement.Origin {
		case types.AnswerEvidenceOriginCommandMeasurement:
			return true
		case types.AnswerEvidenceOriginVCSMetadata:
			if measurement.History {
				return true
			}
		}
	}
	return false
}

func aggregateFactsProvideVisibleCount(facts []types.AnswerAggregateFact, doc *types.AnswerDocumentV2) bool {
	if len(facts) == 0 || doc == nil {
		return false
	}
	visible := answerDocumentIntegerSet(doc)
	if len(visible) == 0 {
		return false
	}
	for _, fact := range facts {
		n, ok := aggregateFactExactCount(fact)
		if !ok {
			continue
		}
		if visible[n] {
			return true
		}
	}
	return false
}

func aggregateFactExactCount(fact types.AnswerAggregateFact) (int, bool) {
	value := strings.TrimSpace(fact.Value)
	if value == "" {
		return 0, false
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0, false
	}
	switch fact.Kind {
	case types.AnswerAggregateMemberSet:
		if len(fact.Members) == 0 || len(fact.Members) != n {
			return 0, false
		}
		return n, true
	case types.AnswerAggregateTotalCount,
		types.AnswerAggregateUniqueCount,
		types.AnswerAggregateGroupedCount,
		types.AnswerAggregateBucketCount,
		types.AnswerAggregateExcluded,
		types.AnswerAggregateScalar:
		if len(fact.Members) > 0 && len(fact.Members) != n {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

func answerDocumentIntegerSet(doc *types.AnswerDocumentV2) map[int]bool {
	out := map[int]bool{}
	if doc == nil {
		return out
	}
	collect := func(text string) {
		for _, n := range integerLiteralsInText(text) {
			out[n] = true
		}
	}
	for _, blk := range doc.Blocks {
		collect(blk.Text)
		collect(blk.Title)
		for _, item := range blk.Items {
			collect(item.Label)
			collect(item.Text)
		}
	}
	return out
}

func integerLiteralsInText(text string) []int {
	var out []int
	start := -1
	flush := func(end int) {
		if start < 0 {
			return
		}
		n, err := strconv.Atoi(text[start:end])
		if err == nil {
			out = append(out, n)
		}
		start = -1
	}
	for i, r := range text {
		if r >= '0' && r <= '9' {
			if start < 0 {
				start = i
			}
			continue
		}
		flush(i)
	}
	flush(len(text))
	return out
}

// --- CardinalityValidator ---------------------------------------

// CardinalityValidator (DimensionCardinality) requires the answer's
// enumerated item count to satisfy DeclaredCount.
//
// Phase 2.B answer-aware 3-tier input:
//
//	Tier 1: AnswerDocumentV2 — sum of Items[] across enumeration-
//	    shaped blocks (BlockBulletList / BlockOrderedList) ≥
//	    DeclaredCount.
//
//	Tier 2: prose-list count via markdown list-item regex on
//	    block.Text.
//
//	Tier 3: len(AnswerSymbols) ≥ DeclaredCount (original Phase 2
//	    fallback).
type CardinalityValidator struct{}

func (CardinalityValidator) Dimension() CompletionDimension { return DimensionCardinality }
func (CardinalityValidator) ViolationKind() types.ViolationKind {
	return types.ViolCardinalityShort
}

func (CardinalityValidator) Validate(input ValidatorInput) *CompletenessFailure {
	if input.IR == nil {
		return nil
	}
	rm := input.IR.RequestModel
	if rm.EnumerationBoundary == nil || rm.EnumerationBoundary.DeclaredCount <= 0 {
		return nil
	}
	declared := rm.EnumerationBoundary.DeclaredCount

	if aggregateFactsProvideVisibleMemberCardinality(input.AggregateFacts, input.AnswerDocumentV2, &rm, declared) {
		return nil
	}

	// Tier 1: typed BlockBulletList / BlockOrderedList items.
	if input.AnswerDocumentV2 != nil {
		typedItems := 0
		for _, blk := range input.AnswerDocumentV2.Blocks {
			if blk.Kind == types.BlockBulletList || blk.Kind == types.BlockOrderedList {
				typedItems += len(blk.Items)
			}
		}
		if typedItems >= declared {
			return nil
		}

		// Tier 2: prose markdown list-item count across all blocks.
		proseItems := 0
		for _, blk := range input.AnswerDocumentV2.Blocks {
			proseItems += countMarkdownListItems(blk.Text)
			for _, item := range blk.Items {
				proseItems += countMarkdownListItems(item.Text)
			}
		}
		if proseItems >= declared {
			return nil
		}
	}

	// Tier 3: AnswerSymbols slate (original Phase 2 fallback).
	if len(input.AnswerSymbols) >= declared {
		return nil
	}

	return &CompletenessFailure{
		Dimension:     DimensionCardinality,
		ViolationKind: types.ViolCardinalityShort,
		Family:        types.QFEnumeration,
		Reason:        "enumerated item count below declared boundary",
		FixHint:       "The question stated an explicit count, but the answer surfaces fewer items than that count requires. Continue investigation to find the missing items, or, if no further items can be located, explicitly acknowledge the count mismatch in the answer (e.g., 'the question mentions N but only M are present in the codebase').",
	}
}

func aggregateFactsProvideVisibleMemberCardinality(facts []types.AnswerAggregateFact, doc *types.AnswerDocumentV2, rm *types.RequestModel, declared int) bool {
	if declared <= 0 || len(facts) == 0 || doc == nil || rm == nil {
		return false
	}
	for _, ref := range types.PrincipalAggregateMemberSetFactRefsForRequest(facts, rm) {
		if len(ref.Fact.Members) < declared {
			continue
		}
		visible := 0
		for _, member := range ref.Fact.Members {
			if answerDocumentMentionsAggregateMember(doc, member) {
				visible++
			}
			if visible >= declared {
				return true
			}
		}
	}
	return false
}

func answerDocumentMentionsAggregateMember(doc *types.AnswerDocumentV2, member string) bool {
	member = strings.TrimSpace(member)
	if doc == nil || member == "" {
		return false
	}
	ob := types.AnswerSupportMemberObligation{
		Label:        member,
		SurfaceTerms: []string{member},
	}
	for _, block := range doc.Blocks {
		if types.AnswerTextMentionsSupportMember(types.AnswerBlockVisibleSurface(block), ob) {
			return true
		}
	}
	return false
}

// countMarkdownListItems counts lines beginning with a markdown
// list marker (`- `, `* `, `<digit>. `). Used by Tier 2 prose
// fallback in CardinalityValidator.
func countMarkdownListItems(s string) int {
	if s == "" {
		return 0
	}
	count := 0
	for _, line := range strings.Split(s, "\n") {
		trim := strings.TrimLeft(line, " \t")
		if trim == "" {
			continue
		}
		if strings.HasPrefix(trim, "- ") || strings.HasPrefix(trim, "* ") {
			count++
			continue
		}
		// Numbered list: leading digit run + dot + space.
		dotAt := strings.Index(trim, ". ")
		if dotAt > 0 {
			isAllDigits := true
			for i := 0; i < dotAt; i++ {
				c := trim[i]
				if c < '0' || c > '9' {
					isAllDigits = false
					break
				}
			}
			if isAllDigits {
				count++
			}
		}
	}
	return count
}

// --- PathDepthValidator -----------------------------------------

// PathDepthValidator (DimensionPathDepth) requires call-chain
// answers to evidence both entry and exit functions PLUS at least
// 3 distinct mid-segment functions.
//
// Phase 2.B answer-aware 3-tier input:
//
//	Tier 1: AnswerDocumentV2 — distinct identifier-shaped tokens in
//	    BlockOrderedList Items + EdgeAnchors / ClaimUses with
//	    function-related FacetIDs.
//
//	Tier 2: distinct identifier-shaped tokens scanned from all
//	    block.Text + items[].Label / items[].Text.
//
//	Tier 3: distinct AnchorSymbol from EvidenceItems (original
//	    Phase 2 fallback).
type PathDepthValidator struct{}

func (PathDepthValidator) Dimension() CompletionDimension { return DimensionPathDepth }
func (PathDepthValidator) ViolationKind() types.ViolationKind {
	return types.ViolPathDepthInsufficient
}

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
	required := len(entities) + 3

	// Tier 1: typed BlockOrderedList items + EdgeAnchors.
	if input.AnswerDocumentV2 != nil {
		mentions := make(map[string]struct{})
		for _, blk := range input.AnswerDocumentV2.Blocks {
			if blk.Kind == types.BlockOrderedList {
				for _, item := range blk.Items {
					addIdentifierTokens(mentions, item.Label)
					addIdentifierTokens(mentions, item.Text)
				}
			}
			// EdgeAnchors carry typed function endpoints in diagram
			// blocks.
			for _, ea := range blk.EdgeAnchors {
				if ea.FromNode != "" {
					mentions[ea.FromNode] = struct{}{}
				}
				if ea.ToNode != "" {
					mentions[ea.ToNode] = struct{}{}
				}
			}
		}
		if len(mentions) >= required {
			return nil
		}

		// Tier 2: prose scan across all block text + items.
		for _, blk := range input.AnswerDocumentV2.Blocks {
			addIdentifierTokens(mentions, blk.Text)
			for _, item := range blk.Items {
				addIdentifierTokens(mentions, item.Label)
				addIdentifierTokens(mentions, item.Text)
			}
		}
		if len(mentions) >= required {
			return nil
		}
	}

	// Tier 3: evidence pool AnchorSymbol fallback.
	mentions := make(map[string]struct{})
	for _, ev := range input.EvidenceItems {
		anchor := strings.TrimSpace(ev.AnchorSymbol)
		if anchor == "" || !looksLikeFunctionSymbol(anchor) {
			continue
		}
		mentions[anchor] = struct{}{}
	}
	if len(mentions) >= required {
		return nil
	}

	missingHint := " The current evidence covers " +
		itoa(len(mentions)) + " distinct functions; this kind of question needs at least " +
		itoa(required) + "."

	return &CompletenessFailure{
		Dimension:     DimensionPathDepth,
		ViolationKind: types.ViolPathDepthInsufficient,
		Family:        types.QFCallChain,
		Reason:        "call-chain evidence does not cover entry → mid-segment → exit",
		FixHint:       "This is a call-chain or processing-pipeline question. The answer must trace from the entry function through the mid-segment processing all the way to the exit function — list every load-bearing intermediate function the data passes through, not just the first two or three." + missingHint + " If the entry function is large (>500 lines), read the full body before stopping; load-bearing pipeline steps are often scattered across the function.",
	}
}

// addIdentifierTokens splits s on whitespace + common punctuation
// (`,` `.` `(` `)` `[` `]` `<` `>` ``` `:` `;` `=` `+` `-` `*` `/`
// `?` `!`) and adds every token that passes looksLikeFunctionSymbol
// to the dedupe map. Used by PathDepthValidator's Tier 1 + Tier 2.
func addIdentifierTokens(out map[string]struct{}, s string) {
	if s == "" {
		return
	}
	splitter := func(r rune) bool {
		switch r {
		case ' ', '\t', '\n', '\r', ',', '.', '(', ')', '[', ']',
			'<', '>', '`', ':', ';', '=', '+', '-', '*', '/', '?', '!',
			'"', '\'', '|', '&', '\\':
			return true
		}
		return false
	}
	// Custom splitter via FieldsFunc (Go's strings.FieldsFunc).
	for _, tok := range strings.FieldsFunc(s, splitter) {
		// "*Type" — strip the leading * via TrimLeft. The function
		// itself already handles &, * prefixes.
		if looksLikeFunctionSymbol(tok) {
			out[strings.TrimSpace(tok)] = struct{}{}
		}
	}
}

// --- EntityParityValidator --------------------------------------

// EntityParityValidator (DimensionEntityParity) checks per-bucket
// evidence sampling balance.
//
// Phase 2.B answer-aware 3-tier input:
//
//	Tier 1: AnswerDocumentV2 — per-bucket evidence count via
//	    Buckets[].Anchors substring match in block.Text.
//
//	Tier 2: per-bucket prose word count proxy (longer prose →
//	    more attention).
//
//	Tier 3: per-bucket EvidenceItem AnchorSymbol matches (original
//	    Phase 2 fallback).
type EntityParityValidator struct{}

func (EntityParityValidator) Dimension() CompletionDimension { return DimensionEntityParity }
func (EntityParityValidator) ViolationKind() types.ViolationKind {
	return types.ViolEntityParityImbalanced
}

func (EntityParityValidator) Validate(input ValidatorInput) *CompletenessFailure {
	if input.IR == nil {
		return nil
	}
	buckets := input.IR.RequestModel.Buckets
	if len(buckets) < 2 {
		return nil
	}
	if aggregateFactsCoverComparisonBuckets(input.AggregateFacts, buckets) {
		return nil
	}

	// Tier 1: typed-block + anchor scan.
	if input.AnswerDocumentV2 != nil {
		counts := make([]int, len(buckets))
		for _, blk := range input.AnswerDocumentV2.Blocks {
			for i, b := range buckets {
				for _, anchor := range b.Anchors {
					a := strings.TrimSpace(anchor)
					if a == "" {
						continue
					}
					counts[i] += countOccurrencesCaseInsensitive(blk.Text, a)
					for _, item := range blk.Items {
						counts[i] += countOccurrencesCaseInsensitive(item.Label, a)
						counts[i] += countOccurrencesCaseInsensitive(item.Text, a)
					}
				}
			}
		}
		if !isLopsided(counts) {
			return nil
		}
	}

	// Tier 3 (and fallback for when V2 is nil or Tier 1 deemed lopsided —
	// confirm via evidence pool before reporting).
	counts := make([]int, len(buckets))
	for _, ev := range input.EvidenceItems {
		anchor := strings.ToLower(strings.TrimSpace(ev.AnchorSymbol))
		if anchor == "" {
			continue
		}
		for i, b := range buckets {
			matched := false
			for _, e := range b.Anchors {
				if strings.EqualFold(strings.TrimSpace(e), anchor) {
					counts[i]++
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
	}
	if !isLopsided(counts) {
		return nil
	}

	return &CompletenessFailure{
		Dimension:     DimensionEntityParity,
		ViolationKind: types.ViolEntityParityImbalanced,
		Family:        types.QFComparison,
		Reason:        "comparison buckets sampled with imbalanced evidence",
		FixHint:       "This is a comparison question. The evidence is heavily skewed to one bucket; continue investigation to gather comparable evidence for the under-sampled bucket(s) so the comparison sides are equally well-grounded.",
	}
}

func aggregateFactsCoverComparisonBuckets(facts []types.AnswerAggregateFact, buckets []types.QuestionBucket) bool {
	if len(facts) == 0 || len(buckets) < 2 {
		return false
	}
	for _, bucket := range buckets {
		if !aggregateFactsCoverComparisonBucket(facts, bucket) {
			return false
		}
	}
	return true
}

func aggregateFactsCoverComparisonBucket(facts []types.AnswerAggregateFact, bucket types.QuestionBucket) bool {
	for _, fact := range facts {
		if fact.Kind != types.AnswerAggregateMemberSet || len(fact.Members) == 0 {
			continue
		}
		if aggregateFactMatchesComparisonBucket(fact, bucket) {
			return true
		}
	}
	return false
}

func aggregateFactMatchesComparisonBucket(fact types.AnswerAggregateFact, bucket types.QuestionBucket) bool {
	candidates := append([]string{bucket.Label}, bucket.Anchors...)
	surfaces := []string{fact.Label}
	for _, dim := range fact.Dimensions {
		surfaces = append(surfaces, dim.Value)
	}
	for _, candidate := range candidates {
		for _, surface := range surfaces {
			if typedBucketSurfaceMatch(candidate, surface) {
				return true
			}
		}
	}
	return false
}

func typedBucketSurfaceMatch(bucket, surface string) bool {
	bucket = strings.TrimSpace(bucket)
	surface = strings.TrimSpace(surface)
	if bucket == "" || surface == "" {
		return false
	}
	b := strings.ToLower(bucket)
	s := strings.ToLower(surface)
	if b == s {
		return true
	}
	if strings.HasPrefix(s, b) && bucketSurfaceBoundaryAfter(s, len(b)) {
		return true
	}
	if strings.HasPrefix(b, s) && bucketSurfaceBoundaryAfter(b, len(s)) {
		return true
	}
	return false
}

func bucketSurfaceBoundaryAfter(s string, n int) bool {
	if n >= len(s) {
		return true
	}
	if s[n] >= 0x80 {
		return true
	}
	return strings.ContainsRune(" \t\r\n:/\\|,;()[]{}<>-_.", rune(s[n]))
}

// isLopsided reports whether the smallest count is below 50% of the
// largest. Empty / all-zero counts return false (no signal).
func isLopsided(counts []int) bool {
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
		return false
	}
	return minCount*2 < maxCount
}

// countOccurrencesCaseInsensitive counts non-overlapping
// case-insensitive occurrences of needle in haystack.
func countOccurrencesCaseInsensitive(haystack, needle string) int {
	if needle == "" || haystack == "" {
		return 0
	}
	return strings.Count(strings.ToLower(haystack), strings.ToLower(needle))
}

// --- helpers ----------------------------------------------------

// hasIntegerLiteral returns true when s contains a digit somewhere.
// Used by ScalarCountValidator for prose / Tier 2 / fallback.
func hasIntegerLiteral(s string) bool {
	for _, r := range s {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

// looksLikeFunctionSymbol returns true when s is a non-path
// identifier-like token. Cross-language: accepts CamelCase /
// camelCase / PascalCase / snake_case / qualified forms; rejects
// file paths and bare basenames ending in short lowercase
// extensions.
func looksLikeFunctionSymbol(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	for len(s) > 0 && (s[0] == '*' || s[0] == '&') {
		s = s[1:]
	}
	if s == "" {
		return false
	}
	if strings.Contains(s, "/") {
		return false
	}
	if dotAt := strings.LastIndexByte(s, '.'); dotAt > 0 && dotAt < len(s)-1 {
		ext := s[dotAt+1:]
		if len(ext) <= 4 && allLowerLetters(ext) {
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

// itoa is a small ASCII int → string helper used by validator
// failure messages.
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
