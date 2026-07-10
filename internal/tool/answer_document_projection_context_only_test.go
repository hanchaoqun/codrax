package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func contextOnlyDisplayProjection() (types.TraceCausalProjection, types.TraceCausalProjectionNode) {
	winner := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "ranked-cause",
		Subject: "waker-10", Predicate: "root_cause_primary", Object: "runnable_wait",
		StateKind: "runnable", Tier: "primary", Rank: 1,
		ImpactMS: 2, CumulativeImpactMS: 2, EffectiveImpactMS: 2,
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
		LineStart: 100, LineEnd: 110, Confidence: 0.9,
	}
	context := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "handoff-context",
		Subject: "worker-20", Predicate: "root_cause_primary", Object: "running",
		StateKind: "running", Tier: types.TraceCausalTierContextOnly,
		ImpactMS: 3, CumulativeImpactMS: 3,
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 2,
		LineStart: 120, LineEnd: 140, Confidence: 0.8,
	}
	return types.TraceCausalProjection{
		WakeupPath:        []string{"waker-10", "worker-20", "app-100"},
		WindowStartTs:     5,
		WindowEndTs:       5.007,
		PrimaryRootCauses: []types.TraceCausalProjectionNode{winner},
		OnChainCauses:     []types.TraceCausalProjectionNode{winner, context},
	}, context
}

func TestContextOnlyDisplayKeepsEvidenceAndNamesNonRankingStatus(t *testing.T) {
	projection, context := contextOnlyDisplayProjection()
	index := newRuntimeTraceCausalProjectionEvidenceIndex()
	model := buildRuntimeTraceProjTreeModel(projection, index, true)

	found := false
	for _, row := range model.TreeRows {
		if row.Node.EvidenceID != context.EvidenceID {
			continue
		}
		found = true
		if strings.TrimSpace(row.EvidenceTag) == "" {
			t.Fatalf("context-only hand-off lost its E# evidence reference: %+v", row)
		}
		if runtimeTraceProjCauseNodeRow(row) {
			t.Fatalf("context-only row must not use the root-cause candidate grammar: %+v", row)
		}
	}
	if !found {
		t.Fatalf("context-only hand-off must retain a tree seat: %+v", model.TreeRows)
	}

	zhFence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(zhFence, "上下文·不参与根因排序") ||
		!strings.Contains(zhFence, "worker-20") {
		t.Fatalf("ZH tree must disclose the context-only status beside the retained row:\n%s", zhFence)
	}
	enModel := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	enFence := runtimeTraceProjTreeFence(enModel, false)
	if !strings.Contains(enFence, "context · not ranked") ||
		!strings.Contains(enFence, "worker-20") {
		t.Fatalf("EN tree must disclose the context-only status beside the retained row:\n%s", enFence)
	}

	if got := runtimeTraceProjDetailPositionMerged(context, true, false); got != "链路上下文(不参与根因排序)" {
		t.Fatalf("ZH detail position drifted: %q", got)
	}
	if got := runtimeTraceProjDetailPositionMerged(context, false, false); got != "chain context (not ranked)" {
		t.Fatalf("EN detail position drifted: %q", got)
	}
	if got := runtimeTraceCausalProjectionPriorityCell(context, true); got != "链路上下文" {
		t.Fatalf("context row must never wear 重点关注: %q", got)
	}

	_, rows := runtimeTraceProjDetailTable(model, true)
	found = false
	for _, row := range rows {
		if len(row.Cells) < 4 || !strings.Contains(row.Cells[0], "worker-20") {
			continue
		}
		found = true
		if row.Cells[3] != "—" {
			t.Fatalf("context-only row must show no root-ranking attribution: %+v", row.Cells)
		}
	}
	if !found {
		t.Fatalf("context-only row missing from the indicator table: %+v", rows)
	}
}

