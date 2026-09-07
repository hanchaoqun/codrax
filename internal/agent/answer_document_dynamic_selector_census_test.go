package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1580CappedSelectorPoolMustRetainConflict(t *testing.T) {
	base := b1580ProductionSelectorEvidence(t)
	for _, conflictFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("conflict_before_padding=%t", conflictFirst), func(t *testing.T) {
			all := b1580PaddedEvidence(t, base, conflictFirst, "same_group")
			ctx := b1580SelectorContext(all)
			full := types.CompileDynamicSelectorResolutionPaths(all, "run_pipeline")
			pool := answerDocDynamicSelectorEvidenceCensus(ctx)
			bounded := types.CompileDynamicSelectorResolutionPaths(pool, "run_pipeline")
			if b1580HasJSONCandidate(full) || len(full.Candidates) != 1 ||
				len(full.Rejected) != 1 || full.Rejected[0].Reason != types.DynamicSelectorRejectAmbiguousCandidate {
				t.Fatalf("invalid witness: full evidence must preserve csv and reject ambiguous json: %+v", full)
			}
			capsule := renderAnswerDocDynamicSelectorResolutionCandidates(ctx, "run_pipeline")
			var recipes strings.Builder
			anchors := renderAnswerDocDynamicSelectorRelationRecipes(&recipes, ctx)
			capsuleJSON := strings.Contains(capsule, "declared_candidate=`JsonPlugin`")
			recipeJSON := strings.Contains(recipes.String(), "declared_candidate=`JsonPlugin`")
			t.Logf("all=%d pool=%d full_candidates=%d full_rejected=%d bounded_candidates=%d bounded_rejected=%d capsule_json=%t recipe_json=%t recipe_anchors=%d",
				len(all), len(pool), len(full.Candidates), len(full.Rejected), len(bounded.Candidates), len(bounded.Rejected), capsuleJSON, recipeJSON, len(anchors))
			if b1580HasJSONCandidate(bounded) || capsuleJSON || recipeJSON {
				t.Errorf("bounded evidence dropped same-owner/literal conflict and published a candidate withheld by the full compiler")
			}
		})
	}
}

func TestB1580SelectorConflictBoundaries(t *testing.T) {
	base := b1580ProductionSelectorEvidence(t)
	for _, variant := range []string{"different_owner", "different_literal", "uncitable"} {
		t.Run(variant, func(t *testing.T) {
			all := b1580PaddedEvidence(t, base, false, variant)
			full := types.CompileDynamicSelectorResolutionPaths(all, "run_pipeline")
			ctx := b1580SelectorContext(all)
			bounded := types.CompileDynamicSelectorResolutionPaths(answerDocDynamicSelectorEvidenceCensus(ctx), "run_pipeline")
			if !b1580HasJSONCandidate(full) || !b1580HasJSONCandidate(bounded) {
				t.Fatalf("unrelated or uncitable late application must not invalidate json: full=%+v bounded=%+v", full, bounded)
			}
		})
	}
}

func b1580HasJSONCandidate(compiled types.DynamicSelectorResolutionCompilation) bool {
	for _, candidate := range compiled.Candidates {
		if candidate.SelectorLiteral == "json" && candidate.CandidateIdentity == "JsonPlugin" {
			return true
		}
	}
	return false
}

