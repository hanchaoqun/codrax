package tool

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPriorityPointAuthorityPayloadPublicationSanitizesEveryCarrier(t *testing.T) {
	maliciousImpact := tracequery.WakeupCausalImpact{
		Thread: tracequery.ThreadRef{Comm: "worker", PID: 200}, DominantState: string(tracequery.StateRunnable),
		PriorityRelation: "lower_priority_dependency", PriorityRelationCaliber: "advisory_nearest",
		PriorityRelationProvenLowerMs: 2, PriorityInversionCandidate: true, PriorityInversionGatedMs: 2, GatedRunnableMs: 2,
		NextStepKind: tracequery.NextStepKindPriorityInversion, NextStep: "MALICIOUS_PRIORITY_NEXT lower-priority dependency",
		Summary: "MALICIOUS_PRIORITY_SUMMARY priority inversion candidate",
	}
	maliciousAggregate := tracequery.WakeupCausalAggregate{
		Thread: tracequery.ThreadRef{Comm: "worker", PID: 200}, DominantState: string(tracequery.StateRunnable),
		PriorityRelation: "lower_priority_dependency", PriorityRelationCaliber: "advisory_nearest",
		PriorityRelationProvenLowerMs: 2, PriorityInversion: true, PriorityInversionGatedMs: 2, GatedRunnableMs: 2,
		Summary: "MALICIOUS_PRIORITY_AGGREGATE priority inversion candidate",
	}
	maliciousRoot := tracequery.RootCauseRankItem{
		Rank: 1, Tier: "primary", Type: "priority_inversion_candidate", Thread: tracequery.ThreadRef{Comm: "worker", PID: 200},
		EffectiveImpactMs: 2, Score: 9, GatedRunnableMs: 2, PriorityRelationCaliber: "advisory_nearest",
		PriorityRelationProvenLowerMs: 2, PriorityRelationUnknownOrNonLowerMs: 3,
		Summary: "MALICIOUS_PRIORITY_ROOT priority inversion candidate",
	}
	chain := &tracequery.ChainResult{
		Edges: []tracequery.WakeupEdge{{
			Waker: tracequery.ThreadRef{Comm: "worker", PID: 200}, Wakee: tracequery.ThreadRef{Comm: "app", PID: 100},
			PriorityRelation: "lower_priority_waker", PriorityRelationCaliber: "advisory_nearest", PriorityInversionCandidate: true,
		}},
		CausalImpacts: []tracequery.WakeupCausalImpact{maliciousImpact}, AggregatedImpacts: []tracequery.WakeupCausalAggregate{maliciousAggregate},
		Nodes: []tracequery.ChainNode{{ID: "n1", Summary: maliciousImpact.Summary, Impact: &maliciousImpact}},
	}
	rank := &tracequery.RootCauseRankResult{Items: []tracequery.RootCauseRankItem{maliciousRoot}}
	raw := tracequery.Result{
		View: "frame_root_cause_bundle", WakeupChain: chain, RootCauseRank: rank,
		FrameRootCauseBundle: &tracequery.FrameRootCauseBundle{WakeupChain: chain, RootCauseRank: rank},
	}
	payload, failure := traceQueryMarshalPayload("trace_query", raw)
	if failure != nil {
		t.Fatalf("payload publication failed: %+v", failure)
	}
	for _, forbidden := range []string{
		`"priority_inversion_candidate": true`, `"type": "priority_inversion_candidate"`,
		`"priority_relation_proven_lower_ms"`, "lower_priority_dependency", "lower_priority_waker",
		"MALICIOUS_PRIORITY_SUMMARY", "MALICIOUS_PRIORITY_NEXT", "MALICIOUS_PRIORITY_AGGREGATE", "MALICIOUS_PRIORITY_ROOT",
	} {
		if strings.Contains(string(payload), forbidden) {
			t.Fatalf("payload retained unauthorized priority claim %q:\n%s", forbidden, payload)
		}
	}
	if !raw.WakeupChain.Edges[0].PriorityInversionCandidate || raw.RootCauseRank.Items[0].Type != "priority_inversion_candidate" {
		t.Fatal("publication sanitizer mutated the engine result")
	}
	var published tracequery.Result
	if err := json.Unmarshal(payload, &published); err != nil {
		t.Fatal(err)
	}
	if published.WakeupChain == nil || published.WakeupChain.Edges[0].PriorityInversionCandidate ||
		published.WakeupChain.CausalImpacts[0].PriorityInversionCandidate || published.WakeupChain.CausalImpacts[0].NextStepKind != "" ||
		published.WakeupChain.CausalImpacts[0].PriorityRelationProvenLowerMs != 0 || published.WakeupChain.CausalImpacts[0].PriorityRelationUnknownOrNonLowerMs != 2 ||
		published.WakeupChain.AggregatedImpacts[0].PriorityInversion || published.RootCauseRank == nil ||
		published.WakeupChain.AggregatedImpacts[0].PriorityRelationProvenLowerMs != 0 || published.WakeupChain.AggregatedImpacts[0].PriorityRelationUnknownOrNonLowerMs != 2 ||
		published.RootCauseRank.Items[0].Type != "unknown_state" || published.RootCauseRank.Items[0].Rank != 0 ||
		published.RootCauseRank.Items[0].PriorityRelationProvenLowerMs != 0 || published.RootCauseRank.Items[0].PriorityRelationUnknownOrNonLowerMs != 5 {
		t.Fatalf("payload priority carriers were not fail-closed: %+v", published)
	}
	if published.FrameRootCauseBundle == nil || published.FrameRootCauseBundle.WakeupChain.Edges[0].PriorityInversionCandidate ||
		published.FrameRootCauseBundle.RootCauseRank.Items[0].Type != "unknown_state" {
		t.Fatalf("frame bundle bypassed the result sanitizer: %+v", published.FrameRootCauseBundle)
	}

	hard := tracequery.Result{
		WakeupChain: &tracequery.ChainResult{CausalImpacts: []tracequery.WakeupCausalImpact{{
			Thread: tracequery.ThreadRef{Comm: "hard", PID: 201}, DominantState: string(tracequery.StateRunnable),
			PriorityRelation: "lower_priority_dependency", PriorityRelationCaliber: "closed_range_stable",
			PriorityRelationProvenLowerMs: 1, PriorityInversionCandidate: true,
			PriorityRelationArtifactSources: []string{"compat:index"},
			PriorityInversionGatedMs:        1, GatedRunnableMs: 1, Summary: "AUTHORIZED_HARD_PRIORITY",
		}}},
		RootCauseRank: &tracequery.RootCauseRankResult{Items: []tracequery.RootCauseRankItem{{
			Rank: 1, Tier: "primary", Type: "priority_inversion_candidate", EffectiveImpactMs: 1,
			PriorityRelationCaliber: "closed_range_stable", PriorityRelationProvenLowerMs: 1,
			PriorityRelationArtifactSources: []string{"compat:index"},
			Summary:                         "AUTHORIZED_HARD_ROOT",
		}}},
	}
	hardPayload, hardFailure := traceQueryMarshalPayload("trace_query", hard)
	if hardFailure != nil || !strings.Contains(string(hardPayload), "AUTHORIZED_HARD_PRIORITY") ||
		!strings.Contains(string(hardPayload), `"type": "priority_inversion_candidate"`) {
		t.Fatalf("authorized hard row drifted: failure=%+v payload=%s", hardFailure, hardPayload)
	}

	overflow := raw
	overflowChain := *raw.WakeupChain
	overflowChain.CausalImpacts = append([]tracequery.WakeupCausalImpact(nil), raw.WakeupChain.CausalImpacts...)
	overflowChain.CausalImpacts[0].PriorityRelationProvenLowerMs = math.MaxFloat64
	overflowChain.CausalImpacts[0].PriorityRelationUnknownOrNonLowerMs = math.MaxFloat64
	overflow.WakeupChain = &overflowChain
	if _, overflowFailure := traceQueryMarshalPayload("trace_query", overflow); overflowFailure == nil {
		t.Fatal("advisory coverage addition overflow was hidden instead of failing serialization")
	}
}

