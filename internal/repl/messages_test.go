package repl

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestFriendlyRunError_TranslatesContextCanceled locks the UX
// contract that "context canceled" — the canonical Ctrl+C symptom —
// gets translated to user-actionable text in BOTH zh and en. Pre-fix
// path printed the raw stream-level error which gave no recovery hint.
func TestFriendlyRunError_TranslatesContextCanceled(t *testing.T) {
	err := fmt.Errorf("LLM call failed: read stream: context canceled")
	enGot := friendlyRunError("en", err)
	if !strings.Contains(enGot, "interrupted") {
		t.Errorf("en: expected 'interrupted' in friendly text, got %q", enGot)
	}
	zhGot := friendlyRunError("zh", err)
	if !strings.Contains(zhGot, "中断") {
		t.Errorf("zh: expected '中断' in friendly text, got %q", zhGot)
	}
}

// TestFriendlyRunError_TranslatesStreamStalled pins the typed
// StreamStalledError path: when the LLM streaming watchdog aborts
// a hung upstream stream, the user sees a dedicated "upstream
// stalled" message naming the idle duration — NOT the generic
// Ctrl+C / network-disconnect prose. Pre-fix the wrapper produced
// "read stream: context canceled" which was substring-matched as
// "context canceled" → mis-attributed to Ctrl+C.
func TestFriendlyRunError_TranslatesStreamStalled(t *testing.T) {
	stalled := &llm.StreamStalledError{
		IdleFor: 60 * time.Second,
		Cause:   fmt.Errorf("read stream: %w", context.Canceled),
	}
	// Wrap in a layer to mirror the real pipeline shape: the orchestrator
	// + agent layers fmt.Errorf-wrap the LLM error before it reaches the
	// REPL's friendlyRunError.
	wrapped := fmt.Errorf("LLM call failed: %w", stalled)

	enGot := friendlyRunError("en", wrapped)
	if !strings.Contains(strings.ToLower(enGot), "upstream llm stream stalled") {
		t.Errorf("en: expected 'upstream LLM stream stalled' phrase; got %q", enGot)
	}
	if !strings.Contains(enGot, "60s") && !strings.Contains(enGot, "1m0s") {
		t.Errorf("en: expected idle duration in friendly text; got %q", enGot)
	}
	if strings.Contains(enGot, "Ctrl+C") {
		t.Errorf("en: stalled message must NOT mention Ctrl+C; got %q", enGot)
	}

	zhGot := friendlyRunError("zh", wrapped)
	if !strings.Contains(zhGot, "停滞") {
		t.Errorf("zh: expected '停滞' in friendly text; got %q", zhGot)
	}
	if strings.Contains(zhGot, "Ctrl+C") {
		t.Errorf("zh: stalled message must NOT mention Ctrl+C; got %q", zhGot)
	}
}

// TestFriendlyRunError_FirstByteTimeoutHasDistinctMessage pins the
// new typed-error branch: provider accepted the request but never
// emitted a body chunk. Operator remediation differs from mid-
// stream stall (provider deadlock / cold-start hang vs flaky
// upstream), so the message must be distinct.
func TestFriendlyRunError_FirstByteTimeoutHasDistinctMessage(t *testing.T) {
	fb := &llm.StreamFirstByteTimeoutError{
		IdleFor: 20 * time.Second,
		Cause:   context.Canceled,
	}
	wrapped := fmt.Errorf("LLM call failed: %w", fb)

	enGot := friendlyRunError("en", wrapped)
	if !strings.Contains(strings.ToLower(enGot), "never sent any response") {
		t.Errorf("en: expected 'never sent any response' phrase; got %q", enGot)
	}
	if !strings.Contains(enGot, "20s") && !strings.Contains(enGot, "20.0s") {
		t.Errorf("en: idle duration missing; got %q", enGot)
	}
	// Must NOT mention "stalled mid-stream" (different error class).
	if strings.Contains(enGot, "mid-stream") || strings.Contains(enGot, "mid-emit") {
		t.Errorf("en: first-byte must not borrow stall prose; got %q", enGot)
	}

	zhGot := friendlyRunError("zh", wrapped)
	if !strings.Contains(zhGot, "无任何响应") {
		t.Errorf("zh: expected '无任何响应'; got %q", zhGot)
	}
}

