package tool

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func b1704ProofContext(t *testing.T, purpose string) *types.BusContext {
	t.Helper()
	ctx := newTestBusCtx()
	ctx.RepoRoot = t.TempDir()
	ctx.Mode = types.ModeApply
	ctx.PipelineStage = types.StagePlan
	if err := os.MkdirAll(filepath.Join(ctx.RepoRoot, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ctx.RepoRoot, "src/widget.ts"), []byte("export function widget(): number { return 1; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx.Mutable.SetWriteWorkflowRun(&types.WriteWorkflowRun{
		RunID: "proof-recovery", ActiveBatchID: "proof", Status: types.WriteWorkflowRunInProgress,
		Batches: []types.WriteWorkflowBatch{{ID: "proof", Purpose: purpose,
			Status: types.WriteWorkflowBatchReadyToPlan, ExpectedPaths: []string{"src/widget.ts"}}},
		ProgressLedger: []types.WriteWorkflowProgress{{BatchID: "source", ReasonCode: "verification_proof_followup_requested"}},
	})
	return ctx
}

func b1704Execute(t *testing.T, name string, ctx *types.BusContext, payload map[string]any) types.ToolResult {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), raw...)
	var result types.ToolResult
	if name == "full" {
		result, err = (&EmitChangePlan{}).Execute(ctx, raw)
	} else {
		result, err = (&EmitPlanSkeleton{}).Execute(ctx, raw)
	}
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, before) {
		t.Fatal("the repair must not rewrite submitted JSON")
	}
	return result
}

func b1704Payload(language string) map[string]any {
	code := "import pathlib\nassert True\n"
	if language == "javascript" {
		code = "const target = require('./src/widget.ts'); if (target.widget() !== 1) throw new Error('widget');"
	}
	return map[string]any{
		"request": "close the current proof obligation", "summary": "Verify the already-applied target without changing source bytes.",
		"changes": []any{}, "verification_probes": []any{map[string]any{
			"id": "widget-proof", "language": language, "code": code, "changed_symbol_refs": []string{"src/widget.ts"},
		}},
	}
}

// Public source-free sentinel entrypoints must retain their exact rejection,
// but cannot recommend fields or writes that the same plan shape refuses.
func TestB1704ProofOnlyLanguageRepairStaysOnCurrentPlan(t *testing.T) {
	for _, entry := range []string{"full", "skeleton"} {
		for _, purpose := range []string{"verification_proof_followup", "impact_and_verification_proof_followup"} {
			t.Run(entry+"/"+purpose, func(t *testing.T) {
				ctx := b1704ProofContext(t, purpose)
				workflow, _ := json.Marshal(ctx.Mutable.WriteWorkflowRun())
				source, _ := os.ReadFile(filepath.Join(ctx.RepoRoot, "src/widget.ts"))
				fullSchema, skeletonSchema := (&EmitChangePlan{}).Parameters(), (&EmitPlanSkeleton{}).Parameters()
				res := b1704Execute(t, entry, ctx, b1704Payload("python"))
				if res.Success {
					t.Fatal("a Python probe cannot execute this TypeScript target")
				}
				pack := mustPlanRepairPack(t, res)
				if pack.ReasonCode != "verification_probe_target_language_mismatch" ||
					!reflect.DeepEqual(pack.FailingFieldPaths, []string{"$.verification_probes[].language"}) ||
					!reflect.DeepEqual(pack.FailingPaths, []string{"src/widget.ts"}) {
					t.Fatalf("precise repair identity changed: %+v", pack)
				}
				for _, text := range []string{res.Summary, pack.Message} {
					for _, forbidden := range []string{"inspect or add its exact assertion", "include that test file in the bounded plan", "bind one concrete assertion through project_test_observations[]", "An unchanged test file may be referenced", "Emit one project_test_observations[] row"} {
						if strings.Contains(text, forbidden) {
							t.Errorf("source-free repair recommends an unavailable current-plan operation %q: %s", forbidden, text)
						}
					}
					for _, want := range []string{"source-free", "do not add", "project_test_observations", "already-applied"} {
						if !strings.Contains(text, want) {
							t.Errorf("repair lost scoped instruction %q: %s", want, text)
						}
					}
				}
				// PLAN_REPAIR_PACK has an existing compact message cap; the
				// full tool summary carries the remaining recovery boundary.
				for _, want := range []string{"keep changes: []", "not per-contract assertion proof", "unverified", "controller", "do not omit required probes"} {
					if !strings.Contains(res.Summary, want) {
						t.Errorf("full repair lost non-closure boundary %q", want)
					}
				}
				if ctx.Mutable.ChangePlan() != nil || ctx.Mutable.PartialChangePlan() != nil {
					t.Fatal("rejected repair installed a plan")
				}
				afterWorkflow, _ := json.Marshal(ctx.Mutable.WriteWorkflowRun())
				afterSource, _ := os.ReadFile(filepath.Join(ctx.RepoRoot, "src/widget.ts"))
				if !bytes.Equal(workflow, afterWorkflow) || !bytes.Equal(source, afterSource) ||
					!bytes.Equal(fullSchema, (&EmitChangePlan{}).Parameters()) || !bytes.Equal(skeletonSchema, (&EmitPlanSkeleton{}).Parameters()) {
					t.Fatal("soft repair changed workflow, source, or tool schema")
				}
			})
		}
	}
}

