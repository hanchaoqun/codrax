package traceinput

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func gzipPreparationBytes(t *testing.T, body []byte) []byte {
	t.Helper()
	var compressed bytes.Buffer
	w := gzip.NewWriter(&compressed)
	if _, err := w.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return compressed.Bytes()
}

func gzipPreparationBody() []byte {
	return []byte(strings.Repeat("# 文本 annotation: preserve bytes and timestamps\n", 150) + "worker-42 (42) [000] d..2 9.000001: sched_wakeup: comm=tail pid=99 prio=120 target_cpu=000\n")
}

func TestPrepareGzipTextKeepsCompleteBytesAndTransportProvenance(t *testing.T) {
	dir := t.TempDir()
	sourceDir := filepath.Join(dir, "只读 capture")
	if err := os.Mkdir(sourceDir, 0700); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(sourceDir, "原始 capture.arbitrary")
	body := gzipPreparationBody()
	original := gzipPreparationBytes(t, body)
	writeTestFile(t, input, original)
	if err := os.Chmod(sourceDir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sourceDir, 0700) })
	t.Setenv("CODRAX_TRACE_STREAMER", filepath.Join(dir, "missing-streamer"))
	material, err := Prepare(context.Background(), Options{InputPath: input, RuntimeAnchor: filepath.Join(dir, "runtime"), PreviewBytes: 640})
	if err != nil {
		t.Fatal(err)
	}
	if material.SourcePath() != input || material.QueryPath() == input || material.SelfContainedText() {
		t.Fatalf("wrong original/transport identity: %+v", material)
	}
	if len(material.Preview()) > 640 || strings.Contains(material.Preview(), "comm=tail") || !strings.Contains(material.Preview(), "truncated") {
		t.Fatalf("preview is not bounded separately: %q", material.Preview())
	}
	decoded, err := os.ReadFile(material.QueryPath())
	if err != nil || !bytes.Equal(decoded, body) {
		t.Fatalf("decoded bytes changed: %v", err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), material.QueryPath())
	if err != nil || len(idx.Events) != 1 || idx.Events[0].WakeePID != 99 || idx.Events[0].Ts != 9.000001 {
		t.Fatalf("complete tail or time lost: %+v %v", idx, err)
	}
	receiptBytes, err := os.ReadFile(filepath.Join(filepath.Dir(material.QueryPath()), "preparation.json"))
	if err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		SourcePath string `json:"source_path"`
		SourceSHA  string `json:"source_sha256"`
		Transport  struct {
			Profile    string `json:"profile"`
			DecodedSHA string `json:"decoded_sha256"`
		} `json:"transport"`
	}
	if err := json.Unmarshal(receiptBytes, &receipt); err != nil {
		t.Fatal(err)
	}
	compressedSHA, decodedSHA := sha256.Sum256(original), sha256.Sum256(body)
	if receipt.SourcePath != input || receipt.SourceSHA != hex.EncodeToString(compressedSHA[:]) || receipt.Transport.Profile != "gzip_trace_text_v1" || receipt.Transport.DecodedSHA != hex.EncodeToString(decodedSHA[:]) {
		t.Fatalf("missing exact byte transport provenance: %+v", receipt)
	}
	if got, err := os.ReadFile(input); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("original changed: %v", err)
	}
	if files, err := os.ReadDir(sourceDir); err != nil || len(files) != 1 {
		t.Fatalf("published next to readonly source: %v %v", files, err)
	}
}

func TestGzipPreparationCommitAndWarmCoordinatorRejectChangedGenerations(t *testing.T) {
	for _, phase := range []string{"before_commit", "warm_hit"} {
		for _, target := range []string{"original", "decoded", "receipt"} {
			t.Run(phase+"/"+target, func(t *testing.T) {
				dir := t.TempDir()
				input := filepath.Join(dir, "capture.sys.gz")
				writeTestFile(t, input, gzipPreparationBytes(t, gzipPreparationBody()))
				opts := Options{InputPath: input, RuntimeAnchor: filepath.Join(dir, "runtime")}
				p, err := Begin(context.Background(), opts)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = p.Discard() })
				material := p.material
				coordinator := NewCoordinator(opts)
				if phase == "warm_hit" {
					if err := p.Discard(); err != nil {
						t.Fatal(err)
					}
					material, err = coordinator.Prepare(context.Background(), input)
					if err != nil {
						t.Fatal(err)
					}
				}
				path := material.QueryPath()
				if target == "original" {
					path = input
				}
				if target == "receipt" {
					path = filepath.Join(filepath.Dir(material.QueryPath()), "preparation.json")
				}
				writeTestFile(t, path, []byte("changed generation\n"))
				if phase == "before_commit" {
					if got, err := p.Commit(context.Background()); got != nil || err == nil {
						t.Fatal("changed material committed")
					}
					if _, err := os.Stat(p.owned.path); !os.IsNotExist(err) {
						t.Fatalf("failed commit leaked output: %v", err)
					}
				} else {
					for _, alias := range []string{input, material.QueryPath()} {
						if got, err := coordinator.Prepare(context.Background(), alias); got != nil || err == nil {
							t.Fatal("stale material reissued through alias")
						}
					}
				}
			})
		}
	}
}

