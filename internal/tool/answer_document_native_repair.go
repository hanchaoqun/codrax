package tool

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/hanchaoqun/codrax/internal/types"
)

const runtimeMeasurementDiagnosticsKey = "runtime_measurement_diagnostics"

// Summary and structured repair both use this one bounded projection. Their
// combined diagnostic JSON is at most 4 KiB; at most 16 short field paths are
// added to Fields. Candidate IDs are omitted whole, never truncated.
const runtimeMeasurementDiagnosticJSONLimit = 2048
const runtimeMeasurementDiagnosticCountLimit = 16

type runtimeMeasurementRepairPair struct {
	ObservationID string                       `json:"observation_id"`
	View          types.RuntimeMeasurementView `json:"view"`
}

type runtimeMeasurementRepairDiagnostic struct {
	Field         string                         `json:"field"`
	ExpectedShape string                         `json:"expected_shape"`
	ActualKind    string                         `json:"actual_kind"`
	Problem       string                         `json:"problem"`
	CurrentPairs  []runtimeMeasurementRepairPair `json:"current_pairs,omitempty"`
	PairCount     int                            `json:"current_pair_count,omitempty"`
	PairsOmitted  int                            `json:"current_pairs_omitted,omitempty"`
}

type runtimeMeasurementDiagnosticSet struct {
	Entries []runtimeMeasurementRepairDiagnostic
	Total   int
}

func (s *runtimeMeasurementDiagnosticSet) add(d runtimeMeasurementRepairDiagnostic) {
	s.Total++
	if len(s.Entries) >= runtimeMeasurementDiagnosticCountLimit {
		return
	}
	for {
		candidate := append(append([]runtimeMeasurementRepairDiagnostic(nil), s.Entries...), d)
		wire, _ := json.Marshal(candidate)
		if len(wire) <= runtimeMeasurementDiagnosticJSONLimit {
			s.Entries = candidate
			break
		}
		if len(d.CurrentPairs) == 0 {
			break
		}
		d.CurrentPairs = d.CurrentPairs[:len(d.CurrentPairs)-1]
		d.PairsOmitted++
	}
}

func (s runtimeMeasurementDiagnosticSet) summary() string {
	wire, _ := json.Marshal(s.Entries)
	return fmt.Sprintf("Native selection diagnostics (shown=%d total=%d omitted=%d): %s", len(s.Entries), s.Total, s.Total-len(s.Entries), wire)
}

func runtimeMeasurementPairDiagnostics(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) runtimeMeasurementDiagnosticSet {
	var diagnostics runtimeMeasurementDiagnosticSet
	if doc == nil {
		return diagnostics
	}
	for i, block := range doc.Blocks {
		if d := runtimeMeasurementPairDiagnostic(block, view, fmt.Sprintf("blocks[%d].runtime_measurement", i)); d != nil {
			diagnostics.add(*d)
		}
	}
	return diagnostics
}

// Diagnostics expose only the same exact pairs the current contract publishes.
// They never repair a selector or rank a candidate by prefix/text similarity.
func runtimeMeasurementPairDiagnostic(block types.AnswerBlock, view *types.AnswerSemanticView, path string) *runtimeMeasurementRepairDiagnostic {
	if block.RuntimeMeasurement == nil {
		return nil
	}
	var contract *types.RuntimeMeasurementContract
	if view != nil {
		contract = view.RuntimeMeasurementContract
	}
	choices := contract.Choices()
	idKnown, pairKnown := false, false
	for _, choice := range choices {
		if choice.ObservationID == block.RuntimeMeasurement.ObservationID {
			idKnown = true
			pairKnown = pairKnown || choice.View == block.RuntimeMeasurement.View
		}
	}
	if pairKnown {
		return nil // Other failures retain their existing structural diagnosis.
	}
	d := &runtimeMeasurementRepairDiagnostic{
		Field: path + ".observation_id", ExpectedShape: "string (exact current observation_id)",
		ActualKind: "string", Problem: "unpublished_observation_id",
	}
	if idKnown {
		d.Field, d.ExpectedShape, d.Problem = path+".view", "string (view published for this exact observation_id)", "unpublished_observation_view_pair"
	}
	if len(choices) == 0 {
		d.Problem = "no_current_measurement_pairs"
	}
	bytesUsed := 0
	for _, choice := range choices {
		if idKnown && choice.ObservationID != block.RuntimeMeasurement.ObservationID {
			continue
		}
		d.PairCount++
		pair := runtimeMeasurementRepairPair{ObservationID: choice.ObservationID, View: choice.View}
		wire, _ := json.Marshal(pair)
		if len(d.CurrentPairs) < 8 && bytesUsed+len(wire) <= 4096 {
			d.CurrentPairs = append(d.CurrentPairs, pair)
			bytesUsed += len(wire)
		}
	}
	d.PairsOmitted = d.PairCount - len(d.CurrentPairs)
	return d
}

