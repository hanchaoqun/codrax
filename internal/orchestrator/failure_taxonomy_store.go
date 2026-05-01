package orchestrator

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// FailureTaxonomyStore is the in-memory + on-disk cache for
// per-repo failure pitfalls (stage 3). Lives at
// <cache-dir>/codrax/<repo-slug>/failure_taxonomy.json with
// deterministic per-repo namespacing so multi-repo workflows
// don't cross-contaminate.
//
// The store is the single read+write surface — reflector
// hands new patterns to Append (which dedups + decays), and
// the planner reads RelevantTo to score patterns against the
// current task. All disk ops use the atomic-rename pattern in
// types/failure_taxonomy_persist.go.
//
// Thread-safety: a per-store mutex guards every disk op.
// Cross-process safety relies on filesystem rename atomicity
// (POSIX); two parallel codrax runs against the same repo
// might lose one pattern emit on a tight race, which is
// acceptable for an advisory cache.
type FailureTaxonomyStore struct {
	mu        sync.Mutex
	cacheDir  string // absolute, includes <repo-slug> dir
	repoSlug  string
	loaded    bool
	taxonomy  *types.FailureTaxonomy
	maxItems  int           // soft cap; oldest low-confidence dropped past this
	decayDays int           // patterns older than this AND HitCount<2 dropped
}

// NewFailureTaxonomyStore constructs a store rooted at
// <cacheDir>/<repoSlug>/failure_taxonomy.json. cacheDir is
// the user-cache dir (typically ~/.codrax/cache); the
// per-repo subdir is created on first Save. Empty cacheDir
// or repoSlug disables the store (Append + Load become
// no-ops; this preserves single-shot CLI flows that don't
// want pitfall accumulation).
func NewFailureTaxonomyStore(cacheDir, repoSlug string, maxItems, decayDays int) *FailureTaxonomyStore {
	cacheDir = strings.TrimSpace(cacheDir)
	repoSlug = strings.TrimSpace(repoSlug)
	if cacheDir != "" && repoSlug != "" {
		cacheDir = filepath.Join(cacheDir, repoSlug)
	}
	if maxItems <= 0 {
		maxItems = 50
	}
	if decayDays <= 0 {
		decayDays = 90
	}
	return &FailureTaxonomyStore{
		cacheDir:  cacheDir,
		repoSlug:  repoSlug,
		maxItems:  maxItems,
		decayDays: decayDays,
	}
}

// taxonomyPath is the canonical on-disk JSON path. Empty
// when the store is disabled (cacheDir / repoSlug missing).
func (s *FailureTaxonomyStore) taxonomyPath() string {
	if s == nil || s.cacheDir == "" || s.repoSlug == "" {
		return ""
	}
	return filepath.Join(s.cacheDir, "failure_taxonomy.json")
}

// Enabled reports whether the store has the path slots wired
// up. Single-shot CLI flows construct a disabled store;
// callers check this before dispatching the reflector's
// emit_failure_pattern.
func (s *FailureTaxonomyStore) Enabled() bool {
	return s != nil && s.taxonomyPath() != ""
}

