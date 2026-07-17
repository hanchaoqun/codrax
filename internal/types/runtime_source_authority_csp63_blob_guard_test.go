package types

// CSP #63 root-fix pins (docs/design/real_trace_campaign_20260705.md
// §29.104.20 CSP63-FIX): reads of the engine's own blob-session files must
// not mint current-source authority. The h9 witness
// (real_trace_h9_conversion_single_basis-20260715-201207, log lines
// 2111..2729) paged the engine-produced trace-query-result blob with
// read_file during verification; the resulting read-coverage records carried
// Origin=current_source with the blob path, runtimeSourceAuthorityCurrent-
// SourceRecord had no blob carve-out, CurrentSourceSatisfied flipped on, and
// every runtime-artifact completion bypass was strangled — good trace-only
// data wholesale DOWNGRADED. The guard rides the typed blob-root structural
// lane (IsCodraxBlobSessionPath), case-folded; real repository source reads
// must keep minting byte-identically.

import (
	"reflect"
	"testing"
)

// —— Guard unit pin: the blob-session path family (absolute / repo-relative /
// backslash / case-fold variants) is recognized, and real repository paths —
// relative or absolute — never are (true-positive current-source minting must
// survive, §29.21 direction audit).
func TestCSP63_IsCodraxBlobSessionPath_PathFamily(t *testing.T) {
	positives := []string{
		// h9 witness spellings, verbatim.
		"/Users/han/opt/claude/codrax/.codrax/blob/20260715-201207-000-89609/trace-query-result-25d95bf1.json",
		"/Users/han/opt/claude/codrax/.codrax/blob/20260715-201207-000-89609/attached_trace.txt",
		// Repo-relative canonical form (CWD == repo).
		".codrax/blob/20260715-201207-000-89609/trace-query-result-25d95bf1.json",
		// Raw trace_query offload spelling.
		".codrax/blob/20260703-111820-000-5208/trace_query-89ea5195.txt",
		// Case-fold gap: default macOS filesystems resolve these to the same
		// engine-materialized file.
		"/Users/han/opt/claude/codrax/.CODRAX/Blob/20260715-201207-000-89609/Trace-Query-Result-25D95BF1.JSON",
		".Codrax/BLOB/20260715-201207-000-89609/trace-query-result-25d95bf1.json",
		// Backslash spelling (shared canonicaliser folds separators).
		`C:\work\.codrax\blob\20260715-201207-000-89609\trace-query-result-25d95bf1.json`,
	}
	for _, p := range positives {
		if !IsCodraxBlobSessionPath(p) {
			t.Fatalf("blob-session path %q must be recognized", p)
		}
	}
	negatives := []string{
		"",
		"internal/types/context.go",
		"/Users/han/opt/claude/codrax/internal/tool/emit_investigation_complete.go",
		// Under .codrax but not under the blob root.
		".codrax/memory/codrax-3afc32b5/notes.md",
		// Blob-ish name outside the reserved root.
		"blob/20260715-201207-000-89609/trace-query-result-25d95bf1.json",
		"trace-query-result-25d95bf1.json",
		// Segment boundary: ".codraxx" must not match ".codrax".
		"src/.codraxx/blob/a.json",
		"attached_trace.txt",
	}
	for _, p := range negatives {
		if IsCodraxBlobSessionPath(p) {
			t.Fatalf("non-blob path %q must NOT be recognized", p)
		}
	}
}

// csp63BlobReadCoverageRecord mirrors observationRecordForReadCoverage output
// for a read_file over an engine blob-session file: current-source origin and
// kind, blob path, line-range span (the span makes the pre-fix record count
// on BOTH the record and the exact-support lanes).
func csp63BlobReadCoverageRecord(id, path string) ObservationRecord {
	return ObservationRecord{
		ID:              id,
		Origin:          AnswerEvidenceOriginCurrentSource,
		Producer:        "read_file",
		Role:            AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: ClaimGroundingRepairable,
		SourceRef: ObservationSourceRef{
			Kind: ObservationSourceCurrentSource,
			Path: path,
		},
		Span:            ObservationSpan{LineStart: 1, LineEnd: 200},
		GroundingStatus: GroundingRecovered,
		ClaimKey:        "read_coverage:" + path,
		Subject:         path,
		Predicate:       "read_file_coverage",
	}
}

