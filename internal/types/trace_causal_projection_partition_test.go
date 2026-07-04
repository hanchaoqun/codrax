package types

// CMP-A structural pins (docs/design/customer_dead_session_audit_20260703.md
// §7.2, customer artifact custom_compare.txt — two systrace files' observations
// once blended into ONE tree):
//   CMP-1 — the partitioned compile entry groups observations by typed artifact
//           identity, one projection per trace artifact; identity-less records
//           in a multi-artifact ledger reach no tree; ≤1 identity compiles
//           byte-identically to the legacy single-artifact entry;
//   CMP-2 — the anchor window falls back to the producer's typed
//           "selected_window=<start>..<end>" rich note (per artifact) when no
//           frame_target_resolution anchor exists; the frame anchor keeps
//           absolute priority, and (F1, adversarial review 2026-07-04) a
//           record's Span NEVER anchors — the wakeup_causal_aggregate Span is
//           the member-impact envelope, not the selected window; (F1,
//           adversarial re-review 2026-07-04) only the two anchor families —
//           wakeup_causal_aggregate predicate / root_cause_ ClaimKey prefix —
//           may supply the note: the four NEW-8 display-only families
//           (wakeup_causal_impact / critical_blocking / state_churn /
//           state_drilldown) carry the same note purely for window-basis
//           display and never anchor;
//   F4    — the ArtifactID lane rejects the known production constants
//           ("attached_trace"/"trace_query") and the locator line-suffix
//           requires non-empty pure-digit 1-based bounds on both sides;
//   F5    — suffix-alias partition paths (relative vs absolute spellings of
//           one file, ≥2 verbatim tail segments) merge; colliding basename
//           labels upgrade to the last two path segments.

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func partitionTestRecord(id, artifactPath, predicate, claimKey, subject, object, value string, impact float64, lineStart, lineEnd int, span ObservationSpan, notes ...string) ObservationRecord {
	base := []string{fmt.Sprintf("impact_ms=%.3f", impact), fmt.Sprintf("cumulative_impact_ms=%.3f", impact)}
	record := ObservationRecord{
		ID:              id,
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		GroundingPolicy: ClaimGroundingHard,
		Predicate:       predicate,
		ClaimKey:        claimKey,
		Subject:         subject,
		Object:          object,
		Value:           value,
		Unit:            "ms",
		Confidence:      0.8,
		Span:            span,
		RichNotes:       append(base, notes...),
	}
	if artifactPath != "" {
		record.SourceRef = ObservationSourceRef{
			Kind:         ObservationSourceRuntimeArtifact,
			Path:         artifactPath,
			ArtifactKind: "trace",
		}
		record.SupportRefs = []string{fmt.Sprintf("%s:%d-%d", artifactPath, lineStart, lineEnd)}
	}
	if span.LineStart == 0 {
		record.Span.LineStart = lineStart
		record.Span.LineEnd = lineEnd
	}
	return record
}

