package multigraph

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/topology"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// BuildFunc is the function MultiGraph calls to load a sub-repo's
// *Graph on demand. The repomap facade supplies repomap.BuildOrLoadGraph
// here so multigraph stays free of import cycles on repomap.
type BuildFunc func(repoRoot, query string) (*rmtypes.Graph, error)

// Config configures New.
type Config struct {
	// Topology is the parent's discovery snapshot. MUST contain at
	// least one SubRepo.
	Topology *topology.RepoTopology

	// Build is the per-sub-repo graph loader. MUST be non-nil.
	Build BuildFunc

	// Query is forwarded to BuildFunc on every load — keeps the
	// rank.go QueryScores signal hot for the user's request.
	Query string

	// Cap is the LRU active-set ceiling. Zero or negative → 3.
	Cap int

	// FocusSlugs are user-pinned slugs (from REPL /repos focus).
	// Phase 4 routing fold treats these as "must active" and fails
	// loud if cap can't accommodate the focus + the routed
	// candidates.
	FocusSlugs []string

	// OracleFactory returns a SymbolOracle for a single sub-repo
	// *Graph. The repomap facade supplies repomap.NewSymbolOracle
	// here so multigraph stays free of import cycles on repomap.
	// Nil falls back to a graph-method-only oracle (SymbolExists
	// works; SymbolExistsFlat returns false — the flat index lives
	// in repomap.graphOracle, not in *Graph itself).
	OracleFactory func(*rmtypes.Graph) types.SymbolOracle

	// LocatorFactory mirrors OracleFactory for SymbolLocator. Nil
	// falls back to a graph-method-only locator.
	LocatorFactory func(*rmtypes.Graph) types.SymbolLocator
}

// MultiGraph is the multi-repo carrier. See package doc.
//
// Two operating modes:
//
//   - IsSingle()==true: topology has exactly one SubRepo at RootRel".".
//     All Z/Y access proxies to that one *Graph; behaviour is byte-
//     identical to direct *Graph access in pre-multi-repo code.
//   - IsSingle()==false: multi sub-repos. Z access (Oracle/Locator)
//     fans out across active graphs; Y access either flattens
//     (Files/ImportEdges) or owner-routes (FileInfoFor/ScoreFor) per
//     the design's audit table.
//
// All public methods are safe for concurrent use. Build calls
// (triggered by EnsureLoaded on cache miss) are serialised through
// the embedded mutex to keep the LRU + thrashing trackers
// consistent.
type MultiGraph struct {
	topo  *topology.RepoTopology
	build BuildFunc
	query string

	mu        sync.Mutex // guards active + thrash + focus
	active    *LRU
	thrash    *ThrashingTracker
	focusSet  map[string]bool

	oracleFactory  func(*rmtypes.Graph) types.SymbolOracle
	locatorFactory func(*rmtypes.Graph) types.SymbolLocator
}

// New constructs a MultiGraph from cfg. Returns an error if topology
// or build is nil.
func New(cfg Config) (*MultiGraph, error) {
	if cfg.Topology == nil {
		return nil, fmt.Errorf("multigraph.New: nil topology")
	}
	if cfg.Build == nil {
		return nil, fmt.Errorf("multigraph.New: nil build func")
	}
	if len(cfg.Topology.Repos) == 0 {
		return nil, fmt.Errorf("multigraph.New: topology has no sub-repos")
	}
	capN := cfg.Cap
	if capN <= 0 {
		capN = 3
	}
	// In single-repo mode the cap is forced to 1 — the LRU only
	// holds one entry and there's no thrashing surface.
	if cfg.Topology.IsSingle() {
		capN = 1
	}
	mg := &MultiGraph{
		topo:           cfg.Topology,
		build:          cfg.Build,
		query:          cfg.Query,
		active:         NewLRU(capN),
		thrash:         NewThrashingTracker(),
		focusSet:       make(map[string]bool, len(cfg.FocusSlugs)),
		oracleFactory:  cfg.OracleFactory,
		locatorFactory: cfg.LocatorFactory,
	}
	for _, slug := range cfg.FocusSlugs {
		mg.focusSet[slug] = true
	}
	return mg, nil
}

