package types

import (
	"fmt"
	"sort"
	"strings"
)

// WriteBehaviorContract is a typed observable that the write workflow should
// preserve or satisfy. The write_analyzer emits these atoms through
// emit_write_analysis; downstream validators only check IDs/enums/coverage
// relationships and never infer contract semantics from prose.
type WriteBehaviorContract struct {
	ID          string                      `json:"id"`
	Kind        WriteBehaviorContractKind   `json:"kind"`
	Polarity    WriteBehaviorPolarity       `json:"polarity,omitempty"`
	Subject     string                      `json:"subject,omitempty"`
	Operator    WriteBehaviorOperator       `json:"operator,omitempty"`
	Expected    string                      `json:"expected,omitempty"`
	Transition  *WriteBehaviorTransition    `json:"transition,omitempty"`
	Placement   *WriteRenderedTextPlacement `json:"placement,omitempty"`
	Comparator  *WriteBehaviorComparator    `json:"comparator,omitempty"`
	EvidenceRef string                      `json:"evidence_ref,omitempty"`
	Required    bool                        `json:"required,omitempty"`
	Source      string                      `json:"source,omitempty"`
}

// WriteBehaviorTransition carries an ordered state/protocol witness without
// asking downstream code to recover ordering from model prose. It is optional
// and advisory: validators preserve the typed phases and probes may bind the
// parent contract ID, but only an executed probe/project runner can establish
// that the changed production code satisfies the sequence.
type WriteBehaviorTransition struct {
	Steps []WriteBehaviorTransitionStep `json:"steps,omitempty"`
}

// WriteBehaviorTransitionStep is one ordered part of a stateful behavior.
// Operation names the call/event/state mutation; Expected is the observable
// result or state after that step. Keeping both fields allows setup/action
// steps and observation/postcondition steps to share one compact carrier.
type WriteBehaviorTransitionStep struct {
	Phase       WriteBehaviorTransitionPhase `json:"phase"`
	Operation   string                       `json:"operation,omitempty"`
	Expected    string                       `json:"expected,omitempty"`
	EvidenceRef string                       `json:"evidence_ref,omitempty"`
}

type WriteBehaviorTransitionPhase string

const (
	WriteBehaviorTransitionSetup         WriteBehaviorTransitionPhase = "setup"
	WriteBehaviorTransitionAction        WriteBehaviorTransitionPhase = "action"
	WriteBehaviorTransitionObservation   WriteBehaviorTransitionPhase = "observation"
	WriteBehaviorTransitionPostcondition WriteBehaviorTransitionPhase = "postcondition"
)

// WriteRenderedTextPlacement describes a line-local rendered-output placement
// obligation. It is a typed contract surface shared by Python reprs, JS CLI
// lines, Go String() output, Java toString(), and UI snapshots. System hard
// gates read this struct only; they must not infer placement from issue prose,
// model rationale, terminal narratives, or prompt text.
type WriteRenderedTextPlacement struct {
	Surface     WriteRenderedTextSurface  `json:"surface,omitempty"`
	Anchor      string                    `json:"anchor,omitempty"`
	Expected    string                    `json:"expected,omitempty"`
	Relation    WriteRenderedTextRelation `json:"relation,omitempty"`
	Delimiter   string                    `json:"delimiter,omitempty"`
	EvidenceRef string                    `json:"evidence_ref,omitempty"`
}

// WriteBehaviorComparator ties an expected behavior contract to a grounded
// reference surface that is already known to work or intentionally contrasts
// with the failing surface. It is carried as typed context so later probes can
// assert the relationship without control flow parsing issue prose.
type WriteBehaviorComparator struct {
	Subject     string                          `json:"subject,omitempty"`
	Operator    WriteBehaviorOperator           `json:"operator,omitempty"`
	Expected    string                          `json:"expected,omitempty"`
	Relation    WriteBehaviorComparatorRelation `json:"relation,omitempty"`
	EvidenceRef string                          `json:"evidence_ref,omitempty"`
}

type WriteBehaviorContractKind string

