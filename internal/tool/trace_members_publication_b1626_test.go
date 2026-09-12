package tool

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func b1626PublicMemberBus(t *testing.T, scope string) *types.BusContext {
	t.Helper()
	ctx := suppCoreContext(t)
	const request = "Analyze worker in 3..3.035 and 3.04..3.08 seconds in this trace."
	ctx.Mutable = types.NewMutableState(request)
	ctx.AttachedHitrace = "trace fixture"
	const windows = `[{"time_start":3,"time_end":3.035,"source_quote":"3..3.035"},{"time_start":3.04,"time_end":3.08,"source_quote":"3.04..3.08"}]`
	raw := b1626MultiWindowAnalysisParams(t, windows, scope, request)
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	payload["entities"] = []string{"worker"}
	payload["runtime_targets"] = []map[string]any{{"kind": "thread", "pid": 200, "thread": "worker", "source": "user_explicit", "confidence": 1}}
	payload["runtime_target_profile"] = map[string]any{"declaration": "named_target", "source_quote": "worker", "confidence": 1}
	raw, _ = json.Marshal(payload)
	result, err := (&EmitAnalysis{}).Execute(ctx, raw)
	if err != nil || !result.Success {
		t.Fatalf("actual analysis: %v %+v", err, result)
	}
	rm := ctx.Mutable.RequestModel()
	if rm == nil {
		t.Fatal("analysis lost request")
	}
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: *rm, AnswerContract: types.AnswerContract{Language: "zh"}}
	ctx.Language = "zh"
	return ctx
}

