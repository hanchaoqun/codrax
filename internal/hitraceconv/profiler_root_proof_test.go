package hitraceconv

import (
	"bytes"
	"context"
	"io"
	"testing"
)

// profilerRootProofForTest runs the production physical-profile authority once
// for a fixture. Tests then pass the immutable proof into semantic extraction,
// exactly as ConvertFile does through the standalone-layout inventory.
func profilerRootProofForTest(t testing.TB, reader io.ReaderAt, bodyEnd int64,
	header profilerTraceHeader, maxFrameBytes uint64,
) *profilerRootProfileProof {
	t.Helper()
	proof, err := validateProfilerRootProfileEnvelope(
		context.Background(), reader, header, bodyEnd, maxFrameBytes)
	if err != nil {
		t.Fatalf("validate profiler root fixture proof: %v", err)
	}
	return &proof
}

func profilerRootProofFromBodyForTest(t testing.TB, body []byte,
	maxFrameBytes uint64,
) *profilerRootProfileProof {
	t.Helper()
	header, ok := readProfilerTraceHeaderAt(bytes.NewReader(body), 0, int64(len(body)))
	if !ok || header.DataType != profilerDataTypeProtobuf {
		return nil
	}
	return profilerRootProofForTest(
		t, bytes.NewReader(body), int64(len(body)), header, maxFrameBytes)
}
