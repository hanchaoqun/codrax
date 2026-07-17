package tracediag

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// maxRenderedTokenBytes bounds any single rendered value token; the cut is
// rune-safe (CJK trace payloads must never be split mid-rune) and marked.
const maxRenderedTokenBytes = 480

// reportWriter emits report lines under the TOTAL line cap. When the cap is
// reached it emits exactly one disclosure line, then suppresses further body
// output; the end-of-report status summary bypasses the cap (bounded by the
// step count) so the run outcome is never silently lost.
type reportWriter struct {
	w          io.Writer
	totalCap   int
	emitted    int
	suppressed int
	capHit     bool
	err        error
}

func newReportWriter(w io.Writer, totalCap int) *reportWriter {
	return &reportWriter{w: w, totalCap: totalCap}
}

func (rw *reportWriter) line(s string) {
	if rw.err != nil {
		return
	}
	if rw.capHit {
		rw.suppressed++
		return
	}
	if rw.emitted >= rw.totalCap {
		rw.capHit = true
		rw.suppressed++
		_, rw.err = fmt.Fprintf(rw.w, "⚠ 报告总行帽 %d 已达,后续输出已抑制(全部步骤仍已执行,状态见文末摘要)\n", rw.totalCap)
		return
	}
	rw.emitted++
	_, rw.err = fmt.Fprintln(rw.w, s)
}

// summaryLine bypasses the total cap: the closing status summary is bounded
// by the step count and must survive even a runaway body.
func (rw *reportWriter) summaryLine(s string) {
	if rw.err != nil {
		return
	}
	_, rw.err = fmt.Fprintln(rw.w, s)
}

func (rw *reportWriter) flushErr() error { return rw.err }

func writeProvenanceHeader(rw *reportWriter, opts Options, script *Script, tracePath, traceSHA256 string, info os.FileInfo, flavorHint tracequery.TraceFlavor, at time.Time) {
	rw.line("# codrax tracediag 采集报告")
	rw.line(fmt.Sprintf("codrax_version=%s build_time=%s", opts.Version, opts.BuildTime))
	rw.line(fmt.Sprintf("generated_at=%s", at.Format(time.RFC3339)))
	// SEC #27 (2026-07-10): the report is THE round-trip artifact (witness
	// recipes instruct customers to return it verbatim), so the provenance
	// header identifies the trace/script by basename + size + content digest
	// — never by the operator's absolute path (/Users/<name>/… must not
	// leak). sha256 keeps exact artifact reconciliation without the path;
	// absolute paths stay in local logs only.
	rw.line(fmt.Sprintf("trace=%s size_bytes=%d sha256=%s", filepath.Base(tracePath), info.Size(), traceSHA256))
	rw.line(fmt.Sprintf("trace_flavor_hint=%s", string(flavorHint)))
	rw.line(fmt.Sprintf("script=%s version=%d steps=%d", filepath.Base(opts.ScriptPath), script.Version, len(script.Steps)))
	if strings.TrimSpace(opts.WindowOverride) != "" {
		rw.line(fmt.Sprintf("window_override=%s source=cli_flag target=defaults.window", clampToken(opts.WindowOverride)))
	}
	if strings.TrimSpace(script.Description) != "" {
		rw.line(fmt.Sprintf("description=%s", clampToken(script.Description)))
	}
	rw.line(fmt.Sprintf("line_caps: per_step_default=%d per_step_hard=%d report_total=%d", DefaultStepMaxLines, HardStepMaxLines, rw.totalCap))
	rw.line("说明: 本报告为确定性证据采集面(零 LLM);数值与 token 均为引擎原文透传,行号/时间坐标均指向 trace 文件本身。")
}

func writeStepHeader(rw *reportWriter, ordinal, total int, step *Step) {
	rw.line("")
	rw.line(strings.Repeat("=", 80))
	rw.line(fmt.Sprintf("[步骤 %d/%d] label=%s view=%s", ordinal, total, step.Label, step.View))
	rw.line("参数: " + stepParamsEcho(step))
	if requested, clamped := step.MaxLinesClamped(); clamped {
		rw.line(fmt.Sprintf("⚠ max_lines=%d 超过硬帽 %d,已夹取为 %d", requested, HardStepMaxLines, step.EffectiveMaxLines()))
	}
	// P1-1 (对抗复核 2026-07-09): faithful restatement of the engine's
	// line/time gate (tracequery eventInQueryWindow, query.go:909-925) —
	// line bounds always apply; TIME bounds apply only when NO line bounds
	// are set. A step carrying both must say so, or the echoed window reads
	// as an active filter it is not.
	//
	// TDIAG 留观① (§28.13 scope 语义分叉, 2026-07-09): the format_census step
	// filters by the INTERSECTION (censusScope.admits: window ∧ lines) — the
	// engine restatement above would be a lie there, so the census step
	// states its OWN semantics instead. No behavior change on either side.
	if _, _, windowSet := step.WindowBounds(); windowSet && (step.LineStart > 0 || step.LineEnd > 0) {
		if step.View == ViewFormatCensus {
			rw.line("注: 本步为普查步,时间窗与行区间取交集过滤(普查步语义;引擎查询步则 line 界恒生效、time 界仅在无 line 界时生效)")
		} else {
			rw.line("注: 行区间生效时时间窗不参与过滤(引擎语义:line 界恒生效,time 界仅在无 line 界时生效)")
		}
	}
	rw.line(strings.Repeat("-", 80))
}

