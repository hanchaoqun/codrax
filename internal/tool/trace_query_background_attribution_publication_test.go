package tool

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestTraceQueryBackgroundOrderingValueDoesNotPublishAsEffectiveAttribution(t *testing.T) {
	priorityBackground := tracequery.RootCauseRankItem{
		Rank: 2, Tier: "tertiary", Type: "priority_inversion_candidate",
		Thread:     tracequery.ThreadRef{Comm: "priority-bg", PID: 901},
		RunnableMs: 100, ImpactMs: 100, CumulativeImpactMs: 100, EffectiveImpactMs: 100,
		PriorityRelationCaliber: "closed_range_stable", PriorityRelationProvenLowerMs: 100,
		PriorityRelationArtifactSources: []string{"compat:index"},
		ChainRelevance:                  "background", Causality: "background",
	}
	if got := traceQueryRootCauseForPublication(priorityBackground); got.Type != priorityBackground.Type {
		t.Fatalf("valid background priority evidence was demoted before background publication: %+v", got)
	}
	priorityResult := tracequery.Result{View: "root_cause_rank", RootCauseRank: &tracequery.RootCauseRankResult{Items: []tracequery.RootCauseRankItem{
		{
			Rank: 1, Tier: "primary", Type: "priority_inversion_candidate",
			Thread:     tracequery.ThreadRef{Comm: "priority-on", PID: 902},
			RunnableMs: 2, ImpactMs: 2, CumulativeImpactMs: 2, EffectiveImpactMs: 2,
			PriorityRelationCaliber: "closed_range_stable", PriorityRelationProvenLowerMs: 2,
			PriorityRelationArtifactSources: []string{"compat:index"},
			ChainRelevance:                  "on_chain", Causality: "on_wakeup_chain",
		},
		priorityBackground,
	}}}
	for _, record := range traceQueryTypedObservations(priorityResult, "customer.systrace", "payload-ref", "raw-ref", "", time.Unix(0, 0).UTC()) {
		if record.Subject == "priority-bg-901" && record.Predicate != "root_cause_background" {
			t.Fatalf("valid priority background row drifted during typed observation publication: %+v", record)
		}
	}

	result := tracequery.Result{
		View: "root_cause_rank", SourcePath: "/trace/customer.systrace", TimeStart: 1, TimeEnd: 1.1,
		RootCauseRank: &tracequery.RootCauseRankResult{
			Window: tracequery.TimeWindow{StartTs: 1, EndTs: 1.1},
			Items: []tracequery.RootCauseRankItem{
				{
					Rank: 1, Tier: "secondary", Type: "runnable_wait",
					Thread:     tracequery.ThreadRef{Comm: "logger", PID: 900},
					RunnableMs: 7, ImpactMs: 19.5, CumulativeImpactMs: 19.5, EffectiveImpactMs: 7,
					Score: 5, ChainRelevance: "background", Causality: "background", Source: "window_stats",
				},
				{
					Rank: 2, Tier: "primary", Type: "runnable_wait",
					Thread:     tracequery.ThreadRef{Comm: "worker", PID: 777},
					RunnableMs: 2, ImpactMs: 2, CumulativeImpactMs: 2, EffectiveImpactMs: 2,
					Score: 2, ChainRelevance: "on_chain", Causality: "on_wakeup_chain", Source: "wakeup_chain",
				},
			},
		},
	}

	published := traceQueryPriorityResultForPublication(result)
	if got := result.RootCauseRank.Items[0].EffectiveImpactMs; got != 7 {
		t.Fatalf("publication mutated the engine-owned background ordering value: %.3f", got)
	}
	background := published.RootCauseRank.Items[0]
	if background.EffectiveImpactMs != 0 || background.ImpactMs != 19.5 || background.CumulativeImpactMs != 19.5 {
		t.Fatalf("background publication must zero only effective attribution and retain measured context: %+v", background)
	}
	if onChain := published.RootCauseRank.Items[1]; onChain.EffectiveImpactMs != 2 {
		t.Fatalf("on-chain effective attribution drifted: %+v", onChain)
	}

	payload, err := json.Marshal(published)
	if err != nil {
		t.Fatalf("marshal published result: %v", err)
	}
	var wire struct {
		RootCauseRank struct {
			Items []map[string]any `json:"items"`
		} `json:"root_cause_rank"`
	}
	if err := json.Unmarshal(payload, &wire); err != nil {
		t.Fatalf("decode published result: %v", err)
	}
	if len(wire.RootCauseRank.Items) != 2 {
		t.Fatalf("unexpected rank payload: %s", payload)
	}
	if _, leaked := wire.RootCauseRank.Items[0]["effective_impact_ms"]; leaked {
		t.Fatalf("background private ordering value leaked on JSON effective attribution key: %s", payload)
	}
	if got := wire.RootCauseRank.Items[0]["cumulative_impact_ms"]; got != 19.5 {
		t.Fatalf("background measured scale disappeared from JSON: got=%v payload=%s", got, payload)
	}

	summary := traceQuerySummary(published, traceQueryParams{View: "root_cause_rank"}, "customer.systrace", "payload-ref")
	for _, want := range []string{"subject=logger-900", "effective_impact=0.000ms", "cumulative_impact=19.500ms", "chain_relevance=background"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary lost background caliber %q:\n%s", want, summary)
		}
	}

	records := traceQueryTypedObservations(published, "customer.systrace", "payload-ref", "raw-ref", "", time.Unix(0, 0).UTC())
	var notes string
	for _, record := range records {
		if record.Subject == "logger-900" && record.Predicate == "root_cause_background" {
			notes = strings.Join(record.RichNotes, "\n")
			if record.Value != "19.500" {
				t.Fatalf("background measured context value drifted: %+v", record)
			}
			break
		}
	}
	if notes == "" || !strings.Contains(notes, "effective_impact_ms=0.000") || strings.Contains(notes, "effective_impact_ms=7.000") {
		t.Fatalf("typed background attribution did not fail closed: %q", notes)
	}

	top, ok := traceQueryPriorityTopRootCauseForPublication(published.RootCauseRank)
	if !ok || top.Thread.PID != 777 || traceQueryRootCauseItemRelevance(top) != "on_chain" {
		t.Fatalf("background row was allowed to become the top root cause: ok=%t top=%+v", ok, top)
	}
}