func TestGzipPreparationCancelAndInvalidInputPublishNothing(t *testing.T) {
	for _, mode := range []string{"cancel_during_prepare", "cancel_before_commit", "crc", "late_nul"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			input, anchor := filepath.Join(dir, "capture.sys.gz"), filepath.Join(dir, "runtime")
			body := gzipPreparationBody()
			if mode == "late_nul" {
				body = append(body, 0)
			}
			original := gzipPreparationBytes(t, body)
			if mode == "crc" {
				original[len(original)-8] ^= 1
			}
			writeTestFile(t, input, original)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			opts := Options{InputPath: input, RuntimeAnchor: anchor}
			if mode == "cancel_during_prepare" {
				opts.Progress = func(hitraceconv.ProgressEvent) { cancel() }
			}
			p, err := Begin(ctx, opts)
			if mode == "cancel_before_commit" {
				if err != nil {
					t.Fatal(err)
				}
				cancel()
				_, err = p.Commit(context.Background())
			} else if p != nil {
				_ = p.Discard()
				t.Fatal("invalid/canceled input prepared")
			}
			if err == nil || strings.HasPrefix(mode, "cancel_") && !errors.Is(err, context.Canceled) {
				t.Fatalf("wrong failure: %v", err)
			}
			if matches, _ := filepath.Glob(filepath.Join(anchor, "trace-input-*")); len(matches) != 0 {
				t.Fatalf("failed preparation leaked: %v", matches)
			}
			if got, err := os.ReadFile(input); err != nil || !bytes.Equal(got, original) {
				t.Fatal("failure touched original", err)
			}
		})
	}
}

// The private capture stays outside the repository. Default runs explicitly
// skip this opt-in real-device receipt rather than equating synthetic legal
// format bytes with a device capture or a cross-platform acceptance run.
func TestPrepareGzipTextRepresentativeCapture(t *testing.T) {
	input := os.Getenv("CODRAX_TEST_GZIP_TRACE")
	if input == "" {
		t.Skip("set CODRAX_TEST_GZIP_TRACE to an authorized local gzip text capture")
	}
	material, err := Prepare(context.Background(), Options{InputPath: input, RuntimeAnchor: t.TempDir(), PreviewBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.Open(input)
	if err != nil {
		t.Fatal(err)
	}
	defer original.Close()
	reader, err := gzip.NewReader(original)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	wantSHA := sha256.New()
	wantBytes, err := io.Copy(wantSHA, reader)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := os.Open(material.QueryPath())
	if err != nil {
		t.Fatal(err)
	}
	defer decoded.Close()
	gotSHA := sha256.New()
	gotBytes, err := io.Copy(gotSHA, decoded)
	if err != nil || gotBytes != wantBytes || !bytes.Equal(wantSHA.Sum(nil), gotSHA.Sum(nil)) {
		t.Fatalf("real capture byte transport differs: got=%d want=%d err=%v", gotBytes, wantBytes, err)
	}
	parsed := 0
	idx, err := tracequery.StreamScan(context.Background(), material.QueryPath(), "", func(tracequery.Event) bool { parsed++; return true })
	if err != nil || parsed == 0 {
		t.Fatalf("real capture cannot be queried: %v", err)
	}
	if err := material.Validate(context.Background(), material.Preview()); err != nil {
		t.Fatal(err)
	}
	t.Logf("complete_bytes=%d parsed_events=%d scanned_lines=%d preview_bytes=%d unchanged_sha256=%x", gotBytes, parsed, idx.ScannedLineCount, len(material.Preview()), gotSHA.Sum(nil))
}

func TestGzipPreparationOversizedSourceRejectedBeforeHash(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("large sparse-file allocation semantics require native Windows fixture")
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "oversized.sys.gz")
	file, err := os.Create(input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte{0x1f, 0x8b, 8, 0}); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Truncate(hitraceconv.MaxGzipTraceInputBytes + 1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	material, err := Prepare(ctx, Options{InputPath: input, RuntimeAnchor: filepath.Join(dir, "runtime")})
	var rejected *hitraceconv.GzipTextTransportError
	if material != nil || !errors.As(err, &rejected) || rejected.Code != hitraceconv.GzipTextCodeResourceLimit {
		t.Fatalf("oversized original was hashed or converted before cap: %v %v", material, err)
	}
}

func TestGzipPreparationBinaryRoutingRequiresCompleteIntegrity(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		t.Run(map[bool]string{false: "intact", true: "corrupt_crc"}[corrupt], func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "perf.data.gz")
			payload := append([]byte("PERFILE2"), make([]byte, 192)...)
			compressed := gzipPreparationBytes(t, payload)
			if corrupt {
				compressed[len(compressed)-8] ^= 1
			}
			writeTestFile(t, input, compressed)
			calls := 0
			material, err := prepare(context.Background(), Options{InputPath: input, RuntimeAnchor: filepath.Join(dir, "runtime")}, func(ctx context.Context, opts hitraceconv.Options) (hitraceconv.Result, error) {
				calls++
				if opts.InputPath != input {
					t.Fatal("binary preparation replaced compressed source identity")
				}
				// The shared converter now owns the one complete inflate, integrity
				// check and semantic routing. A stub here cannot stand in for it.
				opts.DisablePerfAdapter = true
				return hitraceconv.PrepareFile(ctx, opts)
			})
			if material != nil || err == nil {
				t.Fatal("binary payload became plain text")
			}
			if corrupt {
				var rejected *hitraceconv.GzipTextTransportError
				if calls != 1 || !errors.As(err, &rejected) || rejected.Code != hitraceconv.GzipTextCodeIntegrity {
					t.Fatalf("corrupt binary escaped shared transport: calls=%d err=%v", calls, err)
				}
			} else {
				var rejected *hitraceconv.GzipTextTransportError
				if calls != 1 || errors.As(err, &rejected) {
					t.Fatalf("intact binary failed transport rather than semantic capability admission: calls=%d err=%v", calls, err)
				}
			}
		})
	}
}