func TestPriorityPointAuthorityHardTokenCannotOutrunCoverageAccount(t *testing.T) {
	for _, tc := range []struct {
		name   string
		proven float64
		impact float64
	}{
		{name: "missing proven coverage", proven: 0, impact: 1},
		{name: "charged impact exceeds proof", proven: 0.5, impact: 1},
		{name: "nonfinite proven coverage", proven: math.NaN(), impact: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			impact := traceQueryPriorityCausalImpactForPublication(tracequery.WakeupCausalImpact{
				Thread: tracequery.ThreadRef{Comm: "worker", PID: 200}, DominantState: string(tracequery.StateRunnable),
				PriorityRelation: "lower_priority_dependency", PriorityRelationCaliber: "closed_range_stable",
				PriorityRelationProvenLowerMs: tc.proven, PriorityInversionCandidate: true,
				PriorityRelationArtifactSources: []string{"compat:index"},
				PriorityInversionGatedMs:        tc.impact, GatedRunnableMs: tc.impact,
				NextStepKind: tracequery.NextStepKindPriorityInversion, Summary: "UNAUTHORIZED_HARD_TOKEN_IMPACT",
			})
			if impact.PriorityInversionCandidate || impact.PriorityInversionGatedMs != 0 ||
				impact.GatedRunnableMs != 0 || impact.NextStepKind != "" ||
				strings.Contains(impact.Summary, "UNAUTHORIZED_HARD_TOKEN_IMPACT") {
				t.Fatalf("hard token outran causal coverage: %+v", impact)
			}

			aggregate := traceQueryPriorityCausalAggregateForPublication(tracequery.WakeupCausalAggregate{
				Thread: tracequery.ThreadRef{Comm: "worker", PID: 200}, DominantState: string(tracequery.StateRunnable),
				PriorityRelation: "lower_priority_dependency", PriorityRelationCaliber: "closed_range_stable",
				PriorityRelationProvenLowerMs: tc.proven, PriorityInversion: true,
				PriorityRelationArtifactSources: []string{"compat:index"},
				PriorityInversionGatedMs:        tc.impact, GatedRunnableMs: tc.impact, RunnableMs: tc.impact,
				Summary: "UNAUTHORIZED_HARD_TOKEN_AGGREGATE",
			})
			if aggregate.PriorityInversion || aggregate.PriorityInversionGatedMs != 0 ||
				aggregate.GatedRunnableMs != 0 || strings.Contains(aggregate.Summary, "UNAUTHORIZED_HARD_TOKEN_AGGREGATE") {
				t.Fatalf("hard token outran aggregate coverage: %+v", aggregate)
			}

			root := traceQueryPriorityRootCauseForPublication(tracequery.RootCauseRankItem{
				Rank: 1, Tier: "primary", Type: "priority_inversion_candidate",
				Thread: tracequery.ThreadRef{Comm: "worker", PID: 200}, EffectiveImpactMs: tc.impact,
				PriorityRelationCaliber: "closed_range_stable", PriorityRelationProvenLowerMs: tc.proven,
				PriorityRelationArtifactSources: []string{"compat:index"},
				Summary:                         "UNAUTHORIZED_HARD_TOKEN_ROOT",
			})
			if root.Type != "unknown_state" || root.Rank != 0 || root.EffectiveImpactMs != 0 ||
				strings.Contains(root.Summary, "UNAUTHORIZED_HARD_TOKEN_ROOT") {
				t.Fatalf("hard token outran root coverage: %+v", root)
			}
		})
	}
}

func TestPriorityPointAuthorityWakeupEdgeRequiresOneClosedArtifactAndExactWakee(t *testing.T) {
	artifacts := []tracequery.TraceArtifactSource{
		{SourcePath: "/trace/causal.ftrace", CausalCompatible: true},
		{SourcePath: "/trace/isolated.perftrace", CausalCompatible: false},
	}
	base := tracequery.WakeupEdge{
		Waker: tracequery.ThreadRef{Comm: "worker", PID: 200}, Wakee: tracequery.ThreadRef{Comm: "app", PID: 100},
		WakerPriority: 20, WakerPrioritySource: "closed_range_stable", WakerPriorityArtifactSource: "artifact:0",
		WakeePriority: 159, WakeePriorityAuthority: "exact_at_point", WakeePriorityArtifactSource: "artifact:0",
		PriorityRelation: "lower_priority_waker", PriorityRelationCaliber: "closed_range_stable", PriorityInversionCandidate: true,
	}
	publication := func(edge tracequery.WakeupEdge, ledger []tracequery.TraceArtifactSource) tracequery.WakeupEdge {
		result := traceQueryPriorityResultForPublication(tracequery.Result{
			TraceArtifacts: ledger,
			WakeupChain:    &tracequery.ChainResult{Edges: []tracequery.WakeupEdge{edge}},
		})
		return result.WakeupChain.Edges[0]
	}
	if got := publication(base, artifacts); !got.PriorityInversionCandidate || got.PriorityRelation != "lower_priority_waker" {
		t.Fatalf("valid same-artifact exact edge drifted: %+v", got)
	}
	for name, mutate := range map[string]func(*tracequery.WakeupEdge){
		"missing waker source": func(edge *tracequery.WakeupEdge) { edge.WakerPriorityArtifactSource = "" },
		"missing wakee source": func(edge *tracequery.WakeupEdge) { edge.WakeePriorityArtifactSource = "" },
		"cross artifact":       func(edge *tracequery.WakeupEdge) { edge.WakeePriorityArtifactSource = "artifact:1" },
		"out of range": func(edge *tracequery.WakeupEdge) {
			edge.WakerPriorityArtifactSource, edge.WakeePriorityArtifactSource = "artifact:2", "artifact:2"
		},
		"isolated artifact": func(edge *tracequery.WakeupEdge) {
			edge.WakerPriorityArtifactSource, edge.WakeePriorityArtifactSource = "artifact:1", "artifact:1"
		},
		"compat beside ledger": func(edge *tracequery.WakeupEdge) {
			edge.WakerPriorityArtifactSource, edge.WakeePriorityArtifactSource = "compat:index", "compat:index"
		},
		"non exact wakee":     func(edge *tracequery.WakeupEdge) { edge.WakeePriorityAuthority = "closed_range_stable" },
		"advisory waker":      func(edge *tracequery.WakeupEdge) { edge.WakerPrioritySource = "advisory_nearest" },
		"missing waker value": func(edge *tracequery.WakeupEdge) { edge.WakerPriority = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			edge := base
			mutate(&edge)
			got := publication(edge, artifacts)
			if got.PriorityInversionCandidate || got.PriorityRelation != "" {
				t.Fatalf("unclosed edge retained hard claim: %+v", got)
			}
		})
	}
	compat := base
	compat.WakerPriorityArtifactSource, compat.WakeePriorityArtifactSource = "compat:index", "compat:index"
	if got := publication(compat, nil); !got.PriorityInversionCandidate {
		t.Fatalf("ledger-free compatibility edge was not admitted: %+v", got)
	}
	if got := publication(base, nil); got.PriorityInversionCandidate || got.PriorityRelation != "" {
		t.Fatalf("ledger-free artifact:N edge escaped compatibility closure: %+v", got)
	}
}

