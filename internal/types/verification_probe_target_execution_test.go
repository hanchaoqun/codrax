package types

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func b1575TargetFixture(t *testing.T) (*ChangePlan, VerificationProbe, *ChangeReport) {
	t.Helper()
	probe := VerificationProbe{ID: "probe-1", Language: "python", Code: "opaque program", ContractRefs: []string{"runtime"}, PlacementRefs: []string{"placement"}, ChangedSymbolRefs: []string{"path:pkg/client.py"}}
	plan := &ChangePlan{ID: "plan-1", WorktreePath: "/worktree", VerificationProbes: []VerificationProbe{probe},
		BehaviorContracts: []WriteBehaviorContract{{ID: "runtime", Kind: WriteBehaviorObservable, Polarity: WriteBehaviorPolarityExpected, Subject: "result", Operator: WriteBehaviorOpEquals, Expected: "ok", Required: true}},
		PatchEffect:       &PatchEffectRecord{RecordID: "effect-1", PlanID: "plan-1", HeadRef: "applied-head", DiffFingerprint: "applied-diff", Files: []PatchEffectFile{{Path: "pkg/client.py", Hunks: []PatchEffectHunk{{AddedLines: 2, AddedLineNumbers: []int{3, 8}, AddedLineTexts: []PatchEffectLine{{Line: 3, Text: "return value"}, {Line: 8, Text: "return await value"}}}}}}}}
	target := &VerificationProbeTargetExecutionReceipt{Version: 1, PlanID: plan.ID, ProbeID: probe.ID, ExecutionRoot: plan.WorktreePath, PatchEffectID: "effect-1", DiffFingerprint: "applied-diff", HeadRef: "applied-head", Status: "complete", Targets: []VerificationProbeTargetExecutionTarget{{Path: "pkg/client.py", SourceSHA256: strings.Repeat("a", 64), MappingComplete: true, Owners: []VerificationProbeTargetExecutionOwner{
		{Kind: "function", CodeSHA256: strings.Repeat("b", 64), FirstLine: 2, LastLine: 4, ChangedLines: []int{3}, ExecutedLines: []int{3}},
		{Kind: "async_function", CodeSHA256: strings.Repeat("c", 64), FirstLine: 7, LastLine: 9, ChangedLines: []int{8}, ExecutedLines: []int{8}},
	}}}}
	target.SourceCommitSHA = strings.Repeat("e", 40)
	target.ManifestSHA256 = VerificationProbeTargetManifestSHA256(target)
	data, _ := json.Marshal(probe)
	digest := sha256.Sum256(data)
	now := time.Unix(1700000000, 0)
	outer := &VerificationProbeExecutionReceipt{Version: 1, DefinitionSHA256: hex.EncodeToString(digest[:]), InvocationSHA256: strings.Repeat("d", 64), ExecutionID: "invocation-1", StartedAt: now, FinishedAt: now.Add(time.Second), RepositoryRoot: "/main-repo", Executable: "/bin/python3", Args: []string{"python3", "wrapper"}, WorkingDir: "/worktree", TargetExecution: target}
	target.InvocationBindingSHA256 = VerificationProbeTargetInvocationSHA256(outer)
	target.ManifestSHA256 = VerificationProbeTargetManifestSHA256(target)
	report := &ChangeReport{PlanID: plan.ID, Passed: true, VerificationStatus: VerificationStatusPassed,
		TestResults:            []TestResult{{Kind: TestResultKindUnit, Suite: "verification_probe/python", AssertionID: probe.ID, Passed: true}},
		ExecutedCommands:       []ExecutedCommand{{Runner: "verification_probe", Framework: "python", Source: "pre_suite_verification_probe", Outcome: ExecutedCommandOutcomeExecuted, ProbeExecution: outer}},
		ChangedPathCoverage:    []ChangedPathVerificationCoverage{{Path: "pkg/client.py", LanguageFamilies: []VerificationLanguageFamily{VerificationLanguagePython}, Status: ChangedPathVerificationCovered, Caliber: ChangedPathVerificationProbe, Capability: VerificationCapabilityTargetBehavior, Runner: "verification_probe", Source: probe.ID}},
		VerificationConfidence: []VerificationConfidenceRecord{{Source: "verification_probe", Category: "probe_contract_refs", Status: "satisfied", ContractRefs: []string{"runtime"}, WitnessKind: WriteBehaviorWitnessVerificationProbe}, {Source: "verification_probe", Category: "probe_placement_refs", Status: "satisfied", ContractRefs: []string{"placement"}}},
	}
	return plan, probe, report
}

