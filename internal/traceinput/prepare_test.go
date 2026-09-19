package traceinput

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestPrepareTextSYSKeepsCompleteQueryAndBoundedPreview(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "text.sys")
	body := strings.Repeat("# 文本 annotation\n", 150) + "worker-42 ( 42) [000] d..2 9.000001: sched_wakeup: comm=tail pid=99 prio=120 target_cpu=000\n"
	writeTestFile(t, path, []byte(body))
	called := false
	material, err := prepare(context.Background(), Options{InputPath: path, PreviewBytes: 512}, func(context.Context, hitraceconv.Options) (hitraceconv.Result, error) {
		called = true
		return hitraceconv.Result{}, errors.New("text must not convert")
	})
	if err != nil {
		t.Fatal(err)
	}
	if called || material.SourcePath() != path || material.QueryPath() != path {
		t.Fatalf("text passthrough changed identity or called converter: %+v", material)
	}
	if material.SelfContainedText() {
		t.Fatal("truncated plain-text preview became a complete snapshot")
	}
	if len(material.Preview()) > 512 || !utf8.ValidString(material.Preview()) || !strings.Contains(material.Preview(), "truncated; full query material retained") || strings.Contains(material.Preview(), "comm=tail") {
		t.Fatalf("bad bounded preview: %q", material.Preview())
	}
	idx, err := tracequery.BuildIndex(context.Background(), material.QueryPath())
	if err != nil || len(idx.Events) != 1 || idx.Events[0].WakeePID != 99 {
		t.Fatalf("full query lost tail: index=%+v err=%v", idx, err)
	}
	writeTestFile(t, path, []byte(strings.Replace(body, "pid=99", "pid=98", 1)))
	if err := material.Validate(context.Background(), material.Preview()); err == nil {
		t.Fatal("source rewrite retained prepared authority")
	}
}

func TestPrepareRejectsNonConvertibleInputsWithoutConverter(t *testing.T) {
	for _, tc := range []struct {
		name string
		body []byte
	}{
		{"empty.sys", nil},
		{"driver.sys", []byte("MZ\x00\xffdriver")},
		{"sqlite.sys", []byte("SQLite format 3\x00payload")},
		{"late_binary.sys", append(bytes.Repeat([]byte("# text\n"), 12000), 0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), tc.name)
			writeTestFile(t, path, tc.body)
			material, err := prepare(context.Background(), Options{InputPath: path, PreviewBytes: 512}, func(context.Context, hitraceconv.Options) (hitraceconv.Result, error) {
				t.Fatal("unsupported input was sent to converter")
				return hitraceconv.Result{}, nil
			})
			if err == nil || material != nil {
				t.Fatalf("unsupported input admitted: %+v %v", material, err)
			}
		})
	}
}

