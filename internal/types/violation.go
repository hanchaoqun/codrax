package types

// Violation, ViolationKind, and SuspectedRoot live in the types
// package (not internal/analysis/contract) so EvidenceClosure can
// embed a []Violation ledger without creating the circular import
// contract → types → contract. The contract package re-exports these
// via Go type aliases (see internal/analysis/contract/checker.go), so
// call sites continue to write contract.Violation / contract.ViolShape.
//
// Session 11 F1 (2026-04-18) added the Stage / DispatchID /
// EvidenceRefs / SuspectedRoot fields so every enforcer write can
// carry structured self-diagnosis. The F2 aggregator groups events
// by SuspectedRoot.IRField and weights by Confidence so the F3
// IRPatchEngine can decide whether to reconcile upstream IR.

// ViolationKind classifies a contract or enforcer breach.
type ViolationKind string

const (
	// Original contract-checker kinds (pre-session-11).
	ViolShape            ViolationKind = "shape"
	ViolCitation         ViolationKind = "citation"
	ViolMustInclude      ViolationKind = "must_include"
	ViolMustExclude      ViolationKind = "must_exclude"
	ViolAcceptance       ViolationKind = "acceptance"
	ViolSuccessCriterion ViolationKind = "success_criterion"

	// Session 11 F1 — enforcer kinds beyond the classic
	// contract.Check path. Each has a dedicated SuspectedRoot
	// template filled at the enforcer hookup (see the full design
	// doc §1.4 table).

	// ViolGhostAnchor: D2 chain promotion skipped a chain whose
	// anchor file is outside ScannedSet. SuspectedRoot: ScannedSet.
	ViolGhostAnchor ViolationKind = "ghost_anchor"

	// ViolChainDemoted: ERM L0-1 terminal-predicate demote. Only the
	// self-ref subset (terminal literal == primary entity name) is
	// written to the ledger to avoid noise from routine demotes that
	// carry no root-cause signal.
	ViolChainDemoted ViolationKind = "chain_demoted"

	// ViolSelfRefLiteral: emit_evidence / emit_answer_* saw a
	// literal equal to the question's primary entity — self-reference
	// trap. SuspectedRoot: answer_subject.kind.
	ViolSelfRefLiteral ViolationKind = "self_ref_literal"

	// ViolPreCompleteDowngrade: emit_investigation_complete CGEC I3
	// pre-complete simulator rejected a complete-claim because the
	// closure snapshot would not pass the contract checker.
	ViolPreCompleteDowngrade ViolationKind = "pre_complete_downgrade"

	// ViolLiteralFormFailed: C5 G1 literal form check rejected a
	// value-shape literal that does not match the AnswerSubject.Kind
	// regex rule (e.g. skill_name must end with -skill).
	ViolLiteralFormFailed ViolationKind = "literal_form_failed"

	// ViolShapeSwap: B2a/B2b shape swap fired. Kept in the ledger
	// for audit even after C4 removes the can_correct=true rescue —
	// operators still want to observe how often the LLM picks the
	// wrong shape.
	ViolShapeSwap ViolationKind = "shape_swap"

	// Commit 53 P2 — read-mode answer Shape Oracle violations.
	// Defaults are SOFT (advisory only, mirrored to closure but
	// don't trigger finalize retry) so the new gates can ship
	// without breaking edge-case answers; operators can promote
	// to strict via gate_contract_strict_kinds yaml.

	// ViolShapeIntentMismatch: answer Shape contradicts Intent /
	// Scenario from RequestModel (e.g. ShapeValue answer for a
	// "explain how X works" intent). Caught by
	// runAnswerShapeOracle. SuspectedRoot: answer_shape.
	ViolShapeIntentMismatch ViolationKind = "shape_intent_mismatch"

	// ViolSubTopicCountMismatch: doc.AnswerSymbols' distinct
	// SubTopic-bucket count diverges from len(IR.SubTopics).
	// Catches multi-topic answers that under-cover or over-cover.
	// Caught by runAnswerShapeOracle. SuspectedRoot: sub_topics.
	ViolSubTopicCountMismatch ViolationKind = "sub_topic_count_mismatch"

	// ViolDiagramIdentifier: a bare CamelCase / snake_case
	// identifier inside a mermaid block does not resolve to a
	// real symbol via the SymbolOracle. Caught by P4 diagram
	// validator. SuspectedRoot: diagram.
	ViolDiagramIdentifier ViolationKind = "diagram_identifier_unverified"

	// Commit 55 Batch A.3 — declared-count drift between extractor
	// and finalizer. emit_answer_symbol stamps the LLM's self-
	// declared count on Mutable; if the finalizer's rendered
	// doc.Symbols length differs (items dropped by mid-loop
	// grounding etc.), the count claim is no longer accurate.
	// Soft-by-default. SuspectedRoot: answer_symbol.
	ViolDeclaredCountDrift ViolationKind = "declared_count_drift"

	// Commit 62 — answer-prose internal self-contradiction.
	// Independent reviewer LLM compared the answer's summary
	// section vs its body bullets and detected a factual claim
	// asserted with opposite values across the two parts. Default
	// classification is STRICT when
	// pipeline_self_consistency_rewrite_on_contradiction=true
	// (operator opts in to retry-on-contradiction); when false,
	// the kind moves to soft (telemetry-only). SuspectedRoot:
	// answer_summary_body_consistency.
	ViolSelfContradiction ViolationKind = "self_contradiction"
)

