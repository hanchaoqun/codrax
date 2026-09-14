package toolparam

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

var b1681Schema = json.RawMessage(`{"type":"object","properties":{"edit":` + string(b1677EnvelopeSchema) + `,"edits":{"type":"array","items":` + string(b1677EnvelopeSchema) + `},"limit":{"type":"integer"}}}`)

// Public Normalize: both alternatives are valid on their own. A nested
// transport conflict must not be erased by envelope or unrelated scalar repair.
func TestB1681NormalizePreservesNestedCompetingArguments(t *testing.T) {
	for _, lane := range []string{"direct", "function-body", "function-arguments"} {
		for _, array := range []bool{false, true} {
			wrap := func(value string) json.RawMessage {
				if array {
					return json.RawMessage(`{"edits":[` + value + `],"limit":"1"}`)
				}
				return json.RawMessage(`{"edit":` + value + `,"limit":"1"}`)
			}
			a, b := `{"path":"a.go","kind":"modify","limit":"1"}`, `{"path":"b.go","kind":"delete","limit":"2"}`
			for _, alternative := range []string{a, b} {
				one := wrap(`{"arguments":` + alternative + `}`)
				got, report := Normalize(one, b1681Schema, repairPolicy)
				if !report.Changed() || Validate(got, b1681Schema) != nil {
					t.Fatalf("positive premise: %s -> %s (%+v)", one, got, report)
				}
			}
			for _, mode := range []string{types.ToolParamCompatOff, types.ToolParamCompatAudit, types.ToolParamCompatRepair} {
				t.Run(fmt.Sprintf("%s/array=%t/%s", lane, array, mode), func(t *testing.T) {
					raw := wrap(b1677CompetingEnvelope("arguments", lane, a, b))
					before := string(raw)
					got, report := Normalize(raw, b1681Schema, types.ToolParamCompatConfig{Mode: mode})
					if report.NestedEnvelopeIntegrity == nil || !report.NestedEnvelopeIntegrity.HasAmbiguity() {
						t.Errorf("missing out-of-band nested ambiguity in %s: %+v", mode, report)
					}
					if string(got) != before || string(raw) != before || report.Changed() {
						t.Errorf("nested alternatives acquired one repair owner: input=%s output=%s repairs=%+v", before, got, report.Repairs)
					}
				})
			}
		}
	}
}

func TestB1681SchemaConsumedRoutesKeepRawCandidateIdentity(t *testing.T) {
	type route struct {
		name string
		wrap func(string) string
		path []string
	}
	for _, route := range []route{
		{"object", func(v string) string { return `{"edit":` + v + `}` }, []string{"edit"}},
		{"array", func(v string) string { return `{"edits":[` + v + `]}` }, []string{"edits", "[0]"}},
		{"singleton", func(v string) string { return `{"edits":` + v + `}` }, []string{"edits", "[0]"}},
		{"object-string", func(v string) string { return `{"edit":` + b1677Encoded(v, 2) + `}` }, []string{"edit"}},
		{"array-string", func(v string) string { return `{"edits":` + b1677Encoded("["+v+"]", 2) + `}` }, []string{"edits", "[0]"}},
		{"alias", func(v string) string { return `{" EDIT ":` + v + `}` }, []string{"edit"}},
		{"root-string", func(v string) string { return b1677Encoded(`{"edit":`+v+`}`, 2) }, []string{"edit"}},
		{"root-wrapper", func(v string) string { return `{"arguments":{"edit":` + v + `}}` }, []string{"edit"}},
		{"duplicate-array", func(v string) string { return `{"edits":[` + v + `],"edits":[{"path":"fine.go"}]}` }, []string{"edits", "[0]"}},
		{"trailing-comma", func(v string) string { return `{"edit":` + v + `,}` }, []string{"edit"}},
	} {
		for _, key := range envelopeCarrierKeyOrder {
			for _, lane := range []string{"direct", "function-body", "function-arguments"} {
				for _, depth := range []int{0, 2, 4} {
					for _, reverse := range []bool{false, true} {
						t.Run(fmt.Sprintf("%s/%s/%s/depth=%d/reverse=%t", route.name, key, lane, depth, reverse), func(t *testing.T) {
							a := `{"path":"a.go","limit":9007199254740992,"opaque":{"id":"one","id":"two"}}`
							b := `{"path":"a.go","limit":9007199254740993,"opaque":{"id":"one","id":"two"}}`
							if reverse {
								a, b = b, a
							}
							first, second := b1677Encoded(a, depth), b1677Encoded(b, depth)
							raw := json.RawMessage(route.wrap(b1677CompetingEnvelope(key, lane, first, second)))
							before := string(raw)
							got := InspectSchemaConsumedArgumentEnvelopes(raw, b1681Schema)
							if got.Truncated || len(got.Ambiguities) != 1 {
								t.Fatalf("expected one schema-scoped conflict, got %+v", got)
							}
							item := got.Ambiguities[0]
							if !reflect.DeepEqual(item.Path, route.path) || item.Envelope.Status != RawToolArgumentEnvelopeAmbiguous || len(item.Envelope.Candidates) != 2 {
								t.Fatalf("wrong ownership: %+v", item)
							}
							for index, object := range []string{a, b} {
								candidate := item.Envelope.Candidates[index]
								if string(candidate.Object) != object || string(candidate.Raw) != []string{first, second}[index] {
									t.Errorf("candidate %d lost wire identities: %+v", index, candidate)
								}
							}
							out, report := Normalize(raw, b1681Schema, repairPolicy)
							if string(out) != before || string(raw) != before || report.Changed() || report.NestedEnvelopeIntegrity == nil {
								t.Errorf("normalizer laundered nested conflict: %s -> %s; %+v", before, out, report)
							}
							// A nested value is never returned as a complete tool body.
							if root := InspectRawToolArgumentEnvelope(raw, b1681Schema); root.Status == RawToolArgumentEnvelopeAmbiguous {
								t.Errorf("nested conflict polluted root API: %+v", root)
							}
							if body, _, ok := RawToolArgumentEnvelope(raw, b1681Schema); ok && !strings.Contains(string(body), `"edit"`) {
								t.Errorf("root facade returned a nested candidate: %s", body)
							}
						})
					}
				}
			}
		}
	}
}

