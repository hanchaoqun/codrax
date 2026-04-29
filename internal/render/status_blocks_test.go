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

// renderRowsFixedNow is the fixed wall-clock used by renderRows so
// timing-sensitive tests can set detailStart relative to it (e.g.
// "fresh detailStart" = renderRowsFixedNow.Add(-100ms) → bare-thinking
// renders as "sending"; "stale" = .Add(-5s) → "awaiting").
var renderRowsFixedNow = time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)

// renderRows is a convenience that drives buildStatusBlocks +
// renderStatusBlock and returns the joined output (with ANSI
// escapes stripped for substring assertions).
func renderRows(t *testing.T, lang string, rows ...*taskRow) string {
	t.Helper()
	r := newTestRenderer(lang)
	now := renderRowsFixedNow
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
		// Topic group's parent line uses the "evidence" key so the
		// substantive "探索代码" wording reaches the UI. The
		// previous "explore" → "正在深入分析" framing was an
		// artificial umbrella that hid the load-bearing label
		// because production flows are predominantly multi-topic
		// and per-row evidence labels fold into the bullet list.
		"正在探索代码并收集证据", "识别到 3 个关注点",
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
		// The topic-group parent is NOT analyze — analyze finished
		// upstream of these rows. Pin the rule so a regression
		// doesn't collapse evidence into analyze.
		"正在理解问题",
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
		"Exploring code, collecting evidence", "3 focus areas found",
		"Focus 1:", "Focus 2:", "Focus 3:",
		"analyzers package trace analyzers",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in en output; got:\n%s", want, out)
		}
	}
	for _, banned := range []string{
		"[topic 1]", "n1_evidence_t0", "AnalyzerAgent",
		"关注点", "正在理解问题", "Understanding the request",
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
		"正在校核分析结论", "正在整理结论", "正在生成最终答案",
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
		"Cross-checking findings", "Reconciling findings", "Generating final answer",
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
	if !strings.Contains(zhDone, "已理解问题") {
		t.Errorf("zh done: expected '已理解问题'; got:\n%s", zhDone)
	}
	enDone := renderRows(t, "en", rowDone)
	if !strings.Contains(enDone, "Request understood") {
		t.Errorf("en done: expected 'Request understood'; got:\n%s", enDone)
	}
}

// TestStatus_FullFlowOrdering pins the user-visible problem-solving
// narrative across the read-mode pipeline. The five rows below
// mirror what the user sees during a typical Run: log_triage (when
// an attached log fires) → analyze → topic group (explore) →
// validate → reconcile → extract → finalize. Each stage has its
// localized phrasing and they appear in the documented order.
func TestStatus_FullFlowOrdering(t *testing.T) {
	rows := []*taskRow{
		{stage: "log_triage", agent: "log_triager",
			startTime: time.Now().Add(-30 * time.Second),
			endTime:   time.Now().Add(-25 * time.Second), okFinished: true},
		{stage: "analyze", agent: "AnalyzerAgent",
			startTime: time.Now().Add(-25 * time.Second),
			endTime:   time.Now().Add(-20 * time.Second), okFinished: true},
		// (topic group rendered via NodeEvidence rows — covered by
		// TestStatus_TopicAggregation_Zh)
		{isNodeRow: true, nodeID: "vN", nodeKind: "validate",
			startTime: time.Now().Add(-15 * time.Second),
			endTime:   time.Now().Add(-12 * time.Second), okFinished: true},
		{isNodeRow: true, nodeID: "rN", nodeKind: "reconcile",
			startTime: time.Now().Add(-12 * time.Second),
			endTime:   time.Now().Add(-10 * time.Second), okFinished: true},
		{stage: "extract", agent: "ExtractorAgent",
			startTime: time.Now().Add(-10 * time.Second),
			endTime:   time.Now().Add(-5 * time.Second), okFinished: true},
		{stage: "finalize", agent: "FinalizerAgent",
			startTime: time.Now().Add(-5 * time.Second)},
	}
	zhOut := renderRows(t, "zh", rows...)
	expectedOrder := []string{
		"已解析日志",
		"已理解问题",
		"已校核分析结论",
		"已整理结论",
		"已提取关键要点",
		"正在生成最终答案",
	}
	pos := 0
	for _, want := range expectedOrder {
		idx := strings.Index(zhOut[pos:], want)
		if idx < 0 {
			t.Errorf("zh full-flow: expected %q at-or-after position %d; full output:\n%s",
				want, pos, zhOut)
			break
		}
		pos += idx + len(want)
	}
}

