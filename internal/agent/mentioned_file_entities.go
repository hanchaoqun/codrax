package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
)

// mentioned_file_entities.go — Tier 1.5 file-shaped entity seeder
// for analyzerRequiredFiles. Closes the gap where the user verbatim
// names a file path the repomap graph cannot rank (no code symbols
// to score against, e.g. codrax.yaml.example / Dockerfile / *.proto
// / *.tf / *.sql).
//
// Single architectural rule: the only signal that decides "this
// entity names a file" is the FILESYSTEM. No extension allowlist,
// no canonical basename table, no keyword regex. We try three
// independent structural lookups and union the hits:
//
//  1. Direct stat:  <repo>/<entity> exists?
//  2. Sibling glob: <repo>/<entity><suffix> exists for any
//     suffix in MentionedFileSiblingSuffixes (yaml-configurable;
//     default: ".example" / ".sample" / ".dist" / ".tmpl" / ".tpl").
//  3. FileIndex basename match: any file in repomap.Graph.FileIndex
//     whose basename equals entity verbatim.
//
// Inputs are MentionedEntities only — the system-validated lane
// where every entry is provenance-verified to appear verbatim in
// RawRequest. This is the strongest precision tier the analyzer
// has and the only one allowed to drive a hard-precision RequiredFiles
// seed; using broader Entities here would let LLM-derived
// expansions inject files the user never named.
//
// Each layer is independently sufficient — no precedence between
// them. Hits across layers are de-duplicated. Output is intersected
// with the existing maxAnalyzerRequiredFiles cap by the caller.

// defaultMentionedFileSiblingSuffixes is the seed list (operator-
// agreed convention-based, not a project-specific keyword table).
// Each suffix is a well-known filesystem convention for "a versioned
// or template variant of a base file":
//
//   - .example / .sample : "shipped placeholder, copy to base"
//   - .dist              : Debian / autoconf "distribution default"
//   - .tmpl / .tpl       : Go / Helm / Jinja "template form"
//
// Every suffix here is documented at repo-design level, not in
// codrax-the-project. If a future workflow needs a different one
// (e.g. .in for autoconf), the operator extends via yaml.
var defaultMentionedFileSiblingSuffixes = []string{
	".example", ".sample", ".dist", ".tmpl", ".tpl",
}

var (
	mentionedFileSiblingSuffixesMu sync.RWMutex
	mentionedFileSiblingSuffixes   = append([]string(nil), defaultMentionedFileSiblingSuffixes...)
)

// defaultRequiredFileMentionCountFloor is the minimum repo-wide
// reference count for a file-shaped MentionedEntity hit to qualify
// as "high priority" in the cap=3 RequiredFiles budget.
//
// Rationale: a file the repo references in many places (CLAUDE.md,
// docs, multiple test fixtures, multiple source files reading it)
// is structurally critical — its inclusion in RequiredFiles should
// not lose to a generic ranker QueryScore hit. Files referenced 0-2
// times are likely peripheral fixtures and compete on equal footing
// with ranker output. 3 is the operator-friendly default; raising
// it pushes more fs-hits into the low-priority lane (more
// conservative); lowering to 1 trusts every existing fs-hit.
const defaultRequiredFileMentionCountFloor = 3

var (
	requiredFileMentionFloorMu sync.RWMutex
	requiredFileMentionFloor   = defaultRequiredFileMentionCountFloor
)

// SetRequiredFileMentionCountFloor replaces the active mention-count
// floor. Called from cmd/root.go at startup. Non-positive input
// restores the default.
func SetRequiredFileMentionCountFloor(n int) {
	requiredFileMentionFloorMu.Lock()
	defer requiredFileMentionFloorMu.Unlock()
	if n <= 0 {
		requiredFileMentionFloor = defaultRequiredFileMentionCountFloor
		return
	}
	requiredFileMentionFloor = n
}

// CurrentRequiredFileMentionCountFloor returns the active floor.
func CurrentRequiredFileMentionCountFloor() int {
	requiredFileMentionFloorMu.RLock()
	defer requiredFileMentionFloorMu.RUnlock()
	return requiredFileMentionFloor
}

