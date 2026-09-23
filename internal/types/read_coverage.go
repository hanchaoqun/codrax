package types

import (
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/canonpath"
)

// ParseReadFileBanner parses the first-line banner emitted by read_file.
// Supported shapes:
//
//   - [path: showing lines X-Y of Z total]
//   - [path: showing all N lines (B bytes); ...]
//
// Forced-read prefixes are stripped before parsing. The parser consumes only
// tool-owned banner structure; it must not inspect model-authored prose.
func ParseReadFileBanner(summary string) (path string, rng LineRange, totalLines int, ok bool) {
	first := strings.SplitN(summary, "\n", 2)[0]
	first = stripReadFileCoveragePrefix(first)
	if !strings.HasPrefix(first, "[") {
		return "", LineRange{}, 0, false
	}
	if idx := strings.Index(first, ": showing all "); idx > 1 {
		path = first[1:idx]
		if path == "" {
			return "", LineRange{}, 0, false
		}
		rest := first[idx+len(": showing all "):]
		spaceIdx := strings.Index(rest, " ")
		if spaceIdx < 1 {
			return "", LineRange{}, 0, false
		}
		n, err := strconv.Atoi(rest[:spaceIdx])
		if err != nil || n < 0 {
			return "", LineRange{}, 0, false
		}
		if n == 0 {
			return path, LineRange{}, 0, true
		}
		return path, LineRange{Start: 1, End: n}, n, true
	}
	colonIdx := strings.Index(first, ": showing lines ")
	if colonIdx <= 1 {
		return "", LineRange{}, 0, false
	}
	path = first[1:colonIdx]
	rest := first[colonIdx+len(": showing lines "):]
	dashIdx := strings.Index(rest, "-")
	ofIdx := strings.Index(rest, " of ")
	if dashIdx < 0 || ofIdx < 0 || dashIdx > ofIdx {
		return "", LineRange{}, 0, false
	}
	startLine, err1 := strconv.Atoi(strings.TrimSpace(rest[:dashIdx]))
	endLine, err2 := strconv.Atoi(strings.TrimSpace(rest[dashIdx+1 : ofIdx]))
	if err1 != nil || err2 != nil || startLine <= 0 || endLine < startLine {
		return "", LineRange{}, 0, false
	}
	totalStr := strings.TrimSuffix(strings.TrimSuffix(rest[ofIdx+4:], "]"), " total")
	total, _ := strconv.Atoi(strings.TrimSpace(totalStr))
	return path, LineRange{Start: startLine, End: endLine}, total, true
}

// ExtractReadCoverage walks tool results and rebuilds the read_file coverage
// projection for a single round or accumulated history from the typed
// ToolReadCoverage carrier. It intentionally ignores the rendered read_file
// banner: Summary is transparent user/model context, not a coverage authority.
func ExtractReadCoverage(history []ToolResult, repoRoot string) (
	readSet map[string]bool,
	readRanges map[string][]LineRange,
	totals map[string]int,
) {
	readSet = make(map[string]bool)
	readRanges = make(map[string][]LineRange)
	totals = make(map[string]int)
	for _, r := range history {
		if !r.Success || r.ToolName != "read_file" {
			continue
		}
		coverage := r.ReadCoverage
		if coverage == nil {
			continue
		}
		path := canonicalReadCoveragePath(coverage.Path, repoRoot)
		if path == "" {
			continue
		}
		if ReadFileHasKnownEmptyLines(r, repoRoot) {
			readSet[path] = true
			if _, known := totals[path]; !known && len(readRanges[path]) == 0 {
				totals[path] = 0
			}
			if _, observed := readRanges[path]; !observed {
				readRanges[path] = nil
			}
			continue
		}
		// Zero coordinates without the producer's complete empty enumeration
		// are not a line read and must not be repaired into a fictional line 1.
		if coverage.LineStart == 0 && coverage.LineEnd == 0 {
			continue
		}
		start := coverage.LineStart
		if start <= 0 {
			start = 1
		}
		end := coverage.LineEnd
		if end < start {
			end = start
		}
		readSet[path] = true
		readRanges[path] = append(readRanges[path], LineRange{Start: start, End: end})
		if total, known := totals[path]; known && total == 0 {
			delete(totals, path) // a real positive range contradicts empty, not its bytes
		}
		if coverage.TotalLines > 0 && coverage.TotalLines > totals[path] {
			totals[path] = coverage.TotalLines
		}
	}
	return readSet, readRanges, totals
}

// ReadFileHasKnownEmptyLines recognizes an actual empty read using existing
// producer-owned carriers. Zero-valued legacy/missing coverage alone means
// unknown, not empty. Runtime and current-source carriers remain exclusive.
func ReadFileHasKnownEmptyLines(r ToolResult, repoRoot string) bool {
	if !r.Success || r.ToolName != "read_file" || strings.TrimSpace(r.RawRef) == "" ||
		r.EnumerationAuthority == nil || r.EnumerationAuthority.Status != "complete" ||
		len(r.EnumerationAuthority.Boundaries) != 1 {
		return false
	}
	b := r.EnumerationAuthority.Boundaries[0]
	if b.Dimension != "lines" || !b.TotalKnown || b.Total != 0 || b.Emitted != 0 || strings.TrimSpace(b.Scope) == "" {
		return false
	}
	var path, rawRef string
	var start, end, total int
	switch {
	case r.ReadCoverage != nil && r.RuntimeArtifactRead == nil:
		c := r.ReadCoverage
		path, rawRef, start, end, total = c.Path, c.RawRef, c.LineStart, c.LineEnd, c.TotalLines
	case r.RuntimeArtifactRead != nil && r.ReadCoverage == nil:
		c := r.RuntimeArtifactRead
		path, rawRef, start, end, total = c.RequestedPath, c.RawRef, c.LineStart, c.LineEnd, c.TotalLines
	default:
		return false
	}
	return start == 0 && end == 0 && total == 0 && rawRef == r.RawRef &&
		canonicalReadCoveragePath(path, repoRoot) != "" &&
		canonicalReadCoveragePath(path, repoRoot) == canonicalReadCoveragePath(b.Scope, repoRoot)
}

func canonicalReadCoveragePath(path, repoRoot string) string {
	if path == "" {
		return ""
	}
	if repoRoot != "" {
		return canonpath.CanonicalRepoRelative(path, repoRoot)
	}
	cleaned := strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")
	return strings.TrimPrefix(cleaned, "./")
}

func stripReadFileCoveragePrefix(line string) string {
	for _, prefix := range []string{"[forced_read surgical] ", "[forced_read] "} {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return line
}
