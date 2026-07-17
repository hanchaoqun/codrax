package tool

// trace_query_absorbed_seat_c4_test.go — C4 absorbed 行自描述 (RANKDIS-SWEEP
// M7, §29.104.16.1, docs/design/rankdis_sweep_20260716.md 编队 C4, 2026-07-17).
//
// Witness (cust_span_vs_prio): the customer's model grepped `"type":` across
// the rank payload and read absorbed rows as competing seats — the typed
// absorbed identity (tier=absorbed / absorbed_by_rank_family / absorbed_into)
// existed but sat off the grep line. 选型定谳 (delegated default, §29.104.19):
// NO new wire field — `tier` IS the typed seat-status enum
// (tracequery.RootCauseTierAbsorbed) and already sits at the type 邻位 on both
// wire faces; a second field carrying the same enum would be a second wording
// home (观测/引擎单一值源), and under MarshalIndent (one field per line) no
// field can ever share the `"type":` line anyway. What was an ACCIDENT of
// struct order / line format becomes a COMMITMENT here: the adjacency pins
// below redden if a reorder or writer change ever separates the absorbed
// identity from the type token again.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func c4AbsorbedItem() tracequery.RootCauseRankItem {
	return tracequery.RootCauseRankItem{
		Rank: 0, Tier: tracequery.RootCauseTierAbsorbed,
		Type:   "scheduler_latency",
		Thread: tracequery.ThreadRef{Comm: "shadowhook-task", PID: 64305},
		StartTs: 17729.520, EndTs: 17729.530,
		ImpactMs: 8.226, Confidence: 0.8, LineStart: 100, LineEnd: 140,
		Source:               "scheduler_latency_stats",
		AbsorbedByRankFamily: true,
		AbsorbedIntoFamily:   "runnable_wait|pid:64305|on_chain|17729.520000..17729.530000",
		Summary:              "scheduler latency projection absorbed by the formal runnable seat",
	}
}

// The rank TEXT face: the absorbed row's line leads `- rank=0 tier=absorbed`
// BEFORE `type=` — the absorbed identity and the type token share ONE line —
// and the trailing reconciliation marker names the absorbing family.
func TestC4AbsorbedRankTextLineSelfDescribes(t *testing.T) {
	result := tracequery.Result{
		View: "root_cause_rank",
		RootCauseRank: &tracequery.RootCauseRankResult{
			Items: []tracequery.RootCauseRankItem{{
				Rank: 1, Tier: "primary", Type: "runnable_wait",
				Thread:   tracequery.ThreadRef{Comm: "shadowhook-task", PID: 64305},
				ImpactMs: 8.226, EffectiveImpactMs: 8.608, Confidence: 0.9,
				LineStart: 100, LineEnd: 140, Source: "window_stats",
				RankFamilyKey:    "runnable_wait|pid:64305|on_chain|17729.520000..17729.530000",
				AbsorbedRankRows: 1,
				Summary:          "formal runnable seat",
			}},
			AbsorbedItems: []tracequery.RootCauseRankItem{c4AbsorbedItem()},
		},
	}
	summary := traceQuerySummary(result, traceQueryParams{View: "root_cause_rank"}, "path", "/tmp/payload.json")
	absorbedLine := ""
	for _, line := range strings.Split(summary, "\n") {
		if strings.Contains(line, "tier=absorbed") {
			absorbedLine = line
		}
	}
	if absorbedLine == "" {
		t.Fatalf("the absorbed row must render on the rank text face:\n%s", summary)
	}
	tierAt := strings.Index(absorbedLine, "rank=0 tier=absorbed")
	typeAt := strings.Index(absorbedLine, "type=scheduler_latency")
	if tierAt < 0 || typeAt < 0 || tierAt > typeAt {
		t.Fatalf("邻位承诺: rank=0 tier=absorbed must lead the SAME line before type=:\n%s", absorbedLine)
	}
	if !strings.Contains(absorbedLine, "absorbed_by_rank_family=true absorbed_into=") {
		t.Fatalf("the absorbed line must name its absorbing family inline:\n%s", absorbedLine)
	}
	// The absorbing row keeps its own inline counter-marker.
	if !strings.Contains(summary, "absorbed_rank_rows=1 rank_family_key=") {
		t.Fatalf("the absorber must disclose its absorbed count inline:\n%s", summary)
	}
}

// The JSON payload face (MarshalIndent, one field per line): the absorbed
// row's `"tier": "absorbed"` renders on the line IMMEDIATELY above `"type":`
// — struct order puts rank/tier/type together and the recon zeroes
// BackgroundRank so the omitempty slot between them stays empty. A reorder or
// an always-present field wedged between them reddens here.
func TestC4AbsorbedRankJSONTierAdjacentToType(t *testing.T) {
	payload, err := json.MarshalIndent(c4AbsorbedItem(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(payload), "\n")
	tierAt := -1
	for i, line := range lines {
		if strings.Contains(line, "\"tier\": \"absorbed\"") {
			tierAt = i
		}
	}
	if tierAt <= 0 {
		t.Fatalf("absorbed tier line missing:\n%s", payload)
	}
	if !strings.Contains(lines[tierAt-1], "\"rank\": 0") {
		t.Fatalf("邻位承诺: \"rank\": 0 must sit directly above the tier line:\n%s", payload)
	}
	if tierAt+1 >= len(lines) || !strings.Contains(lines[tierAt+1], "\"type\": \"scheduler_latency\"") {
		t.Fatalf("邻位承诺: \"type\" must sit directly below \"tier\": \"absorbed\":\n%s", payload)
	}
	// The full typed self-description family stays on the wire.
	for _, want := range []string{"\"absorbed_by_rank_family\": true", "\"absorbed_into\":"} {
		if !strings.Contains(string(payload), want) {
			t.Fatalf("absorbed wire self-description %q missing:\n%s", want, payload)
		}
	}
}