func TestB1681UnconsumedFieldsRemainOutsideIntegrityScope(t *testing.T) {
	conflict := `{"arguments":{"path":"a.go"},"arguments":{"path":"b.go"}}`
	for _, test := range []struct {
		name, raw string
		schema    json.RawMessage
	}{
		{"unknown-property", `{"unknown":` + conflict + `,"limit":"1"}`, b1681Schema},
		{"opaque-map", `{"edit":` + conflict + `,"limit":"1"}`, json.RawMessage(`{"type":"object","properties":{"edit":{"type":"object"},"limit":{"type":"integer"}}}`)},
		{"schema-owned-name", `{"edit":` + conflict + `,"limit":"1"}`, json.RawMessage(`{"type":"object","properties":{"edit":{"type":"object","properties":{"arguments":{"type":"object"}}},"limit":{"type":"integer"}}}`)},
		{"literal-string", `{"edit":` + b1677Encoded(conflict, 1) + `,"limit":"1"}`, json.RawMessage(`{"type":"object","properties":{"edit":{"type":["object","string"],"properties":{"path":{"type":"string"}}},"limit":{"type":"integer"}}}`)},
		{"array-items-untyped", `{"edits":[` + conflict + `],"limit":"1"}`, json.RawMessage(`{"type":"object","properties":{"edits":{"type":"array"},"limit":{"type":"integer"}}}`)},
		{"array-items-opaque", `{"edits":[` + conflict + `],"limit":"1"}`, json.RawMessage(`{"type":"object","properties":{"edits":{"type":"array","items":{"type":"object"}},"limit":{"type":"integer"}}}`)},
		{"unused-function", `{"edit":{"arguments":{"path":"good.go"},"function":{"arguments":{"path":"bad.go"}},"function":null}}`, b1681Schema},
		{"two-carriers", `{"edit":{"arguments":{"path":"a.go"},"params":{"path":"b.go"}}}`, b1681Schema},
		{"schema-property-disables-envelope", `{"edit":{"path":"actual.go","arguments":{"path":"a.go"},"arguments":{"path":"b.go"}}}`, b1681Schema},
		{"unknown-envelope-key", `{"edit":{"extra":true,"arguments":{"path":"a.go"},"arguments":{"path":"b.go"}}}`, b1681Schema},
		{"unadmitted-carrier", `{"edit":{"arguments":null,"arguments":{}}}`, b1681Schema},
		{"ambiguous-alias-not-consumed", `{" EDIT ":` + conflict + `,"eDiT":{"path":"other.go"}}`, b1681Schema},
		{"exact-key-shadows-alias", `{" EDIT ":` + conflict + `,"edit":{"path":"other.go"}}`, b1681Schema},
		{"deeper-than-decoder", `{"edit":` + b1677Encoded(conflict, 5) + `}`, b1681Schema},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw := json.RawMessage(test.raw)
			if got := InspectSchemaConsumedArgumentEnvelopes(raw, test.schema); got.HasAmbiguity() {
				t.Errorf("unconsumed content acquired authority: %+v", got)
			}
			if _, report := Normalize(raw, test.schema, repairPolicy); report.NestedEnvelopeIntegrity != nil {
				t.Errorf("normalizer rejected unconsumed content: %+v", report)
			}
		})
	}
}

