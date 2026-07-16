package tracewire

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
)

func TestPerfCallchainBuilderIncrementalBoundAndOrder(t *testing.T) {
	var builder PerfCallchainBuilder
	for _, frame := range []string{"root@lib.so", "middle", "leaf"} {
		if err := builder.AppendFrame(context.Background(), frame); err != nil {
			t.Fatalf("AppendFrame(%q): %v", frame, err)
		}
	}
	if got, want := builder.String(), "root@lib.so;middle;leaf"; got != want {
		t.Fatalf("callchain order drift: got %q want %q", got, want)
	}
	if builder.Frames() != 3 || builder.Len() != len(builder.String()) {
		t.Fatalf("builder accounting drift: frames=%d len=%d wire=%q", builder.Frames(), builder.Len(), builder.String())
	}

	var exact PerfCallchainBuilder
	if err := exact.AppendFrame(context.Background(), strings.Repeat("x", MaxPerfCallchainBytes)); err != nil {
		t.Fatalf("inclusive callchain limit rejected: %v", err)
	}
	before := exact.String()
	err := exact.AppendFrame(context.Background(), "y")
	var typed *PerfWireBuildError
	if !errors.As(err, &typed) || typed.Field != "callchain" || typed.Reason != "decoded_value_too_long" || typed.Limit != MaxPerfCallchainBytes || typed.Actual != MaxPerfCallchainBytes+2 {
		t.Fatalf("unexpected overflow error: %T %v", err, err)
	}
	if exact.String() != before || exact.Frames() != 1 {
		t.Fatalf("overflow mutated builder: wire=%q frames=%d", exact.String(), exact.Frames())
	}
}

func TestPerfCallchainBuilderFailureIsTransactional(t *testing.T) {
	var builder PerfCallchainBuilder
	if err := builder.AppendFrame(context.Background(), "root"); err != nil {
		t.Fatal(err)
	}
	before := builder.String()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := builder.AppendFrame(ctx, "cancelled"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error = %v", err)
	}
	if builder.String() != before || builder.Frames() != 1 {
		t.Fatalf("cancellation mutated builder: wire=%q frames=%d", builder.String(), builder.Frames())
	}

	for _, frame := range []string{"", string([]byte{0xff})} {
		if err := builder.AppendFrame(context.Background(), frame); err == nil {
			t.Fatalf("invalid frame %q accepted", frame)
		}
		if builder.String() != before || builder.Frames() != 1 {
			t.Fatalf("invalid frame mutated builder: wire=%q frames=%d", builder.String(), builder.Frames())
		}
	}
}

func TestCheckedPerfSampleWeightDomain(t *testing.T) {
	if got, err := CheckedPerfSampleWeight(1); err != nil || got != 1 {
		t.Fatalf("weight 1: got=%d err=%v", got, err)
	}
	if got, err := CheckedPerfSampleWeight(math.MaxInt64); err != nil || got != math.MaxInt64 {
		t.Fatalf("weight MaxInt64: got=%d err=%v", got, err)
	}
	for _, value := range []uint64{0, math.MaxInt64 + 1, math.MaxUint64} {
		if _, err := CheckedPerfSampleWeight(value); err == nil {
			t.Fatalf("weight %d accepted", value)
		}
	}
}
