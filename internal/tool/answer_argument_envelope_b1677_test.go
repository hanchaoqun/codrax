package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func b1677Envelope(a, b string, function bool, depth int) json.RawMessage {
	encode := func(value string) string {
		for i := 0; i < depth; i++ {
			raw, _ := json.Marshal(value)
			value = string(raw)
		}
		return value
	}
	if function {
		return json.RawMessage(`{"function":{"arguments":` + encode(a) + `},"function":{"arguments":` + encode(b) + `}}`)
	}
	return json.RawMessage(`{"arguments":` + encode(a) + `,"argum\u0065nts":` + encode(b) + `}`)
}

func TestB1677AnswerEnvelopeIndependentBodyAndSelection(t *testing.T) {
	for _, patch := range []bool{false, true} {
		for _, function := range []bool{false, true} {
			for depth := 0; depth <= 2; depth++ {
				for _, reverse := range []bool{false, true} {
					for _, bodyConflict := range []bool{false, true} {
						for _, selectorConflict := range []bool{false, true} {
							t.Run(fmt.Sprintf("patch=%t/function=%t/depth=%d/reverse=%t/body=%t/selector=%t", patch, function, depth, reverse, bodyConflict, selectorConflict), func(t *testing.T) {
								ctx := b1654Context()
								if result := b1654Execute(t, ctx, patch, b1654Report, ""); !result.Success {
									t.Fatal(result.Summary)
								}
								previous := ctx.Mutable.AnswerDocumentV2()
								oldReport := ctx.Mutable.TraceRootCauseReport()
								field, bodyKey := "trace_root_causes", "blocks"
								if patch {
									field, bodyKey = "replace_trace_root_causes", "replace_blocks"
								}
								bodyA := `"` + bodyKey + `":[{"id":"summary","kind":"summary","text":"The model supplied this exact answer."}]`
								bodyB := bodyA
								if bodyConflict {
									bodyB = `"` + bodyKey + `":[{"id":"summary","kind":"summary","text":"A different model answer."}]`
								}
								selectionA := `{"schema_version":2,"root_causes":[{"candidate_id":"candidate-sched"}]}`
								selectionB := selectionA
								if selectorConflict {
									selectionB = `{"schema_version":2,"root_causes":[]}`
								}
								a := "{" + bodyA + `,"` + field + `":` + selectionA + "}"
								b := "{" + bodyB + `,"` + field + `":` + selectionB + "}"
								if reverse {
									a, b = b, a
								}
								raw := b1677Envelope(a, b, function, depth)
								var result types.ToolResult
								var err error
								if patch {
									result, err = (&EmitAnswerDocumentPatch{}).Execute(ctx, raw)
								} else {
									result, err = (&EmitAnswerDocument{}).Execute(ctx, raw)
								}
								if err != nil || result.Success == bodyConflict {
									t.Fatalf("body ownership was lost: err=%v result=%+v", err, result)
								}
								if bodyConflict {
									if !reflect.DeepEqual(previous, ctx.Mutable.AnswerDocumentV2()) {
										t.Fatal("ambiguous body replaced the previous model answer")
									}
								} else if got := ctx.Mutable.AnswerDocumentV2().Blocks[0].Text; got != "The model supplied this exact answer." {
									t.Fatalf("common body changed: %q", got)
								}
								if selectorConflict {
									if !ctx.Mutable.TraceRootCauseSelectorRejected() || len(result.OptionalCarrierOutcomes) != 1 || ctx.Mutable.PendingTraceRootCauseReport() != nil {
										t.Fatalf("competing selection was accepted or not disclosed: %+v", result)
									}
									if (patch || bodyConflict) && !reflect.DeepEqual(oldReport, ctx.Mutable.TraceRootCauseReport()) {
										t.Fatal("rejected replacement changed old report")
									}
									if !patch && !bodyConflict && ctx.Mutable.TraceRootCauseReport() != nil {
										t.Fatal("fresh full answer inherited an invalid selection")
									}
								} else {
									report := ctx.Mutable.TraceRootCauseReport()
									if bodyConflict {
										report = ctx.Mutable.PendingTraceRootCauseReport()
									}
									if report == nil || len(report.RootCauses) != 1 || report.RootCauses[0].ThreadName != "RenderThread" || len(result.OptionalCarrierOutcomes) != 0 {
										t.Fatalf("common valid selection was lost: report=%+v result=%+v", report, result)
									}
								}
							})
						}
					}
				}
			}
		}
	}
}