// TestFriendlyRunError_FirstByteMatchesBeforeStreamStalled guards
// the order of branches: a typed StreamFirstByteTimeoutError must
// take its own branch BEFORE StreamStalledError because the chain
// preserves both error types in its Unwrap path. Reverse order
// would always surface the more generic stall message.
func TestFriendlyRunError_FirstByteMatchesBeforeStreamStalled(t *testing.T) {
	fb := &llm.StreamFirstByteTimeoutError{
		IdleFor: 18 * time.Second,
		Cause:   context.Canceled,
	}
	got := friendlyRunError("en", fb)
	if !strings.Contains(strings.ToLower(got), "never sent any response") {
		t.Errorf("first-byte branch must take precedence over stall branch; got %q", got)
	}
	if strings.Contains(got, "stalled with no bytes") {
		t.Errorf("first-byte error must NOT fall through to stall message; got %q", got)
	}
}

// TestFriendlyRunError_StreamStalledMatchesBeforeGenericContextCanceled
// guards the order of branches in friendlyRunError: the typed
// StreamStalledError matcher MUST come BEFORE the generic
// "context canceled" substring matcher. If the order flips the
// stalled path silently regresses to the Ctrl+C message because
// errors.As(StreamStalledError) implies "context canceled" is in
// the chain.
func TestFriendlyRunError_StreamStalledMatchesBeforeGenericContextCanceled(t *testing.T) {
	stalled := &llm.StreamStalledError{
		IdleFor: 30 * time.Second,
		Cause:   context.Canceled,
	}
	got := friendlyRunError("en", stalled)
	// The stalled path produces a message containing "stalled" /
	// "no bytes". The generic context-canceled path would produce
	// "request interrupted (likely Ctrl+C ...)". They are
	// distinguishable.
	if !strings.Contains(got, "stalled") {
		t.Errorf("typed StreamStalledError must take the stalled branch; got %q", got)
	}
	if strings.Contains(got, "Ctrl+C") {
		t.Errorf("typed StreamStalledError must NOT fall through to Ctrl+C branch; got %q", got)
	}
}

// TestFriendlyRunError_PreservesUnknown locks the fallback contract:
// errors without a known translation pass through untouched so we
// don't accidentally swallow real diagnostic content.
func TestFriendlyRunError_PreservesUnknown(t *testing.T) {
	err := fmt.Errorf("some weird IO error: read /dev/null: no such device")
	got := friendlyRunError("en", err)
	if got != err.Error() {
		t.Errorf("unknown errors must pass through; got %q", got)
	}
}

// TestBannerCapabilityLine locks the startup banner contract: the
// user sees write_enabled state + yaml path immediately, in plain
// text, rather than discovering it via a /mode plan reject deep in
// a session.
func TestBannerCapabilityLine(t *testing.T) {
	cases := []struct {
		name        string
		lang        string
		writable    bool
		path        string
		mustContain []string
	}{
		{"on", "en", true, "/etc/codrax.yaml", []string{"plan", "apply", "verify", "write_enabled=true", "/etc/codrax.yaml"}},
		{"off", "en", false, "/etc/codrax.yaml", []string{"write_enabled=false", "disabled"}},
		{"off-no-yaml", "en", false, "", []string{"write_enabled=false"}},
		{"zh-off", "zh", false, "", []string{"write_enabled=false", "已禁用"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := bannerCapabilityLine(tc.lang, tc.writable, tc.path)
			for _, want := range tc.mustContain {
				if !strings.Contains(got, want) {
					t.Errorf("banner missing %q; got: %q", want, got)
				}
			}
		})
	}
}

// TestEmptyResponseHint_BothLangs locks the contract that the
// empty-response path prints something actionable rather than the
// cryptic "??" pre-fix rendering.
func TestEmptyResponseHint_BothLangs(t *testing.T) {
	en := emptyResponseHint("en")
	if !strings.Contains(strings.ToLower(en), "no content") || !strings.Contains(en, "log") {
		t.Errorf("en hint must explain absence + name log; got: %q", en)
	}
	zh := emptyResponseHint("zh")
	if !strings.Contains(zh, "没有产出") || !strings.Contains(zh, "log") {
		t.Errorf("zh hint must explain absence + name log; got: %q", zh)
	}
}

