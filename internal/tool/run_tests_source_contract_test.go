package tool

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// run_tests_source_contract_test.go — V5-1 (colleague_merge_audit §40.10 /
// §40.35): the post-apply source arm is a `source_text` witness. It satisfies
// only contract kinds that admit that witness (file_layout), only at a line
// bound through the CUMULATIVE diff from the worktree base commit (the tree
// the analyzer took every evidence_ref from) to HEAD; everything else is an
// advisory disclosure in plain words.

func sourceContractRepo(t *testing.T, files map[string]string) (root, base string) {
	t.Helper()
	root = newVerificationWorktreeRepo(t)
	for rel, body := range files {
		writeVerificationWorktreeFile(t, root, rel, body)
	}
	sourceContractCommit(t, root, "base")
	return root, sourceContractHead(t, root)
}

func sourceContractCommit(t *testing.T, root, msg string) {
	t.Helper()
	runVerificationWorktreeGit(t, root, "add", "-A")
	runVerificationWorktreeGit(t, root, "commit", "-q", "-m", msg)
}

func sourceContractHead(t *testing.T, root string) string {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", "rev-parse", "HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func sourceContractPlan(path string, contract types.WriteBehaviorContract) *types.ChangePlan {
	return &types.ChangePlan{ID: "plan-source-contract", TargetPaths: []string{path}, BehaviorContracts: []types.WriteBehaviorContract{contract}}
}

func layoutContract(id, ref, expected string) types.WriteBehaviorContract {
	return types.WriteBehaviorContract{ID: id, Kind: types.WriteBehaviorFileLayout, Polarity: types.WriteBehaviorPolarityExpected,
		Operator: types.WriteBehaviorOpEquals, Expected: expected, EvidenceRef: ref, Required: true}
}

func assertNoInternalRemapNames(t *testing.T, records []types.VerificationConfidenceRecord) {
	t.Helper()
	for _, rec := range records {
		for _, leak := range []string{"no_patch_effect", "hunk_unmapped", "file_not_in_patch", "removed_by_patch", "file_created", "file_deleted", "invalid_line", "PatchEffect"} {
			if strings.Contains(rec.Detail, leak) {
				t.Fatalf("record detail leaks the internal remap name %q: %q", leak, rec.Detail)
			}
		}
	}
}

func TestPostApplySourceContractConfidenceRecordsObservesPlanOwnedExactLine(t *testing.T) {
	root, base := sourceContractRepo(t, map[string]string{"src/main.c": "void greet(void) {\n    char *buf = 0;\n    return buf;\n}\n"})
	writeVerificationWorktreeFile(t, root, "unrelated.txt", "applied elsewhere\n")
	sourceContractCommit(t, root, "apply")
	plan := sourceContractPlan("src/main.c", layoutContract("return-line", "src/main.c:3", "return buf;"))
	records := postApplySourceContractConfidenceRecords(root, base, nil, plan)
	if len(records) != 1 || records[0].Category != "source_contract_refs" || records[0].Status != "satisfied" ||
		records[0].ReasonCode != "post_apply_source_contract_observed" || records[0].WitnessKind != types.WriteBehaviorWitnessSourceText ||
		len(records[0].ContractRefs) != 1 || records[0].ContractRefs[0] != "return-line" {
		t.Fatalf("an untouched plan-owned file keeps its numbering and satisfies the file_layout contract: %+v", records)
	}
}

// §40.10 pin ③ (runtime half): over every known kind, the source arm mints
// `satisfied` iff the matrix admits the source_text witness.
func TestPostApplySourceContractHonoursWitnessMatrixOverEveryKind(t *testing.T) {
	root, base := sourceContractRepo(t, map[string]string{"main.c": "int a;\ntimeout = 10;\n"})
	for _, kind := range types.AllWriteBehaviorContractKinds() {
		plan := sourceContractPlan("main.c", types.WriteBehaviorContract{ID: "retries-zero", Kind: kind, Polarity: types.WriteBehaviorPolarityExpected,
			Operator: types.WriteBehaviorOpContains, Expected: "0", EvidenceRef: "main.c:2", Required: true})
		records := postApplySourceContractConfidenceRecords(root, base, nil, plan)
		if len(records) != 1 || records[0].WitnessKind != types.WriteBehaviorWitnessSourceText || len(records[0].ContractRefs) != 1 {
			t.Fatalf("kind %s: want one stamped record, got %+v", kind, records)
		}
		rec := records[0]
		if types.WriteBehaviorWitnessSatisfies(kind, types.WriteBehaviorWitnessSourceText) {
			if rec.Category != "source_contract_refs" || rec.Status != "satisfied" {
				t.Fatalf("kind %s admits source text and must be satisfied: %+v", kind, rec)
			}
			continue
		}
		if rec.Category != "source_text_presence" || rec.Status != "advisory" || rec.ReasonCode != "post_apply_source_text_present" {
			t.Fatalf("kind %s: `timeout = 10` containing '0' must be an advisory presence, never satisfied: %+v", kind, rec)
		}
	}
}

// §40.10 pin ①: three inserted lines shift the evidence line. Without a known
// base the stale line (`timeout = 10`, contains '0') is never read; with it
// the line is bound to its post-apply position through the base→HEAD diff.
func TestPostApplySourceContractStaleLineIsBoundThroughBase(t *testing.T) {
	root, base := sourceContractRepo(t, map[string]string{"main.c": "int a;\nint b;\nretries = 0;\n"})
	writeVerificationWorktreeFile(t, root, "main.c", "// added 1\n// added 2\ntimeout = 10;\nint a;\nint b;\nretries = 0;\n")
	sourceContractCommit(t, root, "apply")
	plan := sourceContractPlan("main.c", layoutContract("retries-zero", "main.c:3", "retries = 0;"))
	records := postApplySourceContractConfidenceRecords(root, "", nil, plan)
	if len(records) != 1 || records[0].Category != "source_text_presence" || records[0].Status != "advisory" ||
		records[0].ReasonCode != "post_apply_source_contract_line_unresolved" {
		t.Fatalf("an unknown base must be an unresolved disclosure: %+v", records)
	}
	assertNoInternalRemapNames(t, records)
	records = postApplySourceContractConfidenceRecords(root, base, nil, plan)
	if len(records) != 1 || records[0].Status != "satisfied" || records[0].ReasonCode != "post_apply_source_contract_observed" {
		t.Fatalf("the base-bound line 6 must satisfy the file_layout contract: %+v", records)
	}
	observable := layoutContract("retries-zero", "main.c:3", "0")
	observable.Kind, observable.Operator = types.WriteBehaviorObservable, types.WriteBehaviorOpContains
	records = postApplySourceContractConfidenceRecords(root, base, nil, sourceContractPlan("main.c", observable))
	if len(records) != 1 || records[0].Status != "advisory" || records[0].ReasonCode != "post_apply_source_text_present" {
		t.Fatalf("shifted observable contract must stay advisory: %+v", records)
	}
}

// Review fold-in (§40.35 复核 ★multi-slice/multi-unit/earlier-plan): the plan
// only carries the LAST slice's patch record, and earlier plans in the same
// workflow are not on it at all; the binding must still see every commit
// since the base.
func TestPostApplySourceContractBindsThroughCumulativeHistoryNotTheLastSlice(t *testing.T) {
	root, base := sourceContractRepo(t, map[string]string{"main.c": "int a;\nint b;\nretries = 0;\n", "other.c": "int c;\n"})
	// slice 1 (or an earlier plan): shift main.c by three lines
	writeVerificationWorktreeFile(t, root, "main.c", "// s1\n// s1\ntimeout = 10;\nint a;\nint b;\nretries = 0;\n")
	sourceContractCommit(t, root, "slice-1")
	// slice 2 (active): touches other.c only; plan.PatchEffect describes it alone
	writeVerificationWorktreeFile(t, root, "other.c", "int c;\nint d;\n")
	sourceContractCommit(t, root, "slice-2")
	plan := sourceContractPlan("main.c", layoutContract("retries-zero", "main.c:3", "retries = 0;"))
	plan.TargetPaths = []string{"main.c", "other.c"}
	plan.PatchEffect = &types.PatchEffectRecord{SliceID: "s2", Files: []types.PatchEffectFile{{Path: "other.c", OldPath: "other.c", Status: "modified",
		Hunks: []types.PatchEffectHunk{{OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 2, AddedLines: 1, AddedLineNumbers: []int{2}}}}}}
	records := postApplySourceContractConfidenceRecords(root, base, nil, plan)
	if len(records) != 1 || records[0].Status != "satisfied" {
		t.Fatalf("slice-1's shift must be honoured although only slice-2 is on the plan: %+v", records)
	}
	stale := layoutContract("timeout-line", "main.c:3", "10")
	stale.Kind, stale.Operator = types.WriteBehaviorObservable, types.WriteBehaviorOpContains
	records = postApplySourceContractConfidenceRecords(root, base, nil, sourceContractPlan("main.c", stale))
	if len(records) != 1 || records[0].Status != "advisory" || records[0].ReasonCode != "post_apply_source_text_absent" {
		t.Fatalf("the stale line 3 (`timeout = 10`) must never be read; bound line 6 lacks '10': %+v", records)
	}
	// A contract retained from an earlier plan (cumulative scope) binds the same way.
	retained := &types.ChangePlan{ID: "plan-later", TargetPaths: []string{"other.c"},
		CumulativeVerificationScope: &types.CumulativeVerificationScope{SourcePlanIDs: []string{"earlier"}, TargetPaths: []string{"main.c"},
			BehaviorContracts: []types.WriteBehaviorContract{layoutContract("retries-zero", "main.c:3", "retries = 0;")}}}
	records = postApplySourceContractConfidenceRecords(root, base, nil, retained)
	if len(records) != 1 || records[0].Status != "satisfied" {
		t.Fatalf("a retained contract binds through the same cumulative history: %+v", records)
	}
}

func TestPostApplySourceContractBindsGitQuotedNonASCIIPaths(t *testing.T) {
	root, base := sourceContractRepo(t, map[string]string{"文档.c": "line1\nline2\nline3\n"})
	writeVerificationWorktreeFile(t, root, "文档.c", "x\ny\nz\nline1\nline2\nline3\n")
	sourceContractCommit(t, root, "apply")
	plan := sourceContractPlan("文档.c", layoutContract("l3", "文档.c:3", "line3"))
	records := postApplySourceContractConfidenceRecords(root, base, nil, plan)
	if len(records) != 1 || records[0].Status != "satisfied" {
		t.Fatalf("a non-ASCII path must bind (line 3 → 6), not fall back to the stale line: %+v", records)
	}
}

func TestPostApplySourceContractUnboundLinesAreUnresolvedNotRead(t *testing.T) {
	root, base := sourceContractRepo(t, map[string]string{"main.c": "return buf;\nreturn buf;\nint z;\n"})
	assertUnresolved := func(name string, records []types.VerificationConfidenceRecord) {
		t.Helper()
		if len(records) != 1 || records[0].Status != "advisory" || records[0].ReasonCode != "post_apply_source_contract_line_unresolved" {
			t.Fatalf("%s: must be an unresolved disclosure, got %+v", name, records)
		}
		assertNoInternalRemapNames(t, records)
	}
	// The evidence line was removed by the change.
	writeVerificationWorktreeFile(t, root, "main.c", "return buf;\nint z;\n")
	sourceContractCommit(t, root, "remove")
	assertUnresolved("removed line", postApplySourceContractConfidenceRecords(root, base, nil, sourceContractPlan("main.c", layoutContract("l2", "main.c:2", "return buf;"))))
	// The file did not exist at the base.
	writeVerificationWorktreeFile(t, root, "new.c", "return buf;\n")
	sourceContractCommit(t, root, "create")
	assertUnresolved("created file", postApplySourceContractConfidenceRecords(root, base, nil, sourceContractPlan("new.c", layoutContract("n1", "new.c:1", "return buf;"))))
	// The base ref is not a commit of this checkout.
	assertUnresolved("unknown base", postApplySourceContractConfidenceRecords(root, "0123456789abcdef0123456789abcdef01234567", nil, sourceContractPlan("main.c", layoutContract("l1", "main.c:1", "return buf;"))))
}

func TestPostApplySourceContractConfidenceRecordsReportsExactLineMismatch(t *testing.T) {
	root, base := sourceContractRepo(t, map[string]string{"main.c": "retrun buf;\n"})
	records := postApplySourceContractConfidenceRecords(root, base, nil, sourceContractPlan("main.c", layoutContract("return-line", "main.c:1", "return buf;")))
	if len(records) != 1 || records[0].Category != "source_contract_refs" || records[0].Status != "missing" ||
		records[0].ReasonCode != "post_apply_source_contract_value_mismatch" {
		t.Fatalf("exact file_layout mismatch was not retained as typed debt: %+v", records)
	}
	invariant := layoutContract("return-line", "main.c:1", "return buf;")
	invariant.Kind = types.WriteBehaviorInvariant
	records = postApplySourceContractConfidenceRecords(root, base, nil, sourceContractPlan("main.c", invariant))
	if len(records) != 1 || records[0].Category != "source_text_presence" || records[0].Status != "advisory" ||
		records[0].ReasonCode != "post_apply_source_text_absent" {
		t.Fatalf("runtime-kind mismatch must be an advisory absence: %+v", records)
	}
}

func TestPostApplySourceContractConfidenceRecordsRejectsRuntimeRangeAndUnownedContracts(t *testing.T) {
	root, base := sourceContractRepo(t, map[string]string{"main.c": "return buf;\n"})
	plan := &types.ChangePlan{ID: "plan-source-boundaries", TargetPaths: []string{"main.c"},
		BehaviorContracts: []types.WriteBehaviorContract{
			{ID: "runtime-return", Kind: types.WriteBehaviorObservable, Polarity: types.WriteBehaviorPolarityExpected,
				Operator: types.WriteBehaviorOpReturns, Expected: "buf", EvidenceRef: "main.c:1", Required: true},
			{ID: "line-range", Kind: types.WriteBehaviorObservable, Polarity: types.WriteBehaviorPolarityExpected,
				Operator: types.WriteBehaviorOpContains, Expected: "return", EvidenceRef: "main.c:1-2", Required: true},
			{ID: "unowned", Kind: types.WriteBehaviorFileLayout, Polarity: types.WriteBehaviorPolarityExpected,
				Operator: types.WriteBehaviorOpEquals, Expected: "return buf;", EvidenceRef: "other.c:1", Required: true},
		}}
	if records := postApplySourceContractConfidenceRecords(root, base, nil, plan); len(records) != 0 {
		t.Fatalf("runtime-operator, range, or unowned contracts gained source records: %+v", records)
	}
}

func TestPostApplySourceContractConfidenceRecordsRejectsSymlinkEscape(t *testing.T) {
	root, base := sourceContractRepo(t, map[string]string{"keep.txt": "x\n"})
	outside := filepath.Join(t.TempDir(), "outside.c")
	if err := os.WriteFile(outside, []byte("return buf;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "main.c")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	records := postApplySourceContractConfidenceRecords(root, base, nil, sourceContractPlan("main.c", layoutContract("escaped", "main.c:1", "return buf;")))
	for _, rec := range records {
		if rec.Status == "satisfied" || rec.Status == "missing" {
			t.Fatalf("symlink escape gained source authority: %+v", records)
		}
	}
	if len(records) != 1 || records[0].ReasonCode != "post_apply_source_contract_line_unresolved" {
		t.Fatalf("an unreadable bound line is disclosed, never silent: %+v", records)
	}
}

// Review pass two (§40.35 复核二): the witness reads the WORKING TREE, so the
// binding diffs base → working tree — an apply whose checkpoint commit failed
// (pre-commit hooks inherited by the worktree) still binds correctly.
func TestPostApplySourceContractBindsUncommittedApplyBytes(t *testing.T) {
	root, base := sourceContractRepo(t, map[string]string{"cfg.py": "l1\nl2\nl3\ntimeout = 10\n"})
	writeVerificationWorktreeFile(t, root, "cfg.py", "ins1\nins2\nl1\nl2\nl3\ntimeout = 10\n") // NOT committed
	records := postApplySourceContractConfidenceRecords(root, base, nil, sourceContractPlan("cfg.py", layoutContract("c1", "cfg.py:4", "timeout = 10")))
	if len(records) != 1 || records[0].Status != "satisfied" {
		t.Fatalf("uncommitted apply bytes must bind 4 → 6: %+v", records)
	}
	stale := layoutContract("c2", "cfg.py:4", "l2")
	stale.Operator = types.WriteBehaviorOpContains
	records = postApplySourceContractConfidenceRecords(root, base, nil, sourceContractPlan("cfg.py", stale))
	if len(records) != 1 || records[0].Status != "missing" {
		t.Fatalf("the stale line 4 (`l2`) must never be read as the bound line: %+v", records)
	}
}

// Files dirty in the main checkout at analysis time have no base tree.
func TestPostApplySourceContractRefusesFilesDirtyAtAnalysis(t *testing.T) {
	root, base := sourceContractRepo(t, map[string]string{"cfg.py": "l1\nl2\n"})
	records := postApplySourceContractConfidenceRecords(root, base, []string{"cfg.py"}, sourceContractPlan("cfg.py", layoutContract("c1", "cfg.py:1", "l1")))
	if len(records) != 1 || records[0].Status != "advisory" || records[0].ReasonCode != "post_apply_source_contract_line_unresolved" ||
		!strings.Contains(records[0].Detail, "uncommitted changes in the main checkout") {
		t.Fatalf("a base-divergent file must be unresolved with the plain reason: %+v", records)
	}
	assertNoInternalRemapNames(t, records)
}

// Renames pair through the whole-tree diff; binary rewrites have no line
// table; untracked creations have no base line; user git config cannot bend
// the diff shape.
func TestPostApplySourceContractBindsRenamesBinariesCreationsAndPinnedConfig(t *testing.T) {
	root, base := sourceContractRepo(t, map[string]string{"src/old.c": "int a;\nint b;\nretries = 0;\n", "blob.c": "int x;\n"})
	for _, kv := range [][2]string{{"diff.noprefix", "true"}, {"diff.mnemonicPrefix", "true"}, {"diff.external", "/usr/bin/false"}, {"diff.suppressBlankEmpty", "true"}} {
		runVerificationWorktreeGit(t, root, "config", kv[0], kv[1])
	}
	runVerificationWorktreeGit(t, root, "mv", "src/old.c", "src/new.c")
	writeVerificationWorktreeFile(t, root, "src/new.c", "// moved\nint a;\nint b;\nretries = 0;\n")
	writeVerificationWorktreeFile(t, root, "blob.c", "int x;\x00\x01binary\n")
	writeVerificationWorktreeFile(t, root, "fresh.c", "retries = 0;\n") // untracked
	plan := &types.ChangePlan{ID: "p", TargetPaths: []string{"src/old.c", "src/new.c", "blob.c", "fresh.c"}, BehaviorContracts: []types.WriteBehaviorContract{
		layoutContract("moved", "src/old.c:3", "retries = 0;"),
		layoutContract("binary", "blob.c:1", "int x;"),
		layoutContract("fresh", "fresh.c:1", "retries = 0;"),
	}}
	byID := map[string]types.VerificationConfidenceRecord{}
	for _, rec := range postApplySourceContractConfidenceRecords(root, base, nil, plan) {
		byID[rec.ContractRefs[0]] = rec
	}
	if rec := byID["moved"]; rec.Status != "satisfied" {
		t.Fatalf("a renamed file must bind to its new path (old line 3 → new line 4): %+v", rec)
	}
	if rec := byID["binary"]; rec.Status != "advisory" || rec.ReasonCode != "post_apply_source_contract_line_unresolved" || !strings.Contains(rec.Detail, "rewritten region") {
		t.Fatalf("a binary rewrite has no line table: %+v", rec)
	}
	if rec := byID["fresh"]; rec.Status != "advisory" || !strings.Contains(rec.Detail, "did not exist before the change") {
		t.Fatalf("an untracked creation has no base line: %+v", rec)
	}
}

func TestPostApplySourceContractBindsPathsWithSpaces(t *testing.T) {
	root, base := sourceContractRepo(t, map[string]string{"sp ace.c": "l1\nl2\nl3\n"})
	writeVerificationWorktreeFile(t, root, "sp ace.c", "x\ny\nl1\nl2\nl3\n")
	records := postApplySourceContractConfidenceRecords(root, base, nil, sourceContractPlan("sp ace.c", layoutContract("s", "sp ace.c:3", "l3")))
	if len(records) != 1 || records[0].Status != "satisfied" {
		t.Fatalf("a path with spaces must bind (3 → 5): %+v", records)
	}
}
