package types

import "strings"

func sourceInventoryPrincipalRowVisibleNoteSegments(note string) []string {
	note = strings.TrimSpace(note)
	if note == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(note, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if sourceInventoryPrincipalRowNoteSegmentIsSystemCarrier(part) {
			continue
		}
		out = append(out, part)
	}
	return out
}

func sourceInventoryPrincipalRowNoteSegmentIsSystemCarrier(segment string) bool {
	if len(enumerationDisplayMemberNoteSurfaceTerms(segment)) > 0 {
		return true
	}
	key, value, ok := strings.Cut(strings.TrimSpace(segment), "=")
	if !ok || !sourceInventoryPrincipalRowSchemaKey(key) {
		return false
	}
	return sourceInventoryPrincipalRowSchemaPayloadLooksMachineAtom(value)
}

func sourceInventoryPrincipalRowSchemaKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" || len(key) > 48 {
		return false
	}
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-' || r == '.':
		default:
			return false
		}
	}
	return true
}

func sourceInventoryPrincipalRowSchemaPayloadLooksMachineAtom(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, " \t\n\r") {
		return false
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-' || r == '.' || r == '/' || r == ':':
		default:
			return false
		}
	}
	return true
}
