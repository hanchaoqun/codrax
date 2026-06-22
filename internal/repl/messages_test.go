package repl

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/operation"
	"github.com/hanchaoqun/codrax/internal/render"
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

func TestFriendlyRunError_NoVisibleOutputTimeoutHasDistinctMessage(t *testing.T) {
	nv := &llm.StreamNoVisibleOutputTimeoutError{
		IdleFor: 4 * time.Minute,
		Cause:   context.Canceled,
	}
	wrapped := fmt.Errorf("LLM call failed: %w", nv)

	enGot := friendlyRunError("en", wrapped)
	if !strings.Contains(strings.ToLower(enGot), "no user-visible answer") {
		t.Errorf("en: expected no-visible-output guidance; got %q", enGot)
	}
	if strings.Contains(enGot, "Ctrl+C") || strings.Contains(enGot, "stalled with no bytes") {
		t.Errorf("en: no-visible-output message must not look like cancel/stall; got %q", enGot)
	}

	zhGot := friendlyRunError("zh", wrapped)
	if !strings.Contains(zhGot, "流式连接仍在活动") || !strings.Contains(zhGot, "没有产生") {
		t.Errorf("zh: expected active-stream/no-visible-output guidance; got %q", zhGot)
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

func TestCommandOperationResultMarkdownClampsPanelPreview(t *testing.T) {
	preview := strings.Repeat("x", commandOperationDisplayPreviewRunes+512)
	plan := operation.CommandOperationPlan{
		ID: "op-preview",
	}
	result := operation.CommandOperationResult{
		PlanID: "op-preview",
		Status: operation.StatusExecuted,
		StepResults: []operation.CommandStepResult{{
			StepID:        "s1",
			Status:        operation.StatusExecuted,
			OutputPreview: preview,
			PayloadRef:    "/tmp/codrax-operation/op-preview-s1.txt",
		}},
	}
	got := commandOperationResultMarkdown("zh", plan, result)
	if strings.Count(got, "x") > commandOperationDisplayPreviewRunes+16 {
		t.Fatalf("panel preview was not clamped enough: x count=%d", strings.Count(got, "x"))
	}
	for _, want := range []string{
		"面板预览已截断",
		"完整输出",
		"/tmp/codrax-operation/op-preview-s1.txt",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("result markdown missing %q:\n%s", want, got)
		}
	}
}

func TestCommandOperationResultMarkdownClampsPanelPreviewLines(t *testing.T) {
	lines := make([]string, 0, commandOperationDisplayPreviewLines+5)
	for i := 1; i <= commandOperationDisplayPreviewLines+5; i++ {
		lines = append(lines, fmt.Sprintf("line-%02d", i))
	}
	plan := operation.CommandOperationPlan{ID: "op-preview-lines"}
	result := operation.CommandOperationResult{
		PlanID: "op-preview-lines",
		Status: operation.StatusExecuted,
		StepResults: []operation.CommandStepResult{{
			StepID:        "s1",
			Status:        operation.StatusExecuted,
			OutputPreview: strings.Join(lines, "\n"),
			PayloadRef:    "/tmp/codrax-operation/op-preview-lines-s1.txt",
		}},
	}
	got := commandOperationResultMarkdown("zh", plan, result)
	if strings.Contains(got, "line-21") || strings.Contains(got, "line-25") {
		t.Fatalf("panel preview should clamp after %d lines:\n%s", commandOperationDisplayPreviewLines, got)
	}
	for _, want := range []string{
		"line-20",
		"面板预览已截断",
		"完整输出",
		"/tmp/codrax-operation/op-preview-lines-s1.txt",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("result markdown missing %q:\n%s", want, got)
		}
	}
}

func TestCommandOperationAutoExecuteMarkdownShowsCommands(t *testing.T) {
	plan := operation.CommandOperationPlan{
		ID:           "op-auto",
		ApprovalMode: operation.ApprovalAutoLowRisk,
		RiskLevel:    "low",
		WorkDir:      "/repo",
		Steps: []operation.CommandStep{{
			ID:      "s1",
			Program: "uname",
			Args:    []string{"-a"},
		}},
	}
	got := commandOperationAutoExecuteMarkdown("zh", plan)
	for _, want := range []string{
		"将自动执行",
		"审批：`auto_low_risk`",
		"`$ uname -a`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("auto execute markdown missing %q:\n%s", want, got)
		}
	}
}

