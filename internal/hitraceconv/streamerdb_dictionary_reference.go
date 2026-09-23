package hitraceconv

// traceDBDictionaryReference resolves a consumer's stored reference, without
// changing the shared dictionary's int64/TEXT namespace or global diagnostics.
// An empty TEXT value is resolved; each wire decides how it can represent it.
func traceDBDictionaryReference(raw any, dictionary map[int64]string) (string, string) {
	if raw == nil {
		return "", "null_reference"
	}
	id, ok := traceDBStrictSQLiteInt(raw)
	if !ok {
		return "", "invalid_reference_storage_class"
	}
	name, found := dictionary[id]
	if !found {
		// The global resolver distinguishes absent/invalid/ambiguous dictionary
		// rows. Its surviving map cannot safely infer which one caused this miss.
		return "", "unresolved_reference"
	}
	return name, ""
}
