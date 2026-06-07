package dataworkflow

type ViolationRepairability string

const (
	RepairSafePatch          ViolationRepairability = "safe_patch"
	RepairNeedsTypedAction   ViolationRepairability = "needs_typed_action"
	RepairNeedsRecompute     ViolationRepairability = "needs_recompute"
	RepairNeedsClarification ViolationRepairability = "needs_clarification"
)

type WorkflowViolation struct {
	Code                 string                 `json:"code,omitempty"`
	Severity             string                 `json:"severity,omitempty"`
	Repairability        ViolationRepairability `json:"repairability,omitempty"`
	ActionID             string                 `json:"action_id,omitempty"`
	ActionKind           string                 `json:"action_kind,omitempty"`
	InputAlias           string                 `json:"input_alias,omitempty"`
	InputAliases         []string               `json:"input_aliases,omitempty"`
	OutputAlias          string                 `json:"output_alias,omitempty"`
	IdempotencyKey       string                 `json:"idempotency_key,omitempty"`
	DependencyRank       int                    `json:"dependency_rank,omitempty"`
	MissingFields        []string               `json:"missing_fields,omitempty"`
	AvailableFieldSample []string               `json:"available_field_sample,omitempty"`
	CandidateArtifacts   []string               `json:"candidate_artifacts,omitempty"`
	RepairActionHints    []string               `json:"repair_action_hints,omitempty"`
	Reason               string                 `json:"reason,omitempty"`
}
