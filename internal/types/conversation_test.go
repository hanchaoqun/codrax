package types

import "testing"

func TestStripConversationPrefix(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"single-shot passes through", "how many agents", "how many agents"},
		{
			"REPL marker returns current only",
			"## Prior conversation\n### Recent conversation\n- You: old\n\n## Current request\nrepomap的作用",
			"repomap的作用",
		},
		{
			"trailing whitespace trimmed",
			"## Prior conversation\nx\n\n## Current request\nfoo\n\n",
			"foo",
		},
		{"empty input", "", ""},
		{"marker without trailing newline is not matched", "## Current request", "## Current request"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := StripConversationPrefix(c.in); got != c.want {
				t.Errorf("StripConversationPrefix(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestSplitConversation(t *testing.T) {
	cases := []struct {
		name        string
		in          string
		wantPrior   string
		wantCurrent string
	}{
		{
			"single-shot: no marker",
			"repomap的作用",
			"",
			"repomap的作用",
		},
		{
			"REPL with prior and current",
			"## Prior conversation\n### Recent conversation\n- You: old Q\n  Codrax: old A\n\n## Current request\nrepomap的作用",
			"## Prior conversation\n### Recent conversation\n- You: old Q\n  Codrax: old A",
			"repomap的作用",
		},
		{"empty input", "", "", ""},
		{
			"marker only with empty tail",
			"## Prior conversation\nx\n\n## Current request\n",
			"## Prior conversation\nx",
			"",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotPrior, gotCurrent := SplitConversation(c.in)
			if gotPrior != c.wantPrior {
				t.Errorf("prior: got %q, want %q", gotPrior, c.wantPrior)
			}
			if gotCurrent != c.wantCurrent {
				t.Errorf("current: got %q, want %q", gotCurrent, c.wantCurrent)
			}
		})
	}
}

func TestIsContinuation(t *testing.T) {
	prior := "Q: 关于explorer是怎么调用sub-agent的\nA: via propose_sub_agents and SubExplorer."
	cases := []struct {
		name    string
		current string
		want    bool
	}{
		// Rule A — leading continuation prefix.
		{"zh prefix 再详细讲讲", "再详细讲讲那个流程", true},
		{"zh prefix 继续", "继续", true},
		{"zh prefix 那", "那它的 Output 字段是什么", true},
		{"zh prefix 接着", "接着说下一步", true},
		{"zh prefix 上面", "上面说的那个 Handler 怎么 register 的", true},
		{"en prefix more on", "more on the mechanism", true},
		{"en prefix tell me more", "tell me more about X", true},
		{"en prefix elaborate", "Elaborate on the pipeline", true},
		{"en prefix what about", "What about the finalizer?", true},
		{"en prefix and how", "and how does it work?", true},

		// Rule B — bare pronoun without identifier in window.
		{"zh 它", "它是怎么工作的", true},
		{"zh 这", "这个怎么配置", true},
		{"en it", "it uses what mechanism", true},
		{"en this", "this depends on what", true},

		// Self-contained — should be FALSE.
		{"self-contained with identifier", "explorer 是怎么调用 sub-agent 的", false},
		{"self-contained CamelCase", "How does SubExplorer.Run work", false},
		{"self-contained snake_case", "how does propose_sub_agents work", false},
		{"self-contained with dotted identifier", "what does Foo.Bar return", false},
		{"pronoun but identifier present", "它的 SubExplorer 是什么", false},
		{"fresh English question", "how many files are there", false},

		// Edge cases.
		{"empty prior rejects", "再详细讲讲", false},
		{"empty current rejects", "", false},
		{"leading punctuation stripped", "，再详细讲讲", true},
		{"leading English punct stripped", "? more on this", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotPrior := prior
			if c.name == "empty prior rejects" {
				gotPrior = ""
			}
			if got := IsContinuation(c.current, gotPrior); got != c.want {
				t.Errorf("IsContinuation(%q, prior) = %v, want %v", c.current, got, c.want)
			}
		})
	}
}

func TestNormalizeREPLCommandAlias(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"/quit", "/quit"},
		{" /exit ", "/exit"},
		{"\\q", "/quit"},
		{"\\help", "/help"},
		{"/history now", "/history now"},
		{"how does explorer work", ""},
	}
	for _, c := range cases {
		if got := NormalizeREPLCommandAlias(c.in); got != c.want {
			t.Errorf("NormalizeREPLCommandAlias(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsREPLControlInput(t *testing.T) {
	if !IsREPLControlInput("\\q") {
		t.Error(`\q must be treated as a REPL control input`)
	}
	if !IsREPLControlInput("/help") {
		t.Error(`/help must be treated as a REPL control input`)
	}
	if IsREPLControlInput("buildOrLoadGraph 是如何构建图的？") {
		t.Error("normal user question must not be treated as a control input")
	}
}
