package tracequery

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestRootCauseEvidenceDisplayAuthority(t *testing.T) {
	for _, tc := range []struct {
		name, relevance, causality, want string
		target                           ThreadRef
	}{
		{"targetless", "", "", "background observation", ThreadRef{}},
		{"targetless_typed_chain", "on_chain", "on_wakeup_chain", "background observation", ThreadRef{}},
		{"name_only_unresolved", "on_chain", "on_wakeup_chain", "background observation", ThreadRef{Comm: "missing"}},
		{"resolved_without_relation", "", "", "background observation", ThreadRef{PID: 41}},
		{"background", "background", "background", "background observation", ThreadRef{PID: 41}},
		{"adjacent", "adjacent", "adjacent", "adjacent observation", ThreadRef{PID: 41}},
		{"on_chain", "on_chain", "on_wakeup_chain", "primary cause #1", ThreadRef{PID: 41}},
		{"self_semantic", "", RootCauseCausalitySelfDeterministic, "primary cause #1", ThreadRef{PID: 41}},
		{"self_wall_clock", "", RootCauseCausalitySelfWallClock, "primary cause #1", ThreadRef{PID: 41}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := RootCauseRankItem{Rank: 1, Tier: "primary", Type: "running", Thread: ThreadRef{PID: 52}, ChainRelevance: tc.relevance, Causality: tc.causality, Summary: "physical running 7ms", LineStart: 5, LineEnd: 7}
			board := RootCauseRankResult{Target: tc.target, Items: []RootCauseRankItem{item}}
			before, _ := json.Marshal(board)
			facts := evidenceFromRootCauseRank(board)
			after, _ := json.Marshal(board)
			if len(facts) != 1 || !strings.Contains(facts[0].Summary, tc.want) || !strings.Contains(facts[0].Summary, item.Summary) {
				t.Fatalf("wrong display authority, want %q with original observation: %+v", tc.want, facts)
			}
			if tc.want != "primary cause #1" && strings.Contains(facts[0].Summary, "primary cause #1") {
				t.Errorf("support observation promoted to cause: %s", facts[0].Summary)
			}
			if string(before) != string(after) || facts[0].Predicate != "root_cause_primary" || facts[0].LineStart != 5 || facts[0].LineEnd != 7 {
				t.Fatal("display description changed rank/carrier identity")
			}
		})
	}
	// The engine's historical chainless ordinal is not display authority.
	if got := RootCauseRankOrdinalChannelWord(RootCauseRankItem{Rank: 1}); got != "chain" {
		t.Fatalf("display repair changed engine ordinal contract: %q", got)
	}
}

func TestRootCauseEvidenceDisplayPreservesUnseatedAndIOCalibers(t *testing.T) {
	for _, caliber := range []string{"block_rq_issue_to_complete", "block_bio_queue_to_complete", "completion_closed_issuer_blocked", "mixed", "", "future_unknown"} {
		t.Run(caliber, func(t *testing.T) {
			item := RootCauseRankItem{Tier: "tertiary", Type: "io_latency", IOValueCaliber: caliber, Summary: "original measured interval", CumulativeImpactMs: 35, EffectiveImpactMs: 35}
			absorbed := item
			absorbed.AbsorbedByRankFamily, absorbed.AbsorbedIntoFamily = true, "kept-family"
			board := RootCauseRankResult{Target: ThreadRef{PID: 41}, Items: []RootCauseRankItem{item}, AbsorbedItems: []RootCauseRankItem{absorbed}}
			facts := evidenceFromRootCauseRank(board)
			want := caliber
			if want == "" || want == "future_unknown" {
				want = "unspecified"
			}
			if len(facts) != 2 || !strings.Contains(facts[0].Summary, "no rank seat") || !strings.Contains(facts[1].Summary, "absorbed_into=kept-family") {
				t.Fatalf("unseated/absorbed observations lost: %+v", facts)
			}
			for _, fact := range facts {
				if !strings.Contains(fact.Summary, "io_value_caliber="+want) || !strings.Contains(fact.Summary, item.Summary) {
					t.Errorf("IO ruler missing or re-inferred: %s", fact.Summary)
				}
			}
			if !reflect.DeepEqual(board.Items[0], item) || !reflect.DeepEqual(board.AbsorbedItems[0], absorbed) {
				t.Fatal("display repair modified original measurements")
			}
		})
	}
}

func TestRootCauseEvidenceDisplayPublicQuery(t *testing.T) {
	for _, tc := range []struct {
		pid, targetPID int
		thread         string
	}{
		{0, 0, ""}, {41, 41, ""}, {99, 99, ""},
		{0, 41, "issuer"}, {0, 0, "not-present"},
	} {
		t.Run(fmt.Sprintf("%d/%s", tc.pid, tc.thread), func(t *testing.T) {
			result := Run(ioValueCaliberIndex(t, "rq", "S", true), Query{View: "root_cause_rank", PID: tc.pid, Thread: tc.thread, TimeStart: 5, TimeEnd: 5.001, MinDurationMs: .001, Limit: 64})
			if result.RootCauseRank == nil {
				t.Fatal("public query lost rank board")
			}
			if result.RootCauseRank.Target.PID != tc.targetPID {
				t.Fatalf("native target resolution changed: %+v", result.RootCauseRank.Target)
			}
			wantCaliber, wantValue := "block_rq_issue_to_complete", .100
			if tc.targetPID == 41 {
				wantCaliber, wantValue = "completion_closed_issuer_blocked", .090
			}
			foundRow, foundFact := false, false
			for _, row := range append(result.RootCauseRank.Items, result.RootCauseRank.AbsorbedItems...) {
				if row.Type != "io_latency" {
					continue
				}
				foundRow = true
				if row.IOValueCaliber != wantCaliber || !near(row.CumulativeImpactMs, wantValue, .000001) {
					t.Errorf("query's original request/issuer ruler changed: %+v", row)
				}
				if tc.targetPID == 0 && tc.thread == "" && (row.Rank <= 0 || RootCauseRankOrdinalChannelWord(row) != "chain") {
					t.Errorf("targetless engine ordinal must remain, independently of display: %+v", row)
				}
			}
			for _, fact := range result.EvidencePack {
				if fact.Object != "io_latency" || !strings.HasPrefix(fact.Predicate, "root_cause_") {
					continue
				}
				foundFact = true
				if !strings.Contains(fact.Summary, "io_value_caliber="+wantCaliber) {
					t.Errorf("public evidence omits original ruler: %s", fact.Summary)
				}
				if tc.targetPID != 41 && (!strings.Contains(fact.Summary, "background observation") || strings.Contains(fact.Summary, " cause #")) {
					t.Errorf("targetless original rank wrongly described as target cause: %s", fact.Summary)
				}
				if tc.targetPID == 41 && !strings.Contains(fact.Summary, "cause #") {
					t.Errorf("resolved query lost its on-chain cause: %s", fact.Summary)
				}
			}
			if !foundRow || !foundFact {
				t.Fatalf("query lost native IO: row=%t evidence=%t", foundRow, foundFact)
			}
		})
	}
}