const (
	WriteBehaviorObservable    WriteBehaviorContractKind = "observable"
	WriteBehaviorException     WriteBehaviorContractKind = "exception"
	WriteBehaviorOutputPath    WriteBehaviorContractKind = "output_path"
	WriteBehaviorStdout        WriteBehaviorContractKind = "stdout"
	WriteBehaviorStatusCode    WriteBehaviorContractKind = "status_code"
	WriteBehaviorFileLayout    WriteBehaviorContractKind = "file_layout"
	WriteBehaviorCommandResult WriteBehaviorContractKind = "command_result"
	WriteBehaviorInvariant     WriteBehaviorContractKind = "invariant"
)

type WriteBehaviorOperator string

const (
	WriteBehaviorOpSatisfies   WriteBehaviorOperator = "satisfies"
	WriteBehaviorOpEquals      WriteBehaviorOperator = "equals"
	WriteBehaviorOpNotEquals   WriteBehaviorOperator = "not_equals"
	WriteBehaviorOpContains    WriteBehaviorOperator = "contains"
	WriteBehaviorOpNotContains WriteBehaviorOperator = "not_contains"
	WriteBehaviorOpExists      WriteBehaviorOperator = "exists"
	WriteBehaviorOpNotExists   WriteBehaviorOperator = "not_exists"
	WriteBehaviorOpRaises      WriteBehaviorOperator = "raises"
	WriteBehaviorOpNotRaises   WriteBehaviorOperator = "not_raises"
	WriteBehaviorOpReturns     WriteBehaviorOperator = "returns"
)

type WriteBehaviorPolarity string

const (
	WriteBehaviorPolarityExpected  WriteBehaviorPolarity = "expected"
	WriteBehaviorPolarityForbidden WriteBehaviorPolarity = "forbidden"
	WriteBehaviorPolarityObserved  WriteBehaviorPolarity = "observed"
)

const (
	WriteBehaviorContractSourceExpectedOutcomeFallback = "expected_outcome_fallback"
	WriteBehaviorContractSourcePlanAcceptanceFallback  = "plan_acceptance_fallback"
	// WriteBehaviorContractSourcePlanningOnlyUngrounded is stamped by the
	// orchestrator quality calibration when an analyzer-authored example has no
	// exact request or typed evidence authority. The atom remains useful to the
	// controller/planner as a proposed boundary to investigate, but it must not
	// become a verifier completion obligation merely because it was schema-valid.
	WriteBehaviorContractSourcePlanningOnlyUngrounded = "quality_repaired:planning_only_ungrounded"
)

type WriteBehaviorContractGeneration string

const WriteBehaviorContractGenerationPlanAcceptanceRebase WriteBehaviorContractGeneration = "plan_acceptance_rebase"

type WriteBehaviorComparatorRelation string

const (
	WriteBehaviorComparatorSameAs             WriteBehaviorComparatorRelation = "same_as"
	WriteBehaviorComparatorConsistentWith     WriteBehaviorComparatorRelation = "consistent_with"
	WriteBehaviorComparatorContrastsWith      WriteBehaviorComparatorRelation = "contrasts_with"
	WriteBehaviorComparatorRegressionBaseline WriteBehaviorComparatorRelation = "regression_baseline"
)

type WriteRenderedTextSurface string

const (
	WriteRenderedTextSurfaceRepr         WriteRenderedTextSurface = "repr"
	WriteRenderedTextSurfaceStdoutLine   WriteRenderedTextSurface = "stdout_line"
	WriteRenderedTextSurfaceCLILine      WriteRenderedTextSurface = "cli_line"
	WriteRenderedTextSurfaceStringer     WriteRenderedTextSurface = "stringer"
	WriteRenderedTextSurfaceUIText       WriteRenderedTextSurface = "ui_text"
	WriteRenderedTextSurfaceSnapshotText WriteRenderedTextSurface = "snapshot_text"
)

type WriteRenderedTextRelation string

