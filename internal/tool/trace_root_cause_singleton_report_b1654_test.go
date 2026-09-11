package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

const b1654Report = `{"schema_version":2,"root_causes":[{"candidate_id":"candidate-worker","description":"Worker remains independently selected."},{"candidate_id":"candidate-sched","description":"Render thread waits for CPU."}]}`

func b1654Context() *types.BusContext {
	mut := types.NewMutableState("explicit model root-cause selection")
	contract := testSelectableTraceRootCauseContract()
	second := contract.Candidates[0]
	second.Decision.CandidateID = "candidate-worker"
	second.Decision.SubjectName = "WorkerThread"
	second.Decision.EvidenceRefs = []string{"E-worker"}
	contract.Candidates = append(contract.Candidates, second)
	mut.SetTraceFindingContract(contract)
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "summary", Kind: types.BlockSummary, Text: "The model owns this complete answer."}}})
	return &types.BusContext{Mutable: mut}
}

func b1654Execute(t *testing.T, ctx *types.BusContext, patch bool, report, extra string) types.ToolResult {
	t.Helper()
	field, body := "trace_root_causes", `"blocks":[{"id":"summary","kind":"summary","text":"The model owns this complete answer."}]`
	if patch {
		field, body = "replace_trace_root_causes", `"unchanged_block_ids":["summary"]`
	}
	if report != "" {
		body += `,"` + field + `":` + report
	}
	if extra != "" {
		body += "," + extra
	}
	raw := json.RawMessage("{" + body + "}")
	var result types.ToolResult
	var err error
	if patch {
		result, err = (&EmitAnswerDocumentPatch{}).Execute(ctx, raw)
	} else {
		result, err = (&EmitAnswerDocument{}).Execute(ctx, raw)
	}
	if err != nil {
		t.Fatalf("public emit returned execution error: %v; %+v", err, result)
	}
	return result
}

func TestB1654SingletonReportPublicFullAndPatch(t *testing.T) {
	for _, patch := range []bool{false, true} {
		t.Run(fmt.Sprintf("patch=%t", patch), func(t *testing.T) {
			native := b1654Context()
			if result := b1654Execute(t, native, patch, b1654Report, ""); !result.Success || len(result.OptionalCarrierOutcomes) != 0 {
				t.Fatalf("native control: %+v", result)
			}
			ctx := b1654Context()
			before, _ := modelOwnedAnswerBlockWire(ctx.Mutable.AnswerDocumentV2())
			result := b1654Execute(t, ctx, patch, "[ \n"+b1654Report+"\n ]", "")
			if !result.Success || len(result.OptionalCarrierOutcomes) != 0 {
				t.Errorf("single complete native report wrapper was lost: %+v", result)
			}
			got := ctx.Mutable.TraceRootCauseReport()
			if got == nil || !reflect.DeepEqual(got, native.Mutable.TraceRootCauseReport()) {
				t.Fatalf("wrapper changed or lost ordered model selection: got=%+v want=%+v", got, native.Mutable.TraceRootCauseReport())
			}
			if got.RootCauses[0].ThreadName != "WorkerThread" || got.RootCauses[1].ThreadName != "RenderThread" || got.RootCauses[0].Description != "Worker remains independently selected." {
				t.Fatal("selected identity, order or description changed")
			}
			if err := requireModelOwnedAnswerBlockWirePreserved(before, ctx.Mutable.AnswerDocumentV2()); err != nil {
				t.Fatal(err)
			}
			// Omission still retains the accepted report; a complete wrapped
			// report with an explicit empty selection is still a withdrawal.
			if result := b1654Execute(t, ctx, patch, "", ""); !result.Success || !reflect.DeepEqual(ctx.Mutable.TraceRootCauseReport(), got) {
				t.Fatalf("omission lost the accepted report: %+v", result)
			}
			if result := b1654Execute(t, ctx, patch, `[{"schema_version":2,"root_causes":[]}]`, ""); !result.Success || ctx.Mutable.TraceRootCauseReport() == nil || len(ctx.Mutable.TraceRootCauseReport().RootCauses) != 0 {
				t.Fatalf("explicit empty selection did not remain a withdrawal: %+v", result)
			}
		})
	}
}

