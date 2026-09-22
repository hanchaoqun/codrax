package tool

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/preview"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/mattn/go-runewidth"
)

// These tests start with physical trace bytes and the public native tool,
// never a manufactured rank/causal projection. The same observations feed
// publication, the full/reader legends and an unchanged-block patch.
func optimizationCaliberPublicQuery(t *testing.T, trace string, pid int, start, end float64) (*types.BusContext, string) {
	t.Helper()
	ctx, path := businessRefTestContext(t, trace)
	for _, view := range []string{"window_stats", "wakeup_chain", "root_cause_rank"} {
		result := businessRefTestQuery(t, ctx, map[string]any{
			"source": "path", "path": path, "view": view, "pid": pid,
			"time_start": start, "time_end": end, "max_depth": 8, "limit": 32,
			"trace_flavor": "harmony_hitrace",
		})
		if !result.Success || len(result.Observations) == 0 {
			t.Fatalf("native %s query failed or published no evidence: %s", view, result.Summary)
		}
	}
	return ctx, path
}

func optimizationCaliberPublicCaseTrace(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../../eval/cases/trace_query_wakeup_causal_io_chain.case")
	if err != nil {
		t.Fatal(err)
	}
	_, rest, ok := strings.Cut(string(data), "HTRACE='")
	trace, _, closed := strings.Cut(rest, "\n'\n")
	if !ok || !closed || !strings.Contains(trace, "fscache_page_wait_on_page_bit") {
		t.Fatal("causal IO eval must provide its native trace, not generated expected records")
	}
	return trace + "\n"
}

func optimizationCaliberPublicRequest(ctx *types.BusContext, path, lang, subject string, pid int, start, end float64, finite bool) {
	quote := fmt.Sprintf("%.6f..%.6f", start, end)
	rm := types.RequestModel{
		RawRequest: "Inspect " + subject + " in " + quote, Language: lang,
		Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck,
		RuntimeTargets:       []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: pid, Thread: subject, Source: "user_explicit", Confidence: 1}},
		RuntimeTargetProfile: &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: subject, Confidence: 1},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
			TimeStart: &start, TimeEnd: &end, SourceQuote: quote, Confidence: 1},
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis, SourceQuote: "Inspect", Confidence: 1},
	}
	if finite {
		rm.RuntimeQuestionProfile.Scope = types.RuntimeQuestionScopeBoundedFactSet
		rm.RuntimeQuestionProfile.FactFamilies = []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetSchedulerState, types.RuntimeQuestionFactTargetWaitOccurrences}
	}
	ctx.Language = lang
	ctx.Mutable.SetRequestModel(rm)
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: lang}}
	ctx.RuntimeArtifactPreflight = types.RuntimeArtifactPreflightProfile{Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: path, Carrier: "path"}}}
}

func optimizationCaliberPublicSnapshot(t *testing.T, ctx *types.BusContext) (string, types.TraceCausalProjectionSet) {
	t.Helper()
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
	set := types.CompileTraceCausalProjectionSet(ledger)
	var directions [][]types.TraceAnswerDirectionSection
	for _, p := range set.Projections {
		directions = append(directions, TraceAnswerDecisionDirectionSections(p))
	}
	data, err := json.Marshal([]any{ledger, set, directions, ctx.AnalysisIR, ctx.Mutable.RequestModel()})
	if err != nil {
		t.Fatal(err)
	}
	return string(data), set
}

// The deliberately old wording is model-owned quoted prose. Calibration is
// a typed system-carrier change, not permission to scan/rewrite the model.
const optimizationCaliberModelText = "业务结论保持。 Quoted model text: 11ms 可消; fixing one shrinks the other seat's headroom."

