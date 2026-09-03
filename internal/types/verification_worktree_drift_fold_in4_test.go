package types

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

// verification_worktree_drift_fold_in4_test.go — fold-in round four of V5-2
// (colleague_merge_audit §40.36 四轮收编):
//   - L: the locked re-verify skip label and the lockfile fixed-point state
//     are decided from the OWNER SEAT (its own non-zero exit), never from the
//     report-level verdict; skipped_report_failed keeps its published wire
//     bytes with the seat condition documented, the unpushed
//     unproven_report_failed is renamed unproven_suite_failed and its zh/en
//     phrase names the seat's non-zero exit;
//   - M: the write-handoff effect row never cuts into the path basename —
//     the directory prefix shrinks first, then whole tokens drop by priority
//     (action, ownership, kind, owner_runner, disposition, drift_class,
//     lockfile_fixed_point); the dedicated disclosure item keeps the whole
//     phrase and the whole path, and neither typed row is ever trimmed with
//     "..." by pack normalisation.

// declaredStringConstants reads `Name Type = "value"` / `Name = "value"`
// constants of the given type name (or untyped when typeName is "") from
// one source file of this package.
func declaredStringConstants(t *testing.T, fileName, typeName string) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, fileName, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			typed := ""
			if ident, ok := vs.Type.(*ast.Ident); ok {
				typed = ident.Name
			}
			if typed != typeName {
				continue
			}
			for i, name := range vs.Names {
				if i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				value, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatal(err)
				}
				out[name.Name] = value
			}
		}
	}
	return out
}

// L: the closed fixed-point set is exactly the declared constants (go/ast),
// the wire bytes are pinned (skipped_report_failed is published and
// append-only; unproven_suite_failed replaced the unpushed
// unproven_report_failed), and the zh/en phrase of the seat-failed state
// names the owner suite's non-zero exit — never the report.
func TestVerificationLockfileFixedPointSuiteFailedIsKeyedOnTheOwnerSeat(t *testing.T) {
	declared := declaredStringConstants(t, "verification_worktree_drift.go", "VerificationLockfileFixedPoint")
	all := AllVerificationLockfileFixedPoints()
	if len(all) != len(declared) {
		t.Fatalf("AllVerificationLockfileFixedPoints has %d members, %d constants declared: %v", len(all), len(declared), declared)
	}
	for name, value := range declared {
		if !VerificationLockfileFixedPointDeclared(VerificationLockfileFixedPoint(value)) {
			t.Fatalf("declared constant %s (%q) is not a member of the closed set", name, value)
		}
	}
	wantValues := map[string]string{
		"VerificationLockfileFixedPointProven":                       "proven",
		"VerificationLockfileFixedPointDisproven":                    "disproven",
		"VerificationLockfileFixedPointUnprovenSuiteInfraDowngraded": "unproven_suite_infra_downgraded",
		"VerificationLockfileFixedPointUnprovenSuiteFailed":          "unproven_suite_failed",
		"VerificationLockfileFixedPointUnprovenRunRefused":           "unproven_run_refused",
	}
	for name, value := range wantValues {
		if declared[name] != value {
			t.Fatalf("%s = %q, want %q (declared: %v)", name, declared[name], value, declared)
		}
	}
	if _, stale := declared["VerificationLockfileFixedPointUnprovenReportFailed"]; stale {
		t.Fatal("unproven_report_failed was renamed to unproven_suite_failed (unpushed label)")
	}
	// The locked re-verify record labels: published bytes append-only.
	records := declaredStringConstants(t, "verification_worktree_drift.go", "")
	for name, value := range map[string]string{
		"VerificationLockedReverifyPassed":                      "passed",
		"VerificationLockedReverifyFailed":                      "failed",
		"VerificationLockedReverifyDriftRecurred":               "drift_recurred",
		"VerificationLockedReverifyUnavailable":                 "unavailable",
		"VerificationLockedReverifySkippedReportFailed":         "skipped_report_failed",
		"VerificationLockedReverifySkippedSuiteInfraDowngraded": "skipped_suite_infra_downgraded",
	} {
		if records[name] != value {
			t.Fatalf("%s = %q, want the published wire bytes %q", name, records[name], value)
		}
	}
	fp := VerificationLockfileFixedPointUnprovenSuiteFailed
	if !VerificationLockfileFixedPointUnproven(fp) {
		t.Fatal("unproven_suite_failed must be an unproven member")
	}
	zh, en := VerificationLockfileFixedPointDisclosure(fp, true), VerificationLockfileFixedPointDisclosure(fp, false)
	if zh != "锁文件定点未证明：该测试座位的套件以非零退出码结束，未执行锁定复验" ||
		en != "lockfile fixed point UNPROVEN: the owner suite exited non-zero so no locked re-run was attempted" {
		t.Fatalf("the seat-failed phrase must name the owner suite's non-zero exit: zh=%q en=%q", zh, en)
	}
	for _, phrase := range []string{zh, en} {
		if strings.Contains(strings.ToLower(phrase), "report") || strings.Contains(phrase, "报告") || strings.ContainsAny(phrase, ",") {
			t.Fatalf("the phrase must not describe the report verdict and stays comma-free: %q", phrase)
		}
	}
}

