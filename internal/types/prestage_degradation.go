package types

import "strings"

// PreStageDegradation records that an attached-artifact pre-stage
// (log_triage / perf_triage) failed to produce its typed bundle, so
// the run proceeds without the artifact anchors the user supplied.
// The carrier makes that loss visible end to end: the analyzer prompt
// explains WHY artifact anchors are absent, and the final answer
// carries a user-facing caveat. Reason text is the bounded system
// summary of the failure — never parsed, only displayed.
type PreStageDegradation struct {
	// Stage is the pre-stage that degraded (log_triage / perf_triage).
	Stage PipelineStage `json:"stage"`

	// Kind distinguishes a dispatch error (transport/agent failure)
	// from an emit rejection (the model produced output the validators
	// refused). Typed enum: "dispatch_error" | "emit_rejected".
	Kind string `json:"kind"`

	// Summary is the bounded failure text for prompt/telemetry display.
	Summary string `json:"summary,omitempty"`
}

const (
	PreStageDegradationDispatchError = "dispatch_error"
	PreStageDegradationEmitRejected  = "emit_rejected"
)

const preStageDegradationSummaryCap = 400

// AddPreStageDegradation appends one degradation record (bounded summary).
func (m *MutableState) AddPreStageDegradation(d PreStageDegradation) {
	if m == nil {
		return
	}
	d.Summary = strings.TrimSpace(d.Summary)
	if len(d.Summary) > preStageDegradationSummaryCap {
		d.Summary = d.Summary[:preStageDegradationSummaryCap] + "…"
	}
	switch d.Kind {
	case PreStageDegradationDispatchError, PreStageDegradationEmitRejected:
	default:
		d.Kind = PreStageDegradationDispatchError
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.preStageDegradations = append(m.preStageDegradations, d)
}

// PreStageDegradations returns a copy of the recorded degradations.
func (m *MutableState) PreStageDegradations() []PreStageDegradation {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]PreStageDegradation(nil), m.preStageDegradations...)
}