// custom_compare shape: two trace artifacts, each with its own
// wakeup_causal_aggregate (typed selected_window note + member-envelope span),
// root_cause_primary and critical_blocking rows, plus one identity-less
// relevant record. The aggregate Spans are deliberately DIFFERENT from the
// selected_window notes (F1): the span is the member-impact envelope and must
// never anchor the 关注窗口.
func partitionTestCompareRecords() []ObservationRecord {
	const artifactA = "../customlogs/7.0B30SP22_7315.systrace"
	const artifactB = "../customlogs/6.0B138_3900.sys.systrace"
	return []ObservationRecord{
		partitionTestRecord("a-run", artifactA, "root_cause_primary", "root_cause_primary:running",
			"RSUniRenderThre-1963", "running", "807.276", 807.276, 32642, 199899,
			ObservationSpan{LineStart: 32642, LineEnd: 199899},
			"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain", "dominant_state=running"),
		partitionTestRecord("a-agg", artifactA, "wakeup_causal_aggregate", "wakeup_causal_aggregate:OS_FFRT_2_3-49706",
			"OS_FFRT_2_3-49706", "runnable", "175.232", 175.232, 33000, 34000,
			ObservationSpan{LineStart: 33000, LineEnd: 34000, StartTs: 3680.000, EndTs: 3680.300},
			"chain_relevance=on_chain", "causality=on_wakeup_chain", "dominant_state=runnable",
			"actual_impact=175.232", "actual_total=180.000",
			"selected_window=3679.899000..3681.129000"),
		partitionTestRecord("a-block", artifactA, "critical_blocking", "critical_blocking:d_state_or_io_wait",
			"FaceHandler-9132", "unknown-thread", "87.588", 87.588, 31513, 47636,
			ObservationSpan{LineStart: 31513, LineEnd: 47636},
			"type=d_state_or_io_wait", "chain_relevance=adjacent", "causality=adjacent_to_wakeup_chain"),
		partitionTestRecord("b-run", artifactB, "root_cause_primary", "root_cause_primary:sleep",
			"OS_FFRT_2_6-18695", "sleep_wait", "701.000", 701.0, 31022, 123248,
			ObservationSpan{LineStart: 31022, LineEnd: 123248},
			"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain", "dominant_state=s_sleep"),
		partitionTestRecord("b-agg", artifactB, "wakeup_causal_aggregate", "wakeup_causal_aggregate:HASDK-OnEvent-3-11138",
			"HASDK-OnEvent-3-11138", "runnable", "425.336", 425.336, 32000, 33000,
			ObservationSpan{LineStart: 32000, LineEnd: 33000, StartTs: 8144.000, EndTs: 8144.200},
			"chain_relevance=on_chain", "causality=on_wakeup_chain", "dominant_state=runnable",
			"actual_impact=425.336",
			"selected_window=8143.800000..8144.501000"),
		partitionTestRecord("b-block", artifactB, "critical_blocking", "critical_blocking:d_state_or_io_wait",
			"Loc-LocatorBase-18435", "unknown-thread", "115.889", 115.889, 54844, 66682,
			ObservationSpan{LineStart: 54844, LineEnd: 66682},
			"type=d_state_or_io_wait", "chain_relevance=adjacent", "causality=adjacent_to_wakeup_chain"),
		// Identity-less relevant record: no SourceRef, no SupportRefs — must not
		// blend into either artifact's tree.
		partitionTestRecord("keyless", "", "root_cause_context", "root_cause_context:keyless",
			"keyless-thread-1", "gc_pause", "3.000", 3.0, 10, 20,
			ObservationSpan{LineStart: 10, LineEnd: 20},
			"chain_relevance=background", "causality=background"),
	}
}

func partitionTestProjectionNodes(p TraceCausalProjection) []TraceCausalProjectionNode {
	var out []TraceCausalProjectionNode
	out = append(out, p.PrimaryRootCauses...)
	out = append(out, p.OnChainCauses...)
	out = append(out, p.AdjacentCauses...)
	out = append(out, p.BackgroundCauses...)
	out = append(out, p.SemanticSpans...)
	out = append(out, p.SupportingHops...)
	if p.PrimaryRootCause != nil {
		out = append(out, *p.PrimaryRootCause)
	}
	return out
}

func TestTraceCausalProjectionSetPartitionsByArtifactIdentity(t *testing.T) {
	set := TraceCausalProjectionSetFromObservationRecords(partitionTestCompareRecords())
	if len(set.Projections) != 2 {
		t.Fatalf("two artifacts must compile to two projections, got %d: %+v", len(set.Projections), set)
	}
	a, b := set.Projections[0], set.Projections[1]
	if a.ArtifactLabel != "7.0B30SP22_7315.systrace" || b.ArtifactLabel != "6.0B138_3900.sys.systrace" {
		t.Fatalf("artifact labels must follow first-appearance order: %q / %q", a.ArtifactLabel, b.ArtifactLabel)
	}
	if a.ArtifactPath != "../customlogs/7.0B30SP22_7315.systrace" {
		t.Fatalf("artifact path must be the canonicalised SourceRef.Path: %q", a.ArtifactPath)
	}
	// Structural invariant: every node inside ONE projection carries the SAME
	// artifact identity on its evidence locator.
	for _, tc := range []struct {
		projection TraceCausalProjection
		wantBase   string
		otherBase  string
	}{
		{a, "7.0B30SP22_7315.systrace", "6.0B138_3900.sys.systrace"},
		{b, "6.0B138_3900.sys.systrace", "7.0B30SP22_7315.systrace"},
	} {
		nodes := partitionTestProjectionNodes(tc.projection)
		if len(nodes) == 0 {
			t.Fatalf("projection %s compiled empty", tc.wantBase)
		}
		for _, node := range nodes {
			if len(node.SupportRefs) == 0 {
				t.Fatalf("projection %s node %q lost its evidence locator", tc.wantBase, node.EvidenceID)
			}
			for _, ref := range node.SupportRefs {
				if !strings.Contains(ref, tc.wantBase) || strings.Contains(ref, tc.otherBase) {
					t.Fatalf("projection %s node %q carries a foreign locator %q", tc.wantBase, node.EvidenceID, ref)
				}
			}
			if node.Subject == "keyless-thread-1" {
				t.Fatalf("identity-less record must not compile into projection %s", tc.wantBase)
			}
		}
	}
	// CMP-2/F1: each artifact anchors its OWN selected window from the typed
	// selected_window note (no frame anchor in this ledger) — NOT from the
	// aggregate's member-envelope Span (3680.000..3680.300 / 8144.000..8144.200).
	if a.WindowStartTs != 3679.899 || a.WindowEndTs != 3681.129 {
		t.Fatalf("artifact A must anchor its own selected window: %v..%v", a.WindowStartTs, a.WindowEndTs)
	}
	if b.WindowStartTs != 8143.800 || b.WindowEndTs != 8144.501 {
		t.Fatalf("artifact B must anchor its own selected window: %v..%v", b.WindowStartTs, b.WindowEndTs)
	}
	if set.UnattributedObservationCount != 1 {
		t.Fatalf("the identity-less record must land in the unattributed bucket: %d", set.UnattributedObservationCount)
	}
	if len(set.OmittedArtifactLabels) != 0 {
		t.Fatalf("no artifact should be cap-omitted here: %+v", set.OmittedArtifactLabels)
	}
}

