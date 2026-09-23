package tool

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/hanchaoqun/codrax/internal/types"
)

// This observer runs inside the existing native unittest process. Loading,
// TextTestRunner reporting, skips, subtests and exit status remain unittest's.
// Only the new explicit execution intent uses it; no model code is accepted.
const existingTestUnittestObserver = `import hashlib, inspect, json, os, sys, unittest
destination, selected = sys.argv[1:3]
rows = []
overflow = False
class ObservedResult(unittest.TextTestResult):
    current = None
    def record(self, test, **fields):
        global overflow
        if self.current is not None:
            self.current.update(fields)
        elif len(rows) < 512:
            row = dict(assertion_id=str(test)[:1024], suite='unittest.fixture', passed=True, non_asserting=True, started=False, detail='')
            row.update(fields)
            rows.append(row)
        else:
            overflow = True
    def startTest(self, test):
        super().startTest(test)
        self.current = dict(assertion_id=getattr(test, '_testMethodName', ''), suite=type(test).__module__+'.'+type(test).__qualname__, passed=True, non_asserting=False, started=True, detail='')
        try:
            method = getattr(test, test._testMethodName)
            source = os.path.realpath(inspect.getfile(method))
            module = os.path.realpath(sys.modules[type(test).__module__].__file__)
            with open(source, 'rb') as f: digest = hashlib.sha256(f.read()).hexdigest()
            self.current.update(source=source, module=module, sha256=digest)
        except (AttributeError, KeyError, OSError, TypeError):
            pass
    def addFailure(self, test, err):
        self.record(test, passed=False, detail=self._exc_info_to_string(err, test)[:4000]); super().addFailure(test, err)
    def addError(self, test, err):
        self.record(test, passed=False, detail=self._exc_info_to_string(err, test)[:4000]); super().addError(test, err)
    def addSubTest(self, test, subtest, err):
        if err is not None: self.record(test, passed=False, detail=self._exc_info_to_string(err, test)[:4000])
        super().addSubTest(test, subtest, err)
    def addSkip(self, test, reason):
        self.record(test, non_asserting=True, detail='skipped: '+str(reason)[:1000]); super().addSkip(test, reason)
    def addExpectedFailure(self, test, err):
        self.record(test, non_asserting=True, detail='expected failure'); super().addExpectedFailure(test, err)
    def addUnexpectedSuccess(self, test):
        self.record(test, passed=False, detail='unexpected success'); super().addUnexpectedSuccess(test)
    def stopTest(self, test):
        global overflow
        if len(rows) < 512: rows.append(self.current)
        else: overflow = True
        super().stopTest(test)
        self.current = None
class ObservedRunner(unittest.TextTestRunner):
    resultclass = ObservedResult
program = unittest.main(module=None, argv=['unittest', selected, '-v'], testRunner=ObservedRunner, exit=False)
try:
    with open(destination, 'x', encoding='utf-8') as f:
        json.dump(dict(rows=rows, overflow=overflow, successful=program.result.wasSuccessful(), tests_run=program.result.testsRun), f)
except OSError:
    print('[codrax unittest] execution observation could not be written', file=sys.stderr)
sys.exit(not program.result.wasSuccessful())
`

const existingTestUnittestMaxReportBytes = 2 << 20

type existingTestUnittestRow struct {
	AssertionID  string `json:"assertion_id"`
	Suite        string `json:"suite"`
	Passed       bool   `json:"passed"`
	NonAsserting bool   `json:"non_asserting"`
	Started      bool   `json:"started"`
	Detail       string `json:"detail"`
	Source       string `json:"source"`
	Module       string `json:"module"`
	SHA256       string `json:"sha256"`
}

type existingTestUnittestInvocation struct {
	directory, reportPath, target, targetAbs, targetSHA string
	rows                                                []existingTestUnittestRow
	directoryInfo, reportInfo, readInfo                 os.FileInfo
	delivery                                            verificationDeliveryBinding
}

func prepareExistingTestUnittestInvocation(ctx *types.BusContext, invocation runnerPlan) (*existingTestUnittestInvocation, string) {
	if ctx == nil || ctx.Mutable == nil || ctx.PipelineStage != types.StageVerify {
		return nil, ""
	}
	plan := ctx.Mutable.ChangePlan()
	wd := runnerPlanRel(ctx.RepoRoot, invocation)
	for _, target := range types.RequiredExistingTestPaths(plan) {
		if !types.ExistingTestExactFileSelector(invocation.Runner, invocation.Framework, wd, invocation.Suite, target) || safeImpactRelatedPath(ctx.RepoRoot, target) == "" {
			continue
		}
		delivery, ok := bindVerificationDelivery(ctx)
		if !ok {
			return nil, ""
		}
		sha, ok := delivery.currentTestSHA(ctx, target)
		if !ok {
			return nil, ""
		}
		directory, err := os.MkdirTemp("", "codrax-unittest-execution-")
		if err != nil {
			return nil, ""
		}
		directoryInfo, err := os.Lstat(directory)
		if err != nil || !directoryInfo.IsDir() || directoryInfo.Mode()&os.ModeSymlink != 0 {
			return nil, ""
		}
		canonical, err := filepath.EvalSymlinks(directory)
		if err != nil {
			return nil, ""
		}
		directory = canonical
		if info, err := os.Lstat(directory); err != nil || !os.SameFile(directoryInfo, info) {
			return nil, ""
		}
		abs, err := filepath.EvalSymlinks(filepath.Join(ctx.RepoRoot, filepath.FromSlash(target)))
		if err != nil {
			_ = os.Remove(directory)
			return nil, ""
		}
		run := &existingTestUnittestInvocation{directory: directory, directoryInfo: directoryInfo, reportPath: filepath.Join(directory, "result.json"), target: target, targetAbs: abs, targetSHA: sha, delivery: delivery}
		interp := pythonRuntimeInterpreter(invocation.Root, ctx.MainRepoRoot)
		command := fmt.Sprintf("%s -c %s %s %s", interp, shellQuoteWord(existingTestUnittestObserver), shellQuoteWord(run.reportPath), shellQuoteWord(invocation.Suite))
		return run, command
	}
	return nil, ""
}

