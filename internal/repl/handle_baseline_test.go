package repl

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// newBaselineTestREPL builds a minimal REPL fixture with a temp
// runtimeAnchor so the /baseline handler has a real disk surface
// to inspect. Doesn't wire the full input loop — we call the
// handler directly. interactive()=false by setting in to a
// strings.Reader so info/warn/success/errorf write to r.out
// instead of the subdued ANSI printers (which go to stderr and
// are unavailable in test capture).
func newBaselineTestREPL(t *testing.T) (*REPL, string, *bytes.Buffer) {
	t.Helper()
	anchor := t.TempDir()
	out := &bytes.Buffer{}
	r := &REPL{
		runtimeAnchor: anchor,
		out:           out,
		in:            strings.NewReader(""),
		colorMode:     render.ColorNever,
		language:      "en",
	}
	return r, anchor, out
}

// seedBaselineEntry writes a synthetic cache file as the production
// BaselineCache.Store would.
func seedBaselineEntry(t *testing.T, anchor, sha string, report *types.ChangeReport) {
	t.Helper()
	dir := filepath.Join(anchor, "baselines")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if len(sha) > 16 {
		sha = sha[:16]
	}
	path := filepath.Join(dir, sha+".json")
	data, _ := json.MarshalIndent(report, "", "  ")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestHandleBaselineCmd_ListEmpty(t *testing.T) {
	r, _, out := newBaselineTestREPL(t)
	r.handleBaselineCmd("/baseline list")
	got := out.String()
	if !strings.Contains(got, "0 cached") && !strings.Contains(got, "does not exist") {
		t.Errorf("expected zero-entries message; got %q", got)
	}
}

func TestHandleBaselineCmd_ListWithEntries(t *testing.T) {
	r, anchor, out := newBaselineTestREPL(t)
	seedBaselineEntry(t, anchor, "1234567890abcdef1111", &types.ChangeReport{PlanID: "p1", Passed: true})
	seedBaselineEntry(t, anchor, "fedcba0987654321aaaa", &types.ChangeReport{PlanID: "p2", Passed: false})
	r.handleBaselineCmd("/baseline list")
	got := out.String()
	if !strings.Contains(got, "2 cached baseline") {
		t.Errorf("expected '2 cached' line; got %q", got)
	}
	if !strings.Contains(got, "1234567890abcdef") || !strings.Contains(got, "fedcba0987654321") {
		t.Errorf("expected both SHAs in output; got %q", got)
	}
}

func TestHandleBaselineCmd_ShowMatchesPrefix(t *testing.T) {
	r, anchor, out := newBaselineTestREPL(t)
	seedBaselineEntry(t, anchor, "abcdef1234567890aaaa", &types.ChangeReport{
		PlanID: "demo", Passed: true,
		TestResults: []types.TestResult{{AssertionID: "TestFoo", Passed: true}},
	})
	r.handleBaselineCmd("/baseline show abcdef12")
	got := out.String()
	if !strings.Contains(got, "plan_id:") || !strings.Contains(got, "demo") {
		t.Errorf("show output should include plan_id and PlanID; got %q", got)
	}
	if !strings.Contains(got, "test_count:  1") {
		t.Errorf("show output should include test count; got %q", got)
	}
}

func TestHandleBaselineCmd_ShowMissing(t *testing.T) {
	r, anchor, out := newBaselineTestREPL(t)
	// Seed a different SHA so the dir exists; the SHOW arg
	// won't match it. This separates "dir doesn't exist" (covered
	// in ListEmpty) from "dir exists but no match" (the case here).
	seedBaselineEntry(t, anchor, "1111111111111111aaaa", &types.ChangeReport{PlanID: "p"})
	r.handleBaselineCmd("/baseline show deadbeef")
	got := out.String()
	if !strings.Contains(got, "no cached entry") {
		t.Errorf("expected miss message; got %q", got)
	}
}

func TestHandleBaselineCmd_Clear(t *testing.T) {
	r, anchor, out := newBaselineTestREPL(t)
	seedBaselineEntry(t, anchor, "1111111111111111aaaa", &types.ChangeReport{PlanID: "p1"})
	seedBaselineEntry(t, anchor, "2222222222222222bbbb", &types.ChangeReport{PlanID: "p2"})
	r.handleBaselineCmd("/baseline clear")
	got := out.String()
	if !strings.Contains(got, "removed 2") {
		t.Errorf("expected 'removed 2' message; got %q", got)
	}
	// Files should be gone.
	entries, _ := os.ReadDir(filepath.Join(anchor, "baselines"))
	jsonCount := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			jsonCount++
		}
	}
	if jsonCount != 0 {
		t.Errorf("expected 0 .json files after clear; got %d", jsonCount)
	}
}

func TestHandleBaselineCmd_DefaultIsList(t *testing.T) {
	r, _, out := newBaselineTestREPL(t)
	r.handleBaselineCmd("/baseline")
	got := out.String()
	if got == "" {
		t.Error("/baseline (no subcommand) should produce output")
	}
}

// TestHandleBaselineCmd_ShowAmbiguousPrefix pins commit 15 #6:
// when more than one cached entry matches the supplied prefix,
// the handler refuses with a list of candidates instead of
// silently picking one.
func TestHandleBaselineCmd_ShowAmbiguousPrefix(t *testing.T) {
	r, anchor, out := newBaselineTestREPL(t)
	seedBaselineEntry(t, anchor, "abcdef0001000000aaaa", &types.ChangeReport{PlanID: "p1"})
	seedBaselineEntry(t, anchor, "abcdef0002000000bbbb", &types.ChangeReport{PlanID: "p2"})
	r.handleBaselineCmd("/baseline show abcdef")
	got := out.String()
	if !strings.Contains(got, "matches 2 entries") {
		t.Errorf("expected 'matches 2 entries' warning; got %q", got)
	}
	if !strings.Contains(got, "abcdef0001") || !strings.Contains(got, "abcdef0002") {
		t.Errorf("expected both candidate SHAs in output; got %q", got)
	}
	if !strings.Contains(got, "longer prefix") {
		t.Errorf("expected 'longer prefix' guidance; got %q", got)
	}
}

func TestHandleBaselineCmd_UnknownSubcommand(t *testing.T) {
	r, _, out := newBaselineTestREPL(t)
	r.handleBaselineCmd("/baseline blarg")
	got := out.String()
	if !strings.Contains(got, "unknown") {
		t.Errorf("expected 'unknown' warning for /baseline blarg; got %q", got)
	}
}
