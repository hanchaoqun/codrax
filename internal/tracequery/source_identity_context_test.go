package tracequery

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestTraceSourceVersionContextKeepsSelectionAndCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capture.systrace")
	writeBundleMembershipFixture(t, path, traceBundleTestWakeupRow(20, 10))
	legacy, err := CaptureTraceSourceVersion(path)
	if err != nil {
		t.Fatal(err)
	}
	current, err := CaptureTraceSourceVersionContext(nil, path)
	if err != nil || legacy.Fingerprint() != current.Fingerprint() || legacy.SourceBytes() != current.SourceBytes() {
		t.Fatalf("context API changed selection: %v %v", current, err)
	}
	if err := current.ValidateContext(nil, path); err != nil {
		t.Fatal(err)
	}
	for _, deadline := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		want := context.Canceled
		if deadline {
			ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			defer cancel()
			want = context.DeadlineExceeded
		}
		// Cancellation wins even before an unavailable path is opened.
		if _, err := CaptureTraceSourceVersionContext(ctx, path+".missing"); !errors.Is(err, want) {
			t.Fatalf("capture cancellation became file failure: %v", err)
		}
		if err := current.ValidateContext(ctx, path+".missing"); !errors.Is(err, want) {
			t.Fatalf("validation cancellation became file failure: %v", err)
		}
	}
}
