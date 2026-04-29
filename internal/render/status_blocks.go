package render

import (
	"fmt"
	"strings"
	"time"

	"github.com/pterm/pterm"
)

// statusBlock is one logical unit in the spinner area. Two shapes:
//
//   - Kind="line" — a single status row, primary text + detail + meta
//   - Kind="topic_group" — a parent stage line ("Understanding the
//     request") followed by an indented list of focus areas
//
// All taskRow → screen rendering goes through buildStatusBlocks +
// renderStatusBlock so taskRow consumers (event handlers) can stay
// purely structural; visual decisions live here.
type statusBlock struct {
	Kind   string // "line" or "topic_group"
	Line   statusLine
	Topics []statusTopic
}

type statusLine struct {
	Icon        string          // glyph or animated frame
	IconStyle   *pterm.Style    // colour for the glyph slot
	PrimaryText string          // localized stage label
	DetailText  string          // localized thinking / tool / recoverable detail
	MetaText    string          // dim trailer: elapsed + tool count + round
	State       string          // "running" / "done" / "failed" / "pending" / "recoverable"
	ErrorKind   statusErrorKind // statusErrorNone unless an error surfaced
}

type statusTopic struct {
	Index int
	Text  string
}

// buildStatusBlocks is the load-bearing transform: takes the live
// taskRow slice + spinner frame + clock and emits the renderable
// block list. Three responsibilities:
//
//   1. Drop internal rows the user shouldn't see (ungrouped topic
//      shells, stale "[topic N]" markers).
//   2. Aggregate evidence-with-_tN-suffix rows into one topic_group
//      block, parented under the localized "Understanding the
//      request" line.
//   3. Emit one line block per remaining row, with primary/detail/
//      meta strings already localized + error-classified.
//
// frame is the current spinner glyph (caller picks from
// spinnerFrames). now is the wall-clock used for "running elapsed"
// calculations — passed in so tests can pin a deterministic value.
func (r *Renderer) buildStatusBlocks(rows []*taskRow, frame string, now time.Time) []statusBlock {
	var blocks []statusBlock
	var topicRows []*taskRow
	// Insertion index for the topic group in `blocks` — must be
	// the position immediately AFTER the analyze stage block so the
	// timeline reads in pipeline order:
	//
	//   ✓ 已理解问题 (analyze)
	//   ⠙ 正在探索代码并收集证据 (topic group, with focus bullets)
	//   · 正在校核分析结论 (validate)
	//   ...
	//
	// Pre-2026-04-30 the group was UNCONDITIONALLY prepended to
	// blocks[0]; that put "正在探索 …" ABOVE the analyze row, which
	// the user read as "topic group is missing" because the eye
	// scans down from "已理解问题" and lands on the next stage
	// (校核分析结论) without registering the dim pending evidence
	// parent line above. Anchoring on "first non-pre-stage block"
	// places the group in its chronological slot in the timeline.
	insertAt := 0
	insertSet := false
	preStageKeys := map[string]bool{
		"log_triage":  true,
		"perf_triage": true,
		"analyze":     true,
	}

	for _, row := range rows {
		if row.isNodeRow && row.nodeKind == "evidence" {
			if _, ok := topicIndexFromNodeID(row.nodeID); ok {
				topicRows = append(topicRows, row)
				continue
			}
		}
		blocks = append(blocks, statusBlock{
			Kind: "line",
			Line: r.buildStatusLine(row, frame, now),
		})
		// Track the boundary: the topic group's anchor index is the
		// FIRST block whose stage key is NOT a pre-stage (log_triage
		// / perf_triage / analyze). Once we cross that boundary, the
		// remaining blocks (validate / reconcile / extract / finalize
		// / plan / apply / verify) are downstream of evidence and
		// the group must precede them.
		if !insertSet {
			key := stageKeyFor(row)
			if !preStageKeys[key] {
				insertAt = len(blocks) - 1
				insertSet = true
			}
		}
	}

	if len(topicRows) > 0 {
		group := r.buildTopicGroup(topicRows, frame, now)
		if !insertSet {
			// No downstream stages have surfaced yet — append the
			// group at the end so it sits below analyze rather than
			// above it.
			blocks = append(blocks, group)
		} else {
			blocks = append(blocks[:insertAt], append([]statusBlock{group}, blocks[insertAt:]...)...)
		}
	}
	return blocks
}

