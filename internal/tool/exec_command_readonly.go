package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/safety"
	"github.com/hanchaoqun/codrax/internal/types"
)

// readOnlyExecScope carries the deterministic path-scope facts the
// SHELLGUARD-1 (§29.213 排期件2) path-operand arm validates against. The
// read-only shell surface is an LLM-mistake guardrail — it exists to stop
// the model from accidentally breaking its own "read-only, stay inside the
// active repository command root" promise (readOnlyExecRefusal wording) —
// NOT an adversarial security sandbox (§29.213 ruling: no OS sandbox,
// awk/sed stay on the whitelist because the system's own skill prompts
// recommend them).
type readOnlyExecScope struct {
	// CommandRoot is the directory the shell runs from (ctx.RepoRoot →
	// cmd.Dir in ExecCommand.Execute; the write worktree in write mode).
	// Empty means "root unknown": relative-path rules still apply
	// lexically and absolute operands must sit inside an escape lane.
	CommandRoot string
	// AttachedArtifacts lists runtime artifact paths the user attached
	// (RuntimeArtifactPreflight sources). Reading an attached artifact —
	// and its sidecars in the same directory, which trace_query itself
	// promotes — is a legitimate forensic form, so those directories are
	// part of the typed escape lane (§1.6: hard gates need a typed
	// escape lane).
	AttachedArtifacts []string
	// DisableEscapeLaneForTest turns off the ambient escape-lane roots
	// (system temp + runtime anchor) so tests can pin the symlink-escape
	// predicate with fixtures that necessarily live under the system
	// temp directory. Never set outside tests.
	DisableEscapeLaneForTest bool
}

// readOnlyExecScopeFromContext projects the BusContext facts the path
// arm needs. Kept minimal and deterministic: command root + attached
// runtime artifact sources.
func readOnlyExecScopeFromContext(ctx *types.BusContext) readOnlyExecScope {
	if ctx == nil {
		return readOnlyExecScope{}
	}
	scope := readOnlyExecScope{CommandRoot: ctx.RepoRoot}
	for _, artifact := range ctx.RuntimeArtifactPreflight.Artifacts {
		// Only ABSOLUTE artifact paths join the escape lane: a relative
		// source already lives under the command scope, and its Dir
		// (often ".") must not turn the whole CWD into an escape root.
		if src := strings.TrimSpace(artifact.Source); src != "" && toolPathIsAbs(src) {
			scope.AttachedArtifacts = append(scope.AttachedArtifacts, src)
		}
	}
	return scope
}

type shellTokenKind int

const (
	shellTokenWord shellTokenKind = iota
	shellTokenOp
)

type shellToken struct {
	kind shellTokenKind
	text string
}

var readModeAllowedExecCommands = map[string]bool{
	"[":        true,
	"awk":      true,
	"basename": true,
	"cat":      true,
	"cd":       true,
	"cut":      true,
	"date":     true,
	"dirname":  true,
	"du":       true,
	"echo":     true,
	"false":    true,
	"file":     true,
	"find":     true,
	"git":      true,
	"grep":     true,
	"head":     true,
	"jq":       true,
	"ls":       true,
	"printf":   true,
	"pwd":      true,
	"rg":       true,
	"sed":      true,
	"sort":     true,
	"stat":     true,
	"tail":     true,
	"test":     true,
	"tr":       true,
	"true":     true,
	"uniq":     true,
	"wc":       true,
	"xargs":    true,
	"yq":       true,
}

var readModeAllowedGitSubcommands = map[string]bool{
	"branch":     true,
	"cat-file":   true,
	"describe":   true,
	"diff":       true,
	"grep":       true,
	"log":        true,
	"ls-files":   true,
	"merge-base": true,
	"rev-parse":  true,
	"show":       true,
	"status":     true,
}

func shouldGateExecCommandAsReadOnly(ctx *types.BusContext) bool {
	if ctx == nil {
		return false
	}
	if ctx.PipelineStage.IsWrite() {
		return false
	}
	return true
}

func shouldGateExecCommandAsWriteReadOnly(ctx *types.BusContext) bool {
	if ctx == nil {
		return false
	}
	return ctx.PipelineStage.IsWrite()
}

func validateReadOnlyExecCommand(command string) error {
	return validateReadOnlyExecCommandInScope(command, readOnlyExecScope{})
}

func validateReadOnlyExecCommandInScope(command string, scope readOnlyExecScope) error {
	tokens, err := lexShellCommand(command)
	if err != nil {
		return err
	}
	if len(tokens) == 0 {
		return fmt.Errorf("empty command")
	}
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if tok.kind == shellTokenOp && shellOperatorWrites(tok.text) {
			if ok, next := readOnlyRedirectionDisposition(tokens, i); ok {
				i = next - 1
				continue
			}
			return fmt.Errorf("write-capable shell redirection %q is not allowed in read mode", tok.text)
		}
		if tok.kind == shellTokenWord && shellWordContainsCommandSubstitution(tok.text) {
			return fmt.Errorf("command substitution is not allowed in read mode")
		}
	}
	for i := 0; i < len(tokens); {
		i = skipSeparators(tokens, i)
		if i >= len(tokens) {
			break
		}
		start := i
		for i < len(tokens) && isShellAssignment(tokens[i]) {
			if err := validateReadOnlyShellAssignment(tokens[i]); err != nil {
				return err
			}
			i++
		}
		if i >= len(tokens) || tokens[i].kind != shellTokenWord {
			if start < len(tokens) {
				return fmt.Errorf("unsupported shell syntax near %q", tokens[start].text)
			}
			break
		}
		cmd := shellCommandName(tokens[i].text)
		if !readModeAllowedExecCommands[cmd] {
			return fmt.Errorf("command %q is not allowed in read mode exec_command", cmd)
		}
		argsStart := i + 1
		argsEnd := nextCommandBoundary(tokens, argsStart)
		if err := validateReadOnlyCommandArgsInScope(cmd, stripReadOnlyRedirectionTokens(tokens[argsStart:argsEnd]), scope); err != nil {
			return err
		}
		i = argsEnd
	}
	return nil
}

func normalizeReadOnlyExecCommand(command string) (string, string) {
	tokens, err := lexShellCommand(command)
	if err != nil || len(tokens) == 0 {
		return command, ""
	}
	changed := false
	for i := 0; i < len(tokens); {
		i = skipSeparators(tokens, i)
		if i >= len(tokens) {
			break
		}
		for i < len(tokens) && isShellAssignment(tokens[i]) {
			i++
		}
		if i >= len(tokens) || tokens[i].kind != shellTokenWord {
			break
		}
		cmdIdx := i
		argsStart := i + 1
		argsEnd := nextCommandBoundary(tokens, argsStart)
		if shellCommandName(tokens[cmdIdx].text) == "git" && firstGitSubcommand(shellWords(tokens[argsStart:argsEnd])) == "show" {
			out := tokens[:argsStart:argsStart]
			for _, tok := range tokens[argsStart:argsEnd] {
				if tok.kind == shellTokenWord && tok.text == "--no-stat" {
					changed = true
					continue
				}
				out = append(out, tok)
			}
			out = append(out, tokens[argsEnd:]...)
			tokens = out
			argsEnd = nextCommandBoundary(tokens, argsStart)
		}
		i = argsEnd
	}
	if !changed {
		return command, ""
	}
	return renderShellTokens(tokens), "removed unsupported `git show --no-stat` (git show does not accept it; plain `git show` already omits stat output unless --stat is requested)"
}

