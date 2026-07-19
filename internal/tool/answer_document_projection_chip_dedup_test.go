package tool

// answer_document_projection_chip_dedup_test.go — ANSWERFACE-1 件7 (§29.144
// P3-3 「61839 同 chip 双条显示疣」, 2026-07-19): a display row whose wire
// roster carries two ∩ entries resolving to the SAME partner with identical
// values must stamp the clause ONCE — every per-row emitter (the ·∩[E#] chip
// and the 同段重叠 tree sentence) reads the clause list verbatim, so a
// duplicate clause printed the same text twice. Typed same-chip key = full
// clause identity (Ref + OverlapMS + Direction); diverging values are
// different accounts and both stay.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func chipDedupModel(t *testing.T, projection types.TraceCausalProjection) *runtimeTraceProjTreeModel {
	t.Helper()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	return &model
}

func chipDedupSemanticClauses(model *runtimeTraceProjTreeModel) []runtimeTraceProjCrossDirectionClause {
	for _, row := range runtimeTraceProjSMR1AllRows(model) {
		if row.HasData && row.Node.Role == types.TraceCausalRoleSemanticSpan {
			return row.CrossDirectionOverlapClauses
		}
	}
	return nil
}

func TestChipDedupIdenticalWireEntriesStampOnce(t *testing.T) {
	projection := axiomv2CrossDirectionProjection()
	// Duplicate the semantic seat's wire entry verbatim (the family-merge /
	// ×N aggregation shape: two engine items' rosters resolve to one partner).
	projection.SemanticSpans = append([]types.TraceCausalProjectionNode(nil), projection.SemanticSpans...)
	span := &projection.SemanticSpans[0]
	span.CrossDirectionOverlaps = append(append([]types.TraceCausalProjectionCrossDirectionOverlap(nil),
		span.CrossDirectionOverlaps...), span.CrossDirectionOverlaps[0])
	model := chipDedupModel(t, projection)
	clauses := chipDedupSemanticClauses(model)
	if len(clauses) != 1 {
		t.Fatalf("identical wire entries must stamp one clause, got %d: %+v", len(clauses), clauses)
	}
	fence := runtimeTraceProjTreeFence(*model, true)
	for _, line := range strings.Split(fence, "\n") {
		if n := strings.Count(line, "·∩["); n > 1 {
			t.Fatalf("one row must never wear the same ∩ chip twice:\n%s", line)
		}
	}
	if n := strings.Count(fence, "同段重叠 9.586ms"); n > 2 {
		t.Fatalf("the mutual sentence must render once per side, got %d:\n%s", n, fence)
	}
}

func TestChipDedupDivergingValuesBothStay(t *testing.T) {
	projection := axiomv2CrossDirectionProjection()
	projection.SemanticSpans = append([]types.TraceCausalProjectionNode(nil), projection.SemanticSpans...)
	span := &projection.SemanticSpans[0]
	second := span.CrossDirectionOverlaps[0]
	second.OverlapMS = 4.100
	span.CrossDirectionOverlaps = append(append([]types.TraceCausalProjectionCrossDirectionOverlap(nil),
		span.CrossDirectionOverlaps...), second)
	model := chipDedupModel(t, projection)
	clauses := chipDedupSemanticClauses(model)
	if len(clauses) != 2 {
		t.Fatalf("diverging overlap values are different accounts — both must stay, got %d: %+v", len(clauses), clauses)
	}
}
