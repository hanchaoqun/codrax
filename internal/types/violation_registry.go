package types

// violation_registry.go — B0 (v3 runtime consolidation, 2026-05-04).
//
// ViolKindRegistry is the single source of truth for the runtime
// classification of every ViolationKind. Adding a new kind requires
// editing ONE entry here; the legacy DeriveSeverity switch,
// defaultSoftKinds map, DefaultFallbackPolicy map, and inferViolationLayer
// switch all derive from this registry during the v3 migration window.
//
// ── Why this exists ──
//
// Pre-v3, adding a new ViolationKind required editing ≥6 sites:
//   1. internal/types/violation.go            — constant declaration
//   2. internal/types/violation.go            — AllViolationKinds slice
//   3. internal/types/retry_state.go          — DeriveSeverity switch
//   4. internal/orchestrator/contract_check.go — defaultSoftKinds map
//   5. internal/orchestrator/fallback_policy.go — DefaultFallbackPolicy map
//   6. internal/orchestrator/retry_state.go   — inferViolationLayer switch
//
// Six sites, each easy to forget. The 6th-spot-sync red line
// (feedback_typed_signal_six_spot_sync.md) flagged this class of drift
// after V2 runtime eval forensic showed cooccurrence rule omissions
// burning retry budget. v3 collapses the registry surface so a new kind
// declares its full spec once.
//
// ── Migration window ──
//
// During v3 rollout the legacy DeriveSeverity / defaultSoftKinds /
// DefaultFallbackPolicy / inferViolationLayer functions retain their
// hardcoded literals AND consult the registry. The structural test
// TestRegistryDerivesAllLegacyTables asserts byte-identical agreement
// between registry-derived and legacy-literal output for every kind in
// AllViolationKinds(). Once B1-B6 land and no caller is observed
// depending on the legacy literals, a follow-up PR deletes them.
//
// ── Contract ──
//
//   - ViolKindSpec is operator-facing internal data. Description is for
//     telemetry / dashboards / commit messages — NEVER rendered to LLM
//     prompts (R4 red line).
//   - DefaultSeverity drives ViolationProfileFor().Severity. Promotion
//     via pipeline_contract_strict_kinds yaml bumps Soft → Medium
//     uniformly (matches existing isStrict path in DeriveSeverity).
//   - SoftByDefault drives membership in defaultSoftKinds(). The two
//     fields are independent: a kind may be Severity=Medium and also
//     SoftByDefault=true (existing pattern for ViolFacetUncovered:
//     Severity Medium per R14 but in the soft map per Phase-4 default).
//   - Promotable=false means operator yaml CANNOT promote this kind to
//     STRICT. Currently the only such kind is ViolDiagramEdgeLabelMismatch
//     (label inference is permanently noisy per R3). Honoured by
//     SetSoftViolationKinds.
//   - FallbackLocus drives DefaultFallbackPolicy(); the policy map then
//     derives FallbackTarget via targetForLocus.
//   - Layer drives inferViolationLayer; consumed by RetryState rendering
//     to group violations by producer.

import (
	"fmt"
	"sync"
)

