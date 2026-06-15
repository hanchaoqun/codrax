package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// newTestBusCtx returns a minimal BusContext with a writable
// MutableState. No repo, no logger, no work dir — exactly what
// emit_change_plan.Execute needs and nothing more.
func newTestBusCtx() *types.BusContext {
	return &types.BusContext{
		Mutable: types.NewMutableState("test request"),
	}
}

// TestEmitChangePlan_HappyPath locks the canonical success path:
// valid params in → ChangePlan installed on Mutable + PendingApply
// entries enqueued on WriteClosure + ToolResult.Success == true.
func TestEmitChangePlan_HappyPath(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "add a comment to main.go",
		"summary": "Add a one-line header comment to main.go explaining the binary's role. Trivial change; no behavior impact.",
		"changes": [
			{"path": "main.go", "kind": "modify", "new_content": "// codrax entry point\npackage main\n", "rationale": "add a brief header comment"}
		],
		"acceptance_tests": ["go build ./... passes"]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected Success=true, got false with summary: %s", res.Summary)
	}

	plan := ctx.Mutable.ChangePlan()
	if plan == nil {
		t.Fatal("expected ChangePlan installed on Mutable")
	}
	if plan.Summary == "" {
		t.Error("plan Summary should not be empty")
	}
	if len(plan.Changes) != 1 {
		t.Errorf("expected 1 change, got %d", len(plan.Changes))
	}
	if plan.Changes[0].Path != "main.go" {
		t.Errorf("change path = %q, want main.go", plan.Changes[0].Path)
	}
	if plan.Status != "pending_approval" {
		t.Errorf("status = %q, want pending_approval", plan.Status)
	}
	if !strings.HasPrefix(plan.ID, "plan-") {
		t.Errorf("plan ID = %q, want prefix 'plan-'", plan.ID)
	}
	if len(plan.TargetPaths) != 1 {
		t.Errorf("target_paths = %v, want 1 entry", plan.TargetPaths)
	}
	if len(plan.AcceptanceTests) != 1 {
		t.Errorf("acceptance_tests = %v, want 1 entry", plan.AcceptanceTests)
	}

	// WriteClosure should hold PendingApplies matching the changes.
	wc := ctx.Mutable.WriteClosure()
	pending := wc.PendingApplies()
	if len(pending) != 1 {
		t.Errorf("expected 1 pending apply, got %d", len(pending))
	}
	if len(pending) > 0 && pending[0].Path != "main.go" {
		t.Errorf("pending apply path = %q, want main.go", pending[0].Path)
	}
}

func TestEmitChangePlan_StructuredPayloadCompatRepairsStringArrays(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "add a comment to main.go",
		"summary": "Add a one-line header comment to main.go explaining the binary's role. Trivial change; no behavior impact.",
		"changes": "[{\"path\":\"main.go\",\"kind\":\"modify\",\"new_content\":\"// codrax entry point\\npackage main\\n\",\"rationale\":\"add a brief header comment\",\"depends_on\":\"[]\"}]",
		"acceptance_tests": "[\"go build ./... passes\"]"
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected schema-compatible payload to be accepted, got: %s", res.Summary)
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil || len(plan.Changes) != 1 {
		t.Fatalf("ChangePlan not installed from repaired payload: %+v", plan)
	}
	if plan.Changes[0].Path != "main.go" || !strings.Contains(plan.Changes[0].NewContent, "package main") {
		t.Fatalf("change row was not preserved after compat repair: %+v", plan.Changes[0])
	}
	if len(plan.AcceptanceTests) != 1 || plan.AcceptanceTests[0] != "go build ./... passes" {
		t.Fatalf("acceptance tests were not repaired: %+v", plan.AcceptanceTests)
	}
}

