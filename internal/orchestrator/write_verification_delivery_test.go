package orchestrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Inspect the durable public plan wire so this regression compiles before the
// new controller-owned carrier exists. No new delivery helper fabricates it.
type verificationDeliveryWire struct {
	Cumulative *struct {
		AppliedSources []struct {
			SourcePlanID     string                   `json:"source_plan_id"`
			AppliedCommitSHA string                   `json:"applied_commit_sha"`
			PatchEffect      *types.PatchEffectRecord `json:"patch_effect"`
		} `json:"applied_sources"`
	} `json:"cumulative_verification_scope"`
}

func verificationDeliverySource() *types.ChangePlan {
	sha := strings.Repeat("a", 40)
	hunk := types.PatchEffectHunk{AddedLines: 1, AddedLineNumbers: []int{1},
		AddedLineTexts: []types.PatchEffectLine{{Line: 1, Text: "value = 42"}}}
	return &types.ChangePlan{ID: "applied-source", Status: types.PlanStatusVerifyFailed,
		AppliedCommitSHA: sha, AppliedPaths: []string{"src/value.py"},
		TargetPaths: []string{"src/value.py"}, Changes: []types.FileChange{{Path: "src/value.py", Kind: "patch"}},
		PatchEffect: &types.PatchEffectRecord{RecordID: "effect-source", PlanID: "applied-source", Source: "applied_commit",
			BaseRef: strings.Repeat("b", 40), HeadRef: sha, DiffFingerprint: strings.Repeat("c", 64), DiffBytes: 90,
			Files: []types.PatchEffectFile{{Path: "src/value.py", AddedLines: 1, Hunks: []types.PatchEffectHunk{hunk}}}}}
}

func verificationDeliveryProof(id string) *types.ChangePlan {
	return &types.ChangePlan{ID: id, Status: types.PlanStatusNoChangeRequired,
		PersistenceKind: types.PlanPersistenceProofProbeOnly, TargetPaths: []string{"src/value.py"},
		VerificationProbes: []types.VerificationProbe{{ID: "probe", Language: "python", Code: "import value; assert value.value == 42"}}}
}

func verificationDeliveryRun() *types.WriteWorkflowRun {
	return &types.WriteWorkflowRun{RunID: "delivery-run", Batches: []types.WriteWorkflowBatch{{ID: "source-batch",
		Attempts: []types.WriteWorkflowAttempt{{Kind: "apply", Status: "applied", PlanID: "applied-source", FinishedAt: time.Unix(100, 0)}}}}}
}

func readVerificationDeliveryWire(t *testing.T, plan *types.ChangePlan) verificationDeliveryWire {
	t.Helper()
	path := filepath.Join(t.TempDir(), "plan.json")
	if err := types.WritePlanToFile(plan, path); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var wire verificationDeliveryWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if _, err := types.LoadChangePlanFromFile(path); err != nil {
		t.Fatalf("proof plan must remain loadable: %v", err)
	}
	return wire
}

func TestVerificationDeliveryHandoff_PublicDurablePlan(t *testing.T) {
	source := verificationDeliverySource()
	plan := verificationDeliveryProof("proof-current")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: types.NewMutableState("verify"), WorkDir: t.TempDir()}}
	o.stampCumulativeVerificationScope(plan, verificationDeliveryRun(), source)
	wire := readVerificationDeliveryWire(t, plan)
	if wire.Cumulative == nil || len(wire.Cumulative.AppliedSources) != 1 {
		t.Fatalf("durable proof plan lost applied source identity: %+v", wire.Cumulative)
	}
	got := wire.Cumulative.AppliedSources[0]
	if got.SourcePlanID != source.ID || got.AppliedCommitSHA != source.AppliedCommitSHA || got.PatchEffect == nil || got.PatchEffect.PlanID != source.ID || got.PatchEffect.RecordID != source.PatchEffect.RecordID {
		t.Fatalf("source identity was relabelled or incomplete: %+v", got)
	}
	if plan.AppliedCommitSHA != "" || plan.PatchEffect != nil || len(plan.Changes) != 0 {
		t.Fatal("verification carrier must not fabricate an application on the current plan")
	}
}

