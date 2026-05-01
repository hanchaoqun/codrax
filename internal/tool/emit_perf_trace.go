package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

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
	Meta    emitPerfTraceMeta    `json:"meta"`
	Frames  []emitPerfTraceFrame `json:"frames,omitempty"`
	Janks   []emitPerfTraceJank  `json:"janks,omitempty"`
	Stalls  []emitPerfTraceStall `json:"stalls,omitempty"`
	Startup *emitPerfTraceStartup `json:"startup,omitempty"`
	Residue []string              `json:"residue,omitempty"`
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

// Name returns the tool's stable identifier.
func (t *EmitPerfTrace) Name() string { return "emit_perf_trace" }

// Description — one sentence. Strategy guidance lives in
// perf-triage-skill.
func (t *EmitPerfTrace) Description() string {
	return "Emits the structured PerfBundle extracted from the attached HiTrace / atrace / systrace / perfetto text excerpt. " +
		"Call EXACTLY once per dispatch. The system derives ResolvedFiles / Entities / IntentHint / Coverage automatically — " +
		"do not try to resolve paths yourself."
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
			Summary:   "emit_perf_trace requires BusContext.Mutable",
			Timestamp: time.Now(),
		}, nil
	}

	var p emitPerfTraceParams
	dec := json.NewDecoder(strings.NewReader(string(params)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("invalid params: %v", err),
			Timestamp: time.Now(),
		}, err
	}

	// Cross-field sanity: at least one of frames/janks/stalls/startup
	// must be populated. An all-empty emission means the LLM saw
	// nothing useful — better to fail loud and retry than store a
	// vacuous bundle.
	if len(p.Frames) == 0 && len(p.Janks) == 0 && len(p.Stalls) == 0 && p.Startup == nil {
		return types.ToolResult{
			ToolName: t.Name(), Success: false,
			Summary:   "emit_perf_trace rejected: frames / janks / stalls / startup all empty",
			Timestamp: time.Now(),
		}, nil
	}

	bundle := toPerfBundle(&p)

	derivePerfLayer4(bundle)
	ctx.Mutable.SetPerfTrace(bundle)

	logging.Debug("[emit_perf_trace] stored: frames=%d janks=%d stalls=%d signals=%d intent=%q",
		len(bundle.Frames), len(bundle.Janks), len(bundle.Stalls),
		len(bundle.Meta.Signals), bundle.IntentHint)

	return types.ToolResult{
		ToolName: t.Name(),
		Success:  true,
		Summary: fmt.Sprintf(
			"perf bundle stored: frames=%d janks=%d stalls=%d signals=%d intent=%s",
			len(bundle.Frames), len(bundle.Janks), len(bundle.Stalls),
			len(bundle.Meta.Signals), bundle.IntentHint),
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
	return &types.PerfBundle{
		Meta: types.PerfMeta{
			Source: p.Meta.Source, DurationMs: p.Meta.DurationMs,
			AppPID: p.Meta.AppPID, Signals: append([]string(nil), p.Meta.Signals...),
			Summary: p.Meta.Summary,
		},
		Frames: frames, Janks: janks, Stalls: stalls,
		Startup: startup, Residue: append([]string(nil), p.Residue...),
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
			"frames":  map[string]any{"type": "array", "maxItems": 200, "items": frameSchema},
			"janks":   map[string]any{"type": "array", "maxItems": 50, "items": jankSchema},
			"stalls":  map[string]any{"type": "array", "maxItems": 50, "items": stallSchema},
			"startup": startupSchema,
			"residue": map[string]any{"type": "array", "maxItems": 8, "items": map[string]any{"type": "string", "maxLength": 500}},
		},
	}
}

