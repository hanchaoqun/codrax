package render

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// status_messages.go is the renderer's localization layer. Every
// user-facing status string in the spinner area passes through one
// of the friendly* helpers below so the surface text follows the
// configured lang code without any taskRow consumer needing to know
// zh from en.
//
// Design rules:
//   - Render package owns the strings. We intentionally do NOT
//     import internal/repl/messages.go to avoid a render → repl back
//     edge; the project-wide isZh logic is mirrored locally.
//   - All structural decisions (which stage label, which kind of
//     thinking summary, recoverable vs fatal classification) live
//     elsewhere; this file is a vocabulary.
//   - Light text-shaping (CJK ↔ ASCII spacing) lives in
//     normalizeTopicText so topic strings emitted by the analyzer
//     read naturally in either locale.

// isZh reports whether the given lang code maps to Chinese output.
// Empty defaults to zh (mirrors the project-wide isZh fallback used
// in cmd/root.go and internal/orchestrator). Any prefix matching
// "zh" is treated as Chinese; everything else falls through to en.
func isZh(lang string) bool {
	if lang == "" {
		return true
	}
	l := strings.ToLower(strings.TrimSpace(lang))
	return strings.HasPrefix(l, "zh")
}

// localizeRetryReason maps the short-phrase reason
// retryReasonForError emits inside the LLM adapter to the user's
// configured language. retryReasonForError lives in internal/llm
// (no render dep) and only knows English; mixing English jargon
// like "rate limit" or "stream first-byte timeout" into Chinese
// dock prose was the operator-reported gap. Unknown phrases fall
// through verbatim — better to surface an unrecognised label than
// silently drop the cause classification.
func localizeRetryReason(reason, lang string) string {
	if reason == "" {
		return ""
	}
	if !isZh(lang) {
		return reason
	}
	switch reason {
	case "transient":
		return "临时网络抖动"
	case "stream first-byte timeout":
		return "首字节超时"
	case "stream stalled":
		return "响应流中断"
	case "quota":
		// `quota` retry reason covers a body-marker bucket that is
		// mostly transient throttling (insufficient_quota /
		// rate_limit_error / 请求量较高 / 套餐 etc) — adapter retries
		// with a 4-minute long backoff. "配额已用尽" misled users
		// into thinking their billing quota was exhausted; in reality
		// the vendor is just throttling and the next retry usually
		// clears within the longer backoff window.
		return "被临时限流"
	case "rate limit":
		return "限流(请求过频)"
	}
	if strings.HasPrefix(reason, "server ") {
		return "服务端错误 (" + strings.TrimPrefix(reason, "server ") + ")"
	}
	return reason
}

// stagePhraseState is the lifecycle slot stagePhrase resolves
// against. Five values: running ("正在 ..."), done ("已 ..."),
// pending ("待 ..." — queued, has NOT started yet), retry
// ("X 出错,正在重试" — recoverable, orchestrator is retrying), and
// failed ("X 失败" — terminal error). Pending is load-bearing
// because two rows reading "正在 X" and "正在 Y" side by side
// mislead the user into thinking both are executing concurrently —
// but the TaskGraph wires upstream stages as hard dependencies, so
// a row's pending state needs an unambiguous lexical form (not just
// a colour cue) the user reads as "this hasn't started yet".
//
// stagePhraseFailed distinguishes terminal failure from terminal
// success. Pre-2026-04-30 the renderer collapsed both
// (endTime != 0) into stagePhraseDone, which meant a failed plan
// rendered as "已设计改动方案" alongside the ✗ failure glyph —
// icon and label contradicting each other. The stage-row contract
// is "icon and label must agree on lifecycle"; failed stages now
// carry their own dedicated phrasing so the contradiction is
// structurally impossible.
//
// stagePhraseRetry is the recoverable-error slot. Pre-this-fix the
// renderer borrowed stagePhraseFailed text ("未能理解问题") for any
// row that classifyStatusError flagged as Recoverable, which paired
// the ⟳ glyph (yellow, "still working / will retry") with a label
// that said "could not / failed". Same lifecycle-mismatch bug as
// the original failed-vs-done contradiction; same fix shape — give
// recoverable its own column so the label agrees with the glyph.
type stagePhraseState int

const (
	stagePhraseRunning stagePhraseState = iota
	stagePhraseDone
	stagePhrasePending
	stagePhraseFailed
	stagePhraseRetry
)

