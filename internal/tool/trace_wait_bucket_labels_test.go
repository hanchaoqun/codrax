package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// These are disjoint accounting buckets, not separate physical meanings:
// a D-opened IO-marked interval remains physically D while its non-IO bucket
// count is zero. The public producer/emit test independently supplies raw events.
func TestTraceWaitBucketLabelsOccurrenceCounts(t *testing.T) {
	for _, tc := range []struct {
		name         string
		d, io, sleep int
	}{
		{"D_IO", 0, 1, 0},
		{"D_non_IO", 1, 0, 0},
		{"S_IO", 0, 0, 1},
		{"mixed", 2, 3, 1},
	} {
		for _, zh := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/zh=%t", tc.name, zh), func(t *testing.T) {
				wait := types.TraceTargetWaitSummaryAuthority{
					Count: tc.d + tc.io + tc.sleep, DStateOccurrences: tc.d,
					IOWaitOccurrences: tc.io, SleepIOWaitOccurrences: tc.sleep,
					WallClockMS: 2, Callers: []string{"wait_site"},
				}
				before, _ := json.Marshal(wait)
				got := runtimeTraceTargetWaitSummarySuffix(wait, nil, zh)
				want := fmt.Sprintf("(%s %d, scheduler-marked IO wait %d, interruptible sleep carrying an IO-wait marker %d, other 0)", TraceStateNonIODStateWord(zh), tc.d, tc.io, tc.sleep)
				if zh {
					want = fmt.Sprintf("（%s %d、调度器标记的 IO 等待 %d、带 IO 等待标记的可中断睡眠 %d、其他 0）", TraceStateNonIODStateWord(zh), tc.d, tc.io, tc.sleep)
				}
				if !strings.Contains(got, want) {
					t.Errorf("exclusive D bucket has a physical-total label: want %q in %s", want, got)
				}
				if !strings.Contains(got, "2.000ms") || !strings.Contains(got, "wait_site") {
					t.Errorf("label change lost amount or caller: %s", got)
				}
				after, _ := json.Marshal(wait)
				if string(before) != string(after) {
					t.Fatal("display changed the typed wait authority")
				}
			})
		}
	}
}

func TestTraceWaitBucketLabelsAuthorityLeadExplainsDProvenance(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			bus := runtimeWaitCoverageTestBus()
			bus.AnalysisIR.AnswerContract.Language = lang
			const modelText = "Model-owned text: D state 0, IO 1."
			doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "model", Kind: types.BlockSummary, Text: modelText}}}
			if !materializeRuntimeTraceTargetStateAuthorityBlock(doc, bus) {
				t.Fatal("fixture must materialize the existing target-state card")
			}
			block := answerDocumentTestBlockByID(t, doc, runtimeTraceTargetStateAuthorityBlockID)
			lead := strings.SplitN(block.Text, "\n\n", 2)[0]
			provenance := "D-state provenance"
			if lang == "zh" {
				provenance = "D 状态来源"
			}
			for _, want := range []string{TraceStateNonIODStateWord(lang == "zh"), provenance} {
				if !strings.Contains(lead, want) {
					t.Errorf("system lead does not distinguish the non-IO bucket from physical D: missing %q in %s", want, lead)
				}
			}
			if doc.Blocks[0].ID != "model" || doc.Blocks[0].Text != modelText {
				t.Fatal("system label repair rewrote model prose")
			}
			if materializeRuntimeTraceTargetStateAuthorityBlock(doc, bus) {
				t.Fatal("same typed authority must remain idempotent")
			}
		})
	}
}

func TestTraceWaitBucketLabelsSleepInventoryKeepsSplitValues(t *testing.T) {
	inventory := &tracequery.TargetWindowSleepInventory{Total: 3, Emitted: 3, TotalMs: 10, SleepMs: 1, DStateMs: 2, IOWaitMs: 7}
	before, _ := json.Marshal(inventory)
	got := traceSleepInventorySummary(inventory)
	for _, want := range []string{"union=10.000ms", "S=1.000ms", TraceStateNonIODStateWord(false) + "=2.000ms", "scheduler IO=7.000ms", "returned=3/3"} {
		if !strings.Contains(got, want) {
			t.Errorf("sleep inventory lost the accounting label/value %q: %s", want, got)
		}
	}
	after, _ := json.Marshal(inventory)
	if string(before) != string(after) {
		t.Fatal("summary changed the inventory")
	}
}

func TestTraceWaitBucketLabelsRawAndActualSnapshots(t *testing.T) {
	record := types.ObservationRecord{
		Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", Predicate: "state_churn", Subject: "worker-77", Value: "10", Unit: "ms",
		RichNotes: []string{
			types.TraceNoteKeyRunning + "=1.000", types.TraceNoteKeyRunnable + "=1.000", types.TraceNoteKeySleep + "=1.000",
			types.TraceNoteKeyDState + "=2.000", types.TraceNoteKeyIOWait + "=5.000", types.TraceNoteKeyTotal + "=10.000",
			types.TraceNoteKeyFragments + "=5", types.TraceNoteKeySwitches + "=4", types.TraceNoteKeyMaxSegment + "=5.000", types.TraceNoteKeyP95Segment + "=5.000",
			types.TraceNoteKeyActualDState + "=3.000", types.TraceNoteKeyActualIOWait + "=7.000", types.TraceNoteKeyActualTotalMS + "=10.000",
		},
	}
	before, _ := json.Marshal(record)
	for _, zh := range []bool{true, false} {
		t.Run(fmt.Sprintf("zh=%t", zh), func(t *testing.T) {
			raw := runtimeTraceMetricSnapshotDisplayText(record, zh)
			actual := runtimeTraceMetricSnapshotActualInline(record, zh)
			for name, pair := range map[string][2]string{
				"raw":    {raw, TraceStateNonIODStateWord(zh) + " 2.000ms"},
				"actual": {actual, TraceStateNonIODStateWord(zh) + " 3.000ms"},
			} {
				if !strings.Contains(pair[0], pair[1]) {
					t.Errorf("%s D bucket was folded or mislabeled: want %q in %s", name, pair[1], pair[0])
				}
			}
			if !strings.Contains(raw, "iowait 5.000ms") || !strings.Contains(actual, "iowait 7.000ms") {
				t.Errorf("IO companion values changed: raw=%s actual=%s", raw, actual)
			}
		})
	}
	after, _ := json.Marshal(record)
	if string(before) != string(after) {
		t.Fatal("snapshot wording rewrote raw observation notes")
	}
}

func TestTraceWaitBucketLabelsDoNotRelabelPhysicalDOrFoldSleepIOIntoD(t *testing.T) {
	account := types.TraceTargetStateScopeAuthority{Subject: "worker-77", DStateMS: 3, IOWaitMS: 7, SleepMS: 4, SleepIOWaitMS: 2, TotalMS: 14}
	for _, lang := range []string{"zh", "en"} {
		got := types.FormatTargetStateAccount(account, lang)
		want := "uninterruptible wait 10.000 ms"
		if lang == "zh" {
			want = "不可中断等待 10.000 毫秒"
		}
		if !strings.Contains(got, want) || strings.Contains(got, TraceStateNonIODStateWord(lang == "zh")) {
			t.Errorf("the native D fold must keep D+IO=10, with S-IO outside it: %s", got)
		}
	}
	if got := runtimeTraceWaitOccurrenceStateLabel("d_sleep", true); got != "D 状态（不可中断等待）" {
		t.Errorf("an individual D interval is still physically D: %q", got)
	}
}
