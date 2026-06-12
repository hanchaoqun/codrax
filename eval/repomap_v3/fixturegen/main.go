// Package main regenerates the repomap_v3 symbol ground-truth fixture
// (eval/repomap_v3/fixtures/symbols.json) against the CURRENT HEAD.
//
// Why this exists: the fixture pins (name, file, line) triples, and a
// living repository moves them — the 2026-06 audit found the original
// hand-curated set (HEAD 7a60dd4 era) had decayed to 13.6% by-file
// recall purely from drift, turning the recall metric into a measure
// of repository churn instead of extractor quality. Regeneration must
// be one command, not an afternoon of hand-grepping:
//
//	go run ./eval/repomap_v3/fixturegen -repo . -out eval/repomap_v3/fixtures/symbols.json
//
// Selection policy (deterministic for a given HEAD):
//
//   - Load-bearing first: candidates are ranked by graph fan-in
//     (ID-resolved call/ref counts plus name-keyed counters), so the
//     fixture tracks symbols whose disappearance from the graph would
//     matter. Names this central rarely vanish outright — when an
//     entry fails later it is far more likely extractor regression
//     (the thing the recall metric exists to catch) than deletion.
//   - Exported, distinctive names only: len(name) >= 4 and at most
//     maxSameNameDefs same-name definitions repo-wide, so ground
//     truth stays unambiguous under the harness's name-first match.
//   - Spread by package quota (mirroring the original fixture's
//     breadth, extended to subsystems that did not exist then) and
//     kind-diverse within each package via round-robin over
//     type-like / function / method buckets.
//   - Independently line-verified: every emitted entry is checked
//     against the file bytes — the declaration line (±1) must contain
//     the name and a declaration keyword — so an extractor false
//     positive cannot launder itself into its own ground truth.
//
// Queries (fixtures/queries.json) remain hand-curated: expected_files
// encode human judgement about what a query SHOULD surface, which a
// generator cannot synthesize honestly.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
)

type fixtureSymbol struct {
	Name string `json:"name"`
	File string `json:"file"`
	Line int    `json:"line"`
	Kind string `json:"kind"`
}

type symbolsFixture struct {
	Comment string          `json:"_comment"`
	Symbols []fixtureSymbol `json:"symbols"`
}

// Package quotas: breadth mirrors the original fixture (types / tool /
// agent / orchestrator / analysis / context) extended to subsystems
// added since (repomap sub-packages counted separately from tool,
// tracequery, skill, repl, config, memory). Sum = 125.
var packageQuotas = []struct {
	prefix string
	quota  int
}{
	{"internal/types/", 25},
	{"internal/tool/repomap/", 16},
	{"internal/tool/", 18}, // non-repomap tool files (matched after repomap)
	{"internal/agent/", 15},
	{"internal/orchestrator/", 14},
	{"internal/analysis/", 10},
	{"internal/context/", 6},
	{"internal/tracequery/", 8},
	{"internal/skill/", 4},
	{"internal/repl/", 4},
	{"internal/config/", 2},
	{"internal/memory/", 2},
	{"internal/worktree/", 2},
}

const (
	targetTotal     = 125
	minNameLen      = 4
	maxSameNameDefs = 3
)

