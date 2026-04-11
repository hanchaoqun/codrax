package agent

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"math"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
)

// searchTimeout is the maximum wall-clock time for any single search
// command (rg, grep, find). Prevents hangs on large repos or
// pathological regex patterns.
const searchTimeout = 60 * time.Second

// keywordFileScore records how a file scored across multi-level keyword matching.
type keywordFileScore struct {
	Path         string
	Score        float64
	RepoMapScore float64           // raw repo_map structural score (for coverage selection)
	Hits         map[string]string // keyword → best match level for debugging
	Symbols      []string          // symbol summaries from repo_map (e.g. "RegisterDefaultSubAgents function:63")
}

// keywordSearchResult wraps the scored files plus the repo_map graph
// for downstream use (symbol lookups, cross-reference tracking).
type keywordSearchResult struct {
	Files []keywordFileScore
	Graph *repomap.Graph // may be nil if repo_map is unavailable
}

// searchExcludeDirs is the shared exclude list from tool.ExcludeDirs.
var searchExcludeDirs = tool.ExcludeDirs

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
func keywordSearch(keywords []string, repoRoot string) *keywordSearchResult {
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
	return &keywordSearchResult{Files: results, Graph: graph}
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

// grepIDFSearch runs grep/rg for each keyword and weights matches by IDF
// (inverse document frequency). Keywords matching fewer files are more
// informative and contribute more to a file's score.
//
// When ripgrep is available, all keywords are searched in a single rg
// call using multiple -e patterns, reducing process spawns from N*2 to 1.
func grepIDFSearch(keywords []string, repoRoot string) (scores map[string]float64, hits map[string]map[string]string) {
	scores = make(map[string]float64)
	hits = make(map[string]map[string]string)

	// Count total source files for IDF denominator.
	totalFiles := countSourceFiles(repoRoot)
	if totalFiles < 1 {
		totalFiles = 100 // fallback
	}

	// Build per-keyword file lists. When rg is available, batch all
	// keywords into a single call.
	type kwResult struct {
		paths     []string
		matchType string
	}
	kwResults := make(map[string]kwResult, len(keywords))

	if tool.UseRipgrep() {
		// Batch: one rg call with multiple -e patterns using smart-case.
		// Smart-case handles the exact-first/icase-fallback automatically:
		// patterns with uppercase → case-sensitive; all-lowercase → insensitive.
		filesByKw := rgBatchFiles(keywords, repoRoot)
		for _, kw := range keywords {
			paths := filesByKw[kw]
			if len(paths) > 0 {
				// Determine match type: if keyword has uppercase it was
				// exact; otherwise smart-case made it case-insensitive.
				mt := "exact"
				if !strings.ContainsAny(kw, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
					mt = "icase"
				}
				kwResults[kw] = kwResult{paths: paths, matchType: mt}
			}
		}
	} else {
		// Parallel: grep calls run concurrently to reduce wall-clock
		// time from 2N sequential spawns to ~2 (limited by I/O, not CPU).
		var mu sync.Mutex
		var wg sync.WaitGroup
		for _, kw := range keywords {
			wg.Add(1)
			go func(kw string) {
				defer wg.Done()
				paths := grepFiles(kw, repoRoot, false)
				matchType := "exact"
				if len(paths) == 0 {
					paths = grepFiles(kw, repoRoot, true)
					matchType = "icase"
				}
				if len(paths) > 0 {
					mu.Lock()
					kwResults[kw] = kwResult{paths: paths, matchType: matchType}
					mu.Unlock()
				}
			}(kw)
		}
		wg.Wait()
	}

	// Score files by IDF.
	for _, kw := range keywords {
		kr, ok := kwResults[kw]
		if !ok {
			continue
		}

		df := float64(len(kr.paths))
		idf := math.Log2(float64(totalFiles)/df) + 1.0
		kwLower := strings.ToLower(kw)

		for _, p := range kr.paths {
			p = normalizeSearchPath(p, repoRoot)
			if isNoisePath(p) {
				continue
			}

			fileScore := idf * fileTypeWeight(p)
			matchType := kr.matchType

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

// sourceFileCount caches the result of countSourceFiles so the IDF
// denominator is computed at most once per process. The count is
// approximate and does not need to track real-time mutations.
var (
	sourceCountOnce  sync.Once
	sourceCountCache int
)

// countSourceFiles returns an approximate count of source files in the repo.
// Uses rg --files when available (fast, respects .gitignore), falls back to
// Go-native filepath.WalkDir with the unified ExcludeDirs list.
// Result is cached for the process lifetime.
func countSourceFiles(repoRoot string) int {
	sourceCountOnce.Do(func() {
		sourceCountCache = countSourceFilesOnce(repoRoot)
	})
	if sourceCountCache < 1 {
		return 100 // fallback
	}
	return sourceCountCache
}

func countSourceFilesOnce(repoRoot string) int {
	// Prefer rg --files: fast, auto-excludes .gitignore entries.
	if tool.UseRipgrep() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		args := []string{"--files"}
		for _, dir := range searchExcludeDirs {
			args = append(args, "--glob", "!"+dir+"/")
		}
		args = append(args, repoRoot)
		cmd := exec.CommandContext(ctx, "rg", args...)
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		if err := cmd.Run(); err == nil {
			return len(splitLines(stdout.String()))
		}
	}
	// Go-native fallback using the unified exclude list.
	excludeSet := make(map[string]bool, len(searchExcludeDirs))
	for _, d := range searchExcludeDirs {
		excludeSet[d] = true
	}
	count := 0
	filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && excludeSet[d.Name()] {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			count++
		}
		return nil
	})
	return count
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

// grepFiles runs grep/rg on the repo and returns matching file paths.
// All commands are guarded by searchTimeout to prevent hangs.
func grepFiles(pattern, repoRoot string, ignoreCase bool) []string {
	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()

	var cmd *exec.Cmd

	if tool.UseRipgrep() {
		args := []string{"-l"}
		if ignoreCase {
			args = append(args, "-i")
		} else {
			args = append(args, "--case-sensitive")
		}
		for _, dir := range searchExcludeDirs {
			args = append(args, "--glob", "!"+dir+"/")
		}
		args = append(args, pattern, repoRoot)
		cmd = exec.CommandContext(ctx, "rg", args...)
	} else {
		args := []string{"-rlEI"}
		if ignoreCase {
			args = []string{"-rlEIi"}
		}
		for _, dir := range searchExcludeDirs {
			args = append(args, "--exclude-dir="+dir)
		}
		args = append(args, pattern, repoRoot)
		cmd = exec.CommandContext(ctx, "grep", args...)
	}

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil // exit 1 = no matches, timeout, or error — either way no results
	}
	return splitLines(stdout.String())
}

// rgBatchFiles runs a single rg --json call with multiple -e patterns
// and returns per-keyword file lists by parsing which pattern matched
// in each file. This reduces N keyword searches to 1 process spawn
// and avoids reading file contents entirely — rg does all the matching.
//
// Large-repo safe: no file size limits, no truncation, no memory-mapped
// file reads. rg handles binary detection and .gitignore automatically.
func rgBatchFiles(keywords []string, repoRoot string) map[string][]string {
	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()

	args := []string{"--json", "--smart-case"}
	for _, dir := range searchExcludeDirs {
		args = append(args, "--glob", "!"+dir+"/")
	}
	for _, kw := range keywords {
		args = append(args, "-e", kw)
	}
	args = append(args, repoRoot)

	cmd := exec.CommandContext(ctx, "rg", args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil
	}

	// Parse rg JSON output to build per-keyword file lists.
	// Each "match" line contains the file path and submatch texts,
	// which we map back to the original keywords.
	//
	// Build lowercase keyword index for smart-case matching:
	// keywords with uppercase are exact; all-lowercase are insensitive.
	kwLower := make(map[string]string, len(keywords)) // lowercase → original
	kwExact := make(map[string]string, len(keywords)) // exact → original
	for _, kw := range keywords {
		if strings.ContainsAny(kw, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
			kwExact[kw] = kw
		} else {
			kwLower[strings.ToLower(kw)] = kw
		}
	}

	// Track which keywords matched which files (deduplicate).
	type fileKw struct {
		file, kw string
	}
	seen := make(map[fileKw]bool)
	result := make(map[string][]string, len(keywords))

	for _, line := range splitLines(stdout.String()) {
		// Fast pre-filter: only parse lines containing "match".
		if !strings.Contains(line, `"type":"match"`) {
			continue
		}
		// Extract path and submatch texts from JSON.
		filePath, matchTexts := parseRgMatchLine(line)
		if filePath == "" {
			continue
		}
		for _, mt := range matchTexts {
			// Map matched text back to original keyword.
			var origKw string
			if kw, ok := kwExact[mt]; ok {
				origKw = kw
			} else if kw, ok := kwLower[strings.ToLower(mt)]; ok {
				origKw = kw
			}
			if origKw == "" {
				continue
			}
			key := fileKw{filePath, origKw}
			if !seen[key] {
				seen[key] = true
				result[origKw] = append(result[origKw], filePath)
			}
		}
	}

	return result
}

// findClosingQuote returns the index of the next unescaped '"' in s.
// This handles JSON string escaping (\" and \\) correctly, unlike a
// plain strings.Index which breaks on filenames or match texts that
// contain escaped quotes.
func findClosingQuote(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' {
			i++ // skip the escaped character
			continue
		}
		if s[i] == '"' {
			return i
		}
	}
	return -1
}