func optimizationCaliberPublicPublish(t *testing.T, ctx *types.BusContext, path, lang string, finite bool) (string, types.TraceCausalProjectionSet) {
	t.Helper()
	before, set := optimizationCaliberPublicSnapshot(t, ctx)
	rawBefore, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{"blocks": []map[string]any{{"id": "model_summary", "kind": "summary", "surface_role": "principal", "trace_causal_claim_caliber": "no_causal_conclusion", "text": optimizationCaliberModelText}}})
	emit, err := (&EmitAnswerDocument{}).Execute(ctx, payload)
	if err != nil || !emit.Success {
		t.Fatalf("public emit failed: %v; %s", err, emit.Summary)
	}
	check := func() (string, string) {
		t.Helper()
		doc := ctx.Mutable.AnswerDocumentV2()
		count := 0
		system := *doc
		system.Blocks = nil
		for _, block := range doc.Blocks {
			if block.ID == "model_summary" {
				count++
				if block.Text != optimizationCaliberModelText {
					t.Error("calibration rewrote model-owned prose")
				}
			}
			if RuntimeTraceSystemBlock(block) {
				system.Blocks = append(system.Blocks, block)
			}
		}
		if count != 1 {
			t.Errorf("model block was removed or duplicated: %d", count)
		}
		published := render.RenderAnswerDocument(doc, lang)
		if !strings.Contains(published, optimizationCaliberModelText) {
			t.Error("renderer lost the model's exact bytes")
		}
		owned := render.RenderAnswerDocument(&system, lang)
		if finite && (strings.Contains(owned, tracefence.Opener) || strings.Contains(owned, tracefence.ElimOpener)) {
			t.Error("finite fact request acquired a causal/potential board from available query evidence")
		}
		inside := false
		for _, line := range strings.Split(owned, "\n") {
			if line == tracefence.ElimOpener {
				inside = true
			} else if strings.HasPrefix(line, "```") {
				inside = false
			} else if inside && (strings.HasPrefix(line, tracefence.ElimGlyph+" ") || strings.HasPrefix(line, tracefence.ElimSectionGlyph+" ") || strings.HasPrefix(line, "◇ ")) && runewidth.StringWidth(line) > runtimeTraceProjTreeRowMaxWidth {
				t.Errorf("calibration expanded a structural overview line beyond %d cells: %q", runtimeTraceProjTreeRowMaxWidth, line)
			}
		}
		return published, owned
	}
	published, system := check()
	docBefore, _ := json.Marshal(ctx.Mutable.AnswerDocumentV2())
	patch, err := (&EmitAnswerDocumentPatch{}).Execute(ctx, json.RawMessage(`{"unchanged_block_ids":["model_summary"]}`))
	if err != nil || !patch.Success {
		t.Fatalf("public no-op patch failed: %v; %s", err, patch.Summary)
	}
	afterPublished, afterSystem := check()
	docAfter, _ := json.Marshal(ctx.Mutable.AnswerDocumentV2())
	if published != afterPublished || system != afterSystem || string(docBefore) != string(docAfter) {
		t.Error("no-op patch changed or duplicated the published answer")
	}
	after, _ := optimizationCaliberPublicSnapshot(t, ctx)
	if after != before {
		t.Error("publication changed native evidence, projected values/order/credentials, direction arithmetic, request or window")
	}
	rawAfter, err := os.ReadFile(path)
	if err != nil || string(rawBefore) != string(rawAfter) {
		t.Error("publication modified the physical trace")
	}
	return system, set
}

