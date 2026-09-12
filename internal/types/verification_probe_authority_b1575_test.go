package types

import (
	"reflect"
	"strings"
	"testing"
)

func TestB1575ProbeTeachingDoesNotTurnExecutionIntoAssertionWitness(t *testing.T) {
	for _, want := range []string{
		"executor-owned observations bind actual execution to the changed implementation",
		"target_execution, not target_behavior",
		"contract_refs and placement_refs declare intended scope",
		"not assertion-level or placement proof",
		"existing native project assertion",
		"matching passed assertion",
		"Genuine file_layout contracts",
		"do not reclassify runtime behavior as source shape",
		"unverified or blocked",
		"A passing probe alone does not authorize no_change_required",
	} {
		if !strings.Contains(PythonPlainProbeAuthorityTeaching, want) {
			t.Errorf("missing precise authority/recovery boundary %q", want)
		}
	}
	// Teaching limits a producer's receipt, not the existing witness matrix.
	// The matrix still admits actual compatible probe/native-test witnesses;
	// only file_layout has the independent post-apply source-reading route.
	for _, kind := range AllWriteBehaviorContractKinds() {
		want := []WriteBehaviorWitnessKind{WriteBehaviorWitnessVerificationProbe, WriteBehaviorWitnessProjectTest}
		if kind == WriteBehaviorFileLayout {
			want = append(want, WriteBehaviorWitnessSourceText)
		}
		if got := WriteBehaviorContractWitnessKinds(kind); !reflect.DeepEqual(got, want) {
			t.Errorf("teaching changed witness matrix for %s: %v", kind, got)
		}
	}
	if !strings.Contains(WriteBehaviorContractKindWitnessTeaching(), "Execution alone does not prove a contract") {
		t.Error("analyzer kind teaching still treats any execution as a contract witness")
	}
	for name, teaching := range map[string]string{
		"observation":     WriteBehaviorContractObservationTeaching,
		"native recovery": NativeProjectTestObservationRecoveryTeaching,
	} {
		if !strings.Contains(teaching, "Python plain probe") || !strings.Contains(teaching, "assertion") {
			t.Errorf("%s teaching must preserve the plain-probe receipt limit", name)
		}
	}
}
