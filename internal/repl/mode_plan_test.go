package repl

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// newScriptedREPL returns a REPL wired for line-oriented scripted
// tests (no Bubble Tea). Callers drive input by supplying a
// strings.Reader and read output from the returned *bytes.Buffer.
// Store is nil — tests that touch memory must opt in via a real
// store or skip the persistence path.
//
// Language is forced to English here so the historical assertions
// ("No pending plan", "Approve cancelled", "mode set to plan")
// stay deterministic across the bilingual messages.go rollout.
// Tests that want to exercise the zh path explicitly should set
// r.language = "zh" before driving the handler.
func newScriptedREPL(t *testing.T, planStore *PlanStore) (*REPL, *bytes.Buffer) {
	t.Helper()
	in := strings.NewReader("") // unused but non-nil triggers line-oriented mode
	out := &bytes.Buffer{}
	r := New(Config{
		In:           in,
		Out:          out,
		RepoRoot:     "/tmp/repo",
		Branch:       "main",
		Prompt:       "",
		PlanStore:    planStore,
		Language:     "en",
		WriteEnabled: true, // tests target post-gate handler behaviour
	})
	return r, out
}

// TestHandleMode_ShowDefault verifies /mode with no argument prints
// the current mode (ModeRead by default).
func TestHandleMode_ShowDefault(t *testing.T) {
	r, out := newScriptedREPL(t, nil)
	r.handleModeCmd("/mode")
	got := out.String()
	if !strings.Contains(got, "current mode: read") {
		t.Errorf("expected 'current mode: read', got: %q", got)
	}
}

// TestHandleMode_SetPlan verifies /mode plan sets currentMode
// correctly AND the next dispatch would propagate it via SetMode.
func TestHandleMode_SetPlan(t *testing.T) {
	r, out := newScriptedREPL(t, nil)
	r.handleModeCmd("/mode plan")
	if r.currentMode != types.ModePlan {
		t.Errorf("currentMode = %q, want %q", r.currentMode, types.ModePlan)
	}
	if !strings.Contains(out.String(), "Switched to plan mode") {
		t.Errorf("expected success message, got: %q", out.String())
	}
}

// TestHandleMode_PrintsWorkflowHint locks the UX contract that every
// /mode plan|apply|verify transition prints a short workflow hint
// pointing at /approve / /reject / /mode read so a user new to write
// mode does not have to read the docs.
func TestHandleMode_PrintsWorkflowHint(t *testing.T) {
	cases := []struct {
		mode     string
		mustHave []string
	}{
		// Updated to match the customer-feedback rewrite:
		// concise hints, no internal terms (was "ChangePlan"
		// which leaked the schema name; now "change proposal").
		{"plan", []string{"change proposal", "/approve", "/reject", "/mode read"}},
		{"apply", []string{"/approve"}},
		{"verify", []string{"already-applied plan"}},
	}
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			in := strings.NewReader("")
			out := &bytes.Buffer{}
			r := New(Config{
				In: in, Out: out,
				RepoRoot: "/tmp/repo", Branch: "main",
				Language: "en", WriteEnabled: true,
			})
			r.handleModeCmd("/mode " + tc.mode)
			got := out.String()
			for _, want := range tc.mustHave {
				if !strings.Contains(got, want) {
					t.Errorf("/mode %s hint missing %q; got:\n%s", tc.mode, want, got)
				}
			}
		})
	}
	// Read mode prints the success line but no extra hint (nothing to
	// explain about read mode).
	in := strings.NewReader("")
	out := &bytes.Buffer{}
	r := New(Config{
		In: in, Out: out,
		RepoRoot: "/tmp/repo", Branch: "main",
		Language: "en",
	})
	r.handleModeCmd("/mode read")
	got := out.String()
	if !strings.Contains(got, "Switched to read mode") {
		t.Errorf("/mode read should print success line; got: %q", got)
	}
	if strings.Contains(got, "ChangePlan") || strings.Contains(got, "/approve") {
		t.Errorf("/mode read should NOT print write-mode hints; got: %q", got)
	}
}

// TestHandleMode_RejectedWithoutWriteEnabled verifies the L2 gate:
// /mode plan / apply / verify all refuse cleanly when codrax.yaml ::
// write_enabled is false (or unset). Pre-fix path silently accepted
// the mode and the planner failed downstream with a confusing
// analyzer / "context canceled" error that did not name the missing
// yaml gate.
func TestHandleMode_RejectedWithoutWriteEnabled(t *testing.T) {
	for _, m := range []string{"plan", "apply", "verify"} {
		in := strings.NewReader("")
		out := &bytes.Buffer{}
		r := New(Config{
			In: in, Out: out,
			RepoRoot: "/tmp/repo", Branch: "main",
			Language: "en",
			// WriteEnabled left false (default).
		})
		r.handleModeCmd("/mode " + m)
		if r.currentMode == types.PipelineMode(m) {
			t.Errorf("/mode %s must NOT take effect when write_enabled=false; got currentMode=%q", m, r.currentMode)
		}
		if !strings.Contains(out.String(), "write_enabled") {
			t.Errorf("/mode %s rejection must name write_enabled; got: %q", m, out.String())
		}
	}
	// Read mode is always permitted regardless of write_enabled.
	in := strings.NewReader("")
	out := &bytes.Buffer{}
	r := New(Config{
		In: in, Out: out,
		RepoRoot: "/tmp/repo", Branch: "main",
		Language: "en",
	})
	r.handleModeCmd("/mode read")
	if r.currentMode != types.ModeRead {
		t.Errorf("/mode read must succeed without write_enabled; got currentMode=%q", r.currentMode)
	}
}

