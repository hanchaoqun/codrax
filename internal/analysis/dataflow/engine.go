package dataflow

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
	"gopkg.in/yaml.v3"
)

const lowererVersion = "v1"

// Analyze builds bounded, cross-language dataflow evidence from the
// existing repo_map graph plus direct file/config scans.
func Analyze(graph *repomap.Graph, opts Options) Result {
	opts = defaultOptions(opts)
	if graph == nil {
		return Result{}
	}

	candidateFiles, truncated := selectCandidateFiles(graph, opts)
	if len(candidateFiles) == 0 {
		return Result{}
	}

	lowerers := newLowererRegistry()
	lowered := make([]LoweredFile, 0, len(candidateFiles))
	var evidence []types.EvidenceItem
	biasTokens := prepareBiasTokens(opts.EntityBias)
	for _, filePath := range candidateFiles {
		fi := graph.FileIndex[filePath]
		if fi == nil {
			continue
		}
		if fi.Language == "" && !isConfigLikeFile(fi.RelPath) {
			continue
		}
		if isConfigLikeFile(fi.RelPath) {
			cfgEvidence := lowerConfigFile(opts.RepoRoot, fi.RelPath)
			evidence = append(evidence, cfgEvidence...)
			continue
		}

		lines, err := readFileLines(filepath.Join(opts.RepoRoot, fi.RelPath))
		if err != nil {
			continue
		}
		loweredFile := loadOrLowerFile(opts, fi, lines, lowerers[fi.Language])
		if loweredFile.File == "" {
			continue
		}
		// Apply the subject-aware relevance gate and the per-file
		// evidence cap here — AFTER the cache round-trip. The on-disk
		// lowerer cache is keyed on file path + hash only; stashing
		// the post-gate slice would mean a second question with
		// different EntityBias would read the first question's
		// filtered result. Mutating the in-memory return value is
		// safe because saveLoweredFileCache has already run inside
		// loadOrLowerFile on miss and it serialised the pristine set.
		filePathHit := fileHitsBias(loweredFile.File, biasTokens)
		explicitPathHit := fileHitsExplicitBias(loweredFile.File, biasTokens)
		loweredFile.Evidence = applyEvidenceRelevanceGate(loweredFile.Evidence, biasTokens, opts.MaxItemsPerFile, filePathHit, explicitPathHit)
		lowered = append(lowered, loweredFile)
		evidence = append(evidence, loweredFile.Evidence...)
	}

	if truncated {
		evidence = append(evidence, newEvidenceItem(
			types.EvidenceTruncated,
			"candidate_files",
			"truncated_to_budget",
			fmt.Sprintf("%d", opts.MaxFiles),
			"",
			"",
			"",
			0,
			0,
			0.5,
			"dataflow.engine",
			fmt.Sprintf("dataflow candidate set exceeded budget and was truncated to %d files", opts.MaxFiles),
			"",
			"",
		))
	}

	evidence = mergeEvidenceItems(evidence)
	if opts.SkipFindings {
		return Result{
			Evidence: evidence,
			Findings: nil,
		}
	}
	findings := buildFindings(graph, lowered, evidence, opts)
	evidence = append(evidence, evidenceFromFindings(findings)...)
	evidence = mergeEvidenceItems(evidence)
	return Result{
		Evidence: evidence,
		Findings: findings,
	}
}

func selectCandidateFiles(graph *repomap.Graph, opts Options) ([]string, bool) {
	seen := make(map[string]bool)
	var ordered []string
	add := func(path string) {
		path = filepath.ToSlash(path)
		if path == "" || seen[path] {
			return
		}
		if len(opts.Scope) > 0 {
			inScope := false
			for _, prefix := range opts.Scope {
				if strings.HasPrefix(path, filepath.ToSlash(prefix)) {
					inScope = true
					break
				}
			}
			if !inScope {
				return
			}
		}
		seen[path] = true
		ordered = append(ordered, path)
	}

	for _, path := range opts.CandidateFiles {
		add(path)
	}

	seed := append([]string(nil), ordered...)
	for _, path := range seed {
		for _, dep := range graph.FilesImportedBy(path) {
			add(dep)
		}
		for _, rev := range graph.FilesImporting(path) {
			add(rev)
		}
		if fi := graph.FileIndex[path]; fi != nil {
			for _, rel := range fi.Relations {
				if defs := graph.SymbolDefs[rel.To]; len(defs) > 0 {
					for _, def := range defs {
						add(def.File)
					}
				}
			}
		}
	}

	// T2.3: re-rank by EntityBias before truncating. The legacy ordering
	// is alphabetical, which means files containing the question's
	// entities are arbitrarily sorted relative to unrelated files of the
	// same name prefix and may be evicted by MaxFiles. With bias, files
	// whose path or symbol names mention any biased entity float to the
	// top so they survive truncation.
	if len(opts.EntityBias) > 0 {
		ordered = sortByEntityBias(ordered, graph, opts.EntityBias)
	} else {
		sort.Strings(ordered)
	}
	truncated := false
	if len(ordered) > opts.MaxFiles {
		ordered = ordered[:opts.MaxFiles]
		truncated = true
	}
	return ordered, truncated
}

