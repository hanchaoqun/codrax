package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestB1675OriginalSelectorIntegrityPublicLifecycle(t *testing.T) {
	ambiguous := `{"schema_version":3,"schema_version":2,"root_causes":[{"candidate_id":"unknown","candidate_id":"candidate-sched"}]}`
	carriers := []string{ambiguous, "[" + ambiguous + "]"}
	encoded := ambiguous
	for depth := 1; depth <= 4; depth++ {
		raw, _ := json.Marshal(encoded)
		encoded = string(raw)
		carriers = append(carriers, encoded)
	}
	for _, patch := range []bool{false, true} {
		for _, raw := range carriers {
			t.Run(fmt.Sprintf("patch=%t/%s", patch, raw), func(t *testing.T) {
				ctx := b1654Context()
				if result := b1654Execute(t, ctx, patch, b1654Report, ""); !result.Success {
					t.Fatal(result.Summary)
				}
				previous := ctx.Mutable.TraceRootCauseReport()
				result := b1654Execute(t, ctx, patch, raw, "")
				if !result.Success || len(result.OptionalCarrierOutcomes) != 1 || !strings.Contains(result.OptionalCarrierOutcomes[0].Reason, "duplicate or competing") || !ctx.Mutable.TraceRootCauseSelectorRejected() || ctx.Mutable.PendingTraceRootCauseReport() != nil {
					t.Fatalf("ambiguous selector was not independently rejected: %+v", result)
				}
				if patch && !reflect.DeepEqual(ctx.Mutable.TraceRootCauseReport(), previous) {
					t.Fatal("bad replacement lost the old accepted report")
				}
				if !patch && ctx.Mutable.TraceRootCauseReport() != nil {
					t.Fatal("bad full re-emit changed its existing invalid-selector semantics")
				}
				if ctx.Mutable.AnswerDocumentV2().Blocks[0].Text != "The model owns this complete answer." {
					t.Fatal("selector integrity rejected or rewrote the answer")
				}
				ctx = b1654Context()
				result = b1654Execute(t, ctx, patch, raw, `"relation_claims":[]`)
				if result.Success || len(result.OptionalCarrierOutcomes) != 1 || ctx.Mutable.PendingTraceRootCauseReport() != nil || !ctx.Mutable.TraceRootCauseSelectorRejected() {
					t.Fatalf("unrelated pre-decode reject staged an ambiguous selector or lost disclosure: %+v", result)
				}
			})
		}
		ctx := b1654Context()
		_ = b1654Execute(t, ctx, patch, b1654Report, `"relation_claims":[]`)
		pending := ctx.Mutable.PendingTraceRootCauseReport()
		result := b1654Execute(t, ctx, patch, "null", `"schema_version":3`)
		if pending == nil || !result.Success || len(result.OptionalCarrierOutcomes) != 0 || !reflect.DeepEqual(ctx.Mutable.TraceRootCauseReport(), pending) {
			t.Fatalf("patch=%t: null omission failed to inherit the prior pending selection: %+v", patch, result)
		}
		result = b1654Execute(t, ctx, patch, `{"schema_version":2,"root_causes":[]}`, "")
		if !result.Success || len(result.OptionalCarrierOutcomes) != 0 || ctx.Mutable.TraceRootCauseReport() == nil || len(ctx.Mutable.TraceRootCauseReport().RootCauses) != 0 {
			t.Fatalf("patch=%t: explicit empty object was no longer an ordinary withdrawal: %+v", patch, result)
		}
	}
}

func TestB1675OpaqueSelectorFieldsPreserveOriginalWire(t *testing.T) {
	schema := (&EmitAnswerDocument{}).Parameters()
	raw := json.RawMessage(`{"blocks":"[]","schema_version" : 3 , "schema_\u0076ersion":2,"trace_root_causes" : [{"candidate_id":"original","description":"  业务\\dir \"quoted\"  "}],"TRACE_ROOT_CAUSES":{"schema_version":2,"root_causes":[]}}`)
	before := string(raw)
	prepared, restore := PrepareTraceRootCauseParamsForCompatibility("emit_answer_document", raw, schema)
	if restore == nil || string(raw) != before {
		t.Fatal("ambiguous selector was not isolated or its input was mutated")
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(prepared, &body); err != nil || len(body) != 1 || string(body["blocks"]) != `"[]"` {
		t.Fatalf("temporary compatibility input dropped or invented body fields: %s", prepared)
	}
	restored := restore(json.RawMessage(`{"blocks":[],"ordinary_unknown":"retained for its owner"}`))
	originalProps, _ := traceRootCauseRawProperties(raw)
	restoredProps, ok := traceRootCauseRawProperties(restored)
	if !ok {
		t.Fatalf("restored object is not complete JSON: %s", restored)
	}
	var wanted, got []string
	for _, property := range originalProps {
		if traceRootCauseReservedParam(property.key) {
			wanted = append(wanted, string(property.wire))
		}
	}
	for _, property := range restoredProps {
		if traceRootCauseReservedParam(property.key) {
			got = append(got, string(property.wire))
		}
	}
	if !reflect.DeepEqual(got, wanted) || !strings.Contains(string(restored), `"blocks":[]`) || !strings.Contains(string(restored), `"ordinary_unknown":"retained for its owner"`) {
		t.Fatalf("original selector occurrences or independent repaired body were lost: got=%q want=%q restored=%s", got, wanted, restored)
	}
	for _, unchanged := range []string{
		`{"blocks":[]}`, `{"blocks":[{"text":"a","text":"b"}],"trace_root_causes":` + b1654Report + `}`,
		`{"trace_root_causes":` + b1654Report + `,"ordinary_unknown":1,"ordinary_unknown":2}`,
		`{"trace_root_causes":null,"replace_trace_root_causes":null}`,
	} {
		if got, restore := PrepareTraceRootCauseParamsForCompatibility("emit_answer_document", json.RawMessage(unchanged), schema); restore != nil || string(got) != unchanged {
			t.Fatalf("unrelated body/unknown field affected optional selector integrity: %s", unchanged)
		}
	}
}
