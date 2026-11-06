package tool

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
)

// Independent literals for the complete approved Description delta. Do not
// derive them from live teaching: older byte pins must still detect any other
// edit, including extra or misplaced copies of these sentences.
const traceDescriptionBeforeIntervalDepthSHA = "353d6b62c34e7be306f114a9c4f18361c56f4dd73c70637a1d371764568cb347"

const traceDepthDurationApprovedAddition = " For scheduler_concurrency and io_inflight, depth_duration is optional for each measured group and the same full window. peak_duration_ms measures time at peak_depth, not busy_ms or hardware saturation. Divide by window_ms for its share. zero_contribution_ms means no admitted contribution, not measured system/device idle."

func reverseTraceQueryIntervalDepthEvolution(description string) (string, bool) {
	for _, delta := range [][2]string{
		{traceDepthDurationApprovedAddition, ""},
		{"peak_threads is the maximum simultaneous count;", "peak_threads is an instantaneous count;"},
		{"mean divides by the full selected window including zero contribution from admitted pairs.", "mean divides by the full selected window including idle time."},
	} {
		if strings.Count(description, delta[0]) != 1 || delta[1] != "" && strings.Contains(description, delta[1]) {
			return "", false
		}
		description = strings.Replace(description, delta[0], delta[1], 1)
	}
	return description, true
}

func traceQueryDescriptionBeforeIntervalDepthEvolution(t *testing.T, description string) string {
	t.Helper()
	prior, ok := reverseTraceQueryIntervalDepthEvolution(description)
	if !ok {
		t.Fatal("interval-depth teaching is not the unique approved addition and two precise wording corrections")
	}
	return prior
}

func TestHMC361TraceQueryIntervalDepthDescriptionOnlyEvolution(t *testing.T) {
	description := (&TraceQuery{}).Description()
	prior := traceQueryDescriptionBeforeIntervalDepthEvolution(t, description)
	if got := fmt.Sprintf("%x", sha256.Sum256([]byte(prior))); got != traceDescriptionBeforeIntervalDepthSHA {
		t.Fatalf("Description changed outside the exact approved depth-duration teaching: %s", got)
	}
	for name, mutated := range map[string]string{
		"missing":   strings.Replace(description, traceDepthDurationApprovedAddition, "", 1),
		"duplicate": description + traceDepthDurationApprovedAddition,
		"old_idle":  description + "mean divides by the full selected window including idle time.",
	} {
		if _, ok := reverseTraceQueryIntervalDepthEvolution(mutated); ok {
			t.Errorf("%s unapproved edit was reversible", name)
		}
	}
	// The exact historical digest, not a regenerated hash of the current
	// text, continues to reject any unrelated changes after reversal.
	if other, ok := reverseTraceQueryIntervalDepthEvolution("unapproved " + description); !ok ||
		fmt.Sprintf("%x", sha256.Sum256([]byte(other))) == traceDescriptionBeforeIntervalDepthSHA {
		t.Fatal("the reverse projection concealed unrelated dispatch prose")
	}
}
