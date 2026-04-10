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
)

// RankGraph scores every file in the graph by importance.
// If query is non-empty, symbols/files matching the query get a bonus.
func RankGraph(g *Graph, query string) {
	recentFiles := getRecentlyChanged(g.Root, 50)
	entrypoints := detectEntrypoints(g)

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

	// score each file
	for _, fi := range g.Files {
		score := 0.0

		// sum reference/call counts for symbols defined in this file
		for _, sym := range fi.Symbols {
			score += float64(refCount[sym.Name]) * wReference
			score += float64(callCount[sym.Name]) * wInbound
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

		// query match bonus
		if query != "" {
			score += queryMatchScore(fi, query) * wQueryMatch
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
