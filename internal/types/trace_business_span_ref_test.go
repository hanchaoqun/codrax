package types

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func traceBusinessSpanTestCandidate(path string) TraceBusinessSpanCandidate {
	return TraceBusinessSpanCandidate{
		Path: path, TID: 241, Thread: "document-main", Name: "OpenDocument", Kind: "sync",
		StartLine: 2, EndLine: 40, StartTs: 1.001, EndTs: 1.051,
	}
}

func stampTraceBusinessSpanTestResult(m *MutableState, path string, candidates ...TraceBusinessSpanCandidate) ToolResult {
	result := nativeTraceSourceReadResult(path)
	result.TraceBusinessSpanCandidates = candidates
	m.StampTraceQuerySourceRead(m.PrepareTraceQuerySourceRead(path), &result)
	m.StampTraceBusinessSpanRefs(&result)
	return result
}

func TestTraceBusinessSpanRefBindsOneCompleteNativeInstance(t *testing.T) {
	path := writeTraceReadTestFile(t, t.TempDir(), "capture.trace")
	m := NewMutableState("business instances")
	first := traceBusinessSpanTestCandidate(path)
	first.Path, _, _ = traceSourcePhysicalPath(path)
	second := first
	second.TID, second.Thread, second.Name = 242, "document-worker", "LoadDocumentIndex"
	second.StartLine, second.EndLine, second.StartTs, second.EndTs = 4, 30, 1.003, 1.043
	result := stampTraceBusinessSpanTestResult(m, path, first, second)
	if len(result.TraceBusinessSpanRefs) != 2 {
		t.Fatalf("two complete instances must remain separate: %#v", result.TraceBusinessSpanRefs)
	}
	refs := append([]TraceBusinessSpanRef(nil), result.TraceBusinessSpanRefs...)
	if refs[0].Token() == refs[1].Token() || refs[0].Token() == "" {
		t.Fatal("instances must have distinct opaque tokens")
	}
	if _, ok := m.ResolveTraceBusinessSpanRef(refs[0].Token()); ok {
		t.Fatal("stamping alone published a receipt")
	}
	m.AppendDispatchToolResult(result)
	for i, expected := range []TraceBusinessSpanCandidate{first, second} {
		got, ok := m.ResolveTraceBusinessSpanRef(refs[i].Token())
		if !ok || got.Data() != expected || !m.TraceBusinessSpanRefCurrent(got) || got.SourceRead().Path() != first.Path {
			t.Fatalf("instance %d lost its atomic binding: got=%#v current=%v", i, got.Data(), ok)
		}
		copy := got.Data()
		copy.TID, copy.StartTs = second.TID+99, 999
		if next, ok := m.ResolveTraceBusinessSpanRef(got.Token()); !ok || next.Data() != expected {
			t.Fatal("Data accessor exposed mutable registry storage")
		}
	}
	for _, invented := range []string{"OpenDocument", "LoadDocumentIndex", "241", "business-span:invented", " " + refs[0].Token()} {
		if _, ok := m.ResolveTraceBusinessSpanRef(invented); ok {
			t.Fatalf("non-token %q resolved an instance", invented)
		}
	}
	result.TraceBusinessSpanCandidates[0].TID = 999
	result.TraceBusinessSpanRefs[0] = TraceBusinessSpanRef{}
	if got, ok := m.ResolveTraceBusinessSpanRef(refs[0].Token()); !ok || got.Data() != first {
		t.Fatal("caller mutation changed the published value")
	}
	m.ResetDispatchToolResults()
	if _, ok := m.ResolveTraceBusinessSpanRef(refs[0].Token()); !ok {
		t.Fatal("dispatch reset revoked a run-local receipt")
	}
}