// Every uniqueness join consumes the complete same-ID-reconciled snapshot,
// not just the application that exposed the first production witness.
func TestB1580AllSelectorJoinsKeepLateCounterEvidence(t *testing.T) {
	base := b1580ProductionSelectorEvidence(t)
	for _, tc := range []struct {
		claim  types.ClaimForm
		reason types.DynamicSelectorResolutionRejectionReason
	}{
		{types.ClaimRegistrationEdge, types.DynamicSelectorRejectAmbiguousContainer},
		{types.ClaimAssignmentFact, types.DynamicSelectorRejectAmbiguousLookup},
		{types.ClaimReturnFact, types.DynamicSelectorRejectAmbiguousReturn},
		{types.ClaimCallEdge, types.DynamicSelectorRejectAmbiguousEntry},
		{types.ClaimArgumentFlow, types.DynamicSelectorRejectAmbiguousArgument},
	} {
		t.Run(string(tc.reason), func(t *testing.T) {
			var conflict types.EvidenceItem
			path := types.CompileDynamicSelectorResolutionPaths(base, "run_pipeline").Candidates[0]
			var targetID string
			for _, hop := range path.Hops {
				if hop.ClaimForm == tc.claim {
					targetID = hop.EvidenceID
					break
				}
			}
			for _, row := range base {
				if row.ID == targetID {
					conflict = row
					break
				}
			}
			if conflict.ID == "" {
				t.Fatal("fixture lacks join row")
			}
			conflict.ID += "-other-occurrence"
			if tc.claim == types.ClaimArgumentFlow {
				conflict.Subject = "other_kind" // Same source/callsite, different argument.
			} else {
				conflict.LineStart += 100
				conflict.LineEnd += 100
			}
			for _, first := range []bool{false, true} {
				all := append([]types.EvidenceItem(nil), base...)
				if first {
					all = append(all, conflict)
				}
				all = append(all, b1580CensusPadding(900)...)
				if !first {
					all = append(all, conflict)
				}
				full := types.CompileDynamicSelectorResolutionPaths(all, "run_pipeline")
				if len(full.Candidates) != 0 || len(full.Rejected) != 2 || full.Rejected[0].Reason != tc.reason {
					t.Fatalf("invalid full-census witness: %+v", full)
				}
				ctx := b1580SelectorContext(all)
				got := types.CompileDynamicSelectorResolutionPaths(answerDocDynamicSelectorEvidenceCensus(ctx), "run_pipeline")
				if !reflect.DeepEqual(got, full) {
					t.Fatalf("join lost late conflict: got=%+v want=%+v", got, full)
				}
				if capsule := renderAnswerDocDynamicSelectorResolutionCandidates(ctx, "run_pipeline"); capsule != "" {
					t.Fatalf("ambiguous path emitted: %s", capsule)
				}
				var recipes strings.Builder
				if anchors := renderAnswerDocDynamicSelectorRelationRecipes(&recipes, ctx); len(anchors) != 0 || recipes.Len() != 0 {
					t.Fatal("ambiguous recipe emitted")
				}
			}
		})
	}
}

func TestB1580CensusMergesBeforeEntryFilteringWithoutMutation(t *testing.T) {
	base := b1580ProductionSelectorEvidence(t)
	for _, lane := range []string{"turn_a", "mutable", "both"} {
		t.Run(lane, func(t *testing.T) {
			var correction types.EvidenceItem
			for _, row := range base {
				if row.ID == "E-entry" {
					correction = row
				}
			}
			correction.Subject, correction.OwnerSymbol = "other_entry", "other_entry"
			correction.Snippet = "resolve(kind)"
			ctx := b1580SelectorContext(append(append([]types.EvidenceItem(nil), base...), b1580CensusPadding(900)...))
			ctx.Mutable = types.NewMutableState("")
			if lane != "mutable" {
				ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{EvidenceItems: []types.EvidenceItem{correction}})
			}
			if lane != "turn_a" {
				ctx.Mutable.AppendEvidence([]types.EvidenceItem{correction})
			}
			before, _ := json.Marshal(ctx.EvidenceItems)
			pool := answerDocDynamicSelectorEvidenceCensus(ctx)
			for _, row := range pool {
				if row.ID == correction.ID && row.Subject != "other_entry" {
					t.Fatalf("entry filter hid same-ID correction: %+v", row)
				}
			}
			got := types.CompileDynamicSelectorResolutionPaths(pool, "run_pipeline")
			if len(got.Candidates) != 0 || len(got.Rejected) != 2 || got.Rejected[0].Reason != types.DynamicSelectorRejectEntryUnavailable {
				t.Fatalf("stale entry survived correction: %+v", got)
			}
			after, _ := json.Marshal(ctx.EvidenceItems)
			if string(before) != string(after) {
				t.Fatal("census mutated accepted evidence")
			}
		})
	}
}

