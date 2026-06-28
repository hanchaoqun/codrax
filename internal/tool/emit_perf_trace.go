package tool

import (
	"bufio"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/analysis/logtriage"
	"github.com/hanchaoqun/codrax/internal/analysis/perftriage"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// EmitPerfTrace is the perf_triager agent's structured exit channel.
// The LLM reads the attached HiTrace / Android systrace / perfetto
// text excerpt and emits a bundle carrying Layer 1 (Meta: source +
// signals + duration + summary), Layer 2 (Frames / Janks / Stalls /
// Startup) and Layer 3 (Residue: unknown_chunks). Layer 4
// (ResolvedFiles / Entities / IntentHint / Coverage) is system-derived
// by derivePerfLayer4 — the LLM has no channel to write it because
// those fields are not in the tool's JSON schema.
//
// Mirror of EmitLogTriage for the performance channel. Classified
// ReadOnly + NonEvidenceTool for the same reasons: the emission
// mutates BusContext, not the filesystem, and the payload is a
// classification / routing artefact, not a citable repo fact.
type EmitPerfTrace struct {
	ReadOnly
	NonEvidenceTool
}

// emitPerfTraceParams is the wire shape of the emit. It carries
// EXACTLY Layer 1-3; the post-handler derives Layer 4.
type emitPerfTraceParams struct {
	Meta         emitPerfTraceMeta          `json:"meta"`
	Frames       []emitPerfTraceFrame       `json:"frames,omitempty"`
	Janks        []emitPerfTraceJank        `json:"janks,omitempty"`
	Stalls       []emitPerfTraceStall       `json:"stalls,omitempty"`
	Startup      *emitPerfTraceStartup      `json:"startup,omitempty"`
	Observations []emitPerfTraceObservation `json:"observations,omitempty"`
	Residue      []string                   `json:"residue,omitempty"`
}

type emitPerfTraceMeta struct {
	Source     string   `json:"source"`
	DurationMs float64  `json:"duration_ms,omitempty"`
	AppPID     int      `json:"app_pid,omitempty"`
	Signals    []string `json:"signals,omitempty"`
	Summary    string   `json:"summary,omitempty"`
}

type emitPerfTraceFrame struct {
	FrameNo    int     `json:"frame_no,omitempty"`
	TsMs       float64 `json:"ts_ms,omitempty"`
	DurationMs float64 `json:"duration_ms"`
	Phase      string  `json:"phase,omitempty"`
	Janky      bool    `json:"janky,omitempty"`
}

type emitPerfTraceJank struct {
	StartTsMs   float64  `json:"start_ts_ms"`
	DurationMs  float64  `json:"duration_ms"`
	TriggerSpan string   `json:"trigger_span,omitempty"`
	Reason      string   `json:"reason,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

type emitPerfTraceStall struct {
	StartTsMs  float64 `json:"start_ts_ms"`
	DurationMs float64 `json:"duration_ms"`
	Kind       string  `json:"kind,omitempty"`
	Symbol     string  `json:"symbol,omitempty"`
	File       string  `json:"file,omitempty"`
	Line       int     `json:"line,omitempty"`
}

type emitPerfTraceStartup struct {
	Mode          string  `json:"mode"`
	AppLaunchMs   float64 `json:"app_launch_ms,omitempty"`
	AbilityInitMs float64 `json:"ability_init_ms,omitempty"`
	FirstFrameMs  float64 `json:"first_frame_ms,omitempty"`
}

type emitPerfTraceObservation struct {
	Kind       string   `json:"kind,omitempty"`
	Subject    string   `json:"subject"`
	Summary    string   `json:"summary"`
	Evidence   string   `json:"evidence,omitempty"`
	LineStart  int      `json:"line_start,omitempty"`
	LineEnd    int      `json:"line_end,omitempty"`
	StartTsMs  float64  `json:"start_ts_ms,omitempty"`
	EndTsMs    float64  `json:"end_ts_ms,omitempty"`
	DurationMs float64  `json:"duration_ms,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Confidence float64  `json:"confidence,omitempty"`
}

// Name returns the tool's stable identifier.
func (t *EmitPerfTrace) Name() string { return "emit_perf_trace" }

// Description — one sentence. Strategy guidance lives in
// perf-triage-skill.
func (t *EmitPerfTrace) Description() string {
	return "Emits the structured PerfBundle extracted from the attached HiTrace / atrace / systrace / perfetto text excerpt. " +
		"Call EXACTLY once per dispatch. The system derives ResolvedFiles / Entities / IntentHint / Coverage automatically — " +
		"do not try to resolve paths yourself. For perf_sample rows, distinguish sample CPU scope from scheduler event CPU: cpu=-1, cpu_known=false, or sample_kind=off_cpu means the sample is not tied to a concrete CPU/core, even when the surrounding ftrace row has a [CPU] column."
}

// Parameters returns the strict JSON schema.
func (t *EmitPerfTrace) Parameters() json.RawMessage {
	emitPerfTraceSchemaOnce.Do(buildEmitPerfTraceSchemaBytes)
	return emitPerfTraceSchemaCache
}

var (
	emitPerfTraceSchemaOnce  sync.Once
	emitPerfTraceSchemaCache json.RawMessage
)

func buildEmitPerfTraceSchemaBytes() {
	schema := buildEmitPerfTraceSchema()
	raw, err := json.Marshal(schema)
	if err != nil {
		emitPerfTraceSchemaCache = json.RawMessage(fmt.Sprintf(
			`{"type":"object","description":"emit_perf_trace schema build failed: %s"}`, err))
		return
	}
	emitPerfTraceSchemaCache = raw
}

// Execute parses + validates + stores the PerfBundle on MutableState.
func (t *EmitPerfTrace) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	if ctx == nil || ctx.Mutable == nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_perf_trace requires a writable context",
			Timestamp: time.Now(),
		}, nil
	}

	params = applyStructuredPayloadCompat(t.Name(), params, t.Parameters())

	var p emitPerfTraceParams
	dec := json.NewDecoder(strings.NewReader(string(params)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return failStrictDecodeWithError(t.Name(), time.Now(), err, emitPerfTraceMisplacedHints, params)
	}

	// Cross-field sanity: at least one structured trace fact must be
	// populated. observations[] covers user-relevant trace facts that are
	// not jank/stall/startup, such as line/order and threshold checks.
	if len(p.Frames) == 0 && len(p.Janks) == 0 && len(p.Stalls) == 0 &&
		p.Startup == nil && len(p.Observations) == 0 {
		return types.ToolResult{
			ToolName: t.Name(), Success: false,
			Summary:   "emit_perf_trace rejected: frames / janks / stalls / startup / observations all empty",
			Timestamp: time.Now(),
		}, nil
	}

	bundle := toPerfBundle(&p)
	augmentPerfBundleWithTimeSemantics(bundle, ctx)
	augmentPerfBundleWithPrioritySemantics(bundle, ctx, p.Meta.Source)
	normalizeHarmonyPriorityClaims(bundle, ctx, p.Meta.Source)

	// P-TypedDenials Phase B (2026-05-08): mirror log_triage's frame
	// corroborate gate on the perf side. PerfStall has the SAME
	// (File, Symbol) typed pair the log gate validates — a stall
	// claiming to fire in real_path.go::someSymbol is rejected when
	// the real file does not contain that symbol identifier.
	// Pre-Phase-B perf had NO such gate (acknowledged in
	// internal/analysis/perftriage/merge.go's package doc), letting
	// LLM-emitted stalls land File / Symbol pairs that grounded into
	// unrelated real-repo files (same root cause as the
	// logtri_goroutine_dump hallucination class).
	clearedStalls := perftriage.CorroborateStallFiles(bundle, ctx.RepoRoot)
	for _, cs := range clearedStalls {
		ctx.TypedDenials.Add(types.TypedDenial{
			Class:  types.TypedDenialExternalPerfStallUnresolved,
			Token:  cs.OriginalFile,
			Reason: fmt.Sprintf("perf stall symbol %q does not appear in %s", cs.Symbol, cs.OriginalFile),
		})
		// Also stamp the symbol token if it was the offending axis,
		// so the symbol oracle / hallucination validator side also
		// refuses to validate it as a real symbol.
		if cs.Symbol != "" {
			ctx.TypedDenials.Add(types.TypedDenial{
				Class:  types.TypedDenialExternalPerfStallUnresolved,
				Token:  cs.Symbol,
				Reason: fmt.Sprintf("perf stall symbol %q absent from %s", cs.Symbol, cs.OriginalFile),
			})
		}
	}

	// P-BugClass Phase E.3 (2026-05-08): bug-pattern detection on the
	// raw attached perf trace. Same registry the log channel uses —
	// some failure signatures (deadlock, race, resource_exhaustion)
	// also appear in trace tags / panic dumps embedded in trace
	// streams. Empty result on benign performance traces is the
	// common case (no false positives on healthy traces).
	if bundle.Meta.BugClasses == nil {
		if detected := logtriage.DetectBugClasses(ctx.AttachedHitrace); len(detected) > 0 {
			bundle.Meta.BugClasses = detected
		}
	}

	derivePerfLayer4(bundle)
	ctx.Mutable.SetPerfTrace(bundle)

	logging.Debug("[emit_perf_trace] stored: frames=%d janks=%d stalls=%d signals=%d intent=%q",
		len(bundle.Frames), len(bundle.Janks), len(bundle.Stalls),
		len(bundle.Meta.Signals), bundle.IntentHint)

	return types.ToolResult{
		ToolName: t.Name(),
		Success:  true,
		Summary: fmt.Sprintf(
			"[emit_perf_trace: frames=%d janks=%d stalls=%d observations=%d signals=%d intent=%s evidence_origin=%s]\nperf bundle stored",
			len(bundle.Frames), len(bundle.Janks), len(bundle.Stalls),
			len(bundle.Observations), len(bundle.Meta.Signals), bundle.IntentHint, string(types.AnswerEvidenceOriginRuntimeArtifact)),
		Timestamp: time.Now(),
	}, nil
}

