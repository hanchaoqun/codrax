package types

// dispatchRepositoryFileReadKey is a producer-only observation of which
// repository supplied a read. ToolReadCoverage alone cannot carry that axis:
// two repositories may expose the same relative path in a shared MutableState.
type dispatchRepositoryFileReadKey struct {
	repositoryRoot string
	path           string
	rawRef         string
}

// RecordDispatchRepositoryFileRead records a successful read_file's resolved
// physical repository identity. Callers supply canonical producer-resolved
// identities, never model references. This records inspection only: it is not
// a test-file classification, complete-file read, or verification witness.
func (m *MutableState) RecordDispatchRepositoryFileRead(repositoryRoot, path, rawRef string) {
	if m == nil || repositoryRoot == "" || path == "" || rawRef == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dispatchRepositoryFileReads == nil {
		m.dispatchRepositoryFileReads = make(map[dispatchRepositoryFileReadKey]struct{})
	}
	m.dispatchRepositoryFileReads[dispatchRepositoryFileReadKey{repositoryRoot, path, rawRef}] = struct{}{}
}

// HasDispatchRepositoryFileRead matches all three producer identities. It
// deliberately does not infer a repository from a relative path or blob name.
func (m *MutableState) HasDispatchRepositoryFileRead(repositoryRoot, path, rawRef string) bool {
	if m == nil || repositoryRoot == "" || path == "" || rawRef == "" {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.dispatchRepositoryFileReads[dispatchRepositoryFileReadKey{repositoryRoot, path, rawRef}]
	return ok
}
