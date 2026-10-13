package types

import (
	"strings"
	"testing"
)

func TestContextPack234IdentityScopeAndTextBudget(t *testing.T) {
	batch, slice := strings.Repeat("batch-", 60)+"end", strings.Repeat("slice-", 60)+"end"
	prefix := strings.Repeat("identity-", 40)
	pack := WriteContextPack{PackID: prefix + "pack", BatchID: batch, SourceStage: prefix + "stage"}
	for _, suffix := range []string{"one", "two"} {
		pack.Items = append(pack.Items, WriteContextItem{ID: prefix + suffix, BatchID: batch, SliceID: slice, SourceID: prefix + suffix, SourceStage: prefix + "stage", Priority: WriteContextP1, Text: strings.Repeat("text ", 80)})
	}
	normalized := NormalizeWriteContextPack(pack)
	if normalized.PackID != pack.PackID || normalized.BatchID != batch || normalized.SourceStage != pack.SourceStage || len(normalized.Items) != 2 {
		t.Fatal("identity was shortened or independent items merged")
	}
	for i, item := range normalized.Items {
		if item.ID != pack.Items[i].ID || item.SourceID != pack.Items[i].SourceID || item.SliceID != slice || item.ExpiresWithBatchID != batch || item.ExpiresWithSliceID != slice {
			t.Fatal("identity/scope/expiry was truncated")
		}
		if len([]rune(item.Text)) > writeContextPackTextLen+3 || !strings.HasSuffix(item.Text, "...") {
			t.Fatal("advisory text budget changed")
		}
	}
	if got := normalized.ViewForScope(WriteConsumerPlanner, 10, batch, slice); len(got.Items) != 2 {
		t.Fatal("long exact scope lost visible items")
	}
	if got := normalized.ViewForScope(WriteConsumerPlanner, 10, batch+"other", slice); len(got.Items) != 0 {
		t.Fatal("unrelated scope gained items")
	}
	changed := normalized.WithScope(batch+"new", slice+"new")
	if changed.BatchID != batch+"new" || changed.Items[0].ExpiresWithBatchID != batch+"new" {
		t.Fatal("WithScope truncated identities")
	}
	merged := MergeWriteContextPacks(batch, "goal", normalized)
	if merged.BatchID != batch || len(merged.ForScope(batch, slice).Items) != 2 {
		t.Fatal("Merge/ForScope lost long scope")
	}
}