// TestChitchatRouteSummary_BothLangs locks that the chitchat label /
// segments — folded into the dock shutdown line as
// "◇ <label> · <seg> · <seg>" via Renderer.SetRouteSummary — carry
// the route's identifying words in both locales.
func TestChitchatRouteSummary_BothLangs(t *testing.T) {
	enLabel, enSegs := chitchatRouteSummary("en")
	if enLabel != "chat reply" {
		t.Errorf("en label: got %q, want %q", enLabel, "chat reply")
	}
	if len(enSegs) == 0 || !strings.Contains(strings.Join(enSegs, " · "), "no plan") {
		t.Errorf("en segments must include 'no plan'; got: %v", enSegs)
	}
	zhLabel, zhSegs := chitchatRouteSummary("zh")
	if zhLabel != "闲聊回复" {
		t.Errorf("zh label: got %q, want %q", zhLabel, "闲聊回复")
	}
	if len(zhSegs) == 0 || !strings.Contains(strings.Join(zhSegs, " · "), "未生成 plan") {
		t.Errorf("zh segments must include '未生成 plan'; got: %v", zhSegs)
	}
}

// TestWriteModeDisabled_NamesActualPath locks that the L2 gate's
// reject message points the user at the actual codrax.yaml path the
// CLI loaded (when known) rather than a generic "in codrax.yaml" that
// forces the user to hunt through default lookup paths.
func TestWriteModeDisabled_NamesActualPath(t *testing.T) {
	got := strings.Join(writeModeDisabled("en", "/mode plan", "/opt/codrax/codrax.yaml"), "\n")
	if !strings.Contains(got, "/opt/codrax/codrax.yaml") {
		t.Errorf("gate message must name the resolved yaml path; got:\n%s", got)
	}
	// No-yaml case names that directly so the user knows to CREATE one.
	got2 := strings.Join(writeModeDisabled("en", "/mode plan", ""), "\n")
	if !strings.Contains(got2, "No codrax.yaml") {
		t.Errorf("no-yaml case must say so; got:\n%s", got2)
	}
}

// TestPlanReadyNudge_NamesAllActions locks that every plan-mode
// dispatch gets a nudge listing every legal next action so a user
// fresh to write mode does not have to read the docs to find /approve.
func TestPlanReadyNudge_NamesAllActions(t *testing.T) {
	got := strings.Join(planReadyNudge("en", "plan-1", 3), "\n")
	for _, want := range []string{"plan-1", "/plan show", "/approve", "/reject", "/mode read"} {
		if !strings.Contains(got, want) {
			t.Errorf("planReadyNudge missing %q; got:\n%s", want, got)
		}
	}
}

// Locks the zh-as-default contract for every helper in messages.go:
// only an explicit "en" flips to English; everything else (empty,
// "zh", "fr", typos) stays zh.

func TestIsZh_DefaultsToZh(t *testing.T) {
	cases := []struct {
		lang string
		zh   bool
	}{
		{"", true},
		{"zh", true},
		{"ZH", true},
		{"fr", true},
		{"ja", true},
		{"off", true},
		{" zh ", true},
		{"en", false},
		{"EN", false},
		{"  en", false},
		{"english", true}, // strict — only "en" flips
	}
	for _, c := range cases {
		t.Run(c.lang, func(t *testing.T) {
			if got := isZh(c.lang); got != c.zh {
				t.Errorf("isZh(%q) = %v; want %v", c.lang, got, c.zh)
			}
		})
	}
}

func TestApproveTitlePrompt_BothLangs(t *testing.T) {
	zh := approveTitlePrompt("zh", "plan-1", 3, false)
	if !strings.Contains(zh, "批准") || !strings.Contains(zh, "plan-1") || !strings.Contains(zh, "3 处") {
		t.Errorf("zh prompt missing key fragments; got %q", zh)
	}
	en := approveTitlePrompt("en", "plan-1", 3, false)
	if !strings.Contains(en, "Approve") || !strings.Contains(en, "plan-1") {
		t.Errorf("en prompt missing key fragments; got %q", en)
	}
	if zh == en {
		t.Errorf("zh and en prompts should differ; both = %q", zh)
	}
	// --skip-verify variants must mention skip / 跳过 verify so the
	// operator's eye registers the deliberate bypass before they
	// click Yes. Pre-fix the title always said "apply + run verify"
	// regardless of the flag, masking the actual behaviour.
	zhSkip := approveTitlePrompt("zh", "plan-1", 3, true)
	if !strings.Contains(zhSkip, "跳过 verify") {
		t.Errorf("zh skip-verify prompt should mention 跳过 verify; got %q", zhSkip)
	}
	if strings.Contains(zhSkip, "+ 跑 verify") {
		t.Errorf("zh skip-verify prompt must NOT promise run-verify; got %q", zhSkip)
	}
	enSkip := approveTitlePrompt("en", "plan-1", 3, true)
	if !strings.Contains(enSkip, "skip verify") {
		t.Errorf("en skip-verify prompt should mention skip verify; got %q", enSkip)
	}
	if strings.Contains(enSkip, "run verify") {
		t.Errorf("en skip-verify prompt must NOT promise run-verify; got %q", enSkip)
	}
}

