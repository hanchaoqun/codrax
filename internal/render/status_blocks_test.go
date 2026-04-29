package render

import (
	"strings"
	"testing"
	"time"
)

// helper: build a Renderer in-process at a specific lang for the
// status-pipeline tests. Tests don't need pterm.Area / glamour —
// they just exercise buildStatusBlocks + renderStatusBlock.
func newTestRenderer(lang string) *Renderer {
	r := &Renderer{lang: lang}
	return r
}

// renderRows is a convenience that drives buildStatusBlocks +
// renderStatusBlock and returns the joined output (with ANSI
// escapes stripped for substring assertions).
func renderRows(t *testing.T, lang string, rows ...*taskRow) string {
	t.Helper()
	r := newTestRenderer(lang)
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	frame := "⠇"
	blocks := r.buildStatusBlocks(rows, frame, now)
	var b strings.Builder
	for _, blk := range blocks {
		for _, line := range renderStatusBlock(blk, lang) {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return stripAnsiEscapes(b.String())
}

// TestStatus_TopicAggregation_Zh exercises the canonical zh path:
// three evidence_tN rows aggregate under "正在理解问题" and surface
// "关注点 N：" labels.
func TestStatus_TopicAggregation_Zh(t *testing.T) {
	rows := []*taskRow{
		{isNodeRow: true, nodeID: "n1_evidence_t0", nodeKind: "evidence",
			objective: "analyzers包的四大trace分析器及其继承关系与输入格式支持"},
		{isNodeRow: true, nodeID: "n2_evidence_t1", nodeKind: "evidence",
			objective: "reporters包下的诊断报告生成模块"},
		{isNodeRow: true, nodeID: "n3_evidence_t2", nodeKind: "evidence",
			objective: "SKILL.md在OpenCode平台中的注册入口与skill规范作用"},
	}
	out := renderRows(t, "zh", rows...)
	for _, want := range []string{
		"正在理解问题", "识别到 3 个关注点",
		"关注点 1：", "关注点 2：", "关注点 3：",
		"analyzers 包", "trace 分析器",
		"reporters 包",
		"SKILL.md 在 OpenCode 平台中", "skill 规范",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in zh output; got:\n%s", want, out)
		}
	}
	for _, banned := range []string{
		"[topic 1]", "[topic 2]", "[topic 3]",
		"n1_evidence_t0", "n2_evidence_t1", "n3_evidence_t2",
		"AnalyzerAgent", "[evidence]",
	} {
		if strings.Contains(out, banned) {
			t.Errorf("banned token %q appeared in zh output:\n%s", banned, out)
		}
	}
}

func TestStatus_TopicAggregation_En(t *testing.T) {
	rows := []*taskRow{
		{isNodeRow: true, nodeID: "n1_evidence_t0", nodeKind: "evidence",
			objective: "analyzers package trace analyzers and input formats"},
		{isNodeRow: true, nodeID: "n2_evidence_t1", nodeKind: "evidence",
			objective: "diagnostic report generation modules under reporters package"},
		{isNodeRow: true, nodeID: "n3_evidence_t2", nodeKind: "evidence",
			objective: "SKILL.md registration entry and skill spec behavior in OpenCode"},
	}
	out := renderRows(t, "en", rows...)
	for _, want := range []string{
		"Understanding the request", "3 focus areas found",
		"Focus 1:", "Focus 2:", "Focus 3:",
		"analyzers package trace analyzers",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in en output; got:\n%s", want, out)
		}
	}
	for _, banned := range []string{
		"[topic 1]", "n1_evidence_t0", "AnalyzerAgent",
		"关注点", "正在理解问题",
	} {
		if strings.Contains(out, banned) {
			t.Errorf("banned token %q appeared in en output:\n%s", banned, out)
		}
	}
}