func b1580CensusPadding(n int) []types.EvidenceItem {
	out := make([]types.EvidenceItem, 0, n)
	for i := 0; i < n; i++ {
		row := types.EvidenceItem{ID: fmt.Sprintf("pad-%d", i), Kind: types.EvidenceRelationship, Subject: "unrelated_value", Predicate: "assigns", Object: "unrelated_input", OwnerSymbol: "other_owner", AnchorKind: types.AnchorAssignment, Source: "other/source.go", LineStart: i + 1, LineEnd: i + 1, Snippet: "unrelated_value = unrelated_input", Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded}
		if i%3 == 0 {
			row.Subject, row.Predicate, row.Object, row.OwnerSymbol, row.AnchorKind = "run_pipeline", "calls", "unrelated_target", "run_pipeline", types.AnchorCall
		}
		out = append(out, row)
	}
	return out
}

func BenchmarkB1580CompleteCensusCompilation(b *testing.B) {
	base := b1580ProductionSelectorEvidence(b)
	for _, n := range []int{512, 4096, 32768} {
		for _, groups := range []int{1, 8, 32} {
			b.Run(fmt.Sprintf("rows=%d/groups=%d", n, groups), func(b *testing.B) {
				evidence := b1580CompleteSelectorGroups(b, base, groups)
				evidence = append(evidence, b1580CensusPadding(n-len(evidence))...)
				ctx := b1580SelectorContext(evidence)
				compiled := types.CompileDynamicSelectorResolutionPaths(answerDocDynamicSelectorEvidenceCensus(ctx), "run_pipeline")
				if len(compiled.Candidates) != groups || len(compiled.Rejected) != 0 {
					b.Fatalf("benchmark must traverse every complete group, not stop at a missing binding: %+v", compiled)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					compiled = types.CompileDynamicSelectorResolutionPaths(answerDocDynamicSelectorEvidenceCensus(ctx), "run_pipeline")
				}
				b.StopTimer()
				if len(compiled.Candidates) != groups {
					b.Fatal("benchmark lost complete candidates")
				}
			})
		}
	}
}

func b1580SelectorContext(evidence []types.EvidenceItem) *types.AgentContext {
	return &types.AgentContext{EvidenceItems: evidence, AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentTrace, PredicateAxis: types.AxisCall,
		AnalyzerHints:            types.AnalyzerHints{Kind: string(types.ReqCallChain)},
		CallChainEndpointProfile: &types.CallChainEndpointProfile{Source: "run_pipeline", SinkMode: types.CallChainSinkResolutionDiscover},
	}}}
}

func b1580PaddedEvidence(t *testing.T, base []types.EvidenceItem, conflictFirst bool, variant string) []types.EvidenceItem {
	t.Helper()
	var conflict types.EvidenceItem
	for _, item := range base {
		if item.SelectorApplication != nil && item.SelectorApplication.Literal == "json" {
			conflict = item
			selector := *item.SelectorApplication
			conflict.SelectorApplication = &selector
			break
		}
	}
	if conflict.SelectorApplication == nil {
		t.Fatal("real parser fixture did not emit json application")
	}
	// This is a hypothetical second grounded declaration; all other path facts
	// come unchanged from the current real fixture's production enrichment.
	conflict.ID = "E-late-conflicting-selector-application"
	conflict.Object, conflict.OwnerSymbol = "OtherJsonPlugin", "OtherJsonPlugin"
	conflict.Source, conflict.LineStart, conflict.LineEnd = "pipeline/extra_plugins.py", 19, 19
	switch variant {
	case "different_owner":
		conflict.SelectorApplication.Owner = "other_register"
	case "different_literal":
		conflict.SelectorApplication.Literal = "yaml"
	case "uncitable":
		conflict.GroundingStatus = types.GroundingUngrounded
	}
	basePool := answerDocDynamicSelectorEvidenceCensus(b1580SelectorContext(base))
	coreCount := 0
	for _, item := range basePool {
		if types.ClaimFormOf(item) != types.ClaimCallEdge {
			coreCount++
		}
	}
	const coreLimit = 384 // The former pre-compilation core cap.
	all := append([]types.EvidenceItem(nil), base...)
	if conflictFirst {
		all = append(all, conflict)
	}
	for i := coreCount; i < coreLimit; i++ {
		all = append(all, types.EvidenceItem{
			ID: fmt.Sprintf("E-unrelated-assignment-%d", i), Kind: types.EvidenceRelationship,
			Subject: fmt.Sprintf("padding_value_%d", i), Predicate: "assigns", Object: "unrelated_input", OwnerSymbol: "padding_worker",
			AnchorKind: types.AnchorAssignment, Source: "unrelated/padding.py", LineStart: i + 1, LineEnd: i + 1,
			Snippet: fmt.Sprintf("padding_value_%d = unrelated_input", i), Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded,
		})
	}
	if !conflictFirst {
		all = append(all, conflict)
	}
	return all
}