func TestContextOnlyDisplayPreservesRelevanceLane(t *testing.T) {
	tests := []struct {
		relevance string
		zh        string
		en        string
	}{
		{"on_chain", "链路上下文(不参与根因排序)", "chain context (not ranked)"},
		{"adjacent", "邻近上下文(不参与根因排序)", "adjacent context (not ranked)"},
		{"background", "背景上下文(不参与根因排序)", "background context (not ranked)"},
		{"", "上下文(不参与根因排序)", "context (not ranked)"},
	}
	for _, tc := range tests {
		node := types.TraceCausalProjectionNode{
			Tier: types.TraceCausalTierContextOnly, ChainRelevance: tc.relevance,
		}
		if got := runtimeTraceProjDetailPositionMerged(node, true, false); got != tc.zh {
			t.Fatalf("ZH relevance=%q: got %q want %q", tc.relevance, got, tc.zh)
		}
		if got := runtimeTraceProjDetailPositionMerged(node, false, false); got != tc.en {
			t.Fatalf("EN relevance=%q: got %q want %q", tc.relevance, got, tc.en)
		}
	}
}

func TestContextOnlyNeverBoardsBadgesOrLeadsEvenWithStaleRankFields(t *testing.T) {
	projection, context := contextOnlyDisplayProjection()
	context.Rank = 1
	context.EffectiveImpactMS = 99
	context.ImpactMS = 99
	context.CumulativeImpactMS = 99
	context.EvidenceID = "stale-context"
	projection.PrimaryRootCauses = append([]types.TraceCausalProjectionNode{context}, projection.PrimaryRootCauses...)
	projection.OnChainCauses = append(projection.OnChainCauses, context)

	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	for _, row := range runtimeTraceProjRankBoard(model.TreeRows) {
		if row.Node.IsContextOnlyRow() {
			t.Fatalf("context-only tier must override stale board fields: %+v", row.Node)
		}
	}
	for _, row := range model.TreeRows {
		if row.Node.IsContextOnlyRow() && row.Badge != 0 {
			t.Fatalf("context-only row must never wear a TOP badge: %+v", row)
		}
	}
	lead, lane := runtimeTraceProjLeadSelect(projection, model)
	if lead == nil || lead.Subject != "waker-10" || lane != runtimeTraceProjLeadLanePrimary {
		t.Fatalf("stale context rank must not displace the real lead: lane=%d lead=%+v", lane, lead)
	}
	if value := runtimeTraceProjLeadSelectionValue(context); value != 0 {
		t.Fatalf("context-only rows have no lead-election value, got %.3f", value)
	}
	if runtimeTraceProjCauseNodeRow(runtimeTraceProjTreeRow{HasData: true, Node: context}) {
		t.Fatal("stale rank fields must not restore root-cause candidate grammar")
	}
}

func TestContextOnlyCausalHopKeepsHandoffButNeverCrowns(t *testing.T) {
	projection := types.TraceCausalProjectionFromObservationRecords([]types.ObservationRecord{{
		ID:              "context-hop",
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		GroundingPolicy: types.ClaimGroundingHard,
		Predicate:       "wakeup_causal_impact",
		ClaimKey:        "wakeup_causal_impact:worker-20",
		Subject:         "worker-20",
		Object:          "running",
		Value:           "3.000",
		Unit:            "ms",
		Span:            types.ObservationSpan{LineStart: 120, LineEnd: 140},
		RichNotes: []string{
			"tier=" + types.TraceCausalTierContextOnly,
			"rank=0",
			"effective_impact_ms=0",
			"impact_ms=3.000",
			"cumulative_impact_ms=3.000",
			"chain_relevance=on_chain",
			"causality=on_wakeup_chain",
			"chain_depth=1",
		},
	}})
	if len(projection.SupportingHops) != 1 {
		t.Fatalf("context-only causal hop lost its SupportingHops seat: %+v", projection)
	}
	hop := projection.SupportingHops[0]
	if hop.Role != types.TraceCausalRoleCausalHop || !hop.IsContextOnlyRow() {
		t.Fatalf("context-only causal hop lost typed role/tier: %+v", hop)
	}
	if runtimeTraceProjLeadSelectionValue(hop) != 0 {
		t.Fatalf("context-only causal hop must have zero lead-election value: %+v", hop)
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if lead, lane := runtimeTraceProjLeadSelect(projection, model); lead != nil || lane != runtimeTraceProjLeadLaneNone {
		t.Fatalf("context-only causal hop must not lead: lane=%d lead=%+v", lane, lead)
	}
	for _, row := range model.TreeRows {
		if row.Node.IsContextOnlyRow() && row.Badge != 0 {
			t.Fatalf("context-only causal hop must retain evidence without a badge: %+v", row)
		}
	}
}