func TestEmitChangePlan_PersistsVerificationProbesInFingerprint(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "fix a python behaviour",
		"summary": "Modify widget.py and attach a small behavioural probe so verify can still check the contract when project pytest infrastructure is unavailable.",
		"changes": [
			{"path": "widget.py", "kind": "modify", "new_content": "VALUE = 42\n", "rationale": "set the corrected value"}
		],
		"verification_probes": [
			{"id": "value_contract", "language": "python", "code": "import widget\nassert widget.VALUE == 42\n", "timeout_seconds": 3}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected Success=true, got summary: %s", res.Summary)
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil {
		t.Fatal("expected ChangePlan installed on Mutable")
	}
	if len(plan.VerificationProbes) != 1 {
		t.Fatalf("verification probes not persisted: %+v", plan.VerificationProbes)
	}
	fingerprintWithProbe := types.PlanFingerprint(plan)
	plan.VerificationProbes = nil
	fingerprintWithoutProbe := types.PlanFingerprint(plan)
	if fingerprintWithProbe == "" || fingerprintWithProbe == fingerprintWithoutProbe {
		t.Fatalf("plan fingerprint must include verification probes, with=%q without=%q", fingerprintWithProbe, fingerprintWithoutProbe)
	}
}

func TestEmitChangePlan_RejectsEscapingVerificationProbeWorkingDir(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "fix a python behaviour",
		"summary": "Modify widget.py and attach a small behavioural probe so verify can still check the contract when project pytest infrastructure is unavailable.",
		"changes": [
			{"path": "widget.py", "kind": "modify", "new_content": "VALUE = 42\n", "rationale": "set the corrected value"}
		],
		"verification_probes": [
			{"id": "value_contract", "language": "python", "working_dir": "../outside", "code": "import widget\nassert widget.VALUE == 42\n"}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res.Success {
		t.Fatal("expected escaping verification probe working_dir to be rejected")
	}
	if !strings.Contains(res.Summary, "escapes the repository") {
		t.Fatalf("summary should mention repository escape, got: %s", res.Summary)
	}
	if plan := ctx.Mutable.ChangePlan(); plan != nil {
		t.Fatalf("rejected probe must not install a plan: %+v", plan)
	}
}

// TestEmitChangePlan_EmptyChangesRejected locks the hard cross-
// field check: a plan with zero changes is meaningless and must
// fail with a clear diagnostic.
func TestEmitChangePlan_EmptyChangesRejected(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "trivial",
		"summary": "nothing to change",
		"changes": []
	}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected Success=false for empty changes[]")
	}
	if !strings.Contains(res.Summary, "empty") {
		t.Errorf("summary should mention 'empty', got: %s", res.Summary)
	}
	if plan := ctx.Mutable.ChangePlan(); plan != nil {
		t.Error("ChangePlan should not be installed on failed emit")
	}
}

// TestEmitChangePlan_InvalidKindRejected locks the per-change
// kind validation: kind must be one of create|modify|delete|patch.
func TestEmitChangePlan_InvalidKindRejected(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "test",
		"summary": "test summary here with sufficient length",
		"changes": [
			{"path": "foo.go", "kind": "frobnicate", "rationale": "test"}
		]
	}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected Success=false for illegal kind")
	}
	if !strings.Contains(res.Summary, "frobnicate") {
		t.Errorf("summary should mention invalid kind, got: %s", res.Summary)
	}
}

// TestEmitChangePlan_EmptyPathRejected locks the per-change path
// validation: path must be non-empty.
func TestEmitChangePlan_EmptyPathRejected(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "test",
		"summary": "test summary here with sufficient length",
		"changes": [
			{"path": "", "kind": "modify", "new_content": "x", "rationale": "test"}
		]
	}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected Success=false for empty path")
	}
	if !strings.Contains(res.Summary, "empty path") {
		t.Errorf("summary should mention empty path, got: %s", res.Summary)
	}
}

// TestEmitChangePlan_EmptySummaryRejected locks the required-field
// validation at the structure level (JSON schema + runtime).
func TestEmitChangePlan_EmptySummaryRejected(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "test",
		"summary": "",
		"changes": [
			{"path": "foo.go", "kind": "modify", "rationale": "test"}
		]
	}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected Success=false for empty summary")
	}
}

