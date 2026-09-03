package tool

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	manifestlessJavaMaxSources    = 256
	manifestlessJavaMaxMainTests  = 32
	manifestlessJavaMaxSourceSize = 256 * 1024
)

var (
	manifestlessJavaMainPattern    = regexp.MustCompile(`\bpublic\s+static\s+void\s+main\s*\(\s*String(?:\s*\[\s*\]\s+[A-Za-z_$][A-Za-z0-9_$]*|\.\.\.\s+[A-Za-z_$][A-Za-z0-9_$]*|\s+[A-Za-z_$][A-Za-z0-9_$]*\s*\[\s*\])\s*\)`)
	manifestlessJavaPackagePattern = regexp.MustCompile(`(?m)^\s*package\s+([A-Za-z_$][A-Za-z0-9_$.]*)\s*;`)
)

type manifestlessJavaMainSurface struct {
	SourcePaths     []string
	TestMainClasses []string
}

// discoverManifestlessJavaMainSurface recognizes only bounded, conventional
// Java source/test structure. A Test-suffixed file must contain an executable
// main signature; a filename alone is not enough to mint behavior authority.
func discoverManifestlessJavaMainSurface(root string) manifestlessJavaMainSurface {
	var out manifestlessJavaMainSurface
	root = strings.TrimSpace(root)
	if root == "" {
		return out
	}
	overflow := false
	_ = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			if info != nil && info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			if shouldSkipRunnerDir(root, path, info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.ToLower(filepath.Ext(info.Name())) != ".java" {
			return nil
		}
		if len(out.SourcePaths) >= manifestlessJavaMaxSources || info.Size() > manifestlessJavaMaxSourceSize {
			overflow = true
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == "" || strings.HasPrefix(filepath.ToSlash(rel), "../") {
			return nil
		}
		rel = filepath.ToSlash(rel)
		out.SourcePaths = append(out.SourcePaths, rel)
		matcher := testFileMatcher("java")
		if matcher == nil || !matcher(path, info.Name()) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		className := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		sanitized := stripJavaCommentsAndLiterals(data)
		if !manifestlessJavaHasTopLevelMain(sanitized, className) {
			return nil
		}
		if len(out.TestMainClasses) >= manifestlessJavaMaxMainTests {
			overflow = true
			return nil
		}
		if match := manifestlessJavaPackagePattern.FindSubmatch(sanitized); len(match) == 2 {
			className = string(match[1]) + "." + className
		}
		out.TestMainClasses = append(out.TestMainClasses, className)
		return nil
	})
	if overflow {
		return manifestlessJavaMainSurface{}
	}
	sort.Strings(out.SourcePaths)
	sort.Strings(out.TestMainClasses)
	return out
}

// stripJavaCommentsAndLiterals preserves byte offsets while removing regions
// where class/main/braces are not Java declarations. That keeps the discovery
// signal structural: examples in comments and braces in literals cannot mint
// a runnable test surface.
func stripJavaCommentsAndLiterals(src []byte) []byte {
	out := append([]byte(nil), src...)
	const (
		javaLexCode = iota
		javaLexLineComment
		javaLexBlockComment
		javaLexString
		javaLexChar
		javaLexTextBlock
	)
	state := javaLexCode
	for i := 0; i < len(src); i++ {
		switch state {
		case javaLexCode:
			switch {
			case i+1 < len(src) && src[i] == '/' && src[i+1] == '/':
				out[i], out[i+1] = ' ', ' '
				i++
				state = javaLexLineComment
			case i+1 < len(src) && src[i] == '/' && src[i+1] == '*':
				out[i], out[i+1] = ' ', ' '
				i++
				state = javaLexBlockComment
			case i+2 < len(src) && src[i] == '"' && src[i+1] == '"' && src[i+2] == '"':
				out[i], out[i+1], out[i+2] = ' ', ' ', ' '
				i += 2
				state = javaLexTextBlock
			case src[i] == '"':
				out[i] = ' '
				state = javaLexString
			case src[i] == '\'':
				out[i] = ' '
				state = javaLexChar
			}
		case javaLexLineComment:
			if src[i] == '\n' || src[i] == '\r' {
				state = javaLexCode
			} else {
				out[i] = ' '
			}
		case javaLexBlockComment:
			out[i] = ' '
			if i+1 < len(src) && src[i] == '*' && src[i+1] == '/' {
				out[i+1] = ' '
				i++
				state = javaLexCode
			}
		case javaLexString, javaLexChar:
			out[i] = ' '
			if src[i] == '\\' && i+1 < len(src) {
				out[i+1] = ' '
				i++
				continue
			}
			if (state == javaLexString && src[i] == '"') || (state == javaLexChar && src[i] == '\'') {
				state = javaLexCode
			}
		case javaLexTextBlock:
			out[i] = ' '
			if i+2 < len(src) && src[i] == '"' && src[i+1] == '"' && src[i+2] == '"' {
				out[i+1], out[i+2] = ' ', ' '
				i += 2
				state = javaLexCode
			}
		}
	}
	return out
}