// stepParamsEcho echoes the step parameters verbatim (原样回显) so the report
// is self-describing without the script file.
func stepParamsEcho(step *Step) string {
	parts := []string{}
	if step.PID > 0 {
		parts = append(parts, fmt.Sprintf("pid=%d", step.PID))
	}
	if step.Thread != "" {
		parts = append(parts, fmt.Sprintf("thread=%q", step.Thread))
	}
	if step.Window != "" {
		parts = append(parts, fmt.Sprintf("window=%s", step.Window))
	}
	if step.WindowsFrom != nil {
		parts = append(parts, fmt.Sprintf("windows_from=%s", step.WindowsFrom.Discovery))
	}
	if step.LineStart > 0 {
		parts = append(parts, fmt.Sprintf("line_start=%d", step.LineStart))
	}
	if step.LineEnd > 0 {
		parts = append(parts, fmt.Sprintf("line_end=%d", step.LineEnd))
	}
	if step.Pattern != "" {
		parts = append(parts, fmt.Sprintf("pattern=%q", step.Pattern))
	}
	if len(step.EventTypes) > 0 {
		parts = append(parts, fmt.Sprintf("event_types=[%s]", strings.Join(step.EventTypes, ",")))
	}
	if len(step.TraceMarkActions) > 0 {
		parts = append(parts, fmt.Sprintf("trace_mark_actions=[%s]", strings.Join(step.TraceMarkActions, ",")))
	}
	parts = append(parts, fmt.Sprintf("max_lines=%d", step.EffectiveMaxLines()))
	if len(parts) == 0 {
		return "(无)"
	}
	return strings.Join(parts, " ")
}

func writeStatusSummary(rw *reportWriter, statuses []StepStatus) {
	rw.summaryLine("")
	rw.summaryLine(strings.Repeat("=", 80))
	rw.summaryLine("[步骤状态摘要]")
	failed := 0
	for i, st := range statuses {
		if st.Err != nil {
			failed++
			rw.summaryLine(fmt.Sprintf("- 步骤 %d label=%s view=%s 状态=失败: %s", i+1, st.Label, st.View, clampToken(st.Err.Error())))
			continue
		}
		rw.summaryLine(fmt.Sprintf("- 步骤 %d label=%s view=%s 状态=成功 输出行=%d/%d", i+1, st.Label, st.View, st.BodyShown, st.BodyTotal))
	}
	if rw.suppressed > 0 {
		rw.summaryLine(fmt.Sprintf("- 总行帽截断: %d 行未输出(总帽=%d)", rw.suppressed, rw.totalCap))
	}
	if failed > 0 {
		rw.summaryLine(fmt.Sprintf("结论: %d/%d 步骤失败;报告仍为全量采集(失败步骤错误原文见各节)。", failed, len(statuses)))
	} else {
		rw.summaryLine(fmt.Sprintf("结论: 全部 %d 步骤成功。", len(statuses)))
	}
}

// stepBody is the collected (pre-cap) body of one step.
type stepBody struct {
	lines       []string // at most the per-step cap
	total       int      // lines the step would have produced without the cap
	eventSearch *eventSearchReportAccounting
}

// eventSearchReportAccounting records the rows the REPORT actually exposes,
// not merely the rows returned by the engine before report metadata consumed
// its line cap. Generated-window completeness gates consume this face so
// `emitted=N` can never exceed the number of visible raw rows.
type eventSearchReportAccounting struct {
	matched   int
	emitted   int
	compacted bool
}

// bodySink collects body lines under the per-step cap while counting the
// true total, so the truncation disclosure can state N/M/X honestly.
type bodySink struct {
	cap   int
	lines []string
	total int
}

func (b *bodySink) emit(s string) {
	b.total++
	if len(b.lines) < b.cap {
		b.lines = append(b.lines, s)
	}
}

