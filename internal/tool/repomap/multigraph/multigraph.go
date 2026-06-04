package multigraph

import (
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	rmrelation "github.com/hanchaoqun/codrax/internal/tool/repomap/relation"
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

	// Cap is the LRU active-set ceiling. Zero or negative → 2.
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

	// ScanNotifier is an optional callback fired around every
	// EnsureLoaded build call. Phase 4 (2026-05-08) uses it to
	// surface per-sub-repo scan progress to the dock (one push
	// notice per scan via EventOrchestratorNotice). started=true is
	// the start event; ok / elapsedMs apply to the end event. Nil
	// disables the notification path entirely (single-shot CLI
	// runs / tests). Callbacks fire synchronously from the loader
	// goroutine, outside the MultiGraph mutex, so consumers should
	// still keep them non-blocking.
	ScanNotifier func(rootRel, slug string, started bool, ok bool, elapsedMs int64)

	// WaitNotifier is fired when a caller joins an already in-flight
	// same-slug graph load. The real build continues to emit the
	// normal repo_map scan phases; this event only explains why the
	// joining caller is waiting instead of launching duplicate work.
	WaitNotifier func(types.RepoMapScanEvent)
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
// All public methods are safe for concurrent use. Build calls for
// different sub-repos may run in parallel (bounded by the caller's
// active-set cap); duplicate calls for the same slug are coalesced
// through an in-flight table. LRU + thrashing state remains serialized
// under mu.
type MultiGraph struct {
	topo  *topology.RepoTopology
	build BuildFunc
	query string

	mu       sync.Mutex // guards active + thrash + focus + loading
	active   *LRU
	thrash   *ThrashingTracker
	focusSet map[string]bool
	loading  map[string]*loadState

	oracleFactory  func(*rmtypes.Graph) types.SymbolOracle
	locatorFactory func(*rmtypes.Graph) types.SymbolLocator

	scanNotifier func(rootRel, slug string, started bool, ok bool, elapsedMs int64)
	waitNotifier func(types.RepoMapScanEvent)
}

type loadState struct {
	done chan struct{}
	g    *rmtypes.Graph
	err  error
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
		// Defensive fallback — primary callers (cmd/root.go yaml
		// load + REPL `/repos cap`) already pass a config-clamped
		// cap. This path only fires for tests / callers that pass
		// cfg.Cap=0 by accident. internal/config sets the canonical
		// default (2); we mirror it as a literal here so multigraph
		// stays free of an internal/config import (no cycle on
		// types-only consumers).
		capN = 2
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
		loading:        make(map[string]*loadState),
		oracleFactory:  cfg.OracleFactory,
		locatorFactory: cfg.LocatorFactory,
		scanNotifier:   cfg.ScanNotifier,
		waitNotifier:   cfg.WaitNotifier,
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

// SubRepoNames returns every sub-repo identifier callers may see —
// Slug, RootRel, and the last path segment of RootRel — flattened
// into one slice with duplicates suppressed. Used by consumers in
// internal/types (which cannot import topology directly without a
// cycle) to discriminate project-name tokens from code symbols when
// the difference drives hard gate behaviour (e.g.
// CompileRequiredMechanismAnchors must not promote a repository
// name into a required item label).
//
// Returns nil on nil receivers / missing topology.
func (m *MultiGraph) SubRepoNames() []string {
	if m == nil || m.topo == nil {
		return nil
	}
	out := make([]string, 0, len(m.topo.Repos)*3)
	seen := make(map[string]bool, len(m.topo.Repos)*3)
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || s == "." {
			return
		}
		key := strings.ToLower(s)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, s)
	}
	for _, sr := range m.topo.Repos {
		add(sr.Slug)
		add(sr.RootRel)
		if i := strings.LastIndex(sr.RootRel, "/"); i >= 0 && i+1 < len(sr.RootRel) {
			add(sr.RootRel[i+1:])
		}
	}
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

	m.mu.Lock()
	if g, ok := m.active.Get(slug); ok {
		m.mu.Unlock()
		return g, nil
	}
	if state := m.loading[slug]; state != nil {
		done := state.done
		m.mu.Unlock()
		m.emitLoadWaitNotice(sr)
		<-done
		if state.err != nil {
			return nil, state.err
		}
		return state.g, nil
	}
	if m.loading == nil {
		m.loading = make(map[string]*loadState)
	}
	state := &loadState{done: make(chan struct{})}
	m.loading[slug] = state
	m.mu.Unlock()

	g, finalErr := m.buildSubRepoGraph(slug, sr)

	m.mu.Lock()
	if finalErr == nil {
		evictedSlug, _, evicted := m.active.Put(slug, g)
		if evicted {
			m.thrash.Record()
		}
		_ = evictedSlug // future telemetry hook
	}
	state.g = g
	state.err = finalErr
	delete(m.loading, slug)
	close(state.done)
	m.mu.Unlock()

	if finalErr != nil {
		return nil, finalErr
	}
	return g, nil
}