// === Topology read accessors ===

// IsSingle reports the degenerate single-repo posture.
func (m *MultiGraph) IsSingle() bool { return m != nil && m.topo.IsSingle() }

// Topology returns the underlying snapshot. Treat as read-only.
func (m *MultiGraph) Topology() *topology.RepoTopology {
	if m == nil {
		return nil
	}
	return m.topo
}

// Root returns the parent root absolute path.
func (m *MultiGraph) Root() string {
	if m == nil || m.topo == nil {
		return ""
	}
	return m.topo.ParentRoot
}

// SubRepos returns a copy of the topology's sub-repo list.
func (m *MultiGraph) SubRepos() []topology.SubRepo {
	if m == nil || m.topo == nil {
		return nil
	}
	out := make([]topology.SubRepo, len(m.topo.Repos))
	copy(out, m.topo.Repos)
	return out
}

// Cap returns the LRU active-set ceiling.
func (m *MultiGraph) Cap() int {
	if m == nil || m.active == nil {
		return 0
	}
	return m.active.Cap()
}

// Thrashing reports whether the LRU has crossed the eviction-rate
// threshold (default >5 evictions per 60s). Telemetry layer reads
// this to surface a fail-loud operator hint asking for a higher cap.
func (m *MultiGraph) Thrashing() bool {
	if m == nil || m.thrash == nil {
		return false
	}
	return m.thrash.Tripped()
}

// === Loading ===

// EnsureLoaded returns the *Graph for slug, loading and inserting
// into the LRU on cache miss. Eviction is recorded into the thrashing
// tracker.
func (m *MultiGraph) EnsureLoaded(slug string) (*rmtypes.Graph, error) {
	if m == nil {
		return nil, fmt.Errorf("multigraph.EnsureLoaded: nil receiver")
	}
	sr := m.topo.SubRepoBySlug(slug)
	if sr == nil {
		return nil, fmt.Errorf("multigraph.EnsureLoaded: no sub-repo with slug %q", slug)
	}
	// Fast path — already resident.
	if g, ok := m.active.Get(slug); ok {
		return g, nil
	}

	m.mu.Lock()
	// Re-check under the lock — another goroutine may have loaded
	// the same slug while we waited.
	if g, ok := m.active.Get(slug); ok {
		m.mu.Unlock()
		return g, nil
	}
	// We hold the write fence; the build call itself we run with
	// the lock held to keep eviction ordering deterministic. The
	// build is bounded by the per-sub-repo cache (warm second
	// run is ms-class), so this serialisation does not hurt
	// throughput in practice.
	g, err := m.build(sr.RootAbs, m.query)
	if err != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("multigraph.EnsureLoaded(%s): %w", slug, err)
	}
	if g == nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("multigraph.EnsureLoaded(%s): build returned nil graph", slug)
	}
	evictedSlug, _, evicted := m.active.Put(slug, g)
	if evicted {
		m.thrash.Record()
	}
	_ = evictedSlug // future telemetry hook
	m.mu.Unlock()
	return g, nil
}

// EnsureLoadedFor finds the SubRepo owning relPathFromParent and
// loads its graph.
func (m *MultiGraph) EnsureLoadedFor(relPathFromParent string) (*rmtypes.Graph, *topology.SubRepo, error) {
	sr := m.topo.Lookup(relPathFromParent)
	if sr == nil {
		return nil, nil, fmt.Errorf("multigraph.EnsureLoadedFor: no sub-repo owns %q", relPathFromParent)
	}
	g, err := m.EnsureLoaded(sr.Slug)
	return g, sr, err
}

