package tool

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Only the execution path allocates an output location. Inventory/command
// previews stay pure, and each selector (including retries) gets its own file.
// Like the JUnit adapter this establishes invocation ownership, not a sandbox
// against a concurrently hostile child process changing filesystem paths.
type pytestInvocation struct {
	root, directory, reportPath string
	directoryInfo, reportInfo   os.FileInfo
}

const pytestInvocationMaxBytes = 64 << 20

func preparePytestRunnerInvocation(plan runnerPlan, command, extraFile string) (*pytestInvocation, string, string, error) {
	if !shouldAttemptPytestTextFallback(plan) {
		return nil, command, extraFile, nil
	}
	oldArgument := "--json-report-file=" + fmt.Sprintf("%q", extraFile)
	if extraFile == "" || strings.Count(command, oldArgument) != 1 {
		return nil, command, extraFile, fmt.Errorf("pytest invocation has no unique declared report argument")
	}
	root, err := junitInvocationRoot(plan.Root)
	if err != nil {
		return nil, command, extraFile, err
	}
	parent := root
	for _, name := range []string{".codrax", "tmp"} {
		parent = filepath.Join(parent, name)
		if err := os.Mkdir(parent, 0o700); err != nil && !os.IsExist(err) {
			return nil, command, extraFile, err
		}
		info, err := os.Lstat(parent)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, command, extraFile, fmt.Errorf("pytest output parent is not a regular directory: %s", parent)
		}
	}
	directory, err := os.MkdirTemp(parent, "pytest-invocation-")
	if err != nil {
		return nil, command, extraFile, err
	}
	info, err := os.Lstat(directory)
	if err != nil {
		_ = os.Remove(directory)
		return nil, command, extraFile, err
	}
	inv := &pytestInvocation{root: root, directory: directory, directoryInfo: info, reportPath: filepath.Join(directory, "report.json")}
	shell, _ := shellSpec()
	bound := strings.Replace(command, oldArgument, "--json-report-file="+junitReportArgumentForShell(inv.reportPath, shell), 1)
	return inv, bound, inv.reportPath, nil
}

func (inv *pytestInvocation) ownsDirectory() bool {
	if inv == nil || inv.directoryInfo == nil || inv.reportPath != filepath.Join(inv.directory, "report.json") {
		return false
	}
	rel, err := filepath.Rel(inv.root, inv.directory)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	current := inv.root
	for _, part := range append([]string{""}, strings.Split(rel, string(filepath.Separator))...) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
		if current == inv.directory && !os.SameFile(info, inv.directoryInfo) {
			return false
		}
	}
	return true
}

func (inv *pytestInvocation) readReport(stdout, command string, runErr error) (*types.ChangeReport, string, error) {
	if inv == nil {
		return nil, "", fmt.Errorf("pytest invocation not prepared")
	}
	if !inv.ownsDirectory() {
		return nil, "", pytestJSONReportReadError(inv.reportPath, stdout, command, fmt.Errorf("pytest report directory changed"))
	}
	before, err := os.Lstat(inv.reportPath)
	if err != nil {
		return nil, "", pytestJSONReportReadError(inv.reportPath, stdout, command, err)
	}
	if !before.Mode().IsRegular() || before.Size() > pytestInvocationMaxBytes {
		return nil, "", pytestJSONReportReadError(inv.reportPath, stdout, command, fmt.Errorf("not a bounded regular pytest report"))
	}
	file, err := os.Open(inv.reportPath)
	if err != nil {
		return nil, "", pytestJSONReportReadError(inv.reportPath, stdout, command, err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !junitInvocationFileSnapshotEqual(before, opened) {
		return nil, "", pytestJSONReportReadError(inv.reportPath, stdout, command, fmt.Errorf("pytest report changed while opening"))
	}
	data, err := io.ReadAll(io.LimitReader(file, pytestInvocationMaxBytes+1))
	if err != nil || len(data) > pytestInvocationMaxBytes {
		return nil, "", pytestJSONReportReadError(inv.reportPath, stdout, command, fmt.Errorf("pytest report read failed or exceeded %d bytes", pytestInvocationMaxBytes))
	}
	afterRead, err := file.Stat()
	if err != nil || !junitInvocationFileSnapshotEqual(opened, afterRead) || int64(len(data)) != afterRead.Size() {
		return nil, "", pytestJSONReportReadError(inv.reportPath, stdout, command, fmt.Errorf("pytest report changed while reading"))
	}
	after, err := os.Lstat(inv.reportPath)
	if err != nil || !junitInvocationFileSnapshotEqual(afterRead, after) || !inv.ownsDirectory() {
		return nil, "", pytestJSONReportReadError(inv.reportPath, stdout, command, fmt.Errorf("pytest report path changed while reading"))
	}
	// Parsing and digesting share one bounded read. A malformed or unexpected
	// file is not promoted to cleanup ownership and cannot replace a verdict.
	report, err := parsePytestJSONReportBytes(data, inv.reportPath, stdout, command, runErr)
	if err != nil {
		return nil, "", err
	}
	inv.reportInfo = after
	digest := sha256.Sum256(data)
	return report, hex.EncodeToString(digest[:]), nil
}

func (inv *pytestInvocation) cleanup() {
	if !inv.ownsDirectory() {
		return
	}
	info, err := os.Lstat(inv.reportPath)
	if err == nil {
		if inv.reportInfo == nil || !info.Mode().IsRegular() || !junitInvocationFileSnapshotEqual(inv.reportInfo, info) || !inv.ownsDirectory() {
			return
		}
		if err := os.Remove(inv.reportPath); err != nil {
			return
		}
	} else if !os.IsNotExist(err) {
		return
	}
	if inv.ownsDirectory() {
		_ = os.Remove(inv.directory) // Extra files keep the directory intact.
	}
}
