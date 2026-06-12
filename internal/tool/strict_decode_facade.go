package tool

import (
	"encoding/json"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// strict_decode_facade.go — exported entry points into the shared
// structured-payload repair layer for built-in tools that live OUTSIDE
// this package. repo_map (internal/tool/repomap) imports this package
// for ReadOnly/StoreBlob, so the repair layer cannot move there; these
// thin wrappers let such tools ride the SAME compat + strict-decode +
// typed-ToolRepair channels as in-package tools (trace_query is the
// reference wiring) instead of growing one-off parallel repair paths.

// ApplyStructuredPayloadCompat applies the shared schema-driven
// tool-boundary compatibility layer (deterministic JSON carrier
// repairs, x-codrax-* enum aliases, split string arrays). It is the
// exported form of applyStructuredPayloadCompat for out-of-package
// built-in tools.
func ApplyStructuredPayloadCompat(toolName string, raw json.RawMessage, schema json.RawMessage) json.RawMessage {
	return applyStructuredPayloadCompat(toolName, raw, schema)
}

// FailStrictDecodeWithError builds the typed strict-decode rejection
// ToolResult (ToolRepair-coded, R4-sanitized message) and returns the
// remapped error, exactly like trace_query's in-package failure path.
func FailStrictDecodeWithError(name string, now time.Time, err error, hints []MisplacedFieldHint, raw []byte) (types.ToolResult, error) {
	return failStrictDecodeWithError(name, now, err, hints, raw)
}