func toPerfBundle(p *emitPerfTraceParams) *types.PerfBundle {
	frames := make([]types.PerfFrame, len(p.Frames))
	for i, f := range p.Frames {
		frames[i] = types.PerfFrame{
			FrameNo: f.FrameNo, TsMs: f.TsMs, DurationMs: f.DurationMs,
			Phase: f.Phase, Janky: f.Janky,
		}
		if frames[i].DurationMs > types.PerfFrameBudget60HzMs {
			frames[i].Janky = true
		}
	}
	janks := make([]types.PerfJank, len(p.Janks))
	for i, j := range p.Janks {
		janks[i] = types.PerfJank{
			StartTsMs: j.StartTsMs, DurationMs: j.DurationMs,
			TriggerSpan: j.TriggerSpan, Reason: j.Reason, Tags: j.Tags,
		}
	}
	stalls := make([]types.PerfStall, len(p.Stalls))
	for i, s := range p.Stalls {
		stalls[i] = types.PerfStall{
			StartTsMs: s.StartTsMs, DurationMs: s.DurationMs,
			Kind: s.Kind, Symbol: s.Symbol, File: s.File, Line: s.Line,
		}
	}
	var startup *types.PerfStartup
	if p.Startup != nil {
		startup = &types.PerfStartup{
			Mode: p.Startup.Mode, AppLaunchMs: p.Startup.AppLaunchMs,
			AbilityInitMs: p.Startup.AbilityInitMs, FirstFrameMs: p.Startup.FirstFrameMs,
		}
	}
	observations := make([]types.PerfObservation, len(p.Observations))
	for i, obs := range p.Observations {
		observations[i] = types.PerfObservation{
			Kind: obs.Kind, Subject: obs.Subject, Summary: obs.Summary,
			Evidence: obs.Evidence, LineStart: obs.LineStart, LineEnd: obs.LineEnd,
			StartTsMs: obs.StartTsMs, EndTsMs: obs.EndTsMs, DurationMs: obs.DurationMs,
			Tags: append([]string(nil), obs.Tags...), Confidence: obs.Confidence,
		}
	}
	return &types.PerfBundle{
		Meta: types.PerfMeta{
			Source: p.Meta.Source, DurationMs: p.Meta.DurationMs,
			AppPID: p.Meta.AppPID, Signals: append([]string(nil), p.Meta.Signals...),
			Summary: p.Meta.Summary,
		},
		Frames: frames, Janks: janks, Stalls: stalls,
		Startup: startup, Observations: observations, Residue: append([]string(nil), p.Residue...),
		Coverage: 1.0,
	}
}

