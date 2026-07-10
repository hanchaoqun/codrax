package tracequery

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	TraceClockAlignmentIdentity = "identity"
	TraceClockAlignmentAffine   = "affine"
	TraceClockAlignmentIsolated = "isolated"
)

// TraceArtifactSource is one physical artifact in an Index/Result evidence
// universe. Event and derived-result line coordinates are virtual and unique
// inside the index; VirtualLineBase maps them back to artifact-local lines:
//
//	local line = virtual line - VirtualLineBase
//
// CausalCompatible is the hard clock-domain admission verdict. An artifact
// with a different time domain is admitted only when an explicit calibrated
// affine map was present and applied. Isolated artifacts remain visible here
// (and in caveats), but contribute no Event to the shared causal timeline.
type TraceArtifactSource struct {
	SourcePath          string   `json:"source_path"`
	Kind                string   `json:"kind,omitempty"`
	TimeDomain          string   `json:"time_domain"`
	CanonicalTimeDomain string   `json:"canonical_time_domain"`
	VirtualLineBase     int      `json:"virtual_line_base,omitempty"`
	LocalLineCount      int      `json:"local_line_count,omitempty"`
	EventCount          int      `json:"event_count,omitempty"`
	SourceBytes         int64    `json:"source_bytes,omitempty"`
	SourceModUnixNano   int64    `json:"source_mod_unix_nano,omitempty"`
	CausalCompatible    bool     `json:"causal_compatible"`
	ClockAlignment      string   `json:"clock_alignment"`
	ClockCalibrated     bool     `json:"clock_calibrated,omitempty"`
	ClockOffsetSec      *float64 `json:"clock_offset_sec,omitempty"`
	ClockSlope          *float64 `json:"clock_slope,omitempty"`
	IsolationReason     string   `json:"isolation_reason,omitempty"`

	// sourceIdentity is deliberately private: it is a runtime consistency
	// ledger, not report provenance. Public source coordinates remain stable
	// across platforms while cache/raw/head validation can still bind every
	// later read to the exact file version parsed into this artifact.
	sourceIdentity traceFileIdentity
}

// TraceArtifactSpan is the reversible source coordinate for one virtual line
// range carried by an Event or derived fact.
type TraceArtifactSpan struct {
	SourcePath          string `json:"source_path"`
	TimeDomain          string `json:"time_domain"`
	CanonicalTimeDomain string `json:"canonical_time_domain"`
	LocalLineStart      int    `json:"local_line_start"`
	LocalLineEnd        int    `json:"local_line_end,omitempty"`
	VirtualLineStart    int    `json:"virtual_line_start,omitempty"`
	VirtualLineEnd      int    `json:"virtual_line_end,omitempty"`
}

type traceArtifactSpec struct {
	source TraceArtifactSource
	// perfCapability is the typed manifest authority for admitting a
	// companion perftrace into the shared causal index. It is deliberately
	// private: report provenance is published through the existing bundle
	// capability/admission caveats, while parse-time hard gates consume the
	// exact schema fields instead of re-parsing display text.
	perfCapability          *traceBundlePerfCapability
	targetDomain            string
	alignmentDeclared       bool
	alignmentConflict       bool
	canonicalDomainConflict bool
	forcedIsolationReason   string
}

func traceArtifactSpecsForPaths(paths []string) []traceArtifactSpec {
	seen := map[string]bool{}
	out := make([]traceArtifactSpec, 0, len(paths))
	for _, path := range paths {
		path = canonicalTraceIndexPath(strings.TrimSpace(path))
		if path == "" || seen[path] || traceBundlePath(path) {
			continue
		}
		seen[path] = true
		kind := inferTraceArtifactKind(path)
		domain := inferTraceArtifactTimeDomain(path, kind)
		out = append(out, traceArtifactSpec{source: TraceArtifactSource{
			SourcePath: path,
			Kind:       kind,
			TimeDomain: domain,
		}})
	}
	return finalizeTraceArtifactSpecs(out)
}

