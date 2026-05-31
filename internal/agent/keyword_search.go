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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/analysis/declarative"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

// declarativeClassifier is the shared Session 11 R1 / C0' Classifier.
// Package-level singleton so R1 (keyword_search boost) and C0'
// (analyzer Round 2 gate) stay aligned on the same filename
// pattern list / small-file thresholds.
var declarativeClassifier = declarative.New(declarative.DefaultConfig())

// searchTimeout is the maximum wall-clock time for any single search
// command (rg, grep, find). Prevents hangs on large repos or
// pathological regex patterns.
const searchTimeout = 60 * time.Second

// keywordFileScore records how a file scored across multi-level keyword matching.
type keywordFileScore struct {
	Path            string
	Score           float64
	RepoMapScore    float64           // raw repo_map structural score (for coverage selection)
	Hits            map[string]string // keyword → best match level for debugging
	Symbols         []string          // symbol summaries from repo_map (e.g. "RegisterDefaultSubAgents function:63")
	ExactEntityRank int               // >0 when a unique exact entity anchor matched this file
}

// keywordSearchResult wraps the scored files plus the repo_map graph
// for downstream use (symbol lookups, cross-reference tracking).
type keywordSearchResult struct {
	Files []keywordFileScore
	Graph *repomap.Graph // may be nil if repo_map is unavailable

	// MultiGraph is the optional cross-sub-repo carrier (P4-cross-
	// sub-repo Sc 5, 2026-05-08). Non-nil when keyword_search ran
	// against a multi-repo workspace; downstream consumers that want
	// cross-sub-repo Symbol fan-out reach for it via
	// repomap.MultiGraphFromXxx helpers. Files[].Path uses path-
	// from-parent (sub-repo prefix prepended) when MultiGraph != nil
	// AND mg.IsSingle()==false; single-repo posture keeps un-prefixed
	// paths byte-identically.
	MultiGraph any
}

// keywordSearchOptions tunes the scoring and cap behavior of
// keywordSearchWithOptions. Zero values are treated as "use the
// historical defaults" so the thin backwards-compat wrapper
// `keywordSearch` keeps its pre-refactor semantics byte-for-byte.
//
// Added in T1a + T1b (cross-package question debug follow-up):
//   - Entities surfaces the analyzer's high-confidence identifier
//     tokens (verbatim from the user question). When provided,
//     files whose path or indexed symbols match any entity get a
//     multiplicative boost on top of the grep IDF × repo_map score.
//     Entities carry strictly stronger signal than keywords (they
//     are user-authored identifiers, not synonym expansions), so
//     the boost is weighted accordingly — see applyEntityBoost.
//   - MaxFiles caps the returned slice length. When 0, the default
//     of `defaultKeywordSearchMaxFiles` (20) applies; when > 0, the
//     caller's explicit value wins. Used for complexity-aware
//     scaling: complex cross-package questions need a wider
//     candidate pool so the top-20 cap does not drop the
//     dispatch-path files below the LLM's eyeline.
type keywordSearchOptions struct {
	Entities []string
	// EntityProvenance is an optional typed side-lane for Entities.
	// When present, keyword_search uses UseForSearch to keep shape
	// vocabulary from becoming repo_map / exact-anchor / boost input.
	// Empty/nil preserves pre-provenance behavior.
	EntityProvenance []types.EntityProvenance
	// MentionedEntities is the deterministic subset of Entities whose
	// surfaces are explicitly present in the user's RawRequest. When
	// non-empty, exactEntityAnchors should prefer this provenance lane
	// so analyzer-derived context cannot hijack exact-anchor focus.
	MentionedEntities []string
	// PrimaryEntities is the pre-merge top-level entity list captured
	// by the analyzer before sub-topic entities were unioned into
	// Entities. exactEntityAnchors consumes this when non-empty so the
	// unique-anchor selector stays on user-named identifiers and does
	// not latch onto a planner-added sub-topic descriptor that happens
	// to case-match a repo symbol. Falls back to Entities when empty
	// (no sub-topics, or tests that pre-date the split) — preserves
	// pre-fix behaviour for every non-multi-topic call site.
	PrimaryEntities []string
	// DomainHints is the set of TermSymbol Domain tags from the
	// analyzer's TermGraph (non-empty only when the normalizer had a
	// repo-grounded SymbolResolver). Files whose FileInfo.Package
	// matches any hint get a small multiplicative boost — this
	// amplifies siblings of the answer symbol in the same package,
	// which frequently hold collaborating helpers and types without
	// mentioning the symbol name verbatim. The boost is smaller than
	// the entity boost by construction (entities are exact-name
	// matches, domain is a coarser sibling signal).
	DomainHints []string
	MaxFiles    int
	// ExactResolution marks searches where the user asked for one
	// exact target and auxiliary/test/doc mentions must not hijack the
	// candidate ranking.
	ExactResolution *types.ExactResolutionContract
	// SourceScope carries the analyzer's typed answer-scope judgement for
	// repo paths. Production-scope searches keep tests/docs/fixtures/examples
	// visible but down-ranked, while explicit test/docs/all scopes can promote
	// those roles as principal candidates.
	SourceScope *types.SourceScopeProfile
	// SuppressExactEntityAnchors disables the unique-symbol fast path.
	// Used for observation-only runtime artifacts: analyzer entities
	// such as "load" / "config" / "KeyError" describe an attached
	// external artifact, not current-repo targets, so an exact repo
	// symbol collision must remain soft ranking at most.
	SuppressExactEntityAnchors bool
	// MultiGraph carries the multi-repo carrier from the calling
	// AgentContext (Phase 4.3). Stored as `any` to dodge the
	// types↔multigraph import cycle. nil triggers legacy single-graph
	// BuildOrLoadGraph(repoRoot, query) behaviour. Single-repo
	// posture proxies to mg.Single(); multi-repo posture currently
	// falls back to the largest sub-repo's graph (a documented
	// half-step until the raw consumer migration in design §11
	// completes).
	MultiGraph any
	// SearchGraph carries the run-local single-repo graph already
	// built by an earlier stage in the same pipeline. It is read-only
	// input for repo_map ranking: the repomap facade clones and reranks
	// it for the current query. This avoids repeating file inventory /
	// cache validation in single-repo/no-MultiGraph runs while keeping
	// the existing BuildOrLoadGraph fallback when no compatible graph
	// exists.
	SearchGraph any
	// PendingSubRepos mirrors BusContext.PendingSubRepos: the RootRel
	// list of sub-repos the routing fold left inactive for this
	// question. The multi-repo aggregator in repoMapRank skips these
	// so pre-scan ranking stays consistent with the file-system tools'
	// MultiRepoActiveSetGater (which refuses reads on the same set).
	// Without this, LRU-resident-but-inactive sub-repos leak files
	// into Phase1Ranking and the CGEC pre-complete forced-read gate
	// queues paths the explorer cannot satisfy, pushing the model into
	// an evidence_floor_waiver(no_repo_intersection) escape that has
	// nothing to do with attached runtime artifacts.
	PendingSubRepos []string
}

