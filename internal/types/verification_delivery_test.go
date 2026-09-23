package types

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func verificationDeliveryFixture(t *testing.T) (*ChangePlan, VerificationProbe, *ChangeReport) {
	t.Helper()
	p, probe, r := b1575TargetFixture(t)
	verificationDeliveryPublicFollowup(t, p, r)
	return p, probe, r
}

func TestVerificationDeliveryBorrowBoundary(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*ChangePlan)
	}{
		{"missing_scope", func(p *ChangePlan) { p.CumulativeVerificationScope = nil }},
		{"ids_only", func(p *ChangePlan) { p.CumulativeVerificationScope.AppliedSources = nil }},
		{"no_source_ids", func(p *ChangePlan) { p.CumulativeVerificationScope.SourcePlanIDs = nil }},
		{"different_source_id", func(p *ChangePlan) { p.CumulativeVerificationScope.SourcePlanIDs[0] = "other" }},
		{"multiple_source_ids", func(p *ChangePlan) {
			p.CumulativeVerificationScope.SourcePlanIDs = append(p.CumulativeVerificationScope.SourcePlanIDs, "other")
		}},
		{"duplicate_source_ids", func(p *ChangePlan) {
			p.CumulativeVerificationScope.SourcePlanIDs = append(p.CumulativeVerificationScope.SourcePlanIDs, p.CumulativeVerificationScope.SourcePlanIDs[0])
		}},
		{"duplicate_snapshot", func(p *ChangePlan) {
			p.CumulativeVerificationScope.AppliedSources = append(p.CumulativeVerificationScope.AppliedSources, p.CumulativeVerificationScope.AppliedSources[0])
		}},
		{"conflicting_snapshot", func(p *ChangePlan) {
			s := CloneVerificationDeliverySnapshot(p.CumulativeVerificationScope.AppliedSources[0])
			s.AppliedCommitSHA = strings.Repeat("f", 40)
			p.CumulativeVerificationScope.AppliedSources = append(p.CumulativeVerificationScope.AppliedSources, s)
		}},
		{"missing_source_owner", func(p *ChangePlan) { p.CumulativeVerificationScope.AppliedSources[0].SourcePlanID = "" }},
		{"wrong_source_owner", func(p *ChangePlan) { p.CumulativeVerificationScope.AppliedSources[0].PatchEffect.PlanID = p.ID }},
		{"self_source", func(p *ChangePlan) {
			s := &p.CumulativeVerificationScope.AppliedSources[0]
			s.SourcePlanID, s.PatchEffect.PlanID, p.CumulativeVerificationScope.SourcePlanIDs[0] = p.ID, p.ID, p.ID
		}},
		{"missing_effect", func(p *ChangePlan) { p.CumulativeVerificationScope.AppliedSources[0].PatchEffect = nil }},
		{"missing_record", func(p *ChangePlan) { p.CumulativeVerificationScope.AppliedSources[0].PatchEffect.RecordID = "" }},
		{"missing_commit", func(p *ChangePlan) { p.CumulativeVerificationScope.AppliedSources[0].AppliedCommitSHA = "" }},
		{"noncommit", func(p *ChangePlan) { p.CumulativeVerificationScope.AppliedSources[0].AppliedCommitSHA = "HEAD" }},
		{"conflicting_head", func(p *ChangePlan) {
			p.CumulativeVerificationScope.AppliedSources[0].PatchEffect.HeadRef = strings.Repeat("a", 40)
		}},
		{"invalid_diff", func(p *ChangePlan) {
			p.CumulativeVerificationScope.AppliedSources[0].PatchEffect.DiffFingerprint = "unknown"
		}},
		{"missing_head", func(p *ChangePlan) { p.CumulativeVerificationScope.AppliedSources[0].PatchEffect.HeadRef = "" }},
		{"option_head", func(p *ChangePlan) { p.CumulativeVerificationScope.AppliedSources[0].PatchEffect.HeadRef = "--all" }},
		{"unknown_source", func(p *ChangePlan) {
			p.CumulativeVerificationScope.AppliedSources[0].PatchEffect.Source = "model_inference"
		}},
		{"own_commit_without_effect", func(p *ChangePlan) { p.AppliedCommitSHA = strings.Repeat("f", 40) }},
		{"invalid_own_effect", func(p *ChangePlan) {
			p.PatchEffect = CloneVerificationDeliverySnapshot(p.CumulativeVerificationScope.AppliedSources[0]).PatchEffect
		}},
		{"own_applied_paths", func(p *ChangePlan) { p.AppliedPaths = []string{"pkg/client.py"} }},
		{"own_checkpoint", func(p *ChangePlan) { p.ApplyCheckpoint = &ApplyCheckpointRecord{} }},
		{"ordinary_plan", func(p *ChangePlan) { p.PersistenceKind = "" }},
		{"pending_sentinel", func(p *ChangePlan) { p.Status = PlanStatusPending }},
		{"changed_source", func(p *ChangePlan) {
			p.Changes = []FileChange{{Path: "pkg/client.py", Kind: "modify", NewContent: "changed"}}
		}},
		{"project_test_claim", func(p *ChangePlan) { p.ProjectTestObservations = []ProjectTestObservation{{ID: "forged"}} }},
		{"empty_probe", func(p *ChangePlan) { p.VerificationProbes = nil }},
		{"unscoped_probe", func(p *ChangePlan) { p.TargetPaths = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, probe, r := verificationDeliveryFixture(t)
			if _, ok := ResolveVerificationDelivery(p); !ok {
				t.Fatal("positive fixture invalid")
			}
			tc.edit(p)
			before, _ := json.Marshal([]any{p, r})
			if _, ok := ResolveVerificationDelivery(p); ok {
				t.Fatal("invalid source inherited")
			}
			if got := ResolveVerificationProbeTargetExecution(p, probe, r); len(got.Paths) != 0 {
				t.Fatalf("invalid proof: %+v", got)
			}
			after, _ := json.Marshal([]any{p, r})
			if string(after) != string(before) {
				t.Fatal("resolution changed history")
			}
			nativePlan, nativeReport := existingIntentConsumerFixture()
			verificationDeliveryPublicFollowup(t, nativePlan, nativeReport)
			tc.edit(nativePlan)
			assertExistingIntentConsumerStatuses(t, nativePlan, nativeReport, "missing")
		})
	}
}