func traceArtifactVirtualLineReserve(sourceBytes int64, currentBase int) (int, error) {
	if sourceBytes < 0 {
		return 0, fmt.Errorf("negative artifact size %d", sourceBytes)
	}
	maxInt := int64(^uint(0) >> 1)
	if sourceBytes >= maxInt || int64(currentBase) > maxInt-sourceBytes-1 {
		return 0, fmt.Errorf("artifact is too large for stable virtual line coordinates")
	}
	// Every physical line contains at least one byte; size+1 is therefore a
	// stable, query-independent non-overlapping reservation without a second
	// full-file line-count pass over potentially GB-scale traces.
	return int(sourceBytes) + 1, nil
}

func traceArtifactBuildOptions(opts BuildOptions, source TraceArtifactSource) (BuildOptions, bool, error) {
	child := opts
	if opts.LineEnd > 0 && opts.LineEnd <= source.VirtualLineBase {
		return child, false, nil
	}
	virtualCapacityEnd := source.VirtualLineBase + int(source.SourceBytes)
	if opts.LineStart > 0 && opts.LineStart > virtualCapacityEnd {
		return child, false, nil
	}
	if opts.LineStart > 0 {
		child.LineStart = opts.LineStart - source.VirtualLineBase
		if child.LineStart < 1 {
			child.LineStart = 1
		}
	}
	if opts.LineEnd > 0 {
		child.LineEnd = opts.LineEnd - source.VirtualLineBase
		if child.LineEnd < 1 {
			return child, false, nil
		}
	}
	if source.ClockAlignment == TraceClockAlignmentAffine {
		if opts.TimeStartSet {
			mapped, ok := source.toSourceTsChecked(opts.TimeStart)
			if !ok {
				return child, false, fmt.Errorf("clock inverse cannot safely and reversibly represent time_start")
			}
			// Child gates are inclusive. The nearest IEEE-754 inverse can round
			// upward and exclude a source timestamp whose canonical value lies
			// exactly on time_start, so widen the scan bound by one source ULP.
			child.TimeStart = traceClockWidenULPs(mapped, math.Inf(-1), traceClockRoundTripMaxULPs)
		}
		if opts.TimeEndSet {
			mapped, ok := source.toSourceTsChecked(opts.TimeEnd)
			if !ok {
				return child, false, fmt.Errorf("clock inverse cannot safely and reversibly represent time_end")
			}
			child.TimeEnd = traceClockWidenULPs(mapped, math.Inf(1), traceClockRoundTripMaxULPs)
		}
		_, slope := traceClockMapValues(source.ClockOffsetSec, source.ClockSlope)
		child.TimePaddingBefore = traceClockWidenULPs(opts.TimePaddingBefore/slope, math.Inf(1), traceClockRoundTripMaxULPs)
		child.TimePaddingAfter = traceClockWidenULPs(opts.TimePaddingAfter/slope, math.Inf(1), traceClockRoundTripMaxULPs)
	}
	return child, true, nil
}

