package context

import (
	"bytes"
	stdcontext "context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/tracebundle"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestPreparedTracePromptDoesNotPublishPreviewAsComplete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "capture.sys")
	if err := os.WriteFile(path, []byte(strings.Repeat("# text only\n", 1000)), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := traceinput.Prepare(stdcontext.Background(), traceinput.Options{InputPath: path, PreviewBytes: 512})
	if err != nil {
		t.Fatal(err)
	}
	for _, tools := range []attachedTraceRenderOptions{{PreferTraceQuery: true, Material: m}, {ReadFileAvailable: true, Material: m}, {Material: m}} {
		got := formatAttachedTrace(m.Preview(), dir, attachedTriageUnavailable, "", tools)
		for _, want := range []string{"bounded preview", "not physical evidence coordinates", "Original Trace source:", "Conversion alone proves no"} {
			if !strings.Contains(got, want) {
				t.Fatalf("prompt missing %q: %s", want, got)
			}
		}
		if strings.Contains(got, "The complete trace is saved") {
			t.Fatal("preview claimed as whole trace")
		}
		if strings.Contains(got, "sample-only") || tools.PreferTraceQuery && !strings.Contains(got, "Prefer `trace_query` for scheduler state, wakeup chains") {
			t.Fatal("sample-only boundary restricted ordinary trace navigation")
		}
	}
	if _, err := os.Stat(filepath.Join(dir, AttachedTraceBlobName)); !os.IsNotExist(err) {
		t.Fatalf("preview materialized as query blob: %v", err)
	}
	if err := os.WriteFile(path, []byte("# changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got := formatAttachedTrace(m.Preview(), dir, attachedTriageUnavailable, "", attachedTraceRenderOptions{Material: m})
	if !strings.Contains(got, "Reattach") || strings.Contains(got, "```text") {
		t.Fatalf("stale preview exposed: %s", got)
	}
}

func TestPreparedSimpleperfPromptPreservesSampleOnlyBoundary(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "original.capture")
	original := preparedPromptSimpleperfFixture(80)
	if err := os.WriteFile(source, original, 0o600); err != nil {
		t.Fatal(err)
	}
	anchor := filepath.Join(dir, "runtime")
	const previewLimit = 1024
	material, err := traceinput.Prepare(stdcontext.Background(), traceinput.Options{
		InputPath: source, RuntimeAnchor: anchor, PreviewBytes: previewLimit,
	})
	if err != nil {
		t.Fatalf("prepare real SIMPLEPERF wire input: %v", err)
	}
	if len(material.Preview()) > previewLimit || !strings.Contains(material.Preview(), "truncated; full query material retained") {
		t.Fatalf("fixture must use a bounded, explicitly truncated preview: %q", material.Preview())
	}
	canonicalAnchor, err := filepath.EvalSymlinks(anchor)
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(canonicalAnchor, material.QueryPath())
	if err != nil || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		t.Fatalf("query material escaped managed output: %s, %v", material.QueryPath(), err)
	}
	if material.SourcePath() != source || !strings.HasPrefix(material.Preview(), "# codrax-source: "+material.QueryPath()+"\n") {
		t.Fatalf("original and query roles were mixed: source=%s query=%s preview=%q", material.SourcePath(), material.QueryPath(), material.Preview())
	}
	if adjacent, err := filepath.Glob(source + ".*"); err != nil || len(adjacent) != 0 {
		t.Fatalf("automatic conversion wrote beside original: %v, %v", adjacent, err)
	}
	if current, err := os.ReadFile(source); err != nil || !bytes.Equal(current, original) {
		t.Fatalf("original binary was modified: %v", err)
	}
	snapshot, err := tracebundle.Open(stdcontext.Background(), material.QueryPath())
	if err != nil {
		t.Fatal(err)
	}
	var metadata attachedTraceBundleMetadata
	decodeErr := snapshot.Decode(&metadata)
	validateErr := snapshot.Validate()
	closeErr := snapshot.Close()
	if decodeErr != nil || validateErr != nil || closeErr != nil {
		t.Fatalf("inspect complete receipt-bound bundle: %v / %v / %v", decodeErr, validateErr, closeErr)
	}
	if metadata.Systrace != "" || hitraceconv.QueryReadyPerfTracePath(metadata.Artifacts) == "" {
		t.Fatalf("fixture must have only queryable samples, not a scheduling trace: %+v", metadata)
	}
	for _, artifact := range metadata.Artifacts {
		if artifact.Type == hitraceconv.ArtifactSystrace {
			t.Fatal("SIMPLEPERF fixture unexpectedly supplied scheduling material")
		}
	}
	index, err := tracequery.BuildIndex(stdcontext.Background(), material.QueryPath())
	if err != nil || len(index.Events) != 80 {
		t.Fatalf("complete sample material was clipped to preview: index=%+v err=%v", index, err)
	}
	for _, event := range index.Events {
		if event.PerfFields.Source != "simpleperf_report_proto" || event.PerfFields.CPUKnown == nil || *event.PerfFields.CPUKnown {
			t.Fatalf("sample-only fixture gained source/CPU capability: %+v", event)
		}
	}
	for _, lane := range []struct {
		name  string
		state attachedRuntimeTriageState
		opts  attachedTraceRenderOptions
	}{
		{"producer", attachedTriageProducer, attachedTraceRenderOptions{ReadFileAvailable: true, Material: material}},
		{"query", attachedTriageUnavailable, attachedTraceRenderOptions{PreferTraceQuery: true, Material: material}},
	} {
		t.Run(lane.name, func(t *testing.T) {
			prompt := formatAttachedTrace(material.Preview(), dir, lane.state, "", lane.opts)
			for _, want := range []string{
				"Original Trace source: `" + source + "`",
				"Complete query material: `" + material.QueryPath() + "`",
				"bounded preview", "not the complete trace", "preview-local",
				"Conversion alone proves no scheduling, wakeup or frame-causal coverage",
				"does not provide scheduling intervals, wakeup edges or frame-causal evidence",
				"are event/sample weights, not elapsed time", "cpu=-1", "cpu_known=false",
			} {
				if !strings.Contains(prompt, want) {
					t.Errorf("prepared sample prompt missing %q:\n%s", want, prompt)
				}
			}
			if !strings.Contains(strings.ToLower(prompt), "sample-only") {
				t.Errorf("official SIMPLEPERF needs an explicit sample-only capability disclosure:\n%s", prompt)
			}
			for _, misleading := range []string{
				"The complete trace is saved",
				"The raw fenced block below carries tracebundle metadata",
				"Prefer `trace_query` for scheduler state, wakeup chains",
			} {
				if strings.Contains(prompt, misleading) {
					t.Errorf("sample-only preview received an unsupported instruction %q:\n%s", misleading, prompt)
				}
			}
		})
	}
	if _, err := os.Stat(filepath.Join(dir, AttachedTraceBlobName)); !os.IsNotExist(err) {
		t.Fatalf("sample preview was published as a complete query blob: %v", err)
	}
}