// ViolKindSpec captures the runtime classification of one
// ViolationKind. Treated as an immutable record — registration is
// append-only at package init time; runtime mutation is not
// supported.
type ViolKindSpec struct {
	// Kind is the typed identifier. Required.
	Kind ViolationKind

	// DefaultSeverity is the Severity assigned by ViolationProfileFor
	// when isStrict=false. The strict-promotion path (operator yaml)
	// bumps Soft → Medium following the existing DeriveSeverity rule.
	DefaultSeverity Severity

	// SoftByDefault, when true, places this kind in defaultSoftKinds()
	// — meaning hasAnyStrictViolation skips it and the gate does NOT
	// flip Passed=false on this kind alone. Independent from Severity:
	// some Medium-severity kinds are SoftByDefault (e.g.
	// ViolFacetUncovered) because the right repair needs new evidence
	// rather than a finalize retry.
	SoftByDefault bool

	// Promotable=false means operator yaml pipeline_contract_strict_kinds
	// CANNOT lift this kind out of soft. Used for permanently-noisy
	// signals (label inference, frequency bridges). Default true.
	Promotable bool

	// FallbackLocus is the stage that owns the fix. DefaultFallbackPolicy()
	// derives the FallbackTarget via targetForLocus(locus). Special
	// loci:
	//   - LocusFinalizer: re-emit the answer document
	//   - LocusExtract:   re-pick citations / re-emit symbol slate
	//   - LocusExplore:   re-investigate (need new evidence)
	//   - LocusTerminal:  fail-loud (non-recoverable through retry)
	FallbackLocus RepairLocus

	// Layer is the producer-stage label rendered by RetryState
	// "Active Violations (typed, by severity + layer)" section. One of:
	//   "v2_oracle" / "self_consistency" / "semantic_quality" /
	//   "evidence_pool" / "external_artifact" / "authority" / "cgec" /
	//   "reviewer" / "answer_oracle" / "contract_check"
	// Unknown / unspecified maps to "contract_check".
	Layer string

	// Description is the operator-facing one-line summary. NEVER
	// rendered into LLM-facing prompts (R4 red line). May be empty.
	Description string

	// FixableByAgents lists the AgentName values that can plausibly
	// fix this kind through their available tools. The orchestrator's
	// retry-target picker validates the chosen target against this
	// list — when the candidate target is not in FixableByAgents,
	// routing escalates upstream (or surfaces as a caveat) instead
	// of dispatching a retry that cannot succeed.
	//
	// Example: ViolEnumerationLabelUngrounded is owned by the
	// extractor (FallbackLocus=LocusExtract) but the per-kind retry
	// budget often pins target=finalizer_only. The finalizer is a
	// pure synthesizer (no read_file / grep) — it cannot resolve a
	// label-grounding issue. Without FixableByAgents the dispatch
	// fails predictably and burns a round.
	//
	// Empty list = unconstrained (any agent the locus rules pick is
	// acceptable). Filled list = whitelist; orchestrator verifies
	// chosen target is in it.
	//
	// Per W3 (docs/design/iteration_inflation_remediation.md
	// source #4).
	FixableByAgents []AgentName

	// SchemaDescriptionFragment is a sentence-level structural rule
	// the LLM should know AT EMIT TIME (rather than learn
	// post-rejection from a retry hint). When non-empty, the
	// fragment is automatically pulled into the LLM-facing tool
	// Description() — so the constraint preview is single-source-
	// of-truth with the validator that enforces it.
	//
	// Per W3 (docs/design/iteration_inflation_remediation.md), the
	// system migrates from "validate after emit" to "guide before
	// emit" by giving the LLM the structural requirement at the
	// point of generation.
	//
	// Format: ONE complete sentence in LLM-facing language. No
	// internal Go terminology, no ViolKind names, no IRField
	// strings, no operator jargon. Empty (the common case) means
	// the validator is post-hoc only — the LLM cannot prevent the
	// violation by knowing the rule (e.g. self-contradiction).
	SchemaDescriptionFragment string

	// Implies names the ViolKinds whose presence is structurally
	// caused by THIS kind firing. When two validators report on the
	// same root cause from different angles, Implies lets
	// ComputeRootCauseClosure mark the symptom as derived so the
	// retry hint surfaces ONE root, not N facets.
	//
	// Example: ViolBlockCoverageMissing implies both
	// ViolEnumerationLabelUngrounded and ViolFacetUncovered when
	// they fire together against the same fingerprint — fixing the
	// missing block resolves all three.
	//
	// Empty Implies list = leaf root cause (the common case).
	// Cycles are not allowed; the closure algorithm will panic on
	// detection at startup-time validation. Implies is per-Kind not
	// per-cluster, so direct relations only — transitive closure
	// is computed at run-time by walking the graph.
	Implies []ViolationKind

	// CaveatFamilyID groups ViolKinds that share the same user-facing
	// caveat template. Many ViolKinds describe the same root cause
	// from different validators — e.g. richness_regression /
	// facet_uncovered / principal_prose_underfilled all map to one
	// natural-language caveat about "answer coverage may be
	// incomplete". Grouping prevents the user from seeing 3 caveats
	// for one underlying issue.
	//
	// Empty string means this ViolKind has no user-facing caveat
	// (operator-only kinds such as ViolPlanCritic / reviewer-only
	// telemetry). When the materializer encounters such a kind in
	// the unresolved list it logs to telemetry but does NOT emit a
	// caveat.
	//
	// Templates are registered separately in the CaveatFamily
	// registry (see RegisterCaveatFamily). The pair (CaveatFamilyID,
	// language) maps to the user-facing string. Per the
	// "user-panel hygiene" red line (docs/design/iteration_inflation_
	// remediation.md W1), the template MUST NOT contain any of:
	//   - ViolKind name, IRField name
	//   - confidence number, event count
	//   - "yield kill", "Pipeline terminated"
	//   - any other internal jargon
	// It MUST be a complete user-readable sentence.
	CaveatFamilyID string
}

