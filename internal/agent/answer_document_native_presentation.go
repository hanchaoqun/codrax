package agent

import (
	"encoding/json"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// A dispatch-local receipt for actual native presentation, not existence in
// the audit ledger. Renderers record it only after scope projection, conflict
// checks and all their display budgets. It conveys no factual/causal authority
// and is never persisted into a future dispatch.
type answerDocNativePresentationReceipt struct {
	measurementViews       map[string]bool
	measurementViewDetails map[string]answerDocMeasurementViewPresentation
	// Detached native owners from the same fresh measurement contract; only
	// an actually published full-table receipt can use them for field handoff.
	businessAnnotations []types.NativeBusinessDiagramAnnotation
	queries             map[string]bool
	records             map[string]bool
	eventMembers        map[answerDocEventMemberKey]bool
	// Selection and publication completeness are independent: reference
	// previews must not hide exact flat member quantities from the same query.
	// This private routing receipt cannot satisfy completeFor or remove notes.
	samplingSelections map[string]bool
	// Answer-grade aggregates and audit-reference previews have independent
	// completeness. This never signs an omitted reference or event member.
	samplingAggregates map[string]bool
	// Private display equivalence; never a replacement for query identity.
	samplingIdentities      map[string]string
	samplingCohortIDs       map[string]map[string]bool
	nativeSources           map[string]string
	samplingTableIdentities map[samplingFactViewKey]string
	presentationBytes       int
	presentationLimit       int
	navigationBytes         int
	navigationOmitted       int
}

func (p *answerDocNativePresentationReceipt) observeSamplingSelection(r types.ObservationRecord, qualified bool) {
	if p == nil || nativePresentationFamily(r) != "perf_sampling" {
		return
	}
	key := nativePresentationQueryKey(r)
	if key == "" {
		return
	}
	if p.samplingSelections == nil {
		p.samplingSelections = make(map[string]bool)
	}
	if prior, exists := p.samplingSelections[key]; exists {
		qualified = qualified && prior
	}
	p.samplingSelections[key] = qualified
}

// A query can publish several independent families. A complete tree is not
// publication of the same query's I/O, memory or scheduling measurements.
// These are exact producer predicates, never words in a question or answer.
// Add a new provider only with its own post-budget publication receipt.
func nativePresentationFamily(r types.ObservationRecord) string {
	switch r.Predicate {
	case tool.TracePerfSamplingTreePredicate, "perf_sample_statistical_caliber":
		return "perf_sampling"
	case "perf_sample_top_symbol":
		return "perf_flat_members"
	case "perf_sample_top_callchain":
		return "perf_recorded_callchains"
	case types.RuntimeSamplingMeasurementPredicate:
		return "perf_fact_tables"
	case types.TraceEventSearchInventoryPredicate:
		return "event_inventory"
	default:
		return ""
	}
}

func nativePresentationKey(r types.ObservationRecord) string {
	family, query := nativePresentationFamily(r), nativePresentationQueryKey(r)
	if family == "" || query == "" {
		return ""
	}
	if family == "perf_fact_tables" {
		// One query can supply several independently budgeted fact groups.
		// Bind the complete compiled record, not a same-query sibling's key.
		return family + ":" + traceEventPublicationRecordKey(r)
	}
	return family + ":" + query
}

func nativePresentationQueryKey(r types.ObservationRecord) string {
	ref := r.SourceRef
	if r.Origin != types.AnswerEvidenceOriginRuntimeArtifact ||
		!types.RuntimeObservationProducerIsDeterministicQuery(r.Producer) ||
		ref.Path == "" || ref.PayloadRef == "" || ref.QueryScopeID == "" {
		return ""
	}
	// Query/payload identity retains producer generation. Row coordinates do
	// not identify a different query; target/window/clock must still agree.
	data, _ := json.Marshal([]any{ref.Kind, ref.Path, ref.CaptureIdentityPath, ref.ArtifactID, ref.ArtifactKind, ref.PayloadRef, ref.QueryScopeID,
		ref.QueryWindowKnown, ref.QueryWindowStartTs, ref.QueryWindowEndTs,
		ref.QueryTargetPID, ref.QueryTargetThread, ref.QueryTargetScope,
		ref.QueryLineRangeKnown, ref.QueryLineStart, ref.QueryLineEnd,
		ref.TimeDomain, ref.CanonicalTimeDomain, ref.ClockAlignment,
		ref.ClockCalibrated, ref.ClockOffsetSec, ref.ClockSlope})
	return string(data)
}

func (p *answerDocNativePresentationReceipt) observe(r types.ObservationRecord, complete bool) {
	if p == nil {
		return
	}
	key := nativePresentationKey(r)
	if key == "" {
		return
	}
	if p.records == nil {
		p.records = map[string]bool{}
	}
	recordKey := traceEventPublicationRecordKey(r)
	if prior, found := p.records[recordKey]; found {
		complete = complete && prior
	}
	p.records[recordKey] = complete
	if p.queries == nil {
		p.queries = make(map[string]bool)
	}
	if prior, exists := p.queries[key]; exists {
		complete = complete && prior
	}
	p.queries[key] = complete
	if complete && types.IsValidTraceEventSearchInventoryRecord(r) {
		if p.eventMembers == nil {
			p.eventMembers = make(map[answerDocEventMemberKey]bool)
		}
		for _, row := range r.EventSearchInventory.Rows {
			if member, known := answerDocEventMemberIdentity(answerDocEventMemberSourceKey(r.SourceRef), row); known {
				p.eventMembers[member] = true
			}
		}
	}
}

// Exact point mirrors can reuse their own fully presented inventory. Matching
// the parent query alone is insufficient, even for another event predicate.
func (p *answerDocNativePresentationReceipt) eventMirrorKey(r types.ObservationRecord) string {
	member := answerDocEventMemberKey{}
	if types.NativeRuntimeFactPresentationKind(r) == "trace_record" &&
		r.ProvenanceLane == types.ObservationProvenanceArtifactSpan && r.Role == types.AnswerAggregateRoleSupportingCoverage &&
		r.Predicate != "" && r.ClaimKey == "evidence_fact:"+r.Predicate &&
		r.Span.LineStart > 0 && r.Span.LineStart == r.Span.LineEnd && r.Span.StartTs == r.Span.EndTs {
		member = answerDocEventMemberKey{answerDocEventMemberSourceKey(r.SourceRef), r.Span.LineStart, r.Span.StartTs, r.Predicate}
	} else if r.Predicate == "scheduler_wakeup_event" {
		if f, valid := types.DecodeRuntimeWakeupEvent(r); valid {
			member = answerDocEventMemberKey{answerDocEventMemberSourceKey(r.SourceRef), f.Line, f.Timestamp, "sched_wakeup"}
		}
	}
	if !p.eventMembers[member] {
		return ""
	}
	return "event_inventory:" + nativePresentationQueryKey(r)
}

func (p *answerDocNativePresentationReceipt) completeFor(ctx *types.AgentContext) bool {
	if p == nil || len(p.queries) == 0 {
		return false
	}
	seen := false
	for _, r := range answerDocObservationLedger(ctx).Records {
		if r.Origin != types.AnswerEvidenceOriginRuntimeArtifact ||
			!types.RuntimeObservationProducerIsDeterministicQuery(r.Producer) {
			continue
		}
		key := nativePresentationKey(r)
		if key == "" {
			key = p.eventMirrorKey(r)
		}
		if key == "" || !p.queries[key] {
			return false
		}
		seen = true
	}
	return seen
}

// Replace only the current lifecycle synopsis, not independent investigation
// notes. A bounded artifact fact request with no load-bearing source/causal
// lane can use its actually published native facts instead of a second model
// restatement. Missing, conflicted or budget-omitted facts retain the advisory.
func (p *answerDocNativePresentationReceipt) replacesCurrentSynthesis(ctx *types.AgentContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.Mutable == nil {
		return false
	}
	rm := &ctx.AnalysisIR.RequestModel
	q := rm.RuntimeQuestionProfile
	if !q.BoundedFactSet() || q.RequestsRuntimeWorkRelation() || q.RequestsFrameCausality() {
		return false
	}
	plan := answerSurfacePlan(ctx)
	if plan == nil || plan.StableInvestigationResultKind != "resolved" ||
		plan.CurrentStatusDiagnosticRequired ||
		len(ctx.Mutable.StableInvestigationRelationClaims()) > 0 {
		return false
	}
	a := types.BuildRuntimeSourceAnswerAuthoritySnapshotForAgentContext(ctx, answerDocObservationLedger(ctx))
	// Generic legacy intent may request a current_source display origin even
	// when its source obligation is only soft and no code was inspected. The
	// precise shared source authority, not that default label, owns this lane.
	if a.KeepsCurrentSourceLaneLoadBearing() || !a.AllowsRuntimeEvidenceWithoutCurrentSource() ||
		a.CurrentSourceExplanationRequested || a.CurrentSourceRecordCount > 0 || a.ExactCurrentSourceSupportCount > 0 {
		return false
	}
	return strings.TrimSpace(plan.StableInvestigationReason) != "" && p.answerFactsCompleteFor(ctx)
}
