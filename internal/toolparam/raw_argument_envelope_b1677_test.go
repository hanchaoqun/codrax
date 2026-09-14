package toolparam

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

var b1677EnvelopeSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"path":{"type":"string"},"kind":{"type":"string","enum":["modify","delete"]},"limit":{"type":"integer"}}}`)

func b1677Encoded(raw string, depth int) string {
	for i := 0; i < depth; i++ {
		encoded, _ := json.Marshal(raw)
		raw = string(encoded)
	}
	return raw
}

func b1677CompetingEnvelope(key, lane, a, b string) string {
	switch lane {
	case "function-body":
		return `{"function":{"name":"tool","` + key + `":` + a + `},"function":{"name":"tool","` + key + `":` + b + `}}`
	case "function-arguments":
		return `{"function":{"name":"tool","` + key + `":` + a + `,"` + key + `":` + b + `}}`
	default:
		return `{"` + key + `":` + a + `,"` + key + `":` + b + `}`
	}
}

// Public Normalize, with each alternative independently admitted first. These
// are argument-ownership fixtures, not actual filesystem or live-model calls.
func TestB1677NormalizeDoesNotChooseBetweenConsumedWrappers(t *testing.T) {
	for _, key := range envelopeCarrierKeyOrder {
		for _, lane := range []string{"direct", "function-body", "function-arguments"} {
			for _, depth := range []int{0, 2} {
				for _, reverse := range []bool{false, true} {
					a := b1677Encoded(`{"path":"a.go","kind":"modify","limit":"1"}`, depth)
					b := b1677Encoded(`{"path":"b.go","kind":"delete","limit":"2"}`, depth)
					if reverse {
						a, b = b, a
					}
					for _, alternative := range []string{a, b} {
						one := json.RawMessage(`{"` + key + `":` + alternative + `}`)
						normalized, report := Normalize(one, b1677EnvelopeSchema, repairPolicy)
						if !report.Changed() || Validate(normalized, b1677EnvelopeSchema) != nil {
							t.Fatalf("positive premise lost a currently admitted envelope: %s -> %s", one, normalized)
						}
					}
					raw := json.RawMessage(b1677CompetingEnvelope(key, lane, a, b))
					for _, mode := range []string{types.ToolParamCompatOff, types.ToolParamCompatAudit, types.ToolParamCompatRepair} {
						t.Run(fmt.Sprintf("%s/%s/depth=%d/reverse=%t/%s", key, lane, depth, reverse, mode), func(t *testing.T) {
							before := string(raw)
							got, report := Normalize(raw, b1677EnvelopeSchema, types.ToolParamCompatConfig{Mode: mode})
							if report.EnvelopeIntegrity == nil || report.EnvelopeIntegrity.Status != RawToolArgumentEnvelopeAmbiguous || len(report.EnvelopeIntegrity.Candidates) != 2 {
								t.Errorf("mode %s lost exact argument-ownership diagnosis: %+v", mode, report.EnvelopeIntegrity)
							}
							if string(got) != before || string(raw) != before {
								t.Errorf("normalization chose one model argument owner: input=%s output=%s", before, got)
							}
							if report.Changed() {
								t.Errorf("ambiguous wrapper was granted a repair: %+v", report)
							}
							if _, _, ok := RawToolArgumentEnvelope(raw, b1677EnvelopeSchema); ok {
								t.Error("ambiguous wrapper acquired one raw argument owner")
							}
						})
					}
				}
			}
		}
	}
}

func TestB1677InspectionPreservesAllCandidateWireAndDecodedFacts(t *testing.T) {
	a := `{"path":"a.go","kind":"modify","limit":9007199254740992,"selection":{"id":"first","id":"second"}}`
	b := `{"path":"a.go","kind":"modify","limit":9007199254740993,"selection":{"id":"first","id":"second"}}`
	for _, reverse := range []bool{false, true} {
		first, second := a, b
		if reverse {
			first, second = second, first
		}
		for depth := 0; depth <= 4; depth++ {
			firstWire, secondWire := b1677Encoded(first, depth), b1677Encoded(second, depth)
			for _, function := range []bool{false, true} {
				raw := json.RawMessage(`{"arguments":` + firstWire + `,"argu\u006dents":` + secondWire + `}`)
				wantPath, conflictPath := []string{"arguments"}, "$.arguments"
				if function {
					raw = json.RawMessage(`{"function":` + string(raw) + `}`)
					wantPath, conflictPath = []string{"function", "arguments"}, "$.function.arguments"
				}
				before := string(raw)
				got := InspectRawToolArgumentEnvelope(raw, b1677EnvelopeSchema)
				if got.Status != RawToolArgumentEnvelopeAmbiguous || got.Truncated || got.ConflictPath != conflictPath || len(got.Candidates) != 2 || string(raw) != before {
					t.Fatalf("exact source ownership was lost: %+v", got)
				}
				for index, want := range []struct{ wire, object string }{{firstWire, first}, {secondWire, second}} {
					candidate := got.Candidates[index]
					if string(candidate.Raw) != want.wire || string(candidate.Object) != want.object || !reflect.DeepEqual(candidate.Path, wantPath) {
						t.Fatalf("candidate %d lost wire/inner duplicates/precision: %+v", index, candidate)
					}
				}
			}
		}
	}
}

func TestB1677InspectionHasTheExistingBoundedEnvelopeScope(t *testing.T) {
	inner := `{"path":"a.go","kind":"modify","limit":"1"}`
	for _, raw := range []string{
		`{"id":"one","id":"two","arguments":` + inner + `}`,
		`{"function":{"name":"one","name":"two","arguments":` + inner + `}}`,
		`{"arguments":` + inner + `,"function":null,"function":{"arguments":{"path":"foreign.go"}}}`,
		// Current Normalize uses function fallback when there is not exactly
		// one direct carrier key. Keep that existing precedence, not a new one.
		`{"arguments":{},"params":{},"function":{"arguments":` + inner + `}}`,
	} {
		got := InspectRawToolArgumentEnvelope(json.RawMessage(raw), b1677EnvelopeSchema)
		if got.Status != RawToolArgumentEnvelopeUnique || len(got.Candidates) != 1 || string(got.Candidates[0].Object) != inner {
			t.Fatalf("unconsumed metadata or prior branch precedence changed: %s -> %+v", raw, got)
		}
		if _, report := Normalize(json.RawMessage(raw), b1677EnvelopeSchema, repairPolicy); !report.Changed() {
			t.Fatalf("currently admitted positive envelope was not normalized: %s", raw)
		}
	}
	for _, raw := range []string{
		inner,
		`{"path":"explicit.go","arguments":` + inner + `,"arguments":{"path":"other.go"}}`,
		`{"arguments":` + inner + `,"params":` + inner + `}`,
		`{"arguments":` + inner + `,"unknown":true}`,
		`{"arguments":{"arguments":` + inner + `}}`,
		`{"arbitrary":{"function":{"arguments":` + inner + `}}}`,
		`{"function":` + b1677Encoded(`{"arguments":`+inner+`}`, 1) + `}`,
	} {
		if got := InspectRawToolArgumentEnvelope(json.RawMessage(raw), b1677EnvelopeSchema); got.Status != RawToolArgumentEnvelopeNone {
			t.Fatalf("unsupported or schema-owned payload became an envelope: %s -> %+v", raw, got)
		}
	}
	owned := json.RawMessage(`{"type":"object","properties":{"arguments":{"type":"object"},"limit":{"type":"integer"}}}`)
	raw := json.RawMessage(`{"arguments":{"path":"one"},"arguments":{"path":"two"},"limit":"1"}`)
	if got := InspectRawToolArgumentEnvelope(raw, owned); got.Status != RawToolArgumentEnvelopeNone {
		t.Fatalf("schema-owned arguments were treated as transport wrappers: %+v", got)
	}
	for depth := 1; depth <= 5; depth++ {
		original := b1677CompetingEnvelope("arguments", "direct", inner, `{"path":"b.go"}`)
		raw := json.RawMessage(b1677Encoded(original, depth))
		got := InspectRawToolArgumentEnvelope(raw, b1677EnvelopeSchema)
		want := RawToolArgumentEnvelopeAmbiguous
		if depth > 4 {
			want = RawToolArgumentEnvelopeNone
		}
		if got.Status != want {
			t.Fatalf("whole-object string depth=%d broadened/lost existing decoding boundary: %+v", depth, got)
		}
	}
}

func TestB1677IncompleteAlternativeAndCandidateLimitNeverProveAgreement(t *testing.T) {
	valid := `{"path":"a.go","kind":"modify"}`
	for _, invalid := range []string{`null`, `1`, `[]`, `"unparseable"`, `{"unrelated":"no-schema-hit"}`} {
		for _, reverse := range []bool{false, true} {
			a, b, invalidIndex := valid, invalid, 1
			if reverse {
				a, b, invalidIndex = b, a, 0
			}
			raw := json.RawMessage(b1677CompetingEnvelope("arguments", "direct", a, b))
			got := InspectRawToolArgumentEnvelope(raw, b1677EnvelopeSchema)
			if got.Status != RawToolArgumentEnvelopeAmbiguous || len(got.Candidates) != 2 || got.Candidates[invalidIndex].Object != nil || string(got.Candidates[invalidIndex].Raw) != invalid {
				t.Fatalf("invalid alternative was discarded or fabricated: %+v", got)
			}
		}
	}
	for _, count := range []int{63, 64, 65, 130} {
		raw := json.RawMessage(`{` + strings.TrimSuffix(strings.Repeat(`"arguments":`+valid+`,`, count), ",") + `}`)
		got := InspectRawToolArgumentEnvelope(raw, b1677EnvelopeSchema)
		want := RawToolArgumentEnvelopeEquivalent
		if count > rawToolArgumentEnvelopeCandidateLimit {
			want = RawToolArgumentEnvelopeAmbiguous
		}
		if got.Status != want || got.Truncated != (count > rawToolArgumentEnvelopeCandidateLimit) || len(got.Candidates) > rawToolArgumentEnvelopeCandidateLimit {
			t.Fatalf("candidate cap was treated as complete agreement: count=%d inspection=%+v", count, got)
		}
	}
	raw := json.RawMessage(`{` + strings.Repeat(`"id":"irrelevant",`, 130) + `"arguments":` + valid + `}`)
	if got := InspectRawToolArgumentEnvelope(raw, b1677EnvelopeSchema); got.Status != RawToolArgumentEnvelopeUnique || got.Truncated {
		t.Fatal("unrelated repeated metadata consumed argument-candidate capacity")
	}
}

func TestB1677EquivalentWrappersDoNotEraseInnerDuplicates(t *testing.T) {
	inner := `{"path":"a.go","opaque":{"id":"one","id":"two"}}`
	raw := json.RawMessage(`{"arguments":` + inner + `,"arguments":` + b1677Encoded(inner, 2) + `}`)
	got := InspectRawToolArgumentEnvelope(raw, b1677EnvelopeSchema)
	if got.Status != RawToolArgumentEnvelopeEquivalent || len(got.Candidates) != 2 {
		t.Fatalf("equal decoded objects acquired false outer ambiguity: %+v", got)
	}
	for _, candidate := range got.Candidates {
		if string(candidate.Object) != inner || strings.Count(string(candidate.Object), `"id"`) != 2 {
			t.Fatal("equivalence proof laundered an inner duplicate")
		}
	}
}

func TestB1677IdenticalConsumedWrappersHaveOneLosslessArgumentValue(t *testing.T) {
	inner := `{"path":"a.go","kind":"modify","limit":"1"}`
	for _, lane := range []string{"direct", "function-body", "function-arguments"} {
		for _, depth := range []int{0, 1, 4} {
			t.Run(fmt.Sprintf("%s/depth=%d", lane, depth), func(t *testing.T) {
				value := b1677Encoded(inner, depth)
				raw := json.RawMessage(b1677CompetingEnvelope("arguments", lane, value, value))
				got, path, ok := RawToolArgumentEnvelope(raw, b1677EnvelopeSchema)
				wantPath := []string{"arguments"}
				if lane != "direct" {
					wantPath = []string{"function", "arguments"}
				}
				if !ok || string(got) != inner || !reflect.DeepEqual(path, wantPath) {
					t.Errorf("identical repeated argument value has no lossless source: ok=%t path=%v raw=%s", ok, path, got)
				}
				normalized, report := Normalize(raw, b1677EnvelopeSchema, repairPolicy)
				if !report.Changed() || Validate(normalized, b1677EnvelopeSchema) != nil || !strings.Contains(string(normalized), `"path":"a.go"`) {
					t.Fatalf("identical wrapper changed existing execution shape: %s", normalized)
				}
			})
		}
	}
}
