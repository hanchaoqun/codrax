package types_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC229ActualPreparationRetainsMetadataWithoutMeasurementPublication(t *testing.T) {
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_dual_measurements/baseline.data")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "capture.data")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"CREATE TABLE device_info (physical_width, physical_height, physical_frame_rate)", "INSERT INTO device_info VALUES (1440, 2560, 120)", "CREATE TABLE meta (name, value)", "INSERT INTO meta VALUES ('workload', 'not a scenario identity')"} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	material, err := traceinput.Prepare(t.Context(), traceinput.Options{InputPath: path, RuntimeAnchor: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	owner := types.NewMutableState("compare captures")
	pair := types.NewNativeMeasurementPair(owner, [2]types.NativeMeasurementPairSide{{Request: []byte(`{"view":"measurements"}`), Result: types.ToolResult{ToolName: "trace_query", Summary: "query failed"}, Material: material}})
	report, ok := pair.Report()
	if !ok || report.Sides[0].Status != "failed" || report.Sides[0].Generation == "" || !strings.Contains(report.Sides[0].Device, "1440") || !strings.Contains(report.Sides[0].Device, "physical_width") {
		t.Fatalf("real prepared metadata disappeared behind missing publication: %+v", report)
	}
	if strings.Contains(report.Sides[0].Workload, "not a scenario identity") {
		t.Fatal("generic meta key became workload identity")
	}
	fields, err := material.SourceMetadata(t.Context())
	if err != nil || len(fields) == 0 {
		t.Fatal(fields, err)
	}
	fields[0].Value = "forged"
	again, _ := material.SourceMetadata(t.Context())
	if again[0].Value == "forged" {
		t.Fatal("mutable metadata alias")
	}
	raw, _ := json.Marshal(pair)
	var restored types.RuntimeMeasurementPairRef
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if _, ok := restored.Report(); ok {
		t.Fatal("JSON revived pair")
	}
	var decoded attachment.TraceMaterial
	raw, _ = json.Marshal(material)
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, err := decoded.SourceMetadata(t.Context()); err == nil {
		t.Fatal("JSON revived material")
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := material.SourceMetadata(cancelled); err == nil {
		t.Fatal("cancel ignored")
	}
	if err := os.WriteFile(material.QueryPath(), []byte("changed derived"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := material.SourceMetadata(t.Context()); err == nil {
		t.Fatal("stale derived metadata survived")
	}
	report, _ = pair.Report()
	if report.Sides[0].Status != "stale" || len(report.Sides[0].SourceMetadata) != 0 || report.Sides[0].Device != "设备未知" {
		t.Fatal("no-publication early return bypassed validation", report)
	}
}