// buildStatusLine maps one taskRow into a localized statusLine.
// Three sub-decisions:
//
//   - icon + state: derived from row terminal state + classifyStatusError
//   - primary text: stage key from nodeKind / stage / agent
//   - detail text: thinking / tool / recoverable phrase
//   - meta text: elapsed + tool count + round
func (r *Renderer) buildStatusLine(row *taskRow, frame string, now time.Time) statusLine {
	errKind := classifyStatusError(row)
	icon, iconStyle, state := r.statusIcon(row, frame, errKind)
	return statusLine{
		Icon:        icon,
		IconStyle:   iconStyle,
		PrimaryText: friendlyPrimaryText(row, r.lang),
		DetailText:  friendlyDetailText(row, errKind, r.lang, now),
		MetaText:    friendlyMetaText(row, now, r.lang),
		State:       state,
		ErrorKind:   errKind,
	}
}

// buildTopicGroup folds a slice of topic rows into a single block
// whose Line is the parent stage ("Understanding the request") and
// whose Topics list is the per-row focus-area entries.
//
// Caps: when more than 6 topics are present, the first 5 surface
// as visible rows and the rest collapse into one " · N more focus
// areas merged into the follow-up analysis" sentinel. This protects
// the spinner area from overflowing when the analyzer emits a
// long sub-topic list.
func (r *Renderer) buildTopicGroup(topicRows []*taskRow, frame string, now time.Time) statusBlock {
	// Topic group's parent line reads as the EVIDENCE substep —
	// each topic row is a per-sub-topic NodeEvidence
	// (compiler.expandEvidenceNodes splits explore into N evidence
	// nodes when multiple sub-topics are present). Using the
	// "evidence" key surfaces the substantive label "正在探索代码
	// 并收集证据 / Exploring code, collecting evidence" that tells
	// the user this is the read_file / grep / repo_map phase.
	//
	// Pre-2026-04-30 the parent line used "explore" → "正在深入
	// 分析" as an artificial umbrella layer; the user reported the
	// "探索代码" wording never reached the UI because production
	// runs are predominantly multi-topic and individual evidence
	// rows fold into the bullet list. The substantive label only
	// rendered for single-topic flow which is rare. Switching the
	// parent to the evidence key makes the label visible in the
	// common case.
	parentRow := topicRows[0] // any topic row carries the stage; index 0 is the canonical anchor
	parentLine := r.buildStatusLine(parentRow, frame, now)
	// Group state aggregates topic rows along three axes:
	//   - allDone: every topic row terminated → state=done
	//   - allPending: not a single topic row started yet → state=pending
	//   - otherwise: at least one running → state=running
	// The pending case is load-bearing — without it a topic group
	// queued behind upstream nodes would render "正在探索代码 …"
	// while upstream is still running, misleading the user into
	// reading the timeline as concurrent execution.
	allDone := true
	allPending := true
	for _, row := range topicRows {
		if row.endTime.IsZero() {
			allDone = false
		}
		if !row.pending {
			allPending = false
		}
	}
	state := stagePhraseRunning
	switch {
	case allDone:
		state = stagePhraseDone
	case allPending:
		state = stagePhrasePending
	}
	parentLine.PrimaryText = stagePhrase("evidence", r.lang, state)
	count := len(topicRows)
	parentLine.DetailText = topicCountPhrase(count, r.lang)
	// Override Icon / IconStyle / State to follow the aggregated
	// state, not topicRows[0] alone. Three branches keep glyph +
	// primary text + primary colour in lockstep:
	//   - allDone   → ✓ green + done text + gray primary
	//   - allPending → · dim + pending text + dark-gray primary
	//   - else      → spinner glyph + running text + light-blue primary
	switch {
	case allDone:
		parentLine.Icon = string(glyphSuccess)
		parentLine.IconStyle = statusSuccessMuted
		parentLine.State = "done"
	case allPending:
		parentLine.Icon = string(glyphPending)
		parentLine.IconStyle = statusMeta
		parentLine.State = "pending"
	default:
		parentLine.Icon = frame
		parentLine.IconStyle = statusSpinner
		parentLine.State = "running"
	}

	const visibleCap = 5
	visible := topicRows
	overflow := 0
	if count > visibleCap+1 { // keep all when only 1 over the cap
		overflow = count - visibleCap
		visible = topicRows[:visibleCap]
	}

	topics := make([]statusTopic, 0, len(visible)+1)
	for i, row := range visible {
		idx := i + 1
		if n, ok := topicIndexFromNodeID(row.nodeID); ok {
			idx = n + 1
		}
		topics = append(topics, statusTopic{
			Index: idx,
			Text:  normalizeTopicText(row.objective, r.lang),
		})
	}
	if overflow > 0 {
		topics = append(topics, statusTopic{
			Index: 0, // 0 sentinel: render renders this as the overflow line
			Text:  topicOverflowPhrase(overflow, r.lang),
		})
	}
	return statusBlock{
		Kind:   "topic_group",
		Line:   parentLine,
		Topics: topics,
	}
}

