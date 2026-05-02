// Package authority centralises the AuthorityCeiling axis projection
// logic. Lives in its own package (rather than hanging off types/) so
// the projection function can pull in the logtriage drift detector
// without introducing import cycles — analysis/logtriage and
// internal/tool/* both depend on internal/types, but neither should
// depend on each other.
//
// The package exposes three things:
//
//  1. Feature gate: Enabled() / Enable(on). cmd/root.go calls Enable
//     once at startup based on `authority_ceiling_enabled` yaml.
//     When false, ComputeFor* is a no-op (returns zero values) so
//     every emit_* tool that opts in degrades to byte-identical
//     legacy behaviour.
//
//  2. ComputeForEvidence: the deterministic projection from
//     (EvidenceItem, BusContext) → (Origin, Authority, Reason).
//     Reads grounding tier, scope, log/perf bundles, and (when a
//     SymbolLocator is wired) drift status. No mutation.
//
//  3. SymbolLocatorProvider: cmd/root.go provides an implementation
//     that wraps repomap.NewSymbolLocator; emit_evidence consults
//     this provider to obtain a locator without importing repomap.
//     Indirection avoids a cycle between authority and tool/repomap.
package authority

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/hanchaoqun/codrax/internal/analysis/logtriage"
	"github.com/hanchaoqun/codrax/internal/types"
)

var (
	enabled atomic.Bool

	locatorMu       sync.RWMutex
	locatorProvider func(graph any) types.SymbolLocator
)

// Enable toggles the AuthorityCeiling axis on/off. Off (default) means
// every ComputeFor* returns zero values, preserving byte-identical
// legacy behaviour for callers that haven't migrated. cmd/root.go
// calls this once at startup based on the
// authority_ceiling_enabled yaml knob; tests may flip it inside a
// per-test SetEnabled-Reset bracket.
func Enable(on bool) {
	enabled.Store(on)
}

// Enabled reports whether the AuthorityCeiling axis is active.
// emit_evidence and other producers short-circuit their hooks when
// this returns false.
func Enabled() bool {
	return enabled.Load()
}

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

// ComputeForEvidence is the canonical projection from a constructed
// EvidenceItem (post-grounding) and the bus context to a typed
// (ClaimOrigin, AuthorityCeiling, AuthorityReason) triple. Pure
// function: no mutation of inputs; same inputs always produce the
// same output.
//
// Algorithm (priority order — first hit wins):
//
//  1. Disabled gate / nil bus → returns zero values (legacy
//     passthrough; caller leaves item.Origin / Authority untouched).
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
//     - LineDrift / TailRename → log/perf + conditional.
//     - FileMoved / Unmappable → log/perf + historical.
//
//  5. No log/perf match → ClaimOriginCurrentRepo. Authority is
//     factual when grounding tier is strong (line_text /
//     symbol_table) and scope is line-shaped; conditional otherwise
//     (recovered tiers and section/range scopes get a softer cap by
//     default — they survive grounding but the anchor is less
//     precise).
func ComputeForEvidence(item types.EvidenceItem, bus *types.BusContext) (types.ClaimOrigin, types.AuthorityCeiling, string) {
	if !Enabled() || bus == nil {
		return types.ClaimOriginUnknown, types.AuthorityUnknown, ""
	}

	// (2) Schema-level scopes: factual current_repo by construction.
	switch item.Scope {
	case types.ScopeFile, types.ScopeCrossfile, types.ScopeNegative:
		return types.ClaimOriginCurrentRepo, types.AuthorityFactual,
			"schema-level scope: structural assertion about current repo"
	}

	// (3) Illustrative / ungrounded carve-out.
	if item.ContextRole == types.EvidenceContextRoleIllustrativeOnly {
		return types.ClaimOriginCurrentRepo, types.AuthorityIllustrative,
			"context_role=illustrative_only — not a causal claim"
	}
	if item.GroundingStatus == types.GroundingUngrounded {
		return types.ClaimOriginCurrentRepo, types.AuthorityIllustrative,
			"grounding_status=ungrounded — anchor unverified"
	}

	// (4) Log / perf bundle match — strongest evidence of drift.
	if origin, ceiling, reason, matched := computeFromLogPerfDrift(item, bus); matched {
		return origin, ceiling, reason
	}

	// (5) Default current_repo path — graded by grounding tier + scope.
	switch item.GroundingStatus {
	case types.GroundingRecovered:
		// Recovery tier rewrote line/source. Anchor IS now valid, but
		// the LLM's original claim wasn't precise — soften.
		return types.ClaimOriginCurrentRepo, types.AuthorityConditional,
			fmt.Sprintf("grounding recovered via %s — original anchor imprecise", item.GroundingTier)
	}
	if item.Scope.IsLineShaped() {
		return types.ClaimOriginCurrentRepo, types.AuthorityFactual,
			"current_repo line-shaped anchor, grounded"
	}
	// Empty / unset scope or unrecognised — fail safe to factual to
	// preserve legacy behaviour for items that pre-date the scope axis.
	return types.ClaimOriginCurrentRepo, types.AuthorityFactual, ""
}