// stagePhrase returns the localized "primary" label for a normalized
// stage key. Stage keys are derived by friendlyPrimaryText from the
// taskRow's nodeKind / stage / agent fields. The state argument
// picks one of three lifecycle phrasings: running / done / pending.
//
// Phrasing follows the actual problem-solving narrative the user
// experiences:
//
//  1. log_triage / perf_triage (conditional pre-stages) parse the
//     attached runtime log or HiTrace dump.
//  2. analyze (StageAnalyze) understands the user's request and
//     classifies it. After this completes the request is
//     "understood"; deep analysis happens NEXT.
//  3. explore (StageExplore) is the deep-analysis phase. The task
//     graph fans out into per-topic NodeEvidence rows, optionally
//     followed by a NodeValidate that cross-checks the evidence
//     and a NodeReconcile that merges findings into a coherent
//     story.
//  4. extract (StageExtract) pulls the structured facts the
//     finalizer needs out of the explore transcript.
//  5. finalize (StageFinalize) renders the final answer prose.
//
// Write mode (when --mode=plan|apply|verify):
//  1. analyze still runs as a classifier.
//  2. plan (StagePlan) drafts a ChangePlan.
//  3. apply (StageApply) applies patches inside a worktree.
//  4. verify (StageVerify) runs tests to confirm no regression.
//
// Unknown keys fall through to a generic "正在处理任务" /
// "Processing task" so an unmapped stage never surfaces an internal
// label like "ExtractorAgent(extract)".
func stagePhrase(key string, lang string, state stagePhraseState) string {
	zh := isZh(lang)
	type quint struct{ run, done, pending, failed, retry string }
	// Label vocabulary discipline (post-2026-04-30 audit triggered
	// by user feedback "整理结论 / 校核分析结论 / 生成最终答案
	// 三个标签都含'结论/答案',读起来像三个收尾步骤"):
	//
	// The pipeline runs analyze → evidence → validate → reconcile
	// → extract → finalize. The user-facing label MUST distinguish
	// these as DIFFERENT semantic phases without recycling the
	// "结论 / 答案 / conclusion / answer" vocabulary across them —
	// otherwise the timeline reads as "wrap-up, wrap-up, wrap-up,
	// wrap-up". Vocabulary by phase:
	//
	//   evidence   → 探索代码并收集证据 / Exploring code, collecting evidence
	//   validate   → 交叉验证证据      / Cross-validating evidence
	//                    (was "校核分析结论" — no "结论" exists yet at this point)
	//   reconcile  → 归纳探索线索 (running) / 归纳探索结果 (done)
	//                Consolidating exploration threads → Findings consolidated.
	//                Running form uses "线索" (mid-process threads, not yet
	//                finalized) so the row does not read as a stage wrap-up
	//                while K=2/4 — the explore stage's K/N has not advanced
	//                yet. Done form switches to "结果" because by that point
	//                the consolidated output exists.
	//   extract    → 提炼关键发现      / Distilling key findings
	//                    (was "提取关键要点" — slightly tighter)
	//   finalize   → 撰写最终答案      / Composing the final answer
	//                    (was "生成最终答案" — same surface, just verb refresh)
	//
	// Now each phase carries a unique active verb (探索 / 验证 /
	// 归纳 / 提炼 / 撰写) so the user reads phase progression
	// without re-encountering "结论"/"答案" three rows in a row.
	//
	// The fourth "failed" column distinguishes terminal failure from
	// terminal success so a failed run never lies as "已 X". The
	// failure phrase parallels the success phrase one verb at a
	// time (解析→解析失败 / 设计→未生成 / 通过测试验证→未通过测试)
	// so reading down a column tells the user whether the stage
	// reached its goal without scanning glyphs.
	// Retry phrasing pairs verbatim with the failed phrase so the
	// surface text reads as "<X> 出错,正在重试" — the user sees the
	// exact same verb as the failed line (so the meaning is clear)
	// but the trailing "正在重试" / "retrying" tells them the
	// orchestrator has not given up. This is the column the ⟳ glyph
	// resolves against: glyph + label both point at "still working".
	// Retry-column phrasing (post-2026-05-06 audit): for LLM-dispatch
	// stages the recoverable retry is overwhelmingly a model
	// response/request issue (timeout, stream stall, gate-driven
	// rejection of the model output). Pre-fix the column read
	// "<stage>出错,正在重试" — which the user parses as "the stage's
	// business logic failed", because the verb "出错" attaches to the
	// stage gerund. New phrasing fronts "模型响应出错,正在重新…X" so
	// the recovery action and the cause-side ("model response") read
	// together, and the stage gerund describes what the retry is
	// trying to redo. Apply and verify keep tool-side framing — those
	// retries are git/patch and test-runner layers, not LLM.
	tableZh := map[string]quint{
		// Pre-stages
		"log_triage":  {"正在解析日志", "已解析日志", "待解析日志", "未能解析日志", "模型响应出错,正在重新解析日志"},
		"perf_triage": {"正在解析性能数据", "已解析性能数据", "待解析性能数据", "未能解析性能数据", "模型响应出错,正在重新解析性能数据"},
		// Read-mode core flow
		"analyze": {"正在理解问题", "已理解问题", "待理解问题", "未能理解问题", "模型响应出错,正在重新理解问题"},
		// "explore" is the orchestrator-level stage AND the topic-
		// group parent label when multiple sub-topics fan out. As a
		// single status line it reads as "正在深入分析" — the broad
		// umbrella over the per-topic evidence work.
		"explore": {"正在深入分析", "已完成深入分析", "待深入分析", "深入分析未完成", "模型响应出错,正在重新深入分析"},
		// "evidence" is the NodeEvidence sub-step where the agent
		// actually reads code (read_file / grep / repo_map) and
		// emits structured evidence.
		"evidence":  {"正在探索代码并收集证据", "已完成证据收集", "待探索代码并收集证据", "未收集到足够证据", "模型响应出错,正在重新收集证据"},
		"validate":  {"正在交叉验证证据", "已交叉验证证据", "待交叉验证证据", "交叉验证未完成", "模型响应出错,正在重新交叉验证"},
		"reconcile": {"正在归纳探索线索", "已归纳探索结果", "待归纳探索结果", "未能归纳结果", "模型响应出错,正在重新归纳证据"},
		"extract":   {"正在提炼关键发现", "已提炼关键发现", "待提炼关键发现", "未能提炼关键发现", "模型响应出错,正在重新提炼关键发现"},
		"finalize":  {"正在撰写最终答案", "已撰写最终答案", "待撰写最终答案", "未能生成最终答案", "模型响应出错,正在重新撰写答案"},
		// Write-mode flow.
		// plan failure: "未生成改动方案" — the model didn't produce a
		//   usable plan this round; reads as "no plan" not "plan
		//   failed", which is closer to the truth (the LLM may have
		//   produced prose / partial output / nothing at all).
		// apply failure: "应用改动失败" — patches did not all land.
		// verify failure: "测试未通过" — tests ran but failed; never
		//   "已通过测试验证改动" which previously rendered alongside
		//   the ✗ glyph (the bug that prompted this fix).
		// write_analyze is the write-mode pre-stage that classifies the
		// task into kind/scope/risk/constraints/outcomes (~3-10s LLM
		// dispatch). Falls between read-mode analyze and the plan node.
		// Pre-2026-05-01 it had no entry and rendered as the generic
		// "正在处理任务" / "Processing task" — visually invisible during
		// a real LLM call.
		"write_analyze": {"正在分析任务上下文", "已分析任务上下文", "待分析任务上下文", "未能分析任务上下文", "模型响应出错,正在重新分析任务上下文"},
		"plan":          {"正在设计改动方案", "已设计改动方案", "待设计改动方案", "未生成改动方案", "模型响应出错,正在重新设计改动方案"},
		// apply / verify retries originate from the git-patch layer and
		// the test-runner layer respectively — NOT from a model call,
		// so the retry phrasing keeps tool-side framing instead of the
		// "模型响应出错…" prefix.
		"apply":  {"正在应用改动", "已应用改动", "待应用改动", "未能应用改动", "应用改动出错,正在重试"},
		"verify": {"正在跑测试验证改动", "已通过测试验证改动", "待跑测试验证改动", "测试未通过", "测试验证出错,正在重试"},
	}
	// English retry mirrors the Chinese "模型响应出错,正在重新…X"
	// shape: "Model response error, re-running X" puts the
	// recovery + cause together, and the stage gerund describes
	// what is being redone. Apply / verify keep tool-side framing
	// (git-patch / test-runner layers, not LLM).
	tableEn := map[string]quint{
		"log_triage":    {"Parsing attached log", "Log parsed", "Awaiting log", "Could not parse log", "Model response error, re-parsing log"},
		"perf_triage":   {"Parsing performance trace", "Performance trace parsed", "Awaiting trace", "Could not parse trace", "Model response error, re-parsing trace"},
		"analyze":       {"Understanding the request", "Request understood", "Awaiting analysis", "Could not understand request", "Model response error, re-running request understanding"},
		"explore":       {"Investigating", "Investigation complete", "Awaiting investigation", "Investigation incomplete", "Model response error, re-running investigation"},
		"evidence":      {"Exploring code, collecting evidence", "Evidence collected", "Awaiting evidence", "Could not gather evidence", "Model response error, re-gathering evidence"},
		"validate":      {"Cross-validating evidence", "Evidence cross-validated", "Awaiting cross-validation", "Cross-validation incomplete", "Model response error, re-running cross-validation"},
		"reconcile":     {"Consolidating exploration threads", "Findings consolidated", "Awaiting consolidation", "Could not consolidate findings", "Model response error, re-consolidating findings"},
		"extract":       {"Distilling key findings", "Key findings distilled", "Awaiting key findings", "Could not distill findings", "Model response error, re-distilling key findings"},
		"finalize":      {"Composing the final answer", "Final answer composed", "Awaiting final answer", "Could not compose answer", "Model response error, re-composing the final answer"},
		"write_analyze": {"Analyzing task context", "Task context analyzed", "Awaiting task analysis", "Could not analyze task", "Model response error, re-analyzing task context"},
		"plan":          {"Drafting change plan", "Change plan ready", "Awaiting change plan", "No change plan produced", "Model response error, re-drafting change plan"},
		"apply":         {"Applying changes", "Changes applied", "Awaiting apply", "Apply incomplete", "Apply hit an error, retrying"},
		"verify":        {"Running tests", "Tests passed", "Awaiting verification", "Tests did not pass", "Test verification hit an error, retrying"},
	}
	var t quint
	var ok bool
	if zh {
		t, ok = tableZh[key]
	} else {
		t, ok = tableEn[key]
	}
	if !ok {
		return tCommonProcessing(zh, state)
	}
	switch state {
	case stagePhraseDone:
		return t.done
	case stagePhrasePending:
		return t.pending
	case stagePhraseFailed:
		return t.failed
	case stagePhraseRetry:
		return t.retry
	default:
		return t.run
	}
}