func TestVerificationDeliveryHandoff_TransitiveAndDurableRecovery(t *testing.T) {
	for _, mode := range []string{"source-artifact", "prior-proof-snapshot"} {
		t.Run(mode, func(t *testing.T) {
			source, run := verificationDeliverySource(), verificationDeliveryRun()
			dir := t.TempDir()
			o := &Orchestrator{busCtx: &types.BusContext{Mutable: types.NewMutableState("verify"), WorkDir: dir}}
			prior := verificationDeliveryProof("proof-one")
			o.stampCumulativeVerificationScope(prior, run, source)
			var candidates []*types.ChangePlan
			if mode == "source-artifact" {
				if err := types.WritePlanToFile(source, filepath.Join(dir, "plans", source.ID+".json")); err != nil {
					t.Fatal(err)
				}
			} else {
				path := filepath.Join(t.TempDir(), "prior.json")
				if err := types.WritePlanToFile(prior, path); err != nil {
					t.Fatal(err)
				}
				loaded, err := types.LoadChangePlanFromFile(path)
				if err != nil {
					t.Fatal(err)
				}
				candidates = append(candidates, loaded)
			}
			next := verificationDeliveryProof("proof-two")
			o.stampCumulativeVerificationScope(next, run, candidates...)
			got, ok := types.ResolveVerificationDelivery(next)
			if !ok || got.SourcePlanID != source.ID || got.PatchEffect.PlanID != source.ID {
				t.Fatalf("fresh proof plan did not recover original delivery: %+v, %v", got, ok)
			}
			third := verificationDeliveryProof("proof-three")
			o.stampCumulativeVerificationScope(third, run, next)
			if wire := readVerificationDeliveryWire(t, third); wire.Cumulative == nil || len(wire.Cumulative.AppliedSources) != 1 {
				t.Fatal("second handoff lost delivery")
			}
			next.CumulativeVerificationScope.AppliedSources[0].PatchEffect.Files[0].Hunks[0].AddedLineTexts[0].Text = "corrupt prior"
			if got := third.CumulativeVerificationScope.AppliedSources[0].PatchEffect.Files[0].Hunks[0].AddedLineTexts[0].Text; got != "value = 42" {
				t.Fatal("controller handoff shared mutable source line storage")
			}
		})
	}
}

func TestVerificationDeliveryHandoff_IdentityOnlyPriorStillLoadsDelivery(t *testing.T) {
	source := verificationDeliverySource()
	prior := verificationDeliveryProof("proof-old-version")
	prior.CumulativeVerificationScope = &types.CumulativeVerificationScope{SourcePlanIDs: []string{source.ID}, TargetPaths: source.TargetPaths}
	plan := verificationDeliveryProof("proof-new-version")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: types.NewMutableState("verify"), WorkDir: t.TempDir()}}
	o.stampCumulativeVerificationScope(plan, verificationDeliveryRun(), prior, source)
	if got, ok := types.ResolveVerificationDelivery(plan); !ok || got.SourcePlanID != source.ID {
		t.Fatal("ID already present incorrectly suppressed the actual delivery")
	}
}

func TestVerificationDeliveryHandoff_DurableTimestampRoundTrip(t *testing.T) {
	source := verificationDeliverySource()
	// Real applied effects use time.Now, which carries monotonic clock data
	// absent from their otherwise identical persisted JSON representation.
	source.PatchEffect.CreatedAt = time.Now()
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: types.NewMutableState("verify"), WorkDir: t.TempDir()}}
	prior := verificationDeliveryProof("proof-before-persist")
	o.stampCumulativeVerificationScope(prior, verificationDeliveryRun(), source)
	if err := types.WritePlanToFile(source, filepath.Join(o.busCtx.WorkDir, "plans", source.ID+".json")); err != nil {
		t.Fatal(err)
	}
	plan := verificationDeliveryProof("proof-after-persist")
	o.stampCumulativeVerificationScope(plan, verificationDeliveryRun(), prior)
	if _, ok := types.ResolveVerificationDelivery(plan); !ok {
		t.Fatal("same applied delivery conflicted with its durable timestamp representation")
	}
}

