package skill

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// write_analysis_witness_teaching_test.go — V5-1 (§40.10, R2' same source):
// the write-analysis skill carries the contract-kind → witness-kind sentence
// generated from the types-level matrix, verbatim, so the analyzer learns
// which kinds a source reading can witness from the table the verifier uses.
func TestWriteAnalysisSkillTeachesKindWitnessMatrixFromTheSameSource(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("write-analysis-skill")
	if err != nil {
		t.Fatalf("Get(write-analysis-skill) returned error: %v", err)
	}
	corpus := allWorkflowBodies(sk)
	teaching := types.WriteBehaviorContractKindWitnessTeaching()
	if !strings.Contains(corpus, teaching) {
		t.Fatalf("write-analysis skill must carry the matrix teaching verbatim:\n%s", teaching)
	}
	for _, leak := range []string{"source_contract_refs", "WitnessKind", "PatchEffect", "source_text_presence"} {
		if strings.Contains(corpus, leak) {
			t.Fatalf("write-analysis skill leaks internal name %q", leak)
		}
	}
}