func tCommonProcessing(zh bool, state stagePhraseState) string {
	if zh {
		switch state {
		case stagePhraseDone:
			return "已完成任务"
		case stagePhrasePending:
			return "待处理任务"
		case stagePhraseFailed:
			return "任务未完成"
		case stagePhraseRetry:
			return "处理任务出错,正在重试"
		default:
			return "正在处理任务"
		}
	}
	switch state {
	case stagePhraseDone:
		return "Task complete"
	case stagePhrasePending:
		return "Awaiting task"
	case stagePhraseFailed:
		return "Task did not complete"
	case stagePhraseRetry:
		return "Task hit an error, retrying"
	default:
		return "Processing task"
	}
}

// thinkingPhrase localizes a row.detail string that begins with
// "thinking" / "► thinking" / "thinking (round N)" / "thinking: …".
// Returns the localized form OR an empty string when the input is
// not a thinking-shaped detail (caller falls through to other
// detail formatters).
//
// Three distinct phases collapse into the "thinking" detail prefix
// in the event stream; the user asked to see them separately so the
// long quiet windows in front of the spinner read as a specific
// waiting mode rather than an undifferentiated stall:
//
//  1. "发送中" / "sending" — bare "thinking" within `sendingThreshold`
//     of detailStart. EventAgentThinking just fired; the HTTP
//     request body is still being dispatched OR the server hasn't
//     even acknowledged yet. Brief by design (typically <1s).
//
//  2. "等待响应" / "awaiting" — bare "thinking" beyond the threshold.
//     Request is fully in flight; we are waiting on the server /
//     model to start producing tokens. This is the slow window
//     (5-30s on thinking models) where the user previously had no
//     signal that "the request was sent and we're just waiting".
//
//  3. "回复中：…" / "replying: …" — "thinking: …" with a preview
//     tail. EventAgentContent fired with the first response
//     chunk; the model IS actively streaming back tokens.
//
// Phrasing is kept short (3-5 chars zh, ≤ 8 chars en) per the user's
// "尽量简短，不易太长，又能区分" callout. The distinction is the
// load-bearing UX, not the verbosity.
var thinkingTextRe = regexp.MustCompile(`^(?:►\s*)?thinking[:：]\s*(.*)$`)
var thinkingBareRe = regexp.MustCompile(`^(?:►\s*)?thinking\s*$`)

