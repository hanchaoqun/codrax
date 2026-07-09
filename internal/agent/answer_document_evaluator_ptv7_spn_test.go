package agent

// PTV7-SPN F3 pins (§22 dim A P1, docs/design/real_trace_campaign_20260705.md,
// huadong_01 audit 2026-07-07): the system-supplement dump's raw-value
// visibility — ① per-order floored selection (a 112-row state_drilldown flood
// can no longer evict the whole root_cause tertiary lane), ② span_name=
// admitted to the pass-through table, ③ the trace_span family's per-type
// priority pair (span_name= leads).

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func ptv7SpnSupplementContext(records []types.ObservationRecord) *types.AgentContext {
	mu := types.NewMutableState("")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "trace_query", Success: true, Observations: records,
	}}})
	return &types.AgentContext{Mutable: mu}
}

func ptv7SpnDrilldownRecord(i int) types.ObservationRecord {
	return types.ObservationRecord{
		ID:              fmt.Sprintf("trace_query:sup:%d", i),
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: types.ClaimGroundingHard,
		SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "berlin.systrace"},
		Span:            types.ObservationSpan{LineStart: 100 + i*10, LineEnd: 105 + i*10},
		ClaimKey:        fmt.Sprintf("state_drilldown:t%d:S", i),
		Subject:         fmt.Sprintf("t%d-1", i),
		Predicate:       "state_drilldown",
		Object:          "S",
		Value:           "21.000",
		Unit:            "ms",
		RichNotes:       []string{"source=top_sleep"},
		SupportRefs:     []string{fmt.Sprintf("berlin.systrace:%d-%d", 100+i*10, 105+i*10)},
	}
}

// TestPTV7SpnSupplementPerOrderFloor (F3①): the huadong shape — 112
// state_drilldown rows (order 18) plus ONE root_cause tertiary row (order 40)
// — must keep the tertiary row inside the 40-row window (the flat prefix cut
// evicted the entire order-40 lane), and the trim disclosure must say the
// selection is floored (the legacy "仅列前 N 条" wording would lie about a
// non-prefix selection).
func TestPTV7SpnSupplementPerOrderFloor(t *testing.T) {
	records := make([]types.ObservationRecord, 0, 113)
	for i := 0; i < 112; i++ {
		records = append(records, ptv7SpnDrilldownRecord(i))
	}
	tertiary := types.ObservationRecord{
		ID:              "trace_query:q1#root_cause_rank:7",
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRolePrincipalAnswer,
		GroundingPolicy: types.ClaimGroundingHard,
		SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "berlin.systrace"},
		Span:            types.ObservationSpan{LineStart: 1013562, LineEnd: 1016996},
		ClaimKey:        "root_cause_tertiary",
		Subject:         "oney.hmn.berlin-42591",
		Predicate:       "root_cause_tertiary",
		Object:          "trace_span",
		Value:           "9.169",
		Unit:            "ms",
		RichNotes:       []string{"type=trace_span", "span_name=H:ReceiveVsync"},
		SupportRefs:     []string{"berlin.systrace:1013562-1016996"},
		Confidence:      0.75,
	}
	records = append(records, tertiary)
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal, Text: "结论。",
	}}}
	out := renderTraceQueryObservationSupplement(ptv7SpnSupplementContext(records), doc, "zh")
	if !strings.Contains(out, "root_cause_tertiary") {
		t.Fatalf("the order-40 tertiary row must survive the floored selection:\n%s", out)
	}
	// F3②+③ ride the same row: span_name= must reach the rendered notes.
	if !strings.Contains(out, "span_name=H:ReceiveVsync") {
		t.Fatalf("the tertiary row's span_name note must reach the dump:\n%s", out)
	}
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: floored disclosure
	// "仅列 N 条…每序列保底后按序补足" → "列出 N 条:按观测类别配额选取,再按原顺序补足".
	if !strings.Contains(out, "按观测类别配额选取,再按原顺序补足") || strings.Contains(out, "仅列前") {
		t.Fatalf("a floored (non-prefix) selection must disclose itself honestly:\n%s", out)
	}
	if !strings.Contains(out, "(共 113 条,列出 44 条") {
		t.Fatalf("the trim disclosure must state total + listed counts:\n%s", out)
	}
	// Single-bucket overflow keeps the LEGACY prefix wording byte-identically
	// (the quota pass degenerates to the head cut).
	single := make([]types.ObservationRecord, 0, 49)
	for i := 0; i < 49; i++ {
		single = append(single, ptv7SpnDrilldownRecord(i))
	}
	legacy := renderTraceQueryObservationSupplement(ptv7SpnSupplementContext(single), doc, "zh")
	if !strings.Contains(legacy, "仅列前 44 条") {
		t.Fatalf("single-bucket overflow must keep the legacy prefix wording:\n%s", legacy)
	}
}

