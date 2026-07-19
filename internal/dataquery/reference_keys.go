package dataquery

import "strings"

// Reference key-universe candidates: structural metadata describing record
// fields that can define a complete output key universe for a final
// projection (complete_reference machinery and output reference grounding).
// Split from action_runner.go under the DQA LOC ratchet.

// ReferenceKeyCandidate describes a record field that can define the complete
// output key universe for a final projection. It is structural metadata only:
// callers must still decide whether the user's output contract requires using
// the candidate.
type ReferenceKeyCandidate struct {
	Path               string   `json:"path,omitempty"`
	Field              string   `json:"field,omitempty"`
	KeyCount           int      `json:"key_count,omitempty"`
	ExistingMatchCount int      `json:"existing_match_count,omitempty"`
	MissingCount       int      `json:"missing_count,omitempty"`
	Keys               []string `json:"keys,omitempty"`
	MissingKeys        []string `json:"missing_keys,omitempty"`
	// NonEmptyRowCount is the number of source records with a non-empty
	// value in Field (duplicates included). KeyCount == NonEmptyRowCount is
	// the structural key-table credential: every row defines exactly one
	// distinct key, so the field can define an output slot universe. A
	// repeating field (mapping/fact table column) is not a key table.
	NonEmptyRowCount int `json:"non_empty_row_count,omitempty"`
}

// ReferenceKeyCandidateForPath reads an explicitly declared reference key
// universe. It is structural metadata only: callers use the row count and order
// to validate final projection shape without inferring business meaning from
// field names or values.
func (r ActionRunner) ReferenceKeyCandidateForPath(path, field string, maxRecords int) (ReferenceKeyCandidate, bool) {
	path = strings.TrimSpace(path)
	field = strings.TrimSpace(field)
	if path == "" || field == "" {
		return ReferenceKeyCandidate{}, false
	}
	if maxRecords <= 0 {
		maxRecords = 100000
	}
	if r.artifactFiles == nil {
		r.artifactFiles = dataActionArtifactFilesFromSeed(r.Seed.Artifacts, EffectiveMaxFileBytes(r.MaxFileBytes))
	}
	records, headers, _, _, err := r.readActionRecords(path, maxRecords)
	if err != nil || len(records) == 0 || !actionRecordFieldExistsInRecords(headers, records, field) {
		return ReferenceKeyCandidate{}, false
	}
	seen := map[string]bool{}
	var keys []string
	nonEmptyRows := 0
	for _, record := range records {
		key := strings.TrimSpace(recordField(record.Fields, field))
		if key == "" {
			continue
		}
		nonEmptyRows++
		if seen[key] {
			continue
		}
		seen[key] = true
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return ReferenceKeyCandidate{}, false
	}
	return ReferenceKeyCandidate{
		Path:             path,
		Field:            field,
		KeyCount:         len(keys),
		Keys:             keys,
		NonEmptyRowCount: nonEmptyRows,
	}, true
}

// ReferenceKeyCandidatesForPath enumerates structural key-universe candidates
// for every readable field in a declared reference path. It does not infer
// business meaning from field names; callers score the returned value sets
// against their typed output state.
func (r ActionRunner) ReferenceKeyCandidatesForPath(path string, maxRecords int) []ReferenceKeyCandidate {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	if maxRecords <= 0 {
		maxRecords = 100000
	}
	if r.artifactFiles == nil {
		r.artifactFiles = dataActionArtifactFilesFromSeed(r.Seed.Artifacts, EffectiveMaxFileBytes(r.MaxFileBytes))
	}
	records, headers, _, _, err := r.readActionRecords(path, maxRecords)
	if err != nil || len(records) == 0 {
		return nil
	}
	fields := actionRecordFieldNames(headers, records)
	var out []ReferenceKeyCandidate
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" || !actionRecordFieldExistsInRecords(headers, records, field) {
			continue
		}
		candidate := referenceCandidateForField(path, field, records, nil)
		if candidate.KeyCount <= 0 {
			continue
		}
		out = append(out, candidate)
	}
	return out
}

func referenceCandidateForField(path, field string, records []actionRecord, existing map[string]bool) ReferenceKeyCandidate {
	seen := map[string]bool{}
	var keys []string
	matchCount := 0
	nonEmptyRows := 0
	for _, record := range records {
		value := strings.TrimSpace(recordField(record.Fields, field))
		if value == "" {
			continue
		}
		nonEmptyRows++
		if seen[value] {
			continue
		}
		seen[value] = true
		keys = append(keys, value)
		if existing[value] {
			matchCount++
		}
	}
	var missing []string
	for _, key := range keys {
		if !existing[key] {
			missing = append(missing, key)
		}
	}
	return ReferenceKeyCandidate{
		Path:               path,
		Field:              field,
		KeyCount:           len(keys),
		ExistingMatchCount: matchCount,
		MissingCount:       len(missing),
		Keys:               keys,
		MissingKeys:        missing,
		NonEmptyRowCount:   nonEmptyRows,
	}
}
