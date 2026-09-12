package skill

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1665ChangePlanSkillPythonBlockEditingRespectsScope(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("change-plan-skill")
	if err != nil {
		t.Fatal(err)
	}
	var paragraph string
	for _, line := range strings.Split(allWorkflowBodies(sk), "\n") {
		if strings.HasPrefix(line, "STRUCTURED EDITS (kind=patch)") {
			paragraph = line
		}
	}
	if paragraph == "" {
		t.Fatal("real registered skill lost structured editing guidance")
	}
	if !strings.Contains(paragraph, types.StructuredEditPythonScopeTeaching) {
		t.Errorf("registered skill does not carry the shared Python teaching: %s", paragraph)
	}
	for _, want := range []string{"line-range replace", "start_line", "end_line", "scope=micro", "explicitly package, cross, or project", "rewrites most of the existing file"} {
		if !strings.Contains(paragraph, want) {
			t.Errorf("Python block paragraph omits scope-safe route %q: %s", want, paragraph)
		}
	}
	if strings.Contains(paragraph, "or kind=modify with the full corrected file body") {
		t.Errorf("indented block still recommends unconditional full overwrite: %s", paragraph)
	}
}
