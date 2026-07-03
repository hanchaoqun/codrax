package types

// Multi-artifact partitioning for the Trace Causal Projection (CMP-1,
// docs/design/customer_dead_session_audit_20260703.md §7.2 方案 A).
//
// Root cause being fixed: CompileTraceCausalProjection and the whole render
// chain were single-artifact by assumption, so a two-trace comparison ledger
// (custom_compare.txt: 7.0 vs 6.0 bindApplication) blended both traces'
// observations into ONE tree, one bar scale and one background stanza.
//
// Design (方案 A — partition at the compile entry, reuse the single-artifact
// compiler verbatim; NO parallel subsystem):
//   - records partition by their typed artifact identity: canonicalised
//     SourceRef.Path (shared canonicaliser internal/canonpath — never a string
//     heuristic), then SourceRef.ArtifactID (excluding the known production
//     constants "attached_trace"/"trace_query" — F4a), then the artifact path
//     carried by the evidence locator (SupportRefs "path:line-range", both
//     line bounds non-empty pure 1-based digits — F4b);
//   - path partitions that are spelling variants of ONE file (a ≥2-segment
//     verbatim tail-suffix relation, e.g. relative vs absolute) merge before
//     compiling, and colliding basename labels upgrade to the last two path
//     segments (F5);
//   - each partition independently runs TraceCausalProjectionFromObservationRecords
//     (zero behavior change on the single-artifact path);
//   - records with NO artifact identity in a multi-artifact ledger go to the
//     "unattributed" bucket: they compile into no tree and surface only as a
//     renderer caveat;
//   - at most traceCausalProjectionMaxArtifactPartitions partitions are kept
//     (the ones with the most observations; ties keep first-appearance order)
//     to bound pathological inputs — omitted artifacts are named for the
//     caveat;
//   - a ledger with ≤1 distinct artifact identity compiles EXACTLY as before
//     (single projection over ALL records, identity-less records included), so
//     every existing single-artifact golden stays byte-identical.

import (
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/canonpath"
)

// traceCausalProjectionMaxArtifactPartitions bounds the number of per-artifact
// projections compiled from one ledger (CMP-1: 分区数上限 4,防病态输入).
const traceCausalProjectionMaxArtifactPartitions = 4

// traceCausalProjectionSupportRefPlaceholderPath is the fixed placeholder the
// trace_query support-ref builders emit when a record carries neither a source
// path nor an artifact id (traceQueryObservationSupportRefs /
// traceQuerySupportRefs fallback). It is NOT an artifact identity.
const traceCausalProjectionSupportRefPlaceholderPath = "runtime_artifact"

// traceCausalProjectionNonArtifactIDTokens is the typed token set of KNOWN
// production constants that flow through SourceRef.ArtifactID without being an
// artifact identity (F4a, adversarial review 2026-07-04): trace_query's
// traceQueryArtifactID only ever returns "attached_trace" (the attach-lane
// sentinel) or "trace_query" (the tool's own name). Treating either as an
// identity would mint a phantom partition next to a real path identity, so the
// ArtifactID lane rejects them and falls through to the locator lane.
var traceCausalProjectionNonArtifactIDTokens = map[string]bool{
	"attached_trace": true,
	"trace_query":    true,
}

// TraceCausalProjectionSet is the partitioned compile result: one projection
// per trace artifact plus the typed leftovers the renderer must caveat.
type TraceCausalProjectionSet struct {
	// Projections holds one ACTIVE projection per kept artifact partition, in
	// first-appearance order of the artifact in the ledger. A single-identity
	// (or identity-less) ledger yields exactly one entry that is byte-identical
	// to the legacy CompileTraceCausalProjection output (plus the Artifact*
	// provenance fields).
	Projections []TraceCausalProjection
	// UnattributedObservationCount counts projection-relevant trace_query
	// records that carried NO typed artifact identity in a MULTI-artifact
	// ledger. They are compiled into no tree (mixing them into an arbitrary
	// artifact would recreate the CMP-1 blended-tree bug) and surface as a
	// renderer caveat only. Always 0 on the single-identity path.
	UnattributedObservationCount int
	// OmittedArtifactLabels names the artifacts dropped by the partition cap,
	// for the renderer caveat. Empty when every artifact was kept.
	OmittedArtifactLabels []string
}