func TestB1654SingletonReportStagesAcrossUnrelatedPublicReject(t *testing.T) {
	for _, patch := range []bool{false, true} {
		t.Run(fmt.Sprintf("patch=%t", patch), func(t *testing.T) {
			ctx := b1654Context()
			// This pre-decode sibling error exercises the real raw-parameter
			// resolver and deferred selector commit, not a manual pending slot.
			result := b1654Execute(t, ctx, patch, "["+b1654Report+"]", `"relation_claims":[]`)
			if result.Success || !strings.Contains(result.Summary, `top-level field "relation_claims" is not accepted`) {
				t.Fatalf("unrelated rejection premise failed: %+v", result)
			}
			pending := ctx.Mutable.PendingTraceRootCauseReport()
			if pending == nil || len(pending.RootCauses) != 2 || ctx.Mutable.TraceRootCauseReport() != nil || len(result.OptionalCarrierOutcomes) != 0 {
				t.Fatalf("valid wrapped selector was not staged independently: pending=%+v result=%+v", pending, result)
			}
			result = b1654Execute(t, ctx, patch, "", "")
			if !result.Success || !reflect.DeepEqual(ctx.Mutable.TraceRootCauseReport(), pending) || ctx.Mutable.PendingTraceRootCauseReport() != nil {
				t.Fatalf("ordinary accepted follow-up lost pending selection: %+v", result)
			}
		})
	}
}

func TestB1654SingletonReportDoesNotInventOrOverride(t *testing.T) {
	cases := []struct{ name, report string }{
		{"empty wrapper", `[]`},
		{"multiple reports", "[" + b1654Report + "," + b1654Report + "]"},
		{"null element", `[null]`},
		{"scalar element", `[2]`},
		{"nested wrapper", "[[" + b1654Report + "]]"},
		{"encoded object element", `["{\"schema_version\":2,\"root_causes\":[]}"]`},
		{"missing version", `[{"root_causes":[{"candidate_id":"candidate-sched"}]}]`},
		{"missing causes", `[{"schema_version":2}]`},
		{"null causes", `[{"schema_version":2,"root_causes":null}]`},
		{"object causes", `[{"schema_version":2,"root_causes":{"candidate_id":"candidate-sched"}}]`},
		{"bare candidate array", `[{"candidate_id":"candidate-sched"}]`},
		{"duplicate version key", `[{"schema_version":3,"schema_version":2,"root_causes":[{"candidate_id":"candidate-sched"}]}]`},
		{"duplicate causes key", `[{"schema_version":2,"root_causes":[],"root_causes":[{"candidate_id":"candidate-sched"}]}]`},
		{"duplicate candidate key", `[{"schema_version":2,"root_causes":[{"candidate_id":"unknown","candidate_id":"candidate-sched"}]}]`},
		{"duplicate description key", `[{"schema_version":2,"root_causes":[{"candidate_id":"candidate-sched","description":"first","description":"second"}]}]`},
		{"competing version keys", `[{"SCHEMA_VERSION":3,"schema_version":2,"root_causes":[{"candidate_id":"candidate-sched"}]}]`},
		{"competing causes keys", `[{"schema_version":2,"ROOT_CAUSES":[],"root_causes":[{"candidate_id":"candidate-sched"}]}]`},
		{"competing candidate keys", `[{"schema_version":2,"root_causes":[{"CANDIDATE_ID":"unknown","candidate_id":"candidate-sched"}]}]`},
		{"competing description keys", `[{"schema_version":2,"root_causes":[{"candidate_id":"candidate-sched","DESCRIPTION":"first","description":"second"}]}]`},
		{"nested duplicate key", `[{"schema_version":2,"root_causes":[{"candidate_id":"candidate-sched","extra":{"name":1,"name":2}}]}]`},
		{"unknown candidate", `[{"schema_version":2,"root_causes":[{"candidate_id":"unknown"}]}]`},
		{"duplicate candidate selection", `[{"schema_version":2,"root_causes":[{"candidate_id":"candidate-sched"},{"candidate_id":"candidate-sched"}]}]`},
		{"wrong version", `[{"schema_version":3,"root_causes":[{"candidate_id":"candidate-sched"}]}]`},
		{"string version", `[{"schema_version":"2","root_causes":[{"candidate_id":"candidate-sched"}]}]`},
	}
	// Malformed/truncated JSON is tested separately: it rejects the original
	// document decode and must never be repaired into a valid submission.
	for _, patch := range []bool{false, true} {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("patch=%t/%s", patch, tc.name), func(t *testing.T) {
				if !json.Valid([]byte(tc.report)) {
					t.Fatal("test requires a valid outer JSON envelope")
				}
				ctx := b1654Context()
				if result := b1654Execute(t, ctx, patch, b1654Report, ""); !result.Success {
					t.Fatal(result.Summary)
				}
				previous := ctx.Mutable.TraceRootCauseReport()
				result := b1654Execute(t, ctx, patch, tc.report, "")
				if !result.Success || len(result.OptionalCarrierOutcomes) != 1 || !ctx.Mutable.TraceRootCauseSelectorRejected() {
					t.Fatalf("bad optional selector changed ordinary answer acceptance or escaped disclosure: %+v", result)
				}
				// Existing patch behavior retains the previous accepted report.
				// Full re-emit behavior is not changed by this shape repair.
				if patch && !reflect.DeepEqual(ctx.Mutable.TraceRootCauseReport(), previous) {
					t.Fatalf("invalid wrapped replacement overrode previous report: %+v", result)
				}
				if ctx.Mutable.PendingTraceRootCauseReport() != nil {
					t.Fatal("invalid selector was staged")
				}
				if ctx.Mutable.AnswerDocumentV2().Blocks[0].Text != "The model owns this complete answer." {
					t.Fatal("invalid optional selector changed model answer")
				}
			})
		}
	}
}

