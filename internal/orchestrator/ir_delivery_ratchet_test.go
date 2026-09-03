package orchestrator

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestIRDeliveryHotFileLineRatchet(t *testing.T) {
	t.Parallel()
	cases := []struct {
		path     string
		maxLines int
	}{
		{path: filepath.Join("..", "types", "evidence_closure.go"), maxLines: 2630},
		{path: "scheduler.go", maxLines: 743},
		// Tightened 9395→9260 after WRITEFIX-1 split the apply/verify
		// wording single-point into write_verify_render.go (which gets
		// its own budget below) — the freed budget must not silently
		// grow back into the god-file. Tightened 9260→9135 after
		// COMPLETE-2 (§29.140 GAP-4) split the accepted-closure
		// mixed-origin debt gate into accepted_closure_origin_debt.go
		// (own budget below); same rule: freed budget must not grow back.
		// Tightened 9135→9126 after §40.14 V7-2 moved the finalize
		// acceptance exit (evidence-utilization log + node bookkeeping +
		// retry-chain close) into acceptFinalizeNode in retry_state.go —
		// the four repeated triplets became one call each.
		// Tightened 9126→9018 after F14 (§40.14 V7-2 fold-in) moved the
		// reconcile-node auto-complete arm into accepted_closure_reconcile.go
		// and the shared accepted-closure premise into
		// accepted_closure_premise.go (own budgets below).
		{path: "orchestrator.go", maxLines: 9018},
		// F14: the reconcile auto-complete arm and the single accepted-closure
		// premise shared by both auto-complete consumers.
		{path: "accepted_closure_reconcile.go", maxLines: 130},
		{path: "accepted_closure_premise.go", maxLines: 60},
		{path: "write_verify_render.go", maxLines: 420},
		// DELIBERATE 240→280 (§29.146 UPSTREAM-3 件1): the pre-mint
		// withhold half of the current_source waiver double defense
		// (acceptedClosureRequiredOriginLanesBeforeDebtMint +
		// withholdWaivedCurrentSourceOriginLaneBeforeDebtMint) lives in
		// this concern-specific file next to the post-filter backstop it
		// mirrors.
		{path: "accepted_closure_origin_debt.go", maxLines: 280},
		// EVALFIX-2E (CLASS 5): the "[degrade] ledger:" operator
		// aggregate lives in its own concern file so the degradation-
		// ledger surface never grows the god-file.
		{path: "degradation_ledger_log.go", maxLines: 60},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatalf("read %s: %v", tc.path, err)
			}
			lines := bytes.Count(data, []byte{'\n'})
			if len(data) > 0 && data[len(data)-1] != '\n' {
				lines++
			}
			if lines > tc.maxLines {
				t.Fatalf("%s has %d lines; IR delivery ratchet allows at most %d. Split concern-specific code or update the delivery ledger before expanding this budget.", tc.path, lines, tc.maxLines)
			}
		})
	}
}
