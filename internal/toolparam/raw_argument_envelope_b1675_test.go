package toolparam

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1675RawArgumentEnvelopeMatchesExistingAdmission(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"blocks":{"type":"array","items":{"type":"object"}}}}`)
	inner := `{"blocks":[],"trace_root_causes":{"schema_version":3,"schema_version":2,"root_causes":[{"candidate_id":"wrong","candidate_id":"chosen"}]}}`
	quoted, _ := json.Marshal(inner)
	for _, key := range envelopeCarrierKeyOrder {
		for _, encoded := range []bool{false, true} {
			for _, function := range []bool{false, true} {
				value := inner
				if encoded {
					value = string(quoted)
				}
				raw := json.RawMessage(`{"id":"original-call","` + key + `":` + value + `}`)
				wantPath := []string{key}
				if function {
					raw = json.RawMessage(`{"type":"function","function":` + string(raw) + `}`)
					wantPath = append([]string{"function"}, wantPath...)
				}
				before := string(raw)
				got, path, ok := RawToolArgumentEnvelope(raw, schema)
				if !ok || string(got) != inner || !reflect.DeepEqual(path, wantPath) || string(raw) != before {
					t.Fatalf("key=%s encoded=%t function=%t: raw source changed or wrong path: ok=%t path=%v raw=%s", key, encoded, function, ok, path, got)
				}
				if _, report := Normalize(raw, schema, types.DefaultToolParamCompatConfig()); !report.Changed() {
					t.Fatalf("raw locator invented an envelope unsupported by Normalize: %s", raw)
				}
			}
		}
	}
	if got, path, ok := RawToolArgumentEnvelope(quoted, schema); !ok || string(got) != inner || len(path) != 0 {
		t.Fatalf("one whole-object string source was lost: ok=%t path=%v raw=%s", ok, path, got)
	}
	// Normalize gives the direct carrier precedence over function metadata;
	// only duplicate selected path fields are ambiguous, not repeated metadata.
	for _, raw := range []string{
		`{"arguments":` + inner + `,"function":{"arguments":{"blocks":[]}}}`,
		`{"arguments":` + inner + `,"function":{"name":"emit_answer_document"}}`,
		`{"id":"first","id":"second","arguments":` + inner + `}`,
		`{"function":{"name":"first","name":"second","arguments":` + inner + `}}`,
	} {
		got, _, ok := RawToolArgumentEnvelope(json.RawMessage(raw), schema)
		if !ok || string(got) != inner {
			t.Fatalf("unique admitted carrier lost its original raw fields: ok=%t raw=%s input=%s", ok, got, raw)
		}
		if _, report := Normalize(json.RawMessage(raw), schema, types.DefaultToolParamCompatConfig()); !report.Changed() {
			t.Fatalf("metadata control must remain admitted by Normalize: %s", raw)
		}
	}
	for depth := 1; depth <= 4; depth++ {
		encoded := json.RawMessage(inner)
		for index := 0; index < depth; index++ {
			encoded, _ = json.Marshal(string(encoded))
		}
		for _, wrapped := range []bool{false, true} {
			raw := encoded
			wantPath := []string(nil)
			if wrapped {
				raw = json.RawMessage(`{"arguments":` + string(encoded) + `}`)
				wantPath = []string{"arguments"}
			}
			got, path, ok := RawToolArgumentEnvelope(raw, schema)
			if !ok || string(got) != inner || !reflect.DeepEqual(path, wantPath) {
				t.Fatalf("bounded source depth=%d wrapped=%t: ok=%t path=%v raw=%s", depth, wrapped, ok, path, got)
			}
		}
	}
	wholeEnvelope, _ := json.Marshal(`{"arguments":` + string(quoted) + `}`)
	if got, path, ok := RawToolArgumentEnvelope(wholeEnvelope, schema); !ok || string(got) != inner || !reflect.DeepEqual(path, []string{"arguments"}) {
		t.Fatalf("whole-string plus admitted argument envelope lost source: ok=%t path=%v raw=%s", ok, path, got)
	}
	for _, raw := range []string{
		inner,
		`{"blocks":[],"arguments":` + inner + `}`,
		`{"arguments":` + inner + `,"params":` + inner + `}`,
		`{"arguments":` + inner + `,"arguments":` + inner + `}`,
		`{"arguments":` + inner + `,"unknown":true}`,
		`{"arguments":{"arguments":` + inner + `}}`,
		`{"arbitrary":{"arguments":` + inner + `}}`,
		`{"function":{"arguments":` + inner + `,"params":` + inner + `}}`,
		`{"arguments":"{broken"}`,
	} {
		if _, _, ok := RawToolArgumentEnvelope(json.RawMessage(raw), schema); ok {
			t.Fatalf("ambiguous/unsupported envelope acquired a raw source: %s", raw)
		}
	}
}

// Raw selection shares the original decoder's repair order and finite depth,
// including when decoding into a map would otherwise erase duplicate keys.
func TestB1675RawJSONStringDecoderPreservesExistingCandidate(t *testing.T) {
	for _, tc := range []struct{ raw, kind string }{
		{`{"value":1,"value":2}`, "object"},
		{`[{"value":1,"value":2}]`, "array"},
		{`{"value":1,"value":2,}`, "object"},
		{`[{"value":1,"value":2},]`, "array"},
		{`{"value":1,"value":2`, "object"},
		{"{\"value\":1,\"value\":\"line\nend\"}", "object"},
		{`{"value":"a "quoted" text","value":2}`, "object"},
	} {
		for depth := 0; depth <= 4; depth++ {
			t.Run(fmt.Sprintf("%s/depth=%d", tc.raw, depth), func(t *testing.T) {
				encoded := tc.raw
				for index := 0; index < depth; index++ {
					value, _ := json.Marshal(encoded)
					encoded = string(value)
				}
				before := encoded
				raw, rule, ok := DecodeJSONStringAsRaw(encoded, tc.kind)
				value, oldRule, oldOK := decodeJSONStringAs(encoded, tc.kind)
				if ok != oldOK || rule != oldRule || encoded != before {
					t.Fatalf("raw and value decoder policies diverged: raw=(%s,%q,%t) value=(%v,%q,%t)", raw, rule, ok, value, oldRule, oldOK)
				}
				if depth == 4 {
					if ok {
						t.Fatal("raw preservation expanded the existing depth bound")
					}
					return
				}
				if !ok || !json.Valid(raw) || strings.Count(string(raw), `"value"`) != 2 {
					t.Fatalf("admitted raw candidate lost its duplicate fields: %q (%s)", raw, rule)
				}
				decoded, decodedOK := decodeJSONValue(raw)
				if !decodedOK || !reflect.DeepEqual(decoded, value) {
					t.Fatal("raw preservation changed the ordinary decoded value")
				}
			})
		}
	}
}
