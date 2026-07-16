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
	"github.com/hanchaoqun/codrax/internal/tracebundle"
	"github.com/hanchaoqun/codrax/internal/types"
)

func htraceConvertTestSystraceArtifact(path string, ready bool) hitraceconv.Artifact {
	return hitraceconv.Artifact{
		Type: hitraceconv.ArtifactSystrace,
		Path: path,
		Trace: &hitraceconv.TraceArtifactCapability{
			ProviderKind:       "builtin_modern",
			ProviderName:       "codrax_builtin_modern_profiler",
			OutputFormat:       hitraceconv.ArtifactSystrace,
			ValidationProfile:  "builtin_systrace_v1",
			Rows:               1,
			Known:              1,
			AuthoritativeKnown: 1,
			TraceQueryReady:    ready,
		},
	}
}

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
			for _, want := range []string{"/htrace", "/htrace convert [opts] <binary> [out.systrace]", "/htrace tools-status"} {
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
			if !strings.Contains(concise, "/htrace "+htraceConvertSubcommandSyntax) {
				t.Errorf("%s: concise help should surface trace conversion; got:\n%s", lang, concise)
			}
			for _, hidden := range []string{"/htrace tools-status", "/plan clear --all", "/merge --include-failed"} {
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
	if !strings.Contains(concise, "/htrace "+htraceConvertSubcommandSyntax) {
		t.Fatalf("/help should surface trace conversion in concise help; got:\n%s", concise)
	}
	if strings.Contains(concise, "/htrace tools-status") {
		t.Fatalf("/help should keep tools-status in full help only; got:\n%s", concise)
	}

	for _, line := range []string{"/help all", "/help full", "/help --all"} {
		out.Reset()
		r.handleSlash(line)
		full := out.String()
		if !strings.Contains(full, "/htrace convert [opts] <binary> [out.systrace]") {
			t.Fatalf("%s should render the complete command table; got:\n%s", line, full)
		}
		if !strings.Contains(full, "/htrace tools-status") {
			t.Fatalf("%s should render trace tools-status; got:\n%s", line, full)
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
		Role:        "query_ready_export",
		RowsRead:    1,
		RowsEmitted: 1,
		ElapsedUS:   42,
	}}
	zh := strings.Join(htraceConvertCoverageMsgs("zh", "trace_coverage", coverage), "\n")
	if !strings.Contains(zh, "trace_coverage：1 项，输出=1，跳过=0") ||
		!strings.Contains(zh, "族=builtin_modern_ftrace:sched") ||
		!strings.Contains(zh, "用途=可供 trace_query 查询的输出") ||
		!strings.Contains(zh, "耗时us=42") ||
		strings.Contains(zh, "rows_read=") ||
		strings.Contains(zh, "elapsed_us=") ||
		strings.Contains(zh, "emitted=") ||
		strings.Contains(zh, "skipped=") {
		t.Fatalf("zh coverage message malformed:\n%s", zh)
	}
	en := strings.Join(htraceConvertCoverageMsgs("en", "trace_coverage", coverage), "\n")
	if !strings.Contains(en, "trace_coverage: 1 item") ||
		!strings.Contains(en, "family=builtin_modern_ftrace:sched") ||
		!strings.Contains(en, "role=query_ready_export") ||
		!strings.Contains(en, "rows_read=1") ||
		!strings.Contains(en, "elapsed_us=42") {
		t.Fatalf("en coverage message malformed:\n%s", en)
	}
}

func TestHtraceConvertCoverageMsgsPrioritizesExactSorterResourceProof(t *testing.T) {
	coverage := make([]hitraceconv.TraceDBCoverage, 0, 9)
	for i := 0; i < 5; i++ {
		coverage = append(coverage, hitraceconv.TraceDBCoverage{
			Family: "regular", Table: fmt.Sprintf("table_%d", i), Role: "query_ready_export", RowsRead: 1, RowsEmitted: 1,
		})
	}
	coverage = append(coverage,
		hitraceconv.TraceDBCoverage{Family: "sorter_v2", Table: "__systrace_rows__", Role: "systrace_text_output"},
		hitraceconv.TraceDBCoverage{Family: "sorter", Table: "__systrace_rows___", Role: "systrace_text_output"},
		hitraceconv.TraceDBCoverage{Family: "builtin_modern_profiler", Table: "__systrace_rows__", Role: "systrace_text_output_v2"},
		hitraceconv.TraceDBCoverage{
			Family: "builtin_modern_profiler", Table: "__systrace_rows__", Role: "systrace_text_output",
			RowsRead: 11, RowsEmitted: 10,
			FieldSources: map[string]string{
				"row_buffer_limits": "67108864_bytes+200000_rows",
				"merge_limits":      "32_input_runs+33_total_run_fds",
				"temp_limits":       "4294967296_active_bytes+8589934592_live_bytes",
			},
			PeakBuffered: 7, PeakBufferedBytes: 4096, SpillChunks: 2, TempBytes: 8192,
			CurrentLiveTempBytes: 0, PeakLiveTempBytes: 6144, PeakOpenRunFDs: 3, MergePasses: 2,
		},
	)

	zh := strings.Join(htraceConvertCoverageMsgs("zh", "trace_coverage", coverage), "\n")
	for _, want := range []string{
		"trace_coverage[8]：",
		"族=builtin_modern_profiler",
		"行缓冲上限=67108864_bytes+200000_rows",
		"归并上限=32_input_runs+33_total_run_fds",
		"临时存储上限=4294967296_active_bytes+8589934592_live_bytes",
		"缓冲峰值行=7",
		"缓冲峰值字节=4096",
		"累计临时写入字节=8192",
		"当前存活临时字节=0",
		"存活临时字节峰值=6144",
		"打开run文件峰值=3",
		"归并轮次=2",
	} {
		if !strings.Contains(zh, want) {
			t.Fatalf("zh sorter coverage missing %q:\n%s", want, zh)
		}
	}
	for _, absent := range []string{"族=sorter_v2", "表=__systrace_rows___", "用途=systrace_text_output_v2"} {
		if strings.Contains(zh, absent) {
			t.Fatalf("fuzzy sorter identity gained a detail seat via %q:\n%s", absent, zh)
		}
	}

	en := strings.Join(htraceConvertCoverageMsgs("en", "trace_coverage", coverage), "\n")
	for _, want := range []string{
		"trace_coverage[8]:",
		"row_buffer_limits=67108864_bytes+200000_rows",
		"peak_buffered_bytes=4096",
		"cumulative_temp_bytes=8192",
		"current_live_temp_bytes=0",
		"peak_live_temp_bytes=6144",
		"peak_open_run_fds=3",
		"merge_passes=2",
	} {
		if !strings.Contains(en, want) {
			t.Fatalf("en sorter coverage missing %q:\n%s", want, en)
		}
	}
	if strings.Contains(en, " temp_bytes=") {
		t.Fatalf("cumulative TempBytes was mislabeled as a live-footprint gauge:\n%s", en)
	}
}

func TestHtraceConvertCoverageMsgsPrioritizesExactPerfReceiptOnlyInTraceLane(t *testing.T) {
	coverage := make([]hitraceconv.TraceDBCoverage, 0, 9)
	for i := 0; i < 5; i++ {
		coverage = append(coverage, hitraceconv.TraceDBCoverage{
			Family: "regular", Table: fmt.Sprintf("table_%d", i), Role: "query_ready_export", RowsRead: 1, RowsEmitted: 1,
		})
	}
	coverage = append(coverage,
		hitraceconv.TraceDBCoverage{
			Family: tracebundle.PerfReceiptFamily, Table: "perftrace_future", Role: tracebundle.PerfReceiptRole,
			ArtifactPath: "future.perftrace", RowsRead: 1, RowsEmitted: 1,
		},
		hitraceconv.TraceDBCoverage{
			Family: tracebundle.PerfReceiptFamily + "_v2", Table: tracebundle.PerfReceiptTableRawPerf, Role: tracebundle.PerfReceiptRole,
			ArtifactPath: "fuzzy-family.perftrace", RowsRead: 1, RowsEmitted: 1,
		},
		hitraceconv.TraceDBCoverage{
			Family: tracebundle.PerfReceiptFamily, Table: tracebundle.PerfReceiptTableRawPerf, Role: tracebundle.PerfReceiptRole + "_v2",
			ArtifactPath: "fuzzy-role.perftrace", RowsRead: 1, RowsEmitted: 1,
		},
		hitraceconv.TraceDBCoverage{
			Family: tracebundle.PerfReceiptFamily, Table: tracebundle.PerfReceiptTableRawPerf, Role: tracebundle.PerfReceiptRole,
			ArtifactPath: "perf/capture.perftrace", RowsRead: 7, RowsEmitted: 7,
		},
	)

	zh := strings.Join(htraceConvertCoverageMsgs("zh", hitraceconv.TraceCoverageLane, coverage), "\n")
	for _, want := range []string{
		"trace_coverage[8]：",
		"表=perftrace_raw_perf",
		"artifact=perf/capture.perftrace",
		"用途=trace_query 交叉验证",
		"trace_coverage 明细已压缩：总计=9 已显示=6 省略=3",
	} {
		if !strings.Contains(zh, want) {
			t.Fatalf("zh perf receipt coverage missing %q:\n%s", want, zh)
		}
	}
	for _, absent := range []string{"future.perftrace", "fuzzy-family.perftrace", "fuzzy-role.perftrace"} {
		if strings.Contains(zh, absent) {
			t.Fatalf("fuzzy perf receipt gained a trace coverage detail seat via %q:\n%s", absent, zh)
		}
	}

	en := strings.Join(htraceConvertCoverageMsgs("en", hitraceconv.TraceCoverageLane, coverage), "\n")
	for _, want := range []string{
		"trace_coverage[8]:",
		"table=perftrace_raw_perf",
		"artifact=perf/capture.perftrace",
		"role=tracequery_cross_validation",
		"trace_coverage_compacted: total=9 shown=6 omitted=3",
	} {
		if !strings.Contains(en, want) {
			t.Fatalf("en perf receipt coverage missing %q:\n%s", want, en)
		}
	}

	db := strings.Join(htraceConvertCoverageMsgs("en", "trace_db_coverage", coverage), "\n")
	if strings.Contains(db, "perf/capture.perftrace") || strings.Contains(db, "trace_db_coverage[8]") {
		t.Fatalf("exact perf receipt gained a priority seat outside trace coverage lane:\n%s", db)
	}
	if !strings.Contains(db, "trace_db_coverage_compacted: total=9 shown=5 omitted=4") {
		t.Fatalf("DB-lane coverage compaction is not honest:\n%s", db)
	}
}

func TestHtraceConvertNextMsgPinsFourBundleCapabilityStatesInBothLanguages(t *testing.T) {
	readyPerf := hitraceconv.Artifact{
		Type: hitraceconv.ArtifactPerfTrace,
		Path: "capture.perftrace",
		Perf: &hitraceconv.PerfArtifactCapability{TraceQueryReady: true},
	}
	typeOnlyPerf := hitraceconv.Artifact{Type: hitraceconv.ArtifactPerfTrace, Path: "type-only.perftrace"}
	tests := []struct {
		name      string
		result    hitraceconv.Result
		wantZH    []string
		wantEN    []string
		forbidden []string
	}{
		{
			name:   "joint systrace and validated perf",
			result: hitraceconv.Result{OutputPath: "capture.systrace", BundlePath: "capture.tracebundle", Artifacts: []hitraceconv.Artifact{htraceConvertTestSystraceArtifact("capture.systrace", true), readyPerf}},
			wantZH: []string{"/htrace capture.tracebundle", "联合查询 systrace 核心事件与已验证 CPU sample", "clock provenance"},
			wantEN: []string{"/htrace capture.tracebundle", "joint systrace event and validated CPU-sample queries", "clock provenance"},
		},
		{
			name:      "systrace without validated perf",
			result:    hitraceconv.Result{OutputPath: "capture.systrace", BundlePath: "capture.tracebundle", Artifacts: []hitraceconv.Artifact{htraceConvertTestSystraceArtifact("capture.systrace", true), typeOnlyPerf}},
			wantZH:    []string{"/htrace capture.tracebundle", "可查询 systrace 核心事件", "当前没有可供 trace_query 消费的 perftrace CPU sample"},
			wantEN:    []string{"/htrace capture.tracebundle", "core systrace event queries", "no query-ready perftrace CPU samples"},
			forbidden: []string{"联合查询", "joint systrace event and validated"},
		},
		{
			name:   "validated perf without systrace",
			result: hitraceconv.Result{BundlePath: "capture.tracebundle", Artifacts: []hitraceconv.Artifact{readyPerf}},
			wantZH: []string{"/htrace capture.tracebundle", "可聚合已验证的 CPU sample", "没有 systrace trace body", "不能做 trace 时间窗或调度因果关联"},
			wantEN: []string{"/htrace capture.tracebundle", "aggregate validated CPU samples", "no systrace trace body", "cannot correlate trace windows or scheduling causality"},
		},
		{
			name:      "metadata only",
			result:    hitraceconv.Result{BundlePath: "capture.tracebundle", Artifacts: []hitraceconv.Artifact{typeOnlyPerf}},
			wantZH:    []string{"capture.tracebundle", "仅保存 artifact/provenance", "没有可直接查询的 systrace 或已验证 perftrace"},
			wantEN:    []string{"capture.tracebundle", "preserves artifact/provenance metadata only", "no query-ready systrace or validated perftrace"},
			forbidden: []string{"CPU sample", "CPU-sample"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			zh := htraceConvertNextMsg("zh", test.result)
			for _, want := range test.wantZH {
				if !strings.Contains(zh, want) {
					t.Fatalf("zh next message missing %q: %s", want, zh)
				}
			}
			en := htraceConvertNextMsg("en", test.result)
			for _, want := range test.wantEN {
				if !strings.Contains(en, want) {
					t.Fatalf("en next message missing %q: %s", want, en)
				}
			}
			for _, forbidden := range test.forbidden {
				if strings.Contains(zh, forbidden) || strings.Contains(en, forbidden) {
					t.Fatalf("unearned capability wording %q leaked: zh=%q en=%q", forbidden, zh, en)
				}
			}
		})
	}
}

func TestHtraceConvertNextMsgDirectPerfOnlyDisclosesCorrelationBoundary(t *testing.T) {
	result := hitraceconv.Result{Artifacts: []hitraceconv.Artifact{{
		Type: hitraceconv.ArtifactPerfTrace,
		Path: "capture.perftrace",
		Perf: &hitraceconv.PerfArtifactCapability{TraceQueryReady: true},
	}}}
	zh := htraceConvertNextMsg("zh", result)
	for _, want := range []string{"/htrace capture.perftrace", "可聚合已验证的 CPU sample", "没有 systrace trace body", "不能做 trace 时间窗或调度因果关联"} {
		if !strings.Contains(zh, want) {
			t.Fatalf("zh direct perf next message missing %q: %s", want, zh)
		}
	}
	en := htraceConvertNextMsg("en", result)
	for _, want := range []string{"/htrace capture.perftrace", "validated CPU samples can be aggregated", "no systrace trace body", "trace-window or scheduling-causality correlation"} {
		if !strings.Contains(en, want) {
			t.Fatalf("en direct perf next message missing %q: %s", want, en)
		}
	}
}

func TestHtraceConvertNextMsgKeepsInventoryPrimaryOffCausalLane(t *testing.T) {
	result := hitraceconv.Result{
		OutputPath: "primary.systrace",
		BundlePath: "capture.tracebundle",
		Artifacts: []hitraceconv.Artifact{
			htraceConvertTestSystraceArtifact("primary.systrace", false),
			htraceConvertTestSystraceArtifact("secondary.systrace", true),
		},
	}
	for lang, wants := range map[string][]string{
		"en": {"capture.tracebundle", "systrace inventory artifact", "not receipt-validated as query-ready", "cannot support core-event or scheduling-causality queries"},
		"zh": {"capture.tracebundle", "systrace 库存 artifact", "未经收据验证为可查询", "不能用于核心事件或调度因果查询"},
	} {
		got := htraceConvertNextMsg(lang, result)
		for _, want := range wants {
			if !strings.Contains(got, want) {
				t.Fatalf("%s inventory disclosure missing %q: %s", lang, want, got)
			}
		}
		for _, forbidden := range []string{"core systrace event queries", "可查询 systrace 核心事件", "secondary.systrace"} {
			if strings.Contains(got, forbidden) {
				t.Fatalf("%s inventory primary borrowed readiness via %q: %s", lang, forbidden, got)
			}
		}
	}
}

func TestHtraceConvertProgressMsgUsesTerminalMessages(t *testing.T) {
	event := hitraceconv.ProgressEvent{
		Stage:   "trace_streamer_export",
		Status:  hitraceconv.ProgressStatusComplete,
		Message: "completed trace_streamer SQLite DB export",
		Elapsed: 1200 * time.Millisecond,
	}
	zh := htraceConvertProgressMsg("zh", event)
	if !strings.Contains(zh, "状态=完成") ||
		!strings.Contains(zh, "说明=已完成 trace_streamer 导出 SQLite DB") ||
		strings.Contains(zh, "正在运行") {
		t.Fatalf("zh progress complete message malformed:\n%s", zh)
	}
	en := htraceConvertProgressMsg("en", event)
	if !strings.Contains(en, "status=complete") ||
		!strings.Contains(en, "message=completed trace_streamer SQLite DB export") {
		t.Fatalf("en progress complete message malformed:\n%s", en)
	}
}

func TestHtraceConvertTraceProviderDecisionMsgsMatchCLIContract(t *testing.T) {
	decisions := []hitraceconv.TraceProviderDecision{{
		Stage:           "trace_body",
		ProviderKind:    "official_trace_db",
		ProviderName:    "trace_streamer_db",
		OutputPath:      "capture.systrace",
		DBPath:          "capture.trace.db",
		EngineMode:      "auto",
		Selected:        true,
		Attempted:       true,
		Succeeded:       true,
		TraceQueryReady: true,
		ArtifactPath:    "capture.systrace",
		Reason:          "trace_streamer_export_succeeded",
		Caveat:          hitraceconv.EmbeddedTraceStreamerPlatformGapMessage("darwin", "arm64"),
	}}

	en := strings.Join(htraceConvertTraceProviderDecisionMsgs("en", decisions), "\n")
	for _, want := range []string{
		"trace_provider_decision[official_trace_db/trace_streamer_db]:",
		"selected=true", "attempted=true", "succeeded=true", "fallback=false",
		"trace_query_ready=true", "stage=trace_body", "engine=auto",
		"output=capture.systrace", "db=capture.trace.db",
		"artifact=capture.systrace", "reason=trace_streamer_export_succeeded",
		"caveat=default embedded trace_streamer tier has no bundled payload for platform darwin/arm64",
	} {
		if !strings.Contains(en, want) {
			t.Fatalf("English REPL decision is missing CLI parity field %q:\n%s", want, en)
		}
	}

	zh := strings.Join(htraceConvertTraceProviderDecisionMsgs("zh", decisions), "\n")
	for _, want := range []string{
		"trace_provider_decision[official_trace_db/trace_streamer_db]：",
		"已选择=是", "已尝试=是", "已成功=是", "回退路径=否",
		"可供trace_query消费=是", "阶段=trace_body", "引擎=auto",
		"输出=capture.systrace", "DB=capture.trace.db",
		"artifact=capture.systrace", "原因=trace_streamer_export_succeeded",
		"提示=默认内嵌 trace_streamer 层未内嵌 darwin/arm64 平台 payload",
	} {
		if !strings.Contains(zh, want) {
			t.Fatalf("Chinese REPL decision is missing CLI parity field %q:\n%s", want, zh)
		}
	}
}

func TestHitraceConvertFailureAddsTypedUnresolvedStreamerRecovery(t *testing.T) {
	newFallback := func() *hitraceconv.TraceProviderFallbackError {
		return &hitraceconv.TraceProviderFallbackError{
			FirstDecision: hitraceconv.TraceProviderDecision{
				ProviderName: "trace_streamer_db",
				Reason:       "trace_streamer_unavailable",
			},
			FirstSource: "unresolved",
			FirstStage:  "trace_streamer_discovery",
			FirstCode:   "trace_streamer_unavailable",
			Fallback:    fmt.Errorf("built-in sys decoder rejected input: code=invalid_magic magic=0xdf49"),
		}
	}

	for _, test := range []struct {
		lang  string
		lead  string
		clue  string
		check string
	}{
		{lang: "zh", lead: "诊断：", clue: "旧版、slim/external-only 或非标准构建", check: "仅凭本错误不能唯一判定构建类型"},
		{lang: "en", lead: "Diagnosis:", clue: "old, slim/external-only, or non-standard build", check: "cannot uniquely identify the build type"},
	} {
		got := htraceConvertFailedMsg(test.lang, fmt.Errorf("outer wrapper: %w", newFallback()))
		for _, want := range []string{
			"first_provider=\"trace_streamer_db\"",
			"code=\"trace_streamer_unavailable\"",
			"invalid_magic",
			test.lead,
			test.clue,
			test.check,
			"/version",
			"/htrace tools-status",
			`/htrace convert --trace-streamer "<trace_streamer-path>" <input> [out.systrace]`,
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("%s recovery message missing %q:\n%s", test.lang, want, got)
			}
		}
	}
}

