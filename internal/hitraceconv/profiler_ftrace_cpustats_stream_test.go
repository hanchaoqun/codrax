package hitraceconv

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"
)

func profilerCPUStatsRecord(cpu, entries uint64) []byte {
	return protoMessage(2, protoPayload(
		protoVarint(1, cpu),
		protoVarint(2, entries),
	))
}

func TestProfilerFtraceCPUStatsInvalidOccurrenceIsAtomic(t *testing.T) {
	healthyA := profilerCPUStatsRecord(0, 1)
	healthyC := profilerCPUStatsRecord(2, 3)
	badOccurrences := map[string][]byte{
		"per_cpu_wrong_wire": protoPayload(
			profilerCPUStatsRecord(1, 100),
			protoVarint(2, 1),
		),
		"nested_malformed": protoPayload(
			profilerCPUStatsRecord(1, 100),
			protoBytes(2, []byte{0x80}),
		),
		"late_malformed_tail": append(
			profilerCPUStatsRecord(1, 100),
			0x80,
		),
		"late_status_duplicate": protoPayload(
			profilerCPUStatsRecord(1, 100),
			protoVarint(1, 0),
			protoVarint(1, 1),
		),
	}
	for name, bad := range badOccurrences {
		t.Run(name, func(t *testing.T) {
			result := decodeProfilerTracePluginResult(protoPayload(
				protoBytes(1, healthyA),
				protoBytes(1, bad),
				protoBytes(1, healthyC),
			))
			summary, recognized, err := decodeProfilerFtraceSummaryResult(result)
			if err != nil || !recognized {
				t.Fatalf("summary recognized=%t err=%v", recognized, err)
			}
			if summary.StatsMessages != 2 || summary.StartStats != 2 || summary.EndStats != 0 ||
				summary.StatsCPUs.count() != 2 || !summary.StatsCPUs.contains(0) || !summary.StatsCPUs.contains(2) || summary.StatsCPUs.contains(1) ||
				!summary.StartTotalsSeen || !summary.StartTotalsValid || summary.StartTotals.Entries != 4 {
				t.Fatalf("invalid CPUStats occurrence leaked a validated prefix: %+v", summary)
			}
			if summary.Issues.Occurrences[profilerFtraceSummaryIssueCPUStatsMalformed] != 1 {
				t.Fatalf("invalid occurrence issue units drifted: %+v", summary.Issues)
			}
		})
	}
}

func TestProfilerFtraceCPUStatsMillionPerCPURecordsRetainFixedShape(t *testing.T) {
	const occurrences = 1_000_000
	payload := bytes.Repeat(protoBytes(2, nil), occurrences)
	stats, err := decodeProfilerFtraceCPUStatsContext(context.Background(), payload)
	if err != nil {
		t.Fatal(err)
	}
	if stats.PerCPUOccurrences != occurrences || len(stats.payload) != len(payload) {
		t.Fatalf("CPUStats census drifted: %+v", stats)
	}
	visited := 0
	if err := visitProfilerFtracePerCPUStats(context.Background(), stats, func(profilerFtracePerCPUStats) error {
		visited++
		return nil
	}); err != nil || visited != occurrences {
		t.Fatalf("CPUStats replay visited=%d err=%v", visited, err)
	}

	typ := reflect.TypeOf(stats)
	for index := 0; index < typ.NumField(); index++ {
		field := typ.Field(index)
		if field.Type.Kind() == reflect.Map ||
			field.Type.Kind() == reflect.Slice && (field.Name != "payload" || field.Type.Elem().Kind() != reflect.Uint8) {
			t.Fatalf("CPUStats authority retains repeated state: %s %s", field.Name, field.Type)
		}
	}
}

func TestProfilerFtraceCPUStatsVisitorCancelsDuringRepeatedWalk(t *testing.T) {
	const occurrences = 2_000
	stats, err := decodeProfilerFtraceCPUStats(bytes.Repeat(protoBytes(2, nil), occurrences))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	visited := 0
	err = visitProfilerFtracePerCPUStats(ctx, stats, func(profilerFtracePerCPUStats) error {
		visited++
		if visited == 300 {
			cancel()
		}
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CPUStats cancellation identity lost: visited=%d err=%v", visited, err)
	}
	if visited < 300 || visited > 300+255 {
		t.Fatalf("CPUStats cancellation polling bound drifted: visited=%d", visited)
	}
}

func TestProfilerFtraceCPUStatsVisitorPreservesCallbackErrorIdentity(t *testing.T) {
	stats, err := decodeProfilerFtraceCPUStats(profilerCPUStatsRecord(0, 1))
	if err != nil {
		t.Fatal(err)
	}
	sentinel := &traceDBOutputInvariantError{Reason: "test_cpu_stats_callback"}
	if err := visitProfilerFtracePerCPUStats(context.Background(), stats, func(profilerFtracePerCPUStats) error {
		return sentinel
	}); !errors.Is(err, sentinel) {
		t.Fatalf("CPUStats callback error identity lost: %v", err)
	}
}
