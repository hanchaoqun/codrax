package types

import "strings"

func sourceInventoryPrincipalRowVisibleNoteSegments(row SourceInventoryRow) []string {
	note := row.Member.Note
	note = strings.TrimSpace(note)
	if note == "" {
		return nil
	}
	carrierValues := sourceInventoryPrincipalRowTypedNoteCarrierValues(row)
	var out []string
	for _, part := range strings.Split(note, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if sourceInventoryPrincipalRowNoteSegmentIsSystemCarrier(part, carrierValues) {
			continue
		}
		out = append(out, part)
	}
	return out
}
