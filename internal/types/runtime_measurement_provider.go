package types

import (
	"encoding/json"
	"io"
	"reflect"
	"strings"
)

// RuntimeMeasurementPublication is a producer-owned display receipt. It is
// carried only by successful native query observations, never reconstructed
// from answer prose, model aggregate facts, or abbreviated display notes.
// Tables contain the native producer's formatted values, not model inputs.
type RuntimeMeasurementPublication struct {
	Version       int                       `json:"version"`
	ObservationID string                    `json:"observation_id"`
	Source        ObservationSourceRef      `json:"source"`
	Tables        []RuntimeMeasurementTable `json:"tables"`
}

// RuntimeMeasurementPredicateIsRegistered is an exact producer contract, not
// a semantic guess from observation prose. New domains must opt in explicitly.
func RuntimeMeasurementPredicateIsRegistered(predicate string) bool {
	if predicate == TraceEventSearchInventoryPredicate {
		return true
	}
	return predicate == TraceBusinessSpanPredicate || predicate == "io_inflight" || predicate == "io_activity" || predicate == "scheduler_concurrency" || predicate == "cpu_state_frequency_observation" || predicate == "process_measurements_observation" || predicate == "preferred_frame_rate_observation" || predicate == "measurements_observation" || predicate == "thread_timeline_observation" || predicate == "cpu_frequency_timeline_observation" || predicate == "unpositioned_measurements_observation"
}

func runtimeMeasurementPredicateAllowsView(predicate string, view RuntimeMeasurementView) bool {
	if predicate == TraceEventSearchInventoryPredicate {
		return view == RuntimeMeasurementMembers
	}
	if !RuntimeMeasurementPredicateIsRegistered(predicate) {
		return false
	}
	if predicate == TraceBusinessSpanPredicate {
		return view == RuntimeMeasurementSummary
	}
	if predicate == "thread_timeline_observation" || predicate == "cpu_frequency_timeline_observation" {
		return view == RuntimeMeasurementTimeline
	}
	if predicate == "unpositioned_measurements_observation" {
		return view == RuntimeMeasurementMembers
	}
	if view == RuntimeMeasurementSummary || view == RuntimeMeasurementTimeline {
		return true
	}
	if predicate == "scheduler_concurrency" {
		return view == RuntimeMeasurementMembers || view == RuntimeMeasurementDistribution
	}
	if predicate == "process_measurements_observation" || predicate == "measurements_observation" {
		return view == RuntimeMeasurementMembers
	}
	return predicate == "io_inflight" && view == RuntimeMeasurementMembers ||
		(predicate == "io_activity" || predicate == "cpu_state_frequency_observation" || predicate == "preferred_frame_rate_observation") && view == RuntimeMeasurementDistribution
}

