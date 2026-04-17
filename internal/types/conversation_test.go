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
