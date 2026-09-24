package tool

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestSchedulerStateAccountingSummaryPublicWidthPreservesLaterFamilies(t *testing.T) {
	result := wsrB3ExecuteWindowStats(t, true)
	full := result.Summary
	raw, err := os.ReadFile(result.RawRef)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(raw) {
		full = string(raw)
	}
	if strings.Count(full, types.TraceSchedulerStateAccountingSummaryGuidance) != 1 || strings.Contains(full, "times are accounted contributions, envelope is not a continuous interval") {
		t.Fatal("summary repeated the detailed ruler instead of one typed legend")
	}
	// Measure all added accounting bytes, not the possibly shorter preview.
	// The preexisting result already offloads; do not redefine that contract
	// as "all output must fit inline". The repeated long ruler used over 4KiB
	// and hid independent populations; compact metadata stays within 1/8 of
	// the existing inline budget while the original visibility pins remain.
	accountingBytes := len("- " + types.TraceSchedulerStateAccountingSummaryGuidance + "\n")
	for _, line := range strings.Split(full, "\n") {
		if _, suffix, ok := strings.Cut(line, "; account=cumulative("); ok {
			accountingBytes += len("; account=cumulative(") + len(suffix)
		}
	}
	if accountingBytes > MaxInlineBytes/8 {
		t.Fatalf("repeated accounting grew summary by %d bytes (full=%d, existing budget=%d)", accountingBytes, len(full), MaxInlineBytes)
	}
	t.Logf("full_summary_bytes=%d accounting_bytes=%d existing_inline_budget=%d", len(full), accountingBytes, MaxInlineBytes)
	for _, prefix := range []string{"- process_cpu_load process=com.baidu.tieba-59566", "- process_domain_census_thread NetworkService-60595", "- top_running ", "- state_churn "} {
		if !strings.Contains(result.Summary, prefix) {
			t.Fatalf("later measured family lost: %s", prefix)
		}
	}
	var payloadPath string
	for _, row := range result.Observations {
		if row.SourceRef.PayloadRef != "" {
			payloadPath = row.SourceRef.PayloadRef
			break
		}
	}
	data, err := os.ReadFile(payloadPath)
	if err != nil {
		t.Fatal(err)
	}
	var payload tracequery.Result
	if err := json.Unmarshal(data, &payload); err != nil || payload.WindowStats == nil {
		t.Fatalf("full native payload unavailable: %v", err)
	}
	openAccounts := 0
	for _, rows := range [][]tracequery.ThreadDuration{payload.WindowStats.TopRunning, payload.WindowStats.RunnableTop, payload.WindowStats.SleepTop, payload.WindowStats.DStateTop, payload.WindowStats.IOWaitTop} {
		for _, row := range rows {
			if row.Accounting != nil && row.Accounting.OpenTailCount > 0 {
				openAccounts++
				if row.Accounting.OpenTailMs <= 0 || !strings.Contains(full, types.TraceSchedulerStateAccountingSummary(row.Accounting)) {
					t.Fatalf("open contribution lost from payload or short summary: %+v", row.Accounting)
				}
			}
		}
	}
	if openAccounts == 0 {
		t.Fatal("fixture did not exercise nonzero open contributions")
	}
}
