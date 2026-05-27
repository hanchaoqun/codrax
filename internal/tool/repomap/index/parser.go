package index

import (
	"errors"
	"math"
	"os"
	"runtime/debug"
	"sort"
	"sync"
	"time"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

const (
	treeSitterSlowParseWarnAfter  = 10 * time.Second
	defaultTreeSitterParseTimeout = 2 * time.Minute
	parseWorkerMemoryBudget       = 512 << 20
	parseBufferPerWorker          = 2
)

var treeSitterParseTimeout = defaultTreeSitterParseTimeout

// SetTreeSitterParseTimeout caps a single tree-sitter parse. Zero disables the
// safety valve. Called from startup config before scans begin; tests may adjust
// it with restoreTreeSitterParseTimeout.
func SetTreeSitterParseTimeout(d time.Duration) {
	if d < 0 {
		d = defaultTreeSitterParseTimeout
	}
	treeSitterParseTimeout = d
}

// ParseFiles parses all entries in parallel and returns types.FileInfo results.
// Unparseable files (unsupported language, read errors) are included with
// basic metadata but no symbols.
func ParseFiles(entries []FileEntry, repoRoot string) []*types.FileInfo {
	return ParseFilesWithProgress(entries, repoRoot, nil)
}

// ParseFilesWithProgress is ParseFiles plus a best-effort completion
// callback. The callback receives parsed/total counts on the collector
// goroutine, so callers can throttle UI notifications without adding
// locks to the worker hot path.
func ParseFilesWithProgress(entries []FileEntry, repoRoot string, progress func(done, total int)) []*types.FileInfo {
	infos, _ := ParseFilesWithProgressAndSink(entries, repoRoot, progress, nil)
	return infos
}

// ParseFilesWithProgressAndSink streams parsed FileInfo records to sink
// in repository scan order as soon as contiguous results are available.
// This lets large full scans persist cache chunks while parser workers
// are still running instead of building one huge JSON blob at the end.
// A sink error does not stop parsing; it is returned after all results
// are collected so callers can keep serving the current in-memory graph
// and merely degrade cache persistence.
func ParseFilesWithProgressAndSink(entries []FileEntry, repoRoot string, progress func(done, total int), sink func(*types.FileInfo) error) ([]*types.FileInfo, error) {
	return ParseFilesWithProgressSinkAndActive(entries, repoRoot, progress, sink, nil)
}

// ParseFilesWithProgressSinkAndActive is ParseFilesWithProgressAndSink plus a
// local-only active-file callback. It does not alter parsing semantics: the
// callback exists solely so UIs can explain long tail work on very large source
// files instead of appearing stuck at N-1/N.
func ParseFilesWithProgressSinkAndActive(entries []FileEntry, repoRoot string, progress func(done, total int), sink func(*types.FileInfo) error, active func(FileEntry)) ([]*types.FileInfo, error) {
	// Bound the worker pool to the scan CPU budget so a full scan of a
	// huge repository leaves reserved cores free for the host (see
	// ApplyScanGOMAXPROCS, which caps the whole runtime to match).
	// Also respect the active Go soft heap limit: many simultaneous
	// tree-sitter parses create transient file-byte/AST pressure, so
	// low-memory hosts should prefer fewer parser workers over GC churn.
	if len(entries) == 0 {
		return nil, nil
	}
	workers := parseWorkerBudget(entries)

	type result struct {
		idx  int
		info *types.FileInfo
	}

	buffer := parseChannelBufferSize(len(entries), workers)
	jobs := make(chan int, buffer)
	results := make(chan result, buffer)

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				stopActive := startActiveFileHeartbeat(entries[idx], active)
				fi := parseOneFile(entries[idx])
				stopActive()
				results <- result{idx: idx, info: fi}
			}
		}()
	}

	go func() {
		for _, idx := range parseJobOrder(entries) {
			jobs <- idx
		}
		close(jobs)
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	infos := make([]*types.FileInfo, len(entries))
	ready := make([]bool, len(entries))
	done := 0
	nextSink := 0
	var sinkErr error
	for r := range results {
		infos[r.idx] = r.info
		ready[r.idx] = true
		done++
		for sink != nil && nextSink < len(infos) && ready[nextSink] {
			if sinkErr == nil {
				sinkErr = sink(infos[nextSink])
			}
			nextSink++
		}
		if progress != nil {
			progress(done, len(entries))
		}
	}

	// filter nil (shouldn't happen but be safe)
	out := make([]*types.FileInfo, 0, len(infos))
	for _, fi := range infos {
		if fi != nil {
			out = append(out, fi)
		}
	}
	return out, sinkErr
}

func parseWorkerBudget(entries []FileEntry) int {
	workers := scanCPUBudget()
	if total := len(entries); total > 0 && workers > total {
		workers = total
	}
	if workers < 1 {
		workers = 1
	}
	limit := debug.SetMemoryLimit(-1)
	if limit <= 0 || limit == math.MaxInt64 {
		return workers
	}
	memWorkers := int(limit / parseWorkerMemoryBudget)
	if memWorkers < 1 {
		memWorkers = 1
	}
	if memWorkers < workers {
		return memWorkers
	}
	return workers
}

