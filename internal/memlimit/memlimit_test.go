package memlimit

import (
	"runtime"
	"runtime/debug"
	"testing"
)

func TestSystemTotalMemory(t *testing.T) {
	total, ok := systemTotalMemory()
	switch runtime.GOOS {
	case "linux", "darwin", "windows":
		if !ok || total == 0 {
			t.Fatalf("host RAM must be detectable on %s: ok=%v total=%d", runtime.GOOS, ok, total)
		}
		t.Logf("%s host RAM: %s", runtime.GOOS, humanBytes(int64(total)))
	default:
		// Platforms with no detector are expected to report ok=false.
	}
}

// readLimit returns the runtime's current soft memory limit without
// changing it.
func readLimit() int64 { return debug.SetMemoryLimit(-1) }

func TestApply_Disabled(t *testing.T) {
	r := Apply(Config{Enabled: false})
	if r.Applied {
		t.Fatalf("disabled config must not apply a limit: %+v", r)
	}
}

func TestApply_ExplicitBytes(t *testing.T) {
	prev := readLimit()
	defer debug.SetMemoryLimit(prev)
	t.Setenv("GOMEMLIMIT", "")

	want := int64(3) << 30
	r := Apply(Config{Enabled: true, ExplicitBytes: want})
	if !r.Applied || r.LimitBytes != want || r.Source != "config-bytes" {
		t.Fatalf("got %+v, want applied limit=%d source=config-bytes", r, want)
	}
	if got := readLimit(); got != want {
		t.Fatalf("runtime limit = %d, want %d", got, want)
	}
}

func TestApply_RaisesToFloor(t *testing.T) {
	prev := readLimit()
	defer debug.SetMemoryLimit(prev)
	t.Setenv("GOMEMLIMIT", "")

	r := Apply(Config{Enabled: true, ExplicitBytes: 1 << 20}) // 1 MiB
	if !r.Applied || r.LimitBytes != minLimitBytes {
		t.Fatalf("got %+v, want raised to floor %d", r, minLimitBytes)
	}
	if r.Note == "" {
		t.Fatal("a floor-raised result should carry an explanatory note")
	}
}

func TestApply_EnvVarWins(t *testing.T) {
	prev := readLimit()
	defer debug.SetMemoryLimit(prev)
	t.Setenv("GOMEMLIMIT", "2GiB")

	r := Apply(Config{Enabled: true, ExplicitBytes: 1 << 30})
	if r.Applied {
		t.Fatalf("an operator-set GOMEMLIMIT must win over config: %+v", r)
	}
}

func TestApply_HostFraction(t *testing.T) {
	prev := readLimit()
	defer debug.SetMemoryLimit(prev)
	t.Setenv("GOMEMLIMIT", "")

	total, ok := systemTotalMemory()
	if !ok {
		t.Skip("host RAM not detectable on this platform")
	}

	r := Apply(Config{Enabled: true, Fraction: 0.5})
	if !r.Applied || r.Source != "host-fraction" {
		t.Fatalf("got %+v, want applied source=host-fraction", r)
	}
	if r.TotalBytes != total {
		t.Fatalf("reported total %d != detected %d", r.TotalBytes, total)
	}
	want := int64(float64(total) * 0.5)
	if want < minLimitBytes {
		want = minLimitBytes
	}
	if r.LimitBytes != want {
		t.Fatalf("limit %d, want %d", r.LimitBytes, want)
	}
}

func TestApply_FractionClampedWhenOutOfRange(t *testing.T) {
	prev := readLimit()
	defer debug.SetMemoryLimit(prev)
	t.Setenv("GOMEMLIMIT", "")

	if _, ok := systemTotalMemory(); !ok {
		t.Skip("host RAM not detectable on this platform")
	}
	// Fraction 0 and >1 both fall back to defaultFraction.
	for _, frac := range []float64{0, -1, 5} {
		r := Apply(Config{Enabled: true, Fraction: frac})
		if !r.Applied {
			t.Fatalf("fraction %v: not applied: %+v", frac, r)
		}
		want := int64(float64(r.TotalBytes) * defaultFraction)
		if want < minLimitBytes {
			want = minLimitBytes
		}
		if r.LimitBytes != want {
			t.Fatalf("fraction %v: limit %d, want default-fraction %d", frac, r.LimitBytes, want)
		}
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{
		512:               "512B",
		1024:              "1.00KiB",
		3 << 30:           "3.00GiB",
		int64(2560) << 20: "2.50GiB",
	}
	for in, want := range cases {
		if got := humanBytes(in); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}