func manifestlessJavaHasTopLevelMain(src []byte, className string) bool {
	classRE := regexp.MustCompile(`\bclass\s+` + regexp.QuoteMeta(className) + `\b`)
	classLoc := classRE.FindIndex(src)
	if len(classLoc) != 2 {
		return false
	}
	depthBeforeClass := 0
	for _, b := range src[:classLoc[0]] {
		switch b {
		case '{':
			depthBeforeClass++
		case '}':
			if depthBeforeClass > 0 {
				depthBeforeClass--
			}
		}
	}
	if depthBeforeClass != 0 {
		return false
	}
	openRel := strings.IndexByte(string(src[classLoc[1]:]), '{')
	if openRel < 0 {
		return false
	}
	open := classLoc[1] + openRel
	mainStarts := make(map[int]struct{})
	for _, loc := range manifestlessJavaMainPattern.FindAllIndex(src[open+1:], -1) {
		if len(loc) == 2 {
			mainStarts[open+1+loc[0]] = struct{}{}
		}
	}
	if len(mainStarts) == 0 {
		return false
	}
	depth := 1
	for i := open + 1; i < len(src); i++ {
		if depth == 1 {
			if _, ok := mainStarts[i]; ok {
				return true
			}
		}
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return false
			}
		}
	}
	return false
}

