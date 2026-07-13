package hitraceconv

import (
	"context"
	"testing"
)

func profilerTracePluginPayloadsForTest(t *testing.T, result profilerTracePluginResult, field int) [][]byte {
	t.Helper()
	var out [][]byte
	if err := visitProfilerTracePluginResult(context.Background(), result, func(observedField int, raw []byte) error {
		if observedField == field {
			out = append(out, append([]byte(nil), raw...))
		}
		return nil
	}); err != nil {
		t.Fatalf("visit TracePluginResult field %d: %v", field, err)
	}
	return out
}

func profilerTracePluginResultEvents(result profilerTracePluginResult) ([]profilerFtraceEventRecord, error) {
	var out []profilerFtraceEventRecord
	err := visitProfilerTracePluginResultEventsContext(context.Background(), result, func(record profilerFtraceEventRecord) error {
		out = append(out, record)
		return nil
	})
	return out, err
}

func decodeProfilerFtraceStructuredEvents(data []byte) ([]profilerFtraceEventRecord, error) {
	return profilerTracePluginResultEvents(decodeProfilerTracePluginResult(data))
}

func decodeProfilerFtraceCPUDetailEvents(data []byte) ([]profilerFtraceEventRecord, error) {
	authority, err := auditProfilerFtraceCPUDetail(context.Background(), data)
	if err != nil {
		return nil, err
	}
	var out []profilerFtraceEventRecord
	err = visitProfilerFtraceCPUDetailEvents(context.Background(), authority, func(record profilerFtraceEventRecord) error {
		out = append(out, record)
		return nil
	})
	return out, err
}