func TestB1677AnswerEnvelopeIncompleteAlternativesDoNotChooseBody(t *testing.T) {
	for _, patch := range []bool{false, true} {
		for _, alternative := range []string{`null`, `"not JSON"`, `[]`, `{"metadata":"not an argument object"}`} {
			for _, reverse := range []bool{false, true} {
				t.Run(fmt.Sprintf("patch=%t/alternative=%s/reverse=%t", patch, alternative, reverse), func(t *testing.T) {
					ctx := b1654Context()
					previous := ctx.Mutable.AnswerDocumentV2()
					ctx.Mutable.SetPendingAnswerDocumentPatchBase(previous)
					base := ctx.Mutable.PendingAnswerDocumentPatchBase()
					lease := &types.AnswerDiagramRelationRepairLease{}
					ctx.Mutable.SetAnswerDiagramRelationRepairLease(lease)
					beforeLease := ctx.Mutable.AnswerDiagramRelationRepairLease()
					field, body := "trace_root_causes", `"blocks":[{"id":"summary","kind":"summary","text":"Do not choose this alternative."}]`
					if patch {
						field, body = "replace_trace_root_causes", `"replace_blocks":[{"id":"summary","kind":"summary","text":"Do not choose this alternative."}]`
					}
					a, b := `{`+body+`,"`+field+`":`+b1654Report+`}`, alternative
					if reverse {
						a, b = b, a
					}
					var res types.ToolResult
					if patch {
						res, _ = (&EmitAnswerDocumentPatch{}).Execute(ctx, b1677Envelope(a, b, false, 0))
					} else {
						res, _ = (&EmitAnswerDocument{}).Execute(ctx, b1677Envelope(a, b, false, 0))
					}
					if res.Success || !reflect.DeepEqual(previous, ctx.Mutable.AnswerDocumentV2()) || !reflect.DeepEqual(base, ctx.Mutable.PendingAnswerDocumentPatchBase()) || !reflect.DeepEqual(beforeLease, ctx.Mutable.AnswerDiagramRelationRepairLease()) {
						t.Fatalf("incomplete alternatives selected/reset answer state: %+v", res)
					}
					if ctx.Mutable.PendingTraceRootCauseReport() != nil || !ctx.Mutable.TraceRootCauseSelectorRejected() || len(res.OptionalCarrierOutcomes) != 1 {
						t.Fatalf("incomplete alternative authorized selection: %+v", res)
					}
				})
			}
		}
	}
}

func TestB1677EnvelopeSelectionOmissionAndWithdrawal(t *testing.T) {
	for _, patch := range []bool{false, true} {
		for _, selection := range []string{"", "null", `{"schema_version":2,"root_causes":[]}`} {
			ctx := b1654Context()
			_ = b1654Execute(t, ctx, patch, b1654Report, `"relation_claims":[]`)
			pending := ctx.Mutable.PendingTraceRootCauseReport()
			field, body := "trace_root_causes", `"blocks":[{"id":"summary","kind":"summary","text":"The model owns this complete answer."}]`
			if patch {
				field, body = "replace_trace_root_causes", `"unchanged_block_ids":["summary"]`
			}
			if selection != "" {
				body += `,"` + field + `":` + selection
			}
			params := "{" + body + "}"
			var res types.ToolResult
			if patch {
				res, _ = (&EmitAnswerDocumentPatch{}).Execute(ctx, b1677Envelope(params, params, false, 0))
			} else {
				res, _ = (&EmitAnswerDocument{}).Execute(ctx, b1677Envelope(params, params, false, 0))
			}
			if !res.Success || len(res.OptionalCarrierOutcomes) != 0 {
				t.Fatalf("patch=%t selection=%s: %+v", patch, selection, res)
			}
			got := ctx.Mutable.TraceRootCauseReport()
			if selection == "" || selection == "null" {
				if pending == nil || !reflect.DeepEqual(got, pending) {
					t.Fatal("omission lost pending selection")
				}
			} else if got == nil || len(got.RootCauses) != 0 {
				t.Fatal("explicit empty report lost withdrawal semantics")
			}
		}
	}
}

func TestB1677TruncatedEnvelopeComparisonPreservesAllState(t *testing.T) {
	ctx := b1654Context()
	previous := ctx.Mutable.AnswerDocumentV2()
	ctx.Mutable.SetPendingAnswerDocumentPatchBase(previous)
	base := ctx.Mutable.PendingAnswerDocumentPatchBase()
	params := `{"blocks":[{"id":"summary","kind":"summary","text":"Never choose a bounded prefix."}],"trace_root_causes":` + b1654Report + `}`
	entries := make([]string, 65)
	for i := range entries {
		entries[i] = `"arguments":` + params
	}
	result, err := (&EmitAnswerDocument{}).Execute(ctx, json.RawMessage("{"+strings.Join(entries, ",")+"}"))
	if err != nil || result.Success || !reflect.DeepEqual(previous, ctx.Mutable.AnswerDocumentV2()) || !reflect.DeepEqual(base, ctx.Mutable.PendingAnswerDocumentPatchBase()) || ctx.Mutable.PendingTraceRootCauseReport() != nil || len(result.OptionalCarrierOutcomes) != 1 {
		t.Fatalf("truncated candidate prefix acquired ownership: err=%v result=%+v", err, result)
	}
}

func TestB1677UninspectedTailCannotClaimSelectorOmission(t *testing.T) {
	for _, patch := range []bool{false, true} {
		ctx := b1654Context()
		field, body := "trace_root_causes", `"blocks":[{"id":"summary","kind":"summary","text":"Uninspected tail."}]`
		if patch {
			field, body = "replace_trace_root_causes", `"unchanged_block_ids":["summary"]`
		}
		entries := make([]string, 65)
		for i := range entries {
			entries[i] = `"arguments":{` + body + `}`
		}
		entries[64] = `"arguments":{` + body + `,"` + field + `":` + b1654Report + `}`
		raw := json.RawMessage("{" + strings.Join(entries, ",") + "}")
		var result types.ToolResult
		if patch {
			result, _ = (&EmitAnswerDocumentPatch{}).Execute(ctx, raw)
		} else {
			result, _ = (&EmitAnswerDocument{}).Execute(ctx, raw)
		}
		if result.Success || strings.Contains(result.Summary, "omitted") || !strings.Contains(result.Summary, "comparison limit") || len(result.OptionalCarrierOutcomes) != 0 || ctx.Mutable.TraceRootCauseSelectorRejected() || ctx.Mutable.PendingTraceRootCauseReport() != nil {
			t.Fatalf("uninspected selector was guessed as absent/rejected: %+v", result)
		}
	}
}
