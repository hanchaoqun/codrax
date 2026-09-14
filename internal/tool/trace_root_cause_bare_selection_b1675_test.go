package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

const b1675Selection = `[{"candidate_id":"candidate-worker","description":"Worker remains independently selected."},{"candidate_id":"candidate-sched","description":"Render thread waits for CPU."}]`

func TestB1675BareSelectionPublicFullAndPatch(t *testing.T) {
	for _, patch := range []bool{false, true} {
		for _, version := range []string{"", `"schema_version":2`, `"schema_version":"2"`} {
			t.Run(fmt.Sprintf("patch=%t/version=%s", patch, version), func(t *testing.T) {
				native := b1654Context()
				if result := b1654Execute(t, native, patch, b1654Report, ""); !result.Success {
					t.Fatal(result.Summary)
				}
				ctx := b1654Context()
				before, _ := modelOwnedAnswerBlockWire(ctx.Mutable.AnswerDocumentV2())
				contractBefore, _ := json.Marshal(ctx.Mutable.TraceFindingContract())
				result := b1654Execute(t, ctx, patch, b1675Selection, version)
				if !result.Success || len(result.OptionalCarrierOutcomes) != 0 || !reflect.DeepEqual(ctx.Mutable.TraceRootCauseReport(), native.Mutable.TraceRootCauseReport()) {
					t.Fatalf("pure ordered selection was not bound identically to the native report: report=%+v result=%+v", ctx.Mutable.TraceRootCauseReport(), result)
				}
				if err := requireModelOwnedAnswerBlockWirePreserved(before, ctx.Mutable.AnswerDocumentV2()); err != nil {
					t.Fatal(err)
				}
				contractAfter, _ := json.Marshal(ctx.Mutable.TraceFindingContract())
				if string(contractBefore) != string(contractAfter) {
					t.Fatal("container recovery mutated the frozen candidate contract")
				}
				accepted := ctx.Mutable.TraceRootCauseReport()
				if result := b1654Execute(t, ctx, patch, "", ""); !result.Success || !reflect.DeepEqual(ctx.Mutable.TraceRootCauseReport(), accepted) {
					t.Fatalf("omission did not preserve the accepted selection: %+v", result)
				}
				if result := b1654Execute(t, ctx, patch, `{"schema_version":2,"root_causes":[]}`, ""); !result.Success || ctx.Mutable.TraceRootCauseReport() == nil || len(ctx.Mutable.TraceRootCauseReport().RootCauses) != 0 {
					t.Fatalf("standard explicit empty selection stopped being a withdrawal: %+v", result)
				}
			})
		}
	}
}

func TestB1675BareSelectionStagesAcrossPublicBodyReject(t *testing.T) {
	for _, patch := range []bool{false, true} {
		t.Run(fmt.Sprintf("patch=%t", patch), func(t *testing.T) {
			ctx := b1654Context()
			result := b1654Execute(t, ctx, patch, b1675Selection, `"relation_claims":[]`)
			if result.Success || !strings.Contains(result.Summary, `top-level field "relation_claims" is not accepted`) {
				t.Fatalf("unrelated body rejection premise failed: %+v", result)
			}
			pending := ctx.Mutable.PendingTraceRootCauseReport()
			if pending == nil || len(pending.RootCauses) != 2 || len(result.OptionalCarrierOutcomes) != 0 || ctx.Mutable.TraceRootCauseReport() != nil {
				t.Fatalf("valid selection was not staged independently of the body: pending=%+v result=%+v", pending, result)
			}
			if result := b1654Execute(t, ctx, patch, "", ""); !result.Success || !reflect.DeepEqual(ctx.Mutable.TraceRootCauseReport(), pending) || ctx.Mutable.PendingTraceRootCauseReport() != nil {
				t.Fatalf("accepted follow-up did not commit the staged model selection: %+v", result)
			}
		})
	}
}