func TestApproveCancelled_BothLangs(t *testing.T) {
	if approveCancelled("zh") == approveCancelled("en") {
		t.Error("zh and en should differ")
	}
	if !strings.Contains(approveCancelled("zh"), "取消") {
		t.Errorf("zh missing 取消; got %q", approveCancelled("zh"))
	}
}

func TestRejectConfirmed_BothLangs(t *testing.T) {
	zhWith := rejectConfirmedWithReason("zh", "plan-X", "broken patch")
	enWith := rejectConfirmedWithReason("en", "plan-X", "broken patch")
	if !strings.Contains(zhWith, "已拒绝") || !strings.Contains(zhWith, "plan-X") || !strings.Contains(zhWith, "broken patch") {
		t.Errorf("zh-with-reason malformed; got %q", zhWith)
	}
	if !strings.Contains(strings.ToLower(enWith), "rejected") || !strings.Contains(enWith, "plan-X") {
		t.Errorf("en-with-reason malformed; got %q", enWith)
	}
	zhNo := rejectConfirmedNoReason("zh", "plan-Y")
	enNo := rejectConfirmedNoReason("en", "plan-Y")
	if zhNo == zhWith {
		t.Error("with-reason and no-reason zh should differ")
	}
	if enNo == enWith {
		t.Error("with-reason and no-reason en should differ")
	}
}

func TestNoPendingPlan_BothLangs(t *testing.T) {
	zh := noPendingPlan("zh")
	en := noPendingPlan("en")
	if zh == en {
		t.Error("zh and en should differ")
	}
	if !strings.Contains(zh, "/mode plan") || !strings.Contains(en, "/mode plan") {
		t.Errorf("both should reference /mode plan recovery; zh=%q en=%q", zh, en)
	}
}

func TestModeSwitched_BothLangs(t *testing.T) {
	if !strings.Contains(modeSwitched("zh", "plan"), "已切换") {
		t.Error("zh missing 已切换")
	}
	if !strings.Contains(strings.ToLower(modeSwitched("en", "plan")), "switched") {
		t.Error("en missing 'switched'")
	}
}

func TestPromptStickyTag_StateCombinations(t *testing.T) {
	cases := []struct {
		name        string
		mode        string
		branch      string
		hasLog      bool
		hasTrace    bool
		hasPlan     bool
		memPressure bool
		want        string
	}{
		{"empty", "", "", false, false, false, false, ""},
		{"read mode no attachments", "read", "", false, false, false, false, ""},
		{"plan mode only", "plan", "", false, false, false, false, "[mode:plan]"},
		{"log only", "read", "", true, false, false, false, "[log]"},
		{"trace only", "read", "", false, true, false, false, "[trace]"},
		{"pending plan only", "read", "", false, false, true, false, "[plan]"},
		{"memory pressure only", "read", "", false, false, false, true, "[mem!]"},
		{"plan+log", "plan", "", true, false, false, false, "[mode:plan][log]"},
		{"all on", "apply", "", true, true, true, true, "[mode:apply][log][trace][plan][mem!]"},
		{"case-insensitive read", "READ", "", false, false, false, false, ""},
		{"git branch alone", "read", "main", false, false, false, false, "[git:main]"},
		{"git branch + plan mode", "plan", "feature-x", false, false, false, false, "[git:feature-x][mode:plan]"},
		{"git detached + everything", "apply", "detached@abc1234", true, true, true, true, "[git:detached@abc1234][mode:apply][log][trace][plan][mem!]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := promptStickyTag(c.mode, c.branch, c.hasLog, c.hasTrace, c.hasPlan, c.memPressure)
			if got != c.want {
				t.Errorf("promptStickyTag(%q,%q,%v,%v,%v,%v) = %q; want %q",
					c.mode, c.branch, c.hasLog, c.hasTrace, c.hasPlan, c.memPressure, got, c.want)
			}
		})
	}
}

