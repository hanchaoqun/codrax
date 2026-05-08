package types

import "testing"

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
		t.Errorf("invalid Add not rejected: %v", s.Denials)
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