func TestCommandOperationAutoValidateMarkdownShowsCommands(t *testing.T) {
	plan := operation.CommandOperationPlan{
		ID:           "op-validate",
		ApprovalMode: operation.ApprovalAutoLowRisk,
		RiskLevel:    "medium",
		WorkDir:      "/repo",
		Steps: []operation.CommandStep{{
			ID:    "s1",
			Shell: "grep model",
		}},
	}
	got := commandOperationAutoValidateMarkdown("zh", plan)
	for _, want := range []string{
		"将先自动校验",
		"审批：`auto_low_risk`",
		"待校验命令",
		"`$ grep model`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("auto validate markdown missing %q:\n%s", want, got)
		}
	}
}

func TestCommandOperationProgressShowsStepTitle(t *testing.T) {
	step := operation.CommandStep{
		ID:        "s1",
		Title:     "查询模型列表",
		TimeoutMS: 180_000,
	}
	if got := commandOperationProgressMsg("zh", step); got != "操作中：查询模型列表" {
		t.Fatalf("zh progress should show step title only, got %q", got)
	}
	if got := commandOperationProgressMsg("en", step); got != "Operating: 查询模型列表" {
		t.Fatalf("en progress should show step title only, got %q", got)
	}
}

func TestCommandOperationResultMarkdownShowsExecutedCommands(t *testing.T) {
	plan := operation.CommandOperationPlan{
		ID: "op-result",
		Steps: []operation.CommandStep{{
			ID:      "s1",
			Program: "sysctl",
			Args:    []string{"-n", "hw.memsize"},
		}},
	}
	result := operation.CommandOperationResult{
		PlanID: "op-result",
		Status: operation.StatusExecuted,
		StepResults: []operation.CommandStepResult{{
			StepID:        "s1",
			Status:        operation.StatusExecuted,
			OutputPreview: "68719476736",
		}},
	}
	got := commandOperationResultMarkdown("zh", plan, result)
	for _, want := range []string{
		"命令：`$ sysctl -n hw.memsize`",
		"68719476736",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("result markdown missing %q:\n%s", want, got)
		}
	}
}

func TestSplitVisibleThinkBlocksKeepsOperationAnswerPanelClean(t *testing.T) {
	thoughts, answer := splitVisibleThinkBlocks("<think>我先根据输出判断 VPN 候选</think>\n\n## 结果\nShadowrocket 正在运行。")
	if len(thoughts) != 1 || !strings.Contains(thoughts[0], "VPN 候选") {
		t.Fatalf("thoughts=%+v, want extracted operation thought", thoughts)
	}
	if strings.Contains(answer, "<think>") || strings.Contains(answer, "</think>") {
		t.Fatalf("answer still contains think block: %q", answer)
	}
	if !strings.Contains(answer, "Shadowrocket 正在运行") {
		t.Fatalf("answer lost final report: %q", answer)
	}
}

func TestOperationFinalReportDoesNotAppendExecutionDetails(t *testing.T) {
	got := operationFinalReportWithDetails("zh", "最终报告", "命令输出详情")
	if got != "最终报告" {
		t.Fatalf("final report should stay pure, got %q", got)
	}
	fallback := operationFinalReportWithDetails("zh", "", "命令输出详情")
	if fallback != "命令输出详情" {
		t.Fatalf("empty answer should fall back to details, got %q", fallback)
	}
}

