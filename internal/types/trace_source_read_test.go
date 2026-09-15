package types

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func nativeTraceSourceReadResult(path string) ToolResult {
	return ToolResult{ToolName: "trace_query", Success: true, TraceQuerySourceRead: TraceQueryPhysicalSourceReadCandidate(path), Observations: []ObservationRecord{{
		Producer: "trace_query", Origin: AnswerEvidenceOriginRuntimeArtifact, GroundingPolicy: ClaimGroundingHard,
		SourceRef: ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact, ArtifactKind: "trace", Path: path},
	}}}
}

func writeTraceReadTestFile(t *testing.T, root, name string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte("capture bytes\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestB1697SourcePermissionOnlyNativeSuccessfulPhysicalPublication(t *testing.T) {
	root := t.TempDir()
	path := writeTraceReadTestFile(t, root, "capture.go")
	other := writeTraceReadTestFile(t, root, "other.go")
	for _, kind := range []string{"native", "native_run_suffix", "failed", "soft", "other_producer", "source_origin", "blob_only", "summary_only", "manifest", "other_path", "json_replay", "foreign_run", "stale_turn", "memo_hit"} {
		t.Run(kind, func(t *testing.T) {
			m := NewMutableState("native source registration")
			ref := m.PrepareTraceQuerySourceRead(path)
			result := nativeTraceSourceReadResult(path)
			switch kind {
			case "native_run_suffix":
				result.Observations[0].Producer = "trace_query:run2"
			case "failed":
				result.Success = false
			case "soft":
				result.Observations[0].GroundingPolicy = ClaimGroundingSoft
			case "other_producer":
				result.Observations[0].Producer = "aggregate_facts"
			case "source_origin":
				result.Observations[0].Origin = AnswerEvidenceOriginCurrentSource
			case "blob_only":
				result.RawRef = path
				result.Observations[0].SourceRef.Path = ""
				result.Observations[0].SourceRef.PayloadRef = path
				result.Observations[0].SourceRef.RawRef = path
			case "summary_only":
				result.Observations = nil
				result.Summary = "source=" + path + " payload_ref=" + path
			case "manifest":
				result.TraceQuerySourceRead = TraceQuerySourceReadRef{}
			case "other_path":
				result.Observations[0].SourceRef.Path = other
			case "memo_hit":
				result.ReusedFromRunMemo = true
			}
			m.StampTraceQuerySourceRead(ref, &result)
			switch kind {
			case "json_replay":
				serialized, err := json.Marshal(result)
				if err != nil {
					t.Fatal(err)
				}
				result = ToolResult{}
				if err := json.Unmarshal(serialized, &result); err != nil {
					t.Fatal(err)
				}
			case "foreign_run":
				m = NewMutableState("native source registration")
			case "stale_turn":
				m.ResetTurnAArtifacts()
			}
			m.AppendDispatchToolResult(result)
			_, got := m.ResolveTraceQuerySourceRead(path)
			if got != (kind == "native" || kind == "native_run_suffix") {
				t.Fatalf("%s granted=%v; only native physical publication may grant", kind, got)
			}
		})
	}
}

func TestB1697SourcePermissionForkMergeReset(t *testing.T) {
	root := t.TempDir()
	first := writeTraceReadTestFile(t, root, "first.go")
	second := writeTraceReadTestFile(t, root, "second.go")
	parent := NewMutableState("source lifecycle")
	register := func(m *MutableState, path string) ToolResult {
		result := nativeTraceSourceReadResult(path)
		m.StampTraceQuerySourceRead(m.PrepareTraceQuerySourceRead(path), &result)
		m.AppendDispatchToolResult(result)
		return result
	}
	register(parent, first)
	fork := parent.ForkForExploreDispatch()
	register(fork, second)
	if _, ok := parent.ResolveTraceQuerySourceRead(second); ok {
		t.Fatal("fork map leaked before merge")
	}
	fork.ResetDispatchToolResults()
	parent.MergeExploreFork(fork)
	for _, path := range []string{first, second} {
		if _, ok := parent.ResolveTraceQuerySourceRead(path); !ok {
			t.Fatalf("merge lost %s", path)
		}
	}
	parent.ResetTurnAArtifacts()
	parent.MergeExploreFork(fork)
	for _, path := range []string{first, second} {
		if _, ok := parent.ResolveTraceQuerySourceRead(path); ok {
			t.Fatal("stale fork reauthorized previous turn")
		}
	}
}