func TestTraceBusinessSpanRefRejectsIncompleteOrUnownedCandidates(t *testing.T) {
	root := t.TempDir()
	path := writeTraceReadTestFile(t, root, "capture.trace")
	other := writeTraceReadTestFile(t, root, "other.trace")
	cases := map[string]func(*TraceBusinessSpanCandidate){
		"async":            func(c *TraceBusinessSpanCandidate) { c.Kind = "async" },
		"untyped_kind":     func(c *TraceBusinessSpanCandidate) { c.Kind = "" },
		"no_tid":           func(c *TraceBusinessSpanCandidate) { c.TID = 0 },
		"negative_tid":     func(c *TraceBusinessSpanCandidate) { c.TID = -1 },
		"over_cap_tid":     func(c *TraceBusinessSpanCandidate) { c.TID = RuntimeTargetMaxPID + 1 },
		"no_thread":        func(c *TraceBusinessSpanCandidate) { c.Thread = " " },
		"no_name":          func(c *TraceBusinessSpanCandidate) { c.Name = "" },
		"no_start_line":    func(c *TraceBusinessSpanCandidate) { c.StartLine = 0 },
		"missing_end_line": func(c *TraceBusinessSpanCandidate) { c.EndLine = 0 },
		"inverted_lines":   func(c *TraceBusinessSpanCandidate) { c.EndLine = c.StartLine },
		"negative_start":   func(c *TraceBusinessSpanCandidate) { c.StartTs = -1 },
		"missing_end":      func(c *TraceBusinessSpanCandidate) { c.EndTs = 0 },
		"empty_window":     func(c *TraceBusinessSpanCandidate) { c.EndTs = c.StartTs },
		"inverted_window":  func(c *TraceBusinessSpanCandidate) { c.EndTs = c.StartTs - 0.001 },
		"nan_start":        func(c *TraceBusinessSpanCandidate) { c.StartTs = math.NaN() },
		"nan_end":          func(c *TraceBusinessSpanCandidate) { c.EndTs = math.NaN() },
		"infinite_start":   func(c *TraceBusinessSpanCandidate) { c.StartTs = math.Inf(1) },
		"infinite_end":     func(c *TraceBusinessSpanCandidate) { c.EndTs = math.Inf(1) },
		"other_source":     func(c *TraceBusinessSpanCandidate) { c.Path = other },
		"relative_source":  func(c *TraceBusinessSpanCandidate) { c.Path = filepath.Base(path) },
		"missing_source":   func(c *TraceBusinessSpanCandidate) { c.Path = "" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			m := NewMutableState(name)
			candidate := traceBusinessSpanTestCandidate(path)
			mutate(&candidate)
			result := stampTraceBusinessSpanTestResult(m, path, candidate)
			m.AppendDispatchToolResult(result)
			if len(result.TraceBusinessSpanRefs) != 0 || len(m.traceBusinessSpanRefs) != 0 {
				t.Fatalf("invalid %s instance received navigation authority", name)
			}
		})
	}
}

func TestTraceBusinessSpanRefRequiresOriginalNativePublication(t *testing.T) {
	path := writeTraceReadTestFile(t, t.TempDir(), "capture.trace")
	for _, kind := range []string{"unstamped_source", "failed", "memo", "foreign_run", "old_epoch", "serialized_replay", "prose_only", "changed_candidate", "changed_receipt"} {
		t.Run(kind, func(t *testing.T) {
			m := NewMutableState(kind)
			candidate := traceBusinessSpanTestCandidate(path)
			result := stampTraceBusinessSpanTestResult(m, path, candidate)
			token := result.TraceBusinessSpanRefs[0].Token()
			switch kind {
			case "unstamped_source":
				result.TraceQuerySourceRead = TraceQueryPhysicalSourceReadCandidate(path)
				m.StampTraceBusinessSpanRefs(&result)
			case "failed":
				result.Success = false
			case "memo":
				result.ReusedFromRunMemo = true
				m.StampTraceBusinessSpanRefs(&result)
				if len(result.TraceBusinessSpanRefs) != 0 {
					t.Fatal("memo hit minted or retained references")
				}
			case "foreign_run":
				m = NewMutableState(kind)
			case "old_epoch":
				m.ResetTurnAArtifacts()
			case "serialized_replay":
				payload, err := json.Marshal(result)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(payload), token) || strings.Contains(string(payload), "OpenDocument") {
					t.Fatal("private candidate or receipt serialized")
				}
				result = ToolResult{}
				if err := json.Unmarshal(payload, &result); err != nil {
					t.Fatal(err)
				}
			case "prose_only":
				result.TraceBusinessSpanCandidates = nil
				result.Summary = "OpenDocument sync document-main tid=241 [1.001,1.051] lines 2-40"
				m.StampTraceBusinessSpanRefs(&result)
			case "changed_candidate":
				result.TraceBusinessSpanCandidates[0].StartTs += 0.001
			case "changed_receipt":
				result.TraceBusinessSpanRefs[0].data.TID++
			}
			m.AppendDispatchToolResult(result)
			if _, ok := m.ResolveTraceBusinessSpanRef(token); ok || len(m.traceBusinessSpanRefs) != 0 {
				t.Fatalf("%s restored a business instance", kind)
			}
		})
	}
}