func TestB1575TargetExecutionResolution(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*ChangePlan, *VerificationProbe, *ChangeReport)
		want bool
	}{
		{"all_sync_and_async", nil, true},
		{"module_owner", func(_ *ChangePlan, _ *VerificationProbe, r *ChangeReport) {
			r.ExecutedCommands[0].ProbeExecution.TargetExecution.Targets[0].Owners[0].Kind = "module"
		}, true},
		{"class_owner", func(_ *ChangePlan, _ *VerificationProbe, r *ChangeReport) {
			r.ExecutedCommands[0].ProbeExecution.TargetExecution.Targets[0].Owners[0].Kind = "class"
		}, true},
		{"unawaited_async", func(_ *ChangePlan, _ *VerificationProbe, r *ChangeReport) {
			r.ExecutedCommands[0].ProbeExecution.TargetExecution.Targets[0].Owners[1].ExecutedLines = nil
		}, false},
		{"import_only", func(_ *ChangePlan, _ *VerificationProbe, r *ChangeReport) {
			for i := range r.ExecutedCommands[0].ProbeExecution.TargetExecution.Targets[0].Owners {
				r.ExecutedCommands[0].ProbeExecution.TargetExecution.Targets[0].Owners[i].ExecutedLines = nil
			}
		}, false},
		{"mapping_unknown", func(_ *ChangePlan, _ *VerificationProbe, r *ChangeReport) {
			r.ExecutedCommands[0].ProbeExecution.TargetExecution.Targets[0].MappingComplete = false
		}, false},
		{"observer_incomplete", func(_ *ChangePlan, _ *VerificationProbe, r *ChangeReport) {
			r.ExecutedCommands[0].ProbeExecution.TargetExecution.Status = "unknown"
		}, false},
		{"future_owner", func(_ *ChangePlan, _ *VerificationProbe, r *ChangeReport) {
			r.ExecutedCommands[0].ProbeExecution.TargetExecution.Targets[0].Owners[0].Kind = "lambda"
		}, false},
		{"missing_receipt", func(_ *ChangePlan, _ *VerificationProbe, r *ChangeReport) {
			r.ExecutedCommands[0].ProbeExecution.TargetExecution = nil
		}, false},
		{"missing_outer", func(_ *ChangePlan, _ *VerificationProbe, r *ChangeReport) { r.ExecutedCommands[0].ProbeExecution = nil }, false},
		{"failed_command", func(_ *ChangePlan, _ *VerificationProbe, r *ChangeReport) { r.ExecutedCommands[0].ExitCode = 1 }, false},
		{"suite_skipped", func(_ *ChangePlan, _ *VerificationProbe, r *ChangeReport) {
			r.ExecutedCommands[0].Outcome = ExecutedCommandOutcomeSuiteSkipped
		}, false},
		{"failed_probe", func(_ *ChangePlan, _ *VerificationProbe, r *ChangeReport) { r.TestResults[0].Passed = false }, false},
		{"wrong_probe_test", func(_ *ChangePlan, _ *VerificationProbe, r *ChangeReport) { r.TestResults[0].AssertionID = "other" }, false},
		{"wrong_plan", func(p *ChangePlan, _ *VerificationProbe, _ *ChangeReport) { p.ID = "other" }, false},
		{"missing_effect", func(p *ChangePlan, _ *VerificationProbe, _ *ChangeReport) { p.PatchEffect = nil }, false},
		{"stale_diff", func(p *ChangePlan, _ *VerificationProbe, _ *ChangeReport) { p.PatchEffect.DiffFingerprint = "other" }, false},
		{"stale_head", func(p *ChangePlan, _ *VerificationProbe, _ *ChangeReport) { p.PatchEffect.HeadRef = "other" }, false},
		{"other_root", func(p *ChangePlan, _ *VerificationProbe, _ *ChangeReport) { p.WorktreePath = "/other" }, false},
		{"changed_definition", func(_ *ChangePlan, p *VerificationProbe, _ *ChangeReport) { p.Code += "changed" }, false},
		{"pure_deletion", func(p *ChangePlan, _ *VerificationProbe, _ *ChangeReport) {
			p.PatchEffect.Files[0].Hunks = append(p.PatchEffect.Files[0].Hunks, PatchEffectHunk{RemovedLines: 1})
		}, false},
		{"missing_added_line", func(p *ChangePlan, _ *VerificationProbe, _ *ChangeReport) {
			p.PatchEffect.Files[0].Hunks[0].AddedLineNumbers = []int{3, 8, 9}
			p.PatchEffect.Files[0].Hunks[0].AddedLines = 3
		}, false},
		{"duplicate_owner", func(_ *ChangePlan, _ *VerificationProbe, r *ChangeReport) {
			x := &r.ExecutedCommands[0].ProbeExecution.TargetExecution.Targets[0]
			x.Owners = append(x.Owners, x.Owners[0])
		}, false},
		{"duplicate_target", func(_ *ChangePlan, _ *VerificationProbe, r *ChangeReport) {
			x := r.ExecutedCommands[0].ProbeExecution.TargetExecution
			x.Targets = append(x.Targets, x.Targets[0])
		}, false},
		{"execution_line_outside_owner", func(_ *ChangePlan, _ *VerificationProbe, r *ChangeReport) {
			r.ExecutedCommands[0].ProbeExecution.TargetExecution.Targets[0].Owners[0].ExecutedLines = []int{99}
		}, false},
		{"ambiguous_invocation", func(_ *ChangePlan, _ *VerificationProbe, r *ChangeReport) {
			r.ExecutedCommands = append(r.ExecutedCommands, r.ExecutedCommands[0])
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, probe, r := b1575TargetFixture(t)
			if tc.edit != nil {
				tc.edit(p, &probe, r)
			}
			if r.ExecutedCommands[0].ProbeExecution != nil && r.ExecutedCommands[0].ProbeExecution.TargetExecution != nil {
				x := r.ExecutedCommands[0].ProbeExecution.TargetExecution
				x.ManifestSHA256 = VerificationProbeTargetManifestSHA256(x)
			}
			before, _ := json.Marshal(r)
			got := ResolveVerificationProbeTargetExecution(p, probe, r)
			if !got.Applies || (len(got.Paths) > 0) != tc.want {
				t.Fatalf("resolution=%+v want execution=%v", got, tc.want)
			}
			after, _ := json.Marshal(r)
			if string(before) != string(after) {
				t.Fatal("resolver mutated report")
			}
		})
	}
}