func TestCommandOperationFinalMessageFallbackDoesNotReturnRoundDetails(t *testing.T) {
	r := &REPL{language: "zh"}
	records := []commandOperationResultRecord{{
		Plan: operation.CommandOperationPlan{
			ID: "op-1",
			Steps: []operation.CommandStep{{
				ID:      "s1",
				Program: "sysctl",
				Args:    []string{"-n", "hw.memsize"},
			}},
		},
		Result: operation.CommandOperationResult{
			PlanID: "op-1",
			Status: operation.StatusExecuted,
			StepResults: []operation.CommandStepResult{{
				StepID:        "s1",
				Status:        operation.StatusExecuted,
				OutputPreview: "68719476736",
			}},
		},
	}}

	got, thoughts := r.commandOperationFinalMessage(context.Background(), "当前内存多大", records)
	if len(thoughts) != 0 {
		t.Fatalf("fallback should not produce thoughts: %+v", thoughts)
	}
	for _, banned := range []string{"操作计划", "op-1", "sysctl", "68719476736"} {
		if strings.Contains(got, banned) {
			t.Fatalf("fallback final answer leaked round detail %q: %q", banned, got)
		}
	}
	if !strings.Contains(got, "操作已完成") || !strings.Contains(got, "上方过程面板") {
		t.Fatalf("fallback final answer should be a clean status report, got %q", got)
	}
}

func TestProviderOperationFinalMessageFallbackDoesNotReturnRoundDetails(t *testing.T) {
	r := &REPL{language: "zh"}
	records := []providerOperationResultRecord{{
		Plan: operation.Plan{
			Provider: "demo_skill",
		},
		Result: providerOperationResult{
			Status:     operation.StatusExecuted,
			Provider:   "demo_skill",
			Summary:    "provider generated artifact",
			PayloadRef: "/tmp/operation-output.txt",
		},
	}}

	got, thoughts := r.providerOperationFinalMessage(context.Background(), "生成文档", records)
	if len(thoughts) != 0 {
		t.Fatalf("fallback should not produce thoughts: %+v", thoughts)
	}
	for _, banned := range []string{"操作 provider", "demo_skill", "provider generated artifact", "/tmp/operation-output.txt"} {
		if strings.Contains(got, banned) {
			t.Fatalf("provider fallback final answer leaked round detail %q: %q", banned, got)
		}
	}
	if !strings.Contains(got, "操作已完成") || !strings.Contains(got, "上方过程面板") {
		t.Fatalf("provider fallback final answer should be a clean status report, got %q", got)
	}
}

func TestOperationVisibleThinkBlocksRenderOutsideAnswerPanel(t *testing.T) {
	var out bytes.Buffer
	r := &REPL{out: &out, in: strings.NewReader(""), language: "zh"}
	r.emitOperationVisibleThoughts([]string{"The user is asking about system info\nI should verify before answering."})
	got := out.String()
	for _, want := range []string{
		"模型思考（不进入答案）",
		"The user is asking about system info",
		"I should verify before answering.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("operation visible thinking missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "<think>") || strings.Contains(got, "</think>") {
		t.Fatalf("rendered operation thinking should not include raw tags: %q", got)
	}
}

func TestRenderBorderedCompactDoesNotAddExtraBlankLine(t *testing.T) {
	var out bytes.Buffer
	r := &REPL{out: &out}
	r.renderBorderedCompact("操作计划 `op-1` 将自动执行。")
	got := out.String()
	if strings.Contains(got, "│\n\n\n") {
		t.Fatalf("compact border rendered excessive blank lines: %q", got)
	}
	if strings.HasSuffix(got, "\n\n") {
		t.Fatalf("compact border should end with one newline, got %q", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("compact border should still end with a newline, got %q", got)
	}
}

func TestRenderBorderedCompactHonorsColorMode(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	var colored bytes.Buffer
	r := &REPL{out: &colored, colorMode: render.ColorAlways}
	r.renderBorderedCompact("最终答案")
	got := colored.String()
	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("ColorAlways should color the final-answer border: %q", got)
	}
	if !strings.Contains(stripANSIOnly(got), "│ 最终答案") {
		t.Fatalf("colored border lost content: %q", got)
	}

	var plain bytes.Buffer
	r = &REPL{out: &plain, colorMode: render.ColorNever}
	r.renderBorderedCompact("最终答案")
	if strings.Contains(plain.String(), "\x1b[") {
		t.Fatalf("ColorNever must not emit ANSI in bordered panels: %q", plain.String())
	}
}