// renderStepBody renders one successful step outcome into its body lines.
func renderStepBody(step *Step, outcome stepOutcome) stepBody {
	sink := &bodySink{cap: step.EffectiveMaxLines()}
	if outcome.census != nil {
		renderFormatCensus(outcome.census, sink.emit)
		return stepBody{lines: sink.lines, total: sink.total}
	}
	res := outcome.result
	// SEC 捎带⑤: prescan for same-basename source_path collisions so the
	// basename-sanitized display (SEC #27) stays unambiguous in dual-trace
	// (CMP) results; reset after the step body.
	sourcePathAmbiguousBases = collectAmbiguousSourcePathBases(res)
	defer func() { sourcePathAmbiguousBases = nil }()
	if step.View == "event_search" {
		return renderEventSearchBody(step, res)
	}
	var meta []string
	renderResultMeta(res, func(line string) { meta = append(meta, line) })
	rawCaveats := rawPerfCaptureCaveats(res.Caveats)
	rawFoldedIntoMeta := len(rawCaveats) > 0 && step.EffectiveMaxLines() <= len(meta)
	if rawFoldedIntoMeta && len(meta) > 0 {
		// A real selected window consumes both ordinary metadata seats. Fold the
		// same bounded typed token into the first metadata line rather than let
		// bodySink silently discard the global quality boundary.
		meta[0] += " perf_capture={" + rawPerfCaptureHeaderToken(rawCaveats[0], 1, len(rawCaveats)) + "}"
	}
	for _, line := range meta {
		sink.emit(line)
	}
	// Manifest-global capture quality gets the first post-metadata seat. It is
	// deliberately outside the ordinary diagnostic roster below.
	if !rawFoldedIntoMeta {
		renderRawPerfCaptureKeyFirst(res, sink.emit)
	}
	diagnostics := collectNonEventEngineDiagnostics(res)
	if len(diagnostics) > 0 {
		caveats, compactions := countNonEventEngineDiagnostics(diagnostics)
		sink.emit(fmt.Sprintf("engine_diagnostics(引擎原文去重账目): caveats=%d compactions=%d", caveats, compactions))
	}
	// TDIAG-KEY-FIRST: load-bearing typed quality/completeness/pairing/
	// capability facts must precede both diagnostic rosters and ordinary
	// detail. A long caveat list is itself bounded only by max_lines and must
	// not recreate the old CPU-residency-before-counter-quality failure.
	renderNonEventKeyFirstSummaries(res, sink.emit)
	renderNonEventEngineDiagnostics(diagnostics, sink.emit)
	// The generic walker keeps the full detail/evidence face, but skips the
	// exact typed fields already rendered above and walks bulk arrays last.
	renderNonEventResultDetail(res, sink.emit)
	if len(res.Events) > 0 {
		sink.emit(fmt.Sprintf("匹配事件 %d 行:", len(res.Events)))
		for _, ev := range res.Events {
			sink.emit(renderEventRow(ev))
		}
	}
	if len(res.EvidencePack) > 0 {
		sink.emit(fmt.Sprintf("观测记录(引擎 evidence,原文 token,共 %d 条):", len(res.EvidencePack)))
		for _, fact := range res.EvidencePack {
			sink.emit(renderEvidenceFact(fact))
		}
	}
	return stepBody{lines: sink.lines, total: sink.total}
}

// renderEventSearchBody is a dedicated raw-witness renderer. Event search's
// EvidencePack is normally a row-for-row projection of Events (apart from
// structured summaries such as CPUFrequencyCensus and precise causal-negative
// observations such as writeback error-sequence rows), so rendering it after
// the raw rows doubled the same evidence and pushed the load-bearing
// caveat/compaction metadata beyond max_lines. This face instead publishes,
// in priority order:
// result/window metadata, typed compaction + engine caveats, an exact visible
// raw-row header, then the raw rows. The header also carries the accounting
// and priority caveat token, so even the minimum generated cap keeps those
// facts visible while preserving its two-endpoint raw-row floor.
func renderEventSearchBody(step *Step, res *tracequery.Result) stepBody {
	capLines := step.EffectiveMaxLines()
	var meta []string
	renderResultMeta(res, func(line string) { meta = append(meta, line) })

	matched := eventSearchMatchedRows(res)
	available := capLines - len(meta) - 1 // one exact match/header line
	if available < 0 {
		available = 0
	}
	rawFloor := 0
	if len(res.Events) > 0 && available > 0 {
		rawFloor = 1
		if step.windowOrigin != nil && len(res.Events) > 1 && available > 1 {
			rawFloor = 2
		}
	}

	// The diagnostic roster size depends on whether report-level trimming
	// itself creates a compaction. Iterate to the tiny fixed point (false→true
	// at most once in practice); the visible budget is otherwise pure integer
	// arithmetic and therefore deterministic for static/generated twins.
	emitted := minInt(len(res.Events), available)
	compacted := matched > emitted
	var diagnostics, shownDiagnostics []string
	for i := 0; i < 4; i++ {
		diagnostics = eventSearchDiagnosticLines(res, matched, emitted, compacted)
		diagnosticBudget := available - rawFloor
		if diagnosticBudget < 0 {
			diagnosticBudget = 0
		}
		shownCount := minInt(len(diagnostics), diagnosticBudget)
		shownDiagnostics = diagnostics[:shownCount]
		rawCapacity := available - shownCount
		if rawCapacity < 0 {
			rawCapacity = 0
		}
		nextEmitted := minInt(len(res.Events), rawCapacity)
		nextCompacted := matched > nextEmitted
		if nextEmitted == emitted && nextCompacted == compacted {
			emitted, compacted = nextEmitted, nextCompacted
			break
		}
		emitted, compacted = nextEmitted, nextCompacted
	}
	// Rebuild once with the fixed-point accounting values; line cardinality is
	// stable, but the compaction/census tokens must state the final emitted N.
	diagnostics = eventSearchDiagnosticLines(res, matched, emitted, compacted)
	diagnosticBudget := available - rawFloor
	if diagnosticBudget < 0 {
		diagnosticBudget = 0
	}
	shownCount := minInt(len(diagnostics), diagnosticBudget)
	shownDiagnostics = diagnostics[:shownCount]

	// Preserve non-duplicate structured faces (notably TraceArtifacts) when
	// the raw roster leaves room. CPUFrequencyCensus already has the compact,
	// report-visible-count row above; Events/EvidencePack/caveats/compactions
	// are deliberately cleared so this tail cannot restate priority content.
	detailResult := *res
	detailResult.Events = nil
	detailResult.EvidencePack = nil
	detailResult.Caveats = nil
	detailResult.Compactions = nil
	detailResult.CPUFrequencyCensus = nil
	var details []string
	renderResultDetail(&detailResult, func(line string) { details = append(details, line) })
	detailBudget := capLines - (len(meta) + len(shownDiagnostics) + 1 + emitted)
	if detailBudget < 0 {
		detailBudget = 0
	}
	shownDetailCount := minInt(len(details), detailBudget)
	priorityCaveat := eventSearchPriorityCaveat(compacted, res.Caveats)
	header := fmt.Sprintf("匹配事件 %d 行 (matched=%d emitted=%d compacted=%t caveat=%s diagnostics=%d/%d details=%d/%d",
		emitted, matched, emitted, compacted, priorityCaveat,
		len(shownDiagnostics), len(diagnostics), shownDetailCount, len(details))
	// If the protected raw-row floor leaves no independent advisory line, fold
	// one compact typed receipt into the accounting header. This preserves the
	// existing generated start/done endpoint floor while keeping the global
	// quality boundary visible at max_lines=3/5. A 1/N token discloses bounded
	// multi-artifact omission without restating a caveat already shown above.
	rawCaveats := rawPerfCaptureCaveats(res.Caveats)
	if len(rawCaveats) > 0 && shownCount == 0 {
		header += " perf_capture={" + rawPerfCaptureHeaderToken(rawCaveats[0], 1, len(rawCaveats)) + "}"
	}
	header += "):"
	lines := make([]string, 0, len(meta)+len(shownDiagnostics)+1+emitted+shownDetailCount)
	lines = append(lines, meta...)
	lines = append(lines, shownDiagnostics...)
	lines = append(lines, header)
	for i := 0; i < emitted; i++ {
		lines = append(lines, renderEventRow(res.Events[i]))
	}
	lines = append(lines, details[:shownDetailCount]...)
	return stepBody{
		lines: lines,
		// Raw-row compaction and diagnostic/detail suppression are already
		// carried by the typed header above. Returning the presentation total
		// prevents Run's generic "…共 N 行" tail from conflating those three
		// different dimensions into one opaque body-line count.
		total: len(lines),
		eventSearch: &eventSearchReportAccounting{
			matched: matched, emitted: emitted, compacted: compacted,
		},
	}
}

