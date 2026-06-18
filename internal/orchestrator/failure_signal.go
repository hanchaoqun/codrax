package orchestrator

import "github.com/hanchaoqun/codrax/internal/types"

// ExtractFailureSignal preserves the orchestrator-local helper API while the
// shared implementation lives in types for durable write handoff consumers.
func ExtractFailureSignal(detail string, maxChars int) string {
	return types.ExtractFailureSignal(detail, maxChars)
}
