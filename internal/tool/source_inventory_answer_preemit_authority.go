package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// SourceInventoryAnswerPreEmitAuthority is the answer-side source-inventory
// authority view. It folds the shared source-inventory snapshot together with
// precise answer-document / structured-handoff sensors.
//
// The sensors remain precise, typed checks; this view is the single consumer
// surface so pre-complete and pre-emit validation do not independently
// reinterpret the same source-inventory universe.
type SourceInventoryAnswerPreEmitAuthority struct {
	Active                    bool                                     `json:"active,omitempty"`
	Blocking                  bool                                     `json:"blocking,omitempty"`
	ReasonCodes               []string                                 `json:"reason_codes,omitempty"`
	Snapshot                  types.SourceInventoryAuthoritySnapshot   `json:"snapshot,omitempty"`
	View                      types.SourceInventoryAnswerAuthorityView `json:"view,omitempty"`
	CandidateUniverseGap      SourceInventoryCandidateUniverseGap      `json:"candidate_universe_gap,omitempty"`
	DuplicateLocationGap      SourceInventoryCandidateUniverseGap      `json:"duplicate_location_gap,omitempty"`
	SurfaceFamilyGap          SourceInventoryCandidateUniverseGap      `json:"surface_family_gap,omitempty"`
	BestUniverseGap           SourceInventoryCandidateUniverseGap      `json:"best_universe_gap,omitempty"`
	ExactAbsenceBlocking      bool                                     `json:"exact_absence_blocking,omitempty"`
	ExactAbsenceSummary       string                                   `json:"exact_absence_summary,omitempty"`
	AcceptedExactUniverse     bool                                     `json:"accepted_exact_universe,omitempty"`
	AcceptedRequestedUniverse bool                                     `json:"accepted_requested_universe,omitempty"`
	StableAggregateFactCount  int                                      `json:"stable_aggregate_fact_count,omitempty"`
	EnumerationSetCount       int                                      `json:"enumeration_set_count,omitempty"`
	EnumerationRowCount       int                                      `json:"enumeration_row_count,omitempty"`
	EnumerationCoverage       types.EnumerationDisplayCoverage         `json:"enumeration_coverage,omitempty"`
}

func BuildSourceInventoryAnswerPreEmitAuthority(ctx *types.BusContext, facts []types.AnswerAggregateFact, docs ...*types.AnswerDocumentV2) SourceInventoryAnswerPreEmitAuthority {
	if ctx == nil {
		return SourceInventoryAnswerPreEmitAuthority{}
	}
	var doc *types.AnswerDocumentV2
	if len(docs) > 0 {
		doc = docs[0]
	}
	var observation types.SourceInventoryObservation
	if ctx.Mutable != nil {
		observation = types.SourceInventoryObservationFromMutable(ctx.Mutable)
	}
	var rm types.RequestModel
	var requiredFiles []string
	if ctx.AnalysisIR != nil {
		rm = ctx.AnalysisIR.RequestModel
		requiredFiles = append([]string(nil), ctx.AnalysisIR.EvidencePlan.RequiredFiles...)
	}
	acceptedExact := SourceInventoryAcceptedClosureCoversExactUniverse(ctx, facts)
	acceptedRequested := SourceInventoryAcceptedClosureCoversRequestedUniverse(ctx, facts)
	snapshot := types.BuildSourceInventoryAuthoritySnapshot(types.SourceInventoryAuthoritySnapshotInput{
		Observation:               observation,
		RequestModel:              rm,
		ExistingAggregateFacts:    facts,
		AcceptedExactUniverse:     acceptedExact,
		AcceptedRequestedUniverse: acceptedRequested,
		RequiredFiles:             requiredFiles,
	})
	candidate := SourceInventoryCandidateUniverseCoverageGap(ctx, facts)
	duplicate := SourceInventoryObservedDuplicateLocationCoverageGap(ctx, facts)
	surfaceFamily := SourceInventoryObservedSurfaceFamilyCoverageGap(ctx, facts)
	best := candidate
	if sourceInventoryCandidateUniverseGapBetter(duplicate, best) {
		best = duplicate
	}
	if sourceInventoryCandidateUniverseGapBetter(surfaceFamily, best) {
		best = surfaceFamily
	}
	var absenceSummary string
	var absenceBlocking bool
	if ctx.AnalysisIR != nil {
		absenceSummary, absenceBlocking = SourceInventoryExactAbsenceNeedsInventoryProofRepoTruth(
			ctx,
			ctx.AnalysisIR.RequestModel.SourceInventoryProfile,
			observation,
		)
	}
	enumCoverageView := types.AnswerDocumentAcceptedEnumerationDisplayCoverage(ctx, nil, doc)
	enumSets := enumCoverageView.Sets
	enumRowCount := 0
	for _, set := range enumSets {
		enumRowCount += len(set.Rows)
	}
	snapshot = types.NormalizeSourceInventoryAuthoritySnapshot(snapshot)
	view := types.BuildSourceInventoryAnswerAuthorityView(snapshot)
	out := SourceInventoryAnswerPreEmitAuthority{
		Active:                    view.Active || candidate.IsActive() || duplicate.IsActive() || surfaceFamily.IsActive() || absenceBlocking,
		Snapshot:                  snapshot,
		View:                      view,
		CandidateUniverseGap:      candidate,
		DuplicateLocationGap:      duplicate,
		SurfaceFamilyGap:          surfaceFamily,
		BestUniverseGap:           best,
		ExactAbsenceBlocking:      absenceBlocking,
		ExactAbsenceSummary:       strings.TrimSpace(absenceSummary),
		AcceptedExactUniverse:     acceptedExact,
		AcceptedRequestedUniverse: acceptedRequested,
		StableAggregateFactCount:  len(facts),
		EnumerationSetCount:       len(enumSets),
		EnumerationRowCount:       enumRowCount,
		EnumerationCoverage:       enumCoverageView.Coverage,
	}
	if out.EnumerationRowCount > 0 {
		out.Active = true
	}
	out.Blocking = best.Blocking || out.ExactAbsenceBlocking
	out.ReasonCodes = sourceInventoryAnswerPreEmitReasonCodes(out)
	return out
}