func eventSearchMatchedRows(res *tracequery.Result) int {
	matched := len(res.Events)
	for _, compaction := range res.Compactions {
		if compaction.Dimension == tracequery.CompactionDimensionEvents && compaction.Total > matched {
			matched = compaction.Total
		}
	}
	return matched
}

func eventSearchDiagnosticLines(res *tracequery.Result, matched, emitted int, compacted bool) []string {
	var lines []string
	// This global quality boundary must be the first diagnostic and appears
	// only in its compact typed face, never again in the ordinary caveat roster.
	rawCaveats := rawPerfCaptureCaveats(res.Caveats)
	for i, caveat := range rawCaveats {
		lines = append(lines, rawPerfCaptureKeyFirstLine(caveat, i+1, len(rawCaveats)))
	}
	if compacted {
		lastTs := 0.0
		lastLine := 0
		if emitted > 0 && emitted <= len(res.Events) {
			last := res.Events[emitted-1]
			lastTs, lastLine = last.Ts, last.Line
		}
		lines = append(lines, fmt.Sprintf("- 报告截断账目: view=event_search dimension=%s matched=%d emitted=%d compacted=true last_visible_ts=%s last_visible_line=%d",
			tracequery.CompactionDimensionEvents, matched, emitted, formatSecondsToken(lastTs), lastLine))
		lines = append(lines, fmt.Sprintf("- 报告完整性提示: report_event_search_compacted=true; matched=%d emitted=%d; omitted raw rows may contain later frame/span ids, so do not infer absence without a narrower exact query", matched, emitted))
	}

	for _, caveat := range eventSearchEngineCaveats(res.Caveats) {
		lines = append(lines, "- 引擎 caveat 原文: "+clampToken(caveat))
	}
	// CPUFrequencyCensus is the one event_search structured face that is not
	// a duplicate of a raw Event. Keep it as one compact priority row and use
	// the REPORT-visible frequency count, not the engine pre-render count.
	if census := res.CPUFrequencyCensus; census != nil {
		visibleFrequencyRows := 0
		for i := 0; i < emitted && i < len(res.Events); i++ {
			if tracequery.IsPerCPUFrequencySample(res.Events[i].Event) {
				visibleFrequencyRows++
			}
		}
		lines = append(lines, fmt.Sprintf("- cpu_frequency_census: matched_frequency_rows=%d displayed_frequency_rows=%d tiers_khz_x_rows=%s min_khz=%d max_khz=%d cpus=%v",
			census.MatchedFrequencyRows, visibleFrequencyRows,
			tracequery.FormatCPUFrequencyCensusTiers(census.Tiers, 24),
			census.MinKHz, census.MaxKHz, census.CPUs))
	}
	return lines
}

func eventSearchEngineCaveats(in []string) []string {
	out := make([]string, 0, len(in))
	for _, caveat := range in {
		if tracequery.IsRawPerfCaptureCompletenessCaveat(caveat) {
			continue
		}
		// This engine token describes Result.Events, whose Emitted count can be
		// larger than the report-visible roster after tracediag prioritizes
		// metadata. The report-level replacement above owns that final number;
		// retaining this stale twin would present two conflicting rulers.
		if strings.HasPrefix(strings.TrimSpace(caveat), "event_search_stream_compacted=") {
			continue
		}
		out = append(out, caveat)
	}
	return out
}

