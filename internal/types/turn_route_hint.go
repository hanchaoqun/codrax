package types

import "strings"

// TurnRouteHint is typed current-turn routing metadata produced before the
// read pipeline starts. It is not user prose and not evidence; analyzer uses it
// only to avoid doing the wrong kind of pre-scan before emit_analysis.
type TurnRouteHint struct {
	Route                string  `json:"route,omitempty"`
	Source               string  `json:"source,omitempty"`
	Operation            string  `json:"operation,omitempty"`
	OperationKind        string  `json:"operation_kind,omitempty"`
	DataTaskKind         string  `json:"data_task_kind,omitempty"`
	WriteIntent          string  `json:"write_intent,omitempty"`
	TargetSurface        string  `json:"target_surface,omitempty"`
	ConcreteOperation    bool    `json:"concrete_operation,omitempty"`
	NeedsRepoAccess      bool    `json:"needs_repo_access,omitempty"`
	NeedsOperationAccess bool    `json:"needs_operation_access,omitempty"`
	NeedsDataAccess      bool    `json:"needs_data_access,omitempty"`
	Confidence           float64 `json:"confidence,omitempty"`
}

func (h TurnRouteHint) IsZero() bool {
	return strings.TrimSpace(h.Route) == "" &&
		strings.TrimSpace(h.Source) == "" &&
		strings.TrimSpace(h.Operation) == "" &&
		strings.TrimSpace(h.OperationKind) == "" &&
		strings.TrimSpace(h.DataTaskKind) == "" &&
		strings.TrimSpace(h.WriteIntent) == "" &&
		strings.TrimSpace(h.TargetSurface) == "" &&
		!h.ConcreteOperation &&
		!h.NeedsRepoAccess &&
		!h.NeedsOperationAccess &&
		!h.NeedsDataAccess &&
		h.Confidence == 0
}

func (h TurnRouteHint) ExternalObservationFirst() bool {
	if h.IsZero() || h.ConcreteOperation || h.NeedsOperationAccess {
		return false
	}
	switch strings.TrimSpace(h.Source) {
	case "external_tool", "artifact":
		return true
	default:
		return false
	}
}

// ExternalObservationParticipates reports whether the route classifier kept an
// external observation lane in the current turn. Unlike ExternalObservationFirst,
// this includes mixed runtime/source turns where repository evidence is still
// expected to carry part of the answer. It is route metadata only; callers must
// still rely on RequestModel/runtime artifacts for evidence and citations.
func (h TurnRouteHint) ExternalObservationParticipates() bool {
	if h.IsZero() || h.ConcreteOperation || h.NeedsOperationAccess {
		return false
	}
	switch strings.TrimSpace(h.Source) {
	case "external_tool", "artifact", "mixed":
		return true
	default:
		return false
	}
}
