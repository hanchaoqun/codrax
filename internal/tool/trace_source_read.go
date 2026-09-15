package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/tool/width"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// This qualification comes only from the native engine's physical-source
// roster. It is deliberately not attached to serializable observation facts.
func traceQuerySourceReadCandidate(results ...tracequery.Result) types.TraceQuerySourceReadRef {
	if len(results) == 0 {
		return types.TraceQuerySourceReadRef{}
	}
	source := filepath.Clean(results[0].SourcePath)
	for _, result := range results {
		if len(result.TraceArtifacts) != 1 || result.TraceArtifacts[0].VirtualLineBase != 0 ||
			filepath.Clean(result.TraceArtifacts[0].SourcePath) != source || filepath.Clean(result.SourcePath) != source {
			return types.TraceQuerySourceReadRef{}
		}
	}
	return types.TraceQueryPhysicalSourceReadCandidate(results[0].SourcePath)
}

func traceQuerySourceReadTarget(ctx *types.BusContext, requested string) (types.TraceQuerySourceReadRef, bool) {
	if ctx == nil || ctx.Mutable == nil {
		return types.TraceQuerySourceReadRef{}, false
	}
	// Preserve Q5-A's existing published-result aliases: the actual path
	// selected by the reader, not a same-basename repository file, owns role.
	actual, reject := resolveReadFilePath(ctx, requested)
	if reject != nil {
		return types.TraceQuerySourceReadRef{}, false
	}
	return ctx.Mutable.ResolveTraceQuerySourceRead(actual)
}

func traceQuerySourceReadBounds(p readFileParams) bool {
	offset, limit := p.Offset.Int(), p.Limit.Int()
	return offset >= 0 && limit > 0 && limit <= readFileWidthPageWindowMax() && offset <= int(^uint(0)>>1)-limit
}

// TraceQuerySourceReadAllowed is shared by the explorer's trace gates and the
// native reader. It opens only explicit bounded read_file slices of a queried
// physical source, never grep, generic source tools, result blobs or aliases.
func TraceQuerySourceReadAllowed(ctx *types.BusContext, params json.RawMessage) bool {
	return PrepareBoundedTraceQuerySourceRead(ctx, params).Path() != ""
}

func PrepareBoundedTraceQuerySourceRead(ctx *types.BusContext, params json.RawMessage) types.TraceQuerySourceReadRef {
	p, _, _, err := decodeReadFileParams(params)
	if err != nil || !traceQuerySourceReadBounds(p) {
		return types.TraceQuerySourceReadRef{}
	}
	ref, ok := traceQuerySourceReadTarget(ctx, p.Path)
	if !ok || traceQuerySourceReadTerminal(ctx) {
		return types.TraceQuerySourceReadRef{}
	}
	return ref
}

type traceQuerySourceReadContextKey struct{}

// BindTraceQuerySourceRead carries the gate's exact receipt only through this
// one narrowed tool invocation. It cannot be put in tool JSON or inherited by
// another tool call; a later path/epoch change must not turn it into source.
func BindTraceQuerySourceRead(ctx *types.BusContext, ref types.TraceQuerySourceReadRef) {
	if ctx != nil && ref.Path() != "" {
		ctx.Ctx = context.WithValue(contextFromBus(ctx), traceQuerySourceReadContextKey{}, ref)
	}
}

func traceQuerySourceReadForInvocation(ctx *types.BusContext, requested string) (ref types.TraceQuerySourceReadRef, original, bounded bool) {
	if ctx != nil && ctx.Ctx != nil {
		if ref, ok := ctx.Ctx.Value(traceQuerySourceReadContextKey{}).(types.TraceQuerySourceReadRef); ok {
			return ref, true, true // an expired receipt still cannot fall back to source
		}
	}
	ref, original = traceQuerySourceReadTarget(ctx, requested)
	return ref, original, false // direct readers retain their existing page policy
}

func traceQuerySourceReadTerminal(ctx *types.BusContext) bool {
	if ctx == nil || ctx.Mutable == nil {
		return false
	}
	_, terminal := ctx.Mutable.TraceInputAdmissionTerminal(types.StageExplore)
	return terminal
}

func traceQuerySourceReadRefusal() types.ToolResult {
	hint := fmt.Sprintf("Read the queried original trace only with an explicit positive limit of at most %d lines and a non-negative line_offset; otherwise continue with trace_query. A terminal trace-input admission still requires user action.", readFileWidthPageWindowMax())
	return types.ToolResult{ToolName: "read_file", Success: false, Summary: hint,
		Repair: &types.ToolRepair{Code: "trace_source_read_bounded", Fields: []string{"line_offset", "limit"}, Hint: hint}, Timestamp: time.Now()}
}

// readTraceQuerySourceFile holds one descriptor throughout validation and I/O.
// It uses the existing whole-file byte wall (no extra scan/hash), limits even
// a growing file, and rechecks the complete generation and run epoch afterwards.
func readTraceQuerySourceFile(ctx *types.BusContext, ref types.TraceQuerySourceReadRef) ([]byte, error) {
	f, identity, err := filegeneration.OpenRegularReadOnly(ref.Path())
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if !ctx.Mutable.TraceQuerySourceReadCurrent(ref, identity) || traceQuerySourceReadTerminal(ctx) {
		return nil, errors.New("original trace read permission expired or its file identity changed; query the trace again")
	}
	maxBytes := width.Current().ReadFile.MaxWholeReadBytes
	if maxBytes <= 0 {
		maxBytes = width.SourceReadMaxBytes
	}
	if identity.Size() > maxBytes {
		return nil, &width.ErrSourceReadOversized{Path: ref.Path(), Size: identity.Size(), Cap: maxBytes}
	}
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, &width.ErrSourceReadOversized{Path: ref.Path(), Size: int64(len(data)), Cap: maxBytes}
	}
	identity, err = filegeneration.FromFile(f)
	if err != nil {
		return nil, err
	}
	pathIdentity, pathErr := filegeneration.FromPath(ref.Path())
	if pathErr != nil || !identity.SameVersion(pathIdentity) || !ctx.Mutable.TraceQuerySourceReadCurrent(ref, identity) || traceQuerySourceReadTerminal(ctx) {
		return nil, errors.New("original trace changed or its read permission expired during reading; query the trace again")
	}
	return data, nil
}

// The held receipt, not an extension or a second mutable lookup, classifies
// these bytes. In particular capture.go is still runtime-only after reset.
func stampTraceQuerySourceReadResult(result *types.ToolResult, requested string, start, end, total int) {
	result.ReadCoverage = nil
	result.Observations = nil
	result.RuntimeArtifactRead = &types.ToolRuntimeArtifactRead{
		RequestedPath: requested, Kind: "trace", LineStart: start, LineEnd: end, TotalLines: total, RawRef: result.RawRef,
	}
}