// sortByEntityBias re-orders the candidate file list so that files
// matching any biased entity (in path or symbol names) come first.
// Files with equal scores fall back to alphabetical order so the
// result is deterministic across runs.
//
// Scoring is structural and query-independent: the function does NOT
// peek at file contents or query the LLM — it only uses the entity
// strings (already extracted by ERM) and the static graph index.
func sortByEntityBias(files []string, graph *repomap.Graph, entities []string) []string {
	if len(entities) == 0 || len(files) == 0 {
		return files
	}
	lowerEntities := make([]string, 0, len(entities))
	for _, e := range entities {
		if e == "" {
			continue
		}
		lowerEntities = append(lowerEntities, strings.ToLower(e))
	}
	if len(lowerEntities) == 0 {
		return files
	}

	type scored struct {
		path  string
		score int
	}
	out := make([]scored, 0, len(files))
	for _, p := range files {
		lp := strings.ToLower(p)
		score := 0
		for _, ent := range lowerEntities {
			if strings.Contains(lp, ent) {
				score += 2 // path match — high signal
			}
		}
		if graph != nil {
			if fi := graph.FileIndex[p]; fi != nil {
				for _, sym := range fi.Symbols {
					ls := strings.ToLower(sym.Name)
					for _, ent := range lowerEntities {
						if strings.Contains(ls, ent) {
							score++
						}
					}
				}
			}
		}
		out = append(out, scored{path: p, score: score})
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		return out[i].path < out[j].path
	})

	result := make([]string, len(out))
	for i, s := range out {
		result[i] = s.path
	}
	return result
}

func readFileLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return strings.Split(string(data), "\n"), nil
}

func loadOrLowerFile(opts Options, fi *repomap.FileInfo, lines []string, lowerer Lowerer) LoweredFile {
	if lowerer == nil {
		return LoweredFile{}
	}
	if cached, ok := loadLoweredFileFromCache(opts, fi); ok {
		return cached
	}
	lowered := lowerer.LowerFile(opts.RepoRoot, fi, lines, opts)
	saveLoweredFileCache(opts, fi, lowered)
	saveSummaryCache(opts, fi, lowered.Summaries)
	return lowered
}

func loadLoweredFileFromCache(opts Options, fi *repomap.FileInfo) (LoweredFile, bool) {
	cacheFile := loweredCacheFile(opts, fi)
	if cacheFile == "" {
		return LoweredFile{}, false
	}
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return LoweredFile{}, false
	}
	var lowered LoweredFile
	if err := json.Unmarshal(data, &lowered); err != nil {
		return LoweredFile{}, false
	}
	return lowered, lowered.Hash == fi.Hash
}

func saveLoweredFileCache(opts Options, fi *repomap.FileInfo, lowered LoweredFile) {
	cacheFile := loweredCacheFile(opts, fi)
	if cacheFile == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(cacheFile), 0o755)
	_ = os.WriteFile(cacheFile, marshalJSON(lowered), 0o644)
}

func saveSummaryCache(opts Options, fi *repomap.FileInfo, summaries []FunctionSummary) {
	baseDir := summaryCacheDir(opts)
	if baseDir == "" {
		return
	}
	_ = os.MkdirAll(baseDir, 0o755)
	for _, summary := range summaries {
		entry := summaryCacheEntry{
			Key:     summary.SymbolKey + ":" + fi.Hash,
			Summary: summary,
		}
		sum := sha256.Sum256([]byte(entry.Key))
		name := hex.EncodeToString(sum[:6]) + ".json"
		_ = os.WriteFile(filepath.Join(baseDir, name), marshalJSON(entry), 0o644)
	}
}

