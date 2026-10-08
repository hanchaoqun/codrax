package tool

import (
	"crypto/sha256"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStaticInitializePublicDefaultPreparePreservesUnknownCPUAndWindow(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_static_initialize/capture.data")
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, _, _ := hmc17NamedPathContext(t)
	r := hmc17NamedQuery(t, ctx, map[string]any{"source": "path", "path": path, "view": "recipe", "recipe_name": "span_locate", "span_name": "SoInit:", "pid": 101, "time_start": 10, "time_end": 10.05})
	p := hmc17NamedPayload(t, r)
	count, total := 0, 0.0
	unknown := false
	for _, s := range p.SpanWindows {
		if !strings.HasPrefix(s.Name, "SoInit:") {
			continue
		}
		count++
		total += s.DurationMs
		if s.Thread.PID != 101 {
			t.Fatalf("foreign owner %+v", s)
		}
		if strings.Contains(s.Name, "librender") {
			unknown = s.CPUStatus == "unavailable"
			if math.Abs(s.DurationMs-15) > 1e-6 {
				t.Fatalf("unknown CPU lost real duration %+v", s)
			}
		}
		if strings.Contains(s.Name, "later") || strings.Contains(s.Name, "background") {
			t.Fatalf("window/owner leak %+v", s)
		}
	}
	if count != 2 || math.Abs(total-21) > 1e-6 || !unknown {
		t.Fatalf("public static spans count=%d total=%v unknown=%t\n%+v\n%s", count, total, unknown, p.SpanWindows, r.Summary)
	}
	if len(r.Observations) == 0 {
		t.Fatal("no public typed evidence")
	}
	after, err := os.ReadFile(path)
	if err != nil || sha256.Sum256(before) != sha256.Sum256(after) {
		t.Fatal("source SQLite mutated")
	}
	other := hmc17NamedPayload(t, hmc17NamedQuery(t, ctx, map[string]any{"source": "path", "path": path, "view": "recipe", "recipe_name": "span_locate", "span_name": "SoInit:", "pid": 202, "time_start": 10, "time_end": 10.05}))
	otherCount := 0
	for _, s := range other.SpanWindows {
		if strings.HasPrefix(s.Name, "SoInit:") {
			otherCount++
			if !strings.Contains(s.Name, "background") {
				t.Fatalf("foreign span %+v", s)
			}
		}
	}
	if otherCount != 1 {
		t.Fatalf("other owner unavailable %+v", other.SpanWindows)
	}
}
