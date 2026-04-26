package agent

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

type exactResolutionSymbolCandidate struct {
	File   string
	Symbol string
	Line   int
	Score  int
}

func collectExactResolutionSymbolCandidatesFromGraph(graph *repomap.Graph, contract *types.ExactResolutionContract, analyzerKeywords []string, fileSymbols map[string][]string, evidence []types.EvidenceItem) []exactResolutionSymbolCandidate {
	if contract == nil {
		return nil
	}
	termSet := make(map[string]bool)
	for _, term := range types.ExactResolutionContextTerms(contract) {
		term = strings.TrimSpace(strings.ToLower(term))
		if len(term) >= 3 {
			termSet[term] = true
		}
	}
	for _, kw := range analyzerKeywords {
		for _, token := range strings.FieldsFunc(strings.ToLower(kw), func(r rune) bool {
			return (r < 'a' || r > 'z') && (r < '0' || r > '9')
		}) {
			if len(token) >= 3 {
				termSet[token] = true
			}
		}
	}
	if len(termSet) == 0 {
		return nil
	}
	terms := make([]string, 0, len(termSet))
	for term := range termSet {
		terms = append(terms, term)
	}
	sort.Strings(terms)

	candidateFiles := exactResolutionCandidateFiles(graph, fileSymbols)
	if len(candidateFiles) == 0 {
		return nil
	}
	anchoredFiles := exactResolutionAnchoredFiles(contract, evidence)

	var cands []exactResolutionSymbolCandidate
	seen := make(map[string]bool)
	for _, file := range candidateFiles {
		fileLower := strings.ToLower(file)
		if isExactResolutionNoiseFile(fileLower) {
			continue
		}
		for _, sym := range exactResolutionSymbolsForFile(file, graph, fileSymbols) {
			symLower := strings.ToLower(sym.Symbol)
			score := 0
			for _, term := range terms {
				if strings.Contains(symLower, term) {
					score += 4
				}
				if strings.Contains(fileLower, term) {
					score += 2
				}
			}
			if anchoredFiles[canonicalExactResolutionPath(file)] {
				score += 4
			}
			if score < 6 {
				continue
			}
			sym.Score = score
			key := file + "\x00" + sym.Symbol
			if seen[key] {
				continue
			}
			seen[key] = true
			cands = append(cands, sym)
		}
	}
	if len(cands) == 0 {
		return nil
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].Score != cands[j].Score {
			return cands[i].Score > cands[j].Score
		}
		if cands[i].File != cands[j].File {
			return cands[i].File < cands[j].File
		}
		if cands[i].Line != cands[j].Line {
			return cands[i].Line < cands[j].Line
		}
		return cands[i].Symbol < cands[j].Symbol
	})
	if len(cands) > 4 {
		cands = cands[:4]
	}
	return cands
}

func exactResolutionAnchoredFiles(contract *types.ExactResolutionContract, evidence []types.EvidenceItem) map[string]bool {
	if contract == nil || len(evidence) == 0 {
		return nil
	}
	out := make(map[string]bool)
	for _, item := range evidence {
		switch item.GroundingStatus {
		case types.GroundingGrounded, types.GroundingRecovered, "":
		default:
			continue
		}
		if item.ContextRole == types.EvidenceContextRoleIllustrativeOnly {
			continue
		}
		if !exactResolutionSourceIsProductionLike(contract, item.Source) {
			continue
		}
		path := canonicalExactResolutionPath(item.Source)
		if path != "" {
			out[path] = true
		}
	}
	return out
}

func exactResolutionCandidateFiles(graph *repomap.Graph, fileSymbols map[string][]string) []string {
	seen := make(map[string]bool)
	var files []string
	if graph != nil && len(graph.FileIndex) > 0 {
		for path := range graph.FileIndex {
			if !seen[path] {
				seen[path] = true
				files = append(files, path)
			}
		}
	}
	for path := range fileSymbols {
		if path != "" && !seen[path] {
			seen[path] = true
			files = append(files, path)
		}
	}
	sort.Strings(files)
	return files
}

