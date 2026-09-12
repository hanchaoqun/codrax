package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1575ActualProbeSchemasDiscloseExecutionOnlyAuthority(t *testing.T) {
	for name, raw := range map[string]json.RawMessage{
		"emit_change_plan":   (&EmitChangePlan{}).Parameters(),
		"emit_plan_skeleton": (&EmitPlanSkeleton{}).Parameters(),
		"run_tests":          (&RunTests{}).Parameters(),
	} {
		t.Run(name, func(t *testing.T) {
			var schema map[string]any
			if err := json.Unmarshal(raw, &schema); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), types.PythonPlainProbeAuthorityTeaching) {
				t.Error("public schema must use the shared authority boundary verbatim")
			}
			for _, want := range []string{"PYTHON PLAIN-PROBE AUTHORITY", "target_execution, not target_behavior", "contract_refs and placement_refs declare intended scope", "existing native project assertion", "unverified or blocked"} {
				if !strings.Contains(string(raw), want) {
					t.Errorf("public schema lacks %q", want)
				}
			}
			for _, want := range []string{"omit verification_probes", "acceptance_tests", "do not launch an external compiler or test runner"} {
				if !strings.Contains(string(raw), want) {
					t.Errorf("public schema lost optional native route %q", want)
				}
			}
			if strings.Contains(string(raw), "__VERIFICATION_PROBE_") {
				t.Error("unresolved authoring placeholder")
			}
		})
	}
}
