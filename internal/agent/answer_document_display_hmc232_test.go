package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC232ReaderIORulersStayBoundToTypedValue(t *testing.T) {
	for _, zh := range []bool{true, false} {
		for _, caliber := range []string{types.TraceIOValueCaliberRQResidence, types.TraceIOValueCaliberBIOResidence, types.TraceIOValueCaliberIssuerBlocked, types.TraceIOValueCaliberMixed, "", "future_unknown"} {
			for _, token := range []string{"io_latency", "io_wait", "io_burst_episode"} {
				n := types.TraceCausalProjectionNode{TypeToken: token, StateKind: token, IOValueCaliber: caliber}
				before := b1645Snapshot(t, n)
				want := tool.TraceMeasurementTypeDisplayLabel(token, caliber, zh)
				for name, face := range map[string]string{"cause": traceFinalReaderCauseLabel(n, zh), "context": traceFinalReaderContextLabel(n, zh), "roster": answerDocTraceRankReaderMeasuredCauseLabel(types.TraceRankRosterSeat{Type: token, IOValueCaliber: caliber}, zh)} {
					if face != want {
						t.Errorf("%s %s/%s = %q want %q", name, token, caliber, face, want)
					}
				}
				if types.TraceUsesIOValueCaliber(token, caliber) && traceFinalReaderActualCauseLabel(n, zh) != want {
					t.Errorf("actual-state fallback lost %s/%s ruler: %s", token, caliber, traceFinalReaderActualCauseLabel(n, zh))
				}
				r := types.ObservationRecord{Predicate: "root_cause_primary", ClaimKey: "root_cause_primary", Subject: "issuer-42", Object: token, Value: "35.000", Unit: "ms", Role: types.AnswerAggregateRolePrincipalAnswer, RichNotes: []string{"chain_relevance=on_chain", types.TraceNoteKeyIOValueCaliber + "=" + caliber}}
				if types.TraceUsesIOValueCaliber(token, caliber) && !strings.Contains(traceQueryObservationSupplementText(r, zh), want) {
					t.Errorf("supplement borrowed an IO ruler: %+v", r)
				}
				if before != b1645Snapshot(t, n) {
					t.Fatal("reader mutated typed value")
				}
			}
		}
		if got := traceFinalReaderMeasuredStateLabel("io_wait", "", zh); got != traceFinalReaderStateLabel("io_wait", zh) {
			t.Fatal("unqualified scheduler IO state was relabeled")
		}
	}
}

func TestHMC232SupplementRootFamilyNeedsTypedCauseQualification(t *testing.T) {
	for _, zh := range []bool{true, false} {
		for _, tc := range []struct {
			name  string
			role  types.AnswerAggregateRole
			notes []string
			cause bool
		}{
			{"legacy_explicit_chain", "", []string{"chain_relevance=on_chain"}, true},
			{"principal_explicit_causality", types.AnswerAggregateRolePrincipalAnswer, []string{"causality=on_wakeup_chain"}, true},
			{"principal_missing_qualification", types.AnswerAggregateRolePrincipalAnswer, nil, false},
			{"supporting_even_with_chain", types.AnswerAggregateRoleSupportingCoverage, []string{"chain_relevance=on_chain"}, false},
			{"adjacent", types.AnswerAggregateRolePrincipalAnswer, []string{"chain_relevance=adjacent"}, false},
			{"background_over_stale_chain", types.AnswerAggregateRolePrincipalAnswer, []string{"chain_relevance=on_chain", "causality=background"}, false},
			{"context", types.AnswerAggregateRolePrincipalAnswer, []string{"chain_relevance=on_chain", "tier=context_only"}, false},
			{"symptom", types.AnswerAggregateRolePrincipalAnswer, []string{"chain_relevance=on_chain", "tier=target_self_state"}, false},
			{"caliber_side", types.AnswerAggregateRolePrincipalAnswer, []string{"chain_relevance=on_chain", "tier=caliber_side"}, false},
			{"data_gap", types.AnswerAggregateRolePrincipalAnswer, []string{"chain_relevance=on_chain", "tier=data_gap"}, false},
			{"unproved", types.AnswerAggregateRolePrincipalAnswer, []string{"chain_relevance=on_chain", "causality=unproven"}, false},
		} {
			r := types.ObservationRecord{ClaimKey: "root_cause_primary", Predicate: "root_cause_primary", Role: tc.role, RichNotes: tc.notes}
			face := traceQueryObservationSupplementClaimLabel(r, zh)
			cause := strings.Contains(face, "根因观测") || strings.Contains(face, "root-cause observation")
			if cause != tc.cause {
				t.Errorf("%s: %q cause=%v want=%v", tc.name, face, cause, tc.cause)
			}
		}
	}
}