// sendingThreshold splits "sending" from "awaiting". Empirically,
// HTTP request dispatch + initial server handshake completes well
// under 1s on healthy networks; beyond that the latency is
// server-side queue or model think-time, not local IO.
const sendingThreshold = 1500 * time.Millisecond

func thinkingPhrase(detail string, lang string, now, detailStart time.Time) string {
	zh := isZh(lang)
	d := strings.TrimSpace(detail)
	// "thinking: …" — first content chunk arrived; model is replying.
	if m := thinkingTextRe.FindStringSubmatch(d); m != nil {
		body := strings.TrimSpace(m[1])
		if zh {
			return "回复中：" + body
		}
		return "replying: " + body
	}
	// "thinking" — bare; split into "sending" vs "awaiting" by elapsed.
	// Round number lives ONLY in meta now, not detail — single source
	// of truth eliminates the "第 2 轮 / 第 1 轮" duplication that
	// pre-2026-04-30 had detail showing N+1 and meta showing N for
	// the same logical round.
	if thinkingBareRe.MatchString(d) {
		return bareThinkingPhase(now, detailStart, zh)
	}
	return ""
}

// bareThinkingPhase returns the localized "sending" vs "awaiting"
// phrase for a bare-thinking row, splitting on detailStart age. When
// detailStart is zero (test path, or row constructed without an
// event timestamp), default to "awaiting" so we don't claim "sending"
// for stale rows.
//
// NTP-jump guard: if the wall clock moves backward between the time
// detailStart was captured and `now` is read (rare but observed in
// systems whose monotonic clock disagrees with the real-time clock,
// or when the runner re-synchronises on boot), now.Sub(detailStart)
// can be negative — strictly less than sendingThreshold but not in
// the "fresh" semantic sense. We require a non-negative elapsed
// before claiming "sending" so a backward-jumped clock never lies
// about request freshness.
func bareThinkingPhase(now, detailStart time.Time, zh bool) string {
	if !detailStart.IsZero() {
		elapsed := now.Sub(detailStart)
		if elapsed >= 0 && elapsed < sendingThreshold {
			if zh {
				return "发送中"
			}
			return "sending"
		}
	}
	if zh {
		return "等待响应"
	}
	return "awaiting"
}

