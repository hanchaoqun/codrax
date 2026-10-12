package types

// RuntimeDiagramNoteSelection is an explicit display selection, not evidence.
// Only the current private native measurement contract can resolve Selector.
// BodyLine addresses one existing sequence Note statement; no text is parsed
// to infer its source, owner, event, business role, or timestamp.
type RuntimeDiagramNoteSelection struct {
	Selector string `json:"selector"`
	Point    string `json:"point"`
	NodeID   string `json:"node_id"`
	BodyLine int    `json:"body_line"`
}
