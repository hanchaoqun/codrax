package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/safety"
	"github.com/hanchaoqun/codrax/internal/types"
)

// SHELLGUARD-1 (§29.213 排期件2) — read-only shell guardrail narrowing.
//
// The exec_command read-only surface is an LLM-mistake guardrail, NOT an
// adversarial security sandbox (§29.213 ruling). The narrowing arms below
// (awk/sed program-body semantics, git branch display-only, per-command path
// operand models) must therefore NEVER break the awk/sed/git usages the
// system's own skill prompts and tool descriptions recommend:
//   - builtin.go ExecCommand.Description(): "grep -n or awk printing NR"
//   - builtin.go execCommandSearchShapeAdvisory(): the literal repair
//     suggestion `awk '... { printf "%d:%s\n", NR, $0 }' ...`
//   - skill/defaults.go TRACE QUERY lane: "hand-written grep/awk loops" as
//     the documented diagnostic fallback for unsupported trace formats.
// This file pins those lanes FIRST (survival pins), then pins the narrowed
// rejection surface per batch item.

// TestShellGuardSkillLaneSurvivalPins is the first-constraint pin family:
// every command shape the system's own guidance recommends must stay
// allowed by validateReadOnlyExecCommand after the SHELLGUARD-1 narrowing.
func TestShellGuardSkillLaneSurvivalPins(t *testing.T) {
	commands := []string{
		// awk printing NR — the exact repair form the no-line-number
		// advisory (builtin.go execCommandSearchShapeAdvisory) recommends.
		`awk '/2942\.256/ { printf "%d:%s\n", NR, $0 }' record_trace.systrace | head -100`,
		`awk '{print NR": "$0}' trace.txt`,
		`awk '/sched_switch/ {print NR, $0}' record_trace.systrace`,
		// awk -F column extraction (tool-description lane).
		`awk -F: '{print $1}' go.sum`,
		`awk -F '\t' '{print $2}' data.tsv`,
		`awk -v n=3 '{print $n}' file.txt`,
		// awk /pattern/ match — regex literals may contain | (alternation)
		// and -> ; neither is a command pipe or a redirection.
		`awk '/->/{print NR}' internal/tool/builtin.go`,
		`awk '/2942\.1244[1-9]|2942\.260[0-1][0-9]/ && /\[GT\]ColdPool#5-36624/' record_trace.systrace`,
		`awk '/func Execute/' internal/tool/builtin.go | head -20`,
		// awk END/aggregate forms (scalar count lane).
		`awk 'END { print NR }' record_trace.systrace`,
		`awk '{n[$1]++} END {for (k in n) print k, n[k]}' access.log`,
		// awk comparison `>` outside a print statement is a comparison,
		// not a redirection; `>` inside parens of a print is a comparison.
		`awk '$3 > 100 {print NR, $0}' data.txt`,
		`awk '{print ($3 > 100) ? "hot" : "cold"}' data.txt`,
		`awk '{ if ($2 > prev) prev = $2 } END { print prev }' data.txt`,
		// awk division must not be mistaken for a regex start.
		`awk '{print $1/2, $2/3}' data.txt`,
		// sed region print / substitute (spec 件2 survival forms).
		`sed -n '10,20p' internal/tool/builtin.go`,
		`sed -n '/pattern/p' internal/tool/builtin.go`,
		`sed 's/a/b/g' file.txt`,
		`sed -E 's/(a|b)+/x/' file.txt`,
		`sed -n '5p;10p' file.txt`,
		`sed -e 's/a/b/' -e '/x/d' file.txt`,
		`sed 's/\/etc\/passwd/redacted/' notes.txt`,
		`sed '$d' file.txt`,
		// git branch display forms (spec 件3 allowed set).
		`git branch`,
		`git branch --list 'release/*'`,
		`git branch -l 'release/*'`,
		`git branch --show-current`,
		`git branch -a -v`,
		`git branch -vv --contains abc123`,
		`git branch --merged main`,
		`git branch --no-merged main`,
		`git branch -r`,
		`git branch --all --sort=-committerdate`,
		`git branch --list --format='%(refname:short)'`,
		// in-repo read family: PATTERN args are never path-validated even
		// when they contain / or .. (spec hard rule).
		`cat go.mod`,
		`cat internal/tool/builtin.go go.sum`,
		`rg -n 'foo/bar' internal`,
		`rg -e '\.\./escape' internal/tool`,
		`grep -rn '\.\./escape' internal`,
		`grep -n 'internal/tool' README.md`,
		`grep -R 'StageExplore' internal | head -5`,
		`head -n 20 go.mod`,
		`tail -n 50 build.log`,
		`wc -l go.mod internal/tool/builtin.go`,
		`sort -u names.txt | uniq -c | sort -rn | head -10`,
		`cut -d: -f1 go.sum`,
		`du -sh internal`,
		`ls -la internal/tool`,
		`stat -c '%s' go.mod`,
		`find internal/tool -name '*_test.go' | wc -l`,
		`find . -type f -name '*.go' -print0 | xargs -0 wc -l`,
		`jq '.name' package.json`,
		`tr -d '\r' < /dev/null`,
	}
	for _, command := range commands {
		if err := validateReadOnlyExecCommand(command); err != nil {
			t.Fatalf("skill-lane survival pin: validateReadOnlyExecCommand(%q) = %v, want allowed", command, err)
		}
	}
}

