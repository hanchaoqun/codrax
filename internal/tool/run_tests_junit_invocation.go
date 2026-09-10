package tool

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// A missing invocation-bound report is not evidence of a build failure.
// The executor keeps its independently observed exit status and reports this
// as unavailable verification; it must not fall back to an older XML file.
var errJUnitInvocationReportUnavailable = errors.New("invocation-bound JUnit report unavailable")

// junitInvocation is prepared before the command runs, never reconstructed
// from a report name, timestamp, model field, or saved legacy report.
type junitInvocation struct {
	MavenSuffix  string
	ReportPath   string
	root         string
	ownedDir     string
	ownedDirInfo os.FileInfo
}

// Receipts describe bytes actually parsed. They are audit information, not
// a new planner input, source path declaration, or permission to read a file.
type junitInvocationReportFile struct {
	Path                string
	SHA256              string
	Nonce               string
	AmbiguousAssertions []string
}

func newMavenJUnitInvocation(root string) (*junitInvocation, error) {
	canonical, err := junitInvocationRoot(root)
	if err != nil {
		return nil, err
	}
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return nil, err
	}
	return &junitInvocation{root: canonical, MavenSuffix: "codrax-" + hex.EncodeToString(token[:])}, nil
}

func newCTestJUnitInvocation(root string) (*junitInvocation, error) {
	canonical, err := junitInvocationRoot(root)
	if err != nil {
		return nil, err
	}
	parent := canonical
	for _, name := range []string{".codrax", "tmp"} {
		parent = filepath.Join(parent, name)
		if err := os.Mkdir(parent, 0o700); err != nil && !os.IsExist(err) {
			return nil, err
		}
		info, err := os.Lstat(parent)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("JUnit output parent is not a regular directory: %s", parent)
		}
	}
	dir, err := os.MkdirTemp(parent, "junit-invocation-")
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		_ = os.Remove(dir)
		return nil, err
	}
	return &junitInvocation{root: canonical, ownedDir: dir, ownedDirInfo: info, ReportPath: filepath.Join(dir, "ctest.xml")}, nil
}

func junitInvocationRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("JUnit invocation requires an execution root")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(canonical)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("JUnit execution root is not a directory: %s", root)
	}
	return canonical, nil
}

// Cleanup never walks or deletes a user's report directory. Unexpected
// additional files keep the private directory non-empty and thus retained.
func (inv *junitInvocation) Cleanup() {
	if inv == nil || inv.ownedDir == "" || inv.ownedDirInfo == nil {
		return
	}
	// Do not follow a directory replaced by the child process while cleaning
	// our one output. This is ownership protection, not a freshness signal.
	rel, err := filepath.Rel(inv.root, inv.ownedDir)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return
	}
	cur := inv.root
	if info, err := os.Lstat(cur); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return
	}
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return
		}
		if cur == inv.ownedDir && !os.SameFile(info, inv.ownedDirInfo) {
			return
		}
	}
	if inv.ReportPath == filepath.Join(inv.ownedDir, "ctest.xml") {
		_ = os.Remove(inv.ReportPath)
	}
	_ = os.Remove(inv.ownedDir)
}

