// Package authority centralises the AuthorityCeiling axis projection
// logic. Lives in its own package (rather than hanging off types/) so
// the projection function can pull in the logtriage drift detector
// without introducing import cycles — analysis/logtriage and
// internal/tool/* both depend on internal/types, but neither should
// depend on each other.
//
// The package exposes two things:
//
//  1. ComputeForEvidence: the deterministic projection from
//     (EvidenceItem, BusContext) → (Origin, Authority, Reason).
//     Reads grounding tier, scope, log/perf bundles, and (when a
//     SymbolLocator is wired) drift status. No mutation.
//
//  2. SymbolLocatorProvider: cmd/root.go provides an implementation
//     that wraps repomap.NewSymbolLocator; emit_evidence consults
//     this provider to obtain a locator without importing repomap.
//     Indirection avoids a cycle between authority and tool/repomap.
//
// The axis is unconditionally active — there is no feature gate.
// Single-shot CLI flows that don't build a repomap install a nil
// SymbolLocatorProvider; the projection function still runs and
// degrades to "no drift detection" gracefully (Origin=current_repo,
// Authority graded by grounding tier alone).
package authority

import (
	"fmt"
	"strings"
	"sync"

	"github.com/hanchaoqun/codrax/internal/analysis/logtriage"
	"github.com/hanchaoqun/codrax/internal/types"
)

var (
	locatorMu       sync.RWMutex
	locatorProvider func(graph any) types.SymbolLocator

	// multiGraphLocatorProvider is the cross-sub-repo fan-out
	// alternative. cmd/root.go installs an adapter that takes a
	// *multigraph.MultiGraph (as any to keep this package free of
	// the multigraph import) and returns mg.Locator(). When set,
	// LocatorFromBusContext prefers it over locatorProvider so
	// drift detection scans every active sub-repo.
	multiGraphLocatorProvider func(mg any) types.SymbolLocator
)

// SetSymbolLocatorProvider registers a function that adapts an opaque
// repomap.Graph (passed as `any` to honour the no-cycle pattern used
// by Mutable.SearchGraph) into a types.SymbolLocator. cmd/root.go
// installs the canonical provider during init; tests may install a
// fake to exercise drift detection without building a graph.
//
// nil provider clears the registration; subsequent ComputeForEvidence
// calls then proceed without drift detection.
func SetSymbolLocatorProvider(p func(graph any) types.SymbolLocator) {
	locatorMu.Lock()
	defer locatorMu.Unlock()
	locatorProvider = p
}

// BackfillEvidenceProjector returns a types.EvidenceProjector that
// runs ComputeForEvidence on any incoming item lacking Origin /
// Authority. Used for bypass paths (concrete_value extractor,
// mechanism_scan, bridge_literal merge) that build EvidenceItem
// literals directly without going through emit_evidence's hook.
//
// cmd/root.go registers this at startup via
// types.RegisterEvidenceProjector. Items that already have non-
// zero Origin or Authority pass through untouched (idempotent —
// emit_evidence's items have already been projected).
func BackfillEvidenceProjector() types.EvidenceProjector {
	return func(items []types.EvidenceItem, m *types.MutableState) []types.EvidenceItem {
		if len(items) == 0 || m == nil {
			return items
		}
		// Synthesise a minimal bus context. ComputeForEvidence reads
		// only Mutable.LogTriage / PerfTrace / SearchGraph from the
		// bus, so an otherwise-empty BusContext is sufficient.
		bus := &types.BusContext{Mutable: m}
		for i := range items {
			if items[i].Origin != types.ClaimOriginUnknown ||
				items[i].Authority != types.AuthorityUnknown {
				continue
			}
			proj := ComputeForEvidence(items[i], bus)
			items[i].Origin = proj.Origin
			items[i].Authority = proj.Authority
			items[i].AuthorityReason = proj.Reason
			items[i].DriftReason = proj.DriftReason
			// 2026-05-02: alongside the Origin/Authority projection,
			// also fill the second-axis LogPerfSubKind classification
			// when the item anchors on a log/perf frame. Cheap (pure
			// function over typed bundle structures); idempotent (a
			// pre-set non-empty SubKind passes through). Empty result
			// = not log/perf evidence OR no specific sub-classification.
			if items[i].LogPerfSubKind == "" {
				items[i].LogPerfSubKind = types.LogPerfSubKindOf(
					items[i],
					bus.Mutable.LogTriage(),
					bus.Mutable.PerfTrace(),
				)
			}
		}
		return items
	}
}

