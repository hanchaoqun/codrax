package types

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracefence"
)

func TestHMC323DStateCaliberPreservesMarkerKnowledge(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			got := FormatTargetStateAccountCaliber(lang)
			want := "known-zero and unknown"
			if lang == "zh" {
				want = "标记为 0 和标记未知"
			}
			if !strings.Contains(got, tracefence.StateNonIODStateWord(lang == "zh")) || !strings.Contains(got, want) {
				t.Fatalf("fold teaching must use the shared residual label without promoting unknown to false: %s", got)
			}
		})
	}
	if !strings.Contains(TraceSchedulerWaitPartitionTeaching, "known-zero and unknown") ||
		!strings.Contains(TraceSchedulerWaitPartitionTeaching, "not proof that an IO mechanism is absent") {
		t.Fatalf("partition teaching must retain the same marker knowledge boundary: %s", TraceSchedulerWaitPartitionTeaching)
	}
	if got := TraceUninterruptibleWaitMS(4.039, 1.340); got != 5.379 {
		t.Fatalf("display-only repair changed the physical D fold: %.9f", got)
	}
}
