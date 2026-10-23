package tool

import (
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/types"
)

// This test oracle separates a current preparation's background table from
// measured rows. It never classifies by an observation-ID prefix or relaxes a
// source/window gate. Every excluded table must equal the exact current table.
type hmc287MetadataOracle struct {
	table *types.RuntimeMeasurementTable
}

func hmc287MeasurementOriginal(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_measurements/capture.data")
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func hmc287CurrentMetadataOracle(t *testing.T, bus *types.BusContext, original, end string) hmc287MetadataOracle {
	t.Helper()
	contract := types.BuildAnswerSemanticViewForBusContext(bus).RuntimeMeasurementContract
	var oracle hmc287MetadataOracle
	for _, table := range contract.Choices() {
		if table.Label != "采集原件记录的背景信息" {
			continue
		}
		if original == "" || oracle.table != nil {
			t.Fatal("unexpected or duplicate source-metadata table")
		}
		basis, valid := table.NativeSourceTitleBasis()
		if !valid || basis.OriginalSource != original || basis.QuerySource == "" || basis.QuerySourceVersion == "" || basis.QueryWindow != "" || basis.QuerySelection != "" {
			t.Fatalf("metadata lost its exact current original or acquired a query window: %+v", basis)
		}
		var fields []attachment.TraceSourceMetadataField
		if bus.TraceInputPreparer != nil {
			for _, material := range bus.TraceInputPreparer.PreparedMaterials() {
				if material != nil && material.MatchesOriginalSource(original) && material.QueryPath() == basis.QuerySource {
					var err error
					fields, err = material.SourceMetadata(t.Context())
					if err != nil {
						t.Fatal(err)
					}
				}
			}
		}
		wantFields := map[string][2]string{
			"device_info/device/physical_width":      {"missing", ""},
			"device_info/device/physical_height":     {"missing", ""},
			"device_info/device/physical_frame_rate": {"missing", ""},
			"meta/parser/table":                      {"missing", ""},
			"trace_range/source_range/start_ts":      {"observed", "0"},
			"trace_range/source_range/end_ts":        {"missing", ""},
			"trace_range/source_range/duration_ns":   {"missing", ""},
		}
		if end != "" {
			wantFields["trace_range/source_range/end_ts"] = [2]string{"observed", end}
			wantFields["trace_range/source_range/duration_ns"] = [2]string{"observed", end}
		}
		var rows [][]string
		if len(fields) != 7 {
			t.Fatalf("source metadata has %d fields, want independently pinned seven", len(fields))
		}
		for _, field := range fields {
			key := field.Origin + "/" + field.Kind + "/" + field.Name
			want, exists := wantFields[key]
			if !exists || want != [2]string{field.State, field.Value} {
				t.Fatalf("source metadata field changed: %+v", field)
			}
			delete(wantFields, key)
			rows = append(rows, []string{original, field.Origin, field.Kind, field.Name, field.State, strconv.Quote(field.Value), field.Detail})
		}
		if len(wantFields) != 0 || !reflect.DeepEqual(table.Rows, rows) || !reflect.DeepEqual(table.Columns, []string{"原始采集", "记录位置", "字段类别", "字段", "状态", "原值", "说明"}) {
			t.Fatalf("background metadata lost exact original fields: %+v", table)
		}
		if table.View != types.RuntimeMeasurementSummary || !table.DefaultPresentation || !table.HasNativeRawFacts() || table.MemberSet != nil || table.DisplayPopulation != nil || table.EventData != nil || table.HasCompleteNativeDisplayPopulation() || table.CoversMemberSet(&bus.AnalysisIR.RequestModel) {
			t.Fatal("background metadata gained measured-population or completion authority")
		}
		copy := table.Clone()
		oracle.table = &copy
	}
	if original != "" && oracle.table == nil {
		t.Fatal("current preparation metadata table missing")
	}
	return oracle
}

func (o hmc287MetadataOracle) matches(t *testing.T, table types.RuntimeMeasurementTable) bool {
	t.Helper()
	if o.table == nil || table.ObservationID != o.table.ObservationID {
		return false
	}
	if !reflect.DeepEqual(table, *o.table) {
		t.Fatal("metadata selector reused an ID with changed source, rows, view or authority")
	}
	return true
}

func (o hmc287MetadataOracle) splitBlocks(t *testing.T, blocks []types.AnswerBlock) (facts, metadata []types.AnswerBlock) {
	t.Helper()
	for _, block := range blocks {
		if block.RuntimeMeasurement == nil {
			continue
		}
		if !block.RuntimeMeasurement.IsBound() {
			t.Fatal("native display retained an unbound runtime receipt")
		}
		if o.matches(t, *block.RuntimeMeasurement.BoundTable) {
			metadata = append(metadata, block)
		} else {
			facts = append(facts, block)
		}
	}
	return facts, metadata
}