// LocatorFromGraph adapts the opaque graph value via the registered
// provider. Returns nil when no provider is installed or the provider
// itself returns nil.
func LocatorFromGraph(graph any) types.SymbolLocator {
	if graph == nil {
		return nil
	}
	locatorMu.RLock()
	defer locatorMu.RUnlock()
	if locatorProvider == nil {
		return nil
	}
	return locatorProvider(graph)
}

// SetMultiGraphLocatorProvider registers the cross-sub-repo locator
// adapter. cmd/root.go installs an implementation that returns
// mg.Locator() so drift detection fans out across active sub-repos.
// Empty or nil provider disables the multigraph path; LocatorFromBusContext
// then falls through to the legacy single-graph provider.
func SetMultiGraphLocatorProvider(p func(mg any) types.SymbolLocator) {
	locatorMu.Lock()
	defer locatorMu.Unlock()
	multiGraphLocatorProvider = p
}

// LocatorFromBusContext is the preferred entry: prefers the cross-
// sub-repo fan-out locator (BusContext.MultiGraph) when available,
// falls back to the single-graph locator wrapping
// Mutable.SearchGraph(). Either path returning nil tells the caller
// to skip drift detection (graceful degrade — pre-multi-repo
// behaviour preserved).
//
// P4-cross-sub-repo (2026-05-08) — fixes the false-negative where
// a panic stack frame from sub-b was unmappable because authority's
// drift detector only consulted the routed sub-repo's graph.
func LocatorFromBusContext(bus *types.BusContext) types.SymbolLocator {
	if bus == nil {
		return nil
	}
	locatorMu.RLock()
	mgProv := multiGraphLocatorProvider
	graphProv := locatorProvider
	locatorMu.RUnlock()
	if mgProv != nil && bus.MultiGraph != nil {
		if loc := mgProv(bus.MultiGraph); loc != nil {
			return loc
		}
	}
	if graphProv == nil {
		return nil
	}
	if bus.Mutable == nil {
		return nil
	}
	g := bus.Mutable.SearchGraph()
	if g == nil {
		return nil
	}
	return graphProv(g)
}

// Projection is the atomic projection result from ComputeForEvidence.
// All four fields are written together at evidence emit time and
// pinned on the EvidenceItem; downstream consumers (renderer,
// contract, derived drift anchor builders) read these as a typed
// read-only authority cap.
type Projection struct {
	Origin      types.ClaimOrigin
	Authority   types.AuthorityCeiling
	Reason      string
	DriftReason types.DriftReason
}

