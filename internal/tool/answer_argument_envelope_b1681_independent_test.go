package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func b1681IndependentEnvelope(a, b string, function bool) string {
	if function {
		return `{"function":{"arguments":` + a + `},"function":{"arguments":` + b + `}}`
	}
	return `{"arguments":` + a + `,"argum\u0065nts":` + b + `}`
}

func b1681IndependentEmit(t *testing.T, ctx *types.BusContext, patch bool, body, selector string) types.ToolResult {
	t.Helper()
	field, blocks := "trace_root_causes", "blocks"
	if patch {
		field, blocks = "replace_trace_root_causes", "replace_blocks"
	}
	raw := json.RawMessage(`{"` + blocks + `":[` + body + `],"` + field + `":` + selector + `}`)
	var result types.ToolResult
	var err error
	if patch {
		result, err = (&EmitAnswerDocumentPatch{}).Execute(ctx, raw)
	} else {
		result, err = (&EmitAnswerDocument{}).Execute(ctx, raw)
	}
	if err != nil {
		t.Fatalf("owner execution error: %v result=%+v", err, result)
	}
	return result
}

// This uses static Parameters through the actual tool entrypoint, not the
// finalizer's richer dynamic schema. A rejected body must retain the existing
// transaction while its independent valid selection uses the original staging
// lane. A subsequent successful patch consumes that exact pending selection.
func TestB1681IndependentNestedBodyPreservesTransactionAndSelection(t *testing.T) {
	for _, patch := range []bool{false, true} {
		for _, function := range []bool{false, true} {
			for _, reverse := range []bool{false, true} {
				t.Run(fmt.Sprintf("patch=%t/function=%t/reverse=%t", patch, function, reverse), func(t *testing.T) {
					ctx := b1654Context()
					previous := ctx.Mutable.AnswerDocumentV2()
					ctx.Mutable.SetPendingAnswerDocumentPatchBase(previous)
					base := ctx.Mutable.PendingAnswerDocumentPatchBase()
					ctx.Mutable.SetAnswerDiagramRelationRepairLease(&types.AnswerDiagramRelationRepairLease{})
					lease := ctx.Mutable.AnswerDiagramRelationRepairLease()
					a := `{"id":"summary","kind":"summary","text":"First authored alternative."}`
					b := `{"id":"summary","kind":"summary","text":"Second authored alternative."}`
					if reverse {
						a, b = b, a
					}
					result := b1681IndependentEmit(t, ctx, patch, b1681IndependentEnvelope(a, b, function), b1654Report)
					if result.Success || !reflect.DeepEqual(previous, ctx.Mutable.AnswerDocumentV2()) || !reflect.DeepEqual(base, ctx.Mutable.PendingAnswerDocumentPatchBase()) || !reflect.DeepEqual(lease, ctx.Mutable.AnswerDiagramRelationRepairLease()) {
						t.Errorf("nested body selected an alternative or reset transaction state: %+v", result)
					}
					pending := ctx.Mutable.PendingTraceRootCauseReport()
					if pending == nil || len(pending.RootCauses) != 2 || pending.RootCauses[0].ThreadName != "WorkerThread" || pending.RootCauses[0].Description != "Worker remains independently selected." || ctx.Mutable.TraceRootCauseReport() != nil || ctx.Mutable.TraceRootCauseSelectorRejected() || len(result.OptionalCarrierOutcomes) != 0 {
						t.Fatalf("valid independent selector was lost/rejected/published instead of staged: pending=%+v result=%+v", pending, result)
					}
					accepted := b1654Execute(t, ctx, true, "", "")
					if !accepted.Success || !reflect.DeepEqual(pending, ctx.Mutable.TraceRootCauseReport()) || ctx.Mutable.PendingTraceRootCauseReport() != nil || len(accepted.OptionalCarrierOutcomes) != 0 {
						t.Fatalf("ordinary omitted-selector patch did not consume pending selection: %+v", accepted)
					}
				})
			}
		}
	}
}

// Optional-carrier errors have never owned the document transaction. Pin this
// for nested report/item wrappers, including competing explicit withdrawal.
func TestB1681IndependentNestedSelectionCannotOwnBodyOrWithdrawal(t *testing.T) {
	for _, patch := range []bool{false, true} {
		for _, shape := range []string{"report", "item"} {
			for _, reverse := range []bool{false, true} {
				t.Run(fmt.Sprintf("patch=%t/%s/reverse=%t", patch, shape, reverse), func(t *testing.T) {
					ctx := b1654Context()
					if result := b1654Execute(t, ctx, false, b1654Report, ""); !result.Success {
						t.Fatal(result.Summary)
					}
					previous := ctx.Mutable.TraceRootCauseReport()
					a, b := b1654Report, `{"schema_version":2,"root_causes":[]}`
					if shape == "item" {
						a, b = `{"candidate_id":"candidate-worker"}`, `{"candidate_id":"candidate-sched"}`
					}
					if reverse {
						a, b = b, a
					}
					selection := b1681IndependentEnvelope(a, b, false)
					if shape == "item" {
						selection = `{"schema_version":2,"root_causes":[` + selection + `]}`
					}
					body := `{"id":"summary","kind":"summary","text":"The useful body stays exactly model-authored."}`
					result := b1681IndependentEmit(t, ctx, patch, body, selection)
					if !result.Success || ctx.Mutable.AnswerDocumentV2().Blocks[0].Text != "The useful body stays exactly model-authored." || !ctx.Mutable.TraceRootCauseSelectorRejected() || ctx.Mutable.PendingTraceRootCauseReport() != nil || len(result.OptionalCarrierOutcomes) != 1 {
						t.Fatalf("optional nested selection lost the body or gained authority: %+v", result)
					}
					if patch && !reflect.DeepEqual(previous, ctx.Mutable.TraceRootCauseReport()) {
						t.Fatal("ambiguous withdrawal changed the previously accepted patch selection")
					}
					if !patch && ctx.Mutable.TraceRootCauseReport() != nil {
						t.Fatal("fresh full body inherited a rejected selection")
					}
					if explicit := b1654Execute(t, ctx, true, `{"schema_version":2,"root_causes":[]}`, ""); !explicit.Success || ctx.Mutable.TraceRootCauseReport() == nil || len(ctx.Mutable.TraceRootCauseReport().RootCauses) != 0 {
						t.Fatalf("an actual unambiguous empty report no longer withdraws: %+v", explicit)
					}
				})
			}
		}
	}
}

func TestB1681IndependentEquivalentNestedBodyKeepsOriginalContent(t *testing.T) {
	for _, patch := range []bool{false, true} {
		ctx := b1654Context()
		// The body string intentionally resembles JSON; integrity concerns only
		// the schema-consumed object, never a string containing arbitrary prose.
		block := `{"id":"summary","kind":"summary","text":"Example: arguments A and arguments B are two input names."}`
		body := block
		if !patch {
			// The static patch schema does not admit block wrapper repair; this
			// boundary test preserves that existing support limit.
			body = b1681IndependentEnvelope(block, block, false)
		}
		result := b1681IndependentEmit(t, ctx, patch, body, b1654Report)
		if !result.Success || ctx.Mutable.AnswerDocumentV2().Blocks[0].Text != "Example: arguments A and arguments B are two input names." || ctx.Mutable.TraceRootCauseReport() == nil || len(ctx.Mutable.TraceRootCauseReport().RootCauses) != 2 || len(result.OptionalCarrierOutcomes) != 0 {
			t.Fatalf("equivalent schema-owned body was needlessly rejected or rewritten: %+v", result)
		}
	}
}