func runManifestlessJavaMainTests(ctx *types.BusContext, plan runnerPlan, source string) verificationProbeRunResult {
	surface := discoverManifestlessJavaMainSurface(plan.Root)
	if len(surface.SourcePaths) == 0 || len(surface.TestMainClasses) == 0 {
		return verificationProbeRunResult{Report: &types.ChangeReport{
			Passed:         true,
			NoTestsRunners: []string{"java"},
		}, Output: "manifestless Java main surface disappeared before execution"}
	}
	tmpDir, err := createVerificationProbeTempDir(ctx.RepoRoot, "java-main-tests")
	if err != nil {
		detail := fmt.Sprintf("manifestless Java main tests could not create a private class directory: %v", err)
		return verificationProbeRunResult{Report: &types.ChangeReport{
			Passed:         false,
			FailureKind:    types.FailureKindParserError,
			FailureSummary: detail,
		}, Output: detail}
	}
	defer os.RemoveAll(tmpDir)

	sourceArgs := make([]string, 0, len(surface.SourcePaths)+5)
	sourceArgs = append(sourceArgs, "-encoding", "UTF-8", "-d", tmpDir)
	for _, rel := range surface.SourcePaths {
		sourceArgs = append(sourceArgs, filepath.Join(plan.Root, filepath.FromSlash(rel)))
	}
	compileText := fmt.Sprintf("javac <manifestless-java-main sources=%d>", len(surface.SourcePaths))
	compileOutput, compileExit, compileDuration, compileExitKind, compileErr := runManifestlessJavaCommand(ctx, plan.Root, "javac", sourceArgs)
	compileOutcome, compileKind := manifestlessJavaCommandFailure("javac", compileErr, compileOutput, compileExitKind)
	commands := []types.ExecutedCommand{{
		Runner:     "java",
		Framework:  javaFrameworkDirectMain,
		WorkingDir: runnerPlanRel(ctx.RepoRoot, plan),
		Command:    compileText,
		ExitCode:   compileExit,
		DurationMS: compileDuration.Milliseconds(),
		Source:     source,
		Outcome:    types.ExecutedCommandOutcomeSyntaxPreflight,
	}}
	if compileErr != nil {
		commands[0].Outcome = compileOutcome
		detail := manifestlessJavaFailureDetail(compileOutput, compileErr, compileText)
		report := &types.ChangeReport{
			Passed:         false,
			FailureKind:    compileKind,
			FailureSummary: detail,
			BuildFailed:    compileKind == types.FailureKindBuildFailure,
		}
		return verificationProbeRunResult{Report: report, Output: detail, Commands: commands}
	}

	var output strings.Builder
	if strings.TrimSpace(compileOutput) != "" {
		output.WriteString(compileOutput)
		output.WriteByte('\n')
	}
	report := &types.ChangeReport{Passed: true}
	for _, mainClass := range surface.TestMainClasses {
		commandText := "java -ea " + mainClass
		runOutput, runExit, runDuration, runExitKind, runErr := runManifestlessJavaCommand(
			ctx, plan.Root, "java", []string{"-ea", "-cp", tmpDir, mainClass},
		)
		outcome, failureKind := manifestlessJavaCommandFailure("java", runErr, runOutput, runExitKind)
		commands = append(commands, types.ExecutedCommand{
			Runner:     "java",
			Framework:  javaFrameworkDirectMain,
			WorkingDir: runnerPlanRel(ctx.RepoRoot, plan),
			Command:    commandText,
			ExitCode:   runExit,
			DurationMS: runDuration.Milliseconds(),
			Source:     source,
			Outcome:    outcome,
		})
		if strings.TrimSpace(runOutput) != "" {
			fmt.Fprintf(&output, "[%s]\n%s\n", mainClass, strings.TrimSpace(runOutput))
		}
		passed := runErr == nil
		result := types.TestResult{
			Kind:        types.TestResultKindUnit,
			AssertionID: mainClass,
			Suite:       "manifestless-java-main",
			Passed:      passed,
			Duration:    runDuration,
		}
		if !passed {
			result.FailureDetail = manifestlessJavaFailureDetail(runOutput, runErr, commandText)
			report.Passed = false
			report.FailureKind = failureKind
			report.FailureSummary = result.FailureDetail
			report.TestResults = append(report.TestResults, result)
			break
		}
		report.TestResults = append(report.TestResults, result)
	}
	return verificationProbeRunResult{Report: report, Output: strings.TrimSpace(output.String()), Commands: commands}
}

func runManifestlessJavaCommand(ctx *types.BusContext, wd, binary string, args []string) (string, int, time.Duration, SupervisedExitKind, error) {
	timeout := 2 * time.Minute
	execCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(execCtx, binary, args...)
	cmd.Dir = wd
	cmd.Env = runnerExecutionEnv("java", ctx.RepoRoot, wd, ctx.MainRepoRoot)
	return runVerificationProbeCommand(execCtx, cmd, timeout)
}

func manifestlessJavaCommandFailure(binary string, err error, output string, exitKind SupervisedExitKind) (string, types.FailureKind) {
	if err == nil {
		return types.ExecutedCommandOutcomeExecuted, ""
	}
	if verificationProbeRunnerMissing(binary, err, output) || javaRuntimeMissingOutput(output) {
		return types.ExecutedCommandOutcomeRunnerMissing, types.FailureKindRunnerMissing
	}
	switch exitKind {
	case SupervisedExitTimeout:
		return types.ExecutedCommandOutcomeTimeout, types.FailureKindTimeout
	case SupervisedExitOOM:
		return types.ExecutedCommandOutcomeOOM, types.FailureKindOOM
	case SupervisedExitCPULimit:
		return types.ExecutedCommandOutcomeCPULimit, types.FailureKindCPULimit
	}
	if binary == "javac" {
		return types.ExecutedCommandOutcomeExecuted, types.FailureKindBuildFailure
	}
	return types.ExecutedCommandOutcomeExecuted, types.FailureKindTestsFailed
}

func manifestlessJavaFailureDetail(output string, err error, command string) string {
	detail := strings.TrimSpace(output)
	if detail == "" && err != nil {
		detail = err.Error()
	}
	if detail == "" {
		detail = command + " failed"
	}
	return stdoutHead(detail, 4000)
}
