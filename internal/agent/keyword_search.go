package agent

import (
	"bytes"
	"fmt"
	"math"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
)

// keywordFileScore records how a file scored across multi-level keyword matching.
type keywordFileScore struct {
	Path         string
	Score        float64
	RepoMapScore float64           // raw repo_map structural score (for coverage selection)
	Hits         map[string]string // keyword → best match level for debugging
	Symbols      []string          // symbol summaries from repo_map (e.g. "RegisterDefaultSubAgents function:63")
}

// Directories excluded from keyword search (same as grep tool defaults).
var searchExcludeDirs = []string{".git", ".hg", ".svn", "node_modules", "vendor", "__pycache__", ".tox"}

// keywordSearch combines repo_map's structural ranking with grep-based
// keyword matching to produce a scored file list.
//
// Strategy:
//  1. Build/load the repo_map graph (cached, fast) and rank files using
//     the keywords as query. This gives structural signal: files that
//     DEFINE matching symbols score higher than files that merely mention
//     them in comments.
//  2. Run grep for each expanded keyword and compute IDF-weighted scores.
//     Keywords matching fewer files are more informative (IDF = log(N/df)).
//  3. Merge: repo_map score (normalized) + grep IDF score, with repo_map
//     weighted higher because structural definitions are more reliable
//     than text mentions.
func keywordSearch(keywords []string, repoRoot string) []keywordFileScore {
	if len(keywords) == 0 || repoRoot == "" {
		return nil
	}

	keywords = expandKeywords(keywords)

	// --- Phase 1: repo_map structural ranking ---
	repoMapScores, graph := repoMapRank(keywords, repoRoot)

	// --- Phase 2: grep IDF-weighted scoring ---
	grepScores, grepHits := grepIDFSearch(keywords, repoRoot)

	// --- Phase 3: merge ---
	// Only score files that grep found (keyword-relevant). Repo_map
	// provides a structural boost but doesn't introduce new files —
	// this prevents infrastructure files with high structural scores
	// (logger.go, parser.go) from dominating when they don't match
	// any domain keywords.

	// Normalize repo_map scores to 0-1 range for boost calculation.
	maxRM := 0.0
	for _, s := range repoMapScores {
		if s > maxRM {
			maxRM = s
		}
	}

	results := make([]keywordFileScore, 0, len(grepScores))
	for f, grepScore := range grepScores {
		if isNoisePath(f) {
			continue
		}

		// Repo_map boost: files with higher structural importance
		// get a multiplier on their grep score. The boost ranges
		// from 1.0 (no structural signal) to 2.0 (top structural).
		boost := 1.0
		if maxRM > 0 && repoMapScores[f] > 0 {
			boost = 1.0 + (repoMapScores[f]/maxRM)*1.0
		}
		combined := grepScore * boost

		hits := grepHits[f]
		if hits == nil {
			hits = make(map[string]string)
		}
		if repoMapScores[f] > 0 {
			hits["repo_map"] = fmt.Sprintf("%.0f", repoMapScores[f])
		}

		// Extract symbol summaries from repo_map graph.
		var syms []string
		if graph != nil {
			fi, ok := graph.FileIndex[f]
			if !ok {
				logging.Debug("[keyword_search] symbols: %s not in FileIndex", f)
			}
			if ok {
				for _, sym := range fi.Symbols {
					if sym.Exported || sym.Kind == "function" || sym.Kind == "method" {
						summary := fmt.Sprintf("%s %s:%d", sym.Name, sym.Kind, sym.Line)
						if sym.Signature != "" {
							summary += " " + sym.Signature
						}
						syms = append(syms, summary)
					}
				}
			}
		}

		results = append(results, keywordFileScore{
			Path:         f,
			Score:        combined,
			RepoMapScore: repoMapScores[f],
			Hits:         hits,
			Symbols:      syms,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	if len(results) > 20 {
		results = results[:20]
	}

	logging.Debug("[keyword_search] %d keywords → %d files scored (repo_map + grep IDF)", len(keywords), len(results))
	return results
}

// repoMapRank uses the repo_map graph to rank files by structural relevance.
// Only returns files that matched the query (QueryScores > 0), so
// infrastructure files with high structural scores but no query relevance
// are excluded. Also returns the graph for symbol extraction.
func repoMapRank(keywords []string, repoRoot string) (scores map[string]float64, graph *repomap.Graph) {
	query := strings.Join(keywords, " ")
	var err error
	graph, err = repomap.BuildOrLoadGraph(repoRoot, query)
	if err != nil {
		logging.Debug("[keyword_search] repo_map unavailable: %v", err)
		return nil, nil
	}

	// Only include files that actually matched the query.
	scores = make(map[string]float64)
	for path, qScore := range graph.QueryScores {
		if qScore > 0 {
			scores[path] = graph.Scores[path]
		}
	}
	logging.Debug("[keyword_search] repo_map: %d files matched query (of %d total)", len(scores), len(graph.Scores))
	return scores, graph
}

// grepIDFSearch runs grep for each keyword and weights matches by IDF
// (inverse document frequency). Keywords matching fewer files are more
// informative and contribute more to a file's score.
func grepIDFSearch(keywords []string, repoRoot string) (scores map[string]float64, hits map[string]map[string]string) {
	scores = make(map[string]float64)
	hits = make(map[string]map[string]string)

	// Count total source files for IDF denominator.
	totalFiles := countSourceFiles(repoRoot)
	if totalFiles < 1 {
		totalFiles = 100 // fallback
	}

	for _, kw := range keywords {
		// Case-sensitive grep first.
		paths := grepFiles(kw, repoRoot, false)
		matchType := "exact"
		if len(paths) == 0 {
			// Fall back to case-insensitive.
			paths = grepFiles(kw, repoRoot, true)
			matchType = "icase"
		}

		if len(paths) == 0 {
			continue
		}

		// IDF: keywords matching fewer files score higher.
		df := float64(len(paths))
		idf := math.Log2(float64(totalFiles)/df) + 1.0

		// Filename matches get a bonus on top of IDF.
		kwLower := strings.ToLower(kw)

		for _, p := range paths {
			p = normalizeSearchPath(p, repoRoot)
			if isNoisePath(p) {
				continue
			}

			fileScore := idf * fileTypeWeight(p)

			// Filename match bonus: file that contains the keyword in
			// its name is more likely to be the defining file.
			baseLower := strings.ToLower(filepath.Base(p))
			if strings.Contains(baseLower, kwLower) {
				fileScore *= 2.0
				matchType = "filename+" + matchType
			}

			scores[p] += fileScore
			if hits[p] == nil {
				hits[p] = make(map[string]string)
			}
			hits[p][kw] = matchType
		}
	}

	return scores, hits
}

// countSourceFiles returns an approximate count of source files in the repo.
func countSourceFiles(repoRoot string) int {
	args := []string{repoRoot, "-type", "f"}
	for _, dir := range searchExcludeDirs {
		args = append([]string{repoRoot, "-path", "*/" + dir, "-prune", "-o"}, args[1:]...)
	}
	// Simplified: just count all files
	cmd := exec.Command("find", repoRoot, "-type", "f",
		"-not", "-path", "*/.git/*",
		"-not", "-path", "*/node_modules/*",
		"-not", "-path", "*/vendor/*")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return 100
	}
	return len(splitLines(stdout.String()))
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

// fileTypeWeight returns a multiplier based on the file's likely role.
// Source code files get full weight; documentation, config, and other
// non-source files are down-weighted because they are secondary evidence
// when investigating code behavior.
func fileTypeWeight(path string) float64 {
	ext := strings.ToLower(filepath.Ext(path))

	// Source code — full weight.
	switch ext {
	case ".go", ".py", ".js", ".ts", ".tsx", ".jsx",
		".java", ".kt", ".rs", ".rb", ".c", ".cpp", ".h",
		".cs", ".swift", ".scala", ".ex", ".exs", ".erl",
		".php", ".lua", ".zig", ".nim", ".sh", ".bash":
		return 1.0
	}

	// Config/build files — slightly reduced.
	switch ext {
	case ".yaml", ".yml", ".toml", ".json", ".xml",
		".ini", ".cfg", ".conf", ".env", ".properties":
		return 0.7
	}

	// Documentation/prose — significantly reduced.
	switch ext {
	case ".md", ".txt", ".rst", ".adoc", ".org":
		return 0.3
	}

	// Unknown extension — moderate default.
	return 0.5
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