// Load fetches the taxonomy from disk (lazy, once-per-store
// lifetime). Subsequent calls return the cached in-memory
// copy without re-reading. Errors at load time return an
// empty taxonomy + log a warning — a corrupt cache file
// shouldn't block the Run from making progress.
func (s *FailureTaxonomyStore) Load() *types.FailureTaxonomy {
	if !s.Enabled() {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loaded {
		return s.taxonomy
	}
	t, err := types.LoadFailureTaxonomyFromFile(s.taxonomyPath())
	if err != nil {
		logging.Warning("[failure_taxonomy] load %s failed (treating as empty): %v",
			s.taxonomyPath(), err)
		t = nil
	}
	if t == nil {
		t = &types.FailureTaxonomy{
			RepoSlug:      s.repoSlug,
			SchemaVersion: types.CurrentSchemaVersion,
		}
	}
	s.taxonomy = t
	s.loaded = true
	return s.taxonomy
}

// Append inserts a new pattern with dedup-by-similarity and
// decay-on-overflow. Behaviour:
//
//   - If a similar pattern already exists (FailurePattern.SimilarTo),
//     bump its HitCount + LastSeen instead of appending.
//   - Otherwise append the pattern (assigns FirstSeen +
//     LastSeen if zero).
//   - Sweep old / low-confidence entries when the list
//     exceeds maxItems.
//   - Persist atomically.
//
// Returns the persisted pattern (which may be the new one
// OR the existing similar entry that got HitCount-bumped).
// Nil pattern in / disabled store => (nil, nil).
func (s *FailureTaxonomyStore) Append(pattern *types.FailurePattern) (*types.FailurePattern, error) {
	if !s.Enabled() || pattern == nil {
		return nil, nil
	}
	if !pattern.IsValid() {
		return nil, fmt.Errorf("FailureTaxonomyStore.Append: pattern fails validation: %+v", pattern)
	}
	t := s.Load()
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()

	// Dedup pass: bump HitCount on a similar entry if found.
	for i := range t.Patterns {
		if t.Patterns[i].SimilarTo(pattern) {
			t.Patterns[i].HitCount++
			t.Patterns[i].LastSeen = now
			// Lift confidence toward 1.0 on a recurrence:
			// the LLM saw the same pattern twice, that's
			// stronger signal than a single emit.
			if t.Patterns[i].Confidence < pattern.Confidence {
				t.Patterns[i].Confidence = pattern.Confidence
			}
			if err := s.persistLocked(); err != nil {
				return &t.Patterns[i], err
			}
			return &t.Patterns[i], nil
		}
	}

	// New entry. Stamp timestamps + ID if caller didn't.
	if pattern.FirstSeen.IsZero() {
		pattern.FirstSeen = now
	}
	if pattern.LastSeen.IsZero() {
		pattern.LastSeen = now
	}
	if pattern.HitCount == 0 {
		pattern.HitCount = 1
	}
	if strings.TrimSpace(pattern.ID) == "" {
		pattern.ID = fmt.Sprintf("fp-%d-%d", now.UnixNano(), os.Getpid())
	}
	t.Patterns = append(t.Patterns, *pattern)

	// Decay sweep: drop entries that are old AND have
	// HitCount < 2 (one-off observations); they were likely
	// noise from a single botched run.
	if s.decayDays > 0 {
		cutoff := now.AddDate(0, 0, -s.decayDays)
		filtered := t.Patterns[:0]
		for i := range t.Patterns {
			p := t.Patterns[i]
			if p.LastSeen.Before(cutoff) && p.HitCount < 2 {
				continue
			}
			filtered = append(filtered, p)
		}
		t.Patterns = filtered
	}

	// Overflow sweep: when over maxItems, drop the lowest
	// confidence × HitCount × recency score.
	if len(t.Patterns) > s.maxItems {
		sortPatternsByValue(t.Patterns, now)
		t.Patterns = t.Patterns[:s.maxItems]
	}

	// Final pattern reference for the caller — the slice may
	// have been re-sliced; index into the appended position.
	var saved *types.FailurePattern
	for i := range t.Patterns {
		if t.Patterns[i].ID == pattern.ID {
			saved = &t.Patterns[i]
			break
		}
	}
	if err := s.persistLocked(); err != nil {
		return saved, err
	}
	return saved, nil
}

// persistLocked writes the in-memory taxonomy to disk. Caller
// MUST hold s.mu.
func (s *FailureTaxonomyStore) persistLocked() error {
	if !s.Enabled() || s.taxonomy == nil {
		return nil
	}
	return types.WriteFailureTaxonomyToFile(s.taxonomy, s.taxonomyPath())
}

// RelevantTo returns the top-k patterns most relevant to the
// supplied task context, sorted by relevance score. Empty
// k caps to 5 (a sensible planner-prompt budget). Disabled
// store / empty taxonomy returns nil.
//
// Relevance score combines:
//   - Kind match (AppliesToKinds includes taskKind)  — 3.0
//   - Path overlap (any AppliesToPath prefix matches any
//     targetPath)                                    — 2.0 per match (cap 4.0)
//   - Confidence                                     — 1.0 base
//   - HitCount log-scaled                            — log(1+HitCount)
//   - Recency (decay 0..1 over decayDays)            — 0.5 weight
//
// Patterns scoring 0 are filtered out — the operator's
// pitfalls list isn't useful if every emit shows.
func (s *FailureTaxonomyStore) RelevantTo(taskKind string, targetPaths []string, k int) []types.FailurePattern {
	if !s.Enabled() {
		return nil
	}
	t := s.Load()
	if t == nil || len(t.Patterns) == 0 {
		return nil
	}
	if k <= 0 {
		k = 5
	}
	now := time.Now()
	type scored struct {
		p     types.FailurePattern
		score float64
	}
	out := make([]scored, 0, len(t.Patterns))
	for _, p := range t.Patterns {
		score := scorePatternRelevance(&p, taskKind, targetPaths, s.decayDays, now)
		if score <= 0 {
			continue
		}
		out = append(out, scored{p: p, score: score})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		// Tie-break: more recent wins.
		return out[i].p.LastSeen.After(out[j].p.LastSeen)
	})
	if len(out) > k {
		out = out[:k]
	}
	result := make([]types.FailurePattern, 0, len(out))
	for _, e := range out {
		result = append(result, e.p)
	}
	return result
}

