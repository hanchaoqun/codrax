package orchestrator

import (
	"os"
	"strings"
	"testing"
)

func TestReviewerRuntimeUsesCitationAwareAnchorSet(t *testing.T) {
	raw, err := os.ReadFile("contract_check.go")
	if err != nil {
		t.Fatalf("read contract_check.go: %v", err)
	}
	src := string(raw)

	if strings.Contains(src, "BuildEvidenceAnchorSet(mut.EmittedEvidence())") {
		t.Fatalf("reviewer runtime must not use explorer-only emitted evidence anchors; use buildReviewerEvidenceAnchorSet so final AnswerDocument citations participate")
	}

	for _, marker := range []string{
		"func (o *Orchestrator) runSelfConsistencyReviewV2",
		"func (o *Orchestrator) runSemanticQualityReviewWithOutcome",
	} {
		body := reviewerAnchorSourceSection(t, src, marker)
		if !strings.Contains(body, "buildReviewerEvidenceAnchorSet(mut, doc, o.busCtx)") {
			t.Fatalf("%s must build reviewer anchors through buildReviewerEvidenceAnchorSet(mut, doc, o.busCtx)", marker)
		}
	}
}

func reviewerAnchorSourceSection(t *testing.T, src, marker string) string {
	t.Helper()
	start := strings.Index(src, marker)
	if start < 0 {
		t.Fatalf("source section %q not found", marker)
	}
	end := strings.Index(src[start+len(marker):], "\nfunc ")
	if end < 0 {
		return src[start:]
	}
	return src[start : start+len(marker)+end]
}