func TestB1681EquivalentAndDirectFirstNestedWrappersRemainRepairable(t *testing.T) {
	for _, test := range []struct{ name, value string }{
		{"equivalent", `{"arguments":{"path":"same.go","limit":"1"},"arguments":{ "path":"same.go", "limit":"1" }}`},
		{"escaped-key", `{"arguments":{"path":"same.go","limit":"1"},"argu\u006dents":{"path":"same.go","limit":"1"}}`},
		{"metadata", `{"id":"one","id":"two","arguments":{"path":"same.go","limit":"1"}}`},
		{"unused-function", `{"arguments":{"path":"same.go","limit":"1"},"function":null,"function":{"arguments":{"path":"ignored.go"}}}`},
		{"function-fallback", `{"args":null,"params":null,"function":{"arguments":{"path":"same.go","limit":"1"}}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw := json.RawMessage(`{"edits":[` + test.value + `]}`)
			got, report := Normalize(raw, b1681Schema, repairPolicy)
			if report.NestedEnvelopeIntegrity != nil || !report.Changed() || Validate(got, b1681Schema) != nil || !strings.Contains(string(got), `"same.go"`) || strings.Contains(string(got), "ignored.go") {
				t.Errorf("valid repair changed: %s -> %s (%+v)", raw, got, report)
			}
		})
	}
}

func TestB1681InvalidAlternativesAndBoundedConflictSet(t *testing.T) {
	for _, bad := range []string{`null`, `[]`, `1`, `"unreadable"`, `{}`, `{"unrelated":true}`} {
		for _, reverse := range []bool{false, true} {
			good := `{"path":"known.go"}`
			first, second := good, bad
			if reverse {
				first, second = second, first
			}
			raw := json.RawMessage(`{"edit":` + b1677CompetingEnvelope("arguments", "direct", first, second) + `}`)
			got := InspectSchemaConsumedArgumentEnvelopes(raw, b1681Schema)
			if len(got.Ambiguities) != 1 || len(got.Ambiguities[0].Envelope.Candidates) != 2 {
				t.Fatalf("invalid alternative disappeared: %s -> %+v", raw, got)
			}
			index := 1
			if reverse {
				index = 0
			}
			if candidate := got.Ambiguities[0].Envelope.Candidates[index]; candidate.Object != nil || string(candidate.Raw) != bad {
				t.Errorf("invalid original became an admitted object: %+v", candidate)
			}
		}
	}
	conflict := `{"arguments":{"path":"a.go"},"arguments":{"path":"b.go"}}`
	for _, count := range []int{63, 64, 65, 130} {
		values := make([]string, count)
		for index := range values {
			values[index] = conflict
		}
		raw := json.RawMessage(`{"edits":[` + strings.Join(values, ",") + `]}`)
		got := InspectSchemaConsumedArgumentEnvelopes(raw, b1681Schema)
		want := count
		if want > 64 {
			want = 64
		}
		if !got.HasAmbiguity() || got.Truncated != (count > 64) || len(got.Ambiguities) != want {
			t.Errorf("wrong bounded comparison accounting for %d: %+v", count, got)
		}
	}
	if (SchemaConsumedArgumentEnvelopeInspection{}).HasAmbiguity() || !(SchemaConsumedArgumentEnvelopeInspection{Truncated: true}).HasAmbiguity() {
		t.Fatal("truncated ownership must never prove agreement")
	}
}

func TestB1681ArrayCoalescingDoesNotManufactureWrapperAgreement(t *testing.T) {
	raw := json.RawMessage(`{"edits":[{"arguments":{"path":"<tag>"},"arguments":{"path":"\u003ctag\u003e"}}],"edits":[]}`)
	got := InspectSchemaConsumedArgumentEnvelopes(raw, b1681Schema)
	if len(got.Ambiguities) != 1 {
		t.Fatalf("array prepass erased exact original carrier spellings: %+v", got)
	}
	for index, want := range []string{`{"path":"<tag>"}`, `{"path":"\u003ctag\u003e"}`} {
		if actual := string(got.Ambiguities[0].Envelope.Candidates[index].Object); actual != want {
			t.Errorf("array prepass changed candidate %d: want %s got %s", index, want, actual)
		}
	}
	if out, report := Normalize(raw, b1681Schema, repairPolicy); string(out) != string(raw) || report.Changed() {
		t.Errorf("normalization granted manufactured agreement: %s (%+v)", out, report)
	}
}

func TestB1681SchemaArrayFragmentRepairKeepsNestedCarrierConflicts(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"groups":{"type":"array","items":` + string(b1681Schema) + `}}}`)
	for _, value := range []string{
		`{"arguments":{"path":"a.go"}}`,
		`{"arguments":{"path":"a.go"},"arguments":{"path":"b.go"}}`,
	} {
		fragment := `[{"edit":{"path":"good.go"}}, "edit":` + value + `}]`
		raw := json.RawMessage(`{"groups":` + b1677Encoded(fragment, 1) + `}`)
		got := InspectSchemaConsumedArgumentEnvelopes(raw, schema)
		out, report := Normalize(raw, schema, repairPolicy)
		if strings.Contains(value, "b.go") {
			if len(got.Ambiguities) != 1 || !reflect.DeepEqual(got.Ambiguities[0].Path, []string{"groups", "[1]", "edit"}) || string(out) != string(raw) || report.Changed() {
				t.Errorf("fragment repair lost nested conflict: %+v; %s (%+v)", got, out, report)
			}
		} else if got.HasAmbiguity() || !report.Changed() || Validate(out, schema) != nil {
			t.Errorf("fragment positive premise did not reach production repair: %+v; %s (%+v)", got, out, report)
		}
	}
}

func TestB1681ArrayCoalescingPreservesItemsBeforeNormalization(t *testing.T) {
	first := `{"path":"<html> & \"quoted\"","limit":9007199254740993}`
	second := `{"path":"\u003chtml\u003e","limit":9007199254740992}`
	raw := json.RawMessage(`{"edits":[` + first + `],"edits":[` + second + `]}`)
	merged, repairs, ok := coalesceDuplicateSchemaArrayProperties(raw, b1681Schema)
	want := `{"edits":[` + first + `,` + second + `]}`
	if !ok || len(repairs) != 1 || string(merged) != want {
		t.Fatalf("array splice changed original item bytes or order: %s (%+v)", merged, repairs)
	}
	got, report := Normalize(raw, b1681Schema, repairPolicy)
	if !report.Changed() || report.NestedEnvelopeIntegrity != nil || Validate(got, b1681Schema) != nil {
		t.Fatalf("ordinary native array merging stopped working: %s (%+v)", got, report)
	}
	value, ok := decodeJSONValue(got)
	if !ok {
		t.Fatal("merged result is invalid")
	}
	items := value.(map[string]any)["edits"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["limit"] != json.Number("9007199254740993") || items[1].(map[string]any)["limit"] != json.Number("9007199254740992") {
		t.Errorf("native array merge changed integer identities or order: %+v", items)
	}
	for _, empty := range []string{`{"edits":[],"edits":[]}`, `{"edits":null,"edits":[]}`} {
		got, _, ok := coalesceDuplicateSchemaArrayProperties(json.RawMessage(empty), b1681Schema)
		if !ok || string(got) != `{"edits":null}` {
			t.Errorf("existing nil-array merge policy changed: %s -> %s", empty, got)
		}
	}
}

func TestB1681NestedCandidateLimitAndRootAmbiguityStayDistinct(t *testing.T) {
	for _, count := range []int{64, 65} {
		properties := make([]string, count)
		for index := range properties {
			properties[index] = `"arguments":{"path":"same.go"}`
		}
		raw := json.RawMessage(`{"edit":{` + strings.Join(properties, ",") + `}}`)
		got := InspectSchemaConsumedArgumentEnvelopes(raw, b1681Schema)
		if got.HasAmbiguity() != (count > 64) {
			t.Fatalf("wrong per-envelope bound for %d: %+v", count, got)
		}
		if count > 64 && (got.Truncated || !got.Ambiguities[0].Envelope.Truncated || len(got.Ambiguities[0].Envelope.Candidates) != 64) {
			t.Errorf("local candidate truncation conflated with global conflict truncation: %+v", got)
		}
	}
	raw := json.RawMessage(`{"arguments":{"edit":{"path":"a.go"}},"arguments":{"edit":{"path":"b.go"}}}`)
	if got := InspectSchemaConsumedArgumentEnvelopes(raw, b1681Schema); got.HasAmbiguity() {
		t.Errorf("root alternatives were assigned an invented child scope: %+v", got)
	}
	if got, report := Normalize(raw, b1681Schema, repairPolicy); string(got) != string(raw) || report.Changed() || report.EnvelopeIntegrity == nil || report.EnvelopeIntegrity.Status != RawToolArgumentEnvelopeAmbiguous || report.NestedEnvelopeIntegrity != nil {
		t.Errorf("root conflict handling changed: %s (%+v)", got, report)
	}
}