func traceBundleArtifactSpecs(bundlePath string, bundle traceBundleFile) []traceArtifactSpec {
	baseDir := filepath.Dir(bundlePath)
	type declaredArtifact struct {
		path       string
		kind       string
		capability *traceBundlePerfCapability
	}
	seen := map[string]bool{}
	declarations := make([]declaredArtifact, 0, len(bundle.Artifacts)+1)
	declare := func(rawPath, rawKind string, capability *traceBundlePerfCapability) {
		path := resolveTraceBundleArtifactPath(baseDir, strings.TrimSpace(rawPath))
		if path == "" || seen[path] || traceBundlePath(path) {
			return
		}
		kind := strings.ToLower(strings.TrimSpace(rawKind))
		if kind == "" {
			kind = inferTraceArtifactKind(path)
		}
		// The dedicated .perftrace suffix is itself a precise artifact-format
		// signal. A malformed manifest must not reclassify that path through the
		// top-level systrace field (or type=systrace) and thereby bypass the perf
		// capability/admission gate.
		if inferTraceArtifactKind(path) == "perftrace" {
			kind = "perftrace"
		}
		if kind != "systrace" && kind != "perftrace" {
			return
		}
		seen[path] = true
		declarations = append(declarations, declaredArtifact{path: path, kind: kind, capability: capability})
	}
	// Declaration order is also the artifact merge order. In particular, the
	// primary trace body owns its path even if a malformed manifest repeats it
	// later with a different type.
	declare(bundle.Systrace, "systrace", nil)
	for i := range bundle.Artifacts {
		artifact := &bundle.Artifacts[i]
		declare(artifact.Path, artifact.Type, artifact.Perf)
	}

	declaredPerftraces := make(map[string]bool, len(declarations))
	for _, declaration := range declarations {
		if declaration.kind == "perftrace" {
			declaredPerftraces[declaration.path] = true
		}
	}

	alignments := map[string]traceBundlePerfClockAlignment{}
	alignmentConflicts := map[string]bool{}
	for _, alignment := range bundle.PerfClockAlignments {
		path := resolveTraceBundleArtifactPath(baseDir, strings.TrimSpace(alignment.ArtifactPath))
		// Alignment records are capabilities of a declared perftrace artifact,
		// not declarations themselves. Foreign/stale paths and records aimed at
		// the primary systrace must have no authority over either artifact
		// admission or the canonical trace clock.
		if path == "" || !declaredPerftraces[path] {
			continue
		}
		if previous, exists := alignments[path]; exists && !sameTraceClockAlignment(previous, alignment) {
			alignmentConflicts[path] = true
		} else if !exists {
			alignments[path] = alignment
		}
	}

	// Derive the canonical trace clock only from usable, non-conflicting
	// alignment declarations which can themselves be admitted after that
	// derivation. In particular, an uncalibrated/non-finite/incomplete mapping
	// must not relabel the primary trace and thereby make an unrelated third
	// artifact appear same-domain. A rejected declaration remains visible on
	// its artifact below, but has no canonical-clock authority.
	traceDomain := ""
	traceDomainConflict := false
	for _, declaration := range declarations {
		if declaration.kind != "perftrace" || alignmentConflicts[declaration.path] {
			continue
		}
		alignment, exists := alignments[declaration.path]
		if !exists || !traceClockAlignmentCanDefineCanonicalDomain(alignment) {
			continue
		}
		candidate := strings.TrimSpace(alignment.TraceTimeDomain)
		switch {
		case traceDomain == "":
			traceDomain = candidate
		case !sameTraceTimeDomain(traceDomain, candidate):
			traceDomainConflict = true
		}
	}
	if traceDomainConflict {
		traceDomain = ""
	}

	// A time-domain label is not a capture identity. Without a typed shared-
	// capture proof in the manifest schema, merging two distinct systrace files
	// can fabricate ordering and durations across unrelated recordings. The
	// first declared systrace is the sole causal authority; exact duplicate paths
	// were already deduplicated above and therefore remain a compatibility no-op.
	primarySystracePath := ""
	for _, declaration := range declarations {
		if declaration.kind == "systrace" {
			primarySystracePath = declaration.path
			break
		}
	}

	var out []traceArtifactSpec
	for _, declaration := range declarations {
		path, kind, capability := declaration.path, declaration.kind, declaration.capability
		domain := ""
		if capability != nil {
			domain = strings.TrimSpace(capability.TimeDomain)
		}
		if kind == "systrace" && traceDomain != "" {
			domain = traceDomain
		}
		alignment, hasAlignment := alignments[path]
		if alignmentConflicts[path] {
			// No arbitrary member of a conflicting declaration set is valid
			// coefficient provenance.
			alignment = traceBundlePerfClockAlignment{}
			hasAlignment = false
		}
		if hasAlignment && strings.TrimSpace(alignment.PerfTimeDomain) != "" && kind == "perftrace" {
			// The artifact-specific alignment record is the authority for the
			// source clock. A capability label may describe the rendered unit
			// (seconds) while the alignment records the actual clock epoch.
			domain = strings.TrimSpace(alignment.PerfTimeDomain)
		}
		if domain == "" {
			domain = inferTraceArtifactTimeDomain(path, kind)
		}
		spec := traceArtifactSpec{source: TraceArtifactSource{
			SourcePath:      path,
			Kind:            kind,
			TimeDomain:      domain,
			ClockOffsetSec:  alignment.OffsetSec,
			ClockSlope:      alignment.Slope,
			ClockCalibrated: hasAlignment && alignment.Calibrated,
		}, perfCapability: capability}
		if hasAlignment {
			spec.targetDomain = strings.TrimSpace(alignment.TraceTimeDomain)
		}
		spec.alignmentDeclared = hasAlignment || alignmentConflicts[path]
		spec.alignmentConflict = alignmentConflicts[path]
		spec.canonicalDomainConflict = traceDomainConflict && kind != "systrace" && spec.alignmentDeclared
		if kind == "systrace" && primarySystracePath != "" && path != primarySystracePath {
			spec.forcedIsolationReason = "additional systrace lacks explicit shared-capture identity proof; only one systrace causal authority is permitted"
		}
		out = append(out, spec)
	}
	return finalizeTraceArtifactSpecs(out)
}