// derivePerfLayer4 populates Entities, IntentHint, ResolvedFiles,
// and augments Signals from Layer 1-3 content. See PerfBundle doc.
func derivePerfLayer4(b *types.PerfBundle) {
	if b == nil {
		return
	}
	// IntentHint: any jank or stall or slow cold-start promotes to
	// "performance".
	if len(b.Janks) > 0 || len(b.Stalls) > 0 ||
		(b.Startup != nil && b.Startup.AppLaunchMs > types.PerfStartupSlowColdMs) {
		b.IntentHint = "performance"
	}

	// Entities cap at 32.
	seen := map[string]bool{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] || len(b.Entities) >= 32 {
			return
		}
		seen[s] = true
		b.Entities = append(b.Entities, s)
	}
	for _, j := range b.Janks {
		add(j.TriggerSpan)
		for _, t := range j.Tags {
			add(t)
		}
	}
	for _, s := range b.Stalls {
		add(s.Symbol)
		if s.Kind != "" {
			add(s.Kind)
		}
	}
	if b.Startup != nil && b.Startup.Mode != "" {
		add(b.Startup.Mode + "-start")
	}
	for _, obs := range b.Observations {
		add(obs.Subject)
		for _, tag := range obs.Tags {
			add(tag)
		}
	}

	// Signals augmentation.
	sigSeen := map[string]bool{}
	for _, s := range b.Meta.Signals {
		sigSeen[s] = true
	}
	pushSig := func(s string) {
		if sigSeen[s] {
			return
		}
		sigSeen[s] = true
		b.Meta.Signals = append(b.Meta.Signals, s)
	}
	if len(b.Janks) > 0 {
		pushSig("jank")
	}
	for _, s := range b.Stalls {
		if s.DurationMs >= types.PerfMainThreadStallMs {
			pushSig("main-thread-stall")
			break
		}
	}
	if b.Startup != nil && b.Startup.Mode == "cold" &&
		b.Startup.AppLaunchMs > types.PerfStartupSlowColdMs {
		pushSig("cold-start-slow")
	}

	// ResolvedFiles cap at 10.
	fileSeen := map[string]bool{}
	for _, s := range b.Stalls {
		if s.File == "" || fileSeen[s.File] {
			continue
		}
		fileSeen[s.File] = true
		if len(b.ResolvedFiles) >= 10 {
			break
		}
		b.ResolvedFiles = append(b.ResolvedFiles, s.File)
	}

	if b.Coverage < 0 {
		b.Coverage = 0
	}
	if b.Coverage > 1 {
		b.Coverage = 1
	}
}

