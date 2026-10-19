package reportartifact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/types"
)

func fixtureReport() *types.ChangeReport {
	return &types.ChangeReport{PlanID: "plan-260", Passed: true,
		Channel:     types.ChangeReportChannelPostApplyVerify,
		TestResults: []types.TestResult{{AssertionID: "one assertion", Passed: true}}}
}

func TestHMC260ReportHistoryAndRepeatedSave(t *testing.T) {
	dir := t.TempDir()
	r := fixtureReport()
	first, err := PublishSnapshot(dir, r)
	if err != nil {
		t.Fatal(err)
	}
	before, err := filegeneration.FromPath(filepath.Join(dir, first.ReportID))
	if err != nil {
		t.Fatal(err)
	}
	if r.VerificationStatus != "" {
		t.Fatal("publication mutated producer")
	}
	r.Passed = false
	r.TestResults[0].Passed = false
	second, err := PublishSnapshot(dir, r)
	if err != nil || first.ReportID == second.ReportID {
		t.Fatalf("history overwritten: %v", err)
	}
	if err := PublishCurrentView(dir, "plan-260.report.json", second); err != nil {
		t.Fatal(err)
	}
	old, err := LoadExact(dir, first.ReportID, "plan-260")
	if err != nil || !old.Report.Passed || !old.Report.TestResults[0].Passed {
		t.Fatalf("historical report read current: %v", err)
	}
	again, err := PublishSnapshot(dir, first.Report)
	after, statErr := filegeneration.FromPath(filepath.Join(dir, first.ReportID))
	if err != nil || statErr != nil || again.ReportID != first.ReportID || !before.SameVersion(after) {
		t.Fatal("same bytes rewrote history")
	}
	if first.PayloadSHA256 == first.CanonicalSHA256 {
		t.Fatal("pretty and canonical hashes conflated")
	}
	if !ReceiptMatchesReport(first, first.Report) || ReceiptMatchesReport(first, second.Report) {
		t.Fatal("receipt matched wrong version")
	}
}

func TestHMC260ExactReportRejectsWrongCoordinates(t *testing.T) {
	dir := t.TempDir()
	r, err := PublishSnapshot(dir, fixtureReport())
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{"../" + r.ReportID, filepath.Join(dir, r.ReportID), "sub/" + r.ReportID, "sub\\" + r.ReportID, "plan-260.report.json", strings.ToUpper(r.ReportID)} {
		if _, err := LoadExact(dir, ref, "plan-260"); err == nil {
			t.Fatalf("accepted invalid exact ref %q", ref)
		}
	}
	if _, err := LoadExact(dir, r.ReportID, "another-plan"); err == nil {
		t.Fatal("accepted cross-plan payload")
	}
	if err := PublishCurrentView(dir, "plan-260.report.json", r); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, r.ReportID)); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadExact(dir, r.ReportID, "plan-260"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing exact fell back to current: %v", err)
	}
}

func TestHMC260ReportConflictNeverRepairs(t *testing.T) {
	for _, kind := range []string{"bytes", "directory", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			source := t.TempDir()
			r, err := PublishSnapshot(source, fixtureReport())
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			path := filepath.Join(dir, r.ReportID)
			switch kind {
			case "bytes":
				err = os.WriteFile(path, []byte("foreign bytes"), 0600)
			case "directory":
				err = os.Mkdir(path, 0700)
			case "symlink":
				err = os.Symlink(filepath.Join(source, r.ReportID), path)
			}
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := PublishSnapshot(dir, fixtureReport()); err == nil {
				t.Fatal("conflict reused")
			}
			if _, err := LoadExact(dir, r.ReportID, "plan-260"); err == nil {
				t.Fatal("conflict loaded")
			}
			after, err := os.Lstat(path)
			if err != nil || !os.SameFile(before, after) || !filegeneration.FromInfo(before).SameVersion(filegeneration.FromInfo(after)) {
				t.Fatal("conflict replaced")
			}
		})
	}
}

