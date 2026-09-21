package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Current analysis is the authority shared by observation compilation and
// persisted answer rendering. A mutable historical request cannot re-elect a
// discovered worker; raw request/answer strings cannot grant that identity.
func TestTraceUserTargetAuthorityCurrentAnalysisMaterializer(t *testing.T) {
	for _, declaration := range []types.RuntimeTargetDeclaration{
		types.RuntimeTargetDeclarationNoNamedTarget,
		types.RuntimeTargetDeclarationUnspecified,
	} {
		for _, lang := range []string{"zh", "en"} {
			t.Run(string(declaration)+"/"+lang, func(t *testing.T) {
				ctx, ref := supplementBusinessFocusFixture(t, "OpenDocument")
				rm := &ctx.AnalysisIR.RequestModel
				rm.RuntimeTargetProfile = &types.RuntimeTargetProfile{Declaration: declaration}
				rm.AnalyzerHints.Entities = []string{"document-worker", "app-main", "1.000", "1.051"}
				rm.AnalyzerHints.ExactTargets = []string{"document-worker-200"}
				ctx.AnalysisIR.AnswerContract.Language = lang
				// Deliberately stale, contradictory history; no fresh emitter can
				// publish this combination as the current request.
				ctx.Mutable.SetRequestModel(types.RequestModel{
					RuntimeTargetProfile: &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: "worker"},
					RuntimeTargets:       []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 200, Thread: "document-worker", Source: "user_explicit"}},
				})
				for _, view := range []string{"window_stats", "wakeup_chain", "root_cause_rank", "critical_blocking_calls"} {
					got := businessRefTestQuery(t, ctx, map[string]any{"path": ref.Data().Path, "view": view, "pid": 100, "time_start": 1, "time_end": 1.051})
					if !got.Success {
						t.Fatal(got.Summary)
					}
				}
				input := types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit)
				before, err := json.Marshal(input.ToolResults)
				if err != nil {
					t.Fatal(err)
				}
				ledger := types.CompileObservationLedger(input)
				if len(ledger.AnchorUserEntities) != 0 {
					t.Errorf("historical/generic targets gained current user authority: %+v", ledger.AnchorUserEntities)
				}
				set := types.CompileTraceCausalProjectionSet(ledger)
				if len(set.Projections) != 1 {
					t.Fatalf("expected one projection: %+v", set)
				}
				p := set.Projections[0]
				want := []string{"storage-irq-80", "document-worker-200", "app-main-100"}
				if !reflect.DeepEqual(p.WakeupPath, want) || p.WakeupPathUserElected {
					t.Errorf("native path was re-elected: path=%v elected=%t", p.WakeupPath, p.WakeupPathUserElected)
				}
				text := "document-worker is mentioned here, not elected."
				doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: text}}}
				res, err := ApplyAndPersistMutation(ctx, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Unix(1753000000, 0).UTC())
				if err != nil || !res.Success {
					t.Fatalf("apply: %v %s", err, res.Summary)
				}
				final := render.RenderAnswerDocument(ctx.Mutable.AnswerDocumentV2(), lang)
				chip := "‹分析锚点线程›"
				forbidden := "‹用户关注线程›"
				if lang == "en" {
					chip, forbidden = "<analysis anchor thread>", "<user-focused thread>"
				}
				if !strings.Contains(final, "app-main-100 "+chip) || strings.Contains(final, forbidden) {
					t.Errorf("persisted projection minted user identity or lost analysis target:\n%s", final)
				}
				if !strings.Contains(final, text) {
					t.Error("model-owned summary was rewritten")
				}
				after, err := json.Marshal(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit).ToolResults)
				if err != nil {
					t.Fatal(err)
				}
				if string(before) != string(after) {
					t.Fatal("rendering mutated native evidence")
				}
			})
		}
	}
}

func TestTraceUserTargetAuthorityTypedIdentityCompileRenderAgree(t *testing.T) {
	for _, label := range []string{"worker-200 [200]", "worker [200]", "worker-200 (200)"} {
		t.Run(label, func(t *testing.T) {
			ctx := &types.BusContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
				RuntimeTargetProfile: &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: label},
				RuntimeTargets:       []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, Thread: label, Source: "user_explicit"}},
			}}}
			ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{"app-100"}
			ledger := types.CompileObservationLedger(types.ObservationLedgerInput{
				RequestModel: &ctx.AnalysisIR.RequestModel,
				ToolResults: []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: []types.ObservationRecord{
					anchorB1ToolPathRecord("p", "app-100", "irq-80 -> worker-200 -> app-100"),
				}}},
			})
			p := types.CompileTraceCausalProjection(ledger)
			if !p.WakeupPathUserElected || !reflect.DeepEqual(p.WakeupPath, []string{"irq-80", "worker-200"}) {
				t.Fatalf("existing typed identity lost compiler support: %+v", p.WakeupPath)
			}
			m := buildRuntimeTraceProjTreeModel(p, nil, true)
			runtimeTraceProjApplyUserFocus(&m, runtimeTraceProjUserFocusFromBusContext(ctx))
			if m.RootFocusAnchorOnly {
				t.Fatalf("same current typed identity elected in compiler but denied in renderer: %q", label)
			}
		})
	}
}
