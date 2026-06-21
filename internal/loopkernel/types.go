// Package loopkernel contains the deterministic state substrate for Codrax's
// next-generation read/write execution loop. Reducers and authority projections
// are side-effect free: they consume typed artifacts only and do not parse user
// prose, model rationale, prompt text, or terminal narrative.
package loopkernel

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/safety"
	codraxtypes "github.com/hanchaoqun/codrax/internal/types"
)

type LoopRunStatus string

const (
	LoopRunStatusSeeded     LoopRunStatus = "seeded"
	LoopRunStatusInProgress LoopRunStatus = "in_progress"
	LoopRunStatusComplete   LoopRunStatus = "complete"
	LoopRunStatusBlocked    LoopRunStatus = "blocked"
)

type LoopRun struct {
	RunID        string        `json:"run_id,omitempty"`
	Mode         string        `json:"mode,omitempty"`
	Goal         string        `json:"goal,omitempty"`
	Status       LoopRunStatus `json:"status,omitempty"`
	ActiveUnitID string        `json:"active_unit_id,omitempty"`
	EventRefs    []ArtifactRef `json:"event_refs,omitempty"`
	Budget       LoopBudget    `json:"budget,omitempty"`
	CreatedAt    time.Time     `json:"created_at,omitempty"`
	UpdatedAt    time.Time     `json:"updated_at,omitempty"`
}

type LoopBudget struct {
	MaxUnits          int       `json:"max_units,omitempty"`
	MaxRepairs        int       `json:"max_repairs,omitempty"`
	MaxApprovals      int       `json:"max_approvals,omitempty"`
	UnitsUsed         int       `json:"units_used,omitempty"`
	RepairsUsed       int       `json:"repairs_used,omitempty"`
	ApprovalsUsed     int       `json:"approvals_used,omitempty"`
	DeadlineAt        time.Time `json:"deadline_at,omitempty"`
	InterruptedReason string    `json:"interrupted_reason,omitempty"`
}

type LoopEventKind string

const (
	LoopEventRunSeeded             LoopEventKind = "run_seeded"
	LoopEventContextRequested      LoopEventKind = "context_requested"
	LoopEventContextObserved       LoopEventKind = "context_observed"
	LoopEventLocalizationProjected LoopEventKind = "localization_projected"
	LoopEventPlanRequested         LoopEventKind = "plan_requested"
	LoopEventPlanEmitted           LoopEventKind = "plan_emitted"
	LoopEventUnitCreated           LoopEventKind = "unit_created"
	LoopEventEffectDescribed       LoopEventKind = "effect_described"
	LoopEventPermissionDecided     LoopEventKind = "permission_decided"
	LoopEventApprovalRequested     LoopEventKind = "approval_requested"
	LoopEventApprovalResolved      LoopEventKind = "approval_resolved"
	LoopEventCheckpointCreated     LoopEventKind = "checkpoint_created"
	LoopEventUnitApplyStarted      LoopEventKind = "unit_apply_started"
	LoopEventUnitApplyCompleted    LoopEventKind = "unit_apply_completed"
	LoopEventPatchEffectRecorded   LoopEventKind = "patch_effect_recorded"
	LoopEventObserveStarted        LoopEventKind = "observe_started"
	LoopEventObserveCompleted      LoopEventKind = "observe_completed"
	LoopEventProofProjected        LoopEventKind = "proof_projected"
	LoopEventTruthProjected        LoopEventKind = "truth_projected"
	LoopEventRepairRequested       LoopEventKind = "repair_requested"
	LoopEventUnitCompleted         LoopEventKind = "unit_completed"
	LoopEventUnitBlocked           LoopEventKind = "unit_blocked"
	LoopEventRunCompleted          LoopEventKind = "run_completed"
	LoopEventRunBlocked            LoopEventKind = "run_blocked"
)

type RuntimeUnitStatus string

const (
	RuntimeUnitPending   RuntimeUnitStatus = "pending"
	RuntimeUnitApplying  RuntimeUnitStatus = "applying"
	RuntimeUnitObserving RuntimeUnitStatus = "observing"
	RuntimeUnitRepair    RuntimeUnitStatus = "repair_needed"
	RuntimeUnitComplete  RuntimeUnitStatus = "complete"
	RuntimeUnitBlocked   RuntimeUnitStatus = "blocked"
)

