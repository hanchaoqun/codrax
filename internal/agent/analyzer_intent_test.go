package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestReconcileIntent covers the downgrade rule, the three hard gates
// (intent must be enumerate, complexity must be simple, leading clause
// must start with a count-verb cue), and a representative set of
// Chinese/English positives and would-be false positives.
func TestReconcileIntent(t *testing.T) {
	type row struct {
		name       string
		declared   types.Intent
		request    string
		complexity types.Complexity
		want       types.Intent
		wantReason bool
	}
	cases := []row{
		// ── Gate: non-enumerate intents pass through ─────────────
		{"explain passes through", types.IntentExplain,
			"统计 go 代码量", types.ComplexitySimple,
			types.IntentExplain, false},
		{"return_value passes through (rule never over-picks)", types.IntentReturnValue,
			"how many go files", types.ComplexitySimple,
			types.IntentReturnValue, false},
		{"root_cause passes through", types.IntentRootCause,
			"how many times does the login fail", types.ComplexitySimple,
			types.IntentRootCause, false},

		// ── Gate: non-simple complexity pass through ─────────────
		{"moderate enumerate is left alone", types.IntentEnumerate,
			"统计 go 代码量", types.ComplexityModerate,
			types.IntentEnumerate, false},
		{"complex enumerate is left alone", types.IntentEnumerate,
			"how many handlers do X and also list them", types.ComplexityComplex,
			types.IntentEnumerate, false},

		// ── Positives: Chinese count-verb prefixes ───────────────
		{"统计 prefix downgrades to return_value", types.IntentEnumerate,
			"统计一下这个项目下的 go 代码量", types.ComplexitySimple,
			types.IntentReturnValue, true},
		{"数一下 prefix downgrades", types.IntentEnumerate,
			"数一下有多少个 handler", types.ComplexitySimple,
			types.IntentReturnValue, true},
		{"有多少 prefix downgrades", types.IntentEnumerate,
			"有多少个 go 文件", types.ComplexitySimple,
			types.IntentReturnValue, true},
		{"一共 prefix downgrades", types.IntentEnumerate,
			"一共有多少 agent", types.ComplexitySimple,
			types.IntentReturnValue, true},

		// ── Positives: English count-verb prefixes ───────────────
		{"how many prefix downgrades", types.IntentEnumerate,
			"how many Go files are in this repo", types.ComplexitySimple,
			types.IntentReturnValue, true},
		{"count prefix downgrades", types.IntentEnumerate,
			"count the lines in main.go", types.ComplexitySimple,
			types.IntentReturnValue, true},
		{"number of prefix downgrades", types.IntentEnumerate,
			"number of registered agents", types.ComplexitySimple,
			types.IntentReturnValue, true},
		{"total lines prefix downgrades", types.IntentEnumerate,
			"total lines of Go code", types.ComplexitySimple,
			types.IntentReturnValue, true},
		{"size of prefix downgrades", types.IntentEnumerate,
			"size of the binary", types.ComplexitySimple,
			types.IntentReturnValue, true},

		// ── Politeness strip: leading "please / 请 / 帮我" ────────
		{"please-count prefix downgrades", types.IntentEnumerate,
			"please count the handlers", types.ComplexitySimple,
			types.IntentReturnValue, true},
		{"请统计 prefix downgrades", types.IntentEnumerate,
			"请统计 go 文件数量", types.ComplexitySimple,
			types.IntentReturnValue, true},
		{"帮我统计 prefix downgrades", types.IntentEnumerate,
			"帮我统计代码行数", types.ComplexitySimple,
			types.IntentReturnValue, true},

		// ── Negatives: cue appears mid-clause, NOT at start ──────
		{"list that mentions count mid-sentence is NOT downgraded", types.IntentEnumerate,
			"list all handlers that count requests", types.ComplexitySimple,
			types.IntentEnumerate, false},
		{"统计 as a noun mid-sentence is NOT downgraded", types.IntentEnumerate,
			"列出所有做 统计 的模块", types.ComplexitySimple,
			types.IntentEnumerate, false},
		{"which uses of total is NOT downgraded", types.IntentEnumerate,
			"which handlers use the total variable", types.ComplexitySimple,
			types.IntentEnumerate, false},
		{"where is size_of registered is NOT downgraded (not a prefix match)", types.IntentEnumerate,
			"where is size_of registered", types.ComplexitySimple,
			types.IntentEnumerate, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, reason := reconcileIntent(c.declared, c.request, c.complexity)
			if got != c.want {
				t.Fatalf("intent = %q, want %q (request=%q)", got, c.want, c.request)
			}
			if c.wantReason && reason == "" {
				t.Fatalf("expected non-empty reason, got empty (request=%q)", c.request)
			}
			if !c.wantReason && reason != "" {
				t.Fatalf("expected empty reason, got %q (request=%q)", reason, c.request)
			}
		})
	}
}

// TestStripPolitenessPrefix covers the single-strip contract: the
// helper strips at most one leading politeness token, and returns
// the input unchanged when none applies.
func TestStripPolitenessPrefix(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"please count files", "count files"},
		{"could you count files", "count files"},
		{"can you count files", "count files"},
		{"请统计代码", "统计代码"},
		{"请帮我统计代码", "统计代码"},
		{"帮我统计代码", "统计代码"},
		{"麻烦统计代码", "统计代码"},
		{"count files", "count files"}, // identity
		{"", ""},                        // empty identity
	}
	for _, c := range cases {
		got := stripPolitenessPrefix(c.in)
		if got != c.want {
			t.Errorf("stripPolitenessPrefix(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestHasLeadingCountVerb is a tight round-trip on the prefix matcher
// itself — separate from reconcileIntent so a future regression on
// the prefix set is easy to bisect.
func TestHasLeadingCountVerb(t *testing.T) {
	positives := []string{
		"统计 go 代码",
		"数一下文件",
		"how many go files",
		"count the lines",
		"number of agents",
		"total lines in main.go",
	}
	negatives := []string{
		"list handlers that count requests",
		"which files total more than 100 lines",
		"explain how many ways to configure",
		"where is count defined",
		"",
	}
	for _, p := range positives {
		if !hasLeadingCountVerb(p) {
			t.Errorf("expected positive, got false: %q", p)
		}
	}
	for _, n := range negatives {
		if hasLeadingCountVerb(n) {
			t.Errorf("expected negative, got true: %q", n)
		}
	}
}