func TestTraceCausalProjectionSetSupportRefIdentityLane(t *testing.T) {
	// Records without SourceRef but with a path-carrying evidence locator
	// partition through the locator lane (typed character-class split).
	mk := func(id, ref, subject string) ObservationRecord {
		r := partitionTestRecord(id, "", "root_cause_primary", "root_cause_primary:"+id,
			subject, "running", "5.000", 5.0, 1, 2, ObservationSpan{LineStart: 1, LineEnd: 2},
			"rank=1", "tier=primary")
		r.SupportRefs = []string{ref}
		return r
	}
	set := TraceCausalProjectionSetFromObservationRecords([]ObservationRecord{
		mk("x1", "x.systrace:10-20", "worker-1"),
		mk("y1", "y.systrace:30-40", "worker-2"),
	})
	if len(set.Projections) != 2 ||
		set.Projections[0].ArtifactLabel != "x.systrace" ||
		set.Projections[1].ArtifactLabel != "y.systrace" {
		t.Fatalf("locator-lane identity must partition: %+v", set.Projections)
	}
	// The "runtime_artifact" placeholder path is NOT an identity: it must not
	// mint a second partition. One real identity remains → the ≤1-identity
	// legacy lane compiles ALL records into one projection (byte-identity
	// mandate), labeled with the sole real artifact.
	placeholder := TraceCausalProjectionSetFromObservationRecords([]ObservationRecord{
		mk("p1", "runtime_artifact:10-20", "worker-1"),
		mk("y1", "y.systrace:30-40", "worker-2"),
	})
	if len(placeholder.Projections) != 1 || placeholder.Projections[0].ArtifactLabel != "y.systrace" {
		t.Fatalf("placeholder locator must not become an artifact partition: %+v", placeholder.Projections)
	}
	if len(placeholder.Projections[0].PrimaryRootCauses) != 2 {
		t.Fatalf("the ≤1-identity lane must keep legacy all-records compile: %+v", placeholder.Projections[0].PrimaryRootCauses)
	}
	if placeholder.UnattributedObservationCount != 0 {
		t.Fatalf("the ≤1-identity lane must not populate the unattributed counter: %d", placeholder.UnattributedObservationCount)
	}
}

