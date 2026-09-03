package types

import (
	"strings"
	"testing"
)

// verification_worktree_drift_fold_in3_test.go — F-run-tests round-three
// fold-in of V5-2 (§40.36 三轮收编): the closed fixed-point set gains the
// unproven_run_refused member with plain-words zh/en phrases and a Declared
// predicate (A); the write-handoff effect row and the dedicated disclosure
// item put typed tokens first and the path last, fitted rune-safely to the
// per-item bound so no token or phrase is ever cut (E).

func TestVerificationLockfileFixedPointRunRefusedIsADeclaredUnprovenMember(t *testing.T) {
	fp := VerificationLockfileFixedPointUnprovenRunRefused
	if !VerificationLockfileFixedPointDeclared(fp) || !VerificationLockfileFixedPointUnproven(fp) {
		t.Fatalf("unproven_run_refused must be a declared unproven member")
	}
	zh, en := VerificationLockfileFixedPointDisclosure(fp, true), VerificationLockfileFixedPointDisclosure(fp, false)
	if !strings.Contains(zh, "未证明") || !strings.Contains(zh, "被拒绝") || !strings.Contains(en, "UNPROVEN") || !strings.Contains(en, "refused for other paths") {
		t.Fatalf("run-refused phrases must say unproven + refused in plain words: zh=%q en=%q", zh, en)
	}
	if strings.ContainsAny(zh, ",") || strings.ContainsAny(en, ",") {
		t.Fatalf("phrases stay comma-free: zh=%q en=%q", zh, en)
	}
	// The zero value is never a declared state: it belongs to rows of other
	// classes only and renders no phrase (a lockfile row carrying it would
	// be indistinguishable from proven — the producer census in the tool
	// package refuses that).
	if VerificationLockfileFixedPointDeclared("") || VerificationLockfileFixedPointDeclared("proven_by_prose") {
		t.Fatalf("only closed-set members are declared")
	}
	for _, member := range AllVerificationLockfileFixedPoints() {
		if !VerificationLockfileFixedPointDeclared(member) {
			t.Fatalf("closed-set member %q must be declared", member)
		}
	}
}

var longLockfilePaths = []string{
	"Cargo.lock",
	"pnpm-lock.yaml",
	"package-lock.json",
	"crates/foo/Cargo.lock",
	"services/backend/packages/api-gateway/package-lock.json",
}

// pathIsLastElementOf reports whether text ends with " path=<p>" where <p>
// is the whole path or, when the item bound forced a cut, "…" plus a
// non-empty TAIL of the path (the basename end) — never a cut token.
func pathIsLastElementOf(text, path string) bool {
	idx := strings.LastIndex(text, " path=")
	if idx < 0 {
		return false
	}
	seg := text[idx+len(" path="):]
	if seg == path {
		return true
	}
	tail := strings.TrimPrefix(seg, "…")
	return tail != seg && tail != "" && strings.HasSuffix(path, tail)
}

func lockfileEffectForPath(path string, fp VerificationLockfileFixedPoint) VerificationWorktreeEffect {
	return VerificationWorktreeEffect{
		Path: path, Kind: VerificationWorktreeEffectTrackedChanged, Ownership: "git_tracked",
		Action: "disclosed_not_committed_not_auto_reverted", DriftClass: VerificationWorktreeDriftDependencyLockfileRefresh,
		OwnerRunner: "rust", Disposition: VerificationWorktreeEffectDisclosed, LockfileFixedPoint: fp,
	}
}

// E: on every long / deep lockfile path, the effect row keeps every
// rendered typed token intact, ends with the path (last element, basename
// whole), stays within the item bound (so trimWriteContextText is a no-op
// — no "..." mangling), and the dedicated disclosure item keeps the whole
// phrase and the whole path.
//
// EVOLUTION RECORD (fold-in round four, finding M): the row used to keep
// every token and cut INTO the basename ("path=…lock.yaml"); the basename
// is now never cut — under the longest fixed-point member the row drops
// whole tokens by priority instead (action first), and the dedicated
// disclosure item is no longer fitted to the bound at all (whole path).
func TestWriteContextPackWorktreeEffectRowsKeepTypedTokensForLongLockfilePaths(t *testing.T) {
	tokens := []string{"kind=tracked_changed", "ownership=git_tracked", "action=disclosed_not_committed_not_auto_reverted",
		"drift_class=dependency_lockfile_refresh", "disposition=disclosed", "owner_runner=rust", "lockfile_fixed_point=unproven_suite_infra_downgraded"}
	phrase := VerificationLockfileFixedPointDisclosure(VerificationLockfileFixedPointUnprovenSuiteInfraDowngraded, false)
	for _, path := range longLockfilePaths {
		effect := lockfileEffectForPath(path, VerificationLockfileFixedPointUnprovenSuiteInfraDowngraded)
		pack := WriteContextPackFromChangeReport(&ChangeReport{PlanID: "plan-e", Passed: true,
			WorktreeAudit: &VerificationWorktreeAudit{Status: VerificationWorktreeAuditTrackedDriftDisclosed, Effects: []VerificationWorktreeEffect{effect}}})
		var row, item string
		for _, it := range pack.Items {
			switch it.Kind {
			case "verification_worktree_effect":
				row = it.Text
			case "verification_lockfile_fixed_point":
				item = it.Text
			}
		}
		if row == "" || item == "" {
			t.Fatalf("%s: effect row / disclosure item missing: %+v", path, pack.Items)
		}
		if row != WriteContextWorktreeEffectText(effect) || strings.HasSuffix(row, "...") || len([]rune(row)) > writeContextPackTextLen {
			t.Fatalf("%s: effect row must survive the item bound untrimmed (%d runes): %q", path, len([]rune(row)), row)
		}
		assertEffectRowTokensWholeAndBasenameIntact(t, row, tokens, path)
		if strings.Index(row, " path=") < strings.Index(row, "lockfile_fixed_point=") {
			t.Fatalf("%s: the path must be the LAST element, after every token: %q", path, row)
		}
		if item != WriteContextLockfileFixedPointDisclosureText(path, VerificationLockfileFixedPointUnprovenSuiteInfraDowngraded) ||
			!strings.HasPrefix(item, "lockfile_fixed_point=unproven_suite_infra_downgraded (") || !strings.Contains(item, phrase) ||
			!strings.HasSuffix(item, ") path="+path) {
			t.Fatalf("%s: disclosure item must keep token + whole phrase and end with the WHOLE path: %q", path, item)
		}
		if path == "Cargo.lock" && !strings.HasSuffix(row, " path="+path) {
			t.Fatalf("%s: a path that fits is kept whole: %q", path, row)
		}
	}
	// A path too long for the remaining budget is shortened from the left
	// (basename kept), never the tokens.
	deep := strings.Repeat("segment/", 40) + "package-lock.json"
	row := WriteContextWorktreeEffectText(lockfileEffectForPath(deep, VerificationLockfileFixedPointProven))
	if len([]rune(row)) != writeContextPackTextLen || !strings.HasSuffix(row, "/package-lock.json") || !strings.Contains(row, " path=…") ||
		!strings.Contains(row, "lockfile_fixed_point=proven ") {
		t.Fatalf("deep path must be shortened from the left within the bound: %q", row)
	}
}

