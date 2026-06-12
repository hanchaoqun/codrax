package types

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
)

func TestTypedDenialSet_AddDedup(t *testing.T) {
	s := &TypedDenialSet{}
	s.Add(TypedDenial{Class: TypedDenialExternalLogFrameUnresolved, Token: "internal/foo.go", Reason: "1st"})
	s.Add(TypedDenial{Class: TypedDenialExternalLogFrameUnresolved, Token: "internal/foo.go", Reason: "2nd"})
	if got := s.Len(); got != 1 {
		t.Errorf("dedup failed: Len = %d want 1", got)
	}
	s.Add(TypedDenial{Class: TypedDenialExternalLogFrameUnresolved, Token: "internal/bar.go"})
	if got := s.Len(); got != 2 {
		t.Errorf("Len = %d want 2 after 2nd path", got)
	}
}

func TestTypedDenialSet_Add_RejectsInvalid(t *testing.T) {
	s := &TypedDenialSet{}
	s.Add(TypedDenial{Class: TypedDenialExternalLogFrameUnresolved, Token: ""})
	s.Add(TypedDenial{Class: TypedDenialExternalLogFrameUnresolved, Token: "  "})
	s.Add(TypedDenial{Class: "bogus", Token: "x.go"})
	if s.Len() != 0 {
		t.Errorf("invalid Add not rejected: %v", s.Snapshot())
	}
}

