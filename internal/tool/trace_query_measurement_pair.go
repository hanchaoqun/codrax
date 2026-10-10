package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/types"
)

type traceMeasurementPairParams struct {
	Baseline json.RawMessage `json:"baseline"`
	Current  json.RawMessage `json:"current"`
}

// Registration is by producer capability, not metric names or question text.
var traceMeasurementPairViews = []string{"window_stats", "scheduler_concurrency", "cpu_state_frequency", "process_measurements", "preferred_frame_rate", "measurements"}

var traceMeasurementPairFields = []string{"source", "path", "view", "time_start", "time_end", "line_start", "line_end", "pid", "thread", "target_scope", "bucket_ms", "limit", "trace_flavor", "platform", "core_topology"}

func traceQueryMeasurementPairSchema(raw json.RawMessage) json.RawMessage {
	var schema map[string]any
	if json.Unmarshal(raw, &schema) != nil {
		return raw
	}
	properties, _ := schema["properties"].(map[string]any)
	sideProperties := map[string]any{}
	descriptions := map[string]string{
		"source": "attached_trace or path; this side is selected independently.", "path": "Exact trace path for this side; normal binary/SQLite preparation applies.",
		"time_start": "This side's start in trace seconds.", "time_end": "This side's exclusive end in trace seconds.",
		"line_start": "Optional original input line start.", "line_end": "Optional original input line end.",
		"pid": "Exact target PID/TID when the selected native view permits it.", "thread": "Thread selector when the selected native view permits it.",
		"target_scope": "Explicit target identity scope; raw measurements has no thread/process target.", "bucket_ms": "Time-bucket width in milliseconds for views which support it.",
		"limit": "Retained output limit; does not turn a partial population into complete coverage.", "trace_flavor": "Optional producer flavor for this side.",
		"platform": "Optional platform semantics for this side, not proof of device identity.", "core_topology": "Optional explicit CPU class map for this side, not proof of matching hardware.",
	}
	for _, name := range traceMeasurementPairFields {
		if value, ok := properties[name]; ok {
			encoded, _ := json.Marshal(value)
			var property map[string]any
			_ = json.Unmarshal(encoded, &property)
			property["description"] = descriptions[name]
			sideProperties[name] = property
		}
	}
	sideProperties["view"] = map[string]any{"type": "string", "enum": traceMeasurementPairViews, "description": "Native measurement producer view; each side retains its own population, units and interval policy."}
	side := map[string]any{"type": "object", "properties": sideProperties, "required": []string{"source", "view"}, "additionalProperties": false}
	properties["comparison"] = map[string]any{"type": "object", "properties": map[string]any{"baseline": side, "current": side}, "additionalProperties": false,
		"description": "Query two captures independently in one call. Each side uses ordinary source/path/view/window selectors and the same automatic binary preparation. Omit an unknown side rather than invent a path; failed or absent sides retain their own status while the other side stays usable. Do not combine comparison with outer single-source fields. This publishes separate native tables and a two-side status table, not differences, clock alignment or causality; keep interpretation separate."}
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	if encoder.Encode(schema) != nil {
		return raw
	}
	return out.Bytes()
}

func (t *TraceQuery) executeMeasurementPair(ctx *types.BusContext, raw json.RawMessage, pair *traceMeasurementPairParams) (types.ToolResult, error) {
	now := time.Now()
	var outer map[string]json.RawMessage
	if json.Unmarshal(raw, &outer) != nil || len(outer) != 1 || pair == nil || len(pair.Baseline)+len(pair.Current) == 0 {
		return types.ToolResult{ToolName: t.Name(), Timestamp: now, Summary: "comparison must contain at least one side and cannot mix outer single-source selectors", Repair: &types.ToolRepair{Code: "trace_measurement_pair_shape", Fields: []string{"comparison"}}}, nil
	}
	var inputs [2]types.NativeMeasurementPairSide
	for i, request := range []json.RawMessage{pair.Baseline, pair.Current} {
		inputs[i].Request = request
		if len(request) == 0 {
			continue
		}
		var child traceQueryParams
		decoder := json.NewDecoder(strings.NewReader(string(request)))
		decoder.DisallowUnknownFields()
		err := decoder.Decode(&child)
		allowed := false
		for _, view := range traceMeasurementPairViews {
			if child.View == view {
				allowed = true
			}
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(request, &fields) != nil || fields == nil {
			allowed = false
		}
		for field := range fields {
			found := false
			for _, known := range traceMeasurementPairFields {
				if field == known {
					found = true
					break
				}
			}
			if !found {
				allowed = false
			}
		}
		if err != nil || child.Comparison != nil || !allowed || child.Source != "path" && child.Source != "attached_trace" {
			inputs[i].Result = types.ToolResult{ToolName: t.Name(), Summary: fmt.Sprintf("invalid native measurement side: view=%q; decode=%v", child.View, err)}
			continue
		}
		if err := contextFromBus(ctx).Err(); err != nil {
			inputs[i].Result = traceQueryCancellationResult(child.View, child.Path, err)
			continue
		}
		result, err := t.Execute(ctx, request)
		if err != nil && result.Summary == "" {
			result.Summary = err.Error()
		}
		inputs[i].Result = result
		inputs[i].Material = traceMeasurementPairMaterial(ctx, result)
	}
	var owner *types.MutableState
	if ctx != nil {
		owner = ctx.Mutable
	}
	receipt := types.NewNativeMeasurementPair(owner, inputs)
	report, _ := receipt.Report()
	encoded, _ := json.Marshal(report)
	ref := StoreBlobArtifact(ctxWorkDir(ctx), t.Name(), "trace-measurement-pair.json", string(encoded))
	summary := fmt.Sprintf("Independent native measurements: baseline=%s; current=%s. %s\nFull report: %s", report.Sides[0].Status, report.Sides[1].Status, report.Boundary, ref)
	// Composition succeeded even if a side failed. The explicit side statuses,
	// not this wrapper transport flag, report acquisition success.
	return types.ToolResult{ToolName: t.Name(), Success: true, Summary: summary, RawRef: ref, Timestamp: now, RuntimeMeasurementPair: receipt}, nil
}

func traceMeasurementPairMaterial(ctx *types.BusContext, result types.ToolResult) *attachment.TraceMaterial {
	if ctx == nil {
		return nil
	}
	var materials []*attachment.TraceMaterial
	if ctx.AttachedTraceMaterial != nil {
		materials = append(materials, ctx.AttachedTraceMaterial)
	}
	if ctx.TraceInputPreparer != nil {
		materials = append(materials, ctx.TraceInputPreparer.PreparedMaterials()...)
	}
	for _, observation := range result.Observations {
		if !types.RuntimeMeasurementPredicateIsRegistered(observation.Predicate) {
			continue
		}
		for _, material := range materials {
			if material != nil && material.QueryPath() == observation.SourceRef.Path {
				return material
			}
		}
	}
	return nil
}