// TestBannerCapabilityLine locks the startup banner contract: the
// user sees write_enabled state + yaml path immediately, in plain
// text, rather than discovering it via a /mode write reject deep in
// a session.
func TestBannerCapabilityLine(t *testing.T) {
	cases := []struct {
		name        string
		lang        string
		writable    bool
		path        string
		mustContain []string
	}{
		{"on", "en", true, "/etc/codrax.yaml", []string{"auto", "code", "operation", "data", "write", "/etc/codrax.yaml"}},
		{"off", "en", false, "/etc/codrax.yaml", []string{"auto", "code", "operation", "data", "write disabled by write_enabled: false"}},
		{"off-no-yaml", "en", false, "", []string{"write disabled by write_enabled: false"}},
		{"zh-off", "zh", false, "", []string{"write 已被 write_enabled: false 禁用"}},
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
	got := strings.Join(writeModeDisabled("en", "/mode write", "/opt/codrax/codrax.yaml"), "\n")
	if !strings.Contains(got, "/opt/codrax/codrax.yaml") {
		t.Errorf("gate message must name the resolved yaml path; got:\n%s", got)
	}
	// No-yaml case names that directly so the user knows to CREATE one.
	got2 := strings.Join(writeModeDisabled("en", "/mode write", ""), "\n")
	if !strings.Contains(got2, "No codrax.yaml") {
		t.Errorf("no-yaml case must say so; got:\n%s", got2)
	}
}

// TestPlanReadyNudge_RendersStatusCard locks that explicit plan-mode
// dispatch gets state-first guidance; slash commands are advanced
// recovery/audit entry points, not the primary instruction.
func TestPlanReadyNudge_RendersStatusCard(t *testing.T) {
	got := strings.Join(planReadyNudge("en", "plan-1", 3), "\n")
	for _, want := range []string{"plan-1", "Status:", "Next:", "Advanced:", "/plan show", "/approve", "/reject"} {
		if !strings.Contains(got, want) {
			t.Errorf("planReadyNudge missing %q; got:\n%s", want, got)
		}
	}
}

func TestMergeSkipVerifyMessagesNameExplicitAction(t *testing.T) {
	msg := mergeUnverifiedNeedsSkipVerifyMsg("en", "plan-1")
	for _, want := range []string{"plan-1", "/verify plan-1", "/merge --skip-verify"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("merge unverified hint missing %q; got:\n%s", want, msg)
		}
	}
	warning := strings.Join(mergeSkipVerifyWarning("en", "plan-1"), "\n")
	for _, want := range []string{"without a local verification pass", "/plan show", "CI"} {
		if !strings.Contains(warning, want) {
			t.Fatalf("merge skip-verify warning missing %q; got:\n%s", want, warning)
		}
	}
	footer := strings.Join(planShowFooter("en", "unverified"), "\n")
	if !strings.Contains(footer, "/merge --skip-verify") || strings.Contains(footer, "/merge --include-failed") {
		t.Fatalf("unverified footer should prefer explicit skip-verify action; got:\n%s", footer)
	}
}

