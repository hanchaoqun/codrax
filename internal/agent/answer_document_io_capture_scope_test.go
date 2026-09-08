package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func b1618IOCaptureRecord(id, path, predicate string) types.ObservationRecord {
	return types.ObservationRecord{
		ID: id, Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: types.ClaimGroundingHard, Subject: "target-41", Predicate: predicate,
		SourceRef: types.ObservationSourceRef{ArtifactID: "trace_query", Path: path},
		RichNotes: []string{"selected_window=10.000000..10.010000"},
	}
}

func TestB1618RealIOBridgeKeepsSchedulerRosterAndCompletionRulers(t *testing.T) {
	for _, tc := range []struct {
		name, state, reason string
		waitCount           int
		waitMS              float64
	}{
		{"S_completion_closed", "S", "", 0, 0},
		{"D_completion_closed", "D", "", 1, .240},
		{"S_with_scheduler_iowait", "S", "idle-0 (0) [004] .... 1.150060: sched_blocked_reason: pid=41 iowait=1 caller=io_schedule\n", 1, .240},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "trace.ftrace")
			content := fmt.Sprintf(`target-41 (41) [004] .... 1.150000: block_rq_issue: 12,80 RCVHS 32768 () 923339752 + 64 [target]
target-41 (41) [004] .... 1.150050: sched_switch: prev_comm=target prev_pid=41 prev_prio=53 prev_state=%s ==> next_comm=idle/4 next_pid=0 next_prio=120
%sudk-irq-4-80 (2) [004] .... 1.150275: block_rq_complete: 12,80 RCVHS () 923339752 + 64 [0]
udk-irq-4-80 (2) [004] .... 1.150290: sched_wakeup: comm=target pid=41 prio=53 target_cpu=004
target-41 (41) [004] .... 1.150330: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=53
`, tc.state, tc.reason)
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			records := b1607ScopeQuery(t, dir, path, 1, 1.2)
			ctx := b1618IOCaptureContext()
			ledger := types.ObservationLedger{Records: records}
			before, _ := json.Marshal(ledger)
			rosters := types.BuildTargetWaitOccurrenceAuthorities(ledger, &ctx.AnalysisIR.RequestModel)
			if len(rosters) != 1 || rosters[0].Count != tc.waitCount || fmt.Sprintf("%.3f", rosters[0].SumMS) != fmt.Sprintf("%.3f", tc.waitMS) {
				t.Fatalf("real producer premise must publish the original scheduler roster: %+v", rosters)
			}
			var ioRecords []types.ObservationRecord
			for _, r := range records {
				if r.Predicate == "io_latency" || r.Predicate == "io_latency_coverage" {
					ioRecords = append(ioRecords, r)
				}
			}
			got := renderAnswerDocIOMeasurementRelationBridge(ctx, ledger, ioRecords)
			for _, want := range []string{
				"largest visible 0.275ms", "union=0.240ms",
				fmt.Sprintf("Scheduler-marked IO-wait roster: %d occurrence(s), sum=%.3fms", tc.waitCount, tc.waitMS),
			} {
				if !strings.Contains(got, want) {
					t.Errorf("real producer→bridge lost independently measured ruler %q: %s", want, got)
				}
			}
			after, _ := json.Marshal(ledger)
			if string(before) != string(after) {
				t.Fatal("IO bridge changed original observations")
			}
		})
	}
}

func b1618IOCaptureWait(id, path string, durationMS float64) types.ObservationRecord {
	r := b1618IOCaptureRecord(id, path, "io_latency")
	r.Value, r.Unit = fmt.Sprintf("%.3f", durationMS), "ms"
	r.Span = types.ObservationSpan{StartTs: 10.001, EndTs: 10.001 + durationMS/1000}
	r.RichNotes = append(r.RichNotes,
		"request_residence_caliber=block_rq_issue_to_complete", "request_residence="+r.Value,
		"completion_woke_issuer=true", "causal_wait_caliber=completion_closed_issuer_blocked",
		"issuer_blocked_state=s_sleep", "issuer_blocked="+r.Value,
		"issuer_blocked_start=10.001000", fmt.Sprintf("issuer_blocked_end=%.6f", r.Span.EndTs))
	return r
}

