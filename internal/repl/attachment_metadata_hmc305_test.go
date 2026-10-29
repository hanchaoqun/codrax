package repl

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/traceinput"
)

func TestHMC305LocalTranscriptRetainsPreparedMetadata(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.systrace")
	data := strings.Repeat("worker-42 ( 42) [000] d..2 9.000001: sched_wakeup: comm=tail pid=99 prio=120 target_cpu=000\n", 100)
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := traceinput.Prepare(context.Background(), traceinput.Options{InputPath: path, PreviewBytes: 640})
	if err != nil {
		t.Fatal(err)
	}
	r := &REPL{outputDumpDir: filepath.Join(dir, "report"), outputDumpMax: 10, language: "en", attachedHitrace: m.Preview(), attachedTraceMaterial: m}
	result := r.writeLocalMarkdownTranscript("inspect", "answer")
	for _, path := range []string{result.MarkdownPath, result.HTMLPath} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), strconv.Itoa(len(data))+" B") || !strings.Contains(string(body), "preview envelope:") {
			t.Fatalf("local transcript lost prepared metadata: %s", body)
		}
	}
}
