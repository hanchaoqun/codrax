package skill

import (
	"strings"
	"testing"
)

func TestB1627ExtractorDoesNotTurnCandidateTerminalOrCountIntoAuthority(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("extract-skill")
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(sk.Workflow, "\n") + "\n" + sk.OutputFormat + "\n" + strings.Join(sk.Prohibitions, "\n")
	for _, bad := range []string{"emit ONLY the instances that terminal names", "the answer is the terminal that the chain RESOLVES TO", "the larger of: how many items the investigation found", "expected answer count drives the completeness claim"} {
		if strings.Contains(got, bad) {
			t.Errorf("candidate teaching overclaims: %q", bad)
		}
	}
	for _, want := range []string{"candidate", "typed", "emit_answer_symbol", "symbols_completeness"} {
		if !strings.Contains(got, want) {
			t.Errorf("teaching lost %q", want)
		}
	}
	for _, surface := range []string{strings.Join(sk.Workflow, "\n"), sk.OutputFormat, strings.Join(sk.Prohibitions, "\n")} {
		if !strings.Contains(surface, extractorCardinalityAuthorityTeaching) {
			t.Errorf("extractor count teaching diverged from the shared boundary: %s", surface)
		}
	}
	if strings.Contains(strings.Join(sk.ToolSuggestions, " "), "read_file") {
		t.Fatal("closed extractor lane acquired an unavailable read tool")
	}
}