func TestPriorityPointAuthorityDurationClaimsRequireClosedRelationArtifactSet(t *testing.T) {
	artifacts := []tracequery.TraceArtifactSource{
		{SourcePath: "/trace/causal.ftrace", CausalCompatible: true},
		{SourcePath: "/trace/isolated.perftrace", CausalCompatible: false},
	}
	publication := func(sources []string, ledger []tracequery.TraceArtifactSource) tracequery.Result {
		return traceQueryPriorityResultForPublication(tracequery.Result{
			TraceArtifacts: ledger,
			WakeupChain: &tracequery.ChainResult{
				CausalImpacts: []tracequery.WakeupCausalImpact{{
					Thread: tracequery.ThreadRef{Comm: "worker", PID: 200}, DominantState: string(tracequery.StateRunnable),
					PriorityRelation: "lower_priority_dependency", PriorityRelationCaliber: "closed_range_stable",
					PriorityRelationProvenLowerMs: 1, PriorityRelationArtifactSources: append([]string(nil), sources...),
					PriorityInversionCandidate: true, PriorityInversionGatedMs: 1, GatedRunnableMs: 1,
				}},
				AggregatedImpacts: []tracequery.WakeupCausalAggregate{{
					Thread: tracequery.ThreadRef{Comm: "worker", PID: 200}, DominantState: string(tracequery.StateRunnable), DominantImpactMs: 1, RunnableMs: 1,
					PriorityRelation: "lower_priority_dependency", PriorityRelationCaliber: "closed_range_stable",
					PriorityRelationProvenLowerMs: 1, PriorityRelationArtifactSources: append([]string(nil), sources...),
					PriorityInversion: true, PriorityInversionGatedMs: 1, GatedRunnableMs: 1,
				}},
			},
			RootCauseRank: &tracequery.RootCauseRankResult{Items: []tracequery.RootCauseRankItem{{
				Rank: 1, Tier: "primary", Type: "priority_inversion_candidate", EffectiveImpactMs: 1, GatedRunnableMs: 1,
				PriorityRelationCaliber: "closed_range_stable", PriorityRelationProvenLowerMs: 1,
				PriorityRelationArtifactSources: append([]string(nil), sources...),
			}}},
		})
	}
	assertAuthorized := func(t *testing.T, got tracequery.Result) {
		t.Helper()
		if !got.WakeupChain.CausalImpacts[0].PriorityInversionCandidate ||
			!got.WakeupChain.AggregatedImpacts[0].PriorityInversion ||
			got.RootCauseRank.Items[0].Type != "priority_inversion_candidate" {
			t.Fatalf("closed duration carriers drifted: %+v", got)
		}
	}
	assertDenied := func(t *testing.T, got tracequery.Result) {
		t.Helper()
		impact := got.WakeupChain.CausalImpacts[0]
		aggregate := got.WakeupChain.AggregatedImpacts[0]
		root := got.RootCauseRank.Items[0]
		if impact.PriorityInversionCandidate || impact.PriorityRelation != "" ||
			impact.PriorityRelationProvenLowerMs != 0 || impact.PriorityRelationUnknownOrNonLowerMs != 1 ||
			aggregate.PriorityInversion || aggregate.PriorityRelation != "" ||
			aggregate.PriorityRelationProvenLowerMs != 0 || aggregate.PriorityRelationUnknownOrNonLowerMs != 1 ||
			root.Type != "unknown_state" || root.Rank != 0 ||
			root.PriorityRelationProvenLowerMs != 0 || root.PriorityRelationUnknownOrNonLowerMs != 1 {
			t.Fatalf("unclosed duration carriers retained inversion authority: %+v", got)
		}
	}
	assertAuthorized(t, publication([]string{"artifact:0"}, artifacts))
	for name, sources := range map[string][]string{
		"missing":              nil,
		"out of range":         {"artifact:2"},
		"isolated member":      {"artifact:0", "artifact:1"},
		"compat beside ledger": {"compat:index"},
		"noncanonical index":   {"artifact:00"},
	} {
		t.Run(name, func(t *testing.T) { assertDenied(t, publication(sources, artifacts)) })
	}
	assertAuthorized(t, publication([]string{"compat:index"}, nil))
	assertDenied(t, publication([]string{"artifact:0"}, nil))

	// A hard caliber is not a provenance substitute even when an upstream
	// carrier forgot to set the candidate bit. Relation/proven-lower wording
	// must still fail closed, while unrelated priority context remains visible.
	contextOnly := traceQueryPriorityResultForPublication(tracequery.Result{
		TraceArtifacts: artifacts,
		WakeupChain: &tracequery.ChainResult{
			CausalImpacts: []tracequery.WakeupCausalImpact{{
				Priority: 20, PrioritySource: "closed_range_stable",
				PriorityRelation: "lower_priority_dependency", PriorityRelationCaliber: "closed_range_stable",
				PriorityRelationProvenLowerMs: 1,
			}},
			AggregatedImpacts: []tracequery.WakeupCausalAggregate{{
				PriorityRelation: "lower_priority_dependency", PriorityRelationCaliber: "closed_range_stable",
				PriorityRelationProvenLowerMs: 1,
			}},
		},
	})
	contextImpact := contextOnly.WakeupChain.CausalImpacts[0]
	contextAggregate := contextOnly.WakeupChain.AggregatedImpacts[0]
	if contextImpact.Priority != 20 || contextImpact.PrioritySource != "closed_range_stable" ||
		contextImpact.PriorityRelation != "" || contextImpact.PriorityRelationProvenLowerMs != 0 ||
		contextImpact.PriorityRelationUnknownOrNonLowerMs != 1 ||
		contextAggregate.PriorityRelation != "" || contextAggregate.PriorityRelationProvenLowerMs != 0 ||
		contextAggregate.PriorityRelationUnknownOrNonLowerMs != 1 {
		t.Fatalf("invalid non-candidate relation escaped or pure priority context was erased: %+v", contextOnly.WakeupChain)
	}
	negative := traceQueryPriorityCausalImpactForPublication(tracequery.WakeupCausalImpact{
		PriorityRelation: "lower_priority_dependency", PriorityRelationCaliber: "closed_range_stable",
		PriorityRelationProvenLowerMs: -1, PriorityRelationArtifactSources: []string{"compat:index"},
	})
	if negative.PriorityRelation != "lower_priority_dependency" || negative.PriorityRelationProvenLowerMs != 0 ||
		negative.PriorityRelationUnknownOrNonLowerMs != 0 {
		t.Fatalf("negative hard coverage was not normalized without erasing the separately proven relation: %+v", negative)
	}
	nonFinite := tracequery.Result{WakeupChain: &tracequery.ChainResult{CausalImpacts: []tracequery.WakeupCausalImpact{{
		PriorityRelation: "lower_priority_dependency", PriorityRelationCaliber: "closed_range_stable",
		PriorityRelationProvenLowerMs: math.NaN(), PriorityRelationArtifactSources: []string{"compat:index"},
	}}}}
	if _, failure := traceQueryMarshalPayload("trace_query", nonFinite); failure == nil {
		t.Fatal("NaN hard coverage was normalized instead of failing payload serialization")
	}
}

