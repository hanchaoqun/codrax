package types

import "strings"

// Narrative roles control copying only. They do not validate the text, bind a
// source, prove a relation or restore an accepted completion after JSON reload.
type InvestigationInterpretationRole string

const (
	InvestigationIndependentAnalysis     InvestigationInterpretationRole = "independent_analysis"
	InvestigationNativeRestatement       InvestigationInterpretationRole = "native_fact_restatement"
	NativeFactInterpretationRoleTeaching                                 = "Use interpretation_role=independent_analysis for any independent diagnosis, derived quantity, uncertainty, negative finding or source/business clue (including mixed text). For native facts alone, prefer an empty interpretation with interpretation_role=native_fact_restatement; any existing pure restatement stays audit-only and is not copied into downstream narratives. This role applies to both interpretation_units and legacy interpretation. Native facts come from the selected views, not this role."
)

func (r InvestigationInterpretationRole) Valid() bool {
	return r == "" || r == InvestigationIndependentAnalysis || r == InvestigationNativeRestatement
}

// InvestigationNativeFactHandoff separates presentation references from model
// interpretation. It contains no copied measurements or evidence permission.
// JSON retains intent for audit only; every consumer rebinds current sources.
type InvestigationNativeFactHandoff struct {
	RuntimeMeasurements   []AnswerRuntimeMeasurementReceipt `json:"runtime_measurements"`
	AnswerDisplay         []AnswerRuntimeMeasurementReceipt `json:"answer_display,omitempty"`
	Interpretation        string                            `json:"interpretation"`
	InterpretationRole    InvestigationInterpretationRole   `json:"interpretation_role,omitempty"`
	InterpretationUnits   []InvestigationInterpretationUnit `json:"interpretation_units,omitempty"`
	answerDisplayBindings map[string]string
	nativeRowBindings     map[string]string
}

func (h *InvestigationNativeFactHandoff) Clone() *InvestigationNativeFactHandoff {
	if h == nil {
		return nil
	}
	out := &InvestigationNativeFactHandoff{RuntimeMeasurements: cloneMeasurementMemberSetSelections(h.RuntimeMeasurements), AnswerDisplay: cloneMeasurementMemberSetSelections(h.AnswerDisplay), Interpretation: h.Interpretation, InterpretationRole: h.InterpretationRole}
	out.InterpretationUnits = cloneInvestigationInterpretationUnits(h.InterpretationUnits)
	if h.answerDisplayBindings != nil {
		out.answerDisplayBindings = make(map[string]string, len(h.answerDisplayBindings))
		for key, value := range h.answerDisplayBindings {
			out.answerDisplayBindings[key] = value
		}
	}
	if h.nativeRowBindings != nil {
		out.nativeRowBindings = make(map[string]string, len(h.nativeRowBindings))
		for key, value := range h.nativeRowBindings {
			out.nativeRowBindings[key] = value
		}
	}
	return out
}

// Legacy missing roles keep their original advisory behavior. No content is
// classified or rewritten; explicit restatements remain in the audit carrier.
func (h *InvestigationNativeFactHandoff) NarrativeInterpretation() string {
	if h == nil || h.InterpretationRole == InvestigationNativeRestatement || len(h.InterpretationUnits) > 0 {
		return ""
	}
	return h.Interpretation
}

func (h *InvestigationNativeFactHandoff) IsAuditOnlyInterpretationCopy(note string) bool {
	return h != nil && (h.InterpretationRole == InvestigationNativeRestatement || len(h.InterpretationUnits) > 0) && h.Interpretation != "" && note == h.Interpretation
}

type investigationNativeFactCompletion struct {
	handoff    *InvestigationNativeFactHandoff
	generation uint64
	reason     string
}

// AcceptInvestigationNativeFactHandoff runs only at the accepted model-tool
// tail. A new completion without this optional contract explicitly retires it.
func (m *MutableState) AcceptInvestigationNativeFactHandoff(h *InvestigationNativeFactHandoff) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.investigationComplete || m.investigationLiveCompletionGeneration != m.investigationCompleteGeneration {
		return
	}
	m.investigationNativeFactCompletion = nil
	if h != nil {
		m.investigationNativeFactCompletion = &investigationNativeFactCompletion{h.Clone(), m.investigationCompleteGeneration, m.investigationCompleteReason}
	}
}

func (m *MutableState) acceptedNativeFactHandoffLocked() *InvestigationNativeFactHandoff {
	a := m.investigationNativeFactCompletion
	if a == nil || a.generation != m.investigationCompleteGeneration {
		return nil
	}
	if m.investigationComplete && m.investigationLiveCompletionGeneration != a.generation {
		return nil
	}
	reason := strings.TrimSpace(m.investigationCompleteReason)
	if reason == "" {
		reason = strings.TrimSpace(m.retainedInvestigationCompleteReason)
	}
	if a.reason != reason {
		return nil
	}
	return a.handoff.Clone()
}

func (m *MutableState) AcceptedInvestigationNativeFactHandoff() *InvestigationNativeFactHandoff {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.acceptedNativeFactHandoffLocked()
}

// Forks copy immutable accepted intent. Only an actually deciding fork can
// reseat it in the parent's completion generation; inherited/failed forks do
// not overwrite a later closure. This is lifecycle, not source authority.
func cloneInvestigationNativeFactCompletion(a *investigationNativeFactCompletion) *investigationNativeFactCompletion {
	if a == nil {
		return nil
	}
	return &investigationNativeFactCompletion{a.handoff.Clone(), a.generation, a.reason}
}

func BindInvestigationNativeFactHandoff(h *InvestigationNativeFactHandoff, c *RuntimeMeasurementContract) ([]AnswerRuntimeMeasurementReceipt, bool) {
	if h == nil || len(h.RuntimeMeasurements) == 0 || len(h.RuntimeMeasurements) > 16 {
		return nil, false
	}
	seen := map[string]bool{}
	var out []AnswerRuntimeMeasurementReceipt
	for _, r := range h.RuntimeMeasurements {
		key := r.ObservationID + "\x00" + string(r.View)
		if seen[key] || r.EventSelection != nil || !BindRuntimeMeasurementReceipt(&r, c) || !r.IsBound() {
			return nil, false
		}
		seen[key] = true
		out = append(out, r)
	}
	return out, true
}