func TestMultiWindowPublicAnalysisQuerySupplementPublicationB1626(t *testing.T) {
	for _, questionScope := range []string{"bounded_fact_set", "causal_diagnosis"} {
		t.Run(questionScope, func(t *testing.T) {
			ctx := b1626PublicMemberBus(t, questionScope)
			view := "window_stats"
			if questionScope == "causal_diagnosis" {
				view = "root_cause_rank"
			}
			raw, _ := json.Marshal(map[string]any{"view": view, "pid": 200, "time_start": 3.0, "time_end": 3.035})
			suppCoreModelCall(t, ctx, string(raw))
			original, _ := json.Marshal(ctx.ToolResults)
			RunTraceQuerySystemSupplement(ctx)
			ledger := suppCoreLedger(ctx)
			states := types.BuildTraceTargetStateScopeAuthoritiesFromLedger(ledger)
			seen := map[[2]float64]bool{}
			for _, state := range states {
				key := [2]float64{state.WindowStartTs, state.WindowEndTs}
				if key == [2]float64{3, 3.035} || key == [2]float64{3.04, 3.08} {
					seen[key] = true
					if state.TotalMS <= 0 || state.TotalMS > state.WindowMS+.002 {
						t.Fatalf("invalid independent account: %+v", state)
					}
				} else {
					t.Fatalf("unexpected principal account/envelope: %+v", state)
				}
			}
			if len(seen) != 2 {
				t.Fatalf("A must not conceal B state account: %+v", states)
			}
			set := types.CompileTraceCausalProjectionSet(ledger)
			var selection []*types.TraceRootCauseItemV2
			// Use the same typed breadth/seat/representability predicates as
			// prepareTraceFindingContract, not a forced enabled flag. This tool
			// integration does not claim to dispatch the finalizer agent itself.
			decided, allowed := types.RuntimeTraceReportShapeAuthority(&ctx.AnalysisIR.RequestModel)
			if decided && !allowed {
				if questionScope != "bounded_fact_set" || ctx.Mutable.TraceFindingContract() != nil {
					t.Fatal("bounded request acquired a report contract")
				}
			} else {
				seatInput := types.ObservationLedgerInputFromBusContext(ctx, 64)
				contract, err := tracefinding.CompileCandidateContract(ledger, set, tracefinding.BuildSeatFrameCausalityAuthority(seatInput))
				if err != nil {
					t.Fatal(err)
				}
				selectable := tracefinding.SelectableRootCauseCandidates(contract)
				contract.RootCauseReportEnabled = len(selectable) > 0
				if !contract.RootCauseReportEnabled {
					t.Fatalf("physical causal fixture produced no selectable candidate: %+v", contract.Candidates)
				}
				ctx.Mutable.SetTraceFindingContract(contract)
				byMember := make(map[int]types.TraceFindingCandidateV1)
				for _, candidate := range selectable {
					if candidate.Decision.EvidenceFacts == nil {
						t.Fatalf("actual candidate lost typed evidence facts: %+v", candidate)
					}
					scope := candidate.Decision.EvidenceFacts.WindowScope
					if scope == nil {
						t.Fatalf("actual candidate lost parent query scope: %+v", candidate)
					}
					index, ok := ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.MatchExplicitTimeWindow(scope.QueryWindowStartTs, scope.QueryWindowEndTs)
					if !ok || scope.Role != types.TraceQueryWindowScopeRequestedPrincipal {
						t.Fatalf("actual selectable candidate is not bound to one requested member: %+v", candidate)
					}
					if _, exists := byMember[index]; !exists {
						byMember[index] = candidate
					}
				}
				if len(byMember) != 2 {
					t.Fatalf("both real physical windows need selectable causes, not a vacuous sidecar check: %+v", selectable)
				}
				// The model may choose B before A; the binder may not sort it.
				for _, index := range []int{1, 0} {
					selection = append(selection, &types.TraceRootCauseItemV2{CandidateID: byMember[index].Decision.CandidateID, Description: "模型选择的业务排查线索"})
				}
			}
			doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "model", Kind: types.BlockSummary, Text: "模型业务结论保留，不由系统代写。"}}}
			if questionScope == "causal_diagnosis" {
				// The public emitter's existing Trace lead contract is authored
				// by this model fixture, not repaired by the system under test.
				doc.Blocks[0].SurfaceRole = "principal"
				doc.Blocks[0].TraceCausalClaimCaliber = types.TraceCausalClaimBoundedWindow
			}
			wire, err := modelOwnedAnswerBlockWire(doc)
			if err != nil {
				t.Fatal(err)
			}
			payload := map[string]any{"blocks": doc.Blocks}
			if questionScope == "causal_diagnosis" {
				payload["trace_root_causes"] = &types.TraceRootCauseReportV2{SchemaVersion: 2, RootCauses: selection}
			}
			emitRaw, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			published, err := executeAnswerDocumentV2("emit_answer_document", ctx, emitRaw, time.Unix(1, 0))
			if err != nil || !published.Success {
				t.Fatalf("publication: %+v %v", published, err)
			}
			stored := ctx.Mutable.AnswerDocumentV2()
			if err := requireModelOwnedAnswerBlockWirePreserved(wire, stored); err != nil {
				t.Fatal(err)
			}
			// This is the actual successful emit's Mutable report, not a direct
			// binder return or a claim about the CLI's later disk sidecar write.
			report := ctx.Mutable.TraceRootCauseReport()
			if questionScope == "bounded_fact_set" {
				if report != nil {
					t.Fatal("bounded facts acquired a root-cause selection")
				}
			} else {
				if report == nil || len(report.RootCauses) != 2 {
					t.Fatalf("actual emit lost the model's two selections: %+v", report)
				}
				for i, member := range []int{1, 0} {
					item := report.RootCauses[i]
					if item.Description != "模型选择的业务排查线索" || item.WindowScope == nil || item.ImpactSeconds == nil || *item.ImpactSeconds <= 0 {
						t.Fatalf("actual sidecar lost typed cost/model explanation: %+v", item)
					}
					index, ok := ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.MatchExplicitTimeWindow(item.WindowScope.QueryWindowStartTs, item.WindowScope.QueryWindowEndTs)
					if !ok || index != member || item.WindowScope.Role != types.TraceQueryWindowScopeRequestedPrincipal {
						t.Fatalf("actual emit changed member/order or substituted envelope: %+v", item)
					}
				}
			}
			text := render.RenderAnswerDocument(stored, "zh")
			for _, window := range []string{"3.000000", "3.035000", "3.040000", "3.080000"} {
				if !strings.Contains(text, window) {
					t.Errorf("published answer lost member %s", window)
				}
			}
			if questionScope == "bounded_fact_set" && strings.Contains(text, "Trace 因果投影") {
				t.Fatal("time members must not broaden a bounded fact into a causal report")
			}
			if questionScope == "causal_diagnosis" && !strings.Contains(text, "Trace 因果投影") {
				t.Fatal("requested causal projection disappeared")
			}
			after, _ := json.Marshal(ctx.ToolResults)
			if !bytes.Equal(original, after) {
				t.Fatal("supplement/publication modified model exploration")
			}
		})
	}
}
