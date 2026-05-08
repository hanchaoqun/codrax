package perftriage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestCorroborateStallFiles_ClearsHallucinatedSymbol(t *testing.T) {
	dir := t.TempDir()
	// Real file: contains identifier "realFunc" but NOT "fakeSym".
	if err := os.WriteFile(filepath.Join(dir, "real.go"), []byte("package main\nfunc realFunc() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bundle := &types.PerfBundle{
		Stalls: []types.PerfStall{
			{File: "real.go", Symbol: "fakeSym", Kind: "io"},        // file exists, symbol fake
			{File: "real.go", Symbol: "realFunc", Kind: "lock"},     // both real
			{File: "absent.go", Symbol: "anySym", Kind: "sync-rpc"}, // file absent
		},
	}
	cleared := CorroborateStallFiles(bundle, dir)

	// Expect: stall[0] cleared (sym fake), stall[1] kept, stall[2] cleared (file absent)
	if len(cleared) != 2 {
		t.Fatalf("expected 2 cleared stalls, got %d (%v)", len(cleared), cleared)
	}
	if bundle.Stalls[0].File != "" {
		t.Errorf("stall[0] (fake sym) should have File cleared, got %q", bundle.Stalls[0].File)
	}
	// stall[1] file may be normalised; just check it survived (non-empty)
	if bundle.Stalls[1].File == "" {
		t.Errorf("stall[1] (real sym) should have File preserved")
	}
	if bundle.Stalls[2].File != "" {
		t.Errorf("stall[2] (absent file) should have File cleared, got %q", bundle.Stalls[2].File)
	}

	// Cleared list captures both
	gotSyms := []string{cleared[0].Symbol, cleared[1].Symbol}
	if !containsString(gotSyms, "fakeSym") || !containsString(gotSyms, "anySym") {
		t.Errorf("cleared symbols = %v, want both fakeSym + anySym", gotSyms)
	}
}

func TestCorroborateStallFiles_EmptyRepoRoot(t *testing.T) {
	bundle := &types.PerfBundle{
		Stalls: []types.PerfStall{{File: "real.go", Symbol: "anything"}},
	}
	cleared := CorroborateStallFiles(bundle, "")
	if cleared != nil {
		t.Errorf("empty repoRoot should disable gate, got cleared=%v", cleared)
	}
	if bundle.Stalls[0].File != "real.go" {
		t.Errorf("stall.File should be untouched on empty repoRoot")
	}
}

func TestCorroborateStallFiles_SymbolEmptyPasses(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bundle := &types.PerfBundle{
		Stalls: []types.PerfStall{{File: "f.go", Symbol: ""}},
	}
	cleared := CorroborateStallFiles(bundle, dir)
	if len(cleared) != 0 {
		t.Errorf("empty Symbol → gate should pass (file exists), got cleared=%v", cleared)
	}
}

func TestCorroborateStallFiles_DotSegmentMatch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Foo.java"),
		[]byte("class Foo { void doWork() {} }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bundle := &types.PerfBundle{
		Stalls: []types.PerfStall{
			{File: "Foo.java", Symbol: "com.example.Foo.doWork"}, // FQN — last segment doWork is in file
		},
	}
	cleared := CorroborateStallFiles(bundle, dir)
	if len(cleared) != 0 {
		t.Errorf("FQN last-segment match should pass, got cleared=%v", cleared)
	}
}

func TestCorroborateStallFiles_NilBundle(t *testing.T) {
	cleared := CorroborateStallFiles(nil, "/some/dir")
	if cleared != nil {
		t.Errorf("nil bundle should return nil, got %v", cleared)
	}
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