func TestWriteRecoveryHintsPreferAutoPilotPath(t *testing.T) {
	for _, c := range []struct {
		name string
		text string
	}{
		{"mergeNoApplyYet_en", strings.Join(mergeNoApplyYet("en"), "\n")},
		{"mergeNoApplyYet_zh", strings.Join(mergeNoApplyYet("zh"), "\n")},
		{"applyDone_en", strings.Join(applyDoneNudge("en"), "\n")},
		{"applyDone_zh", strings.Join(applyDoneNudge("zh"), "\n")},
		{"afterMerge_en", autoModeReadAfterMergeNudge("en")},
		{"afterMerge_zh", autoModeReadAfterMergeNudge("zh")},
		{"approveRefused_en", approveRefusedStatusMsg("en", "plan-1", "applied", "pending_approval", "verify_failed")},
		{"approveRefused_zh", approveRefusedStatusMsg("zh", "plan-1", "applied", "pending_approval", "verify_failed")},
	} {
		if !strings.Contains(c.text, "Auto Pilot") && !strings.Contains(c.text, "/write") {
			t.Errorf("%s should point at Auto Pilot or /write path; got:\n%s", c.name, c.text)
		}
		if strings.Contains(c.text, "/mode write") {
			t.Errorf("%s should not make /mode write the primary recovery path; got:\n%s", c.name, c.text)
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

	withContext := approveTitlePromptWithContext("en", writeApprovalPromptContext{
		PlanID:      "plan-ctx",
		ChangeCount: 2,
		RunID:       "wf-ctx",
		BatchID:     "batch-ctx",
		Fingerprint: "fp-ctx",
	})
	for _, want := range []string{"Approve plan plan-ctx", "Workflow:", "run=`wf-ctx`", "batch=`batch-ctx`", "fingerprint=`fp-ctx`"} {
		if !strings.Contains(withContext, want) {
			t.Errorf("context prompt missing %q; got %q", want, withContext)
		}
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
	if !strings.Contains(zh, "/write <目标>") || !strings.Contains(zh, "Auto Pilot") {
		t.Errorf("zh should point at low-command Auto Pilot recovery; got %q", zh)
	}
	if !strings.Contains(en, "/write <goal>") || !strings.Contains(en, "Auto Pilot") {
		t.Errorf("en should point at low-command Auto Pilot recovery; got %q", en)
	}
}

func TestModeSwitched_BothLangs(t *testing.T) {
	if !strings.Contains(modeSwitched("zh", "write"), "已切换") {
		t.Error("zh missing 已切换")
	}
	if !strings.Contains(strings.ToLower(modeSwitched("en", "write")), "switched") {
		t.Error("en missing 'switched'")
	}
}

func TestCurrentUserModeMsg_BothLangs(t *testing.T) {
	zh := currentUserModeMsg("zh", "data")
	en := currentUserModeMsg("en", "data")
	if !strings.Contains(zh, "当前任务模式") || !strings.Contains(zh, "/mode") {
		t.Fatalf("zh current mode message malformed: %q", zh)
	}
	if !strings.Contains(en, "Current task mode") || !strings.Contains(en, "/mode") {
		t.Fatalf("en current mode message malformed: %q", en)
	}
}

func TestHelpLines_SurfaceHtraceConvertSubcommand(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			joined := strings.Join(helpLinesAll(lang), "\n")
			for _, want := range []string{"/htrace", "/htrace convert <binary> [out.systrace]"} {
				if !strings.Contains(joined, want) {
					t.Fatalf("/help (%s) missing %q:\n%s", lang, want, joined)
				}
			}
		})
	}
}

func TestHelpLines_DefaultConciseFullDiscoverable(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			concise := strings.Join(helpLines(lang), "\n")
			full := strings.Join(helpLinesAll(lang), "\n")

			if !strings.Contains(concise, "/help all") {
				t.Errorf("%s: concise help should point to /help all; got:\n%s", lang, concise)
			}
			if !strings.Contains(concise, "Auto Pilot") {
				t.Errorf("%s: concise help should describe the write Auto Pilot path; got:\n%s", lang, concise)
			}
			for _, hidden := range []string{"/htrace convert <binary> [out.systrace]", "/plan clear --all", "/merge --include-failed"} {
				if strings.Contains(concise, hidden) {
					t.Errorf("%s: concise help should hide advanced subcommand %q; got:\n%s", lang, hidden, concise)
				}
				if !strings.Contains(full, hidden) {
					t.Errorf("%s: full help should keep advanced subcommand %q; got:\n%s", lang, hidden, full)
				}
			}
		})
	}
}