func TestHitraceConvertFailureExplainsEmbeddedRuntimeIncompatibility(t *testing.T) {
	err := &hitraceconv.TraceProviderFallbackError{
		FirstDecision: hitraceconv.TraceProviderDecision{ProviderName: "trace_streamer_db", Reason: "trace_streamer_unavailable"},
		FirstSource:   "embedded_runtime_incompatible",
		FirstStage:    "trace_streamer_discovery",
		FirstCode:     "trace_streamer_unavailable",
		FirstCaveats: []string{
			hitraceconv.EmbeddedTraceStreamerRuntimeIncompatibleMessage("2.34", fmt.Errorf("loader_missing: /lib64/ld-linux-x86-64.so.2")),
		},
		Fallback: fmt.Errorf("built-in sys decoder rejected input: invalid_magic"),
	}
	for _, test := range []struct {
		lang  string
		wants []string
	}{
		{lang: "zh", wants: []string{"payload 已存在且通过完整性校验", "Codrax 父程序仍可继续工作", "first_caveats", "child_runtime=glibc>=2.34", "<host-compatible-trace_streamer-path>", "/htrace tools-status"}},
		{lang: "en", wants: []string{"payload is present and integrity-verified", "Codrax parent remains usable", "first_caveats", "child_runtime=glibc>=2.34", "<host-compatible-trace_streamer-path>", "/htrace tools-status"}},
	} {
		got := htraceConvertFailedMsg(test.lang, err)
		for _, want := range append(test.wants, "embedded_runtime_incompatible", "invalid_magic") {
			if !strings.Contains(got, want) {
				t.Fatalf("%s runtime recovery missing %q:\n%s", test.lang, want, got)
			}
		}
		if strings.Contains(got, "旧版、slim") || strings.Contains(got, "old, slim") {
			t.Fatalf("%s runtime mismatch was mislabeled as an unresolved artifact: %s", test.lang, got)
		}
	}
}

