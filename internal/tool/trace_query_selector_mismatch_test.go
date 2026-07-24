package tool

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestTraceQueryPublishesTypedSelectorMismatchToSummaryAndLedger(t *testing.T) {
	result := tracequery.Result{
		View: "window_stats",
		ThreadSelection: &tracequery.ThreadSelectorResolution{
			Status:        "exact_tid_name_mismatch",
			RequestedPID:  32788,
			RequestedName: "ss.hm.ugc.aweme",
			Selected:      tracequery.ThreadRef{Comm: "unknown", PID: 32788},
			NameMismatch:  true,
			NameCandidates: []tracequery.ThreadRef{{
				Comm: "ss.hm.ugc.aweme", PID: 33410,
			}},
			Routing: "exact_tid_preserved",
		},
	}
	summary := traceQuerySummary(result, traceQueryParams{View: "window_stats"}, "path", "")
	for _, want := range []string{
		"thread_selection status=exact_tid_name_mismatch",
		"selected=unknown-32788",
		"routing=exact_tid_preserved",
		"name_candidates=[ss.hm.ugc.aweme-33410]",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, summary)
		}
	}
	observations := traceQueryTypedObservations(result, "path", "payload", "raw", "", time.Unix(0, 0))
	found := false
	for _, observation := range observations {
		if observation.Predicate == "thread_selector_exact_name_mismatch" {
			found = true
			if observation.Subject != "unknown-32788" ||
				!strings.Contains(observation.Summary, "diagnostic candidate(s) [ss.hm.ugc.aweme-33410]") {
				t.Fatalf("typed mismatch observation drifted: %+v", observation)
			}
		}
	}
	if !found {
		t.Fatalf("typed mismatch observation not published: %+v", observations)
	}
}
