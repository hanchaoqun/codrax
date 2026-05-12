package types

import "testing"

func TestDeterministicCountProofInteger_AcceptsScalarCountShapes(t *testing.T) {
	cases := map[string]int{
		"4\n": 4,
		"[exec_command: $ rg -n pattern | wc -l]\n4\n": 4,
		"count=4\n":                            4,
		"total: 4\n":                           4,
		"4 total\n":                            4,
		"      4 internal/agent/analyzer.go\n": 4,
		"internal/agent/analyzer.go:4\n":       4,
		"internal/agent/analyzer.go:1\ninternal/foo.go:2\n3 total\n": 3,
	}
	for summary, want := range cases {
		got, ok := DeterministicCountProofInteger(summary)
		if !ok || got != want {
			t.Fatalf("DeterministicCountProofInteger(%q) = (%d,%v), want (%d,true)", summary, got, ok, want)
		}
	}
}

func TestDeterministicCountProofInteger_RejectsMatchListings(t *testing.T) {
	cases := []string{
		"[exec_command: $ rg -n Required]\ninternal/orchestrator/contract_check.go:63:\tc.CitationReq.Required = false\n",
		"internal/agent/analyzer.go:1903:\tout.AnswerContract.CitationReq.Required = false\ninternal/orchestrator/contract_check.go:63:\tc.CitationReq.Required = false\n",
		"internal/agent/analyzer.go:1\ninternal/orchestrator/contract_check.go:2\n",
		"no output produced",
	}
	for _, summary := range cases {
		if got, ok := DeterministicCountProofInteger(summary); ok {
			t.Fatalf("DeterministicCountProofInteger(%q) = (%d,true), want rejected", summary, got)
		}
	}
}
