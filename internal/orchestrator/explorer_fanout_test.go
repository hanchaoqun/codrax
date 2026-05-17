package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Helper: build a minimal AnalysisIR with the given sub-topics and
// predicates. Caller controls every field that matters to the gate.
func buildFanoutTestIR(subTopics []types.SubTopic, preds types.SemanticPredicates) *types.AnalysisIR {
	return &types.AnalysisIR{
		RequestModel: types.RequestModel{
			SubTopics:  subTopics,
			Predicates: preds,
		},
	}
}

// TestExplorerSubTopicFanoutEligible_PositiveBaseline pins the
// canonical happy path: 2 disjoint sub-topics with IsCrossComponent
// → gate fires.
func TestExplorerSubTopicFanoutEligible_PositiveBaseline(t *testing.T) {
	ir := buildFanoutTestIR(
		[]types.SubTopic{
			{
				Summary:  "codrax hallucination prevention",
				Entities: []string{"SelfConsistencyReviewer", "gate.Run", "violRegistry"},
			},
			{
				Summary:  "opencode hallucination prevention",
				Entities: []string{"ApplyPatchTool", "ShellTool", "assertExternalDirectoryEffect"},
			},
		},
		types.SemanticPredicates{IsCrossComponent: true},
	)
	if !explorerSubTopicFanoutEligible(ir) {
		t.Fatal("disjoint cross-component sub-topics should be eligible")
	}
}

// TestExplorerSubTopicFanoutEligible_NilIR confirms nil-safety.
func TestExplorerSubTopicFanoutEligible_NilIR(t *testing.T) {
	if explorerSubTopicFanoutEligible(nil) {
		t.Error("nil AnalysisIR must not be eligible")
	}
}

// TestExplorerSubTopicFanoutEligible_SingleSubTopic pins the
// cardinality floor: 1 sub-topic → no parallelism possible.
func TestExplorerSubTopicFanoutEligible_SingleSubTopic(t *testing.T) {
	ir := buildFanoutTestIR(
		[]types.SubTopic{
			{Summary: "single topic", Entities: []string{"Foo", "Bar"}},
		},
		types.SemanticPredicates{IsCrossComponent: true},
	)
	if explorerSubTopicFanoutEligible(ir) {
		t.Error("single sub-topic must not be eligible")
	}
}

// TestExplorerSubTopicFanoutEligible_TooManySubTopics pins the
// cardinality ceiling: > 8 sub-topics would trip the SubAgentValidator's
// maxSubTasks reject anyway.
func TestExplorerSubTopicFanoutEligible_TooManySubTopics(t *testing.T) {
	subTopics := make([]types.SubTopic, 0, 9)
	for i := 0; i < 9; i++ {
		subTopics = append(subTopics, types.SubTopic{
			Summary:  "topic",
			Entities: []string{strings.Repeat("a", i+1)},
		})
	}
	ir := buildFanoutTestIR(subTopics, types.SemanticPredicates{IsCrossComponent: true})
	if explorerSubTopicFanoutEligible(ir) {
		t.Error("> 8 sub-topics must NOT be eligible (validator would reject)")
	}
}

// TestExplorerSubTopicFanoutEligible_PredicateShapeRequired pins the
// shape gate: a 2-sub-topic decomposition without IsCrossComponent or
// IsCategoryEnumeration is NOT a parallelisable shape.
func TestExplorerSubTopicFanoutEligible_PredicateShapeRequired(t *testing.T) {
	ir := buildFanoutTestIR(
		[]types.SubTopic{
			{Summary: "a", Entities: []string{"A1", "A2"}},
			{Summary: "b", Entities: []string{"B1", "B2"}},
		},
		types.SemanticPredicates{},
	)
	if explorerSubTopicFanoutEligible(ir) {
		t.Error("absent cross-component/enumeration predicates → not eligible")
	}
}

// TestExplorerSubTopicFanoutEligible_CategoryEnumerationAlsoFires
// pins that IsCategoryEnumeration is an alternative shape trigger.
func TestExplorerSubTopicFanoutEligible_CategoryEnumerationAlsoFires(t *testing.T) {
	ir := buildFanoutTestIR(
		[]types.SubTopic{
			{Summary: "cat-a", Entities: []string{"CatA"}},
			{Summary: "cat-b", Entities: []string{"CatB"}},
		},
		types.SemanticPredicates{IsCategoryEnumeration: true},
	)
	if !explorerSubTopicFanoutEligible(ir) {
		t.Error("IsCategoryEnumeration should be a valid shape trigger")
	}
}

