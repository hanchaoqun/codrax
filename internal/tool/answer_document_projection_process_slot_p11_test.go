package tool

// answer_document_projection_process_slot_p11_test.go — CR-3 件③ P11
// display pins (2026-07-12; 冷读案8 关键角色裸线程名无 tgid): the detail
// stanza's identity heading (行1) stays untouched, and the first
// subordinate line under it carries 「进程: tgid=G comm=P」 from the typed
// pair; nodes without the pair render no process line (absence never
// guesses, no fabricated identity).

import (
	"strings"
	"testing"
)

func TestDetailProcessSlotRendersUnderIdentityHeading(t *testing.T) {
	_, model := dstateRefineModel(t, "d_state_or_io_wait",
		"tgid=17267", "process_comm=aweme.lite")
	stanza := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(stanza, "- 进程: tgid=17267 comm=aweme.lite") {
		t.Fatalf("the process slot must render as the first subordinate line:\n%s", stanza)
	}
	// 行1 (the stanza heading) keeps its exact identity shape — the process
	// slot never rides the heading (行1 不动防超载).
	for _, line := range strings.Split(stanza, "\n") {
		if strings.HasPrefix(line, "**[") && strings.Contains(line, "tgid=") {
			t.Fatalf("行1 must stay untouched by the process slot: %q", line)
		}
	}
	// EN face carries the same identity tokens.
	stanzaEN := runtimeTraceProjDetailFullText(model, false)
	if !strings.Contains(stanzaEN, "- process: tgid=17267 comm=aweme.lite") {
		t.Fatalf("EN detail face must carry the process slot:\n%s", stanzaEN)
	}
}

func TestDetailProcessSlotAbsentWithoutTypedPair(t *testing.T) {
	_, model := dstateRefineModel(t, "d_state_or_io_wait")
	stanza := runtimeTraceProjDetailFullText(model, true)
	if strings.Contains(stanza, "进程: tgid=") {
		t.Fatalf("no typed pair → no process line (never fabricate):\n%s", stanza)
	}
}

func TestDetailProcessSlotTGIDOnlyWhenCommUnresolved(t *testing.T) {
	_, model := dstateRefineModel(t, "d_state_or_io_wait", "tgid=17267")
	stanza := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(stanza, "- 进程: tgid=17267") || strings.Contains(stanza, "comm=") {
		t.Fatalf("unresolved comm renders tgid only:\n%s", stanza)
	}
}