func TestB1575StoredPythonProofReadProjection(t *testing.T) {
	for _, modern := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "observed"}[modern], func(t *testing.T) {
			p, _, r := b1575TargetFixture(t)
			if !modern {
				r.ExecutedCommands[0].ProbeExecution.TargetExecution = nil
			}
			before, _ := json.Marshal(r)
			var restored ChangeReport
			if err := json.Unmarshal(before, &restored); err != nil {
				t.Fatal(err)
			}
			if restored.HasTargetExecutionCoverage() != modern {
				t.Fatal("getter replayed wrong authority")
			}
			if restored.HasProductionPathWithoutTargetExecutionCoverage() == modern {
				t.Fatal("production getter disagrees")
			}
			profile := BuildVerificationProofProfile(p, &restored)
			if profile.TargetBehaviorPaths != 0 || (profile.TargetExecutionPaths > 0) != modern {
				t.Fatalf("profile=%+v", profile)
			}
			ledger := BuildVerificationProofLedger(p, &restored, nil)
			for _, item := range ledger.Obligations {
				if item.ContractRef == "runtime" && item.Status == VerificationProofLedgerItemCovered {
					t.Fatalf("plain probe closed runtime: %+v", item)
				}
			}
			if len(missingRequiredWriteBehaviorContractObservationIDs(p, &restored)) != 1 {
				t.Fatal("runtime obligation lost")
			}
			if BuildCumulativeVerificationProofProfile(p, &restored, []VerificationProofArtifact{{Plan: p, Report: &restored}}).TargetBehaviorPaths != 0 {
				t.Fatal("cumulative replay")
			}
			view := EffectiveVerificationProbeReport(p, &restored)
			if !view.Passed || !reflect.DeepEqual(view.TestResults, r.TestResults) || !reflect.DeepEqual(view.ExecutedCommands, restored.ExecutedCommands) {
				t.Fatal("execution outcomes changed")
			}
			view.ChangedPathCoverage[0].LanguageFamilies[0] = "changed"
			view.VerificationConfidence[0].ContractRefs[0] = "changed"
			after, _ := json.Marshal(&restored)
			if string(before) != string(after) {
				t.Fatal("projection changed original JSON")
			}
		})
	}
}

