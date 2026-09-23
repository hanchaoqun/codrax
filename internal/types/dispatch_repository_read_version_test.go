package types

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
)

const dispatchReadVersionSHA = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func dispatchReadVersionResult(ref string, start, end, total int) ToolResult {
	result := ToolResult{ToolName: "read_file", Success: true, RawRef: ref,
		ReadCoverage: &ToolReadCoverage{Path: "tests/native.py", RawRef: ref, LineStart: start, LineEnd: end, TotalLines: total}}
	if total == 0 {
		result.EnumerationAuthority = &ToolEnumerationAuthority{Status: "complete", Boundaries: []ToolEnumerationBoundary{{Scope: "tests/native.py", Dimension: "lines", TotalKnown: true}}}
	}
	return result
}

func dispatchReadVersionAdd(t *testing.T, m *MutableState, ref, sha string, start, end, total int) {
	t.Helper()
	generation := m.BeginDispatchRepositoryFileRead()
	m.RecordDispatchRepositoryFileRead("/repo", "tests/native.py", ref)
	if !m.RecordDispatchRepositoryFileReadVersion(generation, "/repo", "tests/native.py", ref, sha, start, end, total) {
		t.Fatal("valid producer page rejected")
	}
	m.AppendDispatchToolResult(dispatchReadVersionResult(ref, start, end, total))
}

func TestDispatchRepositoryFileReadVersionRequiresPublishedSuccess(t *testing.T) {
	for _, name := range []string{"not_appended", "failed", "other_tool", "runtime", "no_coverage", "wrong_path", "wrong_ref", "wrong_coverage_ref", "wrong_start", "wrong_end", "wrong_total", "valid"} {
		t.Run(name, func(t *testing.T) {
			m := NewMutableState("read")
			generation := m.BeginDispatchRepositoryFileRead()
			m.RecordDispatchRepositoryFileRead("/repo", "tests/native.py", "page")
			if !m.RecordDispatchRepositoryFileReadVersion(generation, "/repo", "tests/native.py", "page", dispatchReadVersionSHA, 1, 3, 3) {
				t.Fatal("pending observation rejected")
			}
			result := dispatchReadVersionResult("page", 1, 3, 3)
			switch name {
			case "failed":
				result.Success = false
			case "other_tool":
				result.ToolName = "grep"
			case "runtime":
				result.RuntimeArtifactRead = &ToolRuntimeArtifactRead{Kind: "trace"}
			case "no_coverage":
				result.ReadCoverage = nil
			case "wrong_path":
				result.ReadCoverage.Path = "other.py"
			case "wrong_ref":
				result.RawRef = "other"
			case "wrong_coverage_ref":
				result.ReadCoverage.RawRef = "other"
			case "wrong_start":
				result.ReadCoverage.LineStart = 2
			case "wrong_end":
				result.ReadCoverage.LineEnd = 2
			case "wrong_total":
				result.ReadCoverage.TotalLines = 4
			}
			if name != "not_appended" {
				m.AppendDispatchToolResult(result)
			}
			if got := m.CompleteDispatchRepositoryFileReadVersion("/repo", "tests/native.py", dispatchReadVersionSHA); got != (name == "valid") {
				t.Fatalf("complete=%v; pending/failed/mismatched result cannot authorize coverage", got)
			}
			if m.BeginDispatchRepositoryFileRead() != generation {
				t.Fatal("append changed dispatch generation")
			}
		})
	}
}

func TestDispatchRepositoryFileReadVersionCoverageUnion(t *testing.T) {
	for _, tc := range []struct {
		name  string
		pages [][3]int
		want  bool
	}{
		{"whole", [][3]int{{1, 10, 10}}, true},
		{"pages_reverse", [][3]int{{6, 10, 10}, {1, 5, 10}}, true},
		{"overlap", [][3]int{{1, 7, 10}, {5, 10, 10}, {2, 3, 10}}, true},
		{"gap", [][3]int{{1, 4, 10}, {6, 10, 10}}, false},
		{"missing_head", [][3]int{{2, 10, 10}}, false},
		{"missing_tail", [][3]int{{1, 9, 10}}, false},
		{"contradictory_total", [][3]int{{1, 10, 10}, {1, 11, 11}}, false},
		{"empty", [][3]int{{0, 0, 0}}, true},
		{"empty_conflicts", [][3]int{{0, 0, 0}, {1, 1, 1}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := NewMutableState("read")
			for i, page := range tc.pages {
				dispatchReadVersionAdd(t, m, fmt.Sprint(i), dispatchReadVersionSHA, page[0], page[1], page[2])
			}
			if got := m.CompleteDispatchRepositoryFileReadVersion("/repo", "tests/native.py", dispatchReadVersionSHA); got != tc.want {
				t.Fatalf("complete=%v want %v", got, tc.want)
			}
		})
	}
}

