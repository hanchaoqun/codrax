package prescan

import (
	"sort"
	"strings"

	repomap "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

type TokenStatus string

const (
	TokenStatusPrimary       TokenStatus = "primary"
	TokenStatusAuxiliaryOnly TokenStatus = "auxiliary_only"
	TokenStatusUnresolved    TokenStatus = "unresolved"
)

type TokenFinding struct {
	Token  string
	Status TokenStatus
	Paths  []string
	Observed bool
}

// ClassifyToken resolves one identifier-shaped prescan token into a
// shared status used by analyzer repo-overview cautions and
// emit_analysis exact-target unverified findings.
func ClassifyToken(graph *repomap.Graph, seenBlob, token string, requireDefiningPrimary bool) TokenFinding {
	finding := TokenFinding{Token: strings.TrimSpace(token), Status: TokenStatusUnresolved}
	if finding.Token == "" {
		return finding
	}
	if graphHasPrimaryExactAnchor(graph, finding.Token) {
		finding.Observed = true
		finding.Status = TokenStatusPrimary
		return finding
	}
	exactPaths := exactTextMatchPathsFromPrescanBlob(seenBlob, finding.Token)
	if len(exactPaths) > 0 {
		finding.Observed = true
		finding.Paths = exactPaths
		if !requireDefiningPrimary || !allAuxiliaryPaths(exactPaths) {
			finding.Status = TokenStatusPrimary
			return finding
		}
		finding.Status = TokenStatusAuxiliaryOnly
		return finding
	}
	if paths := graphExactMatchFiles(graph, finding.Token); len(paths) > 0 {
		finding.Observed = true
		finding.Paths = paths
		if !requireDefiningPrimary || !allAuxiliaryPaths(paths) {
			finding.Status = TokenStatusPrimary
			return finding
		}
		finding.Status = TokenStatusAuxiliaryOnly
		return finding
	}
	if exactPatternObservedInPrescanBlob(seenBlob, finding.Token) {
		finding.Observed = true
	}
	return finding
}

func allAuxiliaryPaths(paths []string) bool {
	if len(paths) == 0 {
		return false
	}
	for _, path := range paths {
		if !types.LooksLikeAuxiliaryEvidencePath(path) {
			return false
		}
	}
	return true
}

func graphHasPrimaryExactAnchor(graph *repomap.Graph, token string) bool {
	if graph == nil {
		return false
	}
	token = strings.TrimSpace(strings.ReplaceAll(token, `\`, `/`))
	if token == "" {
		return false
	}
	if fi, ok := graph.FileIndex[token]; ok && fi != nil {
		return true
	}
	for path := range graph.FileIndex {
		if strings.EqualFold(strings.ReplaceAll(strings.TrimSpace(path), `\`, `/`), token) {
			return true
		}
	}
	norm := types.ExactResolutionLookupKey("symbol", token)
	if norm == "" {
		return false
	}
	for name, defs := range graph.SymbolDefs {
		if types.ExactResolutionLookupKey("symbol", name) != norm {
			continue
		}
		for _, def := range defs {
			if def == nil || types.LooksLikeAuxiliaryEvidencePath(def.File) {
				continue
			}
			return true
		}
	}
	return false
}

func graphExactMatchFiles(graph *repomap.Graph, token string) []string {
	if graph == nil {
		return nil
	}
	token = strings.TrimSpace(strings.ReplaceAll(token, `\`, `/`))
	if token == "" {
		return nil
	}
	seen := make(map[string]bool)
	var out []string
	if fi, ok := graph.FileIndex[token]; ok && fi != nil {
		seen[token] = true
		out = append(out, token)
	}
	for path := range graph.FileIndex {
		canon := strings.TrimSpace(strings.ReplaceAll(path, `\`, `/`))
		if canon == "" || seen[canon] || !strings.EqualFold(canon, token) {
			continue
		}
		seen[canon] = true
		out = append(out, canon)
	}
	norm := types.ExactResolutionLookupKey("symbol", token)
	if norm != "" {
		for name, defs := range graph.SymbolDefs {
			if types.ExactResolutionLookupKey("symbol", name) != norm {
				continue
			}
			for _, def := range defs {
				if def == nil {
					continue
				}
				path := strings.TrimSpace(strings.ReplaceAll(def.File, `\`, `/`))
				if path == "" || seen[path] {
					continue
				}
				seen[path] = true
				out = append(out, path)
			}
		}
	}
	sort.Strings(out)
	return out
}

func exactTextMatchPathsFromPrescanBlob(seenBlob, token string) []string {
	token = strings.ToLower(strings.TrimSpace(token))
	if token == "" || strings.TrimSpace(seenBlob) == "" {
		return nil
	}
	seen := make(map[string]bool)
	var paths []string
	for _, block := range strings.Split(seenBlob, "[grep params:") {
		pattern := grepPatternFromPrescanBlock(block)
		if pattern == "" || !grepPatternHasExactToken(pattern, token) {
			continue
		}
		for _, path := range grepMatchedPathsFromPrescanBlock(block) {
			if path == "" || seen[path] {
				continue
			}
			seen[path] = true
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths
}

func exactPatternObservedInPrescanBlob(seenBlob, token string) bool {
	token = strings.ToLower(strings.TrimSpace(token))
	if token == "" || strings.TrimSpace(seenBlob) == "" {
		return false
	}
	for _, block := range strings.Split(seenBlob, "[grep params:") {
		pattern := grepPatternFromPrescanBlock(block)
		if pattern == "" || !grepPatternHasExactToken(pattern, token) {
			continue
		}
		return true
	}
	return false
}

func grepMatchedPathsFromPrescanBlock(block string) []string {
	var paths []string
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if line == "" ||
			strings.HasPrefix(line, "no matches found") ||
			strings.HasPrefix(line, "pattern=") ||
			strings.HasPrefix(line, "path=") ||
			strings.HasPrefix(line, "files_only=") ||
			strings.HasPrefix(line, "case_insensitive=") ||
			strings.HasPrefix(line, "summary=") ||
			strings.HasPrefix(line, "success=") ||
			strings.HasPrefix(line, "[") {
			continue
		}
		path := line
		if idx := strings.Index(line, ":"); idx > 0 {
			path = line[:idx]
		}
		path = strings.Trim(strings.ReplaceAll(path, `\`, `/`), "`\"' -")
		if path == "" || (!strings.Contains(path, "/") && !strings.Contains(path, ".")) {
			continue
		}
		paths = append(paths, path)
	}
	return paths
}

func grepPatternFromPrescanBlock(block string) string {
	idx := strings.Index(block, "pattern=")
	if idx < 0 {
		return ""
	}
	rest := block[idx+len("pattern="):]
	end := strings.IndexAny(rest, " \n]")
	if end >= 0 {
		rest = rest[:end]
	}
	return strings.ToLower(strings.TrimSpace(rest))
}

func grepPatternHasExactToken(pattern, token string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	token = strings.ToLower(strings.TrimSpace(token))
	if pattern == "" || token == "" {
		return false
	}
	for _, alt := range strings.Split(pattern, "|") {
		alt = strings.Trim(alt, "`\"'()^$")
		if alt == token {
			return true
		}
	}
	return false
}