// CompileTraceCausalProjections is the CMP-1 multi-artifact compile entry:
// one projection per trace artifact (see TraceCausalProjectionSet for the
// partition rules). Callers that also need the unattributed/omitted caveat
// counts use CompileTraceCausalProjectionSet.
func CompileTraceCausalProjections(ledger ObservationLedger) []TraceCausalProjection {
	return CompileTraceCausalProjectionSet(ledger).Projections
}

// CompileTraceCausalProjectionSet compiles the ledger into per-artifact
// projections plus the typed caveat counters.
func CompileTraceCausalProjectionSet(ledger ObservationLedger) TraceCausalProjectionSet {
	return TraceCausalProjectionSetFromObservationRecords(ledger.Records)
}

// TraceCausalProjectionSetFromObservationRecords partitions the records by
// typed artifact identity and runs the existing single-artifact compiler per
// partition. See the file header for the full contract.
func TraceCausalProjectionSetFromObservationRecords(records []ObservationRecord) TraceCausalProjectionSet {
	// Pass 1: resolve every record's typed identity WITHOUT bucketing yet, so
	// the F5a spelling-alias merge can canonicalise keys first and records are
	// then bucketed in one pass in original ledger order (a post-hoc partition
	// concatenation would reorder records of merged aliases).
	keys := make([]string, len(records))
	relevant := make([]bool, len(records))
	var order []*traceCausalProjectionPartition
	byKey := map[string]*traceCausalProjectionPartition{}
	unattributed := 0
	for i, record := range records {
		if !traceCausalProjectionTraceQueryRecord(record) {
			continue
		}
		key, label, path := traceCausalProjectionArtifactIdentity(record)
		relevant[i], keys[i] = true, key
		if key == "" {
			unattributed++
			continue
		}
		if _, ok := byKey[key]; !ok {
			p := &traceCausalProjectionPartition{key: key, label: label, path: path}
			byKey[key] = p
			order = append(order, p)
		}
	}
	order = traceCausalProjectionMergeSuffixAliasPartitions(order, byKey)
	for i, record := range records {
		if !relevant[i] || keys[i] == "" {
			continue
		}
		p := byKey[keys[i]]
		p.records = append(p.records, record)
	}

	if len(order) <= 1 {
		// Single-artifact (or identity-less) ledger: compile ALL records exactly
		// like the legacy entry — byte-identical output, no unattributed bucket.
		projection := TraceCausalProjectionFromObservationRecords(records)
		if len(order) == 1 {
			projection.ArtifactPath = order[0].path
			projection.ArtifactLabel = order[0].label
		}
		return TraceCausalProjectionSet{Projections: []TraceCausalProjection{projection}}
	}

	kept := order
	var omitted []string
	if len(kept) > traceCausalProjectionMaxArtifactPartitions {
		// Keep the partitions with the most observations; ties keep the earlier
		// artifact (stable sort over first-appearance order).
		ranked := append([]*traceCausalProjectionPartition(nil), kept...)
		sort.SliceStable(ranked, func(i, j int) bool {
			return len(ranked[i].records) > len(ranked[j].records)
		})
		keep := map[string]bool{}
		for _, p := range ranked[:traceCausalProjectionMaxArtifactPartitions] {
			keep[p.key] = true
		}
		trimmed := make([]*traceCausalProjectionPartition, 0, traceCausalProjectionMaxArtifactPartitions)
		for _, p := range kept {
			if keep[p.key] {
				trimmed = append(trimmed, p)
				continue
			}
			omitted = append(omitted, p.label)
		}
		kept = trimmed
	}
	traceCausalProjectionDisambiguateLabels(kept)

	out := TraceCausalProjectionSet{
		UnattributedObservationCount: unattributed,
		OmittedArtifactLabels:        omitted,
	}
	for _, p := range kept {
		projection := TraceCausalProjectionFromObservationRecords(p.records)
		if !projection.Active() {
			continue
		}
		projection.ArtifactPath = p.path
		projection.ArtifactLabel = p.label
		out.Projections = append(out.Projections, projection)
	}
	return out
}

