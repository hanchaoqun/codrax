package logtriage

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// fakeLocator is a minimal in-memory SymbolLocator used by drift tests.
// Defs maps symbol name → list of definition sites; Files maps
// repo-relative path → list of symbols defined there.
type fakeLocator struct {
	Defs  map[string][]types.SymbolLocation
	Files map[string][]types.SymbolLocation
}

func (f *fakeLocator) LocateSymbol(name string) []types.SymbolLocation {
	if f == nil {
		return nil
	}
	return f.Defs[name]
}

func (f *fakeLocator) SymbolsInFile(file string) []types.SymbolLocation {
	if f == nil {
		return nil
	}
	return f.Files[normalizeFrameFile(file)]
}

// TestDetectDriftForFrame_NilLocatorReturnsUnknown defends the legacy-
// safety contract: with no locator wired, detection is a no-op and
// callers default to factual via AuthorityCeiling's Unknown
// passthrough. Without this, single-shot CLI flows or tests that
// don't build a repomap would crash on the first emit_evidence.
func TestDetectDriftForFrame_NilLocatorReturnsUnknown(t *testing.T) {
	frame := types.LogFrame{File: "internal/foo.go", Line: 42, Func: "Bar"}
	got := DetectDriftForFrame(frame, nil)
	if got.Status != types.DriftStatusUnknown {
		t.Errorf("nil locator: Status = %q; want %q",
			got.Status, types.DriftStatusUnknown)
	}
}

// TestDetectDriftForFrame_PerfectMatch fires DriftStatusNone when log
// claim is at the exact def line. Authority cap is factual.
func TestDetectDriftForFrame_PerfectMatch(t *testing.T) {
	loc := &fakeLocator{
		Defs: map[string][]types.SymbolLocation{
			"Run": {{File: "internal/agent/explorer.go", Line: 100, EndLine: 200}},
		},
	}
	frame := types.LogFrame{File: "internal/agent/explorer.go", Line: 100, Func: "Run"}
	got := DetectDriftForFrame(frame, loc)
	if got.Status != types.DriftStatusNone {
		t.Errorf("perfect match: Status = %q; want %q", got.Status, types.DriftStatusNone)
	}
	if got.Status.AuthorityCeiling() != types.AuthorityFactual {
		t.Errorf("perfect match: AuthorityCeiling = %q; want %q",
			got.Status.AuthorityCeiling(), types.AuthorityFactual)
	}
}

// TestDetectDriftForFrame_LineWithinFunctionBody — log frame's line
// falls inside [defLine, endLine], still classified as None.
func TestDetectDriftForFrame_LineWithinFunctionBody(t *testing.T) {
	loc := &fakeLocator{
		Defs: map[string][]types.SymbolLocation{
			"Run": {{File: "explorer.go", Line: 100, EndLine: 200}},
		},
	}
	frame := types.LogFrame{File: "explorer.go", Line: 150, Func: "Run"}
	got := DetectDriftForFrame(frame, loc)
	if got.Status != types.DriftStatusNone {
		t.Errorf("line-inside-body: Status = %q; want %q",
			got.Status, types.DriftStatusNone)
	}
}

// TestDetectDriftForFrame_LineDriftBeyondGap — same file + same func,
// log line outside [defLine - gap, endLine + gap]. Authority cap
// downgrades to conditional.
func TestDetectDriftForFrame_LineDriftBeyondGap(t *testing.T) {
	loc := &fakeLocator{
		Defs: map[string][]types.SymbolLocation{
			"Run": {{File: "explorer.go", Line: 100, EndLine: 110}},
		},
	}
	// Log claim at line 50; function now lives at 100-110. gap=2 still
	// puts 50 well outside the window.
	frame := types.LogFrame{File: "explorer.go", Line: 50, Func: "Run"}
	got := DetectDriftForFrame(frame, loc)
	if got.Status != types.DriftStatusLineDrift {
		t.Errorf("line drift: Status = %q; want %q",
			got.Status, types.DriftStatusLineDrift)
	}
	if got.AnchoredLine != 100 {
		t.Errorf("line drift: AnchoredLine = %d; want 100", got.AnchoredLine)
	}
	if got.OriginalLine != 50 {
		t.Errorf("line drift: OriginalLine = %d; want 50", got.OriginalLine)
	}
	if got.Status.AuthorityCeiling() != types.AuthorityConditional {
		t.Errorf("line drift: AuthorityCeiling = %q; want %q",
			got.Status.AuthorityCeiling(), types.AuthorityConditional)
	}
}

// TestDetectDriftForFrame_FileMoved — function defined in a different
// file than the log claims. Authority cap = historical.
func TestDetectDriftForFrame_FileMoved(t *testing.T) {
	loc := &fakeLocator{
		Defs: map[string][]types.SymbolLocation{
			"Run": {{File: "internal/agent/runner.go", Line: 25, EndLine: 80}},
		},
	}
	// Log says Run was in old/explorer.go; current code has it in
	// internal/agent/runner.go.
	frame := types.LogFrame{File: "old/explorer.go", Line: 100, Func: "Run"}
	got := DetectDriftForFrame(frame, loc)
	if got.Status != types.DriftStatusFileMoved {
		t.Errorf("file moved: Status = %q; want %q",
			got.Status, types.DriftStatusFileMoved)
	}
	if got.AnchoredFile != "internal/agent/runner.go" {
		t.Errorf("file moved: AnchoredFile = %q; want internal/agent/runner.go",
			got.AnchoredFile)
	}
	if got.OriginalFile != "old/explorer.go" {
		t.Errorf("file moved: OriginalFile = %q; want old/explorer.go",
			got.OriginalFile)
	}
	if got.Status.AuthorityCeiling() != types.AuthorityHistorical {
		t.Errorf("file moved: AuthorityCeiling = %q; want %q",
			got.Status.AuthorityCeiling(), types.AuthorityHistorical)
	}
}