// statusIcon picks the glyph + colour + state-string for a row.
// Four cases drive the result:
//
//   1. fatal error → ✗ in muted red, state="failed"
//   2. recoverable error → ⟳ in muted amber, state="recoverable"
//   3. cancelled → ✗ in muted red, state="cancelled"
//   4. row terminated cleanly → ✓ muted green, state="done"
//   5. row pending (planned, not run) → · dim grey, state="pending"
//   6. row running → animated spinner frame in dim grey, state="running"
func (r *Renderer) statusIcon(row *taskRow, frame string, errKind statusErrorKind) (string, *pterm.Style, string) {
	switch errKind {
	case statusErrorFatal:
		return string(glyphFatal), statusFatal, "failed"
	case statusErrorCancelled:
		return string(glyphFatal), statusFatal, "cancelled"
	case statusErrorRecoverable:
		return string(glyphRecoverable), statusRecoverable, "recoverable"
	}
	switch {
	case row.isNodeRow && row.pending:
		return string(glyphPending), statusMeta, "pending"
	case !row.endTime.IsZero() && row.okFinished:
		return string(glyphSuccess), statusSuccessMuted, "done"
	case !row.endTime.IsZero() && !row.okFinished:
		return string(glyphFatal), statusFatal, "failed"
	default:
		return frame, statusSpinner, "running"
	}
}

// friendlyPrimaryText derives the localized stage label for a row.
// The stage-key resolver mirrors the priority order the spec calls
// out: nodeKind > stage > agent > generic fallback.
//
// Three lifecycle phrasings: pending ("待 X" — queued, has not
// started), running ("正在 X" — currently executing), done
// ("已 X" — terminated). The pending case is critical so two
// rows of "正在 X" / "正在 Y" can never read as concurrent
// execution when one is actually queued behind the other.
func friendlyPrimaryText(row *taskRow, lang string) string {
	if row == nil {
		return ""
	}
	if row.agent == "" && row.stage != "" && !row.isNodeRow {
		// Collapsed marker row ("+N earlier done"). Localize the
		// "earlier done" English fragment so zh users see Chinese.
		return localizeCollapsedMarker(row.stage, lang)
	}
	key := stageKeyFor(row)
	state := stagePhraseRunning
	switch {
	case row.isNodeRow && row.pending:
		state = stagePhrasePending
	case !row.endTime.IsZero():
		state = stagePhraseDone
	}
	return stagePhrase(key, lang, state)
}