// TestShellGuardSkillLaneSurvivalPinsInRepoScope re-runs the load-bearing
// survival forms with a REAL command root, so the path-operand arm (件4)
// cannot regress the in-repo lanes: relative operands, absolute operands
// under the root, and operands under the temp escape lane all stay allowed.
func TestShellGuardSkillLaneSurvivalPinsInRepoScope(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "tool"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module demo\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	tracePath := filepath.Join(t.TempDir(), "record_trace.systrace")
	if err := os.WriteFile(tracePath, []byte("2942.124416: sched_switch\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	scope := readOnlyExecScope{CommandRoot: root}
	commands := []string{
		// relative operands stay the common case
		`awk '/sched_switch/ { printf "%d:%s\n", NR, $0 }' go.mod`,
		`sed -n '1,5p' go.mod`,
		`cat go.mod`,
		// absolute operand under the command root is inside the promise
		`cat ` + shellQuoteWord(filepath.Join(root, "go.mod")),
		`grep -n module ` + shellQuoteWord(filepath.Join(root, "go.mod")),
		// temp escape lane: runtime artifacts staged under the system
		// temp scratchpad stay grep-able (the trace fallback lane)
		`grep -n sched_switch ` + shellQuoteWord(tracePath),
		`awk 'END { print NR }' ` + shellQuoteWord(tracePath),
		`head -n 5 ` + shellQuoteWord(tracePath),
	}
	for _, command := range commands {
		if err := validateReadOnlyExecCommandInScope(command, scope); err != nil {
			t.Fatalf("in-scope survival pin: validateReadOnlyExecCommandInScope(%q) = %v, want allowed", command, err)
		}
	}
}

// --- 件1: awk program-body semantic validation -----------------------------

func TestShellGuardAwkProgramBodyRejections(t *testing.T) {
	cases := []struct {
		command string
		want    string
	}{
		{`awk 'BEGIN{system("rm -rf /")}' f.txt`, "system("},
		{`awk '{ system("date") }' f.txt`, "system("},
		// The lone `|` arm fires before the getline token is reached — the
		// cmd|getline form is rejected by the pipe grammar.
		{`awk '{"date" | getline d; print d}' f.txt`, "pipes"},
		{`awk 'BEGIN{ while ((getline line < "/etc/passwd") > 0) print line }'`, "getline"},
		{`awk '{print | "sh"}' f.txt`, "pipes"},
		{`awk '{print $0 > "out.txt"}' f.txt`, "redirect"},
		{`awk '{print $0 >> "out.txt"}' f.txt`, "redirect"},
		{`awk '{printf("%s\n", $0) > "out.txt"}' f.txt`, "redirect"},
		{`awk -f prog.awk f.txt`, "-f"},
		{`awk --file=prog.awk f.txt`, "--file"},
		{`awk -e 'BEGIN{system("id")}' f.txt`, "system("},
	}
	for _, tc := range cases {
		err := validateReadOnlyExecCommand(tc.command)
		if err == nil {
			t.Fatalf("awk arm: validateReadOnlyExecCommand(%q) unexpectedly allowed", tc.command)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("awk arm: refusal for %q must name %q, got %v", tc.command, tc.want, err)
		}
	}
}

// --- 件2: sed program-body validation --------------------------------------

func TestShellGuardSedProgramBodyRejections(t *testing.T) {
	cases := []struct {
		command string
		want    string
	}{
		{`sed '1e touch pwned' f.txt`, "execute"},
		{`sed 's/a/b/e' f.txt`, "execute"},
		{`sed 's/a/b/w out.txt' f.txt`, "write"},
		{`sed -n 'w out.txt' f.txt`, "write"},
		{`sed 'W out.txt' f.txt`, "write"},
		{`sed -e 's/a/b/' -e 'w out.txt' f.txt`, "write"},
		{`sed -f script.sed f.txt`, "-f"},
		{`sed --file=script.sed f.txt`, "--file"},
	}
	for _, tc := range cases {
		err := validateReadOnlyExecCommand(tc.command)
		if err == nil {
			t.Fatalf("sed arm: validateReadOnlyExecCommand(%q) unexpectedly allowed", tc.command)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("sed arm: refusal for %q must name %q, got %v", tc.command, tc.want, err)
		}
	}
}

// --- 件3: git branch display-only ------------------------------------------

func TestShellGuardGitBranchRejections(t *testing.T) {
	cases := []struct {
		command string
		want    string
	}{
		{`git branch newname`, "creates"},
		{`git branch newname HEAD~3`, "creates"},
		{`git branch -d old`, "-d"},
		{`git branch -D old`, "-D"},
		{`git branch -m a b`, "-m"},
		{`git branch -M a b`, "-M"},
		{`git branch -c a b`, "-c"},
		{`git branch -C a b`, "-C"},
		{`git branch --delete old`, "--delete"},
		{`git branch --move a b`, "--move"},
		{`git branch --copy a b`, "--copy"},
		{`git branch --set-upstream-to=origin/x feature`, "--set-upstream-to"},
		{`git branch --unset-upstream feature`, "--unset-upstream"},
		{`git branch -f feature abc123`, "-f"},
		{`git branch -u origin/x feature`, "-u"},
		{`git branch --edit-description`, "--edit-description"},
	}
	for _, tc := range cases {
		err := validateReadOnlyExecCommand(tc.command)
		if err == nil {
			t.Fatalf("git branch arm: validateReadOnlyExecCommand(%q) unexpectedly allowed", tc.command)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("git branch arm: refusal for %q must name %q, got %v", tc.command, tc.want, err)
		}
	}
}

// --- 件4: per-command path operand model -----------------------------------

func TestShellGuardPathOperandRejections(t *testing.T) {
	root := t.TempDir()
	scope := readOnlyExecScope{CommandRoot: root}
	cases := []struct {
		command string
		want    string
	}{
		{`cat /etc/passwd`, "outside"},
		{`head -n 5 /etc/shadow`, "outside"},
		{`cat ~/.ssh/id_rsa`, "outside"},
		{`rg pat ../../otherrepo/`, "escapes"},
		{`grep -rn secret ..`, "escapes"},
		{`grep -f /etc/passwd pat .`, "outside"},
		{`wc -l /var/log/system.log`, "outside"},
		{`sed -n '1p' /etc/hosts`, "outside"},
		{`awk '{print $1}' /etc/hosts`, "outside"},
		{`sort ../secrets.txt`, "escapes"},
		{`du -sh /`, "outside"},
		{`find /etc -name '*.conf'`, "outside"},
	}
	for _, tc := range cases {
		err := validateReadOnlyExecCommandInScope(tc.command, scope)
		if err == nil {
			t.Fatalf("path arm: validateReadOnlyExecCommandInScope(%q) unexpectedly allowed", tc.command)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("path arm: refusal for %q must name %q, got %v", tc.command, tc.want, err)
		}
	}

	// PATTERN slots are exempt by construction — a pattern containing / or
	// .. must never trip the path arm (spec hard rule).
	for _, command := range []string{
		`grep '/etc/passwd' go.mod`,
		`rg '\.\./escape' .`,
		`grep -e '/abs/looking/pattern' go.mod`,
	} {
		if err := validateReadOnlyExecCommandInScope(command, scope); err != nil {
			t.Fatalf("path arm: pattern slot of %q must stay unvalidated, got %v", command, err)
		}
	}
}

func TestShellGuardPathOperandSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink escape fixture is POSIX-shaped")
	}
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("s3cret\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("setup: %v", err)
	}
	scope := readOnlyExecScope{CommandRoot: root}
	// NOTE: `outside` is itself a temp dir, which the temp escape lane
	// deliberately allows (runtime artifacts live there). Build a scope
	// whose escape lane excludes it to isolate the symlink predicate.
	scope.DisableEscapeLaneForTest = true
	err := validateReadOnlyExecCommandInScope("cat link/secret.txt", scope)
	if err == nil {
		t.Fatalf("symlink escape must be refused")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlink escape refusal must name the symlink, got %v", err)
	}
	// The same relative shape without a symlink stays allowed.
	if err := os.WriteFile(filepath.Join(root, "plain.txt"), []byte("ok\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := validateReadOnlyExecCommandInScope("cat plain.txt", scope); err != nil {
		t.Fatalf("in-root read must stay allowed, got %v", err)
	}
}

func TestShellGuardEscapeLaneAllowsRuntimeArtifacts(t *testing.T) {
	root := t.TempDir()

	// Typed escape lane 1: the runtime anchor (<CWD>/.codrax) — blob/log/
	// memory artifacts are legitimate forensic reads.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	anchorPath := filepath.Join(cwd, ".codrax", "logs", "run.log")

	// Typed escape lane 2: the system temp scratchpad.
	tempPath := filepath.Join(os.TempDir(), "codrax-attached", "panic.txt")

	// Typed escape lane 3: an attached runtime artifact path (and its
	// sidecar directory — trace_query promotes siblings).
	artifactDir := t.TempDir()
	artifact := filepath.Join(artifactDir, "capture.ftrace")
	sidecar := filepath.Join(artifactDir, "capture.tracebundle.json")

	scope := readOnlyExecScope{
		CommandRoot:       root,
		AttachedArtifacts: []string{artifact},
	}
	for _, command := range []string{
		`grep -n panic ` + shellQuoteWord(anchorPath),
		`cat ` + shellQuoteWord(tempPath),
		`grep -n sched_switch ` + shellQuoteWord(artifact),
		`cat ` + shellQuoteWord(sidecar),
	} {
		if err := validateReadOnlyExecCommandInScope(command, scope); err != nil {
			t.Fatalf("escape lane: validateReadOnlyExecCommandInScope(%q) = %v, want allowed", command, err)
		}
	}
}

// --- 件4 adjacency: write-capable option forms inside read commands --------

func TestShellGuardWriteCapableOptionRejections(t *testing.T) {
	cases := []struct {
		command string
		want    string
	}{
		{`sort -o out.txt names.txt`, "-o"},
		{`sort --output=out.txt names.txt`, "--output"},
		{`sort --compress-program=gzip big.txt`, "--compress-program"},
		{`uniq in.txt out.txt`, "output"},
		{`yq -i '.a = 1' cfg.yaml`, "-i"},
		{`yq --inplace '.a = 1' cfg.yaml`, "--inplace"},
		{`rg --pre cmd pat .`, "--pre"},
		{`rg --pre=cmd pat .`, "--pre"},
		{`find . -name '*.go' -fprint out.txt`, "-fprint"},
		{`find . -fprintf out.txt '%p'`, "-fprintf"},
		{`find . -fls out.txt`, "-fls"},
	}
	for _, tc := range cases {
		err := validateReadOnlyExecCommand(tc.command)
		if err == nil {
			t.Fatalf("write-option arm: validateReadOnlyExecCommand(%q) unexpectedly allowed", tc.command)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("write-option arm: refusal for %q must name %q, got %v", tc.command, tc.want, err)
		}
	}
}

// TestShellGuardScopeFromContextProjection pins the BusContext→scope
// projection: CommandRoot rides ctx.RepoRoot, ABSOLUTE attached-artifact
// sources join the escape lane, relative sources (whose Dir would be "."
// = the whole CWD) do not.
func TestShellGuardScopeFromContextProjection(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "capture.ftrace")
	ctx := newBusContext()
	ctx.RepoRoot = "/repo/a"
	ctx.RuntimeArtifactPreflight = types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{
		Artifacts: []types.RuntimeArtifactPreflightArtifact{
			{Kind: "trace", Source: abs, Carrier: "request_path"},
			{Kind: "trace", Source: "relative_trace.systrace", Carrier: "request_path"},
		},
	})
	scope := readOnlyExecScopeFromContext(ctx)
	if scope.CommandRoot != "/repo/a" {
		t.Fatalf("scope must carry ctx.RepoRoot as command root, got %q", scope.CommandRoot)
	}
	if len(scope.AttachedArtifacts) != 1 || scope.AttachedArtifacts[0] != abs {
		t.Fatalf("scope must carry exactly the absolute artifact source, got %v", scope.AttachedArtifacts)
	}
}