func keywordSearchGraphHandle(ctx *types.AgentContext, repoRoot string) any {
	if ctx == nil {
		return nil
	}
	if keywordSearchGraphMatchesRoot(ctx.SearchGraph, repoRoot) {
		return ctx.SearchGraph
	}
	if ctx.Mutable != nil {
		if g := ctx.Mutable.SearchGraph(); keywordSearchGraphMatchesRoot(g, repoRoot) {
			return g
		}
	}
	if ctx.SearchGraph != nil {
		return ctx.SearchGraph
	}
	if ctx.Mutable != nil {
		return ctx.Mutable.SearchGraph()
	}
	return nil
}

func keywordSearchGraphMatchesRoot(handle any, repoRoot string) bool {
	g, ok := handle.(*repomap.Graph)
	if !ok || g == nil || strings.TrimSpace(g.Root) == "" || strings.TrimSpace(repoRoot) == "" {
		return false
	}
	ga, errA := filepath.Abs(g.Root)
	ra, errB := filepath.Abs(repoRoot)
	if errA != nil || errB != nil {
		return filepath.Clean(g.Root) == filepath.Clean(repoRoot)
	}
	return filepath.Clean(ga) == filepath.Clean(ra)
}

// defaultKeywordSearchMaxFiles is the historical cap preserved for
// callers that don't pass an explicit MaxFiles. Matches the pre-T1b
// hardcoded `results[:20]` behavior byte-for-byte so un-converted
// callers see no change.
const defaultKeywordSearchMaxFiles = 20

// MaxFilesForComplexity returns the recommended keyword-search top-N
// cap for a given analyzer-classified complexity. Complex questions
// (6+ files, cross-component) need a wider candidate pool because
// the critical dispatch-path file may sit at rank 22–28 if the
// question's keywords also match a very large implementation file
// like explorer.go. Simple questions stay at a tighter cap so the
// LLM's prompt budget is not wasted on low-signal candidates.
//
// Mapping:
//
//	simple   → 15 (single-file lookups, tight focus)
//	moderate → 20 (historical default, 3-5 file questions)
//	complex  → 30 (cross-component, 6+ files)
//	other    → 20 (safe default for "" / unknown / future values)
//
// Callers that want a fixed cap regardless of complexity can pass
// MaxFiles directly to keywordSearchOptions.
func MaxFilesForComplexity(complexity types.Complexity) int {
	switch complexity {
	case types.ComplexitySimple:
		return 15
	case types.ComplexityComplex:
		return 30
	default:
		return defaultKeywordSearchMaxFiles
	}
}

// keywordSearch is the pre-T1a thin wrapper preserved for callers
// (and tests) that don't need entity boosting or custom caps. New
// callers should use keywordSearchWithOptions and pass the analyzer's
// entity list + the complexity-derived MaxFiles.
func keywordSearch(keywords []string, repoRoot string) *keywordSearchResult {
	return keywordSearchWithOptions(keywords, repoRoot, keywordSearchOptions{})
}

// keywordSearchFingerprint produces a stable string fingerprint of the
// four inputs that determine keywordSearchWithOptions's output.
// Callers (today: the explorer's BuildInitialInstruction) compare this
// against a cached fingerprint to short-circuit recomputation when the
// same Run re-dispatches explorer with identical analyzer output.
// Order-independent: slices are sorted before joining so keyword
// permutations produce the same key.
func keywordSearchFingerprint(keywords, entities, mentionedEntities, primaryEntities, domainHints, exactTargets []string, exactPolicy string, maxFiles int, suppressExactAnchors bool, sourceScopeFingerprint string) string {
	cp := func(s []string) []string {
		if len(s) == 0 {
			return nil
		}
		out := make([]string, len(s))
		copy(out, s)
		sort.Strings(out)
		return out
	}
	var b strings.Builder
	b.WriteString(strings.Join(cp(keywords), "\x00"))
	b.WriteByte('|')
	b.WriteString(strings.Join(cp(entities), "\x00"))
	b.WriteByte('|')
	b.WriteString(strings.Join(cp(mentionedEntities), "\x00"))
	b.WriteByte('|')
	b.WriteString(strings.Join(cp(primaryEntities), "\x00"))
	b.WriteByte('|')
	b.WriteString(strings.Join(cp(domainHints), "\x00"))
	b.WriteByte('|')
	b.WriteString(strings.Join(cp(exactTargets), "\x00"))
	b.WriteByte('|')
	b.WriteString(strings.TrimSpace(strings.ToLower(exactPolicy)))
	b.WriteByte('|')
	b.WriteString(strconv.Itoa(maxFiles))
	b.WriteByte('|')
	if suppressExactAnchors {
		b.WriteString("suppress_exact_anchors")
	}
	b.WriteByte('|')
	b.WriteString(strings.TrimSpace(strings.ToLower(sourceScopeFingerprint)))
	return b.String()
}

// keywordSearchScopedFingerprint adds run-scope inputs that affect
// keywordSearchWithOptions but do not belong in the semantic search
// fingerprint itself. This keeps same-question intra-Run redispatches
// cacheable while preventing REPL turns in another repo or multi-repo
// active set from reusing a stale ranked graph.
func keywordSearchScopedFingerprint(ctx *types.AgentContext, fp string) string {
	if ctx == nil {
		return fp
	}
	cp := func(s []string) []string {
		if len(s) == 0 {
			return nil
		}
		out := make([]string, len(s))
		copy(out, s)
		sort.Strings(out)
		return out
	}
	var topology []string
	for _, sr := range ctx.SubRepos {
		rootRel := strings.TrimSpace(sr.RootRel)
		if rootRel == "" {
			rootRel = "."
		}
		topology = append(topology, strings.TrimSpace(sr.Slug)+"="+rootRel)
	}
	sort.Strings(topology)

	var b strings.Builder
	b.WriteString(fp)
	b.WriteString("|repo=")
	if repo := strings.TrimSpace(ctx.RepoRoot); repo != "" {
		b.WriteString(filepath.Clean(repo))
	}
	b.WriteString("|pending=")
	b.WriteString(strings.Join(cp(ctx.PendingSubRepos), "\x00"))
	b.WriteString("|topology=")
	b.WriteString(strings.Join(topology, "\x00"))
	return b.String()
}

