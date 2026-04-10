package agent

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
)

// keywordFileScore records how a file scored across multi-level keyword matching.
type keywordFileScore struct {
	Path  string
	Score float64
	Hits  map[string]string // keyword → best match level for debugging
}

// searchLevel defines one tier of the graduated keyword search.
type searchLevel struct {
	Name   string  // human-readable label
	Score  float64 // points per hit at this level
	Search func(keyword, repoRoot string) []string // returns matching file paths
}

// Directories excluded from keyword search (same as grep tool defaults).
var searchExcludeDirs = []string{".git", ".hg", ".svn", "node_modules", "vendor", "__pycache__", ".tox"}

// keywordSearch runs a graduated multi-level search for the given keywords
// against the repository, returning files ranked by weighted score.
//
// Levels (highest confidence first):
//  1. Exact filename match   (score 10) — file basename contains keyword exactly
//  2. Exact content match    (score  6) — grep -l case-sensitive
//  3. Case-insensitive filename (score 4) — file basename contains keyword (case-blind)
//  4. Case-insensitive content  (score 3) — grep -li
//  5. Prefix/stem match      (score  1) — grep -li with keyword truncated to stem
//
// Each file's total score is the sum across all keywords × levels. A file
// that matches keyword A exactly in content (6) and keyword B in filename (10)
// scores 16. Files already matched at a higher level for the same keyword
// are not double-counted at lower levels.
func keywordSearch(keywords []string, repoRoot string) []keywordFileScore {
	if len(keywords) == 0 || repoRoot == "" {
		return nil
	}

	// Expand each keyword into identifier-format variants so the
	// search covers CamelCase, snake_case, concatenated, etc.
	// regardless of which format the analyzer happened to produce.
	keywords = expandKeywords(keywords)

	levels := []searchLevel{
		{Name: "filename-exact", Score: 10, Search: func(kw, root string) []string {
			return findFilesByName(kw, root, false)
		}},
		{Name: "content-exact", Score: 6, Search: func(kw, root string) []string {
			return grepFiles(kw, root, false)
		}},
		{Name: "filename-icase", Score: 4, Search: func(kw, root string) []string {
			return findFilesByName(kw, root, true)
		}},
		{Name: "content-icase", Score: 3, Search: func(kw, root string) []string {
			return grepFiles(kw, root, true)
		}},
		{Name: "stem-icase", Score: 1, Search: func(kw, root string) []string {
			stem := keywordStem(kw)
			if stem == kw || len(stem) < 3 {
				return nil // no useful stem or too short
			}
			return grepFiles(stem, root, true)
		}},
	}

	// scores[path][keyword] = best level name (to prevent double-counting)
	bestLevel := make(map[string]map[string]string)
	fileScores := make(map[string]float64)

	for _, kw := range keywords {
		for _, level := range levels {
			paths := level.Search(kw, repoRoot)
			if len(paths) > 0 {
				logging.Debug("[keyword_search] %s %q → %d hits", level.Name, kw, len(paths))
			}
			for _, p := range paths {
				p = normalizeSearchPath(p, repoRoot)
				if isNoisePath(p) {
					continue
				}
				if bestLevel[p] == nil {
					bestLevel[p] = make(map[string]string)
				}
				if _, already := bestLevel[p][kw]; already {
					continue // this keyword already matched at a higher level
				}
				bestLevel[p][kw] = level.Name
				fileScores[p] += level.Score
			}
		}
	}

	// Convert to sorted slice.
	results := make([]keywordFileScore, 0, len(fileScores))
	for path, score := range fileScores {
		results = append(results, keywordFileScore{
			Path:  path,
			Score: score,
			Hits:  bestLevel[path],
		})
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Cap at top 20 to keep the prompt reasonable.
	if len(results) > 20 {
		results = results[:20]
	}

	logging.Debug("[keyword_search] %d keywords → %d files scored", len(keywords), len(results))
	return results
}

// formatKeywordResults renders scored files for injection into the Phase 1 prompt.
func formatKeywordResults(results []keywordFileScore) string {
	if len(results) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("### Pre-scanned File Ranking\n\n")
	b.WriteString("The following files were found by graduated keyword search (exact match → case-insensitive → stem). ")
	b.WriteString("Higher scores mean more keywords matched at higher precision levels. ")
	b.WriteString("Use this ranking to prioritize your file list — but verify with your own grep/repo_map.\n\n")
	b.WriteString("| Score | File | Matched Keywords |\n")
	b.WriteString("|------:|------|------------------|\n")
	for _, r := range results {
		kwList := make([]string, 0, len(r.Hits))
		for kw, level := range r.Hits {
			kwList = append(kwList, fmt.Sprintf("%s(%s)", kw, level))
		}
		sort.Strings(kwList)
		fmt.Fprintf(&b, "| %.0f | %s | %s |\n", r.Score, r.Path, strings.Join(kwList, ", "))
	}
	b.WriteString("\n")
	return b.String()
}

// --- low-level search helpers ---

// grepFiles runs grep -rlEI on the repo and returns matching paths.
func grepFiles(pattern, repoRoot string, ignoreCase bool) []string {
	args := []string{"-rlEI"}
	if ignoreCase {
		args = []string{"-rlEIi"}
	}
	for _, dir := range searchExcludeDirs {
		args = append(args, "--exclude-dir="+dir)
	}
	args = append(args, pattern, repoRoot)

	cmd := exec.Command("grep", args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil // exit 1 = no matches, or error — either way no results
	}
	return splitLines(stdout.String())
}

// findFilesByName uses find to locate files whose basename contains the keyword.
func findFilesByName(keyword, repoRoot string, ignoreCase bool) []string {
	// Build find command with directory exclusions.
	args := []string{repoRoot}
	for _, dir := range searchExcludeDirs {
		args = append(args, "-path", "*/"+dir, "-prune", "-o")
	}
	nameFlag := "-name"
	if ignoreCase {
		nameFlag = "-iname"
	}
	args = append(args, nameFlag, "*"+keyword+"*", "-type", "f", "-print")

	cmd := exec.Command("find", args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil
	}
	return splitLines(stdout.String())
}

// keywordStem extracts a shorter stem from a keyword for fuzzy matching.
// Handles CamelCase splitting and underscore splitting.
func keywordStem(kw string) string {
	// Try underscore split: "sub_agent" → "sub" (too short), try "agent"
	if parts := strings.Split(kw, "_"); len(parts) > 1 {
		// Return the longest part
		longest := ""
		for _, p := range parts {
			if len(p) > len(longest) {
				longest = p
			}
		}
		return strings.ToLower(longest)
	}

	// Try CamelCase split: "SubAgent" → "Agent" (longest), "SubAgentRuntime" → "Runtime" or "Agent"
	parts := splitCamelCase(kw)
	if len(parts) > 1 {
		longest := ""
		for _, p := range parts {
			if len(p) > len(longest) {
				longest = p
			}
		}
		return strings.ToLower(longest)
	}

	return strings.ToLower(kw)
}

// expandKeywords takes the analyzer's keywords and generates identifier-
// format variants for each multi-word keyword. Single generic words
// (e.g. "call", "invoke") are kept as-is. Multi-part keywords get
// expanded into CamelCase, snake_case, concatenated, and hyphenated
// forms so the search covers all common naming conventions.
//
// Example: "sub_agent" → ["sub_agent", "SubAgent", "subagent", "sub-agent"]
func expandKeywords(keywords []string) []string {
	seen := make(map[string]bool, len(keywords)*4)
	var expanded []string

	add := func(kw string) {
		if kw == "" || len(kw) < 2 {
			return
		}
		lower := strings.ToLower(kw)
		if seen[lower] {
			return
		}
		seen[lower] = true
		expanded = append(expanded, kw)
	}

	for _, kw := range keywords {
		kw = strings.TrimSpace(kw)
		if kw == "" {
			continue
		}

		// Always keep the original.
		add(kw)

		// Split into word parts using any of: underscore, hyphen, CamelCase.
		parts := splitIntoParts(kw)
		if len(parts) <= 1 {
			// No separators or CamelCase — try splitting using other
			// keywords as a dictionary (e.g. "subagent" + known "agent"
			// → ["sub", "agent"]).
			if split := trySplitConcatenated(kw, keywords); split != nil {
				parts = split
			} else {
				continue
			}
		}

		// Generate all common identifier formats from the parts.
		// CamelCase: SubAgent
		var camel strings.Builder
		for _, p := range parts {
			if len(p) > 0 {
				camel.WriteString(strings.ToUpper(p[:1]) + strings.ToLower(p[1:]))
			}
		}
		add(camel.String())

		// snake_case: sub_agent
		lowerParts := make([]string, len(parts))
		for i, p := range parts {
			lowerParts[i] = strings.ToLower(p)
		}
		add(strings.Join(lowerParts, "_"))

		// concatenated: subagent
		add(strings.Join(lowerParts, ""))

		// hyphenated: sub-agent
		add(strings.Join(lowerParts, "-"))
	}

	logging.Debug("[keyword_search] expanded %d → %d keywords", len(keywords), len(expanded))
	return expanded
}

// splitIntoParts splits a keyword into word parts by underscore, hyphen,
// or CamelCase boundaries. For concatenated lowercase words (e.g.
// "subagent"), it tries to split using other keywords as known words.
func splitIntoParts(kw string) []string {
	// First try explicit separators.
	if strings.Contains(kw, "_") {
		return nonEmpty(strings.Split(kw, "_"))
	}
	if strings.Contains(kw, "-") {
		return nonEmpty(strings.Split(kw, "-"))
	}
	// Try CamelCase.
	parts := splitCamelCase(kw)
	if len(parts) > 1 {
		return parts
	}
	return []string{kw}
}

// trySplitConcatenated attempts to split a concatenated lowercase word
// like "subagent" into parts using other keywords as a dictionary.
// Returns the parts if a valid split is found, otherwise nil.
func trySplitConcatenated(kw string, allKeywords []string) []string {
	lower := strings.ToLower(kw)
	if len(lower) < 4 {
		return nil
	}
	// Try each other keyword as a suffix or prefix.
	for _, other := range allKeywords {
		ol := strings.ToLower(other)
		if ol == lower || len(ol) < 2 {
			continue
		}
		// Check if kw = prefix + other
		if strings.HasSuffix(lower, ol) {
			prefix := lower[:len(lower)-len(ol)]
			if len(prefix) >= 2 {
				return []string{prefix, ol}
			}
		}
		// Check if kw = other + suffix
		if strings.HasPrefix(lower, ol) {
			suffix := lower[len(ol):]
			if len(suffix) >= 2 {
				return []string{ol, suffix}
			}
		}
	}
	return nil
}

func nonEmpty(parts []string) []string {
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// splitCamelCase splits "SubAgentRuntime" into ["Sub", "Agent", "Runtime"].
func splitCamelCase(s string) []string {
	var parts []string
	start := 0
	for i := 1; i < len(s); i++ {
		if s[i] >= 'A' && s[i] <= 'Z' && s[i-1] >= 'a' && s[i-1] <= 'z' {
			parts = append(parts, s[start:i])
			start = i
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// normalizeSearchPath strips the repo root prefix and leading ./ from paths.
func normalizeSearchPath(path, repoRoot string) string {
	abs, err := filepath.Abs(repoRoot)
	if err == nil {
		path = strings.TrimPrefix(path, abs+"/")
	}
	path = strings.TrimPrefix(path, repoRoot+"/")
	path = strings.TrimPrefix(path, "./")
	return path
}

func splitLines(s string) []string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	result := make([]string, 0, len(lines))
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			result = append(result, l)
		}
	}
	return result
}
