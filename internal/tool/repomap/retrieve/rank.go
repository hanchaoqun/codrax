package retrieve

import (
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	coretypes "github.com/hanchaoqun/codrax/internal/types"
)

// Ranking weights.
const (
	wReference  = 1.0
	wInbound    = 2.0
	wExported   = 1.5
	wEntrypoint = 3.0
	wRecent     = 2.0
	wQueryMatch = 4.0

	// queryStructuralTieBreakWeight caps structural centrality when a
	// query is present. Query-bearing task maps are navigation surfaces:
	// exact request-token relevance must be the primary rank signal, while
	// graph centrality is useful only as a tie-breaker. Multiplying the
	// whole structural score by a query hit makes high-centrality generic
	// files with broad words ("map", "array", "handler") outrank precise
	// request files in large repos.
	queryStructuralTieBreakWeight = 2.0
	queryNoMatchStructuralWeight  = 0.25
)

// Ranking is the per-query rank output. Query-biased scores are request-local:
// callers that need a transient view should use RankGraphScores instead of
// mutating Graph.Scores / Graph.QueryScores on the shared in-memory graph.
type Ranking struct {
	Scores      map[string]float64
	QueryScores map[string]float64
}

var graphRankMu sync.Mutex

// RankGraph scores every file in the graph by importance.
// If query is non-empty, symbols/files matching the query get a bonus.
//
// The ranking uses two mechanisms to prevent utility files (tool
// registries, loggers, etc.) from drowning out domain-relevant files:
//
//  1. Fan-out dampening: files whose symbols are referenced by many
//     other files have their structural score dampened. A file
//     referenced by 30% or more of all files is clearly infrastructure,
//     not domain logic. The dampener uses a sqrt-based curve so
//     moderate fan-out is barely affected.
//
//  2. Query boost: when a query is provided, files that match the
//     query receive a multiplicative boost to their total score, not
//     just an additive bonus. This lets a domain file with modest
//     structural importance overtake a utility file with high
//     structural importance when the query clearly targets the domain.
func RankGraph(g *types.Graph, query string) {
	if g == nil {
		return
	}
	ranking := RankGraphScores(g, query)
	graphRankMu.Lock()
	g.Scores = ranking.Scores
	g.QueryScores = ranking.QueryScores
	graphRankMu.Unlock()
}

// RankGraphScores computes query-biased ranking into fresh maps and never
// mutates Graph. This is the safe path for concurrent repo_map rendering where
// multiple tool calls can rank the same graph with different queries.
func RankGraphScores(g *types.Graph, query string) Ranking {
	if g == nil {
		return Ranking{}
	}
	scores := make(map[string]float64, len(g.Files))
	queryScores := make(map[string]float64)
	idx := g.RankIndex
	if idx == nil {
		idx = types.BuildRankIndex(g)
	}
	recentFiles := getRecentlyChanged(g.Root, 50)
	entrypoints := idx.Entrypoints
	queryTokens := TokenizeQuery(query)
	querySalience := queryTokenSalience(g, queryTokens)

	// score each file
	for _, fi := range g.Files {
		score := 0.0

		// sum reference/call counts for symbols defined in this file.
		// Resolved-by-ID call counts are credited directly (no
		// ambiguity divisor because they already picked one def);
		// name-keyed fallback counters are divided by the symbol's
		// name-level ambiguity as before.
		for _, sym := range fi.Symbols {
			ambiguity := float64(idx.SymbolDefCount[sym.Name])
			if ambiguity < 1 {
				ambiguity = 1
			}
			if sym.ID != "" {
				score += float64(idx.CallCountByID[sym.ID]) * wInbound
				score += float64(idx.RefCountByID[sym.ID]) * wReference
			}
			score += float64(idx.RefCount[sym.Name]) * wReference / ambiguity
			score += float64(idx.CallCount[sym.Name]) * wInbound / ambiguity
			if sym.Exported {
				score += wExported
			}
		}

		// import fan-in: how many files import this file
		score += float64(len(g.ReverseImports[fi.RelPath])) * wInbound

		// entrypoint bonus
		if entrypoints[fi.RelPath] {
			score += wEntrypoint * 5
		}

		// recently changed bonus
		if recentFiles[fi.RelPath] {
			score += wRecent * 3
		}

		// Fan-out dampening: files imported by a large fraction of the
		// codebase are utility/infrastructure. Dampen their structural
		// score so they don't dominate query results. The curve is:
		//   fanout < 0.15 → no dampening (factor ≈ 1.0)
		//   fanout = 0.30 → factor ≈ 0.55
		//   fanout = 0.50 → factor ≈ 0.32
		//   fanout ≥ 0.80 → factor ≈ 0.15
		fanout := idx.FileFanout[fi.RelPath]
		if fanout > 0.15 {
			dampener := math.Sqrt(0.15 / fanout)
			score *= dampener
		}

		// Query match: query relevance is primary, structural centrality is
		// a bounded tie-breaker. Without this split, a single broad token hit
		// on a highly connected utility file can dominate a precise
		// request-local file in large, mixed-language repositories.
		if len(queryTokens) > 0 {
			qScore := queryMatchScoreWithTokens(fi, queryTokens, querySalience)
			if qScore > 0 {
				queryScores[fi.RelPath] = qScore
				score = qScore*wQueryMatch + math.Log1p(math.Max(score, 0))*queryStructuralTieBreakWeight
			} else {
				score = math.Log1p(math.Max(score, 0)) * queryNoMatchStructuralWeight
			}
		}

		// Parse-tier discount: ArkTS and Cangjie files may have been
		// produced by a fallback tier (regex-only salvage, path-only)
		// with lower confidence. Discount the final score so these
		// cannot outrank a Tier-1 sibling on identical evidence.
		// Red line L-Fallback-2 — no anti-incentive for broken parses.
		if fi.ParseTier >= 2 {
			score *= parseTierDiscount(fi.ParseTier)
		}

		// Commit 53 P5: parse-tier hard gate. When yaml
		// repomap_min_parse_tier > 0, files whose ParseTier exceeds
		// the floor are dropped to score=0 (effective rank-out)
		// instead of merely discounted. Default 0 = behaviour
		// byte-identical with pre-commit-53.
		score = applyParseTierFloor(score, fi.ParseTier)

		scores[fi.RelPath] = score
	}
	return Ranking{Scores: scores, QueryScores: queryScores}
}