var perfTracePrioRE = regexp.MustCompile(`(?:^|[[:space:]])(?:prev_prio|next_prio|prio)=([0-9]+)(?:\b|$)`)
var perfTracePrioTextRE = regexp.MustCompile(`(?:^|[^A-Za-z0-9_])prio=([0-9]+)(?:\b|$)`)
var perfTraceTimestampRE = regexp.MustCompile(`\[[0-9]+\]\s+\S+\s+([0-9]+\.[0-9]+):`)

func augmentPerfBundleWithPrioritySemantics(b *types.PerfBundle, ctx *types.BusContext, source string) {
	if b == nil || ctx == nil || !perfTraceLooksHarmony(ctx, source) {
		return
	}
	classes, firstLine, lastLine := harmonyPriorityClassesFromTrace(ctx.AttachedHitrace)
	if len(classes) == 0 {
		return
	}
	summary := "Harmony priority semantics: 数值越大优先级越高/larger numeric value is higher; 1-40=CFS, 41-139=RT. Observed classes: " + strings.Join(classes, ", ") + "."
	obs := types.PerfObservation{
		Kind:       "priority_semantics",
		Subject:    "HarmonyOS priority semantics",
		Summary:    summary,
		Evidence:   "system-derived from attached trace prio fields",
		LineStart:  firstLine,
		LineEnd:    lastLine,
		Tags:       append([]string{"harmony_priority"}, classes...),
		Confidence: 1,
	}
	prependPerfObservation(b, obs)
}

