package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC218ReaderDedupRefillsBudgetBeforeProjection(t *testing.T) {
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		LogTriage: &types.LogBundle{Errors: []types.LogError{{Message: "first"}, {Message: "second"}}},
	}}}
	ctx.Mutable = types.NewMutableState("observed peer errors")
	ctx.Mutable.SetLogTriage(ctx.AnalysisIR.RequestModel.LogTriage)
	var rows []types.ObservationRecord
	for _, id := range []string{"log:error:0", "log:error:1", "log:cross_error_relation"} {
		rows = append(rows, types.ObservationRecord{ID: id, Origin: types.AnswerEvidenceOriginRuntimeArtifact, Role: types.AnswerAggregateRolePrincipalAnswer})
	}
	for i := 0; i < 9; i++ {
		rows = append(rows, types.ObservationRecord{ID: fmt.Sprintf("retained-%d", i), Origin: types.AnswerEvidenceOriginRuntimeArtifact, Value: "retained fact"})
	}
	projected := answerDocObservationPromptRecords(ctx, rows, 9)
	if len(projected) != 9 {
		t.Fatalf("dedup left unused slots: got %d/9", len(projected))
	}
	for _, row := range projected {
		if !strings.HasPrefix(row.ID, "retained-") {
			t.Fatalf("duplicate consumed a slot before budget: %s", row.ID)
		}
	}
	if len(rows) != 12 || rows[0].ID != "log:error:0" {
		t.Fatal("projection changed lossless ledger")
	}
}

func TestHMC218CarrierReusesShownIdentitiesWithoutMutatingFacts(t *testing.T) {
	carrier := types.ToolHandoffCarrier{ToolName: "log_query", ObservationRefs: []types.ToolObservationRef{
		{ID: "shown-a", Producer: "log_query", Source: "app/log"},
		{ID: "shown-b", Producer: "log_query", Source: "kernel/log"},
		{ID: "unshown", Producer: "log_query", Source: "third/log"},
	}}
	text := renderTypedToolHandoffCarriers("", []types.ToolHandoffCarrier{carrier}, toolHandoffRenderOptions{
		MaxRefs: 1, PresentedObservationIDs: map[string]bool{"shown-a": true, "shown-b": true},
	})
	if !strings.Contains(text, "2 observation(s) already published") || !strings.Contains(text, "unshown") || strings.Contains(text, "observation=`shown-") {
		t.Fatalf("second identity prefix dropped the unrepresented source: %s", text)
	}
	if len(carrier.ObservationRefs) != 3 || carrier.ObservationRefs[0].ID != "shown-a" {
		t.Fatal("display dedup mutated audit carrier")
	}
	// A generic row does not replace richer target-wait details. This holds
	// even when the native source/query scope qualifies for display dedup.
	text = renderTypedToolHandoffCarriers("", []types.ToolHandoffCarrier{carrier}, toolHandoffRenderOptions{
		PresentedObservationIDs: map[string]bool{"shown-a": true},
		ObservationDetails: map[string]types.ObservationPromptRecord{"shown-a": {
			Notes: []string{"target_wait_occurrence=#9 state=d_sleep duration=3ms"},
		}},
	})
	if !strings.Contains(text, "observation=`shown-a`") || !strings.Contains(text, "target_wait_occurrence=#9") {
		t.Fatalf("generic row suppressed richer target-wait witness: %s", text)
	}
}

func TestHMC218ReferencedArtifactHintsDoNotImpersonateUserCoordinates(t *testing.T) {
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		ReferencedArtifactLines: []types.ArtifactLineRef{{Source: "log", StartLine: 99, EndLine: 101}},
	}}}
	text := renderAnswerDocReferencedArtifactLines(ctx)
	if !strings.Contains(text, "log lines 99-101") || !strings.Contains(text, "auto-filled") || strings.Contains(text, "The question references") {
		t.Fatalf("analyzer hint impersonates original user/source authority: %s", text)
	}
}
