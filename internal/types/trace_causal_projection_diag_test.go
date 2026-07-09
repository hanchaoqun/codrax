package types

// trace_causal_projection_diag_test.go — DIAG batch pins (§28.11-3,
// real_trace_campaign_20260705.md, 2026-07-09).
//
// A1 same-value fold-member disclosure (G12 / huadong_79 E23):
//   - E23 replica: a cross-thread take-MAX fold whose two members tie the
//     published MAX to the µs discloses BOTH members with their own line
//     intervals; distinct-value members disclose nothing.
//   - ZERO WEIGHT (自设最强突变形 "披露臂偷偷改折叠值"): with the disclosure
//     active, every published fold value is asserted equal to the
//     no-disclosure expectation — a mutation that lets the disclosure arm
//     touch ImpactMS/CumulativeImpactMS/MergedMinMS/MergedMaxMS/MergedCount
//     turns these pins red. (Mutation run 2026-07-09: adding +0.001 to
//     fold.ImpactMS inside the disclosure arm reddened
//     TestDiagSameValueFoldDisclosureE23Replica as designed.)
//   - strict tie band: |v−max| >= TraceCausalProjectionSameValueTieMS is NOT
//     a tie (user ruling: 值差 < 0.0005ms 视同值).
//   - wire re-materialization: folded_rows records parse the
//     same_value_members note; malformed / sub-2 rosters yield nil.
//
// A2 actual two-caliber disclosure (D-10 / opendir_79 E5):
//   - compile parse of actual_total_ms + actual_caliber_note into the typed
//     node fields, both directions (note present / absent).

import (
	"reflect"
	"strings"
	"testing"
)

func diagOverflowMember(subject string, ms float64, lineStart, lineEnd int) TraceCausalProjectionNode {
	return TraceCausalProjectionNode{
		Role:               TraceCausalRoleRootCauseContext,
		Predicate:          "critical_blocking",
		ChainRelevance:     "on_chain",
		Subject:            subject,
		ImpactMS:           ms,
		CumulativeImpactMS: ms,
		LineStart:          lineStart,
		LineEnd:            lineEnd,
		EvidenceID:         subject + "-ev",
		Confidence:         0.8,
	}
}

// TestDiagSameValueFoldDisclosureE23Replica is the E23 replica pin: two
// cross-thread members, both 14.272ms to the µs, folded by the shared
// overflow-fold constructor → the fold carries both (subject, line-range)
// witnesses AND every published value is exactly the pre-disclosure take-MAX
// shape (zero weight).
func TestDiagSameValueFoldDisclosureE23Replica(t *testing.T) {
	overflow := []TraceCausalProjectionNode{
		diagOverflowMember("hmfs_discard-1234", 14.272, 5001, 5040),
		diagOverflowMember("com.example.app-42", 14.272, 6100, 6180),
	}
	fold := traceCausalProjectionOverflowFoldRow(overflow)
	// Zero-weight assertions FIRST: the disclosure must not have moved a
	// single published value off the take-MAX shape.
	if fold.ImpactMS != 14.272 || fold.CumulativeImpactMS != 14.272 {
		t.Fatalf("disclosure arm changed the fold value: impact=%v cumulative=%v", fold.ImpactMS, fold.CumulativeImpactMS)
	}
	if fold.MergedMinMS != 14.272 || fold.MergedMaxMS != 14.272 || fold.MergedCount != 2 {
		t.Fatalf("disclosure arm changed the fold accounting: min=%v max=%v count=%d", fold.MergedMinMS, fold.MergedMaxMS, fold.MergedCount)
	}
	want := []TraceCausalProjectionSameValueMember{
		{Subject: "hmfs_discard-1234", LineStart: 5001, LineEnd: 5040},
		{Subject: "com.example.app-42", LineStart: 6100, LineEnd: 6180},
	}
	if !reflect.DeepEqual(fold.SameValueMembers, want) {
		t.Fatalf("E23 replica must disclose both µs-tie members with their line intervals:\n got %+v\nwant %+v", fold.SameValueMembers, want)
	}
}

// TestDiagSameValueFoldDistinctValuesNoDisclosure: distinct member values →
// NO disclosure, and the fold row is byte-identical to the pre-DIAG shape
// (the only field the batch may ever set stays nil).
func TestDiagSameValueFoldDistinctValuesNoDisclosure(t *testing.T) {
	overflow := []TraceCausalProjectionNode{
		diagOverflowMember("hmfs_discard-1234", 14.272, 5001, 5040),
		diagOverflowMember("com.example.app-42", 13.900, 6100, 6180),
	}
	fold := traceCausalProjectionOverflowFoldRow(overflow)
	if fold.SameValueMembers != nil {
		t.Fatalf("distinct values must not disclose: %+v", fold.SameValueMembers)
	}
	if fold.ImpactMS != 14.272 || fold.MergedMinMS != 13.900 || fold.MergedMaxMS != 14.272 {
		t.Fatalf("take-MAX accounting drifted: %+v", fold)
	}
}