// TestStatus_WriteModeFlow verifies the plan→apply→verify narrative
// in write mode uses correct labels distinct from the read-mode
// validate phrasing.
func TestStatus_WriteModeFlow(t *testing.T) {
	rows := []*taskRow{
		{isNodeRow: true, nodeID: "pN", nodeKind: "plan",
			startTime: time.Now().Add(-20 * time.Second),
			endTime:   time.Now().Add(-15 * time.Second), okFinished: true},
		{isNodeRow: true, nodeID: "aN", nodeKind: "apply",
			startTime: time.Now().Add(-15 * time.Second),
			endTime:   time.Now().Add(-10 * time.Second), okFinished: true},
		{isNodeRow: true, nodeID: "vN", nodeKind: "verify",
			startTime: time.Now().Add(-10 * time.Second)},
	}
	zhOut := renderRows(t, "zh", rows...)
	for _, want := range []string{
		"已设计改动方案", "已应用改动", "正在跑测试验证改动",
	} {
		if !strings.Contains(zhOut, want) {
			t.Errorf("zh write-mode flow: expected %q in:\n%s", want, zhOut)
		}
	}
	// Verify (write-mode) MUST NOT collapse into validate's
	// "正在校核分析结论" — different phase, different word.
	if strings.Contains(zhOut, "校核分析结论") {
		t.Errorf("zh write-mode: NodeVerify must NOT use NodeValidate phrasing; got:\n%s", zhOut)
	}
	enOut := renderRows(t, "en", rows...)
	for _, want := range []string{
		"Change plan ready", "Changes applied", "Running tests for verification",
	} {
		if !strings.Contains(enOut, want) {
			t.Errorf("en write-mode flow: expected %q in:\n%s", want, enOut)
		}
	}
	if strings.Contains(enOut, "Cross-checking findings") {
		t.Errorf("en write-mode: NodeVerify must NOT use NodeValidate phrasing; got:\n%s", enOut)
	}
}

// TestStatus_CompletedRowDropsLiveThinking pins the "completed rows
// must drop live-progress detail" rule. The detail field on a
// finished row carries whatever the last thinking / tool ticker
// snapshot was at the moment the row terminated — surfacing it
// after the fact reads as "still working on this" when the meta
// trailer (elapsed + tool count + round) clearly says otherwise.
func TestStatus_CompletedRowDropsLiveThinking(t *testing.T) {
	completed := &taskRow{
		stage: "analyze", agent: "AnalyzerAgent",
		startTime:   time.Now().Add(-19 * time.Second),
		endTime:     time.Now(),
		okFinished:  true,
		detail:      "thinking (round 13)",
		detailStart: time.Now().Add(-2 * time.Second),
		iteration:   13,
		toolCount:   5,
	}
	zhOut := renderRows(t, "zh", completed)
	if !strings.Contains(zhOut, "已理解问题") {
		t.Errorf("zh: expected primary done label; got:\n%s", zhOut)
	}
	if !strings.Contains(zhOut, "5 次工具调用") || !strings.Contains(zhOut, "第 13 轮") {
		t.Errorf("zh: completed row must keep meta trailer (elapsed + round + tool count); got:\n%s", zhOut)
	}
	// Live-progress phrasing MUST be gone: neither the legacy "思考中"
	// nor any of the current 3 phases (发送中 / 等待响应 / 回复中)
	// should linger on a finished row.
	for _, leaked := range []string{"思考中", "发送中", "等待响应", "回复中"} {
		if strings.Contains(zhOut, leaked) {
			t.Errorf("zh: completed row leaked live thinking detail (%q); got:\n%s", leaked, zhOut)
		}
	}

	// Running row keeps live progress signal. After 2026-04-30 the
	// round number lives ONLY in row.iteration (rendered via meta);
	// detail is bare "thinking" so thinkingPhrase resolves "等待
	// 响应" once detailStart age crosses the sendingThreshold.
	running := &taskRow{
		stage: "analyze", agent: "AnalyzerAgent",
		startTime:   renderRowsFixedNow.Add(-5 * time.Second),
		detail:      "thinking",
		detailStart: renderRowsFixedNow.Add(-5 * time.Second),
		iteration:   13,
	}
	zhRun := renderRows(t, "zh", running)
	if !strings.Contains(zhRun, "等待响应") {
		t.Errorf("zh: running row must show awaiting-model phrase; got:\n%s", zhRun)
	}
	if !strings.Contains(zhRun, "第 13 轮") {
		t.Errorf("zh: running row must include round number; got:\n%s", zhRun)
	}
}