func TestPriorityPointAuthorityUntypedEvidenceCannotBypassPublication(t *testing.T) {
	const (
		unauthorizedRootSummary = "UNTYPED_ROOT_EVIDENCE_BYPASS priority inversion candidate"
		unauthorizedRootFact    = "UNTYPED_ROOT_FACT_BYPASS priority inversion candidate"
		unauthorizedRankFact    = "UNTYPED_RANK_FACT_BYPASS priority inversion candidate"
		authorizedImpact        = "AUTHORIZED_HARD_IMPACT_RETAINED"
		authorizedRank          = "AUTHORIZED_HARD_RANK_RETAINED"
		safeFactSummary         = "SAFE_NON_PRIORITY_FACT_RETAINED"
	)
	newChain := func() *tracequery.ChainResult {
		return &tracequery.ChainResult{
			CausalImpacts: []tracequery.WakeupCausalImpact{{
				Thread:           tracequery.ThreadRef{Comm: "worker", PID: 200},
				DominantState:    string(tracequery.StateRunnable),
				PriorityRelation: "lower_priority_dependency", PriorityRelationCaliber: "closed_range_stable",
				PriorityRelationProvenLowerMs: 1, PriorityInversionCandidate: true,
				PriorityRelationArtifactSources: []string{"compat:index"},
				PriorityInversionGatedMs:        1, GatedRunnableMs: 1, Summary: authorizedImpact,
			}},
			RootEvidence: []tracequery.RootEvidence{
				{
					Type: "priority_inversion_runnable_wait", Thread: tracequery.ThreadRef{Comm: "worker", PID: 200},
					DurationMs: 1, LineStart: 10, LineEnd: 11, Summary: unauthorizedRootSummary, Confidence: 0.9,
				},
				{
					Type: "priority_inversion_candidate", Thread: tracequery.ThreadRef{Comm: "worker", PID: 200},
					DurationMs: 1, LineStart: 11, LineEnd: 12, Summary: unauthorizedRootSummary, Confidence: 0.9,
				},
				{
					Type: "binder_wait", Thread: tracequery.ThreadRef{Comm: "worker", PID: 200},
					DurationMs: 2, LineStart: 12, LineEnd: 13, Summary: "SAFE_ROOT_EVIDENCE_RETAINED", Confidence: 0.8,
				},
			},
		}
	}
	newRank := func() *tracequery.RootCauseRankResult {
		return &tracequery.RootCauseRankResult{Items: []tracequery.RootCauseRankItem{{
			Rank: 1, Tier: "primary", Type: "priority_inversion_candidate",
			Thread: tracequery.ThreadRef{Comm: "worker", PID: 200}, DominantState: string(tracequery.StateRunnable),
			ImpactMs: 1, EffectiveImpactMs: 1, GatedRunnableMs: 1, Score: 1,
			PriorityRelationCaliber: "closed_range_stable", PriorityRelationProvenLowerMs: 1,
			PriorityRelationArtifactSources: []string{"compat:index"},
			Summary:                         authorizedRank,
		}}}
	}
	newEvidencePack := func() []tracequery.EvidenceFact {
		return []tracequery.EvidenceFact{
			{Subject: "worker-200", Predicate: "priority_inversion_runnable_wait", Summary: unauthorizedRootFact},
			{Subject: "worker-200", Predicate: "root_cause_primary", Object: "priority_inversion_candidate", Summary: unauthorizedRankFact},
			{Subject: "worker-200", Predicate: "wakes", Object: "app-100", Summary: safeFactSummary},
		}
	}
	newResult := func(view string, nestedFrame bool) tracequery.Result {
		chain, rank := newChain(), newRank()
		result := tracequery.Result{
			View: view, WakeupChain: chain, RootCauseRank: rank, EvidencePack: newEvidencePack(),
		}
		if nestedFrame {
			result.FrameRootCauseBundle = &tracequery.FrameRootCauseBundle{WakeupChain: chain, RootCauseRank: rank}
		}
		return result
	}

	for _, tc := range []struct {
		name        string
		view        string
		nestedFrame bool
	}{
		{name: "wakeup_chain", view: "wakeup_chain"},
		{name: "root_cause_rank", view: "root_cause_rank"},
		{name: "frame_root_cause_bundle", view: "frame_root_cause_bundle", nestedFrame: true},
		{name: "trace_perf_bundle", view: "trace_perf_bundle"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := newResult(tc.view, tc.nestedFrame)
			published := traceQueryPriorityResultForPublication(raw)

			if published.WakeupChain == nil || len(published.WakeupChain.RootEvidence) != 1 ||
				published.WakeupChain.RootEvidence[0].Type != "binder_wait" {
				t.Fatalf("untyped inversion RootEvidence was not omitted: %+v", published.WakeupChain)
			}
			if len(published.EvidencePack) != 1 || published.EvidencePack[0].Summary != safeFactSummary {
				t.Fatalf("untyped inversion EvidencePack projections were not omitted: %+v", published.EvidencePack)
			}
			if len(published.WakeupChain.CausalImpacts) != 1 ||
				published.WakeupChain.CausalImpacts[0].Summary != authorizedImpact ||
				!published.WakeupChain.CausalImpacts[0].PriorityInversionCandidate {
				t.Fatalf("authorized structured impact drifted: %+v", published.WakeupChain.CausalImpacts)
			}
			if published.RootCauseRank == nil || len(published.RootCauseRank.Items) != 1 ||
				published.RootCauseRank.Items[0].Summary != authorizedRank ||
				published.RootCauseRank.Items[0].Type != "priority_inversion_candidate" {
				t.Fatalf("authorized structured rank drifted: %+v", published.RootCauseRank)
			}
			if tc.nestedFrame {
				bundle := published.FrameRootCauseBundle
				if bundle == nil || bundle.WakeupChain == nil || len(bundle.WakeupChain.RootEvidence) != 1 ||
					bundle.WakeupChain.RootEvidence[0].Type != "binder_wait" || bundle.RootCauseRank == nil ||
					bundle.RootCauseRank.Items[0].Summary != authorizedRank {
					t.Fatalf("nested frame publication drifted or bypassed sanitizer: %+v", bundle)
				}
			}
			if len(raw.WakeupChain.RootEvidence) != 3 || len(raw.EvidencePack) != 3 {
				t.Fatal("publication sanitizer mutated the engine result")
			}

			payload, failure := traceQueryMarshalPayload("trace_query", raw)
			if failure != nil {
				t.Fatalf("payload publication failed: %+v", failure)
			}
			summary := traceQuerySummary(raw, traceQueryParams{View: tc.view}, "trace.ftrace", "/tmp/payload.json")
			records := traceQueryTypedObservations(raw, "trace.ftrace", "/tmp/payload.json", "/tmp/trace.ftrace", tc.name, time.Unix(0, 0))
			recordJSON, err := json.Marshal(records)
			if err != nil {
				t.Fatal(err)
			}
			for face, output := range map[string]string{
				"payload": string(payload), "summary": summary, "typed_observations": string(recordJSON),
			} {
				for _, forbidden := range []string{unauthorizedRootSummary, unauthorizedRootFact, unauthorizedRankFact} {
					if strings.Contains(output, forbidden) {
						t.Errorf("%s retained untyped inversion bypass %q:\n%s", face, forbidden, output)
					}
				}
				for _, want := range []string{authorizedImpact, authorizedRank, safeFactSummary} {
					if !strings.Contains(output, want) {
						t.Errorf("%s lost authorized/safe structured content %q:\n%s", face, want, output)
					}
				}
			}
		})
	}
}