func TestHitraceConvertFailureRecoveryRequiresExactTypedDiscoveryLane(t *testing.T) {
	baseline := hitraceconv.TraceProviderFallbackError{
		FirstDecision: hitraceconv.TraceProviderDecision{ProviderName: "trace_streamer_db", Reason: "trace_streamer_unavailable"},
		FirstSource:   "unresolved",
		FirstStage:    "trace_streamer_discovery",
		FirstCode:     "trace_streamer_unavailable",
		Fallback:      fmt.Errorf("built-in sys decoder rejected input: invalid_magic"),
	}
	tests := []struct {
		name string
		err  error
	}{
		{name: "plain text forgery", err: fmt.Errorf("%s", baseline.Error())},
		{name: "embedded integrity", err: &hitraceconv.TraceProviderFallbackError{FirstDecision: baseline.FirstDecision, FirstSource: "embedded_integrity_failure", FirstStage: baseline.FirstStage, FirstCode: baseline.FirstCode, Fallback: baseline.Fallback}},
		{name: "platform gap", err: &hitraceconv.TraceProviderFallbackError{FirstDecision: baseline.FirstDecision, FirstSource: "embedded_default_gap", FirstStage: baseline.FirstStage, FirstCode: baseline.FirstCode, Fallback: baseline.Fallback}},
		{name: "configured provider", err: &hitraceconv.TraceProviderFallbackError{FirstDecision: baseline.FirstDecision, FirstSource: "configured trace_streamer", FirstStage: baseline.FirstStage, FirstCode: baseline.FirstCode, Fallback: baseline.Fallback}},
		{name: "execution failure", err: &hitraceconv.TraceProviderFallbackError{FirstDecision: baseline.FirstDecision, FirstSource: baseline.FirstSource, FirstStage: "trace_streamer_execute", FirstCode: "trace_streamer_failed", Fallback: baseline.Fallback}},
		{name: "wrong provider", err: &hitraceconv.TraceProviderFallbackError{FirstDecision: hitraceconv.TraceProviderDecision{ProviderName: "other", Reason: baseline.FirstDecision.Reason}, FirstSource: baseline.FirstSource, FirstStage: baseline.FirstStage, FirstCode: baseline.FirstCode, Fallback: baseline.Fallback}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, lang := range []string{"zh", "en"} {
				got := htraceConvertFailedMsg(lang, test.err)
				if strings.Contains(got, "诊断：") || strings.Contains(got, "Diagnosis:") || strings.Contains(got, "/htrace tools-status") {
					t.Fatalf("%s imprecise lane triggered artifact diagnosis:\n%s", lang, got)
				}
				if !strings.Contains(got, "invalid_magic") {
					t.Fatalf("%s original error was not preserved: %s", lang, got)
				}
			}
		})
	}
}