func parseChannelBufferSize(total, workers int) int {
	if total <= 0 {
		return 0
	}
	if workers < 1 {
		workers = 1
	}
	n := workers * parseBufferPerWorker
	if n < 1 {
		n = 1
	}
	if n > total {
		n = total
	}
	return n
}

func startActiveFileHeartbeat(entry FileEntry, active func(FileEntry)) func() {
	if active == nil {
		return func() {}
	}
	if entry.Size >= 1<<20 {
		active(entry)
	}
	done := make(chan struct{})
	go func() {
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		ticker := (*time.Ticker)(nil)
		for {
			select {
			case <-timerChan(timer):
				active(entry)
				ticker = time.NewTicker(5 * time.Second)
				timer = nil
			case <-tickerChan(ticker):
				active(entry)
			case <-done:
				if ticker != nil {
					ticker.Stop()
				}
				return
			}
		}
	}()
	return func() { close(done) }
}

func tickerChan(t *time.Ticker) <-chan time.Time {
	if t == nil {
		return nil
	}
	return t.C
}

func timerChan(t *time.Timer) <-chan time.Time {
	if t == nil {
		return nil
	}
	return t.C
}

// BasicFileInfo returns metadata for files that repomap deliberately
// indexes but does not parse into symbols. It still records the content
// hash so cache invalidation does not treat every unsupported file as
// changed forever.
func BasicFileInfo(entry FileEntry) *types.FileInfo {
	hash, _ := hashFile(entry.AbsPath)
	fi := &types.FileInfo{
		RelPath:  entry.RelPath,
		Language: entry.Language,
		Size:     entry.Size,
		Hash:     hash,
	}
	if ok, stype := IsSpecialFile(entry.RelPath); ok {
		fi.IsSpecial = true
		fi.SpecialType = stype
	}
	return fi
}

func parseJobOrder(entries []FileEntry) []int {
	order := make([]int, len(entries))
	for i := range entries {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		return entries[order[i]].Size > entries[order[j]].Size
	})
	return order
}