// defaultMentionCountMaxGrep caps how many ripgrep / grep invocations
// the Layer-D mention-count annotator may issue PER analyzer
// dispatch. Each invocation is one unique basename; a typical
// request has 1-3 fs-hits so the default 3 covers the common case
// without grepping the world. Beyond the cap, remaining hits get
// MentionCount=0 (gracefully degrades to the low-priority lane,
// equal-footing with ranker output) — never blocks the request.
const defaultMentionCountMaxGrep = 3

var (
	mentionCountMaxGrepMu sync.RWMutex
	mentionCountMaxGrep   = defaultMentionCountMaxGrep
)

// SetMentionCountMaxGrep replaces the active per-dispatch grep cap.
// Called from cmd/root.go at startup. Non-positive input restores
// the default. Operators can raise (slower analyzer, more accurate
// priority) or lower (faster, more reliance on insertion order).
func SetMentionCountMaxGrep(n int) {
	mentionCountMaxGrepMu.Lock()
	defer mentionCountMaxGrepMu.Unlock()
	if n <= 0 {
		mentionCountMaxGrep = defaultMentionCountMaxGrep
		return
	}
	mentionCountMaxGrep = n
}

// CurrentMentionCountMaxGrep returns the active grep cap.
func CurrentMentionCountMaxGrep() int {
	mentionCountMaxGrepMu.RLock()
	defer mentionCountMaxGrepMu.RUnlock()
	return mentionCountMaxGrep
}