const (
	WriteRenderedTextAfterAnchor               WriteRenderedTextRelation = "after_anchor"
	WriteRenderedTextBeforeAnchor              WriteRenderedTextRelation = "before_anchor"
	WriteRenderedTextSuffixBeforeDelimiter     WriteRenderedTextRelation = "suffix_before_delimiter"
	WriteRenderedTextPrefixAfterDelimiter      WriteRenderedTextRelation = "prefix_after_delimiter"
	WriteRenderedTextBetweenAnchorAndDelimiter WriteRenderedTextRelation = "between_anchor_and_delimiter"
	WriteRenderedTextSameLineContains          WriteRenderedTextRelation = "same_line_contains"
	WriteRenderedTextLineLocalNotContains      WriteRenderedTextRelation = "line_local_not_contains"
)

// AllWriteBehaviorContractKinds is the closed kind set in stable order — the
// single source for IsKnownWriteBehaviorContractKind, the witness matrix
// (write_behavior_contract_witness.go), the analyzer schema enum and the
// kind teaching sentence. Add a kind here and every surface follows.
func AllWriteBehaviorContractKinds() []WriteBehaviorContractKind {
	return []WriteBehaviorContractKind{
		WriteBehaviorObservable, WriteBehaviorException, WriteBehaviorOutputPath,
		WriteBehaviorStdout, WriteBehaviorStatusCode, WriteBehaviorFileLayout,
		WriteBehaviorCommandResult, WriteBehaviorInvariant,
	}
}

func IsKnownWriteBehaviorContractKind(v string) bool {
	for _, kind := range AllWriteBehaviorContractKinds() {
		if string(kind) == v {
			return true
		}
	}
	return false
}

func IsKnownWriteBehaviorOperator(v string) bool {
	switch WriteBehaviorOperator(v) {
	case WriteBehaviorOpSatisfies, WriteBehaviorOpEquals, WriteBehaviorOpNotEquals,
		WriteBehaviorOpContains, WriteBehaviorOpNotContains, WriteBehaviorOpExists,
		WriteBehaviorOpNotExists, WriteBehaviorOpRaises, WriteBehaviorOpNotRaises,
		WriteBehaviorOpReturns:
		return true
	default:
		return false
	}
}

func IsKnownWriteBehaviorPolarity(v string) bool {
	switch WriteBehaviorPolarity(v) {
	case WriteBehaviorPolarityExpected, WriteBehaviorPolarityForbidden, WriteBehaviorPolarityObserved:
		return true
	default:
		return false
	}
}

func IsKnownWriteBehaviorComparatorRelation(v string) bool {
	switch WriteBehaviorComparatorRelation(v) {
	case WriteBehaviorComparatorSameAs, WriteBehaviorComparatorConsistentWith,
		WriteBehaviorComparatorContrastsWith, WriteBehaviorComparatorRegressionBaseline:
		return true
	default:
		return false
	}
}

func IsKnownWriteBehaviorTransitionPhase(v string) bool {
	switch WriteBehaviorTransitionPhase(v) {
	case WriteBehaviorTransitionSetup, WriteBehaviorTransitionAction,
		WriteBehaviorTransitionObservation, WriteBehaviorTransitionPostcondition:
		return true
	default:
		return false
	}
}

func IsKnownWriteRenderedTextSurface(v string) bool {
	switch WriteRenderedTextSurface(v) {
	case WriteRenderedTextSurfaceRepr,
		WriteRenderedTextSurfaceStdoutLine,
		WriteRenderedTextSurfaceCLILine,
		WriteRenderedTextSurfaceStringer,
		WriteRenderedTextSurfaceUIText,
		WriteRenderedTextSurfaceSnapshotText:
		return true
	default:
		return false
	}
}

func IsKnownWriteRenderedTextRelation(v string) bool {
	switch WriteRenderedTextRelation(v) {
	case WriteRenderedTextAfterAnchor,
		WriteRenderedTextBeforeAnchor,
		WriteRenderedTextSuffixBeforeDelimiter,
		WriteRenderedTextPrefixAfterDelimiter,
		WriteRenderedTextBetweenAnchorAndDelimiter,
		WriteRenderedTextSameLineContains,
		WriteRenderedTextLineLocalNotContains:
		return true
	default:
		return false
	}
}