func readOnlyExecRefusal(ctx *types.BusContext, command string, err error) types.ToolResult {
	rootHint := "the repository root"
	if ctx != nil && strings.TrimSpace(ctx.RepoRoot) != "" {
		rootHint = fmt.Sprintf("the repository root (%s)", ctx.RepoRoot)
	}
	return types.ToolResult{
		ToolName: "exec_command",
		Success:  false,
		Summary: fmt.Sprintf(
			"exec_command refused: read-mode shell commands must be read-only and stay inside the active repository command root. %v. exec_command already runs from %s; use repo-relative paths, plain `git log` / `git show` / `git diff`, or built-in read_file / grep / repo_map / git_log / git_show / git_diff / git_history_search. The rejected command was: %s",
			err, rootHint, sanitizeForBanner(command)),
	}
}

func writeModeExecRefusal(ctx *types.BusContext, command string, err error) types.ToolResult {
	rootHint := "the write worktree or repository root"
	if ctx != nil && strings.TrimSpace(ctx.RepoRoot) != "" {
		rootHint = fmt.Sprintf("the active command root (%s)", ctx.RepoRoot)
	}
	return types.ToolResult{
		ToolName: "exec_command",
		Success:  false,
		Summary: fmt.Sprintf(
			"exec_command refused: write-mode shell commands are restricted to read-only observation commands. %s. Use apply_patch for file edits and run_tests for validation; exec_command is not an apply channel in write mode. The command would have run from %s. The rejected command was: %s",
			writeModeExecPolicyError(err), rootHint, sanitizeForBanner(command)),
	}
}

func decideWriteModeExecPermission(command string) safety.PermissionDecision {
	return decideWriteModeExecPermissionInScope(command, readOnlyExecScope{})
}

// decideWriteModeExecPermissionInScope is the write-mode observation gate.
// It reuses the read-mode validator wholesale (builtin.go write arm), so
// the SHELLGUARD-1 narrowing applies to both modes from one decision
// point; the write worktree is the command root, so worktree-internal
// absolute paths pass the path-operand arm naturally.
func decideWriteModeExecPermissionInScope(command string, scope readOnlyExecScope) safety.PermissionDecision {
	if err := validateReadOnlyExecCommandInScope(command, scope); err != nil {
		return safety.DenyPermission("write_exec_command", "write_exec_not_observation", err.Error())
	}
	return safety.AllowPermission("write_exec_command", "write_exec_observation", "command is read-only observation")
}

func writeModeExecPolicyError(err error) string {
	if err == nil {
		return "command violates write-mode observation policy"
	}
	msg := err.Error()
	msg = strings.ReplaceAll(msg, "read mode exec_command", "write-mode observation exec_command")
	msg = strings.ReplaceAll(msg, "read mode", "write-mode observation policy")
	return msg
}

func shellWords(tokens []shellToken) []string {
	out := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		if tok.kind == shellTokenWord {
			out = append(out, tok.text)
		}
	}
	return out
}

func renderShellTokens(tokens []shellToken) string {
	var b strings.Builder
	for i, tok := range tokens {
		if i > 0 {
			b.WriteByte(' ')
		}
		if tok.kind == shellTokenWord {
			b.WriteString(shellQuoteWord(tok.text))
		} else {
			b.WriteString(tok.text)
		}
	}
	return b.String()
}

func shellQuoteWord(s string) string {
	if s == "" {
		return "''"
	}
	if shellWordIsBareSafe(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func shellWordIsBareSafe(s string) bool {
	for _, r := range s {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			continue
		}
		switch r {
		case '_', '-', '.', '/', ':', '=', '@', '+', ',', '%':
			continue
		default:
			return false
		}
	}
	return true
}

func readOnlyRedirectionDisposition(tokens []shellToken, i int) (bool, int) {
	if i < 0 || i >= len(tokens) || tokens[i].kind != shellTokenOp {
		return false, i
	}
	op := strings.TrimLeft(tokens[i].text, "0123456789")
	switch op {
	case ">", ">>", ">|", "&>", "&>>", "<":
		if i+1 < len(tokens) && tokens[i+1].kind == shellTokenWord && isNullDeviceTarget(tokens[i+1].text) {
			return true, i + 2
		}
		if op == ">" && i+2 < len(tokens) &&
			tokens[i+1].kind == shellTokenOp && tokens[i+1].text == "&" &&
			tokens[i+2].kind == shellTokenWord && isShellFDNumber(tokens[i+2].text) {
			return true, i + 3
		}
	}
	return false, i
}

func stripReadOnlyRedirectionTokens(tokens []shellToken) []shellToken {
	if len(tokens) == 0 {
		return nil
	}
	out := make([]shellToken, 0, len(tokens))
	for i := 0; i < len(tokens); {
		if tokens[i].kind == shellTokenOp && shellOperatorWrites(tokens[i].text) {
			if ok, next := readOnlyRedirectionDisposition(tokens, i); ok {
				i = next
				continue
			}
		}
		out = append(out, tokens[i])
		i++
	}
	return out
}

func isNullDeviceTarget(s string) bool {
	trimmed := strings.TrimSpace(s)
	return trimmed == "/dev/null" || strings.EqualFold(trimmed, "NUL")
}

func isShellFDNumber(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func lexShellCommand(command string) ([]shellToken, error) {
	var tokens []shellToken
	var b strings.Builder
	inSingle, inDouble, escaped := false, false, false
	flush := func() {
		if b.Len() == 0 {
			return
		}
		tokens = append(tokens, shellToken{kind: shellTokenWord, text: b.String()})
		b.Reset()
	}
	for i := 0; i < len(command); i++ {
		ch := command[i]
		if escaped {
			b.WriteByte(ch)
			escaped = false
			continue
		}
		if ch == '\\' {
			if inSingle {
				b.WriteByte(ch)
			} else {
				escaped = true
			}
			continue
		}
		if ch == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if ch == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}
		if inSingle || inDouble {
			b.WriteByte(ch)
			continue
		}
		if isShellSpace(ch) {
			flush()
			continue
		}
		if isFDRedirectionStart(command, i) {
			flush()
			j := i
			for j < len(command) && command[j] >= '0' && command[j] <= '9' {
				j++
			}
			op, next := readShellOperator(command, j)
			tokens = append(tokens, shellToken{kind: shellTokenOp, text: command[i:j] + op})
			i = next - 1
			continue
		}
		if isShellOperatorStart(ch) {
			flush()
			op, next := readShellOperator(command, i)
			tokens = append(tokens, shellToken{kind: shellTokenOp, text: op})
			i = next - 1
			continue
		}
		b.WriteByte(ch)
	}
	if escaped {
		return nil, fmt.Errorf("unterminated escape in shell command")
	}
	if inSingle || inDouble {
		return nil, fmt.Errorf("unterminated quote in shell command")
	}
	flush()
	return tokens, nil
}

func isShellSpace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
}

func isShellOperatorStart(ch byte) bool {
	switch ch {
	case ';', '&', '|', '<', '>', '(', ')':
		return true
	default:
		return false
	}
}

func isFDRedirectionStart(s string, i int) bool {
	if i < 0 || i >= len(s) || s[i] < '0' || s[i] > '9' {
		return false
	}
	j := i
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		j++
	}
	return j < len(s) && (s[j] == '>' || s[j] == '<')
}

func readShellOperator(s string, i int) (string, int) {
	if i >= len(s) {
		return "", i
	}
	if i+2 <= len(s) {
		switch s[i : i+2] {
		case "&&", "||", ";;", ">>", ">|", "<<", "<>", "&>":
			if s[i:i+2] == "<<" && i+3 <= len(s) && s[i:i+3] == "<<<" {
				return "<<<", i + 3
			}
			if s[i:i+2] == "&>" && i+3 <= len(s) && s[i:i+3] == "&>>" {
				return "&>>", i + 3
			}
			return s[i : i+2], i + 2
		}
	}
	return s[i : i+1], i + 1
}