// Reuses the exact real Python fixture and production enrichment setup from
// TestBuildConcreteValuesSection_RealPythonSelectorFlowProducesCompleteStaticCandidates.
func b1580ProductionSelectorEvidence(t testing.TB) []types.EvidenceItem {
	t.Helper()
	repoRoot := filepath.Clean(filepath.Join("..", "..", "eval", "fixtures", "python-plugin-mro"))
	entries, err := repomap.ScanFiles(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	graph := repomap.BuildGraph(repoRoot, repomap.ParseFiles(entries, repoRoot))
	eval := runtimeTargetRelationEvaluator(graph)
	eval.structuredEvidence = []types.EvidenceItem{
		{ID: "E-entry", Kind: types.EvidenceRelationship, Subject: "run_pipeline", Predicate: "calls", Object: "resolve", OwnerSymbol: "pipeline.runner.run_pipeline", Source: "pipeline/runner.py", LineStart: 15, LineEnd: 15, Scope: types.ScopeLine, AnchorKind: types.AnchorCall, AnchorSymbol: "resolve", Snippet: "plugin = resolve(kind)", GroundingStatus: types.GroundingGrounded},
		{ID: "E-resolve-def", Kind: types.EvidenceDirect, Subject: "resolve", Predicate: "definition", OwnerSymbol: "resolve", Source: "pipeline/registry.py", LineStart: 24, LineEnd: 24, Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition, AnchorSymbol: "resolve", GroundingStatus: types.GroundingGrounded},
	}
	readSet := map[string]bool{"pipeline/runner.py": true, "pipeline/registry.py": true, "pipeline/plugins.py": true}
	closure := types.NewEvidenceClosure(repoRoot)
	closure.SetReadSet(readSet)
	closure.AddReadRanges(map[string][]types.LineRange{
		"pipeline/runner.py": {{Start: 1, End: 30}}, "pipeline/registry.py": {{Start: 1, End: 40}}, "pipeline/plugins.py": {{Start: 1, End: 40}},
	})
	got := eval.buildConcreteValuesSection(context.Background(), repoRoot, readSet, closure)
	all := mergeEvidenceItems(eval.structuredEvidence, got.evidence)
	all = append(all, types.EvidenceItem{
		ID: "E-grounded-register", Kind: types.EvidenceRegistration, Subject: "REGISTRY", Predicate: "binds", Object: "cls", OwnerSymbol: "register", Source: "pipeline/registry.py", LineStart: 17, LineEnd: 17, Scope: types.ScopeLine, AnchorKind: types.AnchorAssignment, AnchorSymbol: "REGISTRY", Snippet: "REGISTRY[name] = cls", GroundingStatus: types.GroundingGrounded,
	})
	compiled := types.CompileDynamicSelectorResolutionPaths(all, "run_pipeline")
	if len(compiled.Candidates) != 2 || len(compiled.Rejected) != 0 {
		t.Fatalf("production fixture baseline invalid: %+v", compiled)
	}
	return all
}

func TestB1580CensusMergesNonSelectorCorrectionsBeforeClaimFiltering(t *testing.T) {
	base := b1580ProductionSelectorEvidence(t)
	path := types.CompileDynamicSelectorResolutionPaths(base, "run_pipeline").Candidates[0]
	for _, tc := range []struct {
		role   types.DynamicSelectorResolutionHopRole
		reason types.DynamicSelectorResolutionRejectionReason
	}{
		{types.DynamicSelectorHopEntryCall, types.DynamicSelectorRejectEntryUnavailable},
		{types.DynamicSelectorHopFactoryReturn, types.DynamicSelectorRejectReturnUnavailable},
	} {
		for _, lane := range []string{"turn_a", "mutable", "both"} {
			t.Run(string(tc.role)+"/"+lane, func(t *testing.T) {
				var targetID string
				for _, hop := range path.Hops {
					if hop.Role == tc.role {
						targetID = hop.EvidenceID
					}
				}
				var correction types.EvidenceItem
				for _, item := range base {
					if item.ID == targetID {
						correction = item
					}
				}
				if correction.ID == "" {
					t.Fatal("fixture has no selected carrier to correct")
				}
				// Preserve the same complete grounded carrier, but correct its
				// claim to text-only. This must replace rather than preserve the
				// stale call/return merely because text-only is not a selector hop.
				correction.AnchorKind = types.AnchorTextReference
				correction.Predicate = "references"
				ctx := b1580SelectorContext(append(append([]types.EvidenceItem(nil), base...), b1580CensusPadding(900)...))
				ctx.Mutable = types.NewMutableState("")
				if lane != "mutable" {
					ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{EvidenceItems: []types.EvidenceItem{correction}})
				}
				if lane != "turn_a" {
					ctx.Mutable.AppendEvidence([]types.EvidenceItem{correction})
				}
				before, _ := json.Marshal([]any{ctx.EvidenceItems, ctx.Mutable.TurnAArtifacts(), ctx.Mutable.EmittedEvidence()})
				census := answerDocDynamicSelectorEvidenceCensus(ctx)
				seen := 0
				for _, item := range census {
					if item.ID == correction.ID {
						seen++
						if types.ClaimFormOf(item) != types.ClaimTextReferenceFact {
							t.Fatalf("same-ID non-selector correction was hidden: %+v", item)
						}
					}
				}
				if seen != 1 {
					t.Fatalf("corrected identity must appear exactly once, got %d", seen)
				}
				compiled := types.CompileDynamicSelectorResolutionPaths(census, "run_pipeline")
				if len(compiled.Candidates) != 0 || len(compiled.Rejected) != 2 {
					t.Fatalf("text-only correction must remove the stale complete paths: %+v", compiled)
				}
				for _, rejected := range compiled.Rejected {
					if rejected.Reason != tc.reason {
						t.Fatalf("compiler must remain the owner of rejection semantics: %+v", rejected)
					}
				}
				if got := renderAnswerDocDynamicSelectorResolutionCandidates(ctx, "run_pipeline"); got != "" {
					t.Fatalf("stale candidate rendered after correction: %s", got)
				}
				var recipes strings.Builder
				if got := renderAnswerDocDynamicSelectorRelationRecipes(&recipes, ctx); len(got) != 0 || recipes.Len() != 0 {
					t.Fatal("stale relation recipes rendered after correction")
				}
				after, _ := json.Marshal([]any{ctx.EvidenceItems, ctx.Mutable.TurnAArtifacts(), ctx.Mutable.EmittedEvidence()})
				if string(before) != string(after) {
					t.Fatal("census/renderer must not mutate any accepted evidence lane")
				}
			})
		}
	}
}

