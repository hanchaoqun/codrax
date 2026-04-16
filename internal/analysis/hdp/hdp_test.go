package hdp

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/binder"
	"github.com/hanchaoqun/codrax/internal/analysis/priority"
	"github.com/hanchaoqun/codrax/internal/types"
)

func rm(intent types.Intent, symbols ...string) types.RequestModel {
	out := types.RequestModel{Intent: intent}
	for _, s := range symbols {
		out.TermGraph.Canonical = append(out.TermGraph.Canonical, types.CanonicalTerm{
			ID: "code:" + strings.ToLower(s), Surface: s,
			Kind: types.TermSymbol, Confidence: 0.9,
		})
	}
	return out
}

func findByStatement(hs []types.Hypothesis, needle string) *types.Hypothesis {
	for i := range hs {
		if strings.Contains(hs[i].Statement, needle) {
			return &hs[i]
		}
	}
	return nil
}

func TestPlan_RootCause_PrioritizesTopSymbol(t *testing.T) {
	hs := Plan(rm(types.IntentRootCause, "Explorer", "ShouldStop"))
	if len(hs) == 0 {
		t.Fatal("expected at least one hypothesis")
	}
	h := findByStatement(hs, "Explorer")
	if h == nil {
		t.Fatalf("expected hypothesis mentioning Explorer; got %+v", hs)
	}
	// Priority is computed by the priority package; unpack via
	// priority.Raw. A root_cause seed with strong intent match
	// should clear 30 in the human-scale range.
	if priority.Raw(h.Priority) < 30 {
		t.Fatalf("root_cause seed hypothesis should have raw priority ≥30; got %d", priority.Raw(h.Priority))
	}
	if h.Status != types.HypUnknown {
		t.Fatalf("fresh hypotheses must be unknown; got %q", h.Status)
	}
}

func TestPlan_Ambiguity_ProducesHypothesisPerClause(t *testing.T) {
	input := rm(types.IntentExplain, "Foo")
	input.Ambiguities = []types.Ambiguity{
		{Clause: "stop", Options: []string{"soft", "hard"}, Resolution: "both"},
		{Clause: "cache", Options: []string{"memory", "disk"}},
	}
	hs := Plan(input)
	clauseHits := 0
	for _, h := range hs {
		if strings.Contains(h.Statement, `"stop"`) || strings.Contains(h.Statement, `"cache"`) {
			clauseHits++
		}
	}
	if clauseHits != 2 {
		t.Fatalf("expected 2 clause-driven hypotheses; got %d (full=%+v)", clauseHits, hs)
	}
}

func TestPlan_HighSecurity_AddsUntrustedPathHypothesis(t *testing.T) {
	input := rm(types.IntentExplain, "Auth")
	input.RiskMatrix.Security.Level = 4
	hs := Plan(input)
	if findByStatement(hs, "un-sanitized") == nil {
		t.Fatalf("security≥3 must add untrusted-path hypothesis; got %+v", hs)
	}
}

func TestPlan_HighDataIntegrity_AddsInvariantHypothesis(t *testing.T) {
	input := rm(types.IntentRootCause, "Migration")
	input.RiskMatrix.DataIntegrity.Level = 5
	hs := Plan(input)
	if findByStatement(hs, "data invariants") == nil {
		t.Fatalf("data_integrity≥3 must add invariant hypothesis; got %+v", hs)
	}
}

func TestPlan_NeverEmpty(t *testing.T) {
	hs := Plan(types.RequestModel{})
	if len(hs) == 0 {
		t.Fatalf("Plan must never return an empty hypothesis set")
	}
}

func TestBinder_AttachesToEvidenceAndValidate(t *testing.T) {
	tg := types.TaskGraph{Nodes: []types.TaskNode{
		{ID: "n0", Type: types.NodeProbe},
		{ID: "n1", Type: types.NodeEvidence, Objective: "collect evidence for h1"},
		{ID: "n2", Type: types.NodeValidate, Objective: "validate h2"},
		{ID: "n3", Type: types.NodeFinalize, Objective: "render answer"},
	}}
	hs := []types.Hypothesis{{ID: "h1", Statement: "evidence claim"}, {ID: "h2", Statement: "validate claim"}}
	if err := binder.BindByRelevance(&tg, hs, binder.Options{}); err != nil {
		t.Fatalf("binder: %v", err)
	}
	if len(tg.Nodes[0].Hypotheses) != 0 {
		t.Fatalf("probe nodes must not be bound; got %+v", tg.Nodes[0].Hypotheses)
	}
	if len(tg.Nodes[1].Hypotheses) == 0 || len(tg.Nodes[2].Hypotheses) == 0 || len(tg.Nodes[3].Hypotheses) == 0 {
		t.Fatalf("evidence/validate/finalize must be bound; got %+v", tg.Nodes)
	}
}

func TestBinder_PreservesExistingBindings(t *testing.T) {
	tg := types.TaskGraph{Nodes: []types.TaskNode{
		{ID: "n1", Type: types.NodeEvidence, Hypotheses: []string{"h_preexisting"}},
		{ID: "n2", Type: types.NodeValidate, Objective: "validate"},
	}}
	if err := binder.BindByRelevance(&tg, []types.Hypothesis{{ID: "h1", Statement: "validate claim"}}, binder.Options{}); err != nil {
		t.Fatalf("binder: %v", err)
	}
	if tg.Nodes[0].Hypotheses[0] != "h_preexisting" {
		t.Fatalf("binder must not overwrite existing hypothesis list; got %+v", tg.Nodes[0].Hypotheses)
	}
	if len(tg.Nodes[1].Hypotheses) == 0 {
		t.Fatalf("binder must attach to unbound nodes; got %+v", tg.Nodes[1].Hypotheses)
	}
}

func TestValidate_FlagsUnboundNodes(t *testing.T) {
	tg := types.TaskGraph{Nodes: []types.TaskNode{
		{ID: "probe", Type: types.NodeProbe},
		{ID: "ev", Type: types.NodeEvidence},
		{ID: "val", Type: types.NodeValidate, Hypotheses: []string{"h1"}},
		{ID: "fin", Type: types.NodeFinalize},
	}}
	missing := Validate(tg)
	if len(missing) != 2 {
		t.Fatalf("expected 2 missing bindings (ev, fin); got %v", missing)
	}
	wantSet := map[string]bool{"ev": true, "fin": true}
	for _, id := range missing {
		if !wantSet[id] {
			t.Fatalf("unexpected missing node %q", id)
		}
	}
}

func TestValidate_AllBoundPassesEmpty(t *testing.T) {
	tg := types.TaskGraph{Nodes: []types.TaskNode{
		{ID: "probe", Type: types.NodeProbe},
		{ID: "ev", Type: types.NodeEvidence, Hypotheses: []string{"h1"}},
		{ID: "fin", Type: types.NodeFinalize, Hypotheses: []string{"h1"}},
	}}
	if len(Validate(tg)) != 0 {
		t.Fatalf("fully bound graph should pass; got %+v", Validate(tg))
	}
}

func TestPlan_Deterministic(t *testing.T) {
	a := Plan(rm(types.IntentRootCause, "Foo", "Bar"))
	b := Plan(rm(types.IntentRootCause, "Foo", "Bar"))
	if len(a) != len(b) {
		t.Fatalf("non-deterministic length: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].Statement != b[i].Statement {
			t.Fatalf("non-deterministic hypothesis at %d: %+v vs %+v", i, a[i], b[i])
		}
	}
}