func shellOperatorWrites(op string) bool {
	op = strings.TrimLeft(op, "0123456789")
	switch op {
	case ">", ">>", ">|", "<", "<<", "<<<", "<>", "&>", "&>>":
		return true
	default:
		return false
	}
}

func shellWordContainsCommandSubstitution(word string) bool {
	return strings.Contains(word, "$(") || strings.Contains(word, "`")
}

func skipSeparators(tokens []shellToken, i int) int {
	for i < len(tokens) && tokens[i].kind == shellTokenOp {
		switch tokens[i].text {
		case ";", ";;", "&&", "||", "|", "(", ")":
			i++
			continue
		}
		break
	}
	return i
}

func isShellAssignment(tok shellToken) bool {
	if tok.kind != shellTokenWord {
		return false
	}
	text := tok.text
	idx := strings.IndexByte(text, '=')
	if idx <= 0 {
		return false
	}
	for i := 0; i < idx; i++ {
		ch := text[i]
		if !(ch == '_' || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (i > 0 && ch >= '0' && ch <= '9')) {
			return false
		}
	}
	return true
}

func shellCommandName(word string) string {
	base := filepath.Base(strings.TrimSpace(word))
	return strings.ToLower(base)
}

func nextCommandBoundary(tokens []shellToken, i int) int {
	for i < len(tokens) {
		if tokens[i].kind == shellTokenOp {
			switch tokens[i].text {
			case ";", ";;", "&&", "||", "|":
				return i
			}
		}
		i++
	}
	return i
}

func validateReadOnlyCommandArgs(cmd string, args []shellToken) error {
	return validateReadOnlyCommandArgsInScope(cmd, args, readOnlyExecScope{})
}

// validateReadOnlyCommandArgsInScope is the per-command decision point of
// the read-only guardrail (SHELLGUARD-1 narrowing): semantic arms for the
// program-carrying commands (awk/sed), a display-only arm for git branch,
// and a per-command path-operand model for the plain read family. Commands
// with no argv model (echo/printf/date/pwd/true/false/test/[ /basename/
// dirname/tr/jq-yq-filter-slots) are conservatively treated as having no
// path operands — their arguments are data, not file reads.
func validateReadOnlyCommandArgsInScope(cmd string, args []shellToken, scope readOnlyExecScope) error {
	var words []string
	for _, tok := range args {
		if tok.kind == shellTokenWord {
			words = append(words, tok.text)
		}
	}
	switch cmd {
	case "cd":
		return validateReadOnlyCdArgs(words)
	case "awk":
		return validateReadOnlyAwkArgs(words, scope)
	case "sed":
		return validateReadOnlySedArgs(words, scope)
	case "find":
		return validateReadOnlyFindArgs(words, scope)
	case "git":
		if err := validateReadOnlyGitGlobalPathOptions(words); err != nil {
			return err
		}
		sub := firstGitSubcommand(words)
		if sub == "" {
			return nil
		}
		if !readModeAllowedGitSubcommands[sub] {
			return fmt.Errorf("git subcommand %q is not allowed in read mode", sub)
		}
		if sub == "branch" {
			return validateReadOnlyGitBranchArgs(gitSubcommandTrailingArgs(words))
		}
		return nil
	case "xargs":
		return validateReadOnlyXargsArgsInScope(words, scope)
	case "jq":
		return validateReadOnlyPathOperandsInScope(scope, cmd, readOnlyJqPathOperands(words))
	case "yq":
		operands, err := readOnlyYqPathOperands(words)
		if err != nil {
			return err
		}
		return validateReadOnlyPathOperandsInScope(scope, cmd, operands)
	}
	if model, ok := readOnlyPathArgvModels[cmd]; ok {
		operands, err := model.pathOperands(cmd, words)
		if err != nil {
			return err
		}
		return validateReadOnlyPathOperandsInScope(scope, cmd, operands)
	}
	return nil
}

func validateReadOnlyShellAssignment(tok shellToken) error {
	if tok.kind != shellTokenWord {
		return nil
	}
	text := tok.text
	idx := strings.IndexByte(text, '=')
	if idx <= 0 {
		return nil
	}
	name := strings.ToUpper(strings.TrimSpace(text[:idx]))
	value := strings.TrimSpace(text[idx+1:])
	switch name {
	case "GIT_DIR", "GIT_WORK_TREE":
		return validateRepoRelativeShellPathOption(name, value)
	default:
		return nil
	}
}

func validateReadOnlyCdArgs(words []string) error {
	i := 0
	for i < len(words) {
		arg := strings.TrimSpace(words[i])
		switch arg {
		case "":
			i++
		case "--":
			i++
			goto pathArg
		case "-L", "-P":
			i++
		default:
			goto pathArg
		}
	}
pathArg:
	if i >= len(words) {
		return fmt.Errorf("cd without an explicit repo-relative path is not allowed in read mode")
	}
	if i != len(words)-1 {
		return fmt.Errorf("cd accepts exactly one repo-relative path in read mode")
	}
	return validateRepoRelativeShellPathOption("cd", words[i])
}