// applyParseTierFloor zeroes the score when fi.ParseTier exceeds
// the active minParseTierFloor (commit 53 P5). Pure function for
// unit-testability — the floor toggle is verified at the gate
// boundary rather than via a full RankGraph fixture.
//
// Floor of 0 = disabled (score returned unchanged). Tier 0 in
// the input is treated as Tier 1 (primary parse) so an unset
// field doesn't trigger the gate.
func applyParseTierFloor(score float64, tier int) float64 {
	if minParseTierFloor <= 0 {
		return score
	}
	effectiveTier := tier
	if effectiveTier <= 0 {
		effectiveTier = 1
	}
	if effectiveTier > minParseTierFloor {
		return 0
	}
	return score
}

// minParseTierFloor is the active hard-gate floor (commit 53 P5).
// Mutated by SetMinParseTierFloor at startup. Default 0 disables
// the gate (only the soft TierDiscount applies).
var minParseTierFloor int

// SetMinParseTierFloor sets the active hard floor. Called from
// cmd/root.go when reading yaml repomap_min_parse_tier. Negative
// values clamp to 0 (disabled). Values 5+ behave as "drop
// nothing" (Tier never exceeds 4).
func SetMinParseTierFloor(tier int) {
	if tier < 0 {
		tier = 0
	}
	minParseTierFloor = tier
}

// parseTierDiscount returns the rank multiplier for a fallback
// tier. Mirrors index.TierDiscount (retrieve cannot import index
// without a cycle, so the constants are duplicated here — keep in
// sync or the rank test will flag the drift).
func parseTierDiscount(tier int) float64 {
	switch tier {
	case 0, 1:
		return 1.0
	case 2:
		return 0.85
	case 3:
		return 0.6
	default:
		return 0.3
	}
}

// TopFiles returns files sorted by importance score, limited to topN.
func TopFiles(g *types.Graph, topN int) []*types.FileInfo {
	if g == nil {
		return nil
	}
	graphRankMu.Lock()
	defer graphRankMu.Unlock()
	return TopFilesByScore(g, g.Scores, topN)
}

// TopFilesByScore returns files sorted by a caller-provided score map. It does
// not read or mutate Graph.Scores, so query-local renderers can stay fully
// concurrency-safe.
func TopFilesByScore(g *types.Graph, scores map[string]float64, topN int) []*types.FileInfo {
	if g == nil {
		return nil
	}
	sorted := make([]*types.FileInfo, len(g.Files))
	copy(sorted, g.Files)
	sort.Slice(sorted, func(i, j int) bool {
		return scores[sorted[i].RelPath] > scores[sorted[j].RelPath]
	})
	if topN > 0 && topN < len(sorted) {
		sorted = sorted[:topN]
	}
	return sorted
}

// getRecentlyChanged returns files modified in the last N commits.
func getRecentlyChanged(repoRoot string, n int) map[string]bool {
	if strings.TrimSpace(repoRoot) == "" {
		return nil
	}
	cmd, cancel := tool.NewGitCommand(nil, "-C", repoRoot, "log",
		"--pretty=format:", "--name-only", "-n", itoa(n))
	defer cancel()
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	recent := make(map[string]bool)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			recent[line] = true
		}
	}
	return recent
}