func TestMemoryPressureHint_BothLangs(t *testing.T) {
	zh := memoryPressureHint("zh", 30, 60)
	en := memoryPressureHint("en", 30, 60)
	if !strings.Contains(zh, "30") || !strings.Contains(zh, "60") {
		t.Errorf("zh hint should embed concrete counts; got %q", zh)
	}
	if !strings.Contains(zh, "/compact") || !strings.Contains(zh, "/clear") {
		t.Errorf("zh hint must surface both recovery commands; got %q", zh)
	}
	if !strings.Contains(en, "/compact") || !strings.Contains(en, "/clear") {
		t.Errorf("en hint must surface both recovery commands; got %q", en)
	}
}

// /help drift guard: every command in slashCommands must appear
// in helpLines() output. Catches the historical bug where /htrace
// and /atrace were missing from the hardcoded /help list.
func TestHelpLines_CoversEveryCommand(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			lines := helpLines(lang)
			joined := strings.Join(lines, "\n")
			for _, c := range slashCommands {
				if !strings.Contains(joined, c.Name) {
					t.Errorf("/help (%s) missing command %q; full output:\n%s", lang, c.Name, joined)
				}
			}
		})
	}
}

// TestHelpLines_WriteModeGroupingHeader pins commit 41 UX#3:
// /help renders a grouping header before the first write-
// mode command so first-time users see the workflow as a
// coherent block instead of scattered through read commands.
func TestHelpLines_WriteModeGroupingHeader(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			lines := helpLines(lang)
			joined := strings.Join(lines, "\n")
			wantSubstr := "Write-mode commands"
			if isZh(lang) {
				wantSubstr = "写模式命令"
			}
			if !strings.Contains(joined, wantSubstr) {
				t.Errorf("/help (%s) missing write-mode group header %q; got:\n%s",
					lang, wantSubstr, joined)
			}
			// Header must precede /mode (first write command).
			modeIdx := strings.Index(joined, "/mode")
			headerIdx := strings.Index(joined, wantSubstr)
			if headerIdx < 0 || modeIdx < 0 || headerIdx >= modeIdx {
				t.Errorf("write-mode header should appear BEFORE /mode; got header=%d mode=%d", headerIdx, modeIdx)
			}
		})
	}
}

// TestClampToTermWidth pins commit 42 P1: long banner lines
// get truncated with a centered ellipsis so terminals don't
// wrap them onto a second visual line.
func TestClampToTermWidth(t *testing.T) {
	for _, c := range []struct {
		in       string
		maxWidth int
		want     string
	}{
		{"short", 20, "short"},
		{"exactly twenty chars", 20, "exactly twenty chars"},
		// Long input: 19 chars budget = 11 head + 8 tail with "…" between.
		{"this is a very long banner line with many words", 20, "this is a v…ny words"},
		{"trailing spaces here    ", 50, "trailing spaces here"},
		{"a", 0, "a"}, // zero width falls back to default 120; short input stays.
	} {
		got := clampToTermWidth(c.in, c.maxWidth)
		if got != c.want {
			t.Errorf("clampToTermWidth(%q, %d) = %q; want %q", c.in, c.maxWidth, got, c.want)
		}
	}
}

// TestUnsettledBanner_WorktreeMissingTag pins commit 42 P1:
// when the underlying worktree was deleted out-of-band, the
// banner appends an "(orphaned worktree)" tag so the operator
// doesn't fall into a runtime error from /merge / /verify.
func TestUnsettledBanner_WorktreeMissingTag(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		got := unsettledBanner(lang, "plan-x", "applied", true)
		if !strings.Contains(got, "worktree") {
			t.Errorf("%s: expected 'worktree' tag on missing worktree; got %q", lang, got)
		}
		// Without missing flag, no tag.
		got = unsettledBanner(lang, "plan-x", "applied", false)
		if strings.Contains(got, "worktree gone") || strings.Contains(got, "worktree 已不在") {
			t.Errorf("%s: clean worktree should NOT carry orphan tag; got %q", lang, got)
		}
	}
}

