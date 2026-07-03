package tracequery

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// paddingDegradeTraceLines builds a wakeup-only synthetic trace with one event
// per timestamp, so MaxEvents arithmetic in the tests below is exact.
func paddingDegradeTraceLines(t *testing.T, dir, name string, timestamps []string) string {
	t.Helper()
	lines := make([]string, 0, len(timestamps)+1)
	for _, ts := range timestamps {
		lines = append(lines,
			`      app-20  (   20) [001] .... `+ts+`: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001`)
	}
	lines = append(lines, "")
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestBuildIndexPaddingTailBudgetDegradesGracefully pins the padding-tail
// graceful degrade under the QF1-reworked criterion: monotonic trace, the
// budget trips at an event whose ts is STRICTLY beyond the requested TimeEnd
// (trigger inside the padding tail), zero clock regressions — the build must
// succeed with the core window intact, carry the typed PaddingTruncated
// marker, and the note/typed field must state the real parse boundary (QF5).
func TestBuildIndexPaddingTailBudgetDegradesGracefully(t *testing.T) {
	dir := t.TempDir()
	// Request window [2.0,2.1] with ±0.5s padding: 1.90 is padding head,
	// 2.00/2.05/2.10 are the request window, 2.20+ is the padding tail.
	// MaxEvents=5 admits through 2.20; the budget trips at trigger 2.30.
	path := paddingDegradeTraceLines(t, dir, "padded.systrace", []string{
		"1.900000", "2.000000", "2.050000", "2.100000",
		"2.200000", "2.300000", "2.400000",
	})
	idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		TimeStart:          2.0,
		TimeEnd:            2.1,
		TimeStartSet:       true,
		TimeEndSet:         true,
		TimePaddingBefore:  0.5,
		TimePaddingAfter:   0.5,
		AllowWindowedParse: true,
		MaxEvents:          5,
	})
	if err != nil {
		t.Fatalf("padding-tail budget hit must degrade, not deny: %v", err)
	}
	if !idx.PaddingTruncated {
		t.Fatalf("expected typed PaddingTruncated marker: %+v", idx)
	}
	// QF5 pin: the note carries the real parse boundary — LastTs at the
	// trigger point (the trigger event 2.30 already updated LastTs), not a
	// fixed claim. Display surfaces fold the formatted string verbatim.
	want := fmt.Sprintf("index budget hit after request window fully parsed (parsed through ts=%.6f); padding tail truncated", 2.3)
	if idx.PaddingTruncatedNote != want {
		t.Fatalf("padding-truncated note must carry the parse boundary:\n got %q\nwant %q", idx.PaddingTruncatedNote, want)
	}
	// Typed twin of the note's boundary for query-layer caveats.
	if idx.PaddingTruncatedLastTs != 2.3 {
		t.Fatalf("PaddingTruncatedLastTs must be LastTs at the trigger point, got %.6f", idx.PaddingTruncatedLastTs)
	}
	if len(idx.Events) != 5 {
		t.Fatalf("degrade must keep the events parsed so far, got %d", len(idx.Events))
	}
	if idx.FirstTs > 2.0 || idx.LastTs < 2.1 {
		t.Fatalf("degraded index must cover the request window, parsed range %.6f..%.6f", idx.FirstTs, idx.LastTs)
	}
	// Every in-window event made it into the index — degrade loses only tail
	// padding, never core-window rows.
	inWindow := 0
	for _, ev := range idx.Events {
		if ev.Ts >= 2.0 && ev.Ts <= 2.1 {
			inWindow++
		}
	}
	if inWindow != 3 {
		t.Fatalf("degrade lost in-window events: got %d of 3", inWindow)
	}
}