func TestTraceCausalProjectionSetSingleArtifactByteIdentical(t *testing.T) {
	cases := map[string][]ObservationRecord{
		"no identity at all": {
			traceProjectionTestRoot("root-app", "app-100", "compute_supply", "0.020", 0.02, 0.90, 1),
			traceProjectionTestRoot("root-threadpool", "threadpool-400", "io_wait", "11.000", 11.0, 0.86, 4),
			{
				ID: "path", Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
				GroundingPolicy: ClaimGroundingHard, Predicate: "wakeup_chain", ClaimKey: "wakeup_chain:path",
				Object: "threadpool-400 -> app-100",
			},
		},
		"one artifact plus identity-less records": func() []ObservationRecord {
			records := []ObservationRecord{
				partitionTestRecord("a-run", "berlin.systrace", "root_cause_primary", "root_cause_primary:running",
					"worker-2", "running", "7.000", 7.0, 30, 40, ObservationSpan{LineStart: 30, LineEnd: 40},
					"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain"),
				{
					ID: "path", Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
					GroundingPolicy: ClaimGroundingHard, Predicate: "wakeup_chain", ClaimKey: "wakeup_chain:path",
					Object: "worker-2 -> app-1",
				},
			}
			return records
		}(),
	}
	for name, records := range cases {
		legacy := CompileTraceCausalProjection(ObservationLedger{Records: records})
		set := TraceCausalProjectionSetFromObservationRecords(records)
		if len(set.Projections) != 1 {
			t.Fatalf("%s: single-identity ledger must yield exactly one projection, got %d", name, len(set.Projections))
		}
		if set.UnattributedObservationCount != 0 || len(set.OmittedArtifactLabels) != 0 {
			t.Fatalf("%s: single-identity ledger must not populate caveat counters: %+v", name, set)
		}
		got := set.Projections[0]
		got.ArtifactPath, got.ArtifactLabel = "", ""
		if !reflect.DeepEqual(legacy, got) {
			t.Fatalf("%s: partitioned single-artifact compile must be identical to the legacy entry:\nlegacy: %+v\ngot:    %+v", name, legacy, got)
		}
	}
}

func TestTraceCausalProjectionSetCapsPartitions(t *testing.T) {
	var records []ObservationRecord
	// Artifacts a1..a4 get two records each; a5 gets one — a5 is omitted.
	for i := 1; i <= 5; i++ {
		artifact := fmt.Sprintf("a%d.systrace", i)
		count := 2
		if i == 5 {
			count = 1
		}
		for j := 0; j < count; j++ {
			records = append(records, partitionTestRecord(
				fmt.Sprintf("r-%d-%d", i, j), artifact, "root_cause_primary",
				fmt.Sprintf("root_cause_primary:%d-%d", i, j),
				fmt.Sprintf("worker-%d", i), "running", "5.000", 5.0, 10*j+1, 10*j+5,
				ObservationSpan{LineStart: 10*j + 1, LineEnd: 10*j + 5},
				"rank=1", "tier=primary"))
		}
	}
	set := TraceCausalProjectionSetFromObservationRecords(records)
	if len(set.Projections) != 4 {
		t.Fatalf("partition cap must keep 4 projections, got %d", len(set.Projections))
	}
	for i, p := range set.Projections {
		if want := fmt.Sprintf("a%d.systrace", i+1); p.ArtifactLabel != want {
			t.Fatalf("kept partitions must preserve first-appearance order: got %q want %q", p.ArtifactLabel, want)
		}
	}
	if len(set.OmittedArtifactLabels) != 1 || set.OmittedArtifactLabels[0] != "a5.systrace" {
		t.Fatalf("the lowest-observation artifact must be omitted with its label: %+v", set.OmittedArtifactLabels)
	}
}

// --- CMP-2/F1: selected-window anchor fallback ---------------------------------

