package tracequery

import (
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func b1638b2aCopySource(t *testing.T) offCPUStatsResult {
	t.Helper()
	idx := buildTraceIndex(t, "measurement-copy.ftrace", b1607ManySleepsTrace(39))
	return b1638b2aNativeOffCPU(idx, Query{TimeStart: 10, TimeEnd: 10.041, TimeStartSet: true, TimeEndSet: true})
}

func TestB1638B2ANativeOffCPURosterCopiesDoNotAliasCensus(t *testing.T) {
	result := b1638b2aCopySource(t)
	for _, lane := range []struct {
		name   string
		top    []ThreadDuration
		census map[string]ThreadDuration
	}{
		{"runnable", result.runnableTop, result.runnableCensus},
		{"d_state", result.dstateTop, result.dstateCensus},
		{"io_wait", result.iowaitTop, result.iowaitCensus},
	} {
		t.Run(lane.name, func(t *testing.T) {
			row := b1638b2aTargetDuration(t, lane.top, 41)
			original := lane.census[threadCPUKey(row.Thread, row.CPU)]
			if row.MeasurementDomain == nil || original.MeasurementDomain == nil || !reflect.DeepEqual(row, original) {
				t.Fatal("actual native roster must retain the complete original row and source")
			}
			if row.MeasurementDomain == original.MeasurementDomain {
				t.Fatal("published native roster aliases the private census measurement source")
			}
			want := *original.MeasurementDomain
			row.MeasurementDomain.PartitionID = "mutated public roster"
			if *original.MeasurementDomain != want {
				t.Fatal("public source mutation changed the accounting census")
			}
		})
	}
}

func TestB1638B2ADisplayRosterCopiesOnlyMeasurementPointer(t *testing.T) {
	result := b1638b2aCopySource(t)
	before := result.runnableTop
	copy := copyThreadDurationDisplayRoster(before, 1)
	if len(copy) != 1 || !reflect.DeepEqual(copy[0], before[0]) || copy[0].MeasurementDomain == nil {
		t.Fatal("copy must preserve its old cap and all native row fields")
	}
	if copy[0].MeasurementDomain == before[0].MeasurementDomain {
		t.Fatal("display roster retains an alias to its input measurement source")
	}
	copy[0].MeasurementDomain.Method = "mutated display"
	if before[0].MeasurementDomain.Method != "off_cpu_sweep" {
		t.Fatal("display mutation escaped into its source")
	}
	legacy := []ThreadDuration{{Thread: ThreadRef{PID: 41}, DurationMs: 3}}
	if got := copyThreadDurationDisplayRoster(legacy, 1); !reflect.DeepEqual(got, legacy) || got[0].MeasurementDomain != nil {
		t.Fatal("legacy rows must not acquire provenance")
	}
}

func TestB1638B2ARunnableSourceMergePreservesOnlyOneKnownDomain(t *testing.T) {
	result := b1638b2aCopySource(t)
	base := b1638b2aTargetDuration(t, result.runnableTop, 41)
	if base.MeasurementDomain == nil {
		t.Fatal("actual native producer must establish the positive source")
	}
	for _, tc := range []struct {
		name   string
		change func(*types.TraceSchedulerMeasurementDomain) *types.TraceSchedulerMeasurementDomain
		known  bool
	}{
		{"same_source", func(d *types.TraceSchedulerMeasurementDomain) *types.TraceSchedulerMeasurementDomain { return d }, true},
		{"other_partition", func(d *types.TraceSchedulerMeasurementDomain) *types.TraceSchedulerMeasurementDomain {
			d.PartitionID += "different"
			return d
		}, false},
		{"other_method", func(d *types.TraceSchedulerMeasurementDomain) *types.TraceSchedulerMeasurementDomain {
			d.Method = "cpu_running_sweep"
			return d
		}, false},
		{"other_tid", func(d *types.TraceSchedulerMeasurementDomain) *types.TraceSchedulerMeasurementDomain {
			d.TargetTID++
			return d
		}, false},
		{"other_window", func(d *types.TraceSchedulerMeasurementDomain) *types.TraceSchedulerMeasurementDomain {
			d.WindowEndTs++
			return d
		}, false},
		{"unknown", func(*types.TraceSchedulerMeasurementDomain) *types.TraceSchedulerMeasurementDomain { return nil }, false},
	} {
		for _, reverse := range []bool{false, true} {
			t.Run(tc.name+map[bool]string{false: "/forward", true: "/reverse"}[reverse], func(t *testing.T) {
				left, right, tail := base, base, base
				left.CPU, right.CPU, tail.CPU = 0, 1, 2
				left.DurationMs, right.DurationMs, tail.DurationMs = 1, 2, 3
				left.MeasurementDomain = types.CloneTraceSchedulerMeasurementDomain(base.MeasurementDomain)
				right.MeasurementDomain = tc.change(types.CloneTraceSchedulerMeasurementDomain(base.MeasurementDomain))
				tail.MeasurementDomain = types.CloneTraceSchedulerMeasurementDomain(base.MeasurementDomain)
				if reverse {
					left.MeasurementDomain, right.MeasurementDomain = right.MeasurementDomain, left.MeasurementDomain
				}
				rows := map[string]ThreadDuration{"a": left, "b": right, "c": tail}
				got := aggregateChainRunnableCensusByThread(rows, map[int]bool{41: true}, 0)
				if len(got) != 1 || got[0].DurationMs != 6 || got[0].Thread.PID != 41 || got[0].CPU != -1 {
					t.Fatalf("source qualification changed existing aggregation: %+v", got)
				}
				legacyRows := make(map[string]ThreadDuration, len(rows))
				for key, row := range rows {
					row.MeasurementDomain = nil
					legacyRows[key] = row
				}
				legacy := aggregateChainRunnableCensusByThread(legacyRows, map[int]bool{41: true}, 0)
				withoutSource := got[0]
				withoutSource.MeasurementDomain = nil
				if len(legacy) != 1 || !reflect.DeepEqual(withoutSource, legacy[0]) {
					t.Fatal("source handling changed non-source aggregate fields")
				}
				if !tc.known {
					if got[0].MeasurementDomain != nil {
						t.Fatalf("mixed/unknown source borrowed a member domain: %+v", got[0].MeasurementDomain)
					}
					return
				}
				if !reflect.DeepEqual(got[0].MeasurementDomain, base.MeasurementDomain) {
					t.Fatal("same TID native account source must survive cross-CPU display aggregation")
				}
				for _, row := range rows {
					if got[0].MeasurementDomain == row.MeasurementDomain {
						t.Fatal("aggregate source aliases one contributing row")
					}
				}
			})
		}
	}
}