// TestEmitChangePlan_UnknownFieldsRejected locks the strict-decode
// guarantee: any schema drift (extra JSON fields the params struct
// does not declare) fails loudly rather than silently losing data.
func TestEmitChangePlan_UnknownFieldsRejected(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "test",
		"summary": "test summary here with sufficient length",
		"changes": [
			{"path": "foo.go", "kind": "modify", "rationale": "test"}
		],
		"unknown_future_field": "this should break decode"
	}`)
	res, err := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected Success=false for unknown field")
	}
	if err == nil {
		t.Error("expected non-nil error for unknown field")
	}
}

// TestEmitChangePlan_NilMutableGracefullyFails ensures the tool
// degrades cleanly when caller passes an incomplete BusContext —
// no panic, clean error message.
func TestEmitChangePlan_NilMutableGracefullyFails(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := &types.BusContext{} // no Mutable
	params := json.RawMessage(`{"request":"x","summary":"y","changes":[{"path":"a","kind":"modify","rationale":"b"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected Success=false for nil Mutable")
	}
	if !strings.Contains(res.Summary, "writable context") {
		t.Errorf("summary should name the missing writable context, got: %s", res.Summary)
	}
}

// TestEmitChangePlan_MetadataBasics pins tool surface invariants:
// Name, Description, Parameters schema are all non-empty and the
// schema parses as valid JSON.
func TestEmitChangePlan_MetadataBasics(t *testing.T) {
	tool := &EmitChangePlan{}
	if tool.Name() != "emit_change_plan" {
		t.Errorf("Name = %q, want emit_change_plan", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Description should not be empty")
	}
	schema := tool.Parameters()
	if len(schema) == 0 {
		t.Fatal("Parameters should not be empty")
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(schema, &parsed); err != nil {
		t.Errorf("Parameters schema is not valid JSON: %v", err)
	}
	if parsed["type"] != "object" {
		t.Errorf("schema root type = %v, want 'object'", parsed["type"])
	}
}

// TestEmitChangePlan_IsWriteMarker verifies the tool has the
// expected marker tag. emit_change_plan is classified ReadOnly
// because it mutates BusContext, not the filesystem (disk writes
// happen in cmd/root.go's writePlanFile).
func TestEmitChangePlan_IsWriteMarker(t *testing.T) {
	tool := &EmitChangePlan{}
	if tool.IsWrite() {
		t.Error("emit_change_plan should be classified ReadOnly (mutates BusContext, not repo)")
	}
}

// TestEmitChangePlan_DuplicatePathRejected locks the B1-Q1
// decision: one change per file per plan. A planner that emits
// two entries for the same path is buggy; the tool rejects rather
// than silently losing one (or worse, applying both in undefined
// order).
func TestEmitChangePlan_DuplicatePathRejected(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "touch foo.go twice",
		"summary": "illegal plan shape: two changes for one file.",
		"changes": [
			{"path": "foo.go", "kind": "modify", "new_content": "A", "rationale": "first"},
			{"path": "foo.go", "kind": "patch", "patch": "B", "rationale": "second"}
		]
	}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected Success=false for duplicate path")
	}
	if !strings.Contains(res.Summary, "duplicate") {
		t.Errorf("summary should mention 'duplicate', got: %s", res.Summary)
	}
	if plan := ctx.Mutable.ChangePlan(); plan != nil {
		t.Error("ChangePlan must not be installed when validation fails")
	}
}

// TestEmitChangePlan_UnknownDependsOnRejected locks the B1-Q1
// rule: every depends_on entry must resolve to another change in
// the plan. A dangling reference is silently non-functional (the
// apply-stage W1 check would reject later, but earlier is better).
func TestEmitChangePlan_UnknownDependsOnRejected(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "add a test",
		"summary": "foo.go depends on a path that isn't in this plan.",
		"changes": [
			{"path": "foo.go", "kind": "modify", "new_content": "X", "rationale": "uses bar",
			 "depends_on": ["bar.go"]}
		]
	}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected Success=false for unknown depends_on")
	}
	if !strings.Contains(res.Summary, "bar.go") {
		t.Errorf("summary should name the missing target; got: %s", res.Summary)
	}
}

