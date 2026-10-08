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

func nativeRegistrationSelectionFixture(t *testing.T, nested bool) (*types.BusContext, *types.ChangePlan, types.VerificationDeliverySnapshot, *types.ChangeReport, string, types.NativeTestIdentityChoice) {
	t.Helper()
	path, body := "test_widget.py", nativeRegistrationTestBody
	var extra map[string]string
	if nested {
		path = "packages/widget/test_widget.py"
		body = "import os, sys\nsys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), '../..')))\n" + body
		extra = map[string]string{"packages/widget/setup.py": "from setuptools import setup\nsetup(name='widget')\n"}
	}
	ctx, source, delivery := nativeRegistrationPublicFixtureWithFiles(t, body, path, extra)
	ctx.PipelineStage = types.StageVerify
	report := existingTestDeliveryPublicRun(t, ctx)
	ctx.PipelineStage = types.StagePlan
	grant := ctx.Mutable.NativeTestRegistrationAuthorization()
	if grant == nil {
		t.Fatal("authorization lost")
	}
	ctx.Mutable.InstallNativeTestRegistrationIdentity(grant.ID, report)
	for _, choice := range ctx.Mutable.NativeTestRegistrationIdentityChoices(ctx.RepoRoot) {
		if (!nested && choice.AssertionID == "test_value") || (nested && choice.AssertionID == "python/unittest@packages/widget::test_value") {
			return ctx, source, delivery, report, path, choice
		}
	}
	t.Fatalf("real native result identity unavailable: report=%+v snapshot=%s", report, ctx.Mutable.NativeTestRegistrationIdentitySnapshot(ctx.RepoRoot))
	return nil, nil, types.VerificationDeliverySnapshot{}, nil, "", types.NativeTestIdentityChoice{}
}

func nativeRegistrationSelectionPayload(path string, choice types.NativeTestIdentityChoice) map[string]any {
	payload := nativeRegistrationPublicPayload()
	payload["project_test_observations"] = []map[string]any{{"id": "selected-existing", "test_path": path, "assertion_ref": choice.Ref, "contract_refs": []string{"increment-result"}}}
	return payload
}

func TestNativeRegistrationSelectionPublicFreshExactIdentity(t *testing.T) {
	for _, entry := range []string{"full", "skeleton"} {
		for _, nested := range []bool{false, true} {
			t.Run(entry+map[bool]string{false: "/root", true: "/nested"}[nested], func(t *testing.T) {
				ctx, source, delivery, old, path, choice := nativeRegistrationSelectionFixture(t, nested)
				before, _ := json.Marshal([]any{source, old})
				// An identity choice by itself is not a current read receipt.
				payload := nativeRegistrationSelectionPayload(path, choice)
				if !nested {
					row := payload["project_test_observations"].([]map[string]any)[0]
					row["assertion_suite"], row["assertion_id"] = choice.AssertionSuite, choice.AssertionID
				}
				if res := nativeRegistrationPublicEmit(t, ctx, entry, payload); res.Success || !strings.Contains(res.Summary, "fully delivered") {
					t.Fatalf("reference authorized a read: %+v", res)
				}
				nativeRegistrationPublicReadPath(t, ctx, path, true, 100)
				if res := nativeRegistrationPublicEmit(t, ctx, entry, payload); !res.Success {
					t.Fatal(res.Summary)
				}
				plan := ctx.Mutable.ChangePlan()
				row := plan.ProjectTestObservations[0]
				if row.AssertionSuite != choice.AssertionSuite || row.AssertionID != choice.AssertionID || row.TestPath != path || len(plan.Changes) != 0 {
					t.Fatalf("selection changed declaration: %+v", row)
				}
				encoded, _ := json.Marshal(plan)
				if bytes.Contains(encoded, []byte("assertion_ref")) || bytes.Contains(encoded, []byte(choice.Ref)) || len(ctx.Mutable.NativeTestRegistrationIdentityChoices(ctx.RepoRoot)) != 0 {
					t.Fatal("dispatch selector survived registration")
				}
				if len(types.CoveredWriteBehaviorContractIDs(plan.BehaviorContracts, types.EffectiveVerificationConfidence(plan, old))) != 0 {
					t.Fatal("old report closed selected contract")
				}
				nativeRegistrationPublicAuthorizeExecution(t, ctx, delivery, source.BehaviorContracts)
				fresh := existingTestDeliveryPublicRun(t, ctx)
				if len(fresh.ExistingTestExecutions) != 1 || len(types.CoveredWriteBehaviorContractIDs(plan.BehaviorContracts, fresh.VerificationConfidence)) != 1 {
					t.Fatalf("new execution did not bind exact identity: %+v", fresh)
				}
				for _, current := range fresh.ExecutedCommands {
					for _, historical := range old.ExecutedCommands {
						if current.InvocationID != "" && current.InvocationID == historical.InvocationID {
							t.Fatal("execution reused historical invocation")
						}
					}
				}
				after, _ := json.Marshal([]any{source, old})
				if !bytes.Equal(before, after) || strings.TrimSpace(b1575FixtureGit(t, ctx.RepoRoot, "diff", "HEAD", "--")) != "" {
					t.Fatal("selection rewrote source, tests, or historical evidence")
				}
			})
		}
	}
}