// M: the fitting ladder. With a basename that leaves no room, tokens are
// dropped WHOLE in priority order and the basename is rendered intact;
// with a deep directory the prefix shrinks first and every token stays.
func TestWriteContextWorktreeEffectRowNeverCutsTheBasename(t *testing.T) {
	tokens := []string{"kind=tracked_changed", "ownership=git_tracked", "action=disclosed_not_committed_not_auto_reverted",
		"drift_class=dependency_lockfile_refresh", "disposition=disclosed", "owner_runner=swift", "lockfile_fixed_point=unproven_suite_infra_downgraded"}
	effect := func(path string) VerificationWorktreeEffect {
		e := lockfileEffectForPath(path, VerificationLockfileFixedPointUnprovenSuiteInfraDowngraded)
		e.OwnerRunner = "swift"
		return e
	}
	// The real swift owner under the longest member used to render
	// "path=…resolved": the basename is now whole and only `action`
	// (the lowest-priority token) is dropped.
	row := WriteContextWorktreeEffectText(effect("Package.resolved"))
	if !strings.HasSuffix(row, " path=Package.resolved") || strings.Contains(row, "action=") || !strings.Contains(row, " owner_runner=swift ") ||
		!strings.Contains(row, " lockfile_fixed_point=unproven_suite_infra_downgraded ") || len([]rune(row)) > writeContextPackTextLen {
		t.Fatalf("swift Package.resolved row: %q", row)
	}
	assertEffectRowTokensWholeAndBasenameIntact(t, row, tokens, "Package.resolved")
	// Nested: the directory shrinks to "…/Package.resolved" before any
	// further token is dropped.
	row = WriteContextWorktreeEffectText(effect("apps/ios/Package.resolved"))
	if !strings.HasSuffix(row, "/Package.resolved") || len([]rune(row)) > writeContextPackTextLen {
		t.Fatalf("nested swift row: %q", row)
	}
	assertEffectRowTokensWholeAndBasenameIntact(t, row, tokens, "apps/ios/Package.resolved")
	// Ladder: a basename that consumes the whole budget drops every token
	// in ascending priority; the basename is still whole.
	long := strings.Repeat("x", 170) + ".lock"
	row = WriteContextWorktreeEffectText(effect(long))
	if !strings.HasSuffix(row, " path="+long) && row != "path="+long {
		t.Fatalf("long basename must be whole: %q", row)
	}
	assertEffectRowTokensWholeAndBasenameIntact(t, row, tokens, long)
	if !strings.Contains(row, "lockfile_fixed_point=unproven_suite_infra_downgraded") || strings.Contains(row, "drift_class=") {
		t.Fatalf("with 60 runes of token budget only the highest-priority token survives: %q", row)
	}
	for want, budget := range map[string]int{
		"lockfile_fixed_point=unproven_suite_infra_downgraded drift_class=dependency_lockfile_refresh": 100,
		"drift_class=dependency_lockfile_refresh disposition=disclosed owner_runner=swift":             150,
	} {
		base := strings.Repeat("y", writeContextPackTextLen-len("path=")-budget-1) + "z"
		row := WriteContextWorktreeEffectText(effect(base))
		for _, token := range strings.Split(want, " ") {
			if !strings.Contains(row, token+" ") {
				t.Fatalf("budget %d: token %q must survive: %q", budget, token, row)
			}
		}
		assertEffectRowTokensWholeAndBasenameIntact(t, row, tokens, base)
		if len([]rune(row)) > writeContextPackTextLen {
			t.Fatalf("budget %d: row exceeds the bound: %d", budget, len([]rune(row)))
		}
	}
	// A basename longer than the whole bound is rendered whole, over the
	// bound, with no token — never cut.
	huge := strings.Repeat("h", writeContextPackTextLen+10) + ".lock"
	if row := WriteContextWorktreeEffectText(effect(huge)); row != "path="+huge {
		t.Fatalf("a basename beyond the bound is still whole: %q", row)
	}
	// Directory-only rows (untracked outputs) shrink the same way.
	untracked := VerificationWorktreeEffect{Path: strings.Repeat("dir/", 70) + "junk.out", Kind: VerificationWorktreeEffectUntrackedCreated,
		Ownership: "unproven_generated_artifact", Action: "retained_not_committed_not_auto_deleted"}
	row = WriteContextWorktreeEffectText(untracked)
	if !strings.HasSuffix(row, "/junk.out") || !strings.Contains(row, " path=…") || len([]rune(row)) != writeContextPackTextLen ||
		!strings.HasPrefix(row, "kind=untracked_created ownership=unproven_generated_artifact action=retained_not_committed_not_auto_deleted path=") {
		t.Fatalf("untracked row: %q", row)
	}
}

