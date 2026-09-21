package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestB1790PresentTargetProfileOwnsAnchorElection(t *testing.T) {
	fullPath := []string{"irq-80", "worker-200", "app-100"}
	for _, tc := range []struct {
		name    string
		profile *RuntimeTargetProfile
		targets []RuntimeTarget
		want    []string
		elected bool
	}{
		{"legacy_nil", nil, nil, fullPath[:2], true},
		{"no_named", &RuntimeTargetProfile{Declaration: RuntimeTargetDeclarationNoNamedTarget}, nil, fullPath, false},
		{"unspecified", &RuntimeTargetProfile{Declaration: RuntimeTargetDeclarationUnspecified}, nil, fullPath, false},
		{"not_applicable", &RuntimeTargetProfile{Declaration: RuntimeTargetDeclarationNotApplicable}, nil, fullPath, false},
		{"empty_profile", &RuntimeTargetProfile{}, nil, fullPath, false},
		{"invalid_profile", &RuntimeTargetProfile{Declaration: "invalid"}, nil, fullPath, false},
		{"named_no_quote", &RuntimeTargetProfile{Declaration: RuntimeTargetDeclarationNamedTarget}, []RuntimeTarget{{Kind: RuntimeTargetKindThread, PID: 200, Source: "user_explicit"}}, fullPath, false},
		{"named_no_target", &RuntimeTargetProfile{Declaration: RuntimeTargetDeclarationNamedTarget, SourceQuote: "requested thread"}, nil, fullPath, false},
		{"named_missing_path", &RuntimeTargetProfile{Declaration: RuntimeTargetDeclarationNamedTarget, SourceQuote: "requested thread"}, []RuntimeTarget{{Kind: RuntimeTargetKindThread, PID: 900, Source: "user_explicit"}}, fullPath, false},
		{"named_thread", &RuntimeTargetProfile{Declaration: RuntimeTargetDeclarationNamedTarget, SourceQuote: "requested thread"}, []RuntimeTarget{{Kind: RuntimeTargetKindThread, PID: 200, Source: "user_explicit"}}, fullPath[:2], true},
		{"named_alias", &RuntimeTargetProfile{Declaration: RuntimeTargetDeclarationNamedTarget, SourceQuote: "requested thread"}, []RuntimeTarget{{Kind: RuntimeTargetKindThread, Thread: "renamed-200", Source: "user_explicit"}}, fullPath[:2], true},
		{"named_diagnostic_full", &RuntimeTargetProfile{Declaration: RuntimeTargetDeclarationNamedTarget, SourceQuote: "requested thread"}, []RuntimeTarget{{Kind: RuntimeTargetKindThread, Thread: "worker-200 [200]", Source: "user_explicit"}}, fullPath[:2], true},
		{"named_diagnostic_name", &RuntimeTargetProfile{Declaration: RuntimeTargetDeclarationNamedTarget, SourceQuote: "requested thread"}, []RuntimeTarget{{Kind: RuntimeTargetKindThread, Thread: "worker [200]", Source: "user_explicit"}}, fullPath[:2], true},
		{"named_diagnostic_parens", &RuntimeTargetProfile{Declaration: RuntimeTargetDeclarationNamedTarget, SourceQuote: "requested thread"}, []RuntimeTarget{{Kind: RuntimeTargetKindThread, Thread: "worker-200 (200)", Source: "user_explicit"}}, fullPath[:2], true},
		{"named_diagnostic_conflict", &RuntimeTargetProfile{Declaration: RuntimeTargetDeclarationNamedTarget, SourceQuote: "requested thread"}, []RuntimeTarget{{Kind: RuntimeTargetKindThread, Thread: "worker-999 [200]", Source: "user_explicit"}}, fullPath, false},
		{"named_canonical_comm", &RuntimeTargetProfile{Declaration: RuntimeTargetDeclarationNamedTarget, SourceQuote: "requested thread"}, []RuntimeTarget{{Kind: RuntimeTargetKindThread, Thread: "WORKER", Source: "user_explicit"}}, fullPath[:2], true},
		{"named_bare_comm", &RuntimeTargetProfile{Declaration: RuntimeTargetDeclarationNamedTarget, SourceQuote: "requested thread"}, []RuntimeTarget{{Kind: RuntimeTargetKindThread, Thread: "worker", Source: "user_explicit"}}, fullPath[:2], true},
		{"named_process", &RuntimeTargetProfile{Declaration: RuntimeTargetDeclarationNamedTarget, SourceQuote: "requested process"}, []RuntimeTarget{{Kind: RuntimeTargetKindProcess, PID: 100, Source: "user_explicit"}}, fullPath, true},
		{"cursor", &RuntimeTargetProfile{Declaration: RuntimeTargetDeclarationNamedTarget, SourceQuote: "requested thread"}, []RuntimeTarget{{Kind: RuntimeTargetKindThread, PID: 200, Source: RuntimeTargetSourceExplicitToolCall}}, fullPath, false},
		{"artifact_target", &RuntimeTargetProfile{Declaration: RuntimeTargetDeclarationNamedTarget, SourceQuote: "requested thread"}, []RuntimeTarget{{Kind: RuntimeTargetKindThread, PID: 200, Source: "artifact_metadata"}}, fullPath, false},
		{"invalid_kind", &RuntimeTargetProfile{Declaration: RuntimeTargetDeclarationNamedTarget, SourceQuote: "requested thread"}, []RuntimeTarget{{Kind: "invalid", PID: 200, Source: "user_explicit"}}, fullPath, false},
		{"invalid_pid", &RuntimeTargetProfile{Declaration: RuntimeTargetDeclarationNamedTarget, SourceQuote: "requested thread"}, []RuntimeTarget{{Kind: RuntimeTargetKindThread, PID: RuntimeTargetMaxPID + 1, Thread: "worker", Source: "user_explicit"}}, fullPath, false},
		{"process_without_pid", &RuntimeTargetProfile{Declaration: RuntimeTargetDeclarationNamedTarget, SourceQuote: "requested process"}, []RuntimeTarget{{Kind: RuntimeTargetKindProcess, Thread: "worker", Source: "user_explicit"}}, fullPath, false},
		// These contradictory rows cannot be produced by a fresh successful
		// emit_analysis. They test defensive consumption of persisted inputs.
		{"persisted_no_named_user_row", &RuntimeTargetProfile{Declaration: RuntimeTargetDeclarationNoNamedTarget}, []RuntimeTarget{{Kind: RuntimeTargetKindThread, PID: 200, Source: "user_explicit"}}, fullPath, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rm := &RequestModel{RuntimeTargetProfile: tc.profile, RuntimeTargets: tc.targets}
			rm.AnalyzerHints.Entities = []string{"worker", "app", "200", "pid=200"}
			rm.AnalyzerHints.ExactTargets = []string{"worker-200"}
			before, _ := json.Marshal(rm)
			ledger := CompileObservationLedger(ObservationLedgerInput{
				RequestModel: rm,
				ToolResults: []ToolResult{{ToolName: "trace_query", Success: true, Observations: []ObservationRecord{
					anchorB1PathRecord("full", "app-100", "irq-80 -> worker-200 -> app-100"),
					anchorB1FrameRecord("frame", "worker-200", "explicit_query_target"),
				}}},
			})
			projection := CompileTraceCausalProjection(ledger)
			if !reflect.DeepEqual(projection.WakeupPath, tc.want) || projection.WakeupPathUserElected != tc.elected {
				t.Fatalf("profile owns user election: path=%v elected=%v; want %v/%v", projection.WakeupPath, projection.WakeupPathUserElected, tc.want, tc.elected)
			}
			if tc.profile != nil {
				for _, entity := range ledger.AnchorUserEntities {
					if !entity.TypedLane {
						t.Fatalf("present profile acquired a generic entity: %+v", entity)
					}
				}
			}
			after, _ := json.Marshal(rm)
			if string(before) != string(after) {
				t.Fatal("authority selection mutated the request's targets or exploration hints")
			}
		})
	}
}

func TestB1790NamedAnchorDoesNotFallBackAcrossGenericEntityOrder(t *testing.T) {
	for _, entities := range [][]string{{"worker", "app"}, {"app", "worker"}, {"pid=200"}, {"worker-200"}, {"200"}} {
		rm := &RequestModel{
			RuntimeTargetProfile: &RuntimeTargetProfile{Declaration: RuntimeTargetDeclarationNamedTarget, SourceQuote: "thread 900"},
			RuntimeTargets:       []RuntimeTarget{{Kind: RuntimeTargetKindThread, PID: 900, Source: "user_explicit"}},
		}
		rm.AnalyzerHints.Entities = entities
		rm.AnalyzerHints.ExactTargets = entities
		ledger := CompileObservationLedger(ObservationLedgerInput{RequestModel: rm})
		want := []AnchorUserEntity{{Value: "900", TypedLane: true}}
		if !reflect.DeepEqual(ledger.AnchorUserEntities, want) {
			t.Fatalf("generic order changed authorized identities: hints=%v got=%+v", entities, ledger.AnchorUserEntities)
		}
	}
}
