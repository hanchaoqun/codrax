package types

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

// TraceBusinessSpanPredicate is the deterministic trace reader's factual
// ordinary-work interval lane. It deliberately is not a semantic/rank lane.
const TraceBusinessSpanPredicate = "trace_business_span"

// TraceBusinessSpanFactLimit is shared by the visible fact handoff and the
// ordinary-span choice list: never publish an unidentifiable hidden choice.
const TraceBusinessSpanFactLimit = 16

// RuntimeWorkRelationRequested reads only the analyzer's typed request
// carriers. The dedicated profile flag and an active, required legacy answer
// dimension express the same presentation obligation. Neither carrier grants
// runtime evidence authority or selects a work row or conclusion.
func RuntimeWorkRelationRequested(requestModel RequestModel) bool {
	if requestModel.RuntimeQuestionProfile.RequestsRuntimeWorkRelation() {
		return true
	}
	dimensions := requestModel.RequestedAnswerDimensions
	if !dimensions.Active() {
		return false
	}
	for _, dimension := range dimensions.Dimensions {
		if dimension.Required && dimension.Role == RequestedAnswerDimensionRuntimeWorkRelation {
			return true
		}
	}
	return false
}

// RuntimeWorkRelationConclusion is the model-selected conclusion for one
// exact typed runtime-work observation.  The system publishes the available
// rows and evidence ceiling, but never selects this value from request or
// answer prose.
type RuntimeWorkRelationConclusion string

const (
	RuntimeWorkRelationConclusionUnknown                     RuntimeWorkRelationConclusion = ""
	RuntimeWorkRelationConclusionRelatedCausalityUnproven    RuntimeWorkRelationConclusion = "related_causality_unproven"
	RuntimeWorkRelationConclusionTargetSelfWorkObserved      RuntimeWorkRelationConclusion = "target_self_work_observed"
	RuntimeWorkRelationConclusionCausalContributionSupported RuntimeWorkRelationConclusion = "causal_contribution_supported"
	RuntimeWorkRelationConclusionRelationUnproven            RuntimeWorkRelationConclusion = "relation_unproven"
)

func (c RuntimeWorkRelationConclusion) IsValid() bool {
	switch c {
	case RuntimeWorkRelationConclusionRelatedCausalityUnproven,
		RuntimeWorkRelationConclusionTargetSelfWorkObserved,
		RuntimeWorkRelationConclusionCausalContributionSupported,
		RuntimeWorkRelationConclusionRelationUnproven:
		return true
	default:
		return false
	}
}

// RuntimeWorkRelationRow is one exact evidence-backed choice published to the
// finalizer.  Credential and Boundary are closed typed reader facts.  They are
// never inferred from the work name or model-authored prose.
type RuntimeWorkRelationRow struct {
	ObservationID      string
	WorkLabel          string
	Subject            string
	MeasuredDurationMS float64
	AllowedConclusions []RuntimeWorkRelationConclusion
	Credential         string
	Boundary           string
	// Only the deterministic ordinary-span compiler can stamp this value.
	// Kept private and value-only: it neither changes the model wire nor
	// aliases evidence across the existing contract/document clone paths.
	businessMeasurement RuntimeWorkBusinessMeasurement
	businessMeasured    bool
	// FrameCausalityApplicable (RECEIPT-1, §40.31.1 ○14 → §40.32, 2026-09-02)
	// is the analyzer's typed frame decision (FrameCausalityQualifierApplicable)
	// stamped on the system-bound row at contract build time: the receipt's
	// unproven-mechanism wording names a dropped frame / frame deadline only
	// when the request is a frame question; otherwise it speaks the generic
	// target-wait / completion mechanism. System-owned — the model authors
	// only observation_id + conclusion.
	FrameCausalityApplicable bool
}

// RuntimeWorkBusinessMeasurement preserves the two existing observation
// rulers. MeasuredDurationMS remains the query-clipped value on the row;
// FullDurationMS is optional same-record paired-marker metadata, never a
// replacement measurement or a causal credential. Callers receive a copy.
type RuntimeWorkBusinessMeasurement struct {
	QueryStartTs, QueryEndTs float64
	StartTs, EndTs           float64
	FullStartTs, FullEndTs   float64
	FullDurationMS           float64
	FullKnown                bool
}

func (r RuntimeWorkRelationRow) BusinessSpanMeasurement() (RuntimeWorkBusinessMeasurement, bool) {
	return r.businessMeasurement, r.businessMeasured
}

// RuntimeWorkRelationContract is active only when the analyzer explicitly
// declared the independent work/span-to-target subquestion and the ledger has
// at least one exact measured-work row. Ordinary spans retain an unproven
// work-to-target relation, independent of semantic optimization credentials.
type RuntimeWorkRelationContract struct {
	Rows []RuntimeWorkRelationRow
}