func TestVerificationDeliveryReceiptOwnership(t *testing.T) {
	for _, source := range []string{"", "wrong-owner", "proof-followup"} {
		t.Run("native_"+source, func(t *testing.T) {
			p, r := existingIntentConsumerFixture()
			verificationDeliveryPublicFollowup(t, p, r)
			r.ExistingTestExecutions[0].SourcePlanID = source
			assertExistingIntentConsumerStatuses(t, p, r, "missing")
		})
		t.Run("probe_"+source, func(t *testing.T) {
			p, probe, r := verificationDeliveryFixture(t)
			x := r.ExecutedCommands[0].ProbeExecution.TargetExecution
			x.SourcePlanID = source
			x.ManifestSHA256 = VerificationProbeTargetManifestSHA256(x)
			if got := ResolveVerificationProbeTargetExecution(p, probe, r); len(got.Paths) != 0 {
				t.Fatalf("borrowed owner: %+v", got)
			}
		})
	}
	p, probe, r := verificationDeliveryFixture(t)
	x := r.ExecutedCommands[0].ProbeExecution.TargetExecution
	old := x.ManifestSHA256
	x.SourcePlanID = "changed"
	if VerificationProbeTargetManifestSHA256(x) == old {
		t.Fatal("source owner not included in manifest")
	}
	x.SourcePlanID = p.CumulativeVerificationScope.AppliedSources[0].SourcePlanID
	x.SourceCommitSHA = strings.Repeat("b", 40)
	x.ManifestSHA256 = VerificationProbeTargetManifestSHA256(x)
	if got := ResolveVerificationProbeTargetExecution(p, probe, r); len(got.Paths) != 0 {
		t.Fatalf("wrong commit: %+v", got)
	}
}

