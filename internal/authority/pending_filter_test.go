package authority

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

type stubLocator struct {
	locate map[string][]types.SymbolLocation
	inFile map[string][]types.SymbolLocation
}

func (s *stubLocator) LocateSymbol(name string) []types.SymbolLocation {
	return append([]types.SymbolLocation(nil), s.locate[name]...)
}

func (s *stubLocator) SymbolsInFile(file string) []types.SymbolLocation {
	return append([]types.SymbolLocation(nil), s.inFile[file]...)
}

// TestPendingFilteredLocator_LocateSymbol_DropsPendingSubRepo is the
// regression for residual A: ground recovery / drift detection MUST
// NOT anchor a citation to a sub-repo the MultiRepoActiveSetGater
// would refuse. Companion to the 2026-05-16 finalize-loop fix.
func TestPendingFilteredLocator_LocateSymbol_DropsPendingSubRepo(t *testing.T) {
	inner := &stubLocator{
		locate: map[string][]types.SymbolLocation{
			"Orchestrator": {
				{File: "codrax/internal/orchestrator/orchestrator.go", Line: 42, EndLine: 50},
				{File: "claude/codrax/internal/orchestrator/orchestrator.go", Line: 42, EndLine: 50},
				{File: "small/codrax-small/internal/orchestrator/orchestrator.go", Line: 50, EndLine: 60},
			},
		},
	}
	loc := newPendingFilteredLocator(inner, []string{"claude/codrax", "small/codrax-small"})
	got := loc.LocateSymbol("Orchestrator")
	if len(got) != 1 {
		t.Fatalf("expected 1 active-subrepo hit, got %d: %+v", len(got), got)
	}
	if got[0].File != "codrax/internal/orchestrator/orchestrator.go" {
		t.Errorf("expected active codrax hit, got %s", got[0].File)
	}
}

func TestPendingFilteredLocator_SymbolsInFile_RefusesPendingFile(t *testing.T) {
	inner := &stubLocator{
		inFile: map[string][]types.SymbolLocation{
			"claude/codrax/x.go": {{File: "claude/codrax/x.go", Line: 1}},
			"codrax/y.go":        {{File: "codrax/y.go", Line: 1}},
		},
	}
	loc := newPendingFilteredLocator(inner, []string{"claude/codrax"})
	if got := loc.SymbolsInFile("claude/codrax/x.go"); got != nil {
		t.Errorf("query of pending-sub-repo file should return nil, got %+v", got)
	}
	if got := loc.SymbolsInFile("codrax/y.go"); len(got) != 1 {
		t.Errorf("active-sub-repo file should pass through, got %+v", got)
	}
}

func TestPendingFilteredLocator_NoPending_PassesThrough(t *testing.T) {
	inner := &stubLocator{
		locate: map[string][]types.SymbolLocation{
			"X": {{File: "codrax/x.go", Line: 1}},
		},
	}
	// Empty pending list → return the inner locator unchanged.
	got := newPendingFilteredLocator(inner, nil)
	if got != types.SymbolLocator(inner) {
		t.Errorf("empty pending list should return inner locator unchanged")
	}
}

func TestPathInsidePendingSubRepoExported(t *testing.T) {
	pending := []string{"claude/codrax", "small/codrax-small"}
	cases := []struct {
		file string
		want bool
	}{
		{"claude/codrax/internal/x.go", true},
		{"claude/codrax", true},
		{"small/codrax-small/foo", true},
		{"codrax/internal/x.go", false},
		{"opencode/src/x.ts", false},
		{"claude/codrax-extra/x.go", false}, // prefix bleed guard
		{"", false},
	}
	for _, c := range cases {
		if got := PathInsidePendingSubRepo(c.file, pending); got != c.want {
			t.Errorf("PathInsidePendingSubRepo(%q) = %v, want %v", c.file, got, c.want)
		}
	}
}