func TestTypedDenialSet_IsPathDenied(t *testing.T) {
	s := &TypedDenialSet{}
	s.Add(TypedDenial{Class: TypedDenialExternalLogFrameUnresolved, Token: "internal/agent/analyzer.go"})
	cases := []struct {
		path string
		want bool
	}{
		{"internal/agent/analyzer.go", true},                         // exact
		{"/abs/repo/internal/agent/analyzer.go", true},                // abs suffix-matches relative
		{"internal/agent/analyzer.go ", true},                         // trim
		{"internal/agent/analyzer_test.go", false},                    // similar but different
		{"internal/agent/explorer.go", false},                         // unrelated
		{"", false},
	}
	for _, c := range cases {
		if got := s.IsPathDenied(c.path); got != c.want {
			t.Errorf("IsPathDenied(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestTypedDenialSet_IsSymbolDenied(t *testing.T) {
	s := &TypedDenialSet{}
	s.Add(TypedDenial{Class: TypedDenialOracleSymbolUnverified, Token: "writeSession"})
	if !s.IsSymbolDenied("writeSession") {
		t.Errorf("symbol miss")
	}
	if s.IsSymbolDenied("writesession") {
		t.Errorf("case fold should NOT match (oracle flat-form is its own layer)")
	}
	if s.IsSymbolDenied("WriteSession") {
		t.Errorf("case mismatch")
	}
}

func TestTypedDenialSet_Sanitise(t *testing.T) {
	s := &TypedDenialSet{}
	s.Add(TypedDenial{Class: TypedDenialExternalLogFrameUnresolved, Token: "internal/agent/analyzer.go"})
	prose := "main.writeSession at internal/agent/analyzer.go:100 +0x1e — 3 goroutines"
	got := s.Sanitise(prose)
	want := "main.writeSession at <unverified-external-source>:100 +0x1e — 3 goroutines"
	if got != want {
		t.Errorf("Sanitise:\n got  %q\n want %q", got, want)
	}
}

func TestTypedDenialSet_Sanitise_MultiplePathsSubrepo(t *testing.T) {
	s := &TypedDenialSet{}
	s.Add(TypedDenial{Class: TypedDenialExternalLogFrameUnresolved, Token: "foo.go"})
	s.Add(TypedDenial{Class: TypedDenialOracleSymbolUnverified, Token: "Bar"})
	prose := "Bar() called from foo.go:42 — Bar's behaviour in foo.go is suspect"
	got := s.Sanitise(prose)
	if got == prose {
		t.Errorf("Sanitise should have replaced tokens; got unchanged %q", got)
	}
	want := "<unverified-unknown-symbol>() called from <unverified-external-source>:42 — <unverified-unknown-symbol>'s behaviour in <unverified-external-source> is suspect"
	if got != want {
		t.Errorf("Sanitise unexpected result:\n got  %q\n want %q", got, want)
	}
}

// TestTypedDenialSet_Sanitise_CrossOSPathSep: a denial stamped with a
// POSIX path also redacts mentions in prose using Windows separators
// (and vice versa), so an attached log produced on Linux but rendered
// to a Windows-running LLM (or the reverse) gets consistent treatment.
func TestTypedDenialSet_Sanitise_CrossOSPathSep(t *testing.T) {
	s := &TypedDenialSet{}
	s.Add(TypedDenial{Class: TypedDenialExternalLogFrameUnresolved, Token: "internal/agent/analyzer.go"})
	// Prose uses Windows separator
	prose := `main.writeSession at internal\agent\analyzer.go:100 +0x1e`
	got := s.Sanitise(prose)
	want := "main.writeSession at <unverified-external-source>:100 +0x1e"
	if got != want {
		t.Errorf("cross-OS Sanitise:\n got  %q\n want %q", got, want)
	}
}

// TestTypedDenialSet_IsPathDenied_CrossOSPathSep mirrors the above
// for the IsPathDenied gate (tool registry refusal path).
func TestTypedDenialSet_IsPathDenied_CrossOSPathSep(t *testing.T) {
	s := &TypedDenialSet{}
	// Token is POSIX
	s.Add(TypedDenial{Class: TypedDenialExternalLogFrameUnresolved, Token: "internal/agent/analyzer.go"})
	// Query using Windows separator should still match
	if !s.IsPathDenied(`internal\agent\analyzer.go`) {
		t.Errorf("Windows-separator query did not match POSIX-separator denial")
	}
	if !s.IsPathDenied(`C:\repo\internal\agent\analyzer.go`) {
		t.Errorf("Windows abs path did not suffix-match POSIX-separator denial")
	}

	// Reverse: token is Windows-form, query is POSIX
	s2 := &TypedDenialSet{}
	s2.Add(TypedDenial{Class: TypedDenialExternalLogFrameUnresolved, Token: `subdir\file.go`})
	if !s2.IsPathDenied("subdir/file.go") {
		t.Errorf("POSIX-separator query did not match Windows-separator denial")
	}
}

func TestTypedDenialSet_PathTokensSymbolTokens(t *testing.T) {
	s := &TypedDenialSet{}
	s.Add(TypedDenial{Class: TypedDenialExternalLogFrameUnresolved, Token: "p1.go"})
	s.Add(TypedDenial{Class: TypedDenialDriftFrameRelocated, Token: "p2.go"})
	s.Add(TypedDenial{Class: TypedDenialOracleSymbolUnverified, Token: "Sym1"})
	s.Add(TypedDenial{Class: TypedDenialEvidenceSubjectUnverified, Token: "Sym2"})

	paths := s.PathTokens()
	if len(paths) != 2 {
		t.Errorf("PathTokens len = %d want 2 (got %v)", len(paths), paths)
	}
	syms := s.SymbolTokens()
	if len(syms) != 2 {
		t.Errorf("SymbolTokens len = %d want 2 (got %v)", len(syms), syms)
	}
}

// TestTypedDenialSet_ConcurrentStampAndRead pins the internal
// synchronization contract: the Run-level set is shared by pointer
// across parallel explore workers and parallel sub-agents, so
// concurrent stamps (future explore-stage gate sites) and concurrent
// reads (L1 gates, L2 sanitise, JSON marshal) must be race-free.
// Run under -race.
func TestTypedDenialSet_ConcurrentStampAndRead(t *testing.T) {
	s := NewTypedDenialSet()
	var wg sync.WaitGroup
	start := make(chan struct{})

	for w := 0; w < 4; w++ {
		w := w
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < 100; i++ {
				s.Add(TypedDenial{
					Class: TypedDenialExternalLogFrameUnresolved,
					Token: fmt.Sprintf("external/w%d-%d.go", w, i),
				})
			}
		}()
	}
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < 100; i++ {
				_ = s.IsPathDenied("external/w0-1.go")
				_ = s.Sanitise("frame at external/w1-2.go:10")
				_ = s.Snapshot()
				_ = s.Len()
				if _, err := json.Marshal(s); err != nil {
					t.Errorf("marshal under concurrent stamps: %v", err)
					return
				}
			}
		}()
	}

	close(start)
	wg.Wait()
	if got := s.Len(); got != 400 {
		t.Errorf("Len = %d, want 400 distinct concurrent stamps", got)
	}
}