func (inv *junitInvocation) ReadReport() (*types.ChangeReport, []junitInvocationReportFile, error) {
	if inv == nil || inv.root == "" || (inv.MavenSuffix == "") == (inv.ReportPath == "") {
		return nil, nil, errJUnitInvocationReportUnavailable
	}
	paths := []string{inv.ReportPath}
	runner := "cmake"
	if inv.MavenSuffix != "" {
		runner = "java"
		var err error
		paths, err = inv.mavenReportPaths()
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %v", errJUnitInvocationReportUnavailable, err)
		}
	}
	if len(paths) == 0 {
		return nil, nil, errJUnitInvocationReportUnavailable
	}
	var results []types.TestResult
	var receipts []junitInvocationReportFile
	var rowFiles []int
	for _, path := range paths {
		data, err := readJUnitInvocationRegularFile(inv.root, path)
		if err != nil {
			return nil, receipts, fmt.Errorf("%w: %s: %v", errJUnitInvocationReportUnavailable, path, err)
		}
		// The exact same bytes supply the digest and all parsed rows. No
		// metadata check followed by a second parser file read is permitted.
		digest := sha256.Sum256(data)
		receipt := junitInvocationReportFile{Path: path, SHA256: hex.EncodeToString(digest[:]), Nonce: inv.MavenSuffix}
		suites, err := parseJUnitInvocationBytes(data, inv.MavenSuffix)
		if err != nil {
			return nil, receipts, fmt.Errorf("%w: %s: %v", errJUnitInvocationReportUnavailable, path, err)
		}
		fileIndex := len(receipts)
		receipts = append(receipts, receipt)
		for _, suite := range suites {
			rows := junitCasesToResults(suite)
			if runner == "cmake" {
				for i := range rows {
					if !ctestJUnitStatusCanAssert(suite.TestCases[i], rows[i]) {
						rows[i].ObservationScope = types.TestObservationScopeNonAsserting
					}
				}
			}
			results = append(results, rows...)
			for range rows {
				rowFiles = append(rowFiles, fileIndex)
			}
		}
	}
	// TestResult currently has no report-file identity. Do not let a same-
	// named class in another module authorize a project-test/retirement join.
	// Preserve every observed row and verdict, but make that ambiguous
	// assertion's proof scope unknown. Same-file occurrences remain intact.
	type assertionKey struct{ suite, id string }
	firstFile := make(map[assertionKey]int)
	ambiguous := make(map[assertionKey]bool)
	for i, row := range results {
		key := assertionKey{row.Suite, row.AssertionID}
		if prior, exists := firstFile[key]; exists && prior != rowFiles[i] {
			ambiguous[key] = true
		} else if !exists {
			firstFile[key] = rowFiles[i]
		}
	}
	failed := 0
	for i := range results {
		row := &results[i]
		if ambiguous[assertionKey{row.Suite, row.AssertionID}] {
			row.ObservationScope = ""
			receipts[rowFiles[i]].AmbiguousAssertions = append(receipts[rowFiles[i]].AmbiguousAssertions,
				fmt.Sprintf("suite=%q assertion=%q", row.Suite, row.AssertionID))
		}
		if !row.Passed {
			failed++
		}
	}
	report := &types.ChangeReport{Passed: failed == 0, TestResults: results}
	if len(results) == 0 {
		report.NoTestsRunners = []string{runner}
	} else if failed != 0 {
		report.FailureSummary = fmt.Sprintf("%d of %d %s test cases failed (invocation-bound JUnit XML).", failed, len(results), runner)
	}
	return report, receipts, nil
}

// CTest's disabled status has no <skipped> child. Interpret its closed status
// vocabulary only in the bound CTest adapter; other JUnit dialects retain their
// existing semantics. Empty status preserves legacy child-based classification.
// A status never changes the reported outcome or promotes a non-asserting row.
func ctestJUnitStatusCanAssert(tc junitTestCase, row types.TestResult) bool {
	switch tc.Status {
	case "":
		return true
	case "run":
		return row.Passed && tc.Skipped == nil && tc.Failure == nil && tc.Error == nil
	case "fail":
		return !row.Passed && tc.Skipped == nil && (tc.Failure != nil || tc.Error != nil)
	default:
		return false
	}
}

