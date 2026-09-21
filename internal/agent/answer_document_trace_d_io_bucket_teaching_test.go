package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the actual finalizer message, not a stand-alone string formatter.
// The producer's exclusive buckets stay unchanged: D-opened marked IO is a
// subtype of uninterruptible waiting, whereas S with an IO marker is not D.
func TestTraceDIOBucketTeachingActualFinalInstruction(t *testing.T) {
	for _, tc := range []struct {
		name   string
		states []string
		want   string
	}{
		{"d_opened_io", []string{"io_wait", "io_wait"}, "occurrence_count=2; d_state_occurrences=0; io_wait_occurrences=2; sleep_iowait_occurrences=0; other_wait_occurrences=0; wall_clock_sum=1.000ms"},
		{"mixed_d_and_io", []string{"d_sleep", "io_wait"}, "occurrence_count=2; d_state_occurrences=1; io_wait_occurrences=1; sleep_iowait_occurrences=0; other_wait_occurrences=0; wall_clock_sum=1.000ms"},
		{"s_with_io_marker", []string{"s_sleep", "s_sleep"}, "occurrence_count=2; d_state_occurrences=0; io_wait_occurrences=0; sleep_iowait_occurrences=2; other_wait_occurrences=0; wall_clock_sum=1.000ms"},
		{"d_without_io_marker", []string{"d_sleep", "d_sleep"}, "occurrence_count=2; d_state_occurrences=2; io_wait_occurrences=0; sleep_iowait_occurrences=0; other_wait_occurrences=0; wall_clock_sum=1.000ms"},
	} {
		for _, lang := range []string{"zh", "en"} {
			t.Run(tc.name+"/"+lang, func(t *testing.T) {
				ctx := traceDIOBucketTeachingContext(t, tc.states, lang)
				before := traceDIOBucketTeachingSnapshot(t, ctx)
				instruction := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
				if !strings.Contains(instruction, tc.want) {
					t.Fatalf("fixture lost its original typed counts/sum: want %q", tc.want)
				}
				if strings.Count(instruction, "principal_occurrence=`") != len(tc.states) {
					t.Fatal("final recap lost or duplicated a wait occurrence")
				}
				for i, state := range tc.states {
					want := fmt.Sprintf("principal_occurrence=`#%d state=%s", i+1, state)
					if !strings.Contains(instruction, want) {
						t.Fatalf("original state bucket changed: %s", want)
					}
				}
				if strings.Contains(instruction, "Do not rename an `io_wait` row to D-state") {
					t.Fatal("final instruction still contradicts the existing D+IO uninterruptible-wait fold")
				}
				for _, want := range []string{
					types.FormatTargetStateAccountCaliber(lang),
					types.TraceSchedulerWaitPartitionTeaching,
					"`d_state`/`d_state_occurrences` describe the non-IO D bucket",
					"`io_wait`/`io_wait_occurrences` describe D-opened intervals with a paired kernel IO-wait marker",
					"both retain D-state provenance",
					"A zero non-IO D bucket alone does not prove absence of D-state waiting",
					"Preserve published buckets and count each interval only once",
					"`sleep_io_wait`/`sleep_iowait_occurrences` remain interruptible S-state waiting, not D-state",
					"device/request IO latency or independently completion-closed IO blocking",
				} {
					if !strings.Contains(instruction, want) {
						t.Errorf("final instruction is missing accounting/physical-state boundary %q", want)
					}
				}
				assertTraceWaitAbsenceBoundary(t, instruction, lang)
				if after := traceDIOBucketTeachingSnapshot(t, ctx); string(before) != string(after) {
					t.Fatal("teaching changed request, observations, authority, projection or model-owned answer")
				}
			})
		}
	}
}

func traceDIOBucketTeachingContext(t *testing.T, states []string, lang string) *types.AgentContext {
	t.Helper()
	const subject = "storage-client-471"
	start, end := 10.0, 10.1
	ref := types.ObservationSourceRef{
		Kind: types.ObservationSourceRuntimeArtifact, Path: "/tmp/wait-buckets.systrace",
		ArtifactID: "wait-buckets", PayloadRef: "wait-buckets-result.json",
	}
	count := len(states)
	observations := []types.ObservationRecord{{
		ID: "trace_query:bucket#target_window_wait_occurrences", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref,
		Span: types.ObservationSpan{StartTs: start, EndTs: end}, Subject: subject,
		Predicate: "target_window_wait_occurrences", Object: "complete", Value: fmt.Sprint(count), ResultCount: &count,
	}}
	for i, state := range states {
		rowStart := start + float64(i+1)*0.002
		ioWait := "1"
		if state == "d_sleep" {
			ioWait = "0"
		}
		observations = append(observations, types.ObservationRecord{
			ID:     fmt.Sprintf("trace_query:bucket#target_window_wait_occurrence:%d", i+1),
			Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref,
			Span: types.ObservationSpan{StartTs: rowStart, EndTs: rowStart + 0.0005}, Subject: subject,
			Predicate: "target_window_wait_occurrence", Object: fmt.Sprintf("state=%s;iowait=%s;caller=recorded_wait", state, ioWait),
			Value: "0.500", Unit: "ms",
		})
	}
	ctx := tracePrincipalValueAuthorityTestContext(subject, 471, observations)
	rm := &ctx.AnalysisIR.RequestModel
	rm.Intent, rm.Language = types.IntentExplain, lang
	rm.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{
		Scope:        types.RuntimeQuestionScopeBoundedFactSet,
		FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetWaitOccurrences},
	}
	rm.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "10.0..10.1",
	}
	ctx.Mutable.SetRequestModel(*rm)
	ctx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "model", Kind: types.BlockSummary, Text: "Model-owned conclusion stays unchanged."}},
	})
	dir := t.TempDir()
	return ctxbuilder.BuildAgentContext(&types.BusContext{
		RepoRoot: dir, WorkDir: dir, Language: lang, Mutable: ctx.Mutable, AnalysisIR: ctx.AnalysisIR,
	}, types.AgentFinalizer, types.StageFinalize)
}

func traceDIOBucketTeachingSnapshot(t *testing.T, ctx *types.AgentContext) []byte {
	t.Helper()
	ledger := answerDocObservationLedger(ctx)
	b, err := json.Marshal([]any{
		ctx.AnalysisIR.RequestModel, ctx.Mutable.TurnAArtifacts(), ledger,
		types.BuildTraceTargetWaitSummaryAuthorities(ledger, &ctx.AnalysisIR.RequestModel),
		types.CompileTraceCausalProjectionSet(ledger), ctx.Mutable.AnswerDocumentV2(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}
