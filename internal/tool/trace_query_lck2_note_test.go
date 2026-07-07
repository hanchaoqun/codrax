package tool

// LCK-2 note-face pins (§18.E/§18.E.1): the typed ②×③ identity-unification
// declaration and the process-level ns-span identity ride the registered
// blocking-family note keys on the critical_blocking wire face, and both keys
// are registry rows (the emit pin enforces emitted ⊆ registry globally; this
// pins the PRESENCE half for the two new keys). Key literals below are
// deliberate verbatim wire pins — do not replace them with the constants.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestCriticalBlockingNotesCarryNsSpanIdentity(t *testing.T) {
	item := tracequery.CriticalBlockingCandidate{
		Type:                "blocking_span",
		Thread:              tracequery.ThreadRef{Comm: "aweme", PID: 41999, TGID: 41905},
		Peer:                tracequery.ThreadRef{Comm: "nsworker", PID: 42500, TGID: 41905},
		BlockingKind:        "monitor_contention",
		HolderSource:        tracequery.CounterpartSourceNsSpanDerivation,
		OwnerTidRaw:         62020,
		HolderNsUnification: "owner_ns_tid=62020 host=nsworker-42500 lanes=ns_span_derivation+wakeup_edge",
		HolderHostProcess:   "tgid=41905 ns_pid=43000 level=process",
		DurationMs:          112.223,
		Confidence:          0.70,
	}
	notes := traceQueryTypedCriticalBlockingRichNotes(item)
	joined := strings.Join(notes, "\n")
	for _, want := range []string{
		"holder_source=ns_span_derivation",
		"holder_ns_unification=owner_ns_tid=62020 host=nsworker-42500 lanes=ns_span_derivation+wakeup_edge",
		"holder_host_process=tgid=41905 ns_pid=43000 level=process",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("critical_blocking notes must carry %q, got:\n%s", want, joined)
		}
	}
	// Empty values never mint empty-valued notes.
	bare := tracequery.CriticalBlockingCandidate{Type: "blocking_span", Thread: item.Thread}
	for _, note := range traceQueryTypedCriticalBlockingRichNotes(bare) {
		if strings.HasPrefix(note, "holder_ns_unification=") || strings.HasPrefix(note, "holder_host_process=") {
			t.Fatalf("empty LCK-2 fields must not emit notes: %q", note)
		}
	}
}