// ComputeForEvidence is the canonical projection from a constructed
// EvidenceItem (post-grounding) and the bus context to a Projection.
// Pure function: no mutation of inputs; same inputs always produce
// the same output.
//
// Algorithm (priority order — first hit wins):
//
//  1. Nil bus → returns zero values (defensive nil-guard; caller
//     leaves item.Origin / Authority untouched).
//
//  2. Schema-level scopes (File / Crossfile / Negative) → always
//     ClaimOriginCurrentRepo + AuthorityFactual. These scopes are
//     structural assertions about the current repo (layer identity,
//     cross-file contract, confirmed absence) — log drift cannot
//     downgrade them.
//
//  3. ContextRole == illustrative_only OR GroundingStatus is
//     ungrounded → AuthorityIllustrative. The renderer must NOT
//     enter these into causal chains.
//
//  4. Item's anchor (Source, LineStart, AnchorSymbol) matches a
//     LogBundle frame (or PerfBundle frame). Run drift detection:
//
//     - DriftStatusNone with a current-code match → cross_source +
//       factual.
//     - LineDrift / TailRename → cross_source + conditional.
//     - FileMoved / Unmappable → cross_source + historical.
//
//     Important: this branch is for REPO-BACKED evidence whose
//     current-code anchor is corroborated by an attached artifact. It
//     must not collapse to ClaimOriginLog / Perf, because those origins
//     are reserved for pure artifact observations. Otherwise
//     ClaimFormOf would reinterpret current-code call/guard/assignment
//     anchors as external_observation and downstream stages would lose
//     the mechanism/path distinction.
//
//  5. No log/perf match → ClaimOriginCurrentRepo. Authority is
//     factual when grounding tier is strong (line_text /
//     symbol_table) and scope is line-shaped; conditional otherwise
//     (recovered tiers and section/range scopes get a softer cap by
//     default — they survive grounding but the anchor is less
//     precise).
func ComputeForEvidence(item types.EvidenceItem, bus *types.BusContext) Projection {
	if bus == nil {
		return Projection{}
	}

	// (2) Schema-level scopes: factual current_repo by construction.
	switch item.Scope {
	case types.ScopeFile, types.ScopeCrossfile, types.ScopeNegative:
		return Projection{
			Origin:    types.ClaimOriginCurrentRepo,
			Authority: types.AuthorityFactual,
			Reason:    "schema-level scope: structural assertion about current repo",
		}
	}

	// (3) Illustrative / ungrounded carve-out.
	if item.ContextRole == types.EvidenceContextRoleIllustrativeOnly {
		return Projection{
			Origin:    types.ClaimOriginCurrentRepo,
			Authority: types.AuthorityIllustrative,
			Reason:    "context_role=illustrative_only — not a causal claim",
		}
	}
	if item.GroundingStatus == types.GroundingUngrounded {
		return Projection{
			Origin:    types.ClaimOriginCurrentRepo,
			Authority: types.AuthorityIllustrative,
			Reason:    "grounding_status=ungrounded — anchor unverified",
		}
	}

	// (4) Log / perf bundle match — strongest evidence of drift.
	if proj, matched := computeFromLogPerfDrift(item, bus); matched {
		return proj
	}

	// (5) Default current_repo path — graded by grounding tier + scope.
	switch item.GroundingStatus {
	case types.GroundingRecovered:
		// Recovery tier rewrote line/source. Round-3 refinement: only
		// downgrade to conditional when an attached log/perf bundle is
		// actually present. Without an attached artifact, "recovered"
		// is an in-pipeline LLM line-typo correction — NOT a drift
		// signal.
		if hasAttachedArtifact(bus) {
			return Projection{
				Origin:    types.ClaimOriginCurrentRepo,
				Authority: types.AuthorityConditional,
				Reason:    fmt.Sprintf("grounding recovered via %s — original anchor imprecise (log/perf attached)", item.GroundingTier),
			}
		}
		return Projection{
			Origin:    types.ClaimOriginCurrentRepo,
			Authority: types.AuthorityFactual,
			Reason:    fmt.Sprintf("grounding recovered via %s (no attached log; treated as factual self-correction)", item.GroundingTier),
		}
	}
	if item.Scope.IsLineShaped() {
		return Projection{
			Origin:    types.ClaimOriginCurrentRepo,
			Authority: types.AuthorityFactual,
			Reason:    "current_repo line-shaped anchor, grounded",
		}
	}
	// Empty / unset scope or unrecognised — fail safe to factual to
	// preserve legacy behaviour for items that pre-date the scope axis.
	return Projection{
		Origin:    types.ClaimOriginCurrentRepo,
		Authority: types.AuthorityFactual,
	}
}

func hasAttachedArtifact(bus *types.BusContext) bool {
	if bus == nil || bus.Mutable == nil {
		return false
	}
	return bus.Mutable.LogTriage() != nil || bus.Mutable.PerfTrace() != nil
}

