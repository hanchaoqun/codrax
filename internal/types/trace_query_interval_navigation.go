package types

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/hanchaoqun/codrax/internal/attachment"
)

type traceIntervalNavigation struct {
	queryPath string
	views     string
	source    TraceQuerySourceReadRef
	params    string
	material  *attachment.TraceMaterial
}

// NativeTraceIntervalNavigationCandidate is called only by the native parser
// consumer. It cannot be reconstructed from model JSON, a manifest disclosure,
// a readable summary, or a metric name. The producer requires one identity-
// clock artifact; the stamp separately binds the complete prepared material.
func NativeTraceIntervalNavigationCandidate(queryPath string, views []string) TraceQueryWindowReplayRef {
	if len(views) == 0 || len(views) > 8 {
		return TraceQueryWindowReplayRef{}
	}
	seen := map[string]bool{}
	var unique []string
	for _, view := range views {
		if view == "" || len(view) > 80 {
			return TraceQueryWindowReplayRef{}
		}
		if !seen[view] {
			unique = append(unique, view)
			seen[view] = true
		}
	}
	sort.Strings(unique)
	raw, _ := json.Marshal(unique)
	return TraceQueryWindowReplayRef{nativeNavigation: &traceIntervalNavigation{queryPath: queryPath, views: string(raw)}}
}

// StampTraceIntervalNavigation never calls the physical-read registration
// path. Converted input keeps its whole source/manifest/member receipt rather
// than pretending the manifest is the physical event source.
func (m *MutableState) StampTraceIntervalNavigation(ctx context.Context, source TraceQuerySourceReadRef, result *ToolResult, params json.RawMessage, material *attachment.TraceMaterial) {
	if m == nil || result == nil {
		return
	}
	candidate := result.TraceQueryWindowReplay.nativeNavigation
	result.TraceQueryWindowReplay.nativeNavigation = nil
	if candidate == nil || ctx != nil && ctx.Err() != nil || !result.Success || result.ReusedFromRunMemo || result.TraceViewCancellation != nil || CanonicalToolName(result.ToolName) != "trace_query" || len(params) == 0 || len(params) > 16<<10 {
		return
	}
	path, identity, ok := traceSourcePhysicalPath(candidate.queryPath)
	if !ok || path != source.path || !source.identity.SameVersion(identity) {
		return
	}
	if material != nil {
		query, _, valid := traceSourcePhysicalPath(material.QueryPath())
		if !valid || query != path || material.Validate(ctx, material.Preview()) != nil {
			return
		}
	} else {
		// Without a prepared-material receipt only the old single-physical-
		// artifact producer qualification is sufficient.
		physical, id, valid := traceSourcePhysicalPath(result.TraceQuerySourceRead.physicalSource)
		if !valid || physical != path || !identity.SameVersion(id) {
			return
		}
	}
	var args map[string]json.RawMessage
	if json.Unmarshal(params, &args) != nil || args == nil {
		return
	}
	args["path"], _ = json.Marshal(path)
	bound, err := json.Marshal(args)
	if err != nil {
		return
	}
	m.mu.RLock()
	current := source.generation != nil && source.generation == m.traceSourceReadGeneration
	m.mu.RUnlock()
	if current {
		result.TraceQueryWindowReplay.nativeNavigation = &traceIntervalNavigation{queryPath: path, views: candidate.views, source: source, params: string(bound), material: material}
	}
}

// ResolveTraceIntervalNavigation returns an independent list of raw-view
// candidates, not query or completion authority. Callers must preserve the
// requested source/window and check selector compatibility before using one.
func (m *MutableState) ResolveTraceIntervalNavigation(ctx context.Context, ref TraceQueryWindowReplayRef) (string, json.RawMessage, []string, bool) {
	nav := ref.nativeNavigation
	if ctx == nil {
		ctx = context.Background()
	}
	if m == nil || nav == nil || ctx.Err() != nil {
		return "", nil, nil, false
	}
	path, params, current := m.ResolveTraceQueryWindowReplay(TraceQueryWindowReplayRef{source: nav.source, params: nav.params})
	if !current || nav.material != nil && nav.material.Validate(ctx, nav.material.Preview()) != nil {
		return "", nil, nil, false
	}
	var views []string
	if json.Unmarshal([]byte(nav.views), &views) != nil || len(views) == 0 || len(views) > 8 {
		return "", nil, nil, false
	}
	return path, params, views, true
}
