package tool

import (
	"strings"
	"testing"
)

// TestAnswerDocumentCitationLedgerSuffix pins the §29.174 RUN2AUDIT-1
// F6 machine face: the accepted-summary delta tokens fire exactly when
// the model-submitted citation count differs from the registered pool,
// carry per-reason drop counts, and never fire on delta == 0 (the
// negative arm keeps the historical summary byte-identical). Token
// spelling must never contain the literal "citations=" — the renderer's
// registered-count regex scans greedily for that literal and would
// re-bind to the suffix.
func TestAnswerDocumentCitationLedgerSuffix(t *testing.T) {
	// Witness shape: 5 submitted → 0 registered, all runtime reroute.
	got := answerDocumentCitationLedgerSuffix(5, 0, 5, 0, 0, 0)
	if got != " citations_submitted=5 citations_redirected_runtime=5" {
		t.Fatalf("witness-shape suffix wrong: %q", got)
	}
	if strings.Contains(got, "citations=") {
		t.Fatalf("token spelling must not contain the literal \"citations=\": %q", got)
	}

	// Mixed drop reasons all surface.
	mixed := answerDocumentCitationLedgerSuffix(6, 1, 2, 2, 1, 0)
	for _, want := range []string{
		"citations_submitted=6",
		"citations_redirected_runtime=2",
		"citations_rejected_form=2",
		"citations_pruned_unused=1",
	} {
		if !strings.Contains(mixed, want) {
			t.Fatalf("mixed suffix missing %q: %q", want, mixed)
		}
	}

	// Registered can exceed submitted (deterministic backfill /
	// carry-forward): the delta still discloses, reasons stay honest
	// (zero drop counters emit no reason tokens).
	added := answerDocumentCitationLedgerSuffix(3, 5, 0, 0, 0, 0)
	if added != " citations_submitted=3" {
		t.Fatalf("addition-direction suffix wrong: %q", added)
	}

	// Negative arm: delta == 0 with zero persist mint emits nothing.
	if got := answerDocumentCitationLedgerSuffix(5, 5, 0, 0, 0, 0); got != "" {
		t.Fatalf("delta==0 must emit no suffix, got %q", got)
	}
	if got := answerDocumentCitationLedgerSuffix(0, 0, 0, 0, 0, 0); got != "" {
		t.Fatalf("zero-citation emit must stay token-free, got %q", got)
	}

	// A2 件6 (§29.178 B 批 P3 移交, 2026-07-21): the persist chain re-minted
	// pool entries — the suffix fires EVEN at submitted == registered (equal
	// endpoints can hide churn: one dropped + one minted), and the token
	// spelling keeps the no-"citations=" discipline.
	mintedEqual := answerDocumentCitationLedgerSuffix(5, 5, 0, 0, 0, 1)
	if mintedEqual != " citations_submitted=5 citations_minted_persist=1" {
		t.Fatalf("件6: equal-endpoint churn must disclose the persist mint: %q", mintedEqual)
	}
	if strings.Contains(mintedEqual, "citations=") {
		t.Fatalf("token spelling must not contain the literal \"citations=\": %q", mintedEqual)
	}
	mintedGrow := answerDocumentCitationLedgerSuffix(5, 7, 0, 0, 0, 2)
	if !strings.Contains(mintedGrow, "citations_minted_persist=2") {
		t.Fatalf("件6: pool growth must carry the minted token: %q", mintedGrow)
	}
	// ≤0 net mint claims nothing (a shrinking persist pool is not a mint).
	if got := answerDocumentCitationLedgerSuffix(5, 5, 0, 0, 0, -1); got != "" {
		t.Fatalf("件6: negative net mint must stay silent, got %q", got)
	}
}
