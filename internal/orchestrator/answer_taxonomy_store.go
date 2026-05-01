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

// AnswerTaxonomyStore is the in-memory + on-disk cache for
// per-repo answer pitfalls (commit 51 Gap 3, mirror of
// FailureTaxonomyStore). Lives at <cache-dir>/codrax/<repo-slug>/
// answer_taxonomy.json with deterministic per-repo namespacing.
//
// The store is the single read+write surface — answer_reviewer
// hands new patterns to Append (which dedups + decays), and the
// analyzer reads RelevantTo to score patterns against the current
// request. All disk ops use the atomic-rename pattern in
// types/answer_taxonomy_persist.go.
type AnswerTaxonomyStore struct {
	mu        sync.Mutex
	cacheDir  string
	repoSlug  string
	loaded    bool
	taxonomy  *types.AnswerTaxonomy
	maxItems  int
	decayDays int
	// exampleFileExists (commit 60 Batch E.2, audit MEDIUM #11)
	// validates whether a pattern's ExampleFile still exists in
	// the repo. Nil = no validity check (back-compat). Set via
	// SetExampleFileValidator at startup.
	exampleFileExists func(relPath string) bool
}

// NewAnswerTaxonomyStore constructs a store rooted at
// <cacheDir>/<repoSlug>/answer_taxonomy.json. Empty cacheDir or
// repoSlug disables the store (Append + Load become no-ops).
func NewAnswerTaxonomyStore(cacheDir, repoSlug string, maxItems, decayDays int) *AnswerTaxonomyStore {
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
	return &AnswerTaxonomyStore{
		cacheDir:  cacheDir,
		repoSlug:  repoSlug,
		maxItems:  maxItems,
		decayDays: decayDays,
	}
}

func (s *AnswerTaxonomyStore) taxonomyPath() string {
	if s == nil || s.cacheDir == "" || s.repoSlug == "" {
		return ""
	}
	return filepath.Join(s.cacheDir, "answer_taxonomy.json")
}

// Enabled reports whether the store has the path slots wired up.
func (s *AnswerTaxonomyStore) Enabled() bool {
	return s != nil && s.taxonomyPath() != ""
}

// Load fetches the taxonomy from disk (lazy, once-per-store
// lifetime). Subsequent calls return the cached in-memory copy.
func (s *AnswerTaxonomyStore) Load() *types.AnswerTaxonomy {
	if !s.Enabled() {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loaded {
		return s.taxonomy
	}
	t, err := types.LoadAnswerTaxonomyFromFile(s.taxonomyPath())
	if err != nil {
		logging.Warning("[answer_taxonomy] load %s failed (treating as empty): %v",
			s.taxonomyPath(), err)
		t = nil
	}
	if t == nil {
		t = &types.AnswerTaxonomy{
			RepoSlug:      s.repoSlug,
			SchemaVersion: types.CurrentAnswerSchemaVersion,
		}
	}
	s.taxonomy = t
	s.loaded = true
	return s.taxonomy
}

// Append inserts a new pattern with dedup-by-similarity and
// decay-on-overflow. Behaviour mirrors FailureTaxonomyStore.Append.
func (s *AnswerTaxonomyStore) Append(pattern *types.AnswerPattern) (*types.AnswerPattern, error) {
	if !s.Enabled() || pattern == nil {
		return nil, nil
	}
	if !pattern.IsValid() {
		return nil, fmt.Errorf("AnswerTaxonomyStore.Append: pattern fails validation: %+v", pattern)
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
			if t.Patterns[i].Confidence < pattern.Confidence {
				t.Patterns[i].Confidence = pattern.Confidence
			}
			if err := s.persistLocked(); err != nil {
				return &t.Patterns[i], err
			}
			return &t.Patterns[i], nil
		}
	}

	// New entry. Stamp timestamps + ID.
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
		pattern.ID = fmt.Sprintf("ap-%d-%d", now.UnixNano(), os.Getpid())
	}
	t.Patterns = append(t.Patterns, *pattern)

	// Decay sweep: drop entries that are old AND have HitCount < 2.
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

	// Overflow sweep: when over maxItems, drop the lowest value.
	if len(t.Patterns) > s.maxItems {
		sortAnswerPatternsByValue(t.Patterns, now)
		t.Patterns = t.Patterns[:s.maxItems]
	}

	var saved *types.AnswerPattern
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

func (s *AnswerTaxonomyStore) persistLocked() error {
	if !s.Enabled() || s.taxonomy == nil {
		return nil
	}
	return types.WriteAnswerTaxonomyToFile(s.taxonomy, s.taxonomyPath())
}