func TestB1575NativeWitnessAndOtherLanguagePreserved(t *testing.T) {
	p, _, r := b1575TargetFixture(t)
	r.VerificationConfidence = append(r.VerificationConfidence, VerificationConfidenceRecord{Source: "project_test_observation", Category: "project_test_contract_refs", Status: "satisfied", ContractRefs: []string{"runtime"}, WitnessKind: WriteBehaviorWitnessProjectTest}, VerificationConfidenceRecord{Source: "source_contract", Category: "source_contract_refs", Status: "satisfied", ContractRefs: []string{"layout"}, WitnessKind: WriteBehaviorWitnessSourceText})
	view := EffectiveVerificationConfidence(p, r)
	if !reflect.DeepEqual(view[2:], r.VerificationConfidence[2:]) {
		t.Fatal("native/source witnesses changed")
	}
	if len(missingRequiredWriteBehaviorContractObservationIDs(p, r)) != 0 {
		t.Fatal("native assertion no longer closes contract")
	}
	r.TestResults = nil
	r.ExecutedCommands = nil
	r.ChangedPathCoverage[0].LanguageFamilies = []VerificationLanguageFamily{VerificationLanguageGo}
	r.ChangedPathCoverage[0].Source = "go-probe"
	if !reflect.DeepEqual(EffectiveChangedPathVerificationCoverage(nil, r), r.ChangedPathCoverage) {
		t.Fatal("non-Python legacy was migrated")
	}
	if got := ResolveVerificationProbeTargetExecution(p, VerificationProbe{Language: "go"}, r); got.Applies {
		t.Fatal("applies outside Python")
	}
}

func TestB1575ManifestHashStaticAndInvocationIdentityUnchanged(t *testing.T) {
	_, _, r := b1575TargetFixture(t)
	outer := r.ExecutedCommands[0].ProbeExecution
	x := outer.TargetExecution
	id := verificationProbeExecutionIdentity(outer)
	hash := x.ManifestSHA256
	x.Targets[0].Owners[0].ExecutedLines = nil
	x.Status = "unknown"
	x.ReasonCode = "hook_disabled"
	if VerificationProbeTargetManifestSHA256(x) != hash {
		t.Fatal("runtime state changed static manifest")
	}
	x.Targets[0].SourceSHA256 = strings.Repeat("f", 64)
	if VerificationProbeTargetManifestSHA256(x) == hash {
		t.Fatal("source change not bound")
	}
	if verificationProbeExecutionIdentity(outer) != id {
		t.Fatal("source observation changed B1616 invocation identity")
	}
}

func TestB1575BaselineAndCurrentInvocationRemainSeparate(t *testing.T) {
	for _, outcome := range []string{ExecutedCommandOutcomeExpectedFailureObserved, ExecutedCommandOutcomeExpectedFailureNotObserved, ExecutedCommandOutcomeBaselineUnavailable} {
		for _, reverse := range []bool{false, true} {
			t.Run(outcome+map[bool]string{true: "_reverse"}[reverse], func(t *testing.T) {
				p, probe, r := b1575TargetFixture(t)
				data, _ := json.Marshal(r.ExecutedCommands[0])
				var baseline ExecutedCommand
				if err := json.Unmarshal(data, &baseline); err != nil {
					t.Fatal(err)
				}
				baseline.Source = "verification_probe_main_snapshot_baseline"
				baseline.Outcome = outcome
				baseline.ExitCode = 1
				baseline.ProbeExecution.ExecutionID = "baseline-invocation"
				baseline.ProbeExecution.WorkingDir = "/main-repo"
				baseline.ProbeExecution.TargetExecution.ExecutionRoot = "/main-repo"
				baseline.ProbeExecution.TargetExecution.ManifestSHA256 = VerificationProbeTargetManifestSHA256(baseline.ProbeExecution.TargetExecution)
				current := r.ExecutedCommands[0]
				r.ExecutedCommands = []ExecutedCommand{current, baseline}
				if reverse {
					r.ExecutedCommands = []ExecutedCommand{baseline, current}
				}
				if got := ResolveVerificationProbeTargetExecution(p, probe, r); len(got.Paths) != 1 {
					t.Fatalf("baseline blocked current receipt: %+v", got)
				}
				r.ExecutedCommands = []ExecutedCommand{baseline}
				if got := ResolveVerificationProbeTargetExecution(p, probe, r); len(got.Paths) != 0 {
					t.Fatal("baseline granted current capability")
				}
				baseline.Source = "pre_suite_verification_probe"
				r.ExecutedCommands = []ExecutedCommand{current, baseline}
				if got := ResolveVerificationProbeTargetExecution(p, probe, r); len(got.Paths) != 0 {
					t.Fatal("non-baseline conflicting command bypassed")
				}
			})
		}
	}
}

