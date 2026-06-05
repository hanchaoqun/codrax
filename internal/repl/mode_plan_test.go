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
// the current user mode (auto by default).
func TestHandleMode_ShowDefault(t *testing.T) {
	r, out := newScriptedREPL(t, nil)
	r.handleModeCmd("/mode")
	got := out.String()
	if !strings.Contains(got, "current mode: auto") {
		t.Errorf("expected 'current mode: auto', got: %q", got)
	}
}

// TestHandleMode_SetWrite verifies /mode write sets currentMode
// correctly AND the next dispatch would propagate it via SetMode.
func TestHandleMode_SetWrite(t *testing.T) {
	r, out := newScriptedREPL(t, nil)
	r.handleModeCmd("/mode write")
	if r.userMode != UserModeWrite {
		t.Errorf("userMode = %q, want %q", r.userMode, UserModeWrite)
	}
	if r.currentMode != types.ModePlan {
		t.Errorf("currentMode = %q, want %q", r.currentMode, types.ModePlan)
	}
	if !strings.Contains(out.String(), "Switched to write mode") {
		t.Errorf("expected success message, got: %q", out.String())
	}
}

// TestHandleMode_PrintsWorkflowHint locks the UX contract that every
// explicit user mode transition prints a short workflow hint so a user new to
// the escape hatches knows what the mode changes.
// mode does not have to read the docs.
func TestHandleMode_PrintsWorkflowHint(t *testing.T) {
	cases := []struct {
		mode     string
		mustHave []string
	}{
		{"auto", []string{"structured routing"}},
		{"code", []string{"code/source analysis"}},
		{"operation", []string{"computer-operation pipeline"}},
		{"data", []string{"data-processing pipeline"}},
		{"write", []string{"/approve", "/reject", "/mode auto"}},
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
	// Auto mode prints the success line and does not include write-only
	// commands other than in the write-mode case above.
	in := strings.NewReader("")
	out := &bytes.Buffer{}
	r := New(Config{
		In: in, Out: out,
		RepoRoot: "/tmp/repo", Branch: "main",
		Language: "en",
	})
	r.handleModeCmd("/mode auto")
	got := out.String()
	if !strings.Contains(got, "Switched to auto mode") {
		t.Errorf("/mode auto should print success line; got: %q", got)
	}
	if strings.Contains(got, "ChangePlan") || strings.Contains(got, "/approve") {
		t.Errorf("/mode auto should NOT print write-mode hints; got: %q", got)
	}
}

// TestHandleMode_RejectedWithoutWriteEnabled verifies the L2 gate:
// /mode write refuses cleanly when codrax.yaml ::
// write_enabled is false (or unset). Pre-fix path silently accepted
// the mode and the planner failed downstream with a confusing
// analyzer / "context canceled" error that did not name the missing
// yaml gate.
func TestHandleMode_RejectedWithoutWriteEnabled(t *testing.T) {
	in := strings.NewReader("")
	out := &bytes.Buffer{}
	r := New(Config{
		In: in, Out: out,
		RepoRoot: "/tmp/repo", Branch: "main",
		Language: "en",
		// WriteEnabled left false (default).
	})
	r.handleModeCmd("/mode write")
	if r.userMode == UserModeWrite || r.currentMode == types.ModePlan {
		t.Errorf("/mode write must NOT take effect when write_enabled=false; got userMode=%q currentMode=%q", r.userMode, r.currentMode)
	}
	if !strings.Contains(out.String(), "write_enabled") {
		t.Errorf("/mode write rejection must name write_enabled; got: %q", out.String())
	}
	// Non-write explicit modes are always permitted regardless of write_enabled.
	for _, m := range []string{"auto", "code", "operation", "data"} {
		in := strings.NewReader("")
		out := &bytes.Buffer{}
		r := New(Config{
			In: in, Out: out,
			RepoRoot: "/tmp/repo", Branch: "main",
			Language: "en",
		})
		r.handleModeCmd("/mode " + m)
		if r.userMode != UserMode(m) {
			t.Errorf("/mode %s should succeed without write_enabled; got userMode=%q output=%q", m, r.userMode, out.String())
		}
	}
}

// TestHandleMode_AllValidModes covers all user-facing modes.
func TestHandleMode_AllValidModes(t *testing.T) {
	cases := []struct {
		mode             string
		wantUserMode     UserMode
		wantPipelineMode types.PipelineMode
	}{
		{"auto", UserModeAuto, types.ModeRead},
		{"code", UserModeCode, types.ModeRead},
		{"operation", UserModeOperation, types.ModeRead},
		{"data", UserModeData, types.ModeRead},
		{"write", UserModeWrite, types.ModePlan},
	}
	for _, tc := range cases {
		r, _ := newScriptedREPL(t, nil)
		r.handleModeCmd("/mode " + tc.mode)
		if r.userMode != tc.wantUserMode || r.currentMode != tc.wantPipelineMode {
			t.Errorf("mode=%s: userMode=%q currentMode=%q, want %q/%q",
				tc.mode, r.userMode, r.currentMode, tc.wantUserMode, tc.wantPipelineMode)
		}
	}
}

func TestHandleMode_LegacyWritePhasesAreRejectedAsUserModes(t *testing.T) {
	for _, m := range []string{"read", "plan", "apply", "verify"} {
		in := strings.NewReader("")
		out := &bytes.Buffer{}
		r := New(Config{
			In: in, Out: out,
			RepoRoot: "/tmp/repo", Branch: "main",
			Language: "en",
		})
		r.handleModeCmd("/mode " + m)
		if !strings.Contains(strings.ToLower(out.String()), "unknown mode") {
			t.Errorf("/mode %s should be rejected as a user mode; got %q", m, out.String())
		}
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
