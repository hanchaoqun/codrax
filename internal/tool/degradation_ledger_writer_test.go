package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// EVALFIX-2E (CLASS 5) — writer-bridge pins for the citation-quote
// rewrite lane (A2).

// TestRecordCitationQuoteRewriteDegradationBooksRealRewrites drives the
// REAL deterministic rewrite pass against a real on-disk file and pins
// that the call-site pattern (normalize → record) lands the precise
// count on the typed ledger.
func TestRecordCitationQuoteRewriteDegradationBooksRealRewrites(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "real.go"), []byte("package x\nfunc Real() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Citations: []types.Citation{
			{File: "real.go", Line: 2},
			{File: "real.go", Line: 1},
		},
	}
	ctx := &types.BusContext{RepoRoot: repo, Mutable: types.NewMutableState("q")}
	fixed := normalizeCurrentSourceCitationQuotes(doc, ctx)
	if fixed != 2 {
		t.Fatalf("precondition: fixed=%d, want 2", fixed)
	}
	recordCitationQuoteRewriteDegradation(ctx, fixed)
	view := types.BuildDegradationLedgerView(ctx.Mutable)
	if len(view) != 1 || view[0].Lane != types.DegradeLaneCitationQuoteRewrite || view[0].Count != 2 {
		t.Fatalf("ledger view = %+v, want [{%s 2}]", view, types.DegradeLaneCitationQuoteRewrite)
	}
	// Nil-safety (fail-open): no ctx / no mutable / zero count are all
	// inert.
	recordCitationQuoteRewriteDegradation(nil, 3)
	recordCitationQuoteRewriteDegradation(&types.BusContext{}, 3)
	recordCitationQuoteRewriteDegradation(ctx, 0)
	if view := types.BuildDegradationLedgerView(ctx.Mutable); len(view) != 1 || view[0].Count != 2 {
		t.Fatalf("no-op arms must not change the account, got %+v", view)
	}
}

// TestCitationQuoteRewriteDegradationCallSitesStructural pins the
// writer topology: the recorder sits beside the WARN at every HEALTHY
// normalizeCurrentSourceCitationQuotes call site (emit-time first pass
// + pre-emit chain tail in emit_answer_document_v2.go, pre-persist pass
// in answer_document_mutation_runtime.go) and is ABSENT from the
// degraded-export lane, which already self-discloses via the degraded
// footer's citation_quote_backfill entry — booking it there would
// double-disclose.
func TestCitationQuoteRewriteDegradationCallSitesStructural(t *testing.T) {
	count := func(file string) int {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		return strings.Count(string(raw), "recordCitationQuoteRewriteDegradation(")
	}
	if got := count("emit_answer_document_v2.go"); got != 2 {
		t.Fatalf("emit_answer_document_v2.go must book the ledger at BOTH healthy quote passes (first pass + chain tail), got %d call(s)", got)
	}
	if got := count("answer_document_mutation_runtime.go"); got != 1 {
		t.Fatalf("answer_document_mutation_runtime.go must book the ledger at the pre-persist pass, got %d call(s)", got)
	}
	if got := count("answer_document_degraded_export.go"); got != 0 {
		t.Fatalf("degraded-export lane is self_disclosing — the ledger recorder must NOT appear there (double disclosure), got %d call(s)", got)
	}
}
