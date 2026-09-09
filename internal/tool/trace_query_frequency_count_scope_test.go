package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the actual producer/publication boundary, not just a hand-built
// authority. The mixed records deliberately make total count != tuple count.
func TestTraceQueryExecuteFrequencyPolicyCountScope(t *testing.T) {
	dir := t.TempDir()
	body := strings.Join([]string{
		`policy-10 (10) [004] .... 0.900000: cpu_frequency_limits: min=0 max=1000000 cpu_id=4`,
		`policy-10 (10) [004] .... 1.100000: cpu_frequency_limits: min=558000 max=2270000 cpu_id=4`,
		`policy-10 (10) [004] .... 1.200000: cpu_frequency_limits: min=558000 max=2100000 cpu_id=4`,
		`policy-10 (10) [004] .... 1.300000: cpu_frequency_limits: min=558000 max=2270000 cpu_id=4`,
		`policy-10 (10) [004] .... 1.400000: cpu_frequency_limits: min=700000 max=2100000 cpu_id=4`,
		`policy-10 (10) [001] .... 1.500000: cpu_frequency_limits: min=400000 max=1500000 cpu_id=1`,
		`policy-10 (10) [004] .... 1.600000: cpu_frequency_limits: min=0 max=0 cpu_id=4`,
		`policy-10 (10) [006] .... 1.700000: cpu_frequency_limits: min=0 max=0 cpu_id=6`,
		`policy-10 (10) [004] .... 2.100000: cpu_frequency_limits: min=0 max=1000000 cpu_id=4`,
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "policy.systrace"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	params := json.RawMessage(`{"source":"path","path":"policy.systrace","view":"window_stats","time_start":1,"time_end":2}`)
	result, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
	if err != nil || !result.Success {
		t.Fatalf("execute: err=%v result=%+v", err, result)
	}
	if result.TraceEvidenceAuthority == nil {
		t.Fatal("missing authority")
	}
	witnesses := result.TraceEvidenceAuthority.FrequencyLimitWitnesses
	if len(witnesses) != 2 {
		t.Fatalf("zero-only inventory must not mint a positive policy ceiling: %+v", witnesses)
	}
	var found bool
	for _, witness := range witnesses {
		if witness.CPU != 4 {
			continue
		}
		found = true
		if witness.LimitRowCount != 5 || witness.MinFrequencyKHz != 558000 || witness.MaxFrequencyKHz != 2100000 || witness.WitnessLine != 3 || witness.WitnessTs != 1.2 || witness.WindowStartTs != 1 || witness.WindowEndTs != 2 {
			t.Fatalf("total count, first strictest row or query scope changed: %+v", witness)
		}
	}
	if !found {
		t.Fatal("CPU4 witness missing")
	}
	for _, prefix := range []string{"frequency_limit_witness cpu=4 ", "- cpu_frequency_limit cpu=4 "} {
		line := frequencyPolicySummaryLineForTest(t, result.Summary, prefix)
		for _, want := range []string{"min=558000kHz max=2100000kHz", "count_scope=all_valid_policy_rows_in_query_scope", "selected_row_scope=strictest_positive_maximum", "not the selected min/max pair's repetition count or duration"} {
			if !strings.Contains(line, want) {
				t.Errorf("%s missing %q: %s", prefix, want, line)
			}
		}
	}
	zero := frequencyPolicySummaryLineForTest(t, result.Summary, "- cpu_frequency_limit cpu=6 ")
	if !strings.Contains(zero, "selected_row_scope=first_valid_row_no_positive_maximum") || !strings.Contains(zero, "count=1") {
		t.Errorf("zero-only row must retain inventory without a positive ceiling: %s", zero)
	}
}

func frequencyPolicySummaryLineForTest(t *testing.T, summary, prefix string) string {
	t.Helper()
	for _, line := range strings.Split(summary, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	t.Fatalf("missing %q in actual publication:\n%s", prefix, summary)
	return ""
}