func computeFromLogPerfDrift(item types.EvidenceItem, bus *types.BusContext) (types.ClaimOrigin, types.AuthorityCeiling, string, bool) {
	if bus.Mutable == nil {
		return "", "", "", false
	}
	logBundle := bus.Mutable.LogTriage()
	perfBundle := bus.Mutable.PerfTrace()
	if logBundle == nil && perfBundle == nil {
		return "", "", "", false
	}

	// Try to find a matching frame.
	matchedFrame, originKind := findMatchingFrame(item, logBundle, perfBundle)
	if matchedFrame == nil {
		return "", "", "", false
	}

	// Resolve a SymbolLocator from the bus search graph.
	var locator types.SymbolLocator
	if g := bus.Mutable.SearchGraph(); g != nil {
		locator = LocatorFromGraph(g)
	}

	drift := logtriage.DetectDriftForFrame(*matchedFrame, locator)

	switch drift.Status {
	case types.DriftStatusNone:
		return types.ClaimOriginCrossSource, types.AuthorityFactual,
			fmt.Sprintf("log+repo agree at %s:%d", matchedFrame.File, matchedFrame.Line), true
	case types.DriftStatusLineDrift:
		return originKind, types.AuthorityConditional,
			fmt.Sprintf("line drift: log %d → repo %d in %s",
				drift.OriginalLine, drift.AnchoredLine, drift.AnchoredFile), true
	case types.DriftStatusTailRename:
		return originKind, types.AuthorityConditional,
			fmt.Sprintf("function renamed in %s: log func=%s no longer present",
				drift.AnchoredFile, drift.OriginalFunc), true
	case types.DriftStatusFileMoved:
		return originKind, types.AuthorityHistorical,
			fmt.Sprintf("function moved: log file=%s → repo file=%s",
				drift.OriginalFile, drift.AnchoredFile), true
	case types.DriftStatusUnmappable:
		return originKind, types.AuthorityHistorical,
			fmt.Sprintf("log frame %s:%s no longer maps to current code",
				drift.OriginalFile, drift.OriginalFunc), true
	}
	// Unknown / undetectable — treat as a log-derived match without
	// drift information; conservative ceiling of conditional.
	return originKind, types.AuthorityConditional,
		"log/perf-derived anchor, drift detection unavailable", true
}

// findMatchingFrame searches log + perf bundles for a frame whose
// (file, line, func) matches the evidence item's anchor. Returns the
// frame plus whether the match came from log or perf. Frames are
// matched by repo-relative file path equality + (when both have a
// non-zero line) line equality within DefaultDriftLineGap, OR by file
// + AnchorSymbol/Func equality if line is missing.
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
			if frameMatches(f, itemFile, item) {
				found = f
				return
			}
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
	for i := range b.Stalls {
		s := &b.Stalls[i]
		if normalizePath(s.File) != itemFile {
			continue
		}
		if item.LineStart > 0 && s.Line > 0 {
			if abs(item.LineStart-s.Line) > logtriage.DefaultDriftLineGap*4 {
				// Loose match — perf stalls report broader windows than
				// log stack frames, so allow more slack.
				continue
			}
		}
		// Reuse LogFrame as the canonical shape downstream expects.
		return &types.LogFrame{
			File: s.File,
			Line: s.Line,
			Func: s.Symbol,
		}
	}
	return nil
}

func frameMatches(f *types.LogFrame, itemFile string, item types.EvidenceItem) bool {
	if f == nil {
		return false
	}
	if normalizePath(f.File) != itemFile {
		return false
	}
	// Line match (preferred).
	if item.LineStart > 0 && f.Line > 0 {
		if abs(item.LineStart-f.Line) <= logtriage.DefaultDriftLineGap {
			return true
		}
		// Lines disagree but file matches — still a candidate; drift
		// detection downstream will classify the gap.
		return true
	}
	// Symbol-based match as fallback when one side lacks a line.
	if anchor := strings.TrimSpace(item.AnchorSymbol); anchor != "" && f.Func != "" {
		if symbolTailEquals(f.Func, anchor) {
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
