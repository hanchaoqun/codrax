package types

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/threadidentity"
)

func TestRuntimeThreadIdentityHandoffUsesQueryGrammar(t *testing.T) {
	if RuntimeTargetMaxPID != threadidentity.MaxPID {
		t.Fatal("query/evidence PID domain drift")
	}
	for _, spelling := range []string{"ui-10", "ui (tid=10)", "ui-10 (tid=10)", "ui [10]", "ui 10", "tid=10"} {
		rm := &RequestModel{RuntimeTargets: []RuntimeTarget{{Kind: RuntimeTargetKindThread, Thread: spelling, Source: "user_explicit"}}}
		if !ObservationRecordMatchesUserRuntimeTarget(ObservationRecord{Subject: "ui-10"}, rm) {
			t.Errorf("lost typed identity %q", spelling)
		}
		for _, subject := range []string{"ui-11", "worker-110", "process 10"} {
			if ObservationRecordMatchesUserRuntimeTarget(ObservationRecord{Subject: subject}, rm) {
				t.Errorf("identity %q admitted %q", spelling, subject)
			}
		}
	}
	for _, target := range []RuntimeTarget{
		{Thread: "ui-11 (tid=10)", Source: "user_explicit"},
		{Thread: "ui (tid=11)", PID: 10, Source: "user_explicit"},
		{Thread: "ui (tid=10)", Source: RuntimeTargetSourceExplicitToolCall},
	} {
		if ObservationRecordMatchesUserRuntimeTarget(ObservationRecord{Subject: "ui-10"}, &RequestModel{RuntimeTargets: []RuntimeTarget{target}}) {
			t.Errorf("invalid/cursor target matched: %+v", target)
		}
	}
	rm := &RequestModel{RuntimeTargets: []RuntimeTarget{{Kind: RuntimeTargetKindThread, PID: 10, Thread: "ui", Source: "user_explicit"}}}
	if !ObservationRecordMatchesUserRuntimeTarget(ObservationRecord{Subject: "10"}, rm) {
		t.Fatal("typed numeric query subject lost")
	}
	if ObservationRecordMatchesUserRuntimeTarget(ObservationRecord{Subject: "ui-11"}, rm) {
		t.Fatal("shared comm overrode explicit TID")
	}
	if traceCausalProjectionAnchorLabelMatchesEntity("ui-10", traceCausalProjectionAnchorEntity{value: "ui (tid=10)"}) {
		t.Fatal("typed-only grammar leaked into prose authority")
	}
}