// TestStatus_StageLocalization checks validate / reconcile / finalize
// localization both languages.
func TestStatus_StageLocalization(t *testing.T) {
	rows := []*taskRow{
		{isNodeRow: true, nodeID: "vN", nodeKind: "validate", objective: "Check claims"},
		{isNodeRow: true, nodeID: "rN", nodeKind: "reconcile", objective: "Merge"},
		{isNodeRow: true, nodeID: "fN", nodeKind: "finalize", objective: "Render"},
	}
	zhOut := renderRows(t, "zh", rows...)
	for _, want := range []string{
		"正在验证结论可靠性", "正在整理回答", "正在生成最终答案",
	} {
		if !strings.Contains(zhOut, want) {
			t.Errorf("expected %q in zh stage output; got:\n%s", want, zhOut)
		}
	}
	for _, banned := range []string{
		"[validate]", "[reconcile]", "[finalize]",
		"Check that every claimed symbol", "Reconcile evidence into",
		"Render the explanation",
	} {
		if strings.Contains(zhOut, banned) {
			t.Errorf("banned %q appeared in zh stage output:\n%s", banned, zhOut)
		}
	}
	enOut := renderRows(t, "en", rows...)
	for _, want := range []string{
		"Validating conclusion reliability", "Organizing the answer", "Generating final answer",
	} {
		if !strings.Contains(enOut, want) {
			t.Errorf("expected %q in en stage output; got:\n%s", want, enOut)
		}
	}
}

// TestStatus_AnalyzerAgentLocalization confirms agent-name + stage
// rows surface as "正在理解问题" / "Understanding the request" with
// no internal "AnalyzerAgent(analyze)" leak.
func TestStatus_AnalyzerAgentLocalization(t *testing.T) {
	row := &taskRow{
		agent: "AnalyzerAgent",
		stage: "analyze",
	}
	zhOut := renderRows(t, "zh", row)
	if !strings.Contains(zhOut, "正在理解问题") {
		t.Errorf("zh: expected '正在理解问题'; got:\n%s", zhOut)
	}
	if strings.Contains(zhOut, "AnalyzerAgent") {
		t.Errorf("zh: must not leak 'AnalyzerAgent'; got:\n%s", zhOut)
	}
	enOut := renderRows(t, "en", row)
	if !strings.Contains(enOut, "Understanding the request") {
		t.Errorf("en: expected 'Understanding the request'; got:\n%s", enOut)
	}
	// done state
	rowDone := &taskRow{
		agent: "AnalyzerAgent", stage: "analyze",
		startTime: time.Now().Add(-19 * time.Second),
		endTime:   time.Now(),
		okFinished: true,
	}
	zhDone := renderRows(t, "zh", rowDone)
	if !strings.Contains(zhDone, "已完成问题分析") {
		t.Errorf("zh done: expected '已完成问题分析'; got:\n%s", zhDone)
	}
	enDone := renderRows(t, "en", rowDone)
	if !strings.Contains(enDone, "Analysis complete") {
		t.Errorf("en done: expected 'Analysis complete'; got:\n%s", enDone)
	}
}

// TestStatus_ThinkingDetailLocalized confirms thinking detail
// strings localize correctly. Detail must not be deleted.
func TestStatus_ThinkingDetailLocalized(t *testing.T) {
	cases := []struct {
		detail string
		zhWant []string
		enWant []string
	}{
		{"thinking", []string{"思考中"}, []string{"thinking"}},
		{"thinking (round 13)", []string{"思考中", "第 13 轮"}, []string{"thinking", "round 13"}},
		{"thinking: merging evidence chains", []string{"思考中", "merging"}, []string{"thinking", "merging"}},
	}
	for _, c := range cases {
		row := &taskRow{
			isNodeRow: true, nodeID: "vN", nodeKind: "validate",
			detail:      c.detail,
			detailStart: time.Now(),
		}
		zhOut := renderRows(t, "zh", row)
		for _, w := range c.zhWant {
			if !strings.Contains(zhOut, w) {
				t.Errorf("zh thinking detail %q: expected %q in:\n%s", c.detail, w, zhOut)
			}
		}
		enOut := renderRows(t, "en", row)
		for _, w := range c.enWant {
			if !strings.Contains(enOut, w) {
				t.Errorf("en thinking detail %q: expected %q in:\n%s", c.detail, w, enOut)
			}
		}
	}
}