// DecodeRuntimeMeasurementPublication validates a complete, uniquely published
// receipt against its origin. This supplies display authority only, not a
// causal claim, user-target ownership, or a new observation in the ledger.
func DecodeRuntimeMeasurementPublication(r ObservationRecord) (RuntimeMeasurementPublication, bool) {
	if r.Negative || !RuntimeObservationProducerIsDeterministicQuery(r.Producer) || !RuntimeMeasurementPredicateIsRegistered(r.Predicate) ||
		r.Origin != AnswerEvidenceOriginRuntimeArtifact || r.SourceRef.Kind != ObservationSourceRuntimeArtifact ||
		r.GroundingPolicy != ClaimGroundingHard || r.ProvenanceLane != ObservationProvenanceArtifactSpan ||
		r.ID == "" || r.SourceRef.Path == "" || r.SourceRef.PayloadRef == "" || r.SourceRef.QueryScopeID == "" {
		return RuntimeMeasurementPublication{}, false
	}
	var raw string
	found := false
	for _, note := range r.RichNotes {
		if value, ok := strings.CutPrefix(note, TraceNoteKeyRuntimeMeasurement+"="); ok {
			if found { // duplicate publications are ambiguous, even if equal
				return RuntimeMeasurementPublication{}, false
			}
			found = true
			raw = value
		}
	}
	var p RuntimeMeasurementPublication
	d := json.NewDecoder(strings.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&p) != nil || d.Decode(new(any)) != io.EOF || p.Version != 1 ||
		p.ObservationID != r.ID || !reflect.DeepEqual(p.Source, r.SourceRef) || len(p.Tables) == 0 {
		return RuntimeMeasurementPublication{}, false
	}
	seen := map[RuntimeMeasurementView]bool{}
	uniqueMembers := runtimeMeasurementUniqueKeys(raw)
	if r.Predicate == TraceEventSearchInventoryPredicate {
		if !uniqueMembers || !runtimeEventPublicationMatches(r, p) {
			return RuntimeMeasurementPublication{}, false
		}
		for i := range p.Tables {
			p.Tables[i].nativeEventData = true
			p.Tables[i].nativeEventSelectionKey = runtimeEventSelectionKey(nil)
			p.Tables[i].nativeEventScopeNoteIndex = len(p.Tables[i].Notes) - 1
		}
	} else {
		for _, table := range p.Tables {
			if table.EventData != nil {
				return RuntimeMeasurementPublication{}, false
			}
		}
	}
	for i, table := range p.Tables {
		if !table.IsValid() || !runtimeMeasurementPredicateAllowsView(r.Predicate, table.View) || table.ObservationID != r.ID || seen[table.View] ||
			(r.Predicate == "unpositioned_measurements_observation" && table.MemberSet != nil) ||
			(r.Predicate == TraceBusinessSpanPredicate && (table.MemberSet != nil || table.DisplayPopulation != nil)) {
			return RuntimeMeasurementPublication{}, false
		}
		seen[table.View] = true
		start, end, known := TraceObservationContinuousQueryWindow(r.SourceRef)
		if uniqueMembers && table.MemberSet.valid(len(table.Rows)) &&
			(r.ClaimAuthority == ObservationClaimAuthorityUnknown || r.ClaimAuthority == ObservationClaimAuthorityDirectObservation || r.ClaimAuthority == ObservationClaimAuthorityIndependentlyProven) {
			p.Tables[i].coverageScope = &runtimeMeasurementCoverageScope{path: r.SourceRef.Path, windowStart: start, windowEnd: end, windowKnown: known,
				targetPID: r.SourceRef.QueryTargetPID, targetThread: r.SourceRef.QueryTargetThread}
		}
		// Only a complete native, timed query roster can offer display binding.
		// The private current-source check is repeated when building a contract.
		if uniqueMembers && known && table.View == RuntimeMeasurementMembers && r.Predicate != "unpositioned_measurements_observation" &&
			table.DisplayPopulation.valid(len(table.Rows)) && table.DisplayPopulation.QueryScopeID == r.SourceRef.QueryScopeID &&
			(r.ClaimAuthority == ObservationClaimAuthorityUnknown || r.ClaimAuthority == ObservationClaimAuthorityDirectObservation || r.ClaimAuthority == ObservationClaimAuthorityIndependentlyProven) {
			p.Tables[i].nativeTraceDisplayPopulation = true
		}
	}
	return p, true
}

// BuildRuntimeMeasurementContract reads accepted native results directly so a
// prompt Top-N cannot become an authority limit. Identical re-publications are
// coalesced; conflicting publications withhold all views of that observation.
// No values are merged across queries or sources.
func BuildRuntimeMeasurementContract(input ObservationLedgerInput) *RuntimeMeasurementContract {
	out := RuntimeMeasurementContract{Tables: coalesceRuntimeMeasurementPublications(collectRuntimeMeasurementPublications(input))}
	out.qualifyNativeDisplayDimensions(input.RequestModel)
	if !out.Active() {
		return nil
	}
	out.nativeBusinessDiagramAnnotations = buildNativeBusinessDiagramAnnotations(input, &out)
	return &out
}