// RepairLocus is declared in internal/orchestrator/fallback_policy.go;
// the registry references it via a string alias here to avoid a
// cyclic import (orchestrator → types → orchestrator). The orchestrator
// package's RepairLocus values match these constants exactly; the
// translation is verified by TestRegistryDerivesAllLegacyTables.
type RepairLocus string

const (
	LocusFinalizer RepairLocus = "finalizer"
	LocusExtract   RepairLocus = "extract"
	LocusExplore   RepairLocus = "explore"
	LocusTerminal  RepairLocus = "terminal"
)

// IsValid reports whether l is a known RepairLocus.
func (l RepairLocus) IsValid() bool {
	switch l {
	case LocusFinalizer, LocusExtract, LocusExplore, LocusTerminal:
		return true
	}
	return false
}

var (
	violRegistryMu sync.RWMutex
	violRegistry   = map[ViolationKind]ViolKindSpec{}
	// violRegistryOrder preserves declaration order for AllViolKindSpecs.
	violRegistryOrder []ViolationKind

	caveatFamilyMu       sync.RWMutex
	caveatFamilyRegistry = map[string]CaveatFamilyTemplate{}
	// caveatFamilyOrder preserves registration order so AllCaveatFamilies
	// returns templates in a stable, predictable sequence (used by
	// MaterializeUnresolvedViolationsAsCaveats to build deterministic
	// output).
	caveatFamilyOrder []string
)

// CaveatFamilyTemplate carries the user-facing language renderings for
// one caveat family. Multiple ViolKinds may share one family — the
// materializer dedupes so the user sees one caveat per family even
// when 3 ViolKinds in that family fired.
//
// Templates are PLAIN PROSE — no placeholders, no internal jargon,
// no ViolKind names. They are read by users, not LLMs.
type CaveatFamilyTemplate struct {
	// ID is the identifier referenced by ViolKindSpec.CaveatFamilyID.
	ID string

	// ZH is the simplified Chinese rendering.
	ZH string

	// EN is the English rendering.
	EN string
}

// RegisterCaveatFamily appends a template. Idempotent — second call
// with same ID overwrites (test path). Panics on empty ID or empty
// templates so registration errors fail loud at init.
func RegisterCaveatFamily(t CaveatFamilyTemplate) {
	if t.ID == "" {
		panic("RegisterCaveatFamily: ID required")
	}
	if t.ZH == "" || t.EN == "" {
		panic(fmt.Sprintf("RegisterCaveatFamily(%q): both ZH and EN templates required", t.ID))
	}
	caveatFamilyMu.Lock()
	defer caveatFamilyMu.Unlock()
	if _, existed := caveatFamilyRegistry[t.ID]; !existed {
		caveatFamilyOrder = append(caveatFamilyOrder, t.ID)
	}
	caveatFamilyRegistry[t.ID] = t
}

// CaveatFamilyTemplateFor returns the template for id, or (zero,
// false) when no template was registered. Concurrent-safe.
func CaveatFamilyTemplateFor(id string) (CaveatFamilyTemplate, bool) {
	caveatFamilyMu.RLock()
	defer caveatFamilyMu.RUnlock()
	t, ok := caveatFamilyRegistry[id]
	return t, ok
}

// AllCaveatFamilies returns every registered template in
// registration order. Stable across calls — consumers (such as the
// materializer) rely on this for deterministic output.
func AllCaveatFamilies() []CaveatFamilyTemplate {
	caveatFamilyMu.RLock()
	defer caveatFamilyMu.RUnlock()
	out := make([]CaveatFamilyTemplate, 0, len(caveatFamilyOrder))
	for _, id := range caveatFamilyOrder {
		out = append(out, caveatFamilyRegistry[id])
	}
	return out
}

// RegisterViolKind appends a spec to the registry. Idempotent: a
// second call with the same Kind overwrites the previous spec
// (used by tests; production registers once at package init).
//
// Required fields: Kind, DefaultSeverity, FallbackLocus.
// Promotable defaults to true when not explicitly set by the caller
// (Go zero value is false; see RegisterViolKindStrict for explicit-only).
func RegisterViolKind(spec ViolKindSpec) {
	if spec.Kind == "" {
		panic("RegisterViolKind: Kind required")
	}
	if !spec.DefaultSeverity.IsValid() {
		panic(fmt.Sprintf("RegisterViolKind(%q): invalid DefaultSeverity %q", spec.Kind, spec.DefaultSeverity))
	}
	if !spec.FallbackLocus.IsValid() {
		panic(fmt.Sprintf("RegisterViolKind(%q): invalid FallbackLocus %q", spec.Kind, spec.FallbackLocus))
	}
	violRegistryMu.Lock()
	defer violRegistryMu.Unlock()
	if _, existed := violRegistry[spec.Kind]; !existed {
		violRegistryOrder = append(violRegistryOrder, spec.Kind)
	}
	violRegistry[spec.Kind] = spec
}

