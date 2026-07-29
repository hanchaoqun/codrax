package tool

// trace_query_description_golden_test.go — PIN-1 B5 (§29.65 回归口, 2026-07-13):
// byte-golden snapshot of dispatch-sensitive tool Descriptions.
//
// EVOLUTION RECORD / why this exists (§29.64 h3 归因关账, 教训级): the
// trace_query Description is an LLM-FACING DISPATCH SURFACE — inserting one
// teaching sentence mid-Description flipped the h3 golden case 6-green →
// 2/2 red (the model stopped dispatching any rank view), and the root fix was
// Description delta ZERO (byte identity with baseline; the new note-key
// teaching moved to the data-side Summary/legend). The pre-existing substring
// pins (TestTraceQuerySchemaDocumentsViews etc.) cannot see a mid-section
// insertion or reorder, so the §29.64 root-cause shape could silently
// reappear. This golden makes any Description byte change a DELIBERATE act.
//
// EVOLUTION RECORD 2026-07-16 / R2a: one end-appended contract sentence now
// teaches the trusted raw-perf capture-quality advisory, its stream-only
// exact-zero scope, and the inventory-only analysis boundary. This is an
// explicit Description-slot exception because the model must not turn
// manifest inventory into sample/clock/root-cause evidence; the matching
// typed Summary remains the data-side authority.
//
// EVOLUTION RECORD 2026-07-16 (RANKDIS-EXT A2, §29.104.16.1 M2): ONE
// negative-teaching sentence appended immediately after the wakeup_chain
// path-records block ("wakeup_chain path/branch numbers are branch identity,
// not importance — never read a path or branch number as a root-cause
// ranking; ranked ordering lives only in root_cause_rank rows."). Delta is
// exactly that sentence; every other byte identical. Justification: witness
// cust_span_vs_prio.txt — the customer model read the rendered "wakeup
// path#1" as a second Rank#1 headline; the ClaimKey moved to the branch=
// spelling in the same batch and this sentence closes the teaching side.
// Golden-eval A/B weigh: sentence is batch-prescribed (RANKDIS-EXT dispatch,
// the single authorized Description delta); h2/h3 dispatch-variance rerun
// deferred to the batch's eval pass.
//
// EVOLUTION RECORD 2026-07-17 (值词库教学批 C3, §29.104.16.1 M5③④ + M21,
// §29.104.17 queue item; the batch §29.117 named as "the Description 正主"):
// THREE sentence rewrites, each verified against the wire before wording —
// sentence-level old↔new diff is exactly these three, zero collateral bytes:
//  ① effective_impact_ms teaching drops the stale "bounded ranking/
//     hidden-cost signal" semantics (pre-SEM-LEAD era) for the three-caliber
//     reality aligned with the GATED-CAL report words (§29.115 全额/折算/
//     构成): ranking-attribution value under a stated caliber — in full /
//     discounted supply deficit / multi-component composite — never a
//     separate elapsed-time measurement.
//  ② the phantom-key claim "root_cause_rank rows carry projected_impact_ms/
//     projected_total_ms" is corrected: grep-verified, projected_total_ms is
//     NOT a RootCauseRankItem JSON tag (it lives on WakeupCausalImpact/
//     Occurrence/Aggregate), and the rank observation NOTE lane only echoes
//     cumulative_impact_ms under that spelling (tool
//     traceQueryTypedPriorityRichNotes dual-publication). Chosen disposition
//     = a THIRD form beyond the spec's (a)/(b) fork (修补轮 件5 勘正,
//     2026-07-17): keep the alias emission (wire untouched) + honest
//     teaching. Fork (a)'s premise (key nonexistent, no demand) does not
//     hold — the key is real on wakeup_chain rows and echoed on rank notes —
//     and fork (b) (minting the rank JSON field) would enlarge the exact
//     four-names-one-value disease this batch treats; so the echo is taught
//     as one-value-two-names instead. Locked by
//     TestValueWordWireMappingLockstep's negative arm (rank struct growing a
//     projected_total_ms tag breaks the pin).
//  ③ "the top-ranked state" (state_drilldown significance teaching) is
//     scope-fixed to the drill_rank ordinal vocabulary (RANKDIS-EXT A1 data
//     side already forked drilldown ordinals to drill_rank; M21 closes the
//     teaching side so drilldown significance can never be read against the
//     root-cause board's ranked words).
// Golden-eval A/B: full SEAL-1 battery h1..h9 + h2/h3 extra runs prescribed
// by the batch (h2/h3 are the proven dispatch-variance-sensitive cases).
//
// EVOLUTION RECORD 2026-07-17 (修补轮 件4, 对抗 P3-4 + K1): ONE further
// sentence delta — the closed-matrix contract's replacement target claimed
// "assigned only by the closed typed matrix and never falls back generically
// to cumulative_impact_ms", over-claiming against the engine
// (rootCauseEffectiveImpactMsUncapped keeps a terminal residual arm that
// falls back to rootCauseCumulativeImpactMs for rows outside every matrix
// arm, K1-verified reachable). Both LLM faces (Description + Parameters) now
// carry one identical honest geometry sentence: matrix assignment, no
// generic default, residual-arm-only cumulative fallback
// (TestValueWordDescriptionTeachingScoped pins the two-face identity).
// Battery: h2/h3 one run each (Description delta re-validation; 全 9 案不必
// per coordinator).
//
// EVOLUTION RECORD 2026-07-18 (PERF-AGGREGATE-EVENT-UNIT-CHECKED-ADD): ONE
// end-appended contract sentence, immediately beside the existing raw-perf
// capture contract, teaches that event/weight-unit cohorts are the only
// weighted authority in mixed perf data, aggregate_overflow withdraws only
// that cohort's weighted faces, and legacy totals exist only for one exact
// cohort. This cannot live only in a caveat: without the negative arithmetic
// rule a model can still add individually well-grounded cycles/instructions/
// nanoseconds rows. Dispatch-risk weigh: use the already-authorized terminal
// raw-perf contract slot, not a mid-view dispatch paragraph; h2/h3 dispatch
// cases remain the required live-eval follow-up when configured credentials
// are available. The wire Summary and typed cohort records remain the data
// authority.
//
// EVOLUTION RECORD 2026-07-18 (CHAIN-BUDGET): ONE sentence appended
// immediately after the RANKDIS-EXT branch-number negative-teaching sentence
// (the same authorized path-records block boundary): a path record carrying
// side_chains=N is a branch whose N additional sleep segments were
// budget-expanded into sub-chains — the path shows the primary spine, each
// sub-chain edge publishes with segment_ordinal>=2 and its own
// leaf-to-target path note, and segment_ordinal is segment identity within
// one node, never a ranking. Delta is exactly that sentence; every other
// byte identical. Justification: side_chains/segment_ordinal are NEW wire
// note keys of this batch and carry the same misread-as-ranking risk the
// RANKDIS-EXT sentence closed for branch numbers (§29.104.16.1 M2 witness
// family); golden-eval A/B weigh deferred to the batch's eval pass, same as
// the RANKDIS-EXT precedent.
//
// EVOLUTION RECORD (SHADERCACHE-1, customer ruling 2026-07-26): mid-section
// insertion after "while generic trace_span rows remain supporting context."
// teaching the shader cache-outcome split (span_subcategory=shader_cache_miss
// = actionable compilation / shader_cache_hit = cache-served, never
// compilation cost, never precompile advice from hits / never sum the two
// families), plus the closed-matrix contract's per-kind shader arm (rides
// BOTH Description and Parameters). Dispatch sensitivity weighed: the delta
// teaches consumption of an existing typed field (span_subcategory) inside
// the semantic-span section it already governs — no new note-key teaching,
// no dispatch-surface vocabulary change.
//
// EVOLUTION RECORD (B+ 主根因 term-of-art, user ruling 2026-07-28): the
// closed-matrix contract sentence gains the 主根因 definition (the #1-elected
// seat = largest single proven on-chain eliminable contribution; never a
// mechanism verdict — mechanism claims only from chain/blocking evidence).
// Rides BOTH Description and Parameters via the contract applier; anti-
// mechanism-voice guardrail, no dispatch-vocabulary change. 「首要可消除项」
// is the reserved rename if plan A is ever ruled.
//
// UPDATE RITUAL (deliberate gate — do NOT casually regenerate):
//  1. justify the wording change against §29.64 (new note-key teaching goes
//     to the wire Summary/legend, NOT mid-Description; R2' description-slot
//     exemptions must be recorded);
//  2. regenerate via
//     PIN1_UPDATE_DESCRIPTION_GOLDEN=1 go test ./internal/tool/ -run TestToolDescriptionByteGolden
//  3. commit the golden WITH an EVOLUTION RECORD note naming the delta and,
//     for trace_query, weigh a golden-eval A/B (h2/h3 are the proven
//     dispatch-variance-sensitive cases).