func TestVerificationDeliverySnapshotIsolationAndPersistence(t *testing.T) {
	p, _, _ := verificationDeliveryFixture(t)
	source := &p.CumulativeVerificationScope.AppliedSources[0]
	source.PatchEffect.Files[0].Events = []PatchEffectEvent{{Code: "original"}}
	source.PatchEffect.Files[0].Hunks[0].RemovedLineTexts = []PatchEffectLine{{Line: 1, Text: "old"}}
	before, _ := json.Marshal(p)
	copy, ok := ResolveVerificationDelivery(p)
	if !ok {
		t.Fatal("missing delivery")
	}
	copy.PatchEffect.Files[0].Path = "other"
	copy.PatchEffect.Files[0].Events[0].Code = "changed"
	copy.PatchEffect.Files[0].Hunks[0].AddedLineNumbers[0] = 999
	copy.PatchEffect.Files[0].Hunks[0].AddedLineTexts[0].Text = "changed"
	copy.PatchEffect.Files[0].Hunks[0].RemovedLineTexts[0].Text = "changed"
	after, _ := json.Marshal(p)
	if string(after) != string(before) {
		t.Fatal("returned identity aliases retained delivery")
	}
	fingerprint := PlanFingerprint(p)
	without := *p
	without.CumulativeVerificationScope = nil
	if fingerprint != PlanFingerprint(&without) {
		t.Fatal("delivery metadata changed apply fingerprint")
	}
	file := filepath.Join(t.TempDir(), "proof.json")
	if err := WritePlanToFile(p, file); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadChangePlanFromFile(file)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := ResolveVerificationDelivery(loaded)
	if !ok || !reflect.DeepEqual(got, *source) || loaded.ID == got.SourcePlanID || loaded.PatchEffect != nil || loaded.AppliedCommitSHA != "" {
		t.Fatalf("persistence rebadged or lost source: %+v", loaded)
	}
	loaded.CumulativeVerificationScope.AppliedSources[0].PatchEffect.Files[0].Path = "loaded mutation"
	after, _ = json.Marshal(p)
	if string(after) != string(before) {
		t.Fatal("loaded snapshot aliases original")
	}
}

func TestVerificationDeliveryOwnCompatibilityAndStrictConstruction(t *testing.T) {
	p, probe, r := b1575TargetFixture(t)
	if p.AppliedCommitSHA != "" {
		t.Fatal("fixture no longer checks legacy empty own SHA")
	}
	if got := ResolveVerificationProbeTargetExecution(p, probe, r); len(got.Paths) != 1 {
		t.Fatalf("legacy own identity regressed: %+v", got)
	}
	if _, ok := VerificationDeliverySnapshotFromAppliedPlan(p); ok {
		t.Fatal("incomplete own identity allowed for inheritance")
	}
	p.AppliedCommitSHA = strings.Repeat("e", 40)
	p.PatchEffect.HeadRef, p.PatchEffect.Source = p.AppliedCommitSHA, "applied_commit"
	p.PatchEffect.DiffFingerprint = strings.Repeat("d", 64)
	snapshot, ok := VerificationDeliverySnapshotFromAppliedPlan(p)
	if !ok || !snapshot.Valid() {
		t.Fatal("complete source not captured")
	}
	snapshot.PatchEffect.Files[0].Hunks[0].AddedLineNumbers[0] = 100
	if p.PatchEffect.Files[0].Hunks[0].AddedLineNumbers[0] == 100 {
		t.Fatal("source constructor shared nested lines")
	}
	followup, _, _ := verificationDeliveryFixture(t)
	if _, ok := VerificationDeliverySnapshotFromAppliedPlan(followup); ok {
		t.Fatal("inherited source re-authored as own apply")
	}
}

