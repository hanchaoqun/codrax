package tracequery

import (
	"reflect"
	"strings"
	"testing"
)

func TestHMC307ExecutionCatalogAndAdmissionShareDeclarations(t *testing.T) {
	catalog, err := TraceCapabilities("", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Views) != len(CanonicalViewNames()) {
		t.Fatal("catalog universe drift")
	}
	for _, view := range catalog.Views {
		t.Run(view.View, func(t *testing.T) {
			c := ViewExecutionFor(view.View)
			if !reflect.DeepEqual(c, view.Execution) || c.Mode == "" || len(c.Entries) == 0 || c.CoordinateDomain == "" {
				t.Fatalf("missing/shared execution declaration: %+v", view)
			}
			for _, entry := range []ViewExecutionEntry{ExecutionEntryNativeTool, ExecutionEntryTraceDiag, ExecutionEntryIndexedRun, "unknown_entry"} {
				want := entry == ExecutionEntryNativeTool || entry == ExecutionEntryTraceDiag && c.Mode != ExecutionStreamingPacket || entry == ExecutionEntryIndexedRun && c.Mode == ExecutionIndexedTrace
				if got := ValidateViewExecution(view.View, entry) == nil; got != want {
					t.Fatalf("entry %q admitted=%v want=%v", entry, got, want)
				}
			}
			c.Entries[0] = "mutated"
			if err := ValidateViewExecution(view.View, ExecutionEntryNativeTool); err != nil {
				t.Fatalf("consumer mutated admission: %v", err)
			}
		})
	}
}

func TestHMC307UndeclaredExecutionFailsClosed(t *testing.T) {
	const future = "hmc307_undeclared_view"
	viewCapacityTable[future] = ViewCapacity{View: future}
	defer delete(viewCapacityTable, future)
	for _, entry := range []ViewExecutionEntry{ExecutionEntryNativeTool, ExecutionEntryTraceDiag, ExecutionEntryIndexedRun} {
		if err := ValidateViewExecution(future, entry); err == nil {
			t.Fatal("catalog registration granted implicit execution")
		}
		if err := ValidateViewExecution("unknown_view", entry); err == nil {
			t.Fatal("unknown view accepted")
		}
	}
	for _, alias := range []string{"", " frame_bundle ", "frame_rootcause_bundle", "frame_root_cause"} {
		if err := ValidateViewExecution(alias, ExecutionEntryIndexedRun); err != nil {
			t.Fatalf("existing engine alias changed: %v", err)
		}
	}
}

func TestHMC307PacketViewRejectedBeforeIndexedAccess(t *testing.T) {
	for _, view := range []string{ViewPacketFlows, ViewPacketInventory} {
		got := Run(nil, Query{View: view, TimeStart: 1, TimeEnd: 2})
		if got.View != view || len(got.Events) != 0 || len(got.EvidencePack) != 0 || !strings.Contains(strings.Join(got.Caveats, "\n"), "no indexed_run execution adapter") {
			t.Fatalf("unimplemented entry acquired results: %+v", got)
		}
	}
}