func TestTraceCausalProjectionAnchorSelectedWindowFallback(t *testing.T) {
	aggregate := func(notes []string, span ObservationSpan) ObservationRecord {
		return partitionTestRecord("agg", "", "wakeup_causal_aggregate", "wakeup_causal_aggregate:t",
			"OS_FFRT_2_3-49706", "runnable", "175.232", 175.232, 100, 200, span, notes...)
	}
	// (a) the typed selected_window note anchors — and it wins over the
	// record's Span, which is the member-impact envelope (deliberately set to
	// a conflicting 300ms range here).
	got := TraceCausalProjectionFromObservationRecords([]ObservationRecord{
		aggregate([]string{"actual_impact=175.232", "selected_window=3679.899000..3681.129000"},
			ObservationSpan{LineStart: 100, LineEnd: 200, StartTs: 3680.000, EndTs: 3680.300}),
	})
	if got.WindowStartTs != 3679.899 || got.WindowEndTs != 3681.129 {
		t.Fatalf("selected_window note must anchor the window: %v..%v", got.WindowStartTs, got.WindowEndTs)
	}
	// (b) F1 negative pin: a Span-only record — even with the OLD actual_*
	// basis marker — must NOT anchor. The envelope lane is deleted; reviving
	// it would fabricate a 关注窗口 from member FirstTs/LastTs again.
	got = TraceCausalProjectionFromObservationRecords([]ObservationRecord{
		aggregate([]string{"actual_impact=175.232", "actual_total=180.000"},
			ObservationSpan{LineStart: 100, LineEnd: 200, StartTs: 3679.899, EndTs: 3681.129}),
	})
	if got.WindowStartTs != 0 || got.WindowEndTs != 0 {
		t.Fatalf("Span-only records must never anchor (envelope semantics): %v..%v", got.WindowStartTs, got.WindowEndTs)
	}
	// (b') same for the `window=` rich note without selected_window: not an
	// anchor carrier on the fallback lane.
	got = TraceCausalProjectionFromObservationRecords([]ObservationRecord{
		partitionTestRecord("root", "", "root_cause_primary", "root_cause_primary:r",
			"worker-2", "running", "7.000", 7.0, 30, 40, ObservationSpan{LineStart: 30, LineEnd: 40},
			"rank=1", "tier=primary", "actual_impact_ms=8.000", "window=8143.800000..8144.501000"),
	})
	if got.WindowStartTs != 0 || got.WindowEndTs != 0 {
		t.Fatalf("window= note must not anchor the fallback lane: %v..%v", got.WindowStartTs, got.WindowEndTs)
	}
	// (c) root_cause_primary with the note anchors (the production lane that
	// was previously dead: no Span ts, no window= note).
	got = TraceCausalProjectionFromObservationRecords([]ObservationRecord{
		partitionTestRecord("root", "", "root_cause_primary", "root_cause_primary:r",
			"worker-2", "running", "7.000", 7.0, 30, 40, ObservationSpan{LineStart: 30, LineEnd: 40},
			"rank=1", "tier=primary", "selected_window=8143.800000..8144.501000"),
	})
	if got.WindowStartTs != 8143.8 || got.WindowEndTs != 8144.501 {
		t.Fatalf("root_cause_primary selected_window note must anchor: %v..%v", got.WindowStartTs, got.WindowEndTs)
	}
	// (d) malformed notes never anchor: exact prefix + two strict floats +
	// end > start > 0 only.
	for _, malformed := range []string{
		"selected_window=abc..1.200000",
		"selected_window=1.200000",
		"selected_window=0.000000..1.200000",
		"selected_window=2.000000..1.000000",
		"selected_window=1.000000ms..2.000000",
		"my_selected_window=1.000000..2.000000",
	} {
		got = TraceCausalProjectionFromObservationRecords([]ObservationRecord{
			aggregate([]string{malformed}, ObservationSpan{LineStart: 100, LineEnd: 200}),
		})
		if got.WindowStartTs != 0 || got.WindowEndTs != 0 {
			t.Fatalf("malformed note %q must not anchor: %v..%v", malformed, got.WindowStartTs, got.WindowEndTs)
		}
	}
	// (e) the frame anchor keeps absolute priority over the note fallback.
	got = TraceCausalProjectionFromObservationRecords([]ObservationRecord{
		{
			ID: "anchor", Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: ClaimGroundingHard, Predicate: "frame_target_resolution", ClaimKey: "frame_target_resolution:f",
			Subject: "app-1", Object: "frame",
			Span:      ObservationSpan{StartTs: 1.100, EndTs: 1.200},
			RichNotes: []string{"window_source=query_window"},
		},
		aggregate([]string{"selected_window=3679.899000..3681.129000"},
			ObservationSpan{LineStart: 100, LineEnd: 200}),
	})
	if got.WindowStartTs != 1.100 || got.WindowEndTs != 1.200 {
		t.Fatalf("frame anchor must keep priority over the selected-window fallback: %v..%v", got.WindowStartTs, got.WindowEndTs)
	}
	// (f) several qualifying records: the LAST one wins (existing multi-anchor
	// semantics).
	got = TraceCausalProjectionFromObservationRecords([]ObservationRecord{
		aggregate([]string{"selected_window=3679.899000..3681.129000"},
			ObservationSpan{LineStart: 100, LineEnd: 200}),
		partitionTestRecord("agg2", "", "wakeup_causal_aggregate", "wakeup_causal_aggregate:u",
			"other-1", "runnable", "10.000", 10.0, 300, 400,
			ObservationSpan{LineStart: 300, LineEnd: 400},
			"selected_window=8143.800000..8144.501000"),
	})
	if got.WindowStartTs != 8143.800 || got.WindowEndTs != 8144.501 {
		t.Fatalf("the last qualifying record must win: %v..%v", got.WindowStartTs, got.WindowEndTs)
	}
}

// --- F1 (adversarial re-review 2026-07-04): anchor-family whitelist -------------