// keywordSearchWithOptions combines repo_map's structural ranking
// with grep-based keyword matching to produce a scored file list.
//
// Strategy:
//  1. Build/load the repo_map graph (cached, fast) and rank files using
//     the keywords PLUS analyzer entities as query. This gives structural
//     signal: files that DEFINE matching symbols score higher than files
//     that merely mention them in comments.
//  2. Run grep for each expanded keyword and compute IDF-weighted scores.
//     Keywords matching fewer files are more informative (IDF = log(N/df)).
//  3. Merge: repo_map score (normalized) + grep IDF score, with repo_map
//     weighted higher because structural definitions are more reliable
//     than text mentions.
//  4. (T1a) Apply entity boost: files whose path or indexed symbols
//     match any analyzer-emitted entity get a 1.3x–1.6x multiplier.
//     Entities are high-confidence identifier tokens copied verbatim
//     from the user question, so they carry strictly stronger signal
//     than the expanded keyword list.
//  5. Rescue unique exact entity anchors from the repo graph even when
//     grep misses them, then sort anchored files ahead of broad keyword
//     matches so an explicitly named symbol reaches the LLM's eyeline.
//  6. Cap the result list at `opts.MaxFiles` (or the historical 20
//     when opts.MaxFiles is zero).
func keywordSearchWithOptions(keywords []string, repoRoot string, opts keywordSearchOptions) *keywordSearchResult {
	if len(keywords) == 0 || repoRoot == "" {
		return nil
	}

	keywords = expandKeywords(keywords)
	searchEntities := filterEntitiesByProvenance(opts.Entities, opts.EntityProvenance, entityProvenanceRoleSearch)
	if len(opts.EntityProvenance) > 0 && len(searchEntities) != len(opts.Entities) {
		logging.Debug("[keyword_search] entities pre-filter=%d post-filter=%d", len(opts.Entities), len(searchEntities))
	}

	// --- Phase 1: repo_map structural ranking ---
	repoMapScores, graph := repoMapRank(keywords, searchEntities, repoRoot, opts.MultiGraph, opts.SearchGraph, opts.PendingSubRepos)
	// exactEntityAnchors wants user-named entities only. When the
	// exactEntityAnchors wants the strongest provenance lane available.
	// Prefer deterministic MentionedEntities (verbatim RawRequest
	// surfaces), fall back to analyzer-authored PrimaryEntities, then to
	// the broadened Entities list only when no narrower lane exists.
	exactAnchors := exactEntityAnchorsForKeywordSearchOptions(graph, opts)

	// --- Phase 2: grep IDF-weighted scoring ---
	var grepScores map[string]float64
	var grepHits map[string]map[string]string
	if keywordSearchShouldSkipGrepForBroadRepoMap(repoMapScores, graph) {
		logging.Debug("[keyword_search] repo_map query too broad (%d/%d files); skipping grep candidate pre-scan",
			len(repoMapScores), keywordSearchGraphFileCount(graph))
		grepScores = map[string]float64{}
		grepHits = map[string]map[string]string{}
	} else {
		grepScores, grepHits = grepIDFSearchForOptions(keywords, repoRoot, opts)
	}
	// Defense in depth: grepIDFSearchForOptions restricts multi-repo
	// searches to LRU-active sub-repo roots when the MultiGraph carrier
	// is available. Keep the pending-sub-repo filter here for callers
	// without a carrier and for any future fallback that has to scan the
	// parent workspace.
	if len(opts.PendingSubRepos) > 0 {
		for f := range grepScores {
			if types.PathInsidePendingSubRepo(f, opts.PendingSubRepos) {
				delete(grepScores, f)
				delete(grepHits, f)
			}
		}
	}

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

	candidates := keywordSearchCandidatePaths(grepScores, exactAnchors)
	results := make([]keywordFileScore, 0, len(candidates))
	for _, f := range candidates {
		if isNoisePath(f) {
			continue
		}
		grepScore := grepScores[f]

		// Repo_map boost: files with higher structural importance
		// get a multiplier on their grep score. The boost ranges
		// from 1.0 (no structural signal) to 2.0 (top structural).
		boost := 1.0
		if maxRM > 0 && repoMapScores[f] > 0 {
			boost = 1.0 + (repoMapScores[f]/maxRM)*1.0
		}
		combined := grepScore * boost
		if combined == 0 {
			// Unique exact-entity anchor rescue path: if grep missed the
			// file entirely but the repo graph proves a unique exact
			// symbol/path match, seed it from structural signal so it can
			// still enter the candidate pool.
			if maxRM > 0 && repoMapScores[f] > 0 {
				combined = 1.0 + repoMapScores[f]/maxRM
			} else if _, ok := exactAnchors[f]; ok {
				combined = 1.0
			}
		}

		// Session 11 R1 DeclarativeBoost — declarative filenames
		// (topology, defaults, registry, routes, wire, init,
		// manifest, schema, enum) and small literal-density
		// files get an additive bonus that tips registration /
		// config_mapping / call_chain answers toward the right
		// file. The boost is additive (not multiplicative) so it
		// never dwarfs a highly-matched implementation file; it
		// only breaks ties between small declarative files and
		// giant function-body files that happen to mention the
		// same keywords.
		if kind, conf := declarativeClassifier.ClassifyPath(f); kind != declarative.KindNone {
			combined += declarativeClassifier.BoostFor(kind) * conf
		}

		// T1a entity boost — multiplicatively scale the combined
		// score when the file's path or its indexed symbols match
		// any analyzer-emitted entity. Entities are the highest-
		// confidence tokens the analyzer produces (verbatim from
		// the user question, not expanded), so a match here is
		// stronger signal than a keyword hit. The multiplier
		// ladder (path+symbol > path > symbol > none) stacks:
		// a file whose path contains the entity AND whose symbol
		// table declares it gets the full 1.6x; matching only
		// the path or only a symbol gets 1.3x. This is the fix
		// for the 2026-04-18 "explorer是怎么调用subagent的？" root
		// cause where `subagent_runtime.go` ranked below
		// `explorer.go` because the latter's 6000+ lines
		// drowned it on pure-grep IDF scoring.
		if boost := entityBoostFactor(f, graph, searchEntities); boost > 1.0 {
			combined *= boost
		}

		// Domain boost — file whose declared package matches any
		// TermSymbol Domain from the analyzer gets a sibling-level
		// multiplier. Compounding with entity boost is intentional:
		// the definition site (entity match) and its siblings in the
		// same package (domain match) both deserve lift over
		// unrelated files that merely share keywords.
		domainHit := false
		if boost := domainBoostFactor(f, graph, opts.DomainHints); boost > 1.0 {
			combined *= boost
			domainHit = true
		}

		hits := grepHits[f]
		if hits == nil {
			hits = make(map[string]string)
		}
		if domainHit {
			hits["domain_match"] = "1"
		}
		if repoMapScores[f] > 0 {
			hits["repo_map"] = fmt.Sprintf("%.0f", repoMapScores[f])
		}
		exactRank := 0
		if anchor, ok := exactAnchors[f]; ok {
			if shouldDeprioritizeAuxiliaryExactHit(f, opts.ExactResolution) {
				hits["exact_entity_aux"] = anchor.Hit
			} else {
				hits["exact_entity"] = anchor.Hit
				exactRank = anchor.Rank
			}
		}
		if shouldDeprioritizeAuxiliaryExactHit(f, opts.ExactResolution) {
			if combined == 0 {
				continue
			}
			combined *= 0.35
			hits["auxiliary_exact"] = "1"
		}
		if role := types.ClassifySourcePathRole(f); shouldDeprioritizeAuxiliaryBySourceScope(role, opts.SourceScope) {
			if combined == 0 {
				continue
			}
			combined *= 0.35
			hits["auxiliary_scope"] = string(role)
			if exactRank > 0 {
				exactRank = 0
			}
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
			Path:            f,
			Score:           combined,
			RepoMapScore:    repoMapScores[f],
			Hits:            hits,
			Symbols:         syms,
			ExactEntityRank: exactRank,
		})
	}

	sortKeywordResults(results)
	maxFiles := opts.MaxFiles
	if maxFiles <= 0 {
		maxFiles = defaultKeywordSearchMaxFiles
	}
	if len(results) > maxFiles {
		results = results[:maxFiles]
	}

	logging.Debug("[keyword_search] %d keywords, %d entities → %d files scored (cap=%d)",
		len(keywords), len(searchEntities), len(results), maxFiles)
	return &keywordSearchResult{Files: results, Graph: graph, MultiGraph: opts.MultiGraph}
}

