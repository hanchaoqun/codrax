package types

// SourceInventoryObservationFromMutable returns the current durable
// source-inventory view, including Turn A handoff state and the closure-side
// source-class universe. Consumers that need the complete typed inventory
// authority should use this instead of reading one carrier directly.
func SourceInventoryObservationFromMutable(m *MutableState) SourceInventoryObservation {
	if m == nil {
		return SourceInventoryObservation{}
	}
	observation := sourceInventoryObservationFromMutableFields(m.SourceInventoryObservation(), m.TurnAArtifacts())
	if closure := m.EvidenceClosure(); closure != nil {
		observation = MergeSourceInventoryObservation(observation, closure.SourceInventoryObservation())
	}
	return observation
}

func sourceInventoryObservationFromMutableFields(current SourceInventoryObservation, turnA *TurnAArtifacts) SourceInventoryObservation {
	observation := CloneSourceInventoryObservation(current)
	if turnA != nil {
		observation = MergeSourceInventoryObservation(observation, turnA.SourceInventoryObservation)
		if !observation.IsActive() && turnA.SourceInventoryAdvisory.IsActive() {
			observation = SourceInventoryObservationFromAdvisory(turnA.SourceInventoryAdvisory)
		}
	}
	return observation
}
