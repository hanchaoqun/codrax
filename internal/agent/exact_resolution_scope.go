package agent

import (
	"path/filepath"
	"sort"
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

func collectExactResolutionSymbolCandidatesFromGraph(graph *repomap.Graph, contract *types.ExactResolutionContract, analyzerKeywords []string, fileSymbolItems map[string][]repomap.Symbol, evidence []types.EvidenceItem) []exactResolutionSymbolCandidate {
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

	candidateFiles := exactResolutionCandidateFiles(graph, fileSymbolItems)
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
		for _, sym := range exactResolutionSymbolsForFile(file, graph, fileSymbolItems) {
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
		// 2026-05+ scope axis: this function is for line-anchored
		// "exact target's file" extraction (per-line role validation
		// downstream). Schema-level scopes (File / Crossfile /
		// Negative) describe layer identity / cross-file contracts /
		// absences — they don't anchor a per-line target. Schema-
		// level evidence enters the answer surface via the separate
		// validateConfigTraceAbsenceCitationFocus schema-level branch.
		if !item.Scope.IsLineShaped() {
			continue
		}
		if !exactResolutionSourceSupportsContextScope(contract, item) {
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
			!exactResolutionSourceSupportsContextScope(contract, item) {
			continue
		}
		// Same scope discipline as exactResolutionAnchoredFiles —
		// per-line role weighting only.
		if !item.Scope.IsLineShaped() {
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

func exactResolutionCandidateFiles(graph *repomap.Graph, fileSymbolItems map[string][]repomap.Symbol) []string {
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
	for path := range fileSymbolItems {
		if path != "" && !seen[path] {
			seen[path] = true
			files = append(files, path)
		}
	}
	sort.Strings(files)
	return files
}

func exactResolutionSymbolsForFile(path string, graph *repomap.Graph, fileSymbolItems map[string][]repomap.Symbol) []exactResolutionSymbolCandidate {
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
	for _, sym := range fileSymbolItems[path] {
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
	return out
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
		if file == "" || (types.LooksLikeAuxiliaryEvidencePath(file) && !exactResolutionSourceSupportsContextScope(contract, item)) {
			continue
		}
		if item.ContextRole == types.EvidenceContextRoleIllustrativeOnly {
			continue
		}
		coverageRole := scopeShapingDiagramRole(contract, item, preferredFiles)
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
		if item.RequestedDiagramRole != types.EvidenceDiagramRoleUnknown &&
			types.CanonicalEvidenceDiagramRole(string(item.RequestedDiagramRole)) == coverageRole {
			score += 5
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

func scopeShapingDiagramRole(contract *types.ExactResolutionContract, item types.EvidenceItem, requiredFiles []string) types.EvidenceDiagramRole {
	if role := types.ConfigTraceSurfaceDiagramRoleInFiles(contract, item, requiredFiles); role != types.EvidenceDiagramRoleUnknown {
		return role
	}
	requested := types.CanonicalEvidenceDiagramRole(string(item.RequestedDiagramRole))
	if requested == types.EvidenceDiagramRoleUnknown || !requested.IsValid() {
		return types.EvidenceDiagramRoleUnknown
	}
	switch item.GroundingStatus {
	case types.GroundingGrounded, types.GroundingRecovered, "":
		// Grounded candidates should already have been accepted above via
		// the shared surface validator. Keep the broader scope-only lane
		// for unresolved structural leads only.
		return types.EvidenceDiagramRoleUnknown
	}
	if types.LooksLikeAuxiliaryEvidencePath(item.Source) ||
		!types.ConfigTraceDiagramRoleAnchorCompatible(requested, item) {
		return types.EvidenceDiagramRoleUnknown
	}
	switch requested {
	case types.EvidenceDiagramRoleConfig:
		if types.LooksLikeConfigFilePath(item.Source) {
			return requested
		}
		return types.EvidenceDiagramRoleUnknown
	case types.EvidenceDiagramRoleDefault:
		if strings.TrimSpace(item.Source) == "" || types.LooksLikeConfigFilePath(item.Source) {
			return types.EvidenceDiagramRoleUnknown
		}
		if contract != nil {
			surface := strings.Join([]string{
				item.Source,
				item.Subject,
				item.Object,
				item.AnchorSymbol,
				item.Condition,
				item.Snippet,
			}, "\n")
			if types.ExactResolutionSameFamilyMatchScore(contract, surface) > 0 {
				return requested
			}
		}
	case types.EvidenceDiagramRoleRuntime, types.EvidenceDiagramRoleOverride:
		if strings.TrimSpace(item.Source) == "" || types.LooksLikeConfigFilePath(item.Source) {
			return types.EvidenceDiagramRoleUnknown
		}
		if contract != nil {
			surface := strings.Join([]string{
				item.Source,
				item.Subject,
				item.Object,
				item.AnchorSymbol,
				item.Condition,
				item.Snippet,
			}, "\n")
			if types.ExactResolutionSameFamilyMatchScore(contract, surface) > 0 ||
				types.ExactResolutionSpecificStructureMatchScore(contract, surface) > 0 {
				return requested
			}
		}
	}
	return types.EvidenceDiagramRoleUnknown
}

func refreshedExactResolutionContextFiles(
	contract *types.ExactResolutionContract,
	scenario types.Scenario,
	graph *repomap.Graph,
	evidence []types.EvidenceItem,
	candidates []exactResolutionSymbolCandidate,
	preferredFiles []string,
	previousFiles []string,
) []string {
	groundedFiles := exactResolutionContextFilesFromGroundedEvidence(contract, scenario, evidence, preferredFiles)
	coverageFiles := exactResolutionContextFilesFromScenarioCoverageEvidence(contract, scenario, evidence, preferredFiles)
	requestedRoleFiles := exactResolutionContextFilesFromPendingRequestedRoles(contract, scenario, evidence, preferredFiles)
	graphCoverageFiles := exactResolutionContextFilesFromGraphCoverageHops(contract, scenario, graph, evidence, preferredFiles)
	if len(coverageFiles) > 0 {
		combined := mergeContextScopeFiles(previousFiles, groundedFiles, coverageFiles, requestedRoleFiles, graphCoverageFiles)
		if len(combined) > 0 &&
			!types.ExactResolutionAbsenceClosureReady(contract, scenario, contract.Targets, evidence, combined) {
			return combined
		}
	}
	if len(requestedRoleFiles) > 0 {
		combined := mergeContextScopeFiles(previousFiles, groundedFiles, requestedRoleFiles, graphCoverageFiles)
		if len(combined) > 0 &&
			!types.ExactResolutionAbsenceClosureReady(contract, scenario, contract.Targets, evidence, combined) {
			return combined
		}
	}
	if len(graphCoverageFiles) > 0 {
		combined := mergeContextScopeFiles(previousFiles, groundedFiles, graphCoverageFiles)
		if len(combined) > 0 &&
			!types.ExactResolutionAbsenceClosureReady(contract, scenario, contract.Targets, evidence, combined) {
			return combined
		}
	}
	if len(groundedFiles) > 0 {
		if len(previousFiles) > 0 &&
			scenario == types.ScenarioConfigTrace &&
			contract != nil &&
			contract.TargetKind == types.SubjectConfigKey &&
			!types.ExactResolutionAbsenceClosureReady(contract, scenario, contract.Targets, evidence, groundedFiles) {
			return mergeContextScopeFiles(previousFiles, groundedFiles)
		}
		return groundedFiles
	}
	pending := pendingExactResolutionContextCandidates(contract, evidence, candidates)
	return mergeContextScopeFiles(previousFiles, exactResolutionContextFilesFromCandidates(pending, preferredFiles), graphCoverageFiles)
}

func exactResolutionContextFilesFromPendingRequestedRoles(
	contract *types.ExactResolutionContract,
	scenario types.Scenario,
	evidence []types.EvidenceItem,
	preferredFiles []string,
) []string {
	if contract == nil || len(evidence) == 0 || scenario != types.ScenarioConfigTrace || contract.TargetKind != types.SubjectConfigKey {
		return nil
	}
	missing := types.ConfigTraceMissingRequestedDiagramRoles(contract, preferredFiles, evidence)
	if len(missing) == 0 {
		return nil
	}
	missingSet := make(map[types.EvidenceDiagramRole]bool, len(missing))
	for _, role := range missing {
		missingSet[role] = true
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
		role := types.CanonicalEvidenceDiagramRole(string(item.RequestedDiagramRole))
		if role == types.EvidenceDiagramRoleUnknown || !missingSet[role] {
			continue
		}
		if scopeShapingDiagramRole(contract, item, preferredFiles) != role {
			continue
		}
		file := canonicalExactResolutionPath(item.Source)
		if file == "" || types.LooksLikeAuxiliaryEvidencePath(file) {
			continue
		}
		score := 18
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

func exactResolutionContextFilesFromGraphCoverageHops(
	contract *types.ExactResolutionContract,
	scenario types.Scenario,
	graph *repomap.Graph,
	evidence []types.EvidenceItem,
	preferredFiles []string,
) []string {
	if contract == nil || graph == nil || len(evidence) == 0 {
		return nil
	}
	if scenario != types.ScenarioConfigTrace || contract.TargetKind != types.SubjectConfigKey {
		return nil
	}
	if len(types.ConfigTraceMissingRequestedDiagramRoles(contract, preferredFiles, evidence)) == 0 {
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
	add := func(file string, score int) {
		file = canonicalExactResolutionPath(file)
		if file == "" || types.LooksLikeAuxiliaryEvidencePath(file) || score <= 0 {
			return
		}
		if bonus := preferredRank[file]; bonus > 0 {
			score += 8 + bonus
		}
		if q := graph.QueryScores[file]; q > 0 {
			score += int(q * 10)
		}
		if cur := bestByFile[file]; cur < score {
			bestByFile[file] = score
		}
	}
	for _, item := range evidence {
		role := scopeShapingDiagramRole(contract, item, preferredFiles)
		source := canonicalExactResolutionPath(item.Source)
		if source == "" || types.LooksLikeAuxiliaryEvidencePath(source) {
			continue
		}
		if role == types.EvidenceDiagramRoleUnknown && item.ContextRole != types.EvidenceContextRoleAbsenceSupport {
			continue
		}
		for _, importer := range graph.FilesImporting(source) {
			score := 16
			switch role {
			case types.EvidenceDiagramRoleRuntime, types.EvidenceDiagramRoleOverride:
				score = 28
			case types.EvidenceDiagramRoleConfig:
				score = 24
			case types.EvidenceDiagramRoleDefault:
				score = 18
			default:
				if item.ContextRole == types.EvidenceContextRoleAbsenceSupport {
					score = 14
				}
			}
			add(importer, score)
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
	if contract != nil &&
		contract.TargetKind == types.SubjectConfigKey &&
		types.LooksLikePromptSupportPath(source) &&
		!types.LooksLikeConfigFilePath(source) {
		return false
	}
	return types.ExactResolutionSourceIsDefiningPrimaryProofLike(contract, source)
}

func exactResolutionSourceSupportsContextScope(contract *types.ExactResolutionContract, item types.EvidenceItem) bool {
	if exactResolutionSourceIsProductionLike(contract, item.Source) {
		return true
	}
	role := item.DiagramRole
	if role == types.EvidenceDiagramRoleUnknown {
		role = item.RequestedDiagramRole
	}
	return (contract == nil || contract.TargetKind == types.SubjectConfigKey) &&
		role == types.EvidenceDiagramRoleConfig &&
		types.LooksLikeConfigFilePath(item.Source)
}
