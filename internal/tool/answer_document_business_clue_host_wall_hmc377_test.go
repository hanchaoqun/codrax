package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Batch-377 integration ruling R-377-1 (business-clue face host basis): a
// thread that only holds a wakeup credential toward the analysed thread counts
// on the business-clue face only for the part of its spans inside the waits
// of the analysed thread its own wakeups ended, with the span wall shown
// beside it — the same rule as a chain member's row and as the approved
// dependency-credit bound. Before the integration the engine already counted
// the inside part for both bases, but every face keyed the "wall beside"
// clause on the chain-member basis only, so a host row that crossed the wait
// boundary published a part with no wall and a model summary claiming the
// spans lay inside the wait.
func TestHMC377HostEdgeBusinessClueShowsItsWallBeside(t *testing.T) {
	host := types.TraceCausalProjectionBusinessSpanMention{Subject: "binder_host-200", Name: "hostSpan", Count: 1,
		TotalMS: 5, MaxMS: 5, StartLine: 22, EndLine: 28, Basis: "host_wakeup_edge", WallMS: 20}
	if !types.TraceBusinessSpanMentionDependencyPart(host) {
		t.Fatalf("a host-edge row counted only inside the waits its wakeups ended is a dependency part: %+v", host)
	}
	whole := host
	whole.WallMS = whole.TotalMS
	if types.TraceBusinessSpanMentionDependencyPart(whole) {
		t.Fatalf("a host-edge row whose spans lie wholly inside the waits carries no wall clause: %+v", whole)
	}
	zh, ok := runtimeTraceProjBusinessSpanMentionRowText(host, nil, true)
	if !ok || !strings.Contains(zh, "合计5.000ms(仅计其唤醒结束的等待区间内部分,窗内span墙钟20.000ms) 行22..28 凭证:唤醒凭证(等待内)") {
		t.Fatalf("host-edge tree row must show the wall beside the counted part: %s", zh)
	}
	en, _ := runtimeTraceProjBusinessSpanMentionRowText(host, nil, false)
	if !strings.Contains(en, "total 5.000ms (only the part inside the waits its wakeups ended; in-window span wall 20.000ms)") {
		t.Fatalf("host-edge EN row must show the wall beside the counted part: %s", en)
	}
	wholeRow, _ := runtimeTraceProjBusinessSpanMentionRowText(whole, nil, true)
	if strings.Contains(wholeRow, "窗内span墙钟") {
		t.Fatalf("no wall clause when the wall equals the counted part: %s", wholeRow)
	}

	rank := &tracequery.RootCauseRankResult{BusinessSpanMentions: &tracequery.BusinessSpanMentionResult{Families: []tracequery.BusinessSpanMention{{
		Thread: tracequery.ThreadRef{PID: 200, Comm: "binder_host"}, Name: "hostSpan", Count: 1, TotalMs: 5, MaxSingleMs: 5,
		SpanWallMs: 20, StartLine: 22, EndLine: 28, OnChainBasis: tracequery.BusinessSpanMentionBasisHostWakeupEdge,
	}}}}
	records := traceQueryTypedBusinessSpanMentionObservations(rank, types.ObservationSourceRef{}, "s", "")
	if len(records) != 1 {
		t.Fatalf("one host-edge mention record expected, got %d", len(records))
	}
	summary := records[0].Summary
	if !strings.Contains(summary, "counted only inside the waits of the analysis target that this thread's own wakeups ended (raw span wall 20.000ms)") ||
		strings.Contains(summary, "spans lying inside") {
		t.Fatalf("model-facing summary must state the counted part with the raw wall beside it: %s", summary)
	}
	// The mention face is a side channel beside the board; one real rank node
	// rides with it, as in production.
	companion := types.ObservationRecord{ID: "trace_query:s#root_cause_rank:1", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard, Subject: "app.main-100", Predicate: "root_cause_primary",
		ClaimKey: "root_cause_primary", Object: "runnable_wait", Value: "1.598", Unit: "ms", RichNotes: []string{"rank=1", "type=runnable_wait"}}
	projection := types.TraceCausalProjectionFromObservationRecords(append([]types.ObservationRecord{companion}, records...))
	if len(projection.BusinessSpanMentions) != 1 || !types.TraceBusinessSpanMentionDependencyPart(projection.BusinessSpanMentions[0]) {
		t.Fatalf("the compiled host-edge row keeps its wall beside the part: %+v", projection.BusinessSpanMentions)
	}
}