func TestPromptStickyTag_StateCombinations(t *testing.T) {
	cases := []struct {
		name         string
		taskMode     string
		pipelineMode string
		branch       string
		hasLog       bool
		hasTrace     bool
		hasPlan      bool
		memPressure  bool
		focus        []string
		want         string
	}{
		{"empty", "", "", "", false, false, false, false, nil, ""},
		{"auto read no attachments", "auto", "read", "", false, false, false, false, nil, ""},
		{"code task", "code", "read", "", false, false, false, false, nil, "[task:code]"},
		{"operation task", "operation", "read", "", false, false, false, false, nil, "[task:op]"},
		{"data task", "data", "read", "", false, false, false, false, nil, "[task:data]"},
		{"write task hides internal plan phase", "write", "plan", "", false, false, false, false, nil, "[task:write]"},
		{"write task hides internal apply phase", "write", "apply", "", false, false, false, false, nil, "[task:write]"},
		{"legacy plan phase only", "auto", "plan", "", false, false, false, false, nil, "[phase:plan]"},
		{"log only", "auto", "read", "", true, false, false, false, nil, "[log]"},
		{"trace only", "auto", "read", "", false, true, false, false, nil, "[trace]"},
		{"pending plan only", "auto", "read", "", false, false, true, false, nil, "[plan]"},
		{"memory pressure only", "auto", "read", "", false, false, false, true, nil, "[mem!]"},
		{"write+log", "write", "plan", "", true, false, false, false, nil, "[task:write][log]"},
		{"all on", "data", "apply", "", true, true, true, true, nil, "[task:data][phase:apply][log][trace][plan][mem!]"},
		{"case-insensitive auto read", "AUTO", "READ", "", false, false, false, false, nil, ""},
		{"git branch alone", "auto", "read", "main", false, false, false, false, nil, "[git:main]"},
		{"git branch + write task", "write", "plan", "feature-x", false, false, false, false, nil, "[git:feature-x][task:write]"},
		{"git detached + everything", "write", "apply", "detached@abc1234", true, true, true, true, nil, "[git:detached@abc1234][task:write][log][trace][plan][mem!]"},
		// Phase 3 multi-repo focus tag (2026-05-08).
		{"single focus only", "auto", "read", "", false, false, false, false, []string{"repo-go"}, "[focus:repo-go]"},
		{"single focus + git + write", "write", "plan", "main", false, false, false, false, []string{"repo-go"}, "[git:main][focus:repo-go][task:write]"},
		{"two focus", "auto", "read", "", false, false, false, false, []string{"repo-go", "repo-py"}, "[focus:repo-go,repo-py]"},
		{"three focus collapses to count", "auto", "read", "", false, false, false, false, []string{"a", "b", "c"}, "[focus:3 pinned]"},
		{"five focus collapses to count", "auto", "read", "", false, false, false, false, []string{"a", "b", "c", "d", "e"}, "[focus:5 pinned]"},
		{"focus with everything", "operation", "verify", "main", true, true, true, true, []string{"x", "y"}, "[git:main][focus:x,y][task:op][phase:verify][log][trace][plan][mem!]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := promptStickyTag(c.taskMode, c.pipelineMode, c.branch, c.hasLog, c.hasTrace, c.hasPlan, c.memPressure, c.focus)
			if got != c.want {
				t.Errorf("promptStickyTag(%q,%q,%q,%v,%v,%v,%v,%v) = %q; want %q",
					c.taskMode, c.pipelineMode, c.branch, c.hasLog, c.hasTrace, c.hasPlan, c.memPressure, c.focus, got, c.want)
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

func TestHandleSlashHelpAllRendersFullTable(t *testing.T) {
	r, out := newScriptedREPL(t, nil)

	r.handleSlash("/help")
	concise := out.String()
	if strings.Contains(concise, "/htrace convert <binary> [out.systrace]") {
		t.Fatalf("/help should be concise by default; got:\n%s", concise)
	}

	for _, line := range []string{"/help all", "/help full", "/help --all"} {
		out.Reset()
		r.handleSlash(line)
		full := out.String()
		if !strings.Contains(full, "/htrace convert <binary> [out.systrace]") {
			t.Fatalf("%s should render the complete command table; got:\n%s", line, full)
		}
	}
}

// /help all drift guard: every command in slashCommands must appear
// in helpLinesAll() output. Catches the historical bug where /htrace
// and /atrace were missing from the hardcoded /help list.
func TestHelpLines_CoversEveryCommand(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			lines := helpLinesAll(lang)
			joined := strings.Join(lines, "\n")
			for _, c := range slashCommands {
				if !strings.Contains(joined, c.Name) {
					t.Errorf("/help (%s) missing command %q; full output:\n%s", lang, c.Name, joined)
				}
			}
		})
	}
}

// TestWorkflowHelpDemotesWriteCommands keeps /workflow as an
// audit/recovery surface. The primary write-mode path is natural
// language goal -> Auto Pilot; high-risk approval is the only normal
// write interruption that asks the user to type an approval command.
func TestWorkflowHelpDemotesWriteCommands(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			direct := workflowHelpMsg(lang)
			joinedHelp := strings.Join(helpLines(lang), "\n")

			for surface, got := range map[string]string{
				"direct": direct,
				"help":   joinedHelp,
			} {
				if !strings.Contains(got, "Auto Pilot") {
					t.Errorf("%s %s: workflow help should name Auto Pilot as the primary path; got %q", lang, surface, got)
				}
				if isZh(lang) {
					if !strings.Contains(got, "审计") && !strings.Contains(got, "恢复") {
						t.Errorf("%s %s: workflow help should demote commands to audit/recovery; got %q", lang, surface, got)
					}
					if strings.Contains(got, "写模式批次继续使用 /approve 或 /reject") ||
						strings.Contains(got, "恢复已保存的写模式运行") {
						t.Errorf("%s %s: workflow help must not present command chains as the day-to-day write path; got %q", lang, surface, got)
					}
					continue
				}
				if !strings.Contains(got, "audit") && !strings.Contains(got, "recovery") {
					t.Errorf("%s %s: workflow help should demote commands to audit/recovery; got %q", lang, surface, got)
				}
				if strings.Contains(got, "Continue write batches with /approve or /reject") ||
					strings.Contains(got, "resume saved write runs") {
					t.Errorf("%s %s: workflow help must not present command chains as the day-to-day write path; got %q", lang, surface, got)
				}
			}
		})
	}
}

