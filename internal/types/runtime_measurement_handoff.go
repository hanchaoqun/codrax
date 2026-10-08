package types

// This retained choice is navigation/presentation intent, never evidence or
// execution authority. Every consumer must rebind against the current native
// contract; no values, rows, BoundTable or completion waiver are retained.
func cloneMeasurementMemberSetSelections(in []AnswerRuntimeMeasurementReceipt) []AnswerRuntimeMeasurementReceipt {
	var out []AnswerRuntimeMeasurementReceipt
	for _, r := range in {
		out = append(out, AnswerRuntimeMeasurementReceipt{ObservationID: r.ObservationID, View: r.View})
	}
	return out
}

func (m *MutableState) SetInvestigationMeasurementMemberSets(in []AnswerRuntimeMeasurementReceipt) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.investigationComplete {
		m.investigationMeasurementMemberSets = cloneMeasurementMemberSetSelections(in)
	}
}

func (m *MutableState) InvestigationMeasurementMemberSets() []AnswerRuntimeMeasurementReceipt {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneMeasurementMemberSetSelections(m.investigationMeasurementMemberSets)
}