// TestTraceCausalProjectionAnchorIgnoresDisplayOnlySelectedWindowNotes pins the
// F1 correction of the NEW-8 interaction: NEW-8 extended the typed
// selected_window note to four DISPLAY-ONLY families, and the last-wins anchor
// loop silently became "whichever family published last wins" — a mixed-window
// session re-anchored the 关注窗口 onto a later 100ms micro-probe window. Only
// wakeup_causal_aggregate (predicate) and the root_cause_rank family (ClaimKey
// prefix root_cause_) may anchor.
func TestTraceCausalProjectionAnchorIgnoresDisplayOnlySelectedWindowNotes(t *testing.T) {
	const mainWindow = "selected_window=3679.899000..3681.129000"
	const microProbe = "selected_window=100.000000..100.100000"
	churn := func(id string) ObservationRecord {
		return partitionTestRecord(id, "", "state_churn", "state_churn:runnable",
			"worker-2", "runnable", "5.000", 5.0, 50, 60, ObservationSpan{LineStart: 50, LineEnd: 60},
			"dominant_state=runnable", microProbe)
	}
	// (a) review probe shape: main root_cause_rank window + a LATER micro-probe
	// window_stats record (state_churn) — the anchor keeps the main window
	// instead of last-wins flipping onto the 100ms micro-probe.
	got := TraceCausalProjectionFromObservationRecords([]ObservationRecord{
		partitionTestRecord("rank", "", "root_cause_primary", "root_cause_primary:r",
			"worker-2", "running", "7.000", 7.0, 30, 40, ObservationSpan{LineStart: 30, LineEnd: 40},
			"rank=1", "tier=primary", mainWindow),
		churn("churn"),
	})
	if got.WindowStartTs != 3679.899 || got.WindowEndTs != 3681.129 {
		t.Fatalf("a later display-only note must not re-anchor the window: %v..%v", got.WindowStartTs, got.WindowEndTs)
	}
	// (b) the four display-only families ALONE (no anchor-family record): no
	// anchor at all — the renderer falls back to 起止未采集 / relative bars
	// rather than adopting a display carrier.
	got = TraceCausalProjectionFromObservationRecords([]ObservationRecord{
		churn("churn"),
		partitionTestRecord("drill", "", "state_drilldown", "state_drilldown:worker-2:s_sleep",
			"worker-2", "s_sleep", "6.000", 6.0, 61, 70, ObservationSpan{LineStart: 61, LineEnd: 70},
			microProbe),
		partitionTestRecord("impact", "", "wakeup_causal_impact", "wakeup_causal_impact:worker-3",
			"worker-3", "runnable", "4.000", 4.0, 71, 80, ObservationSpan{LineStart: 71, LineEnd: 80},
			"causality=on_wakeup_chain", microProbe),
		partitionTestRecord("block", "", "critical_blocking", "critical_blocking:futex",
			"worker-2", "worker-3", "3.000", 3.0, 81, 90, ObservationSpan{LineStart: 81, LineEnd: 90},
			"type=futex", microProbe),
	})
	if !got.Active() {
		t.Fatalf("fixture must stay active (hop rows) so the anchor lane is really exercised")
	}
	if got.WindowStartTs != 0 || got.WindowEndTs != 0 {
		t.Fatalf("display-only families alone must never anchor: %v..%v", got.WindowStartTs, got.WindowEndTs)
	}
}

// --- NEW-9: capacity-truncation note lift ---------------------------------------

// TestTraceCausalProjectionLiftsCapacityTruncatedNote pins the NEW-9 compile
// lift on both shapes: the producer's exact typed capacity_truncated=true rich
// note (stamped by trace_query when Result.Compactions is non-empty) sets
// TraceCausalProjection.CapacityTruncated; without the note the flag stays
// false. Display-only downstream — the evidence-index header discloses it.
func TestTraceCausalProjectionLiftsCapacityTruncatedNote(t *testing.T) {
	records := func(notes ...string) []ObservationRecord {
		return []ObservationRecord{partitionTestRecord("rank", "", "root_cause_primary", "root_cause_primary:r",
			"worker-2", "running", "7.000", 7.0, 30, 40, ObservationSpan{LineStart: 30, LineEnd: 40},
			append([]string{"rank=1", "tier=primary"}, notes...)...)}
	}
	if got := TraceCausalProjectionFromObservationRecords(records()); got.CapacityTruncated {
		t.Fatalf("no producer note must leave CapacityTruncated false")
	}
	if got := TraceCausalProjectionFromObservationRecords(records("capacity_truncated=true")); !got.CapacityTruncated {
		t.Fatalf("the typed producer note must lift into CapacityTruncated")
	}
	// Exact typed match only — a non-true value never lifts.
	if got := TraceCausalProjectionFromObservationRecords(records("capacity_truncated=false")); got.CapacityTruncated {
		t.Fatalf("capacity_truncated=false must not lift")
	}
}