// TestStatus_SingleTopicEvidenceUsesExploreCodeLabel pins the
// "evidence" canonical key — single-topic NodeEvidence rows
// (no _tN suffix, so no topic-group aggregation) MUST surface as
// the explicit "正在探索代码并收集证据" / "Exploring code,
// collecting evidence" label rather than collapsing into the
// generic "正在深入分析" umbrella. The umbrella loses the most
// informative substep — this test prevents a regression that
// re-collapses evidence into explore.
func TestStatus_SingleTopicEvidenceUsesExploreCodeLabel(t *testing.T) {
	row := &taskRow{
		isNodeRow: true,
		nodeID:    "evN",  // no _tN suffix → single-topic, NOT aggregated
		nodeKind:  "evidence",
		objective: "investigate the explorer agent",
	}
	zhOut := renderRows(t, "zh", row)
	if !strings.Contains(zhOut, "正在探索代码并收集证据") {
		t.Errorf("zh: expected '正在探索代码并收集证据' for single-topic evidence row; got:\n%s", zhOut)
	}
	if strings.Contains(zhOut, "正在深入分析") {
		t.Errorf("zh: single-topic evidence row must NOT collapse into the parent 'explore' umbrella; got:\n%s", zhOut)
	}
	enOut := renderRows(t, "en", row)
	if !strings.Contains(enOut, "Exploring code, collecting evidence") {
		t.Errorf("en: expected 'Exploring code, collecting evidence'; got:\n%s", enOut)
	}
	if strings.Contains(enOut, "Investigating the problem") {
		t.Errorf("en: single-topic evidence row must NOT collapse to 'Investigating'; got:\n%s", enOut)
	}
}