// TestPlanShowFooter_StatusAware pins commit 41 UX#5: the
// footer surfaces status-specific recovery commands rather
// than always showing the same generic line.
func TestPlanShowFooter_StatusAware(t *testing.T) {
	for _, c := range []struct {
		status   string
		mustHave string
	}{
		{"pending_approval", "/approve to apply"},
		{"verify_failed", "--retry"},
		{"partially_applied", "--retry"},
		{"unverified", "--retry"},
		{"applied", "/merge to merge"},
	} {
		lines := planShowFooter("en", c.status)
		joined := strings.Join(lines, " ")
		if !strings.Contains(joined, c.mustHave) {
			t.Errorf("status=%s footer missing %q; got %q", c.status, c.mustHave, joined)
		}
	}
}

// TestPlanReadyMultiPhaseNudge_NamesPhaseCount pins commit 41
// UX#1: the multi-phase nudge tells the operator how many
// phases queued + names /phase show as the inspection tool.
func TestPlanReadyMultiPhaseNudge_NamesPhaseCount(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		lines := planReadyMultiPhaseNudge(lang, 3)
		if len(lines) == 0 {
			t.Errorf("%s: expected at least one line", lang)
			continue
		}
		joined := strings.Join(lines, " ")
		if !strings.Contains(joined, "3") {
			t.Errorf("%s: nudge should name phase count; got %q", lang, joined)
		}
		if !strings.Contains(joined, "/phase show") {
			t.Errorf("%s: nudge should point at /phase show; got %q", lang, joined)
		}
	}
}

// /help renders bilingual: zh by default, en only with explicit lang.
func TestHelpLines_BothLangs(t *testing.T) {
	zhLines := helpLines("zh")
	enLines := helpLines("en")
	zhJoined := strings.Join(zhLines, "\n")
	enJoined := strings.Join(enLines, "\n")
	if !strings.Contains(zhJoined, "可用命令") {
		t.Errorf("zh header missing 可用命令; got %q", zhJoined)
	}
	if !strings.Contains(enJoined, "available commands") {
		t.Errorf("en header missing 'available commands'; got %q", enJoined)
	}
	if zhJoined == enJoined {
		t.Error("zh and en help output should differ")
	}
}

// slashCommand.Help honors lang; both variants must be non-empty
// so a missing translation never silently falls back.
func TestSlashCommand_HelpBothVariantsNonEmpty(t *testing.T) {
	for _, c := range slashCommands {
		if c.HelpEn == "" {
			t.Errorf("command %q: HelpEn is empty", c.Name)
		}
		if c.HelpZh == "" {
			t.Errorf("command %q: HelpZh is empty (would fall back to HelpEn at render time)", c.Name)
		}
	}
}

func TestVerifyDispatching_BothLangs(t *testing.T) {
	zh := verifyDispatching("zh", "plan-V")
	en := verifyDispatching("en", "plan-V")
	if !strings.Contains(zh, "verify") || !strings.Contains(zh, "plan-V") {
		t.Errorf("zh malformed; got %q", zh)
	}
	if !strings.Contains(en, "verify") || !strings.Contains(en, "plan-V") {
		t.Errorf("en malformed; got %q", en)
	}
}

// TestApproveDispatchRequest_NotREPLControlInput pins the contract
// that the synthetic request handed to runner.Run for /approve does
// not look like a REPL control command. Real bug: a literal
// "/approve <id>" was rejected by emit_analysis on every classifier
// iteration, burning the analyzer's iter cap until the user ctrl-C'd.
func TestApproveDispatchRequest_NotREPLControlInput(t *testing.T) {
	cases := []*types.ChangePlan{
		{ID: "plan-1", Summary: "rename foo to bar in pkg/x"},
		{ID: "plan-2", Summary: ""}, // fall back to generic phrasing
	}
	for _, p := range cases {
		req := approveDispatchRequest(p)
		if req == "" {
			t.Errorf("empty request for plan %s", p.ID)
		}
		if types.IsREPLControlInput(req) {
			t.Errorf("approve request %q must not be REPL-control-input shaped", req)
		}
		if !strings.Contains(req, p.ID) {
			t.Errorf("approve request %q missing plan id %s", req, p.ID)
		}
	}
}

