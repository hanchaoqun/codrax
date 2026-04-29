package tool

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// EmitPerfSegmentation is Step A of the two-step perf-triage fallback.
// Mirror of EmitLogSegmentation for the HiTrace / atrace / systrace /
// perfetto channel. When a single-shot emit_perf_trace fails (schema
// reject, low Coverage, or the input trace exceeds
// perf_two_step_bytes) the perf_triager re-dispatches with a
// segmentation-only skill and this tool: the LLM scans the raw trace
// once, identifies the byte ranges of cohesive jank-shaped regions
// (single-PID windows, single startup envelope, single hot-thread
// run), and returns the map. Step B then re-invokes emit_perf_trace
// once per actionable segment, narrowing the LLM's attention to one
// coherent block at a time. MergePerfBundles assembles the parts.
//
// Why HiTrace needs its own segmentation rather than reusing the
// log segmenter: trace events carry timestamps + per-PID + per-tag
// nesting that have no analogue in panic logs (which are
// chronological text). The kind enum below is perf-specific:
//
//	frame_window — a render-frame envelope worth scrutinising
//	jank_region  — a contiguous run of janky frames or stalls
//	startup      — process / activity startup window
//	thread_run   — long-running CPU burn or I/O block
//	context      — header / metadata / non-event banner lines
//	noise        — unrelated / non-actionable
type EmitPerfSegmentation struct {
	ReadOnly
	NonEvidenceTool
}

// PerfSegment is the payload carried on bus state.
type PerfSegment struct {
	ByteStart int    `json:"byte_start"`
	ByteEnd   int    `json:"byte_end"`
	Kind      string `json:"kind"` // frame_window | jank_region | startup | thread_run | context | noise
	Hint      string `json:"hint,omitempty"`
}

type emitPerfSegmentationParams struct {
	Segments []PerfSegment `json:"segments"`
}

func (t *EmitPerfSegmentation) Name() string { return "emit_perf_segmentation" }

func (t *EmitPerfSegmentation) Description() string {
	return "Segments the attached HiTrace / atrace / perfetto excerpt into byte-addressed " +
		"regions by kind (frame_window / jank_region / startup / thread_run / context / noise) " +
		"so the two-step extractor can focus per-segment. Call EXACTLY once per dispatch."
}

func (t *EmitPerfSegmentation) Parameters() json.RawMessage {
	emitPerfSegmentationSchemaOnce.Do(buildEmitPerfSegmentationSchema)
	return emitPerfSegmentationSchemaCache
}

var (
	emitPerfSegmentationSchemaOnce  sync.Once
	emitPerfSegmentationSchemaCache json.RawMessage
)

func buildEmitPerfSegmentationSchema() {
	kindEnum := []string{"frame_window", "jank_region", "startup", "thread_run", "context", "noise"}

	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"segments"},
		"properties": map[string]any{
			"segments": map[string]any{
				"type":     "array",
				"maxItems": 10,
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"byte_start", "byte_end", "kind"},
					"properties": map[string]any{
						"byte_start": map[string]any{"type": "integer", "minimum": 0},
						"byte_end":   map[string]any{"type": "integer", "minimum": 1},
						"kind":       map[string]any{"type": "string", "enum": kindEnum},
						"hint":       map[string]any{"type": "string", "maxLength": 80},
					},
				},
			},
		},
	}

	raw, err := json.Marshal(schema)
	if err != nil {
		emitPerfSegmentationSchemaCache = json.RawMessage(fmt.Sprintf(
			`{"type":"object","description":"emit_perf_segmentation schema build failed: %s"}`, err))
		return
	}
	emitPerfSegmentationSchemaCache = raw
}

// Execute validates segment coordinates against AttachedHitrace
// length and rejects empty / reversed / out-of-bounds ranges. Valid
// segments are stored on MutableState via SetPerfSegments. Bus
// AttachedLog is NOT consulted here — perf segmenter operates on
// the trace channel exclusively.
func (t *EmitPerfSegmentation) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	if ctx == nil || ctx.Mutable == nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_perf_segmentation requires BusContext.Mutable",
			Timestamp: time.Now(),
		}, nil
	}

	var p emitPerfSegmentationParams
	if err := json.Unmarshal(params, &p); err != nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("invalid params: %v", err),
			Timestamp: time.Now(),
		}, err
	}

	traceLen := len(ctx.AttachedHitrace)
	kept := make([]PerfSegment, 0, len(p.Segments))
	normalized := 0
	for _, s := range p.Segments {
		start, end, changed, ok := normalizeSegmentationRange(s.ByteStart, s.ByteEnd, traceLen)
		if !ok {
			continue
		}
		if changed {
			normalized++
		}
		s.ByteStart = start
		s.ByteEnd = end
		// Filter noise + context segments — Step B only re-dispatches
		// emit_perf_trace on actionable segments. Keeping the noise
		// segment on bus is fine for diagnostics but the controller
		// also filters at dispatch time (see isExtractablePerfSegment).
		kept = append(kept, s)
	}
	if len(kept) == 0 {
		return segmentationCoordinateRejected(t.Name(), "performance trace", traceLen, len(p.Segments)), nil
	}

	raw, err := json.Marshal(kept)
	if err != nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("segment marshal failed: %v", err),
			Timestamp: time.Now(),
		}, err
	}
	ctx.Mutable.SetPerfSegments(raw)

	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   fmt.Sprintf("[emit_perf_segmentation: kept=%d bytes=%d normalized=%d]\nsegments recorded", len(kept), traceLen, normalized),
		Timestamp: time.Now(),
	}, nil
}

// IsExtractablePerfSegment reports whether a segment kind warrants a
// Step-B emit_perf_trace dispatch. context / noise are recorded for
// diagnostics but skipped to save LLM calls. Exported because the
// agent two-step controller calls it.
func IsExtractablePerfSegment(kind string) bool {
	switch kind {
	case "frame_window", "jank_region", "startup", "thread_run":
		return true
	}
	return false
}