func TestB1575PythonLanguageDomainAndDefinitionBinding(t *testing.T) {
	for _, language := range []string{"python", "py", "", " PY ", "Python"} {
		t.Run(language, func(t *testing.T) {
			p, probe, r := b1575TargetFixture(t)
			probe.Language = language
			p.VerificationProbes[0] = probe
			data, _ := json.Marshal(probe)
			sum := sha256.Sum256(data)
			r.ExecutedCommands[0].ProbeExecution.DefinitionSHA256 = hex.EncodeToString(sum[:])
			x := r.ExecutedCommands[0].ProbeExecution.TargetExecution
			x.InvocationBindingSHA256 = VerificationProbeTargetInvocationSHA256(r.ExecutedCommands[0].ProbeExecution)
			x.ManifestSHA256 = VerificationProbeTargetManifestSHA256(x)
			if got := ResolveVerificationProbeTargetExecution(p, probe, r); !got.Applies || len(got.Paths) != 1 {
				t.Fatalf("typed alias lost: %+v", got)
			}
			r.ExecutedCommands[0].ProbeExecution.TargetExecution = nil
			if got := EffectiveChangedPathVerificationCoverage(p, r)[0]; got.Status == ChangedPathVerificationCovered {
				t.Fatal("alias restored legacy capability")
			}
			if got := EffectiveVerificationConfidence(p, r)[0]; got.Status == "satisfied" {
				t.Fatal("alias restored legacy contract")
			}
		})
	}
	for _, language := range []string{"python3", "python_future", "go", "javascript"} {
		if VerificationProbeLanguageIsPython(language) {
			t.Fatalf("invented Python alias %q", language)
		}
	}
}

func TestB1575SourceCommitAndManifestIntegrity(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*ChangePlan, *VerificationProbeTargetExecutionReceipt)
		want bool
	}{
		{"resolved_commit", func(p *ChangePlan, r *VerificationProbeTargetExecutionReceipt) {
			p.PatchEffect.HeadRef = r.SourceCommitSHA
			r.HeadRef = r.SourceCommitSHA
		}, true},
		{"missing_commit", func(_ *ChangePlan, r *VerificationProbeTargetExecutionReceipt) { r.SourceCommitSHA = "" }, false},
		{"non_commit", func(_ *ChangePlan, r *VerificationProbeTargetExecutionReceipt) { r.SourceCommitSHA = "HEAD" }, false},
		{"other_commit", func(p *ChangePlan, r *VerificationProbeTargetExecutionReceipt) {
			p.PatchEffect.HeadRef = strings.Repeat("f", 40)
			r.HeadRef = p.PatchEffect.HeadRef
		}, false},
		{"missing_line_text", func(p *ChangePlan, _ *VerificationProbeTargetExecutionReceipt) {
			p.PatchEffect.Files[0].Hunks[0].AddedLineTexts = nil
		}, false},
		{"duplicate_line_text", func(p *ChangePlan, _ *VerificationProbeTargetExecutionReceipt) {
			p.PatchEffect.Files[0].Hunks[0].AddedLineTexts[1].Line = 3
		}, false},
		{"outside_working_dir", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, probe, r := b1575TargetFixture(t)
			target := r.ExecutedCommands[0].ProbeExecution.TargetExecution
			if tc.edit != nil {
				tc.edit(p, target)
			} else {
				r.ExecutedCommands[0].ProbeExecution.WorkingDir = "/foreign"
			}
			target.ManifestSHA256 = VerificationProbeTargetManifestSHA256(target)
			if got := ResolveVerificationProbeTargetExecution(p, probe, r); (len(got.Paths) > 0) != tc.want {
				t.Fatalf("resolution=%+v", got)
			}
		})
	}
	p, probe, r := b1575TargetFixture(t)
	r.ExecutedCommands[0].ProbeExecution.TargetExecution.Targets[0].SourceSHA256 = strings.Repeat("f", 64)
	if got := ResolveVerificationProbeTargetExecution(p, probe, r); len(got.Paths) != 0 {
		t.Fatal("changed manifest retained old digest")
	}
}