func TestB1580CompleteCensusRetainsBoundedDisplayAndOmissionDisclosure(t *testing.T) {
	evidence := b1580CompleteSelectorGroups(t, b1580ProductionSelectorEvidence(t), 5)
	ctx := b1580SelectorContext(evidence)
	census := answerDocDynamicSelectorEvidenceCensus(ctx)
	compiled := types.CompileDynamicSelectorResolutionPaths(census, "run_pipeline")
	if len(compiled.Candidates) != 5 || len(compiled.Rejected) != 0 {
		t.Fatalf("display fixture must hold five complete candidates: %+v", compiled)
	}
	relationCount := 0
	for _, candidate := range compiled.Candidates {
		for _, hop := range candidate.Hops {
			if hop.RelationKind.IsValid() {
				relationCount++
			}
		}
	}
	if relationCount != 25 {
		t.Fatalf("five shared-skeleton candidates must carry 25 core relations, got %d", relationCount)
	}
	before, _ := json.Marshal(evidence)
	capsule := renderAnswerDocDynamicSelectorResolutionCandidates(ctx, "run_pipeline")
	if got := strings.Count(capsule, "- candidate `"); got != 4 {
		t.Fatalf("candidate display must remain capped at four, got %d: %s", got, capsule)
	}
	for _, want := range []string{
		"`1` additional complete candidates are omitted only from this bounded prompt view",
		"do not claim uniqueness from the displayed subset",
		"not proof of the runtime-selected implementation",
	} {
		if !strings.Contains(capsule, want) {
			t.Errorf("candidate display lacks %q: %s", want, capsule)
		}
	}
	var recipes strings.Builder
	anchors := renderAnswerDocDynamicSelectorRelationRecipes(&recipes, ctx)
	if len(anchors) != 24 || strings.Count(recipes.String(), "dynamic_candidate_edge_recipe[") != 24 {
		t.Fatalf("relation display must remain capped at 24 of the 25 compiled hops: anchors=%d\n%s", len(anchors), recipes.String())
	}
	for _, want := range []string{
		"Additional dynamic candidate relation recipes were omitted only from this bounded authoring view",
		"do not infer uniqueness or absence from the displayed subset",
		"candidate-only permissions, not a runtime-selection conclusion",
	} {
		if !strings.Contains(recipes.String(), want) {
			t.Errorf("relation display lacks %q: %s", want, recipes.String())
		}
	}
	after, _ := json.Marshal(evidence)
	if string(before) != string(after) || !reflect.DeepEqual(compiled, types.CompileDynamicSelectorResolutionPaths(answerDocDynamicSelectorEvidenceCensus(ctx), "run_pipeline")) {
		t.Fatal("display limits must not mutate evidence or the complete compiler result")
	}
}