const (
	keywordSearchBroadRepoMapMinMatches = 20000
	keywordSearchBroadRepoMapRatio      = 0.50
)

func keywordSearchShouldSkipGrepForBroadRepoMap(repoMapScores map[string]float64, graph *repomap.Graph) bool {
	matches := len(repoMapScores)
	total := keywordSearchGraphFileCount(graph)
	if matches < keywordSearchBroadRepoMapMinMatches || total <= 0 {
		return false
	}
	return float64(matches)/float64(total) >= keywordSearchBroadRepoMapRatio
}

func keywordSearchGraphFileCount(graph *repomap.Graph) int {
	if graph == nil {
		return 0
	}
	switch {
	case len(graph.FileIndex) > 0:
		return len(graph.FileIndex)
	case len(graph.Scores) > 0:
		return len(graph.Scores)
	case len(graph.Files) > 0:
		return len(graph.Files)
	default:
		return 0
	}
}

func shouldDeprioritizeAuxiliaryBySourceScope(role types.SourcePathRole, profile *types.SourceScopeProfile) bool {
	if !types.SourcePathRoleIsAuxiliary(role) {
		return false
	}
	if profile != nil && profile.AllowsAuxiliaryPrincipal() {
		return false
	}
	return true
}

func exactEntityAnchorsForKeywordSearchOptions(graph *repomap.Graph, opts keywordSearchOptions) map[string]exactEntityAnchor {
	if opts.SuppressExactEntityAnchors {
		return nil
	}
	anchorEntities := opts.MentionedEntities
	if len(anchorEntities) == 0 {
		anchorEntities = opts.PrimaryEntities
	}
	if len(anchorEntities) == 0 {
		anchorEntities = opts.Entities
	}
	anchorEntities = filterEntitiesByProvenance(anchorEntities, opts.EntityProvenance, entityProvenanceRoleSearch)
	return exactEntityAnchors(graph, anchorEntities)
}

func shouldDeprioritizeAuxiliaryExactHit(path string, contract *types.ExactResolutionContract) bool {
	if contract == nil || !types.ExactResolutionRequiresDefiningPrimaryProof(contract) {
		return false
	}
	return types.LooksLikeAuxiliaryEvidencePath(path)
}

type exactEntityAnchor struct {
	Rank int
	Hit  string
}