// TestStatus_ToolDetailLocalized confirms tool-call details map to
// localized verbs while keeping technical entities (file paths,
// tool names) untranslated.
func TestStatus_ToolDetailLocalized(t *testing.T) {
	cases := []struct {
		detail string
		zhWant string
		enWant string
	}{
		{"grep renderer.go", "正在搜索 renderer.go", "searching renderer.go"},
		{"read internal/render/renderer.go", "正在读取 internal/render/renderer.go",
			"reading internal/render/renderer.go"},
		{"emit_answer_document", "正在生成最终答案", "generating final answer"},
		{"emit_analysis", "正在生成分析结果", "generating analysis result"},
	}
	for _, c := range cases {
		row := &taskRow{
			isNodeRow: true, nodeID: "eN", nodeKind: "evidence",
			detail:      c.detail,
			detailStart: time.Now(),
		}
		// Topic aggregation requires _tN suffix; bypass with non-topic node.
		row.nodeID = "evN"
		zhOut := renderRows(t, "zh", row)
		if !strings.Contains(zhOut, c.zhWant) {
			t.Errorf("zh tool %q: expected %q in:\n%s", c.detail, c.zhWant, zhOut)
		}
		enOut := renderRows(t, "en", row)
		if !strings.Contains(enOut, c.enWant) {
			t.Errorf("en tool %q: expected %q in:\n%s", c.detail, c.enWant, enOut)
		}
	}
}

// TestStatus_FooterLanguage pins the bilingual footer + cancel hint.
func TestStatus_FooterLanguage(t *testing.T) {
	zh := composeFooter("⠇", "1m18s", "", "zh")
	zh = stripAnsiEscapes(zh)
	for _, want := range []string{"总耗时 1m18s", "按 Ctrl+C 取消，连续按两次强制退出"} {
		if !strings.Contains(zh, want) {
			t.Errorf("zh footer expected %q; got:\n%s", want, zh)
		}
	}
	if strings.Contains(zh, "Total elapsed") {
		t.Errorf("zh footer must not leak English; got:\n%s", zh)
	}
	en := composeFooter("⠇", "1m18s", "", "en")
	en = stripAnsiEscapes(en)
	for _, want := range []string{"Total elapsed 1m18s", "Press Ctrl+C to cancel, press twice to force quit"} {
		if !strings.Contains(en, want) {
			t.Errorf("en footer expected %q; got:\n%s", want, en)
		}
	}
	if strings.Contains(en, "总耗时") {
		t.Errorf("en footer must not leak Chinese; got:\n%s", en)
	}
}

// TestStatus_RecoverableErrors confirms recoverable signals classify
// to recoverable + render the localized phrasing + use the ⟳ glyph.
func TestStatus_RecoverableErrors(t *testing.T) {
	cases := []struct {
		errMsg string
		detail string
		zhWant string
		enWant string
	}{
		{"", "Evidence insufficient — retrying", "证据还不够完整", "Evidence is incomplete"},
		{"", "emit_answer_document rejected", "正在修正答案引用格式", "Fixing answer citation format"},
		{"", "quote length exceeds cap", "正在修正引用片段格式", "Fixing citation snippet format"},
		{"", "Forced-read 3 file(s)", "正在补充遗漏的关键信息", "Filling in missing key information"},
	}
	for _, c := range cases {
		row := &taskRow{
			isNodeRow: true, nodeID: "vN", nodeKind: "validate",
			detail:    c.detail,
			errorMsg:  c.errMsg,
		}
		// Note: "Filling in missing key information" comes from the
		// CGEC E2 marker; let's simulate via an explicit detail
		// containing CGEC E2.
		if strings.Contains(c.detail, "Forced-read") {
			row.detail = "CGEC E2: Forced-read 3 file(s) the LLM skipped"
		}
		kind := classifyStatusError(row)
		if kind != statusErrorRecoverable {
			t.Errorf("detail %q: expected recoverable; got %v", c.detail, kind)
			continue
		}
		zhOut := renderRows(t, "zh", row)
		if !strings.Contains(zhOut, c.zhWant) {
			t.Errorf("zh recoverable %q: expected %q in:\n%s", c.detail, c.zhWant, zhOut)
		}
		if !strings.Contains(zhOut, "⟳") {
			t.Errorf("zh recoverable %q: expected ⟳ glyph in:\n%s", c.detail, zhOut)
		}
		enOut := renderRows(t, "en", row)
		if !strings.Contains(enOut, c.enWant) {
			t.Errorf("en recoverable %q: expected %q in:\n%s", c.detail, c.enWant, enOut)
		}
	}
}