// TestStatus_PreStageLogTriage covers the conditional pre-stages:
// log_triage / perf_triage are user-visible when an attached log
// or trace fires, and need their own localized labels rather than
// falling through to the generic "处理任务" fallback.
func TestStatus_PreStageLogTriage(t *testing.T) {
	rows := []*taskRow{
		{stage: "log_triage", agent: "log_triager"},
		{stage: "perf_triage", agent: "perf_triager"},
	}
	zhOut := renderRows(t, "zh", rows...)
	for _, want := range []string{"正在解析日志", "正在解析性能数据"} {
		if !strings.Contains(zhOut, want) {
			t.Errorf("zh pre-stage: expected %q in:\n%s", want, zhOut)
		}
	}
	if strings.Contains(zhOut, "正在处理任务") {
		t.Errorf("zh pre-stage: must not fall through to generic phrasing; got:\n%s", zhOut)
	}
	enOut := renderRows(t, "en", rows...)
	for _, want := range []string{"Parsing attached log", "Parsing performance trace"} {
		if !strings.Contains(enOut, want) {
			t.Errorf("en pre-stage: expected %q in:\n%s", want, enOut)
		}
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
		// Bare "thinking" → "等待响应" / "awaiting" (the test row's
		// detailStart is set to 5s ago by the test harness so the
		// >sendingThreshold branch fires). "thinking: ..." carries
		// the "回复中" / "replying" phrase regardless of timing
		// because the body presence indicates the server is
		// responding. The round number is no longer encoded in the
		// detail string post-2026-04-30 — it lives in row.iteration
		// and renders via meta.
		{"thinking", []string{"等待响应"}, []string{"awaiting"}},
		{"thinking: merging evidence chains", []string{"回复中", "merging"}, []string{"replying", "merging"}},
	}
	for _, c := range cases {
		row := &taskRow{
			isNodeRow: true, nodeID: "vN", nodeKind: "validate",
			detail: c.detail,
			// Anchor detailStart against renderRowsFixedNow (the
			// renderer's `now`) so the elapsed delta is bounded.
			// 5s before now → past sendingThreshold → "等待响应"
			// branch fires for the bare-thinking forms.
			detailStart: renderRowsFixedNow.Add(-5 * time.Second),
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

// TestStatus_ThinkingNTPJumpGuard pins the negative-elapsed clamp:
// when the wall clock jumps backward (rare but observed on NTP
// resync) so detailStart > now, the elapsed comparison would
// otherwise return a negative duration < sendingThreshold and
// claim "发送中" for a request that's actually been in flight a
// while. The clamp guarantees "等待响应" surfaces in this case.
func TestStatus_ThinkingNTPJumpGuard(t *testing.T) {
	row := &taskRow{
		isNodeRow: true, nodeID: "vN", nodeKind: "validate",
		detail: "thinking",
		// detailStart 5 seconds AFTER the renderer's `now` —
		// simulates a backward NTP jump.
		detailStart: renderRowsFixedNow.Add(5 * time.Second),
	}
	zh := renderRows(t, "zh", row)
	if !strings.Contains(zh, "等待响应") {
		t.Errorf("zh: backward-clock-jumped row must default to 等待响应; got:\n%s", zh)
	}
	if strings.Contains(zh, "发送中") {
		t.Errorf("zh: backward-clock-jumped row must NOT claim 发送中; got:\n%s", zh)
	}
}

// TestStatus_PendingHasOwnPhrase pins the "待 X" lexical form for
// pending rows. Pre-fix pending rows reused the running phrase
// ("正在 X"), distinguishable only by colour — two queued rows
// rendered as "正在校核分析结论" / "正在整理结论" reading like
// concurrent execution. The pending phrasing kills the ambiguity
// at the lexical level.
func TestStatus_PendingHasOwnPhrase(t *testing.T) {
	pendingRows := []*taskRow{
		{isNodeRow: true, nodeID: "vN", nodeKind: "validate", pending: true},
		{isNodeRow: true, nodeID: "rN", nodeKind: "reconcile", pending: true},
		{isNodeRow: true, nodeID: "fN", nodeKind: "finalize", pending: true},
	}
	zh := renderRows(t, "zh", pendingRows...)
	for _, want := range []string{"待校核分析结论", "待整理结论", "待生成最终答案"} {
		if !strings.Contains(zh, want) {
			t.Errorf("zh: expected pending phrase %q in:\n%s", want, zh)
		}
	}
	// Pending rows MUST NOT use the running form — that would
	// re-introduce the ambiguity.
	for _, banned := range []string{"正在校核分析结论", "正在整理结论", "正在生成最终答案"} {
		if strings.Contains(zh, banned) {
			t.Errorf("zh: pending row leaked running form %q in:\n%s", banned, zh)
		}
	}

	enRows := []*taskRow{
		{isNodeRow: true, nodeID: "vN", nodeKind: "validate", pending: true},
	}
	en := renderRows(t, "en", enRows...)
	if !strings.Contains(en, "Awaiting cross-check") {
		t.Errorf("en: expected pending phrase 'Awaiting cross-check' in:\n%s", en)
	}
	if strings.Contains(en, "Cross-checking findings") {
		t.Errorf("en: pending row must not use running form 'Cross-checking findings' in:\n%s", en)
	}
}

// TestStatus_PendingDistinctFromDone pins issue 5 — pending and
// done must use DIFFERENT primary-text styles even though both are
// in the gray family. statusMeta (DarkGray) for pending vs
// statusPrimaryDone (Gray) for done. The user-visible distinction
// here is "this hasn't started yet" vs "this finished".
func TestStatus_PendingDistinctFromDone(t *testing.T) {
	doneRow := &taskRow{
		isNodeRow: true, nodeID: "vN", nodeKind: "validate",
		startTime:  renderRowsFixedNow.Add(-5 * time.Second),
		endTime:    renderRowsFixedNow,
		okFinished: true,
	}
	pendingRow := &taskRow{
		isNodeRow: true, nodeID: "v2", nodeKind: "validate",
		pending: true,
	}
	r := newTestRenderer("zh")
	frame := "⠇"
	doneLine := r.buildStatusLine(doneRow, frame, renderRowsFixedNow)
	pendingLine := r.buildStatusLine(pendingRow, frame, renderRowsFixedNow)
	doneOut := stripAnsiEscapes(renderStatusLine(doneLine))
	pendingOut := stripAnsiEscapes(renderStatusLine(pendingLine))
	// Both should render text; the visual distinction is in ANSI
	// codes that strip out, but the State must differ so the
	// downstream style branch picks different tokens.
	if doneLine.State != "done" {
		t.Errorf("done row State = %q, want 'done'", doneLine.State)
	}
	if pendingLine.State != "pending" {
		t.Errorf("pending row State = %q, want 'pending'", pendingLine.State)
	}
	// Sanity: ANSI-stripped text labels are non-empty for both.
	if doneOut == "" || pendingOut == "" {
		t.Errorf("both rows must render text — done=%q pending=%q", doneOut, pendingOut)
	}
	// Direct ANSI inspection: the raw rendered strings carry
	// distinct color codes for done (FgGray = 37/38;5;245-ish) vs
	// pending (FgDarkGray = 90/38;5;240-ish). Without comparing
	// exact codes, just ensure the two RAW outputs differ — same
	// text content + identical styling would mean we collapsed.
	rawDone := renderStatusLine(doneLine)
	rawPending := renderStatusLine(pendingLine)
	if rawDone == rawPending {
		t.Errorf("done and pending styling collapsed to identical output:\n%q", rawDone)
	}
}

// TestStatus_TopicGroupOrderingAfterAnalyze pins the chronological
// placement of the topic group: it MUST surface AFTER the analyze
// row but BEFORE downstream stages (validate / reconcile / extract
// / finalize). Pre-2026-04-30 it was prepended to blocks[0] which
// put "正在探索代码 ..." ABOVE "已理解问题"; users read the analyze
// row first and looked below for the next stage, missing the topic
// group entirely. Anchoring on the first downstream block restores
// pipeline order.
func TestStatus_TopicGroupOrderingAfterAnalyze(t *testing.T) {
	now := renderRowsFixedNow
	rows := []*taskRow{
		// Analyze stage row (terminated).
		{
			stage:      "analyze",
			agent:      "AnalyzerAgent",
			startTime:  now.Add(-30 * time.Second),
			endTime:    now.Add(-10 * time.Second),
			okFinished: true,
		},
		// Two topic evidence rows (still running).
		{isNodeRow: true, nodeID: "n1_evidence_t0", nodeKind: "evidence",
			objective: "topic A"},
		{isNodeRow: true, nodeID: "n2_evidence_t1", nodeKind: "evidence",
			objective: "topic B"},
		// Validate stage (pending — not yet started).
		{isNodeRow: true, nodeID: "vN", nodeKind: "validate", pending: true,
			objective: "cross-check"},
	}
	out := renderRows(t, "zh", rows...)
	// All three primary phrases must appear. Validate is pending in
	// this fixture (queued behind running evidence) so it carries
	// the "待校核 …" form, not "正在校核 …".
	for _, want := range []string{"已理解问题", "正在探索代码并收集证据", "待校核分析结论"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output; got:\n%s", want, out)
		}
	}
	// Order check: analyze must come BEFORE the topic group, which
	// must come BEFORE validate.
	idxAnalyze := strings.Index(out, "已理解问题")
	idxEvidence := strings.Index(out, "正在探索代码并收集证据")
	idxValidate := strings.Index(out, "待校核分析结论")
	if !(idxAnalyze < idxEvidence && idxEvidence < idxValidate) {
		t.Errorf("expected order analyze < evidence < validate; got idx %d / %d / %d in:\n%s",
			idxAnalyze, idxEvidence, idxValidate, out)
	}
}

// TestStatus_AgentThinkingParksOtherRows pins the defense-in-depth
// EventAgentThinking parking: even if the dispatch-window timestamp
// grouping mis-classifies sibling vs cross-window starts (e.g.
// scheduler dispatches two nodes within 750 ms but only one is the
// active LLM agent), EventAgentThinking is a strict signal that
// r.current owns the in-flight LLM call, so every OTHER NodeRow
// must be parked. This is the safety net for the production trace
// where evidence + validate kept rendering as concurrent despite
// the dispatch-gen logic.
func TestStatus_AgentThinkingParksOtherRows(t *testing.T) {
	r := newTestRenderer("zh")
	emit := r.Emitter()
	t0 := time.Now()
	emit(Event{Kind: EventAnalysisReady, Timestamp: t0, TaskNodes: []TaskNodeInfo{
		{ID: "ev_node", Type: "evidence", Objective: "topic"},
		{ID: "vN", Type: "validate", Objective: "cross-check"},
	}})
	// Both rows started within the 750 ms grouping window — would
	// both be considered same-window siblings under the dispatchGen
	// logic alone.
	emit(Event{Kind: EventTaskNodeStart, Timestamp: t0.Add(1 * time.Millisecond), NodeID: "ev_node"})
	emit(Event{Kind: EventTaskNodeStart, Timestamp: t0.Add(100 * time.Millisecond), NodeID: "vN"})
	// Defense-in-depth: validate's agent emits a thinking event.
	// EventAgentThinking only fires for the agent currently in a
	// Chat call → validate is the active row → evidence (the only
	// other in-flight NodeRow) MUST be parked even though it shares
	// a dispatch generation.
	emit(Event{Kind: EventAgentThinking, Timestamp: t0.Add(2 * time.Second), Iteration: 0})

	evRow := r.findNodeRow("ev_node")
	if evRow == nil || !evRow.paused {
		t.Errorf("evidence must be parked after EventAgentThinking signalled validate is active; got paused=%v",
			evRow != nil && evRow.paused)
	}
	vRow := r.findNodeRow("vN")
	if vRow == nil || vRow.paused {
		t.Errorf("active validate row must NOT be parked; got paused=%v",
			vRow != nil && vRow.paused)
	}
}

// TestStatus_SingleEvidenceCrossWindowValidate reproduces the user-
// reported "evidence + validate look concurrent" scenario from the
// production trace at 2026-04-29 23:30+ for the HiTraceAnalyzer
// question:
//
//   - Single-topic evidence (NO _tN suffix → renders as a regular
//     row, not a topic group)
//   - Evidence dispatched in window 1 at T=0
//   - Validate dispatched 38 s later in window 2
//
// Expected behaviour: validate's EventTaskNodeStart MUST be detected
// as cross-window (gap >> dispatchWindowGroupingMs), bump
// r.dispatchGen, and park the older-gen evidence row. Evidence then
// reads lexically as pending ("待探索代码并收集证据") and visually
// dim, while only validate carries the active spinner + LightBlue.
//
// If this test PASSES but the user still sees concurrent rows, the
// REPL is running a stale binary (process pre-dates the fix and
// `make` is not enough — the running REPL must exit and restart).
func TestStatus_SingleEvidenceCrossWindowValidate(t *testing.T) {
	r := newTestRenderer("zh")
	emit := r.Emitter()
	t0 := time.Now()
	// Analyzer ready: evidence (no _tN — single topic) + validate
	// + reconcile + finalize.
	emit(Event{Kind: EventAnalysisReady, Timestamp: t0, TaskNodes: []TaskNodeInfo{
		{ID: "ev_node", Type: "evidence", Objective: "HiTraceAnalyzer flow"},
		{ID: "vN", Type: "validate", Objective: "cross-check"},
		{ID: "rN", Type: "reconcile", Objective: "merge"},
		{ID: "fN", Type: "finalize", Objective: "render"},
	}})
	// Window 1: evidence alone.
	emit(Event{Kind: EventTaskNodeStart, Timestamp: t0.Add(1 * time.Millisecond), NodeID: "ev_node"})
	// Window 2: validate fires 38 s later (gap >> 750 ms grouping
	// threshold). Evidence has NOT received another EventTaskNodeStart.
	emit(Event{Kind: EventTaskNodeStart, Timestamp: t0.Add(38 * time.Second), NodeID: "vN"})

	evRow := r.findNodeRow("ev_node")
	if evRow == nil {
		t.Fatal("evidence row missing")
	}
	if !evRow.paused {
		t.Errorf("cross-window evidence row MUST be parked when validate started in a later window; got paused=false (dispatchGen=%d, lastNodeStartAt=%v)",
			evRow.dispatchGen, r.lastNodeStartAt)
	}
	vRow := r.findNodeRow("vN")
	if vRow == nil {
		t.Fatal("validate row missing")
	}
	if vRow.paused {
		t.Errorf("active validate row must NOT be parked; got paused=true")
	}
	// Render check: evidence's primary text must read as pending
	// ("待 X"), validate as running ("正在 X").
	r.mu.Lock()
	rows := []*taskRow{evRow, vRow}
	r.mu.Unlock()
	zhOut := stripAnsiEscapes(renderRows(t, "zh", rows...))
	if !strings.Contains(zhOut, "待探索代码并收集证据") {
		t.Errorf("evidence must render lexically as pending '待 …'; got:\n%s", zhOut)
	}
	if strings.Contains(zhOut, "正在探索代码并收集证据") {
		t.Errorf("evidence must NOT render as running '正在 …'; got:\n%s", zhOut)
	}
	if !strings.Contains(zhOut, "正在校核分析结论") {
		t.Errorf("active validate must render as running; got:\n%s", zhOut)
	}
}

// TestStatus_DispatchGenerationGroupsSiblings pins the
// dispatch-window grouping: nodes started within
// dispatchWindowGroupingMs ms of each other share a generation and
// stay all-active simultaneously. Cross-window starts (separated by
// the prior window's actual LLM dispatch — typically seconds) bump
// the generation, which parks every older-gen row.
func TestStatus_DispatchGenerationGroupsSiblings(t *testing.T) {
	r := newTestRenderer("zh")
	emit := r.Emitter()
	t0 := time.Now()
	// Window 1: three evidence_tN nodes dispatched as siblings,
	// timestamps within microseconds of each other.
	emit(Event{Kind: EventAnalysisReady, Timestamp: t0, TaskNodes: []TaskNodeInfo{
		{ID: "evN_t0", Type: "evidence", Objective: "topic A"},
		{ID: "evN_t1", Type: "evidence", Objective: "topic B"},
		{ID: "evN_t2", Type: "evidence", Objective: "topic C"},
		{ID: "vN", Type: "validate", Objective: "cross-check"},
	}})
	emit(Event{Kind: EventTaskNodeStart, Timestamp: t0.Add(1 * time.Millisecond), NodeID: "evN_t0"})
	emit(Event{Kind: EventTaskNodeStart, Timestamp: t0.Add(2 * time.Millisecond), NodeID: "evN_t1"})
	emit(Event{Kind: EventTaskNodeStart, Timestamp: t0.Add(3 * time.Millisecond), NodeID: "evN_t2"})
	// All three siblings of window 1 must be active (paused=false)
	// because they were started within the same dispatch window.
	for _, id := range []string{"evN_t0", "evN_t1", "evN_t2"} {
		row := r.findNodeRow(id)
		if row == nil {
			t.Fatalf("row %s missing", id)
		}
		if row.paused {
			t.Errorf("row %s same-window sibling must NOT be parked; got paused=true", id)
		}
	}
	// Window 2: validate fires 5s later (well past the 750ms
	// grouping threshold). Should bump dispatchGen and park every
	// window-1 sibling.
	emit(Event{Kind: EventTaskNodeStart, Timestamp: t0.Add(5 * time.Second), NodeID: "vN"})
	for _, id := range []string{"evN_t0", "evN_t1", "evN_t2"} {
		row := r.findNodeRow(id)
		if row == nil || !row.paused {
			t.Errorf("cross-window: row %s must be parked after later window started a different node; got paused=%v",
				id, row != nil && row.paused)
		}
	}
	if vRow := r.findNodeRow("vN"); vRow == nil || vRow.paused {
		t.Errorf("active validate row must NOT be parked; got %v", vRow)
	}
}

// TestStatus_PausedRowRendersAsPending pins the paused-row contract:
// a node whose endTime is still zero (orchestrator did NOT emit
// EventTaskNodeEnd because it was requeued without termination) but
// whose paused flag is true MUST render lexically and visually as
// pending — "待 X" text, `·` glyph, DarkGray colour. Without this
// fold, two rows in flight (e.g. validate just dispatched while
// evidence_tN was requeued) would both render as "正在 X" and read
// as concurrent execution despite being a hard-sequential chain.
func TestStatus_PausedRowRendersAsPending(t *testing.T) {
	now := renderRowsFixedNow
	rows := []*taskRow{
		// Active row — the live dispatch focus.
		{
			isNodeRow: true, nodeID: "vN", nodeKind: "validate",
			startTime:   now.Add(-2 * time.Second),
			detail:      "thinking",
			detailStart: now.Add(-2 * time.Second),
			iteration:   3,
		},
		// Paused row — was started earlier, requeued without
		// EventTaskNodeEnd, scheduler's focus moved to validate.
		{
			isNodeRow: true, nodeID: "n1_evidence_t0", nodeKind: "evidence",
			objective: "topic A",
			startTime: now.Add(-10 * time.Second),
			paused:    true,
		},
	}
	out := renderRows(t, "zh", rows...)
	// Active validate row reads as running.
	if !strings.Contains(out, "正在校核分析结论") {
		t.Errorf("active validate row must show running phrase; got:\n%s", out)
	}
	// Paused evidence reads as pending — same lexical form pending
	// rows use, so the user sees only ONE active stage at a time.
	if !strings.Contains(out, "待探索代码并收集证据") {
		t.Errorf("paused evidence row must show pending phrase; got:\n%s", out)
	}
	if strings.Contains(out, "正在探索代码并收集证据") {
		t.Errorf("paused evidence row must NOT show running phrase; got:\n%s", out)
	}
}

// TestStatus_TopicGroupAllDoneIconAndState pins issue 2 — when ALL
// topic rows have terminated, the parent line MUST flip to ✓ +
// "已完成证据收集", regardless of topic[0]'s individual state.
// Pre-fix the parent line's State was inherited from topic[0] only
// so a partially-done group could render "✓ 正在探索..." (done
// glyph + running text) when topic[0] terminated before others.
func TestStatus_TopicGroupAllDoneIconAndState(t *testing.T) {
	now := renderRowsFixedNow
	rows := []*taskRow{
		{isNodeRow: true, nodeID: "n1_evidence_t0", nodeKind: "evidence",
			objective: "topic A", endTime: now.Add(-10 * time.Second), okFinished: true},
		{isNodeRow: true, nodeID: "n2_evidence_t1", nodeKind: "evidence",
			objective: "topic B", endTime: now.Add(-5 * time.Second), okFinished: true},
	}
	out := renderRows(t, "zh", rows...)
	if !strings.Contains(out, "已完成证据收集") {
		t.Errorf("all-done topic group must show done primary text; got:\n%s", out)
	}
	if strings.Contains(out, "正在探索代码并收集证据") {
		t.Errorf("all-done topic group must NOT show running text; got:\n%s", out)
	}
}

// TestStatus_TopicGroupPartialDoneStaysRunning pins the inverse —
// when topic[0] terminated but other topics still run, parent line
// must read "running": running text + spinner glyph + light-blue
// primary colour (statusPrimary).
func TestStatus_TopicGroupPartialDoneStaysRunning(t *testing.T) {
	now := renderRowsFixedNow
	rows := []*taskRow{
		{isNodeRow: true, nodeID: "n1_evidence_t0", nodeKind: "evidence",
			objective: "topic A", endTime: now.Add(-10 * time.Second), okFinished: true},
		{isNodeRow: true, nodeID: "n2_evidence_t1", nodeKind: "evidence",
			objective: "topic B" /* no endTime → still running */},
	}
	out := renderRows(t, "zh", rows...)
	if !strings.Contains(out, "正在探索代码并收集证据") {
		t.Errorf("partial-done topic group must show running text; got:\n%s", out)
	}
	if strings.Contains(out, "已完成证据收集") {
		t.Errorf("partial-done topic group must NOT show done text; got:\n%s", out)
	}
}

// TestStatus_ThinkingSendingPhase pins the time-based "sending" vs
// "awaiting" split for bare-thinking rows. detailStart fresh (just
// fired EventAgentThinking) → "发送中" / "sending"; detailStart
// older than sendingThreshold → "等待响应" / "awaiting".
//
// The renderer's `now` parameter is fixed (renderRowsFixedNow) so
// the test's detailStart must be expressed relative to that anchor,
// not time.Now(), or the elapsed comparison reads as multi-hour and
// always falls past the threshold.
func TestStatus_ThinkingSendingPhase(t *testing.T) {
	freshRow := &taskRow{
		isNodeRow: true, nodeID: "vN", nodeKind: "validate",
		detail:      "thinking",
		// 100ms before the fixed now → ≪ sendingThreshold → "sending"
		detailStart: renderRowsFixedNow.Add(-100 * time.Millisecond),
	}
	zhFresh := renderRows(t, "zh", freshRow)
	if !strings.Contains(zhFresh, "发送中") {
		t.Errorf("zh fresh-thinking row must show 发送中; got:\n%s", zhFresh)
	}
	if strings.Contains(zhFresh, "等待响应") {
		t.Errorf("zh fresh-thinking row must NOT show 等待响应; got:\n%s", zhFresh)
	}
	enFresh := renderRows(t, "en", freshRow)
	if !strings.Contains(enFresh, "sending") {
		t.Errorf("en fresh-thinking row must show 'sending'; got:\n%s", enFresh)
	}
	if strings.Contains(enFresh, "awaiting") {
		t.Errorf("en fresh-thinking row must NOT show 'awaiting'; got:\n%s", enFresh)
	}

	staleRow := &taskRow{
		isNodeRow: true, nodeID: "vN", nodeKind: "validate",
		detail: "thinking",
		// 5s before the fixed now → ≫ sendingThreshold → "awaiting"
		detailStart: renderRowsFixedNow.Add(-5 * time.Second),
	}
	zhStale := renderRows(t, "zh", staleRow)
	if !strings.Contains(zhStale, "等待响应") {
		t.Errorf("zh stale-thinking row must show 等待响应; got:\n%s", zhStale)
	}
	if strings.Contains(zhStale, "发送中") {
		t.Errorf("zh stale-thinking row must NOT show 发送中; got:\n%s", zhStale)
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
