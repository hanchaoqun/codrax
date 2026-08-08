package tool

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRuntimeTraceCoveragePrefersExplicitWindowStateAccountZ2(t *testing.T) {
	start, end := 34579.472865, 34579.587805
	state := func(id, selected string, windowMS, running, runnable float64) types.ObservationRecord {
		return types.ObservationRecord{
			ID:              id,
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard,
			ProvenanceLane:  types.ObservationProvenanceArtifactSpan,
			Predicate:       "target_window_states",
			ClaimKey:        "target_window_states:com.baidu.tieba-59566",
			Subject:         "com.baidu.tieba-59566",
			Object:          "state_partition",
			RichNotes: []string{
				types.TraceNoteKeySelectedWindow + "=" + selected,
				types.TraceNoteKeyWindowMS + "=" + fmt.Sprintf("%.3f", windowMS),
				types.TraceNoteKeyRunning + "=" + fmt.Sprintf("%.3f", running),
				types.TraceNoteKeyRunnable + "=" + fmt.Sprintf("%.3f", runnable),
				types.TraceNoteKeySleep + "=84.358",
				types.TraceNoteKeySleepIOWait + "=0.000",
				types.TraceNoteKeyDState + "=0.000",
				types.TraceNoteKeyIOWait + "=0.000",
			},
		}
	}
	input := types.ObservationLedgerInput{
		RequestModel: &types.RequestModel{
			RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{
				RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
				TimeStart:      &start,
				TimeEnd:        &end,
				SourceQuote:    "34579.472865s 到 34579.587805s",
			},
		},
		ToolResults: []types.ToolResult{{
			ToolName: "trace_query",
			Success:  true,
			Observations: []types.ObservationRecord{
				// The wider exploratory account comes first and used to win
				// solely because its window_ms was larger.
				state("trace_query:wide#target_window_states", "34579.470000..34579.590000", 120.000, 31.309, 3.983),
				state("trace_query:exact#target_window_states", "34579.472865..34579.587805", 114.940, 26.946, 3.636),
			},
		}},
	}

	authority := runtimeTraceCoverageAuthority(input)
	if !authority.analysisWindowKnown ||
		authority.analysisWindowStart != start ||
		authority.analysisWindowEnd != end {
		t.Fatalf("validated explicit request window must own coverage authority: %+v", authority)
	}
	if len(authority.targetStates) != 1 {
		t.Fatalf("expected one exact-window state account, got %+v", authority.targetStates)
	}
	got := authority.targetStates[0]
	if got.windowMS != 114.940 || got.running != 26.946 || got.runnable != 3.636 {
		t.Fatalf("wider exploratory account displaced the explicit-window account: %+v", got)
	}
	text := runtimeTraceCoverageAuthorityText(authority, true, false)
	if !strings.Contains(text, "窗114.940ms") || strings.Contains(text, "窗120.000ms") {
		t.Fatalf("coverage text must bind the account to the explicit window:\n%s", text)
	}
}

func TestRuntimeTraceCoverageMismatchedDeclaredStateWindowFailsClosedZ2(t *testing.T) {
	records := []types.ObservationRecord{{
		Producer:  "trace_query",
		Predicate: "target_window_states",
		Subject:   "app-1",
		RichNotes: []string{
			types.TraceNoteKeySelectedWindow + "=1.000000..1.120000",
			types.TraceNoteKeyWindowMS + "=120.000",
			types.TraceNoteKeyRunning + "=20.000",
			types.TraceNoteKeySleep + "=100.000",
		},
	}}
	if got := runtimeTraceCoverageTargetStates(records, 1.0, 1.1, true); len(got) != 0 {
		t.Fatalf("a declared 120ms account must not publish under a 100ms principal window: %+v", got)
	}
	legacy := runtimeTraceCoverageTargetStates(records, 0, 0, false)
	if len(legacy) != 1 || legacy[0].windowMS != 120 {
		t.Fatalf("unknown-window legacy runs must retain the widest-account policy: %+v", legacy)
	}
}
