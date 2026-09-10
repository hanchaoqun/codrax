package tool

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func b1651AWriteReport(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func b1651AMavenReport(t *testing.T, inv *junitInvocation, dir, suite, class, name, payload string) string {
	t.Helper()
	path := filepath.Join(inv.root, dir, "TEST-"+suite+"-"+inv.MavenSuffix+".xml")
	body := fmt.Sprintf("<testsuite name=%q><testcase classname=%q name=%q time=\"0.012\">%s</testcase></testsuite>",
		suite+"("+inv.MavenSuffix+")", class+"("+inv.MavenSuffix+")", name, payload)
	b1651AWriteReport(t, path, body)
	return path
}

func TestB1651AMavenNonceRequiresAllThreeExactCarriers(t *testing.T) {
	for _, arm := range []string{"old_file", "filename_only", "suite_only", "class_only", "old_nonce", "mixed_suite", "malformed", "missing_class"} {
		t.Run(arm, func(t *testing.T) {
			inv, err := newMavenJUnitInvocation(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			suite, class := "pkg.C("+inv.MavenSuffix+")", "pkg.C("+inv.MavenSuffix+")"
			filename := "TEST-pkg.C-" + inv.MavenSuffix + ".xml"
			switch arm {
			case "old_file":
				filename = "TEST-pkg.C.xml"
			case "filename_only":
				suite, class = "pkg.C", "pkg.C"
			case "suite_only":
				class = "pkg.C"
			case "class_only":
				suite = "pkg.C"
			case "old_nonce":
				suite, class = "pkg.C(codrax-old)", "pkg.C(codrax-old)"
			case "missing_class":
				class = ""
			}
			body := fmt.Sprintf("<testsuite name=%q><testcase classname=%q name=\"checks\"/></testsuite>", suite, class)
			if arm == "mixed_suite" {
				body = "<testsuites>" + body + "<testsuite name=\"old\"><testcase classname=\"old\" name=\"checks\"/></testsuite></testsuites>"
			}
			if arm == "malformed" {
				body += "<broken"
			}
			b1651AWriteReport(t, filepath.Join(inv.root, "target/surefire-reports", filename), body)
			report, _, err := inv.ReadReport()
			if report != nil || !errors.Is(err, errJUnitInvocationReportUnavailable) {
				t.Fatalf("incomplete invocation binding must be unavailable: report=%+v err=%v", report, err)
			}
		})
	}
}

func TestB1651AMavenRestoresOnlyProducerSuffixAndKeepsVerdicts(t *testing.T) {
	for _, payload := range []string{"", "<failure message=\"bad\"/>", "<error message=\"bad\"/>", "<skipped/>"} {
		t.Run(payload, func(t *testing.T) {
			inv, err := newMavenJUnitInvocation(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			original := "pkg.C(" + inv.MavenSuffix + ")"
			name := "checks(" + inv.MavenSuffix + ")"
			path := b1651AMavenReport(t, inv, "target/surefire-reports", original, original, name, payload)
			report, receipts, err := inv.ReadReport()
			if err != nil || len(report.TestResults) != 1 || len(receipts) != 1 {
				t.Fatalf("report=%+v receipts=%+v err=%v", report, receipts, err)
			}
			row := report.TestResults[0]
			wantPassed := payload == "" || payload == "<skipped/>"
			wantScope := types.TestObservationScopeAssertion
			if payload == "<skipped/>" {
				wantScope = types.TestObservationScopeNonAsserting
			}
			if row.Suite != original || row.AssertionID != original+"#"+name || row.Duration.Milliseconds() != 12 ||
				row.Passed != wantPassed || report.Passed != wantPassed || row.ObservationScope != wantScope {
				t.Fatalf("original identity/verdict altered: %+v report=%+v", row, report)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256(data)
			if receipts[0].Path != path || receipts[0].SHA256 != hex.EncodeToString(digest[:]) || receipts[0].Nonce != inv.MavenSuffix {
				t.Fatalf("not an exact byte receipt: %+v", receipts)
			}
			inv.Cleanup()
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("Maven cleanup touched user report: %v", err)
			}
		})
	}
}

func TestB1651AMavenSameIdentityDifferentFilesIsNotGenericProof(t *testing.T) {
	for _, distinct := range []bool{false, true} {
		t.Run(fmt.Sprint(distinct), func(t *testing.T) {
			inv, err := newMavenJUnitInvocation(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			second := "pkg.C"
			if distinct {
				second = "pkg.D"
			}
			b1651AMavenReport(t, inv, "a/target/surefire-reports", "pkg.C", "pkg.C", "checks", "")
			b1651AMavenReport(t, inv, "b/target/surefire-reports", second, second, "checks", "")
			report, receipts, err := inv.ReadReport()
			if err != nil || len(report.TestResults) != 2 || len(receipts) != 2 {
				t.Fatalf("rows lost: report=%+v receipts=%+v err=%v", report, receipts, err)
			}
			for i, row := range report.TestResults {
				wantScope := types.TestObservationScopeAssertion
				if !distinct {
					wantScope = ""
				}
				if !row.Passed || row.ObservationScope != wantScope || (len(receipts[i].AmbiguousAssertions) != 0) == distinct {
					t.Fatalf("cross-file source ambiguity mislabeled: row=%+v receipt=%+v", row, receipts[i])
				}
			}
		})
	}
}

func TestB1651AMavenDeepReactorReportsCannotBeSilentlyOmitted(t *testing.T) {
	inv, err := newMavenJUnitInvocation(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b1651AMavenReport(t, inv, "target/surefire-reports", "pkg.C", "pkg.C", "checks", "")
	b1651AMavenReport(t, inv, "products/server/plugins/modules/storage/backend/target/surefire-reports", "pkg.C", "pkg.C", "checks", "<failure message=\"deep failure\"/>")
	report, receipts, err := inv.ReadReport()
	if err != nil || len(report.TestResults) != 2 || len(receipts) != 2 || report.Passed {
		t.Fatalf("deep current failure disappeared: report=%+v receipts=%+v err=%v", report, receipts, err)
	}
	for _, row := range report.TestResults {
		if row.ObservationScope != "" {
			t.Fatalf("incomplete source census granted same-name assertion authority: %+v", row)
		}
	}
}

func TestB1651AInvocationRegularFilesOnly(t *testing.T) {
	for _, arm := range []string{"leaf_symlink", "parent_symlink", "directory"} {
		t.Run(arm, func(t *testing.T) {
			inv, err := newMavenJUnitInvocation(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			path := b1651AMavenReport(t, inv, "target/surefire-reports", "pkg.C", "pkg.C", "checks", "")
			switch arm {
			case "leaf_symlink":
				if err := os.Rename(path, path+".actual"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path+".actual", path); err != nil {
					t.Fatal(err)
				}
			case "parent_symlink":
				outside := filepath.Join(t.TempDir(), "reports")
				if err := os.Rename(filepath.Dir(path), outside); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, filepath.Dir(path)); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			report, _, err := inv.ReadReport()
			if report != nil || !errors.Is(err, errJUnitInvocationReportUnavailable) {
				t.Fatalf("unsafe report accepted: report=%+v err=%v", report, err)
			}
		})
	}
}

func TestB1651ACTestOwnsOnePrivateInvocationPath(t *testing.T) {
	root := t.TempDir()
	first, err := newCTestJUnitInvocation(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := newCTestJUnitInvocation(root)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Cleanup()
	if first.ReportPath == second.ReportPath || !strings.HasPrefix(first.ReportPath, filepath.Join(first.root, ".codrax", "tmp")+string(filepath.Separator)) {
		t.Fatalf("not unique private output: %+v %+v", first, second)
	}
	b1651AWriteReport(t, filepath.Join(root, ".codrax-ctest-report.xml"), "<testsuite name=\"old\"><testcase name=\"checks\"/></testsuite>")
	if report, _, err := first.ReadReport(); report != nil || !errors.Is(err, errJUnitInvocationReportUnavailable) {
		t.Fatalf("missing current report borrowed old XML: %+v %v", report, err)
	}
	body := "<testsuite name=\"native\"><testcase classname=\"native\" name=\"checks\"/></testsuite>"
	b1651AWriteReport(t, first.ReportPath, body)
	before, receipts, err := first.ReadReport()
	if err != nil || len(before.TestResults) != 1 || len(receipts) != 1 {
		t.Fatalf("current report unavailable: %+v %v", before, err)
	}
	if report, _, err := second.ReadReport(); report != nil || !errors.Is(err, errJUnitInvocationReportUnavailable) {
		t.Fatalf("second invocation borrowed first: %+v %v", report, err)
	}
	b1651AWriteReport(t, first.ReportPath, body)
	after, _, err := first.ReadReport()
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("same-byte current write rejected: before=%+v after=%+v err=%v", before, after, err)
	}
	unmanaged := filepath.Join(first.ownedDir, "unmanaged.txt")
	b1651AWriteReport(t, unmanaged, "do not delete")
	first.Cleanup()
	if _, err := os.Stat(unmanaged); err != nil {
		t.Fatalf("cleanup touched unrelated file: %v", err)
	}
}

func TestB1651ACTestCannotCreateReportUnderSymlink(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink(t.TempDir(), filepath.Join(root, ".codrax")); err != nil {
		t.Fatal(err)
	}
	if inv, err := newCTestJUnitInvocation(root); inv != nil || err == nil {
		t.Fatalf("symlink output parent accepted: %+v %v", inv, err)
	}
}

func TestB1651ACTestCleanupDoesNotFollowReplacedDirectory(t *testing.T) {
	inv, err := newCTestJUnitInvocation(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(inv.ownedDir, inv.ownedDir+"-original"); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	victim := filepath.Join(outside, "ctest.xml")
	b1651AWriteReport(t, victim, "unrelated report")
	if err := os.Symlink(outside, inv.ownedDir); err != nil {
		t.Fatal(err)
	}
	inv.Cleanup()
	data, err := os.ReadFile(victim)
	if err != nil || string(data) != "unrelated report" {
		t.Fatalf("cleanup followed replacement directory: %q %v", data, err)
	}
}

func TestB1651AInvocationSnapshotChecksAreNotFreshness(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.xml")
	b1651AWriteReport(t, path, "abc")
	first, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := os.Stat(path)
	if err != nil || !junitInvocationFileSnapshotEqual(first, repeated) {
		t.Fatalf("stable snapshot rejected: %v", err)
	}
	if err := os.Chtimes(path, first.ModTime(), first.ModTime().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	changedTime, err := os.Stat(path)
	if err != nil || junitInvocationFileSnapshotEqual(first, changedTime) {
		t.Fatalf("changed modtime missed: %v", err)
	}
	b1651AWriteReport(t, path, "longer")
	if err := os.Chtimes(path, first.ModTime(), first.ModTime()); err != nil {
		t.Fatal(err)
	}
	changedSize, err := os.Stat(path)
	if err != nil || junitInvocationFileSnapshotEqual(first, changedSize) {
		t.Fatalf("changed size missed: %v", err)
	}
	other := filepath.Join(dir, "other.xml")
	b1651AWriteReport(t, other, "abc")
	if err := os.Chtimes(other, first.ModTime(), first.ModTime()); err != nil {
		t.Fatal(err)
	}
	changedIdentity, err := os.Stat(other)
	if err != nil || junitInvocationFileSnapshotEqual(first, changedIdentity) ||
		junitInvocationFileSnapshotEqual(first, nil) {
		t.Fatalf("changed file identity missed: %v", err)
	}
}

func TestB1651AInvocationZeroTestsAndUnknownIdentity(t *testing.T) {
	inv, err := newMavenJUnitInvocation(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b1651AWriteReport(t, filepath.Join(inv.root, "target/surefire-reports/TEST-empty-"+inv.MavenSuffix+".xml"),
		fmt.Sprintf("<testsuite name=%q/>", "empty("+inv.MavenSuffix+")"))
	report, receipts, err := inv.ReadReport()
	if err != nil || report.BuildFailed || len(report.TestResults) != 0 ||
		!reflect.DeepEqual(report.NoTestsRunners, []string{"java"}) || len(receipts) != 1 {
		t.Fatalf("zero execution became fake assertion/build failure: %+v %+v %v", report, receipts, err)
	}
	for _, unknown := range []*junitInvocation{nil, {}, {root: inv.root}} {
		if report, _, err := unknown.ReadReport(); report != nil || !errors.Is(err, errJUnitInvocationReportUnavailable) {
			t.Fatalf("unknown invocation gained source: %+v %v", report, err)
		}
	}
}

// CTest's suite name is the optional dashboard BuildName. Its absence is
// not a missing per-test identity; the native writer supplies classname/name.
func TestB1651ACTestAllowsEmptyDashboardBuildName(t *testing.T) {
	inv, err := newCTestJUnitInvocation(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer inv.Cleanup()
	b1651AWriteReport(t, inv.ReportPath, `<testsuite name=""><testcase name="check" classname="check" status="run" time="0.125"/></testsuite>`)
	report, _, err := inv.ReadReport()
	if err != nil || len(report.TestResults) != 1 || !report.Passed || report.TestResults[0].AssertionID != "check#check" {
		t.Fatalf("CTest's optional dashboard name blocked a current test: %+v %v", report, err)
	}
}

// The first RED used the existing production directory parser. The same
// fixtures now exercise the invocation boundary; Execute is pinned separately.
func TestB1651AMavenInvocationReportDoesNotBorrowOldXML(t *testing.T) {
	root := t.TempDir()
	inv, err := newMavenJUnitInvocation(root)
	if err != nil {
		t.Fatal(err)
	}
	nonce := inv.MavenSuffix
	b1651AWriteReport(t, filepath.Join(root, "target/surefire-reports/TEST-example.Current-"+nonce+".xml"),
		`<testsuite name="example.Current(`+nonce+`)"><testcase classname="example.Current(`+nonce+`)" name="checks"/></testsuite>`)
	b1651AWriteReport(t, filepath.Join(root, "target/surefire-reports/TEST-example.Old.xml"),
		`<testsuite name="example.Old"><testcase classname="example.Old" name="checks"/></testsuite>`)
	report, files, err := inv.ReadReport()
	if err != nil {
		t.Fatal(err)
	}
	if len(report.TestResults) != 1 || report.TestResults[0].AssertionID != "example.Current#checks" ||
		report.TestResults[0].ObservationScope != types.TestObservationScopeAssertion {
		t.Fatalf("only this invocation's exact restored assertion may be published; got %+v", report.TestResults)
	}
	if len(files) != 1 || files[0].Nonce != nonce || files[0].SHA256 == "" {
		t.Fatalf("missing exact-byte report receipt: %+v", files)
	}
}