// TestMemoryClearConfirm_BilingualWithHotkey pins the bilingual
// + hotkey-visible contract for the /clear confirmation prompt.
// Both labels MUST start with the hotkey character (Y / N) so
// huh.Confirm's default "first-alphanumeric-as-binding" semantic
// produces y/n keyboard shortcuts AND the operator sees the
// shortcut without reading docs.
func TestMemoryClearConfirm_BilingualWithHotkey(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		aff := memoryClearConfirmAffirmative(lang)
		neg := memoryClearConfirmNegative(lang)
		if !strings.HasPrefix(aff, "Y") {
			t.Errorf("%s affirmative MUST start with 'Y' for huh hotkey binding; got %q", lang, aff)
		}
		if !strings.HasPrefix(neg, "N") {
			t.Errorf("%s negative MUST start with 'N' for huh hotkey binding; got %q", lang, neg)
		}
	}
	// zh title respects locale (no English wipe verb).
	zhTitle := memoryClearConfirmTitle("zh", 0)
	if !strings.Contains(zhTitle, "清空") {
		t.Errorf("zh title MUST contain '清空'; got %q", zhTitle)
	}
	if strings.Contains(zhTitle, "wipes") {
		t.Errorf("zh title MUST NOT mix English 'wipes'; got %q", zhTitle)
	}
	// Peer-count addendum follows locale too.
	zhPeer := memoryClearConfirmTitle("zh", 2)
	if !strings.Contains(zhPeer, "2 个") {
		t.Errorf("zh peer addendum MUST contain '2 个'; got %q", zhPeer)
	}
	if strings.Contains(zhPeer, "instances") {
		t.Errorf("zh peer addendum MUST NOT contain English 'instances'; got %q", zhPeer)
	}
	// Line hint also bilingual.
	zhHint := memoryClearConfirmLineHint("zh")
	if !strings.Contains(zhHint, "y") {
		t.Errorf("zh line hint MUST mention y; got %q", zhHint)
	}
	if strings.Contains(zhHint, "Type") {
		t.Errorf("zh line hint MUST NOT mix English 'Type'; got %q", zhHint)
	}
	enHint := memoryClearConfirmLineHint("en")
	if !strings.Contains(enHint, "y") {
		t.Errorf("en line hint MUST mention y; got %q", enHint)
	}
}

// TestIdleConfirmExitMsg pins the bilingual content of the idle-
// prompt confirmation message. The Chinese variant must explicitly
// name "Ctrl+C" + the 2-second window so operators understand the
// double-tap semantic without consulting docs; the English variant
// mirrors the same. Both must contain "REPL" so the message is
// distinguishable from in-Run cancel messages.
func TestIdleConfirmExitMsg(t *testing.T) {
	zh := idleConfirmExitMsg("zh")
	if !strings.Contains(zh, "Ctrl+C") {
		t.Errorf("zh msg must name Ctrl+C; got %q", zh)
	}
	if !strings.Contains(zh, "2") {
		t.Errorf("zh msg must mention the 2-second window; got %q", zh)
	}
	if !strings.Contains(zh, "REPL") {
		t.Errorf("zh msg must distinguish from in-Run cancel; got %q", zh)
	}
	en := idleConfirmExitMsg("en")
	if !strings.Contains(en, "Ctrl+C") {
		t.Errorf("en msg must name Ctrl+C; got %q", en)
	}
	if !strings.Contains(en, "2s") {
		t.Errorf("en msg must mention the 2-second window; got %q", en)
	}
	if !strings.Contains(en, "REPL") {
		t.Errorf("en msg must distinguish from in-Run cancel; got %q", en)
	}
	// Distinct from cancelInProgressMsg so the dock area can render
	// either without ambiguity.
	if zh == cancelInProgressMsg("zh") {
		t.Errorf("idleConfirmExit must NOT collide with in-Run cancel msg (zh)")
	}
	if en == cancelInProgressMsg("en") {
		t.Errorf("idleConfirmExit must NOT collide with in-Run cancel msg (en)")
	}
}

func TestVerifyDispatchRequest_NotREPLControlInput(t *testing.T) {
	cases := []*types.ChangePlan{
		{ID: "plan-3", Summary: "add tests for X"},
		{ID: "plan-4", Summary: ""},
	}
	for _, p := range cases {
		req := verifyDispatchRequest(p)
		if req == "" {
			t.Errorf("empty request for plan %s", p.ID)
		}
		if types.IsREPLControlInput(req) {
			t.Errorf("verify request %q must not be REPL-control-input shaped", req)
		}
		if !strings.Contains(req, p.ID) {
			t.Errorf("verify request %q missing plan id %s", req, p.ID)
		}
	}
}