// RelevantTo returns the top-k patterns most relevant to the
// supplied request context, sorted by relevance score. Empty k
// caps to 5. Mirror of FailureTaxonomyStore.RelevantTo.
//
// Relevance score combines:
//   - Scenario match (AppliesToScenarios includes scenario)  — 3.0
//   - Shape match (AppliesToShapes includes shape)           — 2.0
//   - Confidence                                             — 1.0 base
//   - HitCount log-scaled                                    — log(1+HitCount)
//   - Recency (decay 0..1 over decayDays)                    — 0.5 weight
//
// Commit 60 Batch E.2 (audit MEDIUM #11): patterns whose
// ExampleFile no longer exists in the repo are skipped (the
// codebase moved on, the historic pitfall no longer applies).
// Caller can supply repoRoot via SetExampleFileValidator at
// startup; nil validator = no validity check (pre-commit-60
// behaviour).
func (s *AnswerTaxonomyStore) RelevantTo(scenario, shape string, k int) []types.AnswerPattern {
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
		p     types.AnswerPattern
		score float64
	}
	out := make([]scored, 0, len(t.Patterns))
	for _, p := range t.Patterns {
		// Stale-pattern guard: when ExampleFile is named AND the
		// validator says it no longer exists, skip the pattern.
		// The pattern still lives on disk (operators can /answer-
		// pitfalls show <id> to inspect history); just not injected
		// into the analyzer's prompt.
		if !s.exampleFileValid(&p) {
			continue
		}
		score := scoreAnswerPatternRelevance(&p, scenario, shape, s.decayDays, now)
		if score <= 0 {
			continue
		}
		out = append(out, scored{p: p, score: score})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		return out[i].p.LastSeen.After(out[j].p.LastSeen)
	})
	if len(out) > k {
		out = out[:k]
	}
	result := make([]types.AnswerPattern, 0, len(out))
	for _, e := range out {
		result = append(result, e.p)
	}
	return result
}

// SetExampleFileValidator installs a validator that the store
// uses to spot-check whether a pattern's ExampleFile still
// exists. Called from cmd/root.go at startup with a closure that
// resolves repo-relative paths against the current --repo arg.
// Empty validator = no stale-pattern eviction (pre-commit-60
// behaviour preserved when the orchestrator doesn't know the
// repo root).
func (s *AnswerTaxonomyStore) SetExampleFileValidator(fn func(relPath string) bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.exampleFileExists = fn
}

// exampleFileValid runs the validator (if installed) against the
// pattern's ExampleFile. Returns true when the pattern's
// ExampleFile is empty (caller didn't pin a concrete file) OR
// when the validator says the file exists. Returns false ONLY
// when validator is installed AND the file is gone.
func (s *AnswerTaxonomyStore) exampleFileValid(p *types.AnswerPattern) bool {
	if p == nil || strings.TrimSpace(p.ExampleFile) == "" {
		return true
	}
	s.mu.Lock()
	fn := s.exampleFileExists
	s.mu.Unlock()
	if fn == nil {
		return true
	}
	return fn(p.ExampleFile)
}

// All returns a snapshot copy of every pattern, sorted by recency
// descending. Used by /answer-pitfalls list and tests.
func (s *AnswerTaxonomyStore) All() []types.AnswerPattern {
	if !s.Enabled() {
		return nil
	}
	t := s.Load()
	if t == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]types.AnswerPattern, len(t.Patterns))
	copy(out, t.Patterns)
	sort.Slice(out, func(i, j int) bool {
		return out[i].LastSeen.After(out[j].LastSeen)
	})
	return out
}

// Clear wipes the taxonomy file from disk + zeroes the in-memory
// cache. Idempotent — missing file is treated as success.
func (s *AnswerTaxonomyStore) Clear() error {
	if !s.Enabled() {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(s.taxonomyPath()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("AnswerTaxonomyStore.Clear: remove %s: %w", s.taxonomyPath(), err)
	}
	s.taxonomy = &types.AnswerTaxonomy{
		RepoSlug:      s.repoSlug,
		SchemaVersion: types.CurrentAnswerSchemaVersion,
	}
	s.loaded = true
	return nil
}

func scoreAnswerPatternRelevance(p *types.AnswerPattern, scenario, shape string, decayDays int, now time.Time) float64 {
	if p == nil {
		return 0
	}
	score := 1.0 + p.Confidence
	if len(p.AppliesToScenarios) > 0 {
		matched := false
		for _, s := range p.AppliesToScenarios {
			if s == scenario {
				matched = true
				break
			}
		}
		if !matched {
			return 0
		}
		score += 3.0
	}
	if len(p.AppliesToShapes) > 0 {
		matched := false
		for _, sh := range p.AppliesToShapes {
			if sh == shape {
				matched = true
				break
			}
		}
		if !matched {
			return 0
		}
		score += 2.0
	}
	if p.HitCount > 1 {
		score += loglikeBonus(p.HitCount)
	}
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

func sortAnswerPatternsByValue(patterns []types.AnswerPattern, now time.Time) {
	sort.Slice(patterns, func(i, j int) bool {
		return answerPatternValue(&patterns[i], now) > answerPatternValue(&patterns[j], now)
	})
}

func answerPatternValue(p *types.AnswerPattern, now time.Time) float64 {
	if p == nil {
		return 0
	}
	v := p.Confidence + loglikeBonus(p.HitCount)
	if !p.LastSeen.IsZero() {
		ageDays := now.Sub(p.LastSeen).Hours() / 24.0
		v += 1.0 / (1.0 + ageDays/30.0)
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(p.ID))
	v += float64(h.Sum32()%1000) / 1e6
	return v
}