func TestB1704SourceFreePlanQualificationsStayUnchanged(t *testing.T) {
	for _, entry := range []string{"full", "skeleton"} {
		for _, shape := range []string{"observations", "file_edit", "empty", "compatible"} {
			t.Run(entry+"/"+shape, func(t *testing.T) {
				ctx := b1704ProofContext(t, "verification_proof_followup")
				payload := b1704Payload("javascript")
				wantReason := ""
				switch shape {
				case "observations":
					payload["project_test_observations"] = []any{map[string]any{"test_path": "tests/widget.test.ts", "assertion_suite": "widget", "assertion_id": "one", "contract_refs": []string{"c1"}}}
					wantReason = "project_test_observation_without_changes"
				case "file_edit":
					change := map[string]any{"path": "tests/widget.test.ts", "kind": "create", "rationale": "add a test to the proof-only plan"}
					if entry == "full" {
						change["new_content"] = "export const proof = true;\n"
					}
					payload["changes"] = []any{change}
					wantReason = "proof_followup_changes_without_failure"
				case "empty":
					delete(payload, "verification_probes")
					wantReason = "changes_empty"
				}
				res := b1704Execute(t, entry, ctx, payload)
				if wantReason != "" {
					if res.Success || mustPlanRepairPack(t, res).ReasonCode != wantReason {
						t.Fatalf("plan qualification changed; want %s: %+v", wantReason, res)
					}
					return
				}
				plan := ctx.Mutable.ChangePlan()
				if !res.Success || plan == nil || !types.IsPersistedProofProbeOnlyPlan(plan) || len(plan.Changes) != 0 || len(plan.VerificationProbes) != 1 {
					t.Fatalf("compatible proof-only plan was not preserved: result=%+v plan=%+v", res, plan)
				}
			})
		}
	}
}

func TestB1704OrdinarySourceLanguageRecoveryRetainsNativeRoute(t *testing.T) {
	got := validateVerificationProbeTargetLanguageCompatibility(
		[]types.FileChange{{Path: "src/widget.ts", Kind: "modify"}},
		[]types.VerificationProbe{{Language: "python", Code: "assert True"}},
	)
	if !strings.Contains(got, types.NativeProjectTestObservationRecoveryTeaching) || !strings.Contains(got, "An unchanged test file may be referenced by project_test_observations[].test_path without adding it to changes[]") {
		t.Fatalf("ordinary source/test plans lost the legal native assertion route: %s", got)
	}
}

func TestB1704OrdinarySourcePlanPublicRecoveryRetainsNativeRoute(t *testing.T) {
	ctx := b1704ProofContext(t, "implementation")
	payload := b1704Payload("python")
	payload["summary"] = "Update src/widget.ts to return the corrected value and verify its behavior."
	payload["changes"] = []any{map[string]any{
		"path": "src/widget.ts", "kind": "modify", "rationale": "correct the return value",
		"new_content": "export function widget(): number { return 2; }\n",
	}}
	res := b1704Execute(t, "full", ctx, payload)
	if res.Success || mustPlanRepairPack(t, res).ReasonCode != "verification_probe_target_language_mismatch" {
		t.Fatalf("ordinary source plan did not reach the same precise mismatch: %+v", res)
	}
	if !strings.Contains(res.Summary, types.NativeProjectTestObservationRecoveryTeaching) || strings.Contains(res.Summary, "this source-free proof plan") {
		t.Fatalf("ordinary source plan lost its legal native-test repair route: %s", res.Summary)
	}
	if strings.Contains(res.Summary, "include that test file in the bounded plan") ||
		!strings.Contains(res.Summary, "An unchanged test file may be referenced by project_test_observations[].test_path without adding it to changes[]") {
		t.Fatalf("ordinary repair unnecessarily demands a test-file mutation: %s", res.Summary)
	}
	if ctx.Mutable.ChangePlan() != nil {
		t.Fatal("rejected ordinary plan must not be installed")
	}
}

func TestB1704AlwaysOnTeachingScopesNativeTestAuthoring(t *testing.T) {
	text := types.WriteBehaviorContractObservationTeaching
	scope := strings.Index(text, "when source/test changes are authorized")
	edit := strings.Index(text, "Prefer an existing native project-test assertion")
	if scope < 0 || edit < scope {
		t.Fatal("shared always-on teaching must qualify native test authoring before recommending it")
	}
	for _, want := range []string{
		"A source-free proof plan must follow its probe-only instructions, not add files or project_test_observations",
		"earlier declarations remain on their source/test plans",
		"For an authorized condition change",
		"only when its current executor supplies the required assertion witness",
		"does not by itself prove that every independent behavior contract was exercised",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("shared authoring scope or proof qualification lost %q", want)
		}
	}
}