// TestExplorerSubTopicFanoutEligible_ScalarCountCarveOut pins the
// negative carve-out: "how many X" questions are never decomposable.
func TestExplorerSubTopicFanoutEligible_ScalarCountCarveOut(t *testing.T) {
	ir := buildFanoutTestIR(
		[]types.SubTopic{
			{Summary: "a", Entities: []string{"A"}},
			{Summary: "b", Entities: []string{"B"}},
		},
		types.SemanticPredicates{
			IsCrossComponent: true,
			IsScalarAnswer:   true,
			IsCountQuestion:  true,
		},
	)
	if explorerSubTopicFanoutEligible(ir) {
		t.Error("scalar count question must NOT be decomposable")
	}
}

// TestExplorerSubTopicFanoutEligible_OverlapBlocks pins entity
// disjointness: overlapping entity sets (NormalizeCodeKey-equivalent)
// must veto the gate so the append-only reducer does not double-emit
// near-duplicate evidence.
func TestExplorerSubTopicFanoutEligible_OverlapBlocks(t *testing.T) {
	// "selfConsistencyReviewer" and "SelfConsistencyReviewer" collapse
	// to the same NormalizeCodeKey.
	ir := buildFanoutTestIR(
		[]types.SubTopic{
			{Summary: "a", Entities: []string{"selfConsistencyReviewer", "FooA"}},
			{Summary: "b", Entities: []string{"SelfConsistencyReviewer", "FooB"}},
		},
		types.SemanticPredicates{IsCrossComponent: true},
	)
	if explorerSubTopicFanoutEligible(ir) {
		t.Error("overlapping entity sets must block fan-out")
	}
}

// TestExplorerSubTopicFanoutEligible_HyphenUnderscoreEquivalent pins
// that NormalizeCodeKey treats hyphens / underscores as equivalent so
// 'self-consistency' and 'self_consistency' overlap.
func TestExplorerSubTopicFanoutEligible_HyphenUnderscoreEquivalent(t *testing.T) {
	ir := buildFanoutTestIR(
		[]types.SubTopic{
			{Summary: "a", Entities: []string{"self-consistency"}},
			{Summary: "b", Entities: []string{"self_consistency"}},
		},
		types.SemanticPredicates{IsCrossComponent: true},
	)
	if explorerSubTopicFanoutEligible(ir) {
		t.Error("hyphen/underscore variants must collide and block fan-out")
	}
}

// TestExplorerSubTopicFanoutEligible_EmptySubTopic pins the per-topic
// shape gate: a sub-topic with neither Entities nor Scopes cannot
// assemble a valid SubAgentRequest.
func TestExplorerSubTopicFanoutEligible_EmptySubTopic(t *testing.T) {
	ir := buildFanoutTestIR(
		[]types.SubTopic{
			{Summary: "valid", Entities: []string{"A"}},
			{Summary: "empty"}, // no Entities and no Scopes
		},
		types.SemanticPredicates{IsCrossComponent: true},
	)
	if explorerSubTopicFanoutEligible(ir) {
		t.Error("sub-topic with no entities and no scopes must block fan-out")
	}
}

// TestExplorerSubTopicFanoutEligible_ScopesOnlyTopicOK pins that a
// sub-topic with empty Entities but non-empty Scopes still satisfies
// the per-topic shape gate (scope_projection populated Scopes from
// active sub-repo matches).
func TestExplorerSubTopicFanoutEligible_ScopesOnlyTopicOK(t *testing.T) {
	ir := buildFanoutTestIR(
		[]types.SubTopic{
			{Summary: "a", Scopes: []string{"codrax"}},
			{Summary: "b", Scopes: []string{"opencode"}},
		},
		types.SemanticPredicates{IsCrossComponent: true},
	)
	if !explorerSubTopicFanoutEligible(ir) {
		t.Error("scopes-only sub-topics with cross-component shape should be eligible")
	}
}

// TestBuildSystemSubAgentProposal_EmitsExplorerSubAgent pins the
// proposal shape: SubAgent=="explorer" (SubExplorer.Name()),
// Scope[] non-empty, Objective filled from Summary.
func TestBuildSystemSubAgentProposal_EmitsExplorerSubAgent(t *testing.T) {
	ir := buildFanoutTestIR(
		[]types.SubTopic{
			{Summary: "explore codrax", Entities: []string{"SelfConsistencyReviewer"}},
			{Summary: "explore opencode", Entities: []string{"ApplyPatchTool"}},
		},
		types.SemanticPredicates{IsCrossComponent: true},
	)
	proposal := buildSystemSubAgentProposal(ir)
	if proposal == nil {
		t.Fatal("expected non-nil proposal for eligible input")
	}
	if len(proposal.SubTasks) != 2 {
		t.Fatalf("expected 2 sub-tasks, got %d", len(proposal.SubTasks))
	}
	for i, st := range proposal.SubTasks {
		if st.SubAgent != "explorer" {
			t.Errorf("sub_task[%d].SubAgent = %q, want %q", i, st.SubAgent, "explorer")
		}
		if st.ID == "" {
			t.Errorf("sub_task[%d].ID must be non-empty", i)
		}
		if len(st.Scope) == 0 {
			t.Errorf("sub_task[%d].Scope must be non-empty (validator hard-rejects empty)", i)
		}
		if st.Objective == "" {
			t.Errorf("sub_task[%d].Objective must be non-empty (validator hard-rejects empty)", i)
		}
	}
}