func TestPrepareTrueRMQAutoConvertsAndQueriesTail(t *testing.T) {
	dir := t.TempDir()
	sourceDir := filepath.Join(dir, "只读 capture")
	if err := os.Mkdir(sourceDir, 0o700); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(sourceDir, "原始 capture.data")
	body := realRMQFixture(40)
	writeTestFile(t, input, body)
	if err := os.Chmod(sourceDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sourceDir, 0o700) })
	anchor := filepath.Join(dir, "runtime")
	t.Setenv("CODRAX_TRACE_STREAMER", filepath.Join(dir, "no-trace-streamer"))
	material, err := Prepare(context.Background(), Options{InputPath: input, RuntimeAnchor: anchor, PreviewBytes: 640})
	if err != nil {
		t.Fatal(err)
	}
	canonicalAnchor, err := filepath.EvalSymlinks(anchor)
	if err != nil {
		t.Fatal(err)
	}
	if material.SourcePath() != input || !strings.HasPrefix(material.QueryPath(), canonicalAnchor+string(filepath.Separator)) || !strings.HasSuffix(material.QueryPath(), ".tracebundle.json") {
		t.Fatalf("wrong original/query provenance: source=%s query=%s", material.SourcePath(), material.QueryPath())
	}
	if len(material.Preview()) > 640 || !strings.HasPrefix(material.Preview(), "# codrax-source: "+material.QueryPath()+"\n") || strings.Contains(material.Preview(), "pid=139") || !strings.Contains(material.Preview(), "truncated") {
		t.Fatalf("preview does not describe bounded query material: %q", material.Preview())
	}
	idx, err := tracequery.BuildIndex(context.Background(), material.QueryPath())
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Events) != 40 || idx.Events[39].WakeePID != 139 {
		t.Fatalf("complete binary conversion lost tail: %+v", idx.Events)
	}
	if material.SelfContainedText() {
		t.Fatal("converted trace minted plain-text snapshot completeness")
	}
	fullConverted, err := Prepare(context.Background(), Options{InputPath: input, RuntimeAnchor: anchor, PreviewBytes: 1 << 20})
	if err != nil || fullConverted.SelfContainedText() {
		t.Fatalf("untruncated conversion must still retain its artifact provenance: %v %v", fullConverted, err)
	}
	bundleMaterial, err := Prepare(context.Background(), Options{InputPath: material.QueryPath(), PreviewBytes: 1 << 20})
	if err != nil || bundleMaterial.SelfContainedText() {
		t.Fatalf("complete bundle JSON must not become self-contained trace text: %v %v", bundleMaterial, err)
	}
	receiptPath := filepath.Join(filepath.Dir(material.QueryPath()), "preparation.json")
	data, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	var receipt preparationReceipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.SourcePath != input || receipt.SourceSHA256 == "" || receipt.SourceBytes != int64(len(body)) || receipt.Conversion == nil || len(receipt.Conversion.TraceDecisions) == 0 || len(receipt.Conversion.TraceCoverage) == 0 {
		t.Fatalf("conversion provenance/decisions/coverage missing: %+v", receipt)
	}
	if got, err := os.ReadFile(input); err != nil || !bytes.Equal(got, body) {
		t.Fatalf("original modified: %v", err)
	}
	if files, err := os.ReadDir(sourceDir); err != nil || len(files) != 1 || files[0].Name() != filepath.Base(input) {
		t.Fatalf("automatic preparation published adjacent to source: %v, err=%v", files, err)
	}
	writeTestFile(t, receipt.PreviewPath, []byte("# replaced derived capture\n"))
	if err := material.Validate(context.Background(), material.Preview()); err == nil {
		t.Fatal("changed bundle child retained prepared authority")
	}
}

func TestPrepareCompletePlainTextMintsTypedSnapshotCompleteness(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plain.sys")
	body := "# codrax-preview: truncated is merely source prose\nworker-42 (42) [000] .... 1.000000: sched_wakeup: comm=main pid=99 prio=120 target_cpu=000\n"
	writeTestFile(t, path, []byte(body))
	material, err := Prepare(context.Background(), Options{InputPath: path, PreviewBytes: len("# codrax-source: "+path+"\n") + len(body)})
	if err != nil || !material.SelfContainedText() {
		t.Fatalf("exact full text envelope did not retain legacy snapshot capability: %v %v", material, err)
	}
	if material.Preview() != "# codrax-source: "+path+"\n"+body {
		t.Fatal("complete text bytes were changed")
	}
}

func TestPrepareCancellationAndFailureCleanOnlyOwnedOutput(t *testing.T) {
	for _, cancelDuring := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelDuring), func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "capture.sys")
			writeTestFile(t, input, realRMQFixture(1))
			anchor := filepath.Join(dir, "runtime")
			if err := os.MkdirAll(anchor, 0o700); err != nil {
				t.Fatal(err)
			}
			prior := filepath.Join(anchor, "prior-success.systrace")
			writeTestFile(t, prior, []byte("keep existing"))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			failure := errors.New("converter failed")
			calls := 0
			material, err := prepare(ctx, Options{InputPath: input, RuntimeAnchor: anchor}, func(ctx context.Context, opts hitraceconv.Options) (hitraceconv.Result, error) {
				calls++
				if opts.TraceEngine != "auto" || opts.InputPath != input || filepath.Dir(opts.OutputPath) == filepath.Dir(input) {
					t.Fatalf("wrong converter boundary: %+v", opts)
				}
				writeTestFile(t, opts.OutputPath, []byte("owned partial"))
				if cancelDuring {
					cancel()
					return hitraceconv.Result{}, ctx.Err()
				}
				return hitraceconv.Result{}, failure
			})
			if calls != 1 || material != nil || err == nil || cancelDuring && !errors.Is(err, context.Canceled) || !cancelDuring && !errors.Is(err, failure) {
				t.Fatalf("wrong failure/cancel publication: calls=%d material=%v err=%v", calls, material, err)
			}
			if matches, _ := filepath.Glob(filepath.Join(anchor, "trace-input-*")); len(matches) != 0 {
				t.Fatalf("owned failed preparation leaked: %v", matches)
			}
			if data, err := os.ReadFile(prior); err != nil || string(data) != "keep existing" {
				t.Fatalf("existing successful material touched: %s %v", data, err)
			}
			if data, err := os.ReadFile(input); err != nil || !bytes.Equal(data, realRMQFixture(1)) {
				t.Fatalf("source touched: %v", err)
			}
		})
	}
}