func (r *existingTestUnittestInvocation) cleanup() {
	if !r.ownsDirectory() {
		return
	}
	info, err := os.Lstat(r.reportPath)
	if err == nil {
		// Only the regular file whose bytes were opened and bound by this
		// invocation can be removed. Unknown/unread/replaced entries survive.
		if r.reportInfo == nil || !info.Mode().IsRegular() || !junitInvocationFileSnapshotEqual(r.reportInfo, info) || !r.ownsDirectory() {
			return
		}
		if err := os.Remove(r.reportPath); err != nil {
			return
		}
	} else if !os.IsNotExist(err) {
		return
	}
	if r.ownsDirectory() {
		_ = os.Remove(r.directory)
	}
}

func (r *existingTestUnittestInvocation) ownsDirectory() bool {
	if r == nil || r.directoryInfo == nil || r.reportPath != filepath.Join(r.directory, "result.json") {
		return false
	}
	info, err := os.Lstat(r.directory)
	return err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 && os.SameFile(r.directoryInfo, info)
}

// Use the same opened/path snapshot binding as the JUnit reader, but bound
// the actual read, not only the pre-open size. Directory identity is anchored
// at preparation, so a replacement tree cannot provide or lose its files.
func (r *existingTestUnittestInvocation) readReportBytes() ([]byte, error) {
	if !r.ownsDirectory() {
		return nil, fmt.Errorf("unittest observation directory changed")
	}
	before, err := os.Lstat(r.reportPath)
	if err != nil || !before.Mode().IsRegular() || before.Size() > existingTestUnittestMaxReportBytes {
		return nil, fmt.Errorf("current unittest observation unavailable")
	}
	file, err := os.Open(r.reportPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !junitInvocationFileSnapshotEqual(before, opened) || !r.ownsDirectory() {
		return nil, fmt.Errorf("unittest observation changed while opening")
	}
	data, err := readExistingTestUnittestBounded(file)
	if err != nil {
		return nil, err
	}
	afterRead, err := file.Stat()
	if err != nil || !junitInvocationFileSnapshotEqual(opened, afterRead) || int64(len(data)) != afterRead.Size() {
		return nil, fmt.Errorf("unittest observation changed while reading")
	}
	afterPath, err := os.Lstat(r.reportPath)
	if err != nil || !junitInvocationFileSnapshotEqual(afterRead, afterPath) || !r.ownsDirectory() {
		return nil, fmt.Errorf("unittest observation path changed while reading")
	}
	r.readInfo = afterPath
	return data, nil
}

func readExistingTestUnittestBounded(reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, existingTestUnittestMaxReportBytes+1))
	if err != nil || len(data) > existingTestUnittestMaxReportBytes {
		return nil, fmt.Errorf("unittest observation exceeds bounded read")
	}
	return data, nil
}

func (r *existingTestUnittestInvocation) readReport(ctx *types.BusContext, exitCode int, output string, runErr error) (*types.ChangeReport, error) {
	r.readInfo = nil
	data, err := r.readReportBytes()
	if err != nil {
		return nil, err
	}
	var result struct {
		Rows       []existingTestUnittestRow `json:"rows"`
		Overflow   bool                      `json:"overflow"`
		Successful bool                      `json:"successful"`
		TestsRun   int                       `json:"tests_run"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	started := 0
	for _, row := range result.Rows {
		if row.Started {
			started++
		}
	}
	if result.Overflow || len(result.Rows) > types.MaxExistingTestExecutionAssertions || result.TestsRun != started || result.Successful != (exitCode == 0) {
		return nil, fmt.Errorf("unittest observation incomplete or inconsistent")
	}
	sha, current := r.delivery.currentTestSHA(ctx, r.target)
	if !current || sha != r.targetSHA {
		return nil, fmt.Errorf("unittest delivery changed during execution")
	}
	// Keep the existing parser's native verdict, loader-error classification,
	// summary and zero-test markers. The observer supplies exact result rows
	// (notably subtests/fixture events) and adds no stronger failure verdict.
	report, err := parseUnittestOutput(output, runErr)
	if err != nil {
		return nil, err
	}
	report.TestResults = nil
	if !report.Passed && report.FailureKind == "" {
		report.FailureKind = types.FailureKindTestsFailed
	}
	for _, row := range result.Rows {
		scope := types.TestObservationScopeAssertion
		if row.NonAsserting {
			scope = types.TestObservationScopeNonAsserting
		}
		report.TestResults = append(report.TestResults, types.TestResult{Kind: types.TestResultKindUnit, ObservationScope: scope, AssertionID: row.AssertionID, Suite: row.Suite, Passed: row.Passed, FailureDetail: row.Detail})
	}
	info, err := os.Lstat(r.reportPath)
	if err != nil || !r.ownsDirectory() || !junitInvocationFileSnapshotEqual(r.readInfo, info) {
		return nil, fmt.Errorf("unittest observation changed while validating")
	}
	// A colliding non-report file is not ours merely because it was opened.
	// Cleanup ownership is published only after the observation validates.
	r.reportInfo = r.readInfo
	r.rows = result.Rows
	return report, nil
}