func TestNativeRegistrationSelectionPublicRejectsInvalidReferences(t *testing.T) {
	for _, entry := range []string{"full", "skeleton"} {
		for _, condition := range []string{"unknown", "empty", "null", "suffix", "partial_pair", "conflict", "revoked", "new_grant", "foreign_root", "bytes_changed", "unread", "ordinary_change", "unknown_contract"} {
			t.Run(entry+"/"+condition, func(t *testing.T) {
				ctx, source, delivery, _, path, choice := nativeRegistrationSelectionFixture(t, false)
				nativeRegistrationPublicReadPath(t, ctx, path, condition != "unread", 100)
				payload := nativeRegistrationSelectionPayload(path, choice)
				row := payload["project_test_observations"].([]map[string]any)[0]
				switch condition {
				case "unknown":
					row["assertion_ref"] = "invented"
				case "empty":
					row["assertion_ref"] = ""
				case "null":
					row["assertion_ref"] = nil
				case "suffix":
					row["assertion_ref"] = strings.TrimPrefix(choice.Ref, "assertion_")
				case "partial_pair":
					row["assertion_suite"] = choice.AssertionSuite
				case "conflict":
					row["assertion_suite"], row["assertion_id"] = choice.AssertionSuite, "another_assertion"
				case "revoked":
					ctx.Mutable.RevokeNativeTestRegistrationAuthorization()
				case "new_grant":
					if err := ctx.Mutable.AuthorizeNativeTestRegistration(ctx.RepoRoot, delivery, source.BehaviorContracts, source.TargetPaths); err != nil {
						t.Fatal(err)
					}
				case "foreign_root":
					ctx.RepoRoot = t.TempDir()
				case "bytes_changed":
					if err := os.WriteFile(filepath.Join(ctx.RepoRoot, path), []byte(nativeRegistrationTestBody+"# changed\n"), 0600); err != nil {
						t.Fatal(err)
					}
				case "ordinary_change":
					payload["changes"] = []map[string]any{{"path": "widget.py", "kind": "modify", "rationale": "ordinary change", "new_content": "def increment(value):\n    return value + 1\n"}}
					if entry == "skeleton" {
						delete(payload["changes"].([]map[string]any)[0], "new_content")
					}
				case "unknown_contract":
					row["contract_refs"] = []string{"invented"}
				}
				res := nativeRegistrationPublicEmit(t, ctx, entry, payload)
				if res.Success || (ctx.Mutable.ChangePlan() != nil && types.IsPersistedNativeTestRegistrationPlan(ctx.Mutable.ChangePlan())) {
					t.Fatalf("invalid reference acquired registration: %+v", res)
				}
			})
		}
	}
}

func TestNativeRegistrationSelectionCannotChooseTestPath(t *testing.T) {
	ctx, source, delivery, _, _, choice := nativeRegistrationSelectionFixture(t, true)
	const other = "test_anchor.py"
	nativeRegistrationPublicReadPath(t, ctx, other, true, 100)
	if res := nativeRegistrationPublicEmit(t, ctx, "full", nativeRegistrationSelectionPayload(other, choice)); !res.Success {
		t.Fatal(res.Summary)
	}
	plan := ctx.Mutable.ChangePlan()
	if plan.ProjectTestObservations[0].TestPath != other {
		t.Fatal("reference inferred a different test path")
	}
	nativeRegistrationPublicAuthorizeExecution(t, ctx, delivery, source.BehaviorContracts)
	report := existingTestDeliveryPublicRun(t, ctx)
	if len(types.CoveredWriteBehaviorContractIDs(plan.BehaviorContracts, report.VerificationConfidence)) != 0 {
		t.Fatal("identity from another file supplied a behavior witness")
	}
}