func TestHMC260LegacyUsesCanonicalNotPayloadDigest(t *testing.T) {
	dir := t.TempDir()
	report := fixtureReport() // Old JSON can lack normalized status.
	canonical, _ := json.Marshal(report)
	wire, _ := json.MarshalIndent(report, "", "  ")
	sha := sha256.Sum256(canonical)
	byteSHA := sha256.Sum256(wire)
	if err := os.WriteFile(filepath.Join(dir, "plan-260.report.json"), wire, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadLegacy(dir, "plan-260.report.json", "plan-260", hex.EncodeToString(sha[:]))
	if err != nil || loaded.Report.VerificationStatus != "" {
		t.Fatalf("legacy changed before digest verification: %v", err)
	}
	if _, err := LoadLegacy(dir, "plan-260.report.json", "plan-260", hex.EncodeToString(byteSHA[:])); err == nil {
		t.Fatal("payload digest accepted as canonical")
	}
	if _, err := LoadLegacy(dir, "plan-260.report.json", "plan-260", strings.Repeat("a", 64)); err == nil {
		t.Fatal("historical mismatch loaded current")
	}
	if _, err := LoadLegacy(dir, "plan-260.report.json", "plan-260", ""); err != nil {
		t.Fatal(err)
	}
}

func TestHMC260ConcurrentReportsPreserveGeneration(t *testing.T) {
	dir := t.TempDir()
	var wg sync.WaitGroup
	results := make(chan *Receipt, 16)
	errors := make(chan error, 16)
	for i := 0; i < cap(results); i++ {
		wg.Add(1)
		go func() { defer wg.Done(); r, err := PublishSnapshot(dir, fixtureReport()); results <- r; errors <- err }()
	}
	wg.Wait()
	close(results)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first string
	for r := range results {
		if first == "" {
			first = r.ReportID
		}
		if r.ReportID != first {
			t.Fatal("concurrent identical publication diverged")
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("staging leaked: %v %v", entries, err)
	}
}

func TestHMC260CurrentViewIsDetachedAndIdempotent(t *testing.T) {
	dir := t.TempDir()
	r, err := PublishSnapshot(dir, fixtureReport())
	if err != nil {
		t.Fatal(err)
	}
	current := "plan-260.report.json"
	if err := PublishCurrentView(dir, current, r); err != nil {
		t.Fatal(err)
	}
	before, _ := filegeneration.FromPath(filepath.Join(dir, current))
	if err := PublishCurrentView(dir, current, r); err != nil {
		t.Fatal(err)
	}
	after, _ := filegeneration.FromPath(filepath.Join(dir, current))
	if !before.SameVersion(after) {
		t.Fatal("identical current view changed generation")
	}
	if err := PublishCurrentView(dir, r.ReportID, r); err == nil {
		t.Fatal("current writer may overwrite immutable leaf")
	}
	r.Report.Passed = false
	if err := PublishCurrentView(dir, current, r); err == nil {
		t.Fatal("mutated snapshot reused receipt")
	}
	loaded, err := LoadExact(dir, r.ReportID, "plan-260")
	if err != nil || !loaded.Report.Passed {
		t.Fatal("mutable receipt changed original")
	}
	if err := os.WriteFile(filepath.Join(dir, current), []byte("legacy current changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadExact(dir, r.ReportID, "plan-260"); err != nil {
		t.Fatal("current and original share bytes")
	}
}

func TestHMC260ReportNamesRetainFullIdentityAndPlanListSuffix(t *testing.T) {
	r := fixtureReport()
	r.PlanID = strings.Repeat("长plan/", 100)
	receipt, err := PublishSnapshot(t.TempDir(), r)
	if err != nil {
		t.Fatal(err)
	}
	if len(receipt.ReportID) > 255 || !strings.HasSuffix(receipt.ReportID, ".report.json") || !IsExactID(receipt.ReportID) || !strings.Contains(receipt.ReportID, receipt.PayloadSHA256) {
		t.Fatal("unsafe or truncated content identity")
	}
	if receipt.Report.PlanID != r.PlanID || receipt.Report.GeneratedAt != r.GeneratedAt {
		t.Fatal("storage changed producer identity/time")
	}
}

func TestHMC260ReportReceiptFieldsCannotForgeAnotherSnapshot(t *testing.T) {
	dir := t.TempDir()
	r, err := PublishSnapshot(dir, fixtureReport())
	if err != nil {
		t.Fatal(err)
	}
	r.Report.Passed = false
	wire, _ := json.Marshal(r.Report)
	r.CanonicalSHA256 = digest(wire)
	if ReceiptMatchesReport(r, r.Report) {
		t.Fatal("mutable metadata forged frozen identity")
	}
	if err := PublishCurrentView(dir, "plan-260.report.json", r); err == nil {
		t.Fatal("forged receipt published current")
	}
}
