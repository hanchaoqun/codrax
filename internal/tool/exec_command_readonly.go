package tool

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

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

func validateReadOnlyExecCommand(command string) error {
	tokens, err := lexShellCommand(command)
	if err != nil {
		return err
	}
	if len(tokens) == 0 {
		return fmt.Errorf("empty command")
	}
	for _, tok := range tokens {
		if tok.kind == shellTokenOp && shellOperatorWrites(tok.text) {
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
		if err := validateReadOnlyCommandArgs(cmd, tokens[argsStart:argsEnd]); err != nil {
			return err
		}
		i = argsEnd
	}
	return nil
}

func readOnlyExecRefusal(command string, err error) types.ToolResult {
	return types.ToolResult{
		ToolName: "exec_command",
		Success:  false,
		Summary: fmt.Sprintf(
			"exec_command refused: read-mode shell commands must be read-only. %v. Use built-in read_file / grep / repo_map for repository inspection, or use a read-only pipeline such as `find ... | wc -l` for deterministic counts. The rejected command was: %s",
			err, sanitizeForBanner(command)),
	}
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
	var words []string
	for _, tok := range args {
		if tok.kind == shellTokenWord {
			words = append(words, tok.text)
		}
	}
	switch cmd {
	case "sed":
		for _, arg := range words {
			if arg == "-i" || arg == "--in-place" || strings.HasPrefix(arg, "-i.") || strings.HasPrefix(arg, "-i") && len(arg) > 2 {
				return fmt.Errorf("sed in-place editing is not allowed in read mode")
			}
		}
	case "find":
		for _, arg := range words {
			switch arg {
			case "-delete", "-exec", "-execdir", "-ok", "-okdir":
				return fmt.Errorf("find action %q is not allowed in read mode", arg)
			}
		}
	case "git":
		sub := firstGitSubcommand(words)
		if sub == "" {
			return nil
		}
		if !readModeAllowedGitSubcommands[sub] {
			return fmt.Errorf("git subcommand %q is not allowed in read mode", sub)
		}
	case "xargs":
		return validateReadOnlyXargsArgs(words)
	}
	return nil
}

func validateReadOnlyXargsArgs(words []string) error {
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
	if err := validateReadOnlyCommandArgs(cmd, nestedArgs); err != nil {
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

func firstGitSubcommand(args []string) string {
	for _, arg := range args {
		if arg == "" {
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return strings.ToLower(arg)
	}
	return ""
}
