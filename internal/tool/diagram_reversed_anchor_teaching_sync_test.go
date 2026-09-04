package tool

import (
	"os"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
)

// diagram_reversed_anchor_teaching_sync_test.go — V10-4 R2' teaching-sync
// pin (colleague_merge_audit §40.57, pattern of §40.47 ⑤). The typed issue
// value typed_anchor_reversed_against_visible_edge reaches the model on two
// surfaces: the pre-emit relation-gate boundary sentence (this package) and
// the finalizer's stage-order recipe teaching (internal/agent). Both must name
// the same token and the same remedy (swap the anchor endpoints or reverse
// the arrow; never delete the diagram), and neither may carry an internal
// identifier from the glossary blocklist.
func TestDiagramReversedAnchorTeachingSync(t *testing.T) {
	const token = "typed_anchor_reversed_against_visible_edge"
	if diagramCallEdgeIssueAnchorReversedAgainstVisibleEdge != token {
		t.Fatalf("issue constant drifted from the taught token: %q", diagramCallEdgeIssueAnchorReversedAgainstVisibleEdge)
	}
	agentSrc, err := os.ReadFile("../agent/answer_document_evaluator.go")
	if err != nil {
		t.Fatal(err)
	}
	var agentSentence string
	for _, line := range strings.Split(string(agentSrc), "\n") {
		if strings.Contains(line, "`"+token+"`") {
			agentSentence = line
			break
		}
	}
	if agentSentence == "" {
		t.Fatalf("finalizer recipe teaching in internal/agent does not mention %s", token)
	}
	surfaces := map[string]string{
		"pre-emit boundary":  diagramReversedAnchorBoundaryTeaching,
		"finalizer teaching": agentSentence,
	}
	for name, text := range surfaces {
		if !strings.Contains(text, token) {
			t.Fatalf("%s must name the typed issue token %s: %q", name, token, text)
		}
		for _, remedy := range []string{"from_node/to_node", "from_identity/to_identity", "revers", "delete the diagram"} {
			if !strings.Contains(text, remedy) {
				t.Fatalf("%s must teach the alignment remedy (%q): %q", name, remedy, text)
			}
		}
		for _, term := range skill.InternalTermsBlocklist {
			if strings.Contains(text, term) {
				t.Fatalf("%s leaks internal identifier %q: %q", name, term, text)
			}
		}
		for _, leak := range []string{".go", "answer_block_normalize", "NormalizeEmitAnswerBlock", "B698", "§"} {
			if strings.Contains(text, leak) {
				t.Fatalf("%s leaks an internal reference %q: %q", name, leak, text)
			}
		}
	}
}
