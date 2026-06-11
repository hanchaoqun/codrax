package tool

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// EmitLogSegmentation is Step A of the two-step log-triage fallback.
// When a single-shot emit_log_triage fails (schema reject, low
// Coverage, or the input log exceeds log_triage_twostep_bytes) the
// log_triager agent re-dispatches with a segmentation-only skill and
// this tool: the LLM scans the raw log once, identifies the byte
// ranges that look like stacks / headers / trace blocks, and returns
// the map. Step B then re-invokes emit_log_triage once per
// stack-shaped segment, narrowing the LLM's attention to one cohesive
// block at a time. MergeBundles assembles the parts into one bundle.
//
// The tool writes its output onto BusContext.Mutable via a dedicated
// accessor: the two-step controller reads the segments back after the
// tool call instead of rebuilding them from the ToolResult.Summary.
type EmitLogSegmentation struct {
	ReadOnly
	NonEvidenceTool
}

// LogSegment is the payload carried on bus state. Exported so the
// two-step controller (in internal/agent) can type-assert.
type LogSegment struct {
	ByteStart int    `json:"byte_start"`
	ByteEnd   int    `json:"byte_end"`
	Kind      string `json:"kind"` // stack | caused_by | header | context | trace | noise
	Hint      string `json:"hint,omitempty"`
}

type emitLogSegmentationParams struct {
	Segments []LogSegment `json:"segments"`
}

func (t *EmitLogSegmentation) Name() string { return "emit_log_segmentation" }

func (t *EmitLogSegmentation) Description() string {
	return "Segments the attached runtime log into byte-addressed regions by kind " +
		"(stack / caused_by / header / context / trace / noise) so the two-step " +
		"extractor can focus per-segment. Call EXACTLY once per dispatch."
}

func (t *EmitLogSegmentation) Parameters() json.RawMessage {
	emitLogSegmentationSchemaOnce.Do(buildEmitLogSegmentationSchema)
	return emitLogSegmentationSchemaCache
}

var (
	emitLogSegmentationSchemaOnce  sync.Once
	emitLogSegmentationSchemaCache json.RawMessage
)

func buildEmitLogSegmentationSchema() {
	kindEnum := []string{"stack", "caused_by", "header", "context", "trace", "noise"}

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
		emitLogSegmentationSchemaCache = json.RawMessage(fmt.Sprintf(
			`{"type":"object","description":"emit_log_segmentation schema build failed: %s"}`, err))
		return
	}
	emitLogSegmentationSchemaCache = raw
}

// Execute validates segment coordinates against the attached log
// length and rejects empty / reversed / out-of-bounds ranges. Valid
// segments are stored on MutableState via SetLogSegments (added in
// the same phase as this tool so the two-step agent can read them
// back without going through ToolResult.Summary).
func (t *EmitLogSegmentation) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	if ctx == nil || ctx.Mutable == nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_log_segmentation requires a writable context",
			Timestamp: time.Now(),
		}, nil
	}

	params = applyStructuredPayloadCompat(t.Name(), params, t.Parameters())

	var p emitLogSegmentationParams
	if err := json.Unmarshal(params, &p); err != nil {
		return failStrictDecodeWithError(t.Name(), time.Now(), err, nil, params)
	}

	logLen := len(ctx.AttachedLog)
	kept := make([]LogSegment, 0, len(p.Segments))
	normalized := 0
	for _, s := range p.Segments {
		start, end, changed, ok := normalizeSegmentationRange(s.ByteStart, s.ByteEnd, logLen)
		if !ok {
			continue
		}
		if changed {
			normalized++
		}
		s.ByteStart = start
		s.ByteEnd = end
		kept = append(kept, s)
	}
	if len(kept) == 0 {
		return segmentationCoordinateRejected(t.Name(), "runtime log", logLen, len(p.Segments)), nil
	}

	// Marshal segments + store on MutableState via the opaque
	// SetLogSegments accessor added in this phase.
	raw, err := json.Marshal(kept)
	if err != nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("segment marshal failed: %v", err),
			Timestamp: time.Now(),
		}, err
	}
	ctx.Mutable.SetLogSegments(raw)

	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   fmt.Sprintf("[emit_log_segmentation: kept=%d bytes=%d normalized=%d]\nsegments recorded", len(kept), logLen, normalized),
		Timestamp: time.Now(),
	}, nil
}