// Real SIMPLEPERF v1 report-sample protobuf bytes. Production Prepare and the
// actual converter are used above; no test converter or provider is injected.
func preparedPromptSimpleperfFixture(samples int) []byte {
	var out bytes.Buffer
	out.WriteString("SIMPLEPERF\x01\x00")
	varint := func(field int, value uint64) []byte {
		return binary.AppendUvarint(binary.AppendUvarint(nil, uint64(field<<3)), value)
	}
	message := func(field int, parts ...[]byte) []byte {
		body := bytes.Join(parts, nil)
		data := binary.AppendUvarint(nil, uint64(field<<3|2))
		data = binary.AppendUvarint(data, uint64(len(body)))
		return append(data, body...)
	}
	record := func(body []byte) {
		var length [4]byte
		binary.LittleEndian.PutUint32(length[:], uint32(len(body)))
		out.Write(length[:])
		out.Write(body)
	}
	record(message(5, message(1, []byte("cpu-cycles"))))
	record(message(3, varint(1, 0), message(2, []byte("/system/lib64/libfoo.so")), message(3, []byte("main"))))
	record(message(4, varint(1, 42), varint(2, 42), message(3, []byte("worker"))))
	for sample := 0; sample < samples; sample++ {
		record(message(1,
			varint(1, 1_000_000_000+uint64(sample)*1_000_000), varint(2, 42),
			message(3, varint(1, 0x1234), varint(2, 0), varint(3, 0)),
			varint(4, 99), varint(5, 0),
		))
	}
	out.Write(make([]byte, 4))
	return out.Bytes()
}