func priorityAuthorityPublicationNotes(notes []string) map[string]string {
	out := make(map[string]string, len(notes))
	for _, note := range notes {
		key, value, ok := strings.Cut(note, "=")
		if ok {
			out[key] = value
		}
	}
	return out
}

func TestPriorityPointAuthorityPublishesProofAndCoverageOnEveryTypedLane(t *testing.T) {
	edgeNotes := priorityAuthorityPublicationNotes(traceQueryTypedWakeupEdgeRichNotes(tracequery.WakeupEdge{
		WakerPriority: 20, WakerPriorityClass: "ohos_cfs", WakerPrioritySource: "closed_range_stable",
		WakeePriority: 159, WakeePriorityClass: "ohos_rt", WakeePrioritySource: "native_exact",
		WakeePriorityAuthority: "exact_at_point", PriorityRelation: "lower_priority_waker",
		PriorityRelationCaliber: "closed_range_stable", PriorityInversionCandidate: true,
		WakerPriorityArtifactSource: "artifact:0", WakeePriorityArtifactSource: "artifact:0",
	}, "worker-200 -> app-100"))
	for key, want := range map[string]string{
		types.TraceNoteKeyWakerPrioritySource:         "closed_range_stable",
		types.TraceNoteKeyWakeePrioritySource:         "native_exact",
		types.TraceNoteKeyWakeePriorityAuthority:      "exact_at_point",
		types.TraceNoteKeyWakerPriorityArtifactSource: "artifact:0",
		types.TraceNoteKeyWakeePriorityArtifactSource: "artifact:0",
		types.TraceNoteKeyPriorityRelationCaliber:     "closed_range_stable",
		"priority_relation":                           "lower_priority_waker",
		types.TraceNoteKeyPriorityInversionCandidate:  "true",
	} {
		if got := edgeNotes[key]; got != want {
			t.Errorf("edge note %s=%q, want %q", key, got, want)
		}
	}

	impactNotes := priorityAuthorityPublicationNotes(traceQueryTypedCausalImpactRichNotes(tracequery.WakeupCausalImpact{
		OnChain: true, ChainDepth: 1, DominantState: string(tracequery.StateRunnable),
		DominantImpactMs: 6, RunnableMs: 6,
		Priority: 20, PriorityClass: "ohos_cfs", PrioritySource: "closed_range_stable", PriorityArtifactSource: "artifact:1",
		TargetPriority: 159, TargetPriorityClass: "ohos_rt", TargetPrioritySource: "closed_range_stable", TargetPriorityArtifactSource: "artifact:0",
		PriorityRelation: "lower_priority_dependency", PriorityRelationCaliber: "closed_range_stable",
		PriorityRelationProvenLowerMs: 2.25, PriorityRelationUnknownOrNonLowerMs: 3.75,
		PriorityRelationArtifactSources: []string{"artifact:1", "artifact:0", "artifact:1"},
		PriorityInversionCandidate:      true, PriorityInversionGatedMs: 2.25, GatedRunnableMs: 2.25,
	}))
	for key, want := range map[string]string{
		types.TraceNoteKeyPrioritySource:                      "closed_range_stable",
		types.TraceNoteKeyTargetPrioritySource:                "closed_range_stable",
		types.TraceNoteKeyPriorityArtifactSource:              "artifact:1",
		types.TraceNoteKeyTargetPriorityArtifactSource:        "artifact:0",
		types.TraceNoteKeyPriorityRelationArtifactSources:     "artifact:0,artifact:1",
		types.TraceNoteKeyPriorityRelationCaliber:             "closed_range_stable",
		types.TraceNoteKeyPriorityRelationProvenLowerMS:       "2.250",
		types.TraceNoteKeyPriorityRelationUnknownOrNonLowerMS: "3.750",
		"priority_relation":                                   "lower_priority_dependency",
		types.TraceNoteKeyPriorityInversionCandidate:          "true",
	} {
		if got := impactNotes[key]; got != want {
			t.Errorf("impact note %s=%q, want %q", key, got, want)
		}
	}

	aggregateNotes := priorityAuthorityPublicationNotes(traceQueryTypedCausalAggregateRichNotes(tracequery.WakeupCausalAggregate{
		DominantState: string(tracequery.StateRunnable), DominantImpactMs: 6, RunnableMs: 6,
		PriorityRelation: "lower_priority_dependency", PriorityRelationCaliber: "closed_range_stable",
		PriorityRelationProvenLowerMs: 2.25, PriorityRelationUnknownOrNonLowerMs: 3.75,
		PriorityRelationArtifactSources: []string{"artifact:2", "artifact:0"},
		PriorityInversion:               true, PriorityInversionGatedMs: 2.25, GatedRunnableMs: 2.25,
	}))
	for key, want := range map[string]string{
		types.TraceNoteKeyPriorityRelationCaliber:             "closed_range_stable",
		types.TraceNoteKeyPriorityRelationProvenLowerMS:       "2.250",
		types.TraceNoteKeyPriorityRelationUnknownOrNonLowerMS: "3.750",
		types.TraceNoteKeyPriorityRelationArtifactSources:     "artifact:0,artifact:2",
		"priority_relation":                                   "lower_priority_dependency",
		types.TraceNoteKeyPriorityInversionCandidate:          "true",
	} {
		if got := aggregateNotes[key]; got != want {
			t.Errorf("aggregate note %s=%q, want %q", key, got, want)
		}
	}

	rankNotes := priorityAuthorityPublicationNotes(traceQueryTypedRootCauseStateRichNotes(tracequery.RootCauseRankItem{
		PriorityRelationCaliber: "closed_range_stable", PriorityRelationProvenLowerMs: 2.25,
		PriorityRelationUnknownOrNonLowerMs: 3.75,
		PriorityRelationArtifactSources:     []string{"artifact:2", "artifact:0"},
	}))
	for key, want := range map[string]string{
		types.TraceNoteKeyPriorityRelationCaliber:             "closed_range_stable",
		types.TraceNoteKeyPriorityRelationProvenLowerMS:       "2.250",
		types.TraceNoteKeyPriorityRelationUnknownOrNonLowerMS: "3.750",
		types.TraceNoteKeyPriorityRelationArtifactSources:     "artifact:0,artifact:2",
	} {
		if got := rankNotes[key]; got != want {
			t.Errorf("rank note %s=%q, want %q", key, got, want)
		}
	}
}

