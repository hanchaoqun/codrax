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