// toolDetailPhrase localizes a tool-call detail like "grep main.go" /
// "read internal/render/renderer.go" / "emit_answer_document". The
// argument tail (everything after the first whitespace) is preserved
// verbatim — file paths, package names, search patterns must NOT be
// translated.
//
// Returns "" when input doesn't look like a tool call, so the caller
// can fall through to the raw detail text.
func toolDetailPhrase(detail string, lang string) string {
	zh := isZh(lang)
	d := strings.TrimSpace(detail)
	if d == "" {
		return ""
	}
	// Split on first whitespace: tool name + tail (path/args).
	tool := d
	tail := ""
	if i := strings.IndexAny(d, " \t"); i > 0 {
		tool = d[:i]
		tail = strings.TrimSpace(d[i+1:])
	}
	type spec struct{ zh, en string }
	table := map[string]spec{
		"grep":                 {"正在搜索", "searching"},
		"search":               {"正在搜索", "searching"},
		"read":                 {"正在读取", "reading"},
		"read_file":            {"正在读取", "reading"},
		"list_files":           {"正在查看文件列表", "listing files"},
		"repo_map":             {"正在浏览仓库结构", "browsing repo structure"},
		"apply_patch":          {"正在应用改动", "applying changes"},
		"exec_command":         {"正在执行命令", "running command"},
		"run_tests":            {"正在运行测试", "running tests"},
		"emit_analysis":        {"正在生成分析结果", "generating analysis result"},
		"emit_answer_document": {"正在撰写最终答案", "composing the final answer"},
		"emit_change_plan":     {"正在生成改动计划", "generating change plan"},
		"emit_plan_skeleton":   {"正在生成计划骨架", "drafting plan skeleton"},
		"emit_plan_change":     {"正在补全计划文件", "filling plan file"},
		"emit_evidence":        {"正在记录证据", "recording evidence"},
		"emit_answer_symbol":   {"正在生成答案要点", "extracting answer symbols"},
		"emit_test_results":    {"正在汇总测试结果", "summarizing test results"},
	}
	if s, ok := table[tool]; ok {
		var verb string
		if zh {
			verb = s.zh
		} else {
			verb = s.en
		}
		if tail == "" {
			return verb
		}
		return verb + " " + tail
	}
	// Unknown tool — generic phrasing with the tool name kept as the
	// technical entity (NOT translated).
	if !looksLikeToolCall(tool) {
		return ""
	}
	if zh {
		if tail == "" {
			return "正在调用工具 " + tool
		}
		return "正在调用工具 " + tool + " " + tail
	}
	if tail == "" {
		return "calling tool " + tool
	}
	return "calling tool " + tool + " " + tail
}

// looksLikeToolCall is a conservative guard: the detail must look
// like an identifier / underscored token that could be a tool name,
// not a free-form sentence. Empty whitespace or punctuation-heavy
// text returns false so the caller falls through to raw display.
var toolNameLike = regexp.MustCompile(`^[a-z][a-z0-9_]+$`)

func looksLikeToolCall(s string) bool {
	return toolNameLike.MatchString(strings.ToLower(s))
}

