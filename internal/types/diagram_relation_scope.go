package types

import "strings"

// DiagramRelationScopeStatus is a model-authored disclosure about the
// requested relation represented by a diagram. It is intentionally separate
// from per-participant boundaries: every participant may have a truthful local
// incident edge while the available typed evidence still does not prove one
// connected end-to-end relation spanning the requested participant slate.
//
// The status is presentation authority only. It never creates, removes,
// redirects, or upgrades an edge, and it does not replace the model's answer
// conclusion.
type DiagramRelationScopeStatus string

const (
	DiagramRelationScopeUnknown         DiagramRelationScopeStatus = ""
	DiagramRelationScopePartialUnproven DiagramRelationScopeStatus = "partial_unproven"
)

func (s DiagramRelationScopeStatus) IsValid() bool {
	return s == DiagramRelationScopePartialUnproven
}

func NormalizeDiagramRelationScopeStatus(raw string) (DiagramRelationScopeStatus, bool) {
	status := DiagramRelationScopeStatus(strings.TrimSpace(raw))
	if !status.IsValid() {
		return DiagramRelationScopeUnknown, false
	}
	return status, true
}
