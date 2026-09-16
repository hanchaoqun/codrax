package types

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestB1693WriteFinalMaterializationRoundTripAndDetachedNormalization(t *testing.T) {
	receipt := &WriteFinalMaterializationReceipt{SchemaVersion: 1, Status: "available", ReasonCode: "applied_checkpoint_candidates",
		RunID: "run", FinalPlanID: "final", RetainedPlanIDs: []string{"source"},
		Owners: []WriteFinalMaterializationOwner{{PlanID: "source", CommitSHA: strings.Repeat("a", 40), Paths: []string{"src/app.py"}},
			{PlanID: "test-only", CommitSHA: strings.Repeat("b", 40), Paths: []string{}}}}
	input := WriteFinalDeliverySummary{Status: "coherent", PrimarySourcePlanID: "source", Materialization: receipt}
	normalized := NormalizeWriteFinalDeliverySummary(input)
	if !reflect.DeepEqual(input.Materialization, normalized.Materialization) || input.Status != normalized.Status || input.PrimarySourcePlanID != normalized.PrimarySourcePlanID {
		t.Fatalf("normalization reinterpreted the system receipt: got=%+v want=%+v", normalized, input)
	}
	normalized.Materialization.RetainedPlanIDs[0] = "changed-root"
	normalized.Materialization.Owners[0].Paths[0] = "changed-path"
	normalized.Materialization.Owners[0].PlanID = "changed-owner"
	if receipt.RetainedPlanIDs[0] != "source" || receipt.Owners[0].Paths[0] != "src/app.py" || receipt.Owners[0].PlanID != "source" {
		t.Fatal("normalization aliases receipt inputs")
	}
	report := BuildWriteFinalReport(WriteFinalReportInput{Delivery: input})
	path := filepath.Join(t.TempDir(), "final.json")
	if err := WriteFinalReportToFile(&report, path); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadWriteFinalReportFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.Delivery.Materialization, receipt) || loaded.Delivery.Status != "coherent" || loaded.Verification.Passed {
		t.Fatalf("round trip lost candidate/root/empty-path semantics or promoted verification: %+v", loaded.Delivery)
	}
}

func TestB1693LegacyDeliveryDoesNotInventMaterialization(t *testing.T) {
	var old WriteFinalReport
	if err := json.Unmarshal([]byte(`{"schema_version":1,"delivery":{"status":"coherent","primary_source_plan_id":"source"}}`), &old); err != nil {
		t.Fatal(err)
	}
	got := NormalizeWriteFinalReport(old)
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if got.Delivery.Materialization != nil || strings.Contains(string(raw), "materialization") || got.Delivery.Status != "coherent" {
		t.Fatalf("legacy report must remain without a system receipt: %s", raw)
	}
	unknown := &WriteFinalMaterializationReceipt{SchemaVersion: 9, Status: "future_status", ReasonCode: "future_reason"}
	if got := NormalizeWriteFinalDeliverySummary(WriteFinalDeliverySummary{Materialization: unknown}); !reflect.DeepEqual(got.Materialization, unknown) {
		t.Fatalf("normalizer silently upgraded or repaired an unrecognized receipt: %+v", got.Materialization)
	}
}
