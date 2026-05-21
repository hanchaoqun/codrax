package cpulimit

import (
	"runtime"
	"testing"
)

func TestApply_Disabled(t *testing.T) {
	defer func() { resolved.enabled = false }()
	r := Apply(Config{Enabled: false})
	if r.Applied {
		t.Fatalf("disabled config must not apply: %+v", r)
	}
	if resolved.enabled {
		t.Fatal("disabled config must leave resolved state off")
	}
}

func TestApply_Enabled(t *testing.T) {
	defer func() { resolved.enabled = false }()
	r := Apply(Config{Enabled: true, Nice: 10})
	if runtime.GOOS == "windows" || runtime.GOOS == "plan9" || runtime.GOOS == "js" {
		if r.Applied {
			t.Fatalf("niceness should be unsupported on %s: %+v", runtime.GOOS, r)
		}
		return
	}
	if !r.Applied || r.Nice != 10 {
		t.Fatalf("got %+v, want applied nice=10", r)
	}
	if !resolved.enabled || resolved.nice != 10 {
		t.Fatalf("resolved = %+v, want enabled nice=10", resolved)
	}
}

func TestApply_ClampsNice(t *testing.T) {
	defer func() { resolved.enabled = false }()
	for _, tc := range []struct {
		in, want int
	}{
		{-5, niceMin},
		{0, 0},
		{10, 10},
		{19, 19},
		{50, niceMax},
	} {
		Apply(Config{Enabled: true, Nice: tc.in})
		if resolved.nice != tc.want {
			t.Errorf("Nice %d clamped to %d, want %d", tc.in, resolved.nice, tc.want)
		}
	}
}

func TestNiceCurrentThread_NoopWhenDisabled(t *testing.T) {
	resolved.enabled = false
	// Must not panic and must do nothing when politeness is off.
	NiceCurrentThread()
}

func TestNiceCurrentThread_AfterApply(t *testing.T) {
	defer func() { resolved.enabled = false }()
	Apply(Config{Enabled: true, Nice: 12})
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// Exercises the worker call path; on unsupported platforms it is a
	// silent no-op, on Unix it raises this thread's nice value.
	NiceCurrentThread()
}
