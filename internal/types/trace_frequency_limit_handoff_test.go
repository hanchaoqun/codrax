package types

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func b1631HandoffPolicyResult(query string) ToolResult {
	offset, slope := 0.125, 1.0
	ref := ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact,
		Path: "/repo/.codrax/blob/run/attached_trace.txt", CaptureIdentityPath: "/captures/original.ftrace",
		ArtifactID: "attached_trace", ArtifactKind: "trace", QueryScopeID: query,
		PayloadRef: "/repo/.codrax/blob/run/result.json", RawRef: "/repo/.codrax/blob/run/result.txt",
		ToolCallID: "trace-call-1", TimeDomain: "monotonic", CanonicalTimeDomain: "trace_seconds",
		ClockCalibrated: true, ClockOffsetSec: &offset, ClockSlope: &slope}
	w := TraceFrequencyLimitAuthority{CPU: 4, MinFrequencyKHz: 500000, MaxFrequencyKHz: 2100000,
		LimitRowCount: 3, WitnessLine: 17, WitnessTs: 10.002, WindowStartTs: 10, WindowEndTs: 10.006,
		Authority: "direct_in_window_policy_limit", SourceRef: &ref, ObservedAt: "2026-09-08T00:00:00Z"}
	return ToolResult{ToolName: "trace_query", Success: true, RawRef: ref.RawRef, Timestamp: time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC),
		TraceEvidenceAuthority: &TraceEvidenceAuthority{View: "window_stats", FrequencyLimitWitnesses: []TraceFrequencyLimitAuthority{w}},
		Observations: []ObservationRecord{{ID: "result:target_cpu_running", Origin: AnswerEvidenceOriginRuntimeArtifact,
			Producer: "trace_query", GroundingPolicy: ClaimGroundingHard, SourceRef: ref, ObservedAt: w.ObservedAt,
			Predicate: "target_cpu_running", Subject: "target-41", Object: "cpu=4", Value: "1.000", Unit: "ms",
			RichNotes: []string{"selected_window=10.000000..10.006000", "target_cpu_running_cpu=4"}}}}
}

func b1631AssertHandoffPolicy(t *testing.T, got ToolResult, want TraceFrequencyLimitAuthority) {
	t.Helper()
	if got.TraceEvidenceAuthority == nil || len(got.TraceEvidenceAuthority.FrequencyLimitWitnesses) != 1 ||
		!reflect.DeepEqual(got.TraceEvidenceAuthority.FrequencyLimitWitnesses[0], want) {
		t.Fatalf("handoff lost or invented policy receipt/value: got=%+v want=%+v", got.TraceEvidenceAuthority, want)
	}
	before, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	w := got.TraceEvidenceAuthority.FrequencyLimitWitnesses[0]
	key := TraceFrequencyLimitSourceKey(w)
	if (key != "") != (want.SourceRef != nil) {
		t.Fatalf("legacy absence or current source identity changed: key=%q witness=%+v", key, w)
	}
	_ = TraceFrequencyLimitSourceRecord(w)
	_ = FormatTraceFrequencyLimitRecordObservation(w, "zh")
	_ = FormatTraceFrequencyLimitRecordObservation(w, "en")
	if len(got.Observations) > 0 {
		if matched := TraceFrequencyLimitMatchesRecord(w, got.Observations[0]); matched != (want.SourceRef != nil) {
			t.Fatalf("read-only source matching invented/lost receipt: matched=%v witness=%+v", matched, w)
		}
	}
	_ = CompileObservationLedger(ObservationLedgerInput{ToolResults: []ToolResult{got}})
	after, err := json.Marshal(got)
	if err != nil || string(before) != string(after) {
		t.Fatal("read-only consumer mutated a shared immutable ToolResult or policy receipt")
	}
}

func TestB1631FrequencyPolicyHandoffJSONAndLegacy(t *testing.T) {
	known := b1631HandoffPolicyResult("result:query_lines=1..20")
	legacy := b1631HandoffPolicyResult("result:query_lines=1..20")
	legacy.TraceEvidenceAuthority.FrequencyLimitWitnesses[0].SourceRef = nil
	legacy.TraceEvidenceAuthority.FrequencyLimitWitnesses[0].ObservedAt = ""
	for _, tc := range []struct {
		name string
		in   ToolResult
	}{{"current_receipt", known}, {"legacy_without_receipt", legacy}} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			var decoded ToolResult
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatal(err)
			}
			b1631AssertHandoffPolicy(t, decoded, tc.in.TraceEvidenceAuthority.FrequencyLimitWitnesses[0])
			var wire map[string]json.RawMessage
			witnessJSON, _ := json.Marshal(decoded.TraceEvidenceAuthority.FrequencyLimitWitnesses[0])
			if err := json.Unmarshal(witnessJSON, &wire); err != nil {
				t.Fatal(err)
			}
			_, hasSource := wire["source_ref"]
			_, hasObservedAt := wire["observed_at"]
			if hasSource != (tc.name == "current_receipt") || hasObservedAt != hasSource {
				t.Fatalf("legacy JSON omission changed or current receipt was dropped: %s", witnessJSON)
			}
		})
	}
}

