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

// DecodeRuntimeMeasurementPublication validates a complete, uniquely published
// receipt against its origin. This supplies display authority only, not a
// causal claim, user-target ownership, or a new observation in the ledger.
func DecodeRuntimeMeasurementPublication(r ObservationRecord) (RuntimeMeasurementPublication, bool) {
	if !RuntimeObservationProducerIsDeterministicQuery(r.Producer) || r.Predicate != "io_inflight" ||
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
	for _, table := range p.Tables {
		if !table.IsValid() || table.ObservationID != r.ID || seen[table.View] {
			return RuntimeMeasurementPublication{}, false
		}
		seen[table.View] = true
	}
	return p, true
}

// BuildRuntimeMeasurementContract reads accepted native results directly so a
// prompt Top-N cannot become an authority limit. Identical re-publications are
// coalesced; conflicting publications keep their duplicate keys and Choices
// withholds those keys. No values are merged across queries or sources.
func BuildRuntimeMeasurementContract(input ObservationLedgerInput) *RuntimeMeasurementContract {
	var out RuntimeMeasurementContract
	seen := map[string]bool{}
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
				if requested.HasExplicitTimeWindows() && known && !requested.ContainsExplicitTimeWindow(start, end) {
					continue
				}
				key, _ := json.Marshal(p)
				if seen[string(key)] {
					continue
				}
				seen[string(key)] = true
				for _, table := range p.Tables {
					table = table.Clone()
					if requested.HasExplicitTimeWindows() && !known {
						table.Label = "Supplementary query (time scope unverified): " + table.Label
						table.Notes = append(table.Notes, "This query has no verified continuous time window; it cannot substitute for the requested-window statistics.")
					}
					out.Tables = append(out.Tables, table)
				}
			}
		}
	}
	if !out.Active() {
		return nil
	}
	return &out
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