func collectRuntimeMeasurementPublications(input ObservationLedgerInput) []RuntimeMeasurementPublication {
	var publications []RuntimeMeasurementPublication
	for _, results := range [][]ToolResult{input.ToolResults, input.SystemTraceSupplementResults} {
		for _, result := range results {
			if !result.Success || result.ToolName != "trace_query" {
				continue
			}
			for _, r := range result.Observations {
				p, ok := DecodeRuntimeMeasurementPublication(r)
				if !ok {
					continue
				}
				var requested *RuntimeArtifactScopeProfile
				if input.RequestModel != nil {
					requested = input.RequestModel.RuntimeArtifactScopeProfile
				}
				start, end, known := TraceObservationContinuousQueryWindow(r.SourceRef)
				original := runtimeMeasurementOriginalSource(input, result, r.SourceRef)
				requested, qualified := RuntimeArtifactDisplayScopeForOriginal(requested, original, input.RepoRoot)
				if !qualified {
					continue
				}
				// Event receipts own individual points. A wider lookup envelope
				// must be projected row-by-row, not discard legitimate members or
				// count its right boundary as part of the user's half-open window.
				if requested.HasExplicitTimeWindows() && known && !requested.ContainsExplicitTimeWindow(start, end) && r.Predicate != TraceEventSearchInventoryPredicate {
					continue
				}
				validTables := true
				for i, table := range p.Tables {
					table = table.Clone()
					var valid bool
					table, valid = scopeRuntimeEventTable(table, r, requested)
					if !valid {
						validTables = false
						break
					}
					table = qualifyRuntimeMeasurementNativeDisplay(table, r.SourceRef, original != "", requested)
					if !runtimeMeasurementCoverageSourceAllowed(r, input) {
						table.coverageScope = nil
					}
					if requested.HasExplicitTimeWindows() && (!known || r.Predicate == "unpositioned_measurements_observation") {
						table = runtimeMeasurementUnverifiedWindowTable(table)
					}
					p.Tables[i] = table
				}
				if !validTables {
					continue
				}
				if !result.ReusedFromRunMemo {
					qualifyRuntimeMeasurementFactIdentity(&p, original, result.TraceQueryWindowReplay)
					qualifyRuntimeBusinessMeasurementIdentity(&p, r, original, result.TraceQueryWindowReplay)
				}
				publications = append(publications, p)
			}
		}
	}
	for _, table := range buildNativeLogPresentationTables(input) {
		publications = append(publications, RuntimeMeasurementPublication{Version: 1, ObservationID: table.ObservationID, Tables: []RuntimeMeasurementTable{table}})
	}
	publications = append(publications, buildRuntimeMeasurementPairPublications(input)...)
	publications = append(publications, buildRuntimeMeasurementAcquisitionStatus(input, publications)...)
	return publications
}

// A complete drill-down is still displayable, but cannot stand in for a
// different requested window. Source freshness comes only from the current
// private prepared/replay receipt, not from a path string in the publication.
func qualifyRuntimeMeasurementNativeDisplay(table RuntimeMeasurementTable, source ObservationSourceRef, currentOriginal bool, requested *RuntimeArtifactScopeProfile) RuntimeMeasurementTable {
	start, end, known := TraceObservationContinuousQueryWindow(source)
	qualified := currentOriginal && known
	if requested.HasExplicitTimeWindows() {
		_, exact := requested.MatchExplicitTimeWindow(start, end)
		qualified = qualified && exact
	}
	if !qualified {
		table.nativeTraceDisplayPopulation = false
	}
	return table
}

// Unverified time coverage remains available for an explicit supplementary
// selection, but cannot automatically stand in for requested-window facts.
func runtimeMeasurementUnverifiedWindowTable(table RuntimeMeasurementTable) RuntimeMeasurementTable {
	table = table.Clone()
	table.DefaultPresentation = false
	table.nativeTraceDisplayPopulation = false
	table.Label = "Supplementary query (time scope unverified): " + table.Label
	table.Notes = append(table.Notes, "This query has no verified continuous time window; it cannot substitute for the requested-window statistics.")
	return table
}

func runtimeMeasurementCoverageSourceAllowed(r ObservationRecord, input ObservationLedgerInput) bool {
	index := compileRuntimeArtifactPreflightSourceIndex(input.RuntimeArtifactPreflight, input.RepoRoot)
	if len(index.byPath) == 0 {
		// An observed query source is not proof of the requested input set.
		// In particular, directory discovery may include unqueried captures.
		return false
	}
	// A single population cannot settle multiple independent requested
	// captures without a typed all-source selection contract. Keep their
	// tables available for display; only this new completion authority waits.
	if len(index.byPath) > 1 {
		return false
	}
	r = index.requalify(r)
	for _, path := range []string{r.SourceRef.CaptureIdentityPath, r.SourceRef.Path} {
		key := runtimeArtifactIdentityPathKey(canonicalRuntimeArtifactIdentityPath(path, input.RepoRoot))
		if _, ok := index.byPath[key]; ok {
			return true
		}
	}
	return false
}

func applyRuntimeMeasurementContract(view *AnswerSemanticView, input ObservationLedgerInput) {
	if view == nil {
		return
	}
	view.RuntimeMeasurementContract = BuildRuntimeMeasurementContract(input)
	if !view.RuntimeMeasurementContract.Active() {
		return
	}
	for _, group := range [][]BlockRequirement{view.RequiredBlocks, view.OptionalBlocks} {
		for _, block := range group {
			if block.Kind == BlockTable {
				return
			}
		}
	}
	view.OptionalBlocks = append(view.OptionalBlocks, BlockRequirement{Kind: BlockTable,
		Rationale: "Optional producer-bound measurement table; interpretation remains in separate prose."})
}
