package skill

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestWriteAnalysisSkillExplainsExactObservedBaselineProtection(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("write-analysis-skill")
	if err != nil {
		t.Fatal(err)
	}
	corpus := allWorkflowBodies(sk) + "\n" + sk.OutputFormat
	if !strings.Contains(corpus, types.WriteProtectedBaselineTargetTeaching) {
		t.Fatal("schema/skill baseline rule must share one source")
	}
	for _, want := range []string{"current repository during this dispatch", "not test classification or execution proof", "literal filename"} {
		if !strings.Contains(corpus, want) {
			t.Fatalf("missing %q: %s", want, corpus)
		}
	}
}
