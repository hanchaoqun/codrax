package repomap

import (
	"math"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
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
func RankGraph(g *Graph, query string) {
	recentFiles := getRecentlyChanged(g.Root, 50)
	entrypoints := detectEntrypoints(g)
	totalFiles := float64(len(g.Files))

	// count how many files reference each symbol name
	refCount := make(map[string]int)
	callCount := make(map[string]int)
	for _, fi := range g.Files {
		for _, rel := range fi.Relations {
			switch rel.Kind {
			case "call":
				callCount[rel.To]++
			case "type_usage", "reference":
				refCount[rel.To]++
			}
		}
	}

	// Symbol name ambiguity: count how many files define the same
	// symbol name. Common interface methods (Name, Execute, String,
	// Error, etc.) are defined in many files. When a symbol name has
	// N definitions across N files, each file's credit for that
	// symbol's refs/calls should be divided by N — otherwise a file
	// with 7 tools implementing Name() gets 7× the global call count
	// for "Name", which is meaningless noise.
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

	// Compute per-file fan-out: count how many distinct files
	// reference ANY symbol defined in this file. This is broader
	// than import fan-in because it captures call-site and type-usage
	// references, not just import statements. High fan-out (>0.15)
	// strongly suggests infrastructure, not domain code.
	symbolToFile := make(map[string]string, len(g.Files)*10)
	for _, fi := range g.Files {
		for _, sym := range fi.Symbols {
			symbolToFile[sym.Name] = fi.RelPath
		}
	}
	// Count distinct files that reference each file's symbols.
	fileReferrers := make(map[string]map[string]bool)
	for _, fi := range g.Files {
		for _, rel := range fi.Relations {
			if defFile, ok := symbolToFile[rel.To]; ok && defFile != fi.RelPath {
				if fileReferrers[defFile] == nil {
					fileReferrers[defFile] = make(map[string]bool)
				}
				fileReferrers[defFile][fi.RelPath] = true
			}
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

		// sum reference/call counts for symbols defined in this file,
		// divided by the number of files that define the same name.
		for _, sym := range fi.Symbols {
			ambiguity := float64(symDefCount[sym.Name])
			if ambiguity < 1 {
				ambiguity = 1
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
func TopFiles(g *Graph, topN int) []*FileInfo {
	sorted := make([]*FileInfo, len(g.Files))
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
	cmd := exec.Command("git", "-C", repoRoot, "log",
		"--pretty=format:", "--name-only", "-n", itoa(n))
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

func detectEntrypoints(g *Graph) map[string]bool {
	ep := make(map[string]bool)
	for _, fi := range g.Files {
		base := filepath.Base(fi.RelPath)
		switch {
		case fi.Language == LangGo && base == "main.go":
			ep[fi.RelPath] = true
		case fi.Language == LangGo && fi.Package == "main":
			ep[fi.RelPath] = true
		case fi.Language == LangPython && (base == "__main__.py" || base == "manage.py" || base == "app.py"):
			ep[fi.RelPath] = true
		case fi.Language == LangRust && base == "main.rs":
			ep[fi.RelPath] = true
		case fi.Language == LangJava && hasMainMethod(fi):
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

func hasMainMethod(fi *FileInfo) bool {
	for _, sym := range fi.Symbols {
		if sym.Name == "main" && sym.Kind == "method" {
			return true
		}
	}
	return false
}

func queryMatchScore(fi *FileInfo, query string) float64 {
	query = strings.ToLower(query)
	terms := strings.Fields(query)
	score := 0.0

	fileLower := strings.ToLower(fi.RelPath)
	for _, term := range terms {
		if strings.Contains(fileLower, term) {
			score += 3.0
		}
	}

	for _, sym := range fi.Symbols {
		nameLower := strings.ToLower(sym.Name)
		for _, term := range terms {
			if strings.Contains(nameLower, term) {
				score += 2.0
			}
		}
		if sym.Doc != "" {
			docLower := strings.ToLower(sym.Doc)
			for _, term := range terms {
				if strings.Contains(docLower, term) {
					score += 1.0
				}
			}
		}
	}

	return math.Min(score, 20.0) // cap to avoid one file dominating
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
