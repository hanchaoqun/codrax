package skill

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1575PlannerSkillSeparatesPythonExecutionFromAssertionProof(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("change-plan-skill")
	if err != nil {
		t.Fatal(err)
	}
	body := allWorkflowBodies(sk)
	if strings.Count(body, types.PythonPlainProbeAuthorityTeaching) != 1 {
		t.Error("planner must receive the shared plain-probe boundary exactly once")
	}
	for _, want := range []string{
		"PYTHON PLAIN-PROBE AUTHORITY",
		"target_execution, not target_behavior",
		"contract_refs and placement_refs declare intended scope",
		"executing an unchanged method",
		"existing native project assertion",
		"file_layout",
		"unverified or blocked",
		"Do not repeatedly rerun a plain probe",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("actual planner skill lacks %q", want)
		}
	}
	if strings.Contains(body, "A passing probe is bounded local behaviour evidence.") {
		t.Error("planner still grants unconditional behavior evidence to a passing plain probe")
	}
	// Existing optional native-runner escape and model-owned assertions remain.
	for _, want := range []string{"verification_probes[] are optional", "project_test_observations[]", "exact test_path, assertion_suite, assertion_id, and contract_refs", "same-package `TestX(*testing.T)`", "A failing probe is an exact execution observation"} {
		if !strings.Contains(body, want) {
			t.Errorf("planner lost existing positive route %q", want)
		}
	}
}