func (c *RuntimeWorkRelationContract) Active() bool {
	return c != nil && len(c.Rows) > 0
}

// AnswerRuntimeWorkRelationReceipt is authored by the model on one visible
// principal block.  Only the exact row id and conclusion cross the wire.  The
// remaining facts are bound from the typed contract after validation so the
// renderer can display exact name/duration/credential/boundary without asking
// the model to recopy numbers or letting the system choose the conclusion.
type AnswerRuntimeWorkRelationReceipt struct {
	ObservationID string                        `json:"observation_id"`
	Conclusion    RuntimeWorkRelationConclusion `json:"conclusion"`
	BoundRow      RuntimeWorkRelationRow        `json:"-"`
}

func (r *AnswerRuntimeWorkRelationReceipt) IsBound() bool {
	return r != nil && strings.TrimSpace(r.BoundRow.ObservationID) != ""
}

func BindRuntimeWorkRelationReceipt(r *AnswerRuntimeWorkRelationReceipt, contract *RuntimeWorkRelationContract) bool {
	if r == nil || !contract.Active() || !r.Conclusion.IsValid() {
		return false
	}
	id := strings.TrimSpace(r.ObservationID)
	for _, row := range contract.Rows {
		if id == row.ObservationID && runtimeWorkRelationConclusionAllowed(r.Conclusion, row.AllowedConclusions) {
			r.ObservationID = id
			r.BoundRow = row
			return true
		}
	}
	return false
}

// BuildRuntimeWorkRelationContract compiles exact typed measured-work rows.
// It does not inspect the request or answer prose. requested comes
// from RuntimeQuestionProfile.RuntimeWorkRelationRequested.
func BuildRuntimeWorkRelationContract(input ObservationLedgerInput, requested bool) *RuntimeWorkRelationContract {
	if !requested {
		return nil
	}
	ledger := CompileObservationLedger(input)
	set := CompileTraceCausalProjectionSet(ledger)
	frameQuestion := FrameCausalityQualifierApplicable(input.RequestModel)
	seen := map[string]bool{}
	var rows []RuntimeWorkRelationRow
	for _, projection := range set.Projections {
		for _, node := range projection.SemanticSpans {
			id := strings.TrimSpace(node.EvidenceID)
			label := strings.TrimSpace(node.SpanName)
			if id == "" || label == "" || seen[id] {
				continue
			}
			row := runtimeWorkRelationRowFromNode(id, label, node)
			row.FrameCausalityApplicable = frameQuestion
			if row.MeasuredDurationMS <= 0 || math.IsNaN(row.MeasuredDurationMS) || math.IsInf(row.MeasuredDurationMS, 0) {
				continue
			}
			seen[id] = true
			rows = append(rows, row)
		}
	}
	for i, record := range TraceBusinessSpanFacts(ledger, input.RequestModel) {
		if i >= TraceBusinessSpanFactLimit {
			break
		}
		row, ok := runtimeWorkRelationRowFromBusinessSpan(record)
		if !ok || seen[row.ObservationID] {
			continue
		}
		row.FrameCausalityApplicable = frameQuestion
		seen[row.ObservationID] = true
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].ObservationID < rows[j].ObservationID })
	return &RuntimeWorkRelationContract{Rows: rows}
}

