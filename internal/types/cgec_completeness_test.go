package types

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestAllRepairKindsHaveProducer is the CGEC Group H structural
// gate: every RepairKind declared in AllRepairKinds() MUST have at
// least one production-code producer. Without this test, a dead
// Kind can slip in — someone adds a new enum value, Render() case,
// and documentation, but forgets to wire a producer, and the
// retry-hint section never fires for the new Kind in practice.
//
// The check walks every *.go file under internal/ (excluding
// _test.go and internal/types/ itself where enum + getter helpers
// live), counts AddRepair call sites that reference each Kind
// constant, and fails when any Kind has zero production callers.
//
// Why a source-level grep instead of a runtime registry: we want
// the test to catch "wrote new enum value, forgot producer" AT
// test time rather than at first production run. Registering at
// init() would also work but adds ceremony and a new side channel
// to maintain.
func TestAllRepairKindsHaveProducer(t *testing.T) {
	repoRoot := findRepoRoot(t)
	internalDir := filepath.Join(repoRoot, "internal")
	sources := collectGoSourcesExcludingTests(t, internalDir)
	// Exclude internal/types/ — that's where the enum lives and
	// self-referential mentions (AllRepairKinds, IsValid, tests)
	// aren't producers.
	filtered := make([]string, 0, len(sources))
	for _, s := range sources {
		if strings.Contains(s, string(os.PathSeparator)+"types"+string(os.PathSeparator)) {
			continue
		}
		filtered = append(filtered, s)
	}

	// For each Kind, look for the pattern
	//     AddRepair(types.RepairDirective{...Kind: types.<KindSymbol>,...})
	// or any use of AddRepair within the same file that mentions
	// the Kind constant. We use a generous two-step check: find
	// files that call AddRepair, then within those files look for
	// any mention of the Kind constant name.
	kindSymbols := map[RepairKind]string{
		RepairReadFile:               "RepairReadFile",
		RepairExpandSearch:           "RepairExpandSearch",
		RepairSwapShape:              "RepairSwapShape",
		RepairRebindSubject:          "RepairRebindSubject",
		RepairForceCompleteDowngrade: "RepairForceCompleteDowngrade",
	}
	// Sanity: AllRepairKinds() must match the map so this test
	// itself catches new kinds added to the enum.
	if len(AllRepairKinds()) != len(kindSymbols) {
		t.Fatalf("kind coverage table out of date: %d kinds in AllRepairKinds vs %d in test map — add the new kind to kindSymbols and wire a producer", len(AllRepairKinds()), len(kindSymbols))
	}

	addRepairPattern := regexp.MustCompile(`\bAddRepair\s*\(`)

	for kind, sym := range kindSymbols {
		var producerFile string
	fileLoop:
		for _, path := range filtered {
			body := readFileForTest(t, path)
			if !addRepairPattern.MatchString(body) {
				continue
			}
			if !strings.Contains(body, sym) {
				continue
			}
			producerFile = path
			break fileLoop
		}
		if producerFile == "" {
			t.Errorf("RepairKind %s has NO producer in internal/ (searched %d files). Every Kind MUST have ≥1 AddRepair call site outside internal/types/ — if this kind was intentionally left without a producer, delete it from the enum instead of keeping dead code.",
				kind, len(filtered))
		} else {
			t.Logf("RepairKind %s producer found: %s", kind, relToRepo(producerFile, repoRoot))
		}
	}
}

// TestEvidenceClosureAllFieldsHaveConsumer asserts that every
// EvidenceClosure field the CGEC design names is read by at least
// one production code path — not just written. Session 10 revived
// three fields (scannedSet, unverifiedFinds, subjectMatches) that
// had been dead because of missing read-side wiring; this test
// prevents regressions by failing when a write-only state surface
// sneaks back in.
//
// The check looks for get-style methods (ReadSet, PendingReads,
// UnverifiedFindings, ScannedSet, IsScanned, AllSubjectMatches,
// CitedRefs, Fingerprints, PendingRepairs, Stats) in production
// *.go files under internal/ (excluding _test.go and
// internal/types/). Any listed accessor with zero production
// callers is a dead read surface.
func TestEvidenceClosureAllFieldsHaveConsumer(t *testing.T) {
	repoRoot := findRepoRoot(t)
	sources := collectGoSourcesExcludingTests(t, filepath.Join(repoRoot, "internal"))
	filtered := make([]string, 0, len(sources))
	for _, s := range sources {
		if strings.Contains(s, string(os.PathSeparator)+"types"+string(os.PathSeparator)) {
			continue
		}
		filtered = append(filtered, s)
	}
	// Each accessor lists ONE method name. Production code should
	// call at least one of them. Group related accessors together
	// so a backup read-path is still acceptable coverage.
	requiredAccessors := map[string][]string{
		"readSet":         {"ReadSet(", "HasRead(", "CanonicalReadFiles("},
		"scannedSet":      {"IsScanned(", "ScannedSet("},
		"citedRefs":       {"CitedRefs("},
		"pendingReads":    {"PendingReads("},
		"unverifiedFinds": {"UnverifiedFindings("},
		"subjectMatches":  {"AllSubjectMatches(", "BestSubjectMatch(", "SubjectMatch("},
		"fingerprints":    {"Fingerprints("},
		"repairs":         {"PendingRepairs(", "ConsumeRepairs("},
		"stats":           {"Stats("},
	}

	for field, accessors := range requiredAccessors {
		found := false
		var hitFile string
		for _, path := range filtered {
			body := readFileForTest(t, path)
			for _, acc := range accessors {
				if strings.Contains(body, acc) {
					found = true
					hitFile = path
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			t.Errorf("closure field %s has NO production consumer — searched %d files for %v. Either wire a real read-side consumer OR delete the field to prevent dead state surfaces.",
				field, len(filtered), accessors)
		} else {
			t.Logf("closure field %s consumer: %s", field, relToRepo(hitFile, repoRoot))
		}
	}
}

// findRepoRoot walks up from the test working directory looking for
// go.mod. Uses t.Fatalf so we do not silently skip on unexpected
// layouts.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate go.mod ancestor of %s", dir)
	return ""
}

// collectGoSourcesExcludingTests returns every *.go file under root
// that is not a _test.go file. Panics on I/O error so the test
// fails loudly rather than silently under-scanning.
func collectGoSourcesExcludingTests(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		out = append(out, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}

func readFileForTest(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func relToRepo(path, repoRoot string) string {
	rel, err := filepath.Rel(repoRoot, path)
	if err != nil {
		return path
	}
	return rel
}