func computeFromLogPerfDrift(item types.EvidenceItem, bus *types.BusContext) (Projection, bool) {
	if bus.Mutable == nil {
		return Projection{}, false
	}
	logBundle := bus.Mutable.LogTriage()
	perfBundle := bus.Mutable.PerfTrace()
	if logBundle == nil && perfBundle == nil {
		return Projection{}, false
	}

	// Try to find a matching frame.
	matchedFrame, _ := findMatchingFrame(item, logBundle, perfBundle)
	if matchedFrame == nil {
		return Projection{}, false
	}

	// Resolve a SymbolLocator preferring the cross-sub-repo
	// multigraph fan-out (P4-cross-sub-repo, 2026-05-08); falls back
	// to the single-graph locator when MultiGraph isn't wired (legacy
	// path / single-shot tests).
	locator := LocatorFromBusContext(bus)

	drift := logtriage.DetectDriftForFrame(*matchedFrame, locator)

	// Plan-A fuzzy fallback: when graph-driven detection returned
	// Unmappable / FileMoved, but the EVIDENCE POOL has same-file
	// candidates whose anchor names fuzzy-match the log func tail,
	// upgrade to TailRename. This recovers the pre-axis OLD system's
	// tier-2 capability (rename within same file) which graph
	// lookup alone misses because the new symbol name is not in
	// SymbolDefs[oldName].
	if drift.Status == types.DriftStatusUnmappable || drift.Status == types.DriftStatusFileMoved {
		if upgraded, ok := fuzzyTailRenameFallback(*matchedFrame, bus.Mutable.EmittedEvidence()); ok {
			drift = upgraded
		}
	}

	switch drift.Status {
	case types.DriftStatusNone:
		return Projection{
			Origin:    types.ClaimOriginCrossSource,
			Authority: types.AuthorityFactual,
			Reason:    fmt.Sprintf("log+repo agree at %s:%d", matchedFrame.File, matchedFrame.Line),
		}, true
	case types.DriftStatusLineDrift:
		return Projection{
			Origin:    types.ClaimOriginCrossSource,
			Authority: types.AuthorityConditional,
			Reason: fmt.Sprintf("line drift: log %d → repo %d in %s",
				drift.OriginalLine, drift.AnchoredLine, drift.AnchoredFile),
			DriftReason: types.DriftReasonLineDrift,
		}, true
	case types.DriftStatusTailRename:
		return Projection{
			Origin:    types.ClaimOriginCrossSource,
			Authority: types.AuthorityConditional,
			Reason: fmt.Sprintf("function renamed in %s: log func=%s no longer present",
				drift.AnchoredFile, drift.OriginalFunc),
			DriftReason: types.DriftReasonTailRename,
		}, true
	case types.DriftStatusFileMoved:
		return Projection{
			Origin:    types.ClaimOriginCrossSource,
			Authority: types.AuthorityHistorical,
			Reason: fmt.Sprintf("function moved: log file=%s → repo file=%s",
				drift.OriginalFile, drift.AnchoredFile),
			DriftReason: types.DriftReasonFileMoved,
		}, true
	case types.DriftStatusUnmappable:
		return Projection{
			Origin:    types.ClaimOriginCrossSource,
			Authority: types.AuthorityHistorical,
			Reason: fmt.Sprintf("log frame %s:%s no longer maps to current code",
				drift.OriginalFile, drift.OriginalFunc),
			DriftReason: types.DriftReasonFileMoved,
		}, true
	}
	// Unknown / undetectable — treat as a log-derived match without
	// drift information; conservative ceiling of conditional.
	return Projection{
		Origin:    types.ClaimOriginCrossSource,
		Authority: types.AuthorityConditional,
		Reason:    "log/perf-derived anchor, drift detection unavailable",
	}, true
}

// fuzzyTailRenameFallback inspects the evidence pool for items in
// the SAME FILE as the log frame whose anchor symbol or definition
// shape suggests a renamed tail. Returns an upgraded FrameDrift with
// Status=TailRename when fuzzy match plausible. Pure function: no
// side effects on inputs. Plan-A unification preserves the OLD
// system's tier-2 fuzzy capability that graph-only NEW lookup
// misses.
func fuzzyTailRenameFallback(frame types.LogFrame, evidence []types.EvidenceItem) (types.FrameDrift, bool) {
	frameFile := normalizePath(frame.File)
	frameTail := strings.ToLower(strings.TrimSpace(lastSegment(frame.Func)))
	if frameFile == "" || frameTail == "" || len(evidence) == 0 {
		return types.FrameDrift{}, false
	}
	// Scan evidence for same-file definition-anchor items.
	for _, item := range evidence {
		if normalizePath(item.Source) != frameFile {
			continue
		}
		if item.AnchorKind != types.AnchorDefinition {
			continue
		}
		anchor := strings.ToLower(strings.TrimSpace(item.AnchorSymbol))
		if anchor == "" || anchor == frameTail {
			// anchor==frameTail would already match graph-lookup;
			// fuzzy fallback is for the rename case.
			continue
		}
		// Heuristic similarity: shared prefix of length >= 3 OR
		// shared suffix of length >= 3 OR substring containment.
		// Conservative on purpose — fuzzy fallback is best-effort.
		if !looksLikeFuzzyTailRename(anchor, frameTail) {
			continue
		}
		return types.FrameDrift{
			Status:       types.DriftStatusTailRename,
			AnchoredFile: item.Source,
			AnchoredLine: item.LineStart,
			AnchoredFunc: item.AnchorSymbol,
			OriginalFile: frame.File,
			OriginalLine: frame.Line,
			OriginalFunc: frame.Func,
		}, true
	}
	return types.FrameDrift{}, false
}

