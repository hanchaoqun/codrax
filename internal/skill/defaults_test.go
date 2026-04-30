package skill

import (
	"strings"
	"testing"
)

func TestExploreSkillOutputFormatStaysToolFirst(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)

	sk, err := r.Get("explore-skill")
	if err != nil {
		t.Fatalf("Get(explore-skill) returned error: %v", err)
	}
	if strings.Contains(sk.OutputFormat, "\nAnswer:") || strings.Contains(sk.OutputFormat, "\nEvidence:\n") {
		t.Fatalf("explore-skill OutputFormat must not teach answer-shaped labels:\n%s", sk.OutputFormat)
	}
	if !strings.Contains(sk.OutputFormat, "emit_evidence") {
		t.Fatalf("explore-skill OutputFormat must mention emit_evidence:\n%s", sk.OutputFormat)
	}
	if !strings.Contains(sk.OutputFormat, "emit_investigation_complete") {
		t.Fatalf("explore-skill OutputFormat must mention emit_investigation_complete:\n%s", sk.OutputFormat)
	}
}

func TestFinalizerSkillStepListPrefersDiagramsWhenHelpful(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)

	sk, err := r.Get("answer-document-skill")
	if err != nil {
		t.Fatalf("Get(answer-document-skill) returned error: %v", err)
	}
	for _, want := range []string{
		"Even when the Diagram Contract does NOT require one",
		"3+ hops",
		"actor/role handoffs",
		"easier to see than to read in prose",
	} {
		if !strings.Contains(sk.OutputFormat, want) {
			t.Fatalf("finalize-skill OutputFormat missing %q:\n%s", want, sk.OutputFormat)
		}
	}
}

func TestFinalizerSkillKeepsInternalJargonOutOfUserProse(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)

	sk, err := r.Get("answer-document-skill")
	if err != nil {
		t.Fatalf("Get(answer-document-skill) returned error: %v", err)
	}
	for _, want := range []string{
		"Keep internal pipeline jargon out of the user-facing prose",
		"\"grounded\"",
		"'grep' / 'read_file' / 'repo_map' found nothing.",
	} {
		if !strings.Contains(sk.OutputFormat, want) {
			t.Fatalf("answer-document-skill OutputFormat missing %q:\n%s", want, sk.OutputFormat)
		}
	}
}

// TestChangePlanSkill_PhaseAInvestigateWorkflow verifies Module A's
// "investigate before emit" guidance is in the planner's skill
// workflow. Pure description-of-method (PHASE A — INVESTIGATE), no
// if-then prescriptions.
func TestChangePlanSkill_PhaseAInvestigateWorkflow(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)

	sk, err := r.Get("change-plan-skill")
	if err != nil {
		t.Fatalf("Get(change-plan-skill): %v", err)
	}
	wf := strings.Join(sk.Workflow, "\n")
	if !strings.Contains(wf, "PHASE A") || !strings.Contains(wf, "INVESTIGATE") {
		t.Fatalf("change-plan-skill workflow must mention 'PHASE A — INVESTIGATE'; got:\n%s", wf)
	}
	// Tool names the planner is expected to chain — proxies for "the
	// model is told to use the read-only investigation toolbox".
	for _, want := range []string{"repo_map", "grep", "read_file"} {
		if !strings.Contains(wf, want) {
			t.Errorf("PHASE A workflow should reference tool %q; got:\n%s", want, wf)
		}
	}
}

// TestChangePlanSkill_DebugWorkflowOnRetry verifies Module F's
// description of how to read failure data on retry — not as
// prescription ("if X then Y") but as method ("read X, decide which
// side is wrong").
func TestChangePlanSkill_DebugWorkflowOnRetry(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)

	sk, err := r.Get("change-plan-skill")
	if err != nil {
		t.Fatalf("Get(change-plan-skill): %v", err)
	}
	wf := strings.Join(sk.Workflow, "\n")
	if !strings.Contains(wf, "DEBUG WORKFLOW") {
		t.Fatalf("workflow must include DEBUG WORKFLOW guidance; got:\n%s", wf)
	}
	// The three-tactic decision frame is the load-bearing part of
	// this guidance — every word matters because the model uses it
	// to classify the failure on its own.
	for _, want := range []string{
		"the failing test",
		"the production code",
	} {
		if !strings.Contains(wf, want) {
			t.Errorf("DEBUG WORKFLOW should mention %q; got:\n%s", want, wf)
		}
	}
	// Forbidden — must NOT contain prescribed-cause prose like
	// "Most common causes" or "do X don't do Y".
	for _, banned := range []string{"Most common cause", "DO NOT raise the cap"} {
		if strings.Contains(wf, banned) {
			t.Errorf("DEBUG WORKFLOW must not contain prescriptive prose %q", banned)
		}
	}
}