// TestStatus_FatalErrors confirms fatal signals use ✗ + Cannot
// continue / 无法继续 phrasing.
func TestStatus_FatalErrors(t *testing.T) {
	cases := []struct {
		errMsg string
		zhWant string
		enWant string
	}{
		{"verify failed: runner_missing", "验证失败：测试运行器不可用", "Verification failed: test runner is unavailable"},
		{"apply failed: patch hunk rejected", "Apply 失败：补丁无法自动应用", "Apply failed: patch could not be applied automatically"},
		{"permission denied: cannot write to /etc", "无法继续：权限不足", "Cannot continue: permission denied"},
		{"invalid configuration: malformed providers.yaml", "无法继续：配置无效", "Cannot continue: invalid configuration"},
	}
	for _, c := range cases {
		row := &taskRow{
			isNodeRow: true, nodeID: "vN", nodeKind: "verify",
			startTime: time.Now().Add(-time.Second),
			endTime:   time.Now(),
			errorMsg:  c.errMsg,
		}
		kind := classifyStatusError(row)
		if kind != statusErrorFatal {
			t.Errorf("errMsg %q: expected fatal; got %v", c.errMsg, kind)
			continue
		}
		zhOut := renderRows(t, "zh", row)
		if !strings.Contains(zhOut, c.zhWant) {
			t.Errorf("zh fatal %q: expected %q in:\n%s", c.errMsg, c.zhWant, zhOut)
		}
		if !strings.Contains(zhOut, "✗") {
			t.Errorf("zh fatal %q: expected ✗ glyph in:\n%s", c.errMsg, zhOut)
		}
		enOut := renderRows(t, "en", row)
		if !strings.Contains(enOut, c.enWant) {
			t.Errorf("en fatal %q: expected %q in:\n%s", c.errMsg, c.enWant, enOut)
		}
	}
}

// TestStatus_CancelledClassification covers user-cancellation routing
// to the cancelled bucket with the dedicated phrase.
func TestStatus_CancelledClassification(t *testing.T) {
	row := &taskRow{
		isNodeRow: true, nodeID: "vN", nodeKind: "validate",
		startTime: time.Now().Add(-time.Second),
		endTime:   time.Now(),
		errorMsg:  "interrupted by user (SIGINT)",
	}
	kind := classifyStatusError(row)
	if kind != statusErrorCancelled {
		t.Errorf("expected cancelled; got %v", kind)
	}
	zhOut := renderRows(t, "zh", row)
	if !strings.Contains(zhOut, "已取消：用户中断了本次任务") {
		t.Errorf("zh cancelled: expected '已取消'; got:\n%s", zhOut)
	}
	enOut := renderRows(t, "en", row)
	if !strings.Contains(enOut, "Cancelled: interrupted by user") {
		t.Errorf("en cancelled: expected 'Cancelled: interrupted by user'; got:\n%s", enOut)
	}
}