func loweredCacheFile(opts Options, fi *repomap.FileInfo) string {
	if opts.WorkDir == "" || fi == nil {
		return ""
	}
	base := filepath.Join(opts.WorkDir, "dataflow-cache", "lowered")
	sum := sha256.Sum256([]byte(fi.RelPath + ":" + fi.Hash + ":" + lowererVersion))
	return filepath.Join(base, hex.EncodeToString(sum[:8])+".json")
}

func summaryCacheDir(opts Options) string {
	if opts.WorkDir == "" {
		return ""
	}
	return filepath.Join(opts.WorkDir, "dataflow-cache", "summaries")
}

func buildFindings(graph *repomap.Graph, lowered []LoweredFile, evidence []types.EvidenceItem, opts Options) []types.FlowFindingDigest {
	if len(lowered) == 0 {
		return nil
	}

	summariesByName := make(map[string][]FunctionSummary)
	summariesByKey := make(map[string]FunctionSummary)
	for _, file := range lowered {
		for _, summary := range file.Summaries {
			summariesByName[summary.SymbolName] = append(summariesByName[summary.SymbolName], summary)
			summariesByKey[summary.SymbolKey] = summary
		}
	}

	configValues := collectConfigEvidence(evidence)
	evidenceByKey := evidenceIndex(evidence)

	var findings []FlowFinding

	for _, summary := range summariesByKey {
		for _, cfg := range summary.ReadConfigKeys {
			for _, value := range configValues[cfg] {
				findings = append(findings, FlowFinding{
					Path:        []string{cfg, summary.SymbolKey},
					Conditions:  guardTexts(summary.Guards),
					Sources:     []string{value.Source, cfg},
					Sinks:       []string{summary.SymbolKey},
					Hops:        []string{"reads_config"},
					EvidenceIDs: collectEvidenceIDs(evidenceByKey, cfg, summary.SymbolKey),
					Confidence:  0.84,
				})
			}
		}
		for _, callee := range summary.Calls {
			for _, calleeSummary := range summariesByName[callee] {
				findings = append(findings, flowFromCall(summary, calleeSummary, evidenceByKey)...)
			}
		}
		if summary.HasUnknownEffect {
			findings = append(findings, FlowFinding{
				Path:              []string{summary.SymbolKey},
				Sources:           []string{summary.File},
				Sinks:             []string{summary.SymbolKey},
				Hops:              []string{"unknown_effect"},
				EvidenceIDs:       collectEvidenceIDs(evidenceByKey, summary.SymbolKey),
				UnsupportedReason: summary.UnknownReason,
				Confidence:        0.4,
			})
		}
	}

	return digestFindings(findings)
}

func flowFromCall(caller, callee FunctionSummary, evidenceByKey map[string][]types.EvidenceItem) []FlowFinding {
	var findings []FlowFinding

	for _, literal := range callee.Literals {
		findings = append(findings, FlowFinding{
			Path:        []string{callee.SymbolKey, caller.SymbolKey},
			Conditions:  mergeStrings(guardTexts(callee.Guards), guardTexts(caller.Guards)),
			Sources:     []string{callee.SymbolKey, callee.File},
			Sinks:       []string{caller.SymbolKey},
			Hops:        []string{"calls", "returns " + literal},
			EvidenceIDs: collectEvidenceIDs(evidenceByKey, callee.SymbolKey, caller.SymbolKey, literal),
			Confidence:  0.81,
		})
	}

	for _, cfg := range callee.ReadConfigKeys {
		findings = append(findings, FlowFinding{
			Path:        []string{cfg, callee.SymbolKey, caller.SymbolKey},
			Conditions:  mergeStrings(guardTexts(callee.Guards), guardTexts(caller.Guards)),
			Sources:     []string{cfg, callee.File},
			Sinks:       []string{caller.SymbolKey},
			Hops:        []string{"reads_config", "calls"},
			EvidenceIDs: collectEvidenceIDs(evidenceByKey, cfg, callee.SymbolKey, caller.SymbolKey),
			Confidence:  0.79,
		})
	}

	for _, field := range callee.WrittenFields {
		findings = append(findings, FlowFinding{
			Path:        []string{callee.SymbolKey, field, caller.SymbolKey},
			Conditions:  mergeStrings(guardTexts(callee.Guards), guardTexts(caller.Guards)),
			Sources:     []string{callee.File},
			Sinks:       []string{caller.SymbolKey},
			Hops:        []string{"calls", "stores_field", "observed_by_caller"},
			EvidenceIDs: collectEvidenceIDs(evidenceByKey, callee.SymbolKey, caller.SymbolKey, field),
			Confidence:  0.7,
		})
	}

	return findings
}

