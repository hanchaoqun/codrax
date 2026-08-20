package tool

import (
	"bytes"
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

const verificationProbeSyntaxTimeout = 5 * time.Second

// validateVerificationProbeSyntax rejects a plan before it becomes workflow
// authority when an available syntax-only parser proves that an inline probe
// is malformed. It never executes probe code, imports product modules, or
// treats a missing parser/runtime as a failure. Runtime availability and
// dependency resolution remain execution-time typed observations.
func validateVerificationProbeSyntax(ctx *types.BusContext, probes []types.VerificationProbe) string {
	for i, probe := range probes {
		language, ok := normalizeVerificationProbeLanguage(probe.Language)
		if !ok {
			continue
		}
		if detail := verificationProbeSyntaxError(ctx, language, probe.Code); detail != "" {
			id := strings.TrimSpace(probe.ID)
			if id == "" {
				id = fmt.Sprintf("probe-%d", i+1)
			}
			return fmt.Sprintf(
				"verification_probes[%d] id=%q language=%s has invalid syntax: %s; repair the probe source before the plan is accepted",
				i, id, language, detail,
			)
		}
	}
	return ""
}

func verificationProbeSyntaxError(ctx *types.BusContext, language, code string) string {
	switch language {
	case "go":
		_, err := parser.ParseFile(token.NewFileSet(), "codrax_verification_probe.go", code, parser.AllErrors)
		if err != nil {
			return boundedVerificationProbeSyntaxDetail(err.Error())
		}
		return ""
	case "python":
		binary := pythonSyntaxCheckInterpreter(ctx)
		if !verificationProbeSyntaxExecutableAvailable(binary) {
			return ""
		}
		return runVerificationProbeSyntaxCommand(binary, []string{
			"-I", "-c", "import ast, sys; ast.parse(sys.stdin.read(), filename='<codrax_verification_probe>', mode='exec')",
		}, code, []string{"SyntaxError"})
	case "javascript":
		if !verificationProbeSyntaxExecutableAvailable("node") {
			return ""
		}
		return runVerificationProbeSyntaxCommand("node", []string{"--check", "-"}, code, []string{"SyntaxError"})
	case "ruby":
		if !verificationProbeSyntaxExecutableAvailable("ruby") {
			return ""
		}
		return runVerificationProbeSyntaxCommand("ruby", []string{"-c"}, code, []string{"syntax error", "unterminated"})
	case "java":
		return javaVerificationProbeSyntaxError(ctx, code)
	default:
		return ""
	}
}

func pythonSyntaxCheckInterpreter(ctx *types.BusContext) string {
	if ctx == nil {
		return pythonRuntimeInterpreter()
	}
	return pythonRuntimeInterpreter(ctx.RepoRoot, ctx.MainRepoRoot)
}

func verificationProbeSyntaxExecutableAvailable(binary string) bool {
	binary = strings.TrimSpace(binary)
	if binary == "" {
		return false
	}
	if filepath.IsAbs(binary) || strings.ContainsAny(binary, `/\\`) {
		info, err := os.Stat(binary)
		return err == nil && !info.IsDir()
	}
	_, err := exec.LookPath(binary)
	return err == nil
}

func runVerificationProbeSyntaxCommand(binary string, args []string, source string, syntaxMarkers []string) string {
	execCtx, cancel := context.WithTimeout(context.Background(), verificationProbeSyntaxTimeout)
	defer cancel()
	cmd := exec.CommandContext(execCtx, binary, args...)
	cmd.Stdin = strings.NewReader(source)
	var stderr bytes.Buffer
	cmd.Stdout = &stderr
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil || execCtx.Err() != nil {
		return ""
	}
	output := stderr.String()
	for _, marker := range syntaxMarkers {
		if strings.Contains(output, marker) {
			return boundedVerificationProbeSyntaxDetail(output)
		}
	}
	// A binary can be present but unusable (launcher stub, incompatible
	// option, loader failure). That is not proof that the probe is malformed.
	return ""
}

func javaVerificationProbeSyntaxError(ctx *types.BusContext, code string) string {
	if !verificationProbeSyntaxExecutableAvailable("javac") {
		return ""
	}
	repoRoot := ""
	if ctx != nil {
		repoRoot = ctx.RepoRoot
	}
	tmpDir, err := createVerificationProbeTempDir(repoRoot, "java-syntax")
	if err != nil {
		return ""
	}
	defer os.RemoveAll(tmpDir)
	sourcePath := filepath.Join(tmpDir, "CodraxVerificationProbe.java")
	if err := os.WriteFile(sourcePath, []byte(javaVerificationProbeSource(code)), 0o600); err != nil {
		return ""
	}
	execCtx, cancel := context.WithTimeout(context.Background(), verificationProbeSyntaxTimeout)
	defer cancel()
	cmd := exec.CommandContext(execCtx, "javac",
		"-J-Duser.language=en", "-J-Duser.country=US",
		"-XDrawDiagnostics", "-proc:none", "-d", tmpDir, sourcePath,
	)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err == nil || execCtx.Err() != nil {
		return ""
	}
	detail := output.String()
	if !javaOutputHasDefiniteSyntaxDiagnostic(detail) {
		// Missing imports, types, symbols, classpaths, or modules are semantic
		// environment observations. They must not become an emit-time hard gate.
		return ""
	}
	return boundedVerificationProbeSyntaxDetail(detail)
}

func javaOutputHasDefiniteSyntaxDiagnostic(output string) bool {
	for _, marker := range []string{
		"compiler.err.expected",
		"compiler.err.illegal.start.of.expr",
		"compiler.err.illegal.start.of.type",
		"compiler.err.unclosed.str.lit",
		"compiler.err.premature.eof",
		"compiler.err.not.stmt",
		"compiler.err.else.without.if",
		"compiler.err.try.without.catch.finally.or.resource.decls",
		"compiler.err.catch.without.try",
		"compiler.err.finally.without.try",
		"compiler.err.orphaned",
		"compiler.err.reached.end.of.file.while.parsing",
	} {
		if strings.Contains(output, marker) {
			return true
		}
	}
	return false
}

func boundedVerificationProbeSyntaxDetail(raw string) string {
	detail := strings.TrimSpace(raw)
	if detail == "" {
		return "syntax parser rejected the source"
	}
	detail = strings.Join(strings.Fields(detail), " ")
	const maxBytes = 1200
	if len(detail) > maxBytes {
		detail = detail[:maxBytes] + "..."
	}
	return detail
}