import (
	"os"
	"testing"
)

func TestToolDescriptionByteGolden(t *testing.T) {
	cases := []struct {
		name   string
		golden string
		got    string
	}{
		// trace_query is THE proven dispatch-sensitive Description (§29.64
		// h3). Add further trace-facing tools here as they earn evidence of
		// dispatch sensitivity — one golden per tool.
		{"trace_query", "testdata/trace_query_description.golden", (&TraceQuery{}).Description()},
	}
	for _, tc := range cases {
		if os.Getenv("PIN1_UPDATE_DESCRIPTION_GOLDEN") == "1" {
			if err := os.WriteFile(tc.golden, []byte(tc.got), 0o644); err != nil {
				t.Fatalf("%s: update golden: %v", tc.name, err)
			}
			t.Logf("%s: golden regenerated — record an EVOLUTION RECORD in the commit (§29.64)", tc.name)
			continue
		}
		want, err := os.ReadFile(tc.golden)
		if err != nil {
			t.Fatalf("%s: read golden: %v", tc.name, err)
		}
		if tc.got == string(want) {
			continue
		}
		// Locate the first divergent byte for a useful failure message.
		i := 0
		for i < len(tc.got) && i < len(want) && tc.got[i] == want[i] {
			i++
		}
		lo := i - 80
		if lo < 0 {
			lo = 0
		}
		hiGot, hiWant := i+80, i+80
		if hiGot > len(tc.got) {
			hiGot = len(tc.got)
		}
		if hiWant > len(want) {
			hiWant = len(want)
		}
		t.Fatalf("%s: Description drifted from its byte golden at byte %d.\n"+
			"Description is a DISPATCH-SENSITIVE LLM surface (§29.64 h3: one mid-section sentence = 2/2 golden red).\n"+
			"If the change is deliberate, follow the UPDATE RITUAL in this file's header.\n"+
			"--- got  around byte %d ---\n%q\n--- want around byte %d ---\n%q",
			tc.name, i, i, tc.got[lo:hiGot], i, string(want[lo:hiWant]))
	}
}