func TestB1654SingletonReportRawBytesAndOneLayerBoundary(t *testing.T) {
	// Unknown fields remain untouched for the existing decoder/binder. The
	// compatibility layer owns only the enclosing array, not report contents.
	inner := "{\n  \"schema_version\" : 3,\n  \"root_causes\" : [{\"candidate_id\":\"candidate-sched\",\"description\":\"a  b\\n\\u0057\"}],\n  \"extra\": {\"name\": [1, true, null]}\n}"
	raw := json.RawMessage(" \n[ \t" + inner + "\n] \t")
	before := string(raw)
	got, ok := unwrapSingletonTraceRootCauseReport(raw)
	if !ok || string(got) != inner || string(raw) != before {
		t.Fatalf("inner original bytes/version or input changed: ok=%t got=%s", ok, got)
	}
	for _, raw := range []string{
		b1654Report, "[[" + b1654Report + "]]", "[" + b1654Report,
		"[" + b1654Report + "] trailing", "[" + b1654Report + ",]",
		`"[{\"schema_version\":2,\"root_causes\":[]}]"`,
		`[{"schema_version":2,"root_causes":[],"extra":[{"x":1,"\u0078":2}]}]`,
	} {
		got, ok := unwrapSingletonTraceRootCauseReport(json.RawMessage(raw))
		if ok || string(got) != raw {
			t.Fatalf("unsupported shape was changed: ok=%t input=%s output=%s", ok, raw, got)
		}
	}
}

