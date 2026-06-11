package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestApplyCommitMessage_PlanSummarySubjectWithTrailer(t *testing.T) {
	plan := &types.ChangePlan{ID: "plan-x1", Summary: "修复优先级错误,保留负数错误码\n详细解释第二行不该进 subject"}
	msg := applyCommitMessage(plan)
	lines := strings.Split(msg, "\n")
	if lines[0] != "修复优先级错误,保留负数错误码" {
		t.Fatalf("subject must be the summary first line, got %q", lines[0])
	}
	if lines[len(lines)-1] != "plan: plan-x1" {
		t.Fatalf("plan trailer required, got %q", msg)
	}
	long := &types.ChangePlan{ID: "plan-x2", Summary: strings.Repeat("长", 100)}
	if subject := strings.Split(applyCommitMessage(long), "\n")[0]; len([]rune(subject)) > 72 {
		t.Fatalf("subject must be bounded at 72 runes, got %d", len([]rune(subject)))
	}
	empty := &types.ChangePlan{ID: "plan-x3"}
	if got := applyCommitMessage(empty); got != "codrax apply iter (plan=plan-x3)" {
		t.Fatalf("empty summary falls back to legacy form, got %q", got)
	}
}