func b1618IOCaptureContext() *types.AgentContext {
	return &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		RuntimeTargets:         []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 41, Thread: "target-41", Source: "user_explicit"}},
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedEffectVerdict, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactIOLatency}},
	}}}
}

func TestB1618CompletionClosedFactMatchesStateEvidenceCapture(t *testing.T) {
	a := b1618IOCaptureRecord("state-a", "/capture/a/trace.ftrace", "target_window_states")
	b := b1618IOCaptureRecord("state-b", "/capture/b/trace.ftrace", "target_window_states")
	wa := b1618IOCaptureWait("wait-a", a.SourceRef.Path, 1)
	wb := b1618IOCaptureWait("wait-b", b.SourceRef.Path, 3)
	ctx := b1618IOCaptureContext()
	for _, reverse := range []bool{false, true} {
		for _, zh := range []bool{false, true} {
			t.Run(fmt.Sprintf("reverse=%t/zh=%t", reverse, zh), func(t *testing.T) {
				records := []types.ObservationRecord{a, b, wa, wb}
				if reverse {
					records = []types.ObservationRecord{wb, wa, b, a}
				}
				ledger := types.ObservationLedger{Records: records}
				for _, check := range []struct{ id, want, forbidden string }{{a.ID, "1.000", "3.000"}, {b.ID, "3.000", "1.000"}} {
					state := types.TraceTargetStateScopeAuthority{EvidenceID: check.id, Subject: "target-41", WindowStartTs: 10, WindowEndTs: 10.010}
					got := renderAnswerDocBoundedRuntimeCompletionClosedReaderFact(ledger, &ctx.AnalysisIR.RequestModel, state, zh)
					if !strings.Contains(got, check.want) || strings.Contains(got, check.forbidden) {
						t.Fatalf("state %s borrowed a different capture's wait: %s", check.id, got)
					}
				}
			})
		}
	}
}

func TestB1618CompletionClosedFactDoesNotGuessMissingOrAmbiguousStateSource(t *testing.T) {
	ctx := b1618IOCaptureContext()
	a := b1618IOCaptureRecord("state", "/capture/a/trace.ftrace", "target_window_states")
	b := b1618IOCaptureRecord("state", "/capture/b/trace.ftrace", "target_window_states")
	wa := b1618IOCaptureWait("wait", b.SourceRef.Path, 3)
	for _, records := range [][]types.ObservationRecord{{wa}, {a, wa}, {a, b, wa}} {
		state := types.TraceTargetStateScopeAuthority{EvidenceID: "state", Subject: "target-41", WindowStartTs: 10, WindowEndTs: 10.010}
		got := renderAnswerDocBoundedRuntimeCompletionClosedReaderFact(types.ObservationLedger{Records: records}, &ctx.AnalysisIR.RequestModel, state, false)
		if !strings.Contains(got, "not assessed, not measured zero") || strings.Contains(got, "3.000") {
			t.Fatalf("unmatched/ambiguous state acquired another capture's IO ruler: %s", got)
		}
	}
}

func TestB1618IORelationBridgeKeepsOneExactCaptureAndWindow(t *testing.T) {
	ctx := b1618IOCaptureContext()
	a := b1618IOCaptureWait("wait-a", "/capture/a/trace.ftrace", 1)
	b := b1618IOCaptureWait("wait-b", "/capture/b/trace.ftrace", 3)
	ledger := types.ObservationLedger{Records: []types.ObservationRecord{a, b}}
	got := renderAnswerDocIOMeasurementRelationBridge(ctx, ledger, []types.ObservationRecord{b})
	if !strings.Contains(got, "union=3.000ms") || strings.Contains(got, "union=1.000ms") {
		t.Fatalf("B's request borrowed A's completion ruler: %s", got)
	}
	for _, records := range [][]types.ObservationRecord{{a, b}, {b, a}} {
		got = renderAnswerDocIOMeasurementRelationBridge(ctx, ledger, records)
		if strings.Contains(got, "union=") || strings.Contains(got, "largest visible") {
			t.Fatalf("mixed capture request rows became a single IO ruler: %s", got)
		}
	}
	otherWindow := b
	otherWindow.ID = "other-window"
	otherWindow.RichNotes = append([]string{"selected_window=20.000000..20.010000"}, b.RichNotes[1:]...)
	got = renderAnswerDocIOMeasurementRelationBridge(ctx, ledger, []types.ObservationRecord{b, otherWindow})
	if strings.Contains(got, "union=") || strings.Contains(got, "largest visible") {
		t.Fatalf("different windows became one ruler: %s", got)
	}
}