func TestPriorityPointAuthorityAdvisoryNeverPublishesHardInversion(t *testing.T) {
	edge := tracequery.WakeupEdge{
		Waker: tracequery.ThreadRef{Comm: "worker", PID: 200}, Wakee: tracequery.ThreadRef{Comm: "app", PID: 100},
		WakerPriority: 20, WakerPrioritySource: "advisory_nearest",
		WakeePriority: 159, WakeePriorityAuthority: "exact_at_point",
		PriorityRelation: "lower_priority_waker", PriorityRelationCaliber: "advisory_nearest",
		PriorityInversionCandidate: true,
	}
	edgeNotes := priorityAuthorityPublicationNotes(traceQueryTypedWakeupEdgeRichNotes(edge, "worker-200 -> app-100"))
	if edgeNotes[types.TraceNoteKeyWakerPrioritySource] != "advisory_nearest" || edgeNotes[types.TraceNoteKeyPriorityRelationCaliber] != "advisory_nearest" {
		t.Fatalf("advisory provenance must remain visible: %#v", edgeNotes)
	}
	if edgeNotes["priority_relation"] != "" || edgeNotes[types.TraceNoteKeyPriorityInversionCandidate] != "" {
		t.Fatalf("advisory edge minted a hard relation/candidate: %#v", edgeNotes)
	}
	if summary := traceQueryWakeupEdgeSummary(edge); strings.Contains(summary, "relation=lower_priority_waker") || strings.Contains(summary, "priority_inversion_candidate=true") {
		t.Fatalf("advisory edge summary claimed a hard inversion: %s", summary)
	}

	impactNotes := priorityAuthorityPublicationNotes(traceQueryTypedCausalImpactRichNotes(tracequery.WakeupCausalImpact{
		DominantState: string(tracequery.StateRunnable), DominantImpactMs: 6, RunnableMs: 6,
		PriorityRelation: "lower_priority_dependency", PriorityRelationCaliber: "advisory_nearest",
		PriorityRelationProvenLowerMs: 2, PriorityRelationUnknownOrNonLowerMs: 3,
		PriorityInversionCandidate: true, PriorityInversionGatedMs: 6, GatedRunnableMs: 6,
	}))
	for _, key := range []string{"priority_relation", types.TraceNoteKeyPriorityInversionCandidate, "priority_inversion_gated", types.TraceNoteKeyGatedRunnable} {
		if impactNotes[key] != "" {
			t.Errorf("advisory impact published hard note %s=%q", key, impactNotes[key])
		}
	}
	if impactNotes[types.TraceNoteKeyPriorityRelationCaliber] != "advisory_nearest" || impactNotes[types.TraceNoteKeyPriorityRelationProvenLowerMS] != "0.000" || impactNotes[types.TraceNoteKeyPriorityRelationUnknownOrNonLowerMS] != "5.000" {
		t.Errorf("advisory impact failed to reclassify malformed proven coverage as unknown: %#v", impactNotes)
	}

	malformedImpact := tracequery.WakeupCausalImpact{
		Thread: tracequery.ThreadRef{Comm: "worker", PID: 200}, DominantState: string(tracequery.StateRunnable),
		PriorityRelation: "lower_priority_dependency", PriorityRelationCaliber: "advisory_nearest",
		PriorityInversionCandidate: true, PriorityInversionGatedMs: 6, GatedRunnableMs: 6,
		NextStepKind: tracequery.NextStepKindPriorityInversion,
		NextStep:     "inspect lower-priority dependency scheduling delay",
		Summary:      "priority_inversion_candidate=true lower-priority dependency",
	}
	publishedImpact := traceQueryPriorityCausalImpactForPublication(malformedImpact)
	if publishedImpact.PriorityInversionCandidate || publishedImpact.PriorityRelation != "" || publishedImpact.NextStepKind != "" {
		t.Fatalf("advisory impact retained typed hard-claim fields: %+v", publishedImpact)
	}
	for _, forbidden := range []string{"priority_inversion_candidate=true", "lower-priority dependency"} {
		if strings.Contains(publishedImpact.Summary, forbidden) || strings.Contains(publishedImpact.NextStep, forbidden) {
			t.Fatalf("advisory impact retained hard prose %q: %+v", forbidden, publishedImpact)
		}
	}

	malformedAggregate := tracequery.WakeupCausalAggregate{
		Thread: tracequery.ThreadRef{Comm: "worker", PID: 200}, DominantState: string(tracequery.StateRunnable),
		DominantImpactMs: 6, RunnableMs: 6,
		PriorityRelation: "lower_priority_dependency", PriorityRelationCaliber: "advisory_nearest",
		PriorityInversion: true, PriorityInversionGatedMs: 6, GatedRunnableMs: 6,
		Summary: "priority_inversion_candidate=true lower-priority aggregate",
	}
	publishedAggregate := traceQueryPriorityCausalAggregateForPublication(malformedAggregate)
	if publishedAggregate.PriorityInversion || publishedAggregate.PriorityRelation != "" || publishedAggregate.GatedRunnableMs != 0 {
		t.Fatalf("advisory aggregate retained typed hard-claim fields: %+v", publishedAggregate)
	}
	if strings.Contains(publishedAggregate.Summary, "priority_inversion_candidate=true") || strings.Contains(publishedAggregate.Summary, "lower-priority aggregate") {
		t.Fatalf("advisory aggregate retained hard prose: %+v", publishedAggregate)
	}

	result := tracequery.Result{View: "wakeup_chain", WakeupChain: &tracequery.ChainResult{
		CausalImpacts:     []tracequery.WakeupCausalImpact{malformedImpact},
		AggregatedImpacts: []tracequery.WakeupCausalAggregate{malformedAggregate},
	}}
	summary := traceQuerySummary(result, traceQueryParams{View: "wakeup_chain"}, "trace.ftrace", "/tmp/payload.json")
	for _, forbidden := range []string{"priority_inversion_candidate=true", "lower-priority dependency", "lower-priority aggregate"} {
		if strings.Contains(summary, forbidden) {
			t.Fatalf("advisory summary re-minted hard prose %q:\n%s", forbidden, summary)
		}
	}
}

