package orchestrator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/budget"
	"github.com/hanchaoqun/codrax/internal/analysis/compiler"
	"github.com/hanchaoqun/codrax/internal/analysis/criterion"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestSourceInventoryRuntimeContextDoesNotSuppressExplicitPinReads(t *testing.T) {
	for _, inventory := range []bool{false, true} {
		t.Run(map[bool]string{false: "pin_only", true: "pin_and_inventory"}[inventory], func(t *testing.T) {
			o := newTestOrch(t)
			if err := os.WriteFile(filepath.Join(o.busCtx.RepoRoot, "Makefile"), []byte("all:\n\ttrue\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			rm := types.RequestModel{Intent: types.IntentEnumerate, Predicates: types.SemanticPredicates{HasPerMemberTable: true}, UserPinnedFiles: []string{"Makefile"}, AnalyzerHints: types.AnalyzerHints{RequiredFileHints: []types.RequiredFileHint{{Path: "Makefile", Confidence: 1}}}}
			if inventory {
				rm.SourceInventoryProfile = &types.SourceInventoryProfile{IsSourceInventory: true, TargetRoles: []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction}, Confidence: 1}
			}
			o.busCtx.AnalysisIR = &types.AnalysisIR{RequestModel: rm}
			o.busCtx.AttachedHitrace = "runtime artifact"
			if types.SourceInventoryCurrentSourceApplicableFromBus(o.busCtx) {
				t.Fatal("pin without a source suffix must exercise the independent pin arm")
			}
			if got := o.seedRequiredFileHintForcedReadsBeforeExplore(); got != 1 {
				t.Fatalf("explicit Makefile pin lost its required read: %d", got)
			}
			pending := o.busCtx.Mutable.EvidenceClosure().PendingReads()
			if len(pending) != 1 || pending[0].File != "Makefile" {
				t.Fatalf("pin read changed: %+v", pending)
			}
		})
	}
}

func TestSourceInventoryRuntimeCompiledLensUsesApplicableNavigation(t *testing.T) {
	for _, tc := range []struct {
		name          string
		origin        types.SourceInventoryDeclarationOrigin
		precise, want bool
	}{
		{"legacy_runtime", "", false, false},
		{"synthesized_runtime", types.SourceInventoryDeclarationSynthesized, false, false},
		{"declared_soft_navigation", types.SourceInventoryDeclarationModelProvided, false, true},
		{"legacy_precise_source", "", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rm := types.RequestModel{Intent: types.IntentEnumerate, Scenario: types.ScenarioGeneric, Predicates: types.SemanticPredicates{HasPerMemberTable: true},
				PerfTrace:              &types.PerfBundle{Observations: []types.PerfObservation{{Kind: "stage", Subject: "renamed"}}},
				SourceInventoryProfile: &types.SourceInventoryProfile{IsSourceInventory: true, DeclarationOrigin: tc.origin, TargetRoles: []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction}, SourceQuotes: []string{"records"}, Confidence: 1}}
			if tc.precise {
				rm.AnalyzerHints.ExactTargets = []string{"src/owner.go"}
			}
			out := compiler.Compile(rm, budget.BudgetSignals{})
			ir := &types.AnalysisIR{RequestModel: rm, TaskGraph: out.TaskGraph, EvidencePlan: out.EvidencePlan, AnswerContract: out.AnswerContract}
			ctx := &types.BusContext{AnalysisIR: ir, Mutable: types.NewMutableState("records")}
			active := sourceInventoryLensNavigationActive(ir)
			if active != tc.want || sourceInventoryProfileActive(ctx) != tc.want {
				t.Fatalf("scheduler applicability differs: %v want %v", active, tc.want)
			}
			seen := 0
			for _, node := range out.TaskGraph.Nodes {
				for _, entry := range node.EntryConditions {
					if entry.Kind != types.CritSourceInventoryLensMissing {
						continue
					}
					seen++
					if !node.Optional || !node.OneShot {
						t.Fatal("lens became an unconditional graph obligation")
					}
					if got := criterion.Eval(entry, criterion.Env{SourceInventoryProfileActive: active}); got.Satisfied != tc.want {
						t.Fatalf("compiled entry disagrees: %+v", got)
					}
				}
			}
			if seen != 1 {
				t.Fatalf("optional navigation disappeared: %d", seen)
			}
		})
	}
}

func TestSourceInventoryRuntimeRestoredContextDoesNotMintDispatchLane(t *testing.T) {
	for _, carrier := range []string{"route", "attachment", "accepted_bundle"} {
		t.Run(carrier, func(t *testing.T) {
			rm := types.RequestModel{Intent: types.IntentEnumerate, Predicates: types.SemanticPredicates{HasPerMemberTable: true}, SourceInventoryProfile: &types.SourceInventoryProfile{IsSourceInventory: true, TargetRoles: []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction}, SourceQuotes: []string{"records"}, Confidence: 1}}
			ctx := &types.BusContext{AnalysisIR: &types.AnalysisIR{RequestModel: rm}, Mutable: types.NewMutableState("records")}
			switch carrier {
			case "route":
				ctx.TurnRouteHint = types.TurnRouteHint{Route: "repo", Source: "external_tool", NeedsRepoAccess: true, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional}
			case "attachment":
				ctx.AttachedHitrace = "runtime artifact"
			case "accepted_bundle":
				ctx.Mutable.SetPerfTrace(&types.PerfBundle{Observations: []types.PerfObservation{{Kind: "stage", Subject: "renamed"}}})
			}
			if sourceInventoryProfileActive(ctx) {
				t.Fatal("context-only runtime carrier promoted restored unknown profile")
			}
			ctx.AnalysisIR.RequestModel.AnalyzerHints.ExactTargets = []string{"src/owner.go"}
			if !sourceInventoryProfileActive(ctx) {
				t.Fatal("independently precise source request lost dispatch")
			}
		})
	}
}