// Keep the five real core relation carriers shared by every group. Only the
// declaration application is replicated into distinct hypothetical literals
// and candidate identities, so every group reaches every compiler join. No
// optional callback/type inventory is needed to exercise this N×G bound.
func b1580CompleteSelectorGroups(t testing.TB, base []types.EvidenceItem, groups int) []types.EvidenceItem {
	t.Helper()
	compiled := types.CompileDynamicSelectorResolutionPaths(base, "run_pipeline")
	if groups <= 0 || len(compiled.Candidates) == 0 {
		t.Fatal("complete selector group fixture requires a real baseline and positive group count")
	}
	byID := make(map[string]types.EvidenceItem, len(base))
	for _, item := range base {
		byID[item.ID] = item
	}
	var out []types.EvidenceItem
	var application types.EvidenceItem
	for _, hop := range compiled.Candidates[0].Hops {
		item, ok := byID[hop.EvidenceID]
		if !ok {
			t.Fatalf("real fixture lost core hop %s", hop.EvidenceID)
		}
		if hop.Role == types.DynamicSelectorHopSelectorApplication {
			application = item
		} else {
			out = append(out, item)
		}
	}
	if len(out) != 5 || application.SelectorApplication == nil {
		t.Fatalf("expected five real shared relations plus a declaration: core=%d app=%+v", len(out), application)
	}
	for i := 0; i < groups; i++ {
		item := application
		selector := *application.SelectorApplication
		selector.Literal = fmt.Sprintf("format-%03d", i)
		item.ID = fmt.Sprintf("app-group-%03d", i)
		item.Object = fmt.Sprintf("Candidate%03d", i)
		item.OwnerSymbol = item.Object
		item.SelectorApplication = &selector
		out = append(out, item)
	}
	return out
}
