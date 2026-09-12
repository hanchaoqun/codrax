package tool

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func b1665AssertScopeAwarePythonTeaching(t *testing.T, text string) {
	t.Helper()
	if !strings.Contains(text, types.StructuredEditPythonScopeTeaching) {
		t.Errorf("actual consumer does not carry the shared Python teaching: %s", text)
	}
	for _, want := range []string{"line-range replace", "start_line", "end_line", "kind=patch", "scope=micro", "explicitly package, cross, or project", "rewrites most of the existing file"} {
		if !strings.Contains(text, want) {
			t.Errorf("Python edit guidance missing %q: %s", want, text)
		}
	}
	for _, bad := range []string{"or full modify when the edit spans an indented block", "or kind=modify with the full corrected file body"} {
		if strings.Contains(text, bad) {
			t.Errorf("Python indentation still recommends unconditional overwrite: %s", text)
		}
	}
}

func b1665AssertRepairTeachingSurvivesJSON(t *testing.T, pack types.PlanRepairPack) {
	t.Helper()
	if len(types.StructuredEditPythonScopeTeaching) > 480 {
		t.Fatal("shared teaching exceeds the existing repair budget")
	}
	normalized := types.NormalizePlanRepairPack(pack)
	roundTrip, ok := types.PlanRepairPackFromJSON(types.PlanRepairPackJSON(normalized))
	if !ok || roundTrip.RetryInstruction != types.StructuredEditPythonScopeTeaching {
		t.Fatalf("normalized repair JSON lost scope/EOF guidance: %+v", roundTrip)
	}
}

func TestB1665PublicPlanSchemasKeepPythonBlockPatchExit(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  json.RawMessage
		path []string
	}{
		{"single", (&EmitChangePlan{}).Parameters(), []string{"properties", "changes", "items", "properties", "edits", "items", "properties", "content", "description"}},
		{"staged", (&EmitPlanChange{}).Parameters(), []string{"properties", "edits", "items", "properties", "content", "description"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var value any
			if err := json.Unmarshal(tc.raw, &value); err != nil {
				t.Fatal(err)
			}
			for _, key := range tc.path {
				obj, ok := value.(map[string]any)
				if !ok {
					t.Fatalf("missing native schema object before %q", key)
				}
				value = obj[key]
			}
			text, ok := value.(string)
			if !ok {
				t.Fatal("missing content description")
			}
			b1665AssertScopeAwarePythonTeaching(t, text)
		})
	}
}

const b1665PythonBefore = "def calc(value):\n    scaled = value\n    return scaled\n"
const b1665PythonBlock = "    if value is None:\n        return 0\n    return value + 1\n"

func b1665PythonContext(t *testing.T, scope types.WriteScope) *types.BusContext {
	t.Helper()
	ctx := busCtxWithScope(scope)
	ctx.RepoRoot = t.TempDir()
	writeSurfaceFile(t, ctx.RepoRoot, "calc.py", b1665PythonBefore)
	return ctx
}

func b1665PlanParams(t *testing.T, change types.FileChange) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"request": "Correct calc for an absent value and increment ordinary values.",
		"summary": "Update the existing calc function while preserving the surrounding source.",
		"changes": []types.FileChange{change},
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func b1665BlockReplacement() types.FileChange {
	return types.FileChange{Path: "calc.py", Kind: "patch", Rationale: "replace the indented body with its corrected source", Edits: []types.StructuredEdit{{
		Kind: "replace", StartLine: 2, EndLine: 3, OldText: "    scaled = value\n    return scaled\n", Content: b1665PythonBlock,
	}}}
}

