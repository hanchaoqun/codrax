package tool

import (
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func (t *TraceQuery) streamResourceStack(ctx *types.BusContext, p traceQueryParams, path, sourceLabel, callCaveat string, window traceQueryNormalizedWindow) (types.ToolResult, bool) {
	if tracequery.CanonicalViewName(p.View) != tracequery.ViewResourceStack {
		return types.ToolResult{}, false
	}
	q := traceQueryBuildQuery(ctx, p, sourceLabel, path, window.RequestedStart, window.RequestedEnd)
	result, err := tracequery.StreamResourceStack(contextFromBus(ctx), path, q)
	if err != nil {
		if traceQueryIsCancellation(err) {
			return traceQueryCancellationResult(p.View, path, err), true
		}
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: "Resource stack query failed: " + err.Error(), Timestamp: time.Now()}, true
	}
	traceQueryAppendCallCaveats(&result, callCaveat)
	payload, failure := traceQueryMarshalPayload(t.Name(), result)
	if failure != nil {
		return *failure, true
	}
	payloadRef := StoreBlobArtifact(ctxWorkDir(ctx), t.Name(), "trace-query-result.json", string(payload))
	preview, rawRef := StoreBlob(ctx, t.Name(), traceQuerySummary(result, p, sourceLabel, payloadRef))
	if rawRef == "" {
		rawRef = payloadRef
	}
	now := time.Now()
	return types.ToolResult{ToolName: t.Name(), Success: true, Summary: preview, RawRef: rawRef, Timestamp: now,
		Observations:           traceQueryTypedObservations(result, sourceLabel, payloadRef, rawRef, "", now, q),
		TraceQuerySourceRead:   traceQuerySourceReadCandidate(result),
		TraceEvidenceAuthority: traceQueryEvidenceAuthorityWithSource(result, sourceLabel, payloadRef, rawRef, "", now, q)}, true
}
