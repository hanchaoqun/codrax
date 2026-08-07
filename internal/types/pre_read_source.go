package types

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/canonpath"
)

// RecordPreReadSource records the exact line slice injected into an explorer
// prompt. It never re-reads the filesystem, so later grounding sees precisely
// the bytes the model saw.
func (m *MutableState) RecordPreReadSource(file string, lines []string) {
	if m == nil || len(lines) == 0 {
		return
	}
	file = canonpath.CanonicalRepoRelative(strings.TrimSpace(file), m.repoRoot)
	if file == "" || file == "." {
		return
	}
	next := make(map[int]string, len(lines))
	for i, line := range lines {
		next[i+1] = line
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if preReadSourceFileEqual(m.preReadSourceLines[file], next) {
		return
	}
	if m.preReadSourceLines == nil {
		m.preReadSourceLines = make(map[string]map[int]string)
	}
	m.preReadSourceLines[file] = next
	m.preReadSourceRevision++
}

func clonePreReadSourceLines(in map[string]map[int]string) map[string]map[int]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]map[int]string, len(in))
	for file, lines := range in {
		copyLines := make(map[int]string, len(lines))
		for line, text := range lines {
			copyLines[line] = text
		}
		out[file] = copyLines
	}
	return out
}

func preReadSourceFileEqual(left, right map[int]string) bool {
	if len(left) != len(right) {
		return false
	}
	for line, text := range left {
		if right[line] != text {
			return false
		}
	}
	return true
}

func mergePreReadSourceLines(dst, src map[string]map[int]string) bool {
	changed := false
	for file, lines := range src {
		if preReadSourceFileEqual(dst[file], lines) {
			continue
		}
		copyLines := make(map[int]string, len(lines))
		for line, text := range lines {
			copyLines[line] = text
		}
		dst[file] = copyLines
		changed = true
	}
	return changed
}