// keywordSearchCandidatePaths returns the union of grep-discovered
// files and unique exact-entity anchors from the repo graph.
func keywordSearchCandidatePaths(
	grepScores map[string]float64,
	exactAnchors map[string]exactEntityAnchor,
) []string {
	seen := make(map[string]bool, len(grepScores)+len(exactAnchors))
	paths := make([]string, 0, len(seen))
	for path := range grepScores {
		if !seen[path] {
			seen[path] = true
			paths = append(paths, path)
		}
	}
	for path := range exactAnchors {
		if !seen[path] {
			seen[path] = true
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths
}

// sortKeywordResults orders files so uniquely exact-anchored entity
// hits outrank broad keyword matches, then falls back to score and
// path for stability.
func sortKeywordResults(results []keywordFileScore) {
	sort.Slice(results, func(i, j int) bool {
		if results[i].ExactEntityRank != results[j].ExactEntityRank {
			return results[i].ExactEntityRank > results[j].ExactEntityRank
		}
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Path < results[j].Path
	})
}

// entityBoostFactor returns the multiplicative score factor for a
// file based on how many analyzer-emitted entities it references.
// Return 1.0 means "no boost"; >1.0 means the file is rescored up.
//
// Signal ladder (strictest wins):
//   - path match + symbol match → 1.6
//   - path match only           → 1.3
//   - symbol match only         → 1.3
//   - neither                   → 1.0
//
// Matching is case-insensitive and entity-boundary-aware for short
// generic nouns. Short entities (< 4 chars) are skipped to avoid false
// positives on 3-letter prefixes like "get", "run", "new" — this is
// especially important because the analyzer's generic-entity blocklist
// does NOT filter by length.
func entityBoostFactor(path string, graph *repomap.Graph, entities []string) float64 {
	if len(entities) == 0 {
		return 1.0
	}
	base := normalizeEntityHaystack(strings.ToLower(filepath.Base(path)))
	pathHit := false
	symbolHit := false
	for _, ent := range entities {
		norm := strings.ToLower(strings.TrimSpace(ent))
		if len(norm) < 4 {
			continue
		}
		if !pathHit && entityHits(base, norm) {
			pathHit = true
		}
		if !symbolHit && graph != nil {
			if fi, ok := graph.FileIndex[path]; ok {
				for _, sym := range fi.Symbols {
					if entityHits(normalizeEntityHaystack(strings.ToLower(sym.Name)), norm) {
						symbolHit = true
						break
					}
				}
			}
		}
		if pathHit && symbolHit {
			break
		}
	}
	switch {
	case pathHit && symbolHit:
		return 1.6
	case pathHit || symbolHit:
		return 1.3
	default:
		return 1.0
	}
}

// domainBoostFactor returns the multiplicative score factor for a
// file whose declared package matches any analyzer-extracted Domain
// hint. Domain hints come from TermGraph TermSymbol entries — they
// represent the package context of symbols the user's question is
// about, flowing from normalizer.SymbolResolver at analyze time.
//
// The boost magnitude (1.15) is intentionally smaller than
// entityBoostFactor's 1.3/1.6 ladder: entities are exact-name matches
// so they imply "this file is THE answer file"; domain matches only
// imply "this file is a sibling of the answer file in the same
// package", a coarser signal that still deserves lift over unrelated
// keyword-only matches. The value is calibrated as
// max(declarativeBoost) ≈ 0.15 additive and moved to the
// multiplicative scale for composition with the other factors — not
// tuned to any specific eval query.
func domainBoostFactor(path string, graph *repomap.Graph, domains []string) float64 {
	if graph == nil || len(domains) == 0 {
		return 1.0
	}
	fi, ok := graph.FileIndex[path]
	if !ok || fi == nil || fi.Package == "" {
		return 1.0
	}
	for _, d := range domains {
		if d == "" {
			continue
		}
		if d == fi.Package {
			return 1.15
		}
	}
	return 1.0
}

// exactEntityAnchors returns files that uniquely and exactly match an
// analyzer-emitted entity by symbol definition or file path. These
// anchors are stronger than broad keyword matches and are used to keep
// the user-named implementation file near the top of the first-round
// ranking.
func exactEntityAnchors(graph *repomap.Graph, entities []string) map[string]exactEntityAnchor {
	if graph == nil || len(entities) == 0 {
		return nil
	}
	type entityTarget struct {
		raw       string
		qualified bool
	}
	var targets []entityTarget
	symbolTargets := make(map[string][]string)
	pathTargets := make(map[string][]string)
	qualifiedTargets := make(map[string][]string)
	seenEntity := make(map[string]bool, len(entities))
	for _, raw := range entities {
		ent := strings.TrimSpace(raw)
		if len(ent) < 4 {
			continue
		}
		key := ent
		if seenEntity[key] {
			continue
		}
		seenEntity[key] = true
		symbolKey := strings.ToLower(ent)
		pathKey := strings.ToLower(filepath.ToSlash(ent))
		target := entityTarget{
			raw:       ent,
			qualified: strings.Contains(ent, "."),
		}
		targets = append(targets, target)
		symbolTargets[symbolKey] = append(symbolTargets[symbolKey], ent)
		pathTargets[pathKey] = append(pathTargets[pathKey], ent)
		if target.qualified {
			qualifiedTargets[symbolKey] = append(qualifiedTargets[symbolKey], ent)
		}
	}
	if len(targets) == 0 {
		return nil
	}

	symbolMatches := make(map[string][]string)
	for name, defs := range graph.SymbolDefs {
		ents := symbolTargets[strings.ToLower(name)]
		if len(ents) == 0 || len(defs) == 0 {
			continue
		}
		files := uniqueSymbolDefFiles(defs)
		for _, ent := range ents {
			symbolMatches[ent] = appendUniqueStrings(symbolMatches[ent], files...)
		}
	}

	pathMatches := make(map[string][]string)
	qualifiedMatches := make(map[string][]string)
	for rel, fi := range graph.FileIndex {
		normPath := strings.ToLower(filepath.ToSlash(rel))
		base := strings.ToLower(filepath.Base(normPath))
		stem := strings.TrimSuffix(base, filepath.Ext(base))
		for _, key := range []string{normPath, base, stem} {
			for _, ent := range pathTargets[key] {
				pathMatches[ent] = appendUniqueStrings(pathMatches[ent], rel)
			}
		}
		if len(qualifiedTargets) == 0 || fi == nil {
			continue
		}
		for _, sym := range fi.Symbols {
			qualified := sym.Name
			switch {
			case sym.Receiver != "":
				qualified = sym.Receiver + "." + sym.Name
			case sym.Parent != "":
				qualified = sym.Parent + "." + sym.Name
			}
			for _, ent := range qualifiedTargets[strings.ToLower(qualified)] {
				qualifiedMatches[ent] = appendUniqueStrings(qualifiedMatches[ent], rel)
			}
		}
	}
	for ent := range symbolMatches {
		sort.Strings(symbolMatches[ent])
	}
	for ent := range pathMatches {
		sort.Strings(pathMatches[ent])
	}
	for ent := range qualifiedMatches {
		sort.Strings(qualifiedMatches[ent])
	}

	out := make(map[string]exactEntityAnchor)
	add := func(paths []string, rank int, hit string) {
		if len(paths) != 1 {
			return
		}
		path := paths[0]
		if cur, ok := out[path]; ok && cur.Rank >= rank {
			return
		}
		out[path] = exactEntityAnchor{Rank: rank, Hit: hit}
	}
	for _, target := range targets {
		if target.qualified {
			add(qualifiedMatches[target.raw], 3, "qualified_symbol_exact")
		}
		add(symbolMatches[target.raw], 2, "symbol_exact")
		add(pathMatches[target.raw], 2, "path_exact")
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func uniqueSymbolDefFiles(defs []*repomap.Symbol) []string {
	if len(defs) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(defs))
	var files []string
	for _, def := range defs {
		if def == nil || def.File == "" || seen[def.File] {
			continue
		}
		seen[def.File] = true
		files = append(files, def.File)
	}
	sort.Strings(files)
	return files
}

func appendUniqueStrings(dst []string, values ...string) []string {
	if len(values) == 0 {
		return dst
	}
	seen := make(map[string]bool, len(dst)+len(values))
	for _, v := range dst {
		seen[v] = true
	}
	for _, v := range values {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		dst = append(dst, v)
	}
	return dst
}

func exactQualifiedSymbolFiles(graph *repomap.Graph, entity string) []string {
	if graph == nil || entity == "" {
		return nil
	}
	var files []string
	seen := make(map[string]bool)
	for path, fi := range graph.FileIndex {
		if fi == nil {
			continue
		}
		for _, sym := range fi.Symbols {
			qualified := sym.Name
			switch {
			case sym.Receiver != "":
				qualified = sym.Receiver + "." + sym.Name
			case sym.Parent != "":
				qualified = sym.Parent + "." + sym.Name
			}
			if !strings.EqualFold(qualified, entity) {
				continue
			}
			if !seen[path] {
				seen[path] = true
				files = append(files, path)
			}
		}
	}
	sort.Strings(files)
	return files
}

func exactSymbolFiles(graph *repomap.Graph, entity string) []string {
	if graph == nil || entity == "" {
		return nil
	}
	var defs []*repomap.Symbol
	if exact := graph.SymbolDefs[entity]; len(exact) > 0 {
		defs = exact
	} else {
		for name, candidate := range graph.SymbolDefs {
			if strings.EqualFold(name, entity) {
				defs = candidate
				break
			}
		}
	}
	if len(defs) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	var files []string
	for _, def := range defs {
		if def == nil || def.File == "" || seen[def.File] {
			continue
		}
		seen[def.File] = true
		files = append(files, def.File)
	}
	sort.Strings(files)
	return files
}

func exactPathFiles(graph *repomap.Graph, entity string) []string {
	if graph == nil || entity == "" {
		return nil
	}
	ent := strings.ToLower(filepath.ToSlash(strings.TrimSpace(entity)))
	var files []string
	for path := range graph.FileIndex {
		normPath := strings.ToLower(filepath.ToSlash(path))
		base := strings.ToLower(filepath.Base(normPath))
		stem := strings.TrimSuffix(base, filepath.Ext(base))
		switch {
		case normPath == ent, base == ent, stem == ent:
			files = append(files, path)
		}
	}
	sort.Strings(files)
	return files
}

// repoMapRank uses the repo_map graph to rank files by structural relevance.
// Only returns files that matched the query (QueryScores > 0), so
// infrastructure files with high structural scores but no query relevance
// are excluded. Also returns the graph for symbol extraction.
//
// P4-cross-sub-repo (Sc 5, 2026-05-08): when mgHandle resolves to a
// multi-repo MultiGraph (mg.IsSingle()==false), aggregate scores
// across every active sub-repo. Per-sub-repo scores are collected
// from each sub-repo's QueryScores/Scores maps and the path keys
// are rewritten to path-from-parent form (sub-repo prefix
// prepended) so cross-sub-repo files don't collide on names like
// "main.go" or "lib.rs". The returned *Graph is the routed sub-repo
// (best-effort primary) so downstream typed lookups (graph.SymbolDefs,
// FileIndex direct access) keep their pre-multi-repo semantics for
// the routed sub-repo. Cross-sub-repo Symbol queries should go
// through ctx.MultiGraph.Oracle() / .LookupSymbol — the carrier is
// also preserved on keywordSearchResult.MultiGraph for consumers
// that want it.
func repoMapRank(keywords []string, entities []string, repoRoot string, mgHandle any, searchGraph any, pendingSubRepos []string) (scores map[string]float64, graph *repomap.Graph) {
	terms := make([]string, 0, len(keywords)+len(entities))
	seen := make(map[string]bool, len(keywords)+len(entities))
	for _, term := range append(append([]string(nil), keywords...), entities...) {
		trimmed := strings.TrimSpace(term)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if seen[key] {
			continue
		}
		seen[key] = true
		terms = append(terms, trimmed)
	}
	query := strings.Join(terms, " ")

	// Multi-repo aggregation path: when MultiGraph carrier is wired
	// AND it's truly multi-repo (cap > 1, ≥ 2 active sub-repos),
	// merge QueryScores/Scores across active sub-repos.
	if mg := repomap.MultiGraphFromContext(&types.BusContext{MultiGraph: mgHandle}); mg != nil && !mg.IsSingle() {
		// Make sure every active sub-repo's graph has been built
		// against this query. Routing fold has already EnsureMany'd
		// the active set at Run entry; here we just iterate what's
		// resident.
		active := mg.AllGraphs()
		if len(active) > 0 {
			// Active-set filter: skip sub-repos the routing fold left
			// inactive. Shares the predicate with the FS-tool gate,
			// chain_promotion, and the symbol resolver via
			// types.PathInsidePendingSubRepo (single source of truth).
			scores = make(map[string]float64)
			subMap := make(map[string]bool, len(active))
			topo := mg.Topology()
			subRepoMap := map[string]string{}
			if topo != nil {
				for i := range topo.Repos {
					subRepoMap[topo.Repos[i].Slug] = topo.Repos[i].RootRel
				}
			}
			for slug, g := range active {
				if g == nil {
					continue
				}
				rootRel := subRepoMap[slug]
				if types.PathInsidePendingSubRepo(rootRel, pendingSubRepos) {
					logging.Debug("[keyword_search] repo_map (multi-repo): skipping pending sub-repo slug=%s rootRel=%s", slug, rootRel)
					continue
				}
				for path, qScore := range g.QueryScores {
					if qScore > 0 {
						prefixed := path
						if rootRel != "" && rootRel != "." {
							prefixed = rootRel + "/" + path
						}
						scores[prefixed] = g.Scores[path]
					}
				}
				subMap[slug] = true
			}
			// Pick the routed sub-repo's *Graph for the second return
			// (downstream typed access). Prefer the first active
			// (LRU MRU); fall back to mg.Single() if that fails.
			for _, g := range active {
				if g != nil {
					graph = g
					break
				}
			}
			logging.Debug("[keyword_search] repo_map (multi-repo): %d files matched query across %d sub-repo(s)", len(scores), len(active))
			return scores, graph
		}
	}

	// Single-graph (legacy / single-repo / fallback) path.
	var err error
	graph, err = repomap.GraphFromBusContextOrLoad(&types.BusContext{
		RepoRoot:    repoRoot,
		MultiGraph:  mgHandle,
		SearchGraph: searchGraph,
	}, repoRoot, query)
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

type keywordSearchRootScope struct {
	RootAbs string
	RootRel string
}

type keywordPathMatch struct {
	path      string
	matchType string
}

type keywordGrepResult struct {
	paths     []string
	matchType string
}

// grepIDFSearch runs grep/rg for each keyword and weights matches by IDF
// (inverse document frequency). Keywords matching fewer files are more
// informative and contribute more to a file's score.
//
// When ripgrep is available, all keywords are searched in a single rg
// call using multiple -e patterns, reducing process spawns from N*2 to 1.
func grepIDFSearch(keywords []string, repoRoot string) (scores map[string]float64, hits map[string]map[string]string) {
	return grepIDFSearchAcrossRoots(keywords, []keywordSearchRootScope{{RootAbs: repoRoot}})
}

func grepIDFSearchForOptions(keywords []string, repoRoot string, opts keywordSearchOptions) (scores map[string]float64, hits map[string]map[string]string) {
	return grepIDFSearchAcrossRoots(keywords, keywordSearchActiveRoots(repoRoot, opts.MultiGraph, opts.PendingSubRepos))
}

func keywordSearchActiveRoots(repoRoot string, mgHandle any, pendingSubRepos []string) []keywordSearchRootScope {
	if strings.TrimSpace(repoRoot) == "" {
		return nil
	}
	fallback := []keywordSearchRootScope{{RootAbs: repoRoot}}
	mg := repomap.MultiGraphFromContext(&types.BusContext{MultiGraph: mgHandle})
	if mg == nil || mg.IsSingle() {
		return fallback
	}
	topo := mg.Topology()
	if topo == nil {
		return fallback
	}
	active := mg.AllGraphs()
	if len(active) == 0 {
		return fallback
	}
	roots := make([]keywordSearchRootScope, 0, len(active))
	for i := range topo.Repos {
		sr := topo.Repos[i]
		if _, ok := active[sr.Slug]; !ok {
			continue
		}
		if types.PathInsidePendingSubRepo(sr.RootRel, pendingSubRepos) {
			continue
		}
		if strings.TrimSpace(sr.RootAbs) == "" {
			continue
		}
		roots = append(roots, keywordSearchRootScope{
			RootAbs: sr.RootAbs,
			RootRel: sr.RootRel,
		})
	}
	if len(roots) == 0 {
		return fallback
	}
	sort.Slice(roots, func(i, j int) bool {
		return roots[i].RootRel < roots[j].RootRel
	})
	if len(roots) > 1 {
		logging.Debug("[keyword_search] grep scoped to %d active sub-repo roots", len(roots))
	}
	return roots
}

func grepIDFSearchAcrossRoots(keywords []string, roots []keywordSearchRootScope) (scores map[string]float64, hits map[string]map[string]string) {
	scores = make(map[string]float64)
	hits = make(map[string]map[string]string)
	if len(keywords) == 0 || len(roots) == 0 {
		return scores, hits
	}

	// Count total source files for IDF denominator.
	totalFiles := 0
	for _, root := range roots {
		totalFiles += countSourceFiles(root.RootAbs)
	}
	if totalFiles < 1 {
		totalFiles = 100 // fallback
	}

	kwResults := make(map[string][]keywordPathMatch, len(keywords))
	var kwResultsMu sync.Mutex
	addResults := func(root keywordSearchRootScope, local map[string]keywordGrepResult) {
		kwResultsMu.Lock()
		defer kwResultsMu.Unlock()
		seen := make(map[keywordPathMatch]bool)
		for kw, kr := range local {
			for _, p := range kr.paths {
				rel := normalizeSearchPath(p, root.RootAbs)
				rel = prefixKeywordSearchPath(root.RootRel, rel)
				match := keywordPathMatch{path: rel, matchType: kr.matchType}
				if seen[match] {
					continue
				}
				seen[match] = true
				kwResults[kw] = append(kwResults[kw], match)
			}
		}
	}

	var wg sync.WaitGroup
	for _, root := range roots {
		root := root
		if strings.TrimSpace(root.RootAbs) == "" {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			addResults(root, grepKeywordMatchesInRoot(keywords, root.RootAbs))
		}()
	}
	wg.Wait()

	// Score files by IDF.
	for _, kw := range keywords {
		matches := kwResults[kw]
		if len(matches) == 0 {
			continue
		}

		df := float64(len(matches))
		idf := math.Log2(float64(totalFiles)/df) + 1.0
		kwLower := strings.ToLower(kw)

		for _, match := range matches {
			p := match.path
			if isNoisePath(p) {
				continue
			}

			fileScore := idf * fileTypeWeight(p)
			matchType := match.matchType

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

func grepKeywordMatchesInRoot(keywords []string, repoRoot string) map[string]keywordGrepResult {
	kwResults := make(map[string]keywordGrepResult, len(keywords))
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
				kwResults[kw] = keywordGrepResult{paths: paths, matchType: mt}
			}
		}
		return kwResults
	}

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
				kwResults[kw] = keywordGrepResult{paths: paths, matchType: matchType}
				mu.Unlock()
			}
		}(kw)
	}
	wg.Wait()
	return kwResults
}

func prefixKeywordSearchPath(rootRel, rel string) string {
	rel = strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(rel)), "./")
	rootRel = strings.TrimSuffix(strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(rootRel)), "./"), "/")
	if rootRel == "" || rootRel == "." {
		return rel
	}
	if rel == "" || rel == "." {
		return rootRel
	}
	if rel == rootRel || strings.HasPrefix(rel, rootRel+"/") {
		return rel
	}
	return rootRel + "/" + rel
}