type ArtifactRef struct {
	Kind string `json:"kind,omitempty"`
	ID   string `json:"id,omitempty"`
	Path string `json:"path,omitempty"`
}

type LoopEvent struct {
	ID           string          `json:"id,omitempty"`
	RunID        string          `json:"run_id,omitempty"`
	UnitID       string          `json:"unit_id,omitempty"`
	Sequence     int64           `json:"sequence,omitempty"`
	Kind         LoopEventKind   `json:"kind,omitempty"`
	Source       string          `json:"source,omitempty"`
	ReasonCode   string          `json:"reason_code,omitempty"`
	ArtifactRefs []ArtifactRef   `json:"artifact_refs,omitempty"`
	Payload      json.RawMessage `json:"payload,omitempty"`
	At           time.Time       `json:"at,omitempty"`
}

type RuntimeUnitView struct {
	UnitID     string                  `json:"unit_id,omitempty"`
	Status     RuntimeUnitStatus       `json:"status,omitempty"`
	ReasonCode string                  `json:"reason_code,omitempty"`
	Truth      codraxtypes.TruthLedger `json:"truth,omitempty"`
	UpdatedAt  time.Time               `json:"updated_at,omitempty"`
}

type EffectEventPayload struct {
	Effect      safety.EffectDescriptor `json:"effect,omitempty"`
	Fingerprint string                  `json:"fingerprint,omitempty"`
}

type LoopStateView struct {
	RunID          string                     `json:"run_id,omitempty"`
	Status         LoopRunStatus              `json:"status,omitempty"`
	ActiveUnitID   string                     `json:"active_unit_id,omitempty"`
	LastEventKind  LoopEventKind              `json:"last_event_kind,omitempty"`
	LastReasonCode string                     `json:"last_reason_code,omitempty"`
	Units          []RuntimeUnitView          `json:"units,omitempty"`
	Localization   LocalizationAuthorityView  `json:"localization,omitempty"`
	Proof          ProofCoverageAuthorityView `json:"proof,omitempty"`
	Permission     PermissionAuthorityView    `json:"permission,omitempty"`
	Truth          codraxtypes.TruthLedger    `json:"truth,omitempty"`
	EventCount     int                        `json:"event_count,omitempty"`
}

func NormalizeLoopEvent(in LoopEvent) LoopEvent {
	in.ID = strings.TrimSpace(in.ID)
	in.RunID = strings.TrimSpace(in.RunID)
	in.UnitID = strings.TrimSpace(in.UnitID)
	in.Source = strings.TrimSpace(in.Source)
	in.ReasonCode = strings.TrimSpace(in.ReasonCode)
	refs := make([]ArtifactRef, 0, len(in.ArtifactRefs))
	for _, ref := range in.ArtifactRefs {
		ref.Kind = strings.TrimSpace(ref.Kind)
		ref.ID = strings.TrimSpace(ref.ID)
		ref.Path = strings.TrimSpace(ref.Path)
		if ref.Kind == "" && ref.ID == "" && ref.Path == "" {
			continue
		}
		refs = append(refs, ref)
	}
	in.ArtifactRefs = refs
	if len(in.Payload) == 0 || strings.TrimSpace(string(in.Payload)) == "" {
		in.Payload = nil
	}
	return in
}

func NormalizeLoopEvents(events []LoopEvent) []LoopEvent {
	out := make([]LoopEvent, 0, len(events))
	for _, event := range events {
		event = NormalizeLoopEvent(event)
		if event.Kind == "" && event.ID == "" && event.RunID == "" && event.UnitID == "" {
			continue
		}
		out = append(out, event)
	}
	SortLoopEvents(out)
	return out
}

func SortLoopEvents(events []LoopEvent) {
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].Sequence != events[j].Sequence {
			return events[i].Sequence < events[j].Sequence
		}
		if !events[i].At.Equal(events[j].At) {
			if events[i].At.IsZero() {
				return false
			}
			if events[j].At.IsZero() {
				return true
			}
			return events[i].At.Before(events[j].At)
		}
		if events[i].ID != events[j].ID {
			return events[i].ID < events[j].ID
		}
		return events[i].Kind < events[j].Kind
	})
}

func EventPayload(v any) json.RawMessage {
	if v == nil {
		return nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return json.RawMessage(data)
}