// TestBuildIndexPaddingDegradeDeniesTriggerAtWindowEnd pins the QF1 endpoint
// regression: a trigger event with ts EXACTLY equal to TimeEnd is an in-window
// match (timeInWindow includes both endpoints). The pre-rework criterion
// (FirstTs <= TimeStart && LastTs >= TimeEnd) degraded here — LastTs had
// already been updated BY the trigger event itself — and silently dropped a
// real event_search match at the window endpoint. The strict ev.Ts > TimeEnd
// comparison must keep this a hard denial.
func TestBuildIndexPaddingDegradeDeniesTriggerAtWindowEnd(t *testing.T) {
	dir := t.TempDir()
	// MaxEvents=4 admits 1.90/2.00/2.05/2.08; the budget trips at trigger
	// 2.10 == TimeEnd, an in-window event that must not be discarded.
	path := paddingDegradeTraceLines(t, dir, "endpoint.systrace", []string{
		"1.900000", "2.000000", "2.050000", "2.080000",
		"2.100000", "2.200000",
	})
	_, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		TimeStart:          2.0,
		TimeEnd:            2.1,
		TimeStartSet:       true,
		TimeEndSet:         true,
		TimePaddingBefore:  0.5,
		TimePaddingAfter:   0.5,
		AllowWindowedParse: true,
		MaxEvents:          4,
	})
	var limitErr *IndexEventLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("budget trip at ts==TimeEnd would drop an endpoint event; must stay a hard denial, got %T %v", err, err)
	}
}

// TestBuildIndexPaddingDegradeDeniesOnClockRegression pins conjunct (2) of
// the criterion: a trace with any observed clock regression loses the
// monotonicity proof that everything after the trigger is also past TimeEnd,
// so the degrade must not fire even when the trigger itself is in the padding
// tail. ClockRegressions is the in-loop run-time signal (incremented before
// the budget guard within the same iteration), so it is visible here.
func TestBuildIndexPaddingDegradeDeniesOnClockRegression(t *testing.T) {
	dir := t.TempDir()
	// 2.02 after 2.05 is a clock regression inside the padded window.
	// MaxEvents=5 admits through 2.10; trigger 2.30 is beyond TimeEnd, but
	// the regression forbids the degrade.
	path := paddingDegradeTraceLines(t, dir, "regressed.systrace", []string{
		"1.900000", "2.000000", "2.050000", "2.020000",
		"2.100000", "2.300000", "2.400000",
	})
	_, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		TimeStart:          2.0,
		TimeEnd:            2.1,
		TimeStartSet:       true,
		TimeEndSet:         true,
		TimePaddingBefore:  0.5,
		TimePaddingAfter:   0.5,
		AllowWindowedParse: true,
		MaxEvents:          5,
	})
	var limitErr *IndexEventLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("clock-regressed trace must keep the hard denial, got %T %v", err, err)
	}
}

// TestBuildIndexPaddingDegradeWithoutHeadEvents pins the QF3 fix: the old
// FirstTs <= TimeStart conjunct was structurally redundant for head coverage
// (the window gate plus anchor seek guarantee parsing starts at or before the
// padded window head) and was ALWAYS false when no event exists at or before
// TimeStart — a trace starting mid-window, or a time_start=0 request — which
// demoted a perfectly degradable shape to a hard denial. Both shapes must now
// degrade when the budget trips strictly beyond TimeEnd.
func TestBuildIndexPaddingDegradeWithoutHeadEvents(t *testing.T) {
	cases := []struct {
		name      string
		timeStart float64
	}{
		// Trace's first event 2.05 > TimeStart 2.0: nothing at/before the
		// window start exists in the file at all.
		{name: "trace_starts_mid_window", timeStart: 2.0},
		// time_start=0: FirstTs > 0 makes the old conjunct unconditionally
		// false for any real trace.
		{name: "time_start_zero", timeStart: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			// MaxEvents=4 admits 2.05/2.08/2.10/2.20; trigger 2.30 > TimeEnd.
			path := paddingDegradeTraceLines(t, dir, "midstart.systrace", []string{
				"2.050000", "2.080000", "2.100000",
				"2.200000", "2.300000", "2.400000",
			})
			idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
				TimeStart:          tc.timeStart,
				TimeEnd:            2.1,
				TimeStartSet:       true,
				TimeEndSet:         true,
				TimePaddingBefore:  0.5,
				TimePaddingAfter:   0.5,
				AllowWindowedParse: true,
				MaxEvents:          4,
			})
			if err != nil {
				t.Fatalf("no-head-event window must degrade, not deny (QF3): %v", err)
			}
			if !idx.PaddingTruncated {
				t.Fatalf("expected typed PaddingTruncated marker: %+v", idx)
			}
			if idx.PaddingTruncatedLastTs != 2.3 {
				t.Fatalf("PaddingTruncatedLastTs must be the trigger-point LastTs, got %.6f", idx.PaddingTruncatedLastTs)
			}
			if len(idx.Events) != 4 {
				t.Fatalf("degrade must keep the events parsed so far, got %d", len(idx.Events))
			}
		})
	}
}