func TestB1654SingletonReportExistingStringCoercionComposition(t *testing.T) {
	// The public parameter layer already losslessly decodes JSON strings.
	// The shared resolver therefore receives a native array in the valid
	// case. This repair does not add a second string-decoding path of its own.
	cases := []struct {
		name, decoded string
		valid         bool
	}{
		{"complete array", "[" + b1654Report + "]", true},
		{"nested arrays", "[[" + b1654Report + "]]", false},
		{"duplicate keys", `[{"schema_version":3,"schema_version":2,"root_causes":[{"candidate_id":"candidate-sched"}]}]`, false},
		{"competing keys", `[{"schema_version":2,"root_causes":[{"CANDIDATE_ID":"unknown","candidate_id":"candidate-sched"}]}]`, false},
		{"missing version", `[{"root_causes":[{"candidate_id":"candidate-sched"}]}]`, false},
	}
	for _, patch := range []bool{false, true} {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("patch=%t/%s", patch, tc.name), func(t *testing.T) {
				encoded, err := json.Marshal(tc.decoded)
				if err != nil {
					t.Fatal(err)
				}
				ctx := b1654Context()
				result := b1654Execute(t, ctx, patch, string(encoded), "")
				if !result.Success {
					t.Fatalf("optional carrier changed body acceptance: %+v", result)
				}
				if !tc.valid {
					if len(result.OptionalCarrierOutcomes) != 1 || ctx.Mutable.TraceRootCauseReport() != nil || ctx.Mutable.PendingTraceRootCauseReport() != nil {
						t.Fatalf("string decoding bypassed wrapper restrictions: %+v", result)
					}
					return
				}
				control := b1654Context()
				b1654Execute(t, control, patch, b1654Report, "")
				if len(result.OptionalCarrierOutcomes) != 0 || !reflect.DeepEqual(ctx.Mutable.TraceRootCauseReport(), control.Mutable.TraceRootCauseReport()) {
					t.Fatalf("existing string decoding changed or lost the native report: %+v", result)
				}
			})
		}
	}
}

func TestB1654SingletonReportCannotFillMissingVersionOrEnableContract(t *testing.T) {
	for _, patch := range []bool{false, true} {
		t.Run(fmt.Sprintf("patch=%t/missing-version", patch), func(t *testing.T) {
			ctx := b1654Context()
			result := b1654Execute(t, ctx, patch, `[{"root_causes":[{"candidate_id":"candidate-sched"}]}]`, `"schema_version":2`)
			if ctx.Mutable.TraceRootCauseReport() != nil || ctx.Mutable.PendingTraceRootCauseReport() != nil || len(result.OptionalCarrierOutcomes) != 1 {
				t.Fatalf("array wrapper borrowed a version from another layer: %+v", result)
			}
		})
		t.Run(fmt.Sprintf("patch=%t/not-enabled", patch), func(t *testing.T) {
			ctx := b1654Context()
			ctx.Mutable.SetTraceFindingContract(nil)
			result := b1654Execute(t, ctx, patch, "["+b1654Report+"]", "")
			if !result.Success || len(result.OptionalCarrierOutcomes) != 1 || ctx.Mutable.TraceRootCauseReport() != nil || ctx.Mutable.PendingTraceRootCauseReport() != nil {
				t.Fatalf("wrapper changed the existing optional-carrier contract gate: %+v", result)
			}
		})
	}
}

func TestB1654SingletonReportTruncationDoesNotRepairPublicEnvelope(t *testing.T) {
	for _, patch := range []bool{false, true} {
		t.Run(fmt.Sprintf("patch=%t", patch), func(t *testing.T) {
			ctx := b1654Context()
			if result := b1654Execute(t, ctx, patch, b1654Report, ""); !result.Success {
				t.Fatal(result.Summary)
			}
			previous := ctx.Mutable.TraceRootCauseReport()
			before, _ := modelOwnedAnswerBlockWire(ctx.Mutable.AnswerDocumentV2())
			raw := json.RawMessage(`{"blocks":[],"trace_root_causes":[{"schema_version":2,"root_causes":[`)
			var result types.ToolResult
			var err error
			if patch {
				raw = json.RawMessage(`{"unchanged_block_ids":["summary"],"replace_trace_root_causes":[{"schema_version":2,"root_causes":[`)
				result, err = (&EmitAnswerDocumentPatch{}).Execute(ctx, raw)
			} else {
				result, err = (&EmitAnswerDocument{}).Execute(ctx, raw)
			}
			if result.Success || !reflect.DeepEqual(previous, ctx.Mutable.TraceRootCauseReport()) || ctx.Mutable.PendingTraceRootCauseReport() != nil {
				t.Fatalf("truncation repaired or overwrote the prior report: err=%v result=%+v", err, result)
			}
			if err := requireModelOwnedAnswerBlockWirePreserved(before, ctx.Mutable.AnswerDocumentV2()); err != nil {
				t.Fatal(err)
			}
		})
	}
}