func TestHtraceConvertPerfProviderDecisionMsgsMatchCLIContract(t *testing.T) {
	decisions := []hitraceconv.PerfProviderDecision{{
		Stage:           "perf_sidecar",
		ProviderKind:    "builtin_raw",
		ProviderName:    "codrax_raw_perf",
		InputFormat:     "linux_perf_data",
		OutputPath:      "capture.perftrace",
		ParserMode:      "raw",
		Selected:        true,
		Attempted:       true,
		Succeeded:       false,
		Fallback:        true,
		TraceQueryReady: false,
		ArtifactPath:    "capture.perftrace",
		Reason:          "raw_provider_failed",
		Caveat:          "raw perf.data sidecar preserved; normalized .perftrace was generated for trace_query CPU-sample aggregation",
	}}

	en := strings.Join(htraceConvertProviderDecisionMsgs("en", decisions), "\n")
	for _, want := range []string{
		"provider_decision[builtin_raw/codrax_raw_perf]:",
		"selected=true", "attempted=true", "succeeded=false", "fallback=true",
		"trace_query_ready=false", "stage=perf_sidecar", "parser=raw",
		"input=linux_perf_data", "output=capture.perftrace",
		"artifact=capture.perftrace", "reason=raw_provider_failed",
		"caveat=raw perf.data sidecar preserved; normalized .perftrace was generated for trace_query CPU-sample aggregation",
	} {
		if !strings.Contains(en, want) {
			t.Fatalf("English REPL perf decision is missing CLI parity field %q:\n%s", want, en)
		}
	}

	zh := strings.Join(htraceConvertProviderDecisionMsgs("zh", decisions), "\n")
	for _, want := range []string{
		"provider_decision[builtin_raw/codrax_raw_perf]：",
		"已选择=是", "已尝试=是", "已成功=否", "回退路径=是",
		"可供trace_query消费=否", "阶段=perf_sidecar", "解析模式=raw",
		"输入格式=linux_perf_data", "输出=capture.perftrace",
		"artifact=capture.perftrace", "原因=raw_provider_failed",
		"提示=raw perf.data sidecar 已保留；已生成标准化 .perftrace，trace_query 可用于 CPU sample 聚合",
	} {
		if !strings.Contains(zh, want) {
			t.Fatalf("Chinese REPL perf decision is missing CLI parity field %q:\n%s", want, zh)
		}
	}
}

