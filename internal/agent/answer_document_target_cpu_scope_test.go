package agent

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func b1630CPUObservation(id, capture, window, value string) types.ObservationRecord {
	notes := []string{"target_cpu_running_roster_status=complete", "target_cpu_running_assignment_status=complete"}
	if window != "" {
		notes = append(notes, "selected_window="+window)
	}
	return types.ObservationRecord{
		ID: id, Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		Role: types.AnswerAggregateRoleSupportingCoverage, GroundingPolicy: types.ClaimGroundingHard,
		SourceRef: types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: capture,
			PayloadRef: id + ".json", RawRef: id + ".json"},
		// This is a CPU occurrence envelope, deliberately not the query ruler.
		Span:    types.ObservationSpan{LineStart: 5, LineEnd: 15, StartTs: 2.001, EndTs: 2.009},
		Subject: "render-42", Predicate: "target_cpu_running", Object: "cpu=12", Value: value, Unit: "ms",
		RichNotes: notes,
	}
}

func b1630CPUContext(rows []types.ObservationRecord, requested bool) *types.AgentContext {
	mu := types.NewMutableState("inspect target CPU placement")
	mu.AppendDispatchToolResult(types.ToolResult{ToolName: "trace_query", Success: true, Observations: rows})
	ctx := &types.AgentContext{Mutable: mu}
	if requested {
		start, end := 2.0, 2.1
		ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentRootCause, Scenario: types.ScenarioRootCause,
			RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{
				RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
				TimeStart:      &start, TimeEnd: &end, SourceQuote: "a validated explicit window",
			},
		}}
	}
	return ctx
}

func b1630CPUSection(t *testing.T, prompt string) string {
	t.Helper()
	const header = "## Final Target CPU Identity Boundary (Typed Facts; Model-Owned Conclusion)"
	_, section, ok := strings.Cut(prompt, header)
	if !ok {
		t.Fatalf("actual finalizer prompt omitted CPU handoff: %s", prompt)
	}
	if i := strings.Index(section, "\n## "); i >= 0 {
		section = section[:i]
	}
	return section
}