func normalizeHarmonyPriorityClaims(b *types.PerfBundle, ctx *types.BusContext, source string) {
	if b == nil || ctx == nil || !perfTraceLooksHarmony(ctx, source) {
		return
	}
	classes, firstLine, lastLine := harmonyPriorityClassesFromTrace(ctx.AttachedHitrace)
	if len(classes) == 0 {
		return
	}
	summary := "Harmony priority class normalized from raw prio fields: " + strings.Join(classes, ", ") + ". Rule: 数值越大优先级越高; 1-40=CFS, 41-139=RT."
	if harmonyPriorityTextContradicts(b.Meta.Summary) {
		b.Meta.Summary = summary
	}
	for i := range b.Observations {
		obs := &b.Observations[i]
		if obs.Kind == "priority_semantics" || obs.Kind == "time_semantics" {
			continue
		}
		text := strings.Join([]string{obs.Subject, obs.Summary, obs.Evidence, strings.Join(obs.Tags, " ")}, " ")
		if !harmonyPriorityTextContradicts(text) {
			continue
		}
		obs.Kind = "priority_semantics_normalized"
		obs.Subject = "HarmonyOS priority semantics"
		obs.Summary = summary
		obs.Evidence = "system-normalized conflicting model-authored priority class label; raw event timing remains available in adjacent observations"
		if obs.LineStart == 0 {
			obs.LineStart = firstLine
		}
		if obs.LineEnd == 0 {
			obs.LineEnd = lastLine
		}
		obs.Tags = append([]string{"harmony_priority_normalized"}, classes...)
		if obs.Confidence <= 0 || obs.Confidence > 1 {
			obs.Confidence = 1
		}
	}
}

func augmentPerfBundleWithTimeSemantics(b *types.PerfBundle, ctx *types.BusContext) {
	if b == nil || ctx == nil {
		return
	}
	first, last, firstLine, lastLine, ok := traceTimestampWindowFromTrace(ctx.AttachedHitrace)
	if !ok || last < first {
		return
	}
	durationMs := (last - first) * 1000
	summary := fmt.Sprintf("Trace timestamps are seconds; attached excerpt spans %.6fs..%.6fs = %s.", first, last, formatTraceDuration(durationMs))
	obs := types.PerfObservation{
		Kind:       "time_semantics",
		Subject:    "Trace timestamp unit",
		Summary:    summary,
		Evidence:   "system-derived from attached trace timestamp fields",
		LineStart:  firstLine,
		LineEnd:    lastLine,
		StartTsMs:  first * 1000,
		EndTsMs:    last * 1000,
		DurationMs: durationMs,
		Tags:       []string{"trace_time_seconds"},
		Confidence: 1,
	}
	prependPerfObservation(b, obs)
}

func prependPerfObservation(b *types.PerfBundle, obs types.PerfObservation) {
	if b == nil {
		return
	}
	b.Observations = append([]types.PerfObservation{obs}, b.Observations...)
	if len(b.Observations) > 50 {
		b.Observations = b.Observations[:50]
	}
}