func (inv *junitInvocation) mavenReportPaths() ([]string, error) {
	var paths []string
	err := filepath.WalkDir(inv.root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != inv.root {
				// Reactor nesting depth is not an authority boundary. Omitting
				// a deep current report can hide failures or a same-name class.
				if entry.Name() == ".git" || entry.Name() == ".codrax" ||
					entry.Name() == "node_modules" || entry.Name() == ".gradle" {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if strings.HasPrefix(entry.Name(), "TEST-") && strings.HasSuffix(entry.Name(), "-"+inv.MavenSuffix+".xml") {
			paths = append(paths, path)
		}
		return nil
	})
	return paths, err
}

// Descendant symlinks are never followed, including a symlinked report
// directory. The caller's root is canonicalized once before execution.
func readJUnitInvocationRegularFile(root, path string) ([]byte, error) {
	checkPath := func() (os.FileInfo, error) {
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("report is outside its execution root")
		}
		cur := root
		if info, err := os.Lstat(cur); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("report execution root is no longer a regular directory")
		}
		parts := strings.Split(rel, string(filepath.Separator))
		var info os.FileInfo
		for index, part := range parts {
			cur = filepath.Join(cur, part)
			info, err = os.Lstat(cur)
			if err != nil {
				return nil, err
			}
			if info.Mode()&os.ModeSymlink != 0 || (index < len(parts)-1 && !info.IsDir()) {
				return nil, fmt.Errorf("report path contains a symlink or non-directory")
			}
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("report is not a regular file")
		}
		return info, nil
	}
	before, err := checkPath()
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !junitInvocationFileSnapshotEqual(before, opened) {
		return nil, fmt.Errorf("report changed while opening")
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	afterRead, err := file.Stat()
	if err != nil || !junitInvocationFileSnapshotEqual(opened, afterRead) || int64(len(data)) != afterRead.Size() {
		return nil, fmt.Errorf("report changed while reading")
	}
	after, err := checkPath()
	if err != nil || !junitInvocationFileSnapshotEqual(afterRead, after) {
		return nil, fmt.Errorf("report path changed while reading")
	}
	return data, nil
}

// These attributes detect mutation during one open read. They never establish
// that a report belongs to the invocation; only the prepared nonce/path does.
func junitInvocationFileSnapshotEqual(a, b os.FileInfo) bool {
	return a != nil && b != nil && os.SameFile(a, b) &&
		a.Size() == b.Size() && a.ModTime().Equal(b.ModTime()) && a.Mode() == b.Mode()
}

func parseJUnitInvocationBytes(data []byte, suffix string) ([]junitTestSuite, error) {
	var root struct{ XMLName xml.Name }
	decoder := xml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&root); err != nil {
		return nil, err
	}
	// Unmarshal alone accepts a valid first root followed by another root or
	// malformed trailing bytes. All bytes in the audit digest must be parsed.
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch value := token.(type) {
		case xml.CharData:
			if strings.TrimSpace(string(value)) != "" {
				return nil, fmt.Errorf("non-whitespace after JUnit root")
			}
		case xml.Comment, xml.ProcInst:
		default:
			return nil, fmt.Errorf("extra content after JUnit root")
		}
	}
	var suites []junitTestSuite
	switch root.XMLName.Local {
	case "testsuite":
		var suite junitTestSuite
		if err := xml.Unmarshal(data, &suite); err != nil {
			return nil, err
		}
		suites = []junitTestSuite{suite}
	case "testsuites":
		var wrapper junitTestSuites
		if err := xml.Unmarshal(data, &wrapper); err != nil {
			return nil, err
		}
		suites = wrapper.Suites
	default:
		return nil, fmt.Errorf("not a JUnit report")
	}
	for i := range suites {
		suite := &suites[i]
		if suffix != "" && strings.TrimSpace(suite.Name) == "" {
			return nil, fmt.Errorf("missing suite identity")
		}
		if suffix == "" {
			continue
		}
		tail := "(" + suffix + ")"
		if !strings.HasSuffix(suite.Name, tail) || len(suite.Name) == len(tail) {
			return nil, fmt.Errorf("suite does not carry the invocation suffix")
		}
		suite.Name = strings.TrimSuffix(suite.Name, tail)
		for j := range suite.TestCases {
			row := &suite.TestCases[j]
			if !strings.HasSuffix(row.ClassName, tail) || len(row.ClassName) == len(tail) {
				return nil, fmt.Errorf("test class does not carry the invocation suffix")
			}
			row.ClassName = strings.TrimSuffix(row.ClassName, tail)
			// Surefire does not append reportNameSuffix to testcase.name.
		}
	}
	return suites, nil
}