func TestB1618IOPhysicalKeyUsesCaptureNotChannelLabel(t *testing.T) {
	a := b1618IOCaptureWait("a", "/capture/A/trace.ftrace", 1)
	a.RichNotes = append(a.RichNotes, "io_endpoint_family=block_rq", "dev=12,80", "io_sector=77", "io_len=64",
		"io_issue_ts=10.001000", "io_complete_ts=10.002000", "io_issue_thread=target-41")
	b := a
	b.ID, b.SourceRef.Path = "b", "/capture/a/trace.ftrace"
	if answerDocBoundedRuntimeFactPhysicalKey(a) == answerDocBoundedRuntimeFactPhysicalKey(b) {
		t.Fatal("identical event coordinates in distinct case-sensitive capture paths were deduplicated")
	}
	c := a
	c.ID, c.SourceRef.ArtifactID = "later-publication", "attached_trace"
	if answerDocBoundedRuntimeFactPhysicalKey(a) != answerDocBoundedRuntimeFactPhysicalKey(c) {
		t.Fatal("same physical capture acquired a different identity from a channel marker")
	}
}

func TestB1618IORelationBridgeNeverBorrowsUnscopedOrConflictingCoverage(t *testing.T) {
	ctx := b1618IOCaptureContext()
	pair := b1618IOCaptureWait("pair", "/capture/a/trace.ftrace", 1)
	coverage := b1618IOCaptureRecord("coverage", pair.SourceRef.Path, "io_latency_coverage")
	coverage.Subject, coverage.Value = "block_request_pairs", "3"
	coverage.RichNotes = append(coverage.RichNotes, "io_latency_emitted=2", "total=3", "io_latency_overflow_pairs=1", "io_latency_overflow_request_ms=9.000")
	other := coverage
	other.ID, other.Value = "other-result", "5"
	for _, records := range [][]types.ObservationRecord{{pair, coverage, other}, {pair, other, coverage}} {
		got := renderAnswerDocIOMeasurementRelationBridge(ctx, types.ObservationLedger{Records: records}, records)
		if !strings.Contains(got, "union=1.000ms") || strings.Contains(got, "Global selected-window block-request coverage:") {
			t.Fatalf("conflicting coverage counts were picked by record order: %s", got)
		}
	}
	coverage.SourceRef.Path = "" // generic channel ID carries no source authority
	got := renderAnswerDocIOMeasurementRelationBridge(ctx, types.ObservationLedger{Records: []types.ObservationRecord{pair, coverage}}, []types.ObservationRecord{pair, coverage})
	if strings.Contains(got, "Global selected-window block-request coverage:") {
		t.Fatalf("unscoped coverage was silently attributed to the only known capture: %s", got)
	}
}

func TestB1618IORelationBridgeRejectsDifferentCoverageIdentities(t *testing.T) {
	ctx := b1618IOCaptureContext()
	pair := b1618IOCaptureWait("pair", "/capture/a/trace.ftrace", 1)
	coverage := b1618IOCaptureRecord("coverage", pair.SourceRef.Path, "io_latency_coverage")
	coverage.Subject, coverage.Value, coverage.Unit = "block_request_pairs", "3", "count"
	coverage.RichNotes = append(coverage.RichNotes, "io_latency_emitted=2", "total=3", "io_latency_overflow_pairs=1")
	for _, field := range []string{"subject", "object", "unit"} {
		t.Run(field, func(t *testing.T) {
			other := coverage
			other.ID = "other-result"
			switch field {
			case "subject":
				other.Subject = "target-41"
			case "object":
				other.Object = "another-population"
			case "unit":
				other.Unit = "ms"
			}
			for _, records := range [][]types.ObservationRecord{{pair, coverage, other}, {pair, other, coverage}} {
				got := renderAnswerDocIOMeasurementRelationBridge(ctx, types.ObservationLedger{Records: records}, records)
				if !strings.Contains(got, "union=1.000ms") || strings.Contains(got, "Global selected-window block-request coverage:") {
					t.Fatalf("matching counts hid distinct coverage facts: %s", got)
				}
			}
		})
	}
}
