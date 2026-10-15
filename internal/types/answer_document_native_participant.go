package types

// RuntimeDiagramParticipantSelection is an explicit display choice, not an
// identity or relation credential. Every consumer rebinds NativeParticipant
// to the current native provider. The first selection for a native lane owns
// its visible declaration; other precisely bound aliases share that lane.
type RuntimeDiagramParticipantSelection struct {
	NodeID            string `json:"node_id"`
	NativeParticipant string `json:"native_participant"`
}