// TestHandleMode_InvalidRejected verifies unknown modes do not
// silently set currentMode.
func TestHandleMode_InvalidRejected(t *testing.T) {
	r, out := newScriptedREPL(t, nil)
	before := r.currentMode
	r.handleModeCmd("/mode bogus")
	if r.currentMode != before {
		t.Errorf("invalid /mode should not change state; before=%q after=%q", before, r.currentMode)
	}
	if !strings.Contains(strings.ToLower(out.String()), "unknown mode") {
		t.Errorf("expected 'unknown mode' warning, got: %q", out.String())
	}
}

// TestHandleMode_AllValidModes covers read, plan, apply, verify.
func TestHandleMode_AllValidModes(t *testing.T) {
	for _, m := range []string{"read", "plan", "apply", "verify"} {
		r, _ := newScriptedREPL(t, nil)
		r.handleModeCmd("/mode " + m)
		if string(r.currentMode) != m {
			t.Errorf("mode=%s: currentMode = %q, want %q", m, r.currentMode, m)
		}
	}
}

// TestHandlePlan_DisabledWhenNoStore verifies /plan without a
// configured PlanStore prints a disabled message.
func TestHandlePlan_DisabledWhenNoStore(t *testing.T) {
	r, out := newScriptedREPL(t, nil)
	r.handlePlanCmd("/plan show")
	if !strings.Contains(out.String(), "/plan disabled") {
		t.Errorf("expected disabled message, got: %q", out.String())
	}
}

// TestHandlePlan_ShowEmpty verifies /plan show with no pending
// plan prints the guidance message.
func TestHandlePlan_ShowEmpty(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	r, out := newScriptedREPL(t, store)
	r.handlePlanCmd("/plan show")
	if !strings.Contains(out.String(), "No pending plan") {
		t.Errorf("expected 'No pending plan' message, got: %q", out.String())
	}
}

// TestHandlePlan_ShowWithPending verifies /plan show renders a
// saved plan's summary when pendingPlanPath is set.
func TestHandlePlan_ShowWithPending(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	plan := &types.ChangePlan{
		ID:      "plan-test-show",
		Summary: "test plan for /plan show command",
		Status:  "pending_approval",
		Changes: []types.FileChange{
			{Path: "main.go", Kind: "modify", Rationale: "test"},
		},
		TargetPaths: []string{"main.go"},
	}
	path, err := store.SaveForTest(plan)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	r, out := newScriptedREPL(t, store)
	r.pendingPlanPath = path
	r.handlePlanCmd("/plan show")
	rendered := out.String()
	if !strings.Contains(rendered, "plan-test-show") {
		t.Errorf("expected plan ID in output, got: %q", rendered)
	}
	if !strings.Contains(rendered, "main.go") {
		t.Errorf("expected target path in output, got: %q", rendered)
	}
	if !strings.Contains(rendered, "1 file(s)") {
		t.Errorf("expected change count in output, got: %q", rendered)
	}
}

// TestHandlePlan_Clear verifies /plan clear removes the pending
// plan's file AND resets pendingPlanPath.
func TestHandlePlan_Clear(t *testing.T) {
	dir := t.TempDir()
	store := NewPlanStore(dir)
	plan := &types.ChangePlan{
		ID:      "plan-test-clear",
		Summary: "to be cleared",
		Status:  "pending_approval",
	}
	path, err := store.SaveForTest(plan)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	r, out := newScriptedREPL(t, store)
	r.pendingPlanPath = path
	r.handlePlanCmd("/plan clear")
	if r.pendingPlanPath != "" {
		t.Errorf("pendingPlanPath should be empty after clear, got: %q", r.pendingPlanPath)
	}
	if !strings.Contains(out.String(), "pending plan cleared") {
		t.Errorf("expected success message, got: %q", out.String())
	}
	// The file should be gone.
	if _, err := store.Load("plan-test-clear"); err == nil {
		t.Errorf("plan file should be removed after /plan clear")
	}
}

// TestHandlePlan_ClearEmpty verifies /plan clear without a pending
// plan prints a benign "nothing to clear" message.
func TestHandlePlan_ClearEmpty(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	r, out := newScriptedREPL(t, store)
	r.handlePlanCmd("/plan clear")
	if !strings.Contains(out.String(), "No pending plan to clear") {
		t.Errorf("expected 'no pending plan to clear' message, got: %q", out.String())
	}
}