// —— 件3④ + blob inertness in one identity: a trace-only run's authority
// snapshot is byte-identical with and without engine blob-session reads
// (any spelling, including the case-fold variant). Blob reads carry ZERO
// authority weight — the runtime bypass stays活 (accept direction).
func TestCSP63_BlobSessionReadsAreAuthorityInert_TraceOnlyRun(t *testing.T) {
	input := func(extra ...ObservationRecord) RuntimeSourceAnswerAuthorityInput {
		records := []ObservationRecord{runtimeSourceTraceRecord("trace:root", "trace_query")}
		records = append(records, extra...)
		return RuntimeSourceAnswerAuthorityInput{
			RouteHint: TurnRouteHint{Source: "artifact"},
			Ledger:    ObservationLedger{Records: records},
		}
	}
	base := BuildRuntimeSourceAnswerAuthoritySnapshot(input())
	withBlobReads := BuildRuntimeSourceAnswerAuthoritySnapshot(input(
		csp63BlobReadCoverageRecord("tool:1#read_coverage",
			"/Users/han/opt/claude/codrax/.codrax/blob/20260715-201207-000-89609/trace-query-result-25d95bf1.json"),
		csp63BlobReadCoverageRecord("tool:2#read_coverage",
			".CODRAX/Blob/20260715-201207-000-89609/trace_query-89ea5195.txt"),
	))
	if !reflect.DeepEqual(base, withBlobReads) {
		t.Fatalf("engine blob-session reads must be authority-inert:\nbase=%+v\nwith=%+v", base, withBlobReads)
	}
	if withBlobReads.CurrentSourceRecordCount != 0 || withBlobReads.ExactCurrentSourceSupportCount != 0 ||
		withBlobReads.CurrentSourceSatisfied {
		t.Fatalf("blob reads must not mint current-source proof: %+v", withBlobReads)
	}
	if withBlobReads.KeepsCurrentSourceLaneLoadBearing() {
		t.Fatalf("blob reads must not make the current-source lane load-bearing: %+v", withBlobReads)
	}
	if !withBlobReads.AllowsRuntimeEvidenceWithoutCurrentSource() {
		t.Fatalf("runtime completion bypass must stay alive in a trace-only run with blob reads: %+v", withBlobReads)
	}
}

// —— 件A-① same-family mirror pin (§29.121): the surface-plan fallback lane's
// raw twin (observationLedgerHasCurrentSourceRecord) carries the same blob
// carve as the authority guard — engine blob-session reads do not keep the
// current-source surface pressure alive, artifact-basename records stay
// carved, and repository reads still count.
func TestCSP63_SurfacePlanLedgerMirrorSkipsBlobSessionReads(t *testing.T) {
	blobOnly := ObservationLedger{Records: []ObservationRecord{
		csp63BlobReadCoverageRecord("tool:1#read_coverage",
			"/Users/han/opt/claude/codrax/.codrax/blob/20260715-201207-000-89609/trace-query-result-25d95bf1.json"),
		csp63BlobReadCoverageRecord("tool:2#read_coverage",
			".CODRAX/Blob/20260715-201207-000-89609/trace_query-89ea5195.txt"),
	}}
	if observationLedgerHasCurrentSourceRecord(blobOnly) {
		t.Fatalf("engine blob-session reads must not count as current-source records in the surface-plan mirror")
	}
	artifactBasename := ObservationLedger{Records: []ObservationRecord{
		csp63BlobReadCoverageRecord("tool:1#read_coverage", ".codrax/blob/20260715-201207-000-89609/attached_trace.txt"),
	}}
	if observationLedgerHasCurrentSourceRecord(artifactBasename) {
		t.Fatalf("artifact-basename carve must stay intact in the mirror")
	}
	repoRead := ObservationLedger{Records: []ObservationRecord{
		csp63BlobReadCoverageRecord("tool:1#read_coverage", "internal/agent/gate.go"),
	}}
	if !observationLedgerHasCurrentSourceRecord(repoRead) {
		t.Fatalf("repository source reads must keep counting in the surface-plan mirror")
	}
}

// —— 件3③ negative arm (true-positive kept, §29.21 direction audit): a mixed
// run that reads BOTH repository source and engine blobs keeps minting
// current-source authority from the repository read, and the whole snapshot
// is byte-identical to the same run without the blob reads.
func TestCSP63_RepoSourceReadStillMintsCurrentSource_MixedRunByteIdentical(t *testing.T) {
	repoRead := csp63BlobReadCoverageRecord("tool:1#read_coverage", "internal/agent/gate.go")
	input := func(extra ...ObservationRecord) RuntimeSourceAnswerAuthorityInput {
		records := []ObservationRecord{
			runtimeSourceTraceRecord("trace:root", "trace_query"),
			repoRead,
		}
		records = append(records, extra...)
		return RuntimeSourceAnswerAuthorityInput{
			RouteHint: TurnRouteHint{Source: "artifact"},
			Ledger:    ObservationLedger{Records: records},
		}
	}
	base := BuildRuntimeSourceAnswerAuthoritySnapshot(input())
	if base.CurrentSourceRecordCount != 1 || base.ExactCurrentSourceSupportCount != 1 || !base.CurrentSourceSatisfied {
		t.Fatalf("repository source read must keep minting current-source proof: %+v", base)
	}
	withBlobRead := BuildRuntimeSourceAnswerAuthoritySnapshot(input(
		csp63BlobReadCoverageRecord("tool:2#read_coverage",
			"/Users/han/opt/claude/codrax/.codrax/blob/20260715-201207-000-89609/trace-query-result-25d95bf1.json"),
	))
	if !reflect.DeepEqual(base, withBlobRead) {
		t.Fatalf("mixed-run snapshot must be byte-identical with blob reads added:\nbase=%+v\nwith=%+v", base, withBlobRead)
	}
}
