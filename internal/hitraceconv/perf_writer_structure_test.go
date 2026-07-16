package hitraceconv

import (
	"os"
	"strings"
	"testing"
)

func TestOwnedPerfWritersUseSingleTypedBodyAuthority(t *testing.T) {
	files := []string{
		"simpleperf_text.go",
		"simpleperf_proto.go",
		"hiperf_proto.go",
		"raw_perfdata.go",
		"streamerdb_export_perf.go",
	}
	for _, path := range files {
		t.Run(path, func(t *testing.T) {
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			text := string(body)
			if got := strings.Count(text, "tracewire.BuildPerfSampleBody("); got != 1 {
				t.Fatalf("typed perf body authority call count=%d, want 1", got)
			}
			if strings.Contains(text, "perf_sample:") {
				t.Fatal("writer retained a direct perf_sample body formatter")
			}
		})
	}

	raw, err := os.ReadFile("raw_perfdata.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, retired := range []string{"rawPerfSampleExtraFields", "appendU64Field", "appendHexU64Field"} {
		if strings.Contains(string(raw), retired) {
			t.Fatalf("raw writer retained arbitrary suffix helper %q", retired)
		}
	}
}