func eventSearchPriorityCaveat(compacted bool, caveats []string) string {
	if compacted {
		return "report_event_search_compacted=true"
	}
	caveats = eventSearchEngineCaveats(caveats)
	if len(caveats) == 0 {
		return "none"
	}
	token := strings.TrimSpace(strings.SplitN(caveats[0], ";", 2)[0])
	if token == "" {
		return "present"
	}
	return clampToken(token)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func renderResultMeta(res *tracequery.Result, emit func(string)) {
	parts := []string{"result:", "view=" + res.View}
	if res.TraceFlavor != "" {
		parts = append(parts, fmt.Sprintf("flavor=%s(conf=%s)", res.TraceFlavor, formatFloatToken(res.FlavorConfidence)))
	}
	if res.Platform != "" {
		parts = append(parts, "platform="+res.Platform)
	}
	if res.TimeUnit != "" {
		parts = append(parts, "time_unit="+res.TimeUnit)
	}
	parts = append(parts,
		fmt.Sprintf("line_count=%d", res.LineCount),
		fmt.Sprintf("scanned_lines=%d", res.ScannedLineCount),
		fmt.Sprintf("event_count=%d", res.EventCount),
		fmt.Sprintf("unparsed_lines=%d", res.UnparsedLineCount),
		fmt.Sprintf("clock_regressions=%d", res.ClockRegressions),
	)
	emit(strings.Join(parts, " "))
	windowParts := []string{}
	if res.TimeStart > 0 || res.TimeEnd > 0 {
		windowParts = append(windowParts, fmt.Sprintf("query=[%s..%s]", formatSecondsToken(res.TimeStart), formatSecondsToken(res.TimeEnd)))
	}
	if res.IndexWindowed {
		windowParts = append(windowParts, fmt.Sprintf("index=[%s..%s] index_lines=[%d..%d]",
			formatSecondsToken(res.IndexTimeStart), formatSecondsToken(res.IndexTimeEnd), res.IndexLineStart, res.IndexLineEnd))
	}
	if len(windowParts) > 0 {
		emit("window: " + strings.Join(windowParts, " "))
	}
}

// renderEventRow renders one event_search row: coordinates as engine tokens,
// then the raw trace line verbatim (the evidence itself).
func renderEventRow(ev tracequery.EventView) string {
	raw := ev.Raw
	if raw == "" {
		raw = ev.FieldText
	}
	return fmt.Sprintf("- line=%d ts=%s type=%s | %s", ev.Line, formatSecondsToken(ev.Ts), string(ev.Type), clampToken(raw))
}

// renderEvidenceFact renders one engine evidence fact in the 系统补充
// observation-row token style: subject/predicate/object, verbatim summary,
// line span, time span.
func renderEvidenceFact(fact tracequery.EvidenceFact) string {
	parts := []string{}
	head := fact.Subject
	if fact.Predicate != "" {
		head += " " + fact.Predicate
	}
	if fact.Object != "" {
		head += " -> " + fact.Object
	}
	if strings.TrimSpace(head) != "" {
		parts = append(parts, clampToken(head))
	}
	if fact.Summary != "" {
		parts = append(parts, clampToken(fact.Summary))
	}
	if fact.LineStart > 0 {
		if fact.LineEnd > fact.LineStart {
			parts = append(parts, fmt.Sprintf("lines=%d-%d", fact.LineStart, fact.LineEnd))
		} else {
			parts = append(parts, fmt.Sprintf("lines=%d", fact.LineStart))
		}
	}
	if fact.StartTs > 0 || fact.EndTs > 0 {
		parts = append(parts, fmt.Sprintf("ts=[%s..%s]", formatSecondsToken(fact.StartTs), formatSecondsToken(fact.EndTs)))
	}
	if fact.Confidence > 0 {
		parts = append(parts, "conf="+formatFloatToken(fact.Confidence))
	}
	return "- " + strings.Join(parts, "; ")
}

// resultMetaFields are the Result fields the meta block / dedicated faces
// already render; the generic detail walker skips them.
var resultMetaFields = map[string]bool{
	"View": true, "SourcePath": true, "TraceFlavor": true, "Platform": true,
	"PlatformCandidate": true, "PlatformCandidateConfidence": true,
	"PlatformCandidateSignals": true, "FlavorConfidence": true,
	"FlavorSignals": true, "FrameworkMode": true, "FrameworkSurfaces": true,
	"TimeUnit": true, "PrioritySemantics": true, "LineCount": true,
	"ScannedLineCount": true, "IndexWindowed": true, "IndexTimeStart": true,
	"IndexTimeEnd": true, "IndexLineStart": true, "IndexLineEnd": true,
	"EventCount": true, "UnparsedLineCount": true, "ParseLinePanics": true,
	"ClockRegressions": true, "TimeStart": true, "TimeEnd": true,
	"Events": true, "EvidencePack": true, "Caveats": true, "Compactions": true,
}

// renderResultDetail walks every structured payload of the Result (Timeline,
// WindowStats, RootCauseRank, WakeupChain, …) generically and emits the
// engine's per-item Summary line families (窗口统计类 view 的 Summary 行族)
// plus key=value token lines for structs without a Summary. Reflection keeps
// the face complete and drift-free: payload fields added by future engine
// batches render automatically instead of silently vanishing.
func renderResultDetail(res *tracequery.Result, emit func(string)) {
	renderResultDetailWithPolicy(res, emit, nil)
}

func renderNonEventResultDetail(res *tracequery.Result, emit func(string)) {
	renderResultDetailWithPolicy(res, emit, &nonEventDetailPolicy)
}

func renderResultDetailWithPolicy(res *tracequery.Result, emit func(string), policy *detailRenderPolicy) {
	v := reflect.ValueOf(res).Elem()
	t := v.Type()
	for _, i := range orderedDetailFieldIndexes(v, policy) {
		field := t.Field(i)
		if field.PkgPath != "" || resultMetaFields[field.Name] || jsonExcluded(field) || policySkipsDetailField(policy, t, field.Name) {
			continue
		}
		fv := v.Field(i)
		if isZeroValue(fv) {
			continue
		}
		path := jsonTagName(field)
		headered := false
		lazyEmit := func(s string) {
			if !headered {
				headered = true
				emit(fmt.Sprintf("明细 %s:", path))
			}
			emit(s)
		}
		walkDetailWithPolicy(fv, path, lazyEmit, 0, policy)
	}
}

// walkDetailMaxDepth bounds the reflection recursion; engine payloads are
// shallow (≤6), the bound only guards against future cyclic structures.
const walkDetailMaxDepth = 10

func walkDetail(v reflect.Value, path string, emit func(string), depth int) {
	walkDetailWithPolicy(v, path, emit, depth, nil)
}

func walkDetailWithPolicy(v reflect.Value, path string, emit func(string), depth int, policy *detailRenderPolicy) {
	if depth > walkDetailMaxDepth {
		return
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return
		}
		walkDetailWithPolicy(v.Elem(), path, emit, depth, policy)
	case reflect.Struct:
		walkStructDetailWithPolicy(v, path, emit, depth, policy)
	case reflect.Slice, reflect.Array:
		if v.Len() == 0 {
			return
		}
		if isScalarKind(v.Type().Elem().Kind()) {
			emit(fmt.Sprintf("- %s: %s", path, formatScalarSlice(v)))
			return
		}
		for i := 0; i < v.Len(); i++ {
			walkDetailWithPolicy(v.Index(i), fmt.Sprintf("%s[%d]", path, i), emit, depth+1, policy)
		}
	case reflect.Map:
		if v.Len() == 0 {
			return
		}
		emit(fmt.Sprintf("- %s: %s", path, formatMapTokens(v)))
	default:
		emit(fmt.Sprintf("- %s: %s", path, formatScalar(v)))
	}
}

