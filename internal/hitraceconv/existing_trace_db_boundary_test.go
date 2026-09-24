package hitraceconv

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareExistingTraceDBAliasChecksBothAuxiliaryNamespaces(t *testing.T) {
	for _, auxiliaryOnTarget := range []bool{false, true} {
		t.Run(map[bool]string{false: "alias", true: "canonical"}[auxiliaryOnTarget], func(t *testing.T) {
			input := existingTraceDBFixture(t)
			alias := filepath.Join(t.TempDir(), "database-alias")
			if err := os.Symlink(input, alias); err != nil {
				t.Skipf("symlink fixture unavailable: %v", err)
			}
			if err := ValidateExistingTraceDBSource(t.Context(), alias); err != nil {
				t.Fatal(err)
			}
			base := alias
			if auxiliaryOnTarget {
				base = input
			}
			if err := os.WriteFile(base+"-shm", nil, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := ValidateExistingTraceDBSource(t.Context(), alias); !errors.Is(err, errTraceStreamerDBAuxiliaryState) {
				t.Fatalf("source guard ignored %s namespace: %v", base, err)
			}
			opts := existingTraceDBOptions(t, alias)
			result, err := PrepareExistingTraceDB(t.Context(), opts)
			assertExistingTraceDBFailureClean(t, opts, result, err)
		})
	}
}

func TestPrepareExistingTraceDBPreCanceledAndExistingOutput(t *testing.T) {
	input := existingTraceDBFixture(t)
	opts := existingTraceDBOptions(t, input)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result, err := PrepareExistingTraceDB(ctx, opts)
	assertExistingTraceDBFailureClean(t, opts, result, err)
	if !errors.Is(err, context.Canceled) || !errors.Is(ValidateExistingTraceDBSource(ctx, input), context.Canceled) {
		t.Fatalf("pre-cancellation lost: %v", err)
	}
	want := []byte("user output must survive")
	if err := os.WriteFile(opts.OutputPath, want, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareExistingTraceDB(t.Context(), opts); err == nil {
		t.Fatal("existing output accepted")
	}
	got, err := os.ReadFile(opts.OutputPath)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("existing output changed: %q %v", got, err)
	}
}