func TestB1630CPUFinalizerContextKeepsRequestedAndExplorationRulers(t *testing.T) {
	rows := []types.ObservationRecord{
		b1630CPUObservation("principal", "captures/main.trace", "2.000000..2.100000", "96.081"),
		b1630CPUObservation("exploration", "captures/main.trace", "2.001000..2.099000", "94.933"),
	}
	ctx := b1630CPUContext(rows, true)
	before, err := json.Marshal(ctx.Mutable.DispatchToolResults())
	if err != nil {
		t.Fatal(err)
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromAgentContext(ctx, 1))
	if len(ledger.Records) != 2 {
		t.Fatalf("fixture must exercise two real accepted query records: %+v", ledger.Records)
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	got := b1630CPUSection(t, prompt)
	for _, want := range []string{
		"running=`96.081ms`", "running=`94.933ms`", "artifact=`captures/main.trace`",
		"query_window=`2.000000..2.100000`", "query_window=`2.001000..2.099000`",
		"matches the requested window", "supplementary query window 2.001000–2.099000",
		"does not substitute for an account of the requested window", "The final explanation remains model-authored",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("finalizer lost CPU scope/value %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "query_window=`2.001000..2.009000`") {
		t.Fatalf("occurrence span was promoted to a query ruler:\n%s", got)
	}
	after, err := json.Marshal(ctx.Mutable.DispatchToolResults())
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("CPU context projection mutated original tool observations")
	}
}

func TestB1630CPUScopeDedupRequiresCompleteIdentity(t *testing.T) {
	for _, tc := range []struct {
		name string
		rows []types.ObservationRecord
		want int
		text []string
	}{
		{"different_windows_equal_values", []types.ObservationRecord{
			b1630CPUObservation("a", "one/capture.trace", "0.000000..0.020000", "10.000"),
			b1630CPUObservation("b", "one/capture.trace", "0.000000..0.021000", "10.000"),
		}, 2, []string{"query_window=`0.000000..0.020000`", "query_window=`0.000000..0.021000`"}},
		{"same_basename_different_captures", []types.ObservationRecord{
			b1630CPUObservation("a", "one/capture.trace", "0.000000..0.020000", "10.000"),
			b1630CPUObservation("b", "two/capture.trace", "0.000000..0.020000", "10.000"),
		}, 2, []string{"artifact=`one/capture.trace`", "artifact=`two/capture.trace`"}},
		{"missing_query_not_occurrence_span", []types.ObservationRecord{
			b1630CPUObservation("a", "one/capture.trace", "", "10.000"),
			b1630CPUObservation("b", "one/capture.trace", "", "10.000"),
		}, 2, []string{"query_window=`query window not stated`", "actual query window is unknown"}},
		{"missing_capture_not_shared", []types.ObservationRecord{
			b1630CPUObservation("a", "", "0.000000..0.020000", "10.000"),
			b1630CPUObservation("b", "", "0.000000..0.020000", "10.000"),
		}, 2, []string{"artifact=`artifact not stated`", "query_window=`0.000000..0.020000`"}},
		{"complete_same_scope_duplicate", []types.ObservationRecord{
			b1630CPUObservation("a", "one/capture.trace", "0.000000..0.020000", "10.000"),
			b1630CPUObservation("b", "one/capture.trace", "0.000000..0.020000", "10.000"),
		}, 1, []string{"running=`10.000ms`", "query_window=`0.000000..0.020000`"}},
		{"same_scope_different_measurements", []types.ObservationRecord{
			b1630CPUObservation("a", "one/capture.trace", "0.000000..0.020000", "10.000"),
			b1630CPUObservation("b", "one/capture.trace", "0.000000..0.020000", "9.000"),
		}, 2, []string{"running=`10.000ms`", "running=`9.000ms`"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, reverse := range []bool{false, true} {
				rows := append([]types.ObservationRecord(nil), tc.rows...)
				if reverse {
					rows[0], rows[1] = rows[1], rows[0]
				}
				ctx := b1630CPUContext(rows, false)
				got := renderAnswerDocTargetCPUIdentityBoundary(ctx)
				if n := strings.Count(got, "- target=`"); n != tc.want {
					t.Errorf("reverse=%t emitted %d CPU rows, want %d:\n%s", reverse, n, tc.want, got)
				}
				for _, want := range tc.text {
					if !strings.Contains(got, want) {
						t.Errorf("reverse=%t missing %q:\n%s", reverse, want, got)
					}
				}
				if strings.Contains(got, "query_window=`0.000000..0.000000`") || strings.Contains(got, "query_window=`2.001000..2.009000`") {
					t.Fatalf("missing query was fabricated:\n%s", got)
				}
			}
		})
	}
}

func TestB1630CPUCaptureIdentityPrefersOriginalCaptureAndKeepsTypedEligibility(t *testing.T) {
	rows := []types.ObservationRecord{}
	for i, capture := range []string{"capture-A/trace.txt", "capture-B/trace.txt"} {
		row := b1630CPUObservation(fmt.Sprintf("row%d", i), "materialized/shared.txt", "2.000000..2.100000", "8.000")
		row.SourceRef.CaptureIdentityPath = capture
		rows = append(rows, row)
	}
	ignored := rows[0]
	ignored.ID, ignored.Producer, ignored.Value = "model-row", "emit_evidence", "999.000"
	rows = append(rows, ignored)
	got := renderAnswerDocTargetCPUIdentityBoundary(b1630CPUContext(rows, false))
	for _, want := range []string{"artifact=`capture-A/trace.txt`", "artifact=`capture-B/trace.txt`", "parenthesized numeric field is PID/TGID identity, not a CPU number"} {
		if !strings.Contains(got, want) {
			t.Errorf("capture identity/old boundary missing %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "- target=`") != 2 || strings.Contains(got, "999.000") || strings.Contains(got, "artifact=`materialized/shared.txt`") {
		t.Fatalf("scope display borrowed capture or expanded original producer eligibility:\n%s", got)
	}
}

func TestB1630CPULegacyTypedArtifactIdentityAndRosterValuesRemainDistinct(t *testing.T) {
	for _, lane := range []string{"support_ref", "artifact_id"} {
		t.Run(lane, func(t *testing.T) {
			var rows []types.ObservationRecord
			for i, artifact := range []string{"one/capture.trace", "two/capture.trace"} {
				row := b1630CPUObservation(fmt.Sprintf("legacy%d", i), "", "2.000000..2.100000", "8.000")
				if lane == "support_ref" {
					row.SupportRefs = []string{artifact + ":5-15"}
				} else {
					row.SourceRef.ArtifactID = artifact
				}
				rows = append(rows, row)
			}
			got := renderAnswerDocTargetCPUIdentityBoundary(b1630CPUContext(rows, false))
			for _, artifact := range []string{"one/capture.trace", "two/capture.trace"} {
				if !strings.Contains(got, "artifact=`"+artifact+"`") {
					t.Errorf("shared %s identity lost %q:\n%s", lane, artifact, got)
				}
			}
			if strings.Count(got, "- target=`") != 2 {
				t.Fatalf("distinct typed capture identities collapsed:\n%s", got)
			}
		})
	}
	complete := b1630CPUObservation("complete", "one/capture.trace", "2.000000..2.100000", "8.000")
	partial := b1630CPUObservation("partial", "one/capture.trace", "2.000000..2.100000", "8.000")
	partial.RichNotes[0] = "target_cpu_running_roster_status=capacity_truncated"
	partial.RichNotes[1] = "target_cpu_running_assignment_status=unknown"
	got := renderAnswerDocTargetCPUIdentityBoundary(b1630CPUContext([]types.ObservationRecord{complete, partial}, false))
	for _, want := range []string{
		"roster_status=`complete`; assignment_status=`complete`",
		"roster_status=`capacity_truncated`; assignment_status=`unknown`",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("scope-aware dedup replaced a published roster/assignment value %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "- target=`") != 2 {
		t.Fatalf("different roster facts cannot merge just because scope/value match:\n%s", got)
	}
}

func TestB1630CPUMatchingTimeRangeDoesNotBindRequestedCaptureOrTarget(t *testing.T) {
	rows := []types.ObservationRecord{
		b1630CPUObservation("a", "one/capture.trace", "2.000000..2.100000", "10.000"),
		b1630CPUObservation("b", "two/capture.trace", "2.000000..2.100000", "10.000"),
	}
	rows[1].Subject = "other-99"
	got := b1630CPUSection(t, (&answerDocumentEvaluator{}).BuildInitialInstruction(b1630CPUContext(rows, true), nil))
	for _, want := range []string{
		"Window comparisons below describe timestamp ranges only",
		"they do not establish that a row belongs to the requested capture or target",
		"target=`render-42`", "target=`other-99`",
		"artifact=`one/capture.trace`", "artifact=`two/capture.trace`",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("same-time foreign capture/target acquired ambiguous request ownership; missing %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "- target=`") != 2 || strings.Count(got, "matches the requested window") != 2 {
		t.Fatalf("both independently scoped timestamp matches must remain visible without electing a requested capture:\n%s", got)
	}
}