// topicCountPhrase produces "拆分为 N 个调查单元" / "N investigation
// units" — the secondary line below the parent stage when topic rows are
// aggregated. "关注点 / focus area" was ambiguous: users read "2/2" as
// completion, while the value only identifies the active analyzer-created
// investigation unit.
func topicCountPhrase(n int, lang string) string {
	if isZh(lang) {
		return fmt.Sprintf("拆分为 %d 个调查单元", n)
	}
	if n == 1 {
		return "1 investigation unit"
	}
	return fmt.Sprintf("%d investigation units", n)
}

// topicLabelPhrase produces the per-row "调查单元 N：" / "Unit N:"
// prefix shown next to the topic body text.
func topicLabelPhrase(idx int, lang string) string {
	if isZh(lang) {
		return fmt.Sprintf("调查单元 %d：", idx)
	}
	return fmt.Sprintf("Unit %d: ", idx)
}

// topicProgressPhrase produces the compact ordinal segment used in the
// dock stage row and commit lines. It describes which focus is currently
// being handled, not how much of that focus has completed.
func topicProgressPhrase(idxZero int, total int, lang string) string {
	if total < 2 || idxZero < 0 {
		return ""
	}
	idx := idxZero + 1
	if isZh(lang) {
		return fmt.Sprintf("第 %d 个调查单元，共 %d 个", idx, total)
	}
	return fmt.Sprintf("unit %d of %d", idx, total)
}

func parallelActivityPhrase(p *parallelActivitySnapshot, lang string) string {
	if p == nil {
		if isZh(lang) {
			return "并行探索中"
		}
		return "Parallel exploration"
	}
	allActive := p.activeUnits > 0
	if allActive && p.retrying == p.activeUnits {
		if isZh(lang) {
			return "并行重新请求模型中"
		}
		return "Retrying parallel model requests"
	}
	if allActive && p.requesting == p.activeUnits {
		if isZh(lang) {
			return "并行请求模型中"
		}
		return "Calling models in parallel"
	}
	if allActive && p.receiving == p.activeUnits {
		if isZh(lang) {
			return "并行接收模型输出"
		}
		return "Receiving parallel model output"
	}
	if allActive && p.tool == p.activeUnits {
		if isZh(lang) {
			return "并行调用工具中"
		}
		return "Calling tools in parallel"
	}
	if isZh(lang) {
		return "并行探索中"
	}
	return "Parallel exploration"
}

func parallelActivitySegments(p *parallelActivitySnapshot, lang string) []string {
	if p == nil {
		return nil
	}
	var parts []string
	if p.total > 0 && p.doneUnits > 0 && p.doneUnits < p.total {
		if isZh(lang) {
			parts = append(parts, fmt.Sprintf("%d/%d 已完成", p.doneUnits, p.total))
		} else {
			parts = append(parts, fmt.Sprintf("%d/%d done", p.doneUnits, p.total))
		}
	}
	if p.activeUnits > 0 {
		allOneKind := p.requesting == p.activeUnits ||
			p.receiving == p.activeUnits ||
			p.tool == p.activeUnits ||
			p.retrying == p.activeUnits
		if isZh(lang) {
			if allOneKind && p.doneUnits == 0 {
				parts = append(parts, fmt.Sprintf("%d 路", p.activeUnits))
			} else {
				parts = append(parts, fmt.Sprintf("%d 路活跃", p.activeUnits))
			}
		} else if allOneKind && p.doneUnits == 0 {
			parts = append(parts, fmt.Sprintf("%d lanes", p.activeUnits))
		} else {
			parts = append(parts, fmt.Sprintf("%d active lanes", p.activeUnits))
		}
	}
	nonZeroKinds := 0
	for _, n := range []int{p.requesting, p.receiving, p.tool, p.retrying} {
		if n > 0 {
			nonZeroKinds++
		}
	}
	if nonZeroKinds > 1 {
		if p.requesting > 0 {
			if isZh(lang) {
				parts = append(parts, fmt.Sprintf("请求 %d", p.requesting))
			} else {
				parts = append(parts, fmt.Sprintf("%d requesting", p.requesting))
			}
		}
		if p.receiving > 0 {
			if isZh(lang) {
				parts = append(parts, fmt.Sprintf("接收 %d", p.receiving))
			} else {
				parts = append(parts, fmt.Sprintf("%d receiving", p.receiving))
			}
		}
		if p.tool > 0 {
			if isZh(lang) {
				parts = append(parts, fmt.Sprintf("工具 %d", p.tool))
			} else {
				parts = append(parts, fmt.Sprintf("%d in tools", p.tool))
			}
		}
		if p.retrying > 0 {
			if isZh(lang) {
				parts = append(parts, fmt.Sprintf("重试 %d", p.retrying))
			} else {
				parts = append(parts, fmt.Sprintf("%d retrying", p.retrying))
			}
		}
	}
	return parts
}

