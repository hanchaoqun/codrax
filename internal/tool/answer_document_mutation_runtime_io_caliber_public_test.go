package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func ioCaliberRenderCases() []string {
	return []string{types.TraceIOValueCaliberRQResidence, types.TraceIOValueCaliberBIOResidence,
		types.TraceIOValueCaliberIssuerBlocked, types.TraceIOValueCaliberMixed, "", "future_unknown"}
}

// This is the public publication path over tool-owned typed observations,
// not a claim that this fixture executes the native TraceQuery producer.
func TestRuntimeIOCaliberPublicEmitRenderPatch(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, caliber := range ioCaliberRenderCases() {
			t.Run(lang+"/"+caliber, func(t *testing.T) {
				bus := newBusForMutationTest()
				bus.Language = lang
				bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck}, AnswerContract: types.AnswerContract{Language: lang}}
				obs := traceProjectionObservation("io-receipt", "issuer-42", "io_latency", "35.000", "35.000", 1)
				obs.Summary = "Opposite prose: IO wait, device latency, 999ms. 摘要不能决定等待口径。"
				obs.RichNotes = append(obs.RichNotes, "type=io_latency", "io_value_caliber="+caliber, "chain_relevance=on_chain", "effective_impact_ms=35.000", "start_ts=1.005", "end_ts=1.040", "window_start_ts=1.000", "window_end_ts=1.050", "resource_completion_closure=true")
				bus.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: []types.ObservationRecord{obs}}}
				before, _ := json.Marshal([]any{bus.ToolResults, bus.AnalysisIR})
				const model = "Model-owned text: IO wait / device delay is an unverified interpretation. 模型原文保留。"
				params, _ := json.Marshal(map[string]any{"blocks": []map[string]any{{"id": "model_summary", "kind": "summary", "surface_role": "principal", "trace_causal_claim_caliber": "no_causal_conclusion", "text": model}}})
				result, err := (&EmitAnswerDocument{}).Execute(bus, params)
				if err != nil || !result.Success {
					t.Fatalf("public emit: %v; %s", err, result.Summary)
				}
				check := func() string {
					t.Helper()
					doc := bus.Mutable.AnswerDocumentV2()
					label := types.TraceIOValueCaliberLabel(caliber, lang == "zh")
					for _, id := range []string{"runtime_trace_causal_projection", "runtime_trace_causal_projection_detail_full"} {
						block := projectionClusterBlock(doc.Blocks, id)
						if block == nil || !strings.Contains(types.AnswerBlockVisibleSurface(*block), label) {
							t.Errorf("%s must retain its own measurement label %q: %+v", id, label, block)
						}
					}
					if block := projectionClusterBlock(doc.Blocks, "model_summary"); block == nil || block.Text != model {
						t.Fatal("publication rewrote the model's prose")
					}
					text := render.RenderAnswerDocument(doc, lang)
					if !strings.Contains(text, "35.000") || !strings.Contains(text, "issuer-42") {
						t.Fatal("measurement or subject disappeared")
					}
					return text
				}
				published, doc := check(), bus.Mutable.AnswerDocumentV2()
				result, err = (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(`{"unchanged_block_ids":["model_summary"]}`))
				if err != nil || !result.Success {
					t.Fatalf("public no-op patch: %v; %s", err, result.Summary)
				}
				if got := check(); got != published || !reflect.DeepEqual(doc, bus.Mutable.AnswerDocumentV2()) {
					t.Error("no-op patch changed the document")
				}
				after, _ := json.Marshal([]any{bus.ToolResults, bus.AnalysisIR})
				if string(before) != string(after) {
					t.Fatal("display changed input evidence or request authority")
				}
			})
		}
	}
}