// sourceFileCount caches the result of countSourceFiles per root so
// multi-repo pre-scans do not rescan `rg --files` for the same sub-repo
// and do not reuse a count from a different workspace.
var (
	sourceCountMu     sync.Mutex
	sourceCountByRoot = make(map[string]int)
)

// countSourceFiles returns an approximate count of source files in the repo.
// Uses rg --files when available (fast, respects .gitignore), falls back to
// Go-native filepath.WalkDir with the shared SearchDirFilter policy.
// Result is cached for the process lifetime.
func countSourceFiles(repoRoot string) int {
	key := canonicalKeywordSearchRoot(repoRoot)
	sourceCountMu.Lock()
	if cached, ok := sourceCountByRoot[key]; ok {
		sourceCountMu.Unlock()
		if cached < 1 {
			return 100
		}
		return cached
	}
	sourceCountMu.Unlock()

	count := countSourceFilesOnce(repoRoot)
	sourceCountMu.Lock()
	sourceCountByRoot[key] = count
	sourceCountMu.Unlock()
	if count < 1 {
		return 100 // fallback
	}
	return count
}

func canonicalKeywordSearchRoot(repoRoot string) string {
	if repoRoot == "" {
		return ""
	}
	abs, err := filepath.Abs(repoRoot)
	if err != nil {
		return filepath.Clean(repoRoot)
	}
	return filepath.Clean(abs)
}