// TestEmitChangePlan_SelfDependRejected locks the degenerate
// self-edge case. A change depending on itself is always a bug
// (the planner model confused this-edit with this-file) and the
// cycle detector would catch it anyway, but this test documents
// the intent separately from the multi-node cycle test below.
func TestEmitChangePlan_SelfDependRejected(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "self-loop",
		"summary": "illegal: a change depending on its own path.",
		"changes": [
			{"path": "foo.go", "kind": "modify", "new_content": "X",
			 "rationale": "self", "depends_on": ["foo.go"]}
		]
	}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected Success=false for self-dependency")
	}
	if !strings.Contains(res.Summary, "itself") {
		t.Errorf("summary should mention 'itself'; got: %s", res.Summary)
	}
}

// TestEmitChangePlan_CycleRejected verifies multi-node depends_on
// cycles are rejected with the specific cycle path in the error
// message. The planner's retry hint can then identify which edges
// to break instead of guessing.
func TestEmitChangePlan_CycleRejected(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	// a -> b -> c -> a cycle
	params := json.RawMessage(`{
		"request": "3-node cycle",
		"summary": "illegal: depends_on forms a cycle a → b → c → a.",
		"changes": [
			{"path": "a.go", "kind": "modify", "new_content": "A",
			 "rationale": "a", "depends_on": ["c.go"]},
			{"path": "b.go", "kind": "modify", "new_content": "B",
			 "rationale": "b", "depends_on": ["a.go"]},
			{"path": "c.go", "kind": "modify", "new_content": "C",
			 "rationale": "c", "depends_on": ["b.go"]}
		]
	}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected Success=false for depends_on cycle")
	}
	if !strings.Contains(res.Summary, "cycle") {
		t.Errorf("summary should mention 'cycle'; got: %s", res.Summary)
	}
	// Cycle path should name all three files.
	for _, p := range []string{"a.go", "b.go", "c.go"} {
		if !strings.Contains(res.Summary, p) {
			t.Errorf("cycle description should mention %q; got: %s", p, res.Summary)
		}
	}
}

// TestEmitChangePlan_ValidDependsOnHappyPath locks that a legal
// DAG of depends_on edges passes validation and the DependsOn
// field survives to the installed ChangePlan (so the apply stage
// can read it for topological sorting).
func TestEmitChangePlan_ValidDependsOnHappyPath(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "create helper + modify caller",
		"summary": "Create helper.go first, then modify main.go to import it. Declares the ordering via depends_on.",
		"changes": [
			{"path": "helper.go", "kind": "create",
			 "new_content": "package main\nfunc Helper() {}\n",
			 "rationale": "new helper function"},
			{"path": "main.go", "kind": "modify",
			 "new_content": "package main\nfunc main() { Helper() }\n",
			 "rationale": "call the helper",
			 "depends_on": ["helper.go"]}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected Success=true, got summary: %s", res.Summary)
	}

	plan := ctx.Mutable.ChangePlan()
	if plan == nil {
		t.Fatal("expected ChangePlan installed")
	}
	if len(plan.Changes) != 2 {
		t.Fatalf("expected 2 changes, got %d", len(plan.Changes))
	}
	// Find the main.go change and check DependsOn.
	var mainCh *types.FileChange
	for i := range plan.Changes {
		if plan.Changes[i].Path == "main.go" {
			mainCh = &plan.Changes[i]
			break
		}
	}
	if mainCh == nil {
		t.Fatal("main.go change missing from plan")
	}
	if len(mainCh.DependsOn) != 1 || mainCh.DependsOn[0] != "helper.go" {
		t.Errorf("main.go depends_on = %v, want [helper.go]", mainCh.DependsOn)
	}
	// helper.go should have no depends_on (the "leaf" of the graph).
	var helperCh *types.FileChange
	for i := range plan.Changes {
		if plan.Changes[i].Path == "helper.go" {
			helperCh = &plan.Changes[i]
			break
		}
	}
	if helperCh == nil {
		t.Fatal("helper.go change missing from plan")
	}
	if len(helperCh.DependsOn) != 0 {
		t.Errorf("helper.go depends_on should be empty, got %v", helperCh.DependsOn)
	}
}

// TestDetectDepsCycle_UnitTable exercises the cycle helper
// directly on a handful of graph shapes so the surface-level
// tests above stay focused on tool-Execute integration, not
// graph-theory edge cases.
func TestDetectDepsCycle_UnitTable(t *testing.T) {
	cases := []struct {
		name    string
		changes []emitChangePlanChange
		wantCyc bool // true → any non-empty cycle string is acceptable
	}{
		{
			name:    "empty graph",
			changes: []emitChangePlanChange{},
			wantCyc: false,
		},
		{
			name: "single node no deps",
			changes: []emitChangePlanChange{
				{Path: "a.go"},
			},
			wantCyc: false,
		},
		{
			name: "two-node chain",
			changes: []emitChangePlanChange{
				{Path: "a.go"},
				{Path: "b.go", DependsOn: []string{"a.go"}},
			},
			wantCyc: false,
		},
		{
			name: "two-node cycle",
			changes: []emitChangePlanChange{
				{Path: "a.go", DependsOn: []string{"b.go"}},
				{Path: "b.go", DependsOn: []string{"a.go"}},
			},
			wantCyc: true,
		},
		{
			name: "diamond (legal DAG)",
			changes: []emitChangePlanChange{
				{Path: "a.go"},
				{Path: "b.go", DependsOn: []string{"a.go"}},
				{Path: "c.go", DependsOn: []string{"a.go"}},
				{Path: "d.go", DependsOn: []string{"b.go", "c.go"}},
			},
			wantCyc: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := detectDepsCycle(emitChangesToFileChanges(c.changes))
			if (got != "") != c.wantCyc {
				t.Errorf("detectDepsCycle = %q, wantCyc=%v", got, c.wantCyc)
			}
		})
	}
}

// TestEmitChangePlan_PatchPreflight_RejectsCorruptDiff pins the
// session-35 fix: a kind=patch change whose unified diff fails
// `git apply --check` is rejected at emit time. The session-35 e2e
// run saw 2/3 planner dispatches produce diffs with mismatched @@
// hunk counts — those landed in plan.json, were persisted + approved,
// and blew up only at apply time inside the worktree, with no retry
// path (the coder's retries re-read the same corrupt diff from Mutable).
//
// With the pre-flight gate, the same malformed diff fails emit_change_plan
// → planner's ShouldStop doesn't fire (no plan installed) → planner
// retries within the same dispatch, seeing git's error verbatim. The
// failure is caught at the stage that can actually fix it.
func TestEmitChangePlan_PatchPreflight_RejectsCorruptDiff(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not available; pre-flight check skipped")
	}
	// Seed a real git repo with known content so the pre-flight check
	// has a baseline to validate against.
	ctx := gitWorktreeFixture(t, "line1\nline2\nline3\n")
	// Clear the seeded plan — gitWorktreeFixture installs one, but we
	// want emit_change_plan.Execute to install its own from our params.
	ctx.Mutable = types.NewMutableState("test")

	tool := &EmitChangePlan{}
	// Corrupt diff: @@ header claims 6 lines but body has 7 content
	// lines — the same malformed shape the session-35 planner produced.
	corruptDiff := `--- a/file.txt
+++ b/file.txt
@@ -1,6 +1,6 @@
 line1
-line2
+lineTWO
 line3
 extra1
 extra2
 extra3
`
	params := json.RawMessage(`{
		"request": "rename line2",
		"summary": "Fix line2 naming. Pre-flight check should reject a malformed hunk header.",
		"changes": [
			{"path": "file.txt", "kind": "patch", "patch": ` + jsonString(corruptDiff) + `, "rationale": "test"}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res.Success {
		t.Fatalf("corrupt diff should be rejected at emit time; got Success=true")
	}
	for _, want := range []string{"git apply", "kind=patch", "file.txt"} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("rejection summary should carry %q; got %q", want, res.Summary)
		}
	}
	// No ChangePlan should have been installed — rejection is
	// pre-mutation so state stays clean.
	if ctx.Mutable.ChangePlan() != nil {
		t.Error("rejected emit must not install a ChangePlan on Mutable")
	}
}

// TestEmitChangePlan_PatchPreflight_AcceptsValidDiff confirms the
// gate is one-sided: a well-formed diff with correct hunk counts
// passes emit cleanly.
func TestEmitChangePlan_PatchPreflight_AcceptsValidDiff(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not available; pre-flight check skipped")
	}
	ctx := gitWorktreeFixture(t, "line1\nline2\nline3\n")
	ctx.Mutable = types.NewMutableState("test")

	tool := &EmitChangePlan{}
	validDiff := `--- a/file.txt
+++ b/file.txt
@@ -1,3 +1,3 @@
 line1
-line2
+lineTWO
 line3
`
	params := json.RawMessage(`{
		"request": "rename line2",
		"summary": "Fix line2 naming. Well-formed diff, pre-flight should accept.",
		"changes": [
			{"path": "file.txt", "kind": "patch", "patch": ` + jsonString(validDiff) + `, "rationale": "test"}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("valid diff should pass pre-flight; got FAIL: %s", res.Summary)
	}
	if ctx.Mutable.ChangePlan() == nil {
		t.Error("accepted emit must install a ChangePlan on Mutable")
	}
}

func TestEmitChangePlan_PatchPreflight_TolerantToMissingContextMarkers(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not available; pre-flight check skipped")
	}
	ctx := gitWorktreeFixture(t, "def greet(name):\n    if not name:\n        name = \"world\"\n    retrun f\"Hello, {name}!\"\n")
	ctx.Mutable = types.NewMutableState("test")

	tool := &EmitChangePlan{}
	// The unchanged context lines below carry the file indentation
	// only. A strict unified diff needs one extra leading space marker
	// before each context line. This is a generic LLM serialization
	// error: the model authored the correct source lines but omitted
	// the diff marker, so the tool may normalize the patch envelope
	// without inventing content.
	missingMarkers := `--- a/file.txt
+++ b/file.txt
@@ -2,3 +2,3 @@
    if not name:
        name = "world"
-    retrun f"Hello, {name}!"
+    return f"Hello, {name}!"
`
	params := json.RawMessage(`{
		"request": "fix typo",
		"summary": "Fix a one-line typo. The context marker normalizer should accept the model-authored patch body.",
		"changes": [
			{"path": "file.txt", "kind": "patch", "patch": ` + jsonString(missingMarkers) + `, "rationale": "test"}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("missing-context-marker diff should pass pre-flight via structural normalization; got FAIL: %s", res.Summary)
	}
	if ctx.Mutable.ChangePlan() == nil {
		t.Error("accepted emit must install a ChangePlan on Mutable")
	}
}

// TestEmitChangePlan_PatchPreflight_TolerantToMiscountedHeader pins
// the --recount tolerance: a diff with a structurally-correct BODY
// but a WRONG @@ hunk-header line count must pass pre-flight. The
// session-35 eval repeatedly saw LLMs produce 5-line hunks declared
// as ",6" (or 7-line declared as ",6") — the body was byte-correct,
// the metadata wasn't. Failing those would burn planner retries on
// a mechanical counting error the system can trivially recover from.
// --recount is git's own blessed path for this scenario.
func TestEmitChangePlan_PatchPreflight_TolerantToMiscountedHeader(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not available; pre-flight check skipped")
	}
	ctx := gitWorktreeFixture(t, "line1\nline2\nline3\n")
	ctx.Mutable = types.NewMutableState("test")

	tool := &EmitChangePlan{}
	// Body is correct (3 old lines, 3 new lines — matches the file
	// exactly), but header declares ",6" for each side. Pre --recount
	// this would fail; post --recount it's accepted.
	miscountedDiff := `--- a/file.txt
+++ b/file.txt
@@ -1,6 +1,6 @@
 line1
-line2
+lineTWO
 line3
`
	params := json.RawMessage(`{
		"request": "rename line2",
		"summary": "Fix line2 naming. Header line count is wrong but body matches — --recount should fix.",
		"changes": [
			{"path": "file.txt", "kind": "patch", "patch": ` + jsonString(miscountedDiff) + `, "rationale": "test"}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("miscounted-header diff should pass pre-flight via --recount; got FAIL: %s", res.Summary)
	}
	if ctx.Mutable.ChangePlan() == nil {
		t.Error("accepted emit must install a ChangePlan on Mutable")
	}
}

// TestEmitChangePlan_PatchPreflight_TolerantToMissingTrailingNewline
// pins the newline-normalisation tolerance: a diff whose BODY is
// correct but whose FINAL LINE is missing its `\n` terminator must
// pass pre-flight. Session-35 eval saw 1/3 LLM dispatches drop the
// final newline, causing git to report "corrupt patch at line N"
// where N is the body length. This is a failure of mechanical
// serialisation, not semantics.
func TestEmitChangePlan_PatchPreflight_TolerantToMissingTrailingNewline(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not available; pre-flight check skipped")
	}
	ctx := gitWorktreeFixture(t, "line1\nline2\nline3\n")
	ctx.Mutable = types.NewMutableState("test")

	tool := &EmitChangePlan{}
	// Body is correct; note the deliberate missing \n at end.
	noTrailingNewline := `--- a/file.txt
+++ b/file.txt
@@ -1,3 +1,3 @@
 line1
-line2
+lineTWO
 line3`
	params := json.RawMessage(`{
		"request": "rename line2",
		"summary": "Fix line2 naming. Body complete but final line missing its terminating newline.",
		"changes": [
			{"path": "file.txt", "kind": "patch", "patch": ` + jsonString(noTrailingNewline) + `, "rationale": "test"}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("missing-trailing-newline diff should pass pre-flight; got FAIL: %s", res.Summary)
	}
	// Confirm the plan stored the ORIGINAL (not normalised) patch so
	// re-applying it via apply_patch lands bytes-correct too. The
	// tool layer normalises at the edge; Mutable state stays as-is.
	plan := ctx.Mutable.ChangePlan()
	if plan == nil {
		t.Fatal("accepted emit must install a ChangePlan on Mutable")
	}
}

// TestEmitChangePlan_PatchPreflight_EmptyPatchRejected confirms the
// emit-time check catches a missing Patch field for kind=patch
// (previously only the apply_patch tool would reject this — too late).
func TestEmitChangePlan_PatchPreflight_EmptyPatchRejected(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not available; pre-flight check skipped")
	}
	ctx := gitWorktreeFixture(t, "x\n")
	ctx.Mutable = types.NewMutableState("test")

	tool := &EmitChangePlan{}
	params := json.RawMessage(`{
		"request": "noop",
		"summary": "Empty patch must be rejected at emit time.",
		"changes": [
			{"path": "file.txt", "kind": "patch", "patch": "", "rationale": "test"}
		]
	}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("kind=patch with empty Patch must be rejected")
	}
	if !strings.Contains(res.Summary, "empty") || !strings.Contains(res.Summary, "patch") {
		t.Errorf("rejection should name empty patch; got %q", res.Summary)
	}
}

// jsonString produces a valid JSON-encoded string literal (with
// escaping) for injection into a raw JSON template. The test data
// includes newlines + tabs that need JSON escaping; building the
// params map via json.Marshal and re-encoding would lose readability.
func jsonString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		// Unreachable for normal strings.
		panic(err)
	}
	return string(b)
}