// NormalizeWriteBehaviorContracts validates and normalizes analyzer-emitted
// contract atoms. When no structured atoms are emitted but expected_outcomes
// exist, it creates generic observable atoms that preserve the outcome text
// without attempting semantic parsing.
func NormalizeWriteBehaviorContracts(in []WriteBehaviorContract, expectedOutcomes []string) []WriteBehaviorContract {
	return normalizeWriteBehaviorContractsReserving(in, expectedOutcomes, nil)
}

// normalizeWriteBehaviorContractsReserving is NormalizeWriteBehaviorContracts
// with a reserved id set: a fallback row minted for expectedOutcomes never
// reuses a reserved id. A verify-failure rebase reserves its tombstoned ids so
// the new generation's `outcome-N` row cannot collide with a tombstone that
// would then shadow it out of every verification view (§40.24 second finding).
func normalizeWriteBehaviorContractsReserving(in []WriteBehaviorContract, expectedOutcomes []string, reserved []string) []WriteBehaviorContract {
	out := make([]WriteBehaviorContract, 0, len(in)+len(expectedOutcomes))
	seen := map[string]struct{}{}
	for _, id := range reserved {
		if id = strings.TrimSpace(id); id != "" {
			seen[id] = struct{}{}
		}
	}
	seenExpected := map[string]struct{}{}
	for i, c := range in {
		c.ID = strings.TrimSpace(c.ID)
		if c.ID == "" {
			c.ID = fmt.Sprintf("contract-%d", i+1)
		}
		c.Kind = WriteBehaviorContractKind(strings.ToLower(strings.TrimSpace(string(c.Kind))))
		if !IsKnownWriteBehaviorContractKind(string(c.Kind)) {
			c.Kind = WriteBehaviorObservable
		}
		c.Polarity = WriteBehaviorPolarity(strings.ToLower(strings.TrimSpace(string(c.Polarity))))
		if !IsKnownWriteBehaviorPolarity(string(c.Polarity)) {
			c.Polarity = WriteBehaviorPolarityExpected
		}
		c.Source = strings.TrimSpace(c.Source)
		if c.Source == "" {
			c.Source = "write_analyzer"
		}
		// expected_outcomes[] and plan acceptance_tests[] are model-authored
		// summaries. Preserve them for planning, but never mint verifier
		// completion authority from prose merely because it decoded into the
		// schema. Exact project receipts or evidence-backed typed contracts must
		// carry hard completion authority.
		if IsExpectedOutcomeFallbackWriteBehaviorContract(c) {
			c.Source = appendWriteBehaviorContractSource(c.Source, WriteBehaviorContractSourcePlanningOnlyUngrounded)
		}
		if c.Polarity == WriteBehaviorPolarityObserved || IsPlanningOnlyWriteBehaviorContract(c) {
			c.Required = false
		} else {
			c.Required = true
		}
		c.Operator = WriteBehaviorOperator(strings.ToLower(strings.TrimSpace(string(c.Operator))))
		if !IsKnownWriteBehaviorOperator(string(c.Operator)) {
			c.Operator = WriteBehaviorOpSatisfies
		}
		c.Subject = strings.TrimSpace(c.Subject)
		c.Expected = strings.TrimSpace(c.Expected)
		c.Transition = NormalizeWriteBehaviorTransition(c.Transition)
		c.Placement = NormalizeWriteRenderedTextPlacement(c.Placement, c.Expected)
		if c.Expected == "" && c.Placement != nil {
			c.Expected = c.Placement.Expected
		}
		c.Comparator = normalizeWriteBehaviorComparator(c.Comparator, c.Operator)
		c.EvidenceRef = strings.TrimSpace(c.EvidenceRef)
		if c.Expected == "" && c.Subject == "" && c.Transition == nil && c.Placement == nil {
			continue
		}
		if _, ok := seen[c.ID]; ok {
			continue
		}
		seen[c.ID] = struct{}{}
		if key := writeBehaviorContractExpectedKey(c.Expected); key != "" {
			seenExpected[key] = struct{}{}
		}
		out = append(out, c)
	}
	for i, outcome := range expectedOutcomes {
		outcome = strings.TrimSpace(outcome)
		if outcome == "" {
			continue
		}
		if key := writeBehaviorContractExpectedKey(outcome); key != "" {
			if _, ok := seenExpected[key]; ok {
				continue
			}
			seenExpected[key] = struct{}{}
		}
		id := uniqueWriteBehaviorContractID(fmt.Sprintf("outcome-%d", i+1), seen)
		seen[id] = struct{}{}
		out = append(out, WriteBehaviorContract{
			ID:       id,
			Kind:     WriteBehaviorObservable,
			Polarity: WriteBehaviorPolarityExpected,
			Operator: WriteBehaviorOpSatisfies,
			Expected: outcome,
			Required: false,
			Source: WriteBehaviorContractSourceExpectedOutcomeFallback + ";" +
				WriteBehaviorContractSourcePlanningOnlyUngrounded,
		})
	}
	const maxContracts = 12
	if len(out) > maxContracts {
		out = out[:maxContracts]
	}
	return out
}