// --- F4: partition identity-lane hardening --------------------------------------

func TestTraceCausalProjectionArtifactIDLaneRejectsProductionConstants(t *testing.T) {
	// trace_query only ever publishes ArtifactID="attached_trace" or
	// ="trace_query" — lane markers, not artifact identities. A record carrying
	// one of them (and no path anywhere) must stay identity-less instead of
	// minting a phantom partition next to the real path identity.
	for _, token := range []string{"attached_trace", "trace_query"} {
		withPath := partitionTestRecord("real", "x.systrace", "root_cause_primary", "root_cause_primary:real",
			"worker-1", "running", "5.000", 5.0, 1, 2, ObservationSpan{LineStart: 1, LineEnd: 2},
			"rank=1", "tier=primary")
		tokenOnly := partitionTestRecord("token", "", "root_cause_primary", "root_cause_primary:token",
			"worker-2", "running", "6.000", 6.0, 3, 4, ObservationSpan{LineStart: 3, LineEnd: 4},
			"rank=1", "tier=primary")
		tokenOnly.SourceRef = ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact, ArtifactID: token, ArtifactKind: "trace"}
		set := TraceCausalProjectionSetFromObservationRecords([]ObservationRecord{withPath, tokenOnly})
		if len(set.Projections) != 1 || set.Projections[0].ArtifactLabel != "x.systrace" {
			t.Fatalf("ArtifactID=%q must not mint a partition: %+v", token, set.Projections)
		}
		// ≤1-identity lane: legacy all-records compile, both primaries kept.
		if len(set.Projections[0].PrimaryRootCauses) != 2 {
			t.Fatalf("ArtifactID=%q ledger must stay on the ≤1-identity lane: %+v", token, set.Projections[0].PrimaryRootCauses)
		}
	}
	// A real (non-constant) artifact id still partitions through the id lane.
	real := partitionTestRecord("real", "x.systrace", "root_cause_primary", "root_cause_primary:real",
		"worker-1", "running", "5.000", 5.0, 1, 2, ObservationSpan{LineStart: 1, LineEnd: 2},
		"rank=1", "tier=primary")
	idOnly := partitionTestRecord("id", "", "root_cause_primary", "root_cause_primary:id",
		"worker-2", "running", "6.000", 6.0, 3, 4, ObservationSpan{LineStart: 3, LineEnd: 4},
		"rank=1", "tier=primary")
	idOnly.SourceRef = ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact, ArtifactID: "bundle-7f3a", ArtifactKind: "trace"}
	set := TraceCausalProjectionSetFromObservationRecords([]ObservationRecord{real, idOnly})
	if len(set.Projections) != 2 || set.Projections[1].ArtifactLabel != "bundle-7f3a" {
		t.Fatalf("a real artifact id must keep partitioning: %+v", set.Projections)
	}
}

func TestTraceCausalProjectionLocatorLineSuffixTightened(t *testing.T) {
	mk := func(id, ref, subject string) ObservationRecord {
		r := partitionTestRecord(id, "", "root_cause_primary", "root_cause_primary:"+id,
			subject, "running", "5.000", 5.0, 1, 2, ObservationSpan{LineStart: 1, LineEnd: 2},
			"rank=1", "tier=primary")
		r.SupportRefs = []string{ref}
		return r
	}
	// F4b negatives: a CPU-range shape ("cpu:0-3" — lines are 1-based, 0 is
	// never a line) and a dangling dash ("v2:1-") are NOT line locators; the
	// records stay identity-less and no "cpu"/"v2" partition is minted.
	for _, ref := range []string{"cpu:0-3", "v2:1-", "x:0", "x:-3", "x:3-", "x:1-2-3"} {
		set := TraceCausalProjectionSetFromObservationRecords([]ObservationRecord{
			mk("bad", ref, "worker-1"),
			mk("real", "y.systrace:30-40", "worker-2"),
		})
		if len(set.Projections) != 1 || set.Projections[0].ArtifactLabel != "y.systrace" {
			t.Fatalf("locator %q must not mint a partition: %+v", ref, set.Projections)
		}
	}
	// Positives: single line and 1-based ranges keep working.
	set := TraceCausalProjectionSetFromObservationRecords([]ObservationRecord{
		mk("x1", "x.systrace:7", "worker-1"),
		mk("y1", "y.systrace:30-40", "worker-2"),
	})
	if len(set.Projections) != 2 ||
		set.Projections[0].ArtifactLabel != "x.systrace" ||
		set.Projections[1].ArtifactLabel != "y.systrace" {
		t.Fatalf("valid line locators must keep partitioning: %+v", set.Projections)
	}
}