// TestTypedDenialSet_JSONRoundTrip pins the wire shape across the
// unexported-slice refactor: {"denials":[...]} both directions.
func TestTypedDenialSet_JSONRoundTrip(t *testing.T) {
	s := NewTypedDenialSet()
	s.Add(TypedDenial{Class: TypedDenialExternalLogFrameUnresolved, Token: "a.go", Reason: "r"})
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"denials":[{"class":"external_log_frame_unresolved","token":"a.go","reason":"r"}]}`
	if string(raw) != want {
		t.Errorf("wire shape drifted:\n got  %s\n want %s", raw, want)
	}
	back := NewTypedDenialSet()
	if err := json.Unmarshal(raw, back); err != nil {
		t.Fatal(err)
	}
	if back.Len() != 1 || !back.IsPathDenied("a.go") {
		t.Errorf("round-trip lost the denial: %+v", back.Snapshot())
	}
}

// TestTypedDenialSet_FirstPathDenial_SuffixMatchKeepsClassReason pins
// the refusal-rendering contract: FirstPathDenial uses the SAME
// exact/suffix match rules as IsPathDenied, so a gate that fires on a
// repo-relative ↔ absolute mismatch still surfaces the class-specific
// denial (and from it HumanRefusalReason) instead of a zero value.
// Regression target: the repo_map refusal helper used exact-only
// matching, so suffix-matched refusals degraded to the generic
// "marked unverifiable" fallback.
func TestTypedDenialSet_FirstPathDenial_SuffixMatchKeepsClassReason(t *testing.T) {
	s := NewTypedDenialSet()
	s.Add(TypedDenial{
		Class:  TypedDenialExternalLogFrameUnresolved,
		Token:  "internal/agent/analyzer.go",
		Reason: "frame func missing",
	})
	s.Add(TypedDenial{Class: TypedDenialOracleSymbolUnverified, Token: "PhantomSym"})

	queries := []string{
		"internal/agent/analyzer.go",           // exact
		"/abs/repo/internal/agent/analyzer.go", // absolute query, relative token
		`internal\agent\analyzer.go`,           // cross-OS separator
	}
	for _, q := range queries {
		d, ok := s.FirstPathDenial(q)
		if !ok {
			t.Errorf("FirstPathDenial(%q) missed; IsPathDenied=%v", q, s.IsPathDenied(q))
			continue
		}
		if d.Class != TypedDenialExternalLogFrameUnresolved || d.Reason != "frame func missing" {
			t.Errorf("FirstPathDenial(%q) returned wrong denial: %+v", q, d)
		}
	}
	// Parity with the gate predicate: anything IsPathDenied accepts,
	// FirstPathDenial must resolve (they share one implementation).
	if _, ok := s.FirstPathDenial("internal/agent/explorer.go"); ok {
		t.Error("FirstPathDenial matched an undenied path")
	}
	// Symbol-shaped denials never satisfy the path lookup and vice versa.
	if _, ok := s.FirstPathDenial("PhantomSym"); ok {
		t.Error("FirstPathDenial matched a symbol-shaped denial")
	}
	if d, ok := s.FirstSymbolDenial("PhantomSym"); !ok || d.Class != TypedDenialOracleSymbolUnverified {
		t.Errorf("FirstSymbolDenial(PhantomSym) = %+v, %v", d, ok)
	}
	if _, ok := s.FirstSymbolDenial("internal/agent/analyzer.go"); ok {
		t.Error("FirstSymbolDenial matched a path-shaped denial")
	}
	// nil safety mirrors the predicates.
	var nilSet *TypedDenialSet
	if _, ok := nilSet.FirstPathDenial("x.go"); ok {
		t.Error("nil receiver FirstPathDenial should report no match")
	}
	if _, ok := nilSet.FirstSymbolDenial("X"); ok {
		t.Error("nil receiver FirstSymbolDenial should report no match")
	}
}

func TestTypedDenialSet_NilSafety(t *testing.T) {
	var s *TypedDenialSet // nil
	s.Add(TypedDenial{Class: TypedDenialExternalLogFrameUnresolved, Token: "x.go"})
	if got := s.Len(); got != 0 {
		t.Errorf("nil receiver Len should be 0")
	}
	if s.IsPathDenied("x.go") {
		t.Errorf("nil receiver IsPathDenied should be false")
	}
	if s.Sanitise("foo x.go bar") != "foo x.go bar" {
		t.Errorf("nil receiver Sanitise should pass through")
	}
}
