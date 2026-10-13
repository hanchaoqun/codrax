package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The targetless rank query is the public route used by the business live:
// the native rank is real, but its two IO rows are supporting observations,
// and their numbers measure request residence, not issuer sleep.
func TestHMC232PublicDisplayPreservesBackgroundAndIORuler(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			path, err := filepath.Abs(filepath.Join("..", "..", "eval", "fixtures", "hmosperf_business_io_chain", "events.systrace"))
			if err != nil {
				t.Fatal(err)
			}
			ctx := &types.AgentContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: lang, AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
				Mutable: types.NewMutableState("Explain business response"), AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
					Language: lang, Intent: types.IntentRootCause, PerfTrace: &types.PerfBundle{},
					RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis},
				}}}
			raw, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "root_cause_rank", "time_start": .999, "time_end": 1.051})
			result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
			if err != nil || !result.Success {
				t.Fatalf("query premise: %v %+v", err, result)
			}
			ctx.Mutable.AppendDispatchToolResult(result)
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
			ledger := answerDocObservationLedger(ctx)
			set := types.CompileTraceCausalProjectionSet(ledger)
			before := b1645Snapshot(t, []any{ctx.Mutable.TurnAArtifacts(), ledger, set})
			seen := 0
			for _, r := range ledger.Records {
				if r.Predicate != "root_cause_unattributed" || r.Object != "io_latency" {
					continue
				}
				seen++
				if r.Role != types.AnswerAggregateRoleSupportingCoverage || traceQueryObservationSupplementNoteValue(r, types.TraceNoteKeyIOValueCaliber) != types.TraceIOValueCaliberRQResidence {
					t.Fatalf("native premise: %+v", r)
				}
				face := traceQueryObservationSupplementText(r, lang == "zh")
				want := types.TraceIOValueCaliberLabel(types.TraceIOValueCaliberRQResidence, lang == "zh")
				if !strings.Contains(face, want) || strings.Contains(face, "根因观测") || strings.Contains(face, "root-cause observation") {
					t.Errorf("public native supplement mislabels noncausal residence: %s", face)
				}
			}
			if seen != 2 {
				t.Fatalf("expected native background IO pair, got %d", seen)
			}
			for _, p := range set.Projections {
				for _, n := range p.BackgroundCauses {
					if n.TypeToken == "io_latency" && traceFinalReaderContextLabel(n, lang == "zh") != types.TraceIOValueCaliberLabel(n.IOValueCaliber, lang == "zh") {
						t.Errorf("public final-context dropped exact ruler: %s", traceFinalReaderContextLabel(n, lang == "zh"))
					}
				}
			}
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			if !strings.Contains(prompt, types.TraceIOValueCaliberLabel(types.TraceIOValueCaliberRQResidence, lang == "zh")) {
				t.Fatal("public finalizer context lost the native measurement vocabulary")
			}
			roster := renderAnswerDocTraceRankAuthority(ledger, lang)
			for _, line := range strings.Split(roster, "\n") {
				if (strings.Contains(line, "#1 backup-900") || strings.Contains(line, "#2 document-worker-200")) && !strings.Contains(line, types.TraceIOValueCaliberLabel(types.TraceIOValueCaliberRQResidence, lang == "zh")) {
					t.Errorf("rank roster dropped row-bound IO caliber: %s", line)
				}
			}
			raw = []byte(`{"document_model":"v2","blocks":[{"id":"summary","kind":"summary","text":"Model-owned wording: 8.765 ms."}]}`)
			emitted, err := (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), raw)
			if err != nil || !emitted.Success {
				t.Fatalf("emit premise: %v %+v", err, emitted)
			}
			doc := ctx.Mutable.AnswerDocumentV2()
			for _, id := range []string{"runtime_trace_causal_projection", "runtime_trace_causal_projection_detail_full"} {
				found := false
				for _, block := range doc.Blocks {
					if block.ID == id {
						found = strings.Contains(types.AnswerBlockVisibleSurface(block), types.TraceIOValueCaliberLabel(types.TraceIOValueCaliberRQResidence, lang == "zh"))
					}
				}
				if !found {
					t.Errorf("public tree/detail surface %s lost request ruler", id)
				}
			}
			face := render.RenderAnswerDocument(doc, lang)
			for _, privateToken := range []string{"io_value_caliber=", types.TraceIOValueCaliberRQResidence} {
				if strings.Contains(face, privateToken) {
					t.Errorf("wire vocabulary leaked to final display: %s", privateToken)
				}
			}
			if !strings.Contains(face, "backup") || !strings.Contains(face, "document-worker") {
				t.Fatalf("public rendering lost evidence: %s", face)
			}
			for _, line := range strings.Split(face, "\n") {
				if !strings.Contains(line, "因果位置：背景") && !strings.Contains(line, "causal position: background") {
					continue
				}
				if strings.Contains(line, "根因排序") || strings.Contains(line, "root-cause rank") || strings.Contains(line, "口径未明确") || strings.Contains(line, "measurement unspecified") {
					t.Errorf("public evidence index contradicts native background/ruler: %s", line)
				}
			}
			out, err := (&answerDocumentEvaluator{language: lang}).ParseOutput(ctx, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.FinalAnswer, "Model-owned wording: 8.765 ms.") {
				t.Fatal("model wording changed")
			}
			patch, err := (&tool.EmitAnswerDocumentPatch{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), json.RawMessage(`{"unchanged_block_ids":["summary"]}`))
			if err != nil || !patch.Success || b1645Snapshot(t, doc) != b1645Snapshot(t, ctx.Mutable.AnswerDocumentV2()) {
				t.Fatalf("no-op patch changed native display: %v %+v", err, patch)
			}
			after := b1645Snapshot(t, []any{ctx.Mutable.TurnAArtifacts(), answerDocObservationLedger(ctx), types.CompileTraceCausalProjectionSet(answerDocObservationLedger(ctx))})
			if before != after {
				t.Fatal("display mutated evidence, ranking or causal qualifications")
			}
		})
	}
}