func digestFindings(findings []FlowFinding) []types.FlowFindingDigest {
	if len(findings) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	var digests []types.FlowFindingDigest
	for _, finding := range findings {
		digest := types.FlowFindingDigest{
			Path:              normalizeStrings(finding.Path),
			Conditions:        normalizeStrings(finding.Conditions),
			Sources:           normalizeStrings(finding.Sources),
			Sinks:             normalizeStrings(finding.Sinks),
			Hops:              normalizeStrings(finding.Hops),
			EvidenceIDs:       normalizeStrings(finding.EvidenceIDs),
			UnsupportedReason: finding.UnsupportedReason,
			Confidence:        finding.Confidence,
		}
		digest.ID = types.StableFlowFindingID(digest.Path, digest.Conditions, digest.Sources, digest.Sinks)
		if seen[digest.ID] {
			continue
		}
		seen[digest.ID] = true
		digests = append(digests, digest)
	}
	sort.Slice(digests, func(i, j int) bool {
		return digests[i].Confidence > digests[j].Confidence
	})
	return digests
}

func guardTexts(guards []GuardExpr) []string {
	var out []string
	for _, guard := range guards {
		if guard.Expr != "" {
			out = append(out, guard.Expr)
		}
	}
	return out
}

func evidenceIndex(items []types.EvidenceItem) map[string][]types.EvidenceItem {
	index := make(map[string][]types.EvidenceItem)
	for _, item := range items {
		for _, key := range []string{item.Subject, item.Object, item.Source} {
			if key != "" {
				index[key] = append(index[key], item)
			}
		}
	}
	return index
}