func (m *MultiGraph) emitLoadWaitNotice(sr *topology.SubRepo) {
	if m == nil || sr == nil || m.waitNotifier == nil {
		return
	}
	m.waitNotifier(types.RepoMapScanEvent{
		RepoRoot:       sr.RootAbs,
		DisplayName:    sr.RootRel,
		SubRepoRootRel: sr.RootRel,
		Phase:          types.RepoMapScanPhaseWait,
		Started:        true,
		OK:             true,
		TotalFiles:     sr.FileCount,
	})
}

func (m *MultiGraph) buildSubRepoGraph(slug string, sr *topology.SubRepo) (*rmtypes.Graph, error) {
	if m == nil || sr == nil {
		return nil, fmt.Errorf("multigraph.EnsureLoaded(%s): empty sub-repo metadata", slug)
	}
	// Phase 2 (2026-05-08): Log per-sub-repo scan start + end with
	// the human-readable RootRel and the slug — without these the
	// only "repo_map: full scan / cache hit" lines from the build
	// helper carry no sub-repo identifier, leaving operators with
	// 50+ sub-repo workspaces unable to correlate timing back to a
	// specific sub-repo when the REPL appears stuck on "preparing
	// pipeline".
	//
	// Single-repo guard (2026-05-08 audit): the MultiGraph carrier
	// wraps single-repo workspaces too — Single() routes through
	// EnsureLoaded with RootRel=".". Surfacing "scanning sub-repo
	// `.`" makes no sense to a single-repo operator and clutters
	// the dock + log with chrome that was designed for multi-repo
	// disambiguation. Emit the multigraph-level scan logs / scan
	// notices only when there are ≥2 sub-repos. The legacy
	// `repo_map: full scan / cache hit` line from the build helper
	// continues to fire in both postures.
	emitScanChrome := !m.IsSingle()
	scanStart := time.Now()
	if emitScanChrome {
		logging.Info("multigraph: scanning sub-repo %s (slug=%s)", sr.RootRel, slug)
		if m.scanNotifier != nil {
			m.scanNotifier(sr.RootRel, slug, true /*started*/, true /*ok placeholder*/, 0)
		}
	}
	g, err := m.build(sr.RootAbs, m.query)
	if err != nil {
		elapsedMs := time.Since(scanStart).Milliseconds()
		if emitScanChrome {
			logging.Warning("multigraph: scan failed sub-repo %s (slug=%s) elapsed=%dms err=%v",
				sr.RootRel, slug, elapsedMs, err)
			if m.scanNotifier != nil {
				m.scanNotifier(sr.RootRel, slug, false /*ended*/, false /*ok=false*/, elapsedMs)
			}
		}
		return nil, fmt.Errorf("multigraph.EnsureLoaded(%s): %w", slug, err)
	}
	if g == nil {
		elapsedMs := time.Since(scanStart).Milliseconds()
		if emitScanChrome {
			logging.Warning("multigraph: scan returned nil graph sub-repo %s (slug=%s) elapsed=%dms",
				sr.RootRel, slug, elapsedMs)
			if m.scanNotifier != nil {
				m.scanNotifier(sr.RootRel, slug, false, false, elapsedMs)
			}
		}
		return nil, fmt.Errorf("multigraph.EnsureLoaded(%s): build returned nil graph", slug)
	}
	elapsedMs := time.Since(scanStart).Milliseconds()
	if emitScanChrome {
		logging.Info("multigraph: scan complete sub-repo %s (slug=%s) elapsed=%dms",
			sr.RootRel, slug, elapsedMs)
		if m.scanNotifier != nil {
			m.scanNotifier(sr.RootRel, slug, false, true, elapsedMs)
		}
	}
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
	unique := make([]string, 0, len(slugs))
	seen := make(map[string]bool, len(slugs))
	for _, slug := range slugs {
		if slug == "" || seen[slug] {
			continue
		}
		seen[slug] = true
		unique = append(unique, slug)
	}
	if len(unique) == 0 {
		return nil
	}
	if len(unique) == 1 {
		_, err := m.EnsureLoaded(unique[0])
		return err
	}
	errCh := make(chan error, len(unique))
	var wg sync.WaitGroup
	for _, slug := range unique {
		slug := slug
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := m.EnsureLoaded(slug); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
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

// ActiveSlugSnapshot returns the slug → bool set of currently LRU-
// resident sub-repos. Phase 5 (2026-05-08) /repos UX uses it to
// label rows as "auto-active" vs "inactive". Empty map when m is
// nil — caller treats that as "no auto-active rows".
func (m *MultiGraph) ActiveSlugSnapshot() map[string]bool {
	if m == nil || m.active == nil {
		return map[string]bool{}
	}
	graphs := m.active.Snapshot()
	out := make(map[string]bool, len(graphs))
	for slug := range graphs {
		out[slug] = true
	}
	return out
}

// PreviewActiveSet returns the slug list the routing fold WOULD
// pick if a Run started right now with NO question-specific signals
// (B/C/D channels empty). Phase 6 (2026-05-08) /repos UX uses it
// when the LRU is empty (pre-first-Run) so the operator sees which
// sub-repos would be auto-active under fallback before issuing a
// question — without this, the listing would show every sub-repo
// as "inactive" with no preview, leaving the operator wondering
// whether the routing layer is even working.
//
// Inputs:
//   - focusSlugs: current operator pin set (REPL multiRepoFocus).
//     The fold's A channel honours pins ahead of the cap; without
//     the parameter the preview would understate the real next-Run
//     active set.
//
// Returns nil for the single-repo / nil receiver case so callers
// can detect "no preview applicable" vs an explicit empty preview.
func (m *MultiGraph) PreviewActiveSet(focusSlugs []string) []string {
	if m == nil || m.IsSingle() {
		return nil
	}
	decision := m.RouteActiveSet(RoutingInputs{
		FocusSlugs:      focusSlugs,
		StrictFocus:     len(focusSlugs) > 0,
		DisableFallback: len(focusSlugs) > 0,
		// B / C / D intentionally empty — the preview is "if the
		// user submitted a generic question with no path anchor, no
		// log, no language hint, what would the fold pick?" When no
		// focus is pinned, E (biggest-first fallback) fills the cap.
	})
	return decision.Active
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

// === Graph-method fan-out helpers (Y-class richer than Oracle/Locator) ===

// LookupSymbol fans out across active sub-repos and returns every
// matching definition with its owning sub-repo. The Symbol's File
// field is rewritten to the path-from-parent form (sub-repo prefix
// prepended) so callers comparing positions across sub-repos do not
// collide on "main.go" / "lib.rs". Returns nil on no match.
//
// This is the Y-flavoured cousin of Locator().LocateSymbol — same
// fan-out semantics but returns *Symbol directly so consumers that
// need richer fields (Kind, Receiver, Signature, ParseTier) than
// SymbolLocation provides can use them.
func (m *MultiGraph) LookupSymbol(name string) []SymbolHit {
	if m == nil {
		return nil
	}
	graphs := m.AllGraphs()
	if len(graphs) == 0 {
		return nil
	}
	subMap := make(map[string]*topology.SubRepo, len(m.topo.Repos))
	for i := range m.topo.Repos {
		subMap[m.topo.Repos[i].Slug] = &m.topo.Repos[i]
	}
	var out []SymbolHit
	for slug, g := range graphs {
		if g == nil {
			continue
		}
		defs, ok := g.SymbolDefs[name]
		if !ok || len(defs) == 0 {
			continue
		}
		sr := subMap[slug]
		for _, sym := range defs {
			if sym == nil {
				continue
			}
			out = append(out, SymbolHit{Symbol: sym, Sub: sr})
		}
	}
	return out
}

// SymbolHit pairs a *Symbol with the SubRepo that owns it. The Symbol
// pointer is shared with the underlying *Graph (treat as read-only;
// mutating taints the cached graph). Sub.RootRel can be used to
// rebuild the path-from-parent form when needed.
type SymbolHit struct {
	Symbol *rmtypes.Symbol
	Sub    *topology.SubRepo
}

// ImplementersOf fans out the "which symbols implement interface
// `name`?" query across active sub-repos. Returns SymbolID slices
// with their owning sub-repo. Cross-sub-repo implementers are
// independent because Go module / Java pom / Cargo crate namespaces
// are independent (design §3.5).
type ImplementerHit struct {
	ID  rmtypes.SymbolID
	Sub *topology.SubRepo
}

func (m *MultiGraph) ImplementersOf(interfaceName string) []ImplementerHit {
	if m == nil {
		return nil
	}
	graphs := m.AllGraphs()
	if len(graphs) == 0 {
		return nil
	}
	subMap := make(map[string]*topology.SubRepo, len(m.topo.Repos))
	for i := range m.topo.Repos {
		subMap[m.topo.Repos[i].Slug] = &m.topo.Repos[i]
	}
	var out []ImplementerHit
	for slug, g := range graphs {
		if g == nil {
			continue
		}
		ids := g.ImplementersOf(interfaceName)
		if len(ids) == 0 {
			continue
		}
		sr := subMap[slug]
		for _, id := range ids {
			out = append(out, ImplementerHit{ID: id, Sub: sr})
		}
	}
	return out
}

// ImplementerMembersOf exposes the same typed implementer relation through a
// types-only interface so packages that cannot import multigraph directly
// (notably internal/tool) can still reason about multi-repo relation members
// without creating an import cycle. Returned File values are path-from-parent
// surfaces via SubRepoRelPath.
func (m *MultiGraph) ImplementerMembersOf(interfaceName string) []types.TypedRelationMember {
	hits := m.ImplementersOf(interfaceName)
	if len(hits) == 0 {
		return nil
	}
	out := make([]types.TypedRelationMember, 0, len(hits))
	seen := map[string]bool{}
	for _, hit := range hits {
		sym, _, ok := m.LookupSymbolByID(hit.ID)
		if !ok || sym == nil || strings.TrimSpace(sym.Name) == "" {
			continue
		}
		file := SubRepoRelPath(hit.Sub, sym.File)
		key := strings.ToLower(strings.TrimSpace(sym.Name)) + "|" + file
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, types.TypedRelationMember{
			Name:     strings.TrimSpace(sym.Name),
			File:     file,
			Line:     sym.Line,
			Kind:     sym.Kind,
			Distance: 1,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].File < out[j].File
	})
	return out
}

// TypedRelationCandidates exposes multi-repo typed relation rows through the
// shared types-only provider boundary. Exact implementers are served by the
// multi-repo index directly; graph-backed inheritance, call, and import/export
// rows are delegated to the single-graph provider and path-prefixed here so
// downstream packages do not learn the concrete MultiGraph type.
func (m *MultiGraph) TypedRelationCandidates(q types.TypedRelationQuery) []types.TypedRelationCandidate {
	if m == nil {
		return nil
	}
	var out []types.TypedRelationCandidate
	seen := map[string]bool{}
	add := func(row types.TypedRelationCandidate) bool {
		if row.Relation == "" || strings.TrimSpace(row.SourceName) == "" || strings.TrimSpace(row.Member.Name) == "" {
			return false
		}
		key := strings.ToLower(string(row.Relation)) + "|" +
			strings.ToLower(strings.TrimSpace(row.SourceName)) + "|" +
			strings.ToLower(strings.TrimSpace(row.Member.Name)) + "|" +
			strings.TrimSpace(row.Member.File) + "|" +
			strings.TrimSpace(row.SourceFile)
		if seen[key] {
			return false
		}
		seen[key] = true
		out = append(out, row)
		return q.MaxMembers > 0 && len(out) >= q.MaxMembers
	}
	if q.AllowsKind(types.TypedRelationImplements) {
		for _, source := range q.Sources {
			source = strings.TrimSpace(source)
			if source == "" {
				continue
			}
			members := m.ImplementerMembersOf(source)
			for _, member := range members {
				name := strings.TrimSpace(member.Name)
				if name == "" {
					continue
				}
				member.Name = name
				if add(types.TypedRelationCandidate{
					Relation:   types.TypedRelationImplements,
					SourceName: source,
					Member:     member,
					Carrier:    types.TypedRelationCarrierMultiGraph,
					Precision:  types.TypedRelationPrecisionExactSymbolID,
				}) {
					return out
				}
			}
		}
	}
	for _, row := range m.graphBackedRelationCandidates(q) {
		if add(row) {
			return out
		}
	}
	return out
}

func (m *MultiGraph) graphBackedRelationCandidates(q types.TypedRelationQuery) []types.TypedRelationCandidate {
	kinds := graphBackedRelationKinds(q)
	if m == nil || len(kinds) == 0 {
		return nil
	}
	graphs := m.AllGraphs()
	if len(graphs) == 0 {
		return nil
	}
	subMap := make(map[string]*topology.SubRepo, len(m.topo.Repos))
	for i := range m.topo.Repos {
		subMap[m.topo.Repos[i].Slug] = &m.topo.Repos[i]
	}
	var out []types.TypedRelationCandidate
	for _, source := range q.Sources {
		source = strings.TrimSpace(strings.ReplaceAll(source, `\`, `/`))
		if source == "" {
			continue
		}
		if g, sr, hit := m.GraphFor(source); hit && g != nil {
			local := q
			local.Sources = []string{stripSubRepoPrefix(sr, source)}
			local.Kinds = kinds
			for _, row := range rmrelation.TypedRelationCandidates(g, local) {
				out = append(out, prefixTypedRelationCandidate(sr, row))
				if q.MaxMembers > 0 && len(out) >= q.MaxMembers {
					return out
				}
			}
			continue
		}
		for slug, g := range graphs {
			if g == nil {
				continue
			}
			local := q
			local.Sources = []string{source}
			local.Kinds = kinds
			for _, row := range rmrelation.TypedRelationCandidates(g, local) {
				out = append(out, prefixTypedRelationCandidate(subMap[slug], row))
				if q.MaxMembers > 0 && len(out) >= q.MaxMembers {
					return out
				}
			}
		}
	}
	return out
}

func graphBackedRelationKinds(q types.TypedRelationQuery) []types.TypedRelationKind {
	var out []types.TypedRelationKind
	if q.AllowsKind(types.TypedRelationCalledBy) {
		out = append(out, types.TypedRelationCalledBy)
	}
	if q.AllowsKind(types.TypedRelationReferences) {
		out = append(out, types.TypedRelationReferences)
	}
	if q.AllowsKind(types.TypedRelationExtends) {
		out = append(out, types.TypedRelationExtends)
	}
	if q.AllowsKind(types.TypedRelationImports) {
		out = append(out, types.TypedRelationImports)
	}
	if q.AllowsKind(types.TypedRelationExports) {
		out = append(out, types.TypedRelationExports)
	}
	return out
}

func prefixTypedRelationCandidate(sr *topology.SubRepo, row types.TypedRelationCandidate) types.TypedRelationCandidate {
	row.Carrier = types.TypedRelationCarrierMultiGraph
	if row.Member.File != "" {
		row.Member.File = SubRepoRelPath(sr, row.Member.File)
	}
	if row.SourceFile != "" {
		row.SourceFile = SubRepoRelPath(sr, row.SourceFile)
	}
	switch row.SourceKind {
	case "file", "directory":
		if row.SourceName != "" {
			row.SourceName = SubRepoRelPath(sr, row.SourceName)
		}
	}
	return row
}

// TypedRelationSourceFacts exposes exact symbol-kind facts for relation query
// selection. It lets prompt-hint selection discover that a source entity is an
// interface / trait / protocol without downstream code importing MultiGraph or
// scanning user/model prose.
func (m *MultiGraph) TypedRelationSourceFacts(sources []string) []types.TypedRelationSourceFact {
	if m == nil || len(sources) == 0 {
		return nil
	}
	graphs := m.AllGraphs()
	if len(graphs) == 0 {
		return nil
	}
	sourceSet := map[string]string{}
	for _, source := range sources {
		source = strings.TrimSpace(source)
		if source == "" {
			continue
		}
		sourceSet[strings.ToLower(source)] = source
	}
	if len(sourceSet) == 0 {
		return nil
	}
	var out []types.TypedRelationSourceFact
	seen := map[string]bool{}
	for _, g := range graphs {
		if g == nil {
			continue
		}
		for lower, source := range sourceSet {
			defs := g.SymbolDefs[source]
			if len(defs) == 0 {
				for name, candidates := range g.SymbolDefs {
					if strings.EqualFold(name, source) {
						defs = candidates
						break
					}
				}
			}
			for _, def := range defs {
				if def == nil || strings.TrimSpace(def.Name) == "" || strings.TrimSpace(def.Kind) == "" {
					continue
				}
				file := strings.TrimSpace(def.File)
				key := lower + "|" + strings.ToLower(def.Name) + "|" + strings.ToLower(def.Kind) + "|" + file
				if seen[key] {
					continue
				}
				seen[key] = true
				out = append(out, types.TypedRelationSourceFact{
					Name:      strings.TrimSpace(def.Name),
					Kind:      strings.TrimSpace(def.Kind),
					File:      file,
					Line:      def.Line,
					Carrier:   types.TypedRelationCarrierMultiGraph,
					Precision: types.TypedRelationPrecisionExactSymbolID,
				})
			}
		}
	}
	return out
}

// LookupSymbolByID fans out the SymbolID lookup across active
// sub-repos. Returns the (Symbol, SubRepo) of the first match —
// SymbolID is canonical and globally unique per Symbol so multi
// matches across sub-repos imply the same logical symbol got indexed
// twice (which the topology's separate sub-repo namespaces should
// prevent in practice).
func (m *MultiGraph) LookupSymbolByID(id rmtypes.SymbolID) (*rmtypes.Symbol, *topology.SubRepo, bool) {
	if m == nil {
		return nil, nil, false
	}
	graphs := m.AllGraphs()
	if len(graphs) == 0 {
		return nil, nil, false
	}
	subMap := make(map[string]*topology.SubRepo, len(m.topo.Repos))
	for i := range m.topo.Repos {
		subMap[m.topo.Repos[i].Slug] = &m.topo.Repos[i]
	}
	for slug, g := range graphs {
		if g == nil {
			continue
		}
		if sym, ok := g.SymbolByID[id]; ok && sym != nil {
			return sym, subMap[slug], true
		}
	}
	return nil, nil, false
}

// IterateSymbolDefs iterates every (name, defs) pair across active
// sub-repos. Useful for explorer-style "find all symbols matching
// pattern" scans. The yield func receives the symbol name, defs
// slice (read-only), and owning sub-repo. Return false to stop.
//
// Iteration order is the LRU's internal order; callers MUST NOT
// depend on it.
func (m *MultiGraph) IterateSymbolDefs(yield func(name string, defs []*rmtypes.Symbol, sub *topology.SubRepo) bool) {
	if m == nil {
		return
	}
	graphs := m.AllGraphs()
	if len(graphs) == 0 {
		return
	}
	subMap := make(map[string]*topology.SubRepo, len(m.topo.Repos))
	for i := range m.topo.Repos {
		subMap[m.topo.Repos[i].Slug] = &m.topo.Repos[i]
	}
	for slug, g := range graphs {
		if g == nil {
			continue
		}
		sr := subMap[slug]
		for name, defs := range g.SymbolDefs {
			if !yield(name, defs, sr) {
				return
			}
		}
	}
}

// IterateFileIndex iterates every (relPathInternal, *FileInfo, sub)
// across active sub-repos. The internal path is sub-repo-relative
// (NOT prefixed with sub.RootRel); callers that need
// path-from-parent form should compose via SubRepoRelPath(sub, rel).
func (m *MultiGraph) IterateFileIndex(yield func(internalRel string, fi *rmtypes.FileInfo, sub *topology.SubRepo) bool) {
	if m == nil {
		return
	}
	graphs := m.AllGraphs()
	if len(graphs) == 0 {
		return
	}
	subMap := make(map[string]*topology.SubRepo, len(m.topo.Repos))
	for i := range m.topo.Repos {
		subMap[m.topo.Repos[i].Slug] = &m.topo.Repos[i]
	}
	for slug, g := range graphs {
		if g == nil {
			continue
		}
		sr := subMap[slug]
		for path, fi := range g.FileIndex {
			if !yield(path, fi, sr) {
				return
			}
		}
	}
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

// MultiRepoSubRepoCount returns the discovered sub-repo count through
// the narrow types.MultiRepoFocusResolver interface.
func (m *MultiGraph) MultiRepoSubRepoCount() int {
	if m == nil || m.topo == nil {
		return 0
	}
	return len(m.topo.Repos)
}

// ResolveMultiRepoSubRepo resolves a selector token against the
// topology and returns only stable, prompt-safe identifiers. This
// avoids forcing generic tools to import the concrete topology package.
func (m *MultiGraph) ResolveMultiRepoSubRepo(token string) (rootRel string, slug string, ok bool) {
	if m == nil || m.topo == nil {
		return "", "", false
	}
	sr := m.topo.Resolve(token)
	if sr == nil {
		return "", "", false
	}
	return sr.RootRel, sr.Slug, true
}

// SetFocus replaces the user-pinned slug set. Called by the REPL
// when /repos focus / /repos unfocus changes pinning between Runs.
// Concurrency-safe; mutating mid-Run is allowed but the routing fold
// only consults FocusSlugs at Run entry, so the change takes effect
// at the next Run.
func (m *MultiGraph) SetFocus(slugs []string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.focusSet = make(map[string]bool, len(slugs))
	for _, s := range slugs {
		m.focusSet[s] = true
	}
}

// SetCap resizes the LRU active-set ceiling. The new cap takes
// effect on the NEXT Put — already-resident graphs are not evicted
// just because the cap shrank. Operators raising the cap see the
// extra capacity available immediately. Operators lowering the cap
// see the LRU bleed off as eviction triggers naturally.
//
// Single-repo posture ignores the call (cap stays 1).
func (m *MultiGraph) SetCap(n int) {
	if m == nil || n <= 0 {
		return
	}
	if m.IsSingle() {
		return
	}
	if m.active != nil {
		m.active.SetCap(n)
	}
}

// === Routing fold (design §3.5 channels A/B/C/D/E) ===

// RoutingInputs aggregates the signals the routing fold consumes to
// pick the active sub-repo set. Empty fields are tolerated; the fold
// degrades through channels in priority order.
type RoutingInputs struct {
	// FocusSlugs are user-pinned slugs (REPL /repos focus). Channel A —
	// MUST be in the active set (caller responsibility to keep within Cap).
	FocusSlugs []string

	// StrictFocus means FocusSlugs are the complete active set. It is
	// used for explicit user pins so the system does not silently add
	// unrelated sub-repos through fallback.
	StrictFocus bool

	// ExactPrescanSlugs are sub-repos with exact request-literal hits in
	// source/config-like files. Channel P — precise enough to beat the
	// model selector, but still only a routing signal. Explorer must read
	// or grep the selected files before using the fact in an answer.
	ExactPrescanSlugs []string

	// ModelRecommendedSlugs are the typed output of the multi-repo focus
	// selector. Channel M — chosen after precise B/C/P signals and before
	// noisy language/fallback channels. Callers that want to honor the
	// model selection exactly should also set DisableFallback.
	ModelRecommendedSlugs []string

	// RequiredFiles are paths-from-parent the analyzer asked for
	// (MutableState.exactContextRequiredFiles). Channel B — precise.
	RequiredFiles []string

	// LogFrameFiles are file paths from log-triage frame.File
	// (paths-from-parent). Channel C — precise.
	LogFrameFiles []string

	// QueryLanguages are the languages the user's query implies
	// (e.g., "Go-only question" → ["go"]). Channel D — noisy rank.
	QueryLanguages []string

	// DisableFallback suppresses channel E. Use this when an exact
	// user/model source already chose the active set. Leave false for
	// legacy biggest-first fallback behavior.
	DisableFallback bool
}

// RoutingDecision is the fold's output. Active is the slug list to
// EnsureMany; SubRepoFocus reasons map slug → channel-letter for
// telemetry. Inactive is the slugs NOT chosen (becomes
// PendingSubRepoNames in BusContext).
type RoutingDecision struct {
	Active       []string
	Reasons      map[string]string // slug → "A"|"B"|"C"|"P"|"M"|"D"|"E"
	Inactive     []string
	OverflowDrop []string // slugs that channels B/C wanted but cap excluded
}

// RouteActiveSet folds the inputs into an Active slug list bounded
// by Cap. Channel priority A > B > C > P > M > D > E. Pre-trim guarantees
// EnsureMany never returns ErrTooManyActive on the fold's output —
// the cap fail-loud is a defense-in-depth.
//
// Single-repo posture short-circuits to {parent-slug} regardless of
// inputs. Multi-repo: walks channels in priority order, dedupes,
// trims to Cap.
func (m *MultiGraph) RouteActiveSet(inputs RoutingInputs) RoutingDecision {
	if m == nil || m.topo == nil {
		return RoutingDecision{}
	}
	cap := m.Cap()
	if cap <= 0 {
		cap = 2
	}

	// Single-repo fast path.
	if m.IsSingle() {
		slug := m.topo.Repos[0].Slug
		return RoutingDecision{
			Active:  []string{slug},
			Reasons: map[string]string{slug: "S"}, // single-repo
		}
	}

	type entry struct {
		slug   string
		reason string
	}
	seen := make(map[string]string, cap*2)
	addUnique := func(slug, reason string) {
		if _, ok := seen[slug]; ok {
			return
		}
		// Verify slug exists in topology (defense against stale focus pins).
		if m.topo.SubRepoBySlug(slug) == nil {
			return
		}
		seen[slug] = reason
	}

	// A — focus pins (must be active; over-cap is operator error).
	for _, slug := range inputs.FocusSlugs {
		addUnique(slug, "A")
	}
	if inputs.StrictFocus && len(inputs.FocusSlugs) > 0 {
		active := make([]string, 0, len(seen))
		reasons := make(map[string]string, len(seen))
		for _, sr := range m.topo.Repos {
			if r, ok := seen[sr.Slug]; ok && r == "A" {
				active = append(active, sr.Slug)
				reasons[sr.Slug] = r
			}
		}
		inactive := make([]string, 0, len(m.topo.Repos)-len(active))
		activeSet := make(map[string]bool, len(active))
		for _, slug := range active {
			activeSet[slug] = true
		}
		for _, sr := range m.topo.Repos {
			if !activeSet[sr.Slug] {
				inactive = append(inactive, sr.Slug)
			}
		}
		return RoutingDecision{Active: active, Reasons: reasons, Inactive: inactive}
	}
	// B — analyzer's RequiredFiles (precise).
	var bSlugs []string
	for _, rel := range inputs.RequiredFiles {
		if sr := m.topo.Lookup(rel); sr != nil {
			if _, ok := seen[sr.Slug]; !ok {
				bSlugs = append(bSlugs, sr.Slug)
			}
			addUnique(sr.Slug, "B")
		}
	}
	// C — log frame files (precise).
	var cSlugs []string
	for _, rel := range inputs.LogFrameFiles {
		if sr := m.topo.Lookup(rel); sr != nil {
			if _, ok := seen[sr.Slug]; !ok {
				cSlugs = append(cSlugs, sr.Slug)
			}
			addUnique(sr.Slug, "C")
		}
	}
	// P — exact request-literal pre-scan hits.
	for _, slug := range inputs.ExactPrescanSlugs {
		addUnique(slug, "P")
	}
	// M — typed model selector recommendations.
	for _, slug := range inputs.ModelRecommendedSlugs {
		addUnique(slug, "M")
	}
	// D — language affinity (noisy rank).
	if len(inputs.QueryLanguages) > 0 {
		langSet := make(map[string]bool, len(inputs.QueryLanguages))
		for _, l := range inputs.QueryLanguages {
			langSet[l] = true
		}
		for _, sr := range m.topo.Repos {
			for _, lang := range sr.PrimaryLangs {
				if langSet[lang] {
					addUnique(sr.Slug, "D")
					break
				}
			}
		}
	}
	// E — biggest-first fallback (noisy).
	if len(seen) < cap && !inputs.DisableFallback {
		bySize := make([]topology.SubRepo, len(m.topo.Repos))
		copy(bySize, m.topo.Repos)
		// Stable sort by FileCount desc (largest first). This can run
		// on every REPL turn before the first pipeline event, so keep
		// it O(n log n) for large multi-repo workspaces.
		sort.SliceStable(bySize, func(i, j int) bool {
			return bySize[i].FileCount > bySize[j].FileCount
		})
		for _, sr := range bySize {
			if len(seen) >= cap {
				break
			}
			addUnique(sr.Slug, "E")
		}
	}

	// Trim to cap (preserving channel priority — earlier-added wins).
	// Walk topology repos in fixed order to make the trimming stable
	// across runs (no map iteration ordering surprises).
	active := make([]string, 0, cap)
	overflow := make([]string, 0)
	reasons := make(map[string]string, cap)

	// First pass: emit A entries (always, even if over cap).
	for slug, reason := range seen {
		if reason == "A" {
			active = append(active, slug)
			reasons[slug] = reason
		}
	}
	// Second pass: emit B/C (precise) up to cap.
	addRest := func(want string) {
		if len(active) >= cap {
			return
		}
		// Walk topology in order so emission is stable.
		for _, sr := range m.topo.Repos {
			if r, ok := seen[sr.Slug]; ok && r == want {
				if _, already := reasons[sr.Slug]; already {
					continue
				}
				if len(active) >= cap {
					overflow = append(overflow, sr.Slug)
					continue
				}
				active = append(active, sr.Slug)
				reasons[sr.Slug] = want
			}
		}
	}
	addRest("B")
	addRest("C")
	addRest("P")
	addRest("M")
	addRest("D")
	addRest("E")

	// Build inactive list.
	inactive := make([]string, 0, len(m.topo.Repos)-len(active))
	activeSet := make(map[string]bool, len(active))
	for _, slug := range active {
		activeSet[slug] = true
	}
	for _, sr := range m.topo.Repos {
		if !activeSet[sr.Slug] {
			inactive = append(inactive, sr.Slug)
		}
	}

	return RoutingDecision{
		Active:       active,
		Reasons:      reasons,
		Inactive:     inactive,
		OverflowDrop: overflow,
	}
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