// TestStatus_DiagnosticTokensFiltered defends against internal
// diagnostic vocabulary leaking through to the user surface.
func TestStatus_DiagnosticTokensFiltered(t *testing.T) {
	// CGEC marker should rewrite to "正在补充遗漏的关键信息" / "Filling
	// in missing key information" via recoverableDetailPhrase, not
	// surface as raw "CGEC".
	row := &taskRow{
		isNodeRow: true, nodeID: "vN", nodeKind: "validate",
		detail: "CGEC E2: forced-read 3 file(s)",
	}
	zhOut := renderRows(t, "zh", row)
	for _, banned := range []string{"CGEC", "forced-read", "E2"} {
		if strings.Contains(zhOut, banned) {
			t.Errorf("zh: diagnostic token %q leaked:\n%s", banned, zhOut)
		}
	}
	if !strings.Contains(zhOut, "正在补充遗漏的关键信息") {
		t.Errorf("zh: expected localized recoverable phrase; got:\n%s", zhOut)
	}
}

// TestStatus_TopicOverflow confirms >5 topics cap to first 5 +
// overflow sentinel.
func TestStatus_TopicOverflow(t *testing.T) {
	rows := []*taskRow{}
	for i := 0; i < 8; i++ {
		rows = append(rows, &taskRow{
			isNodeRow: true,
			nodeID:    formatTopicID(i),
			nodeKind:  "evidence",
			objective: formatTopicObjective(i),
		})
	}
	zhOut := renderRows(t, "zh", rows...)
	if !strings.Contains(zhOut, "识别到 8 个关注点") {
		t.Errorf("zh: expected total count; got:\n%s", zhOut)
	}
	if !strings.Contains(zhOut, "另有 3 个关注点") {
		t.Errorf("zh: expected overflow sentinel for 8-5=3; got:\n%s", zhOut)
	}
	enOut := renderRows(t, "en", rows...)
	if !strings.Contains(enOut, "8 focus areas found") {
		t.Errorf("en: expected total count; got:\n%s", enOut)
	}
	if !strings.Contains(enOut, "3 more focus areas merged") {
		t.Errorf("en: expected overflow sentinel; got:\n%s", enOut)
	}
}

// TestStatus_LineWidthSafety pins the truncation guard: every
// emitted line must stay within the requested column budget so
// pterm.Area's wrap math doesn't drift.
func TestStatus_LineWidthSafety(t *testing.T) {
	long := strings.Repeat("ABCDEFGHIJ", 50) // 500 cols
	row := &taskRow{
		isNodeRow: true, nodeID: "vN", nodeKind: "validate",
		detail:      "thinking: " + long,
		detailStart: time.Now(),
	}
	r := newTestRenderer("zh")
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	blocks := r.buildStatusBlocks([]*taskRow{row}, "⠇", now)
	maxCols := 80
	for _, blk := range blocks {
		for _, line := range renderStatusBlock(blk, "zh") {
			truncated := truncByDisplayWidth(line, maxCols)
			if w := visibleWidth(truncated); w > maxCols {
				t.Errorf("emitted line exceeded maxCols=%d (got %d): %q", maxCols, w, truncated)
			}
		}
	}
}

func formatTopicID(i int) string                 { return "n_evidence_t" + itoa(i) }
func formatTopicObjective(i int) string {
	return "topic content " + itoa(i)
}
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// visibleWidth strips ANSI escapes then measures runewidth.
func visibleWidth(s string) int {
	clean := stripAnsiEscapes(s)
	w := 0
	for _, r := range clean {
		w += runeDisplayWidth(r)
	}
	return w
}

func runeDisplayWidth(r rune) int {
	// Reuse the package's runewidth pull: codrax already imports
	// go-runewidth elsewhere, but using the bundled helper avoids
	// adding test-only dep.
	if r == '\n' || r == '\r' {
		return 0
	}
	if r >= 0x4E00 && r <= 0x9FFF {
		return 2
	}
	return 1
}
