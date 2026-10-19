package repl

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
)

func TestHMC262REPLZIPNamedMemberFullQueryAndFailedReload(t *testing.T) {
	t.Setenv("CODRAX_TRACE_STREAMER", filepath.Join(t.TempDir(), "missing-converter"))
	for _, alias := range []string{"/htrace", "/atrace"} {
		t.Run(strings.TrimPrefix(alias, "/"), func(t *testing.T) {
			dir := t.TempDir()
			source := filepath.Join(dir, "正常命名 archive.capture")
			original := hmc262REPLZIP(t, map[string][]byte{"采集/capture.data": hmc17BinaryTraceTailFixture()})
			if err := os.WriteFile(source, original, 0400); err != nil {
				t.Fatal(err)
			}
			runner, store, _, _ := hmc17LoadTraceThroughLoop(t, alias, source, 1024)
			material := runner.seenMaterials[0]
			if material.SourcePath() != source || material.QueryPath() == source || len(material.Preview()) > 1024 || strings.Contains(material.Preview(), "tail-target") {
				t.Fatal("ZIP replaced full query with preview")
			}
			result, err := (&tool.TraceQuery{}).Execute(hmc17AttachedToolContext(t, runner), json.RawMessage(`{"source":"attached_trace","view":"event_search","pattern":"tail-target","time_start":1.0105,"time_end":1.012,"limit":10}`))
			if err != nil || !result.Success || !strings.Contains(result.Summary, "tail-target") {
				t.Fatalf("ZIP tail query failed: %+v %v", result, err)
			}
			latest, err := store.LoadLatest()
			if err != nil || !latest.Trace.TraceRequiresReattach || latest.Trace.TraceOriginalPath != source {
				t.Fatalf("archive preview gained persistent query authority: %+v %v", latest, err)
			}
			if after, err := os.ReadFile(source); err != nil || !bytes.Equal(original, after) {
				t.Fatal("original ZIP changed")
			}
		})
	}
	// Use the real script loop for failed replacement and its following
	// question: both old attachment identity and the recovery barrier survive.
	ambiguous := filepath.Join(t.TempDir(), "two-members.zip")
	body := hmc262REPLZIP(t, map[string][]byte{"a.data": hmc17BinaryTraceTailFixture(), "b.data": hmc17BinaryTraceTailFixture()})
	if err := os.WriteFile(ambiguous, body, 0400); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	r, runner := hmc17RecoveryREPL(t, strings.NewReader("/htrace "+ambiguous+"\ninspect the new capture\n/exit\n"), &out)
	old := r.attachedTraceMaterial
	if err := r.Loop(); err != nil {
		t.Fatal(err)
	}
	if r.attachedTraceMaterial != old || runner.material != old || len(runner.seenMaterials) != 0 || len(r.heldTraceInputs) != 1 || !r.traceAttachmentFailed || !strings.Contains(out.String(), "multiple_trace_members") {
		t.Fatalf("ambiguous ZIP borrowed/replaced old attachment: %s", out.String())
	}
}

func hmc262REPLZIP(t *testing.T, members map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range members {
		member, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := member.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