func collectEvidenceIDs(index map[string][]types.EvidenceItem, keys ...string) []string {
	seen := make(map[string]bool)
	var ids []string
	for _, key := range keys {
		for _, item := range index[key] {
			if item.ID == "" || seen[item.ID] {
				continue
			}
			seen[item.ID] = true
			ids = append(ids, item.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

func mergeEvidenceItems(items []types.EvidenceItem) []types.EvidenceItem {
	if len(items) == 0 {
		return nil
	}
	merged := make(map[string]types.EvidenceItem, len(items))
	for _, item := range items {
		if item.ID == "" {
			item.ID = types.StableEvidenceID(item.Kind, item.Subject, item.Predicate, item.Object, item.Condition, item.Source, item.LineStart, item.LineEnd)
		}
		if existing, ok := merged[item.ID]; ok {
			if existing.Summary == "" && item.Summary != "" {
				existing.Summary = item.Summary
			}
			existing.Confidence = max(existing.Confidence, item.Confidence)
			existing.DerivedFrom = normalizeStrings(append(existing.DerivedFrom, item.DerivedFrom...))
			if existing.EvidenceRef == "" {
				existing.EvidenceRef = item.EvidenceRef
			}
			merged[item.ID] = existing
			continue
		}
		item.DerivedFrom = normalizeStrings(item.DerivedFrom)
		merged[item.ID] = item
	}
	result := make([]types.EvidenceItem, 0, len(merged))
	for _, item := range merged {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Source != result[j].Source {
			return result[i].Source < result[j].Source
		}
		if result[i].LineStart != result[j].LineStart {
			return result[i].LineStart < result[j].LineStart
		}
		return result[i].ID < result[j].ID
	})
	return result
}

func normalizeStrings(items []string) []string {
	seen := make(map[string]bool, len(items))
	var out []string
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func mergeStrings(parts ...[]string) []string {
	var all []string
	for _, part := range parts {
		all = append(all, part...)
	}
	return normalizeStrings(all)
}

func collectConfigEvidence(items []types.EvidenceItem) map[string][]types.EvidenceItem {
	index := make(map[string][]types.EvidenceItem)
	for _, item := range items {
		if item.Kind != types.EvidenceConcrete || item.Predicate != "config_value" {
			continue
		}
		index[item.Subject] = append(index[item.Subject], item)
	}
	return index
}

func evidenceFromFindings(findings []types.FlowFindingDigest) []types.EvidenceItem {
	var items []types.EvidenceItem
	for _, finding := range findings {
		kind := types.EvidenceDataflowPath
		if finding.UnsupportedReason != "" {
			kind = types.EvidenceUnresolved
		}
		subject := strings.Join(finding.Sources, ", ")
		if subject == "" && len(finding.Path) > 0 {
			subject = finding.Path[0]
		}
		object := strings.Join(finding.Sinks, ", ")
		if object == "" && len(finding.Path) > 0 {
			object = finding.Path[len(finding.Path)-1]
		}
		summary := strings.Join(finding.Path, " -> ")
		if summary == "" {
			summary = strings.Join(finding.Hops, " -> ")
		}
		if len(finding.Conditions) > 0 {
			summary += " IF " + strings.Join(finding.Conditions, " AND ")
		}
		if finding.UnsupportedReason != "" {
			summary += " [uncertain: " + finding.UnsupportedReason + "]"
		}
		item := newEvidenceItem(
			kind,
			subject,
			"flows_to",
			object,
			strings.Join(finding.Conditions, " AND "),
			firstPathSource(finding),
			"",
			0,
			0,
			finding.Confidence,
			"dataflow.finding",
			summary,
			"",
			"",
		)
		item.DerivedFrom = append(item.DerivedFrom, finding.EvidenceIDs...)
		items = append(items, item)
	}
	return items
}

func firstPathSource(finding types.FlowFindingDigest) string {
	for _, source := range finding.Sources {
		if strings.Contains(source, "/") {
			return source
		}
	}
	for _, sink := range finding.Sinks {
		if strings.Contains(sink, "/") {
			return sink
		}
	}
	return ""
}

func lowerConfigFile(repoRoot, relPath string) []types.EvidenceItem {
	path := filepath.Join(repoRoot, relPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var flattened []struct {
		Key   string
		Value string
	}

	switch {
	case strings.HasSuffix(relPath, ".yaml"), strings.HasSuffix(relPath, ".yml"):
		var root any
		if err := yaml.Unmarshal(data, &root); err != nil {
			return nil
		}
		flattened = flattenConfig(root, "")
	case strings.HasSuffix(relPath, ".json"):
		var root any
		if err := json.Unmarshal(data, &root); err != nil {
			return nil
		}
		flattened = flattenConfig(root, "")
	case strings.HasSuffix(relPath, ".toml"):
		flattened = flattenTOML(data)
	default:
		return nil
	}

	var items []types.EvidenceItem
	for _, entry := range flattened {
		if entry.Key == "" || entry.Value == "" {
			continue
		}
		items = append(items, newEvidenceItem(
			types.EvidenceConcrete,
			entry.Key,
			"config_value",
			entry.Value,
			"",
			relPath,
			"",
			1,
			1,
			0.9,
			"dataflow.config",
			fmt.Sprintf("config %s = %s", entry.Key, entry.Value),
			types.AnchorDefinition,
			entry.Key,
		))
	}
	return items
}

func flattenConfig(value any, prefix string) []struct {
	Key   string
	Value string
} {
	var out []struct {
		Key   string
		Value string
	}
	switch v := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			next := key
			if prefix != "" {
				next = prefix + "." + key
			}
			out = append(out, flattenConfig(v[key], next)...)
		}
	case map[any]any:
		for rawKey, rawValue := range v {
			key := fmt.Sprintf("%v", rawKey)
			next := key
			if prefix != "" {
				next = prefix + "." + key
			}
			out = append(out, flattenConfig(rawValue, next)...)
		}
	case []any:
		for idx, item := range v {
			next := fmt.Sprintf("%s[%d]", prefix, idx)
			out = append(out, flattenConfig(item, next)...)
		}
	default:
		if prefix != "" {
			out = append(out, struct {
				Key   string
				Value string
			}{Key: prefix, Value: fmt.Sprintf("%v", v)})
		}
	}
	return out
}

func flattenTOML(data []byte) []struct {
	Key   string
	Value string
} {
	lines := strings.Split(string(data), "\n")
	var prefix string
	var out []struct {
		Key   string
		Value string
	}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			prefix = strings.Trim(line, "[]")
			continue
		}
		if idx := strings.Index(line, "="); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])
			if prefix != "" {
				key = prefix + "." + key
			}
			out = append(out, struct {
				Key   string
				Value string
			}{Key: key, Value: value})
		}
	}
	return out
}

func isConfigLikeFile(path string) bool {
	return strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") ||
		strings.HasSuffix(path, ".json") || strings.HasSuffix(path, ".toml")
}