func optimizationCaliberAssertWords(t *testing.T, face, text, lang string, overview bool) {
	t.Helper()
	// These checks concern only system-owned carriers, never model prose.
	for _, old := range []string{
		"已证链上单项最大可消除量", "已证链上候选中单项最大可消除量",
		"因果候选成立时至多好这么多", "at most this much if the causal candidate holds",
		"largest single proven on-chain eliminable contribution", "largest single PROVEN on-chain eliminable contribution",
		"真实可消除量只多不少", "the truly removable amount can only be larger",
		"最大可消", "max eliminable", "ms 可消)", "ms eliminable)", "规则可消除", "Rule-eliminable",
		"修其一后另一席空间会缩", "fixing one shrinks the other seat's headroom",
		"修其一,另一席收益随之收缩", "fix one and the other seat's gain shrinks",
	} {
		if strings.Contains(text, old) {
			t.Errorf("%s promises unverified realized benefit via %q", face, old)
		}
	}
	want := []string{"modeled potential", "on-chain evidence does not establish realized benefit", "actual benefit requires post-change measurement"}
	if lang == "zh" {
		want = []string{"估算优化潜力", "按既定规则估算", "链上依据不等于收益已验证", "实际收益需实施后复测"}
	}
	if overview {
		label := "Modeled-potential overview"
		if lang == "zh" {
			label = "窗内估算优化潜力总览"
		}
		want = append(want, label)
	}
	for _, needle := range want {
		if !strings.Contains(text, needle) {
			t.Errorf("%s lacks the calibrated meaning %q", face, needle)
		}
	}
}

func TestOptimizationCaliberPublicIOQueryEmitRenderPatch(t *testing.T) {
	trace := optimizationCaliberPublicCaseTrace(t)
	for _, lang := range []string{"zh", "en"} {
		for _, finite := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/finite=%t", lang, finite), func(t *testing.T) {
				ctx, path := optimizationCaliberPublicQuery(t, trace, 100, 2, 2.020)
				optimizationCaliberPublicRequest(ctx, path, lang, "app-100", 100, 2, 2.020, finite)
				system, set := optimizationCaliberPublicPublish(t, ctx, path, lang, finite)
				if len(set.Projections) != 1 {
					t.Fatalf("native IO query must retain one projection: %d", len(set.Projections))
				}
				p := set.Projections[0]
				if p.WindowStartTs != 2 || p.WindowEndTs != 2.020 || !reflect.DeepEqual(p.WakeupPath, []string{"threadpool-400", "network-300", "cookie-200", "app-100"}) {
					t.Errorf("20ms native window/four-node path changed: %.6f..%.6f %v", p.WindowStartTs, p.WindowEndTs, p.WakeupPath)
				}
				found := map[string]bool{}
				for _, n := range p.RankedSeats {
					key := n.Subject + "/" + n.Object
					t.Logf("native primary %s rank=%d value=%.3f", key, n.Rank, n.EffectiveImpactMS)
					if n.Subject == "threadpool-400" && n.Object == "io_wait" && n.Rank == 1 && math.Abs(n.EffectiveImpactMS-11) < 1e-6 {
						found["io"] = true
					}
					if n.Object == "priority_inversion_candidate" && math.Abs(n.EffectiveImpactMS-1) < 1e-6 && math.Abs(n.GatedRunnableMS-1) < 1e-6 && n.GatedRunningDeficitMS == 0 {
						found[n.Subject] = true
					}
				}
				for _, key := range []string{"io", "threadpool-400", "network-300", "cookie-200"} {
					if !found[key] {
						t.Errorf("native 11ms IO + three 1ms seats must survive; missing %s", key)
					}
				}
				var order []string
				for _, n := range p.RankedSeats {
					order = append(order, fmt.Sprintf("%d:%s/%s=%.3f", n.Rank, n.Subject, n.Object, n.EffectiveImpactMS))
				}
				wantOrder := []string{"1:threadpool-400/io_wait=11.000", "2:cookie-200/priority_inversion_candidate=1.000", "3:threadpool-400/priority_inversion_candidate=1.000", "4:network-300/priority_inversion_candidate=1.000"}
				if !reflect.DeepEqual(order, wantOrder) {
					t.Errorf("existing rank election/tie order changed: %v", order)
				}
				if finite {
					return
				}
				optimizationCaliberAssertWords(t, "published system appendix", system, lang, true)
				html, err := preview.RenderMarkdownHTML([]byte(system))
				if err != nil || !strings.Contains(html, `class="trace-projection-tree trace-elim-overview"`) {
					t.Errorf("typed overview must still render as the same preview surface: %v", err)
				}
				if strings.Contains(html, `aria-label="Trace eliminable-in-window overview"`) || !strings.Contains(html, `aria-label="Modeled-potential overview"`) {
					t.Error("preview accessible label must share the calibrated overview meaning")
				}
				for _, value := range []string{"11.000ms", "1.000ms", "threadpool-400", "network-300", "cookie-200", "app-100"} {
					if !strings.Contains(system, value) {
						t.Errorf("publication lost measured value/chain identity %q", value)
					}
				}
				crown := "Primary root cause"
				if lang == "zh" {
					crown = "主根因"
				}
				if !strings.Contains(system, crown) {
					t.Error("unverified benefit must not decrown the existing elected cause")
				}
				model := buildRuntimeTraceProjTreeModel(p, newRuntimeTraceCausalProjectionEvidenceIndex(), lang == "zh")
				_ = runtimeTraceProjTreeFence(model, lang == "zh")
				_ = runtimeTraceProjElimOverviewFence(p, model, lang == "zh")
				full := strings.Join(runtimeTraceProjLegendGroupLines(model.Marks, lang == "zh"), "\n")
				reader := strings.Join(runtimeTraceProjReaderLegendLines(model.Marks, lang == "zh", false), "\n")
				optimizationCaliberAssertWords(t, "full legend from native projection", full, lang, false)
				optimizationCaliberAssertWords(t, "reader legend from native projection", reader, lang, false)
			})
		}
	}
}