// TestHandlePlan_List verifies /plan list enumerates saved plans
// with newest first AND renders the per-plan Status column.
func TestHandlePlan_List(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	p1 := &types.ChangePlan{ID: "plan-a", Summary: "first", Status: types.PlanStatusPending}
	p2 := &types.ChangePlan{ID: "plan-b", Summary: "second", Status: types.PlanStatusApplied}
	if _, err := store.SaveForTest(p1); err != nil {
		t.Fatalf("Save p1: %v", err)
	}
	if _, err := store.SaveForTest(p2); err != nil {
		t.Fatalf("Save p2: %v", err)
	}

	r, out := newScriptedREPL(t, store)
	r.handlePlanCmd("/plan list")
	rendered := out.String()
	if !strings.Contains(rendered, "plan-a") {
		t.Errorf("expected plan-a in list, got: %q", rendered)
	}
	if !strings.Contains(rendered, "plan-b") {
		t.Errorf("expected plan-b in list, got: %q", rendered)
	}
	if !strings.Contains(rendered, "2 plan(s)") {
		t.Errorf("expected '2 plan(s)' header, got: %q", rendered)
	}
	// Status column must show each plan's state, not just the ID.
	if !strings.Contains(rendered, "status=pending_approval") {
		t.Errorf("expected pending_approval status; got: %q", rendered)
	}
	if !strings.Contains(rendered, "status=applied") {
		t.Errorf("expected applied status; got: %q", rendered)
	}
}

// TestPlanStore_ListCarriesStatus exercises the JSON probe inside
// List: after Save, a subsequent List must surface the plan's
// Status field.
func TestPlanStore_ListCarriesStatus(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	plan := &types.ChangePlan{ID: "plan-probe", Summary: "x", Status: types.PlanStatusVerifyFailed}
	if _, err := store.SaveForTest(plan); err != nil {
		t.Fatalf("Save: %v", err)
	}
	infos, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("want 1 PlanInfo; got %d", len(infos))
	}
	if infos[0].Status != types.PlanStatusVerifyFailed {
		t.Errorf("PlanInfo.Status = %q, want %q", infos[0].Status, types.PlanStatusVerifyFailed)
	}
}

// TestPlanStore_UpdateStatus_PersistsToDisk verifies PlanStore's
// UpdateStatus wrapper round-trips through the on-disk JSON.
func TestPlanStore_UpdateStatus_PersistsToDisk(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	plan := &types.ChangePlan{ID: "plan-upd", Summary: "x", Status: types.PlanStatusPending}
	if _, err := store.SaveForTest(plan); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := store.UpdateStatus("plan-upd", types.PlanStatusRejected, nil); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	reloaded, err := store.Load("plan-upd")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if reloaded.Status != types.PlanStatusRejected {
		t.Errorf("Status = %q, want %q", reloaded.Status, types.PlanStatusRejected)
	}
}

// TestHandlePlan_UnknownSubcommand verifies a typo surfaces an
// error message rather than silently succeeding.
func TestHandlePlan_UnknownSubcommand(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	r, out := newScriptedREPL(t, store)
	r.handlePlanCmd("/plan frobnicate")
	if !strings.Contains(strings.ToLower(out.String()), "unknown /plan subcommand") {
		t.Errorf("expected unknown subcommand warning, got: %q", out.String())
	}
}

// TestPlanStore_RoundTrip is a PlanStore-focused test verifying
// Save → Load returns the same content. Kept in the test file for
// co-location with the /plan command tests since they share fixtures.
func TestPlanStore_RoundTrip(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	original := &types.ChangePlan{
		ID:      "plan-roundtrip-1",
		Summary: "round-trip test",
		Status:  "pending_approval",
		Changes: []types.FileChange{
			{Path: "foo.go", Kind: "modify", NewContent: "hello\n", Rationale: "trivial"},
		},
		TargetPaths: []string{"foo.go"},
	}
	path, err := store.SaveForTest(original)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if filepath.Base(path) != original.ID+".json" {
		t.Errorf("saved path basename = %q, want %q.json", filepath.Base(path), original.ID)
	}
	loaded, err := store.Load(original.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.ID != original.ID {
		t.Errorf("ID: got %q, want %q", loaded.ID, original.ID)
	}
	if loaded.Summary != original.Summary {
		t.Errorf("Summary: got %q, want %q", loaded.Summary, original.Summary)
	}
	if len(loaded.Changes) != 1 {
		t.Errorf("Changes length: got %d, want 1", len(loaded.Changes))
	}
	if len(loaded.Changes) > 0 && loaded.Changes[0].Path != "foo.go" {
		t.Errorf("Changes[0].Path: got %q, want foo.go", loaded.Changes[0].Path)
	}
}