// EnsureMany loads every slug in the input list. Returns
// ErrTooManyActive (fail-loud) if the request exceeds Cap — caller
// must fold first (design §3.4 R3 red line).
func (m *MultiGraph) EnsureMany(slugs []string) error {
	if m == nil {
		return fmt.Errorf("multigraph.EnsureMany: nil receiver")
	}
	if len(slugs) > m.Cap() {
		return fmt.Errorf("multigraph.EnsureMany: too many active slugs (%d > cap %d) — caller must routing-fold first (R3 red line)", len(slugs), m.Cap())
	}
	for _, slug := range slugs {
		if _, err := m.EnsureLoaded(slug); err != nil {
			return err
		}
	}
	return nil
}

// === Y-class accessors (raw field flatten / owner-aware) ===

// AllGraphs returns a slug→*Graph map of every active sub-repo. Used
// by per-sub-repo computation (rank, subgraph) and exhaustive iter
// consumers (explorer entity-discovery scans).
func (m *MultiGraph) AllGraphs() map[string]*rmtypes.Graph {
	if m == nil || m.active == nil {
		return nil
	}
	return m.active.Snapshot()
}

// GraphFor returns the *Graph for the SubRepo owning relPathFromParent
// IF that graph is currently active. The third return is true on
// active-hit. When the owning sub-repo exists in topology but is not
// active, returns (nil, sr, false) so callers can decide whether to
// EnsureLoad (avoiding implicit eviction).
func (m *MultiGraph) GraphFor(relPathFromParent string) (*rmtypes.Graph, *topology.SubRepo, bool) {
	if m == nil {
		return nil, nil, false
	}
	sr := m.topo.Lookup(relPathFromParent)
	if sr == nil {
		return nil, nil, false
	}
	if g, ok := m.active.Get(sr.Slug); ok {
		return g, sr, true
	}
	return nil, sr, false
}

// SubRepoRelPath converts a sub-repo-internal relPath into the
// equivalent path-from-parent — i.e. prefixes the SubRepo.RootRel.
// Returns the input unchanged when SubRepo.RootRel == ".".
func SubRepoRelPath(sr *topology.SubRepo, internal string) string {
	if sr == nil || sr.RootRel == "" || sr.RootRel == "." {
		return internal
	}
	return path.Join(sr.RootRel, internal)
}

// FlattenedFile pairs a *FileInfo with the SubRepo that owns it. The
// File field's RelPath has been rewritten to the path-from-parent
// form so cross-sub-repo dedup / sort work without collisions.
type FlattenedFile struct {
	File *rmtypes.FileInfo
	Sub  *topology.SubRepo
}

// Files flattens *FileInfo across every active sub-repo. The returned
// slice is fresh; callers may sort/filter freely.
//
// Note: the *FileInfo pointer is shared with the underlying *Graph —
// mutating its fields would taint the cached graph. Treat as
// read-only (this matches the existing single-repo contract).
func (m *MultiGraph) Files() []FlattenedFile {
	if m == nil {
		return nil
	}
	graphs := m.AllGraphs()
	if len(graphs) == 0 {
		return nil
	}
	// Build a slug → *SubRepo map once.
	subMap := make(map[string]*topology.SubRepo, len(m.topo.Repos))
	for i := range m.topo.Repos {
		subMap[m.topo.Repos[i].Slug] = &m.topo.Repos[i]
	}
	var total int
	for _, g := range graphs {
		total += len(g.Files)
	}
	out := make([]FlattenedFile, 0, total)
	for slug, g := range graphs {
		sr := subMap[slug]
		for _, fi := range g.Files {
			out = append(out, FlattenedFile{File: fi, Sub: sr})
		}
	}
	return out
}

// FileInfoFor looks up a single file by path-from-parent. The
// returned *FileInfo's RelPath is the sub-repo-internal form (matches
// what's stored in the *Graph index).
func (m *MultiGraph) FileInfoFor(relPathFromParent string) (*rmtypes.FileInfo, *topology.SubRepo, bool) {
	g, sr, hit := m.GraphFor(relPathFromParent)
	if !hit || g == nil {
		return nil, sr, false
	}
	internal := stripSubRepoPrefix(sr, relPathFromParent)
	if fi, ok := g.FileIndex[internal]; ok {
		return fi, sr, true
	}
	return nil, sr, false
}

