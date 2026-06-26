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
		if err != nil || n <= 0 {
			return "", LineRange{}, 0, false
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
		if coverage.TotalLines > 0 && coverage.TotalLines > totals[path] {
			totals[path] = coverage.TotalLines
		}
	}
	return readSet, readRanges, totals
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
