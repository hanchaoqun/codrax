package tool

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1603CriticalWaitRealProducerRetainsExactIntervalAuthority(t *testing.T) {
	idx, err := tracequery.BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu.ftrace")
	if err != nil {
		t.Fatal(err)
	}
	result := tracequery.Run(idx, tracequery.Query{View: "critical_blocking_calls", PID: 17267,
		TimeStart: 13762.791708, TimeEnd: 13763.024898, MaxDepth: 4, Limit: 20})
	before, _ := json.Marshal(result)
	obs := traceQueryTypedObservations(result, "donghu.ftrace", "b1603-critical", "raw", "", time.Unix(1751600000, 0).UTC())
	rm := types.RequestModel{Intent: types.IntentTrace, RuntimeTargets: []types.RuntimeTarget{{
		Kind: types.RuntimeTargetKindThread, PID: 17267, Thread: ".ugc.aweme.lite-17267", Source: "user_explicit",
	}}}
	var critical types.ObservationRecord
	for _, record := range obs {
		if record.Predicate == "critical_blocking" && record.ClaimKey == "critical_blocking:binder_wait" {
			critical = record
		}
	}
	if critical.ID == "" {
		t.Fatal("real critical publisher omitted the binder row")
	}
	var found *types.TraceBlockingWallClockAuthority
	for _, authority := range types.BuildTraceBlockingWallClockAuthorities(types.ObservationLedger{Records: obs}, &rm) {
		if authority.Type == "binder_wait" {
			copy := authority
			found = &copy
		}
	}
	if found == nil || found.CoverageStatus != "lower_bound_capacity_truncated" || len(found.Occurrences) != 1 || math.Abs(found.ObservedMS-1.409) > 0.000001 {
		t.Fatalf("critical-only exact measurement must survive without a second rank query: %+v", found)
	}
	occurrence := found.Occurrences[0]
	if occurrence.StartTs != 13762.835861 || occurrence.EndTs != 13762.837270 ||
		len(occurrence.RecordIDs) != 1 || occurrence.RecordIDs[0] != critical.ID || occurrence.Peer == "" {
		t.Fatalf("exact sleep interval, original counterpart or provenance lost: %+v", occurrence)
	}
	// Repair the source, not the duration-consistency gate. An independently
	// widened transaction envelope remains inadmissible even for this same row.
	wrong := critical
	wrong.ID += "-wrong-envelope"
	wrong.Span.StartTs = 13762.835811
	if got := types.BuildTraceBlockingWallClockAuthorities(types.ObservationLedger{Records: []types.ObservationRecord{wrong}}, &rm); len(got) != 0 {
		t.Fatalf("transaction duration mismatch must still fail precise admission: %+v", got)
	}
	after, _ := json.Marshal(result)
	if string(before) != string(after) {
		t.Fatal("authority publication rewrote engine evidence")
	}
}

func TestB1603UnclosedWaitPublicationDoesNotAcquirePricedAuthority(t *testing.T) {
	content, err := os.ReadFile("../../eval/fixtures/real_traces/donghu.ftrace")
	if err != nil {
		t.Fatal(err)
	}
	const switchPrefix = "13762.835861: sched_switch: prev_comm=.ugc.aweme.lite prev_pid=17267 prev_prio=53 prev_state="
	if strings.Count(string(content), switchPrefix+"S") != 1 {
		t.Fatal("fixture must contain the exact target sleep transition")
	}
	source := filepath.Join(t.TempDir(), "donghu-d-wait.ftrace")
	if err := os.WriteFile(source, []byte(strings.Replace(string(content), switchPrefix+"S", switchPrefix+"D", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	var all []types.ObservationRecord
	var critical types.ObservationRecord
	var sawRank bool
	for _, view := range []string{"root_cause_rank", "critical_blocking_calls"} {
		result := tracequery.Run(idx, tracequery.Query{View: view, PID: 17267,
			TimeStart: 13762.8355, TimeEnd: 13762.8375, MaxDepth: 3, MaxBranches: 4, MinDurationMs: 0.01, Limit: 20})
		if result.RootCauseRank != nil {
			for _, item := range result.RootCauseRank.Items {
				if item.Type == "binder_wait" && item.Thread.PID == 17267 {
					sawRank = true
					if item.Rank != 0 || item.Tier != tracequery.RootCauseTierTargetSelfState || traceQueryRootCauseCanBePrincipal(item) {
						t.Fatalf("unclosed wait became a principal root-cause row: %+v", item)
					}
				}
			}
		}
		obs := traceQueryTypedObservations(result, "donghu-d-wait.ftrace", "b1603-unclosed-"+view, "raw", "", time.Unix(1751600000, 0).UTC())
		all = append(all, obs...)
		for _, record := range obs {
			if record.ClaimKey == "critical_blocking:binder_wait" {
				critical = record
			}
		}
	}
	if !sawRank || critical.ID == "" || critical.Role != types.AnswerAggregateRoleSupportingCoverage ||
		critical.Span.StartTs != 0 || critical.Span.EndTs != 0 || critical.Value != "1.409" {
		t.Fatalf("unclosed measured wait must survive only as a supporting observation: rank=%t critical=%+v", sawRank, critical)
	}
	for _, note := range critical.RichNotes {
		if strings.HasPrefix(note, types.TraceNoteKeyEffectiveImpactMS+"=") || strings.HasPrefix(note, types.TraceNoteKeyRank+"=") {
			t.Fatalf("critical wait invented a price or rank annotation: %q", note)
		}
	}
	rm := types.RequestModel{Intent: types.IntentTrace, RuntimeTargets: []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 17267, Source: "user_explicit"}}}
	if got := types.BuildTraceBlockingWallClockAuthorities(types.ObservationLedger{Records: []types.ObservationRecord{critical}}, &rm); len(got) != 0 {
		t.Fatalf("identity membership without endpoints cannot authorize a measured blocking interval: %+v", got)
	}
	projection := types.CompileTraceCausalProjection(types.ObservationLedger{Records: all})
	for _, section := range TraceAnswerDecisionDirectionSections(projection) {
		for _, member := range section.Members {
			if member.TypeToken == "binder_wait" && member.Subject == critical.Subject {
				t.Fatalf("unclosed target symptom became a priced eliminable member: %+v", member)
			}
		}
	}
}