func TestPriorityPointAuthorityImpactCandidateRequiresFinitePositivePrioritySensitiveGate(t *testing.T) {
	base := tracequery.WakeupCausalImpact{
		Thread: tracequery.ThreadRef{Comm: "worker", PID: 200}, DominantState: string(tracequery.StateRunnable),
		PriorityRelation: "lower_priority_dependency", PriorityRelationCaliber: "closed_range_stable",
		PriorityRelationProvenLowerMs:   1,
		PriorityRelationArtifactSources: []string{"compat:index"},
		PriorityInversionCandidate:      true, NextStepKind: tracequery.NextStepKindPriorityInversion,
		NextStep: "untrusted inversion next step", Summary: "untrusted priority inversion candidate",
	}
	for name, mutate := range map[string]func(*tracequery.WakeupCausalImpact){
		"zero gated impact": func(impact *tracequery.WakeupCausalImpact) {},
		"non priority-sensitive state": func(impact *tracequery.WakeupCausalImpact) {
			impact.DominantState = string(tracequery.StateSSleep)
			impact.PriorityInversionGatedMs = 1
			impact.GatedRunnableMs = 1
		},
	} {
		t.Run(name, func(t *testing.T) {
			impact := base
			mutate(&impact)
			published := traceQueryPriorityCausalImpactForPublication(impact)
			if published.PriorityInversionCandidate || published.NextStepKind != "" ||
				strings.Contains(published.Summary, "untrusted") || strings.Contains(published.NextStep, "untrusted") {
				t.Fatalf("malformed hard impact retained inversion publication: %+v", published)
			}
			if published.PriorityRelation != "lower_priority_dependency" || published.PriorityRelationCaliber != "closed_range_stable" {
				t.Fatalf("candidate sanitation discarded a valid hard relation: %+v", published)
			}
			notes := priorityAuthorityPublicationNotes(traceQueryTypedCausalImpactRichNotes(impact))
			if notes[types.TraceNoteKeyPriorityInversionCandidate] != "" || notes["priority_inversion_gated"] != "" || notes[types.TraceNoteKeyNextStepKind] == tracequery.NextStepKindPriorityInversion {
				t.Fatalf("malformed hard impact escaped through typed notes: %#v", notes)
			}
		})
	}

	valid := base
	valid.PriorityInversionGatedMs = 1
	valid.GatedRunnableMs = 1
	if got := traceQueryPriorityCausalImpactForPublication(valid); !got.PriorityInversionCandidate || got.Summary != valid.Summary || got.NextStep != valid.NextStep {
		t.Fatalf("valid hard impact drifted: got=%+v want=%+v", got, valid)
	}

	nonFinite := base
	nonFinite.PriorityInversionGatedMs = math.Inf(1)
	payloadResult := tracequery.Result{WakeupChain: &tracequery.ChainResult{CausalImpacts: []tracequery.WakeupCausalImpact{nonFinite}}}
	if _, failure := traceQueryMarshalPayload("trace_query", payloadResult); failure == nil {
		t.Fatal("non-finite hard impact was hidden instead of failing payload serialization")
	}
}

func TestPriorityPointAuthorityPathCountsUseTheSameHardGateAsEdges(t *testing.T) {
	chain := tracequery.ChainResult{
		Target: tracequery.ThreadRef{Comm: "app", PID: 100},
		Nodes: []tracequery.ChainNode{
			{ID: "1", Thread: tracequery.ThreadRef{Comm: "worker", PID: 200}, Branch: 1, Depth: 1},
			{ID: "2", Thread: tracequery.ThreadRef{Comm: "app", PID: 100}, Branch: 1, Depth: 0},
		},
		Edges: []tracequery.WakeupEdge{{
			From: "1", To: "2", Branch: 1,
			WakerPriority: 20, WakerPrioritySource: "closed_range_stable", WakerPriorityArtifactSource: "compat:index",
			WakeePriority: 159, WakeePriorityAuthority: "exact_at_point", WakeePriorityArtifactSource: "compat:index",
			PriorityRelation: "lower_priority_waker", PriorityRelationCaliber: "advisory_nearest",
			PriorityInversionCandidate: true,
		}},
	}
	branches := traceQueryWakeupChainBranches(chain)
	if len(branches) != 1 || branches[0].PriorityInversionEdges != 0 {
		t.Fatalf("advisory edge leaked into branch inversion count: %+v", branches)
	}
	pathNotes := priorityAuthorityPublicationNotes(traceQueryTypedWakeupPathRichNotes(chain, "worker-200 -> app-100"))
	if got := pathNotes["priority_inversion_edges"]; got != "" {
		t.Fatalf("advisory edge leaked into legacy path inversion count: %q", got)
	}

	chain.Edges[0].PriorityRelationCaliber = "closed_range_stable"
	branches = traceQueryWakeupChainBranches(chain)
	if len(branches) != 1 || branches[0].PriorityInversionEdges != 1 {
		t.Fatalf("hard edge disappeared from branch inversion count: %+v", branches)
	}
	pathNotes = priorityAuthorityPublicationNotes(traceQueryTypedWakeupPathRichNotes(chain, "worker-200 -> app-100"))
	if got := pathNotes["priority_inversion_edges"]; got != "1" {
		t.Fatalf("hard edge missing from legacy path inversion count: %q", got)
	}
}

func TestPriorityPointAuthorityCoverageFailsClosedForEveryNonHardCaliber(t *testing.T) {
	for _, caliber := range []string{"", "advisory_nearest", "unknown", "future_caliber"} {
		proven, unknown := traceQueryPriorityCoverageNoteValues(caliber, 2.25, 3.75)
		if proven != "0.000" || unknown != "6.000" {
			t.Errorf("caliber %q published malformed hard coverage: proven=%q unknown=%q", caliber, proven, unknown)
		}
	}
	proven, unknown := traceQueryPriorityCoverageNoteValues("advisory_nearest", 2.25, math.Inf(1))
	if proven != "0.000" || unknown != "" {
		t.Fatalf("non-finite advisory account became a finite-looking remainder: proven=%q unknown=%q", proven, unknown)
	}
}

func TestPriorityPointAuthorityRootPublicationRequiresHardFinitePositiveEvidence(t *testing.T) {
	invalid := tracequery.RootCauseRankItem{
		Rank: 1, Tier: "primary", Type: "priority_inversion_candidate",
		Thread: tracequery.ThreadRef{Comm: "worker", PID: 200}, DominantState: string(tracequery.StateRunnable),
		RunnableMs: 6, ImpactMs: 2.25, CumulativeImpactMs: 6, EffectiveImpactMs: 2.25, Score: 9,
		PriorityRelationCaliber: "advisory_nearest", PriorityRelationProvenLowerMs: 2.25,
		PriorityRelationUnknownOrNonLowerMs: 3.75, GatedRunnableMs: 2.25,
		PriorityRelationArtifactSources: []string{"compat:index"},
		Summary:                         "priority inversion candidate from a legacy producer",
	}
	result := tracequery.Result{View: "root_cause_rank", RootCauseRank: &tracequery.RootCauseRankResult{
		Items: []tracequery.RootCauseRankItem{invalid},
	}}
	summary := traceQuerySummary(result, traceQueryParams{View: "root_cause_rank"}, "trace.ftrace", "/tmp/payload.json")
	for _, forbidden := range []string{"type=priority_inversion_candidate", "priority inversion candidate", "score=9.000"} {
		if strings.Contains(summary, forbidden) {
			t.Errorf("advisory root retained hard wording %q:\n%s", forbidden, summary)
		}
	}
	for _, want := range []string{"rank=0", "tier=context_only", "type=unknown_state", "effective_impact=0.000ms", "priority_relation_proven_lower_ms=0.000", "priority_relation_unknown_or_nonlower_ms=6.000"} {
		if !strings.Contains(summary, want) {
			t.Errorf("demoted root summary missing %q:\n%s", want, summary)
		}
	}

	records := traceQueryTypedObservations(result, "trace.ftrace", "/tmp/payload.json", "/tmp/trace.ftrace", "priority-authority", time.Unix(0, 0))
	var root *types.ObservationRecord
	for i := range records {
		if strings.Contains(records[i].ID, "#root_cause_rank:") {
			root = &records[i]
			break
		}
	}
	if root == nil {
		t.Fatalf("demoted root context record missing: %+v", records)
	}
	if root.Object != "unknown_state" || root.ClaimKey != "root_cause_context_only" || root.Role != types.AnswerAggregateRoleSupportingCoverage {
		t.Errorf("advisory root was not demoted to supporting context: %+v", *root)
	}
	rootNotes := priorityAuthorityPublicationNotes(root.RichNotes)
	for _, key := range []string{types.TraceNoteKeyGatedRunnable, types.TraceNoteKeyGatedRunningDeficit, "priority_inversion_gated"} {
		if rootNotes[key] != "" {
			t.Errorf("demoted root retained gated note %s=%q", key, rootNotes[key])
		}
	}
	if rootNotes[types.TraceNoteKeyPriorityRelationProvenLowerMS] != "0.000" || rootNotes[types.TraceNoteKeyPriorityRelationUnknownOrNonLowerMS] != "6.000" {
		t.Errorf("demoted root coverage was not fail-closed: %#v", rootNotes)
	}

	malformedHard := invalid
	malformedHard.PriorityRelationCaliber = "closed_range_stable"
	malformedHard.EffectiveImpactMs = math.NaN()
	if got := traceQueryPriorityRootCauseForPublication(malformedHard); got.Type != "unknown_state" || got.Rank != 0 || got.Tier != tracequery.RootCauseTierContextOnly {
		t.Fatalf("non-finite hard root was not demoted: %+v", got)
	}
	valid := invalid
	valid.PriorityRelationCaliber = "closed_range_stable"
	if got := traceQueryPriorityRootCauseForPublication(valid); got.Type != valid.Type || got.Rank != valid.Rank || got.GatedRunnableMs != valid.GatedRunnableMs {
		t.Fatalf("valid hard root drifted during publication: got=%+v want=%+v", got, valid)
	}
}

