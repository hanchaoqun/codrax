package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// (TestRankChainsBySubject_PrefersMatchingTerminalKind removed in
// the Session 11 over-fitting audit: the test relied on the
// now-deleted SubjectSkillName kind + the judgeSkillName "-skill"
// suffix heuristic to prove "skill chain beats agent chain". Both
// scaffolds are gone. The general "subject-matching chain beats
// subject-mismatching chain" invariant is still covered at the
// generic-kind level — see Score tests in subject/taxonomy_test.go
// for FunctionName/TypeName/ConfigKey kind discrimination.)

// SubjectUnknown disables ranking — insertion order preserved.
func TestRankChainsBySubject_UnknownSubject_PreservesOrder(t *testing.T) {
	chains := []string{"first", "second", "third"}
	anchors := []chainAnchorInfo{
		{Summary: "first"},
		{Summary: "second"},
		{Summary: "third"},
	}
	gotChains, _ := rankChainsBySubject(chains, anchors, types.AnswerSubject{Kind: types.SubjectUnknown}, nil, nil)
	for i, want := range chains {
		if gotChains[i] != want {
			t.Errorf("position %d: got %q, want %q", i, gotChains[i], want)
		}
	}
}

// Empty / single-element inputs short-circuit cleanly.
func TestRankChainsBySubject_EdgeCases(t *testing.T) {
	if got, _ := rankChainsBySubject(nil, nil, types.AnswerSubject{Kind: types.SubjectFunctionName}, nil, nil); got != nil {
		t.Errorf("nil input must return nil, got %v", got)
	}
	one := []string{"only-chain"}
	oneAnchor := []chainAnchorInfo{{Summary: "only-chain"}}
	got, _ := rankChainsBySubject(one, oneAnchor, types.AnswerSubject{Kind: types.SubjectFunctionName}, nil, nil)
	if len(got) != 1 || got[0] != "only-chain" {
		t.Errorf("single-element input returned %v", got)
	}
}

// TestRankChainsBySubject_TopicalRelevanceBreaksGenericTie:
// When Kind==SubjectGeneric, Score returns 0.5 uniformly so subject
// match cannot differentiate chains. EntityAxes token overlap is the
// tiebreaker. The 2026-04-19 regression had `checkRequirementSatisfaction`
// (off-topic, zero overlap) tying with `RegisterDefaultSubAgents binds
// NewSubExplorer` (on-topic, full overlap) on a question about
// explorer→subagent — the off-topic chain leaked into citations.
func TestRankChainsBySubject_TopicalRelevanceBreaksGenericTie(t *testing.T) {
	offTopic := "checkRequirementSatisfaction updates ERM status"
	onTopic := "RegisterDefaultSubAgents binds NewSubExplorer"
	chains := []string{offTopic, onTopic}
	anchors := []chainAnchorInfo{
		{Summary: offTopic, Files: []string{"internal/agent/explorer_erm.go"}},
		{Summary: onTopic, Files: []string{"internal/agent/subagent.go"}},
	}
	subject := types.AnswerSubject{
		Kind:       types.SubjectGeneric,
		Confidence: 0.9,
		EntityAxes: []string{"explorer → subagent"},
	}
	got, _ := rankChainsBySubject(chains, anchors, subject, nil, nil)
	if got[0] != onTopic {
		t.Errorf("expected on-topic chain first, got order: %v", got)
	}
}

// Zero-overlap chain drops below a high-overlap chain even with
// identical baseRank — topical relevance multiplier wins.
func TestRankChainsBySubject_ZeroOverlapPenalty(t *testing.T) {
	zero := "unrelated discussion of foo and bar"
	full := "explorer invokes subagent via registry"
	chains := []string{zero, full} // zero has higher baseRank (inserted first)
	anchors := []chainAnchorInfo{
		{Summary: zero},
		{Summary: full},
	}
	subject := types.AnswerSubject{
		Kind:       types.SubjectGeneric,
		Confidence: 0.9,
		EntityAxes: []string{"explorer", "subagent"},
	}
	got, _ := rankChainsBySubject(chains, anchors, subject, nil, nil)
	if got[0] != full {
		t.Errorf("full-overlap chain should lead, got order: %v", got)
	}
}

// Empty EntityAxes → relevance multiplier is 1 for every chain, so
// ordering falls back to subject-match-only behaviour (preserved).
func TestRankChainsBySubject_EmptyAxes_NoPenalty(t *testing.T) {
	chains := []string{"first", "second"}
	anchors := []chainAnchorInfo{{Summary: "first"}, {Summary: "second"}}
	subject := types.AnswerSubject{
		Kind:       types.SubjectGeneric,
		Confidence: 0.9,
		EntityAxes: nil,
	}
	got, _ := rankChainsBySubject(chains, anchors, subject, nil, nil)
	if got[0] != "first" {
		t.Errorf("empty axes: insertion order must hold, got: %v", got)
	}
}

func TestExtractChainTopicTokens(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"arrow splits", []string{"explorer → subagent"}, []string{"explorer", "subagent"}},
		{"ascii arrow splits", []string{"A -> B"}, nil}, // single chars filtered by len>=3
		{"multi-axis", []string{"explorer → subagent", "registry → binder"}, []string{"explorer", "subagent", "registry", "binder"}},
		{"short tokens dropped", []string{"X → Y → Z"}, nil},
		{"lowercase normalized", []string{"Explorer → SubAgent"}, []string{"explorer", "subagent"}},
		{"empty", nil, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extractChainTopicTokens(c.in)
			for _, w := range c.want {
				if !got[w] {
					t.Errorf("missing token %q; got %v", w, got)
				}
			}
			if len(got) != len(c.want) {
				t.Errorf("token count = %d, want %d (got=%v)", len(got), len(c.want), got)
			}
		})
	}
}
