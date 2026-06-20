package types

const (
	ProgressDeltaDowngradeBlocker ProgressDeltaKind = "downgrade_blocker"

	ProgressReasonContinue  = "progress_delta_continue"
	ProgressReasonConverged = "progress_delta_converged"
)

type ProgressDeltaKind string

// ProgressDelta is the typed low-delta observation used by read/write repair
// loops to decide whether another replan/retry can add information. It is built
// from structured counters/fingerprints only and never from model prose.
type ProgressDelta struct {
	Kind          ProgressDeltaKind `json:"kind,omitempty"`
	DowngradeLane DowngradeLane     `json:"downgrade_lane,omitempty"`
	BlockerKey    uint32            `json:"blocker_key,omitempty"`
	Consecutive   int               `json:"consecutive,omitempty"`
	HardThreshold int               `json:"hard_threshold,omitempty"`
}

type ProgressDecision struct {
	ShouldReplan bool          `json:"should_replan"`
	ReasonCode   string        `json:"reason_code,omitempty"`
	Delta        ProgressDelta `json:"delta"`
}

func ShouldReplan(delta ProgressDelta) ProgressDecision {
	if delta.Consecutive < 0 {
		delta.Consecutive = 0
	}
	if delta.HardThreshold < 2 {
		delta.HardThreshold = 2
	}
	decision := ProgressDecision{
		ShouldReplan: true,
		ReasonCode:   ProgressReasonContinue,
		Delta:        delta,
	}
	if delta.Consecutive >= delta.HardThreshold {
		decision.ShouldReplan = false
		decision.ReasonCode = ProgressReasonConverged
	}
	return decision
}