func exactResolutionSymbolsForFile(path string, graph *repomap.Graph, fileSymbols map[string][]string) []exactResolutionSymbolCandidate {
	var out []exactResolutionSymbolCandidate
	seen := make(map[string]bool)
	if graph != nil && graph.FileIndex != nil {
		if fi := graph.FileIndex[path]; fi != nil {
			for _, sym := range fi.Symbols {
				name := strings.TrimSpace(sym.Name)
				if name == "" || seen[name] {
					continue
				}
				seen[name] = true
				out = append(out, exactResolutionSymbolCandidate{
					File:   path,
					Symbol: name,
					Line:   sym.Line,
				})
			}
		}
	}
	for _, summary := range fileSymbols[path] {
		name, line := parseExactResolutionSymbolSummary(summary)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, exactResolutionSymbolCandidate{
			File:   path,
			Symbol: name,
			Line:   line,
		})
	}
	return out
}

func parseExactResolutionSymbolSummary(summary string) (string, int) {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return "", 0
	}
	name := summary
	if fields := strings.Fields(summary); len(fields) > 0 {
		name = fields[0]
	}
	line := 0
	if idx := strings.LastIndex(summary, ":"); idx >= 0 && idx+1 < len(summary) {
		if parsed, err := strconv.Atoi(strings.TrimSpace(summary[idx+1:])); err == nil && parsed > 0 {
			line = parsed
		}
	}
	return name, line
}

func isExactResolutionNoiseFile(lowerPath string) bool {
	return types.LooksLikeAuxiliaryEvidencePath(lowerPath)
}

func pendingExactResolutionContextCandidates(contract *types.ExactResolutionContract, evidence []types.EvidenceItem, candidates []exactResolutionSymbolCandidate) []exactResolutionSymbolCandidate {
	if contract == nil || len(candidates) == 0 {
		return nil
	}
	var pending []exactResolutionSymbolCandidate
	for _, cand := range candidates {
		if !exactResolutionEvidenceMentionsCandidate(contract, evidence, cand) {
			pending = append(pending, cand)
		}
	}
	return pending
}

func exactResolutionEvidenceMentionsCandidate(contract *types.ExactResolutionContract, evidence []types.EvidenceItem, cand exactResolutionSymbolCandidate) bool {
	if cand.Symbol == "" {
		return false
	}
	normSym := normalizeExactResolutionLooseToken(cand.Symbol)
	normFile := canonicalExactResolutionPath(cand.File)
	if normSym == "" {
		return false
	}
	for _, item := range evidence {
		switch item.GroundingStatus {
		case types.GroundingGrounded, types.GroundingRecovered, "":
		default:
			continue
		}
		if !exactResolutionSourceIsProductionLike(contract, item.Source) {
			continue
		}
		if normFile != "" && canonicalExactResolutionPath(item.Source) != normFile {
			continue
		}
		text := normalizeExactResolutionLooseToken(strings.Join([]string{
			item.Subject,
			item.Predicate,
			item.Object,
			item.AnchorSymbol,
			item.Condition,
			item.Snippet,
			item.Summary,
		}, "\n"))
		if strings.Contains(text, normSym) {
			return true
		}
	}
	return false
}

func normalizeExactResolutionLooseToken(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func canonicalExactResolutionPath(path string) string {
	path = strings.TrimSpace(strings.ReplaceAll(path, `\`, `/`))
	path = strings.TrimPrefix(path, "./")
	if path == "." {
		return ""
	}
	return filepath.ToSlash(path)
}

func exactResolutionSourceIsProductionLike(contract *types.ExactResolutionContract, source string) bool {
	return types.ExactResolutionSourceIsDefiningPrimaryProofLike(contract, source)
}