// assertEffectRowTokensWholeAndBasenameIntact pins the fitted-row contract:
// the segment after " path=" is the whole path or "…" plus a suffix that
// still contains the whole basename; every "key=value" element before it
// is exactly one of the expected tokens (never a truncated one); and the
// tokens present are a priority-consistent subset (a dropped token never
// outranks a kept one).
func assertEffectRowTokensWholeAndBasenameIntact(t *testing.T, row string, expected []string, path string) {
	t.Helper()
	idx := strings.LastIndex(row, " path=")
	if idx < 0 && !strings.HasPrefix(row, "path=") {
		t.Fatalf("row lacks the path element: %q", row)
	}
	var seg, head string
	if idx < 0 {
		seg, head = strings.TrimPrefix(row, "path="), ""
	} else {
		seg, head = row[idx+len(" path="):], row[:idx]
	}
	base := path
	if slash := strings.LastIndex(path, "/"); slash >= 0 {
		base = path[slash:]
	}
	if seg != path {
		tail := strings.TrimPrefix(seg, "…")
		if tail == seg || tail == "" || !strings.HasSuffix(path, tail) || !strings.HasSuffix(tail, base) {
			t.Fatalf("path segment %q cuts into the basename of %q: %q", seg, path, row)
		}
	}
	rank := func(token string) int {
		for i, want := range expected {
			if want == token {
				return i
			}
		}
		return -1
	}
	present := map[int]bool{}
	if head != "" {
		for _, token := range strings.Split(head, " ") {
			r := rank(token)
			if r < 0 {
				t.Fatalf("row carries a token that is not a whole expected token: %q in %q", token, row)
			}
			present[r] = true
		}
	}
	// Priority ladder in display order: kind(3) ownership(2) action(1)
	// drift_class(6) disposition(5) owner_runner(4) lockfile_fixed_point(7).
	priority := []int{3, 2, 1, 6, 5, 4, 7}
	for i := range expected {
		if present[i] {
			continue
		}
		for j := range expected {
			if present[j] && priority[j] < priority[i] {
				t.Fatalf("token %q was dropped while the lower-priority %q was kept: %q", expected[i], expected[j], row)
			}
		}
	}
}

// A (final report): a refused run's disclosed lockfile row carries the
// run-refused phrase in the warning beside the refused error.
func TestWriteFinalResidualRisksNameRunRefusedLockfileFixedPoint(t *testing.T) {
	report := WriteFinalReport{}
	report.Verification.WorktreeAuditStatus = VerificationWorktreeAuditTrackedDrift
	report.Verification.WorktreeEffects = []VerificationWorktreeEffect{
		lockfileEffectForPath("Cargo.lock", VerificationLockfileFixedPointUnprovenRunRefused),
		{Path: "src/other.rs", Kind: VerificationWorktreeEffectTrackedChanged, DriftClass: VerificationWorktreeDriftUnclassified, Disposition: VerificationWorktreeEffectRefused},
	}
	risks := writeFinalResidualRisks(report)
	want := VerificationLockfileFixedPointDisclosure(VerificationLockfileFixedPointUnprovenRunRefused, false)
	if !writeFinalHasRisk(risks, VerificationTrackedSideEffectDisclosedReason, "warning", "Cargo.lock=dependency_lockfile_refresh ["+want+"]") {
		t.Fatalf("the disclosed warning must name the run-refused fixed point: %+v", risks)
	}
	if !writeFinalHasRisk(risks, "verification_worktree_tracked_drift", "error", "src/other.rs") {
		t.Fatalf("refused row stays the error: %+v", risks)
	}
}
