package tracewire

import (
	"encoding/base64"
	"math"
	"reflect"
	"strings"
	"testing"
)

func cpuMeasureInt(v int64) *int64 { return &v }

func TestCPUMeasureIntervalExactRoundTrip(t *testing.T) {
	for _, start := range []int64{-99816000, 0, 9007199254740993} {
		r := CPUMeasureInterval{RowID: -7, FilterID: 12, CPU: 4095, Kind: "idle", Encoding: "native_sql_idle", StartNS: cpuMeasureInt(start), DurationNS: cpuMeasureInt(17), Value: cpuMeasureInt(math.MaxUint32)}
		line, err := FormatCPUMeasureInterval(r)
		if err != nil {
			t.Fatal(err)
		}
		got, ok := ParseCPUMeasureInterval(line)
		if !ok || !reflect.DeepEqual(got, r) {
			t.Fatalf("exact signed nanoseconds or raw state changed: %+v", got)
		}
		if start < 0 && got.TimestampNS() != 0 || start >= 0 && got.TimestampNS() != start {
			t.Fatal("publication sort coordinate drifted")
		}
	}
}

func TestCPUMeasureIntervalStrictCanonicalProtocol(t *testing.T) {
	r := CPUMeasureInterval{RowID: 1, FilterID: 2, CPU: 0, Kind: "frequency", Encoding: "khz", StartNS: cpuMeasureInt(0), DurationNS: cpuMeasureInt(10), Value: cpuMeasureInt(1000000)}
	good, _ := FormatCPUMeasureInterval(r)
	for name, mutate := range map[string]func(*CPUMeasureInterval){
		"cpu":                     func(v *CPUMeasureInterval) { v.CPU = 4096 },
		"filter":                  func(v *CPUMeasureInterval) { v.FilterID = -1 },
		"encoding":                func(v *CPUMeasureInterval) { v.Encoding = "native_sql_idle" },
		"negative_duration":       func(v *CPUMeasureInterval) { v.DurationNS = cpuMeasureInt(-1) },
		"overflow":                func(v *CPUMeasureInterval) { v.StartNS = cpuMeasureInt(math.MaxInt64) },
		"missing_without_receipt": func(v *CPUMeasureInterval) { v.DurationNS = nil },
		"contradictory_receipt":   func(v *CPUMeasureInterval) { v.Issue = "invalid_value" },
		"unknown_issue":           func(v *CPUMeasureInterval) { v.Issue = "fine" },
	} {
		t.Run(name, func(t *testing.T) {
			bad := r
			mutate(&bad)
			if _, err := FormatCPUMeasureInterval(bad); err == nil {
				t.Fatal("invalid carrier accepted")
			}
		})
	}
	b, _ := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(good, CPUMeasureIntervalPrefix+" record="))
	for _, bad := range []string{
		strings.Replace(good, "/v1", "/v2", 1), good + " ",
		CPUMeasureIntervalPrefix + " record=" + base64.RawURLEncoding.EncodeToString([]byte(strings.Replace(string(b), `"cpu":0`, `"cpu":0,"unknown":1`, 1))),
		CPUMeasureIntervalPrefix + " record=" + base64.RawURLEncoding.EncodeToString([]byte(strings.Replace(string(b), `"cpu":0`, `"cpu":0,"cpu":0`, 1))),
	} {
		if _, ok := ParseCPUMeasureInterval(bad); ok {
			t.Fatalf("noncanonical/unknown-version carrier accepted: %s", bad)
		}
	}
	r.DurationNS, r.Issue = nil, "unknown_duration"
	if line, err := FormatCPUMeasureInterval(r); err != nil {
		t.Fatal(err)
	} else if got, ok := ParseCPUMeasureInterval(line); !ok || got.DurationNS != nil {
		t.Fatal("unknown duration replaced by zero")
	}
}

func TestCPUMeasureIntervalUnrelatedRowsDoNotAllocate(t *testing.T) {
	allocs := testing.AllocsPerRun(100, func() {
		if _, ok := ParseCPUMeasureInterval("worker-1 [001] .... 1.000000: sched_switch: prev_pid=1 next_pid=2"); ok {
			t.Fatal("unrelated row recognized")
		}
	})
	if allocs != 0 {
		t.Fatalf("new carrier added %.1f allocations to every unrelated trace row", allocs)
	}
}