// All returns a snapshot copy of every pattern in the
// taxonomy, sorted by recency descending. Used by /pitfalls
// list and tests.
func (s *FailureTaxonomyStore) All() []types.FailurePattern {
	if !s.Enabled() {
		return nil
	}
	t := s.Load()
	if t == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]types.FailurePattern, len(t.Patterns))
	copy(out, t.Patterns)
	sort.Slice(out, func(i, j int) bool {
		return out[i].LastSeen.After(out[j].LastSeen)
	})
	return out
}

// Clear wipes the taxonomy file from disk + zeroes the
// in-memory cache. Used by /pitfalls clear and tests.
// Idempotent — missing file is treated as success.
func (s *FailureTaxonomyStore) Clear() error {
	if !s.Enabled() {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(s.taxonomyPath()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("FailureTaxonomyStore.Clear: remove %s: %w", s.taxonomyPath(), err)
	}
	s.taxonomy = &types.FailureTaxonomy{
		RepoSlug:      s.repoSlug,
		SchemaVersion: types.CurrentSchemaVersion,
	}
	s.loaded = true
	return nil
}

// scorePatternRelevance computes the relevance heuristic. See
// RelevantTo for the score breakdown. Pure function for
// unit-testability.
func scorePatternRelevance(p *types.FailurePattern, taskKind string, targetPaths []string, decayDays int, now time.Time) float64 {
	if p == nil {
		return 0
	}
	score := 1.0 + p.Confidence
	// Kind match contributes only when AppliesToKinds is
	// constrained AND the task matches. Empty list = applies
	// to all (no boost, no penalty).
	if len(p.AppliesToKinds) > 0 {
		matched := false
		for _, k := range p.AppliesToKinds {
			if k == taskKind {
				matched = true
				break
			}
		}
		if !matched {
			// Constraint declared but task doesn't match —
			// drop to zero so unrelated pitfalls get
			// filtered out entirely.
			return 0
		}
		score += 3.0
	}
	// Path overlap (cap 4.0).
	if len(p.AppliesToPaths) > 0 {
		hits := 0
		for _, prefix := range p.AppliesToPaths {
			for _, tp := range targetPaths {
				if strings.HasPrefix(tp, prefix) {
					hits++
					break
				}
			}
		}
		add := float64(hits) * 2.0
		if add > 4.0 {
			add = 4.0
		}
		score += add
	}
	// HitCount log-scaled.
	if p.HitCount > 1 {
		score += loglikeBonus(p.HitCount)
	}
	// Recency: 1.0 at LastSeen=now, 0.0 at LastSeen=now-decayDays.
	if decayDays > 0 && !p.LastSeen.IsZero() {
		age := now.Sub(p.LastSeen).Hours() / 24.0
		recency := 1.0 - age/float64(decayDays)
		if recency < 0 {
			recency = 0
		}
		score += 0.5 * recency
	}
	return score
}

// loglikeBonus is a cheap log-shape scoring contribution for
// HitCount: 2 → 0.69, 5 → 1.61, 10 → 2.30, 100 → 4.60. Used
// instead of math.Log to keep the package import surface tight
// (no runtime maths import).
func loglikeBonus(hitCount int) float64 {
	if hitCount <= 1 {
		return 0
	}
	// Approximation: 0.69 + 0.4 * (hitCount-2) capped at ~3.
	approx := 0.69 + 0.4*float64(hitCount-2)
	if approx > 3.0 {
		approx = 3.0
	}
	return approx
}

// sortPatternsByValue is the overflow-sweep ordering — least
// valuable first, so the truncation at maxItems drops the
// noise. Value combines Confidence + log(HitCount) + recency
// over decayDays.
func sortPatternsByValue(patterns []types.FailurePattern, now time.Time) {
	sort.Slice(patterns, func(i, j int) bool {
		return patternValue(&patterns[i], now) > patternValue(&patterns[j], now)
	})
}

func patternValue(p *types.FailurePattern, now time.Time) float64 {
	if p == nil {
		return 0
	}
	v := p.Confidence + loglikeBonus(p.HitCount)
	if !p.LastSeen.IsZero() {
		ageDays := now.Sub(p.LastSeen).Hours() / 24.0
		v += 1.0 / (1.0 + ageDays/30.0)
	}
	// fnv hash of ID for deterministic tie-break — purely
	// cosmetic but keeps test output stable.
	h := fnv.New32a()
	_, _ = h.Write([]byte(p.ID))
	v += float64(h.Sum32()%1000) / 1e6
	return v
}
