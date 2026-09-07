package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1598WaitReaderDisplayPreservesRowsAndModel(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			bus := runtimeWaitCoverageTestBus()
			bus.AnalysisIR.AnswerContract.Language = lang
			rm := &bus.AnalysisIR.RequestModel
			rm.Language = lang
			rm.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{
				Scope:        types.RuntimeQuestionScopeBoundedFactSet,
				FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetWaitOccurrences},
			}
			ref := bus.ToolResults[0].Observations[0].SourceRef
			target := ".ugc.aweme.lite-17267"
			kinds := []struct{ state, marker, caller string }{
				{"d_sleep", "0", "dma_fence_default_w+0x260/0x4dc[devhost.elf]"},
				{"d_sleep", "unknown", "unknown"},
				{"io_wait", "1", "sync_buffer_read_wi"},
				{"s_sleep", "1", "sleep_wait"},
				{"future_wait", "unknown", "future_symbol"},
			}
			count := len(kinds)
			records := []types.ObservationRecord{{
				ID: "trace_query:reader#target_window_wait_occurrences", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
				Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref,
				Span:      types.ObservationSpan{StartTs: 13762.791708, EndTs: 13763.024898},
				Predicate: "target_window_wait_occurrences", Subject: target, Object: "complete",
				Value: "5", ResultCount: &count,
			}}
			for i, kind := range kinds {
				start := 13762.800 + float64(i)*0.010
				records = append(records, types.ObservationRecord{
					ID:     fmt.Sprintf("trace_query:reader#target_window_wait_occurrence:%d", i+1),
					Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
					GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref,
					Span:      types.ObservationSpan{StartTs: start, EndTs: start + 0.001},
					Predicate: "target_window_wait_occurrence", Subject: target,
					Object: fmt.Sprintf("state=%s;iowait=%s;caller=%s", kind.state, kind.marker, kind.caller),
					Value:  "1.000", Unit: "ms",
				})
			}
			bus.ToolResults[0].Observations = append(bus.ToolResults[0].Observations, records...)
			before, _ := json.Marshal(bus.ToolResults)
			model := types.AnswerBlock{ID: "summary", Kind: types.BlockSummary, Text: "Model-owned d_sleep iowait=0 caller=verbatim stays unchanged."}
			doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{model}}
			result, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
			if err != nil || !result.Success {
				t.Fatalf("real persist failed: %+v / %v", result, err)
			}
			persisted := bus.Mutable.AnswerDocumentV2()
			if persisted == nil || persisted.Blocks[0].Text != model.Text || doc.Blocks[0].Text != model.Text {
				t.Fatal("reader localization changed the model-owned answer")
			}
			after, _ := json.Marshal(bus.ToolResults)
			if string(before) != string(after) {
				t.Fatal("reader localization changed typed observations")
			}
			surface := types.AnswerBlockVisibleSurface(answerDocumentTestBlockByID(t, persisted, runtimeTraceTargetStateAuthorityBlockID))
			wants := []string{"D state (uninterruptible wait)", "kernel IO-wait marker: not marked", "kernel IO-wait marker: marked", "kernel IO-wait marker: not provided", "kernel call-site/symbol: unresolved", "other wait state", "5.000ms", kinds[0].caller, "13762.800000..13762.801000"}
			if lang == "zh" {
				wants = []string{"D 状态（不可中断等待）", "内核 IO 等待标记：未标记", "内核 IO 等待标记：已标记", "内核 IO 等待标记：未提供", "内核调用点/符号：未解析", "其他等待状态", "5.000ms", kinds[0].caller, "13762.800000..13762.801000"}
			}
			for _, want := range wants {
				if !strings.Contains(surface, want) {
					t.Errorf("system reader face missing %q:\n%s", want, surface)
				}
			}
			for _, bad := range []string{"d_sleep", "io_wait", "s_sleep", "future_wait", "iowait=", "caller=", "没有 IO 等待", "no IO wait"} {
				if strings.Contains(surface, bad) {
					t.Errorf("system reader face leaked or overstated %q", bad)
				}
			}
			canonical := types.TargetWaitOccurrenceAuthorityRow{Ordinal: 1, State: "d_sleep", StartTs: 1, EndTs: 2, DurationM: 1000, IOWait: "0", Caller: "raw_symbol"}.CanonicalLine()
			if canonical != "#1 state=d_sleep 1.000000..2.000000 duration=1000.000ms iowait=0 caller=raw_symbol" {
				t.Fatalf("canonical wire/fingerprint text changed: %s", canonical)
			}
			// A reader label is not a new qualification: removing one exact
			// same-result occurrence must still withdraw the complete roster.
			bus.ToolResults[0].Observations = bus.ToolResults[0].Observations[:len(bus.ToolResults[0].Observations)-1]
			missingRowDoc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{model}}
			if materializeRuntimeTraceTargetStateAuthorityBlock(missingRowDoc, bus) ||
				len(missingRowDoc.Blocks) != 1 || missingRowDoc.Blocks[0].Text != model.Text {
				t.Fatal("reader labels promoted an incomplete roster or rewrote model prose")
			}
		})
	}
}

func TestB1598EventSearchExecuteSeparatesCountingFromReturnedRows(t *testing.T) {
	oldThreshold := traceQueryWindowedIndexMinBytes
	traceQueryWindowedIndexMinBytes = 1
	defer func() { traceQueryWindowedIndexMinBytes = oldThreshold }()
	dir := t.TempDir()
	var trace strings.Builder
	for i := 0; i < 5; i++ {
		fmt.Fprintf(&trace, "app-20 (20) [000] .... 9.%06d: print: B|20|needle\n", i)
	}
	if err := os.WriteFile(filepath.Join(dir, "events.systrace"), []byte(trace.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, pattern, want string
		limit               int
		incomplete          bool
	}{
		{"truncated", "needle", "engine returned 2 of 5 matched rows", 2, true},
		{"all", "needle", "engine returned 5 of 5 matched rows", 10, false},
		{"zero", "absent", "engine returned 0 of 0 matched rows", 10, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params, _ := json.Marshal(map[string]any{"source": "path", "path": "events.systrace", "view": "event_search", "pattern": tc.pattern, "limit": tc.limit})
			result, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
			if err != nil || !result.Success {
				t.Fatalf("query failed: %+v / %v", result, err)
			}
			for _, want := range []string{"enumeration_complete=true", "Match counting is complete", tc.want, "Counting completion does not mean all matched rows were returned or displayed"} {
				if !strings.Contains(result.Summary, want) {
					t.Errorf("real tool summary missing %q:\n%s", want, result.Summary)
				}
			}
			if result.EnumerationAuthority == nil || (result.EnumerationAuthority.Status == "incomplete") != tc.incomplete {
				t.Fatalf("display changed existing enumeration authority: %+v", result.EnumerationAuthority)
			}
		})
	}
	unknown := traceQuerySummary(tracequery.Result{View: "event_search", EventSearchCoverage: &tracequery.EventSearchCoverage{MatchedTotal: 4, Emitted: 4}}, traceQueryParams{}, "attached_trace", "")
	if !strings.Contains(unknown, "Match counting is not confirmed complete") || !strings.Contains(unknown, "4 matches counted so far") {
		t.Fatalf("equal counts must not invent a completed census:\n%s", unknown)
	}
}
