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
	if !strings.Contains(res.Summary, "Mutable") {
		t.Errorf("summary should name the missing Mutable, got: %s", res.Summary)
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
			name: "empty graph",
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
			got := detectDepsCycle(c.changes)
			if (got != "") != c.wantCyc {
				t.Errorf("detectDepsCycle = %q, wantCyc=%v", got, c.wantCyc)
			}
		})
	}
}