// ViolKindSpecFor returns the registered spec for kind, or
// (zero, false) when no spec exists. Lookups are read-locked; safe
// under concurrent access.
func ViolKindSpecFor(kind ViolationKind) (ViolKindSpec, bool) {
	violRegistryMu.RLock()
	defer violRegistryMu.RUnlock()
	spec, ok := violRegistry[kind]
	return spec, ok
}

// AllViolKindSpecs returns every registered spec in declaration
// order. Used by structural tests to sweep the registry. Returns a
// fresh slice; safe to mutate.
func AllViolKindSpecs() []ViolKindSpec {
	violRegistryMu.RLock()
	defer violRegistryMu.RUnlock()
	out := make([]ViolKindSpec, 0, len(violRegistryOrder))
	for _, k := range violRegistryOrder {
		out = append(out, violRegistry[k])
	}
	return out
}

// IsViolKindPromotable reports whether the registered kind allows
// operator yaml to promote it to STRICT. Defaults to true for
// unregistered kinds (back-compat — pre-registry the only opt-out
// was the SeveritySoft permanent path, which Promotable=false
// emulates).
func IsViolKindPromotable(kind ViolationKind) bool {
	if spec, ok := ViolKindSpecFor(kind); ok {
		return spec.Promotable
	}
	return true
}

// Caveat family identifiers. ViolKindSpec.CaveatFamilyID references
// these constants. Adding a new family: define the const, register
// the template in init(), then point relevant ViolKindSpec entries
// at it. Prefer growing existing families over creating new ones —
// 7-8 families covers the natural taxonomy.
const (
	CaveatFamilyAnswerCoverage   = "answer_coverage"
	CaveatFamilyDiagramFidelity  = "diagram_fidelity"
	CaveatFamilyEnumerationDepth = "enumeration_depth"
	CaveatFamilyCitationGrounded = "citation_grounding"
	CaveatFamilyAuthorityHedging = "authority_hedging"
	CaveatFamilyConsistency      = "consistency"
	CaveatFamilyAcceptance       = "acceptance"
	CaveatFamilyShapeFit         = "shape_fit"
)