func parallelStagePhrase(p *parallelActivitySnapshot, lang string) string {
	if p == nil || p.parallelism <= 1 {
		return ""
	}
	if isZh(lang) {
		return fmt.Sprintf("并行 %d 路", p.parallelism)
	}
	return fmt.Sprintf("%d-way parallel", p.parallelism)
}

func parallelFocusTotalPhrase(total int, lang string) string {
	if total < 2 {
		return ""
	}
	if isZh(lang) {
		return fmt.Sprintf("%d 个调查单元", total)
	}
	if total == 1 {
		return "1 investigation unit"
	}
	return fmt.Sprintf("%d investigation units", total)
}

func parallelLaneLabelsPhrase(labels []string, lang string) string {
	if len(labels) == 0 {
		return ""
	}
	const maxVisible = 3
	total := len(labels)
	visibleCap := total
	if visibleCap > maxVisible {
		visibleCap = maxVisible
	}
	visible := make([]string, 0, visibleCap)
	for i, label := range labels {
		if i >= maxVisible {
			break
		}
		visible = append(visible, localizeParallelLaneLabel(label, lang))
	}
	if len(visible) == 0 {
		return ""
	}
	if isZh(lang) {
		text := strings.Join(visible, "、")
		if total > maxVisible {
			return fmt.Sprintf("证据通道：%s等 %d 类", text, total)
		}
		return "证据通道：" + text
	}
	text := strings.Join(visible, ", ")
	if total > maxVisible {
		return fmt.Sprintf("evidence lanes: %s, +%d", text, total-maxVisible)
	}
	return "evidence lanes: " + text
}

func parallelLaneLabelsInlinePhrase(labels []string, lang string) string {
	if len(labels) == 0 {
		return ""
	}
	const maxVisible = 2
	total := len(labels)
	visibleCap := total
	if visibleCap > maxVisible {
		visibleCap = maxVisible
	}
	visible := make([]string, 0, visibleCap)
	for i, label := range labels {
		if i >= maxVisible {
			break
		}
		visible = append(visible, localizeParallelLaneLabel(label, lang))
	}
	if len(visible) == 0 {
		return ""
	}
	if isZh(lang) {
		text := strings.Join(visible, "、")
		if total > maxVisible {
			return fmt.Sprintf("%s等 %d 类", text, total)
		}
		return text
	}
	text := strings.Join(visible, ", ")
	if total > maxVisible {
		return fmt.Sprintf("%s, +%d", text, total-maxVisible)
	}
	return text
}

func localizeParallelLaneLabel(label string, lang string) string {
	key := strings.ToLower(strings.TrimSpace(label))
	if isZh(lang) {
		switch key {
		case "current_source":
			return "当前源码"
		case "vcs_history", "vcs_metadata":
			return "历史记录"
		case "vcs_diff":
			return "历史差异"
		case "runtime_artifact":
			return "日志/Trace"
		case "command_measurement":
			return "命令结果"
		case "negative_search":
			return "未命中搜索"
		case "cross_repo_index":
			return "跨仓索引"
		case "external_document":
			return "外部文档"
		case "web":
			return "网页"
		case "mcp":
			return "MCP"
		case "connector":
			return "连接器"
		}
	}
	switch key {
	case "current_source":
		return "current source"
	case "vcs_history", "vcs_metadata":
		return "VCS history"
	case "vcs_diff":
		return "VCS diff"
	case "runtime_artifact":
		return "log/trace"
	case "command_measurement":
		return "command output"
	case "negative_search":
		return "negative search"
	case "cross_repo_index":
		return "cross-repo index"
	case "external_document":
		return "external document"
	case "web":
		return "web"
	case "mcp":
		return "MCP"
	case "connector":
		return "connector"
	}
	if label = strings.TrimSpace(label); label != "" {
		return label
	}
	return key
}

// topicOverflowPhrase covers the >5 topics case: first 5 are shown
// individually, the rest collapse into a single line.
func topicOverflowPhrase(extra int, lang string) string {
	if isZh(lang) {
		return fmt.Sprintf("另有 %d 个调查单元，已合并到后续分析中", extra)
	}
	if extra == 1 {
		return "1 more investigation unit merged into the follow-up analysis"
	}
	return fmt.Sprintf("%d more investigation units merged into the follow-up analysis", extra)
}

// metaToolCountPhrase produces the dim "5 次工具调用" / "5 tool
// calls" trailer used on completed status lines.
func metaToolCountPhrase(n int, lang string) string {
	if isZh(lang) {
		return fmt.Sprintf("%d 次工具调用", n)
	}
	if n == 1 {
		return "1 tool call"
	}
	return fmt.Sprintf("%d tool calls", n)
}

