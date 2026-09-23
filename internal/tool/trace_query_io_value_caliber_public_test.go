package tool

import (
	"encoding/json"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the real parser, native query, public payload, ledger publication and
// projection together. The 35ms request contains a 31ms S-state wait; a separate
// 47ms request is background evidence, not a wait or an admitted root cause.
func TestTraceQueryIOValueCaliberPublicNativeCarriage(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range []string{"root_cause_rank", "critical_blocking_calls"} {
		t.Run(view, func(t *testing.T) {
			ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("IO measurement")}
			result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": view, "pid": 100,
				"time_start": .999, "time_end": 1.051})
			payload := businessSpanSchedulerPublicPayload(t, result)
			if view == "root_cause_rank" && payload.WindowStats == nil {
				t.Fatal("native query lost window facts")
			}
			if payload.WindowStats != nil {
				requestFound := false
				for _, io := range payload.WindowStats.IOLatencies {
					if math.Abs(io.DurationMs-35) < 1e-6 && io.CompletionWokeIssuer && math.Abs(io.IssuerBlockedMs-31) < 1e-6 {
						requestFound = true
					}
				}
				if !requestFound {
					t.Fatalf("native request/wait premise missing: %+v", payload.WindowStats.IOLatencies)
				}
			}
			for _, want := range []struct{ subject, caliber, value string }{
				{"document-worker-200", types.TraceIOValueCaliberIssuerBlocked, "31.000"},
				{"backup-900", types.TraceIOValueCaliberRQResidence, "47.000"},
			} {
				found := false
				for _, record := range result.Observations {
					if record.Subject != want.subject || (!strings.HasPrefix(record.Predicate, "root_cause_") && record.Predicate != "critical_blocking") ||
						!strings.Contains(strings.Join(record.RichNotes, "\n"), "type=io_latency") {
						continue
					}
					found = true
					if record.Value != want.value || !strings.Contains(strings.Join(record.RichNotes, "\n"), "io_value_caliber="+want.caliber) {
						t.Errorf("native %s value=%s notes=%v; want %s / %s", record.Subject, record.Value, record.RichNotes, want.value, want.caliber)
					}
				}
				if !found {
					t.Errorf("missing native %s IO observation", want.subject)
				}
			}
			projection := types.TraceCausalProjectionFromObservationRecords(result.Observations)
			nodes := append(append(append([]types.TraceCausalProjectionNode{}, projection.RankedSeats...), projection.OnChainCauses...), projection.BackgroundCauses...)
			for _, want := range []struct{ subject, caliber string }{
				{"document-worker-200", types.TraceIOValueCaliberIssuerBlocked}, {"backup-900", types.TraceIOValueCaliberRQResidence},
			} {
				found := false
				for _, node := range nodes {
					if node.Subject != want.subject || node.TypeToken != "io_latency" {
						continue
					}
					found = true
					if node.IOValueCaliber != want.caliber {
						t.Errorf("projection %s caliber=%q want %q", node.Subject, node.IOValueCaliber, want.caliber)
					}
					if want.subject == "backup-900" && node.ChainRelevance == "on_chain" {
						t.Errorf("background request became a chain cause: %+v", node)
					}
				}
				if !found {
					t.Errorf("missing projection node for native %s IO", want.subject)
				}
			}
		})
	}
}

func TestTraceQueryIOValueCaliberNativeToAnswerAndSidecar(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			ctx := &types.BusContext{Language: lang, RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("IO measurement")}
			result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "root_cause_rank", "pid": 100,
				"time_start": .999, "time_end": 1.051})
			if !result.Success {
				t.Fatal(result.Summary)
			}
			ctx.ToolResults = []types.ToolResult{result}
			ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck}, AnswerContract: types.AnswerContract{Language: lang}}
			ledger := types.ObservationLedger{Records: result.Observations}
			contract, err := tracefinding.CompileCandidateContract(ledger, types.CompileTraceCausalProjectionSet(ledger), tracefinding.SeatFrameCausalityAuthority{})
			if err != nil {
				t.Fatal(err)
			}
			selected := ""
			for _, candidate := range contract.Candidates {
				if candidate.Decision.Token.Token != "io_latency" {
					continue
				}
				value := candidate.Decision.Magnitude.Value
				if candidate.PrimaryEligible && math.Abs(value-31) < 1e-6 {
					selected = candidate.Decision.CandidateID
				} else if candidate.PrimaryEligible {
					t.Fatalf("background request admitted: %+v", candidate)
				}
			}
			if selected == "" {
				t.Fatal("native chain wait lost its eligibility")
			}
			contract.RootCauseReportEnabled = true
			ctx.Mutable.SetTraceFindingContract(contract)
			const prose = "Model-owned business explanation remains unchanged."
			raw, _ := json.Marshal(map[string]any{
				"blocks":            []map[string]any{{"id": "model_summary", "kind": "summary", "surface_role": "principal", "trace_causal_claim_caliber": "no_causal_conclusion", "text": prose}},
				"trace_root_causes": map[string]any{"schema_version": 2, "root_causes": []map[string]any{{"candidate_id": selected, "description": prose}}},
			})
			emitted, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
			if err != nil || !emitted.Success {
				t.Fatalf("public emit: %v %s", err, emitted.Summary)
			}
			doc, report := ctx.Mutable.AnswerDocumentV2(), ctx.Mutable.TraceRootCauseReport()
			text := render.RenderAnswerDocument(doc, lang)
			for _, want := range []string{"31.000", "47.000", "document-worker-200", "backup-900",
				types.TraceIOValueCaliberLabel(types.TraceIOValueCaliberIssuerBlocked, lang == "zh"),
				types.TraceIOValueCaliberLabel(types.TraceIOValueCaliberRQResidence, lang == "zh")} {
				if !strings.Contains(text, want) {
					t.Errorf("native answer lost %q", want)
				}
			}
			// The existing sidecar binder uses its fixed Chinese evidence face;
			// the answer renderer is localized independently. Both use one ruler.
			if report == nil || len(report.RootCauses) != 1 || report.RootCauses[0].ImpactSeconds == nil ||
				math.Abs(*report.RootCauses[0].ImpactSeconds-.031) > 1e-9 || report.RootCauses[0].Description != prose ||
				!strings.Contains(strings.Join(report.RootCauses[0].Evidence, " "), types.TraceIOValueCaliberLabel(types.TraceIOValueCaliberIssuerBlocked, true)) {
				t.Fatalf("native sidecar lost value/caliber/model ownership: %+v", report)
			}
			emitted, err = (&EmitAnswerDocumentPatch{}).Execute(ctx, json.RawMessage(`{"unchanged_block_ids":["model_summary"]}`))
			if err != nil || !emitted.Success || !reflect.DeepEqual(doc, ctx.Mutable.AnswerDocumentV2()) || !reflect.DeepEqual(report, ctx.Mutable.TraceRootCauseReport()) {
				t.Fatalf("no-op patch changed native publication: %v %s", err, emitted.Summary)
			}
		})
	}
}