// TestBuildIndexEventLimitStillDeniesWhenWindowNotCovered pins the unchanged
// hard denial: when the budget trips at an event still inside the requested
// window (ev.Ts <= TimeEnd, window not yet fully parsed), losing events would
// corrupt the core window, so IndexEventLimitError must keep firing.
func TestBuildIndexEventLimitStillDeniesWhenWindowNotCovered(t *testing.T) {
	dir := t.TempDir()
	path := paddingDegradeTraceLines(t, dir, "dense.systrace", []string{
		"2.000000", "2.010000", "2.020000", "2.030000", "2.040000",
	})
	_, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		TimeStart:          2.0,
		TimeEnd:            2.1,
		TimeStartSet:       true,
		TimeEndSet:         true,
		TimePaddingBefore:  0.5,
		TimePaddingAfter:   0.5,
		AllowWindowedParse: true,
		MaxEvents:          3,
	})
	var limitErr *IndexEventLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("in-window budget hit must stay a hard denial, got %T %v", err, err)
	}
	if limitErr.Events != 3 {
		t.Fatalf("unexpected limit metadata: %+v", limitErr)
	}
}

// TestBuildIndexPaddingDegradeRequiresCompleteTimeWindow pins conjunct (1) of
// the criterion: the degrade reads TimeStartSet && TimeEndSet — a half-open
// window has no TimeEnd to prove tail coverage against, so it keeps the
// denial even when plenty of tail events were parsed.
func TestBuildIndexPaddingDegradeRequiresCompleteTimeWindow(t *testing.T) {
	dir := t.TempDir()
	path := paddingDegradeTraceLines(t, dir, "halfopen.systrace", []string{
		"1.900000", "2.000000", "2.050000", "2.100000",
		"2.200000", "2.300000", "2.400000",
	})
	_, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		TimeStart:          2.0,
		TimeStartSet:       true,
		TimePaddingBefore:  0.5,
		AllowWindowedParse: true,
		MaxEvents:          5,
	})
	var limitErr *IndexEventLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("half-open window budget hit must stay a hard denial, got %T %v", err, err)
	}
}

// TestArtifactPathListPropagatesPaddingTruncated pins the multi-artifact
// merge: a padding-tail-degraded child must keep its typed marker, note, and
// parse-boundary field on the merged index, or the bundle path would silently
// present a truncated build as complete (and strand the query layer without
// the typed boundary).
func TestArtifactPathListPropagatesPaddingTruncated(t *testing.T) {
	dir := t.TempDir()
	degraded := paddingDegradeTraceLines(t, dir, "child_degraded.systrace", []string{
		"1.900000", "2.000000", "2.050000", "2.100000",
		"2.200000", "2.300000", "2.400000",
	})
	// All events beyond the padded window: parses to zero retained events, so
	// the merged MaxEvents guard stays quiet and only the marker matters.
	empty := paddingDegradeTraceLines(t, dir, "child_out_of_window.systrace", []string{
		"5.000000",
	})
	idx, err := parseTraceArtifactPathList(context.Background(), filepath.Join(dir, "bundle"), 0, 0, BuildOptions{
		TimeStart:          2.0,
		TimeEnd:            2.1,
		TimeStartSet:       true,
		TimeEndSet:         true,
		TimePaddingBefore:  0.5,
		TimePaddingAfter:   0.5,
		AllowWindowedParse: true,
		MaxEvents:          5,
	}, []string{degraded, empty})
	if err != nil {
		t.Fatalf("degraded child must merge without error: %v", err)
	}
	if !idx.PaddingTruncated || idx.PaddingTruncatedNote == "" {
		t.Fatalf("merged index must keep the child's typed degrade marker: %+v", idx)
	}
	if !strings.Contains(idx.PaddingTruncatedNote, "parsed through ts=2.300000") {
		t.Fatalf("merged note must keep the child's parse boundary, got %q", idx.PaddingTruncatedNote)
	}
	if idx.PaddingTruncatedLastTs != 2.3 {
		t.Fatalf("merged index must keep the child's PaddingTruncatedLastTs, got %.6f", idx.PaddingTruncatedLastTs)
	}
}
