package tool

import (
	"crypto/sha256"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestCPUStateFrequencyNativeSQLiteDefaultPreparation(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_cpu_native_intervals/capture.data")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, _, _ := hmc17NamedPathContext(t)
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "cpu_state_frequency", "time_start": 0, "time_end": 0.04})
	r, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !r.Success {
		t.Fatalf("default prepare/query: %v %+v", err, r)
	}
	var p tracequery.CPUStateFrequencyResult
	found := false
	for _, record := range r.Observations {
		if decoded, ok := DecodeTraceCPUStateFrequency(record); ok {
			p, found = decoded, true
		}
	}
	if !found || p.Status != "available" || p.CPUCount != 2 || math.Abs(p.CPUTimeMs-80) > 1e-6 || math.Abs(p.KnownJointMs-35) > 1e-6 || math.Abs(p.UnknownJointMs-45) > 1e-6 {
		t.Fatalf("native intervals lost at public handoff: %+v\n%s", p, r.Summary)
	}
	for i, want := range [][3]float64{{20, 35, 15}, {40, 20, 20}} {
		cpu := p.CPUs[i]
		if cpu.CPU != i || math.Abs(cpu.IdleKnownMs-want[0]) > 1e-6 || math.Abs(cpu.FrequencyKnownMs-want[1]) > 1e-6 || math.Abs(cpu.JointKnownMs-want[2]) > 1e-6 {
			t.Errorf("CPU%d wrong account: %+v", i, cpu)
		}
		for _, row := range cpu.Intervals {
			if row.StateKnown && row.StateEncoding != "native_sql_idle" {
				t.Errorf("source encoding lost: %+v", row)
			}
		}
	}
	for _, want := range []string{"源CPU状态码0（含义未核实）", "源CPU状态码1（含义未核实）", "源CPU状态码2（含义未核实）", "联合未知=45", "全部核时间=80", "1000000", "2000000", "800000"} {
		if !strings.Contains(r.Summary, want) {
			t.Errorf("preview lost %q: %s", want, r.Summary)
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || sha256.Sum256(before) != sha256.Sum256(after) {
		t.Fatal("source database mutated")
	}
}

func TestCPUStateFrequencyLabelsKeepNativeCodesOpaque(t *testing.T) {
	zero, khz := uint32(0), int64(1000000)
	v := tracequery.CPUStateFrequencyValue{State: "native_idle", StateKnown: true, IdleState: &zero, StateEncoding: "native_sql_idle", FrequencyKnown: true, FrequencyKHz: &khz}
	state, frequency := cpuStateFrequencyLabels(v)
	if state != "源CPU状态码0（含义未核实）" || frequency != "1000000" {
		t.Fatalf("semantic/unit conversion: %s %s", state, frequency)
	}
	v.StateEncoding, v.State = "", "idle"
	if state, _ := cpuStateFrequencyLabels(v); state != "idle状态0" {
		t.Fatal("raw ftrace semantics changed")
	}
}