// --- F5: spelling-alias merge + basename disambiguation -------------------------

func TestTraceCausalProjectionSuffixAliasPartitionsMerge(t *testing.T) {
	// Relative and absolute spellings of the SAME file (≥2 verbatim tail
	// segments) merge into ONE partition → the ≤1-identity legacy lane.
	relative := partitionTestRecord("rel", "dir/sub/x.trace", "root_cause_primary", "root_cause_primary:rel",
		"worker-1", "running", "5.000", 5.0, 1, 2, ObservationSpan{LineStart: 1, LineEnd: 2},
		"rank=1", "tier=primary")
	absolute := partitionTestRecord("abs", "/repo/dir/sub/x.trace", "root_cause_primary", "root_cause_primary:abs",
		"worker-2", "running", "6.000", 6.0, 3, 4, ObservationSpan{LineStart: 3, LineEnd: 4},
		"rank=1", "tier=primary")
	set := TraceCausalProjectionSetFromObservationRecords([]ObservationRecord{relative, absolute})
	if len(set.Projections) != 1 {
		t.Fatalf("relative/absolute spellings of one file must merge: %+v", set.Projections)
	}
	if got := set.Projections[0]; got.ArtifactLabel != "x.trace" ||
		got.ArtifactPath != "/repo/dir/sub/x.trace" {
		t.Fatalf("the merged partition must keep the longer spelling: %q %q", got.ArtifactLabel, got.ArtifactPath)
	}
	if len(set.Projections[0].PrimaryRootCauses) != 2 {
		t.Fatalf("both spellings' records must compile into the one projection: %+v", set.Projections[0].PrimaryRootCauses)
	}
	if set.UnattributedObservationCount != 0 {
		t.Fatalf("merged single-identity ledger must not count unattributed: %d", set.UnattributedObservationCount)
	}
}

func TestTraceCausalProjectionBasenameCollisionKeepsPartitionsAndDisambiguates(t *testing.T) {
	// dir1/x vs dir2/x share only ONE tail segment — different files, no merge
	// — and their colliding basename labels upgrade to the last two segments.
	one := partitionTestRecord("one", "dir1/x.trace", "root_cause_primary", "root_cause_primary:one",
		"worker-1", "running", "5.000", 5.0, 1, 2, ObservationSpan{LineStart: 1, LineEnd: 2},
		"rank=1", "tier=primary")
	two := partitionTestRecord("two", "dir2/x.trace", "root_cause_primary", "root_cause_primary:two",
		"worker-2", "running", "6.000", 6.0, 3, 4, ObservationSpan{LineStart: 3, LineEnd: 4},
		"rank=1", "tier=primary")
	set := TraceCausalProjectionSetFromObservationRecords([]ObservationRecord{one, two})
	if len(set.Projections) != 2 {
		t.Fatalf("basename twins in different directories are different artifacts: %+v", set.Projections)
	}
	if set.Projections[0].ArtifactLabel != "dir1/x.trace" || set.Projections[1].ArtifactLabel != "dir2/x.trace" {
		t.Fatalf("colliding basenames must upgrade to last-two-segment labels: %q / %q",
			set.Projections[0].ArtifactLabel, set.Projections[1].ArtifactLabel)
	}
	// Non-colliding basenames keep the plain basename label (no churn).
	other := partitionTestRecord("two", "dir2/y.trace", "root_cause_primary", "root_cause_primary:two",
		"worker-2", "running", "6.000", 6.0, 3, 4, ObservationSpan{LineStart: 3, LineEnd: 4},
		"rank=1", "tier=primary")
	set = TraceCausalProjectionSetFromObservationRecords([]ObservationRecord{one, other})
	if set.Projections[0].ArtifactLabel != "x.trace" || set.Projections[1].ArtifactLabel != "y.trace" {
		t.Fatalf("distinct basenames must keep plain labels: %q / %q",
			set.Projections[0].ArtifactLabel, set.Projections[1].ArtifactLabel)
	}
}