func TestPrepareTrueSimpleperfIsSampleOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "capture.arbitrary-extension")
	writeTestFile(t, path, simpleperfFixture())
	material, err := Prepare(context.Background(), Options{InputPath: path, RuntimeAnchor: filepath.Join(dir, "runtime")})
	if err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), material.QueryPath())
	if err != nil || len(idx.Events) != 1 {
		t.Fatalf("simpleperf sample query: index=%+v err=%v", idx, err)
	}
	if idx.Events[0].PerfFields.Source != "simpleperf_report_proto" || idx.Events[0].PerfFields.CPUKnown == nil || *idx.Events[0].PerfFields.CPUKnown {
		t.Fatalf("simpleperf gained false CPU/trace capability: %+v", idx.Events[0])
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(material.QueryPath()), "preparation.json"))
	if err != nil {
		t.Fatal(err)
	}
	var receipt preparationReceipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Conversion == nil || hitraceconv.QueryReadySystracePath(*receipt.Conversion) != "" || hitraceconv.QueryReadyPerfTracePath(receipt.Conversion.Artifacts) == "" {
		t.Fatalf("sample-only conversion gained scheduling capability: %+v", receipt)
	}
	canonicalAnchor, err := filepath.EvalSymlinks(filepath.Join(dir, "runtime"))
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Conversion.InputPath != path || !strings.HasPrefix(material.QueryPath(), canonicalAnchor+string(filepath.Separator)) {
		t.Fatalf("direct perf lost original provenance or escaped managed output: %+v", receipt)
	}
	if matches, _ := filepath.Glob(path + ".*"); len(matches) != 0 {
		t.Fatalf("direct perf wrote original-adjacent output: %v", matches)
	}
}

func TestPrepareProgressCancellationRemovesManagedOutput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "capture.sys")
	writeTestFile(t, path, realRMQFixture(2))
	anchor := filepath.Join(dir, "runtime")
	t.Setenv("CODRAX_TRACE_STREAMER", filepath.Join(dir, "no-trace-streamer"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	seen := false
	material, err := Prepare(ctx, Options{InputPath: path, RuntimeAnchor: anchor, Progress: func(hitraceconv.ProgressEvent) {
		seen = true
		cancel()
	}})
	if !seen || material != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("preparation progress cancellation ignored: seen=%v material=%v err=%v", seen, material, err)
	}
	if matches, _ := filepath.Glob(filepath.Join(anchor, "trace-input-*")); len(matches) != 0 {
		t.Fatalf("canceled converter output leaked: %v", matches)
	}
}

func TestPreparePreCanceledAndTooSmallPreview(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if material, err := Prepare(ctx, Options{InputPath: "missing"}); material != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-cancellation: %v %v", material, err)
	}
	path := filepath.Join(t.TempDir(), "text.sys")
	writeTestFile(t, path, []byte("# trace text\n"))
	if material, err := Prepare(context.Background(), Options{InputPath: path, PreviewBytes: 1}); material != nil || err == nil {
		t.Fatalf("tiny cap silently exceeded: %v %v", material, err)
	}
}

func TestPrepareInventoryIsNotQueryReadyAndSourceSwapDominates(t *testing.T) {
	for _, swap := range []bool{false, true} {
		t.Run(fmt.Sprint(swap), func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "capture.sys")
			writeTestFile(t, input, realRMQFixture(1))
			material, err := prepare(context.Background(), Options{InputPath: input, RuntimeAnchor: filepath.Join(dir, "runtime")}, func(context.Context, hitraceconv.Options) (hitraceconv.Result, error) {
				if swap {
					writeTestFile(t, input, realRMQFixture(2))
				}
				return hitraceconv.Result{EventsWritten: 100, OutputPath: "pretend.systrace"}, nil
			})
			if material != nil || err == nil {
				t.Fatalf("inventory/source swap admitted: %v %v", material, err)
			}
			if swap && !strings.Contains(err.Error(), "generation changed") || !swap && !strings.Contains(err.Error(), "no_query_ready_material") {
				t.Fatalf("wrong authoritative failure: %v", err)
			}
		})
	}
}

