// Package main is the deterministic repomap_v3 eval harness.
//
// It builds the current HEAD's repomap over the codrax repository,
// loads curated ground-truth fixtures, and computes six metrics with
// no LLM in the loop. Output is written as a JSON report plus a
// markdown summary so a human can diff two runs.
//
// Why this exists: prior eval runs used substring-gated LLM output,
// which silently goes "false green" when the LLM happens to emit the
// expected token. The repomap refactor (plan memo:
// project_repomap_refactor_plan.md) needs a trustworthy gate before
// Phase 1 SymbolID lands. This harness IS that gate. Every metric is
// reproducible without a network call, so the same commit produces
// the same numbers every time.
//
// Usage:
//
//	go run ./eval/repomap_v3/harness -repo . -fixtures eval/repomap_v3/fixtures -out eval/repomap_v3/baseline.json
//
// Or via the wrapper Makefile target (not written here — user can add
// one later). Exit code is 0 on successful run regardless of metric
// values; a gate/compare step is a separate tool.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
)

// ── Ground-truth fixture schemas ────────────────────────────────────────

type fixtureSymbol struct {
	Name     string `json:"name"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Kind     string `json:"kind"`
	Receiver string `json:"receiver,omitempty"`
}

type symbolsFixture struct {
	Symbols []fixtureSymbol `json:"symbols"`
}

type fixtureQuery struct {
	Query         string   `json:"query"`
	ExpectedFiles []string `json:"expected_files"`
	TopK          int      `json:"top_k,omitempty"`
}

type queriesFixture struct {
	DefaultTopK int            `json:"default_top_k"`
	Queries     []fixtureQuery `json:"queries"`
}

// ── Metric result schemas ───────────────────────────────────────────────

type symbolPrecision struct {
	// Precision is measured by sampling the extracted symbol set: for
	// each extracted Symbol, open its file, read the declaration line,
	// and check whether the line mentions the symbol name. A mismatch
	// counts as a false positive. This is a lower bound on precision
	// (some symbols whose declaration spans multiple lines may show up
	// on a neighbour line), but it catches the categories that matter:
	// extractor emitting a symbol that doesn't exist, line drift, etc.
	TotalChecked  int     `json:"total_checked"`
	LinePresent   int     `json:"line_present"`   // file exists and line is in range
	NameOnLine    int     `json:"name_on_line"`   // name found on or near the declared line
	Precision     float64 `json:"precision"`      // NameOnLine / LinePresent
	MissedSamples []string `json:"missed_samples,omitempty"` // up to 20 failure exemplars
}

type symbolRecall struct {
	// Recall is measured against the curated fixture: for each fixture
	// entry, look up the symbol in graph.SymbolDefs[name] and check
	// whether any of the returned definitions matches the fixture's
	// (file, line±tolerance).
	TotalExpected   int      `json:"total_expected"`
	Found           int      `json:"found"`
	FoundByName     int      `json:"found_by_name"` // name present but wrong file/line
	Recall          float64  `json:"recall"`
	NameThenFileRecall float64 `json:"name_then_file_recall"` // FoundByName / TotalExpected
	Missing         []string `json:"missing,omitempty"`
}

type importAccuracy struct {
	// For every import relation extracted by repomap, check whether
	// the target file exists in graph.FileIndex. Unresolved imports
	// count as inaccurate — the resolver failed to map path→file.
	TotalRelations int     `json:"total_relations"`
	Resolved       int     `json:"resolved"`
	Accuracy       float64 `json:"accuracy"`
	PerLanguage    map[string]importLangStats `json:"per_language"`
}

type importLangStats struct {
	Total    int     `json:"total"`
	Resolved int     `json:"resolved"`
	Accuracy float64 `json:"accuracy"`
}

type callEdgeAmbiguity struct {
	// For every call relation, count how many symbol definitions match
	// the `To` name. The receiver-drift bug (ContinuationPrompt
	// defined on 3 receivers) shows up here as ambiguity > 1. A clean
	// graph has ambiguity ≤ 1 for every call edge.
	TotalCallEdges     int            `json:"total_call_edges"`
	UnambiguousEdges   int            `json:"unambiguous_edges"`
	Unambiguous        float64        `json:"unambiguous_ratio"`
	AmbiguityHistogram map[int]int    `json:"ambiguity_histogram"` // ambiguity → edge count
	TopAmbiguous       []ambiguousRow `json:"top_ambiguous"`

	// Phase 1 P1.2a additions. These read ToEP (the structured call
	// endpoint) rather than the legacy rel.To string, and measure
	// how much receiver information the Go extractor is now capturing
	// and how much of it is usable for disambiguation.
	CallsWithReceiver      int     `json:"calls_with_receiver"`      // ToEP.Receiver != ""
	ReceiverCaptureRatio   float64 `json:"receiver_capture_ratio"`   // CallsWithReceiver / TotalCallEdges
	UnambigByReceiverType  int     `json:"unambig_by_receiver_type"` // receiver matches a type in the graph AND that type has exactly one method with the given name
	UnambigByReceiverRatio float64 `json:"unambig_by_receiver_ratio"`

	// Phase 1 drift-resolution KPI. A "drift call" is a call to a
	// name whose legacy SymbolDefs has more than one definition —
	// i.e. a call that would be ambiguous under name-only lookup.
	// DriftCallsResolved is the subset whose scope-resolved receiver
	// narrows the candidate defs down to exactly one. This is the
	// direct numerical answer to "how much of the receiver-drift bug
	// is Phase 1 actually fixing on local symbols".
	DriftCalls          int     `json:"drift_calls"`
	DriftCallsResolved  int     `json:"drift_calls_resolved"`
	DriftResolvedRatio  float64 `json:"drift_resolved_ratio"`
}

type ambiguousRow struct {
	SymbolName string `json:"symbol_name"`
	Count      int    `json:"count"`
}

type taskMapHitAtK struct {
	TotalQueries int            `json:"total_queries"`
	MeanHitAtK   float64        `json:"mean_hit_at_k"`
	PerfectHits  int            `json:"perfect_hits"` // queries with hit@k == 1.0
	PerQuery     []queryResult  `json:"per_query"`
}

type queryResult struct {
	Query         string   `json:"query"`
	ExpectedFiles []string `json:"expected_files"`
	TopK          int      `json:"top_k"`
	RankedTop     []string `json:"ranked_top"`
	Hit           float64  `json:"hit"`
	Missing       []string `json:"missing,omitempty"`
}

type scanLatency struct {
	FullScanSeconds float64 `json:"full_scan_seconds"`
	FileCount       int     `json:"file_count"`
	SymbolCount     int     `json:"symbol_count"`
	RelationCount   int     `json:"relation_count"`
}

type baseline struct {
	Timestamp    string            `json:"timestamp"`
	RepoRoot     string            `json:"repo_root"`
	SymbolPrec   symbolPrecision   `json:"symbol_precision"`
	SymbolRec    symbolRecall      `json:"symbol_recall"`
	ImportAcc    importAccuracy    `json:"import_accuracy"`
	CallAmbig    callEdgeAmbiguity `json:"call_edge_ambiguity"`
	TaskMapHit   taskMapHitAtK     `json:"task_map_hit_at_k"`
	Latency      scanLatency       `json:"scan_latency"`
}

// ── Entry point ─────────────────────────────────────────────────────────

func main() {
	repoFlag := flag.String("repo", ".", "repository root to scan")
	fixturesFlag := flag.String("fixtures", "eval/repomap_v3/fixtures", "fixtures directory")
	outFlag := flag.String("out", "eval/repomap_v3/baseline.json", "JSON output path")
	mdFlag := flag.String("md", "eval/repomap_v3/baseline.md", "markdown summary path")
	precisionSample := flag.Int("precision-sample", 500, "max extracted symbols to sample for precision (0 = all)")
	flag.Parse()

	repoRoot, err := filepath.Abs(*repoFlag)
	must(err)

	fixSyms, err := loadSymbolsFixture(filepath.Join(*fixturesFlag, "symbols.json"))
	must(err)
	fixQueries, err := loadQueriesFixture(filepath.Join(*fixturesFlag, "queries.json"))
	must(err)

	fmt.Fprintf(os.Stderr, "repomap_v3 harness: repo=%s symbols=%d queries=%d\n",
		repoRoot, len(fixSyms.Symbols), len(fixQueries.Queries))

	// Metric 6 (latency) wraps the full-scan call.
	start := time.Now()
	graph, err := repomap.BuildOrLoadGraph(repoRoot, "")
	must(err)
	latency := scanLatency{
		FullScanSeconds: time.Since(start).Seconds(),
		FileCount:       graph.Metadata.FileCount,
		SymbolCount:     graph.Metadata.SymbolCount,
		RelationCount:   graph.Metadata.RelationCount,
	}
	fmt.Fprintf(os.Stderr, "scan: %.2fs, %d files, %d symbols, %d relations\n",
		latency.FullScanSeconds, latency.FileCount, latency.SymbolCount, latency.RelationCount)

	result := baseline{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		RepoRoot:   repoRoot,
		Latency:    latency,
		SymbolPrec: computeSymbolPrecision(graph, repoRoot, *precisionSample),
		SymbolRec:  computeSymbolRecall(graph, fixSyms),
		ImportAcc:  computeImportAccuracy(graph),
		CallAmbig:  computeCallAmbiguity(graph),
		TaskMapHit: computeTaskMapHitAtK(repoRoot, fixQueries),
	}

	writeJSON(*outFlag, result)
	writeMarkdown(*mdFlag, result)
	fmt.Fprintf(os.Stderr, "wrote %s and %s\n", *outFlag, *mdFlag)
}

// ── Metric 1: symbol precision ──────────────────────────────────────────

func computeSymbolPrecision(g *repomap.Graph, repoRoot string, sampleCap int) symbolPrecision {
	type flat struct {
		name, file string
		line       int
	}
	var all []flat
	for _, fi := range g.Files {
		for _, s := range fi.Symbols {
			all = append(all, flat{name: s.Name, file: fi.RelPath, line: s.Line})
		}
	}
	// Deterministic sampling: sort and stride so a fixed cap always
	// picks the same subset across runs.
	sort.Slice(all, func(i, j int) bool {
		if all[i].file != all[j].file {
			return all[i].file < all[j].file
		}
		if all[i].line != all[j].line {
			return all[i].line < all[j].line
		}
		return all[i].name < all[j].name
	})
	if sampleCap > 0 && len(all) > sampleCap {
		stride := len(all) / sampleCap
		if stride < 1 {
			stride = 1
		}
		picked := make([]flat, 0, sampleCap)
		for i := 0; i < len(all) && len(picked) < sampleCap; i += stride {
			picked = append(picked, all[i])
		}
		all = picked
	}

	fileLines := make(map[string][]string)
	out := symbolPrecision{}
	for _, s := range all {
		out.TotalChecked++
		lines, ok := fileLines[s.file]
		if !ok {
			l, err := readLines(filepath.Join(repoRoot, s.file))
			if err != nil {
				lines = nil
			} else {
				lines = l
			}
			fileLines[s.file] = lines
		}
		if lines == nil || s.line <= 0 || s.line > len(lines) {
			if len(out.MissedSamples) < 20 {
				out.MissedSamples = append(out.MissedSamples, fmt.Sprintf("%s:%d %s (line out of range)", s.file, s.line, s.name))
			}
			continue
		}
		out.LinePresent++
		// Check the target line and a ±1 window for name presence.
		hit := false
		for off := -1; off <= 1; off++ {
			idx := s.line - 1 + off
			if idx < 0 || idx >= len(lines) {
				continue
			}
			if strings.Contains(lines[idx], s.name) {
				hit = true
				break
			}
		}
		if hit {
			out.NameOnLine++
		} else if len(out.MissedSamples) < 20 {
			out.MissedSamples = append(out.MissedSamples, fmt.Sprintf("%s:%d %s (name not on line)", s.file, s.line, s.name))
		}
	}
	if out.LinePresent > 0 {
		out.Precision = float64(out.NameOnLine) / float64(out.LinePresent)
	}
	return out
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 8*1024*1024)
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	return out, sc.Err()
}

// ── Metric 2: symbol recall ─────────────────────────────────────────────

func computeSymbolRecall(g *repomap.Graph, fx symbolsFixture) symbolRecall {
	out := symbolRecall{TotalExpected: len(fx.Symbols)}
	const tolerance = 2
	for _, want := range fx.Symbols {
		defs := g.SymbolDefs[want.Name]
		if len(defs) == 0 {
			out.Missing = append(out.Missing, fmt.Sprintf("%s (no definitions extracted)", want.Name))
			continue
		}
		out.FoundByName++
		matched := false
		for _, d := range defs {
			if d.File == want.File && abs(d.Line-want.Line) <= tolerance {
				matched = true
				break
			}
		}
		if !matched {
			out.Missing = append(out.Missing, fmt.Sprintf("%s @ %s:%d (found at %s)",
				want.Name, want.File, want.Line, fmtDefs(defs)))
			continue
		}
		out.Found++
	}
	if out.TotalExpected > 0 {
		out.Recall = float64(out.Found) / float64(out.TotalExpected)
		out.NameThenFileRecall = float64(out.FoundByName) / float64(out.TotalExpected)
	}
	return out
}

func fmtDefs(defs []*repomap.Symbol) string {
	parts := make([]string, 0, len(defs))
	for _, d := range defs {
		parts = append(parts, fmt.Sprintf("%s:%d", d.File, d.Line))
	}
	return strings.Join(parts, ", ")
}

func abs(x int) int { if x < 0 { return -x }; return x }

// ── Metric 3: import edge accuracy ──────────────────────────────────────

func computeImportAccuracy(g *repomap.Graph) importAccuracy {
	out := importAccuracy{PerLanguage: make(map[string]importLangStats)}

	// Per-(file,rawPath) unresolved lookup populated by
	// resolveImportGraph. Keyed by "relpath|raw" so duplicate Import
	// entries in a single file (rare but legal, e.g. JS re-imports)
	// are each counted individually.
	unresolved := make(map[string]int)
	for _, u := range g.Metadata.UnresolvedImports {
		unresolved[u.File+"|"+u.Raw]++
	}

	for _, fi := range g.Files {
		if len(fi.Imports) == 0 {
			continue
		}
		langStats := out.PerLanguage[fi.Language]
		for _, imp := range fi.Imports {
			out.TotalRelations++
			langStats.Total++
			key := fi.RelPath + "|" + imp.Path
			if n := unresolved[key]; n > 0 {
				unresolved[key] = n - 1
				continue
			}
			out.Resolved++
			langStats.Resolved++
		}
		out.PerLanguage[fi.Language] = langStats
	}
	if out.TotalRelations > 0 {
		out.Accuracy = float64(out.Resolved) / float64(out.TotalRelations)
	}
	for lang, s := range out.PerLanguage {
		if s.Total > 0 {
			s.Accuracy = float64(s.Resolved) / float64(s.Total)
			out.PerLanguage[lang] = s
		}
	}
	return out
}

// ── Metric 4: call edge ambiguity ───────────────────────────────────────

func computeCallAmbiguity(g *repomap.Graph) callEdgeAmbiguity {
	out := callEdgeAmbiguity{
		AmbiguityHistogram: make(map[int]int),
	}
	symDefCount := make(map[string]int)
	for name, defs := range g.SymbolDefs {
		symDefCount[name] = len(defs)
	}
	// typeSet: the set of symbol names that ARE a type (struct,
	// interface, type alias). Checked ACROSS all packages — a call's
	// receiver text may name a type declared in a different package
	// (the common `*foreign.Type` case), so a same-package check
	// undercounts. The set is a flat name → true map; a type name
	// shared by two packages still counts as a matchable receiver.
	typeSet := make(map[string]bool)
	for _, fi := range g.Files {
		for idx := range fi.Symbols {
			s := &fi.Symbols[idx]
			switch s.Kind {
			case "type", "struct", "interface", "class":
				typeSet[s.Name] = true
			}
		}
	}
	ambigCount := make(map[string]int)
	for _, fi := range g.Files {
		for _, rel := range fi.Relations {
			if rel.Kind != "call" {
				continue
			}
			out.TotalCallEdges++
			// Legacy name-only ambiguity — the pre-Phase-1 metric.
			// Kept so we can track it through the cutover; P1.2b
			// replaces it.
			n := symDefCount[rel.To]
			if n == 0 {
				out.AmbiguityHistogram[0]++
			} else {
				out.AmbiguityHistogram[n]++
				if n == 1 {
					out.UnambiguousEdges++
				} else {
					ambigCount[rel.To] += 1
				}
			}
			// Phase 1 receiver-aware accounting. ToEP.Receiver is only
			// populated by the Go extractor's selector_expression path
			// (P1.2a); older extractors leave it empty.
			if rel.ToEP.Receiver == "" {
				continue
			}
			out.CallsWithReceiver++
			// When the receiver text matches a type declared anywhere
			// in the graph, filter defs by matching receiver and count
			// as receiver-resolved if exactly one method survives.
			// Cross-package types are included because the Phase 1
			// scope resolver rewrites call-site receivers to their
			// declared Go type name, which commonly lives in a
			// different package than the caller.
			if typeSet[rel.ToEP.Receiver] {
				matchingDefs := 0
				for _, d := range g.SymbolDefs[rel.To] {
					if d.Receiver == rel.ToEP.Receiver {
						matchingDefs++
					}
				}
				if matchingDefs == 1 {
					out.UnambigByReceiverType++
				}
			}
			// Drift-resolution KPI: direct measurement of Phase 1's
			// receiver-drift fix on multi-def local symbols. Skip
			// single-def symbols (not drift) and symbols with zero
			// defs (external — cannot possibly be resolved against
			// the local graph).
			if n > 1 {
				out.DriftCalls++
				matchingDefs := 0
				for _, d := range g.SymbolDefs[rel.To] {
					if d.Receiver == rel.ToEP.Receiver {
						matchingDefs++
					}
				}
				if matchingDefs == 1 {
					out.DriftCallsResolved++
				}
			}
		}
	}
	if out.TotalCallEdges > 0 {
		out.Unambiguous = float64(out.UnambiguousEdges) / float64(out.TotalCallEdges)
		out.ReceiverCaptureRatio = float64(out.CallsWithReceiver) / float64(out.TotalCallEdges)
		out.UnambigByReceiverRatio = float64(out.UnambigByReceiverType) / float64(out.TotalCallEdges)
	}
	if out.DriftCalls > 0 {
		out.DriftResolvedRatio = float64(out.DriftCallsResolved) / float64(out.DriftCalls)
	}
	// Top 20 ambiguous symbols by # of call edges pointing at them.
	type kv struct {
		k string
		v int
	}
	rows := make([]kv, 0, len(ambigCount))
	for k, v := range ambigCount {
		rows = append(rows, kv{k, v})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].v != rows[j].v {
			return rows[i].v > rows[j].v
		}
		return rows[i].k < rows[j].k
	})
	for i, r := range rows {
		if i >= 20 {
			break
		}
		out.TopAmbiguous = append(out.TopAmbiguous, ambiguousRow{SymbolName: r.k, Count: r.v})
	}
	return out
}

// ── Metric 5: task_map hit@k ────────────────────────────────────────────

func computeTaskMapHitAtK(repoRoot string, fx queriesFixture) taskMapHitAtK {
	out := taskMapHitAtK{TotalQueries: len(fx.Queries)}
	defaultK := fx.DefaultTopK
	if defaultK <= 0 {
		defaultK = 5
	}
	var sumHit float64
	for _, q := range fx.Queries {
		k := q.TopK
		if k <= 0 {
			k = defaultK
		}
		// Re-scan with the query so QueryScores gets populated. The
		// underlying scan is cache-hit after the first run, so this is
		// cheap (graph build happens once; only rank is re-run).
		g, err := repomap.BuildOrLoadGraph(repoRoot, q.Query)
		if err != nil {
			out.PerQuery = append(out.PerQuery, queryResult{
				Query: q.Query, ExpectedFiles: q.ExpectedFiles, TopK: k,
				Missing: []string{fmt.Sprintf("scan error: %v", err)},
			})
			continue
		}
		top := topKQueryFiles(g, k)
		hit, missing := hitFraction(top, q.ExpectedFiles)
		sumHit += hit
		if hit == 1.0 {
			out.PerfectHits++
		}
		out.PerQuery = append(out.PerQuery, queryResult{
			Query:         q.Query,
			ExpectedFiles: q.ExpectedFiles,
			TopK:          k,
			RankedTop:     top,
			Hit:           hit,
			Missing:       missing,
		})
	}
	if len(fx.Queries) > 0 {
		out.MeanHitAtK = sumHit / float64(len(fx.Queries))
	}
	return out
}

func topKQueryFiles(g *repomap.Graph, k int) []string {
	type kv struct {
		f string
		s float64
	}
	rows := make([]kv, 0, len(g.QueryScores))
	for f, s := range g.QueryScores {
		if s > 0 {
			rows = append(rows, kv{f, s})
		}
	}
	// Fall back to structural Scores when no query match surfaced.
	if len(rows) == 0 {
		for f, s := range g.Scores {
			rows = append(rows, kv{f, s})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].s != rows[j].s {
			return rows[i].s > rows[j].s
		}
		return rows[i].f < rows[j].f
	})
	if k > len(rows) {
		k = len(rows)
	}
	out := make([]string, k)
	for i := 0; i < k; i++ {
		out[i] = rows[i].f
	}
	return out
}

func hitFraction(top, expected []string) (float64, []string) {
	set := make(map[string]bool, len(top))
	for _, f := range top {
		set[f] = true
	}
	var missing []string
	hits := 0
	for _, e := range expected {
		if set[e] {
			hits++
		} else {
			missing = append(missing, e)
		}
	}
	if len(expected) == 0 {
		return 0, nil
	}
	return float64(hits) / float64(len(expected)), missing
}

// ── I/O helpers ─────────────────────────────────────────────────────────

func loadSymbolsFixture(path string) (symbolsFixture, error) {
	var fx symbolsFixture
	data, err := os.ReadFile(path)
	if err != nil {
		return fx, err
	}
	err = json.Unmarshal(data, &fx)
	return fx, err
}

func loadQueriesFixture(path string) (queriesFixture, error) {
	var fx queriesFixture
	data, err := os.ReadFile(path)
	if err != nil {
		return fx, err
	}
	err = json.Unmarshal(data, &fx)
	return fx, err
}

func writeJSON(path string, v any) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		must(err)
	}
	data, err := json.MarshalIndent(v, "", "  ")
	must(err)
	must(os.WriteFile(path, data, 0o644))
}

func writeMarkdown(path string, b baseline) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# repomap_v3 baseline — %s\n\n", b.Timestamp)
	fmt.Fprintf(&sb, "- repo: `%s`\n", b.RepoRoot)
	fmt.Fprintf(&sb, "- files: %d, symbols: %d, relations: %d\n", b.Latency.FileCount, b.Latency.SymbolCount, b.Latency.RelationCount)
	fmt.Fprintf(&sb, "- full scan: %.2fs\n\n", b.Latency.FullScanSeconds)

	fmt.Fprintf(&sb, "## Metrics\n\n")
	fmt.Fprintf(&sb, "| metric | value |\n|---|---|\n")
	fmt.Fprintf(&sb, "| symbol precision (sample %d / %d checked) | %.3f |\n",
		b.SymbolPrec.NameOnLine, b.SymbolPrec.LinePresent, b.SymbolPrec.Precision)
	fmt.Fprintf(&sb, "| symbol recall (%d / %d) | %.3f |\n",
		b.SymbolRec.Found, b.SymbolRec.TotalExpected, b.SymbolRec.Recall)
	fmt.Fprintf(&sb, "| symbol recall by name only | %.3f |\n", b.SymbolRec.NameThenFileRecall)
	fmt.Fprintf(&sb, "| import edge accuracy (%d / %d) | %.3f |\n",
		b.ImportAcc.Resolved, b.ImportAcc.TotalRelations, b.ImportAcc.Accuracy)
	fmt.Fprintf(&sb, "| call edge unambiguous ratio (%d / %d) | %.3f |\n",
		b.CallAmbig.UnambiguousEdges, b.CallAmbig.TotalCallEdges, b.CallAmbig.Unambiguous)
	fmt.Fprintf(&sb, "| call receiver capture ratio (%d / %d) | %.3f |\n",
		b.CallAmbig.CallsWithReceiver, b.CallAmbig.TotalCallEdges, b.CallAmbig.ReceiverCaptureRatio)
	fmt.Fprintf(&sb, "| unambiguous by receiver-type (%d / %d) | %.3f |\n",
		b.CallAmbig.UnambigByReceiverType, b.CallAmbig.TotalCallEdges, b.CallAmbig.UnambigByReceiverRatio)
	fmt.Fprintf(&sb, "| **drift calls resolved (%d / %d)** | **%.3f** |\n",
		b.CallAmbig.DriftCallsResolved, b.CallAmbig.DriftCalls, b.CallAmbig.DriftResolvedRatio)
	fmt.Fprintf(&sb, "| task_map mean hit@k (%d / %d perfect) | %.3f |\n",
		b.TaskMapHit.PerfectHits, b.TaskMapHit.TotalQueries, b.TaskMapHit.MeanHitAtK)

	fmt.Fprintf(&sb, "\n## Import accuracy per language\n\n| language | resolved / total | accuracy |\n|---|---|---|\n")
	langs := make([]string, 0, len(b.ImportAcc.PerLanguage))
	for l := range b.ImportAcc.PerLanguage {
		langs = append(langs, l)
	}
	sort.Strings(langs)
	for _, l := range langs {
		s := b.ImportAcc.PerLanguage[l]
		fmt.Fprintf(&sb, "| %s | %d / %d | %.3f |\n", l, s.Resolved, s.Total, s.Accuracy)
	}

	fmt.Fprintf(&sb, "\n## Call edge ambiguity histogram\n\n| ambiguity | count |\n|---|---|\n")
	keys := make([]int, 0, len(b.CallAmbig.AmbiguityHistogram))
	for k := range b.CallAmbig.AmbiguityHistogram {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	for _, k := range keys {
		fmt.Fprintf(&sb, "| %d | %d |\n", k, b.CallAmbig.AmbiguityHistogram[k])
	}

	if len(b.CallAmbig.TopAmbiguous) > 0 {
		fmt.Fprintf(&sb, "\n## Top ambiguous call targets\n\n| symbol | call edges |\n|---|---|\n")
		for _, r := range b.CallAmbig.TopAmbiguous {
			fmt.Fprintf(&sb, "| %s | %d |\n", r.SymbolName, r.Count)
		}
	}

	fmt.Fprintf(&sb, "\n## task_map per-query hits\n\n| query | hit | missing |\n|---|---|---|\n")
	for _, q := range b.TaskMapHit.PerQuery {
		miss := "—"
		if len(q.Missing) > 0 {
			miss = strings.Join(q.Missing, ", ")
		}
		fmt.Fprintf(&sb, "| `%s` | %.2f | %s |\n", escapePipe(q.Query), q.Hit, escapePipe(miss))
	}

	if len(b.SymbolRec.Missing) > 0 {
		fmt.Fprintf(&sb, "\n## Missing symbols (recall gaps)\n\n")
		for _, m := range b.SymbolRec.Missing {
			fmt.Fprintf(&sb, "- %s\n", m)
		}
	}

	if len(b.SymbolPrec.MissedSamples) > 0 {
		fmt.Fprintf(&sb, "\n## Precision miss samples\n\n")
		for _, m := range b.SymbolPrec.MissedSamples {
			fmt.Fprintf(&sb, "- %s\n", m)
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		must(err)
	}
	must(os.WriteFile(path, []byte(sb.String()), 0o644))
}

func escapePipe(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