// RebaseExpectedOutcomeFallbackWriteBehaviorContracts preserves explicit
// analyzer-authored contracts while replacing only the soft fallback layer
// with the current plan generation's acceptance tests. It is retained for
// ordinary callers that have not received a typed verification counterexample.
// Callers must select the generation from typed workflow state; this helper
// never compares or interprets contract prose.
func RebaseExpectedOutcomeFallbackWriteBehaviorContracts(in []WriteBehaviorContract, expectedOutcomes []string) []WriteBehaviorContract {
	explicit := make([]WriteBehaviorContract, 0, len(in))
	for _, contract := range in {
		if IsExpectedOutcomeFallbackWriteBehaviorContract(contract) {
			continue
		}
		explicit = append(explicit, contract)
	}
	rebased := NormalizeWriteBehaviorContracts(explicit, expectedOutcomes)
	for i := range rebased {
		if IsExpectedOutcomeFallbackWriteBehaviorContract(rebased[i]) {
			rebased[i].Source = WriteBehaviorContractSourcePlanAcceptanceFallback + ";" +
				WriteBehaviorContractSourcePlanningOnlyUngrounded
			rebased[i].Required = false
		}
	}
	return rebased
}

// RebaseVerifyFailureWriteBehaviorContracts builds the active behavior
// contract generation after a typed post-apply verification failure.
//
// Hard contracts and typed-grounded/observed facts always survive. Both
// fallback generations (expected_outcome / plan_acceptance rows) are replaced
// by the current plan's acceptance tests: they are planning-only prose, never
// a proof obligation, so their replacement is a generation change recorded
// with reason fallback_generation_rebase. An ungrounded soft expected contract
// (satisfies-only, no evidence_ref anywhere) is retired ONLY on relevance
// evidence — the failure kind opened the relevance subset (tests_failed) and
// a failed typed row of the same plan names the id — or on an explicit
// planner supersession; every other soft contract is retained byte-identical
// (§40.23). Ids the run's ledger already retired (decision.Prior) stay retired
// under their original tombstone whatever this attempt says (§40.46:
// monotonic retirement). Retired ids are reserved so the new fallback rows
// cannot re-mint them. The decision uses only typed contract fields and the typed handoff
// projection; it never scans the request, plan prose, test output, or source.
//
// The returned tombstones are controller-owned. They prevent an earlier
// still-applied plan in cumulative verification scope from silently
// reintroducing a superseded contract with the same ID, and they carry the
// evidence ids that authorized the retirement.
func RebaseVerifyFailureWriteBehaviorContracts(in []WriteBehaviorContract, expectedOutcomes []string, decision WriteBehaviorContractRetirementDecision) ([]WriteBehaviorContract, []WriteBehaviorContractTombstone) {
	retained := make([]WriteBehaviorContract, 0, len(in))
	tombstones := make([]WriteBehaviorContractTombstone, 0, len(in))
	hits := decision.Relevance.HitByContractID()
	planner := map[string]struct{}{}
	for _, id := range decision.PlannerSupersededIDs {
		if id = strings.TrimSpace(id); id != "" {
			planner[id] = struct{}{}
		}
	}
	seenTombstone := map[string]struct{}{}
	retire := func(tombstone WriteBehaviorContractTombstone) {
		if tombstone.ID == "" {
			return
		}
		if _, ok := seenTombstone[tombstone.ID]; ok {
			return
		}
		seenTombstone[tombstone.ID] = struct{}{}
		tombstones = append(tombstones, tombstone)
	}
	prior := map[string]WriteBehaviorContractTombstone{}
	for _, tombstone := range decision.Prior {
		if id := strings.TrimSpace(tombstone.ID); id != "" {
			if _, ok := prior[id]; !ok {
				prior[id] = tombstone
			}
		}
	}
	for _, contract := range in {
		id := strings.TrimSpace(contract.ID)
		// §40.46: retirement is monotonic — an id the run's ledger already
		// retired stays retired under its ORIGINAL evidence, whatever this
		// attempt's failure kind or relevance says.
		if tombstone, ok := prior[id]; ok {
			retire(tombstone)
			continue
		}
		// Planner supersession is checked first over the whole supersedable
		// class (fallback / planning-only / ungrounded soft): an accepted
		// declaration always mints a planner_supersession tombstone with the
		// plan evidence (accept-set == retire-set, one predicate).
		if _, ok := planner[id]; ok {
			if supersedable, _ := PlannerSupersedableWriteBehaviorContract(contract); supersedable {
				retire(decision.tombstone(id, WriteBehaviorContractRetiredPlannerSupersession, decision.planEvidence()))
				continue
			}
		}
		if IsExpectedOutcomeFallbackWriteBehaviorContract(contract) {
			retire(decision.tombstone(id, WriteBehaviorContractRetiredFallbackGenerationRebase, decision.planEvidence()))
			continue
		}
		if isUngroundedSoftExpectedWriteBehaviorContract(contract) {
			if hit, ok := hits[id]; ok && decision.Lane == FailureKindContractRetireRelevanceSubset {
				retire(decision.tombstone(id, hit.Reason, hit.EvidenceRefs))
				continue
			}
		}
		retained = append(retained, contract)
	}
	sort.SliceStable(tombstones, func(i, j int) bool { return tombstones[i].ID < tombstones[j].ID })
	reserved := make([]string, 0, len(tombstones))
	for _, tombstone := range tombstones {
		reserved = append(reserved, tombstone.ID)
	}
	rebased := normalizeWriteBehaviorContractsReserving(retained, expectedOutcomes, reserved)
	for i := range rebased {
		if IsExpectedOutcomeFallbackWriteBehaviorContract(rebased[i]) {
			rebased[i].Source = WriteBehaviorContractSourcePlanAcceptanceFallback + ";" +
				WriteBehaviorContractSourcePlanningOnlyUngrounded
			rebased[i].Required = false
		}
	}
	return rebased, tombstones
}

