package retrieve

import (
	"math"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// Ranking weights.
const (
	wReference  = 1.0
	wInbound    = 2.0
	wExported   = 1.5
	wEntrypoint = 3.0
	wRecent     = 2.0
	wQueryMatch = 4.0

	// queryBoostMultiplier is applied to the entire score of files
	// that match the query. Without this, utility files with high
	// structural scores (builtin.go, logger.go) always dominate even
	// when the query clearly targets specific domain files.
	queryBoostMultiplier = 3.0
)

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
	recentFiles := getRecentlyChanged(g.Root, 50)
	entrypoints := detectEntrypoints(g)
	totalFiles := float64(len(g.Files))

	// Phase 1 receiver-aware accounting. For every call / reference
	// relation, try to resolve it to a canonical types.Symbol via
	// MethodIndex. Resolved edges credit the one target types.Symbol by
	// ID. Unresolved edges (unknown receiver, external packages)
	// fall back to name-keyed counters that still get divided by
	// ambiguity at scoring time so the noise doesn't dominate.
	callCountByID := make(map[types.SymbolID]int)
	refCountByID := make(map[types.SymbolID]int)
	callCount := make(map[string]int)
	refCount := make(map[string]int)
	for _, fi := range g.Files {
		for _, rel := range fi.Relations {
			switch rel.Kind {
			case "call":
				if s := g.ResolveCallTarget(fi, rel); s != nil && s.ID != "" {
					callCountByID[s.ID]++
				} else {
					callCount[rel.To]++
				}
			case "type_usage", "reference":
				refCount[rel.To]++
			}
		}
	}

	// types.Symbol name ambiguity: count how many files define the same
	// symbol name. Common interface methods (Name, Execute, String,
	// Error, etc.) are defined in many files. Used ONLY for the
	// name-keyed fallback counters — resolved-by-ID counters already
	// pick a single definition and do not need damping.
	symDefCount := make(map[string]int)
	for _, fi := range g.Files {
		localNames := make(map[string]bool)
		for _, sym := range fi.Symbols {
			if !localNames[sym.Name] {
				symDefCount[sym.Name]++
				localNames[sym.Name] = true
			}
		}
	}

	// Compute per-file fan-out using the resolved call edges so a
	// file with 7 tools implementing Name() no longer gets credited
	// with the global call count for "Name". The symbol→file index
	// is keyed by types.SymbolID for resolved edges; name-keyed fallback
	// reproduces the legacy behavior for unresolved edges.
	symbolIDToFile := make(map[types.SymbolID]string, len(g.Files)*10)
	symbolToFile := make(map[string]string, len(g.Files)*10)
	for _, fi := range g.Files {
		for idx := range fi.Symbols {
			sym := &fi.Symbols[idx]
			if sym.ID != "" {
				symbolIDToFile[sym.ID] = fi.RelPath
			}
			symbolToFile[sym.Name] = fi.RelPath
		}
	}
	// Count distinct files that reference each file's symbols.
	fileReferrers := make(map[string]map[string]bool)
	for _, fi := range g.Files {
		for _, rel := range fi.Relations {
			var defFile string
			if s := g.ResolveCallTarget(fi, rel); s != nil && s.ID != "" {
				defFile = symbolIDToFile[s.ID]
			} else if f, ok := symbolToFile[rel.To]; ok {
				defFile = f
			}
			if defFile == "" || defFile == fi.RelPath {
				continue
			}
			if fileReferrers[defFile] == nil {
				fileReferrers[defFile] = make(map[string]bool)
			}
			fileReferrers[defFile][fi.RelPath] = true
		}
	}
	fileFanout := make(map[string]float64, len(g.Files))
	for path, refs := range fileReferrers {
		if totalFiles > 0 {
			fileFanout[path] = float64(len(refs)) / totalFiles
		}
	}

	// score each file
	for _, fi := range g.Files {
		score := 0.0

		// sum reference/call counts for symbols defined in this file.
		// Resolved-by-ID call counts are credited directly (no
		// ambiguity divisor because they already picked one def);
		// name-keyed fallback counters are divided by the symbol's
		// name-level ambiguity as before.
		for _, sym := range fi.Symbols {
			ambiguity := float64(symDefCount[sym.Name])
			if ambiguity < 1 {
				ambiguity = 1
			}
			if sym.ID != "" {
				score += float64(callCountByID[sym.ID]) * wInbound
				score += float64(refCountByID[sym.ID]) * wReference
			}
			score += float64(refCount[sym.Name]) * wReference / ambiguity
			score += float64(callCount[sym.Name]) * wInbound / ambiguity
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
		fanout := fileFanout[fi.RelPath]
		if fanout > 0.15 {
			dampener := math.Sqrt(0.15 / fanout)
			score *= dampener
		}

		// query match: additive bonus + multiplicative boost
		if query != "" {
			qScore := queryMatchScore(fi, query)
			if qScore > 0 {
				g.QueryScores[fi.RelPath] = qScore
			}
			score += qScore * wQueryMatch
			if qScore > 0 {
				// Multiplicative boost: files matching the query get
				// their total score amplified. This ensures that a
				// query-relevant file with moderate structural score
				// can overtake a query-irrelevant utility file.
				score *= queryBoostMultiplier
			}
		}

		g.Scores[fi.RelPath] = score
	}
}

// TopFiles returns files sorted by importance score, limited to topN.
func TopFiles(g *types.Graph, topN int) []*types.FileInfo {
	sorted := make([]*types.FileInfo, len(g.Files))
	copy(sorted, g.Files)
	sort.Slice(sorted, func(i, j int) bool {
		return g.Scores[sorted[i].RelPath] > g.Scores[sorted[j].RelPath]
	})
	if topN > 0 && topN < len(sorted) {
		sorted = sorted[:topN]
	}
	return sorted
}

// getRecentlyChanged returns files modified in the last N commits.
func getRecentlyChanged(repoRoot string, n int) map[string]bool {
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
	ep := make(map[string]bool)
	for _, fi := range g.Files {
		base := filepath.Base(fi.RelPath)
		switch {
		case fi.Language == types.LangGo && base == "main.go":
			ep[fi.RelPath] = true
		case fi.Language == types.LangGo && fi.Package == "main":
			ep[fi.RelPath] = true
		case fi.Language == types.LangPython && (base == "__main__.py" || base == "manage.py" || base == "app.py"):
			ep[fi.RelPath] = true
		case fi.Language == types.LangRust && base == "main.rs":
			ep[fi.RelPath] = true
		case fi.Language == types.LangJava && hasMainMethod(fi):
			ep[fi.RelPath] = true
		case base == "index.js" || base == "index.ts" || base == "index.tsx":
			ep[fi.RelPath] = true
		case fi.IsSpecial && fi.SpecialType == "dockerfile":
			ep[fi.RelPath] = true
		}

		// files with no importers are potential entrypoints
		if len(g.ReverseImports[fi.RelPath]) == 0 && len(fi.Symbols) > 0 {
			// weak entrypoint signal — only if the file exports something
			for _, sym := range fi.Symbols {
				if sym.Exported {
					ep[fi.RelPath] = true
					break
				}
			}
		}
	}
	return ep
}

func hasMainMethod(fi *types.FileInfo) bool {
	for _, sym := range fi.Symbols {
		if sym.Name == "main" && sym.Kind == "method" {
			return true
		}
	}
	return false
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
	if len(tokens) == 0 {
		return 0.0
	}

	score := 0.0
	pathLower := strings.ToLower(fi.RelPath)
	for _, t := range tokens {
		if strings.Contains(pathLower, t.Text) {
			score += 3.0 * t.Weight
		}
	}

	for _, sym := range fi.Symbols {
		nameLower := strings.ToLower(sym.Name)
		for _, t := range tokens {
			if strings.Contains(nameLower, t.Text) {
				score += 2.0 * t.Weight
			}
		}
		if sym.Doc != "" {
			docLower := strings.ToLower(sym.Doc)
			for _, t := range tokens {
				if strings.Contains(docLower, t.Text) {
					score += 1.0 * t.Weight
				}
			}
		}
	}

	if isTestFile(fi.RelPath) {
		score *= 0.5
	}

	return math.Min(score, 20.0)
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
	lower := strings.ToLower(relPath)
	base := lower
	if slash := strings.LastIndex(base, "/"); slash != -1 {
		base = base[slash+1:]
	}
	// Go
	if strings.HasSuffix(base, "_test.go") {
		return true
	}
	// Python
	if strings.HasSuffix(base, "_test.py") || strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py") {
		return true
	}
	// Rust: crates put unit tests inline and integration tests under tests/
	if strings.HasSuffix(base, ".rs") &&
		(strings.Contains(lower, "/tests/") || strings.HasPrefix(lower, "tests/")) {
		return true
	}
	// Java: FooTest.java, FooTests.java
	if strings.HasSuffix(base, "test.java") || strings.HasSuffix(base, "tests.java") {
		return true
	}
	// JS/TS: *.test.* and *.spec.*
	for _, ext := range []string{".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx"} {
		if strings.HasSuffix(base, ".test"+ext) || strings.HasSuffix(base, ".spec"+ext) {
			return true
		}
	}
	return false
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