func walkStructDetail(v reflect.Value, path string, emit func(string), depth int) {
	walkStructDetailWithPolicy(v, path, emit, depth, nil)
}

func walkStructDetailWithPolicy(v reflect.Value, path string, emit func(string), depth int, policy *detailRenderPolicy) {
	t := v.Type()
	// Inline token types render as one token, never as a line family.
	if t == reflect.TypeOf(tracequery.ThreadRef{}) || t == reflect.TypeOf(tracequery.TimeWindow{}) {
		if token := formatInlineStruct(v); token != "" {
			emit(fmt.Sprintf("- %s: %s", path, token))
		}
		return
	}
	summary := ""
	// Direct (non-promoted) field lookup only: reflect promotion through a
	// nil embedded pointer group panics, and the P4 side-table ban forbids
	// promoted reads anyway.
	if f, ok := directField(v, "Summary"); ok && f.Kind() == reflect.String {
		summary = f.String()
	}
	if summary != "" {
		line := fmt.Sprintf("- %s: %s", path, clampToken(summary))
		if extra := structCoordinateTokens(v); extra != "" {
			line += "; " + extra
		}
		emit(line)
	} else if kv := structScalarTokensWithPolicy(v, policy); kv != "" {
		emit(fmt.Sprintf("- %s: %s", path, kv))
	}
	// Recurse into composite children so nested families (rank items,
	// chain nodes, per-thread breakdowns) keep their own lines. Composite
	// slices/maps are structurally bulk and run after compact pointer/struct
	// children; the exact key-first fields are governed by the closed policy.
	for _, i := range orderedDetailFieldIndexes(v, policy) {
		field := t.Field(i)
		if field.PkgPath != "" || field.Name == "Summary" || jsonExcluded(field) || policySkipsDetailField(policy, t, field.Name) {
			continue
		}
		fv := v.Field(i)
		if isZeroValue(fv) {
			continue
		}
		switch fv.Kind() {
		case reflect.Struct, reflect.Slice, reflect.Array, reflect.Map, reflect.Pointer, reflect.Interface:
			ft := fv.Type()
			for ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft == reflect.TypeOf(tracequery.ThreadRef{}) || ft == reflect.TypeOf(tracequery.TimeWindow{}) {
				continue // already rendered inline by structScalarTokens/coordinates
			}
			childPath := path + "." + jsonTagName(field)
			if field.Anonymous {
				childPath = path
			}
			walkDetailWithPolicy(fv, childPath, emit, depth+1, policy)
		}
	}
}