func TestVerificationDeliveryNeverCreatesContractWitness(t *testing.T) {
	p, probe, r := verificationDeliveryFixture(t)
	if got := ResolveVerificationProbeTargetExecution(p, probe, r); len(got.Paths) != 1 {
		t.Fatalf("changed-owner execution lost: %+v", got)
	}
	for _, rec := range EffectiveVerificationConfidence(p, r) {
		if rec.Category == "probe_contract_refs" && rec.Status == "satisfied" {
			t.Fatal("retained source identity turned target execution into contract proof")
		}
	}
	before, _ := json.Marshal([]any{p, r})
	_ = BuildVerificationProofProfile(p, r)
	_ = BuildVerificationProofLedger(p, r, nil)
	after, _ := json.Marshal([]any{p, r})
	if string(before) != string(after) {
		t.Fatal("proof projection mutated execution or source identity")
	}
}

func TestVerificationDeliveryCumulativeDiffAndSourceWithdrawal(t *testing.T) {
	p, _, _ := verificationDeliveryFixture(t)
	s := &p.CumulativeVerificationScope.AppliedSources[0]
	s.PatchEffect.Source, s.PatchEffect.HeadRef, s.PatchEffect.BaseRef = "workflow_cumulative_owned_diff", "HEAD", strings.Repeat("a", 40)
	if !s.Valid() {
		t.Fatal("valid captured cumulative diff lost")
	}
	if got, ok := ResolveVerificationDelivery(p); !ok || got.PatchEffect.PlanID != s.SourcePlanID {
		t.Fatal("cumulative source owner changed")
	}
	s.PatchEffect.BaseRef = ""
	if s.Valid() {
		t.Fatal("cumulative diff lost its base")
	}
	s.PatchEffect.BaseRef = strings.Repeat("a", 40)
	p.CumulativeVerificationScope.SourcePlanIDs = nil // restore-aware producer withdrew the source
	if _, ok := ResolveVerificationDelivery(p); ok {
		t.Fatal("withdrawn source retained delivery authority")
	}
}

// The controller's verify-infra, skipped-verify and status-persistence paths
// set AppliedAt even on proof-only plans. Exercise their durable status-update
// API, not an invented interpretation that a timestamp proves a source apply.
func TestVerificationDeliveryPostVerifyLifecyclePersistence(t *testing.T) {
	for _, status := range []string{PlanStatusUnverified, PlanStatusApplied, PlanStatusVerifyFailed} {
		for _, lane := range []string{"native", "probe"} {
			t.Run(status+"_"+lane, func(t *testing.T) {
				p, probe, r := verificationDeliveryFixture(t)
				if lane == "native" {
					p, r = existingIntentConsumerFixture()
					verificationDeliveryPublicFollowup(t, p, r)
				}
				before, _ := json.Marshal(p.CumulativeVerificationScope.AppliedSources)
				file := filepath.Join(t.TempDir(), "post-verify.json")
				if err := WritePlanToFile(p, file); err != nil {
					t.Fatal(err)
				}
				at := time.Date(2026, 9, 23, 1, 2, 3, 4, time.UTC)
				if err := UpdatePlanStatusOnDisk(file, status, &at, p.WorktreePath); err != nil {
					t.Fatal(err)
				}
				loaded, err := LoadChangePlanFromFile(file)
				if err != nil {
					t.Fatal(err)
				}
				if loaded.Status != status || loaded.AppliedAt == nil || !loaded.AppliedAt.Equal(at) || loaded.AppliedCommitSHA != "" || loaded.PatchEffect != nil {
					t.Fatal("lifecycle update changed actual apply identity")
				}
				after, _ := json.Marshal(loaded.CumulativeVerificationScope.AppliedSources)
				if string(after) != string(before) {
					t.Fatal("lifecycle update changed source delivery")
				}
				if _, ok := ResolveVerificationDelivery(loaded); !ok {
					t.Fatal("verification timestamp hid retained source")
				}
				if lane == "native" {
					assertExistingIntentConsumerStatuses(t, loaded, r, "satisfied")
				} else if got := ResolveVerificationProbeTargetExecution(loaded, probe, r); len(got.Paths) != 1 {
					t.Fatalf("verification lifecycle lost current target receipt: %+v", got)
				}
			})
		}
	}
}