func TestDispatchRepositoryFileReadVersionSeparatesByteVersionsAndIdentities(t *testing.T) {
	m := NewMutableState("read")
	otherSHA := strings.Repeat("b", 64)
	dispatchReadVersionAdd(t, m, "old-head", dispatchReadVersionSHA, 1, 5, 10)
	dispatchReadVersionAdd(t, m, "new-tail", otherSHA, 6, 10, 10)
	for _, sha := range []string{dispatchReadVersionSHA, otherSHA} {
		if m.CompleteDispatchRepositoryFileReadVersion("/repo", "tests/native.py", sha) {
			t.Fatal("different bytes filled each other's missing page")
		}
	}
	dispatchReadVersionAdd(t, m, "old-tail", dispatchReadVersionSHA, 6, 10, 10)
	if !m.CompleteDispatchRepositoryFileReadVersion("/repo", "tests/native.py", dispatchReadVersionSHA) {
		t.Fatal("matching version pages did not complete")
	}
	for _, pair := range [][2]string{{"/other", "tests/native.py"}, {"/repo", "other.py"}, {"", "tests/native.py"}, {"/repo", ""}} {
		if m.CompleteDispatchRepositoryFileReadVersion(pair[0], pair[1], dispatchReadVersionSHA) {
			t.Fatalf("identity mismatch authorized: %v", pair)
		}
	}
	for _, identity := range [][3]string{{"/other", "tests/native.py", "old-head"}, {"/repo", "other.py", "old-head"}, {"/repo", "tests/native.py", "unknown-ref"}} {
		if m.RecordDispatchRepositoryFileReadVersion(m.BeginDispatchRepositoryFileRead(), identity[0], identity[1], identity[2], dispatchReadVersionSHA, 1, 10, 10) {
			t.Fatalf("pending version borrowed a different physical read receipt: %v", identity)
		}
	}
}

func TestDispatchRepositoryFileReadVersionRejectsInvalidCoordinatesAndDigests(t *testing.T) {
	m := NewMutableState("read")
	m.RecordDispatchRepositoryFileRead("/repo", "tests/native.py", "page")
	for _, page := range [][3]int{{-1, 1, 1}, {0, 1, 1}, {2, 1, 2}, {1, 3, 2}, {0, 0, 1}, {1, 1, 0}, {0, 0, -1}} {
		if m.RecordDispatchRepositoryFileReadVersion(m.BeginDispatchRepositoryFileRead(), "/repo", "tests/native.py", "page", dispatchReadVersionSHA, page[0], page[1], page[2]) {
			t.Fatalf("invalid coordinates accepted: %v", page)
		}
	}
	for _, sha := range []string{"", "short", strings.Repeat("g", 64), strings.ToUpper(dispatchReadVersionSHA), dispatchReadVersionSHA + "0"} {
		if m.RecordDispatchRepositoryFileReadVersion(m.BeginDispatchRepositoryFileRead(), "/repo", "tests/native.py", "page", sha, 1, 1, 1) || m.CompleteDispatchRepositoryFileReadVersion("/repo", "tests/native.py", sha) {
			t.Fatalf("invalid digest accepted: %q", sha)
		}
	}
	var missing *MutableState
	if missing.BeginDispatchRepositoryFileRead() != 0 || missing.RecordDispatchRepositoryFileReadVersion(0, "/repo", "tests/native.py", "page", dispatchReadVersionSHA, 1, 1, 1) || missing.CompleteDispatchRepositoryFileReadVersion("/repo", "tests/native.py", dispatchReadVersionSHA) {
		t.Fatal("nil receiver acquired coverage")
	}
}

