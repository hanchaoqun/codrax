package agent

import (
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// write_controller_worktree_fold_in4_test.go — fold-in round four of V5-2
// (colleague_merge_audit §40.36 四轮收编, finding M): over EVERY runner
// manifest row that declares lockfiles (tool.RunnerSideEffectLockfileBasenames,
// bound to the real table) × EVERY fixed-point member × nested paths, the
// context-pack effect row and the controller prompt line keep the path
// basename whole and every rendered token intact (the row drops whole
// tokens by priority instead of cutting into "…resolved" / "…file.lock" /
// "…argo.lock"), and the dedicated disclosure item / line keeps the whole
// phrase and the whole path.

func effectRowBasenameAndTokensIntact(t *testing.T, label, row string, effect types.VerificationWorktreeEffect) {
	t.Helper()
	idx := strings.LastIndex(row, " path=")
	if idx < 0 {
		t.Fatalf("%s: row lacks the path element: %q", label, row)
	}
	seg, head := row[idx+len(" path="):], row[:idx]
	base := effect.Path
	if slash := strings.LastIndex(effect.Path, "/"); slash >= 0 {
		base = effect.Path[slash:]
	}
	if seg != effect.Path {
		tail := strings.TrimPrefix(seg, "…")
		if tail == seg || !strings.HasSuffix(effect.Path, tail) || !strings.HasSuffix(tail, base) {
			t.Fatalf("%s: path segment %q cuts into the basename of %q: %q", label, seg, effect.Path, row)
		}
	}
	expected := map[string]bool{
		"kind=" + string(effect.Kind): true, "ownership=" + effect.Ownership: true, "action=" + effect.Action: true,
		"drift_class=" + string(effect.DriftClass): true, "disposition=" + string(effect.Disposition): true,
		"owner_runner=" + effect.OwnerRunner: true, "lockfile_fixed_point=" + string(effect.LockfileFixedPoint): true,
	}
	for _, token := range strings.Split(head, " ") {
		if !expected[token] {
			t.Fatalf("%s: row carries a token that is not a whole typed token: %q in %q", label, token, row)
		}
	}
	// For every real owner basename the decision tokens always fit.
	for _, token := range []string{"lockfile_fixed_point=" + string(effect.LockfileFixedPoint), "drift_class=" + string(effect.DriftClass),
		"disposition=" + string(effect.Disposition), "owner_runner=" + effect.OwnerRunner} {
		if !strings.Contains(head+" ", token+" ") {
			t.Fatalf("%s: decision token %q must survive on a real owner row: %q", label, token, row)
		}
	}
}

func TestWriteControllerPromptKeepsBasenameAndTokensForEveryLockfileOwnerAndFixedPoint(t *testing.T) {
	owners := tool.RunnerSideEffectLockfileBasenames()
	if len(owners) < 4 {
		t.Fatalf("manifest exposes too few lockfile owners: %v", owners)
	}
	runners := make([]string, 0, len(owners))
	for runner := range owners {
		runners = append(runners, runner)
	}
	sort.Strings(runners)
	deepDir := strings.Repeat("segment/", 40)
	for _, runner := range runners {
		for _, base := range owners[runner] {
			for _, fp := range types.AllVerificationLockfileFixedPoints() {
				for _, path := range []string{base, "crates/foo/" + base, "services/backend/packages/api-gateway/" + base, deepDir + base} {
					label := runner + "/" + string(fp) + "/" + path
					effect := types.VerificationWorktreeEffect{
						Path: path, Kind: types.VerificationWorktreeEffectTrackedChanged, Ownership: "git_tracked",
						Action: "disclosed_not_committed_not_auto_reverted", DriftClass: types.VerificationWorktreeDriftDependencyLockfileRefresh,
						OwnerRunner: runner, OwnerWorkingDir: ".", Disposition: types.VerificationWorktreeEffectDisclosed, LockfileFixedPoint: fp,
					}
					row := types.WriteContextWorktreeEffectText(effect)
					if len([]rune(row)) > 240 {
						t.Fatalf("%s: effect row exceeds the item bound: %d", label, len([]rune(row)))
					}
					effectRowBasenameAndTokensIntact(t, label+" (pack row)", row, effect)

					// Context pack: the row and, for unproven members, the item.
					report := &types.ChangeReport{PlanID: "plan-m4", Passed: true,
						WorktreeAudit: &types.VerificationWorktreeAudit{Status: types.VerificationWorktreeAuditTrackedDriftDisclosed, TrackedEffectCount: 1,
							DisclosedTrackedEffectCount: 1, Effects: []types.VerificationWorktreeEffect{effect}}}
					pack := types.NormalizeWriteContextPack(types.WriteContextPackFromChangeReport(report))
					phrase := types.VerificationLockfileFixedPointDisclosure(fp, false)
					packRow, packItem := "", ""
					for _, it := range pack.Items {
						switch it.Kind {
						case "verification_worktree_effect":
							packRow = it.Text
						case "verification_lockfile_fixed_point":
							packItem = it.Text
						}
					}
					if packRow != row {
						t.Fatalf("%s: pack row differs from the shared helper: %q vs %q", label, packRow, row)
					}
					if phrase != "" && !strings.HasSuffix(packItem, " ("+phrase+") path="+path) {
						t.Fatalf("%s: pack disclosure item must keep the whole phrase and the whole path: %q", label, packItem)
					}

					// Controller prompt line + disclosure line.
					mut := types.NewMutableState("fold-in four")
					mut.SetChangePlan(&types.ChangePlan{ID: "plan-m4", Status: types.PlanStatusPending})
					mut.SetChangeReport(report)
					got := (&writeControllerEvaluator{}).BuildInitialInstruction(&types.AgentContext{Mutable: mut}, nil)
					line, disclosure := "", ""
					for _, candidate := range strings.Split(got, "\n") {
						if strings.HasPrefix(candidate, "- verification_worktree_effect: ") {
							line = strings.TrimPrefix(candidate, "- verification_worktree_effect: ")
						}
						if strings.HasPrefix(candidate, "- verification_lockfile_fixed_point: ") {
							disclosure = strings.TrimPrefix(candidate, "- verification_lockfile_fixed_point: ")
						}
					}
					if line != row {
						t.Fatalf("%s: controller line differs from the shared helper: %q vs %q", label, line, row)
					}
					effectRowBasenameAndTokensIntact(t, label+" (controller line)", line, effect)
					if phrase != "" && disclosure != "lockfile_fixed_point="+string(fp)+" ("+phrase+") path="+path {
						t.Fatalf("%s: controller disclosure line must keep the whole phrase and the whole path: %q", label, disclosure)
					}
					if phrase == "" && disclosure != "" {
						t.Fatalf("%s: proven/disproven rows carry no disclosure line: %q", label, disclosure)
					}
				}
			}
		}
	}
}