// TestPTV7SpnSupplementQuotaSelect pins the selection mechanics: per-order
// floor min(N, 4) head rows first, spare seats fill head-first, both slices
// keep the sort order, and prefix reports the degenerate head-cut case.
func TestPTV7SpnSupplementQuotaSelect(t *testing.T) {
	rows := make([]traceQueryObservationSupplementRow, 0, 12)
	for i := 0; i < 10; i++ {
		rows = append(rows, traceQueryObservationSupplementRow{Order: 18, Key: fmt.Sprintf("a%02d", i)})
	}
	rows = append(rows,
		traceQueryObservationSupplementRow{Order: 40, Key: "t1"},
		traceQueryObservationSupplementRow{Order: 40, Key: "t2"},
	)
	kept, omitted, prefix := traceQueryObservationSupplementQuotaSelect(rows, 8)
	if prefix {
		t.Fatalf("a floored multi-bucket selection is not the prefix")
	}
	if len(kept) != 8 || len(omitted) != 4 {
		t.Fatalf("kept/omitted split drifted: %d/%d", len(kept), len(omitted))
	}
	// Floor: 4 seats per bucket; fill: the remaining 2 seats go head-first —
	// order-18 rows a00..a05 plus BOTH order-40 rows survive.
	wantKept := []string{"a00", "a01", "a02", "a03", "a04", "a05", "t1", "t2"}
	for i, want := range wantKept {
		if kept[i].Key != want {
			t.Fatalf("kept[%d] = %q, want %q (full: %+v)", i, kept[i].Key, want, kept)
		}
	}
	// Degenerate single bucket: the selection IS the head cut.
	kept, omitted, prefix = traceQueryObservationSupplementQuotaSelect(rows[:10], 6)
	if !prefix || len(kept) != 6 || kept[5].Key != "a05" || len(omitted) != 4 {
		t.Fatalf("single-bucket selection must degenerate to the prefix cut: prefix=%v kept=%+v", prefix, kept)
	}
}

// TestPTV7SpnSupplementTraceSpanPriority (F3③): a trace_span-family row fronts
// span_name= into the 4-note window even when the producer's note order parks
// it behind four other admitted notes (the pre-fix shape: type/source/
// causality/chain_relevance filled the window first — the name never showed).
func TestPTV7SpnSupplementTraceSpanPriority(t *testing.T) {
	record := types.ObservationRecord{RichNotes: []string{
		"type=trace_span", "source=wakeup_chain", "causality=adjacent_to_wakeup_chain",
		"chain_relevance=adjacent", "span_name=H:ReceiveVsync",
	}}
	got := traceQueryObservationSupplementNotes(record, false)
	want := "notes=span_name=H:ReceiveVsync, type=trace_span, source=wakeup_chain, causality=adjacent_to_wakeup_chain"
	if got != want {
		t.Fatalf("trace_span priority selection:\n got %q\nwant %q", got, want)
	}
	// Non-span rows keep the legacy first-4 selection untouched (control).
	control := types.ObservationRecord{RichNotes: []string{
		"type=io_latency", "source=window_stats", "causality=background",
		"chain_relevance=background", "span_name=phantom",
	}}
	if got := traceQueryObservationSupplementNotes(control, false); strings.HasPrefix(got, "notes=span_name=") {
		t.Fatalf("non-span families must not front span_name: %q", got)
	}
}

// TestSupplementQuotaOrderUniverseBalancesCap is the F3① structural pin
// (§21.1/§22 PTV7-SPN review observation ①): the "每序列保底" disclosure is
// honest only because every reachable order bucket can receive its full floor
// before the cap runs out — reachable buckets × floor ≤ cap. The order-0
// default bucket is structurally excluded at admission (order == 0 rows are
// skipped before sorting), so the reachable universe is exactly the eleven
// values below. Adding a twelfth order value (or raising the floor) breaks
// the balance and MUST come back here to re-adjudicate the quota wording.
// EVOLUTION RECORD (INODE §28.6, 2026-07-09): the top_io_inode statistical
// lane joined as the eleventh bucket (order 65) and the cap was re-balanced
// 40 → 44 = buckets × floor, keeping every lane's floor intact.
func TestSupplementQuotaOrderUniverseBalancesCap(t *testing.T) {
	base := func(claimKey string) types.ObservationRecord {
		return types.ObservationRecord{
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: types.ClaimGroundingHard,
			ClaimKey:        claimKey,
		}
	}
	wantOrders := map[string]int{
		"root_cause_primary:x":      10,
		"critical_blocking:x":       15,
		"state_drilldown:x":         18,
		"wakeup_chain:path":         20,
		"root_cause_secondary:x":    30,
		"root_cause_tertiary:x":     40,
		"wakeup_causal_impact:x":    50,
		"wakeup_causal_aggregate:x": 55,
		"root_evidence:trace_gap":   60,
		"top_io_inode:0xaa":         65,
	}
	distinct := map[int]bool{}
	for claimKey, want := range wantOrders {
		got := traceQueryObservationSupplementOrder(base(claimKey))
		if got != want {
			t.Fatalf("order(%q) = %d, want %d", claimKey, got, want)
		}
		distinct[got] = true
	}
	// The occurrence_windows rich-note escape is the tenth bucket (order 8).
	withNote := base("wakeup_causal_impact:x")
	withNote.RichNotes = []string{"occurrence_windows=1..2"}
	if got := traceQueryObservationSupplementOrder(withNote); got != 8 {
		t.Fatalf("occurrence_windows escape order = %d, want 8", got)
	}
	distinct[8] = true
	// Unmatched claim keys land in the excluded default bucket.
	if got := traceQueryObservationSupplementOrder(base("window_stats:x")); got != 0 {
		t.Fatalf("unmatched claim key must fall to the excluded order-0 bucket, got %d", got)
	}
	if len(distinct) != 11 {
		t.Fatalf("reachable order universe changed: %d buckets, want 11 — re-adjudicate the quota wording", len(distinct))
	}
	if got := len(distinct) * traceQueryObservationSupplementBucketFloor; got > traceQueryObservationSupplementMaxRows {
		t.Fatalf("buckets(%d) x floor(%d) = %d exceeds cap %d — the per-order floor can starve tail buckets and the disclosure wording lies",
			len(distinct), traceQueryObservationSupplementBucketFloor, got, traceQueryObservationSupplementMaxRows)
	}
}
