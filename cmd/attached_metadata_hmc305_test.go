package cmd

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestHMC305CLIPreparedMetadataReachesStatus(t *testing.T) {
	resetPreparedTraceFlags(t)
	path := filepath.Join(t.TempDir(), "plain.systrace")
	data := strings.Repeat("worker-42 ( 42) [000] d..2 9.000001: sched_wakeup: comm=tail pid=99 prio=120 target_cpu=000\n", 100)
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	flagAttachHitrace = []string{path}
	maxAttachedTraceBytes = 640
	loaded, err := loadPreparedAttachedTrace(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	rows := cliRuntimeArtifacts("inspect "+path, "", loaded.body, loaded.material)
	status := strings.Join(cliRuntimeArtifactStatusLines("en", rows), "\n")
	for _, want := range []string{strconv.Itoa(len(data)) + " B", "preview envelope:", "query material: same as original", "original format: text", "kinds=text=1", "primary: text "} {
		if !strings.Contains(status, want) {
			t.Fatalf("CLI lost %q: %s", want, status)
		}
	}
}