func parseOneFile(entry FileEntry) *types.FileInfo {
	source, err := os.ReadFile(entry.AbsPath)
	if err != nil {
		return &types.FileInfo{
			RelPath:  entry.RelPath,
			Language: entry.Language,
			Size:     entry.Size,
			Hash:     "",
		}
	}

	fi := &types.FileInfo{
		RelPath:  entry.RelPath,
		Language: entry.Language,
		Size:     entry.Size,
		Hash:     contentHash(source),
	}

	// Check for special files
	if ok, stype := IsSpecialFile(entry.RelPath); ok {
		fi.IsSpecial = true
		fi.SpecialType = stype
	}

	// HarmonyOS dual-language dispatch. Both paths own their own
	// tree-sitter / regex pipelines + tier assignment, so we
	// short-circuit before the generic tree-sitter block.
	//
	//   - ArkTS (LangArkTS): tree-sitter-typescript + ArkTS
	//     post-pass (see extract_arkts.go). Tier 1 = TS grammar
	//     parsed successfully; Tier 2 = regex-only salvage;
	//     Tier 3 = path-only.
	//   - Cangjie (LangCangjie): Go-native scanner (no tree-sitter
	//     grammar exists). Tier 1 = state-aware scanner; Tier 2 =
	//     raw regex salvage; Tier 3 = path-only.
	//
	// Tier + reason are recorded on FileInfo via recordFallback so
	// the retrieve.rank layer can discount lower-confidence parses
	// and the build log surfaces degradations (red line L-Fallback-1).
	switch entry.Language {
	case types.LangArkTS:
		pkg, syms, imps, rels, tier := extractArkTS(source, entry.RelPath)
		fi.Package = pkg
		fi.Symbols = syms
		fi.Imports = imps
		fi.Relations = rels
		if tier > 1 {
			recordFallback(fi, 1, tier, "arkts extractor downgraded")
		} else {
			fi.ParseTier = 1
		}
		return fi
	case types.LangCangjie:
		pkg, syms, imps, rels, tier := extractCangjie(source, entry.RelPath)
		fi.Package = pkg
		fi.Symbols = syms
		fi.Imports = imps
		fi.Relations = rels
		if tier > 1 {
			recordFallback(fi, 1, tier, "cangjie extractor downgraded")
		} else {
			fi.ParseTier = 1
		}
		return fi
	}

	// If language is unsupported, return basic info
	lang := types.GetSitterLanguage(entry.Language)
	if lang == nil {
		return fi
	}

	root, elapsed, err := parseTreeSitterRoot(entry.Language, source)
	if elapsed >= treeSitterSlowParseWarnAfter {
		logging.Warning("repomap: slow parse %s %s size=%d elapsed=%s", entry.Language, entry.RelPath, entry.Size, elapsed.Round(time.Millisecond))
	}
	if err != nil {
		if errors.Is(err, errTreeSitterParseTimeout) {
			recordFallback(fi, 1, 4, err.Error())
		}
		return fi
	}
	if root == nil {
		return fi
	}

	switch entry.Language {
	case types.LangGo:
		fi.Package, fi.Symbols, fi.Imports, fi.Relations = extractGo(root, source, entry.RelPath)
	case types.LangPython:
		fi.Package, fi.Symbols, fi.Imports, fi.Relations = extractPython(root, source, entry.RelPath)
	case types.LangJavaScript:
		fi.Package, fi.Symbols, fi.Imports, fi.Relations = extractJS(root, source, entry.RelPath, false)
	case types.LangTypeScript:
		fi.Package, fi.Symbols, fi.Imports, fi.Relations = extractJS(root, source, entry.RelPath, true)
	case types.LangJava:
		fi.Package, fi.Symbols, fi.Imports, fi.Relations = extractJava(root, source, entry.RelPath)
	case types.LangKotlin:
		fi.Package, fi.Symbols, fi.Imports, fi.Relations = extractKotlin(root, source, entry.RelPath)
	case types.LangRust:
		fi.Package, fi.Symbols, fi.Imports, fi.Relations = extractRust(root, source, entry.RelPath)
	case types.LangC, types.LangCpp:
		fi.Package, fi.Symbols, fi.Imports, fi.Relations = extractCCpp(root, source, entry.RelPath, entry.Language)
	case types.LangRuby:
		fi.Package, fi.Symbols, fi.Imports, fi.Relations = extractRuby(root, source, entry.RelPath)
	case types.LangSwift:
		fi.Package, fi.Symbols, fi.Imports, fi.Relations = extractSwift(root, source, entry.RelPath)
	case types.LangLua:
		fi.Package, fi.Symbols, fi.Imports, fi.Relations = extractLua(root, source, entry.RelPath)
	case types.LangProto:
		fi.Package, fi.Symbols, fi.Imports, fi.Relations = extractProto(root, source, entry.RelPath)
	}

	// Phase 6 stage 18 (2026-05-03) — populate per-line typed AST
	// node-shape features. Generic across languages: the walker
	// reads tree-sitter node Type() names and maps the closed set
	// (return_statement / call_expression / new_expression /
	// arrow_function / composite_literal / etc.) to typed
	// LineFeature enum values. Languages whose grammar uses
	// different node names skip silently — callers treat absence
	// as "no signal" rather than guessing via byte tokens.
	fi.LineFeatures = extractLineFeatures(root, source)

	// Phase 6 stage 21 (2026-05-03) — populate Symbol.ReturnTypeNames
	// for every function-like declaration across all languages.
	// Single post-pass walks the AST, finds every
	// function/method/lambda node via isFunctionNodeKind, extracts
	// its return-type names via extractReturnTypeNames, and matches
	// to the corresponding Symbol by line range. Languages whose
	// grammar uses different return-type field names degrade to
	// empty ReturnTypeNames (typed-only contract). Existing inline
	// wiring (goExtractFunc / goExtractMethod) is preserved — the
	// post-pass only sets the field when currently empty.
	backfillReturnTypeNames(root, source, fi.Symbols)

	return fi
}

// --- helpers ---

func nodeText(node *sitter.Node, source []byte) string {
	if node == nil {
		return ""
	}
	return node.Content(source)
}

func nodeLine(node *sitter.Node) int {
	if node == nil {
		return 0
	}
	return int(node.StartPoint().Row) + 1 // 1-based
}

func nodeEndLine(node *sitter.Node) int {
	if node == nil {
		return 0
	}
	return int(node.EndPoint().Row) + 1
}

// childByType finds the first direct child with the given type.
func childByType(node *sitter.Node, nodeType string) *sitter.Node {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		ch := node.NamedChild(i)
		if ch.Type() == nodeType {
			return ch
		}
	}
	return nil
}

// childrenByType returns all direct children with the given type.
func childrenByType(node *sitter.Node, nodeType string) []*sitter.Node {
	var out []*sitter.Node
	for i := 0; i < int(node.NamedChildCount()); i++ {
		ch := node.NamedChild(i)
		if ch.Type() == nodeType {
			out = append(out, ch)
		}
	}
	return out
}

// prevSiblingComment returns the text of an immediately preceding comment.
func prevSiblingComment(node *sitter.Node, source []byte) string {
	prev := node.PrevSibling()
	if prev == nil {
		return ""
	}
	t := prev.Type()
	if t == "comment" || t == "line_comment" || t == "block_comment" {
		text := nodeText(prev, source)
		return firstLine(text)
	}
	return ""
}

func firstLine(s string) string {
	for i, c := range s {
		if c == '\n' {
			return s[:i]
		}
	}
	return s
}

// walkNamedChildren calls fn for every named child, recursively if recurse is true.
func walkNamedChildren(node *sitter.Node, recurse bool, fn func(*sitter.Node)) {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		ch := node.NamedChild(i)
		fn(ch)
		if recurse {
			walkNamedChildren(ch, true, fn)
		}
	}
}