func TestB1675BareSelectionDoesNotOverrideOrInvent(t *testing.T) {
	cases := []struct{ name, selection, extra string }{
		{"empty", `[]`, ""},
		{"empty with explicit version", `[]`, `"schema_version":2`},
		{"unknown field", `[{"candidate_id":"candidate-sched","rank":1}]`, ""},
		{"missing identity", `[{"description":"A selected cause"}]`, ""},
		{"null identity", `[{"candidate_id":null}]`, ""},
		{"number identity", `[{"candidate_id":2}]`, ""},
		{"null description", `[{"candidate_id":"candidate-sched","description":null}]`, ""},
		{"number description", `[{"candidate_id":"candidate-sched","description":2}]`, ""},
		{"null element", `[null]`, ""},
		{"scalar element", `[2]`, ""},
		{"nested array", `[[{"candidate_id":"candidate-sched"}]]`, ""},
		{"duplicate candidate key", `[{"candidate_id":"unknown","candidate_id":"candidate-sched"}]`, ""},
		{"escaped duplicate key", `[{"candidate_id":"unknown","candidate_\u0069d":"candidate-sched"}]`, ""},
		{"competing candidate key", `[{"CANDIDATE_ID":"unknown","candidate_id":"candidate-sched"}]`, ""},
		{"duplicate description", `[{"candidate_id":"candidate-sched","description":"first","description":"second"}]`, ""},
		{"competing description", `[{"candidate_id":"candidate-sched","DESCRIPTION":"first","description":"second"}]`, ""},
		{"wrong version", b1675Selection, `"schema_version":3`},
		{"null version", b1675Selection, `"schema_version":null`},
		{"float version", b1675Selection, `"schema_version":2.0`},
		{"spaced version", b1675Selection, `"schema_version":" 2"`},
		{"duplicate version", b1675Selection, `"schema_version":3,"schema_version":2`},
		{"competing version", b1675Selection, `"SCHEMA_VERSION":3,"schema_version":2`},
		{"case variant version only", b1675Selection, `"SCHEMA_VERSION":3`},
		{"unknown candidate", `[{"candidate_id":"unknown"}]`, ""},
		{"duplicate selected identity", `[{"candidate_id":"candidate-sched"},{"candidate_id":"candidate-sched"}]`, ""},
		{"competing top level selection", b1675Selection, `"root_causes":[]`},
		{"case variant top level selection", b1675Selection, `"ROOT_CAUSES":[]`},
		{"multiple complete reports", "[" + b1654Report + "," + b1654Report + "]", ""},
		{"wrapped items", `[{"candidate_id":"candidate-worker","description":"outer","schema_version":2,"root_causes":[{"candidate_id":"candidate-worker"}]},{"candidate_id":"candidate-sched","schema_version":2,"root_causes":[{"candidate_id":"candidate-sched"}]}]`, ""},
	}
	for _, patch := range []bool{false, true} {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("patch=%t/%s", patch, tc.name), func(t *testing.T) {
				ctx := b1654Context()
				if result := b1654Execute(t, ctx, patch, b1654Report, ""); !result.Success {
					t.Fatal(result.Summary)
				}
				previous := ctx.Mutable.TraceRootCauseReport()
				before, _ := modelOwnedAnswerBlockWire(ctx.Mutable.AnswerDocumentV2())
				result := b1654Execute(t, ctx, patch, tc.selection, tc.extra)
				if !result.Success || len(result.OptionalCarrierOutcomes) != 1 || !ctx.Mutable.TraceRootCauseSelectorRejected() || ctx.Mutable.PendingTraceRootCauseReport() != nil {
					t.Fatalf("invalid optional selector escaped disclosure or altered body acceptance: %+v", result)
				}
				if patch && !reflect.DeepEqual(ctx.Mutable.TraceRootCauseReport(), previous) {
					t.Fatal("invalid replacement overwrote the previous selection")
				}
				if err := requireModelOwnedAnswerBlockWirePreserved(before, ctx.Mutable.AnswerDocumentV2()); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestB1675BareSelectionSingleAndBinderBoundaries(t *testing.T) {
	for _, patch := range []bool{false, true} {
		for _, description := range []string{"", "The request waits for CPU.", strings.Repeat("x", 321), "See candidate-sched for the exact receipt."} {
			t.Run(fmt.Sprintf("patch=%t/description=%d", patch, len(description)), func(t *testing.T) {
				selection, _ := json.Marshal([]map[string]string{{"candidate_id": "candidate-sched", "description": description}})
				native := b1654Context()
				control := b1654Execute(t, native, patch, `{"schema_version":2,"root_causes":`+string(selection)+`}`, "")
				ctx := b1654Context()
				result := b1654Execute(t, ctx, patch, string(selection), "")
				if !result.Success || !control.Success || !reflect.DeepEqual(ctx.Mutable.TraceRootCauseReport(), native.Mutable.TraceRootCauseReport()) || !reflect.DeepEqual(result.OptionalCarrierOutcomes, control.OptionalCarrierOutcomes) {
					t.Fatalf("container repair bypassed binder description handling: result=%+v control=%+v", result, control)
				}
			})
		}
		ctx := b1654Context()
		contract := ctx.Mutable.TraceFindingContract()
		background := contract.Candidates[0]
		background.Decision.CandidateID = "candidate-background"
		background.PrimaryEligible = false
		contract.Candidates = append(contract.Candidates, background)
		ctx.Mutable.SetTraceFindingContract(contract)
		result := b1654Execute(t, ctx, patch, `[{"candidate_id":"candidate-background"}]`, "")
		if !result.Success || len(result.OptionalCarrierOutcomes) != 1 || !strings.Contains(result.OptionalCarrierOutcomes[0].Reason, "outside the selectable typed on-chain roster") || ctx.Mutable.TraceRootCauseReport() != nil {
			t.Fatalf("patch=%t: container repair promoted a background-only candidate: %+v", patch, result)
		}
	}
}

func TestB1675BareSelectionCompetingCarriersStayInvalid(t *testing.T) {
	for _, patch := range []bool{false, true} {
		other := "replace_trace_root_causes"
		if patch {
			other = "trace_root_causes"
		}
		for _, competitor := range []string{"null", "[]", b1654Report} {
			ctx := b1654Context()
			result := b1654Execute(t, ctx, patch, b1675Selection, `"`+other+`":`+competitor)
			if !result.Success || len(result.OptionalCarrierOutcomes) != 1 || ctx.Mutable.TraceRootCauseReport() != nil || ctx.Mutable.PendingTraceRootCauseReport() != nil {
				t.Fatalf("patch=%t competing=%s: recovery chose between competing carriers: %+v", patch, competitor, result)
			}
		}
	}
}

func TestB1675BareSelectionConflictCannotStageOnBodyReject(t *testing.T) {
	for _, patch := range []bool{false, true} {
		for _, extra := range []string{`"schema_version":3`, `"schema_version":3,"schema_version":2`, `"SCHEMA_VERSION":3,"schema_version":2`, `"root_causes":[]`} {
			ctx := b1654Context()
			result := b1654Execute(t, ctx, patch, b1675Selection, extra+`,"relation_claims":[]`)
			if result.Success || len(result.OptionalCarrierOutcomes) != 1 || ctx.Mutable.TraceRootCauseReport() != nil || ctx.Mutable.PendingTraceRootCauseReport() != nil {
				t.Fatalf("patch=%t extra=%s: pre-decode recovery ignored a conflict: %+v", patch, extra, result)
			}
		}
	}
}

func TestB1675BareSelectionRawValuesAndOnePassBoundary(t *testing.T) {
	// The compatibility layer must not run the binder's whitespace/description
	// policy itself, and never reinterpret identifiers, escapes or their order.
	selection := `[ {"candidate_id":"candidate-worker","description":"  业务\\path \"quoted\" <T>\nline\t\u0057  "}, {"candidate_id":"candidate-sched"} ]`
	for _, field := range []string{"trace_root_causes", "replace_trace_root_causes"} {
		for _, unchanged := range []string{`{"blocks":[{"text":"unchanged"}]}`, `{ "` + field + `" : ` + b1654Report + ` }`} {
			if got, changed := normalizeBareTraceRootCauseSelectionCarrier(json.RawMessage(unchanged), field); changed || string(got) != unchanged {
				t.Fatalf("ordinary omitted/native report request was modified: %s", unchanged)
			}
		}
		raw := json.RawMessage(`{"` + field + `":` + selection + `,"schema_version":"2","blocks":[{"text":"unchanged <T> \"text\""}]}`)
		before := string(raw)
		repaired, ok := normalizeBareTraceRootCauseSelectionCarrier(raw, field)
		if !ok || string(raw) != before {
			t.Fatal("pure array was not repaired or input bytes mutated")
		}
		var root map[string]json.RawMessage
		if err := json.Unmarshal(repaired, &root); err != nil {
			t.Fatal(err)
		}
		var report struct {
			SchemaVersion int                 `json:"schema_version"`
			RootCauses    []map[string]string `json:"root_causes"`
		}
		if err := json.Unmarshal(root[field], &report); err != nil {
			t.Fatal(err)
		}
		var want []map[string]string
		if err := json.Unmarshal([]byte(selection), &want); err != nil {
			t.Fatal(err)
		}
		if report.SchemaVersion != types.TraceRootCauseReportSchemaVersion || !reflect.DeepEqual(report.RootCauses, want) {
			t.Fatalf("model-owned ordered values changed: got=%+v want=%+v", report, want)
		}
		if _, exists := root["schema_version"]; exists {
			t.Fatal("explicit outer version was duplicated rather than moved")
		}
		var original map[string]json.RawMessage
		_ = json.Unmarshal(raw, &original)
		var originalBlocks, repairedBlocks any
		_ = json.Unmarshal(original["blocks"], &originalBlocks)
		_ = json.Unmarshal(root["blocks"], &repairedBlocks)
		if !reflect.DeepEqual(originalBlocks, repairedBlocks) {
			t.Fatal("sibling model answer values changed")
		}
		if again, changed := normalizeBareTraceRootCauseSelectionCarrier(repaired, field); changed || string(again) != string(repaired) {
			t.Fatal("recovery is not idempotent")
		}
		for _, value := range []string{`[]`, b1654Report, "[" + b1654Report + "]", `[[{"candidate_id":"candidate-sched"}]]`, `"[{\"candidate_id\":\"candidate-sched\"}]"`} {
			original := json.RawMessage(`{"` + field + `":` + value + `}`)
			if got, changed := normalizeBareTraceRootCauseSelectionCarrier(original, field); changed || string(got) != string(original) {
				t.Fatalf("non-pure shape changed or acquired another decoding layer: %s", value)
			}
		}
	}
}

func TestB1675BareSelectionCannotEnableContract(t *testing.T) {
	for _, patch := range []bool{false, true} {
		ctx := b1654Context()
		ctx.Mutable.SetTraceFindingContract(nil)
		result := b1654Execute(t, ctx, patch, b1675Selection, "")
		if !result.Success || len(result.OptionalCarrierOutcomes) != 1 || ctx.Mutable.TraceRootCauseReport() != nil || ctx.Mutable.PendingTraceRootCauseReport() != nil {
			t.Fatalf("patch=%t: pure array enabled an absent contract: %+v", patch, result)
		}
	}
}