// TestShellGuardExecuteWiresCommandRootScope pins the PRODUCTION wiring
// (builtin.go read arm → readOnlyExecScopeFromContext): an absolute
// operand under the command root must survive through ExecCommand.Execute
// itself. The fixture root is the package directory — deliberately NOT
// under the ambient temp escape lane — so this pin goes red if Execute
// ever passes an empty scope to the validator (a unit-level scope pin
// alone cannot see that wiring deletion).
func TestShellGuardExecuteWiresCommandRootScope(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	ctx := newBusContext()
	ctx.RepoRoot = cwd
	tool := &ExecCommand{}
	params, _ := json.Marshal(execCommandParams{Command: "head -n 1 " + shellQuoteWord(filepath.Join(cwd, "tool.go"))})
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("absolute in-root operand must pass through Execute's scope wiring, got: %s", result.Summary)
	}
	// And the refusal face still fires through the same wiring.
	params, _ = json.Marshal(execCommandParams{Command: "cat /etc/hosts"})
	result, err = tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success || !strings.Contains(result.Summary, "outside the repo command root") {
		t.Fatalf("outside-root operand must be refused through Execute, got success=%v: %s", result.Success, result.Summary)
	}
}

// The write-mode observation validator reuses the same scope machinery: a
// worktree command root admits its own absolute paths (builtin.go write arm).
func TestShellGuardWriteModeScopeReuse(t *testing.T) {
	worktree := t.TempDir()
	if err := os.WriteFile(filepath.Join(worktree, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	scope := readOnlyExecScope{CommandRoot: worktree}
	allowed := decideWriteModeExecPermissionInScope("cat "+shellQuoteWord(filepath.Join(worktree, "main.go")), scope)
	if allowed.Action != safety.PermissionAllow {
		t.Fatalf("worktree-internal absolute read must stay allowed, got %+v", allowed)
	}
	denied := decideWriteModeExecPermissionInScope("cat /etc/passwd", scope)
	if denied.Action != safety.PermissionDeny {
		t.Fatalf("outside-root read must be denied in write observation mode, got %+v", denied)
	}
}
