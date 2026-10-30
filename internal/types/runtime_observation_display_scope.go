package types

import "encoding/json"

// RuntimeArtifactDisplayScopeForOriginal selects a request ruler for an
// already verified original input. The path is not itself evidence: callers
// must obtain it from a current private read/preparation receipt, never a
// model parameter or display label. This grants presentation scope only.
func RuntimeArtifactDisplayScopeForOriginal(requested *RuntimeArtifactScopeProfile, originalPath, repoRoot string) (*RuntimeArtifactScopeProfile, bool) {
	if requested == nil || requested.Comparison == nil {
		return requested, true
	}
	if !requested.Comparison.Valid() || originalPath == "" {
		return nil, false
	}
	var matched *RuntimeArtifactScopeProfile
	for i, side := range requested.Comparison.Sides() {
		if !RuntimeArtifactComparisonPathEqual(side.Path, originalPath, repoRoot) {
			continue
		}
		if matched != nil {
			return nil, false
		}
		window, ok := requested.EffectiveComparisonWindow(i)
		if !ok {
			return nil, false
		}
		matched = &RuntimeArtifactScopeProfile{RequestedScope: RuntimeArtifactScopeUnspecified, SourceQuote: side.SourceQuote}
		if window.Valid() {
			matched.RequestedScope = RuntimeArtifactScopeExplicitWindow
			matched.TimeWindows = []RuntimeArtifactTimeWindow{window}
		} else if requested.RequestedScope == RuntimeArtifactScopeFullArtifact {
			matched.RequestedScope = RuntimeArtifactScopeFullArtifact
		}
	}
	return matched, matched != nil
}

// RuntimeObservationDisplayScopes is an ephemeral, read-only index of actual
// query receipts. It is neither serialized authority nor a filtered ledger.
// A copied/cropped ledger retains the receipts; JSON and new hand-built rows
// cannot reconstruct them. Each use rechecks the receiving run and source.
type RuntimeObservationDisplayScopes struct {
	input       ObservationLedgerInput
	queries     map[runtimeDisplayQueryKey]ToolResult
	currentRows map[runtimeDisplayQueryKey]runtimeCurrentQueryPopulation
}

type runtimeDisplayQueryKey string

func runtimeDisplayKey(source ObservationSourceRef) runtimeDisplayQueryKey {
	// Bind producer-owned source/query/clock coordinates only. The ledger
	// independently adds ToolCallID/RawRef/capture display aliases; those are
	// neither source authority nor part of the stable producer-to-ledger join.
	stable := ObservationSourceRef{Path: source.Path, PayloadRef: source.PayloadRef, QueryScopeID: source.QueryScopeID,
		QueryWindowKnown: source.QueryWindowKnown, QueryWindowStartTs: source.QueryWindowStartTs, QueryWindowEndTs: source.QueryWindowEndTs,
		QueryLineRangeKnown: source.QueryLineRangeKnown, QueryLineStart: source.QueryLineStart, QueryLineEnd: source.QueryLineEnd,
		QueryTargetPID: source.QueryTargetPID, QueryTargetThread: source.QueryTargetThread, QueryTargetScope: source.QueryTargetScope,
		TimeDomain: source.TimeDomain, CanonicalTimeDomain: source.CanonicalTimeDomain, ClockAlignment: source.ClockAlignment,
		ClockCalibrated: source.ClockCalibrated, ClockOffsetSec: source.ClockOffsetSec, ClockSlope: source.ClockSlope}
	raw, err := json.Marshal(stable)
	if err != nil {
		return ""
	}
	return runtimeDisplayQueryKey(raw)
}

func BuildRuntimeObservationDisplayScopes(input ObservationLedgerInput) *RuntimeObservationDisplayScopes {
	comparison := input.RequestModel != nil && input.RequestModel.RuntimeArtifactScopeProfile != nil && input.RequestModel.RuntimeArtifactScopeProfile.Comparison != nil
	currentRows := input.RequestModel != nil && input.RequestModel.DiagramHint != nil && input.RequestModel.DiagramHint.Required
	if !comparison && !currentRows {
		return nil
	}
	// Keep only the consumer and source-path context, not entire result bodies.
	index := &RuntimeObservationDisplayScopes{input: ObservationLedgerInput{runtimeMeasurementPairConsumer: input.runtimeMeasurementPairConsumer,
		RequestModel: input.RequestModel, RepoRoot: input.RepoRoot}, queries: map[runtimeDisplayQueryKey]ToolResult{}}
	if currentRows {
		index.currentRows = buildRuntimeCurrentQueryPopulations(input)
	}
	if !comparison {
		return index
	}
	for _, group := range [][]ToolResult{input.ToolResults, input.SystemTraceSupplementResults} {
		for _, result := range group {
			if !result.Success || result.ToolName != "trace_query" || result.ReusedFromRunMemo {
				continue
			}
			seen := map[runtimeDisplayQueryKey]bool{}
			for _, record := range result.Observations {
				ref := record.SourceRef
				key := runtimeDisplayKey(ref)
				if seen[key] {
					continue
				}
				seen[key] = true
				if key == "" || ref.Path == "" || ref.PayloadRef == "" || ref.QueryScopeID == "" || runtimeMeasurementOriginalSource(input, result, ref) == "" {
					continue
				}
				index.queries[key] = ToolResult{TraceQueryWindowReplay: result.TraceQueryWindowReplay, TraceStatistics: result.TraceStatistics}
			}
		}
	}
	return index
}

// Scope preserves the old scope unchanged outside independent comparison.
// Unknown/stale/foreign sources stay in the lossless ledger, but cannot borrow
// another original's requested window on a principal display surface.
func (index *RuntimeObservationDisplayScopes) Scope(record ObservationRecord, requested *RuntimeArtifactScopeProfile) (*RuntimeArtifactScopeProfile, bool) {
	if requested == nil || requested.Comparison == nil {
		return requested, true
	}
	if index == nil {
		return nil, false
	}
	result, found := index.queries[runtimeDisplayKey(record.SourceRef)]
	if !found {
		return nil, false
	}
	input := index.input
	input.RequestModel = &RequestModel{RuntimeArtifactScopeProfile: requested}
	original := runtimeMeasurementOriginalSource(input, result, record.SourceRef)
	return RuntimeArtifactDisplayScopeForOriginal(requested, original, input.RepoRoot)
}

func (ledger ObservationLedger) RuntimeDisplayScope(record ObservationRecord, requested *RuntimeArtifactScopeProfile) (*RuntimeArtifactScopeProfile, bool) {
	return ledger.runtimeDisplayScopes.Scope(record, requested)
}