func TestHtraceToolsStatusMsgsExposeExecutionBlockerLikeCLI(t *testing.T) {
	blocker := "direct perf input has no trace body and cannot be combined with trace-only option(s) --trace-streamer"
	status := hitraceconv.TraceToolStatus{
		RequestedEngine:  "auto",
		OrderedRoute:     []string{"direct_perf"},
		FirstLane:        "direct_perf",
		PreflightEngine:  "direct_perf",
		ExecutionBlocker: blocker,
		Caveats: []string{
			"trace provider route is not applicable because the inspected input is a typed standalone perf capture with no trace body",
			"execution_blocked: " + blocker,
		},
		TraceStreamer: hitraceconv.TraceToolProviderStatus{Kind: "official_trace_db", Name: "trace_streamer_db", InstallCommand: "must not render"},
		BuiltinModern: hitraceconv.TraceToolProviderStatus{Kind: "builtin_modern", Name: "codrax_builtin_modern_profiler", InstallCommand: "must not render"},
	}
	en := strings.Join(htraceToolsStatusMsgs("en", status), "\n")
	if !strings.Contains(en, "execution_blocker: "+blocker) || strings.Count(en, blocker) != 1 {
		t.Fatalf("English REPL status did not expose exactly one explicit execution blocker:\n%s", en)
	}
	for _, want := range []string{"trace_provider[official_trace_db/trace_streamer_db]: state=not_applicable", "trace_provider[builtin_modern/codrax_builtin_modern_profiler]: state=not_applicable"} {
		if !strings.Contains(en, want) {
			t.Fatalf("direct-perf REPL provider status missing %q:\n%s", want, en)
		}
	}
	if strings.Contains(en, "state=missing") || strings.Contains(en, "install=must not render") {
		t.Fatalf("direct-perf REPL status falsely advertised a trace provider dependency gap:\n%s", en)
	}
	zh := strings.Join(htraceToolsStatusMsgs("zh", status), "\n")
	for _, want := range []string{"执行阻断：", "direct perf 输入不包含 trace body", "--trace-streamer"} {
		if !strings.Contains(zh, want) {
			t.Fatalf("Chinese REPL status execution blocker missing %q:\n%s", want, zh)
		}
	}
	if strings.Contains(zh, "direct perf input has no trace body") || strings.Contains(zh, "trace provider route is not applicable") || strings.Count(zh, "执行阻断：") != 1 || strings.Contains(zh, "execution_blocker") || strings.Contains(zh, "状态=缺失") || strings.Contains(zh, "安装=must not render") {
		t.Fatalf("Chinese REPL status leaked/duplicated execution blocker:\n%s", zh)
	}
}