// structCoordinateTokens extracts the standard evidence coordinates riding a
// Summary-bearing struct: thread identity, line span, time span.
func structCoordinateTokens(v reflect.Value) string {
	parts := []string{}
	for _, name := range []string{"Thread", "Target"} {
		if f, ok := directField(v, name); ok && f.Type() == reflect.TypeOf(tracequery.ThreadRef{}) {
			if token := formatInlineStruct(f); token != "" {
				parts = append(parts, "thread="+token)
			}
		}
	}
	lineStart, lineEnd := 0, 0
	if f, ok := directField(v, "LineStart"); ok && f.Kind() == reflect.Int {
		lineStart = int(f.Int())
	}
	if f, ok := directField(v, "StartLine"); ok && f.Kind() == reflect.Int {
		lineStart = int(f.Int())
	}
	if f, ok := directField(v, "LineEnd"); ok && f.Kind() == reflect.Int {
		lineEnd = int(f.Int())
	}
	if f, ok := directField(v, "EndLine"); ok && f.Kind() == reflect.Int {
		lineEnd = int(f.Int())
	}
	if lineStart > 0 {
		if lineEnd > lineStart {
			parts = append(parts, fmt.Sprintf("lines=%d-%d", lineStart, lineEnd))
		} else {
			parts = append(parts, fmt.Sprintf("lines=%d", lineStart))
		}
	}
	return strings.Join(parts, "; ")
}

// structScalarTokens renders a struct's non-zero scalar fields as verbatim
// key=value tokens (json tag names), inlining ThreadRef/TimeWindow values.
func structScalarTokens(v reflect.Value) string {
	return structScalarTokensWithPolicy(v, nil)
}

func structScalarTokensWithPolicy(v reflect.Value, policy *detailRenderPolicy) string {
	t := v.Type()
	parts := []string{}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" || jsonExcluded(field) || policySkipsDetailField(policy, t, field.Name) {
			continue
		}
		fv := v.Field(i)
		if isZeroValue(fv) {
			continue
		}
		ft := fv.Type()
		if ft == reflect.TypeOf(tracequery.ThreadRef{}) || ft == reflect.TypeOf(tracequery.TimeWindow{}) {
			if token := formatInlineStruct(fv); token != "" {
				parts = append(parts, jsonTagName(field)+"="+token)
			}
			continue
		}
		if isScalarKind(fv.Kind()) {
			tag := jsonTagName(field)
			parts = append(parts, tag+"="+formatScalarForTag(fv, tag))
		}
	}
	return strings.Join(parts, " ")
}

// formatScalarForTag applies the P1-2 quantity-form ruling on the kv fallback
// face: engine "_ms" fields align with the engine's own %.3f publication form
// and "_ts" fields render as fixed-point trace coordinates; every other
// scalar keeps the exact shortest round-trip form.
//
// SEC #27 exception to verbatim passthrough: `source_path` fields carry the
// COLLECTION MACHINE's absolute file path (engine provenance, not trace
// evidence) — the report is the round-trip artifact, so they render as
// basename only, matching the provenance header's trace= line. Trace-internal
// paths (ResourceFields.Path etc., tag "path") stay verbatim: they are device
// paths from the trace itself, i.e. evidence.
func formatScalarForTag(v reflect.Value, tag string) string {
	if v.Kind() == reflect.String && tag == "source_path" {
		return clampToken(sourcePathDisplayToken(v.String()))
	}
	if v.Kind() == reflect.Float64 || v.Kind() == reflect.Float32 {
		switch {
		case strings.HasSuffix(tag, "_ms"):
			return formatMsToken(v.Float())
		case strings.HasSuffix(tag, "_ts"):
			return formatSecondsToken(v.Float())
		}
	}
	return formatScalar(v)
}

// sourcePathAmbiguousBases is the per-step render state for the SEC 捎带⑤
// disambiguation: when one result carries DISTINCT source paths sharing a
// basename (CMP dual-trace comparisons with same-named files), the basename
// alone is ambiguous. renderStepBody prescans the result and stamps the
// colliding basenames here; sourcePathDisplayToken then appends a short
// non-path identifier (file size when statable, else a path-derived short
// id). Rendering is single-threaded (one CLI report writer), so plain
// package state set/reset around each step body is safe.
var sourcePathAmbiguousBases map[string]bool

// sourcePathDisplayToken renders one machine-local source path as its
// basename, plus a short disambiguator when the current step carries a
// same-basename collision. The disambiguator never contains path segments.
func sourcePathDisplayToken(p string) string {
	base := filepath.Base(p)
	if !sourcePathAmbiguousBases[strings.ToLower(base)] {
		return base
	}
	if info, err := os.Stat(p); err == nil {
		return fmt.Sprintf("%s(size_bytes=%d)", base, info.Size())
	}
	sum := sha256.Sum256([]byte(p))
	return fmt.Sprintf("%s(id=%s)", base, hex.EncodeToString(sum[:4]))
}