func TestVerificationDeliveryHandoff_RejectsUntrustedOrAmbiguousIdentity(t *testing.T) {
	for _, name := range []string{"model-injection", "missing-source", "missing-commit", "partial-application", "effect-owner", "same-id-conflict", "same-id-incomplete", "rolled-back", "restored-pointer-rolled-back", "durable-conflict"} {
		t.Run(name, func(t *testing.T) {
			source, plan, run := verificationDeliverySource(), verificationDeliveryProof("proof-current"), verificationDeliveryRun()
			o := &Orchestrator{busCtx: &types.BusContext{Mutable: types.NewMutableState("verify"), WorkDir: t.TempDir()}}
			prior := verificationDeliveryProof("proof-prior")
			o.stampCumulativeVerificationScope(prior, run, source)
			candidates := []*types.ChangePlan{source}
			switch name {
			case "model-injection":
				plan.CumulativeVerificationScope = prior.CumulativeVerificationScope
				candidates = nil
			case "missing-source":
				candidates = nil
			case "missing-commit":
				source.AppliedCommitSHA = ""
			case "partial-application":
				source.Status = types.PlanStatusPartiallyApplied
			case "effect-owner":
				source.PatchEffect.PlanID = "other-owner"
			case "same-id-conflict":
				other := verificationDeliverySource()
				other.PatchEffect.DiffFingerprint = strings.Repeat("d", 64)
				candidates = append(candidates, other)
			case "same-id-incomplete":
				other := verificationDeliverySource()
				other.PatchEffect = nil
				candidates = append(candidates, other)
			case "rolled-back", "restored-pointer-rolled-back":
				run.ProgressLedger = []types.WriteWorkflowProgress{{BatchID: "source-batch", ReasonCode: "checkpoint_restored_before_replan", At: time.Unix(200, 0)}}
				candidates = []*types.ChangePlan{prior}
				if name == "restored-pointer-rolled-back" {
					plan = prior
				}
			case "durable-conflict":
				source.PatchEffect.DiffFingerprint = strings.Repeat("d", 64)
				if err := types.WritePlanToFile(source, filepath.Join(o.busCtx.WorkDir, "plans", source.ID+".json")); err != nil {
					t.Fatal(err)
				}
				candidates = []*types.ChangePlan{prior}
			}
			o.stampCumulativeVerificationScope(plan, run, candidates...)
			if _, ok := types.ResolveVerificationDelivery(plan); ok {
				t.Fatalf("%s borrowed an untrusted delivery", name)
			}
			if plan.CumulativeVerificationScope != nil && len(plan.CumulativeVerificationScope.AppliedSources) > 0 {
				t.Fatalf("%s retained invalid source snapshots", name)
			}
		})
	}
}

func TestVerificationDeliveryHandoff_MultipleSourcesDoNotPickFirst(t *testing.T) {
	source, second := verificationDeliverySource(), verificationDeliverySource()
	second.ID, second.PatchEffect.PlanID = "source-two", "source-two"
	run := verificationDeliveryRun()
	run.Batches[0].Attempts = append(run.Batches[0].Attempts, types.WriteWorkflowAttempt{Kind: "apply", Status: "applied", PlanID: second.ID})
	plan := verificationDeliveryProof("proof-multiple")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: types.NewMutableState("verify"), WorkDir: t.TempDir()}}
	o.stampCumulativeVerificationScope(plan, run, source, second)
	if wire := readVerificationDeliveryWire(t, plan); wire.Cumulative == nil || len(wire.Cumulative.AppliedSources) != 2 {
		t.Fatal("multi-source provenance was truncated")
	}
	if _, ok := types.ResolveVerificationDelivery(plan); ok {
		t.Fatal("multi-source execution silently chose one delivery")
	}
}