func TestAttachedRuntimeLoadedMsgsFollowLanguage(t *testing.T) {
	if got := attachedHitraceLoadedMsg("zh", "capture.tracebundle.json", 12); !strings.Contains(got, "已附加 hitrace") || strings.Contains(got, "attached hitrace loaded") {
		t.Fatalf("zh hitrace attach message malformed: %q", got)
	}
	if got := attachedHitraceLoadedMsg("en", "capture.tracebundle.json", 12); !strings.Contains(got, "attached hitrace loaded") {
		t.Fatalf("en hitrace attach message malformed: %q", got)
	}
	if got := attachedLogLoadedMsg("zh", "crash.log", 34); !strings.Contains(got, "已附加 log") || strings.Contains(got, "attached log loaded") {
		t.Fatalf("zh log attach message malformed: %q", got)
	}
}

func TestHtraceConvertSnapshotProgressTranslationsStayInParity(t *testing.T) {
	for _, test := range []struct {
		stage   string
		message string
		zhStage string
		zhMsg   string
	}{
		{stage: "trace_streamer_input_snapshot", message: "copying immutable trace_streamer input", zhStage: "准备trace_streamer输入快照", zhMsg: "正在复制不可变的 trace_streamer 输入快照"},
		{stage: "simpleperf_input_snapshot", message: "copying immutable simpleperf input", zhStage: "准备simpleperf输入快照", zhMsg: "正在复制不可变的 simpleperf 输入快照"},
		{stage: "hiperf_input_snapshot", message: "copying immutable hiperf input", zhStage: "准备hiperf输入快照", zhMsg: "正在复制不可变的 hiperf 输入快照"},
	} {
		if got := htraceConvertProgressStageZh(test.stage); got != test.zhStage {
			t.Fatalf("stage %s zh=%q want=%q", test.stage, got, test.zhStage)
		}
		if got := htraceConvertProgressMessageZh(test.message); got != test.zhMsg {
			t.Fatalf("message %q zh=%q want=%q", test.message, got, test.zhMsg)
		}
		if got := htraceConvertProgressStageEn(test.stage); got != test.stage {
			t.Fatalf("stage %s en=%q", test.stage, got)
		}
	}
	for message, want := range map[string]string{
		"trace_streamer command boundary rejected":      "trace_streamer 命令完成后的一致性校验失败",
		"simpleperf command boundary rejected":          "simpleperf 命令完成后的一致性校验失败",
		"hiperf command boundary rejected":              "hiperf 命令完成后的一致性校验失败",
		"completed official simpleperf adapter command": "已完成官方 simpleperf 适配器命令",
		"official simpleperf adapter command failed":    "官方 simpleperf 适配器命令失败",
		"completed official hiperf adapter command":     "已完成官方 hiperf 适配器命令",
		"official hiperf adapter command failed":        "官方 hiperf 适配器命令失败",
	} {
		if got := htraceConvertProgressMessageZh(message); got != want {
			t.Fatalf("boundary message %q zh=%q want=%q", message, got, want)
		}
	}
}
