package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestRankChainsBySubject_PrefersMatchingTerminalKind is the CGEC C2
// regression. Two chains compete: one terminal is "explore-skill"
// (a SkillName), the other is "explorer" (an AgentName, NOT a
// skill). When the question's expected AnswerSubject is SkillName,
// the SkillName chain MUST rank above the AgentName chain even
// though it was inserted second.
func TestRankChainsBySubject_PrefersMatchingTerminalKind(t *testing.T) {
	skillChain := "`Register()` registers `explore-skill`"
	agentChain := "`SubExplorer.Name()` returns \"explorer\""
	chains := []string{
		// agentChain inserted first → wins under unranked insertion
		// order. After CGEC C2 ranking with expected=SkillName,
		// skillChain must overtake it.
		agentChain,
		skillChain,
	}
	anchors := []chainAnchorInfo{
		{Summary: agentChain, Files: []string{"internal/agent/subagent.go"}},
		{Summary: skillChain, Files: []string{"internal/skill/defaults.go"}},
	}
	graph := &repomap.Graph{
		SymbolDefs: map[string][]*repomap.Symbol{
			"explore-skill": {
				&repomap.Symbol{Name: "explore-skill", Kind: "var", File: "internal/skill/defaults.go", Line: 14},
			},
			"explorer": {
				&repomap.Symbol{Name: "explorer", Kind: "var", File: "internal/agent/explorer.go", Line: 10},
			},
		},
	}
	expected := types.AnswerSubject{Kind: types.SubjectSkillName, Confidence: 0.8}

	gotChains, gotAnchors := rankChainsBySubject(chains, anchors, expected, graph, nil)

	if len(gotChains) != 2 || len(gotAnchors) != 2 {
		t.Fatalf("ranking lost entries: chains=%d anchors=%d", len(gotChains), len(gotAnchors))
	}
	if gotChains[0] != skillChain {
		t.Errorf("expected skillChain to rank first, got %q", gotChains[0])
	}
	if gotChains[1] != agentChain {
		t.Errorf("expected agentChain to rank second, got %q", gotChains[1])
	}
	// Anchors must move with their chains (parallel array contract).
	if gotAnchors[0].Summary != skillChain {
		t.Errorf("anchor[0] desync — got Summary=%q", gotAnchors[0].Summary)
	}
	if gotAnchors[1].Summary != agentChain {
		t.Errorf("anchor[1] desync — got Summary=%q", gotAnchors[1].Summary)
	}
}

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
	if got, _ := rankChainsBySubject(nil, nil, types.AnswerSubject{Kind: types.SubjectSkillName}, nil, nil); got != nil {
		t.Errorf("nil input must return nil, got %v", got)
	}
	one := []string{"only-chain"}
	oneAnchor := []chainAnchorInfo{{Summary: "only-chain"}}
	got, _ := rankChainsBySubject(one, oneAnchor, types.AnswerSubject{Kind: types.SubjectSkillName}, nil, nil)
	if len(got) != 1 || got[0] != "only-chain" {
		t.Errorf("single-element input returned %v", got)
	}
}