// stageKeyFor extracts a normalized stage key from a taskRow's
// structural fields. Priority: nodeKind > stage > agent (lowered
// to the corresponding stage). Returns "" for unmappable rows so
// stagePhrase falls through to its generic phrasing.
func stageKeyFor(row *taskRow) string {
	if row == nil {
		return ""
	}
	if row.isNodeRow && row.nodeKind != "" {
		return canonicalStageKey(row.nodeKind)
	}
	if row.stage != "" {
		return canonicalStageKey(row.stage)
	}
	if row.agent != "" {
		return canonicalStageKey(strings.TrimSuffix(strings.ToLower(row.agent), "agent"))
	}
	return ""
}

// canonicalStageKey collapses orchestrator-internal aliases to the
// canonical stage label keys that stagePhrase knows about. Mappings
// cover both PipelineStage values (StageAnalyze == "analyze",
// StageLogTriage == "log_triage", etc.), TaskNodeType values
// (NodeEvidence == "evidence", NodeValidate == "validate", etc.)
// and AgentName-derived strings (the trailing "agent" suffix is
// stripped before this function is called).
func canonicalStageKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "log_triage", "logtriage", "log_triager":
		return "log_triage"
	case "perf_triage", "perftriage", "perf_triager":
		return "perf_triage"
	case "analyzer", "analyze":
		return "analyze"
	case "explorer", "explore", "search":
		return "explore"
	case "extractor", "extract":
		return "extract"
	// Evidence is the load-bearing sub-step where the agent reads
	// code (read_file / grep / repo_map) and emits structured
	// evidence. Distinct key from "explore" so the row label tells
	// the user THIS is the code-exploration phase, not the
	// abstract "investigating" umbrella. The topic-group parent
	// line still uses "explore" — it's the umbrella over multiple
	// per-topic evidence rows.
	case "evidence", "ground", "rank", "dedupe", "collect":
		return "evidence"
	// NodeValidate cross-checks the explore phase's evidence
	// findings — distinct from StageVerify (which verifies write-
	// mode test runs). Different word; do NOT collapse them.
	case "validate", "validator":
		return "validate"
	case "reconcile", "summarize", "summary", "summariser":
		return "reconcile"
	case "finalize", "finaliser", "finalizer", "render":
		return "finalize"
	// Write-mode stages: distinct user surface from validate.
	case "planner", "plan":
		return "plan"
	case "coder", "apply":
		return "apply"
	case "verify", "verifier":
		return "verify"
	}
	return s
}

func localizeCollapsedMarker(s string, lang string) string {
	zh := isZh(lang)
	if zh {
		return strings.ReplaceAll(s, "earlier done", "条已完成")
	}
	return s
}

// friendlyDetailText returns the localized DetailText payload for a
// row's status line. Priority:
//
//   1. recoverable error → recoverableDetailPhrase
//   2. fatal error       → fatalDetailPhrase
//   3. cancelled         → cancelledDetailPhrase
//   4. thinking shape    → thinkingPhrase
//   5. tool-call shape   → toolDetailPhrase
//   6. anything else     → sanitized raw detail (truncated by caller)
//
// Completion rule: when the row has TERMINATED (endTime != 0) AND
// is not an error case, the live "thinking / round N / current
// tool" detail is intentionally suppressed. The completed row's
// meta segment (elapsed + iteration + tool count) carries enough
// information; surfacing the last in-flight thinking sentence
// after the row is done would mislead the user into thinking the
// stage is still working on it. Live (still-running) rows keep
// their detail as the "what is happening right now" signal.
func friendlyDetailText(row *taskRow, errKind statusErrorKind, lang string, now time.Time) string {
	if row == nil {
		return ""
	}
	switch errKind {
	case statusErrorRecoverable:
		if p := recoverableDetailPhrase(row, lang); p != "" {
			return p
		}
		// Fall through to raw detail when no specific recoverable
		// phrase fires, but ALWAYS strip any internal "[tag] " head.
		return stripInternalTagPrefix(row.detail)
	case statusErrorFatal:
		if p := fatalDetailPhrase(row, lang); p != "" {
			return p
		}
		return stripInternalTagPrefix(row.errorMsg)
	case statusErrorCancelled:
		return cancelledDetailPhrase(lang)
	}
	// Terminated cleanly: drop the stale live-progress detail. Only
	// the meta trailer (elapsed / round / tool count) survives. The
	// detail kept ticking up during the row's lifetime so its last
	// snapshot is whatever happened to be in flight at termination
	// — usually misleading after the fact ("正在思考" on a row that
	// finished 30 s ago reads as a bug).
	if !row.endTime.IsZero() {
		return ""
	}
	d := strings.TrimSpace(row.detail)
	if d == "" {
		return ""
	}
	// thinkingPhrase needs `now` so it can split bare "thinking"
	// into "sending" (very recent — request still being dispatched)
	// vs "awaiting" (the network-bound wait while the server thinks).
	// detailStart is the timestamp of the most recent state change
	// on this row.
	if p := thinkingPhrase(d, lang, now, row.detailStart); p != "" {
		return p
	}
	if p := toolDetailPhrase(d, lang); p != "" {
		return p
	}
	return stripInternalTagPrefix(d)
}