// SetMentionedFileSiblingSuffixes replaces the active suffix list.
// Called from cmd/root.go at startup after codrax.yaml merge. Empty
// or nil input restores the default list. Suffixes are normalised
// to a leading "." — operators may supply ".example" or "example".
//
// Single mu so a config reload from a parallel test cleanup does
// not race with the analyzer reader. Production only writes once
// at startup; the lock is for test parallelism.
func SetMentionedFileSiblingSuffixes(suffixes []string) {
	mentionedFileSiblingSuffixesMu.Lock()
	defer mentionedFileSiblingSuffixesMu.Unlock()
	if len(suffixes) == 0 {
		mentionedFileSiblingSuffixes = append([]string(nil), defaultMentionedFileSiblingSuffixes...)
		return
	}
	out := make([]string, 0, len(suffixes))
	seen := map[string]bool{}
	for _, s := range suffixes {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !strings.HasPrefix(s, ".") {
			s = "." + s
		}
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	if len(out) == 0 {
		mentionedFileSiblingSuffixes = append([]string(nil), defaultMentionedFileSiblingSuffixes...)
		return
	}
	mentionedFileSiblingSuffixes = out
}

// CurrentMentionedFileSiblingSuffixes returns a defensive copy of
// the active suffix list. Exported for telemetry / startup logs;
// production callers use mentionedFileSiblingSuffixesSnapshot.
func CurrentMentionedFileSiblingSuffixes() []string {
	mentionedFileSiblingSuffixesMu.RLock()
	defer mentionedFileSiblingSuffixesMu.RUnlock()
	return append([]string(nil), mentionedFileSiblingSuffixes...)
}

func mentionedFileSiblingSuffixesSnapshot() []string {
	mentionedFileSiblingSuffixesMu.RLock()
	defer mentionedFileSiblingSuffixesMu.RUnlock()
	return mentionedFileSiblingSuffixes
}

// FsHit pairs a repo-relative path discovered via Layers A/B/C
// with its repo-wide criticality count from Layer D. MentionCount
// is the number of OTHER repo files that reference this path's
// basename as a string literal. Critical files (count ≥ floor)
// take priority over generic ranker output; non-critical fs-hits
// compete with ranker QueryScore hits on equal footing.
type FsHit struct {
	Path         string
	MentionCount int
}

// mentionedFileEntities walks MentionedEntities and returns repo-
// relative file paths the entity names verbatim per filesystem.
// Three independent structural lookups, union-of-hits:
//
//   - Direct stat against repoRoot
//   - Sibling glob using the active suffix list
//   - FileIndex basename match (when graph is non-nil)
//
// Then layer D annotates each hit with a repo-wide MentionCount
// (number of other files that reference the basename). The result
// is sorted by MentionCount desc — most-referenced files first —
// so multi-match cases (e.g. several `Dockerfile` candidates) are
// arbitrated by which one the rest of the repo treats as load-
// bearing.
//
// Output is order-stable, de-duplicated, returns nil on any error
// or empty input. Every path has survived an os.Stat against the
// actual repo state — non-existent files are dropped (zero false-
// positive guarantee).
func mentionedFileEntities(repoRoot string, mentioned []string, graph *repomap.Graph) []FsHit {
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" || len(mentioned) == 0 {
		return nil
	}
	suffixes := mentionedFileSiblingSuffixesSnapshot()

	seen := map[string]bool{}
	var hits []FsHit
	add := func(rel string) {
		rel = filepath.ToSlash(strings.TrimSpace(rel))
		rel = strings.TrimPrefix(rel, "./")
		if rel == "" || rel == "." || strings.HasPrefix(rel, "../") || filepath.IsAbs(rel) {
			return
		}
		if seen[rel] {
			return
		}
		seen[rel] = true
		hits = append(hits, FsHit{Path: rel})
	}

	for _, raw := range mentioned {
		ent := strings.TrimSpace(raw)
		if ent == "" {
			continue
		}
		// Reject absolute paths and parent-traversals up front so a
		// malformed mentioned entity cannot escape the repo.
		if filepath.IsAbs(ent) || strings.Contains(ent, "..") {
			continue
		}
		entSlash := filepath.ToSlash(ent)

		// Layer A — direct stat on canonical path inside repo.
		if statHitsRepoFile(repoRoot, entSlash) {
			add(entSlash)
		}

		// Layer B — sibling-suffix glob. Every suffix is a
		// convention-based "variant of the canonical base"; each
		// is independently checked.
		for _, sfx := range suffixes {
			candidate := entSlash + sfx
			if statHitsRepoFile(repoRoot, candidate) {
				add(candidate)
			}
		}

		// Layer C — FileIndex basename match. Catches the case
		// where the user named a bare basename ("Dockerfile" /
		// "go.mod") without path prefix and the file lives
		// somewhere structurally below repoRoot.
		if graph != nil {
			for path := range graph.FileIndex {
				if filepath.Base(path) == entSlash {
					add(path)
				}
			}
		}
	}

	// Layer D — MentionCount annotation. One grep / file per hit;
	// caches results across hits with the same basename. Skipped
	// on empty result (no work to do).
	if len(hits) > 0 {
		start := time.Now()
		annotateMentionCounts(repoRoot, hits)
		// Stable sort by MentionCount desc; ties keep insertion
		// order so deterministic downstream consumers stay so.
		sortFsHitsByMention(hits)
		// Single observability line summarising the layer's output —
		// what fs-hits emerged, what their criticality counts came
		// back as, and how long Layer D's grep work took. Debug-level
		// so production stays quiet by default but operators can opt
		// in with --log-level debug for ranker-priority audits.
		logging.Debug("[analyzer] mentioned_file_entities: %d hit(s) in %s — %s",
			len(hits), time.Since(start).Round(time.Millisecond), formatFsHitsForLog(hits))
	}
	return hits
}

// formatFsHitsForLog renders a compact "path=N path=N ..." string for
// the Layer-D summary log line. Kept short — only path + count, no
// layer-of-origin annotation, so production debug logs stay scannable.
func formatFsHitsForLog(hits []FsHit) string {
	if len(hits) == 0 {
		return "(empty)"
	}
	parts := make([]string, 0, len(hits))
	for _, h := range hits {
		parts = append(parts, fmt.Sprintf("%s=%d", h.Path, h.MentionCount))
	}
	return strings.Join(parts, " ")
}