// init registers the canonical spec for every ViolationKind declared
// in violation.go. The order mirrors AllViolationKinds() for
// telemetry stability. Adding a new kind: declare the constant in
// violation.go, append to AllViolationKinds, register here.
func init() {
	// ── Caveat family templates (W1.1, 2026-05-05) ──
	// Grouped by user-facing root cause, not by validator origin.
	// Wording is deliberately tentative ("可能" / "may") to match the
	// reality that retry exhausted but the answer is still shipped.
	RegisterCaveatFamily(CaveatFamilyTemplate{
		ID: CaveatFamilyAnswerCoverage,
		ZH: "答案在某些维度的覆盖度可能不充分，建议结合源码进一步核对相关组件。",
		EN: "Coverage on some dimensions of the answer may be incomplete; cross-check with source for the affected components.",
	})
	RegisterCaveatFamily(CaveatFamilyTemplate{
		ID: CaveatFamilyDiagramFidelity,
		ZH: "图示中部分边的关系标注未达到完整的类型化支持，渲染图可仅作辅助参考。",
		EN: "Some diagram edges lack fully typed relation backing; treat the diagram as a supporting illustration rather than authoritative.",
	})
	RegisterCaveatFamily(CaveatFamilyTemplate{
		ID: CaveatFamilyEnumerationDepth,
		ZH: "枚举类条目中部分项的证据支持稍弱，请按需对该类别的列表进一步核实完整性。",
		EN: "Some enumerated items have weaker evidence backing; verify completeness of the listed set against source if it is load-bearing.",
	})
	RegisterCaveatFamily(CaveatFamilyTemplate{
		ID: CaveatFamilyCitationGrounded,
		ZH: "答案引用的部分锚点未能完整通过校验，相关文件:行 信息建议作为参考而非定论。",
		EN: "Some cited anchors did not fully pass verification; treat the file:line references as guidance rather than definitive proof.",
	})
	RegisterCaveatFamily(CaveatFamilyTemplate{
		ID: CaveatFamilyAuthorityHedging,
		ZH: "答案在部分论断的可信强度上做了较强声明，实际证据来源可能仅支撑较弱断言。",
		EN: "The answer states some claims more confidently than the underlying evidence may support; calibrate accordingly.",
	})
	RegisterCaveatFamily(CaveatFamilyTemplate{
		ID: CaveatFamilyConsistency,
		ZH: "答案前后某些表述存在不完全一致之处，请按需对该处具体段落复核。",
		EN: "Some passages of the answer are not fully consistent with each other; review the specific section if precision matters.",
	})
	RegisterCaveatFamily(CaveatFamilyTemplate{
		ID: CaveatFamilyAcceptance,
		ZH: "答案在部分验收检查上未达到预期标准，结果中的相应部分可能需要补充验证。",
		EN: "Some acceptance checks did not meet expected standards; the corresponding parts of the answer may need additional verification.",
	})
	RegisterCaveatFamily(CaveatFamilyTemplate{
		ID: CaveatFamilyShapeFit,
		ZH: "答案的整体结构与问题最贴合的形态略有出入，可能影响个别局部的清晰度。",
		EN: "The answer's overall structure does not perfectly match the most natural shape for the question, which may affect clarity in specific spots.",
	})

	// ── Original contract-checker kinds ──
	RegisterViolKind(ViolKindSpec{
		Kind: ViolFamilyMismatch, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "contract_check", CaveatFamilyID: CaveatFamilyAcceptance,
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolCitation, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusExplore,
		Layer: "contract_check", CaveatFamilyID: CaveatFamilyCitationGrounded,
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolMustInclude, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "contract_check", CaveatFamilyID: CaveatFamilyAnswerCoverage,
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolMustExclude, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "contract_check", CaveatFamilyID: CaveatFamilyAcceptance,
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolAcceptance, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusExplore,
		Layer: "contract_check", CaveatFamilyID: CaveatFamilyAcceptance,
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolSuccessCriterion, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusExplore,
		Layer: "contract_check", CaveatFamilyID: CaveatFamilyAcceptance,
	})

	// ── Session 11 enforcer kinds ──
	RegisterViolKind(ViolKindSpec{
		Kind: ViolGhostAnchor, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusExplore,
		Layer: "cgec", CaveatFamilyID: CaveatFamilyCitationGrounded,
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolChainDemoted, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusExplore,
		Layer: "cgec", CaveatFamilyID: CaveatFamilyCitationGrounded,
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolSelfRefLiteral, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "cgec", CaveatFamilyID: CaveatFamilyConsistency,
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolPreCompleteDowngrade, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "cgec", CaveatFamilyID: CaveatFamilyConsistency,
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolLiteralFormFailed, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "cgec", CaveatFamilyID: CaveatFamilyCitationGrounded,
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolViewSwap, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "cgec", CaveatFamilyID: CaveatFamilyShapeFit,
	})

	// ── Commit 53 P2 read-mode shape oracle ──
	RegisterViolKind(ViolKindSpec{
		Kind: ViolViewIntentMismatch, DefaultSeverity: SeverityMedium,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "answer_oracle", CaveatFamilyID: CaveatFamilyShapeFit,
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolSubTopicCountMismatch, DefaultSeverity: SeverityMedium,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "answer_oracle", CaveatFamilyID: CaveatFamilyAnswerCoverage,
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolDiagramIdentifier, DefaultSeverity: SeverityMedium,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "v2_oracle", CaveatFamilyID: CaveatFamilyDiagramFidelity,
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolDeclaredCountDrift, DefaultSeverity: SeverityMedium,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusExtract,
		Layer: "v2_oracle", CaveatFamilyID: CaveatFamilyEnumerationDepth,
		SchemaDescriptionFragment: "When emit_answer_symbol declared a count of N, the rendered enumeration block MUST contain exactly N items — drift below or above the claim is rejected.",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolSelfContradiction, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "self_consistency", CaveatFamilyID: CaveatFamilyConsistency,
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolExternalArtifactUnderdecoded, DefaultSeverity: SeverityMedium,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "external_artifact", CaveatFamilyID: CaveatFamilyAcceptance,
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolAuthorityOverreach, DefaultSeverity: SeverityCritical,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "authority", CaveatFamilyID: CaveatFamilyAuthorityHedging,
	})

	// ── Block 1 reviewer kinds (telemetry-only) ──
	// Reviewer kinds are operator-only telemetry — no user-visible
	// caveat (CaveatFamilyID intentionally empty).
	RegisterViolKind(ViolKindSpec{
		Kind: ViolPlanCritic, DefaultSeverity: SeveritySoft,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusTerminal,
		Layer: "reviewer",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolReflectorObservation, DefaultSeverity: SeveritySoft,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusTerminal,
		Layer: "reviewer",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolAnswerReviewerDistilled, DefaultSeverity: SeveritySoft,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusTerminal,
		Layer: "reviewer",
	})

	// ── Block 2 Intent / Subject / PredicateAxis oracle ──
	RegisterViolKind(ViolKindSpec{
		Kind: ViolIntentTraceShallow, DefaultSeverity: SeverityMedium,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusExplore,
		Layer: "answer_oracle", CaveatFamilyID: CaveatFamilyAnswerCoverage,
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolIntentEnumerateNotList, DefaultSeverity: SeverityMedium,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "answer_oracle", CaveatFamilyID: CaveatFamilyShapeFit,
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolIntentRootCauseNoCause, DefaultSeverity: SeverityMedium,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusExplore,
		Layer: "answer_oracle", CaveatFamilyID: CaveatFamilyAnswerCoverage,
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolIntentConfigNoTrail, DefaultSeverity: SeverityMedium,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusExplore,
		Layer: "answer_oracle", CaveatFamilyID: CaveatFamilyAnswerCoverage,
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolSubjectAnchorMissing, DefaultSeverity: SeverityMedium,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusExtract,
		Layer: "answer_oracle", CaveatFamilyID: CaveatFamilyCitationGrounded,
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolPredicateAxisMissing, DefaultSeverity: SeverityMedium,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusExplore,
		Layer: "answer_oracle", CaveatFamilyID: CaveatFamilyCitationGrounded,
	})

	// ── Phase 4 Semantic Surface Contract ──
	RegisterViolKind(ViolKindSpec{
		Kind: ViolFacetUncovered, DefaultSeverity: SeverityMedium,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusExplore,
		Layer: "v2_oracle", CaveatFamilyID: CaveatFamilyAnswerCoverage,
		SchemaDescriptionFragment: "Every required facet listed in the user section MUST be claimed by at least one block — set block.facet_ids=[<facet>] OR block.claim_uses[j].facet_id=<facet>.",
		// Fix path: finalizer can claim existing facet via
		// facet_ids; if facet has no evidence, explorer re-investigation
		// is needed.
		FixableByAgents: []AgentName{AgentFinalizer, AgentExplorer},
		// FacetUncovered → richness drops because a required facet
		// has no block. Same root, different reporter angle.
		Implies: []ViolationKind{ViolRichnessRegression},
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolClaimFormUnsupported, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "v2_oracle", CaveatFamilyID: CaveatFamilyAcceptance,
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolAbsenceScopeExceeded, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusExtract,
		Layer: "v2_oracle", CaveatFamilyID: CaveatFamilyAcceptance,
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolMissingRequestedRoleUndisclosed, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "v2_oracle", CaveatFamilyID: CaveatFamilyAnswerCoverage,
		SchemaDescriptionFragment: "When the semantic view names missing_requested_roles[] for a config-precedence exact-absence answer, copy every listed role into the document-level missing_requested_roles[] field. Do not omit a requested missing layer and do not replace it with vague placeholders like N/A.",
		FixableByAgents:           []AgentName{AgentFinalizer},
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolStepIdentifierUnverified, DefaultSeverity: SeverityMedium,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusExplore,
		Layer: "answer_oracle", CaveatFamilyID: CaveatFamilyCitationGrounded,
	})
	// Phase 5 — telemetry-only, permanently SOFT.
	RegisterViolKind(ViolKindSpec{
		Kind: ViolRichnessRegression, DefaultSeverity: SeveritySoft,
		SoftByDefault: true, Promotable: false, FallbackLocus: LocusTerminal,
		Layer: "v2_oracle", CaveatFamilyID: CaveatFamilyAnswerCoverage,
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolValueSecondaryCitationOffFocus, DefaultSeverity: SeverityMedium,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusExtract,
		// Legacy inferViolationLayer groups this under answer_oracle
		// alongside other Block 2 oracle kinds; preserved verbatim
		// for migration parity. v3 follow-up may regroup.
		Layer: "answer_oracle", CaveatFamilyID: CaveatFamilyCitationGrounded,
	})

	// ── B4 V2 block-only carrier validators ──
	RegisterViolKind(ViolKindSpec{
		Kind: ViolBlockCoverageMissing, DefaultSeverity: SeverityCritical,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusExtract,
		Layer: "v2_oracle", CaveatFamilyID: CaveatFamilyAnswerCoverage,
		SchemaDescriptionFragment: "Every block kind named in the user section's Required Answer Blocks list MUST appear in blocks[] at least the listed minimum count, with the required FacetIDs covered.",
		// Fix path: finalizer can usually add the missing block
		// kind from existing evidence; if facets need new evidence,
		// extractor + explorer come into play. Finalizer is the
		// primary fix agent.
		FixableByAgents: []AgentName{AgentFinalizer, AgentExtractor},
		// A missing block means its items + claim_uses don't exist
		// to be checked, so symptom violations on those become
		// derivations of the missing-block root.
		Implies: []ViolationKind{
			ViolEnumerationLabelUngrounded,
			ViolPrincipalClaimUseMissing,
		},
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolPrincipalClaimUseMissing, DefaultSeverity: SeverityCritical,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "v2_oracle", CaveatFamilyID: CaveatFamilyCitationGrounded,
		SchemaDescriptionFragment: "Every principal block (surface_role=principal) whose user-section contract lists allowed `claim_form` values MUST emit at least one matching entry in the block's claim_uses[] array.",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolDiagramEdgeUnsupported, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "v2_oracle", CaveatFamilyID: CaveatFamilyDiagramFidelity,
		SchemaDescriptionFragment: "Every diagram edge that carries a labelled relation MUST have a matching entry in the diagram block's edge_anchors[] with a typed relation_kind (call/guard/import/precedence/contain/observe) and from_node/to_node names that appear verbatim in the diagram body.",
		// "Edge unsupported" is the structural lack of an
		// edge_anchor. When LLM "fixes" by adding an anchor whose
		// label/relation pair drifts, label_mismatch /
		// relation_label_only fire — same root edge problem.
		// Without this Implies, qfa-mr3 saw Round 0 unsupported ->
		// Round 1 label_mismatch as a "new" cluster (stable=0).
		Implies: []ViolationKind{
			ViolDiagramEdgeLabelMismatch,
			ViolDiagramRelationLabelOnly,
		},
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolDiagramEdgeLabelMismatch, DefaultSeverity: SeveritySoft,
		SoftByDefault: true, Promotable: false, FallbackLocus: LocusFinalizer,
		Layer: "v2_oracle", CaveatFamilyID: CaveatFamilyDiagramFidelity,
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolUncertaintyBlockMissing, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "v2_oracle", CaveatFamilyID: CaveatFamilyAnswerCoverage,
	})

	// ── G5 / 修 B post_v2_runtime_gap_remediation ──
	RegisterViolKind(ViolKindSpec{
		Kind: ViolAnswerSemanticUnderfilled, DefaultSeverity: SeveritySoft,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "semantic_quality", CaveatFamilyID: CaveatFamilyAnswerCoverage,
		// Semantic-underfilled is the broad reviewer signal that
		// principal prose is thin or enumeration evidence is sparse;
		// those specific symptoms are derivations of the same root.
		Implies: []ViolationKind{
			ViolPrincipalProseUnderfilled,
			ViolEnumerationEvidenceUnderspecified,
		},
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolEnumerationEvidenceUnderspecified, DefaultSeverity: SeveritySoft,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusExplore,
		Layer: "evidence_pool", CaveatFamilyID: CaveatFamilyEnumerationDepth,
	})

	// ── B3 v3 (2026-05-04) — diagram relation typed-first ──
	// SOFT-by-default; promotable. Encourages typed relation_kind
	// declaration when label-only inference would fill the contract
	// minimum.
	RegisterViolKind(ViolKindSpec{
		Kind: ViolDiagramRelationLabelOnly, DefaultSeverity: SeveritySoft,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "v2_oracle", CaveatFamilyID: CaveatFamilyDiagramFidelity,
	})

	// ── B2 v3 (2026-05-04) — three-layer quality contract ──
	// Severity=Medium so RetryEligible=true; SoftByDefault=false so
	// the gate flips Passed=false. Operators may demote via
	// pipeline_contract_soft_kinds when noise rate is too high.
	RegisterViolKind(ViolKindSpec{
		Kind: ViolRichnessGlaringGap, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "v2_oracle", CaveatFamilyID: CaveatFamilyAnswerCoverage,
		// A "glaring" gap on a facet is the more severe form of
		// generic richness regression — when both fire on the same
		// fingerprint, the regression is the symptom.
		Implies: []ViolationKind{ViolRichnessRegression},
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolPrincipalProseUnderfilled, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "v2_oracle", CaveatFamilyID: CaveatFamilyAnswerCoverage,
	})

	// ── Structural / forensic kinds ──
	// Legacy defaultSoftKinds does NOT list StructuralEnumerationDivergence;
	// the kind's SOFT semantic comes from its DefaultSeverity flowing
	// through ViolationProfileFor. Preserved verbatim for migration parity.
	RegisterViolKind(ViolKindSpec{
		Kind: ViolStructuralEnumerationDivergence, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusExtract,
		Layer: "v2_oracle", CaveatFamilyID: CaveatFamilyEnumerationDepth,
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolCrossCitationConflict, DefaultSeverity: SeverityMedium,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusExtract,
		Layer: "v2_oracle", CaveatFamilyID: CaveatFamilyCitationGrounded,
	})

	// ── R10 CGEC frequency bridges (telemetry-only, permanently SOFT) ──
	// Pure operator telemetry — no user-visible caveat (CaveatFamilyID empty).
	RegisterViolKind(ViolKindSpec{
		Kind: ViolDemotionStorm, DefaultSeverity: SeveritySoft,
		SoftByDefault: false, Promotable: false, FallbackLocus: LocusTerminal,
		// Legacy inferViolationLayer falls through to "contract_check".
		Layer: "contract_check",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolForcedReadStorm, DefaultSeverity: SeveritySoft,
		SoftByDefault: false, Promotable: false, FallbackLocus: LocusTerminal,
		Layer: "contract_check",
	})

	// ── P1 / forensic anchors ──
	// Legacy defaultSoftKinds does NOT list SymbolAnchorMismatch
	// explicitly; the kind is treated as Medium retry-eligible. Layer
	// is v2_oracle in the legacy switch.
	RegisterViolKind(ViolKindSpec{
		Kind: ViolSymbolAnchorMismatch, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusExplore,
		Layer: "v2_oracle", CaveatFamilyID: CaveatFamilyCitationGrounded,
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolEnumerationLabelUngrounded, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusExtract,
		Layer: "contract_check", CaveatFamilyID: CaveatFamilyEnumerationDepth,
		SchemaDescriptionFragment: "For enumeration ordered_list / bullet_list blocks, every items[i].label MUST be the verbatim identifier copied from one of the evidence pool's anchor_symbol / subject / object values — fabricated labels are rejected.",
		// Fix path: extractor re-emit_answer_symbol with grounded
		// labels, OR explorer re-investigation. Finalizer is a
		// pure synthesizer with no file tools — cannot ground
		// labels by itself. Routing through finalizer_only burns
		// a round.
		FixableByAgents: []AgentName{AgentExtractor, AgentExplorer},
		// When labels are ungrounded, the extractor's label-emit
		// drift symptom + the underspecified-evidence symptom both
		// derive from the same root.
		Implies: []ViolationKind{
			ViolEnumerationItemLabelExtractorDrift,
			ViolEnumerationEvidenceUnderspecified,
		},
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolEnumerationItemLabelExtractorDrift, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "contract_check", CaveatFamilyID: CaveatFamilyEnumerationDepth,
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolEnumerationLabelHallucinated, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "contract_check", CaveatFamilyID: CaveatFamilyEnumerationDepth,
		SchemaDescriptionFragment: "For enumeration ordered_list / bullet_list / table blocks, every items[i].label whose leading identifier is ≥10 chars MUST be a real function / type / constant declared in the codebase. Fabricated names that no source file declares are silent answer-quality regressions and are rejected.",
		// Fix path: finalizer single-agent — the extractor's slate is
		// fine; only the rendered name is wrong. Re-emitting from the
		// existing slate substitutes a real identifier without
		// re-running explore.
		FixableByAgents: []AgentName{AgentFinalizer},
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolDiagramEdgeEndpointHallucinated, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "contract_check", CaveatFamilyID: CaveatFamilyDiagramFidelity,
		SchemaDescriptionFragment: "For diagram blocks, every mermaid edge endpoint (the `from` and `to` node identifiers) whose leading identifier is ≥10 chars MUST be a real function / type / constant declared in the codebase. Fabricated node names that no source file declares are rejected — the rendered diagram cannot reference identifiers that don't exist.",
		// Fix path: finalizer single-agent — the diagram body is
		// LLM-rendered and the extractor's slate is fine; the only
		// fix is re-emitting the diagram with a real identifier.
		FixableByAgents: []AgentName{AgentFinalizer},
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolLaneBlockKindMismatch, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "contract_check", CaveatFamilyID: CaveatFamilyAcceptance,
		SchemaDescriptionFragment: "Each support lane (Observed artifact / Current grounded code path / Nearest grounded mechanism / Boundary disclosures) declares an Allowed block kinds list. A principal block whose citations come from a lane MUST be one of that lane's allowed kinds — for example, an Observed artifact lane that allows only summary/caveat cannot be rendered as a principal ordered_list or diagram.",
		FixableByAgents:           []AgentName{AgentFinalizer},
	})
}
