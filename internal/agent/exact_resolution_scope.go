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
	keywordSet := make(map[string]bool)
	for _, kw := range analyzerKeywords {
		for _, token := range strings.FieldsFunc(strings.ToLower(kw), func(r rune) bool {
			return (r < 'a' || r > 'z') && (r < '0' || r > '9')
		}) {
			if len(token) >= 3 {
				keywordSet[token] = true
			}
		}
	}
	if len(termSet) == 0 && len(keywordSet) == 0 && contract.RelatedContextPolicy != types.ExactContextSameFamilyGrounded {
		return nil
	}
	terms := make([]string, 0, len(termSet))
	for term := range termSet {
		terms = append(terms, term)
	}
	sort.Strings(terms)
	keywords := make([]string, 0, len(keywordSet))
	for term := range keywordSet {
		keywords = append(keywords, term)
	}
	sort.Strings(keywords)

	candidateFiles := exactResolutionCandidateFiles(graph, fileSymbols)
	if len(candidateFiles) == 0 {
		return nil
	}
	anchoredFiles := exactResolutionAnchoredFiles(contract, evidence)
	roleAnchoredFiles := exactResolutionRoleAnchoredFiles(contract, evidence)
	strictRoleAnchors := contract.TargetKind == types.SubjectConfigKey &&
		contract.RelatedContextPolicy == types.ExactContextSameFamilyGrounded &&
		len(roleAnchoredFiles) > 0

	var cands []exactResolutionSymbolCandidate
	seen := make(map[string]bool)
	for _, file := range candidateFiles {
		fileLower := strings.ToLower(file)
		if isExactResolutionNoiseFile(fileLower) {
			continue
		}
		canonFile := canonicalExactResolutionPath(file)
		if strictRoleAnchors && roleAnchoredFiles[canonFile] == 0 {
			continue
		}
		for _, sym := range exactResolutionSymbolsForFile(file, graph, fileSymbols) {
			symLower := strings.ToLower(sym.Symbol)
			score := 0
			keywordHit := false
			combinedSurface := sym.Symbol + " " + file
			familyScore := types.ExactResolutionSameFamilyMatchScore(contract, combinedSurface)
			if contract.RelatedContextPolicy == types.ExactContextSameFamilyGrounded {
				if familyScore == 0 {
					continue
				}
				score += familyScore
			} else {
				for _, term := range terms {
					if strings.Contains(symLower, term) {
						score += 4
					}
					if strings.Contains(fileLower, term) {
						score += 2
					}
				}
			}
			if !strictRoleAnchors {
				for _, term := range keywords {
					if strings.Contains(symLower, term) {
						score += 2
						keywordHit = true
					}
					if strings.Contains(fileLower, term) {
						score += 1
						keywordHit = true
					}
				}
			}
			if contract.TargetKind == types.SubjectConfigKey &&
				contract.RelatedContextPolicy == types.ExactContextSameFamilyGrounded &&
				!strictRoleAnchors &&
				len(keywords) > 0 &&
				!keywordHit &&
				!anchoredFiles[canonFile] {
				continue
			}
			if anchoredFiles[canonFile] {
				score += 4
			}
			if bonus := roleAnchoredFiles[canonFile]; bonus > 0 {
				score += bonus
			}
			minScore := 6
			if contract.RelatedContextPolicy == types.ExactContextSameFamilyGrounded {
				minScore = 5
			}
			if score < minScore {
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

func exactResolutionRoleAnchoredFiles(contract *types.ExactResolutionContract, evidence []types.EvidenceItem) map[string]int {
	if contract == nil || len(evidence) == 0 {
		return nil
	}
	out := make(map[string]int)
	for _, item := range evidence {
		switch item.GroundingStatus {
		case types.GroundingGrounded, types.GroundingRecovered, "":
		default:
			continue
		}
		if item.ContextRole == types.EvidenceContextRoleIllustrativeOnly ||
			item.DiagramRole == types.EvidenceDiagramRoleUnknown ||
			!exactResolutionSourceIsProductionLike(contract, item.Source) {
			continue
		}
		path := canonicalExactResolutionPath(item.Source)
		if path == "" {
			continue
		}
		bonus := 6
		if item.ContextRole == types.EvidenceContextRoleDefining {
			bonus += 2
		}
		out[path] += bonus
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

func exactResolutionContextFilesFromCandidates(candidates []exactResolutionSymbolCandidate, preferredFiles []string) []string {
	if len(candidates) == 0 {
		return nil
	}
	candidates = exactResolutionFilterCandidatesToPreferredFiles(candidates, preferredFiles)
	if len(candidates) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	var files []string
	for _, cand := range candidates {
		file := canonicalExactResolutionPath(cand.File)
		if file == "" || seen[file] {
			continue
		}
		seen[file] = true
		files = append(files, file)
	}
	return files
}

func exactResolutionContextFilesFromGroundedEvidence(
	contract *types.ExactResolutionContract,
	scenario types.Scenario,
	evidence []types.EvidenceItem,
	preferredFiles []string,
) []string {
	if contract == nil || len(evidence) == 0 {
		return nil
	}
	preferredRank := make(map[string]int, len(preferredFiles))
	for i, file := range preferredFiles {
		file = canonicalExactResolutionPath(file)
		if file != "" {
			preferredRank[file] = len(preferredFiles) - i
		}
	}
	type scoredFile struct {
		File  string
		Score int
	}
	bestByFile := make(map[string]int)
	for _, item := range evidence {
		if !types.ExactResolutionRelatedContextProofAllowedInFiles(contract, scenario, true, item, nil) {
			continue
		}
		file := canonicalExactResolutionPath(item.Source)
		if file == "" {
			continue
		}
		score := 12
		if item.ContextRole == types.EvidenceContextRoleDefining {
			score += 4
		}
		if item.DiagramRole != types.EvidenceDiagramRoleUnknown {
			score += 4
		}
		if item.LineStart > 0 {
			score += 1
		}
		if bonus := preferredRank[file]; bonus > 0 {
			score += 8 + bonus
		}
		if cur := bestByFile[file]; cur < score {
			bestByFile[file] = score
		}
	}
	if len(bestByFile) == 0 {
		return nil
	}
	scored := make([]scoredFile, 0, len(bestByFile))
	for file, score := range bestByFile {
		scored = append(scored, scoredFile{File: file, Score: score})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		return scored[i].File < scored[j].File
	})
	files := make([]string, 0, len(scored))
	for _, item := range scored {
		files = append(files, item.File)
	}
	return files
}

func exactResolutionContextFilesFromScenarioCoverageEvidence(
	contract *types.ExactResolutionContract,
	scenario types.Scenario,
	evidence []types.EvidenceItem,
	preferredFiles []string,
) []string {
	if contract == nil || len(evidence) == 0 {
		return nil
	}
	if scenario != types.ScenarioConfigTrace || contract.TargetKind != types.SubjectConfigKey {
		return nil
	}
	preferredRank := make(map[string]int, len(preferredFiles))
	for i, file := range preferredFiles {
		file = canonicalExactResolutionPath(file)
		if file != "" {
			preferredRank[file] = len(preferredFiles) - i
		}
	}
	type scoredFile struct {
		File  string
		Score int
	}
	bestByFile := make(map[string]int)
	for _, item := range evidence {
		file := canonicalExactResolutionPath(item.Source)
		if file == "" || types.LooksLikeAuxiliaryEvidencePath(file) {
			continue
		}
		if item.ContextRole == types.EvidenceContextRoleIllustrativeOnly {
			continue
		}
		coverageRole := scopeShapingDiagramRole(item)
		if coverageRole == types.EvidenceDiagramRoleUnknown &&
			item.ContextRole != types.EvidenceContextRoleAbsenceSupport {
			continue
		}
		score := 0
		if item.DiagramRole != types.EvidenceDiagramRoleUnknown {
			score += 12
		} else if coverageRole != types.EvidenceDiagramRoleUnknown {
			score += 7
		}
		if item.ContextRole == types.EvidenceContextRoleAbsenceSupport {
			score += 4
		}
		if types.LooksLikeConfigFilePath(item.Source) {
			score += 6
		}
		switch item.GroundingStatus {
		case types.GroundingGrounded:
			score += 3
		case types.GroundingRecovered:
			score += 1
		}
		if item.LineStart > 0 {
			score += 1
		}
		if bonus := preferredRank[file]; bonus > 0 {
			score += 8 + bonus
		}
		if score < 8 {
			continue
		}
		if cur := bestByFile[file]; cur < score {
			bestByFile[file] = score
		}
	}
	if len(bestByFile) == 0 {
		return nil
	}
	scored := make([]scoredFile, 0, len(bestByFile))
	for file, score := range bestByFile {
		scored = append(scored, scoredFile{File: file, Score: score})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		return scored[i].File < scored[j].File
	})
	files := make([]string, 0, len(scored))
	for _, item := range scored {
		files = append(files, item.File)
	}
	return files
}

func scopeShapingDiagramRole(item types.EvidenceItem) types.EvidenceDiagramRole {
	if item.DiagramRole != types.EvidenceDiagramRoleUnknown {
		return item.DiagramRole
	}
	role := item.RequestedDiagramRole
	if role == types.EvidenceDiagramRoleUnknown {
		return types.EvidenceDiagramRoleUnknown
	}
	if types.LooksLikeAuxiliaryEvidencePath(item.Source) {
		return types.EvidenceDiagramRoleUnknown
	}
	if !types.ConfigTraceDiagramRoleAnchorCompatible(role, item) {
		return types.EvidenceDiagramRoleUnknown
	}
	switch role {
	case types.EvidenceDiagramRoleConfig:
		if types.LooksLikeConfigFilePath(item.Source) {
			return role
		}
	case types.EvidenceDiagramRoleDefault, types.EvidenceDiagramRoleRuntime, types.EvidenceDiagramRoleOverride:
		if item.Source != "" && !types.LooksLikeConfigFilePath(item.Source) {
			return role
		}
	}
	return types.EvidenceDiagramRoleUnknown
}

func refreshedExactResolutionContextFiles(
	contract *types.ExactResolutionContract,
	scenario types.Scenario,
	evidence []types.EvidenceItem,
	candidates []exactResolutionSymbolCandidate,
	preferredFiles []string,
) []string {
	groundedFiles := exactResolutionContextFilesFromGroundedEvidence(contract, scenario, evidence, preferredFiles)
	coverageFiles := exactResolutionContextFilesFromScenarioCoverageEvidence(contract, scenario, evidence, preferredFiles)
	if len(coverageFiles) > 0 {
		combined := mergeContextScopeFiles(groundedFiles, coverageFiles)
		if len(combined) > 0 &&
			!types.ExactResolutionAbsenceClosureReady(contract, scenario, contract.Targets, evidence, combined) {
			return combined
		}
	}
	if len(groundedFiles) > 0 {
		return groundedFiles
	}
	pending := pendingExactResolutionContextCandidates(contract, evidence, candidates)
	return exactResolutionContextFilesFromCandidates(pending, preferredFiles)
}

func mergeContextScopeFiles(groups ...[]string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, files := range groups {
		for _, file := range files {
			file = canonicalExactResolutionPath(file)
			if file == "" || seen[file] {
				continue
			}
			seen[file] = true
			out = append(out, file)
		}
	}
	return out
}

func exactResolutionFilterCandidatesToPreferredFiles(candidates []exactResolutionSymbolCandidate, preferredFiles []string) []exactResolutionSymbolCandidate {
	if len(candidates) == 0 {
		return nil
	}
	preferred := make(map[string]bool)
	for _, file := range preferredFiles {
		file = canonicalExactResolutionPath(file)
		if file != "" {
			preferred[file] = true
		}
	}
	filterToPreferred := false
	if len(preferred) > 0 {
		for _, cand := range candidates {
			if preferred[canonicalExactResolutionPath(cand.File)] {
				filterToPreferred = true
				break
			}
		}
	}
	if !filterToPreferred {
		return candidates
	}
	filtered := make([]exactResolutionSymbolCandidate, 0, len(candidates))
	for _, cand := range candidates {
		if !preferred[canonicalExactResolutionPath(cand.File)] {
			continue
		}
		filtered = append(filtered, cand)
	}
	return filtered
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
		if item.GroundingStatus != types.GroundingGrounded {
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