// AllViolationKinds returns every declared ViolationKind in a stable
// enumeration order. Mirrors the AllRepairKinds pattern (see
// internal/types/repair.go) and is consumed by the Session 11
// TestAllViolationKindsHaveProducer structural gate — when a new
// Kind is added here it MUST also be wired to at least one producer
// call site. The enumeration list is the single source of truth for
// kind coverage tooling.
func AllViolationKinds() []ViolationKind {
	return []ViolationKind{
		ViolShape,
		ViolCitation,
		ViolMustInclude,
		ViolMustExclude,
		ViolAcceptance,
		ViolSuccessCriterion,
		ViolGhostAnchor,
		ViolChainDemoted,
		ViolSelfRefLiteral,
		ViolPreCompleteDowngrade,
		ViolLiteralFormFailed,
		ViolShapeSwap,
		ViolShapeIntentMismatch,
		ViolSubTopicCountMismatch,
		ViolDiagramIdentifier,
		ViolDeclaredCountDrift,
		ViolSelfContradiction,
	}
}

// SuspectedRoot is the enforcer's structured self-diagnosis attached
// to every Session-11 ledger write. The F2 aggregator groups events
// by IRField and weights by Confidence so the F3 IRPatchEngine can
// decide whether to reconcile the upstream IR.
//
// IRField uses dotted-path notation matching the mutation API in
// internal/analysis/patcher (e.g. "answer_shape",
// "answer_subject.kind", "question_kind", "entity_axes",
// "EvidencePlan.SourceMix", "ScannedSet", "CitationReq",
// "AcceptanceTests"). Reason is a ≤ 140-char hint the operator can
// skim; Confidence is a [0,1] score calibrated per enforcer (see
// full-design doc §1.4).
//
// A zero-value SuspectedRoot (IRField="" Confidence=0) signals
// "this enforcer did not self-diagnose"; F2 filters such events
// out of aggregation so only classified events drive IR patches.
type SuspectedRoot struct {
	IRField    string  // dotted-path target for F3 patcher
	Reason     string  // ≤ 140 chars
	Confidence float64 // [0, 1]
}

// Violation is one specific contract breach with a short reason and
// an optional repair hint the orchestrator can pass to the explorer
// when it reroutes the task.
//
// The three classic fields (Kind/Detail/Repair) remain the stable
// API. Session 11 F1 added four optional fields so every enforcer
// write can carry (a) which stage produced it, (b) which dispatch ID
// (trace-N-iter-M), (c) which chain / evidence / citation it blames,
// and (d) a structured self-diagnosis the F2 aggregator consumes.
// Legacy callers that only populate Kind/Detail/Repair produce
// zero-confidence events that F2 silently skips, preserving backward
// compat.
type Violation struct {
	Kind   ViolationKind
	Detail string
	Repair string // e.g. "collect evidence for <symbol>"

	// Session 11 F1 extensions — all optional.
	Stage         string        // PipelineStage name: "analyze" / "explore" / "extract" / "finalize"
	DispatchID    string        // trace-N-iter-M or equivalent correlation token
	EvidenceRefs  []string      // chain_id / evidence_id / citation_idx references
	SuspectedRoot SuspectedRoot // F2-consumed root-cause hypothesis
}