// traceCausalProjectionPartition is one per-artifact record bucket of the
// partitioned compile (key = typed identity, label/path = display identity).
type traceCausalProjectionPartition struct {
	key     string
	label   string
	path    string
	records []ObservationRecord
}

// traceCausalProjectionMergeSuffixAliasPartitions collapses spelling variants
// of the SAME artifact file into one partition (F5a, adversarial review
// 2026-07-04): two path-lane partitions merge when one path's segment sequence
// is a verbatim suffix of the other's with at least 2 segments — the
// relative-vs-absolute split ("dir/sub/x.trace" vs "/repo/dir/sub/x.trace").
// A 1-segment (bare basename) suffix is exactly the dir1/x-vs-dir2/x collision
// that must NOT merge. The earlier partition keeps its first-appearance slot;
// the LONGER path becomes the surviving identity spelling, and every absorbed
// key aliases to the survivor in byKey so record bucketing lands in one place.
func traceCausalProjectionMergeSuffixAliasPartitions(order []*traceCausalProjectionPartition, byKey map[string]*traceCausalProjectionPartition) []*traceCausalProjectionPartition {
	for {
		merged := false
	scan:
		for i := 0; i < len(order); i++ {
			for j := i + 1; j < len(order); j++ {
				if !traceCausalProjectionSuffixAliasPaths(order[i].path, order[j].path) {
					continue
				}
				survivor, absorbed := order[i], order[j]
				if len(traceCausalProjectionPathSegments(absorbed.path)) >
					len(traceCausalProjectionPathSegments(survivor.path)) {
					survivor.path = absorbed.path
					survivor.label = traceCausalProjectionArtifactBasename(absorbed.path)
				}
				byKey[absorbed.key] = survivor
				order = append(order[:j], order[j+1:]...)
				merged = true
				break scan
			}
		}
		if !merged {
			return order
		}
	}
}

// traceCausalProjectionSuffixAliasPaths reports whether two canonical
// path-lane identities are spelling variants of one file: the shorter segment
// sequence (≥2 segments) is a verbatim suffix of the longer. Empty paths
// (artifact_id-lane partitions) never alias.
func traceCausalProjectionSuffixAliasPaths(a, b string) bool {
	short, long := traceCausalProjectionPathSegments(a), traceCausalProjectionPathSegments(b)
	if len(short) > len(long) {
		short, long = long, short
	}
	if len(short) < 2 || len(short) >= len(long) {
		return false
	}
	for i := range short {
		if short[len(short)-1-i] != long[len(long)-1-i] {
			return false
		}
	}
	return true
}