func detectEntrypoints(g *types.Graph) map[string]bool {
	return types.DetectEntrypoints(g)
}

// queryMatchScore scores a file by how well its RelPath, symbol
// names, and symbol doc comments match the query. The query is
// tokenized via the Phase 2b multilingual tokenizer
// (TokenizeQuery in query.go): primary tokens (weight 1.0) are the
// whole whitespace-separated lowered identifier, secondary sub-
// tokens (weight 0.5) are CamelCase / snake_case / kebab-case /
// dotted / slashed decompositions, and CJK runs are emitted as
// primary-weight bi-grams. Each token hit contributes its weight
// times the field multiplier (path=3, symbol=2, doc=1); total is
// capped at 20 to prevent a single file from dominating rank.
//
// Test files receive a 0.5 multiplier on the final score. They
// tend to share every symbol name with their target plus extra
// test helpers, so without the penalty a query like
// "finalizer answer shape translation" ranks finalizer_test.go
// ahead of finalizer.go. The penalty is applied as a final
// adjustment so path/symbol/doc matches still contribute and test
// files can still be selected when no implementation match
// exists.
func queryMatchScore(fi *types.FileInfo, query string) float64 {
	tokens := TokenizeQuery(query)
	return queryMatchScoreWithTokens(fi, tokens, nil)
}

func queryMatchScoreWithTokens(fi *types.FileInfo, tokens []QueryToken, salience map[string]float64) float64 {
	if len(tokens) == 0 {
		return 0.0
	}

	score := 0.0
	pathLower := strings.ToLower(fi.RelPath)
	for _, t := range tokens {
		if strings.Contains(pathLower, t.Text) {
			score += 3.0 * t.Weight * queryTokenSalienceWeight(t, salience)
		}
	}

	for _, sym := range fi.Symbols {
		nameLower := strings.ToLower(sym.Name)
		for _, t := range tokens {
			if strings.Contains(nameLower, t.Text) {
				score += 2.0 * t.Weight * queryTokenSalienceWeight(t, salience)
			}
		}
		if sym.Doc != "" {
			docLower := strings.ToLower(sym.Doc)
			for _, t := range tokens {
				if strings.Contains(docLower, t.Text) {
					score += 1.0 * t.Weight * queryTokenSalienceWeight(t, salience)
				}
			}
		}
	}

	if isTestFile(fi.RelPath) {
		score *= 0.5
	}

	return math.Min(score, 20.0)
}

func queryTokenSalience(g *types.Graph, tokens []QueryToken) map[string]float64 {
	if g == nil || len(tokens) == 0 || len(g.Files) == 0 {
		return nil
	}
	df := make(map[string]int, len(tokens))
	for _, fi := range g.Files {
		seen := make(map[string]bool, len(tokens))
		pathLower := strings.ToLower(fi.RelPath)
		for _, t := range tokens {
			if strings.Contains(pathLower, t.Text) {
				seen[t.Text] = true
			}
		}
		for _, sym := range fi.Symbols {
			nameLower := strings.ToLower(sym.Name)
			docLower := strings.ToLower(sym.Doc)
			for _, t := range tokens {
				if seen[t.Text] {
					continue
				}
				if strings.Contains(nameLower, t.Text) || (docLower != "" && strings.Contains(docLower, t.Text)) {
					seen[t.Text] = true
				}
			}
		}
		for text := range seen {
			df[text]++
		}
	}
	n := float64(len(g.Files))
	out := make(map[string]float64, len(tokens))
	for _, t := range tokens {
		if _, ok := out[t.Text]; ok {
			continue
		}
		count := float64(df[t.Text])
		if count <= 0 {
			out[t.Text] = 1.0
			continue
		}
		weight := math.Log((n+1.0)/(count+1.0)) + 0.35
		if weight < 0.35 {
			weight = 0.35
		}
		if weight > 2.5 {
			weight = 2.5
		}
		out[t.Text] = weight
	}
	return out
}

func queryTokenSalienceWeight(t QueryToken, salience map[string]float64) float64 {
	if len(salience) == 0 {
		return 1.0
	}
	if weight := salience[t.Text]; weight > 0 {
		return weight
	}
	return 1.0
}

// isTestFile reports whether the RelPath looks like a test / spec
// file under the conventions of the languages repomap covers:
// Go (`_test.go`), Python (`test_*.py`, `*_test.py`), Rust
// (`tests/*.rs`), Java (`*Test.java`, `*Tests.java`), JS/TS
// (`*.test.{js,ts,tsx,jsx}`, `*.spec.{js,ts,tsx,jsx}`). The match
// is deliberately conservative — only unambiguous test-file shapes
// qualify, so files that happen to contain "test" as a substring
// are unaffected.
func isTestFile(relPath string) bool {
	return coretypes.LooksLikeTestFilePath(relPath)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	if neg {
		s = "-" + s
	}
	return s
}