func TestPriorityPointAuthoritySummaryCarriesProofCoverageWithoutLegacyDrift(t *testing.T) {
	window := tracequery.TimeWindow{StartTs: 5, EndTs: 5.007}
	result := tracequery.Result{
		View: "root_cause_rank", TimeStart: window.StartTs, TimeEnd: window.EndTs,
		TraceArtifacts: []tracequery.TraceArtifactSource{
			{SourcePath: "/trace/primary.ftrace", CausalCompatible: true},
			{SourcePath: "/trace/secondary.ftrace", CausalCompatible: true},
		},
		WakeupChain: &tracequery.ChainResult{
			Window: window,
			Edges: []tracequery.WakeupEdge{{
				Waker: tracequery.ThreadRef{Comm: "worker", PID: 200}, Wakee: tracequery.ThreadRef{Comm: "app", PID: 100},
				WakerPriority: 20, WakerPriorityClass: "ohos_cfs", WakerPrioritySource: "closed_range_stable",
				WakeePriority: 159, WakeePriorityClass: "ohos_rt", WakeePrioritySource: "native_exact",
				WakeePriorityAuthority: "exact_at_point", PriorityRelation: "lower_priority_waker",
				PriorityRelationCaliber: "closed_range_stable", PriorityInversionCandidate: true,
				WakerPriorityArtifactSource: "artifact:0", WakeePriorityArtifactSource: "artifact:0",
			}},
			CausalImpacts: []tracequery.WakeupCausalImpact{{
				Thread: tracequery.ThreadRef{Comm: "worker", PID: 200}, Window: window, ChainDepth: 1, OnChain: true,
				DominantState: string(tracequery.StateRunnable), RunnableMs: 2.25,
				PrioritySource: "closed_range_stable", PriorityArtifactSource: "artifact:1",
				TargetPrioritySource: "closed_range_stable", TargetPriorityArtifactSource: "artifact:0",
				PriorityRelation: "lower_priority_dependency", PriorityRelationCaliber: "closed_range_stable",
				PriorityRelationProvenLowerMs: 2.25, PriorityRelationUnknownOrNonLowerMs: 4.75,
				PriorityRelationArtifactSources: []string{"artifact:0", "artifact:1"},
				PriorityInversionCandidate:      true,
			}},
			AggregatedImpacts: []tracequery.WakeupCausalAggregate{{
				Thread: tracequery.ThreadRef{Comm: "worker", PID: 200}, Path: "worker-200 -> app-100",
				ChainDepth: 1, DominantState: string(tracequery.StateRunnable), RunnableMs: 2.25,
				PriorityRelation: "lower_priority_dependency", PriorityRelationCaliber: "closed_range_stable",
				PriorityRelationProvenLowerMs: 2.25, PriorityRelationUnknownOrNonLowerMs: 4.75,
				PriorityRelationArtifactSources: []string{"artifact:0", "artifact:1"},
				PriorityInversion:               true,
			}},
		},
		RootCauseRank: &tracequery.RootCauseRankResult{Window: window, Items: []tracequery.RootCauseRankItem{{
			Rank: 1, Tier: "primary", Type: "priority_inversion_candidate",
			Thread: tracequery.ThreadRef{Comm: "worker", PID: 200}, StartTs: window.StartTs, EndTs: window.EndTs,
			PriorityRelationCaliber: "closed_range_stable", PriorityRelationProvenLowerMs: 2.25,
			PriorityRelationUnknownOrNonLowerMs: 4.75,
			PriorityRelationArtifactSources:     []string{"artifact:0", "artifact:1"},
		}}},
	}
	summary := traceQuerySummary(result, traceQueryParams{View: "root_cause_rank"}, "trace.ftrace", "/tmp/payload.json")
	for _, want := range []string{
		"waker_prio_source=closed_range_stable",
		"wakee_prio_authority=exact_at_point",
		"waker_priority_artifact_source=artifact:0",
		"wakee_priority_artifact_source=artifact:0",
		"priority_source=closed_range_stable",
		"priority_artifact_source=artifact:1",
		"target_priority_source=closed_range_stable",
		"target_priority_artifact_source=artifact:0",
		"priority_relation_artifact_sources=artifact:0,artifact:1",
		"priority_relation_caliber=closed_range_stable",
		"priority_relation_proven_lower_ms=2.250",
		"priority_relation_unknown_or_nonlower_ms=4.750",
	} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary missing proof/coverage token %q:\n%s", want, summary)
		}
	}

	legacy := tracequery.Result{View: "wakeup_chain", WakeupChain: &tracequery.ChainResult{Edges: []tracequery.WakeupEdge{{
		Waker: tracequery.ThreadRef{Comm: "worker", PID: 200}, Wakee: tracequery.ThreadRef{Comm: "app", PID: 100},
		PriorityRelation: "lower_priority_waker", PriorityInversionCandidate: true,
	}}}}
	legacySummary := traceQuerySummary(legacy, traceQueryParams{View: "wakeup_chain"}, "trace.ftrace", "/tmp/payload.json")
	if strings.Contains(legacySummary, "relation=lower_priority_waker") || strings.Contains(legacySummary, "priority_inversion_candidate=true") {
		t.Fatalf("pre-authority persisted edge minted an unproved inversion: %s", legacySummary)
	}
	if !strings.Contains(legacySummary, "relation= priority_inversion_candidate=false") {
		t.Fatalf("pre-authority edge did not preserve an explicit fail-closed result: %s", legacySummary)
	}
}
