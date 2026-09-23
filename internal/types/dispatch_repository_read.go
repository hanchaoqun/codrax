package types

import (
	"encoding/hex"
	"sort"
	"strings"
)

// dispatchRepositoryFileReadKey is a producer-only observation of which
// repository supplied a read. ToolReadCoverage alone cannot carry that axis:
// two repositories may expose the same relative path in a shared MutableState.
type dispatchRepositoryFileReadKey struct {
	repositoryRoot string
	path           string
	rawRef         string
}

// A pending version records producer-observed bytes and the visible line
// range. It acquires complete-read status only after the matching successful
// read_file result has been appended to this same dispatch. Neither phase is
// execution evidence, test classification, or a behavior-contract witness.
type dispatchRepositoryFileReadVersion struct {
	sha256             string
	lineStart, lineEnd int
	totalLines         int
}

// BeginDispatchRepositoryFileRead freezes the dispatch generation before IO.
// Appending unrelated results does not change it; resetting the dispatch does.
func (m *MutableState) BeginDispatchRepositoryFileRead() uint64 {
	if m == nil {
		return 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.dispatchRepositoryFileReadGeneration
}

// RecordDispatchRepositoryFileReadVersion stages a producer-only byte/range
// observation. read_file returns before BaseAgent appends its result, so this
// method deliberately grants no completed-read qualification by itself.
func (m *MutableState) RecordDispatchRepositoryFileReadVersion(generation uint64, root, path, rawRef, sha256 string, start, end, total int) bool {
	if m == nil || root == "" || path == "" || rawRef == "" || !dispatchRepositoryReadSHA256(sha256) ||
		!(total == 0 && start == 0 && end == 0 || total > 0 && start > 0 && end >= start && end <= total) {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	key := dispatchRepositoryFileReadKey{root, path, rawRef}
	if generation != m.dispatchRepositoryFileReadGeneration {
		return false
	}
	if _, ok := m.dispatchRepositoryFileReads[key]; !ok {
		return false
	}
	version := dispatchRepositoryFileReadVersion{sha256, start, end, total}
	for _, prior := range m.dispatchRepositoryFileReadVersions[key] {
		if prior == version {
			return true
		}
	}
	if m.dispatchRepositoryFileReadVersions == nil {
		m.dispatchRepositoryFileReadVersions = make(map[dispatchRepositoryFileReadKey][]dispatchRepositoryFileReadVersion)
	}
	m.dispatchRepositoryFileReadVersions[key] = append(m.dispatchRepositoryFileReadVersions[key], version)
	return true
}

// CompleteDispatchRepositoryFileReadVersion requires gap-free visible coverage
// of one exact byte version and one consistent line total. Every contributing
// page must still have its exact successful current-dispatch result. The caller
// supplies the desired byte digest; this lookup does not reread the filesystem
// or claim that the bytes are still current, executed, or behaviorally correct.
func (m *MutableState) CompleteDispatchRepositoryFileReadVersion(root, path, sha256 string) bool {
	if m == nil || root == "" || path == "" || !dispatchRepositoryReadSHA256(sha256) {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	total := -1
	var ranges []LineRange
	for key, versions := range m.dispatchRepositoryFileReadVersions {
		if key.repositoryRoot != root || key.path != path {
			continue
		}
		if _, ok := m.dispatchRepositoryFileReads[key]; !ok {
			continue
		}
		for _, version := range versions {
			if version.sha256 != sha256 || !m.dispatchRepositoryReadVersionHasResultLocked(key, version) {
				continue
			}
			if total >= 0 && total != version.totalLines {
				return false
			}
			total = version.totalLines
			ranges = append(ranges, LineRange{Start: version.lineStart, End: version.lineEnd})
		}
	}
	if total < 0 {
		return false
	}
	if total == 0 {
		return true
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].Start < ranges[j].Start })
	covered := 0
	for _, page := range ranges {
		if page.Start > covered && page.Start-covered > 1 {
			return false
		}
		if page.End > covered {
			covered = page.End
		}
	}
	return covered == total
}

func (m *MutableState) dispatchRepositoryReadVersionHasResultLocked(key dispatchRepositoryFileReadKey, version dispatchRepositoryFileReadVersion) bool {
	for _, result := range m.dispatchToolResults {
		coverage := result.ReadCoverage
		if !result.Success || result.ToolName != "read_file" || result.RuntimeArtifactRead != nil || coverage == nil ||
			result.RawRef != key.rawRef || coverage.RawRef != key.rawRef || coverage.Path != key.path ||
			coverage.LineStart != version.lineStart || coverage.LineEnd != version.lineEnd || coverage.TotalLines != version.totalLines {
			continue
		}
		if version.totalLines != 0 || ReadFileHasKnownEmptyLines(result, key.repositoryRoot) {
			return true
		}
	}
	return false
}

func dispatchRepositoryReadSHA256(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
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
