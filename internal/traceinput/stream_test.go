package traceinput

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestStreamCompleteBinaryTextGzipAndSQLiteQueryTail(t *testing.T) {
	text := []byte("app-42 [000] .... 1.000000: tracing_mark_write: B|42|load\napp-42 [000] .... 1.004000: tracing_mark_write: E|42\n")
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	_, _ = zw.Write(text)
	_ = zw.Close()
	db, err := os.ReadFile("../../eval/fixtures/hmosperf_referenced_dictionary/capture.data")
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string][]byte{"rmq": realRMQFixture(40), "text": text, "gzip": compressed.Bytes(), "sqlite": db} {
		t.Run(name, func(t *testing.T) {
			p, err := BeginStream(context.Background(), io.NopCloser(bytes.NewReader(body)), StreamOptions{Options: Options{RuntimeAnchor: t.TempDir(), PreviewBytes: 640}, MaxBytes: int64(len(body))})
			if err != nil {
				t.Fatal(err)
			}
			defer p.Discard()
			root := p.owned.path
			material, err := p.Commit(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(material.Preview()) > 640 {
				t.Fatal("unbounded preview")
			}
			idx, err := tracequery.BuildIndex(context.Background(), material.QueryPath())
			if err != nil || len(idx.Events) == 0 {
				t.Fatalf("full query material: %v", err)
			}
			if name == "rmq" && (len(idx.Events) != 40 || idx.Events[39].WakeePID != 139) {
				t.Fatal("binary tail truncated to model preview")
			}
			data, err := os.ReadFile(filepath.Join(root, "input-stream.json"))
			if err != nil {
				t.Fatal(err)
			}
			var receipt streamReceipt
			if err := json.Unmarshal(data, &receipt); err != nil {
				t.Fatal(err)
			}
			if !receipt.EOFConfirmed || receipt.SourceBytes != int64(len(body)) || receipt.InputByteLimit != int64(len(body)) || receipt.SourceSHA256 != fmt.Sprintf("%x", sha256.Sum256(body)) || receipt.SourceGeneration == "" {
				t.Fatalf("missing full-stream witness: %+v", receipt)
			}
			if err := p.Discard(); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(root); err != nil {
				t.Fatal("discard removed published output")
			}
			if err := os.WriteFile(filepath.Join(root, "input-stream.json"), []byte("changed"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := material.Validate(context.Background(), material.Preview()); err == nil {
				t.Fatal("changed EOF receipt retained query authority")
			}
		})
	}
}

type failedStream struct {
	body         []byte
	failure      error
	closeFailure error
}

func (s *failedStream) Read(p []byte) (int, error) {
	n := copy(p, s.body)
	s.body = s.body[n:]
	if len(s.body) == 0 {
		return n, s.failure
	}
	return n, nil
}
func (s *failedStream) Close() error { return s.closeFailure }

func TestStreamFailureNeverConvertsOrPublishesPrefix(t *testing.T) {
	body := realRMQFixture(1)
	broken := errors.New("transport broke")
	for _, tc := range []struct {
		name  string
		input io.ReadCloser
		limit int64
	}{
		{"limit", io.NopCloser(bytes.NewReader(body)), int64(len(body) - 1)},
		{"empty", io.NopCloser(strings.NewReader("")), 20},
		{"read error with bytes", &failedStream{body: body, failure: broken}, int64(len(body))},
		{"close error", &failedStream{body: body, failure: io.EOF, closeFailure: broken}, int64(len(body))},
		{"no progress", &failedStream{}, 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			anchor := t.TempDir()
			p, err := beginStream(context.Background(), tc.input, StreamOptions{Options: Options{RuntimeAnchor: anchor}, MaxBytes: tc.limit}, func(context.Context, hitraceconv.Options) (hitraceconv.Result, error) {
				t.Fatal("prefix reached converter")
				return hitraceconv.Result{}, nil
			})
			if err == nil || p != nil {
				t.Fatal("failed stream published")
			}
			files, err := os.ReadDir(anchor)
			if err != nil || len(files) != 0 {
				t.Fatalf("private partial output leaked: %v %v", files, err)
			}
		})
	}
}

func TestStreamCancellationUnblocksReadAndRollsBackBeforeCommit(t *testing.T) {
	for _, late := range []bool{false, true} {
		t.Run(fmt.Sprint(late), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			anchor := t.TempDir()
			if late {
				p, err := BeginStream(ctx, io.NopCloser(bytes.NewReader(realRMQFixture(1))), StreamOptions{Options: Options{RuntimeAnchor: anchor}})
				if err != nil {
					t.Fatal(err)
				}
				defer p.Discard()
				cancel()
				if material, err := p.Commit(context.Background()); material != nil || !errors.Is(err, context.Canceled) {
					t.Fatalf("late cancel committed: %v", err)
				}
			} else {
				reader, writer := io.Pipe()
				defer writer.Close()
				done := make(chan error, 1)
				go func() {
					p, err := BeginStream(ctx, reader, StreamOptions{Options: Options{RuntimeAnchor: anchor}})
					if p != nil {
						_ = p.Discard()
					}
					done <- err
				}()
				// A successful write proves that the spool is already reading.
				if _, err := writer.Write([]byte("prefix")); err != nil {
					t.Fatal(err)
				}
				cancel()
				select {
				case err := <-done:
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("cancel lost: %v", err)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("blocked read survived cancellation")
				}
			}
			files, err := os.ReadDir(anchor)
			if err != nil || len(files) != 0 {
				t.Fatalf("cancel leaked unpublished files: %v %v", files, err)
			}
		})
	}
}

func TestStreamMalformedBinaryAndChangedSealedInputRollBack(t *testing.T) {
	anchor := t.TempDir()
	if p, err := BeginStream(context.Background(), io.NopCloser(strings.NewReader("OHOSPROF\x00truncated")), StreamOptions{Options: Options{RuntimeAnchor: anchor}}); err == nil || p != nil {
		t.Fatal("malformed complete stream published")
	}
	files, err := os.ReadDir(anchor)
	if err != nil || len(files) != 0 {
		t.Fatalf("conversion failure leaked: %v %v", files, err)
	}
	p, err := BeginStream(context.Background(), io.NopCloser(bytes.NewReader(realRMQFixture(1))), StreamOptions{Options: Options{RuntimeAnchor: anchor}})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Discard()
	if err := os.WriteFile(p.material.SourcePath(), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if material, err := p.Commit(context.Background()); err == nil || material != nil {
		t.Fatal("changed sealed source committed")
	}
	files, err = os.ReadDir(anchor)
	if err != nil || len(files) != 0 {
		t.Fatalf("uncommitted input leaked: %v %v", files, err)
	}
}