func isUngroundedSoftExpectedWriteBehaviorContract(contract WriteBehaviorContract) bool {
	if !contract.Required || contract.Polarity == WriteBehaviorPolarityObserved || IsHardRequiredWriteBehaviorContract(contract) {
		return false
	}
	return !writeBehaviorContractHasEvidence(contract)
}

func dedupSortedWriteBehaviorContractIDs(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func IsExpectedOutcomeFallbackWriteBehaviorContract(c WriteBehaviorContract) bool {
	for _, source := range strings.Split(c.Source, ";") {
		switch strings.TrimSpace(source) {
		case WriteBehaviorContractSourceExpectedOutcomeFallback, WriteBehaviorContractSourcePlanAcceptanceFallback:
			return true
		}
	}
	return false
}

func appendWriteBehaviorContractSource(source, marker string) string {
	source = strings.TrimSpace(source)
	marker = strings.TrimSpace(marker)
	if marker == "" {
		return source
	}
	for _, part := range strings.Split(source, ";") {
		if strings.TrimSpace(part) == marker {
			return source
		}
	}
	if source == "" {
		return marker
	}
	return source + ";" + marker
}

// NormalizeWriteBehaviorTransition preserves only schema-known ordered steps.
// It does not infer a sequence from Expected/Subject text and therefore cannot
// turn noisy prose into a hard control signal.
func NormalizeWriteBehaviorTransition(in *WriteBehaviorTransition) *WriteBehaviorTransition {
	if in == nil {
		return nil
	}
	const maxTransitionSteps = 8
	steps := make([]WriteBehaviorTransitionStep, 0, min(len(in.Steps), maxTransitionSteps))
	for _, raw := range in.Steps {
		step := raw
		step.Phase = WriteBehaviorTransitionPhase(strings.ToLower(strings.TrimSpace(string(step.Phase))))
		if !IsKnownWriteBehaviorTransitionPhase(string(step.Phase)) {
			continue
		}
		step.Operation = strings.TrimSpace(step.Operation)
		step.Expected = strings.TrimSpace(step.Expected)
		step.EvidenceRef = strings.TrimSpace(step.EvidenceRef)
		if step.Operation == "" && step.Expected == "" {
			continue
		}
		steps = append(steps, step)
		if len(steps) == maxTransitionSteps {
			break
		}
	}
	if len(steps) == 0 {
		return nil
	}
	return &WriteBehaviorTransition{Steps: steps}
}

func NormalizeWriteRenderedTextPlacement(in *WriteRenderedTextPlacement, fallbackExpected string) *WriteRenderedTextPlacement {
	if in == nil {
		return nil
	}
	p := *in
	p.Surface = WriteRenderedTextSurface(strings.ToLower(strings.TrimSpace(string(p.Surface))))
	if !IsKnownWriteRenderedTextSurface(string(p.Surface)) {
		p.Surface = ""
	}
	p.Anchor = strings.TrimSpace(p.Anchor)
	p.Expected = strings.TrimSpace(p.Expected)
	if p.Expected == "" {
		p.Expected = strings.TrimSpace(fallbackExpected)
	}
	p.Relation = WriteRenderedTextRelation(strings.ToLower(strings.TrimSpace(string(p.Relation))))
	if !IsKnownWriteRenderedTextRelation(string(p.Relation)) {
		p.Relation = ""
	}
	p.Delimiter = strings.TrimSpace(p.Delimiter)
	p.EvidenceRef = strings.TrimSpace(p.EvidenceRef)
	if p.Anchor == "" && p.Expected == "" && p.Relation == "" && p.Delimiter == "" && p.EvidenceRef == "" && p.Surface == "" {
		return nil
	}
	return &p
}

func normalizeWriteBehaviorComparator(in *WriteBehaviorComparator, fallbackOperator WriteBehaviorOperator) *WriteBehaviorComparator {
	if in == nil {
		return nil
	}
	c := *in
	c.Subject = strings.TrimSpace(c.Subject)
	c.Expected = strings.TrimSpace(c.Expected)
	c.EvidenceRef = strings.TrimSpace(c.EvidenceRef)
	c.Operator = WriteBehaviorOperator(strings.ToLower(strings.TrimSpace(string(c.Operator))))
	if !IsKnownWriteBehaviorOperator(string(c.Operator)) {
		if IsKnownWriteBehaviorOperator(string(fallbackOperator)) {
			c.Operator = fallbackOperator
		} else {
			c.Operator = WriteBehaviorOpSatisfies
		}
	}
	c.Relation = WriteBehaviorComparatorRelation(strings.ToLower(strings.TrimSpace(string(c.Relation))))
	if !IsKnownWriteBehaviorComparatorRelation(string(c.Relation)) {
		c.Relation = WriteBehaviorComparatorSameAs
	}
	if c.Subject == "" && c.Expected == "" && c.EvidenceRef == "" {
		return nil
	}
	return &c
}

func writeBehaviorContractExpectedKey(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(s)), " "))
}

