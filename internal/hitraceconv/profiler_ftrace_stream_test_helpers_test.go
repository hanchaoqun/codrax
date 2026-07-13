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