func TestOptimizationCaliberPublicToolDescriptionAndParameters(t *testing.T) {
	tool := &TraceQuery{}
	for name, face := range map[string]string{"description": tool.Description(), "parameters": string(tool.Parameters())} {
		t.Run(name, func(t *testing.T) {
			optimizationCaliberAssertWords(t, name, face, "en", false)
			for _, required := range []string{"DEFINED term of art", "never a mechanism-level verdict", "missing or zero deficit is context_only", "D-state and IO use a mutually exclusive typed sum", "edge=credential, pre-edge=effective, post-edge=released", "off-chain semantic work is background-only", "span_subcategory=shader_cache_hit"} {
				if !strings.Contains(face, required) {
					t.Errorf("calibration must retain the closed typed matrix rule %q", required)
				}
			}
		})
	}
}

// CPU7 has a known 800MHz state against the 2.4GHz global basis. CPU8 is
// actually missing frequency evidence: its late sample is outside the query
// window and may not be carried backwards. VerifyClass crosses the host's
// wakeup edge, so only its pre-edge share participates. An unrelated shader
// span is longer but has no path credential and must remain background.
const optimizationCaliberMixedNativeTrace = `# tracer: nop
idle-0 (0) [007] .... 4.900000: cpu_frequency: state=800000 cpu_id=7
idle-0 (0) [001] .... 4.900000: cpu_frequency: state=2400000 cpu_id=1
idle-0 (0) [001] .... 4.990000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
idle-0 (0) [007] .... 5.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=120
background-900 (900) [003] .... 5.000100: print: B|900|shader compile unrelated
dep-200 (100) [007] .... 5.002000: print: B|100|VerifyClass Example
dep-200 (100) [007] .... 5.010000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=120 prev_state=R ==> next_comm=idle next_pid=0 next_prio=120
idle-0 (0) [008] .... 5.010000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=120
dep-200 (100) [008] .... 5.019000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
dep-200 (100) [008] .... 5.019500: print: E|100
dep-200 (100) [008] .... 5.019600: sched_switch: prev_comm=dep prev_pid=200 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
background-900 (900) [003] .... 5.019900: print: E|900
idle-0 (0) [001] .... 5.020000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
idle-0 (0) [008] .... 5.030000: cpu_frequency: state=800000 cpu_id=8
`