// looksLikeFuzzyTailRename reports whether two normalised lowercase
// tail names plausibly refer to the same logical function under a
// rename (shared prefix, shared suffix, or substring containment).
// Both inputs are non-empty.
func looksLikeFuzzyTailRename(a, b string) bool {
	if strings.Contains(a, b) || strings.Contains(b, a) {
		return true
	}
	const minOverlap = 3
	prefix := commonPrefixLen(a, b)
	if prefix >= minOverlap {
		return true
	}
	suffix := commonSuffixLen(a, b)
	return suffix >= minOverlap
}

func commonPrefixLen(a, b string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

func commonSuffixLen(a, b string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[len(a)-1-i] != b[len(b)-1-i] {
			return i
		}
	}
	return n
}

// findMatchingFrame searches log + perf bundles for a frame whose
// (file, line, func) matches the evidence item's anchor. Returns the
// frame plus whether the match came from log or perf. Frames are
// matched by repo-relative file path equality PLUS a same-locus test:
//
//   - exact / near-exact line agreement when both sides have lines
//   - OR a symbol-tail agreement between the frame function and the
//     item's semantic locus (anchor / owner / subject / object)
//
// Bare same-file equality is intentionally NOT enough. Otherwise any
// repo line in a logged file inherits log/perf semantics and current
// code anchors get misprojected as external_observation.
func findMatchingFrame(item types.EvidenceItem, logBundle *types.LogBundle, perfBundle *types.PerfBundle) (*types.LogFrame, types.ClaimOrigin) {
	itemFile := normalizePath(item.Source)
	if itemFile == "" {
		return nil, types.ClaimOriginUnknown
	}
	if logBundle != nil {
		if f := walkLogBundleForMatch(logBundle, itemFile, item); f != nil {
			return f, types.ClaimOriginLog
		}
	}
	if perfBundle != nil {
		if f := walkPerfBundleForMatch(perfBundle, itemFile, item); f != nil {
			return f, types.ClaimOriginPerf
		}
	}
	return nil, types.ClaimOriginUnknown
}

func walkLogBundleForMatch(b *types.LogBundle, itemFile string, item types.EvidenceItem) *types.LogFrame {
	var found *types.LogFrame
	var walk func(*types.LogError)
	walk = func(e *types.LogError) {
		if found != nil || e == nil {
			return
		}
		for i := range e.Frames {
			f := &e.Frames[i]
			if !frameMatches(f, itemFile, item) {
				continue
			}
			// Round-9: when the match was symbol-only (frame.File empty),
			// synthesize a fresh frame with File backed by item.Source so
			// downstream DetectDriftForFrame has a where-to-look anchor.
			// Also returns a fresh pointer so we don't accidentally mutate
			// the bundle.
			if normalizePath(f.File) == "" && itemFile != "" {
				synth := *f
				synth.File = item.Source
				found = &synth
				return
			}
			found = f
			return
		}
		walk(e.Cause)
	}
	for i := range b.Errors {
		walk(&b.Errors[i])
		if found != nil {
			break
		}
	}
	return found
}