func TestDispatchRepositoryFileReadVersionEmptyRequiresMatchingEnumeration(t *testing.T) {
	for _, name := range []string{"absent", "partial", "unknown_total", "wrong_scope", "wrong_dimension", "nonzero_emitted", "valid"} {
		t.Run(name, func(t *testing.T) {
			m := NewMutableState("read")
			m.RecordDispatchRepositoryFileRead("/repo", "tests/native.py", "page")
			m.RecordDispatchRepositoryFileReadVersion(m.BeginDispatchRepositoryFileRead(), "/repo", "tests/native.py", "page", dispatchReadVersionSHA, 0, 0, 0)
			r := dispatchReadVersionResult("page", 0, 0, 0)
			switch name {
			case "absent":
				r.EnumerationAuthority = nil
			case "partial":
				r.EnumerationAuthority.Status = "partial"
			case "unknown_total":
				r.EnumerationAuthority.Boundaries[0].TotalKnown = false
			case "wrong_scope":
				r.EnumerationAuthority.Boundaries[0].Scope = "other.py"
			case "wrong_dimension":
				r.EnumerationAuthority.Boundaries[0].Dimension = "events"
			case "nonzero_emitted":
				r.EnumerationAuthority.Boundaries[0].Emitted = 1
			}
			m.AppendDispatchToolResult(r)
			if got := m.CompleteDispatchRepositoryFileReadVersion("/repo", "tests/native.py", dispatchReadVersionSHA); got != (name == "valid") {
				t.Fatalf("empty complete=%v", got)
			}
		})
	}
}

func TestDispatchRepositoryFileReadVersionResetAndSerialization(t *testing.T) {
	m := NewMutableState("read")
	dispatchReadVersionAdd(t, m, "private-version-ref", dispatchReadVersionSHA, 1, 1, 1)
	old := m.BeginDispatchRepositoryFileRead()
	encoded, err := json.Marshal(m)
	if err != nil || strings.Contains(string(encoded), dispatchReadVersionSHA) || strings.Contains(string(encoded), "private-version-ref") {
		t.Fatalf("private receipt serialized: %s %v", encoded, err)
	}
	var restored MutableState
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.CompleteDispatchRepositoryFileReadVersion("/repo", "tests/native.py", dispatchReadVersionSHA) || m.ForkForExploreDispatch().CompleteDispatchRepositoryFileReadVersion("/repo", "tests/native.py", dispatchReadVersionSHA) {
		t.Fatal("receipt inherited by JSON or another dispatch")
	}
	m.ResetDispatchToolResults()
	if m.BeginDispatchRepositoryFileRead() == old || m.CompleteDispatchRepositoryFileReadVersion("/repo", "tests/native.py", dispatchReadVersionSHA) {
		t.Fatal("reset retained generation or completed bytes")
	}
	// A late producer may still append an identically named legacy result;
	// its pre-IO generation must nevertheless fail after reset.
	m.RecordDispatchRepositoryFileRead("/repo", "tests/native.py", "private-version-ref")
	m.AppendDispatchToolResult(dispatchReadVersionResult("private-version-ref", 1, 1, 1))
	if m.RecordDispatchRepositoryFileReadVersion(old, "/repo", "tests/native.py", "private-version-ref", dispatchReadVersionSHA, 1, 1, 1) || m.CompleteDispatchRepositoryFileReadVersion("/repo", "tests/native.py", dispatchReadVersionSHA) {
		t.Fatal("late read crossed reset")
	}
	if !m.HasDispatchRepositoryFileRead("/repo", "tests/native.py", "private-version-ref") {
		t.Fatal("new private version checks changed legacy identity semantics")
	}
	dispatchReadVersionAdd(t, m, "new-version-ref", dispatchReadVersionSHA, 1, 1, 1)
	if !m.CompleteDispatchRepositoryFileReadVersion("/repo", "tests/native.py", dispatchReadVersionSHA) {
		t.Fatal("fresh dispatch read rejected")
	}
}

func TestDispatchRepositoryFileReadVersionConcurrentAccess(t *testing.T) {
	m := NewMutableState("read")
	var wg sync.WaitGroup
	for worker := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 25 {
				generation := m.BeginDispatchRepositoryFileRead()
				ref := fmt.Sprintf("page-%d-%d", worker, i)
				m.RecordDispatchRepositoryFileRead("/repo", "tests/native.py", ref)
				m.RecordDispatchRepositoryFileReadVersion(generation, "/repo", "tests/native.py", ref, dispatchReadVersionSHA, 1, 1, 1)
				m.AppendDispatchToolResult(dispatchReadVersionResult(ref, 1, 1, 1))
				m.CompleteDispatchRepositoryFileReadVersion("/repo", "tests/native.py", dispatchReadVersionSHA)
				m.ResetDispatchToolResults()
			}
		}()
	}
	wg.Wait()
	m.ResetDispatchToolResults()
	dispatchReadVersionAdd(t, m, "final", dispatchReadVersionSHA, 1, 1, 1)
	if !m.CompleteDispatchRepositoryFileReadVersion("/repo", "tests/native.py", dispatchReadVersionSHA) {
		t.Fatal("final serial read rejected")
	}
}