func countSourceFilesOnce(repoRoot string) int {
	dirFilter := tool.NewSearchDirFilter(repoRoot, repoRoot)
	// Prefer rg --files: fast, auto-excludes .gitignore entries.
	if tool.UseRipgrep() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		args := []string{"--files"}
		for _, glob := range dirFilter.RipgrepGlobs() {
			args = append(args, "--glob", glob)
		}
		args = append(args, repoRoot)
		cmd := exec.CommandContext(ctx, tool.SearchExecutable(), args...)
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		if err := cmd.Run(); err == nil {
			return len(filterKeywordSearchPaths(splitLines(stdout.String()), repoRoot, dirFilter))
		}
	}
	// Go-native fallback using the unified exclude list.
	count := 0
	filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if rel, relErr := filepath.Rel(repoRoot, path); relErr == nil {
			rel = filepath.Clean(rel)
			if rel != "." && dirFilter.ExcludesRepoRelativePath(strings.ReplaceAll(rel, `\`, `/`)) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
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
	b.WriteString("### Pre-scanned Navigation Candidates\n\n")
	b.WriteString("These files were found by keyword / repo_map navigation. Treat them as candidate routes, not read obligations. ")
	b.WriteString("Ranked by graduated keyword search (exact match → case-insensitive → stem). ")
	b.WriteString("Higher scores mean more keywords matched at higher precision levels, but support/tool/test/helper hits can still be collateral. ")
	b.WriteString("Inspect a file only when it visibly matches the user's requested answer path; otherwise use repo_map / focused grep to find a better path.\n\n")
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

func filterKeywordSearchPaths(paths []string, repoRoot string, dirFilter tool.SearchDirFilter) []string {
	if len(paths) == 0 {
		return nil
	}
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if dirFilter.ExcludesRepoRelativePath(normalizeSearchPath(path, repoRoot)) {
			continue
		}
		out = append(out, path)
	}
	return out
}

// grepFiles runs grep/rg on the repo and returns matching file paths.
// All commands are guarded by searchTimeout to prevent hangs.
// When neither ripgrep nor grep is on PATH, falls through to the
// Go-native regex scanner so breadth scans keep working on minimal
// container images (see tool.UseNativeGrep).
func grepFiles(pattern, repoRoot string, ignoreCase bool) []string {
	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()
	dirFilter := tool.NewSearchDirFilter(repoRoot, repoRoot)

	if tool.UseNativeGrep() {
		res, err := tool.NativeGrep(ctx, tool.NativeGrepOpts{
			Pattern:     pattern,
			Root:        repoRoot,
			IgnoreCase:  ignoreCase,
			FilesOnly:   true,
			ExcludeDirs: dirFilter.AnyLevelPatterns(),
			ShouldSkip: func(path string, d fs.DirEntry) bool {
				rel, err := filepath.Rel(repoRoot, path)
				if err != nil {
					return false
				}
				rel = filepath.Clean(rel)
				if rel == "." {
					return false
				}
				return dirFilter.ExcludesRepoRelativePath(strings.ReplaceAll(rel, `\`, `/`))
			},
		})
		if err != nil {
			return nil
		}
		return filterKeywordSearchPaths(splitLines(res.Output), repoRoot, dirFilter)
	}

	var cmd *exec.Cmd

	if tool.UseRipgrep() {
		args := []string{"-l"}
		if ignoreCase {
			args = append(args, "-i")
		} else {
			args = append(args, "--case-sensitive")
		}
		for _, glob := range dirFilter.RipgrepGlobs() {
			args = append(args, "--glob", glob)
		}
		args = append(args, pattern, repoRoot)
		cmd = exec.CommandContext(ctx, tool.SearchExecutable(), args...)
	} else {
		args := []string{"-rlEI"}
		if ignoreCase {
			args = []string{"-rlEIi"}
		}
		for _, dir := range dirFilter.AnyLevelPatterns() {
			args = append(args, "--exclude-dir="+dir)
		}
		args = append(args, pattern, repoRoot)
		cmd = exec.CommandContext(ctx, tool.SearchExecutable(), args...)
	}

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil // exit 1 = no matches, timeout, or error — either way no results
	}
	return filterKeywordSearchPaths(splitLines(stdout.String()), repoRoot, dirFilter)
}

// grepLiteralFilesByKeywords returns per-keyword file hits for literal text.
// It is used by soft navigation seeders that need DISTINCT-token coverage:
// one broad regex search per token is both slower and semantically wrong for
// user/model surfaces such as config keys that may contain regex punctuation.
func grepLiteralFilesByKeywords(keywords []string, repoRoot string, ignoreCase bool) map[string][]string {
	keywords = dedupStringList(keywords)
	if len(keywords) == 0 || strings.TrimSpace(repoRoot) == "" {
		return nil
	}
	if tool.UseRipgrep() {
		return rgBatchFixedFiles(keywords, repoRoot, ignoreCase)
	}

	type result struct {
		keyword string
		paths   []string
	}
	const maxConcurrentFallbackSearches = 4
	sem := make(chan struct{}, maxConcurrentFallbackSearches)
	ch := make(chan result, len(keywords))
	var wg sync.WaitGroup
	for _, kw := range keywords {
		kw := kw
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			paths := grepLiteralFiles(kw, repoRoot, ignoreCase)
			<-sem
			if len(paths) > 0 {
				ch <- result{keyword: kw, paths: paths}
			}
		}()
	}
	wg.Wait()
	close(ch)

	out := make(map[string][]string, len(keywords))
	for r := range ch {
		out[r.keyword] = r.paths
	}
	return out
}

func grepLiteralFiles(literal, repoRoot string, ignoreCase bool) []string {
	literal = strings.TrimSpace(literal)
	if literal == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()
	dirFilter := tool.NewSearchDirFilter(repoRoot, repoRoot)

	if tool.UseNativeGrep() {
		res, err := tool.NativeGrep(ctx, tool.NativeGrepOpts{
			Pattern:     literal,
			Root:        repoRoot,
			IgnoreCase:  ignoreCase,
			FixedString: true,
			FilesOnly:   true,
			ExcludeDirs: dirFilter.AnyLevelPatterns(),
			ShouldSkip: func(path string, d fs.DirEntry) bool {
				rel, err := filepath.Rel(repoRoot, path)
				if err != nil {
					return false
				}
				rel = filepath.Clean(rel)
				if rel == "." {
					return false
				}
				return dirFilter.ExcludesRepoRelativePath(strings.ReplaceAll(rel, `\`, `/`))
			},
		})
		if err != nil {
			return nil
		}
		return filterKeywordSearchPaths(splitLines(res.Output), repoRoot, dirFilter)
	}

	var args []string
	if tool.UseRipgrep() {
		args = []string{"-F", "-l"}
		if ignoreCase {
			args = append(args, "-i")
		} else {
			args = append(args, "--case-sensitive")
		}
		for _, glob := range dirFilter.RipgrepGlobs() {
			args = append(args, "--glob", glob)
		}
		args = append(args, "-e", literal, repoRoot)
	} else {
		args = []string{"-r", "-l", "-F", "-I"}
		if ignoreCase {
			args = append(args, "-i")
		}
		for _, dir := range dirFilter.AnyLevelPatterns() {
			args = append(args, "--exclude-dir="+dir)
		}
		args = append(args, "-e", literal, repoRoot)
	}

	cmd := exec.CommandContext(ctx, tool.SearchExecutable(), args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil
	}
	return filterKeywordSearchPaths(splitLines(stdout.String()), repoRoot, dirFilter)
}

func rgBatchFixedFiles(keywords []string, repoRoot string, ignoreCase bool) map[string][]string {
	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()
	dirFilter := tool.NewSearchDirFilter(repoRoot, repoRoot)

	args := []string{"--json", "--fixed-strings"}
	if ignoreCase {
		args = append(args, "-i")
	} else {
		args = append(args, "--case-sensitive")
	}
	for _, glob := range dirFilter.RipgrepGlobs() {
		args = append(args, "--glob", glob)
	}
	for _, kw := range keywords {
		kw = strings.TrimSpace(kw)
		if kw != "" {
			args = append(args, "-e", kw)
		}
	}
	args = append(args, repoRoot)

	cmd := exec.CommandContext(ctx, tool.SearchExecutable(), args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil
	}

	kwExact := make(map[string]string, len(keywords))
	kwLower := make(map[string]string, len(keywords))
	for _, kw := range keywords {
		kw = strings.TrimSpace(kw)
		if kw == "" {
			continue
		}
		if ignoreCase {
			kwLower[strings.ToLower(kw)] = kw
		} else {
			kwExact[kw] = kw
		}
	}

	type fileKw struct {
		file, kw string
	}
	seen := make(map[fileKw]bool)
	result := make(map[string][]string, len(keywords))
	for _, line := range splitLines(stdout.String()) {
		if !strings.Contains(line, `"type":"match"`) {
			continue
		}
		filePath, matchTexts := parseRgMatchLine(line)
		if filePath == "" {
			continue
		}
		if dirFilter.ExcludesRepoRelativePath(normalizeSearchPath(filePath, repoRoot)) {
			continue
		}
		for _, mt := range matchTexts {
			origKw := ""
			if ignoreCase {
				origKw = kwLower[strings.ToLower(mt)]
			} else {
				origKw = kwExact[mt]
			}
			if origKw == "" {
				continue
			}
			key := fileKw{file: filePath, kw: origKw}
			if seen[key] {
				continue
			}
			seen[key] = true
			result[origKw] = append(result[origKw], filePath)
		}
	}
	return result
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
	dirFilter := tool.NewSearchDirFilter(repoRoot, repoRoot)

	args := []string{"--json", "--smart-case"}
	for _, glob := range dirFilter.RipgrepGlobs() {
		args = append(args, "--glob", glob)
	}
	for _, kw := range keywords {
		args = append(args, "-e", kw)
	}
	args = append(args, repoRoot)

	cmd := exec.CommandContext(ctx, tool.SearchExecutable(), args...)
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
		if dirFilter.ExcludesRepoRelativePath(normalizeSearchPath(filePath, repoRoot)) {
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

// findFilesByName locates files whose basename contains the keyword.
// Tries `find` first for speed on large trees, then falls back to a
// Go-native walk so the same code path works on Windows (no POSIX
// find) and inside stripped container images.
// Guarded by searchTimeout to prevent hangs on large directory trees.
func findFilesByName(keyword, repoRoot string, ignoreCase bool) []string {
	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()
	dirFilter := tool.NewSearchDirFilter(repoRoot, repoRoot)

	if _, err := exec.LookPath("find"); err == nil {
		args := []string{repoRoot}
		for _, dir := range dirFilter.AnyLevelPatterns() {
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
		if err := cmd.Run(); err == nil {
			lines := filterKeywordSearchPaths(splitLines(stdout.String()), repoRoot, dirFilter)
			if len(lines) > 0 {
				return lines
			}
		}
		// find failed or produced no output — fall through to the Go
		// walker. The walker is also the sole path on Windows where
		// POSIX `find` is usually absent (Windows ships a different
		// find.exe that does not speak -path / -name).
	}

	needle := keyword
	if ignoreCase {
		needle = strings.ToLower(needle)
	}
	var out []string
	_ = filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			if rel, relErr := filepath.Rel(repoRoot, path); relErr == nil {
				rel = filepath.Clean(rel)
				if rel != "." && dirFilter.ExcludesRepoRelativePath(strings.ReplaceAll(rel, `\`, `/`)) {
					return filepath.SkipDir
				}
			}
			if tool.DirNameMatchesExcludePattern(d.Name(), dirFilter.AnyLevelPatterns()) {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if ignoreCase {
			name = strings.ToLower(name)
		}
		if strings.Contains(name, needle) {
			out = append(out, path)
		}
		return nil
	})
	return filterKeywordSearchPaths(out, repoRoot, dirFilter)
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
	path = strings.ReplaceAll(filepath.ToSlash(strings.TrimSpace(path)), "\\", "/")
	abs, err := filepath.Abs(repoRoot)
	if err == nil {
		abs = strings.ReplaceAll(filepath.ToSlash(abs), "\\", "/")
		path = strings.TrimPrefix(path, strings.TrimRight(abs, "/")+"/")
	}
	repoRoot = strings.ReplaceAll(filepath.ToSlash(strings.TrimSpace(repoRoot)), "\\", "/")
	path = strings.TrimPrefix(path, strings.TrimRight(repoRoot, "/")+"/")
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