func validateReadOnlyGitGlobalPathOptions(words []string) error {
	for i := 0; i < len(words); i++ {
		arg := strings.TrimSpace(words[i])
		switch arg {
		case "-C", "--git-dir", "--work-tree":
			if i+1 >= len(words) {
				return fmt.Errorf("git option %q requires a repo-relative path", arg)
			}
			if err := validateRepoRelativeShellPathOption("git "+arg, words[i+1]); err != nil {
				return err
			}
			i++
		default:
			for _, prefix := range []string{"--git-dir=", "--work-tree="} {
				if strings.HasPrefix(arg, prefix) {
					if err := validateRepoRelativeShellPathOption("git "+strings.TrimSuffix(prefix, "="), strings.TrimPrefix(arg, prefix)); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func validateRepoRelativeShellPathOption(label, value string) error {
	v := strings.TrimSpace(value)
	if v == "" {
		return fmt.Errorf("%s path is empty; use a repo-relative path", label)
	}
	if normalized, ok := normalizeWindowsPOSIXPath(v); ok {
		v = normalized
	}
	if strings.HasPrefix(v, "~") || toolPathIsAbs(v) {
		return fmt.Errorf("%s path %q is outside the repo command scope; exec_command already runs from the repository root, so use `.` or a repo-relative path", label, v)
	}
	cleaned := filepath.ToSlash(filepath.Clean(v))
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("%s path %q escapes the repo command scope; use a path under the repository root", label, v)
	}
	return nil
}

func validateReadOnlyXargsArgsInScope(words []string, scope readOnlyExecScope) error {
	i := 0
	for i < len(words) {
		arg := strings.TrimSpace(words[i])
		if arg == "" {
			i++
			continue
		}
		if arg == "--" {
			i++
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			break
		}
		// -a/--arg-file reads its argument list FROM a file — that file is
		// a path operand (件4).
		if (arg == "-a" || arg == "--arg-file") && i+1 < len(words) {
			if err := validateReadOnlyPathOperandInScope(scope, "xargs", words[i+1]); err != nil {
				return err
			}
		}
		if strings.HasPrefix(arg, "--arg-file=") {
			if err := validateReadOnlyPathOperandInScope(scope, "xargs", strings.TrimPrefix(arg, "--arg-file=")); err != nil {
				return err
			}
		}
		advance, err := readOnlyXargsOptionArgCount(arg)
		if err != nil {
			return err
		}
		if i+advance > len(words) {
			return fmt.Errorf("xargs option %q requires an argument", arg)
		}
		i += advance
	}
	if i >= len(words) {
		// POSIX xargs defaults to echo, which is read-only.
		return nil
	}
	cmd := shellCommandName(words[i])
	if cmd == "xargs" {
		return fmt.Errorf("nested xargs is not allowed in read mode")
	}
	if !readModeAllowedExecCommands[cmd] {
		return fmt.Errorf("xargs nested command %q is not allowed in read mode", cmd)
	}
	nestedArgs := make([]shellToken, 0, len(words)-i-1)
	for _, arg := range words[i+1:] {
		nestedArgs = append(nestedArgs, shellToken{kind: shellTokenWord, text: arg})
	}
	if err := validateReadOnlyCommandArgsInScope(cmd, nestedArgs, scope); err != nil {
		return fmt.Errorf("xargs nested command %q is not read-only: %w", cmd, err)
	}
	return nil
}

func readOnlyXargsOptionArgCount(arg string) (int, error) {
	switch arg {
	case "-p", "--interactive":
		return 0, fmt.Errorf("xargs interactive prompt option %q is not allowed in read mode", arg)
	case "-0", "--null", "-r", "--no-run-if-empty", "-t", "--verbose", "-x", "--exit", "-o", "--open-tty", "--show-limits":
		return 1, nil
	case "-n", "--max-args", "-L", "--max-lines", "-s", "--max-chars", "-P", "--max-procs", "-I", "--replace",
		"-d", "--delimiter", "-E", "-e", "--eof", "-a", "--arg-file":
		return 2, nil
	}
	if strings.HasPrefix(arg, "--") {
		if readOnlyXargsLongOptionWithInlineValue(arg) {
			return 1, nil
		}
		return 0, fmt.Errorf("xargs option %q is not allowed in read mode", arg)
	}
	if strings.HasPrefix(arg, "-n") || strings.HasPrefix(arg, "-L") ||
		strings.HasPrefix(arg, "-s") || strings.HasPrefix(arg, "-P") ||
		strings.HasPrefix(arg, "-I") || strings.HasPrefix(arg, "-d") ||
		strings.HasPrefix(arg, "-E") || strings.HasPrefix(arg, "-e") {
		return 1, nil
	}
	if strings.HasPrefix(arg, "-") && len(arg) > 1 {
		for _, r := range strings.TrimPrefix(arg, "-") {
			switch r {
			case '0', 'r', 't', 'x', 'o':
				continue
			case 'p':
				return 0, fmt.Errorf("xargs interactive prompt option %q is not allowed in read mode", arg)
			default:
				return 0, fmt.Errorf("xargs option %q is not allowed in read mode", arg)
			}
		}
		return 1, nil
	}
	return 0, fmt.Errorf("xargs option %q is not allowed in read mode", arg)
}

func readOnlyXargsLongOptionWithInlineValue(arg string) bool {
	for _, prefix := range []string{
		"--max-args=",
		"--max-lines=",
		"--max-chars=",
		"--max-procs=",
		"--replace=",
		"--delimiter=",
		"--eof=",
		"--arg-file=",
	} {
		if strings.HasPrefix(arg, prefix) {
			return true
		}
	}
	return false
}

// --- SHELLGUARD-1 件1: awk program-body semantic validation ----------------

// validateReadOnlyAwkArgs walks awk's argv: program-from-file options are
// rejected outright (-f/--file/-E/--exec/-i/--include — a program file is a
// validator bypass), -e/--source program text is validated like the
// positional program body, -F/-v values are plain data, post-program
// var=val assignments are data, and the remaining operands are file reads
// that flow into the path-operand arm (件4).
func validateReadOnlyAwkArgs(words []string, scope readOnlyExecScope) error {
	programSeen := false
	var operands []string
	i := 0
	for ; i < len(words); i++ {
		arg := words[i]
		if arg == "--" {
			i++
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			break
		}
		switch {
		case arg == "-f" || arg == "--file" || strings.HasPrefix(arg, "--file="):
			return fmt.Errorf("awk %s program files are not allowed in read mode: the program body must be inline so the read-only validator can check it", awkOptionName(arg))
		case arg == "-E" || arg == "--exec" || strings.HasPrefix(arg, "--exec="):
			return fmt.Errorf("awk %s program files are not allowed in read mode: the program body must be inline so the read-only validator can check it", awkOptionName(arg))
		case arg == "-i" || arg == "--include" || strings.HasPrefix(arg, "--include="):
			return fmt.Errorf("awk %s include files are not allowed in read mode: the program body must be inline so the read-only validator can check it", awkOptionName(arg))
		case arg == "-e" || arg == "--source":
			if i+1 < len(words) {
				if err := validateReadOnlyAwkProgram(words[i+1]); err != nil {
					return err
				}
				programSeen = true
				i++
			}
		case strings.HasPrefix(arg, "--source="):
			if err := validateReadOnlyAwkProgram(strings.TrimPrefix(arg, "--source=")); err != nil {
				return err
			}
			programSeen = true
		case arg == "-v" || arg == "--assign" || arg == "-F" || arg == "--field-separator":
			i++ // value is plain data (separator / var=val), never code
		default:
			// -F<sep> / -v<var=val> attached forms and other flags: data.
		}
	}
	for ; i < len(words); i++ {
		arg := words[i]
		if !programSeen {
			programSeen = true
			if err := validateReadOnlyAwkProgram(arg); err != nil {
				return err
			}
			continue
		}
		if arg == "-" || isAwkAssignmentWord(arg) {
			continue
		}
		operands = append(operands, arg)
	}
	return validateReadOnlyPathOperandsInScope(scope, "awk", operands)
}

func awkOptionName(arg string) string {
	if idx := strings.IndexByte(arg, '='); idx > 0 {
		return arg[:idx]
	}
	return arg
}

func isAwkAssignmentWord(word string) bool {
	idx := strings.IndexByte(word, '=')
	if idx <= 0 {
		return false
	}
	for i := 0; i < idx; i++ {
		ch := word[i]
		if !(ch == '_' || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (i > 0 && ch >= '0' && ch <= '9')) {
			return false
		}
	}
	return true
}

// validateReadOnlyAwkProgram enforces the awk program-body decision
// grammar (件1 判定文法). The scanner walks the program at token level,
// skipping string literals ("…" with \-escapes), regex literals (/…/ with
// \-escapes and […] classes; a / starts a regex only when the previous
// significant token cannot end an expression — the classic awk lexer
// rule, so a/2 stays division) and # comments. It rejects, precisely:
//   - identifier `system` immediately followed by `(` — command execution;
//   - identifier `getline` anywhere — file/command input (both the
//     `"cmd" | getline` and `getline < "file"` forms ride on it);
//   - a single `|` operator — awk's only use of a lone pipe is piping
//     to/from an external command (`print | "cmd"`, `"cmd" | getline`);
//     `||` stays allowed;
//   - `>` / `>>` at the top parenthesis level of a print/printf statement
//     — output redirection to a file. Comparisons outside print
//     statements (`$3 > 100 {…}`) and parenthesised comparisons inside
//     print (`print ($3 > 100) ? …`) stay allowed; `>=` is always a
//     comparison.
func validateReadOnlyAwkProgram(program string) error {
	n := len(program)
	prevEndsExpr := false
	inPrint := false
	printDepth := 0
	parenDepth := 0
	endPrint := func() { inPrint = false }
	for i := 0; i < n; {
		ch := program[i]
		switch {
		case ch == ' ' || ch == '\t' || ch == '\r':
			i++
		case ch == '\n' || ch == ';':
			endPrint()
			prevEndsExpr = false
			i++
		case ch == '#':
			for i < n && program[i] != '\n' {
				i++
			}
		case ch == '"':
			i++
			for i < n {
				if program[i] == '\\' {
					i += 2
					continue
				}
				if program[i] == '"' {
					i++
					break
				}
				i++
			}
			prevEndsExpr = true
		case ch == '/' && !prevEndsExpr:
			// regex literal
			i++
			inClass := false
			for i < n {
				c := program[i]
				if c == '\\' {
					i += 2
					continue
				}
				if c == '[' {
					inClass = true
				} else if c == ']' {
					inClass = false
				} else if c == '/' && !inClass {
					i++
					break
				}
				i++
			}
			prevEndsExpr = true
		case ch == '_' || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z'):
			start := i
			for i < n && (program[i] == '_' ||
				(program[i] >= 'A' && program[i] <= 'Z') ||
				(program[i] >= 'a' && program[i] <= 'z') ||
				(program[i] >= '0' && program[i] <= '9')) {
				i++
			}
			id := program[start:i]
			switch id {
			case "system":
				j := i
				for j < n && (program[j] == ' ' || program[j] == '\t') {
					j++
				}
				if j < n && program[j] == '(' {
					return fmt.Errorf("awk program calls system(, which executes a shell command; read mode is read-only — do the work inline in awk or with grep/sed instead")
				}
				prevEndsExpr = true
			case "getline":
				return fmt.Errorf("awk getline reads from files or commands outside the validated operand list and is not allowed in read mode; pass the input file as a normal awk operand instead")
			case "print", "printf":
				inPrint = true
				printDepth = parenDepth
				prevEndsExpr = false
			default:
				prevEndsExpr = !awkKeywordBreaksExpr(id)
			}
		case ch >= '0' && ch <= '9':
			for i < n && ((program[i] >= '0' && program[i] <= '9') || program[i] == '.' ||
				program[i] == 'x' || program[i] == 'X' ||
				(program[i] >= 'a' && program[i] <= 'f') || (program[i] >= 'A' && program[i] <= 'F') ||
				program[i] == 'e' || program[i] == 'E') {
				i++
			}
			prevEndsExpr = true
		case ch == '|':
			if i+1 < n && program[i+1] == '|' {
				i += 2
				prevEndsExpr = false
				break
			}
			return fmt.Errorf("awk program pipes to or from an external command with `|`, which is not allowed in read mode; keep the processing inside one awk program")
		case ch == '>':
			if i+1 < n && program[i+1] == '=' {
				i += 2 // >= comparison
				prevEndsExpr = false
				break
			}
			width := 1
			if i+1 < n && program[i+1] == '>' {
				width = 2
			}
			if inPrint && parenDepth == printDepth {
				return fmt.Errorf("awk program redirects print output to a file with `%s`, which is not allowed in read mode; print to stdout and let exec_command capture it", program[i:i+width])
			}
			i += width
			prevEndsExpr = false
		case ch == '(':
			parenDepth++
			prevEndsExpr = false
			i++
		case ch == ')':
			parenDepth--
			if inPrint && parenDepth < printDepth {
				printDepth = parenDepth
			}
			prevEndsExpr = true
			i++
		case ch == '{' || ch == '}':
			endPrint()
			prevEndsExpr = false
			i++
		case ch == ']':
			prevEndsExpr = true
			i++
		default:
			// operators / punctuation: , = + - * % ^ ! ~ ? : [ $ & < etc.
			prevEndsExpr = false
			i++
		}
	}
	return nil
}

func awkKeywordBreaksExpr(id string) bool {
	switch id {
	case "if", "else", "while", "for", "do", "return", "in", "function", "func", "delete", "exit", "next", "nextfile":
		return true
	default:
		return false
	}
}

// --- SHELLGUARD-1 件2: sed program-body validation --------------------------

// validateReadOnlySedArgs walks sed's argv: -i (pre-existing arm) and
// -f/--file script files reject, -e/--expression scripts and the first
// positional script go through the script scanner, `r`/`R` filenames the
// scanner returns are read operands, and trailing positionals are file
// reads — both flow into the path-operand arm (件4).
func validateReadOnlySedArgs(words []string, scope readOnlyExecScope) error {
	scriptSeen := false
	var operands []string
	afterDoubleDash := false
	for i := 0; i < len(words); i++ {
		arg := words[i]
		if !afterDoubleDash && arg == "--" {
			afterDoubleDash = true
			continue
		}
		if !afterDoubleDash && strings.HasPrefix(arg, "-") && arg != "-" {
			switch {
			case arg == "-i" || arg == "--in-place" || strings.HasPrefix(arg, "--in-place=") ||
				strings.HasPrefix(arg, "-i.") || (strings.HasPrefix(arg, "-i") && len(arg) > 2):
				return fmt.Errorf("sed in-place editing is not allowed in read mode")
			case arg == "-f" || arg == "--file" || strings.HasPrefix(arg, "--file="):
				return fmt.Errorf("sed %s script files are not allowed in read mode: the script must be inline so the read-only validator can check it", awkOptionName(arg))
			case arg == "-e" || arg == "--expression":
				if i+1 < len(words) {
					reads, err := validateReadOnlySedScript(words[i+1])
					if err != nil {
						return err
					}
					operands = append(operands, reads...)
					scriptSeen = true
					i++
				}
			case strings.HasPrefix(arg, "-e") && len(arg) > 2:
				reads, err := validateReadOnlySedScript(arg[2:])
				if err != nil {
					return err
				}
				operands = append(operands, reads...)
				scriptSeen = true
			case strings.HasPrefix(arg, "--expression="):
				reads, err := validateReadOnlySedScript(strings.TrimPrefix(arg, "--expression="))
				if err != nil {
					return err
				}
				operands = append(operands, reads...)
				scriptSeen = true
			case arg == "-l" || arg == "--line-length":
				i++
			default:
				// -n -E -r -s -u -z --quiet --silent … : no argument
			}
			continue
		}
		if !scriptSeen {
			scriptSeen = true
			reads, err := validateReadOnlySedScript(arg)
			if err != nil {
				return err
			}
			operands = append(operands, reads...)
			continue
		}
		if arg == "-" {
			continue
		}
		operands = append(operands, arg)
	}
	return validateReadOnlyPathOperandsInScope(scope, "sed", operands)
}

// validateReadOnlySedScript enforces the sed script decision grammar
// (件2 判定文法). The scanner walks address prefixes (line numbers with
// GNU first~step, `$`, `/regex/` and `\cREc` with \-escapes, `,` ranges,
// `!` negation) and then dispatches on the command letter. Rejected:
// `e` (executes a shell command), `w`/`W` (write a file), and the
// `e`/`w` flags of `s///`. `r`/`R` filenames are returned as read
// operands for the path arm. `a`/`i`/`c` text, `b`/`t`/`:` labels and
// `y` transliterations are consumed as data; unknown command letters are
// skipped (guardrail fail-open — this is a mistake gate, not a sandbox).
func validateReadOnlySedScript(script string) ([]string, error) {
	var reads []string
	i := 0
	n := len(script)
	skipBlanks := func() {
		for i < n && (script[i] == ' ' || script[i] == '\t') {
			i++
		}
	}
	skipDelimited := func(delim byte) {
		for i < n {
			if script[i] == '\\' {
				i += 2
				continue
			}
			if script[i] == delim {
				i++
				return
			}
			i++
		}
	}
	parseAddr := func() bool {
		if i >= n {
			return false
		}
		switch c := script[i]; {
		case c >= '0' && c <= '9':
			for i < n && ((script[i] >= '0' && script[i] <= '9') || script[i] == '~') {
				i++
			}
			return true
		case c == '$':
			i++
			return true
		case c == '/':
			i++
			skipDelimited('/')
			for i < n && (script[i] == 'I' || script[i] == 'M') {
				i++
			}
			return true
		case c == '\\':
			i++
			if i >= n {
				return true
			}
			delim := script[i]
			i++
			skipDelimited(delim)
			for i < n && (script[i] == 'I' || script[i] == 'M') {
				i++
			}
			return true
		}
		return false
	}
	for {
		for i < n && (script[i] == ' ' || script[i] == '\t' || script[i] == '\n' || script[i] == ';') {
			i++
		}
		if i >= n {
			return reads, nil
		}
		if script[i] == '#' {
			for i < n && script[i] != '\n' {
				i++
			}
			continue
		}
		if parseAddr() {
			skipBlanks()
			if i < n && script[i] == ',' {
				i++
				skipBlanks()
				parseAddr()
				skipBlanks()
			}
		}
		for i < n && (script[i] == '!' || script[i] == ' ' || script[i] == '\t') {
			i++
		}
		if i >= n {
			return reads, nil
		}
		cmd := script[i]
		i++
		switch cmd {
		case 'e':
			return nil, fmt.Errorf("sed `e` executes a shell command, which is not allowed in read mode")
		case 'w', 'W':
			return nil, fmt.Errorf("sed `%c` writes a file, which is not allowed in read mode", cmd)
		case 'r', 'R':
			skipBlanks()
			start := i
			for i < n && script[i] != '\n' {
				i++
			}
			if name := strings.TrimSpace(script[start:i]); name != "" {
				reads = append(reads, name)
			}
		case 's':
			if i >= n {
				return reads, nil
			}
			delim := script[i]
			i++
			skipDelimited(delim)
			skipDelimited(delim)
			for i < n {
				c := script[i]
				if (c >= '0' && c <= '9') || c == 'g' || c == 'p' || c == 'i' || c == 'I' || c == 'm' || c == 'M' {
					i++
					continue
				}
				if c == 'e' {
					return nil, fmt.Errorf("sed `s///e` executes the substitution result as a shell command, which is not allowed in read mode")
				}
				if c == 'w' || (c == ' ' && strings.HasPrefix(strings.TrimLeft(script[i:], " \t"), "w")) {
					return nil, fmt.Errorf("sed `s///w` writes a file, which is not allowed in read mode")
				}
				break
			}
		case 'y':
			if i >= n {
				return reads, nil
			}
			delim := script[i]
			i++
			skipDelimited(delim)
			skipDelimited(delim)
		case 'a', 'i', 'c':
			for i < n && script[i] != '\n' {
				if script[i] == '\\' && i+1 < n {
					i += 2
					continue
				}
				i++
			}
		case 'b', 't', 'T', ':':
			for i < n && script[i] != ';' && script[i] != '\n' && script[i] != '}' {
				i++
			}
		case 'q', 'Q', 'l', 'L':
			for i < n && script[i] >= '0' && script[i] <= '9' {
				i++
			}
		default:
			// p P d D g G h H n N x z F = { } and unknown letters: no argument.
		}
	}
}

// --- SHELLGUARD-1 件3: git branch display-only ------------------------------

// gitSubcommandTrailingArgs returns the args after the first git
// subcommand token, mirroring firstGitSubcommand's global-option walk.
func gitSubcommandTrailingArgs(args []string) []string {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "" {
			continue
		}
		if gitGlobalOptionConsumesNext(arg) {
			i++
			continue
		}
		if gitGlobalOptionHasInlineValue(arg) {
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return args[i+1:]
	}
	return nil
}

// validateReadOnlyGitBranchArgs enforces the git branch decision grammar
// (件3 判定文法): the rejection predicate is "a mutating flag, or a
// positional argument outside --list mode". Allowed display forms: bare
// `branch`, `--list [pattern…]` (`-l`), `--show-current`, plus the
// display/filter flag family (-a/-r/-v/-vv/--all/--remotes/--contains/
// --no-contains/--merged/--no-merged/--points-at/--sort/--format/… —
// value-taking filters consume one following non-flag word). A bare
// positional is branch creation; -d/-D/-m/-M/-c/-C/--delete/--move/
// --copy/-f/--force/-u/--set-upstream-to/--unset-upstream/
// --edit-description mutate branch state.
func validateReadOnlyGitBranchArgs(args []string) error {
	listMode := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-d", "-D", "--delete", "-m", "-M", "--move", "-c", "-C", "--copy",
			"-f", "--force", "-u", "--set-upstream-to", "--unset-upstream", "--edit-description":
			return fmt.Errorf("git branch option %q mutates branch state and is not allowed in read mode; only display forms (bare `git branch`, --list, --show-current, -a/-r/-v/-vv, --contains, --merged, --no-merged) are allowed", arg)
		case "-l", "--list":
			listMode = true
			continue
		case "--contains", "--no-contains", "--merged", "--no-merged", "--points-at", "--sort", "--format", "--abbrev":
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
			}
			continue
		}
		if strings.HasPrefix(arg, "--set-upstream-to=") {
			return fmt.Errorf("git branch option %q mutates branch state and is not allowed in read mode (--set-upstream-to)", arg)
		}
		if strings.HasPrefix(arg, "-") {
			continue // display flags (-a/-r/-v/-vv/--color/--column/… and inline = forms)
		}
		if listMode {
			continue // --list pattern
		}
		return fmt.Errorf("git branch with a positional argument %q creates or retargets a branch and is not allowed in read mode; use `git branch --list <pattern>` to filter the display", arg)
	}
	return nil
}

// --- SHELLGUARD-1 件4: per-command path-operand model -----------------------

// validateReadOnlyFindArgs: find's path operands are the leading words
// before the first expression token (a word starting with `-`, `(` or
// `!`); expression values (-name patterns, -newer references) are data.
// Write actions extend the pre-existing reject set: -fprint/-fprint0/
// -fprintf/-fls write files.
func validateReadOnlyFindArgs(words []string, scope readOnlyExecScope) error {
	inExpression := false
	for _, arg := range words {
		if !inExpression {
			if strings.HasPrefix(arg, "-") || arg == "(" || arg == "!" {
				inExpression = true
			} else {
				if err := validateReadOnlyPathOperandInScope(scope, "find", arg); err != nil {
					return err
				}
				continue
			}
		}
		switch arg {
		case "-delete", "-exec", "-execdir", "-ok", "-okdir":
			return fmt.Errorf("find action %q is not allowed in read mode", arg)
		case "-fprint", "-fprint0", "-fprintf", "-fls":
			return fmt.Errorf("find action %q writes a file and is not allowed in read mode", arg)
		}
	}
	return nil
}

// readOnlyArgvPathModel is the per-command argv decision grammar for the
// plain read family (件4 判定文法): every argv word is classified as an
// option (exact, attached short, or inline --opt= form), an option value
// (path or non-path), a pattern-slot positional (grep/rg PATTERN — NEVER
// path-validated; patterns legitimately contain `/` and `..`), or a path
// operand. Unknown options are skipped without consuming a value
// (fail-open: a mis-modelled option value would surface as a harmless
// relative "path"; the guardrail must not reject structurally fine
// commands).
type readOnlyArgvPathModel struct {
	valueOpts         map[string]bool
	pathValueOpts     map[string]bool
	pathValuePrefixes []string
	rejectOpts        map[string]string
	rejectPrefixes    map[string]string
	patternSlot       bool
	patternOpts       map[string]bool
	maxPathOperands   int
	extraOperandError string
}

func (m readOnlyArgvPathModel) pathOperands(cmd string, words []string) ([]string, error) {
	var operands []string
	patternFilled := false
	afterDoubleDash := false
	for i := 0; i < len(words); i++ {
		arg := words[i]
		if !afterDoubleDash && arg == "--" {
			afterDoubleDash = true
			continue
		}
		if !afterDoubleDash && strings.HasPrefix(arg, "-") && arg != "-" {
			if reason, ok := m.rejectOpts[arg]; ok {
				return nil, fmt.Errorf("%s option %q %s", cmd, arg, reason)
			}
			for prefix, reason := range m.rejectPrefixes {
				if strings.HasPrefix(arg, prefix) {
					return nil, fmt.Errorf("%s option %q %s", cmd, arg, reason)
				}
			}
			if m.patternOpts[arg] {
				patternFilled = true
			}
			if m.pathValueOpts[arg] {
				if i+1 < len(words) {
					operands = append(operands, words[i+1])
					i++
				}
				continue
			}
			if m.valueOpts[arg] {
				i++
				continue
			}
			matchedInline := false
			for _, prefix := range m.pathValuePrefixes {
				if strings.HasPrefix(arg, prefix) {
					operands = append(operands, strings.TrimPrefix(arg, prefix))
					matchedInline = true
					break
				}
			}
			if matchedInline {
				continue
			}
			if len(arg) > 2 && !strings.HasPrefix(arg, "--") {
				short := arg[:2]
				if m.patternOpts[short] {
					patternFilled = true
				}
				if m.pathValueOpts[short] {
					operands = append(operands, arg[2:])
				}
			}
			continue
		}
		if m.patternSlot && !patternFilled {
			patternFilled = true
			continue
		}
		if arg == "-" {
			continue
		}
		operands = append(operands, arg)
	}
	if m.maxPathOperands > 0 && len(operands) > m.maxPathOperands {
		return nil, fmt.Errorf("%s %s", cmd, m.extraOperandError)
	}
	return operands, nil
}

func argvOptSet(opts ...string) map[string]bool {
	out := make(map[string]bool, len(opts))
	for _, opt := range opts {
		out[opt] = true
	}
	return out
}

// readOnlyPathArgvModels — the per-command path-operand models (件4).
// Whitelisted commands WITHOUT a model here or a dedicated arm above are
// conservatively treated as having no path operands: `[`, test, basename,
// dirname, date, echo, false, printf, pwd, true, tr (their arguments are
// strings/metadata probes, not data-file reads).
var readOnlyPathArgvModels = map[string]readOnlyArgvPathModel{
	"cat": {},
	"head": {
		valueOpts: argvOptSet("-n", "-c", "--lines", "--bytes"),
	},
	"tail": {
		valueOpts: argvOptSet("-n", "-c", "-s", "--lines", "--bytes", "--sleep-interval", "--pid", "--max-unchanged-stats"),
	},
	"ls": {
		valueOpts: argvOptSet("-I", "-T", "-w", "--ignore", "--tabsize", "--width", "--block-size", "--time-style", "--format", "--sort", "--hide", "--quoting-style", "--time"),
	},
	"stat": {
		valueOpts: argvOptSet("-c", "-t", "--format", "--printf"),
	},
	"wc": {
		pathValueOpts:     argvOptSet("--files0-from"),
		pathValuePrefixes: []string{"--files0-from="},
	},
	"file": {
		valueOpts:         argvOptSet("-e", "-F", "--exclude", "--separator"),
		pathValueOpts:     argvOptSet("-f", "-m", "--files-from", "--magic-file"),
		pathValuePrefixes: []string{"--files-from=", "--magic-file="},
	},
	"du": {
		valueOpts:         argvOptSet("-d", "-B", "-t", "--max-depth", "--threshold", "--block-size", "--exclude"),
		pathValueOpts:     argvOptSet("-X", "--exclude-from", "--files0-from"),
		pathValuePrefixes: []string{"--exclude-from=", "--files0-from="},
	},
	"sort": {
		valueOpts:         argvOptSet("-t", "-k", "-S", "-T", "--field-separator", "--key", "--buffer-size", "--temporary-directory", "--parallel", "--batch-size"),
		pathValueOpts:     argvOptSet("--files0-from", "--random-source"),
		pathValuePrefixes: []string{"--files0-from=", "--random-source="},
		rejectOpts: map[string]string{
			"-o":                 "writes its output file, which is not allowed in read mode; print to stdout instead",
			"--output":           "writes its output file, which is not allowed in read mode; print to stdout instead",
			"--compress-program": "runs an external compression program, which is not allowed in read mode",
		},
		rejectPrefixes: map[string]string{
			"--output=":           "writes its output file, which is not allowed in read mode; print to stdout instead",
			"--compress-program=": "runs an external compression program, which is not allowed in read mode",
		},
	},
	"uniq": {
		valueOpts:         argvOptSet("-f", "-s", "-w", "--skip-fields", "--skip-chars", "--check-chars"),
		maxPathOperands:   1,
		extraOperandError: "with a second positional operand writes that operand as its output file, which is not allowed in read mode; pipe the output instead",
	},
	"cut": {
		valueOpts: argvOptSet("-b", "-c", "-f", "-d", "--bytes", "--characters", "--fields", "--delimiter", "--output-delimiter"),
	},
	"grep": {
		patternSlot:       true,
		patternOpts:       argvOptSet("-e", "-f", "--regexp", "--file"),
		valueOpts:         argvOptSet("-e", "-m", "-A", "-B", "-C", "-d", "-D", "--regexp", "--max-count", "--after-context", "--before-context", "--context", "--include", "--exclude", "--exclude-dir", "--include-dir", "--label", "--binary-files", "--devices", "--directories"),
		pathValueOpts:     argvOptSet("-f", "--file"),
		pathValuePrefixes: []string{"--file="},
	},
	"rg": {
		patternSlot:       true,
		patternOpts:       argvOptSet("-e", "-f", "--regexp", "--file"),
		valueOpts:         argvOptSet("-e", "--regexp", "-A", "-B", "-C", "--after-context", "--before-context", "--context", "-m", "--max-count", "-g", "--glob", "--iglob", "-t", "--type", "-T", "--type-not", "--type-add", "--max-depth", "--max-filesize", "-E", "--encoding", "-M", "--max-columns", "-j", "--threads", "--color", "--colors", "--context-separator", "--engine", "--path-separator", "--regex-size-limit", "--dfa-size-limit", "--sort", "--sortr"),
		pathValueOpts:     argvOptSet("-f", "--file", "--ignore-file"),
		pathValuePrefixes: []string{"--file=", "--ignore-file="},
		rejectOpts: map[string]string{
			"--pre":          "runs an external preprocessor command per file, which is not allowed in read mode",
			"--hostname-bin": "runs an external program, which is not allowed in read mode",
		},
		rejectPrefixes: map[string]string{
			"--pre=":          "runs an external preprocessor command per file, which is not allowed in read mode",
			"--hostname-bin=": "runs an external program, which is not allowed in read mode",
		},
	},
}

// readOnlyJqPathOperands: jq's first positional is the FILTER (never a
// path); the rest are input files. --arg/--argjson consume two data
// words, --slurpfile/--rawfile consume a name plus a path, -f/--from-file
// reads the filter from a path. After --args/--jsonargs everything is
// positional string data (fail-open).
func readOnlyJqPathOperands(words []string) []string {
	var operands []string
	filterSeen := false
	for i := 0; i < len(words); i++ {
		arg := words[i]
		switch {
		case arg == "--args" || arg == "--jsonargs":
			return operands
		case arg == "--arg" || arg == "--argjson":
			i += 2
		case arg == "--slurpfile" || arg == "--rawfile":
			// consumes a variable name at i+1 and a PATH at i+2
			if i+2 < len(words) {
				operands = append(operands, words[i+2])
			}
			i += 2
		case arg == "-f" || arg == "--from-file":
			if i+1 < len(words) {
				operands = append(operands, words[i+1])
				i++
			}
			filterSeen = true
		case arg == "--indent" || arg == "-L":
			i++
		case strings.HasPrefix(arg, "-") && arg != "-":
			// other flags: no argument
		case arg == "-":
			// stdin
		default:
			if !filterSeen {
				filterSeen = true
				continue
			}
			operands = append(operands, arg)
		}
	}
	return operands
}

// readOnlyYqPathOperands: yq's in-place flag rejects (it edits the file);
// an optional eval subcommand and the first positional expression are
// skipped; trailing positionals are input files.
func readOnlyYqPathOperands(words []string) ([]string, error) {
	var operands []string
	exprSeen := false
	subcommandChecked := false
	for i := 0; i < len(words); i++ {
		arg := words[i]
		switch {
		case arg == "-i" || arg == "--inplace":
			return nil, fmt.Errorf("yq option %q edits the file in place, which is not allowed in read mode; print to stdout instead", arg)
		case arg == "-o" || arg == "--output-format" || arg == "-p" || arg == "--input-format" ||
			arg == "--indent" || arg == "-s" || arg == "--split-exp" || arg == "--expression":
			i++
		case arg == "--from-file":
			if i+1 < len(words) {
				operands = append(operands, words[i+1])
				i++
			}
			exprSeen = true
		case strings.HasPrefix(arg, "-") && arg != "-":
			// other flags: no argument
		case arg == "-":
			// stdin
		default:
			if !subcommandChecked {
				subcommandChecked = true
				switch strings.ToLower(arg) {
				case "eval", "e", "eval-all", "ea":
					continue
				}
			}
			if !exprSeen {
				exprSeen = true
				continue
			}
			operands = append(operands, arg)
		}
	}
	return operands, nil
}

func validateReadOnlyPathOperandsInScope(scope readOnlyExecScope, cmd string, operands []string) error {
	for _, operand := range operands {
		if err := validateReadOnlyPathOperandInScope(scope, cmd, operand); err != nil {
			return err
		}
	}
	return nil
}

// validateReadOnlyPathOperandInScope enforces the "stay inside the active
// repository command root" half of the refusal contract on one path
// operand (件4 判定文法, token/argv precise — no whole-line heuristics):
//  1. ""/"-" (stdin) never rejects.
//  2. "~…" rejects (home escape).
//  3. Relative operands: filepath.Clean; ".." or "../…" rejects (lexical
//     escape). With a known command root, an EXISTING target must
//     EvalSymlinks-resolve inside the canonical root or an escape-lane
//     root (symlink escape); nonexistent paths — including glob words —
//     stay lexical-only (mistake guardrail, not a sandbox).
//  4. Absolute operands: allowed iff the canonicalized path sits inside
//     the canonical command root or an escape-lane root.
//
// Typed escape lane (§1.6 red line — hard gates carry a typed escape):
// the system temp scratchpad roots (os.TempDir, /tmp, /var/tmp,
// /private/tmp), the runtime anchor <CWD>/.codrax (blob/log/memory run
// artifacts are legitimate forensic reads), and the directories of
// attached runtime artifacts (RuntimeArtifactPreflight sources — the
// documented trace grep/awk fallback lane, sidecars included).
func validateReadOnlyPathOperandInScope(scope readOnlyExecScope, cmd, operand string) error {
	v := strings.TrimSpace(operand)
	if v == "" || v == "-" {
		return nil
	}
	if normalized, ok := normalizeWindowsPOSIXPath(v); ok {
		v = normalized
	}
	if strings.HasPrefix(v, "~") {
		return fmt.Errorf("%s path %q is outside the repo command root (home-relative paths are not allowed in read mode); use a repo-relative path", cmd, operand)
	}
	if toolPathIsAbs(v) {
		if readOnlyPathWithinScopeRoots(filepath.Clean(v), scope) {
			return nil
		}
		return fmt.Errorf("%s path %q is outside the repo command root; exec_command already runs from the repository root, so use a repo-relative path (attached runtime artifacts and .codrax run artifacts stay readable)", cmd, operand)
	}
	cleaned := filepath.ToSlash(filepath.Clean(v))
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("%s path %q escapes the repo command root; use a path under the repository root", cmd, operand)
	}
	if scope.CommandRoot != "" {
		full := filepath.Join(scope.CommandRoot, v)
		if resolved, err := filepath.EvalSymlinks(full); err == nil {
			if !readOnlyPathWithinScopeRoots(resolved, scope) {
				return fmt.Errorf("%s path %q resolves through a symlink to %q outside the repo command root", cmd, operand, resolved)
			}
		}
	}
	return nil
}