func TestB1665PublicPythonEOFRepairOffersExecutableMicroPatch(t *testing.T) {
	ctx := b1665PythonContext(t, types.ScopeMicro)
	bad := types.FileChange{Path: "calc.py", Kind: "patch", Rationale: "add inside the function", Edits: []types.StructuredEdit{{Kind: "insert_at_eof", Content: "    return 2\n"}}}
	result, err := (&EmitChangePlan{}).Execute(ctx, b1665PlanParams(t, bad))
	if err != nil || result.Success {
		t.Fatalf("expected unchanged structured EOF rejection, got %+v err=%v", result, err)
	}
	pack := mustPlanRepairPack(t, result)
	if pack.ReasonCode != "python_indented_eof_insert" || ctx.Mutable.ChangePlan() != nil {
		t.Fatalf("wrong repair lane or rejected plan installed: %+v", pack)
	}
	b1665AssertScopeAwarePythonTeaching(t, pack.RetryInstruction)
	b1665AssertRepairTeachingSurvivesJSON(t, pack)
	// Follow the offered line-range path through the same public emitter.
	good := b1665BlockReplacement()
	accepted, err := (&EmitChangePlan{}).Execute(ctx, b1665PlanParams(t, good))
	if err != nil || !accepted.Success {
		t.Fatalf("micro Python block replacement must remain executable: %+v err=%v", accepted, err)
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil || len(plan.Changes) != 1 || len(plan.Changes[0].Edits) != 1 || plan.Changes[0].Edits[0].Content != b1665PythonBlock || !strings.Contains(plan.Changes[0].Patch, "+    if value is None:") {
		t.Fatalf("accepted model patch was changed: %+v", plan)
	}
	data, err := os.ReadFile(filepath.Join(ctx.RepoRoot, "calc.py"))
	if err != nil || string(data) != b1665PythonBefore {
		t.Fatalf("plan emitter must not apply source edits: %q err=%v", data, err)
	}
}

func TestB1665PublicStagedPythonRepairKeepsFilledSlotAndScope(t *testing.T) {
	ctx := b1665PythonContext(t, types.ScopeMicro)
	installSkeletonForTest(t, ctx, `{
		"request":"Correct calc and add a supporting constant.",
		"summary":"Replace the existing function body with a local patch and retain the independently filled new-file slot.",
		"changes":[
			{"path":"support.py","kind":"create","rationale":"supporting constant"},
			{"path":"calc.py","kind":"patch","rationale":"correct the indented body"}
		]
	}`)
	const filledBody = "MARKER = 1\n"
	filled, err := (&EmitPlanChange{}).Execute(ctx, json.RawMessage(`{"path":"support.py","new_content":"MARKER = 1\n"}`))
	if err != nil || !filled.Success || ctx.Mutable.ChangePlan() != nil {
		t.Fatalf("first real staged slot must stay partial: %+v err=%v", filled, err)
	}
	bad := json.RawMessage(`{"path":"calc.py","edits":[{"kind":"insert_at_eof","content":"    return 2\n"}]}`)
	rejected, err := (&EmitPlanChange{}).Execute(ctx, bad)
	if err != nil || rejected.Success {
		t.Fatalf("expected unchanged staged EOF rejection: %+v err=%v", rejected, err)
	}
	pack := mustPlanRepairPack(t, rejected)
	if pack.ReasonCode != "python_indented_eof_insert" || !pack.PartialPlanRetained || ctx.Mutable.ChangePlan() != nil {
		t.Fatalf("staged repair must retain its partial plan: %+v", pack)
	}
	b1665AssertScopeAwarePythonTeaching(t, pack.RetryInstruction)
	b1665AssertRepairTeachingSurvivesJSON(t, pack)
	partial := ctx.Mutable.PartialChangePlan()
	if partial == nil || len(partial.Changes) != 2 || partial.Changes[0].NewContent != filledBody {
		t.Fatalf("unrelated filled slot was lost: %+v", partial)
	}
	good, err := json.Marshal(map[string]any{"path": "calc.py", "edits": b1665BlockReplacement().Edits})
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := (&EmitPlanChange{}).Execute(ctx, good)
	if err != nil || !accepted.Success {
		t.Fatalf("same-slot line-range replacement must finalize: %+v err=%v", accepted, err)
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil || ctx.Mutable.PartialChangePlan() != nil || len(plan.Changes) != 2 || plan.Changes[0].NewContent != filledBody || plan.Changes[1].Kind != "patch" || len(plan.Changes[1].Edits) != 1 || plan.Changes[1].Edits[0].Content != b1665PythonBlock {
		t.Fatalf("staged model content/kinds were changed: %+v", plan)
	}
	data, err := os.ReadFile(filepath.Join(ctx.RepoRoot, "calc.py"))
	if err != nil || string(data) != b1665PythonBefore {
		t.Fatalf("staged plan changed existing source: %q err=%v", data, err)
	}
	if _, err := os.Stat(filepath.Join(ctx.RepoRoot, "support.py")); !os.IsNotExist(err) {
		t.Fatalf("staged plan unexpectedly applied the create: %v", err)
	}
}

func TestB1665PublicPlanScopeKindAuthorityRemainsUnchanged(t *testing.T) {
	for _, tc := range []struct {
		name   string
		scope  types.WriteScope
		change types.FileChange
		accept bool
	}{
		{"micro block patch", types.ScopeMicro, b1665BlockReplacement(), true},
		{"micro overwrite rejected", types.ScopeMicro, types.FileChange{Path: "calc.py", Kind: "modify", NewContent: "def calc(value):\n" + b1665PythonBlock}, false},
		{"package overwrite retained", types.ScopePackage, types.FileChange{Path: "calc.py", Kind: "modify", NewContent: "def calc(value):\n" + b1665PythonBlock}, true},
		{"unknown scope not newly gated", "", types.FileChange{Path: "calc.py", Kind: "modify", NewContent: "def calc(value):\n" + b1665PythonBlock}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := b1665PythonContext(t, tc.scope)
			raw := b1665PlanParams(t, tc.change)
			before := append([]byte(nil), raw...)
			result, err := (&EmitChangePlan{}).Execute(ctx, raw)
			if err != nil || result.Success != tc.accept {
				t.Fatalf("existing scope authority changed: %+v err=%v", result, err)
			}
			if !bytes.Equal(raw, before) {
				t.Fatal("model parameters were mutated")
			}
			if !tc.accept {
				if pack := mustPlanRepairPack(t, result); pack.ReasonCode != "scope_kind_alignment_failed" {
					t.Fatalf("wrong micro overwrite rejection: %+v", pack)
				}
				if ctx.Mutable.ChangePlan() != nil {
					t.Fatal("rejected overwrite installed a plan")
				}
			} else if plan := ctx.Mutable.ChangePlan(); plan == nil || len(plan.Changes) != 1 || plan.Changes[0].Kind != tc.change.Kind || plan.Changes[0].NewContent != tc.change.NewContent {
				t.Fatalf("accepted change bytes/kind not preserved: %+v", plan)
			}
			data, readErr := os.ReadFile(filepath.Join(ctx.RepoRoot, "calc.py"))
			if readErr != nil || string(data) != b1665PythonBefore {
				t.Fatalf("emission changed source bytes: %q err=%v", data, readErr)
			}
		})
	}
}
