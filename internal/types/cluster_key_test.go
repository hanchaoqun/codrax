package types

import "testing"

func TestComposeClusterKey_TrimsAndSkipsEmptyParts(t *testing.T) {
	got := ComposeClusterKey(" block ", " summary ", "root", " block_claim_use ", "", "ignored")
	want := "block:summary|root:block_claim_use"
	if got != want {
		t.Fatalf("ComposeClusterKey()=%q, want %q", got, want)
	}
}

func TestComposeClusterKey_IgnoresOddTrailingKey(t *testing.T) {
	got := ComposeClusterKey("block", "summary", "root")
	want := "block:summary"
	if got != want {
		t.Fatalf("ComposeClusterKey()=%q, want %q", got, want)
	}
}

func TestIdentityClusterKey_PreservesPrelabeledIdentity(t *testing.T) {
	got := IdentityClusterKey("label:TurnAArtifacts", "answer_citations_locus")
	want := "label:TurnAArtifacts|root:answer_citations_locus"
	if got != want {
		t.Fatalf("IdentityClusterKey()=%q, want %q", got, want)
	}
}