// metaRoundPhrase produces "第 N 轮" / "round N" for the iteration
// trailer.
func metaRoundPhrase(n int, lang string) string {
	if isZh(lang) {
		return fmt.Sprintf("第 %d 轮", n)
	}
	return fmt.Sprintf("round %d", n)
}

func metaModelPhrase(modelID string, lang string) string {
	modelID = compactModelID(modelID, 36)
	if modelID == "" {
		return ""
	}
	if isZh(lang) {
		return "模型 " + modelID
	}
	return "model " + modelID
}

func metaContextTokensPhrase(used, window int, lang string) string {
	if used <= 0 {
		return ""
	}
	usedText := compactTokenCount(used)
	if window > 0 {
		usedText += "/" + compactTokenCount(window)
	}
	if isZh(lang) {
		return "约 " + usedText + " tok"
	}
	return "~" + usedText + " tok"
}

func compactModelID(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	if s == "" || maxRunes <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	if maxRunes <= 6 {
		return string(runes[:maxRunes])
	}
	head := (maxRunes - 3) / 2
	tail := maxRunes - 3 - head
	return string(runes[:head]) + "..." + string(runes[len(runes)-tail:])
}

func compactTokenCount(n int) string {
	switch {
	case n < 1000:
		return fmt.Sprintf("%d", n)
	case n < 10000:
		return trimCompactDecimal(fmt.Sprintf("%.1fk", float64(n)/1000))
	case n < 1000000:
		return fmt.Sprintf("%dk", (n+500)/1000)
	case n < 10000000:
		return trimCompactDecimal(fmt.Sprintf("%.1fM", float64(n)/1000000))
	default:
		return fmt.Sprintf("%dM", (n+500000)/1000000)
	}
}

func trimCompactDecimal(s string) string {
	s = strings.TrimSuffix(s, "0")
	return strings.TrimSuffix(s, ".")
}

// footerPhrase returns the spinner-area footer ("总耗时 1m18s" /
// "Total elapsed 1m18s"). The elapsed string is the caller's job
// (formatted via time.Duration.String).
func footerPhrase(elapsed string, lang string) string {
	if isZh(lang) {
		return "总耗时 " + elapsed
	}
	return "Total elapsed " + elapsed
}

// defaultCancelHint produces the bilingual cancel-hint surfaced in
// the footer when no caller-supplied hint is set. Used by the REPL
// after SetLang installs the user's locale.
func defaultCancelHint(lang string) string {
	if isZh(lang) {
		return "按 Ctrl+C 取消，连续按两次强制退出"
	}
	return "Press Ctrl+C to cancel, press twice to force quit"
}

// normalizeTopicText cleans up an analyzer-emitted objective string
// for display under "调查单元 N：". Two transforms:
//
//  1. Collapse runs of whitespace to single spaces.
//  2. zh locale: insert a space between adjacent CJK and ASCII
//     letters/digits so "analyzers包" reads as "analyzers 包" —
//     the analyzer's prose generation skips the canonical CJK
//     typography rule and the result reads cramped.
func normalizeTopicText(s string, lang string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Collapse runs of any whitespace (spaces/tabs/newlines) to one.
	s = whitespaceRunRe.ReplaceAllString(s, " ")
	if !isZh(lang) {
		return s
	}
	// CJK ↔ ASCII boundary spacing. The two regexes inject a space
	// only at boundaries that have neither side already a space, so
	// idempotent on well-typed input.
	s = cjkAsciiBoundaryRe1.ReplaceAllString(s, "$1 $2")
	s = cjkAsciiBoundaryRe2.ReplaceAllString(s, "$1 $2")
	return s
}

var (
	whitespaceRunRe     = regexp.MustCompile(`\s+`)
	cjkAsciiBoundaryRe1 = regexp.MustCompile(`([\p{Han}\p{Hiragana}\p{Katakana}\p{Hangul}])([A-Za-z0-9])`)
	cjkAsciiBoundaryRe2 = regexp.MustCompile(`([A-Za-z0-9])([\p{Han}\p{Hiragana}\p{Katakana}\p{Hangul}])`)
)

// stripInternalTagPrefix removes the "[topic 1] " / "[validate] "
// style prefix that the legacy formatTaskLine emitted, in case a
// pre-localized string is fed back through the new pipeline. Used
// as a defensive last step before display.
var internalTagPrefixRe = regexp.MustCompile(`^\[\w[\w\s]*\]\s+`)

func stripInternalTagPrefix(s string) string {
	return internalTagPrefixRe.ReplaceAllString(s, "")
}