func TestB1631FrequencyPolicyHandoffMutableLifecycle(t *testing.T) {
	known := b1631HandoffPolicyResult("result:query_lines=1..20")
	legacy := b1631HandoffPolicyResult("result:query_lines=1..20")
	legacy.TraceEvidenceAuthority.FrequencyLimitWitnesses[0].SourceRef = nil
	legacy.TraceEvidenceAuthority.FrequencyLimitWitnesses[0].ObservedAt = ""
	wants := []TraceFrequencyLimitAuthority{known.TraceEvidenceAuthority.FrequencyLimitWitnesses[0], legacy.TraceEvidenceAuthority.FrequencyLimitWitnesses[0]}
	original, _ := json.Marshal([]ToolResult{known, legacy})
	parent := NewMutableState("policy receipt handoff")
	for i, result := range []ToolResult{known, legacy} {
		key := []string{"known", "legacy"}[i]
		parent.StoreToolResultMemo("trace_query", key, result)
		parent.AppendDispatchToolResult(result)
		memo, ok := parent.ToolResultMemo("trace_query", key)
		if !ok {
			t.Fatalf("memo lost %s", key)
		}
		b1631AssertHandoffPolicy(t, memo, wants[i])
	}
	parent.SetTurnAArtifacts(TurnAArtifacts{ToolResults: []ToolResult{known, legacy}})
	assertRows := func(label string, results []ToolResult, expected []TraceFrequencyLimitAuthority) {
		t.Helper()
		if len(results) != len(expected) {
			t.Fatalf("%s result count=%d want=%d", label, len(results), len(expected))
		}
		for i, result := range results {
			b1631AssertHandoffPolicy(t, result, expected[i])
		}
	}
	assertRows("dispatch", parent.DispatchToolResults(), wants)
	assertRows("Turn A", parent.TurnAArtifacts().ToolResults, wants)
	fork := parent.ForkForExploreDispatch()
	if len(fork.DispatchToolResults()) != 0 {
		t.Fatal("new explorer dispatch inherited the parent's local buffer")
	}
	assertRows("fork Turn A", fork.TurnAArtifacts().ToolResults, wants)
	for i, key := range []string{"known", "legacy"} {
		memo, ok := fork.ToolResultMemo("trace_query", key)
		if !ok {
			t.Fatalf("fork memo lost %s", key)
		}
		b1631AssertHandoffPolicy(t, memo, wants[i])
	}
	child := b1631HandoffPolicyResult("result:query_lines=21..40")
	child.TraceEvidenceAuthority.FrequencyLimitWitnesses[0].ObservedAt = "2026-09-08T00:00:01Z"
	child.Observations[0].ObservedAt = "2026-09-08T00:00:01Z"
	childWant := child.TraceEvidenceAuthority.FrequencyLimitWitnesses[0]
	fork.AppendDispatchToolResult(child)
	fork.StoreToolResultMemo("trace_query", "child", child)
	turnA := fork.TurnAArtifacts()
	turnA.ToolResults = append(turnA.ToolResults, child)
	fork.SetTurnAArtifacts(*turnA)
	if _, ok := parent.ToolResultMemo("trace_query", "child"); ok || len(parent.TurnAArtifacts().ToolResults) != 2 {
		t.Fatal("child receipt leaked into parent before merge")
	}
	parent.MergeExploreFork(fork)
	assertRows("merged Turn A", parent.TurnAArtifacts().ToolResults, append(wants, childWant))
	assertRows("parent dispatch stays local", parent.DispatchToolResults(), wants)
	merged, ok := parent.ToolResultMemo("trace_query", "child")
	if !ok {
		t.Fatal("merge lost the child memo")
	}
	b1631AssertHandoffPolicy(t, merged, childWant)
	parent.ResetDispatchToolResults()
	if len(parent.DispatchToolResults()) != 0 {
		t.Fatal("dispatch reset retained a policy receipt")
	}
	assertRows("dispatch reset preserves Turn A", parent.TurnAArtifacts().ToolResults, append(wants, childWant))
	if _, ok := parent.ToolResultMemo("trace_query", "known"); !ok {
		t.Fatal("dispatch reset must not erase the run memo")
	}
	parent.ResetTurnAArtifacts()
	if parent.TurnAArtifacts() != nil {
		t.Fatal("task reset retained the previous policy handoff")
	}
	for _, key := range []string{"known", "legacy", "child"} {
		if _, ok := parent.ToolResultMemo("trace_query", key); ok {
			t.Fatalf("task reset retained memo receipt %s", key)
		}
	}
	fresh := parent.ForkForExploreDispatch()
	if fresh.TurnAArtifacts() != nil || len(fresh.DispatchToolResults()) != 0 {
		t.Fatal("post-reset fork resurrected a prior receipt")
	}
	after, _ := json.Marshal([]ToolResult{known, legacy})
	if string(original) != string(after) {
		t.Fatal("handoff consumers mutated original published immutable values")
	}
}
