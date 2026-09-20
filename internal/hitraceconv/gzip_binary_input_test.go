package hitraceconv

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

func gzipBinaryTestSource(t *testing.T, encoded []byte) (Options, *conversionInputAuthority, *privateConversionDir) {
	t.Helper()
	opts := gzipTextOptions(t, encoded)
	source, err := openConversionInputAuthority(opts.InputPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := source.Close(); err != nil {
			t.Error(err)
		}
	})
	staging, err := newRuntimePrivateConversionDir(opts.RuntimeAnchor, ".codrax-gzip-input-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := staging.FinalizeCleanup(); err != nil {
			t.Error(err)
		}
	})
	return opts, source, staging
}

func TestGzipBinaryInputRetainsAnyPayloadAndExactHeldReceipt(t *testing.T) {
	for _, body := range [][]byte{nil, []byte("# tracer: nop\n"), []byte("PERFILE2\x00\xffpayload"), []byte("OHOSPROF\x00body"), {0x1f, 0x8b, 8, 0}, []byte("SQLite format 3\x00")} {
		t.Run(hex.EncodeToString(body), func(t *testing.T) {
			encoded := gzipTextFixture(t, body, gzip.DefaultCompression)
			opts, source, staging := gzipBinaryTestSource(t, encoded)
			input, receipt, err := prepareGzipBinaryInput(context.Background(), opts, source, staging)
			if err != nil {
				t.Fatal(err)
			}
			defer input.Close()
			sourceSHA, decodedSHA := sha256.Sum256(encoded), sha256.Sum256(body)
			if receipt.SourceBytes != int64(len(encoded)) || receipt.SourceSHA256 != hex.EncodeToString(sourceSHA[:]) || receipt.DecodedBytes != int64(len(body)) || receipt.DecodedSHA256 != hex.EncodeToString(decodedSHA[:]) || receipt.DecodedGeneration == "" || receipt.DecodedGeneration != input.snapshot.identity.CacheToken() {
				t.Fatalf("receipt mismatch: %+v", receipt)
			}
			if input.DisplayPath() != opts.InputPath || input.traceStreamerSnapshotLeaf() != gzipBinaryInputSnapshotLeaf || input.Size() != int64(len(body)) {
				t.Fatalf("view mislabeled source: %+v", input)
			}
			if err := input.Validate(conversionInputStageRoute); err != nil {
				t.Fatal(err)
			}
			got, err := io.ReadAll(io.NewSectionReader(input, 0, input.Size()))
			if err != nil || !bytes.Equal(got, body) {
				t.Fatalf("held bytes changed: %v", err)
			}
			if err := input.withOpenFile(func(held *os.File) error {
				actual := make([]byte, len(body))
				_, err := held.ReadAt(actual, 0)
				if err != nil && err != io.EOF {
					return err
				}
				if !bytes.Equal(actual, body) {
					return errors.New("held descriptor differs from logical input")
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(opts.OutputPath); !os.IsNotExist(err) {
				t.Fatalf("decoder preparation published output: %v", err)
			}
			if err := input.Close(); err != nil {
				t.Fatal(err)
			}
			if err := source.Validate(conversionInputStageRoute); err != nil {
				t.Fatalf("closing decoded input closed source: %v", err)
			}
			if err := staging.Validate(); err != nil {
				t.Fatalf("closing input claimed directory ownership: %v", err)
			}
			if err := input.Validate(conversionInputStageRoute); err == nil {
				t.Fatal("closed input remains valid")
			}
		})
	}
}

func TestGzipBinaryInputReadAtBoundsAndSourceInvalidation(t *testing.T) {
	opts, source, staging := gzipBinaryTestSource(t, gzipTextFixture(t, []byte("PERFILE2body"), gzip.DefaultCompression))
	input, _, err := prepareGzipBinaryInput(context.Background(), opts, source, staging)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	var buf [32]byte
	if n, err := input.ReadAt(buf[:], input.Size()-2); n != 2 || err != io.EOF || string(buf[:n]) != "dy" {
		t.Fatalf("overrun: n=%d err=%v body=%q", n, err, buf[:n])
	}
	if n, err := input.ReadAt(buf[:], input.Size()); n != 0 || err != io.EOF {
		t.Fatalf("EOF: %d %v", n, err)
	}
	if _, err := input.ReadAt(buf[:], -1); err == nil {
		t.Fatal("negative offset admitted")
	}
	if err := os.Rename(opts.InputPath, opts.InputPath+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(opts.InputPath, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := input.Validate(conversionInputStageBuiltinMetadata); err == nil {
		t.Fatal("decoded view outlived original generation")
	}
}

func TestGzipBinaryInputLateSnapshotReplacementFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("held snapshot deny-delete is the Windows exclusion; Unix replacement arm here")
	}
	body := []byte("PERFILE2body")
	opts, source, staging := gzipBinaryTestSource(t, gzipTextFixture(t, body, gzip.DefaultCompression))
	opts.Progress = func(event ProgressEvent) {
		if event.Stage != "gzip_input_decompress" || event.Status != ProgressStatusComplete {
			return
		}
		path, err := staging.ChildPath(gzipBinaryInputSnapshotLeaf)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(path, path+".old"); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	input, receipt, err := prepareGzipBinaryInput(context.Background(), opts, source, staging)
	if err == nil || input != nil || receipt != (gzipInputReceipt{}) {
		t.Fatalf("same-byte replacement inherited receipt: input=%v receipt=%+v err=%v", input, receipt, err)
	}
	var rejected *GzipTextTransportError
	if errors.As(err, &rejected) {
		t.Fatalf("generation change reported as data failure: %v", err)
	}
}

func TestGzipBinaryInputIntegrityAndCancellationReturnNoAuthority(t *testing.T) {
	valid := gzipTextFixture(t, []byte("PERFILE2body"), gzip.DefaultCompression)
	badCRC := append([]byte(nil), valid...)
	badCRC[len(badCRC)-8] ^= 1
	for _, tc := range []struct {
		name   string
		body   []byte
		cancel bool
		code   string
	}{
		{"crc", badCRC, false, GzipTextCodeIntegrity},
		{"truncated", valid[:len(valid)-1], false, GzipTextCodeIntegrity},
		{"multiple", append(append([]byte(nil), valid...), valid...), false, GzipTextCodeTrailingData},
		{"trailing", append(append([]byte(nil), valid...), 0), false, GzipTextCodeTrailingData},
		{"ratio", gzipTextFixture(t, bytes.Repeat([]byte{0}, 8<<20), gzip.BestCompression), false, GzipTextCodeResourceLimit},
		{"cancel", valid, true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts, source, staging := gzipBinaryTestSource(t, tc.body)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancel {
				opts.Progress = func(event ProgressEvent) {
					if event.Stage == "gzip_input_decompress" {
						cancel()
					}
				}
			}
			input, receipt, err := prepareGzipBinaryInput(ctx, opts, source, staging)
			if err == nil || input != nil || receipt != (gzipInputReceipt{}) {
				t.Fatalf("failure granted authority: %v %+v %v", input, receipt, err)
			}
			var rejected *GzipTextTransportError
			if tc.cancel {
				if !errors.Is(err, context.Canceled) || errors.As(err, &rejected) {
					t.Fatalf("cancellation mislabeled: %v", err)
				}
			} else if !errors.As(err, &rejected) || rejected.Code != tc.code {
				t.Fatalf("wrong data verdict: %v", err)
			}
			if err := staging.FinalizeCleanup(); err != nil {
				t.Fatalf("failure retained snapshot handle: %v", err)
			}
			if err := source.Validate(conversionInputStageRoute); err != nil {
				t.Fatalf("failure closed original: %v", err)
			}
		})
	}
}

type gzipCountingSource struct {
	conversionInputView
	readBytes int64
}

func (source *gzipCountingSource) ReadAt(buffer []byte, offset int64) (int, error) {
	n, err := source.conversionInputView.ReadAt(buffer, offset)
	source.readBytes += int64(n)
	return n, err
}

func TestPublishGzipInputTextKeepsInputAliveWithoutReinflating(t *testing.T) {
	body := []byte("# tracer: nop\n" + strings.Repeat("# complete original text\n", 100))
	opts, source, staging := gzipBinaryTestSource(t, gzipTextFixture(t, body, gzip.DefaultCompression))
	counted := &gzipCountingSource{conversionInputView: source}
	input, receipt, err := prepareGzipBinaryInput(context.Background(), opts, counted, staging)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	if err := attachment.ValidateTextReaderAtFull(context.Background(), attachment.KindTrace, source.DisplayPath(), input, input.Size(), attachment.TracePhysicalLineMaxBytes); err != nil {
		t.Fatal(err)
	}
	ledger, err := newConversionFileLedgerForAuthority(source)
	if err != nil {
		t.Fatal(err)
	}
	ledger.stagingRoot = opts.RuntimeAnchor
	defer ledger.cleanup()
	compressedReadBytes := counted.readBytes
	result, err := publishGzipInputText(context.Background(), opts, input, receipt, ledger)
	if err != nil {
		t.Fatal(err)
	}
	if counted.readBytes != compressedReadBytes {
		t.Fatal("text publication re-read compressed source")
	}
	if result.Profile != GzipTextTransportProfile || result.SourceGeneration != "" || result.SourceBytes != receipt.SourceBytes || result.SourceSHA256 != receipt.SourceSHA256 || result.DecodedSHA256 != receipt.DecodedSHA256 || result.DecodedPath != opts.OutputPath {
		t.Fatalf("published receipt mismatch: %+v", result)
	}
	id, err := filegeneration.FromPath(result.DecodedPath)
	if err != nil || result.DecodedGeneration == "" || result.DecodedGeneration != id.CacheToken() {
		t.Fatalf("public generation missing: %+v %v", result, err)
	}
	if err := input.Validate(conversionInputStagePreCommit); err != nil {
		t.Fatalf("publication consumed decoded view: %v", err)
	}
	got, err := os.ReadFile(result.DecodedPath)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("published bytes changed: %v", err)
	}
	if err := ledger.validateOwnedPaths(); err != nil {
		t.Fatal(err)
	}
	if err := ledger.cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(result.DecodedPath); !os.IsNotExist(err) {
		t.Fatalf("caller rollback did not remove publication: %v", err)
	}
	if err := input.Validate(conversionInputStagePreCommit); err != nil {
		t.Fatalf("ledger rollback consumed decoded view: %v", err)
	}
}

func TestPublishGzipInputTextRejectsForgedReceiptAndNoReplace(t *testing.T) {
	opts, source, staging := gzipBinaryTestSource(t, gzipTextFixture(t, []byte("# text\n"), gzip.DefaultCompression))
	input, receipt, err := prepareGzipBinaryInput(context.Background(), opts, source, staging)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	ledger, err := newConversionFileLedgerForAuthority(source)
	if err != nil {
		t.Fatal(err)
	}
	ledger.stagingRoot = opts.RuntimeAnchor
	defer ledger.cleanup()
	for _, field := range []string{"source_sha", "decoded_sha", "generation", "size"} {
		forged := receipt
		switch field {
		case "source_sha":
			forged.SourceSHA256 = strings.Repeat("0", 64)
		case "decoded_sha":
			forged.DecodedSHA256 = strings.Repeat("0", 64)
		case "generation":
			forged.DecodedGeneration += "changed"
		case "size":
			forged.DecodedBytes++
		}
		if result, err := publishGzipInputText(context.Background(), opts, input, forged, ledger); err == nil || result != (GzipTextTransportResult{}) {
			t.Fatalf("forged %s published: %+v %v", field, result, err)
		}
	}
	if err := os.WriteFile(opts.OutputPath, []byte("competitor"), 0o600); err != nil {
		t.Fatal(err)
	}
	if result, err := publishGzipInputText(context.Background(), opts, input, receipt, ledger); err == nil || result != (GzipTextTransportResult{}) {
		t.Fatalf("publication overwrote competitor: %+v %v", result, err)
	}
	if got, err := os.ReadFile(opts.OutputPath); err != nil || string(got) != "competitor" {
		t.Fatalf("competitor changed: %q %v", got, err)
	}
	if err := input.Validate(conversionInputStagePreCommit); err != nil {
		t.Fatalf("rejected publication closed input: %v", err)
	}
}

type gzipErrorWriter struct{ err error }

func (writer gzipErrorWriter) Write([]byte) (int, error) { return 0, writer.err }

func TestGzipInflateWriterFailureIsHardAndReceiptZero(t *testing.T) {
	opts, source, _ := gzipBinaryTestSource(t, gzipTextFixture(t, []byte("payload"), gzip.DefaultCompression))
	errWrite := errors.New("output storage failed")
	receipt, err := inflateGzipToWriter(context.Background(), opts, &gzipTextCheckedInput{conversionInputView: source}, gzipErrorWriter{errWrite}, "gzip_input_decompress", time.Now())
	var rejected *GzipTextTransportError
	if !errors.Is(err, errWrite) || receipt != (gzipInputReceipt{}) || errors.As(err, &rejected) {
		t.Fatalf("storage failure got data authority: %+v %v", receipt, err)
	}
}

func TestGzipBinaryInputLateCancellationReturnsNoLiveView(t *testing.T) {
	opts, source, staging := gzipBinaryTestSource(t, gzipTextFixture(t, []byte("PERFILE2body"), gzip.DefaultCompression))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opts.Progress = func(event ProgressEvent) {
		if event.Stage == "gzip_input_decompress" && event.Status == ProgressStatusComplete {
			cancel()
		}
	}
	input, receipt, err := prepareGzipBinaryInput(ctx, opts, source, staging)
	if !errors.Is(err, context.Canceled) || input != nil || receipt != (gzipInputReceipt{}) {
		t.Fatalf("late cancellation leaked view: %v %+v %v", input, receipt, err)
	}
	if err := staging.FinalizeCleanup(); err != nil {
		t.Fatalf("late cancellation retained private handle: %v", err)
	}
	if err := source.Validate(conversionInputStageRoute); err != nil {
		t.Fatalf("late cancellation closed outer authority: %v", err)
	}
}

func TestGzipBinaryInputExternalLeaseConsumesDecodedBytes(t *testing.T) {
	body := []byte("PERFILE2\x00decoded body")
	opts, source, staging := gzipBinaryTestSource(t, gzipTextFixture(t, body, gzip.DefaultCompression))
	input, _, err := prepareGzipBinaryInput(context.Background(), opts, source, staging)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	lease, err := newExternalToolInputLease(context.Background(), input, staging, "provider_input.perf.data", externalToolInputSnapshotOnly)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if err := lease.Validate(); err != nil {
		t.Fatal(err)
	}
	if lease.snapshot == nil || lease.snapshot.size != int64(len(body)) {
		t.Fatalf("lease did not snapshot decoded logical input: %+v", lease)
	}
	got := make([]byte, len(body))
	if _, err := lease.snapshot.file.ReadAt(got, 0); err != nil || !bytes.Equal(got, body) {
		t.Fatalf("provider would consume compressed bytes: %q %v", got, err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	if err := input.Validate(conversionInputStageRoute); err != nil {
		t.Fatalf("lease consumed gzip view ownership: %v", err)
	}
}

func TestPublishGzipInputTextCancellationKeepsCallerOwnership(t *testing.T) {
	opts, source, staging := gzipBinaryTestSource(t, gzipTextFixture(t, []byte("# text\n"), gzip.DefaultCompression))
	input, receipt, err := prepareGzipBinaryInput(context.Background(), opts, source, staging)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	ledger, err := newConversionFileLedgerForAuthority(source)
	if err != nil {
		t.Fatal(err)
	}
	ledger.stagingRoot = opts.RuntimeAnchor
	defer ledger.cleanup()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := publishGzipInputText(ctx, opts, input, receipt, ledger)
	if !errors.Is(err, context.Canceled) || result != (GzipTextTransportResult{}) {
		t.Fatalf("canceled publication gained receipt: %+v %v", result, err)
	}
	if err := input.Validate(conversionInputStagePreCommit); err != nil {
		t.Fatalf("canceled publication closed input: %v", err)
	}
	if _, err := os.Stat(opts.OutputPath); !os.IsNotExist(err) {
		t.Fatalf("canceled publication wrote output: %v", err)
	}
}
