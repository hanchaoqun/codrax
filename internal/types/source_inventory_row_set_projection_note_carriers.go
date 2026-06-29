package types

import "strings"

func sourceInventoryPrincipalRowTypedNoteCarrierValues(row SourceInventoryRow) []string {
	var out []string
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	add(row.Member.Name)
	add(row.Member.Key)
	add(row.Member.Language)
	add(row.Member.File)
	add(sourceInventoryPrincipalRowLocation(row))
	for _, term := range row.Member.SurfaceTerms {
		add(term)
	}
	for _, attr := range row.Member.Attributes {
		add(attr.Name)
		add(attr.Language)
		add(attr.File)
		if attr.File != "" && attr.Line > 0 {
			add(aggregateSupportLocationKeyForDisplay(attr.File, attr.Line))
		}
		for _, term := range attr.SurfaceTerms {
			add(term)
		}
	}
	return out
}

func sourceInventoryPrincipalRowNoteSegmentIsSystemCarrier(segment string, carrierValues []string) bool {
	if sourceInventoryPrincipalRowNoteValueMatchesCarrier(segment, carrierValues) {
		return true
	}
	key, value, ok := strings.Cut(strings.TrimSpace(segment), "=")
	if !ok || !sourceInventoryPrincipalRowSchemaKey(key) {
		return false
	}
	value = strings.TrimSpace(value)
	if sourceInventoryPrincipalRowNoteValueMatchesCarrier(value, carrierValues) {
		return true
	}
	for _, term := range enumerationDisplayMemberNoteSurfaceTerms(segment) {
		if sourceInventoryPrincipalRowNoteValueMatchesCarrier(term, carrierValues) {
			return true
		}
	}
	return false
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

func sourceInventoryPrincipalRowNoteValueMatchesCarrier(value string, carrierValues []string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	valueKey := aggregateMemberSetProjectionMemberKey(value)
	if valueKey == "" {
		return false
	}
	for _, carrier := range carrierValues {
		carrier = strings.TrimSpace(carrier)
		if carrier == "" {
			continue
		}
		carrierKey := aggregateMemberSetProjectionMemberKey(carrier)
		if valueKey == carrierKey || sourceInventoryPrincipalRowNoteLocationValueMatches(value, carrier) {
			return true
		}
	}
	return false
}

func sourceInventoryPrincipalRowNoteLocationValueMatches(value, carrier string) bool {
	valueKey := sourceInventoryPrincipalRowNoteLocationKey(value)
	carrierKey := sourceInventoryPrincipalRowNoteLocationKey(carrier)
	return valueKey != "" && valueKey == carrierKey
}

func sourceInventoryPrincipalRowNoteLocationKey(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if _, loc, ok := ParseAnswerSupportRefMemberLocation(raw); ok {
		return normalizeAnswerSupportLocation(aggregateMemberStartLocation(loc))
	}
	if loc, ok := parseAnswerSupportLocationSurface(raw); ok {
		return normalizeAnswerSupportLocation(aggregateMemberStartLocation(loc))
	}
	if idx := strings.LastIndex(raw, " @ "); idx >= 0 {
		if loc, ok := parseAnswerSupportLocationSurface(strings.TrimSpace(raw[idx+len(" @ "):])); ok {
			return normalizeAnswerSupportLocation(aggregateMemberStartLocation(loc))
		}
	}
	return ""
}
