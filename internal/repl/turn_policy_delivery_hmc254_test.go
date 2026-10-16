package repl

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Canned decisions reproduce the guard's amplification, not natural model accuracy.
func TestHMC254ProducerOwnedDeliveryStaysInAnalysis(t *testing.T) {
	for _, lane := range []string{"repl", "single_shot"} {
		for _, mode := range []string{"optional", "required"} {
			t.Run(lane+"/"+mode, func(t *testing.T) {
				params := map[string]any{
					"route": "hybrid", "needs_repo_access": true, "needs_operation_access": true,
					"current_source_evidence_mode": mode, "operation": "investigate", "write_intent": "analysis_only",
					"source": "artifact", "confidence": .95, "reason": "analysis plus producer-owned publication",
					"requires_diagram": false, "required_outcomes": []string{"measurement", "external_artifact"},
					"delivery_owner": "analysis_pipeline", "target_surface": "file_artifact", "risk_level": "low",
					"side_effects": []string{"local_file_write"},
				}
				if mode == "required" {
					params["required_outcomes"] = []string{"measurement", "external_artifact", "source_explanation"}
					params["source"] = "mixed"
				}
				raw, err := json.Marshal(params)
				if err != nil {
					t.Fatal(err)
				}
				a := &scriptedChatAdapter{responses: []llm.Response{turnPolicyResp(string(raw))}}
				c := NewChitchatClassifier(a)
				var got TurnPolicy
				if lane == "single_shot" {
					got, err = c.(SingleShotTurnPolicyClassifier).ClassifyPolicySingleShot(context.Background(), "检查本次采集并保存证据包", "", false)
				} else {
					got, err = c.(TurnPolicyClassifier).ClassifyPolicy(context.Background(), "检查本次采集并保存证据包", "", false)
				}
				if err != nil {
					t.Fatal(err)
				}
				got = ApplyTurnPolicyGuards(got, false, false)
				if got.Route != RouteHybrid || got.NeedsOperationAccess || got.NeedsDataAccess || len(got.SideEffects) != 0 || IsConcreteOperationPolicy(got) {
					t.Fatalf("producer-owned publication stole native analysis: %+v", got)
				}
				if got.CurrentSourceEvidenceMode != types.TurnRouteCurrentSourceEvidenceMode(mode) || !got.RequiredOutcomes.Has(types.TurnOutcomeExternalArtifact) {
					t.Fatalf("independent source/delivery obligations lost: %+v", got)
				}
			})
		}
	}
}

func TestHMC254DeliveryOwnershipDoesNotGrantOrEraseOtherObligations(t *testing.T) {
	base := hmc254OperationAnalysisPolicy()
	base.DeliveryOwner = analysisPipelineDeliveryOwner
	for _, tc := range []struct {
		name string
		edit func(*TurnPolicy)
	}{
		{"missing_owner", func(p *TurnPolicy) { p.DeliveryOwner = "" }},
		{"unknown_owner", func(p *TurnPolicy) { p.DeliveryOwner = "native-ish" }},
		{"independent_owner", func(p *TurnPolicy) { p.DeliveryOwner = independentOperationDeliveryOwner }},
		{"computer_action", func(p *TurnPolicy) { p.RequiredOutcomes |= types.TurnOutcomeComputerAction }},
		{"file_copy", func(p *TurnPolicy) { p.Operation, p.OperationKind = "computer_operation", "computer_operation" }},
		{"office_workflow", func(p *TurnPolicy) { p.Operation = "document_generation" }},
		{"browser", func(p *TurnPolicy) { p.TargetSurface = "browser_ui" }},
		{"overwrite", func(p *TurnPolicy) { p.SideEffects = []string{"local_file_overwrite"} }},
		{"network", func(p *TurnPolicy) { p.SideEffects = []string{"network_submit"} }},
		{"high_risk", func(p *TurnPolicy) { p.RiskLevel = "high" }},
		{"approval", func(p *TurnPolicy) { p.RequiresConfirmation = true }},
		{"no_analysis_outcome", func(p *TurnPolicy) { p.RequiredOutcomes = types.TurnOutcomeExternalArtifact }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := base
			tc.edit(&p)
			if analysisOwnsRequestedDelivery(p) {
				t.Fatalf("ineligible ownership reconciled: %+v", p)
			}
			got := ApplyTurnPolicyGuards(p, false, false)
			if got.Route != RouteOperation || !got.NeedsOperationAccess || !IsConcreteOperationPolicy(got) {
				t.Fatalf("independent operation erased: %+v", got)
			}
		})
	}
	p := base
	p.RequiredOutcomes |= types.TurnOutcomeSourceExplanation
	p.CurrentSourceEvidenceMode = types.TurnRouteCurrentSourceEvidenceOptional
	if got := ApplyTurnPolicyGuards(p, false, false); got.CurrentSourceEvidenceMode != types.TurnRouteCurrentSourceEvidenceRequired {
		t.Fatalf("producer ownership excused real source explanation: %+v", got)
	}
	p = base
	p.Route, p.WriteIntent = RouteWrite, WriteIntentExplicitChange
	p.RequiredOutcomes |= types.TurnOutcomeSourceChange
	if got := ApplyTurnPolicyGuards(p, false, false); got.Route != RouteWrite || got.WriteIntent != WriteIntentExplicitChange {
		t.Fatalf("write route changed: %+v", got)
	}
}

func TestHMC254DeliveryOwnerSchemaTeachingAndAuditAreConsistent(t *testing.T) {
	for _, owner := range []string{"analysis_pipeline", "independent_operation", "", "made_up"} {
		params := `{"route":"repo","needs_repo_access":true,"operation":"investigate","source":"artifact","confidence":0.95,"reason":"typed","requires_diagram":false,"delivery_owner":"` + owner + `"}`
		var parsed turnPolicyParams
		err := unmarshalReplStructuredToolParams(turnPolicyTool, []byte(params), &parsed, "test")
		if (err == nil) != (owner != "made_up") {
			t.Fatalf("owner %q schema gate: %v", owner, err)
		}
	}
	a := &scriptedChatAdapter{responses: []llm.Response{turnPolicyResp(`{"route":"repo","needs_repo_access":true,"operation":"investigate","source":"artifact","confidence":0.95,"reason":"typed","requires_diagram":false,"delivery_owner":"analysis_pipeline"}`)}}
	_, err := NewChitchatClassifier(a).(TurnPolicyClassifier).ClassifyPolicy(context.Background(), "分析材料并交付结果", "", false)
	if err != nil {
		t.Fatal(err)
	}
	var prompt string
	for _, m := range a.calls[0].messages {
		if m.Role == "system" {
			prompt += m.Content
		}
	}
	if strings.Count(string(a.calls[0].tools[0].Parameters), turnDeliveryOwnershipContract) != 1 || strings.Contains(prompt, turnDeliveryOwnershipContract) || !strings.Contains(prompt, "Use delivery_owner according to its schema") {
		t.Fatal("ownership teaching duplicated or absent")
	}
	if !strings.Contains(turnPolicyDebugLine(TurnPolicy{DeliveryOwner: analysisPipelineDeliveryOwner}), "delivery_owner=analysis_pipeline") {
		t.Fatal("typed owner missing from audit log")
	}
}
