package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestReconcileComplexity covers every rule in the catalogue + the
// identity case (no rule fires, declared returned unchanged).
func TestReconcileComplexity(t *testing.T) {
	type row struct {
		name       string
		declared   types.Complexity
		request    string
		entities   []string
		keywords   []string
		subTopics  int
		wantResult types.Complexity
		wantReason bool // true when a reason string is expected (non-empty)
	}
	cases := []row{
		// Identity — no rule applies.
		{"simple passes through on short lookup", types.ComplexitySimple,
			"what is the login function", []string{"login"}, []string{"login", "function"}, 0,
			types.ComplexitySimple, false},
		{"moderate passes through on mechanism question", types.ComplexityModerate,
			"how does the auth module work", []string{"auth"}, []string{"auth", "module", "work"}, 0,
			types.ComplexityModerate, false},

		// Rule 1: subTopics>=3 → complex.
		{"3 subtopics forces complex", types.ComplexityModerate,
			"explain A and B and C and D", []string{"A", "B"}, []string{"A", "B", "C"}, 3,
			types.ComplexityComplex, true},
		{"2 subtopics does not trigger rule 1", types.ComplexityModerate,
			"ask two things", []string{"X", "Y"}, []string{"X", "Y"}, 2,
			types.ComplexityModerate, false},

		// Rule 2: cross-component cue → complex.
		{"compare-A-and-B upgrades to complex", types.ComplexityModerate,
			"compare logger and tracer", []string{"logger", "tracer"}, []string{"logger", "tracer"}, 0,
			types.ComplexityComplex, true},
		{"Chinese 对比 cue upgrades", types.ComplexitySimple,
			"对比 A 和 B 的区别", []string{"A", "B"}, []string{"A", "B"}, 0,
			types.ComplexityComplex, true},
		{"across cue upgrades", types.ComplexityModerate,
			"trace flow across the API and storage layers", []string{"API", "storage"}, []string{"flow"}, 0,
			types.ComplexityComplex, true},

		// Rule 3: LLM claimed complex but single-entity lookup shape.
		{"complex + what-is-X downgrades to simple", types.ComplexityComplex,
			"what is repomap", []string{"repomap"}, []string{"repomap"}, 0,
			types.ComplexitySimple, true},
		{"complex + X-是什么 downgrades to simple", types.ComplexityComplex,
			"repomap 是什么", []string{"repomap"}, []string{"repomap"}, 0,
			types.ComplexitySimple, true},

		// Rule 4: 5+ entities and 10+ keywords → complex regardless of LLM pick.
		{"five entities + rich keywords upgrade", types.ComplexityModerate,
			"tell me about these things",
			[]string{"A", "B", "C", "D", "E"},
			[]string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}, 0,
			types.ComplexityComplex, true},

		// Rule 5: zero entities + tiny keyword pool → simple floor.
		{"empty entities + 3 keywords downgrades from complex", types.ComplexityComplex,
			"lang?", nil, []string{"lang", "?", "question"}, 0,
			types.ComplexitySimple, true},

		// Conflict ordering — the first matching rule wins. Rule 1
		// (subTopics>=3) fires before Rule 3's downgrade because
		// structural breadth trumps lookup-shape signal.
		{"subTopics>=3 beats what-is-X downgrade", types.ComplexityComplex,
			"what is X", []string{"X"}, []string{"X"}, 4,
			types.ComplexityComplex, false}, // already complex, rule 1 is no-op because declared==complex
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, reason := reconcileComplexity(c.declared, c.request, c.entities, c.keywords, c.subTopics)
			if got != c.wantResult {
				t.Errorf("result = %q, want %q (reason=%q)", got, c.wantResult, reason)
			}
			if c.wantReason && reason == "" {
				t.Errorf("expected a reason when result changed; got empty")
			}
			if !c.wantReason && reason != "" {
				t.Errorf("expected no reason when result unchanged; got %q", reason)
			}
		})
	}
}

// TestContainsCrossComponentCue / TestContainsSimpleLookupCue lock
// the cue lists so adding a new cue does not accidentally drop an
// existing one. Each cue has exactly one positive example and one
// negative-by-absence example to catch over-fitting.
func TestContainsCrossComponentCue(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"compare a and b", true},
		{"a versus b", true},
		{"对比 a 和 b", true},
		{"what is a", false},
		{"load the module", false},
	}
	for _, c := range cases {
		if got := containsCrossComponentCue(c.in); got != c.want {
			t.Errorf("containsCrossComponentCue(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestContainsSimpleLookupCue(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"what is repomap", true},
		{"repomap 是什么", true},
		{"where is the handler", true},
		{"compare logger and tracer", false},
		{"对比 A 和 B", false},
	}
	for _, c := range cases {
		if got := containsSimpleLookupCue(c.in); got != c.want {
			t.Errorf("containsSimpleLookupCue(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