// M: the dedicated disclosure item keeps the whole phrase and the WHOLE
// path for every unproven member and survives pack normalisation untrimmed,
// as does the effect row.
func TestWriteContextLockfileFixedPointItemKeepsWholePathThroughNormalisation(t *testing.T) {
	deep := strings.Repeat("segment/", 40) + "package-lock.json"
	for _, fp := range AllVerificationLockfileFixedPoints() {
		phrase := VerificationLockfileFixedPointDisclosure(fp, false)
		item := WriteContextLockfileFixedPointDisclosureText(deep, fp)
		if phrase == "" {
			if item != "" {
				t.Fatalf("%s: no phrase ⇒ no item: %q", fp, item)
			}
			continue
		}
		if item != "lockfile_fixed_point="+string(fp)+" ("+phrase+") path="+deep {
			t.Fatalf("%s: item must be token + whole phrase + whole path: %q", fp, item)
		}
		pack := NormalizeWriteContextPack(WriteContextPackFromChangeReport(&ChangeReport{PlanID: "plan-m", Passed: true,
			WorktreeAudit: &VerificationWorktreeAudit{Status: VerificationWorktreeAuditTrackedDriftDisclosed,
				Effects: []VerificationWorktreeEffect{lockfileEffectForPath(deep, fp)}}}))
		foundItem, foundRow := false, false
		for _, it := range pack.Items {
			switch it.Kind {
			case "verification_lockfile_fixed_point":
				foundItem = true
				if it.Text != item {
					t.Fatalf("%s: normalisation altered the disclosure item: %q", fp, it.Text)
				}
			case "verification_worktree_effect":
				foundRow = true
				if strings.HasSuffix(it.Text, "...") || !strings.HasSuffix(it.Text, "/package-lock.json") {
					t.Fatalf("%s: normalisation trimmed the effect row: %q", fp, it.Text)
				}
			}
		}
		if !foundItem || !foundRow {
			t.Fatalf("%s: pack lost the typed rows: %+v", fp, pack.Items)
		}
	}
	// A generic item is still bounded by the pack trim (the exemption is
	// closed to the two typed rows).
	generic := normalizeWriteContextItem(WriteContextItem{Kind: "note", Text: strings.Repeat("n", writeContextPackTextLen+5)})
	if !strings.HasSuffix(generic.Text, "...") {
		t.Fatalf("generic items keep the trim: %q", generic.Text)
	}
}