// stripSubRepoPrefix removes sr.RootRel from path-from-parent,
// returning the sub-repo-internal relPath.
func stripSubRepoPrefix(sr *topology.SubRepo, relFromParent string) string {
	if sr == nil || sr.RootRel == "" || sr.RootRel == "." {
		return relFromParent
	}
	r := strings.TrimPrefix(relFromParent, sr.RootRel)
	r = strings.TrimPrefix(r, "/")
	return r
}

// ImportEdges flattens ImportGraph across every active sub-repo. Keys
// are path-from-parent on both sides; cross-sub-repo edges do NOT
// appear (design §3.5 — sub-repos have independent namespaces).
func (m *MultiGraph) ImportEdges() map[string][]string {
	return m.flattenStringSliceMap(func(g *rmtypes.Graph) map[string][]string { return g.ImportGraph })
}

// ReverseImportEdges flattens ReverseImports across every active sub-repo.
func (m *MultiGraph) ReverseImportEdges() map[string][]string {
	return m.flattenStringSliceMap(func(g *rmtypes.Graph) map[string][]string { return g.ReverseImports })
}

func (m *MultiGraph) flattenStringSliceMap(pick func(*rmtypes.Graph) map[string][]string) map[string][]string {
	graphs := m.AllGraphs()
	if len(graphs) == 0 {
		return nil
	}
	out := make(map[string][]string)
	subMap := make(map[string]*topology.SubRepo, len(m.topo.Repos))
	for i := range m.topo.Repos {
		subMap[m.topo.Repos[i].Slug] = &m.topo.Repos[i]
	}
	for slug, g := range graphs {
		sr := subMap[slug]
		src := pick(g)
		if src == nil {
			continue
		}
		for from, tos := range src {
			fromQ := SubRepoRelPath(sr, from)
			outTos := make([]string, len(tos))
			for i, t := range tos {
				outTos[i] = SubRepoRelPath(sr, t)
			}
			out[fromQ] = outTos
		}
	}
	return out
}

// ScoreFor returns the rank score for relPathFromParent, looking it
// up against the owning sub-repo's *Graph.Scores map. Returns 0 when
// the sub-repo is not active or the file is not scored.
func (m *MultiGraph) ScoreFor(relPathFromParent string) float64 {
	g, sr, hit := m.GraphFor(relPathFromParent)
	if !hit || g == nil {
		return 0
	}
	internal := stripSubRepoPrefix(sr, relPathFromParent)
	if v, ok := g.Scores[internal]; ok {
		return v
	}
	return 0
}

// QueryScoreFor returns the query-relevance score for relPathFromParent.
func (m *MultiGraph) QueryScoreFor(relPathFromParent string) float64 {
	g, sr, hit := m.GraphFor(relPathFromParent)
	if !hit || g == nil {
		return 0
	}
	internal := stripSubRepoPrefix(sr, relPathFromParent)
	if v, ok := g.QueryScores[internal]; ok {
		return v
	}
	return 0
}

// Metadata aggregates per-sub-repo metadata into one struct. File
// counts and language histograms sum across active graphs;
// SpecialFiles concatenates with sub-repo prefix (so collisions on
// "go.mod" between two Go sub-repos do not collapse to one entry).
// ScanTime is the LATEST scan time across active graphs.
func (m *MultiGraph) Metadata() rmtypes.Metadata {
	graphs := m.AllGraphs()
	if len(graphs) == 0 {
		return rmtypes.Metadata{}
	}
	out := rmtypes.Metadata{
		Languages: make(map[string]int),
	}
	subMap := make(map[string]*topology.SubRepo, len(m.topo.Repos))
	for i := range m.topo.Repos {
		subMap[m.topo.Repos[i].Slug] = &m.topo.Repos[i]
	}
	for slug, g := range graphs {
		sr := subMap[slug]
		out.FileCount += g.Metadata.FileCount
		for lang, n := range g.Metadata.Languages {
			out.Languages[lang] += n
		}
		for _, sf := range g.Metadata.SpecialFiles {
			out.SpecialFiles = append(out.SpecialFiles, SubRepoRelPath(sr, sf))
		}
		if g.Metadata.ScanTime.After(out.ScanTime) {
			out.ScanTime = g.Metadata.ScanTime
		}
	}
	return out
}