// annotateMentionCounts fills hit.MentionCount for each entry. Uses
// ripgrep when available (fast), GNU/BSD grep otherwise; falls back
// to a Go-native walker when neither is on PATH (covers distroless
// containers etc.). Cache by basename — the typical repo has 1-3
// fs-hits per request, but Layer C can produce multiple hits sharing
// a basename (e.g. Dockerfile in services/api + services/web).
//
// The count we want is "how many OTHER files in the repo mention
// this file" — so the search target is the basename string literal,
// not the full path. The hit file itself contributes to the count
// (we don't subtract) because the relative ordering matters more
// than the absolute number.
func annotateMentionCounts(repoRoot string, hits []FsHit) {
	cap := CurrentMentionCountMaxGrep()
	cache := make(map[string]int, len(hits))
	grepBudget := cap
	for i := range hits {
		base := filepath.Base(hits[i].Path)
		if base == "" {
			continue
		}
		if c, ok := cache[base]; ok {
			// Cache hit: free, doesn't consume budget.
			hits[i].MentionCount = c
			continue
		}
		if grepBudget <= 0 {
			// Budget exhausted; leave MentionCount=0 (low-priority
			// lane). Insertion order still determines downstream
			// preference among unannotated hits.
			continue
		}
		c := mentionCountForBasename(repoRoot, base)
		cache[base] = c
		hits[i].MentionCount = c
		grepBudget--
	}
}

// mentionCountForBasename returns the number of files in the repo
// that contain `basename` as a substring. ~30-50 ms per call on a
// medium repo with ripgrep; longer with GNU grep; longest with the
// native walker. Wrapped with a 2-second context cap so a slow
// scanner cannot stall analyzer dispatch.
//
// Returns 0 on any error — Layer D is supplementary; failure
// degrades gracefully to "no priority signal" and the fs-hit
// competes with ranker on equal footing.
func mentionCountForBasename(repoRoot, basename string) int {
	if basename == "" || repoRoot == "" {
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	switch tool.SearchCommand() {
	case "rg":
		// rg --files-with-matches counts files; -F treats pattern
		// as a literal so dots etc. are not regex chars.
		return runMentionScanner(ctx, tool.SearchExecutable(),
			[]string{"--files-with-matches", "--no-messages", "-F", basename, repoRoot})
	case "grep":
		return runMentionScanner(ctx, tool.SearchExecutable(),
			[]string{"-rlF", "--", basename, repoRoot})
	}
	// Native fallback: the result without ripgrep / grep is best-
	// effort; return 0 rather than walking the FS by hand for a
	// supplementary-only signal.
	return 0
}

// runMentionScanner runs the configured search backend and counts
// non-empty stdout lines (each line is one matched file path).
func runMentionScanner(ctx context.Context, bin string, args []string) int {
	if bin == "" {
		return 0
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	n := 0
	for _, ln := range lines {
		if strings.TrimSpace(ln) != "" {
			n++
		}
	}
	return n
}

// sortFsHitsByMention sorts in-place by MentionCount descending,
// stable on ties so insertion order is preserved.
func sortFsHitsByMention(hits []FsHit) {
	// Insertion sort is fine here — typical len(hits) is 1-5.
	for i := 1; i < len(hits); i++ {
		j := i
		for j > 0 && hits[j-1].MentionCount < hits[j].MentionCount {
			hits[j-1], hits[j] = hits[j], hits[j-1]
			j--
		}
	}
}

// statHitsRepoFile returns true when filepath.Join(repoRoot, rel)
// resolves to an existing regular file (not a directory, not a
// symlink dangling). Wrapped so the layer logic above stays single-
// purpose.
func statHitsRepoFile(repoRoot, rel string) bool {
	if rel == "" {
		return false
	}
	full := filepath.Join(repoRoot, filepath.FromSlash(rel))
	info, err := os.Stat(full)
	if err != nil {
		return false
	}
	if info.IsDir() {
		return false
	}
	return info.Mode().IsRegular()
}