func walkPerfBundleForMatch(b *types.PerfBundle, itemFile string, item types.EvidenceItem) *types.LogFrame {
	// PerfBundle's Stalls carry (Symbol, File, Line) tuples that map
	// onto LogFrame shape. Synthesize a LogFrame for matching purposes;
	// drift detection downstream needs the same fields whether the
	// origin was a log or a trace.
	//
	// Round-8 trace-symmetry: also match by SYMBOL when stall.File is
	// empty (unsymbolicated traces). This lets trace-only drift
	// detection still fire on the symbol axis even without source
	// path mapping.
	for i := range b.Stalls {
		s := &b.Stalls[i]
		stallFile := normalizePath(s.File)
		fileMatch := stallFile != "" && stallFile == itemFile
		// Symbol-axis fallback: when stall has no file but its symbol
		// matches the item's anchor symbol or owner symbol, synthesize
		// a frame anchored at the item's source so drift detection
		// can still classify (cross-source / line drift / etc).
		symbolMatch := stallFile == "" &&
			strings.TrimSpace(s.Symbol) != "" &&
			(symbolTailEquals(item.AnchorSymbol, s.Symbol) ||
				symbolTailEquals(item.OwnerSymbol, s.Symbol))
		if !fileMatch && !symbolMatch {
			continue
		}
		if fileMatch && item.LineStart > 0 && s.Line > 0 {
			if abs(item.LineStart-s.Line) > logtriage.DefaultDriftLineGap*4 {
				// Loose match — perf stalls report broader windows than
				// log stack frames, so allow more slack.
				continue
			}
		}
		// Reuse LogFrame as the canonical shape downstream expects.
		// For symbol-only matches, set File from the item so downstream
		// knows where to look in current code.
		frame := &types.LogFrame{
			File: s.File,
			Line: s.Line,
			Func: s.Symbol,
		}
		if symbolMatch && frame.File == "" {
			frame.File = item.Source
		}
		return frame
	}
	return nil
}

func frameMatches(f *types.LogFrame, itemFile string, item types.EvidenceItem) bool {
	if f == nil {
		return false
	}
	frameFile := normalizePath(f.File)
	frameFunc := strings.TrimSpace(f.Func)
	// Round-9 user red line: log frames without resolved File MUST
	// still be matchable when the user's question intent aligns
	// with the frame's symbol (e.g. Java basename-failed resolution,
	// <init> constructors, frames whose file path resolution silently
	// degraded). Symbol-axis match works whenever frameFunc is set
	// and equals the item's anchor symbol or owner symbol tail.
	fileMatch := frameFile != "" && frameFile == itemFile
	symbolMatch := frameFile == "" && frameFunc != "" &&
		(symbolTailEquals(item.AnchorSymbol, frameFunc) ||
			symbolTailEquals(item.OwnerSymbol, frameFunc))
	if !fileMatch && !symbolMatch {
		return false
	}
	// Line match (preferred) for the file-equality case.
	if fileMatch && item.LineStart > 0 && f.Line > 0 {
		if abs(item.LineStart-f.Line) <= logtriage.DefaultDriftLineGap {
			return true
		}
		// Large line disagreement is only a valid drift candidate when
		// the semantic locus still names the same function/symbol.
		return itemMatchesFrameSymbol(item, frameFunc)
	}
	if fileMatch {
		// When one side lacks a concrete line, we still require a symbol
		// agreement; file-only equality is too coarse and pollutes every
		// same-file anchor with artifact semantics.
		return itemMatchesFrameSymbol(item, frameFunc)
	}
	// Symbol-only match: already validated by the symbolMatch gate.
	return true
}

func itemMatchesFrameSymbol(item types.EvidenceItem, frameFunc string) bool {
	if strings.TrimSpace(frameFunc) == "" {
		return false
	}
	for _, raw := range []string{
		item.AnchorSymbol,
		item.OwnerSymbol,
		item.Subject,
		item.Object,
	} {
		if symbolTailEquals(raw, frameFunc) {
			return true
		}
	}
	return false
}

func symbolTailEquals(a, b string) bool {
	an := lastSegment(a)
	bn := lastSegment(b)
	if an == "" || bn == "" {
		return false
	}
	return an == bn
}

func lastSegment(s string) string {
	s = strings.TrimSpace(s)
	for _, sep := range []string{"::", "."} {
		if idx := strings.LastIndex(s, sep); idx >= 0 {
			s = s[idx+len(sep):]
		}
	}
	return s
}

func normalizePath(p string) string {
	s := strings.TrimSpace(strings.ReplaceAll(p, `\`, `/`))
	return strings.TrimPrefix(s, "./")
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