// TestDiagSameValueTieBandStrict pins the user-ruled strict band: a 0.0005ms
// difference is NOT a tie; a 0.0004ms difference is.
func TestDiagSameValueTieBandStrict(t *testing.T) {
	base := diagOverflowMember("a-1", 14.2720, 100, 110)
	atBand := diagOverflowMember("b-2", 14.2715, 200, 210)  // exactly 0.0005 apart
	inBand := diagOverflowMember("c-3", 14.27164, 300, 310) // 0.00036 apart
	if fold := traceCausalProjectionOverflowFoldRow([]TraceCausalProjectionNode{base, atBand}); fold.SameValueMembers != nil {
		t.Fatalf("0.0005ms apart is outside the strict tie band: %+v", fold.SameValueMembers)
	}
	fold := traceCausalProjectionOverflowFoldRow([]TraceCausalProjectionNode{base, inBand})
	if len(fold.SameValueMembers) != 2 {
		t.Fatalf("sub-0.0005ms members must disclose: %+v", fold.SameValueMembers)
	}
}

// TestDiagSameValueFoldSkipsSubjectlessMembers (复核 P3-1): the fold-of-fold
// shape — a subjectless inner fold row (hop cap re-folding a bucket fold,
// Subject "") whose value ties the outer max — must NOT mint a degenerate
// nameless witness (audit face "same_value_members=,xxx"), symmetric with
// the wire parser's empty-subject discard arm. ≥2 NON-EMPTY subjects still
// disclose beside a subjectless tie member; a tie whose only second member
// is subjectless discloses nothing.
func TestDiagSameValueFoldSkipsSubjectlessMembers(t *testing.T) {
	subjectless := diagOverflowMember("", 14.272, 7000, 7050)
	subjectless.OnChainOverflowFold = true
	subjectless.MergedCount = 3

	// Direction 1: the only tie partner is subjectless → no disclosure.
	fold := traceCausalProjectionOverflowFoldRow([]TraceCausalProjectionNode{
		diagOverflowMember("hmfs_discard-1234", 14.272, 5001, 5040),
		subjectless,
		diagOverflowMember("com.example.app-42", 13.900, 6100, 6180),
	})
	if fold.SameValueMembers != nil {
		t.Fatalf("a subjectless tie member must not count toward the ≥2 witness floor: %+v", fold.SameValueMembers)
	}

	// Direction 2: two nameable members tie beside the subjectless one — the
	// roster carries exactly the nameable pair, no degenerate entry.
	fold = traceCausalProjectionOverflowFoldRow([]TraceCausalProjectionNode{
		diagOverflowMember("hmfs_discard-1234", 14.272, 5001, 5040),
		subjectless,
		diagOverflowMember("com.example.app-42", 14.272, 6100, 6180),
	})
	want := []TraceCausalProjectionSameValueMember{
		{Subject: "hmfs_discard-1234", LineStart: 5001, LineEnd: 5040},
		{Subject: "com.example.app-42", LineStart: 6100, LineEnd: 6180},
	}
	if !reflect.DeepEqual(fold.SameValueMembers, want) {
		t.Fatalf("nameable ties must disclose without the subjectless member:\n got %+v\nwant %+v", fold.SameValueMembers, want)
	}
	for _, member := range fold.SameValueMembers {
		if strings.TrimSpace(member.Subject) == "" {
			t.Fatalf("degenerate nameless witness minted: %+v", fold.SameValueMembers)
		}
	}
}

// TestDiagSameValueFoldMemberCap: five tied members disclose only the first
// four (帽 4) — the count stays on MergedCount.
func TestDiagSameValueFoldMemberCap(t *testing.T) {
	var overflow []TraceCausalProjectionNode
	for _, s := range []string{"t1-1", "t2-2", "t3-3", "t4-4", "t5-5"} {
		overflow = append(overflow, diagOverflowMember(s, 7.777, 100, 110))
	}
	fold := traceCausalProjectionOverflowFoldRow(overflow)
	if len(fold.SameValueMembers) != 4 {
		t.Fatalf("member roster must cap at 4: %+v", fold.SameValueMembers)
	}
	if fold.MergedCount != 5 {
		t.Fatalf("the fold count keeps the true membership: %d", fold.MergedCount)
	}
}