func TestBinaryCandidateIncludesExistingConverterFamilies(t *testing.T) {
	for _, prefix := range [][]byte{[]byte("SIMPLEPERF"), {0x49, 0xdf}, {0xce, 0x0a}, []byte("OHOSPROF"), []byte("PERFILE2"), {0x1f, 0x8b}, []byte("PK\x03\x04")} {
		if kind, candidate := binaryCandidate(prefix); kind == "" || !candidate {
			t.Fatalf("existing converter family omitted: %q", prefix)
		}
	}
	if _, candidate := binaryCandidate([]byte("# textual capture.sys\n")); candidate {
		t.Fatal("text classified by suffix")
	}
}

func writeTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// Real RMQ wire bytes, not a converter stub. The public converter's existing
// strict decoder and receipts are exercised by the integration test above.
func realRMQFixture(pages int) []byte {
	var out bytes.Buffer
	header := make([]byte, 12)
	binary.LittleEndian.PutUint16(header, 0x0ace)
	header[2] = 1
	binary.LittleEndian.PutUint16(header[4:], 1)
	binary.LittleEndian.PutUint32(header[8:], 2)
	out.Write(header)
	segment := func(kind uint32, body []byte) {
		var head [8]byte
		binary.LittleEndian.PutUint32(head[:4], kind)
		binary.LittleEndian.PutUint32(head[4:], uint32(len(body)))
		out.Write(head[:])
		out.Write(body)
	}
	format := "name: sched_wakeup\nID: 10\nformat:\n"
	for _, field := range []struct {
		name         string
		offset, size int
	}{
		{"unsigned short common_type", 0, 2}, {"unsigned char common_flags", 2, 1},
		{"unsigned char common_preempt_count", 3, 1}, {"int common_pid", 4, 4},
		{"char comm[16]", 8, 16}, {"int pid", 24, 4}, {"int prio", 28, 4}, {"int target_cpu", 32, 4},
	} {
		signed := 0
		if strings.HasPrefix(field.name, "int ") {
			signed = 1
		}
		format += fmt.Sprintf("\tfield:%s;\toffset:%d;\tsize:%d;\tsigned:%d;\n", field.name, field.offset, field.size, signed)
	}
	format += "print fmt: \"comm=%s pid=%d prio=%d target_cpu=%03d\"\n"
	segment(1, []byte(format))
	segment(2, []byte("42 worker\n"))
	segment(3, []byte("42 42\n"))
	var raw bytes.Buffer
	for i := 0; i < pages; i++ {
		page := make([]byte, 4096)
		binary.LittleEndian.PutUint64(page, uint64(i+1)*1_000_000_000)
		binary.LittleEndian.PutUint64(page[8:], 42)
		binary.LittleEndian.PutUint16(page[21:], 36)
		payload := page[23:59]
		binary.LittleEndian.PutUint16(payload, 10)
		binary.LittleEndian.PutUint32(payload[4:], 42)
		copy(payload[8:24], "worker")
		binary.LittleEndian.PutUint32(payload[24:], uint32(100+i))
		binary.LittleEndian.PutUint32(payload[28:], 120)
		raw.Write(page)
	}
	segment(4, raw.Bytes())
	return out.Bytes()
}

func simpleperfFixture() []byte {
	var out bytes.Buffer
	out.WriteString("SIMPLEPERF\x01\x00")
	varint := func(field int, value uint64) []byte {
		data := binary.AppendUvarint(nil, uint64(field<<3))
		return binary.AppendUvarint(data, value)
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
	record(message(1, varint(1, 1_234_567_000), varint(2, 42), message(3, varint(1, 0x1234), varint(2, 0), varint(3, 0)), varint(4, 99), varint(5, 0)))
	out.Write(make([]byte, 4))
	return out.Bytes()
}
