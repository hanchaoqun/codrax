package tracediag

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// Options configures one tracediag collection run.
type Options struct {
	ScriptPath string
	TracePath  string
	// WindowOverride replaces script defaults.window before strict validation.
	// Explicit step/discovery windows are unchanged.
	WindowOverride string
	// TIDOverride supplies a positive trace thread ID only to v2 steps that
	// explicitly bind pid_from: tid. The CLI input does not scope unbound
	// steps; their ordinary YAML defaults remain authoritative.
	TIDOverride string
	// FlavorHint is the CLI --trace-flavor value. STRICT enum
	// (auto|harmony_hitrace|android_atrace|generic_ftrace); empty means auto.
	// tracequery.NormalizeTraceFlavor's silent unknown→auto tolerance is an
	// LLM-noise arm — a deterministic CLI must fail loud instead.
	FlavorHint string
	// Version / BuildTime feed the provenance header.
	Version   string
	BuildTime string
	// Now overrides the provenance clock (tests); nil = time.Now.
	Now func() time.Time
	// TotalMaxLines bounds the whole report; 0 = DefaultTotalMaxLines.
	TotalMaxLines int
}

// StepStatus is the per-step outcome recorded for the end-of-report summary
// and the exit-code decision.
type StepStatus struct {
	Label     string
	View      string
	Err       error
	BodyTotal int // body lines the step produced before the per-step cap
	BodyShown int // body lines actually emitted
}

// flavorHintEnum is the closed --trace-flavor value set.
var flavorHintEnum = []string{
	string(tracequery.TraceFlavorAuto),
	string(tracequery.TraceFlavorHarmonyHitrace),
	string(tracequery.TraceFlavorAndroidAtrace),
	string(tracequery.TraceFlavorGenericFtrace),
}

// stepIndexMaxEvents overrides the engine's per-index event budget for
// index-backed steps; 0 keeps the engine default (250K). Production leaves it
// 0 — it exists so the IndexEventLimitError step-failure lane is reachable in
// tests without a quarter-million-line fixture, and as the natural seam for a
// future budget flag.
var stepIndexMaxEvents = 0

// eventSearchReportBaseLines is the invariant report overhead shared by
// static and generated event_search steps: one result line, one window line,
// and one match-accounting/header line. A manifest-global key-first quality
// advisory is budgeted dynamically by the renderer only when it exists; it
// must not unconditionally reduce every ordinary event-search raw-row budget.
const eventSearchReportBaseLines = 3

func eventSearchEngineRowLimit(maxLines int) int {
	limit := maxLines - eventSearchReportBaseLines
	// Query.Limit==0 means "use the engine default", not zero rows. Keep a
	// one-row internal sample for metadata/compaction accounting when a tiny
	// static step has no room to publish raw rows; the renderer will honestly
	// report emitted=0 under that cap.
	if limit < 1 {
		return 1
	}
	return limit
}

// Run executes the collection script against the trace and writes the full
// report to w. It ALWAYS runs every step (collection completeness beats early
// abort): a step engine error is rendered verbatim into the report and
// counted; failed reports how many steps errored so the CLI can exit nonzero
// while the report stays complete.
func Run(ctx context.Context, opts Options, w io.Writer) (failed int, runErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	flavorHint, err := resolveFlavorHint(opts.FlavorHint)
	if err != nil {
		return 0, err
	}
	script, err := LoadScriptWithOverrides(opts.ScriptPath, ScriptOverrides{Window: opts.WindowOverride, TID: opts.TIDOverride})
	if err != nil {
		return 0, err
	}
	tracePath := strings.TrimSpace(opts.TracePath)
	if tracePath == "" {
		return 0, fmt.Errorf("tracediag: --trace <path> is required")
	}
	if abs, absErr := filepath.Abs(tracePath); absErr == nil {
		tracePath = abs
	}
	info, err := os.Stat(tracePath)
	if err != nil {
		return 0, fmt.Errorf("tracediag: trace file: %w", err)
	}
	if info.IsDir() {
		return 0, fmt.Errorf("tracediag: trace path %s is a directory, want a trace file", tracePath)
	}

	totalCap := opts.TotalMaxLines
	if totalCap <= 0 {
		totalCap = DefaultTotalMaxLines
	}
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	if script.Version == ScriptVersionV2 {
		return runV2(ctx, opts, w, script, tracePath, info, flavorHint, now())
	}

	rw := newReportWriter(w, totalCap)
	writeProvenanceHeader(rw, opts, script, tracePath, traceFileSHA256(tracePath), info, flavorHint, now())

	statuses := make([]StepStatus, 0, len(script.Steps))
	for i := range script.Steps {
		step := &script.Steps[i]
		status := StepStatus{Label: step.Label, View: step.View}
		writeStepHeader(rw, i+1, len(script.Steps), step)
		outcome := runStep(ctx, tracePath, flavorHint, step)
		if outcome.err != nil {
			status.Err = outcome.err
			// Collection discipline (§28.12): the error text is evidence too
			// — verbatim, then keep going with the remaining steps.
			rw.line(fmt.Sprintf("[步骤失败] engine error (verbatim): %v", outcome.err))
			rw.line("[步骤失败] 该步骤无结果输出;继续执行后续步骤。")
			statuses = append(statuses, status)
			continue
		}
		body := renderStepBody(step, outcome)
		status.BodyTotal = body.total
		status.BodyShown = len(body.lines)
		for _, line := range body.lines {
			rw.line(line)
		}
		if body.total > len(body.lines) {
			// Honest truncation disclosure — verbatim form pinned by tests.
			rw.line(fmt.Sprintf("…共 %d 行,按帽截断至 %d,余 %d 行未列", body.total, len(body.lines), body.total-len(body.lines)))
		}
		statuses = append(statuses, status)
	}

	writeStatusSummary(rw, statuses)
	if rw.flushErr() != nil {
		return 0, fmt.Errorf("tracediag: write report: %w", rw.flushErr())
	}
	for _, st := range statuses {
		if st.Err != nil {
			failed++
		}
	}
	return failed, nil
}