func TestOptimizationCaliberPublicSemanticAndMissingFrequency(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			ctx, path := optimizationCaliberPublicQuery(t, optimizationCaliberMixedNativeTrace, 100, 5, 5.020)
			optimizationCaliberPublicRequest(ctx, path, lang, "app-100", 100, 5, 5.020, false)
			system, set := optimizationCaliberPublicPublish(t, ctx, path, lang, false)
			if len(set.Projections) != 1 {
				t.Fatalf("mixed native query must publish one projection, got %d", len(set.Projections))
			}
			p := set.Projections[0]
			if p.WindowStartTs != 5 || p.WindowEndTs != 5.020 {
				t.Error("mixed native query lost its explicit 20ms window")
			}
			semantic, fold := false, false
			for _, n := range append(append([]types.TraceCausalProjectionNode(nil), p.PrimaryRootCauses...), p.OnChainCauses...) {
				t.Logf("mixed chain %s/%s rank=%d effective=%.3f semantic=%.3f edge=%.6f fold=%.3f known=%.3f unknown=%.3f", n.Subject, n.Object, n.Rank, n.EffectiveImpactMS, n.SemanticChainProjectedMS, n.HostWakeupEdgeAnchorTS, n.SupplyFoldDeficitMS, n.SupplyFoldKnownMS, n.SupplyFoldUnknownMS)
				if n.Subject == "background-900" {
					t.Error("off-chain semantic work acquired root-cause authority")
				}
				if n.Subject == "dep-200" && n.SemanticClass == "class_verification" {
					semantic = true
					if math.Abs(n.EffectiveImpactMS-17) > 1e-6 || n.OnChainBasis != types.TraceCausalOnChainBasisSemanticChainIntervalRelation {
						t.Errorf("17ms pre-edge interval intersection or credential changed: value=%.3f basis=%s", n.EffectiveImpactMS, n.OnChainBasis)
					}
				}
				if n.SupplyFoldComputed && n.SupplyFoldKnownMS > 0 && n.SupplyFoldUnknownMS > 0 {
					fold = true
					if math.Abs(n.SupplyFoldDeficitMS-6.667) > .001 || math.Abs(n.SupplyFoldKnownMS-10) > .001 || n.SupplyFoldUnknownMS < 9 {
						t.Errorf("known-frequency deficit must remain 6.667ms, unknown slices price zero: deficit=%.3f known=%.3f unknown=%.3f", n.SupplyFoldDeficitMS, n.SupplyFoldKnownMS, n.SupplyFoldUnknownMS)
					}
				}
			}
			if !semantic || !fold {
				t.Errorf("native fixture failed to exercise pre-edge semantic/frequency-unknown authority: %t/%t", semantic, fold)
			}
			background := false
			for _, n := range append(append([]types.TraceCausalProjectionNode(nil), p.BackgroundCauses...), p.SemanticSpans...) {
				if n.Subject == "background-900" && n.ChainRelevance != "on_chain" && n.SemanticClass != "" {
					background = true
				}
			}
			if !background {
				t.Error("off-chain semantic negative control was not actually observed")
			}
			optimizationCaliberAssertWords(t, "mixed published appendix", system, lang, true)
			model := buildRuntimeTraceProjTreeModel(p, newRuntimeTraceCausalProjectionEvidenceIndex(), lang == "zh")
			_ = runtimeTraceProjTreeFence(model, lang == "zh")
			_ = runtimeTraceProjElimOverviewFence(p, model, lang == "zh")
			full := strings.Join(runtimeTraceProjLegendGroupLines(model.Marks, lang == "zh"), "\n")
			optimizationCaliberAssertWords(t, "mixed full legend", full, lang, false)
			bound := "a lower bound within the stated ideal compute model"
			if lang == "zh" {
				bound = "既定理想算力模型内的下界"
			}
			if !strings.Contains(full, bound) {
				t.Errorf("frequency missing-data lower bound needs its model scope %q", bound)
			}
			for _, want := range []string{"17.000ms", "6.667ms", "VerifyClass"} {
				if !strings.Contains(system, want) {
					t.Errorf("calibration lost native semantic/fold value %q", want)
				}
			}
		})
	}
}