func uniqueWriteBehaviorContractID(base string, seen map[string]struct{}) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "outcome"
	}
	if _, ok := seen[base]; !ok {
		return base
	}
	for i := 2; ; i++ {
		id := fmt.Sprintf("%s-%d", base, i)
		if _, ok := seen[id]; !ok {
			return id
		}
	}
}

func RequiredWriteBehaviorContractIDs(contracts []WriteBehaviorContract, includeFallback bool) map[string]struct{} {
	ids := make(map[string]struct{}, len(contracts))
	for _, c := range contracts {
		if !c.Required || strings.TrimSpace(c.ID) == "" {
			continue
		}
		if c.Polarity == WriteBehaviorPolarityObserved || IsPlanningOnlyWriteBehaviorContract(c) {
			continue
		}
		if !includeFallback && IsExpectedOutcomeFallbackWriteBehaviorContract(c) {
			continue
		}
		ids[c.ID] = struct{}{}
	}
	return ids
}

func IsHardRequiredWriteBehaviorContract(c WriteBehaviorContract) bool {
	if !c.Required || strings.TrimSpace(c.ID) == "" {
		return false
	}
	if c.Polarity == WriteBehaviorPolarityObserved || IsPlanningOnlyWriteBehaviorContract(c) {
		return false
	}
	if IsExpectedOutcomeFallbackWriteBehaviorContract(c) {
		return false
	}
	if c.Placement != nil {
		return true
	}
	switch c.Operator {
	case WriteBehaviorOpEquals, WriteBehaviorOpNotEquals,
		WriteBehaviorOpContains, WriteBehaviorOpNotContains,
		WriteBehaviorOpExists, WriteBehaviorOpNotExists,
		WriteBehaviorOpRaises, WriteBehaviorOpNotRaises,
		WriteBehaviorOpReturns:
		return true
	default:
		return false
	}
}