func readOnlyPathWithinScopeRoots(path string, scope readOnlyExecScope) bool {
	candidates := []string{filepath.Clean(path)}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		candidates = append(candidates, resolved)
	}
	for _, root := range readOnlyExecScopeRoots(scope) {
		for _, candidate := range candidates {
			if readOnlyPathHasRoot(candidate, root) {
				return true
			}
		}
	}
	return false
}

// readOnlyExecScopeRoots assembles the allowed containment roots: the
// command root plus the typed escape lane. Each root is kept in both its
// lexical and symlink-resolved form so /tmp-style symlinked roots match
// operands written either way.
func readOnlyExecScopeRoots(scope readOnlyExecScope) []string {
	var roots []string
	add := func(dir string) {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			return
		}
		cleaned := filepath.Clean(dir)
		roots = append(roots, cleaned)
		if resolved, err := filepath.EvalSymlinks(cleaned); err == nil && resolved != cleaned {
			roots = append(roots, resolved)
		}
	}
	add(scope.CommandRoot)
	for _, artifact := range scope.AttachedArtifacts {
		add(filepath.Dir(artifact))
	}
	if !scope.DisableEscapeLaneForTest {
		add(os.TempDir())
		add("/tmp")
		add("/var/tmp")
		add("/private/tmp")
		if cwd, err := os.Getwd(); err == nil {
			add(filepath.Join(cwd, ".codrax"))
		}
	}
	return roots
}

func readOnlyPathHasRoot(path, root string) bool {
	if path == root {
		return true
	}
	sep := string(filepath.Separator)
	return strings.HasPrefix(path, strings.TrimSuffix(root, sep)+sep)
}

func firstGitSubcommand(args []string) string {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "" {
			continue
		}
		if gitGlobalOptionConsumesNext(arg) {
			i++
			continue
		}
		if gitGlobalOptionHasInlineValue(arg) {
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return strings.ToLower(arg)
	}
	return ""
}

func gitGlobalOptionConsumesNext(arg string) bool {
	switch arg {
	case "-C", "-c", "--git-dir", "--work-tree", "--namespace", "--exec-path", "--config-env", "--super-prefix":
		return true
	default:
		return false
	}
}

func gitGlobalOptionHasInlineValue(arg string) bool {
	for _, prefix := range []string{
		"--git-dir=",
		"--work-tree=",
		"--namespace=",
		"--exec-path=",
		"--config-env=",
		"--super-prefix=",
	} {
		if strings.HasPrefix(arg, prefix) {
			return true
		}
	}
	return false
}
