package sourceinventory

import (
	"context"
	"testing"
)

func TestBudgetBoundsBroadSourceInventory(t *testing.T) {
	budget := NewBudget(BudgetOptions{
		TopN:              50,
		RepoFileCount:     10000,
		GraphFileCount:    10000,
		ForceAdvisoryOnly: true,
	})
	if budget.MaxPerRole() != 200 {
		t.Fatalf("max per role = %d, want 200", budget.MaxPerRole())
	}
	if budget.MaxScanPerRole() == 0 || budget.MaxQueryScanPerRole() <= budget.MaxScanPerRole() {
		t.Fatalf("scan/query scan limits not widened: scan=%d query=%d", budget.MaxScanPerRole(), budget.MaxQueryScanPerRole())
	}
	if !budget.MaterializationExceeded(200) {
		t.Fatal("materialization should be exceeded at the role cap")
	}
}

func TestBudgetInactiveForSmallOrNonAdvisoryLens(t *testing.T) {
	budget := NewBudget(BudgetOptions{
		TopN:              50,
		RepoFileCount:     100,
		GraphFileCount:    100,
		ForceAdvisoryOnly: true,
	})
	if budget.MaterializationEnabled() || budget.ScanEnabled() {
		t.Fatalf("small repo budget should be inactive: %+v", budget)
	}
	budget = NewBudget(BudgetOptions{
		TopN:              50,
		RepoFileCount:     10000,
		GraphFileCount:    10000,
		ForceAdvisoryOnly: false,
	})
	if budget.MaterializationEnabled() || budget.ScanEnabled() {
		t.Fatalf("non-advisory budget should be inactive: %+v", budget)
	}
}

func TestCursorOffsetAndPage(t *testing.T) {
	if got := CursorOffset(7, "100"); got != 7 {
		t.Fatalf("explicit offset should win, got %d", got)
	}
	if got := CursorOffset(0, "12"); got != 12 {
		t.Fatalf("cursor offset = %d, want 12", got)
	}
	if got := CursorOffset(0, "-1"); got != 0 {
		t.Fatalf("negative cursor should normalize to 0, got %d", got)
	}
	page, ok := Page(12, 5, 20, false)
	if !ok {
		t.Fatal("expected page")
	}
	if page.Offset != 12 || page.Limit != 5 || page.Emitted != 5 || page.NextCursor != "17" || page.Complete {
		t.Fatalf("page mismatch: %+v", page)
	}
	page, ok = Page(18, 5, 20, true)
	if !ok {
		t.Fatal("expected page")
	}
	if page.NextCursor != "" || page.Complete {
		t.Fatalf("budget-truncated terminal page should not be complete: %+v", page)
	}
}

func TestBudgetInterruptedByContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	budget := NewBudget(BudgetOptions{
		Context:           ctx,
		TopN:              50,
		RepoFileCount:     10000,
		GraphFileCount:    10000,
		ForceAdvisoryOnly: true,
	})
	if !budget.Interrupted() || !budget.ScanExceeded(0) {
		t.Fatalf("canceled budget should be interrupted")
	}
}
