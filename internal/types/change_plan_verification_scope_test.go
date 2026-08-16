package types

import (
	"reflect"
	"testing"
)

func TestChangePlanVerificationScopeKeepsApplyAndVerifyScopesSeparate(t *testing.T) {
	plan := &ChangePlan{
		ID:                 "plan-new",
		TargetPaths:        []string{"new.go"},
		BehaviorContracts:  []WriteBehaviorContract{{ID: "contract-new"}},
		VerificationProbes: []VerificationProbe{{ID: "probe-new"}},
		CumulativeVerificationScope: &CumulativeVerificationScope{
			SourcePlanIDs:      []string{"plan-old"},
			TargetPaths:        []string{"old.java"},
			BehaviorContracts:  []WriteBehaviorContract{{ID: "contract-old"}},
			VerificationProbes: []VerificationProbe{{ID: "probe-old"}},
		},
	}

	applyPaths, _ := ActiveChangePlanApplyTargetPaths(plan, nil)
	if !reflect.DeepEqual(applyPaths, []string{"new.go"}) {
		t.Fatalf("apply paths widened by verification scope: %+v", applyPaths)
	}
	if got := ChangePlanVerificationTargetPaths(plan, nil); !reflect.DeepEqual(got, []string{"new.go", "old.java"}) {
		t.Fatalf("verification paths = %+v", got)
	}
	if got := ChangePlanVerificationBehaviorContracts(plan); len(got) != 2 || got[0].ID != "contract-new" || got[1].ID != "contract-old" {
		t.Fatalf("verification contracts = %+v", got)
	}
	if got := ChangePlanVerificationProbes(plan); len(got) != 2 || got[0].ID != "probe-new" || got[1].ID != "probe-old" {
		t.Fatalf("verification probes = %+v", got)
	}
}

func TestChangePlanVerificationBehaviorContractsDoesNotResurrectSupersededCumulativeContract(t *testing.T) {
	plan := &ChangePlan{
		BehaviorContracts:             []WriteBehaviorContract{{ID: "current"}},
		SupersededBehaviorContractIDs: []string{"stale", "stale"},
		CumulativeVerificationScope: &CumulativeVerificationScope{
			BehaviorContracts: []WriteBehaviorContract{{ID: "stale"}, {ID: "retained-old"}},
		},
	}

	got := ChangePlanVerificationBehaviorContracts(plan)
	if len(got) != 2 || got[0].ID != "current" || got[1].ID != "retained-old" {
		t.Fatalf("verification contracts resurrected a superseded id: %+v", got)
	}
}