// unescapeJSON handles the two most common JSON string escapes that
// affect file paths and match texts. Full JSON unescaping is not
// needed because rg only escapes these two in practice.
var jsonUnescaper = strings.NewReplacer(`\"`, `"`, `\\`, `\`)

// parseRgMatchLine extracts the file path and submatch texts from a
// single rg --json "match" line. Uses lightweight string scanning
// instead of json.Unmarshal to avoid allocating full structs for
// thousands of match lines in large repos. Handles JSON-escaped
// quotes in file paths and match texts.
func parseRgMatchLine(line string) (filePath string, matchTexts []string) {
	// Extract path: "path":{"text":"FILE"}
	const pathKey = `"path":{"text":"`
	if idx := strings.Index(line, pathKey); idx >= 0 {
		start := idx + len(pathKey)
		if end := findClosingQuote(line[start:]); end >= 0 {
			filePath = jsonUnescaper.Replace(line[start : start+end])
		}
	}
	// Extract submatches: "match":{"text":"MATCHED"}
	const matchKey = `"match":{"text":"`
	rest := line
	for {
		idx := strings.Index(rest, matchKey)
		if idx < 0 {
			break
		}
		start := idx + len(matchKey)
		rest = rest[start:]
		if end := findClosingQuote(rest); end >= 0 {
			matchTexts = append(matchTexts, rest[:end])
			rest = rest[end:]
		}
	}
	return
}

// findFilesByName uses find to locate files whose basename contains the keyword.
// Guarded by searchTimeout to prevent hangs on large directory trees.
func findFilesByName(keyword, repoRoot string, ignoreCase bool) []string {
	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()

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

	cmd := exec.CommandContext(ctx, "find", args...)
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

	// Abbreviation expansion: add common short↔long pairs so searching
	// "auth" also tries "authentication" and vice versa.
	abbrevPairs := [][2]string{
		{"auth", "authentication"},
		{"config", "configuration"},
		{"init", "initialization"},
		{"impl", "implementation"},
		{"msg", "message"},
		{"req", "request"},
		{"resp", "response"},
		{"err", "error"},
		{"ctx", "context"},
		{"conn", "connection"},
		{"cmd", "command"},
		{"mgr", "manager"},
		{"svc", "service"},
		{"repo", "repository"},
		{"env", "environment"},
		{"exec", "execute"},
		{"eval", "evaluate"},
		{"reg", "register"},
		{"sig", "signal"},
		{"param", "parameter"},
	}
	// Work on a snapshot of current expanded list to avoid infinite loop.
	snapshot := make([]string, len(expanded))
	copy(snapshot, expanded)
	for _, kw := range snapshot {
		kwLower := strings.ToLower(kw)
		for _, pair := range abbrevPairs {
			if kwLower == pair[0] {
				add(pair[1])
			} else if kwLower == pair[1] {
				add(pair[0])
			}
		}
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
