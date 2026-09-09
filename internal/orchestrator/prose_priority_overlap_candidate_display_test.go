package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1635bActualReconciliationAppendixKeepsCandidateCaliber(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "same-cpu.systrace")
	body := `app-20 (20) [001] .... 8.000000: cpu_frequency: state=1000000 cpu_id=1
rival-30 (30) [001] .... 8.010000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=R+ ==> next_comm=rival next_pid=30 next_prio=20
rival-30 (30) [001] .... 8.090000: sched_switch: prev_comm=rival prev_pid=30 prev_prio=20 prev_state=R+ ==> next_comm=app next_pid=20 next_prio=53
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "root_cause_rank", "pid": 20,
		"time_start": 8, "time_end": 8.1, "trace_flavor": "harmony_hitrace", "min_duration_ms": 0.05})
	result, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
	if err != nil || !result.Success {
		t.Fatalf("actual query: %v / %s", err, result.Summary)
	}
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			bus := &types.BusContext{Mutable: types.NewMutableState("candidate label"), Language: lang,
				AnalysisIR:  &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck}},
				ToolResults: []types.ToolResult{result}}
			doc := psgProseDoc("The root cause is app-20.")
			got, err := tool.ApplyAndPersistMutation(bus, "candidate_display_test", types.NewReplaceAllMutation(doc), nil, time.Now())
			if err != nil || !got.Success {
				t.Fatalf("publication: %v / %s", err, got.Summary)
			}
			before, _ := json.Marshal(bus.Mutable.AnswerDocumentV2())
			rowsBefore := tool.RuntimeTraceReconciliationRows(bus)
			var rank *tool.RuntimeTraceReconciliationRow
			for i := range rowsBefore {
				if row := &rowsBefore[i]; row.Kind == tool.RuntimeTraceReconciliationRankOne && row.CauseToken == "priority_inversion_runnable_wait" {
					rank = row
				}
			}
			if rank == nil || rank.Subject != "app-20" || rank.Rank != 1 || fmt.Sprintf("%.3f", rank.EffectiveMS) != "80.000" || rank.EvidenceTag == "" {
				t.Fatalf("fixture lacks the real positive candidate receipt: %+v", rowsBefore)
			}
			o := &Orchestrator{busCtx: bus}
			out := &agent.StageOutput{FinalAnswer: "The root cause is app-20."}
			o.attachSystemCrossCheckAppendix(out, "", nil)
			var published strings.Builder
			for _, attachment := range bus.Mutable.AnswerDisplayAttachments() {
				if attachment.Source == types.AnswerDisplayAttachmentSourceSystemCrossCheck {
					published.WriteString(attachment.Body)
				}
			}
			want := tool.TraceRootCauseTypeDisplayLabel(rank.CauseToken, lang == "zh")
			if !strings.Contains(published.String(), want) || !strings.Contains(published.String(), "80.000ms ["+rank.EvidenceTag+"]") {
				t.Fatalf("actual appendix lost candidate caliber or its numerical/source receipt, want %q:\n%s", want, published.String())
			}
			after, _ := json.Marshal(bus.Mutable.AnswerDocumentV2())
			// The final string is re-rendered with the new attachment. The
			// accepted document and its exact model sentence remain unchanged.
			if string(before) != string(after) || !strings.Contains(out.FinalAnswer, "The root cause is app-20.") || !reflect.DeepEqual(rowsBefore, tool.RuntimeTraceReconciliationRows(bus)) {
				t.Fatal("display changed model/system blocks or typed rank/source/window rows")
			}
		})
	}
}

func TestB1635bOfflineHeadlineLabelUsesSameCandidateCaliber(t *testing.T) {
	// This old diagnostic is deliberately disconnected from publication; the
	// shared display correction must not reconnect its prose-derived verdict.
	seat := proseHeadlineSeatRow{subject: "app-20", classToken: "priority_inversion_runnable_wait", rank: 1, hasEff: true, eff: 80}
	for _, zh := range []bool{true, false} {
		word := tool.TraceRootCauseTypeDisplayLabel(seat.classToken, zh)
		before := seat
		if got := proseHeadlineSeatLabel(seat, zh, true); !strings.Contains(got, word) || !strings.Contains(got, "80.000ms") {
			t.Errorf("offline label dropped candidate qualifier or value: %q", got)
		}
		if seat != before {
			t.Fatal("display changed the original diagnostic seat")
		}
	}
}

func TestB1635bEnglishSiblingAndUnknownFallbacksStayUnchanged(t *testing.T) {
	for _, token := range []string{"priority_inversion_candidate", "low_frequency", "running", "future_cause_token", ""} {
		seat := proseHeadlineSeatRow{classToken: token}
		if got := proseHeadlineSeatLabel(seat, false, false); got != token {
			t.Errorf("unrelated headline token changed: got %q want %q", got, token)
		}
		row := tool.RuntimeTraceReconciliationRow{Subject: "worker-7", CauseToken: token, EffectiveMS: 12.345, EvidenceTag: "E2"}
		want := "Like-for-like reference: root-cause rank #1 worker-7 / " + strings.ReplaceAll(token, "_", " ") + ", published attribution 12.345ms [E2]"
		if got := renderRankOneReconciliation(row).entry; got != want {
			t.Errorf("unrelated reconciliation token changed: got %q want %q", got, want)
		}
	}
}
