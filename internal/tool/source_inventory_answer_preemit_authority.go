package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// SourceInventoryAnswerPreEmitAuthority is the answer-side source-inventory
// authority view. It folds the shared source-inventory snapshot together with
// the precise pre-emit sensors that need the visible answer document/fact lane.
//
// The sensors remain precise, typed checks; this view is the single consumer
// surface so final-answer validation does not independently reinterpret the
// same source-inventory universe.
type SourceInventoryAnswerPreEmitAuthority struct {
	Active                    bool                                   `json:"active,omitempty"`
	Blocking                  bool                                   `json:"blocking,omitempty"`
	ReasonCodes               []string                               `json:"reason_codes,omitempty"`
	Snapshot                  types.SourceInventoryAuthoritySnapshot `json:"snapshot,omitempty"`
	CandidateUniverseGap      SourceInventoryCandidateUniverseGap    `json:"candidate_universe_gap,omitempty"`
	DuplicateLocationGap      SourceInventoryCandidateUniverseGap    `json:"duplicate_location_gap,omitempty"`
	SurfaceFamilyGap          SourceInventoryCandidateUniverseGap    `json:"surface_family_gap,omitempty"`
	BestUniverseGap           SourceInventoryCandidateUniverseGap    `json:"best_universe_gap,omitempty"`
	ExactAbsenceBlocking      bool                                   `json:"exact_absence_blocking,omitempty"`
	ExactAbsenceSummary       string                                 `json:"exact_absence_summary,omitempty"`
	AcceptedExactUniverse     bool                                   `json:"accepted_exact_universe,omitempty"`
	AcceptedRequestedUniverse bool                                   `json:"accepted_requested_universe,omitempty"`
	StableAggregateFactCount  int                                    `json:"stable_aggregate_fact_count,omitempty"`
}

func BuildSourceInventoryAnswerPreEmitAuthority(ctx *types.BusContext, facts []types.AnswerAggregateFact) SourceInventoryAnswerPreEmitAuthority {
	if ctx == nil {
		return SourceInventoryAnswerPreEmitAuthority{}
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
	out := SourceInventoryAnswerPreEmitAuthority{
		Active:                    snapshot.Active || candidate.IsActive() || duplicate.IsActive() || surfaceFamily.IsActive() || absenceBlocking,
		Snapshot:                  types.NormalizeSourceInventoryAuthoritySnapshot(snapshot),
		CandidateUniverseGap:      candidate,
		DuplicateLocationGap:      duplicate,
		SurfaceFamilyGap:          surfaceFamily,
		BestUniverseGap:           best,
		ExactAbsenceBlocking:      absenceBlocking,
		ExactAbsenceSummary:       strings.TrimSpace(absenceSummary),
		AcceptedExactUniverse:     acceptedExact,
		AcceptedRequestedUniverse: acceptedRequested,
		StableAggregateFactCount:  len(facts),
	}
	out.Blocking = best.Blocking || out.ExactAbsenceBlocking
	out.ReasonCodes = sourceInventoryAnswerPreEmitReasonCodes(out)
	return out
}

func sourceInventoryAnswerPreEmitReasonCodes(a SourceInventoryAnswerPreEmitAuthority) []string {
	var out []string
	add := func(code string) {
		code = strings.TrimSpace(code)
		if code == "" {
			return
		}
		for _, existing := range out {
			if existing == code {
				return
			}
		}
		out = append(out, code)
	}
	for _, code := range a.Snapshot.ReasonCodes {
		add("snapshot:" + code)
	}
	if a.CandidateUniverseGap.Blocking {
		add("candidate_universe_blocking")
	}
	if a.DuplicateLocationGap.Blocking {
		add("duplicate_location_blocking")
	}
	if a.SurfaceFamilyGap.Blocking {
		add("surface_family_blocking")
	}
	if a.ExactAbsenceBlocking {
		add("exact_absence_requires_inventory_proof")
	}
	if a.AcceptedExactUniverse {
		add("accepted_exact_universe")
	}
	if a.AcceptedRequestedUniverse {
		add("accepted_requested_universe")
	}
	return out
}