// This pins the finalizer's observation supplement, which retains the native
// ledger. The existing causal-projection duplicate fold is a separate boundary
// and may already combine equal-valued IO rows before the tool renderer runs.
func TestHMC232PublicEqualValueDifferentIOCalibersRemainVisible(t *testing.T) {
	path := filepath.Join(t.TempDir(), "two-rulers.ftrace")
	body := "issuer-41 (41) [001] .... 5.000000: block_rq_issue: 8,0 R 4096 () 123 + 8 [issuer]\n" +
		"issuer-41 (41) [001] .... 5.000000: block_bio_queue: 8,0 R 456 + 8 [issuer]\n" +
		"irq-2 (2) [001] .... 5.010000: block_rq_complete: 8,0 R () 123 + 8 [0]\n" +
		"irq-2 (2) [001] .... 5.010000: block_bio_complete: 8,0 R 456 + 8 [0]\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := &types.AgentContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("report native IO observations"), Language: "en"}
	raw, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "critical_blocking_calls", "time_start": 4.999, "time_end": 5.011})
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
	if err != nil || !result.Success {
		t.Fatalf("query premise: %v %+v", err, result)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	ledger := answerDocObservationLedger(ctx)
	before := b1645Snapshot(t, ledger)
	calibers, keys := map[string]bool{}, map[string]bool{}
	for _, r := range ledger.Records {
		if r.Predicate != "critical_blocking" || traceQueryObservationSupplementNoteValue(r, types.TraceNoteKeyType) != "io_latency" {
			continue
		}
		calibers[traceQueryObservationSupplementNoteValue(r, types.TraceNoteKeyIOValueCaliber)] = true
		keys[traceQueryObservationSupplementKey(r)] = true
		if r.Value != "10.000" {
			t.Fatalf("native equal value premise: %+v", r)
		}
	}
	if len(calibers) != 2 || !calibers[types.TraceIOValueCaliberRQResidence] || !calibers[types.TraceIOValueCaliberBIOResidence] {
		t.Fatalf("native pair premise: %v", calibers)
	}
	if len(keys) != 2 {
		t.Error("native equal-value IO records collide in supplement display key")
	}
	raw = []byte(`{"document_model":"v2","blocks":[{"id":"summary","kind":"summary","text":"Observed request pairs."}]}`)
	emitted, err := (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), raw)
	if err != nil || !emitted.Success {
		t.Fatalf("emit premise: %v %+v", err, emitted)
	}
	for _, lang := range []string{"zh", "en"} {
		out, err := (&answerDocumentEvaluator{language: lang}).ParseOutput(ctx, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		for caliber := range calibers {
			if !strings.Contains(out.FinalAnswer, types.TraceIOValueCaliberLabel(caliber, lang == "zh")) {
				t.Errorf("public supplement lost independent IO ruler %s: %s", caliber, out.FinalAnswer)
			}
		}
	}
	if before != b1645Snapshot(t, answerDocObservationLedger(ctx)) {
		t.Fatal("display changed native records")
	}
}

func TestHMC232PublicClosedIssuerWaitKeepsCausalQualification(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, wake := range []bool{false, true} {
			ctx := b1645ActualCausalIOContext(t, "S", lang, wake)
			ledger := answerDocObservationLedger(ctx)
			set := types.CompileTraceCausalProjectionSet(ledger)
			before := b1645Snapshot(t, []any{ledger, set, types.BuildTraceBlockingWallClockAuthorities(ledger, &ctx.AnalysisIR.RequestModel)})
			seen, principal := 0, 0
			for _, r := range ledger.Records {
				if !strings.HasPrefix(r.Predicate, "root_cause_") || r.Object != "io_latency" {
					continue
				}
				seen++
				caliber := traceQueryObservationSupplementNoteValue(r, types.TraceNoteKeyIOValueCaliber)
				label := types.TraceIOValueCaliberLabel(caliber, lang == "zh")
				face := traceQueryObservationSupplementText(r, lang == "zh")
				if !strings.Contains(face, label) {
					t.Errorf("native wait/residence lost its ruler: %s", face)
				}
				if r.Role == types.AnswerAggregateRolePrincipalAnswer {
					principal++
					if !wake || caliber != types.TraceIOValueCaliberIssuerBlocked || r.Value != "0.090" {
						t.Fatalf("native positive premise: %+v", r)
					}
					if !strings.Contains(face, "根因观测") && !strings.Contains(face, "root-cause observation") {
						t.Errorf("qualified cause label disappeared: %s", face)
					}
				} else if strings.Contains(face, "根因观测") || strings.Contains(face, "root-cause observation") {
					t.Errorf("non-principal source acquired a cause label: %s", face)
				}
			}
			if seen == 0 || wake && principal == 0 || !wake && principal != 0 {
				t.Fatalf("native qualification premise: wake=%v seen=%d principal=%d", wake, seen, principal)
			}
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			if wake && !strings.Contains(prompt, types.TraceIOValueCaliberLabel(types.TraceIOValueCaliberIssuerBlocked, lang == "zh")) {
				t.Fatal("closed issuer ruler lost from public finalizer")
			}
			afterLedger := answerDocObservationLedger(ctx)
			after := b1645Snapshot(t, []any{afterLedger, types.CompileTraceCausalProjectionSet(afterLedger), types.BuildTraceBlockingWallClockAuthorities(afterLedger, &ctx.AnalysisIR.RequestModel)})
			if before != after {
				t.Fatal("display altered causal or blocking authority")
			}
		}
	}
}