func TestRuntimeIOCaliberNodeDisplaySurfaces(t *testing.T) {
	for _, zh := range []bool{true, false} {
		for _, caliber := range ioCaliberRenderCases() {
			for _, object := range []string{"io_latency", "irq-peer-9", "unknown_thread"} {
				t.Run(fmt.Sprintf("zh=%t/%s/%s", zh, caliber, object), func(t *testing.T) {
					node := types.TraceCausalProjectionNode{Subject: "issuer-42", Object: object, TypeToken: "io_latency", Predicate: "root_cause_primary", IOValueCaliber: caliber, ImpactMS: 35, EffectiveImpactMS: 35, ResourceCompletionClosure: true}
					before := node
					want := types.TraceIOValueCaliberLabel(caliber, zh)
					category, _ := runtimeTraceProjCauseCategoryWord(node, "", zh)
					diagnosis, _ := runtimeTraceProjElimVerdictTokenWord(node, "io_latency", zh)
					faces := map[string]string{
						"name":     runtimeTraceCausalProjectionDisplayCauseNameNode(node, zh),
						"short":    runtimeTraceProjDedupRowShortStateWord(runtimeTraceProjTreeRow{Node: node}, zh),
						"shape":    runtimeTraceCausalProjectionImpactShapeCell(node, zh),
						"family":   runtimeTraceProjImpactFormFamilyWord(node, zh),
						"category": category, "diagnosis": diagnosis,
						"detail": runtimeTraceCausalProjectionDetailTypeLabel(node, zh),
					}
					for face, got := range faces {
						if !strings.Contains(got, want) {
							t.Errorf("%s = %q, want own caliber %q", face, got, want)
						}
					}
					if !reflect.DeepEqual(node, before) {
						t.Fatal("display mutated a measurement")
					}
				})
			}
		}
	}
}