// TestHelpLines_WriteModeGroupingHeader pins commit 41 UX#3:
// /help all renders a grouping header before the first write-
// mode command so first-time users see the workflow as a
// coherent block instead of scattered through read commands.
func TestHelpLines_WriteModeGroupingHeader(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			lines := helpLinesAll(lang)
			joined := strings.Join(lines, "\n")
			wantSubstr := "Write-mode commands"
			if isZh(lang) {
				wantSubstr = "写模式命令"
			}
			if !strings.Contains(joined, wantSubstr) {
				t.Errorf("/help (%s) missing write-mode group header %q; got:\n%s",
					lang, wantSubstr, joined)
			}
			// Header must precede /write (first write-only command).
			modeIdx := strings.Index(joined, "/write")
			headerIdx := strings.Index(joined, wantSubstr)
			if headerIdx < 0 || modeIdx < 0 || headerIdx >= modeIdx {
				t.Errorf("write-mode header should appear BEFORE /mode; got header=%d mode=%d", headerIdx, modeIdx)
			}
		})
	}
}

// TestHelpLines_NonWriteCommandsAboveWriteHeader pins the
// 2026-05-08 invariant: every non-write command (anything for
// which isWriteModeCommand returns false) must render ABOVE the
// "── Write-mode commands ──" header. Putting a non-write entry
// below the header — e.g. /branch / /cancel / /env / /repos /
// /version / /exit — would mislead operators into thinking the
// command needs codrax.yaml :: write_enabled: true. The
// regression this guards against: slashCommands order drifted so
// /mode (the header trigger) appeared before genuinely universal
// helpers, sweeping them into the write-mode section.
func TestHelpLines_NonWriteCommandsAboveWriteHeader(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			lines := helpLinesAll(lang)
			joined := strings.Join(lines, "\n")
			headerSubstr := "Write-mode commands"
			if isZh(lang) {
				headerSubstr = "写模式命令"
			}
			headerIdx := strings.Index(joined, headerSubstr)
			if headerIdx < 0 {
				t.Fatalf("/help (%s) missing write-mode header %q", lang, headerSubstr)
			}
			for _, c := range slashCommands {
				if isWriteModeCommand(c.Name) {
					continue
				}
				cmdIdx := strings.Index(joined, "  "+c.Name+" ")
				if cmdIdx < 0 {
					// Pad-aware lookup: helpLines pads the name to
					// the longest column. Try a tab-loose match.
					cmdIdx = strings.Index(joined, c.Name)
				}
				if cmdIdx < 0 {
					t.Errorf("%s: non-write command %q absent from /help output", lang, c.Name)
					continue
				}
				if cmdIdx > headerIdx {
					t.Errorf(
						"%s: non-write command %q renders BELOW the write-mode header (cmd=%d header=%d) — operator will read it as write-only. "+
							"Move it earlier in slashCommands or add it to isWriteModeCommand if it really is write-only.",
						lang, c.Name, cmdIdx, headerIdx)
				}
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

// TestPlanShowFooter_StatusAware pins that the footer surfaces
// status-specific next-action cards rather than a flat command menu.
func TestPlanShowFooter_StatusAware(t *testing.T) {
	for _, c := range []struct {
		status   string
		mustHave string
	}{
		{"pending_approval", "approval required"},
		{"verify_failed", "--retry"},
		{"partially_applied", "partially applied"},
		{"unverified", "--skip-verify"},
		{"applied", "applied and verified"},
	} {
		lines := planShowFooter("en", c.status)
		joined := strings.Join(lines, " ")
		if !strings.Contains(joined, c.mustHave) {
			t.Errorf("status=%s footer missing %q; got %q", c.status, c.mustHave, joined)
		}
	}
}

// TestPlanReadyMultiPhaseNudge_NamesPhaseCount pins the multi-phase
// nudge: it tells the operator how many phases queued and points at the
// LIVE inspection surface (/workflow — the PlanGroup lane is retired).
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
		if !strings.Contains(joined, "/workflow show") {
			t.Errorf("%s: nudge should point at /workflow show; got %q", lang, joined)
		}
		if strings.Contains(joined, "/phase rollback") {
			t.Errorf("%s: nudge must not advertise retired verbs; got %q", lang, joined)
		}
	}
}