func finalizeTraceArtifactSpecs(specs []traceArtifactSpec) []traceArtifactSpec {
	if len(specs) == 0 {
		return nil
	}
	canonical := strings.TrimSpace(specs[0].source.TimeDomain)
	// Prefer the trace-body clock when present even if a hand-authored bundle
	// listed a perf artifact first.
	for _, spec := range specs {
		if spec.source.Kind == "systrace" {
			canonical = strings.TrimSpace(spec.source.TimeDomain)
			break
		}
	}
	if canonical == "" {
		canonical = "trace_seconds"
	}
	for i := range specs {
		source := &specs[i].source
		if strings.TrimSpace(source.TimeDomain) == "" {
			source.TimeDomain = inferTraceArtifactTimeDomain(source.SourcePath, source.Kind)
		}
		source.CanonicalTimeDomain = canonical
		sameDomain := sameTraceTimeDomain(source.TimeDomain, canonical)
		validMap := validTraceClockMap(source.ClockOffsetSec, source.ClockSlope)
		identityMap := traceClockMapIsIdentity(source.ClockOffsetSec, source.ClockSlope)
		switch {
		case specs[i].forcedIsolationReason != "":
			source.CausalCompatible = false
			source.ClockAlignment = TraceClockAlignmentIsolated
			source.IsolationReason = specs[i].forcedIsolationReason
		case specs[i].alignmentConflict:
			source.CausalCompatible = false
			source.ClockAlignment = TraceClockAlignmentIsolated
			source.IsolationReason = "conflicting duplicate clock alignment records for artifact"
		case specs[i].canonicalDomainConflict:
			source.CausalCompatible = false
			source.ClockAlignment = TraceClockAlignmentIsolated
			source.IsolationReason = "bundle declares conflicting canonical trace clock domains"
		case sameDomain && specs[i].alignmentDeclared && source.ClockCalibrated && validMap && sameTraceTimeDomain(specs[i].targetDomain, canonical):
			// An explicit calibrated non-identity map is authoritative even when
			// the source/target labels are textually equal; silently treating it
			// as identity corrupts the canonical time coordinate.
			source.CausalCompatible = true
			source.ClockAlignment = TraceClockAlignmentAffine
		case sameDomain && specs[i].alignmentDeclared && source.ClockCalibrated:
			source.CausalCompatible = false
			source.ClockAlignment = TraceClockAlignmentIsolated
			switch {
			case !sameTraceTimeDomain(specs[i].targetDomain, canonical):
				source.IsolationReason = "calibrated mapping targets a different canonical domain"
			default:
				source.IsolationReason = "calibrated mapping is missing a finite affine coefficient"
			}
		case sameDomain && specs[i].alignmentDeclared && !source.ClockCalibrated && !identityMap:
			source.CausalCompatible = false
			source.ClockAlignment = TraceClockAlignmentIsolated
			source.IsolationReason = "non-identity clock mapping was not marked calibrated"
		case sameDomain:
			source.CausalCompatible = true
			source.ClockAlignment = TraceClockAlignmentIdentity
		case source.ClockCalibrated &&
			sameTraceTimeDomain(specs[i].targetDomain, canonical) &&
			validTraceClockMap(source.ClockOffsetSec, source.ClockSlope):
			source.CausalCompatible = true
			source.ClockAlignment = TraceClockAlignmentAffine
		default:
			source.CausalCompatible = false
			source.ClockAlignment = TraceClockAlignmentIsolated
			switch {
			case !source.ClockCalibrated:
				source.IsolationReason = "different clock domain without calibrated mapping"
			case !validMap:
				source.IsolationReason = "calibrated mapping is missing a finite affine coefficient"
			case !sameTraceTimeDomain(specs[i].targetDomain, canonical):
				source.IsolationReason = "calibrated mapping targets a different canonical domain"
			}
		}
	}
	return specs
}

