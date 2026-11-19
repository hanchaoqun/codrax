package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC377 R-377-4 (owner ruling on §376 A audit failure 1): the finalizer
// handoff supplement never publishes a chain thread's absorbed window account
// as a separate time-adjacent "not proven on chain" candidate, and the 值=
// line of a priority_inversion_runnable_wait row names its whole ready-time
// account as such, with the same-CPU lower-priority overlap named apart
// (Honeybird 64176: 45.878 ms of ready time, overlap 8.408 ms).
func TestHMC377SupplementAbsorbedInversionAccountIsMergedDetail(t *testing.T) {
	absorbed := types.ObservationRecord{
		Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard,
		ClaimKey: "root_cause_absorbed", Predicate: "root_cause_absorbed",
		Subject: "Signal Catcher-64176", Object: "priority_inversion_runnable_wait", Value: "45.878", Unit: "ms",
		RichNotes: []string{
			"tier=absorbed", "type=priority_inversion_runnable_wait", "impact_ms=45.878", "cumulative_impact_ms=45.878",
			"effective_impact_ms=8.408", "source=window_stats", "causality=adjacent_to_wakeup_chain",
			"dominant_state=runnable", "gated_runnable=8.408", "chain_relevance=adjacent", "absorbed_by_rank_family=true",
		},
	}
	background := absorbed
	background.ClaimKey, background.Predicate = "root_cause_background", "root_cause_background"
	background.Subject = "bg-77"
	background.RichNotes = []string{"type=priority_inversion_runnable_wait", "effective_impact_ms=0.000",
		"gated_runnable=1.500", "chain_relevance=background", "causality=background"}
	for _, tc := range []struct {
		zh                                  bool
		label, account, overlap, adjacentBy string
		backgroundOverlap                   string
	}{
		{true, "合并明细观测", "值=45.878ms(全部 runnable 时间)", "其中同核低优先级运行重叠=8.408ms", "时间相邻，未证链上因果", "其中同核低优先级运行重叠=1.500ms"},
		{false, "merged-detail observation", "value=45.878ms (whole runnable time)", "same-CPU lower-priority running overlap=8.408ms",
			"temporally adjacent; on-chain causality unproved", "same-CPU lower-priority running overlap=1.500ms"},
	} {
		text := traceQueryObservationSupplementText(absorbed, tc.zh)
		if !strings.HasPrefix(text, tc.label) {
			t.Fatalf("zh=%v: an absorbed row is the merged detail of another row: %q", tc.zh, text)
		}
		if !strings.Contains(text, tc.account) || !strings.Contains(text, tc.overlap) {
			t.Fatalf("zh=%v: the value must name the whole ready account and the overlap apart: %q", tc.zh, text)
		}
		if strings.Contains(text, tc.adjacentBy) || strings.Contains(text, "邻近支撑观测") || strings.Contains(text, "adjacent supporting") {
			t.Fatalf("zh=%v: an absorbed account is not a separate time-adjacent candidate: %q", tc.zh, text)
		}
		if strings.Contains(text, "同核可运行重叠；值") || strings.Contains(text, "runnable overlap; value") {
			t.Fatalf("zh=%v: the whole account is never labelled as the overlap: %q", tc.zh, text)
		}
		// A background row's effective 0 is its no-attribution sentinel, not
		// its overlap: the overlap comes from the measurement itself.
		if bg := traceQueryObservationSupplementText(background, tc.zh); !strings.Contains(bg, tc.backgroundOverlap) {
			t.Fatalf("zh=%v: the overlap must come from the measurement, not the attribution sentinel: %q", tc.zh, bg)
		}
	}
}
