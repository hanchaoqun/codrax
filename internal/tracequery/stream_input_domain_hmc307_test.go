package tracequery

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func hmc307TraceStreams() []struct {
	name string
	view string
	run  func(context.Context, string, Query) (Result, error)
} {
	return []struct {
		name string
		view string
		run  func(context.Context, string, Query) (Result, error)
	}{
		{"event_search", "event_search", StreamEventSearch},
		{"window_sweep", ViewWindowSweep, StreamWindowSweep},
		{"cpu_state_frequency", ViewCPUStateFrequency, StreamCPUStateFrequency},
		{"state_cluster", "window_stats", func(ctx context.Context, path string, q Query) (Result, error) {
			return StreamStateCluster(ctx, path, q, 8)
		}},
	}
}

func TestHMC307StreamingTraceInputRejectsForeignDomainBeforeRead(t *testing.T) {
	path := writeTraceMarkActionFilterFixture(t)
	for _, stream := range hmc307TraceStreams() {
		t.Run(stream.name, func(t *testing.T) {
			for _, view := range []string{ViewPacketInventory, ViewPacketFlows} {
				t.Run(view, func(t *testing.T) {
					for _, input := range []struct{ name, path string }{{"readable_trace", path}, {"absent_source", path + ".absent"}} {
						t.Run(input.name, func(t *testing.T) {
							got, err := stream.run(context.Background(), input.path, Query{View: view, TimeStart: 1, TimeEnd: 2})
							if err == nil || !strings.Contains(err.Error(), "requires a Trace-domain input view") || !reflect.DeepEqual(got, Result{}) {
								t.Fatalf("foreign input must reject before source access or result publication: result=%+v error=%v", got, err)
							}
						})
					}
				})
			}
		})
	}
}

func TestHMC307StreamingTraceInputRejectsUndeclaredOrFutureDomains(t *testing.T) {
	const future = "hmc307_future_input_domain"
	defer delete(viewCapacityTable, future)
	for _, mode := range []ViewExecutionMode{"", "future_domain", ExecutionStreamingPacket} {
		viewCapacityTable[future] = ViewCapacity{View: future, Execution: mode}
		t.Run(string(mode), func(t *testing.T) {
			for _, stream := range hmc307TraceStreams() {
				t.Run(stream.name, func(t *testing.T) {
					got, err := stream.run(context.Background(), "", Query{View: future, TimeStart: 1, TimeEnd: 2})
					if err == nil || !strings.Contains(err.Error(), "requires a Trace-domain input view") || !reflect.DeepEqual(got, Result{}) {
						t.Fatalf("registration alone granted Trace recovery authority: mode=%q result=%+v error=%v", mode, got, err)
					}
				})
			}
		})
	}
}

func TestHMC307StreamingTraceInputKeepsDefaultAliasesAndHeavyRecovery(t *testing.T) {
	path := writeTraceMarkActionFilterFixture(t)
	canonicalPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, stream := range hmc307TraceStreams() {
		t.Run(stream.name, func(t *testing.T) {
			for _, view := range []string{"", "  ", "event_search", " frame_bundle ", "wakeup_chain", "root_cause_rank", ViewWindowSweep, ViewCPUStateFrequency} {
				t.Run(view, func(t *testing.T) {
					got, err := stream.run(context.Background(), path, Query{View: view, TimeStart: 1, TimeEnd: 2})
					if err != nil || got.View != stream.view || got.SourcePath != canonicalPath || got.EventCount == 0 {
						t.Fatalf("valid Trace recovery changed: requested=%q actual_view=%q source=%q events=%d error=%v", view, got.View, got.SourcePath, got.EventCount, err)
					}
				})
			}
		})
	}
}
