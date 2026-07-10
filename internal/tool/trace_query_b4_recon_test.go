package tool

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB4AbsorbedRankObservationSurvivesActiveCompaction(t *testing.T) {
	const familyKey = "rank_pair:d_state_or_io_wait>io_burst_episode|pid:100|on_chain|window:5.000000..5.007000|interval:5.001000..5.002062|lines:757-758"
	window := tracequery.TimeWindow{StartTs: 5.0, EndTs: 5.007}
	absorber := tracequery.RootCauseRankItem{
		Rank: 1, Tier: "primary", Type: "d_state_or_io_wait",
		Thread:  tracequery.ThreadRef{Comm: "app-100", PID: 100},
		StartTs: 5.001, EndTs: 5.002062, LineStart: 757, LineEnd: 758,
		StatsWindowStartTs: 5.0, StatsWindowEndTs: 5.007,
		ImpactMs: 1.062, CumulativeImpactMs: 1.062, EffectiveImpactMs: 1.062,
		Source: "window_stats", Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
		RankFamilyKey: familyKey, AbsorbedRankRows: 1,
	}
	active := []tracequery.RootCauseRankItem{absorber}
	// The engine root_cause_rank cap is 12. Fill every active seat and verify
	// the lossless absorbed carrier still publishes beyond that active board.
	for i := 2; i <= 12; i++ {
		active = append(active, tracequery.RootCauseRankItem{
			Rank: i, Tier: "tertiary", Type: "running",
			Thread:   tracequery.ThreadRef{Comm: "other", PID: 100 + i},
			ImpactMs: float64(20 - i), CumulativeImpactMs: float64(20 - i),
			EffectiveImpactMs: float64(20 - i), Source: "window_stats",
			Causality: "background", ChainRelevance: "background",
			LineStart: 800 + i, LineEnd: 800 + i,
		})
	}
	absorbed := tracequery.RootCauseRankItem{
		Rank: 0, Tier: tracequery.RootCauseTierAbsorbed, Type: "io_burst_episode",
		Thread:  tracequery.ThreadRef{Comm: "app-100", PID: 100},
		StartTs: 5.001, EndTs: 5.002062, LineStart: 757, LineEnd: 758,
		StatsWindowStartTs: 5.0, StatsWindowEndTs: 5.007,
		ImpactMs: 1.062, CumulativeImpactMs: 1.062, EffectiveImpactMs: 1.062,
		Source: "window_stats.io_burst_episodes", Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
		AbsorbedByRankFamily: true, AbsorbedIntoFamily: familyKey,
	}
	result := tracequery.Result{
		View: "root_cause_rank",
		RootCauseRank: &tracequery.RootCauseRankResult{
			Window: window, Items: active, AbsorbedItems: []tracequery.RootCauseRankItem{absorbed},
			Compactions: []tracequery.ViewCompaction{{View: "root_cause_rank", Total: 13, Emitted: 12}},
		},
	}
	records := traceQueryTypedObservations(result, "b4.systrace", "payload", "raw", "", time.Unix(1, 0).UTC())
	rootRecords := 0
	foundFamily, foundAbsorbed := false, false
	for _, record := range records {
		if !strings.HasPrefix(record.ClaimKey, "root_cause_") {
			continue
		}
		rootRecords++
		notes := strings.Join(record.RichNotes, "\n")
		if strings.Contains(notes, "rank_family_key="+familyKey) {
			foundFamily = true
		}
		if record.ClaimKey == "root_cause_absorbed" {
			foundAbsorbed = strings.Contains(notes, "absorbed_by_rank_family=true") &&
				strings.Contains(notes, "absorbed_into="+familyKey)
			if record.Role != types.AnswerAggregateRoleSupportingCoverage ||
				record.ProvenanceLane != types.ObservationProvenanceArtifactSpan {
				t.Fatalf("absorbed row must be supporting-only but hard-grounded: %+v", record)
			}
		}
	}
	if rootRecords != 13 || !foundFamily || !foundAbsorbed {
		t.Fatalf("active cap must not erase absorbed provenance: roots=%d family=%t absorbed=%t", rootRecords, foundFamily, foundAbsorbed)
	}

	set := types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: records})
	if len(set.Projections) != 1 || len(set.Projections[0].AbsorbedChainRows) != 1 {
		t.Fatalf("projection must relocate the B4 duplicate losslessly: %+v", set.Projections)
	}
	model := buildRuntimeTraceProjTreeModel(set.Projections[0], newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	detail := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(detail, "链上并入") || !strings.Contains(detail, "1 条同源观测已并入本行") {
		t.Fatalf("absorbing D-state row must disclose the merged observation:\n%s", detail)
	}
	for _, rows := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent, model.Background} {
		for _, row := range rows {
			if row.Node.AbsorbedByRankFamily {
				t.Fatalf("absorbed io_burst_episode must own no render seat: %+v", row.Node)
			}
		}
	}
}