// === Z-class accessors (interface-layer fan-out) ===

// Oracle returns a SymbolOracle that fans out across every active
// sub-repo. Found = ANY active graph reports found; minTier is the
// minimum across all matches (most reliable). PartialTypedLane()
// surfaces whether some sub-repos were not consulted (cap-trimmed).
func (m *MultiGraph) Oracle() types.SymbolOracle {
	return &multiGraphOracle{mg: m}
}

// Locator returns a SymbolLocator that fans out across every active
// sub-repo. Each returned SymbolLocation's File is the path-from-parent
// (sub-repo prefix prepended) so callers comparing positions across
// sub-repos do not collide.
func (m *MultiGraph) Locator() types.SymbolLocator {
	return &multiGraphLocator{mg: m}
}

// PartialTypedLane reports whether any topology sub-repo is currently
// outside the active set. When true, Oracle/Locator queries may have
// missed symbols defined in inactive sub-repos — the LLM-facing
// summary MUST disclose this (R3 precise-vs-noisy red line).
func (m *MultiGraph) PartialTypedLane() bool {
	if m == nil {
		return false
	}
	if m.IsSingle() {
		return false
	}
	active := m.active.Slugs()
	if len(active) >= len(m.topo.Repos) {
		return false
	}
	return true
}

// PendingSubRepoNames returns the user-visible RootRel names for
// sub-repos NOT in the active set. Used by routing/telemetry to
// disclose what was skipped (R6 red line: surface RootRel, never
// the slug, to LLM-facing prompts).
func (m *MultiGraph) PendingSubRepoNames() []string {
	if m == nil || m.IsSingle() {
		return nil
	}
	activeSet := make(map[string]bool)
	for _, slug := range m.active.Slugs() {
		activeSet[slug] = true
	}
	var out []string
	for _, sr := range m.topo.Repos {
		if !activeSet[sr.Slug] {
			out = append(out, sr.RootRel)
		}
	}
	return out
}

// FocusSlugs returns the user-pinned slug set passed at construction.
// Phase 4 routing fold reads this so /repos focus pins survive across
// turns.
func (m *MultiGraph) FocusSlugs() []string {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.focusSet))
	for slug := range m.focusSet {
		out = append(out, slug)
	}
	return out
}

// === Single-repo degenerate helper ===

// Single returns the single-repo *Graph if IsSingle()==true, loading
// it on demand. Returns (nil, error) in multi-repo mode — callers
// have no business asking for "the" graph in that posture.
func (m *MultiGraph) Single() (*rmtypes.Graph, error) {
	if m == nil {
		return nil, fmt.Errorf("multigraph.Single: nil receiver")
	}
	if !m.IsSingle() {
		return nil, fmt.Errorf("multigraph.Single: topology has %d sub-repos (not single-repo posture)", len(m.topo.Repos))
	}
	return m.EnsureLoaded(m.topo.Repos[0].Slug)
}

// === path normalisation (kept here to avoid importing filepath in
// every call site; mirrors topology.Lookup's normalisation) ===

// normaliseRelFromParent strips leading "./" + leading "/" from a
// path so callers can pass either form.
func normaliseRelFromParent(rel string) string {
	rel = filepath.ToSlash(strings.TrimPrefix(rel, "./"))
	rel = strings.TrimPrefix(rel, "/")
	return rel
}