func TestB1575EffectiveContextAndPlanReplayAreReadOnly(t *testing.T) {
	p, probe, r := b1575TargetFixture(t)
	r.VerificationConfidence[0].Detail = "historical diagnostic bytes: 保留"
	before, _ := json.Marshal(r)
	pack := WriteContextPackFromChangeReport(r)
	encoded, _ := json.Marshal(pack)
	if strings.Contains(string(encoded), "status=satisfied") || !strings.Contains(string(encoded), "python_plain_probe_assertion_witness_missing") {
		t.Fatalf("raw satisfied confidence replayed: %s", encoded)
	}
	if !strings.Contains(string(encoded), "historical diagnostic bytes") {
		t.Fatal("historical detail erased")
	}
	view := EffectiveVerificationProbeReport(p, r)
	if got := EffectiveVerificationProbeReport(p, view); !reflect.DeepEqual(view, got) {
		t.Fatal("effective projection is not idempotent")
	}
	oldPlan := *p
	oldPlan.ID = "unrelated-plan"
	if got := BuildVerificationProofProfile(&oldPlan, r); got.TargetExecutionPaths != 0 {
		t.Fatal("report transferred to unrelated plan")
	}
	if got := ResolveVerificationProbeTargetExecution(nil, probe, r); len(got.Paths) != 1 {
		t.Fatal("self-contained historical receipt disappeared")
	}
	// A missing modern carrier is not reconstructed from path/ref labels.
	var legacy ChangeReport
	if err := json.Unmarshal(before, &legacy); err != nil {
		t.Fatal(err)
	}
	legacy.ExecutedCommands[0].ProbeExecution.TargetExecution = nil
	if got := ResolveVerificationProbeTargetExecution(nil, probe, &legacy); len(got.Paths) != 0 {
		t.Fatal("history invented receipt")
	}
	for i := 0; i < 2; i++ {
		BuildVerificationProofLedger(p, r, nil)
		BuildCumulativeVerificationProofProfile(p, r, []VerificationProofArtifact{{Plan: p, Report: r}})
		WriteContextPackFromChangeReport(r)
	}
	after, _ := json.Marshal(r)
	if string(before) != string(after) {
		t.Fatal("read consumers rewrote report")
	}
}

func TestB1575OuterInvocationBindingCannotBeReattached(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*VerificationProbeExecutionReceipt)
	}{
		{"other_repository_domain", func(r *VerificationProbeExecutionReceipt) { r.RepositoryRoot = "/other-main" }},
		{"other_execution", func(r *VerificationProbeExecutionReceipt) { r.ExecutionID = "other-invocation" }},
		{"other_working_directory", func(r *VerificationProbeExecutionReceipt) { r.WorkingDir = "/worktree/subdir" }},
		{"other_executable", func(r *VerificationProbeExecutionReceipt) { r.Executable = "/other/python" }},
		{"other_argv", func(r *VerificationProbeExecutionReceipt) { r.Args[0] = "other" }},
		{"other_finished_at", func(r *VerificationProbeExecutionReceipt) { r.FinishedAt = r.FinishedAt.Add(time.Second) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, probe, r := b1575TargetFixture(t)
			if got := ResolveVerificationProbeTargetExecution(p, probe, r); len(got.Paths) != 1 {
				t.Fatal("distinct main/worktree roots should be valid")
			}
			tc.edit(r.ExecutedCommands[0].ProbeExecution)
			if got := ResolveVerificationProbeTargetExecution(p, probe, r); len(got.Paths) != 0 {
				t.Fatal("nested observation was reattached to a different outer invocation")
			}
		})
	}
}

