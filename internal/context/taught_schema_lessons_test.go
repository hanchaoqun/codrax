package context

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// EMITBURN-1 件B (§29.174 RUN2AUDIT-1): a schema lesson one explore window
// already paid for rides every later explore dispatch prompt verbatim, so a
// fresh window does not re-learn the contract through fresh emit rejects.
func TestBuildPromptContext_TaughtSchemaLessonReplaysIntoLaterExploreDispatch(t *testing.T) {
	lesson := "Previous attempt gathered an exhaustive principal-member enumeration but did not close through a model-authored aggregate_facts.member_set. Reuse the already-read evidence, call emit_investigation_complete(result_kind=\"resolved\"), and include aggregate_facts with kind=\"member_set\", value=len(members), and every principal answer member in members[]. Do not leave the complete set only in thinking, read_file output, or closure prose."
	mut := types.NewMutableState("q")
	mut.RecordTaughtSchemaLesson("explorer.retry.structured-member-set", lesson)

	render := func(ac *types.AgentContext) string {
		pc := BuildPromptContext(ac, &skill.Config{Name: "explore-skill"})
		body := ""
		for _, m := range ToMessages(pc) {
			if m.Role == "user" {
				body += m.Content + "\n"
			}
		}
		return body
	}

	// A later explore window (fresh dispatch, no retry hint) sees the lesson.
	body := render(&types.AgentContext{
		AgentName: types.AgentExplorer,
		Stage:     types.StageExplore,
		Objective: "q",
		Mutable:   mut,
	})
	if !strings.Contains(body, SectionTaughtSchemaLessons) {
		t.Fatalf("later explore dispatch must carry the taught-lesson section, got:\n%s", body)
	}
	if !strings.Contains(body, lesson) {
		t.Fatalf("lesson text must be replayed verbatim (typed transfer, no regeneration), got:\n%s", body)
	}

	// The resume dispatch whose RetryHint IS the lesson is not double-taught.
	body = render(&types.AgentContext{
		AgentName: types.AgentExplorer,
		Stage:     types.StageExplore,
		Objective: "q",
		Mutable:   mut,
		RetryHint: lesson,
	})
	if strings.Contains(body, SectionTaughtSchemaLessons) {
		t.Fatalf("resume dispatch with the identical retry hint must not duplicate the lesson section, got:\n%s", body)
	}

	// Non-explore stages stay untouched.
	body = render(&types.AgentContext{
		AgentName: types.AgentFinalizer,
		Stage:     types.StageFinalize,
		Objective: "q",
		Mutable:   mut,
	})
	if strings.Contains(body, SectionTaughtSchemaLessons) {
		t.Fatalf("non-explore dispatch must not carry the explore lesson section, got:\n%s", body)
	}
}