// /help renders concise bilingual guidance: zh by default, en only
// with explicit lang. The complete command table is /help all.
func TestHelpLines_BothLangs(t *testing.T) {
	zhLines := helpLines("zh")
	enLines := helpLines("en")
	zhJoined := strings.Join(zhLines, "\n")
	enJoined := strings.Join(enLines, "\n")
	if !strings.Contains(zhJoined, "常用入口") {
		t.Errorf("zh header missing 常用入口; got %q", zhJoined)
	}
	if !strings.Contains(enJoined, "common paths") {
		t.Errorf("en header missing 'common paths'; got %q", enJoined)
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

func TestHtraceConvertCoverageMsgsFollowLanguage(t *testing.T) {
	coverage := []hitraceconv.TraceDBCoverage{{
		Family:      "builtin_modern_ftrace:sched",
		Table:       "sched_switch",
		RowsRead:    1,
		RowsEmitted: 1,
	}}
	zh := strings.Join(htraceConvertCoverageMsgs("zh", "trace_coverage", coverage), "\n")
	if !strings.Contains(zh, "trace_coverage：1 项，输出=1，跳过=0") ||
		!strings.Contains(zh, "族=builtin_modern_ftrace:sched") ||
		strings.Contains(zh, "rows_read=") ||
		strings.Contains(zh, "emitted=") ||
		strings.Contains(zh, "skipped=") {
		t.Fatalf("zh coverage message malformed:\n%s", zh)
	}
	en := strings.Join(htraceConvertCoverageMsgs("en", "trace_coverage", coverage), "\n")
	if !strings.Contains(en, "trace_coverage: 1 item") ||
		!strings.Contains(en, "family=builtin_modern_ftrace:sched") ||
		!strings.Contains(en, "rows_read=1") {
		t.Fatalf("en coverage message malformed:\n%s", en)
	}
}
