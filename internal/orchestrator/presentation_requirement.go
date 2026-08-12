package orchestrator

// SetPresentationDiagramRequired installs the precise current-turn hard
// visual authority. It is consume-once and intentionally independent from the
// free-form presentation directive.
func (o *Orchestrator) SetPresentationDiagramRequired(required bool) {
	o.presentationDiagramRequired = required
}