// TestDetectDriftForFrame_TailRename — log file is indexed, but the
// claimed function name doesn't appear anywhere; the file has other
// symbols. Authority cap = conditional.
func TestDetectDriftForFrame_TailRename(t *testing.T) {
	loc := &fakeLocator{
		Defs: map[string][]types.SymbolLocation{},
		Files: map[string][]types.SymbolLocation{
			"explorer.go": {
				{File: "explorer.go", Line: 50, EndLine: 80},
				{File: "explorer.go", Line: 100, EndLine: 200},
			},
		},
	}
	// "RunOld" is not defined anywhere; explorer.go does have other
	// symbols → tail rename.
	frame := types.LogFrame{File: "explorer.go", Line: 110, Func: "RunOld"}
	got := DetectDriftForFrame(frame, loc)
	if got.Status != types.DriftStatusTailRename {
		t.Errorf("tail rename: Status = %q; want %q",
			got.Status, types.DriftStatusTailRename)
	}
	if got.AnchoredFile != "explorer.go" {
		t.Errorf("tail rename: AnchoredFile = %q; want explorer.go",
			got.AnchoredFile)
	}
	if got.Status.AuthorityCeiling() != types.AuthorityConditional {
		t.Errorf("tail rename: AuthorityCeiling = %q; want %q",
			got.Status.AuthorityCeiling(), types.AuthorityConditional)
	}
}

// TestDetectDriftForFrame_Unmappable — file not indexed AND function
// not defined anywhere. Function/file removed entirely. Authority cap =
// historical.
func TestDetectDriftForFrame_Unmappable(t *testing.T) {
	loc := &fakeLocator{} // empty — nothing indexed
	frame := types.LogFrame{File: "deleted/orphan.go", Line: 10, Func: "Gone"}
	got := DetectDriftForFrame(frame, loc)
	if got.Status != types.DriftStatusUnmappable {
		t.Errorf("unmappable: Status = %q; want %q",
			got.Status, types.DriftStatusUnmappable)
	}
	if got.AnchoredFile != "" {
		t.Errorf("unmappable: AnchoredFile = %q; want \"\"", got.AnchoredFile)
	}
	if got.Status.AuthorityCeiling() != types.AuthorityHistorical {
		t.Errorf("unmappable: AuthorityCeiling = %q; want %q",
			got.Status.AuthorityCeiling(), types.AuthorityHistorical)
	}
}

// TestDetectDriftForFrame_MissingFuncOrFileReturnsUnknown — both fields
// are required for a useful verdict. Empty Func or empty File on the
// frame yields Unknown so the AuthorityCeiling consumer treats it as
// legacy factual.
func TestDetectDriftForFrame_MissingFuncOrFileReturnsUnknown(t *testing.T) {
	loc := &fakeLocator{
		Defs: map[string][]types.SymbolLocation{
			"Run": {{File: "x.go", Line: 1}},
		},
	}
	cases := []types.LogFrame{
		{File: "", Line: 10, Func: "Run"},
		{File: "x.go", Line: 10, Func: ""},
		{File: "", Line: 0, Func: ""},
	}
	for i, frame := range cases {
		if got := DetectDriftForFrame(frame, loc); got.Status != types.DriftStatusUnknown {
			t.Errorf("case %d (frame=%+v): Status = %q; want %q",
				i, frame, got.Status, types.DriftStatusUnknown)
		}
	}
}

// TestNormalizeFrameFuncForDrift exercises the symbol-tail extractor
// against the eight forms documented in the function comment.
func TestNormalizeFrameFuncForDrift(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"main.main", "main"},
		{"runtime.gopark", "gopark"},
		{"(*MutableState).SetLogTriage", "SetLogTriage"},
		{"github.com/foo/bar.(*Server).Run", "Run"},
		{"com.example.Foo.bar", "bar"},
		{"Foo::bar(int, char*)", "bar"},
		{"main.handler.func1", "func1"},
		{"   ", ""},
		{"", ""},
	}
	for _, c := range cases {
		got := normalizeFrameFuncForDrift(c.in)
		if got != c.want {
			t.Errorf("normalize(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

// TestFrameDriftStatusAuthorityCeiling_TableMappings asserts the
// status → ceiling projection matches the documented table.
func TestFrameDriftStatusAuthorityCeiling_TableMappings(t *testing.T) {
	cases := map[types.FrameDriftStatus]types.AuthorityCeiling{
		types.DriftStatusUnknown:    types.AuthorityUnknown,
		types.DriftStatusNone:       types.AuthorityFactual,
		types.DriftStatusLineDrift:  types.AuthorityConditional,
		types.DriftStatusTailRename: types.AuthorityConditional,
		types.DriftStatusFileMoved:  types.AuthorityHistorical,
		types.DriftStatusUnmappable: types.AuthorityHistorical,
	}
	for status, want := range cases {
		if got := status.AuthorityCeiling(); got != want {
			t.Errorf("FrameDriftStatus(%q).AuthorityCeiling() = %q; want %q",
				status, got, want)
		}
	}
}