func TestB1575CumulativeLegacyPythonRefsCannotCoverEachOther(t *testing.T) {
	makeReport := func(id, missing, claimed string) *ChangeReport {
		return &ChangeReport{PlanID: id, Passed: true, VerificationStatus: VerificationStatusPassed,
			TestResults:            []TestResult{{Suite: "verification_probe/python", AssertionID: "probe/" + id, Passed: true}},
			ExecutedCommands:       []ExecutedCommand{{Runner: "verification_probe", Framework: "python", Suite: "verification_probe/python", Outcome: ExecutedCommandOutcomeExecuted}},
			VerificationConfidence: []VerificationConfidenceRecord{{Source: "verification_probe", Category: "probe_soft_contract_refs", Status: "missing", ReasonCode: "verification_probe_missing_soft_contract_ref", ContractRefs: []string{missing}}, {Source: "verification_probe", Category: "probe_soft_contract_refs", Status: "satisfied", ReasonCode: "verification_probe_soft_contract_ref_covered", ContractRefs: []string{claimed}}}}
	}
	a, b := makeReport("a", "left", "right"), makeReport("b", "right", "left")
	before, _ := json.Marshal([]*ChangeReport{a, b})
	for _, reports := range [][2]*ChangeReport{{a, b}, {b, a}} {
		profile := BuildCumulativeVerificationProofProfile(nil, reports[0], []VerificationProofArtifact{{Report: reports[1]}})
		if profile.Status != VerificationProofWeak || !verificationProofHasReason(profile, "verification_probe_missing_soft_contract_ref") {
			t.Fatalf("legacy reports mutually signed refs: %+v", profile)
		}
		ledger := BuildVerificationProofLedger(nil, reports[0], []VerificationProofArtifact{{Report: reports[1]}})
		for _, item := range ledger.Obligations {
			if (item.ContractRef == "left" || item.ContractRef == "right") && item.Status == VerificationProofLedgerItemCovered {
				t.Fatal("ledger restored legacy Python proof")
			}
		}
	}
	after, _ := json.Marshal([]*ChangeReport{a, b})
	if string(before) != string(after) {
		t.Fatal("cumulative projection rewrote history")
	}
}

func TestB1575ReportLocalChangedIdentityPreservesActualExecution(t *testing.T) {
	for _, modern := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "modern"}[modern], func(t *testing.T) {
			p, _, r := b1575TargetFixture(t)
			r.VerificationConfidence = []VerificationConfidenceRecord{{Source: "verification_probe", Category: "probe_changed_symbol", Status: "satisfied", ReasonCode: "verification_probe_changed_symbol_coupled", ChangedSymbolRefs: []string{"path:pkg/client.py"}, Detail: "unchanged original diagnostic"}}
			if !modern {
				r.ExecutedCommands[0].ProbeExecution.TargetExecution = nil
			}
			before, _ := json.Marshal(r)
			confidence := EffectiveVerificationConfidence(nil, r)
			if (confidence[0].Status == "satisfied") != modern {
				t.Fatalf("report-local identity projection=%+v", confidence)
			}
			if got := EffectiveChangedPathVerificationCoverage(nil, r)[0]; (got.Capability == VerificationCapabilityTargetExecution) != modern {
				t.Fatalf("coverage=%+v", got)
			}
			pack := WriteContextPackFromChangeReport(r)
			data, _ := json.Marshal(pack)
			if strings.Contains(string(data), "status=satisfied") != modern {
				t.Fatalf("context capability disagreement: %s", data)
			}
			final := writeFinalVerificationSummary(r)
			if strings.Contains(strings.Join(final.ConfidenceReasonCodes, ","), "python_target_execution_unobserved") == modern {
				t.Fatalf("final summary=%+v", final)
			}
			if got := BuildVerificationProofProfile(p, r); (got.TargetExecutionPaths == 1) != modern {
				t.Fatalf("profile=%+v", got)
			}
			after, _ := json.Marshal(r)
			if string(before) != string(after) {
				t.Fatal("report-local consumer mutated original")
			}
		})
	}
}