// friendlyMetaText returns the dim trailer string: elapsed time +
// optional iteration round + optional tool count. Localized to lang.
// Empty when the row has no meta worth surfacing.
func friendlyMetaText(row *taskRow, now time.Time, lang string) string {
	if row == nil {
		return ""
	}
	var parts []string
	if !row.endTime.IsZero() {
		parts = append(parts, row.endTime.Sub(row.startTime).Truncate(time.Second).String())
	} else if !row.detailStart.IsZero() {
		parts = append(parts, now.Sub(row.detailStart).Truncate(time.Second).String())
	}
	if row.iteration > 0 {
		parts = append(parts, metaRoundPhrase(row.iteration, lang))
	}
	if row.toolCount > 0 {
		parts = append(parts, metaToolCountPhrase(row.toolCount, lang))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " · ")
}

// renderStatusBlock turns one statusBlock into one or more terminal-
// ready strings. Caller is responsible for truncation (each emitted
// line passes through truncByDisplayWidth at the redraw layer) and
// for joining them into the final area buffer.
func renderStatusBlock(b statusBlock, lang string) []string {
	var lines []string
	lines = append(lines, renderStatusLine(b.Line))
	if b.Kind != "topic_group" {
		return lines
	}
	// Propagate parent done-ness into the bullets so a topic group
	// that has terminated demotes its "关注点 N：" label to the
	// gray family — pre-2026-04-30 the label stayed FgLightBlue
	// regardless of state, so a done parent line (gray "已完成证据
	// 收集") sat above light-blue topic labels and the colours
	// disagreed on whether this section was past or live.
	parentDone := b.Line.State == "done"
	for _, t := range b.Topics {
		lines = append(lines, renderTopic(t, lang, parentDone))
	}
	return lines
}