// TestBuildSystemSubAgentProposal_PrefersScopesOverEntities pins the
// scope-assembly priority: Scopes wins when non-empty, Entities fall-back
// when Scopes is empty.
func TestBuildSystemSubAgentProposal_PrefersScopesOverEntities(t *testing.T) {
	ir := buildFanoutTestIR(
		[]types.SubTopic{
			{Summary: "a", Entities: []string{"EntA"}, Scopes: []string{"scope-a"}},
			{Summary: "b", Entities: []string{"EntB"}}, // scopes empty
		},
		types.SemanticPredicates{IsCrossComponent: true},
	)
	proposal := buildSystemSubAgentProposal(ir)
	if proposal == nil {
		t.Fatal("expected non-nil proposal")
	}
	if got, want := proposal.SubTasks[0].Scope[0], "scope-a"; got != want {
		t.Errorf("sub_task[0].Scope[0] = %q, want %q (Scopes should win)", got, want)
	}
	if got, want := proposal.SubTasks[1].Scope[0], "EntB"; got != want {
		t.Errorf("sub_task[1].Scope[0] = %q, want %q (Entities fall-back)", got, want)
	}
}

// TestBuildSystemSubAgentProposal_DedupesScopesAndEntities pins the
// dedupedNonEmpty helper integration.
func TestBuildSystemSubAgentProposal_DedupesScopesAndEntities(t *testing.T) {
	ir := buildFanoutTestIR(
		[]types.SubTopic{
			{Summary: "a", Entities: []string{"A", "A", " ", "B"}},
			{Summary: "b", Entities: []string{"C"}},
		},
		types.SemanticPredicates{IsCrossComponent: true},
	)
	proposal := buildSystemSubAgentProposal(ir)
	if proposal == nil {
		t.Fatal("expected non-nil proposal")
	}
	scope0 := proposal.SubTasks[0].Scope
	if len(scope0) != 2 {
		t.Errorf("expected deduped scope length 2 ([A, B]), got %d (%v)", len(scope0), scope0)
	}
}

// TestBuildSystemSubAgentProposal_NilWhenIneligible pins the
// composability rule: builder returns nil when the input cannot
// produce a valid proposal, so the dispatch helper falls back
// transparently.
func TestBuildSystemSubAgentProposal_NilWhenIneligible(t *testing.T) {
	// IR with only 1 sub-topic (below cardinality floor for the builder).
	ir := buildFanoutTestIR(
		[]types.SubTopic{
			{Summary: "alone", Entities: []string{"A"}},
		},
		types.SemanticPredicates{IsCrossComponent: true},
	)
	if got := buildSystemSubAgentProposal(ir); got != nil {
		t.Errorf("single sub-topic must produce nil proposal, got %+v", got)
	}
}

// TestExplorerSubTopicFanoutEligible_AllNegativeShapes confirms each
// individual condition vetoes the gate independently. Belt-and-braces:
// guards against future refactors silently relaxing one branch.
func TestExplorerSubTopicFanoutEligible_AllNegativeShapes(t *testing.T) {
	cases := []struct {
		name string
		ir   *types.AnalysisIR
	}{
		{
			name: "nil_ir",
			ir:   nil,
		},
		{
			name: "single_sub_topic",
			ir: buildFanoutTestIR(
				[]types.SubTopic{{Entities: []string{"A"}}},
				types.SemanticPredicates{IsCrossComponent: true},
			),
		},
		{
			name: "no_predicate_shape",
			ir: buildFanoutTestIR(
				[]types.SubTopic{
					{Entities: []string{"A"}},
					{Entities: []string{"B"}},
				},
				types.SemanticPredicates{},
			),
		},
		{
			name: "scalar_count_carve_out",
			ir: buildFanoutTestIR(
				[]types.SubTopic{
					{Entities: []string{"A"}},
					{Entities: []string{"B"}},
				},
				types.SemanticPredicates{
					IsCrossComponent: true,
					IsScalarAnswer:   true,
					IsCountQuestion:  true,
				},
			),
		},
		{
			name: "overlap_blocks",
			ir: buildFanoutTestIR(
				[]types.SubTopic{
					{Entities: []string{"Foo"}},
					{Entities: []string{"foo"}},
				},
				types.SemanticPredicates{IsCrossComponent: true},
			),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if explorerSubTopicFanoutEligible(c.ir) {
				t.Errorf("case %q must be ineligible", c.name)
			}
		})
	}
}
