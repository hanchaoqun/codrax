package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestVerifierNoTestsTeachingKeepsInvocationScopeAndToolPolicy(t *testing.T) {
	registry := skill.NewRegistry()
	skill.RegisterDefaults(registry)
	sk, err := registry.Get("test-execute-skill")
	if err != nil {
		t.Fatal(err)
	}
	evaluator := &verifierEvaluator{}
	instruction := evaluator.BuildInitialInstruction(verifierFixtureCtx(nil, &types.ChangePlan{ID: "scope-teaching"}), sk)
	for name, text := range map[string]string{"instruction": instruction, "skill": strings.Join(sk.Workflow, "\n")} {
		for _, phrase := range []string{"individual invocations", "recorded working directories", "same runner in another directory", "verdict", "authoritative", "exec_command"} {
			if !strings.Contains(text, phrase) {
				t.Errorf("%s lacks %q", name, phrase)
			}
		}
		for _, old := range []string{"that means the selected runner found no direct test work", "that runner ran cleanly but discovered zero test cases"} {
			if strings.Contains(text, old) {
				t.Errorf("%s retains runner-wide no-tests claim", name)
			}
		}
	}
}