// traceCausalProjectionPathSegments splits a canonical (slash-normalised)
// path into its non-empty segments.
func traceCausalProjectionPathSegments(canon string) []string {
	canon = strings.Trim(strings.TrimSpace(canon), "/")
	if canon == "" {
		return nil
	}
	parts := strings.Split(canon, "/")
	out := parts[:0]
	for _, part := range parts {
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// traceCausalProjectionDisambiguateLabels upgrades colliding basename labels
// among the KEPT partitions to their last TWO path segments (F5b): dir1/x and
// dir2/x are distinct artifacts whose section titles and compare rows would
// otherwise be indistinguishable. Path-lane labels only; artifact-id labels
// are already the full id and never collide with themselves.
func traceCausalProjectionDisambiguateLabels(kept []*traceCausalProjectionPartition) {
	counts := map[string]int{}
	for _, p := range kept {
		counts[p.label]++
	}
	for _, p := range kept {
		if counts[p.label] < 2 {
			continue
		}
		segments := traceCausalProjectionPathSegments(p.path)
		if len(segments) >= 2 {
			p.label = segments[len(segments)-2] + "/" + segments[len(segments)-1]
		}
	}
}

// traceCausalProjectionArtifactIdentity extracts a record's typed artifact
// identity: partition key, display label (basename), and the canonical path
// (empty when the identity came from a bare artifact id). Precise typed lanes
// only — SourceRef.Path (shared canonicaliser), SourceRef.ArtifactID, then the
// evidence locator's artifact path; never prose. Empty key = no identity.
func traceCausalProjectionArtifactIdentity(record ObservationRecord) (string, string, string) {
	if path := strings.TrimSpace(record.SourceRef.Path); path != "" {
		canon := canonpath.CanonicalRepoRelative(path, "")
		return "path\x00" + canon, traceCausalProjectionArtifactBasename(canon), canon
	}
	// F4a: the known production constants ("attached_trace" / "trace_query")
	// are lane markers, not artifact identities — fall through to the locator.
	if id := strings.TrimSpace(record.SourceRef.ArtifactID); id != "" &&
		!traceCausalProjectionNonArtifactIDTokens[id] {
		return "artifact_id\x00" + id, id, ""
	}
	for _, ref := range record.SupportRefs {
		pathPart, suffix := traceCausalProjectionLocatorSplitLineSuffix(ref)
		if suffix == "" {
			continue
		}
		pathPart = strings.TrimSpace(pathPart)
		if pathPart == "" || pathPart == traceCausalProjectionSupportRefPlaceholderPath {
			continue
		}
		canon := canonpath.CanonicalRepoRelative(pathPart, "")
		return "path\x00" + canon, traceCausalProjectionArtifactBasename(canon), canon
	}
	return "", "", ""
}

// traceCausalProjectionArtifactBasename returns the display basename of a
// canonical (slash-normalised) artifact path.
func traceCausalProjectionArtifactBasename(canon string) string {
	canon = strings.Trim(strings.TrimSpace(canon), "/")
	if canon == "" {
		return ""
	}
	parts := strings.Split(canon, "/")
	return parts[len(parts)-1]
}

// traceCausalProjectionLocatorSplitLineSuffix splits a trailing ":<line>" /
// ":<start>-<end>" locator suffix off an evidence ref (exact digit/dash
// character class — the types-layer isomorph of the tool-side
// runtimeTraceCausalProjectionSplitLineSuffix; scanning from the end keeps
// Windows drive colons intact because their remainder is not numeric).
func traceCausalProjectionLocatorSplitLineSuffix(ref string) (string, string) {
	for i := len(ref) - 1; i >= 0; i-- {
		if ref[i] != ':' {
			continue
		}
		if traceCausalProjectionLocatorLineSuffix(ref[i+1:]) {
			return ref[:i], ref[i:]
		}
	}
	return ref, ""
}

// traceCausalProjectionLocatorLineSuffix validates the exact line-locator
// character class after the colon (F4b, adversarial review 2026-07-04):
// "<line>" or "<start>-<end>" with BOTH sides non-empty pure digits and
// 1-based values. The former loose digits-and-dashes scan accepted "cpu:0-3"
// (a CPU-range shape — lines are 1-based, so a 0 bound is never a line) and
// "v2:1-" (dangling dash), minting phantom "cpu"/"v2" artifact partitions.
func traceCausalProjectionLocatorLineSuffix(s string) bool {
	s = strings.TrimSpace(s)
	start, end := s, ""
	hasDash := false
	if i := strings.IndexByte(s, '-'); i >= 0 {
		start, end, hasDash = s[:i], s[i+1:], true
	}
	if !traceCausalProjectionPositiveLineNumber(start) {
		return false
	}
	return !hasDash || traceCausalProjectionPositiveLineNumber(end)
}

// traceCausalProjectionPositiveLineNumber reports a non-empty pure-digit
// string whose value is non-zero (line numbers are 1-based).
func traceCausalProjectionPositiveLineNumber(s string) bool {
	if s == "" {
		return false
	}
	nonZero := false
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
		if s[i] != '0' {
			nonZero = true
		}
	}
	return nonZero
}