// collectAmbiguousSourcePathBases reflect-walks a result the same way the
// detail renderer does and returns the lowercased basenames that map to
// more than one distinct source_path value.
func collectAmbiguousSourcePathBases(res *tracequery.Result) map[string]bool {
	pathsByBase := map[string]map[string]bool{}
	var walk func(v reflect.Value, depth int)
	walk = func(v reflect.Value, depth int) {
		if depth > walkDetailMaxDepth {
			return
		}
		switch v.Kind() {
		case reflect.Pointer, reflect.Interface:
			if v.IsNil() {
				return
			}
			walk(v.Elem(), depth)
		case reflect.Struct:
			t := v.Type()
			for i := 0; i < t.NumField(); i++ {
				field := t.Field(i)
				if field.PkgPath != "" || jsonExcluded(field) {
					continue
				}
				fv := v.Field(i)
				if fv.Kind() == reflect.String && jsonTagName(field) == "source_path" {
					full := fv.String()
					if strings.TrimSpace(full) == "" {
						continue
					}
					base := strings.ToLower(filepath.Base(full))
					if pathsByBase[base] == nil {
						pathsByBase[base] = map[string]bool{}
					}
					pathsByBase[base][full] = true
					continue
				}
				walk(fv, depth+1)
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < v.Len(); i++ {
				walk(v.Index(i), depth+1)
			}
		}
	}
	if res != nil {
		walk(reflect.ValueOf(res).Elem(), 0)
	}
	out := map[string]bool{}
	for base, paths := range pathsByBase {
		if len(paths) > 1 {
			out[base] = true
		}
	}
	return out
}

func formatInlineStruct(v reflect.Value) string {
	switch ref := v.Interface().(type) {
	case tracequery.ThreadRef:
		if ref.Comm == "" && ref.PID == 0 {
			return ""
		}
		token := ref.Comm
		if ref.PID > 0 {
			if token != "" {
				token += "-"
			}
			token += strconv.Itoa(ref.PID)
		}
		if ref.TGID > 0 {
			token += fmt.Sprintf("(tgid=%d)", ref.TGID)
		}
		return token
	case tracequery.TimeWindow:
		if ref.StartTs == 0 && ref.EndTs == 0 {
			return ""
		}
		return fmt.Sprintf("[%s..%s]", formatSecondsToken(ref.StartTs), formatSecondsToken(ref.EndTs))
	}
	return ""
}

// jsonExcluded reports a `json:"-"` field: the engine's explicit "not an
// observation face" marker (e.g. WindowStats.ClusterFrequencyCeilings, the
// CFC internal computation structure kept off EVERY JSON surface). The
// evidence walker honors the same exclusion — rendering such a field would
// mint a display surface the engine deliberately withheld.
func jsonExcluded(field reflect.StructField) bool {
	return field.Tag.Get("json") == "-"
}

// directField returns a struct's OWN field by name — never a promoted field
// from an embedded type (reflect promotion through a nil embedded pointer
// panics, and the tracequery P4 side-table promotion ban applies).
func directField(v reflect.Value, name string) (reflect.Value, bool) {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.Anonymous && f.PkgPath == "" && f.Name == name {
			return v.Field(i), true
		}
	}
	return reflect.Value{}, false
}

func jsonTagName(field reflect.StructField) string {
	tag := field.Tag.Get("json")
	if tag == "" || tag == "-" {
		return field.Name
	}
	if idx := strings.IndexByte(tag, ','); idx >= 0 {
		tag = tag[:idx]
	}
	if tag == "" {
		return field.Name
	}
	return tag
}

func isScalarKind(k reflect.Kind) bool {
	switch k {
	case reflect.Bool, reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

func isZeroValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Slice, reflect.Map:
		return v.Len() == 0
	case reflect.Pointer, reflect.Interface:
		return v.IsNil()
	}
	return v.IsZero()
}

func formatScalar(v reflect.Value) string {
	switch v.Kind() {
	case reflect.Bool:
		return strconv.FormatBool(v.Bool())
	case reflect.String:
		return clampToken(v.String())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		return formatFloatToken(v.Float())
	}
	return fmt.Sprintf("%v", v.Interface())
}

// formatScalarSlice / formatMapTokens (TDIAG 留观② 反射单行字节无界,
// §28.13, 2026-07-09): per-element clamping bounds each element but a long
// slice/map still joined into an unbounded single line — the JOINED token now
// passes through clampToken too (rune-safe byte cap + truncation marker; the
// full payload stays in the engine result).
func formatScalarSlice(v reflect.Value) string {
	parts := make([]string, 0, v.Len())
	for i := 0; i < v.Len(); i++ {
		parts = append(parts, formatScalar(v.Index(i)))
	}
	return clampToken("[" + strings.Join(parts, ", ") + "]")
}

func formatMapTokens(v reflect.Value) string {
	keys := v.MapKeys()
	tokens := make([]string, 0, len(keys))
	for _, k := range keys {
		tokens = append(tokens, fmt.Sprintf("%v=%s", k.Interface(), formatScalar(v.MapIndex(k))))
	}
	sort.Strings(tokens)
	return clampToken(strings.Join(tokens, " "))
}

// formatFloatToken renders floats with the shortest exact round-trip form —
// verbatim value fidelity without invented precision. NEVER use it for
// second-scale trace coordinates: 'g' switches to scientific notation at
// berlin-magnitude timestamps (6793222.031 → 6.793222031e+06), which is
// unreadable as a trace coordinate (P1-2, 对抗复核 2026-07-09). Coordinates
// go through formatSecondsToken; ms fields align with the engine's %.3f.
func formatFloatToken(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// formatSecondsToken renders a second-scale trace coordinate (ts / window
// endpoint) in the trace file's own fixed-point form: 6 decimals, matching
// the ftrace timestamp column, never scientific notation.
func formatSecondsToken(f float64) string {
	return strconv.FormatFloat(f, 'f', 6, 64)
}

// formatMsToken renders a millisecond value in the engine's own publication
// form (%.3f — every engine Summary string uses it), so the kv fallback face
// and the Summary face cannot publish two shapes for one quantity.
func formatMsToken(f float64) string {
	return strconv.FormatFloat(f, 'f', 3, 64)
}

// clampToken bounds one rendered token CJK-safely (types.CutPrefixRuneSafe,
// the shared rune-safe truncation authority) and marks the cut.
func clampToken(s string) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	if len(s) <= maxRenderedTokenBytes {
		return s
	}
	return types.CutPrefixRuneSafe(s, maxRenderedTokenBytes) + "…(截断)"
}