// IsPlanningOnlyWriteBehaviorContract reads only the system-stamped source
// roster. It never infers authority from the contract's model-authored prose.
func IsPlanningOnlyWriteBehaviorContract(c WriteBehaviorContract) bool {
	// Legacy persisted plans may predate the explicit planning-only marker.
	// Their fallback source token is still enough to prove that the text came
	// from a model-authored outcome/acceptance summary rather than grounded
	// verifier authority, so never let reload upgrade it.
	if IsExpectedOutcomeFallbackWriteBehaviorContract(c) {
		return true
	}
	for _, source := range strings.Split(c.Source, ";") {
		if strings.TrimSpace(source) == WriteBehaviorContractSourcePlanningOnlyUngrounded {
			return true
		}
	}
	return false
}

func IsPlacementRequiredWriteBehaviorContract(c WriteBehaviorContract) bool {
	if !IsHardRequiredWriteBehaviorContract(c) {
		return false
	}
	return c.Placement != nil
}

func HardRequiredWriteBehaviorContractIDs(contracts []WriteBehaviorContract) map[string]struct{} {
	ids := make(map[string]struct{}, len(contracts))
	for _, c := range contracts {
		if IsHardRequiredWriteBehaviorContract(c) {
			ids[c.ID] = struct{}{}
		}
	}
	return ids
}

// writeBehaviorContractIDs is deliberately unexported: the raw id set of a
// contract slice is not an authority any gate may consult directly. Gates
// resolve ids through WriteBehaviorContractResolution (V5-4), which is the
// only caller; the census test guards against a re-export.
func writeBehaviorContractIDs(contracts []WriteBehaviorContract) map[string]struct{} {
	ids := make(map[string]struct{}, len(contracts))
	for _, c := range contracts {
		if id := strings.TrimSpace(c.ID); id != "" {
			ids[id] = struct{}{}
		}
	}
	return ids
}

func PlacementRequiredWriteBehaviorContractIDs(contracts []WriteBehaviorContract) map[string]struct{} {
	ids := make(map[string]struct{}, len(contracts))
	for _, c := range contracts {
		if IsPlacementRequiredWriteBehaviorContract(c) {
			ids[strings.TrimSpace(c.ID)] = struct{}{}
		}
	}
	return ids
}