// traceFileSHA256 streams the trace once through sha256 for the provenance
// header (SEC #27: basename+size+digest replace the absolute path; the
// digest keeps exact artifact reconciliation). A read failure degrades to a
// disclosed `unavailable` token instead of aborting — collection
// completeness beats early abort, and any step would surface the same IO
// error verbatim anyway.
func traceFileSHA256(tracePath string) string {
	f, err := os.Open(tracePath)
	if err != nil {
		return "unavailable(open_error)"
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "unavailable(read_error)"
	}
	return hex.EncodeToString(h.Sum(nil))
}

func resolveFlavorHint(raw string) (tracequery.TraceFlavor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return tracequery.TraceFlavorAuto, nil
	}
	for _, v := range flavorHintEnum {
		if raw == v {
			return tracequery.TraceFlavor(raw), nil
		}
	}
	return "", fmt.Errorf("tracediag: unknown --trace-flavor %q; supported: %s", raw, strings.Join(flavorHintEnum, "|"))
}

// stepOutcome carries one executed step's engine result: exactly one of
// result / census is set on success.
type stepOutcome struct {
	result *tracequery.Result
	census *formatCensus
	err    error
}

// runStep dispatches one step to the engine through its exported API only.
func runStep(ctx context.Context, tracePath string, flavorHint tracequery.TraceFlavor, step *Step) stepOutcome {
	switch step.View {
	case "event_search":
		q := stepQuery(step, flavorHint)
		// Static and generated steps use the SAME visible-row budget. max_lines
		// caps the whole rendered step body, not merely Result.Events; handing
		// the full cap to the engine made a static report say "匹配事件 N 行"
		// while only N-3 raw rows fitted after result/window/header metadata.
		q.Limit = eventSearchEngineRowLimit(step.EffectiveMaxLines())
		// A V2 bundle (including a sibling-promoted systrace) must take the
		// indexed path. Streaming intentionally rejects composite universes and
		// therefore cannot carry their manifest-global capture-quality advisory.
		if tracequery.TracePathRequiresCompositeIndex(tracePath) {
			idx, err := buildStepIndex(ctx, tracePath, step)
			if err != nil {
				return stepOutcome{err: err}
			}
			res := tracequery.Run(idx, q)
			return stepOutcome{result: &res}
		}
		res, err := tracequery.StreamEventSearch(ctx, tracePath, q)
		if err != nil {
			return stepOutcome{err: err}
		}
		return stepOutcome{result: &res}
	case tracequery.ViewWindowSweep:
		q := stepQuery(step, flavorHint)
		res, err := tracequery.StreamWindowSweep(ctx, tracePath, q)
		if err != nil {
			return stepOutcome{err: err}
		}
		return stepOutcome{result: &res}
	case ViewFormatCensus:
		// TDIAG B2 (§28.13): the census streams the whole file through the
		// engine's exported StreamScan — no event index is built, so the
		// 250K index budget (IndexEventLimitError) cannot bound a whole-file
		// census; that error lane stays with the index-backed steps below.
		// Scope (window/line) filters inside the accumulator (交集语义).
		acc := newFormatCensusAcc(censusScopeFromStep(step))
		shell, err := tracequery.StreamScan(ctx, tracePath, flavorHint, func(ev tracequery.Event) bool {
			acc.observe(&ev)
			return true
		})
		if err != nil {
			return stepOutcome{err: err}
		}
		acc.finalize(shell)
		return stepOutcome{census: acc}
	default:
		idx, err := buildStepIndex(ctx, tracePath, step)
		if err != nil {
			return stepOutcome{err: err}
		}
		res := tracequery.Run(idx, stepQuery(step, flavorHint))
		return stepOutcome{result: &res}
	}
}