// TraceBusinessSpanFacts exposes the same factual supply to receipt and prompt
// consumers. Repeated queries of one exact source/window/span do not multiply
// the work, while distinct artifacts or selected windows never collapse.
// Callers may supply a window-filtered ledger; this reader never broadens it.
func TraceBusinessSpanFacts(ledger ObservationLedger, requestModel *RequestModel) []ObservationRecord {
	type factKey struct {
		path, artifact, timeDomain, canonicalTimeDomain       string
		window, actualWindow, actualMS, subject, label, value string
		span                                                  ObservationSpan
		queryWindowKnown                                      bool
		queryStart, queryEnd                                  float64
		clockAlignment                                        string
		clockCalibrated                                       bool
		clockOffset, clockSlope                               string
	}
	seen := map[factKey]bool{}
	var facts []ObservationRecord
	for _, record := range ledger.Records {
		if _, ok := runtimeWorkRelationRowFromBusinessSpan(record); !ok {
			continue
		}
		if requested := ledger.RuntimeArtifactScopeProfile; requested.HasExplicitTimeWindows() {
			start, end, _ := TraceCausalProjectionSelectedWindowNote(record.RichNotes)
			if len(requested.ExplicitTimeWindows()) > 1 && record.SourceRef.QueryWindowKnown {
				start, end = record.SourceRef.QueryWindowStartTs, record.SourceRef.QueryWindowEndTs
			}
			if !requested.ContainsExplicitTimeWindow(start, end) {
				continue
			}
		}
		// Mirror the ordinary-fact named-target prompt lane. Causal diagnosis
		// deliberately keeps dependency-thread work; finite target-only lookup
		// must not publish a receipt for work its prompt has filtered away.
		if requestModel != nil && requestModel.RuntimeQuestionProfile.CarriesBoundedFactFamilies() &&
			!requestModel.RuntimeQuestionProfile.RequestsFactFamily(RuntimeQuestionFactOtherObservedValue) {
			named := false
			for _, target := range requestModel.RuntimeTargets {
				if !RuntimeTargetIsExplorationCursorSource(target.Source) &&
					((target.PID > 0 && target.PID <= RuntimeTargetMaxPID) || strings.TrimSpace(target.Thread) != "") {
					named = true
					break
				}
			}
			if named && !ObservationRecordMatchesUserRuntimeTarget(record, requestModel) {
				continue
			}
		}
		key := factKey{
			path: record.SourceRef.Path, artifact: record.SourceRef.ArtifactID,
			timeDomain: record.SourceRef.TimeDomain, canonicalTimeDomain: record.SourceRef.CanonicalTimeDomain,
			window:       traceObservationRichNoteValue(record.RichNotes, TraceNoteKeySelectedWindow),
			actualWindow: traceObservationRichNoteValue(record.RichNotes, TraceNoteKeyActualWindow),
			actualMS:     traceObservationRichNoteValue(record.RichNotes, TraceNoteKeyActualImpactMS),
			subject:      record.Subject, label: record.Object, value: record.Value, span: record.Span,
			queryWindowKnown: record.SourceRef.QueryWindowKnown,
			queryStart:       record.SourceRef.QueryWindowStartTs, queryEnd: record.SourceRef.QueryWindowEndTs,
			clockAlignment: record.SourceRef.ClockAlignment, clockCalibrated: record.SourceRef.ClockCalibrated,
		}
		if record.SourceRef.ClockOffsetSec != nil {
			key.clockOffset = strconv.FormatFloat(*record.SourceRef.ClockOffsetSec, 'g', -1, 64)
		}
		if record.SourceRef.ClockSlope != nil {
			key.clockSlope = strconv.FormatFloat(*record.SourceRef.ClockSlope, 'g', -1, 64)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		facts = append(facts, record)
	}
	return facts
}

// A paired ordinary business interval proves only its name, owner and elapsed
// time. Never reinterpret its duration as CPU time, removable time or a chain
// contribution, even if its thread also occurs in a wakeup-chain projection.
func runtimeWorkRelationRowFromBusinessSpan(record ObservationRecord) (RuntimeWorkRelationRow, bool) {
	if record.Predicate != TraceBusinessSpanPredicate || !RuntimeObservationProducerIsDeterministicQuery(record.Producer) ||
		record.Origin != AnswerEvidenceOriginRuntimeArtifact ||
		record.SourceRef.Kind != ObservationSourceRuntimeArtifact ||
		record.ProvenanceLane != ObservationProvenanceArtifactSpan ||
		record.GroundingPolicy != ClaimGroundingHard || record.Unit != "ms" ||
		strings.TrimSpace(record.ID) == "" || strings.TrimSpace(record.Subject) == "" ||
		strings.TrimSpace(record.Object) == "" || strings.TrimSpace(record.SourceRef.Path) == "" ||
		record.Span.LineStart <= 0 || record.Span.LineEnd < record.Span.LineStart ||
		!TraceCausalProjectionWindowPresent(record.Span.StartTs, record.Span.EndTs) {
		return RuntimeWorkRelationRow{}, false
	}
	start, end, windowOK := TraceCausalProjectionSelectedWindowNote(record.RichNotes)
	ms, err := strconv.ParseFloat(record.Value, 64)
	// selected_window is a producer-formatted microsecond receipt. Allow only
	// that fixed formatting half-unit, not an inferred timing relationship.
	if !windowOK || record.Span.StartTs < start-0.000000501 || record.Span.EndTs > end+0.000000501 ||
		err != nil || ms <= 0 || math.IsNaN(ms) || math.IsInf(ms, 0) ||
		math.Abs(ms-(record.Span.EndTs-record.Span.StartTs)*1000) > 0.000501 {
		return RuntimeWorkRelationRow{}, false
	}
	for _, value := range []float64{start, end, record.Span.StartTs, record.Span.EndTs} {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return RuntimeWorkRelationRow{}, false
		}
	}
	measurement := RuntimeWorkBusinessMeasurement{
		QueryStartTs: start, QueryEndTs: end,
		StartTs: record.Span.StartTs, EndTs: record.Span.EndTs,
	}
	fullStart, fullEnd, fullWindowOK := TraceCausalProjectionParseWindowValue(traceObservationRichNoteValue(record.RichNotes, TraceNoteKeyActualWindow))
	fullMS, fullErr := strconv.ParseFloat(traceObservationRichNoteValue(record.RichNotes, TraceNoteKeyActualImpactMS), 64)
	// Both notes are producer-formatted: endpoints to 1us and duration to
	// 1us. Accommodate only their combined rounding, never infer a full pair
	// from another observation, a label match, or one surviving note.
	if fullWindowOK && fullErr == nil && fullMS > 0 &&
		!math.IsNaN(fullMS) && !math.IsInf(fullMS, 0) &&
		!math.IsNaN(fullStart) && !math.IsInf(fullStart, 0) && !math.IsNaN(fullEnd) && !math.IsInf(fullEnd, 0) &&
		fullStart <= record.Span.StartTs+0.000000501 && fullEnd >= record.Span.EndTs-0.000000501 &&
		(fullStart < record.Span.StartTs-0.000000501 || fullEnd > record.Span.EndTs+0.000000501) &&
		math.Abs(fullMS-(fullEnd-fullStart)*1000) <= 0.001501 {
		measurement.FullStartTs, measurement.FullEndTs = fullStart, fullEnd
		measurement.FullDurationMS, measurement.FullKnown = fullMS, true
	}
	return RuntimeWorkRelationRow{
		ObservationID: record.ID, WorkLabel: record.Object, Subject: record.Subject,
		MeasuredDurationMS: ms,
		AllowedConclusions: []RuntimeWorkRelationConclusion{RuntimeWorkRelationConclusionRelationUnproven},
		Credential:         "none", Boundary: "work_to_target_relation_unproven",
		businessMeasurement: measurement, businessMeasured: true,
	}, true
}

