package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The public query is the only producer. Reusing a displayed context row must
// neither remove its full canonical fields nor coalesce source/query evidence.
func TestHMC355RuntimeContextFactsPublishedOnce(t *testing.T) {
	for _, domain := range []string{"io", "scheduler"} {
		for _, lang := range []string{"zh", "en"} {
			for _, budgeted := range []bool{false, true} {
				name := domain + "/" + lang
				if budgeted {
					name += "/budgeted"
				}
				t.Run(name, func(t *testing.T) {
					var ctx *types.AgentContext
					var result types.ToolResult
					if domain == "scheduler" {
						ctx, result, _ = schedulerConcurrencyContext(t, lang, types.RuntimeQuestionScopeBoundedFactSet)
					} else {
						ctx, result, _ = ioInFlightPublicContext(t, lang, types.RuntimeQuestionScopeBoundedFactSet, "")
					}
					ctx.AnalysisIR.RequestModel.RuntimeTargets = nil
					ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.FactFamilies = []types.RuntimeQuestionFactFamily{
						types.RuntimeQuestionFactCountOrDuration, types.RuntimeQuestionFactResourcePressure,
					}
					ledger := answerDocObservationLedger(ctx)
					before, _ := json.Marshal([]any{result, ledger, ctx.AnalysisIR.RequestModel, types.CompileTraceCausalProjection(ledger)})
					e := &answerDocumentEvaluator{}
					var prompt string
					if budgeted {
						prompt = e.BuildInitialInstructionWithBudget(ctx, nil, func() *finalizerPromptBudget {
							return &finalizerPromptBudget{RequestAllocationBytes: 700000}
						})
					} else {
						prompt = e.BuildInitialInstruction(ctx, nil)
					}
					t.Logf("actual_finalizer_prompt_bytes=%d", len(prompt))
					checked := 0
					for _, row := range ledger.Records {
						wanted := domain == "io" && (row.Predicate == "io_inflight" || row.Predicate == "io_inflight_coverage") ||
							domain == "scheduler" && answerDocSchedulerConcurrencyPredicate(row.Predicate)
						if !wanted {
							continue
						}
						checked++
						body := answerDocBoundedRuntimeFactAuthorityRow(row, &ctx.AnalysisIR.RequestModel, lang)
						if count := strings.Count(prompt, body); count != 1 {
							t.Errorf("complete source-bound context row %s rendered %d times, want once", row.ID, count)
						}
						for _, field := range []string{row.SourceRef.Path, row.SourceRef.PayloadRef, row.Summary} {
							if !strings.Contains(prompt, field) {
								t.Errorf("canonical context row lost %q", field)
							}
						}
					}
					if checked == 0 {
						t.Fatal("actual query published no context rows")
					}
					afterLedger := answerDocObservationLedger(ctx)
					after, _ := json.Marshal([]any{result, afterLedger, ctx.AnalysisIR.RequestModel, types.CompileTraceCausalProjection(afterLedger)})
					if string(before) != string(after) {
						t.Fatal("display reuse changed facts, task scope or causal projection")
					}
				})
			}
		}
	}
}

func TestHMC355ContextRowReferencesKeepExactIdentityAndFields(t *testing.T) {
	ctx, _, _ := ioInFlightPublicContext(t, "en", types.RuntimeQuestionScopeBoundedFactSet, "")
	var row types.ObservationRecord
	for _, r := range answerDocObservationLedger(ctx).Records {
		if r.Predicate == "io_inflight" {
			row = r
			break
		}
	}
	if row.ID == "" {
		t.Fatal("actual public IO group missing")
	}
	rm := &ctx.AnalysisIR.RequestModel
	for _, name := range []string{"source", "query", "payload", "summary", "notes", "source_metadata", "owner", "value", "unknown_window", "line_selection", "soft", "producer", "origin"} {
		t.Run(name, func(t *testing.T) {
			other := row
			switch name {
			case "source":
				other.SourceRef.Path += ".other"
			case "query":
				other.SourceRef.QueryScopeID += ".other"
			case "payload":
				other.SourceRef.PayloadRef += ".other"
			case "summary":
				other.Summary += " independent boundary"
			case "notes":
				other.RichNotes = append(append([]string(nil), row.RichNotes...), "independent_exact_boundary")
			case "source_metadata":
				other.SourceRef.CaptureIdentityPath += ".other"
			case "owner":
				other.Subject += ".other"
			case "value":
				other.Value += "0"
			case "unknown_window":
				other.SourceRef.QueryWindowKnown = false
			case "line_selection":
				other.SourceRef.QueryLineRangeKnown, other.SourceRef.QueryLineStart, other.SourceRef.QueryLineEnd = true, 1, 10
			case "soft":
				other.GroundingPolicy = ""
			case "producer":
				other.Producer = "model"
			case "origin":
				other.Origin = types.AnswerEvidenceOriginCurrentSource
			}
			var rows runtimeContextRowPresentation
			var out strings.Builder
			rows.write(&out, row, rm, "en", true)
			rows.write(&out, other, rm, "en", false)
			if strings.Contains(out.String(), "context_row_ref=") || !strings.Contains(out.String(), answerDocBoundedRuntimeFactAuthorityRow(other, rm, "en")) {
				t.Fatal("different identity, fields or unknown scope borrowed a context row")
			}
		})
	}
	var rows runtimeContextRowPresentation
	var out strings.Builder
	rows.write(&out, row, rm, "en", true)
	definitionEnd := out.Len()
	rows.write(&out, row, rm, "en", false)
	if !strings.Contains(out.String()[:definitionEnd], "context_row_id=\"context-row-1\"") || !strings.Contains(out.String()[definitionEnd:], "context_row_ref=\"context-row-1\"") {
		t.Fatal("reference did not follow its full definition")
	}
	var later runtimeContextRowPresentation
	var next strings.Builder
	later.write(&next, row, rm, "en", false)
	if strings.Contains(next.String(), "context_row_ref=") || !strings.Contains(next.String(), answerDocBoundedRuntimeFactAuthorityRow(row, rm, "en")) {
		t.Fatal("a later render reused a prior dispatch's definition")
	}
}

func TestHMC355ContextReuseReducesRepeatedAccountBytes(t *testing.T) {
	ctx, _, _ := ioInFlightPublicContext(t, "en", types.RuntimeQuestionScopeBoundedFactSet, "")
	ctx.AnalysisIR.RequestModel.RuntimeTargets = nil
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.FactFamilies = []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactResourcePressure, types.RuntimeQuestionFactCountOrDuration}
	ledger := answerDocObservationLedger(ctx)
	rm := &ctx.AnalysisIR.RequestModel
	legacy := renderAnswerDocIOInFlightMeasurements(ledger, rm, "en") + renderAnswerDocBoundedRuntimeFactAuthority(ctx, ledger)
	var rows runtimeContextRowPresentation
	shared := renderAnswerDocIOInFlightMeasurementsWithRows(ledger, rm, "en", &rows) + renderAnswerDocBoundedRuntimeFactAuthorityWithRows(ctx, ledger, &rows)
	if !strings.Contains(shared, "context_row_ref=") || len(shared) >= len(legacy) {
		t.Fatalf("exact account sharing did not save bytes: legacy=%d shared=%d", len(legacy), len(shared))
	}
	t.Logf("same_query_account_bytes legacy=%d shared=%d; only renderer-owned context sections, not total tokens", len(legacy), len(shared))
}