func TestOptimizationCaliberPublicCustomerPreEdgeSemantic(t *testing.T) {
	data, err := os.ReadFile("../../eval/fixtures/real_traces/donghu_tieba_frame.systrace")
	if err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			ctx, path := optimizationCaliberPublicQuery(t, string(data), 59566, 34579.490, 34579.500)
			optimizationCaliberPublicRequest(ctx, path, lang, "com.baidu.tieba-59566", 59566, 34579.490, 34579.500, false)
			system, set := optimizationCaliberPublicPublish(t, ctx, path, lang, false)
			if len(set.Projections) != 1 {
				t.Fatalf("customer window must retain one projection: %d", len(set.Projections))
			}
			found := false
			for _, n := range set.Projections[0].RankedSeats {
				if n.OnChainBasis == types.TraceCausalOnChainBasisHostWakeupEdgeSpan && strings.Contains(n.SpanName, "VerifyClass") {
					found = true
					if math.Abs(n.EffectiveImpactMS-.285) > .002 || n.Rank <= 0 || n.ChainRelevance != "on_chain" || n.HostWakeupEdgeAnchorTS != 34579.496810 {
						t.Errorf("customer's pre-edge semantic share lost its value/credential: %.3f rank=%d chain=%s edge=%.6f", n.EffectiveImpactMS, n.Rank, n.ChainRelevance, n.HostWakeupEdgeAnchorTS)
					}
				}
			}
			if !found {
				t.Error("customer native query did not exercise the host-wakeup-edge semantic rule")
			}
			optimizationCaliberAssertWords(t, "customer pre-edge appendix", system, lang, true)
			if !strings.Contains(system, "0.285ms") || !strings.Contains(system, "VerifyClass") {
				t.Error("pre-edge deterministic semantic optimization point disappeared from publication")
			}
		})
	}
}

// The native public tests above protect evidence and publication. These
// existing, independently pinned typed fixtures isolate the renderer's
// symmetric-overlap and adjacent-board reader branches without pretending
// that a new physical trace must produce a particular presentation fold.
func TestOptimizationCaliberTypedOverlapAndAdjacentReaders(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			p := elimv2DirectionBoardProjection()
			before, _ := json.Marshal(p)
			model := buildRuntimeTraceProjTreeModel(p, newRuntimeTraceCausalProjectionEvidenceIndex(), lang == "zh")
			tree := runtimeTraceProjTreeFence(model, lang == "zh")
			overview := runtimeTraceProjElimOverviewFence(p, model, lang == "zh")
			full := strings.Join(runtimeTraceProjLegendGroupLines(model.Marks, lang == "zh"), "\n")
			if !strings.Contains(overview, "·∩[") || !strings.Contains(overview, "2.500ms") || !strings.Contains(overview, "0.700ms") {
				t.Error("typed fixture must retain overlap pair, its measure and the adjacent value")
			}
			for _, face := range []struct{ name, text string }{{"typed tree", tree}, {"typed overview", overview}, {"typed full legend", full}} {
				for _, old := range []string{"修其一后另一席空间会缩", "fixing one shrinks the other seat's headroom", "修其一,另一席收益随之收缩", "fix one and the other seat's gain shrinks", "因果候选成立时至多好这么多", "at most this much if the causal candidate holds"} {
					if strings.Contains(face.text, old) {
						t.Errorf("%s infers unverified repair benefit via %q", face.name, old)
					}
				}
			}
			optimizationCaliberAssertWords(t, "typed full legend", full, lang, false)
			after, _ := json.Marshal(p)
			if string(before) != string(after) {
				t.Error("reader calibration mutated typed values, relations or rank order")
			}
		})
	}
}