func traceClockAlignmentCanDefineCanonicalDomain(alignment traceBundlePerfClockAlignment) bool {
	// Uncalibrated same-domain declarations remain admissible as identity only
	// when they already match the primary's independently known domain. They do
	// not have enough authority to rename that primary domain.
	if !alignment.Calibrated || !validTraceClockMap(alignment.OffsetSec, alignment.Slope) {
		return false
	}
	source := strings.TrimSpace(alignment.PerfTimeDomain)
	target := strings.TrimSpace(alignment.TraceTimeDomain)
	return source != "" && target != "" && !strings.EqualFold(target, "missing_trace_body")
}

func sameTraceClockAlignment(a, b traceBundlePerfClockAlignment) bool {
	return normalizeTraceTimeDomain(a.PerfTimeDomain) == normalizeTraceTimeDomain(b.PerfTimeDomain) &&
		normalizeTraceTimeDomain(a.TraceTimeDomain) == normalizeTraceTimeDomain(b.TraceTimeDomain) &&
		a.Calibrated == b.Calibrated &&
		sameOptionalFloat(a.OffsetSec, b.OffsetSec) &&
		sameOptionalFloat(a.Slope, b.Slope)
}

func sameOptionalFloat(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return math.Float64bits(*a) == math.Float64bits(*b)
}

func traceClockMapIsIdentity(offset, slope *float64) bool {
	if !validTraceClockMap(offset, slope) {
		return offset == nil && slope == nil
	}
	off, mul := traceClockMapValues(offset, slope)
	return off == 0 && mul == 1
}

func inferTraceArtifactKind(path string) string {
	lower := strings.ToLower(strings.TrimSpace(path))
	switch {
	case strings.HasSuffix(lower, ".perftrace"):
		return "perftrace"
	case strings.HasSuffix(lower, ".systrace"):
		return "systrace"
	case strings.HasSuffix(lower, ".htrace"):
		return "htrace"
	case strings.HasSuffix(lower, ".atrace"):
		return "atrace"
	case strings.HasSuffix(lower, ".ftrace"):
		return "ftrace"
	case strings.HasSuffix(lower, ".trace"):
		return "trace"
	default:
		return strings.TrimPrefix(strings.ToLower(filepath.Ext(lower)), ".")
	}
}

