package tool

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryRootCauseAuthorityRequiresTypedOnChainLane(t *testing.T) {
	items := []tracequery.RootCauseRankItem{
		{
			Rank: 1, Tier: "primary", Type: "runnable_wait",
			Thread:     tracequery.ThreadRef{Comm: "adjacent", PID: 200},
			RunnableMs: 9, ImpactMs: 9, CumulativeImpactMs: 9, EffectiveImpactMs: 9,
			ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Source: "window_stats",
		},
		{
			Rank: 1, Tier: "primary", Type: "runnable_wait",
			Thread:     tracequery.ThreadRef{Comm: "background", PID: 300},
			RunnableMs: 20, ImpactMs: 20, CumulativeImpactMs: 20, EffectiveImpactMs: 20,
			ChainRelevance: "background", Causality: "background", Source: "window_stats",
		},
		{
			Rank: 1, Tier: "primary", Type: "runnable_wait",
			Thread:     tracequery.ThreadRef{Comm: "onchain", PID: 100},
			RunnableMs: 4, ImpactMs: 4, CumulativeImpactMs: 4, EffectiveImpactMs: 4,
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain", Source: "wakeup_chain",
		},
	}
	rank := &tracequery.RootCauseRankResult{Window: tracequery.TimeWindow{StartTs: 1, EndTs: 1.1}, Items: items}
	bundle := &tracequery.FrameRootCauseBundle{
		Target: tracequery.ThreadRef{Comm: "app", PID: 10}, Window: rank.Window, RootCauseRank: rank,
	}
	if got := traceQueryBundleRootCauseCount(bundle); got != 1 {
		t.Fatalf("bundle root-cause count included adjacent/background rows: %d", got)
	}
	if top, ok := traceQueryPriorityTopRootCauseForPublication(rank); !ok || top.Thread.PID != 100 {
		t.Fatalf("bundle top cause was not restricted to typed on-chain rows: ok=%t top=%+v", ok, top)
	}

	result := traceQueryPriorityResultForPublication(tracequery.Result{
		View: "frame_root_cause_bundle", SourcePath: "/trace/customer.systrace", TimeStart: 1, TimeEnd: 1.1,
		RootCauseRank: rank, FrameRootCauseBundle: bundle,
	})
	published := result.FrameRootCauseBundle.RootCauseRank.Items
	if published[0].EffectiveImpactMs != 9 {
		t.Fatalf("adjacent impact must remain available as a separate support quantity: %+v", published[0])
	}
	if published[1].EffectiveImpactMs != 0 || published[1].ImpactMs != 20 {
		t.Fatalf("background caliber drifted: %+v", published[1])
	}

	records := traceQueryTypedObservations(result, "customer.systrace", "payload-ref", "raw-ref", "", time.Unix(0, 0).UTC())
	bySubject := map[string]types.ObservationRecord{}
	for _, record := range records {
		bySubject[record.Subject] = record
	}
	adjacent := bySubject["adjacent-200"]
	if adjacent.Predicate != "root_cause_adjacent" || adjacent.Role != types.AnswerAggregateRoleSupportingCoverage ||
		!strings.Contains(strings.Join(adjacent.RichNotes, "\n"), "effective_impact_ms=9.000") {
		t.Fatalf("adjacent row lost its support-only authority or measured quantity: %+v", adjacent)
	}
	background := bySubject["background-300"]
	if background.Predicate != "root_cause_background" || background.Role != types.AnswerAggregateRoleSupportingCoverage {
		t.Fatalf("background-only authority drifted: %+v", background)
	}
	onChain := bySubject["onchain-100"]
	if onChain.Predicate != "root_cause_primary" || onChain.Role != types.AnswerAggregateRolePrincipalAnswer {
		t.Fatalf("typed on-chain row lost principal authority: %+v", onChain)
	}

	var summary strings.Builder
	writeTraceFrameRootCauseBundleSummary(&summary, result.FrameRootCauseBundle)
	text := summary.String()
	if !strings.Contains(text, "root_causes=1") || !strings.Contains(text, "bundle_top_cause type=runnable_wait thread=onchain-100") ||
		strings.Contains(text, "bundle_top_cause type=runnable_wait thread=adjacent-200") {
		t.Fatalf("bundle head did not use on-chain-only root authority:\n%s", text)
	}
	fullSummary := traceQuerySummary(result, traceQueryParams{View: "frame_root_cause_bundle"}, "customer.systrace", "payload-ref")
	for _, want := range []string{"principal_eligibility=typed_on_chain_only", "adjacent_role=support_only", "background_role=support_only"} {
		if !strings.Contains(fullSummary, want) {
			t.Fatalf("model-facing summary lost root authority field %q:\n%s", want, fullSummary)
		}
	}
}

func TestTraceQueryBackgroundOnlyRankNeverPublishesPrincipalRole(t *testing.T) {
	result := tracequery.Result{View: "root_cause_rank", RootCauseRank: &tracequery.RootCauseRankResult{Items: []tracequery.RootCauseRankItem{{
		Rank: 1, Tier: "primary", Type: "runnable_wait",
		Thread:     tracequery.ThreadRef{Comm: "logger", PID: 900},
		RunnableMs: 7, ImpactMs: 19.5, CumulativeImpactMs: 19.5, EffectiveImpactMs: 7,
		ChainRelevance: "background", Causality: "background",
	}}}}
	records := traceQueryTypedObservations(result, "customer.systrace", "payload-ref", "raw-ref", "", time.Unix(0, 0).UTC())
	for _, record := range records {
		if record.Subject != "logger-900" {
			continue
		}
		if record.Predicate != "root_cause_background" || record.Role != types.AnswerAggregateRoleSupportingCoverage {
			t.Fatalf("a foregroundless background row acquired principal authority: %+v", record)
		}
		return
	}
	t.Fatal("background rank observation missing")
}
