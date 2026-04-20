package agent

import (
	"reflect"
	"sort"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestIRDomainHints_NilContextOrIR(t *testing.T) {
	if got := irDomainHints(nil); got != nil {
		t.Fatalf("nil ctx should return nil, got %v", got)
	}
	ctx := &types.AgentContext{}
	if got := irDomainHints(ctx); got != nil {
		t.Fatalf("nil AnalysisIR should return nil, got %v", got)
	}
}

func TestIRDomainHints_OnlyTermSymbolsContribute(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				TermGraph: types.TermGraph{
					Canonical: []types.CanonicalTerm{
						{ID: "code:blob", Kind: types.TermSymbol, Domain: "types"},
						{ID: "en:config", Kind: types.TermConcept, Domain: "config"},
						{ID: "code:agent", Kind: types.TermSymbol, Domain: "agent"},
						{ID: "zh:explain", Kind: types.TermConcept, Domain: ""},
					},
				},
			},
		},
	}
	got := irDomainHints(ctx)
	sort.Strings(got)
	want := []string{"agent", "types"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected only TermSymbol domains, got %v", got)
	}
}

func TestIRDomainHints_DedupSameDomain(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				TermGraph: types.TermGraph{
					Canonical: []types.CanonicalTerm{
						{ID: "code:a", Kind: types.TermSymbol, Domain: "agent"},
						{ID: "code:b", Kind: types.TermSymbol, Domain: "agent"},
						{ID: "code:c", Kind: types.TermSymbol, Domain: "agent"},
					},
				},
			},
		},
	}
	got := irDomainHints(ctx)
	if len(got) != 1 || got[0] != "agent" {
		t.Fatalf("expected single dedup'd domain, got %v", got)
	}
}

func TestIRDomainHints_EmptyDomainSkipped(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				TermGraph: types.TermGraph{
					Canonical: []types.CanonicalTerm{
						{ID: "code:a", Kind: types.TermSymbol, Domain: ""},
					},
				},
			},
		},
	}
	if got := irDomainHints(ctx); got != nil {
		t.Fatalf("empty-Domain TermSymbol should not produce hint, got %v", got)
	}
}
