package orchestrator

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// EVALFIX-2E (CLASS 5, 2026-07-30) — operator-side aggregate of the
// typed degradation ledger, emitted from the emitCGECSummary neighbor
// slot at end-of-Run.
//
// The line is the operator mirror of the user-facing disclosure footer
// (internal/agent/degradation_disclosure_footer.go): one grep
// ("[degrade] ledger:") recovers the full per-Run degradation account
// instead of collecting the scattered per-site WARN lines. Internal
// lane tokens are fine here (log surface, never user-facing), and —
// unlike the footer — every class is included: plumbing lanes are
// operator-visible even though they never disclose to users.
//
// 0 entries → nothing printed, so healthy-path logs stay byte-stable.
func emitDegradationLedgerSummary(mut *types.MutableState) {
	entries := types.BuildDegradationLedgerView(mut)
	if len(entries) == 0 {
		return
	}
	parts := make([]string, 0, len(entries))
	for _, entry := range entries {
		parts = append(parts, fmt.Sprintf("%s=%d", entry.Lane, entry.Count))
	}
	logging.Info("[degrade] ledger: %s", strings.Join(parts, " "))
}
