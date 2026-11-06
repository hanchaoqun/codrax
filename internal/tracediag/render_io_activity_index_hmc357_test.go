package tracediag

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func evalcaseQualifiedIOIndexRows(t *testing.T, report string, wantCount int) []string {
	t.Helper()
	var rows []string
	for _, row := range strings.Split(report, "\n") {
		if !strings.HasPrefix(row, "- window_stats.io_pressure_summary:") {
			continue
		}
		rows = append(rows, row)
		for _, want := range []string{
			"Only admitted values contribute to this background index.",
			"An unaccounted duration is not a measured zero.",
			"These inputs do not establish device queue state, target blocking time or a cause of delay.",
		} {
			if !strings.Contains(row, want) {
				t.Errorf("index report lost its qualification %q: %s", want, row)
			}
		}
		for _, forbidden := range []string{"io_wait=0.000ms", "d_state=0.000ms", "…(截断)"} {
			if strings.Contains(row, forbidden) {
				t.Errorf("index report used a false zero or clipped the account %q: %s", forbidden, row)
			}
		}
	}
	if len(rows) != wantCount {
		t.Fatalf("got %d qualified index rows, want %d:\n%s", len(rows), wantCount, report)
	}
	return rows
}

func TestIOActivityIndexReportKeepsCountOnlyMethodAndBoundaryTogether(t *testing.T) {
	dir := t.TempDir()
	script, capture := filepath.Join(dir, "window.yaml"), filepath.Join(dir, "markers.systrace")
	if err := os.WriteFile(script, []byte("version: 1\nsteps:\n  - label: marker_account\n    view: window_stats\n    window: 1.000000..1.010000\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A blocked-reason marker has an identity but no closed scheduling state.
	// A one-marker index has a count method; it is not an elapsed duration.
	if err := os.WriteFile(capture, []byte("worker-41 (41) [000] .... 1.005000: sched_blocked_reason: pid=41 iowait=1 caller=wait_on_page\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var report bytes.Buffer
	failed, err := Run(nil, Options{ScriptPath: script, TracePath: capture}, &report)
	if err != nil || failed != 0 {
		t.Fatalf("public report failed: steps=%d err=%v\n%s", failed, err, report.String())
	}
	row := evalcaseQualifiedIOIndexRows(t, report.String(), 1)[0]
	evalcaseWantTokens(t, "count-only index", row, []string{
		"signal=blocked_reason_iowait_count_only activity_index=5.000",
		"evidence_quality=activity_marker_only score_caliber=count_weighted_activity_index",
		"block_max=not_accounted storage_max=not_accounted",
		"iowait_blocked=1 d_state=not_accounted io_wait=not_accounted",
		"score_breakdown=iowait_blocked_count(1)*5=5.000",
		"comparison_scope=same_score_caliber_capture_conditions_and_window_duration",
		"absolute_level=not_defined",
	})
}
