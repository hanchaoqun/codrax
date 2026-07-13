package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// WO-N1 (SMR-1 批 SMR-S13, smr_audit_report §②, 2026-07-12; 8411 witness):
// the ▒ background stanza rendered three same-thread IO rows — E23(+2)
// 13.418ms io family (×3, wall envelope), E24 4.262ms whose wall clock sits
// INSIDE that envelope (~99% overlap, additive read double-counts ≈4.23ms),
// and E25 2.411ms with ZERO wall-clock overlap. The NEW-3 fold now (a) reaches
// the ◇/▒ direct-mint lanes and (b) judges connectivity on the members'
// WALL-CLOCK segments — 行号包络连通判被禁 (the line envelope swallowed the
// disjoint E25 in row-number space). E24 折入 / E25 存续 双断言.

func smr1N1BackgroundIONode(id, token string, impact, startTs, endTs float64, lineStart, lineEnd int) types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		Role:               types.TraceCausalRoleRootCauseContext,
		EvidenceID:         id,
		Subject:            "threadpoolforeg-60555",
		Object:             token,
		TypeToken:          token,
		Predicate:          "critical_blocking",
		ChainRelevance:     "background",
		ImpactMS:           impact,
		CumulativeImpactMS: impact,
		StartTs:            startTs,
		EndTs:              endTs,
		LineStart:          lineStart,
		LineEnd:            lineEnd,
		SupportRefs:        []string{"xxx_all.systrace:4600-15029"},
		Confidence:         0.8,
	}
}

func smr1N1Projection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"waker-1", "target-1"},
		WindowStartTs: 34579.472865,
		WindowEndTs:   34579.587805,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "chain-1",
				Subject: "waker-1", Predicate: "wakeup_causal_impact", Object: "s_sleep",
				StateKind: "s_sleep", ChainRelevance: "on_chain",
				ImpactMS: 20.000, CumulativeImpactMS: 20.000,
				Confidence: 0.78, LineStart: 50, LineEnd: 60},
		},
		BackgroundCauses: []types.TraceCausalProjectionNode{
			// E23: the io family row — wall envelope 34579.525..34579.545
			// (line envelope 4600..15029 would swallow E25 in row space).
			smr1N1BackgroundIONode("e23", "io_wait", 13.418, 34579.525000, 34579.545000, 4600, 15029),
			// E24: wall clock INSIDE the envelope → same segment, folds.
			smr1N1BackgroundIONode("e24", "io_latency", 4.262, 34579.526000, 34579.530262, 5000, 5200),
			// E25: wall clock DISJOINT from the envelope → independent row
			// (its LINE span 13814..14292 sits inside E23's line envelope —
			// exactly the banned row-number containment shape).
			smr1N1BackgroundIONode("e25", "io_latency", 2.411, 34579.560000, 34579.562411, 13814, 14292),
		},
	}
}

func TestSMR1N1BackgroundLaneFoldsInsideWallClockKeepsDisjoint(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(smr1N1Projection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	var rows []runtimeTraceProjTreeRow
	for _, row := range model.Background {
		if row.HasData {
			rows = append(rows, row)
		}
	}
	// E24 folded into E23's seat; E25 keeps its independent row → 2 seats.
	if len(rows) != 2 {
		t.Fatalf("E24 折入/E25 存续: want 2 background seats, got %d", len(rows))
	}
	var family *runtimeTraceProjTreeRow
	sawDisjoint := false
	for i := range rows {
		switch rows[i].Node.EvidenceID {
		case "e23":
			family = &rows[i]
		case "e25":
			sawDisjoint = true
		case "e24":
			t.Fatalf("E24's wall clock sits inside the family envelope — it must fold, not seat")
		}
	}
	if family == nil || !sawDisjoint {
		t.Fatalf("family seat and the disjoint row must both stay: %+v", rows)
	}
	if len(family.IOFoldPeers) != 1 || family.IOFoldPeers[0].ImpactMS != 4.262 {
		t.Fatalf("the folded E24 caliber must ride the family's note carrier: %+v", family.IOFoldPeers)
	}
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "同段IO另有") || !strings.Contains(fence, "4.262") {
		t.Fatalf("the ▒ face must carry the fold note with the absorbed caliber:\n%s", fence)
	}
	// 56643 E10 松动实锤: the disjoint 2.411 must NEVER be called 同段.
	for _, line := range strings.Split(fence, "\n") {
		if strings.Contains(line, "同段IO另有") && strings.Contains(line, "2.411") {
			t.Fatalf("wall-clock-disjoint member stamped 同段 (banned row-number containment):\n%s", line)
		}
	}
}

// Row-number containment alone must never connect (the banned judgment) —
// with NO wall-clock intervals at all, nothing folds (fail-closed).
func TestSMR1N1RowNumberEnvelopeAloneNeverFolds(t *testing.T) {
	projection := smr1N1Projection()
	for i := range projection.BackgroundCauses {
		projection.BackgroundCauses[i].StartTs = 0
		projection.BackgroundCauses[i].EndTs = 0
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	count := 0
	for _, row := range model.Background {
		if row.HasData {
			count++
			if len(row.IOFoldPeers) > 0 {
				t.Fatalf("no wall-clock proof = no fold (行号包络连通判被禁): %+v", row.IOFoldPeers)
			}
		}
	}
	if count != 3 {
		t.Fatalf("fail-closed keeps all three independent seats, got %d", count)
	}
}