// stepQuery converts one validated step into an engine query. Mirrors the
// tool layer's construction shape (Thread doubles as ThreadInput; TimeStart/
// TimeEnd carry explicit Set flags) without importing internal/tool.
func stepQuery(step *Step, flavorHint tracequery.TraceFlavor) tracequery.Query {
	q := tracequery.Query{
		View:        step.View,
		Thread:      step.Thread,
		ThreadInput: step.Thread,
		PID:         step.PID,
		LineStart:   step.LineStart,
		LineEnd:     step.LineEnd,
		Pattern:     step.Pattern,
	}
	if start, end, ok := step.WindowBounds(); ok {
		q.TimeStart, q.TimeEnd = start, end
		q.TimeStartSet, q.TimeEndSet = true, true
	}
	for _, et := range step.EventTypes {
		q.EventTypes = append(q.EventTypes, tracequery.EventType(et))
	}
	for _, action := range step.TraceMarkActions {
		q.TraceMarkActions = append(q.TraceMarkActions, tracequery.TraceMarkAction(action))
	}
	if flavorHint != "" && flavorHint != tracequery.TraceFlavorAuto {
		q.TraceFlavorHint = flavorHint
		q.TraceFlavorHintSource = "cli_flag"
	}
	if step.windowOrigin != nil {
		q.FrameWindowAutoDerived = true
	}
	return q
}

// traceDiagWindowedIndexMinBytes mirrors the tool layer's 64MiB windowed-
// index threshold (internal/tool/trace_query.go traceQueryWindowedIndexMinBytes,
// :78; not importable — internal/tool is off-limits to this package by the
// §28.12 boundary rule). Below it a full index is cheap and correct for
// every view. Literal-pinned by TestWindowedIndexMirrorLiteralsPinned: if the
// tool side moves, that pin turns red and the mirror is re-audited.
const traceDiagWindowedIndexMinBytes int64 = 64 << 20

// traceDiagWindowedIndexLinePadding mirrors the tool layer's ±200-line
// padding around an explicit line window (internal/tool/trace_query.go
// traceQueryWindowedIndexOptions, :1510-1512). Same literal-pin regime.
const traceDiagWindowedIndexLinePadding = 200

// traceDiagWindowedIndexTimePadding mirrors the tool layer's per-view padding
// (same source as above): wakeup/interval context immediately around the
// requested window must be parsed or interval reconstruction degrades.
func traceDiagWindowedIndexTimePadding(view string) float64 {
	switch view {
	case "event_search":
		return 0.050
	case "thread_timeline", "scheduler_latency_stats":
		return 0.250
	default:
		return 0.500
	}
}

// buildStepIndex builds the engine index for index-backed steps: small traces
// take the plain full build; large traces with an explicit window take the
// windowed build so GB-scale collection stays feasible. The engine's typed
// IndexEventLimitError surfaces verbatim as a step failure — its message
// carries the narrow-the-window guidance the customer needs.
func buildStepIndex(ctx context.Context, tracePath string, step *Step) (*tracequery.Index, error) {
	info, err := os.Stat(tracePath)
	if err != nil {
		return nil, err
	}
	_, _, windowSet := step.WindowBounds()
	explicitWindow := windowSet || step.LineStart > 0 || step.LineEnd > 0
	if !traceDiagUseWindowedIndex(tracePath, info.Size(), explicitWindow) {
		if stepIndexMaxEvents > 0 {
			return tracequery.BuildIndexWithOptions(ctx, tracePath, tracequery.BuildOptions{MaxEvents: stepIndexMaxEvents})
		}
		return tracequery.BuildIndex(ctx, tracePath)
	}
	padding := traceDiagWindowedIndexTimePadding(step.View)
	start, end, _ := step.WindowBounds()
	opts := tracequery.BuildOptions{
		TimeStart:          start,
		TimeEnd:            end,
		TimeStartSet:       windowSet,
		TimeEndSet:         windowSet,
		TimePaddingBefore:  padding,
		TimePaddingAfter:   padding,
		LineStart:          step.LineStart,
		LineEnd:            step.LineEnd,
		LinePaddingBefore:  traceDiagWindowedIndexLinePadding,
		LinePaddingAfter:   traceDiagWindowedIndexLinePadding,
		AllowWindowedParse: true,
	}
	if stepIndexMaxEvents > 0 {
		opts.MaxEvents = stepIndexMaxEvents
	}
	return tracequery.BuildIndexWithOptions(ctx, tracePath, opts)
}

func traceDiagUseWindowedIndex(tracePath string, primaryBytes int64, explicitWindow bool) bool {
	if !explicitWindow {
		return false
	}
	// A bundle manifest is intentionally tiny even when its held children are
	// gigabytes. Once a composite query has an explicit window, keying this
	// decision only to the entry-file size would turn the indexed event-search
	// compatibility path into an unbounded full-child parse.
	return primaryBytes >= traceDiagWindowedIndexMinBytes || tracequery.TracePathRequiresCompositeIndex(tracePath)
}
