package types_test

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Tiny legal perf.data uses official feature IDs, one SAMPLE, no helper/parser
// caveat shortcuts. Distinct paths do not prove distinct hardware or workloads.
func hmc229RawMetadataCapture() []byte {
	const header, attr, dataSize = 104, 48, 48
	out := make([]byte, header+attr+dataSize+64)
	copy(out, "PERFILE2")
	put := func(at int, n uint64) { binary.LittleEndian.PutUint64(out[at:at+8], n) }
	put(8, header)
	put(16, attr)
	put(24, header)
	put(32, attr)
	put(40, header+attr)
	put(48, dataSize)
	binary.LittleEndian.PutUint32(out[header+4:], 40)
	put(header+24, 1|2|4|128|256)
	p := header + attr
	binary.LittleEndian.PutUint32(out[p:], 9)
	binary.LittleEndian.PutUint16(out[p+6:], dataSize)
	put(p+8, 0x1000)
	binary.LittleEndian.PutUint32(out[p+16:], 41)
	binary.LittleEndian.PutUint32(out[p+20:], 41)
	put(p+24, 1500000000)
	put(p+40, 1)
	for i, f := range []struct {
		id    int
		value string
	}{{3, " device A "}, {8, "arm CPU"}, {9, "0x41"}, {193, " app --arg=raw "}} {
		out[72+f.id/8] |= 1 << uint(f.id%8)
		desc := header + attr + dataSize + i*16
		put(desc, uint64(len(out)))
		put(desc+8, uint64(5+len(f.value)))
		section := make([]byte, 5+len(f.value))
		binary.LittleEndian.PutUint32(section, uint32(len(f.value)+1))
		copy(section[4:], f.value)
		out = append(out, section...)
	}
	return out
}

func TestHMC229RawPerfActualPrepareSingleAndPairMetadata(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "capture.data")
	if err := os.WriteFile(path, hmc229RawMetadataCapture(), 0600); err != nil {
		t.Fatal(err)
	}
	preparer := traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(dir, "runtime")})
	material, err := preparer.Prepare(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	fields, err := material.SourceMetadata(t.Context())
	if err != nil || len(fields) != 4 {
		t.Fatal(fields, err)
	}
	if fields[0].Value != " device A " || fields[3].Value != " app --arg=raw " {
		t.Fatal("original whitespace lost", fields)
	}
	owner := types.NewMutableState("采集记录")
	rm := types.RequestModel{Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactResourcePressure}}}
	owner.SetRequestModel(rm)
	ctx := &types.BusContext{Ctx: t.Context(), RepoRoot: dir, WorkDir: dir, Mutable: owner, AnalysisIR: &types.AnalysisIR{RequestModel: rm}, TraceInputPreparer: preparer}
	args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "perf_stats", "time_start": 1, "time_end": 2})
	result, err := (&tool.TraceQuery{}).Execute(ctx, args)
	if err != nil || !result.Success {
		t.Fatalf("raw native query: %v %s", err, result.Summary)
	}
	owner.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	contract := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInputFromBusContext(ctx, 0))
	found := false
	for _, table := range contract.Choices() {
		if strings.HasPrefix(table.ObservationID, "trace_source_metadata:") {
			found = true
			raw, _ := json.Marshal(table)
			if !strings.Contains(string(raw), "WorkloadCmd") || !strings.Contains(string(raw), " app --arg=raw ") {
				t.Fatal(string(raw))
			}
		}
	}
	if !found {
		t.Fatal("raw single metadata not projected")
	}
	pair := types.NewNativeMeasurementPair(owner, [2]types.NativeMeasurementPairSide{{Request: args, Result: result, Material: material}, {Request: []byte(`{"view":"measurements"}`), Result: types.ToolResult{ToolName: "trace_query", Summary: "failed"}, Material: material}})
	report, ok := pair.Report()
	if !ok || !strings.Contains(report.Sides[0].Device, " device A ") || !strings.Contains(report.Sides[1].Workload, " app --arg=raw ") || report.Sides[1].Status != "failed" {
		t.Fatal(report)
	}
	owner.ResetTurnAArtifacts()
	report, _ = pair.Report()
	for _, side := range report.Sides {
		if side.Status != "stale" || len(side.SourceMetadata) != 0 {
			t.Fatal("old epoch metadata survived", side)
		}
	}
}
