package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC241SelectorTransportRestoresOnlyInsideProvenArguments(t *testing.T) {
	for _, patch := range []bool{false, true} {
		for _, function := range []bool{false, true} {
			for _, encodedBody := range []bool{false, true} {
				t.Run(fmt.Sprintf("patch=%t/function=%t/string-body=%t", patch, function, encodedBody), func(t *testing.T) {
					name, field, bodyKey := "emit_answer_document", "trace_root_causes", "blocks"
					schema := (&EmitAnswerDocument{}).Parameters()
					body := json.RawMessage(`[{"id":"summary","kind":"summary","text":"Original model answer."}]`)
					if patch {
						name, field, bodyKey = "emit_answer_document_patch", "replace_trace_root_causes", "unchanged_block_ids"
						schema = (&EmitAnswerDocumentPatch{}).Parameters()
						body = json.RawMessage(`["summary"]`)
					}
					if encodedBody {
						body, _ = json.Marshal(string(body))
					}
					selection := `"` + field + `" : {"schema_version":3,"schema_version":2,"root_causes":[{"candidate_id":"unknown","candidate_id":"candidate-sched","description":"  原始\\path  "}]}`
					inner := json.RawMessage(`{"` + bodyKey + `":` + string(body) + `,` + selection + `}`)
					wrapped := json.RawMessage(`{"arguments":` + string(inner) + `}`)
					if function {
						encoded, _ := json.Marshal(string(inner))
						wrapped = json.RawMessage(`{"function":{"arguments":` + string(encoded) + `}}`)
					}
					raw, _ := json.Marshal(string(wrapped))
					before := string(raw)
					prepared, restore := PrepareTraceRootCauseParamsForCompatibility(name, raw, schema)
					var properties map[string]json.RawMessage
					if restore == nil || json.Unmarshal(prepared, &properties) != nil || len(properties[bodyKey]) == 0 || len(properties) != 1 {
						t.Fatalf("compatibility must receive the proven inner body, not its transport: %s", prepared)
					}
					normalized, _ := toolparam.Normalize(prepared, schema, types.DefaultToolParamCompatConfig())
					restored := restore(normalized)
					originalFields, _ := traceRootCauseRawProperties(inner)
					restoredFields, ok := traceRootCauseRawProperties(restored)
					if !ok || string(raw) != before {
						t.Fatal("restoration lost the complete argument object or rewrote the input")
					}
					var want, got []string
					for _, property := range originalFields {
						if traceRootCauseReservedParam(property.key) {
							want = append(want, string(property.wire))
						}
					}
					for _, property := range restoredFields {
						if traceRootCauseReservedParam(property.key) {
							got = append(got, string(property.wire))
						}
					}
					if !reflect.DeepEqual(want, got) || json.Unmarshal(restored, &properties) != nil || len(properties[bodyKey]) == 0 || len(properties["arguments"]) > 0 || len(properties["function"]) > 0 {
						t.Fatalf("selector wire or repaired body changed: want=%q got=%q payload=%s", want, got, restored)
					}
					var rows []json.RawMessage
					if json.Unmarshal(properties[bodyKey], &rows) != nil || len(rows) != 1 {
						t.Fatal("one normalization did not repair the independent body")
					}
				})
			}
		}
	}
}