func TestRuntimeIOCaliberFoldAndNarrowDisplay(t *testing.T) {
	for _, zh := range []bool{true, false} {
		for _, caliber := range ioCaliberRenderCases() {
			node := types.TraceCausalProjectionNode{Subject: "long-worker-thread-name-4242", Object: "completion-peer-99", TypeToken: "io_latency", IOValueCaliber: caliber,
				Role: types.TraceCausalRoleRootCauseContext, ChainRelevance: "adjacent", ImpactMS: 35, CumulativeImpactMS: 35, DuplicatePublications: 2, Confidence: .9}
			row := runtimeTraceProjTreeRow{Node: node, Kind: runtimeTraceProjTreeRowAdjacent, HasData: true, EvidenceTag: "E9"}
			text := runtimeTraceProjTreeRowLine(row, 14, 50, true, zh)
			want := types.TraceIOValueCaliberLabel(caliber, zh)
			if !strings.Contains(partsplitSquash(text), partsplitSquash(want)) || !strings.Contains(text, "35.000") || !strings.Contains(text, "E9") {
				t.Errorf("narrow layout lost measurement qualifier/value/evidence: %s", text)
			}
			peer := runtimeTraceProjNewIOFoldPeer(node, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
			peer.EvidenceTag = "E9"
			other := node
			other.IOValueCaliber, other.ImpactMS = types.TraceIOValueCaliberIssuerBlocked, 31
			otherPeer := runtimeTraceProjNewIOFoldPeer(other, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
			otherPeer.EvidenceTag = "E10"
			note := runtimeTraceProjIOFoldNoteText([]runtimeTraceProjIOFoldPeer{peer, otherPeer}, zh)
			for _, fragment := range []string{want + " 35.000ms", types.TraceIOValueCaliberLabel(other.IOValueCaliber, zh) + " 31.000ms", "E9", "E10"} {
				if !strings.Contains(note, fragment) {
					t.Errorf("folded members must keep their own value/caliber pairing; missing %q: %s", fragment, note)
				}
			}
			if peer.IOValueCaliber != caliber || peer.ImpactMS != 35 || otherPeer.ImpactMS != 31 {
				t.Fatal("folding changed a member's measurement")
			}
		}
	}
}

func TestRuntimeIOCaliberPreservesOtherTypedSemantics(t *testing.T) {
	for _, zh := range []bool{true, false} {
		base := types.TraceCausalProjectionNode{TypeToken: "io_latency", Object: "io_latency", IOValueCaliber: types.TraceIOValueCaliberRQResidence}
		inversion := base
		inversion.PriorityInversionCandidate = true
		want, _ := runtimeTraceProjInversionFamilyWord(inversion, zh)
		category, _ := runtimeTraceProjCauseCategoryWord(inversion, "", zh)
		if runtimeTraceCausalProjectionImpactShapeCell(inversion, zh) != want || category != want {
			t.Fatal("IO measurement display replaced the existing inversion category")
		}
		lock := base
		lock.BlockingKind = "monitor_contention"
		category, _ = runtimeTraceProjCauseCategoryWord(lock, "", zh)
		if category != runtimeTraceCausalProjectionImpactShapeCell(lock, zh) || strings.Contains(category, types.TraceIOValueCaliberLabel(base.IOValueCaliber, zh)) {
			t.Fatal("IO measurement display replaced the existing lock category")
		}
		category, _ = runtimeTraceProjCauseCategoryWord(base, runtimeTraceProjTreeRowSemantic, zh)
		if strings.Contains(category, types.TraceIOValueCaliberLabel(base.IOValueCaliber, zh)) {
			t.Fatal("IO measurement display replaced the semantic row category")
		}
		state := base
		state.StateKind = "runnable"
		if got := runtimeTraceProjDedupRowShortStateWord(runtimeTraceProjTreeRow{Node: state}, zh); got != "runnable" {
			t.Fatalf("physical scheduler-state short label changed: %s", got)
		}
		for _, closure := range []bool{false, true} {
			base.ResourceCompletionClosure, base.IOValueCaliber = closure, ""
			if got := runtimeTraceCausalProjectionDetailTypeLabel(base, zh); got != types.TraceIOValueCaliberLabel("", zh) {
				t.Fatal("completion closure lent a missing measurement ruler")
			}
		}
		for _, label := range []string{runtimeTraceCausalProjectionPeerRelationShortWord("io_latency", zh), runtimeTraceCausalProjectionResolvedPeerText("io_latency", "peer-99", zh), TraceRootCauseTypeDisplayLabel("io_latency", zh)} {
			if !strings.Contains(label, types.TraceIOValueCaliberLabel("", zh)) {
				t.Fatalf("kind-only label guessed a measurement ruler: %s", label)
			}
		}
	}
}

func TestRuntimeIOCaliberFacetUnionKeepsPublishedRuler(t *testing.T) {
	for _, token := range []string{"io_wait", "io_burst_episode"} {
		for _, zh := range []bool{false, true} {
			node := types.TraceCausalProjectionNode{TypeToken: token, Object: token, IOValueCaliber: types.TraceIOValueCaliberMixed, ImpactMS: .109, StateKind: "io_wait"}
			want := types.TraceIOValueCaliberLabel(node.IOValueCaliber, zh)
			short := runtimeTraceProjDedupRowShortStateWord(runtimeTraceProjTreeRow{Node: node}, zh)
			verdict, _ := runtimeTraceProjElimVerdictTokenWord(node, token, zh)
			if short != want || verdict != want || runtimeTraceCausalProjectionDetailTypeLabel(node, zh) != want {
				t.Fatalf("facet representative lent one member's wait ruler to a mixed union: short=%q verdict=%q", short, verdict)
			}
			peer := runtimeTraceProjNewIOFoldPeer(node, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
			if got := runtimeTraceProjIOFoldNoteText([]runtimeTraceProjIOFoldPeer{peer}, zh); !strings.Contains(got, want+" 0.109ms") {
				t.Fatalf("folded facet union lost its published ruler: %s", got)
			}
			for _, absent := range []string{"", "future_unknown"} {
				node.IOValueCaliber = absent
				if runtimeTraceIOValueNode(node) {
					t.Fatal("ordinary IO state/activity acquired a duration ruler without a published valid field")
				}
				if got := runtimeTraceProjDedupRowShortStateWord(runtimeTraceProjTreeRow{Node: node}, zh); got != "iowait" {
					t.Fatalf("ordinary IO state short label changed: %s", got)
				}
			}
		}
	}
}

func TestRuntimeIOCaliberAdjacentDuplicateRetainsNumericDonor(t *testing.T) {
	for _, tc := range []struct {
		name                          string
		impact, cumulative, effective float64
		dupImpact, dupCumulative      float64
		want                          string
	}{
		{"both_from_duplicate", 35, 35, 0, 35.1, 35.1, types.TraceIOValueCaliberIssuerBlocked},
		{"impact_from_duplicate_cumulative_retained", 35, 36, 0, 35.1, 35.1, types.TraceIOValueCaliberMixed},
		{"cumulative_from_duplicate_impact_retained", 35.1, 35, 0, 35, 35.1, types.TraceIOValueCaliberMixed},
		{"effective_retained", 35, 35, 1, 35.1, 35.1, types.TraceIOValueCaliberMixed},
		{"exact_tie", 35, 35, 0, 35, 35, types.TraceIOValueCaliberRQResidence},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := types.TraceCausalProjectionNode{EvidenceID: "a", Subject: "issuer-42", Object: "worker-99", TypeToken: "io_latency", LineStart: 10, LineEnd: 20, ImpactMS: tc.impact, CumulativeImpactMS: tc.cumulative, EffectiveImpactMS: tc.effective, IOValueCaliber: types.TraceIOValueCaliberRQResidence}
			b := a
			b.EvidenceID, b.ImpactMS, b.CumulativeImpactMS, b.IOValueCaliber = "b", tc.dupImpact, tc.dupCumulative, types.TraceIOValueCaliberIssuerBlocked
			input := []types.TraceCausalProjectionNode{a, b}
			out := runtimeTraceProjAdjacentNodesForDisplay(input)
			if len(out) != 1 || out[0].DuplicatePublications != 2 || out[0].ImpactMS != max(tc.impact, tc.dupImpact) || out[0].CumulativeImpactMS != max(tc.cumulative, tc.dupCumulative) || out[0].EffectiveImpactMS != tc.effective {
				t.Fatalf("existing fold admission/value/count changed: %+v", out)
			}
			if out[0].IOValueCaliber != tc.want {
				t.Fatalf("measurement did not follow surviving numeric axes: got %q want %q", out[0].IOValueCaliber, tc.want)
			}
			if !reflect.DeepEqual(input, []types.TraceCausalProjectionNode{a, b}) {
				t.Fatal("display fold mutated source nodes")
			}
		})
	}
}

func TestRuntimeIOCaliberSelfCrownPreservesTypedFormEligibility(t *testing.T) {
	for _, zh := range []bool{false, true} {
		base := types.TraceCausalProjectionNode{Subject: "issuer-42", TypeToken: "io_latency", Object: "io_latency", IOValueCaliber: types.TraceIOValueCaliberRQResidence}
		model := runtimeTraceProjTreeModel{Target: "issuer-42"}
		state, category := runtimeTraceProjSelfCauseCrownState(base, types.TraceCausalProjection{}, model, zh)
		if state != types.TraceIOValueCaliberLabel(base.IOValueCaliber, zh) || category == "" {
			t.Fatalf("eligible IO crown lost its measurement: %q %q", state, category)
		}
		for _, kind := range []string{"inversion", "lock"} {
			node := base
			if kind == "inversion" {
				node.PriorityInversionCandidate = true
			} else {
				node.BlockingKind = "monitor_contention"
			}
			state, category := runtimeTraceProjSelfCauseCrownState(node, types.TraceCausalProjection{}, model, zh)
			if state != "" || category != "" {
				t.Fatalf("IO label bypassed existing %s crown exclusion: %q %q", kind, state, category)
			}
		}
	}
}