// TestDiagSameValueUnknownBackgroundFold: the R3 unknown-background take-MAX
// fold discloses on the same criterion (both directions).
func TestDiagSameValueUnknownBackgroundFold(t *testing.T) {
	mk := func(subject string, ms float64, ls, le int) TraceCausalProjectionNode {
		n := diagOverflowMember(subject, ms, ls, le)
		n.ChainRelevance = "background"
		n.Object = "unknown-thread"
		return n
	}
	// Keep=2 seats stay individual; members 3..4 fold — give the folded pair
	// tied values.
	nodes := []TraceCausalProjectionNode{
		mk("keep1-1", 90, 10, 11),
		mk("keep2-2", 80, 20, 21),
		mk("foldA-3", 14.272, 30, 39),
		mk("foldB-4", 14.272, 40, 49),
	}
	out := traceCausalProjectionFoldUnknownBackground(nodes)
	fold := out[len(out)-1]
	if fold.MergedCount != 2 {
		t.Fatalf("expected the R3 fold row last: %+v", fold)
	}
	if len(fold.SameValueMembers) != 2 ||
		fold.SameValueMembers[0].Subject != "foldA-3" || fold.SameValueMembers[1].Subject != "foldB-4" {
		t.Fatalf("R3 tie must disclose both members: %+v", fold.SameValueMembers)
	}
	if fold.ImpactMS != 14.272 || fold.CumulativeImpactMS != 14.272 {
		t.Fatalf("R3 take-MAX value drifted: %+v", fold)
	}
	// Distinct-value direction.
	nodes[3] = mk("foldB-4", 13.9, 40, 49)
	out = traceCausalProjectionFoldUnknownBackground(nodes)
	if fold := out[len(out)-1]; fold.SameValueMembers != nil {
		t.Fatalf("distinct R3 members must not disclose: %+v", fold.SameValueMembers)
	}
}

// TestDiagSameValueMembersWireParse: the folded_rows record re-materializes
// the producer's same_value_members note; malformed entries drop and a
// sub-2 roster yields nil.
func TestDiagSameValueMembersWireParse(t *testing.T) {
	record := ObservationRecord{
		ID:        "trace_query:t#wakeup_causal_impact_fold",
		Predicate: "wakeup_causal_impact",
		Subject:   "trace",
		RichNotes: []string{
			"causality=on_wakeup_chain",
			"chain_relevance=on_chain",
			"impact=14.272",
			"folded_rows=2",
			"folded_min_ms=14.272",
			"folded_max_ms=14.272",
			"folded_subjects=hmfs_discard-1234,com.example.app-42",
			"same_value_members=hmfs_discard-1234@5001-5040,com.example.app-42@6100-6180",
		},
	}
	node := traceCausalProjectionNodeFromRecord("", record)
	want := []TraceCausalProjectionSameValueMember{
		{Subject: "hmfs_discard-1234", LineStart: 5001, LineEnd: 5040},
		{Subject: "com.example.app-42", LineStart: 6100, LineEnd: 6180},
	}
	if !reflect.DeepEqual(node.SameValueMembers, want) {
		t.Fatalf("wire roster must re-materialize:\n got %+v\nwant %+v", node.SameValueMembers, want)
	}
	if node.MergedCount != 2 || node.MergedMaxMS != 14.272 {
		t.Fatalf("fold re-materialization drifted: %+v", node)
	}

	// Malformed halves drop; fewer than two survivors → nil (never a
	// single-member "tie").
	if got := traceCausalProjectionParseSameValueMembers("only@1-2"); got != nil {
		t.Fatalf("single entry is not a tie: %+v", got)
	}
	if got := traceCausalProjectionParseSameValueMembers("a@1-2,broken@x-y,@3-4"); got != nil {
		t.Fatalf("malformed roster must drop to nil, got %+v", got)
	}
	if got := traceCausalProjectionParseSameValueMembers("a@1-2,b@3-4,broken"); len(got) != 2 {
		t.Fatalf("well-formed survivors keep the roster: %+v", got)
	}
}

// TestDiagActualCaliberCompileParse (A2): the typed thread-total caliber and
// the producer's divergence note reach the node; absence stays absent.
func TestDiagActualCaliberCompileParse(t *testing.T) {
	record := ObservationRecord{
		ID:        "trace_query:t#wakeup_causal_impact:1",
		Predicate: "wakeup_causal_impact",
		Subject:   "OS_FFRT_2_2-43037",
		RichNotes: []string{
			"causality=on_wakeup_chain",
			"actual_impact=59.050ms",
			"actual_total=112.234ms",
			"actual_caliber_note=state_segment_vs_thread_total",
		},
	}
	node := traceCausalProjectionNodeFromRecord("", record)
	if node.ActualImpactMS != 59.050 || node.ActualTotalMS != 112.234 {
		t.Fatalf("actual caliber pair must parse: impact=%v total=%v", node.ActualImpactMS, node.ActualTotalMS)
	}
	if node.ActualCaliberNote != TraceActualCaliberStateSegmentVsThreadTotal {
		t.Fatalf("caliber note must parse: %q", node.ActualCaliberNote)
	}
	// Absent note → absent field (absence never guesses).
	record.RichNotes = []string{"actual_impact=59.050ms", "actual_total=60.000ms"}
	node = traceCausalProjectionNodeFromRecord("", record)
	if node.ActualCaliberNote != "" {
		t.Fatalf("no note → no caliber disclosure, got %q", node.ActualCaliberNote)
	}
	if node.ActualTotalMS != 60.0 {
		t.Fatalf("actual_total parses independently of the note: %v", node.ActualTotalMS)
	}
}
