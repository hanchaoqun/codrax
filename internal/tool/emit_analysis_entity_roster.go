package tool

// emit_analysis_entity_roster.go — the analyzer's entity roster frozen
// immediately after decode (V4-3 §40.21 ③, §40.47 fold-in A5).
//
// Ruling ③: a hard gate's input must be the model's ORIGINAL emission or a
// lossless normalization of it. The local `entities` slice inside
// (*EmitAnalysis).Execute shares its backing array with the persisted
// RequestModel roster and is handed to several helpers before the diagram
// gate runs, so a census over assignments to one identifier could not prove
// what the gate actually read. modelEntityRoster is the structural answer:
// the roster is copied once, right after the schema decode and blocklist
// filter, into a value whose only accessor returns a fresh copy. The
// participant gate judges that value and the persisted roster is minted from
// it, so no later in-place rewrite — on the local slice, on the RequestModel
// field, or through a pointer helper — can change either. The census in
// emit_analysis_entity_roster_census_test.go pins that the capture happens
// after the last decode assignment, that the frozen value is never written
// after capture, and that the gate reads no other roster.

// modelEntityRoster is the frozen, read-only entity roster.
type modelEntityRoster struct {
	entities []string
}

// freezeModelEntityRoster copies the decoded roster. nil stays nil so the
// persisted carrier keeps its pre-freeze JSON shape.
func freezeModelEntityRoster(entities []string) modelEntityRoster {
	if entities == nil {
		return modelEntityRoster{}
	}
	frozen := make([]string, len(entities))
	copy(frozen, entities)
	return modelEntityRoster{entities: frozen}
}

// Entities returns a fresh copy of the frozen roster; the backing array is
// never exposed.
func (r modelEntityRoster) Entities() []string {
	if r.entities == nil {
		return nil
	}
	out := make([]string, len(r.entities))
	copy(out, r.entities)
	return out
}