// renderStatusLine assembles a single status row from its component
// parts. Layout:
//
//   "  <ICON> <PRIMARY> [· <DETAIL>] [· <META>]"
//
// All separators dim; meta strictly the dimmest.
func renderStatusLine(line statusLine) string {
	iconStyle := line.IconStyle
	if iconStyle == nil {
		iconStyle = statusSpinner
	}
	var b strings.Builder
	b.WriteString("  ")
	b.WriteString(iconStyle.Sprint(line.Icon))
	b.WriteString(" ")
	// Brightness ladder for the primary stage label:
	//   - error states (fatal / cancelled / recoverable) get the
	//     loudest hue so the user notices regressions
	//   - running rows use statusPrimary (light blue) so the active
	//     stage spotlights against the timeline
	//   - DONE rows demote to statusPrimaryDone (gray) so the
	//     completed timeline reads as "history" not "live work"
	//   - PENDING rows use statusMeta (dark gray) — distinct from
	//     done because pending is "not yet started" while done is
	//     "finished". Same gray family for visual cohesion, but
	//     two luminance steps apart so a pending row never reads
	//     as a completed row
	primaryStyle := statusPrimary
	switch line.ErrorKind {
	case statusErrorFatal, statusErrorCancelled:
		primaryStyle = statusFatal
	case statusErrorRecoverable:
		primaryStyle = statusRecoverable
	default:
		switch line.State {
		case "done":
			primaryStyle = statusPrimaryDone
		case "pending":
			primaryStyle = statusMeta
		}
	}
	b.WriteString(primaryStyle.Sprint(line.PrimaryText))
	if line.DetailText != "" {
		b.WriteString(" ")
		b.WriteString(statusMeta.Sprint("·"))
		b.WriteString(" ")
		b.WriteString(statusDetail.Sprint(line.DetailText))
	}
	if line.MetaText != "" {
		b.WriteString(" ")
		b.WriteString(statusMeta.Sprint("·"))
		b.WriteString(" ")
		b.WriteString(statusMeta.Sprint(line.MetaText))
	}
	return b.String()
}

// renderTopic renders one topic line: indented bullet + label +
// body. Overflow lines (Index==0) skip the label and just surface
// the "N more focus areas" sentinel as a dim continuation.
//
// parentDone flips the "关注点 N：" label from statusTopicLabel
// (light blue, "live") to statusPrimaryDone (gray, "history") so
// the bullets stay in lockstep with the parent line's state.
// Otherwise a done parent + light-blue bullets would read as
// "section is past but the items are still active".
func renderTopic(t statusTopic, lang string, parentDone bool) string {
	var b strings.Builder
	b.WriteString("    ")
	b.WriteString(statusMeta.Sprint(string(glyphTopicBullet)))
	b.WriteString(" ")
	if t.Index == 0 {
		b.WriteString(statusMeta.Sprint(t.Text))
		return b.String()
	}
	labelStyle := statusTopicLabel
	if parentDone {
		labelStyle = statusPrimaryDone
	}
	b.WriteString(labelStyle.Sprint(topicLabelPhrase(t.Index, lang)))
	b.WriteString(statusTopicText.Sprint(t.Text))
	return b.String()
}

// composeFooter builds the "总耗时 1m18s · 按 Ctrl+C 取消…" /
// "Total elapsed 1m18s · Press Ctrl+C…" trailer. cancelHint is
// caller-supplied (REPL passes its localized hint); empty falls
// back to defaultCancelHint.
func composeFooter(frame, elapsed, cancelHint, lang string) string {
	var b strings.Builder
	b.WriteString("  ")
	b.WriteString(statusSpinner.Sprint(frame))
	b.WriteString(" ")
	b.WriteString(statusMeta.Sprint(footerPhrase(elapsed, lang)))
	hint := cancelHint
	if hint == "" {
		hint = defaultCancelHint(lang)
	}
	b.WriteString(" ")
	b.WriteString(statusMeta.Sprint("·"))
	b.WriteString(" ")
	b.WriteString(statusMeta.Sprint(hint))
	return b.String()
}

// composeObjectiveLine renders the user-question header above the
// task list. Done-state uses muted green check; running uses a
// "❯" prompt chevron. Text uses statusObjective (bold light cyan)
// rather than statusPrimary (light blue) so the user-question
// header is unambiguously distinct from the running-stage row
// labels in the timeline below — the user's session 37+1 callout
// "状态颜色和用户输入问题的回显也没有区分" is the contract this
// styling has to honour.
func composeObjectiveLine(objective string, done bool) string {
	var icon string
	var iconStyle *pterm.Style
	var textStyle *pterm.Style
	if done {
		icon = string(glyphSuccess)
		iconStyle = statusSuccessMuted
		textStyle = statusObjectiveDone
	} else {
		icon = "❯"
		iconStyle = statusObjective
		textStyle = statusObjective
	}
	return fmt.Sprintf("  %s %s", iconStyle.Sprint(icon), textStyle.Sprint(objective))
}
