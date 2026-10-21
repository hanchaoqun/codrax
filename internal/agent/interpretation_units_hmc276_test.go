package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC276InterpretationUnitsPublicHandoff(t *testing.T) {
	for _, mode := range []string{"complete", "whitespace", "over_budget", "safety_filtered", "restatement", "restored"} {
		t.Run(mode, func(t *testing.T) {
			ctx := nativeFactHandoffContextHMC247(t, true)
			ctx.TurnRouteHint = types.TurnRouteHint{Route: "repo", Source: "mixed", NeedsRepoAccess: true, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceRequired}
			choices := tool.NativeFactHandoffChoices(types.ToolBusContext(ctx, types.AgentExplorer))
			const claim = "HMC276_INDEPENDENT_BUSINESS_CLUE observed through read_file"
			const boundary = "HMC276_EXECUTION_STATE_NOT_ESTABLISHED"
			const other = "HMC276_OTHER_INDEPENDENT_FINDING"
			const legacy = "HMC276_LEGACY_DUPLICATE_AUDIT_ONLY"
			supportRef := "trace_query:" + choices[0].ObservationID + "#measurements/" + string(choices[0].View)
			unitText, unitRef := claim, supportRef
			limit := boundary
			if mode == "whitespace" {
				unitText = " \n" + claim + "\n\n \nadditional business clue \n"
				unitRef = " " + unitRef + "\n"
				limit = " " + limit + "\n"
			}
			if mode == "over_budget" {
				limit += strings.Repeat(" bounded observation", 200)
			}
			if mode == "safety_filtered" {
				limit = "<think>HMC276_PRIVATE_BOUNDARY</think>"
			}
			role := "independent_analysis"
			if mode == "restatement" {
				role = "native_fact_restatement"
			}
			raw, _ := json.Marshal(map[string]any{
				"reason": "HMC276 lifecycle", "confidence": "high", "result_kind": "resolved",
				"native_fact_handoff": map[string]any{
					"runtime_measurements": choices[:1], "interpretation": legacy, "interpretation_role": role,
					"interpretation_units": []any{
						map[string]any{"text": unitText, "boundaries": []string{limit}, "support_refs": []string{unitRef}},
						map[string]any{"text": other, "boundaries": []string{"HMC276_NO_CAUSAL_PERMISSION"}},
					},
				},
			})
			r, err := (&tool.EmitInvestigationComplete{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
			if err != nil || !r.Success {
				t.Fatalf("public completion: %v %s", err, r.Summary)
			}
			before, _ := json.Marshal(ctx.Mutable.AcceptedInvestigationNativeFactHandoff())
			if !strings.Contains(string(before), boundary) && mode != "safety_filtered" {
				t.Fatal("accepted handoff lost structured explanation")
			}
			if mode == "restored" {
				ta := ctx.Mutable.TurnAArtifacts()
				ta.AcceptedClosureReason = "HMC276 lifecycle"
				ta.AcceptedNativeFactHandoff = ctx.Mutable.AcceptedInvestigationNativeFactHandoff()
				wire, _ := json.Marshal(ta)
				var restored types.TurnAArtifacts
				if err := json.Unmarshal(wire, &restored); err != nil {
					t.Fatal(err)
				}
				ctx.Mutable = types.NewMutableState("restored")
				ctx.Mutable.SetTurnAArtifacts(restored)
				ctx.Mutable.SetInvestigationComplete("HMC276 lifecycle")
			}
			ctx = ctx.ShallowClone()
			projection := answerDocSeparatedNativeFactHandoff(ctx, nil)
			wantClaim := mode == "complete" || mode == "restored" || mode == "whitespace"
			if strings.Contains(projection.text, claim) != wantClaim || strings.Contains(projection.text, legacy) {
				t.Fatalf("atomic or exclusive interpretation broken: %s", projection.text)
			}
			if wantClaim && (!strings.Contains(projection.text, boundary) || !strings.Contains(projection.text, supportRef)) {
				t.Fatal("claim published without its full boundaries/reference")
			}
			if strings.Contains(projection.text, other) != (mode != "restatement") || strings.Contains(projection.text, "HMC276_PRIVATE_BOUNDARY") {
				t.Fatal("independent unit lost or safety-filtered unit escaped")
			}
			if projection.ownsCurrentNarrative != (mode != "restored") || projection.publishedInterpretationBytes > 1800 {
				t.Fatal("restored authority or narrative budget changed")
			}
			if mode == "complete" || mode == "whitespace" {
				actual := dependencyObservationMessages(t, ctx)
				if !strings.Contains(actual, claim) || !strings.Contains(actual, boundary) || strings.Contains(actual, legacy) {
					t.Fatalf("actual Finalizer: claim=%t boundary=%t legacy=%t", strings.Contains(actual, claim), strings.Contains(actual, boundary), strings.Contains(actual, legacy))
				}
				extracted := renderExtractorNativeFactHandoff(ctx, "HMC276 lifecycle", nil)
				if !strings.Contains(extracted, claim) || !strings.Contains(extracted, boundary) || !strings.Contains(extracted, supportRef) || strings.Contains(extracted, legacy) {
					t.Fatal("extractor did not use shared atomic interpretation")
				}
				after, _ := json.Marshal(ctx.Mutable.AcceptedInvestigationNativeFactHandoff())
				if string(before) != string(after) {
					t.Fatal("render changed original handoff")
				}
			}
		})
	}
}

func TestHMC276InterpretationWholeUnitBudgets(t *testing.T) {
	h := &types.InvestigationNativeFactHandoff{InterpretationUnits: []types.InvestigationInterpretationUnit{{Text: "业务线索", Boundaries: []string{"未知执行状态，不是链上主因"}, SupportRefs: []string{"capture-A:window-1", "capture-B:window-2"}}}}
	whole := previewInvestigationInterpretation(h, 1800, sanitizePriorDraftForSummary)
	if whole.publishedBytes == 0 {
		t.Fatal("unit not published")
	}
	for _, n := range []int{0, 1, whole.publishedBytes - 1, whole.publishedBytes} {
		p := previewInvestigationInterpretation(h, n, sanitizePriorDraftForSummary)
		complete := n == whole.publishedBytes
		for _, field := range []string{"业务线索", "未知执行状态，不是链上主因", "capture-A:window-1", "capture-B:window-2"} {
			if strings.Contains(p.text, field) != complete {
				t.Fatalf("budget %d split unit: %s", n, p.text)
			}
		}
		if p.publishedBytes > n {
			t.Fatal("budget grew")
		}
	}
	legacy := "LEGACY_CLAIM_START " + strings.Repeat("长段", 600) + " LEGACY_CLAIM_END\n\nLEGACY_COMPLETE_LIMITATION"
	p := previewInvestigationInterpretation(&types.InvestigationNativeFactHandoff{Interpretation: legacy}, 900, sanitizePriorDraftForSummary)
	if strings.Contains(p.text, "LEGACY_CLAIM_START") || strings.Contains(p.text, "LEGACY_CLAIM_END") || !strings.Contains(p.text, "LEGACY_COMPLETE_LIMITATION") || !strings.Contains(p.text, "essential limitations") {
		t.Fatal("legacy paragraph split or completeness misrepresented")
	}
	if p.publishedBytes > 900 || p.totalBytes <= p.publishedBytes {
		t.Fatal("legacy omission accounting wrong")
	}
}