func TestB1575ProjectedPythonProofDebtWeakensProfileWithoutLoadedPlan(t *testing.T) {
	_, _, r := b1575TargetFixture(t)
	if !r.HasTargetExecutionCoverage() {
		t.Fatal("actual target execution positive missing")
	}
	profile := BuildVerificationProofProfile(nil, r)
	if profile.Status != VerificationProofWeak || !verificationProofHasReason(profile, "python_plain_probe_assertion_witness_missing") {
		t.Fatalf("profile hid projected assertion debt: %+v", profile)
	}
	ledger := BuildVerificationProofLedger(nil, r, nil)
	if ledger.State == VerificationProofLedgerVerified || ledger.UncoveredCount == 0 {
		t.Fatal("unknown assertion refs became fully proved")
	}
}

func TestB1575IndependentNativeReceiptCanCloseProjectedDebt(t *testing.T) {
	for _, cumulative := range []bool{false, true} {
		for _, complete := range []bool{false, true} {
			t.Run(map[bool]string{false: "local", true: "cumulative"}[cumulative]+map[bool]string{false: "_partial", true: "_complete"}[complete], func(t *testing.T) {
				p, _, r := b1575TargetFixture(t)
				// Keep two runtime debts, not a placement obligation: their closure is
				// demonstrated by exact independent native assertion refs below.
				r.VerificationConfidence = r.VerificationConfidence[:1]
				r.VerificationConfidence[0].ContractRefs = []string{"runtime", "boundary"}
				p.BehaviorContracts = append(p.BehaviorContracts, WriteBehaviorContract{ID: "boundary", Kind: WriteBehaviorObservable, Polarity: WriteBehaviorPolarityExpected, Operator: WriteBehaviorOpSatisfies, Expected: "boundary result", Required: true})
				refs := []string{"runtime"}
				if complete {
					refs = append(refs, "boundary")
				}
				native := &ChangeReport{PlanID: p.ID, Passed: true, VerificationStatus: VerificationStatusPassed, TestResults: []TestResult{{Suite: "tests/test_client.py", AssertionID: "test_client", ObservationScope: TestObservationScopeAssertion, Passed: true}}, ExecutedCommands: []ExecutedCommand{{Runner: "pytest", Framework: "pytest", Suite: "tests/test_client.py", Outcome: ExecutedCommandOutcomeExecuted, Source: "declared_coverage_test_surface"}}, VerificationConfidence: []VerificationConfidenceRecord{{Source: "project_test_observation", Category: "project_test_contract_refs", Status: "satisfied", ReasonCode: "project_test_contract_ref_observed", ContractRefs: refs, WitnessKind: WriteBehaviorWitnessProjectTest}}}
				var artifacts []VerificationProofArtifact
				if cumulative {
					other := *p
					other.ID = "native-plan"
					native.PlanID = other.ID
					artifacts = []VerificationProofArtifact{{Plan: &other, Report: native}}
				} else {
					r.TestResults = append(r.TestResults, native.TestResults...)
					r.ExecutedCommands = append(r.ExecutedCommands, native.ExecutedCommands...)
					r.VerificationConfidence = append(r.VerificationConfidence, native.VerificationConfidence...)
				}
				before, _ := json.Marshal([]any{r, artifacts})
				profile := BuildCumulativeVerificationProofProfile(p, r, artifacts)
				ledger := BuildVerificationProofLedger(p, r, artifacts)
				if complete {
					if profile.Status != VerificationProofStrong || ledger.UncoveredCount != 0 || ledger.State != VerificationProofLedgerVerified {
						t.Fatalf("native exact receipt could not close debt: %+v %+v", profile, ledger)
					}
				} else if profile.Status != VerificationProofWeak || ledger.UncoveredCount == 0 {
					t.Fatal("partial native receipt closed all debt")
				}
				for _, item := range ledger.Obligations {
					if item.Source == "verification_probe" && item.Status == VerificationProofLedgerItemCovered {
						t.Fatal("independent native proof relabeled Python as an assertion witness")
					}
				}
				after, _ := json.Marshal([]any{r, artifacts})
				if string(before) != string(after) {
					t.Fatal("native resolution rewrote original reports")
				}
			})
		}
	}
}