func TestTraceBusinessSpanRefDoesNotBorrowNewSourceReceipt(t *testing.T) {
	root := t.TempDir()
	path := writeTraceReadTestFile(t, root, "capture.trace")
	m := NewMutableState("file replacement")
	first := stampTraceBusinessSpanTestResult(m, path, traceBusinessSpanTestCandidate(path))
	m.AppendDispatchToolResult(first)
	old := first.TraceBusinessSpanRefs[0]
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	replacement := writeTraceReadTestFile(t, root, "replacement.trace")
	if err := os.Chtimes(replacement, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	second := stampTraceBusinessSpanTestResult(m, path, traceBusinessSpanTestCandidate(path))
	if len(second.TraceBusinessSpanRefs) != 1 {
		t.Fatal("new native read failed to mint its own receipt")
	}
	m.AppendDispatchToolResult(second)
	if _, ok := m.ResolveTraceQuerySourceRead(path); !ok {
		t.Fatal("test requires a fresh registered receipt at the same path")
	}
	if _, ok := m.ResolveTraceBusinessSpanRef(old.Token()); ok || m.TraceBusinessSpanRefCurrent(old) {
		t.Fatal("same-byte/size/mtime replacement revived the old instance using the fresh source receipt")
	}
	newRef := second.TraceBusinessSpanRefs[0]
	if newRef.Token() == old.Token() || !m.TraceBusinessSpanRefCurrent(newRef) {
		t.Fatal("new file generation reused the old token or lacks current authority")
	}
	m.ResetTurnAArtifacts()
	third := stampTraceBusinessSpanTestResult(m, path, traceBusinessSpanTestCandidate(path))
	m.AppendDispatchToolResult(third)
	if third.TraceBusinessSpanRefs[0].Token() == newRef.Token() || m.TraceBusinessSpanRefCurrent(newRef) {
		t.Fatal("turn reset reused or revived a receipt")
	}
}

func TestTraceBusinessSpanRefForkPublicationLifecycle(t *testing.T) {
	for _, dataOnly := range []bool{false, true} {
		name := "normal_merge"
		if dataOnly {
			name = "published_tools_only"
		}
		t.Run(name, func(t *testing.T) {
			path := writeTraceReadTestFile(t, t.TempDir(), "capture.trace")
			parent := NewMutableState(name)
			first := stampTraceBusinessSpanTestResult(parent, path, traceBusinessSpanTestCandidate(path))
			parent.AppendDispatchToolResult(first)
			inherited := first.TraceBusinessSpanRefs[0]
			fork := parent.ForkForExploreDispatch()
			if !fork.TraceBusinessSpanRefCurrent(inherited) {
				t.Fatal("fork lost inherited receipt")
			}
			candidate := traceBusinessSpanTestCandidate(path)
			candidate.TID, candidate.Thread, candidate.Name = 242, "worker", "LoadDocumentIndex"
			second := stampTraceBusinessSpanTestResult(fork, path, candidate)
			fork.AppendDispatchToolResult(second)
			childRef := second.TraceBusinessSpanRefs[0]
			if parent.TraceBusinessSpanRefCurrent(childRef) {
				t.Fatal("child registry leaked before publication handoff")
			}
			merge := func() {
				if dataOnly {
					parent.MergeExploreForkPublishedTools(fork)
				} else {
					parent.MergeExploreFork(fork)
				}
			}
			merge()
			if !parent.TraceBusinessSpanRefCurrent(childRef) || !parent.TraceBusinessSpanRefCurrent(inherited) {
				t.Fatal("merge lost one of the independent instance bindings")
			}
			parent.ResetTurnAArtifacts()
			merge()
			if parent.TraceBusinessSpanRefCurrent(childRef) || parent.TraceBusinessSpanRefCurrent(inherited) {
				t.Fatal("late fork reopened a prior turn")
			}
			if _, ok := parent.ResolveTraceBusinessSpanRef(childRef.Token()); ok {
				t.Fatal("late fork token resolved after reset")
			}
		})
	}
}

func TestTraceBusinessSpanRefCanonicalSourceAndForeignRef(t *testing.T) {
	root := t.TempDir()
	path := writeTraceReadTestFile(t, root, "capture.trace")
	alias := filepath.Join(root, "alias.trace")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	m := NewMutableState("physical source")
	result := stampTraceBusinessSpanTestResult(m, path, traceBusinessSpanTestCandidate(alias))
	m.AppendDispatchToolResult(result)
	ref := result.TraceBusinessSpanRefs[0]
	physical, _, _ := traceSourcePhysicalPath(path)
	if ref.Data().Path != physical || !m.TraceBusinessSpanRefCurrent(ref) {
		t.Fatal("same physical capture alias was not canonicalized")
	}
	forged := ref
	forged.data.StartTs += 0.001
	if m.TraceBusinessSpanRefCurrent(forged) {
		t.Fatal("a token with changed window validated")
	}
	if NewMutableState("foreign run").TraceBusinessSpanRefCurrent(ref) {
		t.Fatal("opaque reference crossed run identity")
	}
	if err := os.WriteFile(path, []byte("changed capture\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if m.TraceBusinessSpanRefCurrent(ref) {
		t.Fatal("in-place changed capture retained authority")
	}
}

func TestTraceBusinessSpanRefCapacityIsSharedBeforeForkPublication(t *testing.T) {
	path := writeTraceReadTestFile(t, t.TempDir(), "capture.trace")
	parent := NewMutableState("bounded fork publication")
	var candidates []TraceBusinessSpanCandidate
	for i := 0; i < TraceBusinessSpanFactLimit+2; i++ {
		candidate := traceBusinessSpanTestCandidate(path)
		candidate.StartLine += i
		candidate.EndLine += i
		candidates = append(candidates, candidate)
	}
	const forks = 4
	states := make([]*MutableState, forks)
	for i := range states {
		states[i] = parent.ForkForExploreDispatch()
	}
	var wg sync.WaitGroup
	for _, fork := range states {
		wg.Add(1)
		go func(fork *MutableState) {
			defer wg.Done()
			for j := 0; j < TraceBusinessSpanRefRunLimit/TraceBusinessSpanFactLimit/forks+2; j++ {
				result := stampTraceBusinessSpanTestResult(fork, path, candidates...)
				if len(result.TraceBusinessSpanRefs) > TraceBusinessSpanFactLimit || len(result.TraceBusinessSpanCandidates) > TraceBusinessSpanFactLimit {
					t.Error("per-result candidate/reference cap exceeded")
				}
				fork.AppendDispatchToolResult(result)
			}
		}(fork)
	}
	wg.Wait()
	all := make(map[string]TraceBusinessSpanRef)
	for i, fork := range states {
		for token, ref := range fork.traceBusinessSpanRefs {
			if _, duplicate := all[token]; duplicate {
				t.Fatal("forks issued the same opaque token")
			}
			all[token] = ref
		}
		if i%2 == 0 {
			parent.MergeExploreFork(fork)
		} else {
			parent.MergeExploreForkPublishedTools(fork)
		}
	}
	if len(all) != TraceBusinessSpanRefRunLimit || len(parent.traceBusinessSpanRefs) != len(all) {
		t.Fatalf("shared budget or merge lost capacity: issued=%d retained=%d limit=%d", len(all), len(parent.traceBusinessSpanRefs), TraceBusinessSpanRefRunLimit)
	}
	for token := range all {
		if _, ok := parent.ResolveTraceBusinessSpanRef(token); !ok {
			t.Fatalf("issued token was evicted by merge: %s", token)
		}
	}
	exhausted := stampTraceBusinessSpanTestResult(parent, path, candidates...)
	if len(exhausted.TraceBusinessSpanRefs) != 0 {
		t.Fatal("capacity exhaustion exposed unregistered tokens")
	}
	parent.ResetTurnAArtifacts()
	fresh := stampTraceBusinessSpanTestResult(parent, path, candidates...)
	if len(fresh.TraceBusinessSpanRefs) != TraceBusinessSpanFactLimit {
		t.Fatal("new epoch failed to restore its issuance budget")
	}
}

func TestTraceBusinessSpanToolResultSnapshotsOwnPrivateSlices(t *testing.T) {
	path := writeTraceReadTestFile(t, t.TempDir(), "capture.trace")
	m := NewMutableState("private slice ownership")
	result := stampTraceBusinessSpanTestResult(m, path, traceBusinessSpanTestCandidate(path))
	token := result.TraceBusinessSpanRefs[0].Token()
	m.AppendDispatchToolResult(result)
	m.StoreToolResultMemo("trace_query", "business", result)
	m.SetTurnAArtifacts(TurnAArtifacts{ToolResults: []ToolResult{result}})
	mutate := func(r *ToolResult) {
		r.TraceBusinessSpanCandidates[0].TID = 999
		r.TraceBusinessSpanRefs[0] = TraceBusinessSpanRef{}
	}
	mutate(&result)
	check := func(name string, r ToolResult) {
		if r.TraceBusinessSpanCandidates[0].TID != 241 || r.TraceBusinessSpanRefs[0].Token() != token {
			t.Fatalf("%s shares private slice storage", name)
		}
		mutate(&r)
	}
	for i := 0; i < 2; i++ {
		check("dispatch snapshot", m.DispatchToolResults()[0])
		memo, _ := m.ToolResultMemo("trace_query", "business")
		check("memo snapshot", memo)
		check("TurnA snapshot", m.TurnAArtifacts().ToolResults[0])
		turnA, dispatch, _, _, _, _, _, _ := m.GroundingContextSnapshot()
		check("grounding TurnA snapshot", turnA.ToolResults[0])
		check("grounding dispatch snapshot", dispatch[0])
		fork := m.ForkForExploreDispatch()
		check("fork TurnA", fork.turnAArtifacts.ToolResults[0])
		forkMemo, _ := fork.ToolResultMemo("trace_query", "business")
		check("fork memo", forkMemo)
	}
	if !m.TraceBusinessSpanRefCurrent(m.traceBusinessSpanRefs[token]) {
		t.Fatal("snapshot mutation changed registered value")
	}
	before := nativeTraceSourceReadResult(path)
	after := m.DispatchToolResults()[0]
	if TurnAToolResultBytes(after) <= TurnAToolResultBytes(before) {
		t.Fatal("private business candidates/refs escape handoff byte accounting")
	}
}

func TestTraceBusinessSpanQuerySourceMatchesSinglePhysicalWithoutRows(t *testing.T) {
	root := t.TempDir()
	path := writeTraceReadTestFile(t, root, "capture.trace")
	other := writeTraceReadTestFile(t, root, "sibling.trace")
	m := NewMutableState("follow-up physical provenance")
	publication := stampTraceBusinessSpanTestResult(m, path, traceBusinessSpanTestCandidate(path))
	m.AppendDispatchToolResult(publication)
	ref := publication.TraceBusinessSpanRefs[0]
	for _, name := range []string{"same_physical_zero_rows", "different_pre_read", "different_result_source", "promoted_composite", "foreign_pre_read", "empty_ref"} {
		t.Run(name, func(t *testing.T) {
			before := m.PrepareTraceQuerySourceRead(path)
			result := nativeTraceSourceReadResult(path)
			result.Observations = nil
			candidateRef := ref
			switch name {
			case "different_pre_read":
				before = m.PrepareTraceQuerySourceRead(other)
			case "different_result_source":
				result.TraceQuerySourceRead = TraceQueryPhysicalSourceReadCandidate(other)
			case "promoted_composite":
				result.TraceQuerySourceRead = TraceQuerySourceReadRef{}
			case "foreign_pre_read":
				before = NewMutableState("foreign run").PrepareTraceQuerySourceRead(path)
			case "empty_ref":
				candidateRef = TraceBusinessSpanRef{}
			}
			if got := TraceBusinessSpanQuerySourceMatches(candidateRef, before, result); got != (name == "same_physical_zero_rows") {
				t.Fatalf("%s source match=%v", name, got)
			}
		})
	}
	before := m.PrepareTraceQuerySourceRead(path)
	if err := os.WriteFile(path, []byte("modified trace\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if TraceBusinessSpanQuerySourceMatches(ref, before, nativeTraceSourceReadResult(path)) {
		t.Fatal("follow-up accepted a file changed after its pre-read")
	}
}