func perfTraceLooksHarmony(ctx *types.BusContext, source string) bool {
	source = strings.ToLower(strings.TrimSpace(source))
	if source == "hitrace" {
		return true
	}
	var text string
	if ctx != nil {
		objective := ""
		if ctx.Mutable != nil {
			objective = ctx.Mutable.Objective()
		}
		text = strings.ToLower(strings.TrimSpace(ctx.AttachedHitraceSource + " " + objective))
	}
	for _, marker := range []string{"harmony", "openharmony", "ohos", "hitrace", "bytrace", "鸿蒙", "东湖"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func harmonyPriorityClassesFromTrace(trace string) ([]string, int, int) {
	if strings.TrimSpace(trace) == "" {
		return nil, 0, 0
	}
	seen := map[int]bool{}
	firstLine, lastLine := 0, 0
	scanner := newPerfTraceLineScanner(trace)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		for _, match := range perfTracePrioRE.FindAllStringSubmatch(line, -1) {
			if len(match) < 2 {
				continue
			}
			var prio int
			if _, err := fmt.Sscanf(match[1], "%d", &prio); err != nil || prio <= 0 {
				continue
			}
			seen[prio] = true
			if firstLine == 0 {
				firstLine = lineNo
			}
			lastLine = lineNo
		}
	}
	if len(seen) == 0 {
		return nil, 0, 0
	}
	prios := make([]int, 0, len(seen))
	for prio := range seen {
		prios = append(prios, prio)
	}
	sort.Ints(prios)
	classes := make([]string, 0, min(len(prios), 12))
	for _, prio := range prios {
		if len(classes) >= 12 {
			break
		}
		classes = append(classes, fmt.Sprintf("prio=%d/%s", prio, harmonyPriorityClass(prio)))
	}
	return classes, firstLine, lastLine
}

func harmonyPriorityTextContradicts(text string) bool {
	lower := strings.ToLower(text)
	if !strings.Contains(lower, "prio=") {
		return false
	}
	for _, match := range perfTracePrioTextRE.FindAllStringSubmatchIndex(lower, -1) {
		if len(match) < 4 {
			continue
		}
		raw := lower[match[2]:match[3]]
		prio, err := strconv.Atoi(raw)
		if err != nil || prio <= 0 {
			continue
		}
		start := max(0, match[0]-96)
		end := min(len(lower), match[1]+96)
		window := lower[start:end]
		switch harmonyPriorityClass(prio) {
		case "ohos_rt":
			if containsPriorityClassToken(window, "cfs") || strings.Contains(window, "nice=0") || strings.Contains(window, "基准") {
				return true
			}
		case "ohos_cfs":
			if containsPriorityClassToken(window, "rt") || strings.Contains(window, "实时") {
				return true
			}
		}
	}
	return false
}

func containsPriorityClassToken(text, token string) bool {
	if token == "" {
		return false
	}
	for _, idx := range allStringIndexes(text, token) {
		beforeOK := idx == 0 || !isASCIIAlphaNum(rune(text[idx-1]))
		afterIdx := idx + len(token)
		afterOK := afterIdx >= len(text) || !isASCIIAlphaNum(rune(text[afterIdx]))
		if beforeOK && afterOK {
			return true
		}
	}
	return false
}

func allStringIndexes(text, token string) []int {
	var out []int
	offset := 0
	for {
		idx := strings.Index(text[offset:], token)
		if idx < 0 {
			return out
		}
		abs := offset + idx
		out = append(out, abs)
		offset = abs + len(token)
	}
}

func traceTimestampWindowFromTrace(trace string) (float64, float64, int, int, bool) {
	if strings.TrimSpace(trace) == "" {
		return 0, 0, 0, 0, false
	}
	var first, last float64
	firstLine, lastLine := 0, 0
	scanner := newPerfTraceLineScanner(trace)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		match := perfTraceTimestampRE.FindStringSubmatch(scanner.Text())
		if len(match) < 2 {
			continue
		}
		ts, err := strconv.ParseFloat(match[1], 64)
		if err != nil {
			continue
		}
		if firstLine == 0 {
			first = ts
			firstLine = lineNo
		}
		last = ts
		lastLine = lineNo
	}
	if firstLine == 0 {
		return 0, 0, 0, 0, false
	}
	return first, last, firstLine, lastLine, true
}

func newPerfTraceLineScanner(trace string) *bufio.Scanner {
	scanner := bufio.NewScanner(strings.NewReader(trace))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	return scanner
}

func formatTraceDuration(durationMs float64) string {
	switch {
	case durationMs < 0:
		return "unknown duration"
	case durationMs < 1:
		return fmt.Sprintf("%.3fms (%.0fus)", durationMs, durationMs*1000)
	case durationMs < 1000:
		return fmt.Sprintf("%.3fms", durationMs)
	default:
		return fmt.Sprintf("%.3fs", durationMs/1000)
	}
}

func harmonyPriorityClass(prio int) string {
	switch {
	case prio >= 1 && prio <= 40:
		return "ohos_cfs"
	case prio >= 41 && prio <= 139:
		return "ohos_rt"
	default:
		return "system_or_kernel_raw"
	}
}