func main() {
	repoFlag := flag.String("repo", ".", "repository root to scan")
	outFlag := flag.String("out", "eval/repomap_v3/fixtures/symbols.json", "output fixture path")
	flag.Parse()

	repoRoot, err := filepath.Abs(*repoFlag)
	must(err)

	graph, err := repomap.BuildOrLoadGraph(repoRoot, "")
	must(err)
	idx := graph.RankIndex
	if idx == nil {
		fmt.Fprintln(os.Stderr, "fixturegen: graph has no rank index")
		os.Exit(1)
	}

	type candidate struct {
		sym   fixtureSymbol
		score float64
		pkg   string
		class string // "type" | "function" | "method"
	}
	byPkg := map[string][]candidate{}

	for _, fi := range graph.Files {
		if strings.HasSuffix(fi.RelPath, "_test.go") ||
			strings.Contains(fi.RelPath, "thirdparty/") ||
			strings.HasPrefix(fi.RelPath, "eval/") {
			continue
		}
		pkg := quotaBucket(fi.RelPath)
		if pkg == "" {
			continue
		}
		for _, sym := range fi.Symbols {
			class := kindClass(sym.Kind)
			if class == "" || !sym.Exported || len(sym.Name) < minNameLen {
				continue
			}
			if len(graph.SymbolDefs[sym.Name]) > maxSameNameDefs {
				continue
			}
			score := float64(idx.CallCount[sym.Name] + idx.RefCount[sym.Name])
			if sym.ID != "" {
				score += float64(idx.CallCountByID[sym.ID] + idx.RefCountByID[sym.ID])
			}
			byPkg[pkg] = append(byPkg[pkg], candidate{
				sym:   fixtureSymbol{Name: sym.Name, File: sym.File, Line: sym.Line, Kind: sym.Kind},
				score: score,
				pkg:   pkg,
				class: class,
			})
		}
	}

	var out []fixtureSymbol
	for _, q := range packageQuotas {
		cands := byPkg[q.prefix]
		// Deterministic: score desc, then name, then file.
		sort.Slice(cands, func(i, j int) bool {
			if cands[i].score != cands[j].score {
				return cands[i].score > cands[j].score
			}
			if cands[i].sym.Name != cands[j].sym.Name {
				return cands[i].sym.Name < cands[j].sym.Name
			}
			return cands[i].sym.File < cands[j].sym.File
		})
		// Round-robin over kind classes for diversity, verifying each
		// pick against the file bytes before acceptance.
		buckets := map[string][]candidate{}
		for _, c := range cands {
			buckets[c.class] = append(buckets[c.class], c)
		}
		order := []string{"type", "function", "method"}
		taken := 0
		seen := map[string]bool{}
		for taken < q.quota {
			progressed := false
			for _, class := range order {
				if taken >= q.quota {
					break
				}
				for len(buckets[class]) > 0 {
					c := buckets[class][0]
					buckets[class] = buckets[class][1:]
					key := c.sym.Name + "\x00" + c.sym.File
					if seen[key] || !verifyDeclarationLine(repoRoot, c.sym) {
						continue
					}
					seen[key] = true
					out = append(out, c.sym)
					taken++
					progressed = true
					break
				}
			}
			if !progressed {
				break // package exhausted below quota
			}
		}
		if taken < q.quota {
			fmt.Fprintf(os.Stderr, "fixturegen: %s filled %d/%d (not enough verified candidates)\n", q.prefix, taken, q.quota)
		}
	}

	if len(out) > targetTotal {
		out = out[:targetTotal]
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Line < out[j].Line
	})

	fx := symbolsFixture{
		Comment: fmt.Sprintf(
			"GENERATED ground truth for repomap_v3 symbol recall — regenerate with `go run ./eval/repomap_v3/fixturegen` after large refactors (the harness warns when by-name recall far exceeds by-file recall, the moved-files signature). Selection: exported, load-bearing (graph fan-in ranked), distinctive names, package quotas, every entry independently line-verified against file bytes. Generated at HEAD %s; %d symbols; harness tolerates ±2 line drift.",
			gitHead(repoRoot), len(out)),
		Symbols: out,
	}
	data, err := json.MarshalIndent(fx, "", "  ")
	must(err)
	must(os.WriteFile(*outFlag, append(data, '\n'), 0o644))
	fmt.Fprintf(os.Stderr, "fixturegen: wrote %d symbols to %s\n", len(out), *outFlag)
}

// quotaBucket maps a rel path to its quota prefix. repomap files must
// match their own bucket before the broader internal/tool/ prefix.
func quotaBucket(relPath string) string {
	if strings.HasPrefix(relPath, "internal/tool/repomap/") {
		return "internal/tool/repomap/"
	}
	for _, q := range packageQuotas {
		if q.prefix == "internal/tool/repomap/" {
			continue
		}
		if strings.HasPrefix(relPath, q.prefix) {
			return q.prefix
		}
	}
	return ""
}

func kindClass(kind string) string {
	switch kind {
	case "type", "struct", "interface", "enum", "trait", "class":
		return "type"
	case "function":
		return "function"
	case "method":
		return "method"
	default:
		return "" // const/var/field/route/… excluded: line drift too noisy for ground truth
	}
}

// verifyDeclarationLine independently checks the pinned line against
// the file bytes: the name must appear on the declaration line (±1)
// together with a declaration keyword. This keeps extractor false
// positives from laundering themselves into their own ground truth.
func verifyDeclarationLine(repoRoot string, sym fixtureSymbol) bool {
	data, err := os.ReadFile(filepath.Join(repoRoot, sym.File))
	if err != nil {
		return false
	}
	lines := strings.Split(string(data), "\n")
	for off := -1; off <= 1; off++ {
		n := sym.Line - 1 + off
		if n < 0 || n >= len(lines) {
			continue
		}
		line := lines[n]
		if !strings.Contains(line, sym.Name) {
			continue
		}
		if strings.Contains(line, "func ") || strings.Contains(line, "type ") ||
			strings.Contains(line, "const ") || strings.Contains(line, "var ") ||
			strings.Contains(line, sym.Name+" struct") || strings.Contains(line, sym.Name+" interface") {
			return true
		}
	}
	return false
}

func gitHead(repoRoot string) string {
	cmd := exec.Command("git", "-C", repoRoot, "rev-parse", "--short", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "fixturegen:", err)
		os.Exit(1)
	}
}
