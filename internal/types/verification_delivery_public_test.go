package types

import (
	"encoding/json"
	"strings"
	"testing"
)

// Exercise exported receipt consumers through persisted JSON. These are
// protocol fixtures, not claims that this test executed a native subprocess.
// JSON deliberately keeps the pre-change reproduction compilable.
func verificationDeliveryPublicFollowup(t *testing.T, p *ChangePlan, r *ChangeReport) {
	t.Helper()
	sourceID, effect := p.ID, p.PatchEffect
	commit := strings.Repeat("e", 40)
	effect.HeadRef, effect.Source = commit, "applied_commit"
	effect.DiffFingerprint = strings.Repeat("d", 64)
	p.ID, r.PlanID = "proof-followup", "proof-followup"
	p.Status, p.PersistenceKind = PlanStatusNoChangeRequired, PlanPersistenceProofProbeOnly
	p.TargetPaths = []string{"pkg/client.py"}
	if len(p.VerificationProbes) == 0 {
		p.VerificationProbes = []VerificationProbe{{ID: "probe-1", Language: "python", Code: "assert True"}}
	}
	p.AppliedCommitSHA, p.PatchEffect = "", nil
	var doc map[string]any
	raw, _ := json.Marshal(p)
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	doc["cumulative_verification_scope"] = map[string]any{
		"source_plan_ids": []string{sourceID}, "target_paths": []string{"pkg/client.py"},
		"applied_sources": []any{map[string]any{"source_plan_id": sourceID, "applied_commit_sha": commit, "patch_effect": effect}},
	}
	raw, _ = json.Marshal(doc)
	if err := json.Unmarshal(raw, p); err != nil {
		t.Fatal(err)
	}
	for i := range r.ExistingTestExecutions {
		r.ExistingTestExecutions[i].PlanID = p.ID
		r.ExistingTestExecutions[i].AppliedCommitSHA = commit
		r.ExistingTestExecutions[i].DiffFingerprint = effect.DiffFingerprint
	}
	for i := range r.ExecutedCommands {
		if outer := r.ExecutedCommands[i].ProbeExecution; outer != nil && outer.TargetExecution != nil {
			x := outer.TargetExecution
			x.PlanID, x.HeadRef, x.SourceCommitSHA, x.DiffFingerprint = p.ID, commit, commit, effect.DiffFingerprint
		}
	}
	raw, _ = json.Marshal(r)
	doc = nil
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if receipts, ok := doc["existing_test_executions"].([]any); ok {
		for _, row := range receipts {
			row.(map[string]any)["source_plan_id"] = sourceID
		}
	}
	if commands, ok := doc["executed_commands"].([]any); ok {
		for _, row := range commands {
			if outer, ok := row.(map[string]any)["probe_execution"].(map[string]any); ok {
				if target, ok := outer["target_execution"].(map[string]any); ok {
					target["source_plan_id"] = sourceID
				}
			}
		}
	}
	raw, _ = json.Marshal(doc)
	if err := json.Unmarshal(raw, r); err != nil {
		t.Fatal(err)
	}
	for i := range r.ExecutedCommands {
		if outer := r.ExecutedCommands[i].ProbeExecution; outer != nil && outer.TargetExecution != nil {
			outer.TargetExecution.ManifestSHA256 = VerificationProbeTargetManifestSHA256(outer.TargetExecution)
		}
	}
}

func TestVerificationDeliveryPublicConsumers(t *testing.T) {
	t.Run("native_fresh_followup_receipt", func(t *testing.T) {
		p, r := existingIntentConsumerFixture()
		assertExistingIntentConsumerStatuses(t, p, r, "satisfied")
		verificationDeliveryPublicFollowup(t, p, r)
		assertExistingIntentConsumerStatuses(t, p, r, "satisfied")
		r.PlanID = "current-plan"
		assertExistingIntentConsumerStatuses(t, p, r, "missing")
	})
	t.Run("probe_fresh_followup_receipt", func(t *testing.T) {
		p, probe, r := b1575TargetFixture(t)
		if got := ResolveVerificationProbeTargetExecution(p, probe, r); len(got.Paths) != 1 {
			t.Fatalf("own control: %+v", got)
		}
		verificationDeliveryPublicFollowup(t, p, r)
		if got := ResolveVerificationProbeTargetExecution(p, probe, r); len(got.Paths) != 1 {
			t.Fatalf("fresh followup denied: %+v", got)
		}
		r.PlanID = "plan-1"
		if got := ResolveVerificationProbeTargetExecution(p, probe, r); len(got.Paths) != 0 {
			t.Fatalf("old report borrowed: %+v", got)
		}
	})
}
