package logging

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"sync/atomic"
)

type traceTimingKey struct{}

type traceTimingCall struct {
	id, parent uint64
	role       string
}

var traceTimingSequence atomic.Uint64

// The process ID in the CLI diagnostic record scopes a single --request.
// Call IDs distinguish each tool invocation, including the two pair children.
func TraceTimingCall(ctx context.Context) context.Context {
	parent, _ := ctx.Value(traceTimingKey{}).(traceTimingCall)
	return context.WithValue(ctx, traceTimingKey{}, traceTimingCall{
		id: traceTimingSequence.Add(1), parent: parent.id, role: parent.role,
	})
}

func TraceTimingRole(ctx context.Context, role string) context.Context {
	call, _ := ctx.Value(traceTimingKey{}).(traceTimingCall)
	call.role = role
	return context.WithValue(ctx, traceTimingKey{}, call)
}

func TraceTiming(ctx context.Context, format string, args ...any) {
	if !IsDebug() {
		return
	}
	call, _ := ctx.Value(traceTimingKey{}).(traceTimingCall)
	Debug("[trace_timing] call=%d parent=%d role=%q %s", call.id, call.parent, call.role, fmt.Sprintf(format, args...))
}

func TraceTimingAttachment(path string) string {
	if path == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(filepath.ToSlash(filepath.Clean(path))))
	return fmt.Sprintf("%x", digest[:8])
}