// buildEmitPerfTraceSchema assembles the JSON-schema the LLM sees.
func buildEmitPerfTraceSchema() map[string]any {
	frameSchema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"duration_ms"},
		"properties": map[string]any{
			"frame_no":    map[string]any{"type": "integer", "minimum": 0},
			"ts_ms":       map[string]any{"type": "number"},
			"duration_ms": map[string]any{"type": "number", "minimum": 0},
			"phase":       map[string]any{"type": "string", "enum": []string{"", "measure", "layout", "draw", "composite"}},
			"janky":       map[string]any{"type": "boolean"},
		},
	}
	jankSchema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"start_ts_ms", "duration_ms"},
		"properties": map[string]any{
			"start_ts_ms":  map[string]any{"type": "number"},
			"duration_ms":  map[string]any{"type": "number", "minimum": 0},
			"trigger_span": map[string]any{"type": "string", "maxLength": 120},
			"reason":       map[string]any{"type": "string", "enum": []string{"", "io", "lock", "sync-call", "heavy-compute"}},
			"tags":         map[string]any{"type": "array", "maxItems": 16, "items": map[string]any{"type": "string", "maxLength": 120}},
		},
	}
	stallSchema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"start_ts_ms", "duration_ms"},
		"properties": map[string]any{
			"start_ts_ms": map[string]any{"type": "number"},
			"duration_ms": map[string]any{"type": "number", "minimum": 0},
			"kind":        map[string]any{"type": "string", "enum": []string{"", "io", "lock", "sync-rpc", "native-call"}},
			"symbol":      map[string]any{"type": "string", "maxLength": 120},
			"file":        map[string]any{"type": "string"},
			"line":        map[string]any{"type": "integer", "minimum": 0},
		},
	}
	startupSchema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"mode"},
		"properties": map[string]any{
			"mode":            map[string]any{"type": "string", "enum": []string{"cold", "warm", "hot"}},
			"app_launch_ms":   map[string]any{"type": "number", "minimum": 0},
			"ability_init_ms": map[string]any{"type": "number", "minimum": 0},
			"first_frame_ms":  map[string]any{"type": "number", "minimum": 0},
		},
	}
	observationSchema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"subject", "summary"},
		"properties": map[string]any{
			"kind":        map[string]any{"type": "string", "maxLength": 64, "description": "trace-local fact type, e.g. span, threshold_check, line_anchor, duration, absence"},
			"subject":     map[string]any{"type": "string", "maxLength": 120},
			"summary":     map[string]any{"type": "string", "maxLength": 300, "description": "Concise trace-local fact. For perf_sample facts, keep sample CPU scope separate from scheduler event CPU; cpu=-1/cpu_known=false/sample_kind=off_cpu cannot support concrete CPU/core sample attribution."},
			"evidence":    map[string]any{"type": "string", "maxLength": 300, "description": "Verbatim key fields from the event row. Include cpu_known/sample_kind/weight_unit when present so downstream consumers do not infer sample CPU or time units from nearby scheduler rows."},
			"line_start":  map[string]any{"type": "integer", "minimum": 0},
			"line_end":    map[string]any{"type": "integer", "minimum": 0},
			"start_ts_ms": map[string]any{"type": "number"},
			"end_ts_ms":   map[string]any{"type": "number"},
			"duration_ms": map[string]any{"type": "number", "minimum": 0},
			"tags":        map[string]any{"type": "array", "maxItems": 16, "items": map[string]any{"type": "string", "maxLength": 120}},
			"confidence":  map[string]any{"type": "number", "minimum": 0, "maximum": 1},
		},
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"meta"},
		"properties": map[string]any{
			"meta": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"source"},
				"properties": map[string]any{
					"source": map[string]any{
						"type": "string",
						"enum": []string{"hitrace", "atrace", "systrace", "perfetto", "unknown"},
					},
					"duration_ms": map[string]any{"type": "number", "minimum": 0},
					"app_pid":     map[string]any{"type": "integer", "minimum": 0},
					"signals": map[string]any{
						"type": "array", "maxItems": 8,
						"items": map[string]any{
							"type": "string",
							"enum": []string{"jank", "cold-start-slow", "main-thread-stall", "io-block", "gc-pause", "render-miss"},
						},
					},
					"summary": map[string]any{"type": "string", "maxLength": 200},
				},
			},
			"frames":       map[string]any{"type": "array", "maxItems": 200, "items": frameSchema},
			"janks":        map[string]any{"type": "array", "maxItems": 50, "items": jankSchema},
			"stalls":       map[string]any{"type": "array", "maxItems": 50, "items": stallSchema},
			"startup":      startupSchema,
			"observations": map[string]any{"type": "array", "maxItems": 50, "items": observationSchema},
			"residue":      map[string]any{"type": "array", "maxItems": 8, "items": map[string]any{"type": "string", "maxLength": 500}},
		},
	}
}

// emitPerfTraceMisplacedHints pre-maps the LLM-frequent shape drifts for
// this tool so a strict-decode rejection carries field-precise repair
// guidance instead of a bare parser error.
var emitPerfTraceMisplacedHints = []MisplacedFieldHint{
	{
		Field:          "stalls",
		ContainerNames: []string{"the top-level tool payload"},
		CorrectPaths:   []string{"stalls (a JSON array of stall objects, not a JSON-encoded string)"},
	},
	{
		Field:          "frames",
		ContainerNames: []string{"the top-level tool payload"},
		CorrectPaths:   []string{"frames (a JSON array of frame objects, not a JSON-encoded string)"},
	},
	{
		Field:          "observations",
		ContainerNames: []string{"the top-level tool payload"},
		CorrectPaths:   []string{"observations (a JSON array of observation objects, not a JSON-encoded string)"},
	},
}
