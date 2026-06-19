package tool

import "testing"

func TestRunTestsDefaultTimeoutSeconds_DefaultOverrideAndReset(t *testing.T) {
	SetRunTestsDefaultTimeoutSeconds(0)
	t.Cleanup(func() { SetRunTestsDefaultTimeoutSeconds(0) })

	if got := RunTestsDefaultTimeoutSeconds(); got != 900 {
		t.Fatalf("default run_tests timeout = %d, want 900", got)
	}

	SetRunTestsDefaultTimeoutSeconds(1200)
	if got := RunTestsDefaultTimeoutSeconds(); got != 1200 {
		t.Fatalf("overridden run_tests timeout = %d, want 1200", got)
	}

	SetRunTestsDefaultTimeoutSeconds(-1)
	if got := RunTestsDefaultTimeoutSeconds(); got != 900 {
		t.Fatalf("reset run_tests timeout = %d, want 900", got)
	}
}
