package safety

import "testing"

func TestFoldPermissionDecisionsDenyWins(t *testing.T) {
	got := FoldPermissionDecisions("test",
		AllowPermission("test", "read_only", ""),
		AskPermission("test", "needs_review", ""),
		DenyPermission("test", "dangerous", ""))
	if got.Action != PermissionDeny || got.ReasonCode != "dangerous" {
		t.Fatalf("deny should win, got %+v", got)
	}
}

func TestFoldPermissionDecisionsAskWinsOverAllow(t *testing.T) {
	got := FoldPermissionDecisions("test",
		AllowPermission("test", "read_only", ""),
		AskPermission("test", "needs_review", ""))
	if got.Action != PermissionAsk || got.ReasonCode != "needs_review" {
		t.Fatalf("ask should win over allow, got %+v", got)
	}
}