func withRuntimeMeasurementBindingRepair(repair *types.ToolRepair, doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) *types.ToolRepair {
	if repair == nil || doc == nil {
		return repair
	}
	attachRuntimeMeasurementDiagnostics(repair, runtimeMeasurementPairDiagnostics(doc, view))
	return repair
}

func attachRuntimeMeasurementDiagnostics(repair *types.ToolRepair, diagnostics runtimeMeasurementDiagnosticSet) {
	if diagnostics.Total == 0 {
		return
	}
	if repair.Metadata == nil {
		repair.Metadata = make(map[string]string)
	}
	repair.Metadata["runtime_measurement_diagnostic_count"] = strconv.Itoa(diagnostics.Total)
	repair.Metadata["runtime_measurement_diagnostics_omitted"] = strconv.Itoa(diagnostics.Total - len(diagnostics.Entries))
	wire, _ := json.Marshal(diagnostics.Entries)
	repair.Metadata[runtimeMeasurementDiagnosticsKey] = string(wire)
}

// This annotates an existing strict-decoder rejection, not a new validator.
// A sibling non-runtime error retains its own diagnosis even when the payload
// also contains a native selector. No malformed wrapper is ever unboxed.
func withRuntimeEventSelectionDecodeRepair(failed types.ToolResult, decodeErr error, raw []byte, operations ...string) types.ToolResult {
	var typed *json.UnmarshalTypeError
	if !errors.As(decodeErr, &typed) || typed.Struct != "RuntimeEventSelection" {
		return failed
	}
	var root map[string]any
	if json.Unmarshal(raw, &root) != nil {
		return failed
	}
	var diagnostics runtimeMeasurementDiagnosticSet
	for _, operation := range operations {
		blocks, _ := root[operation].([]any)
		for i, rawBlock := range blocks {
			block, _ := rawBlock.(map[string]any)
			measurement, _ := block["runtime_measurement"].(map[string]any)
			selection, _ := measurement["event_selection"].(map[string]any)
			for _, field := range []string{"row_ids", "field_keys", "group_by"} {
				value, exists := selection[field]
				if !exists || value == nil { // JSON null is accepted by []string decoding.
					continue
				}
				path := fmt.Sprintf("%s[%d].runtime_measurement.event_selection.%s", operation, i, field)
				items, array := value.([]any)
				if !array {
					diagnostics.add(runtimeMeasurementRepairDiagnostic{Field: path, ExpectedShape: "array<string>", ActualKind: runtimeRepairJSONKind(value), Problem: "invalid_json_shape"})
					continue
				}
				for j, item := range items {
					if _, text := item.(string); text || item == nil {
						continue
					}
					diagnostics.add(runtimeMeasurementRepairDiagnostic{Field: fmt.Sprintf("%s[%d]", path, j), ExpectedShape: "string", ActualKind: runtimeRepairJSONKind(item), Problem: "invalid_json_shape"})
				}
			}
		}
	}
	if diagnostics.Total == 0 {
		return failed
	}
	if failed.Repair == nil {
		failed.Repair = &types.ToolRepair{}
	}
	failed.Repair.Code = "answer_doc_native_selection_shape"
	failed.Repair.Hint = "event_selection.row_ids, field_keys and group_by require array<string>, not objects with an item/items property. Array shape example: []. Preserve the intended published identifiers and unrelated operations; change only their JSON representation. No answer values are supplied by this diagnostic."
	failed.Repair.Fields = nil
	for _, d := range diagnostics.Entries {
		failed.Repair.Fields = append(failed.Repair.Fields, d.Field)
	}
	attachRuntimeMeasurementDiagnostics(failed.Repair, diagnostics)
	failed.Repair = attachToolJSONSurfaceMetadata(failed.ToolName, failed.Repair)
	failed.Summary += "\n" + diagnostics.summary() + "\n" + failed.Repair.Hint
	return failed
}

func runtimeRepairJSONKind(value any) string {
	switch value.(type) {
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64:
		return "number"
	default:
		return "null"
	}
}

func runtimeMeasurementPairRepairAction(d *runtimeMeasurementRepairDiagnostic) string {
	action := "Use an exact current observation_id/view pair from the current schema; keep the intended event_selection and unrelated blocks. A patch uses a complete replace_blocks entry, not a partial receipt. No identifier is repaired automatically."
	if d.PairCount == 0 {
		action = "No current native measurement pairs are published; omit the optional runtime_measurement selector without inventing replacement facts. Preserve unrelated blocks."
	}
	return action
}
