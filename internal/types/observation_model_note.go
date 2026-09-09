package types

import (
	"encoding/json"
	"strings"
)

// ObservationModelNote preserves an aggregate member's proposed explanation
// separately from the observation's measured/declaration facts. MemberIndex is
// the original zero-based position; SupportRef locates that member, not proof
// that Text is true. Record authority never promotes these candidate notes.
type ObservationModelNote struct {
	MemberIndex int    `json:"member_index"`
	Member      string `json:"member,omitempty"`
	Text        string `json:"text"`
	SupportRef  string `json:"support_ref,omitempty"`
}

func aggregateFactObservationModelNoteAt(fact AnswerAggregateFact, i int) *ObservationModelNote {
	if i < 0 || i >= len(fact.MemberNotes) || strings.TrimSpace(fact.MemberNotes[i]) == "" {
		return nil
	}
	note := &ObservationModelNote{MemberIndex: i, Text: strings.TrimSpace(fact.MemberNotes[i])}
	if i < len(fact.Members) {
		note.Member = strings.TrimSpace(fact.Members[i])
	}
	// A shorter support list is not position-aligned. Preserve it on the
	// enclosing fact/record, but do not manufacture a member-to-ref binding.
	if len(fact.SupportRefs) == len(fact.Members) && i < len(fact.SupportRefs) {
		note.SupportRef = strings.TrimSpace(fact.SupportRefs[i])
	}
	return note
}

func aggregateFactObservationModelNotes(fact AnswerAggregateFact) []ObservationModelNote {
	var out []ObservationModelNote
	for i := range fact.MemberNotes {
		if note := aggregateFactObservationModelNoteAt(fact, i); note != nil {
			out = append(out, *note)
		}
	}
	return out
}

func cloneObservationModelNotes(notes []ObservationModelNote) []ObservationModelNote {
	return append([]ObservationModelNote(nil), notes...)
}

func mergeObservationModelNotes(a, b []ObservationModelNote) []ObservationModelNote {
	var out []ObservationModelNote
	seen := make(map[ObservationModelNote]bool, len(a)+len(b))
	for _, notes := range [][]ObservationModelNote{a, b} {
		for _, note := range notes {
			if !seen[note] {
				seen[note] = true
				out = append(out, note)
			}
		}
	}
	return out
}

// FormatObservationModelNotes is shared by compact prompt renderers. It never
// interprets the prose or derives a claim form from an aligned source location.
// Legacy records without the typed field are not retrospectively classified.
func FormatObservationModelNotes(notes []ObservationModelNote) string {
	if len(notes) == 0 {
		return ""
	}
	raw, _ := json.Marshal(notes)
	return "model_notes/advisory=" + string(raw) + " (candidate explanations; not covered by record claim_authority; support_ref locates the member, not proof of the explanation)"
}
