package types

import "strings"

// TurnRouteCurrentSourceEvidenceMode separates pipeline/repository execution
// access from the evidence obligation of the current answer. Runtime artifacts
// still use route=repo and NeedsRepoAccess=true so they enter the analysis
// pipeline, but artifact-only investigations can keep current checkout evidence
// optional. Only this schema-validated enum may change that obligation; callers
// must not infer it from raw user or model-authored prose.
type TurnRouteCurrentSourceEvidenceMode string

const (
	TurnRouteCurrentSourceEvidenceUnspecified TurnRouteCurrentSourceEvidenceMode = ""
	TurnRouteCurrentSourceEvidenceRequired    TurnRouteCurrentSourceEvidenceMode = "required"
	TurnRouteCurrentSourceEvidenceOptional    TurnRouteCurrentSourceEvidenceMode = "optional"
)

func NormalizeTurnRouteCurrentSourceEvidenceMode(raw string) TurnRouteCurrentSourceEvidenceMode {
	switch strings.TrimSpace(raw) {
	case string(TurnRouteCurrentSourceEvidenceRequired):
		return TurnRouteCurrentSourceEvidenceRequired
	case string(TurnRouteCurrentSourceEvidenceOptional):
		return TurnRouteCurrentSourceEvidenceOptional
	default:
		return TurnRouteCurrentSourceEvidenceUnspecified
	}
}

// TurnRouteHint is typed current-turn routing metadata produced before the
// read pipeline starts. It is not user prose and not evidence; analyzer uses it
// only to avoid doing the wrong kind of pre-scan before emit_analysis.
type TurnRouteHint struct {
	Route                     string                             `json:"route,omitempty"`
	Source                    string                             `json:"source,omitempty"`
	Operation                 string                             `json:"operation,omitempty"`
	OperationKind             string                             `json:"operation_kind,omitempty"`
	DataTaskKind              string                             `json:"data_task_kind,omitempty"`
	WriteIntent               string                             `json:"write_intent,omitempty"`
	TargetSurface             string                             `json:"target_surface,omitempty"`
	CurrentSourceEvidenceMode TurnRouteCurrentSourceEvidenceMode `json:"current_source_evidence_mode,omitempty"`
	ConcreteOperation         bool                               `json:"concrete_operation,omitempty"`
	NeedsRepoAccess           bool                               `json:"needs_repo_access,omitempty"`
	NeedsOperationAccess      bool                               `json:"needs_operation_access,omitempty"`
	NeedsDataAccess           bool                               `json:"needs_data_access,omitempty"`
	Confidence                float64                            `json:"confidence,omitempty"`
}

func (h TurnRouteHint) IsZero() bool {
	return strings.TrimSpace(h.Route) == "" &&
		strings.TrimSpace(h.Source) == "" &&
		strings.TrimSpace(h.Operation) == "" &&
		strings.TrimSpace(h.OperationKind) == "" &&
		strings.TrimSpace(h.DataTaskKind) == "" &&
		strings.TrimSpace(h.WriteIntent) == "" &&
		strings.TrimSpace(h.TargetSurface) == "" &&
		h.CurrentSourceEvidenceMode == TurnRouteCurrentSourceEvidenceUnspecified &&
		!h.ConcreteOperation &&
		!h.NeedsRepoAccess &&
		!h.NeedsOperationAccess &&
		!h.NeedsDataAccess &&
		h.Confidence == 0
}

// RequiresCurrentSourceEvidence reports the typed current-source obligation.
// Unspecified retains the historical NeedsRepoAccess behavior for persisted
// route hints and test/adapter implementations that predate this field. New
// production classifier output always emits required or optional.
func (h TurnRouteHint) RequiresCurrentSourceEvidence() bool {
	switch NormalizeTurnRouteCurrentSourceEvidenceMode(string(h.CurrentSourceEvidenceMode)) {
	case TurnRouteCurrentSourceEvidenceRequired:
		return true
	case TurnRouteCurrentSourceEvidenceOptional:
		return false
	default:
		return h.NeedsRepoAccess
	}
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