func runtimeWorkRelationRowFromNode(id, label string, node TraceCausalProjectionNode) RuntimeWorkRelationRow {
	row := RuntimeWorkRelationRow{
		ObservationID: id,
		WorkLabel:     label,
		Subject:       strings.TrimSpace(node.Subject),
	}
	switch {
	case node.ActualImpactMS > 0:
		row.MeasuredDurationMS = node.ActualImpactMS
	case node.ImpactMS > 0:
		row.MeasuredDurationMS = node.ImpactMS
	case node.EndTs > node.StartTs:
		row.MeasuredDurationMS = (node.EndTs - node.StartTs) * 1000
	}
	switch strings.TrimSpace(node.OnChainBasis) {
	case TraceCausalOnChainBasisHostWakeupEdgeSpan:
		row.AllowedConclusions = []RuntimeWorkRelationConclusion{
			RuntimeWorkRelationConclusionRelatedCausalityUnproven,
			RuntimeWorkRelationConclusionRelationUnproven,
		}
		row.Credential = "host_direct_wakeup_edge"
		row.Boundary = "work_completion_target_wait_and_frame_causality_unproven"
	case TraceCausalOnChainBasisSemanticChainIntervalRelation:
		row.AllowedConclusions = []RuntimeWorkRelationConclusion{
			RuntimeWorkRelationConclusionRelatedCausalityUnproven,
			RuntimeWorkRelationConclusionRelationUnproven,
		}
		row.Credential = "typed_chain_interval_overlap"
		row.Boundary = "work_completion_target_wait_and_frame_causality_unproven"
	case TraceCausalOnChainBasisSelfDeterministicSpan:
		row.AllowedConclusions = []RuntimeWorkRelationConclusion{
			RuntimeWorkRelationConclusionTargetSelfWorkObserved,
			RuntimeWorkRelationConclusionRelationUnproven,
		}
		row.Credential = "target_self_execution"
		row.Boundary = "frame_or_deadline_causality_unproven"
	default:
		if node.EffectiveImpactPublished && node.EffectiveImpactMS > 0 {
			row.AllowedConclusions = []RuntimeWorkRelationConclusion{
				RuntimeWorkRelationConclusionCausalContributionSupported,
				RuntimeWorkRelationConclusionRelatedCausalityUnproven,
				RuntimeWorkRelationConclusionRelationUnproven,
			}
			row.Credential = "typed_chain_effective_attribution"
			row.Boundary = "bounded_by_typed_chain_evidence"
		} else {
			row.AllowedConclusions = []RuntimeWorkRelationConclusion{
				RuntimeWorkRelationConclusionRelationUnproven,
			}
			row.Credential = "none"
			row.Boundary = "work_to_target_relation_unproven"
		}
	}
	return row
}

func runtimeWorkRelationConclusionAllowed(got RuntimeWorkRelationConclusion, allowed []RuntimeWorkRelationConclusion) bool {
	for _, candidate := range allowed {
		if got == candidate {
			return true
		}
	}
	return false
}