func tracePathRequiresCompositeIndex(path string) bool {
	path = canonicalTraceIndexPath(path)
	if traceBundlePath(path) {
		return true
	}
	// Explicit perftrace is the intentional per-clock-domain escape hatch.
	// Never promote it back into a sibling universe.
	if strings.HasSuffix(strings.ToLower(path), ".perftrace") {
		return false
	}
	if promoted := promoteSiblingTraceBundlePath(path); promoted != path && traceBundlePath(promoted) {
		return true
	}
	return len(siblingTraceArtifactPaths(path)) > 0
}

// TracePathRequiresCompositeIndex reports whether path belongs to a
// multi-artifact evidence universe. Callers must use the indexed path for
// these sources so artifact coordinates and clock-domain admission are not
// bypassed by a single-file streaming optimization.
func TracePathRequiresCompositeIndex(path string) bool {
	return tracePathRequiresCompositeIndex(path)
}

func inferTraceArtifactTimeDomain(path, kind string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "perftrace" || strings.HasSuffix(strings.ToLower(strings.TrimSpace(path)), ".perftrace") {
		return "perf_event_time"
	}
	return "trace_seconds"
}

func normalizeTraceTimeDomain(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func sameTraceTimeDomain(a, b string) bool {
	a = normalizeTraceTimeDomain(a)
	b = normalizeTraceTimeDomain(b)
	return a != "" && a == b
}

func validTraceClockMap(offset, slope *float64) bool {
	if offset == nil && slope == nil {
		return false
	}
	if offset != nil && (math.IsNaN(*offset) || math.IsInf(*offset, 0)) {
		return false
	}
	if slope != nil && (math.IsNaN(*slope) || math.IsInf(*slope, 0) || *slope <= 0) {
		return false
	}
	return true
}

func traceClockMapValues(offset, slope *float64) (float64, float64) {
	off, mul := 0.0, 1.0
	if offset != nil {
		off = *offset
	}
	if slope != nil {
		mul = *slope
	}
	return off, mul
}

func applyTraceClockMap(ts float64, offset, slope *float64) float64 {
	off, mul := traceClockMapValues(offset, slope)
	return ts*mul + off
}

func invertTraceClockMap(ts float64, offset, slope *float64) float64 {
	off, mul := traceClockMapValues(offset, slope)
	return (ts - off) / mul
}

// ToCanonicalTs maps an artifact-local timestamp into the index canonical
// domain. Callers must first require CausalCompatible.
func (s TraceArtifactSource) ToCanonicalTs(sourceTs float64) float64 {
	if s.ClockAlignment == TraceClockAlignmentAffine {
		return applyTraceClockMap(sourceTs, s.ClockOffsetSec, s.ClockSlope)
	}
	return sourceTs
}

func (s TraceArtifactSource) toCanonicalTsChecked(sourceTs float64) (float64, bool) {
	if math.IsNaN(sourceTs) || math.IsInf(sourceTs, 0) {
		return 0, false
	}
	mapped := s.ToCanonicalTs(sourceTs)
	if math.IsNaN(mapped) || math.IsInf(mapped, 0) {
		return 0, false
	}
	if s.ClockAlignment == TraceClockAlignmentAffine {
		roundTrip := s.ToSourceTs(mapped)
		if !traceClockRoundTripWithinULPs(sourceTs, roundTrip) {
			return 0, false
		}
	}
	return mapped, true
}

// ToSourceTs is the exact inverse used when a canonical query boundary must
// be applied while scanning the physical artifact.
func (s TraceArtifactSource) ToSourceTs(canonicalTs float64) float64 {
	if s.ClockAlignment == TraceClockAlignmentAffine {
		return invertTraceClockMap(canonicalTs, s.ClockOffsetSec, s.ClockSlope)
	}
	return canonicalTs
}

func (s TraceArtifactSource) toSourceTsChecked(canonicalTs float64) (float64, bool) {
	if math.IsNaN(canonicalTs) || math.IsInf(canonicalTs, 0) {
		return 0, false
	}
	mapped := s.ToSourceTs(canonicalTs)
	if math.IsNaN(mapped) || math.IsInf(mapped, 0) {
		return 0, false
	}
	if s.ClockAlignment == TraceClockAlignmentAffine {
		roundTrip := s.ToCanonicalTs(mapped)
		if !traceClockRoundTripWithinULPs(canonicalTs, roundTrip) {
			return 0, false
		}
	}
	return mapped, true
}

const traceClockRoundTripMaxULPs = uint64(8)

func traceClockWidenULPs(value, toward float64, ulps uint64) float64 {
	for i := uint64(0); i < ulps; i++ {
		value = math.Nextafter(value, toward)
	}
	return value
}

// traceClockRoundTripWithinULPs rejects affine maps which are finite but lose
// enough precision that the inverse no longer identifies the input timestamp.
// A small ULP allowance absorbs the two IEEE-754 operations in the affine
// round trip without using a scale-relative epsilon that could accidentally
// bless seconds (or years) of loss at very large clock epochs.
func traceClockRoundTripWithinULPs(want, got float64) bool {
	if math.IsNaN(want) || math.IsNaN(got) || math.IsInf(want, 0) || math.IsInf(got, 0) {
		return false
	}
	if want == got { // also treats +0 and -0 as equivalent
		return true
	}
	wantOrdered := traceClockOrderedFloatBits(want)
	gotOrdered := traceClockOrderedFloatBits(got)
	if wantOrdered > gotOrdered {
		return wantOrdered-gotOrdered <= traceClockRoundTripMaxULPs
	}
	return gotOrdered-wantOrdered <= traceClockRoundTripMaxULPs
}

func traceClockOrderedFloatBits(value float64) uint64 {
	bits := math.Float64bits(value)
	if bits&(uint64(1)<<63) != 0 {
		return ^bits
	}
	return bits | (uint64(1) << 63)
}

func singleTraceArtifactSource(path string, sourceBytes int64, sourceModUnixNano int64, lineCount, eventCount int) TraceArtifactSource {
	identity := traceFileIdentity{size: sourceBytes, modUnixNano: sourceModUnixNano, initialized: true}
	if info, err := os.Stat(path); err == nil && info.Size() == sourceBytes && info.ModTime().UnixNano() == sourceModUnixNano {
		identity = traceFileIdentityFromInfo(info)
	}
	return singleTraceArtifactSourceWithIdentity(path, identity, lineCount, eventCount)
}

func singleTraceArtifactSourceWithIdentity(path string, identity traceFileIdentity, lineCount, eventCount int) TraceArtifactSource {
	kind := inferTraceArtifactKind(path)
	domain := inferTraceArtifactTimeDomain(path, kind)
	return TraceArtifactSource{
		SourcePath:          path,
		Kind:                kind,
		TimeDomain:          domain,
		CanonicalTimeDomain: domain,
		LocalLineCount:      lineCount,
		EventCount:          eventCount,
		SourceBytes:         identity.size,
		SourceModUnixNano:   identity.modUnixNano,
		CausalCompatible:    true,
		ClockAlignment:      TraceClockAlignmentIdentity,
		sourceIdentity:      identity,
	}
}

func (s TraceArtifactSource) identityMatchesInfo(info os.FileInfo) bool {
	if info == nil {
		return false
	}
	if s.sourceIdentity.initialized {
		return s.sourceIdentity.matchesInfo(info)
	}
	// Compatibility for hand-built and deserialized Index values that predate
	// the private ledger. Production parsers always populate sourceIdentity.
	if s.SourceBytes > 0 && info.Size() != s.SourceBytes {
		return false
	}
	if s.SourceModUnixNano != 0 && info.ModTime().UnixNano() != s.SourceModUnixNano {
		return false
	}
	return true
}

func (s TraceArtifactSource) identityToken(info os.FileInfo) string {
	if s.sourceIdentity.initialized {
		return s.sourceIdentity.cacheToken()
	}
	return traceFileIdentityFromInfo(info).cacheToken()
}

// ResolveArtifactSpans maps an index-global virtual range back to physical
// source coordinates. Isolated artifacts have no admitted virtual events and
// therefore never resolve here.
func (idx *Index) ResolveArtifactSpans(lineStart, lineEnd int) []TraceArtifactSpan {
	if idx == nil {
		return nil
	}
	return resolveTraceArtifactSpans(idx.TraceArtifacts, lineStart, lineEnd)
}

// ResolveArtifactSpans is the Result-side twin used after an Index has left
// the query engine (notably by the typed observation publisher).
func (r Result) ResolveArtifactSpans(lineStart, lineEnd int) []TraceArtifactSpan {
	return resolveTraceArtifactSpans(r.TraceArtifacts, lineStart, lineEnd)
}

func resolveTraceArtifactSpans(sources []TraceArtifactSource, lineStart, lineEnd int) []TraceArtifactSpan {
	if lineStart <= 0 || len(sources) == 0 {
		return nil
	}
	if lineEnd <= 0 || lineEnd < lineStart {
		lineEnd = lineStart
	}
	var out []TraceArtifactSpan
	for _, source := range sources {
		if !source.CausalCompatible || source.LocalLineCount <= 0 {
			continue
		}
		virtualStart := source.VirtualLineBase + 1
		virtualEnd := source.VirtualLineBase + source.LocalLineCount
		start := lineStart
		if start < virtualStart {
			start = virtualStart
		}
		end := lineEnd
		if end > virtualEnd {
			end = virtualEnd
		}
		if start > end {
			continue
		}
		out = append(out, TraceArtifactSpan{
			SourcePath:          source.SourcePath,
			TimeDomain:          source.TimeDomain,
			CanonicalTimeDomain: source.CanonicalTimeDomain,
			LocalLineStart:      start - source.VirtualLineBase,
			LocalLineEnd:        end - source.VirtualLineBase,
			VirtualLineStart:    start,
			VirtualLineEnd:      end,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].VirtualLineStart < out[j].VirtualLineStart })
	return out
}

func (idx *Index) eventView(ev Event, raw string) EventView {
	view := EventView{Event: ev, Raw: raw}
	spans := idx.ResolveArtifactSpans(ev.Line, ev.Line)
	if len(spans) != 1 {
		return view
	}
	span := spans[0]
	if source, ok := traceArtifactSourceForPath(idx.TraceArtifacts, span.SourcePath); ok {
		return eventViewFromSource(ev, raw, source, span.LocalLineStart)
	}
	view.SourcePath = span.SourcePath
	view.LocalLine = span.LocalLineStart
	view.TimeDomain = span.TimeDomain
	view.CanonicalTimeDomain = span.CanonicalTimeDomain
	view.SourceTs = ev.Ts
	return view
}

func eventViewFromSource(ev Event, raw string, source TraceArtifactSource, localLine int) EventView {
	view := EventView{
		Event:               ev,
		Raw:                 raw,
		SourcePath:          source.SourcePath,
		LocalLine:           localLine,
		TimeDomain:          source.TimeDomain,
		CanonicalTimeDomain: source.CanonicalTimeDomain,
		SourceTs:            ev.Ts,
	}
	if source.ClockAlignment == TraceClockAlignmentAffine {
		if sourceTs, ok := source.toSourceTsChecked(ev.Ts); ok {
			view.SourceTs = sourceTs
			view.ClockAligned = true
		} else {
			view.RawUnavailableReason = "clock_inverse_unsafe"
		}
	}
	return view
}

func traceArtifactSourceForPath(sources []TraceArtifactSource, path string) (TraceArtifactSource, bool) {
	for _, source := range sources {
		if source.SourcePath == path {
			return source, true
		}
	}
	return TraceArtifactSource{}, false
}

func attachEvidenceFactProvenance(facts []EvidenceFact, sources []TraceArtifactSource) {
	for i := range facts {
		facts[i].SourceSpans = resolveTraceArtifactSpans(sources, facts[i].LineStart, facts[i].LineEnd)
	}
}
