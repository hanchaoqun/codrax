package attachment

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestValidateTextAcceptsRuntimeText(t *testing.T) {
	data := []byte("2026-06-08 10:00:00 WARN service started\n\tframe: foo.bar\n")
	if err := ValidateText(KindLog, "run.log", data, false); err != nil {
		t.Fatalf("text log should be accepted: %v", err)
	}
}

func TestValidateTextRejectsNULBinary(t *testing.T) {
	err := ValidateText(KindTrace, "capture.htrace", []byte{'H', 'T', 0, 1, 2, 3}, false)
	if err == nil {
		t.Fatal("binary trace should be rejected")
	}
	issue, ok := err.(TextIssue)
	if !ok {
		t.Fatalf("err type = %T, want TextIssue", err)
	}
	if issue.Reason != "contains NUL bytes" {
		t.Fatalf("reason=%q", issue.Reason)
	}
	msg := issue.Message("zh", SurfaceREPL)
	for _, want := range []string{"/htrace convert", "codrax trace convert", "capture.htrace"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message missing %q: %s", want, msg)
		}
	}
}

func TestTracePhysicalLineLimitMessageDoesNotPrescribeBinaryConversion(t *testing.T) {
	issue := TextIssue{
		Kind:   KindTrace,
		Path:   "oversized.systrace",
		Reason: "physical line exceeds 16777216 bytes",
	}
	for _, test := range []struct {
		lang    string
		surface Surface
		want    string
	}{
		{lang: "zh", surface: SurfaceCLI, want: "重新导出或采集"},
		{lang: "zh", surface: SurfaceREPL, want: "重新导出或采集"},
		{lang: "en", surface: SurfaceCLI, want: "Export or recapture"},
		{lang: "en", surface: SurfaceREPL, want: "Export or recapture"},
	} {
		message := issue.Message(test.lang, test.surface)
		if !strings.Contains(message, test.want) {
			t.Fatalf("message lang=%s surface=%s missing %q: %s", test.lang, test.surface, test.want, message)
		}
		for _, forbidden := range []string{"trace convert", "/htrace convert"} {
			if strings.Contains(message, forbidden) {
				t.Fatalf("line-limit message must not prescribe binary conversion (%q): %s", forbidden, message)
			}
		}
	}
}

func TestValidateTextRejectsInvalidUTF8(t *testing.T) {
	err := ValidateText(KindLog, "bad.log", []byte{0xff, 0xfe, 'x'}, false)
	if err == nil {
		t.Fatal("invalid UTF-8 should be rejected")
	}
	if !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("error should mention UTF-8: %v", err)
	}
}

func TestValidateTextRejectsLateNULBeyondReaderProbeSizeInMemory(t *testing.T) {
	data := bytes.Repeat([]byte{'x'}, textProbeBytes+17)
	data[len(data)-1] = 0
	err := ValidateText(KindTrace, "late-nul.systrace", data, false)
	if err == nil || !strings.Contains(err.Error(), "NUL") {
		t.Fatalf("full in-memory attachment must reject late NUL: %v", err)
	}
}

func TestValidateTextStringMatchesBytePolicyWithoutBodyCopy(t *testing.T) {
	good := strings.Repeat("正常 trace row\n", textProbeBytes/16)
	if err := ValidateTextString(KindTrace, "large.systrace", good, false); err != nil {
		t.Fatalf("valid string trace was rejected: %v", err)
	}
	bad := good + "\x00"
	if err := ValidateTextString(KindTrace, "late-nul.systrace", bad, false); err == nil || !strings.Contains(err.Error(), "NUL") {
		t.Fatalf("full string scan must reject late NUL: %v", err)
	}
	for _, body := range []string{"\xce\x0a\x01", "valid prefix \xff", strings.Repeat("\u0080", 40)} {
		byteErr := ValidateText(KindTrace, "parity.systrace", []byte(body), false)
		stringErr := ValidateTextString(KindTrace, "parity.systrace", body, false)
		if (byteErr == nil) != (stringErr == nil) || (byteErr != nil && byteErr.Error() != stringErr.Error()) {
			t.Fatalf("byte/string policy drift for %q: byte=%v string=%v", body, byteErr, stringErr)
		}
	}
}

func TestValidateTextRejectsInvalidTruncatedUTF8Tail(t *testing.T) {
	err := ValidateText(KindTrace, "bad-tail.systrace", []byte("valid prefix \xff"), true)
	if err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("truncation must not excuse an invalid byte: %v", err)
	}
}

func TestValidateTextAllowsTruncatedUTF8Tail(t *testing.T) {
	data := []byte("正常日志行 ")
	data = append(data, []byte("中")[:2]...)
	if err := ValidateText(KindLog, "truncated.log", data, true); err != nil {
		t.Fatalf("truncated UTF-8 tail at cap boundary should be accepted: %v", err)
	}
}

func TestValidateTextAllowsOnlyWellFormedIncompleteUTF8Tail(t *testing.T) {
	for _, suffix := range [][]byte{
		{0xe4},
		{0xe4, 0xb8},
		{0xf0, 0x9f, 0x98},
	} {
		data := append([]byte("trace row "), suffix...)
		if err := ValidateText(KindTrace, "truncated.systrace", data, true); err != nil {
			t.Fatalf("well-formed incomplete suffix %x should be accepted: %v", suffix, err)
		}
	}
	for _, suffix := range [][]byte{
		{0x80},
		{0xc0},
		{0xe0, 0x80},
		{0xf4, 0x90},
	} {
		data := append([]byte("trace row "), suffix...)
		if err := ValidateText(KindTrace, "bad-truncated.systrace", data, true); err == nil {
			t.Fatalf("invalid incomplete suffix %x was accepted", suffix)
		}
	}
}

func TestValidateTextCountsUnicodeControlRunes(t *testing.T) {
	data := []byte(strings.Repeat("\u0080", 40))
	err := ValidateText(KindTrace, "controls.systrace", data, false)
	if err == nil || !strings.Contains(err.Error(), "control") {
		t.Fatalf("Unicode C1 controls must enter the control ratio: %v", err)
	}
	if err := ValidateText(KindTrace, "whitespace.systrace", []byte(strings.Repeat("\t\r\n\f\b\x1b", 8)), false); err != nil {
		t.Fatalf("existing admitted whitespace/control formatting must remain accepted: %v", err)
	}
}

func TestKnownBinaryTraceFormatExactMagics(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want BinaryTraceFormat
	}{
		{name: "harmony-rmq", data: []byte{0xce, 0x0a, 1}, want: BinaryTraceFormatHarmonyRMQ},
		{name: "openharmony-profiler", data: []byte("OHOSPROFrest"), want: BinaryTraceFormatOHOSProfile},
		{name: "perf-little-endian", data: []byte("PERFILE2rest"), want: BinaryTraceFormatLinuxPerf},
		{name: "perf-big-endian", data: []byte("2ELIFREPrest"), want: BinaryTraceFormatLinuxPerf},
		{name: "gzip", data: []byte{0x1f, 0x8b, 0x08}, want: BinaryTraceFormatGZIP},
		{name: "zip-local", data: []byte{'P', 'K', 0x03, 0x04}, want: BinaryTraceFormatZIP},
		{name: "zip-empty", data: []byte{'P', 'K', 0x05, 0x06}, want: BinaryTraceFormatZIP},
		{name: "zip-spanned", data: []byte{'P', 'K', 0x07, 0x08}, want: BinaryTraceFormatZIP},
		{name: "sqlite", data: []byte("SQLite format 3\x00rest"), want: BinaryTraceFormatSQLite},
		{name: "truncated-rmq", data: []byte{0xce}, want: BinaryTraceFormatUnknown},
		{name: "ordinary-text", data: []byte("PERFILE text"), want: BinaryTraceFormatUnknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := KnownBinaryTraceFormat(test.data); got != test.want {
				t.Fatalf("KnownBinaryTraceFormat(%x)=%q want=%q", test.data, got, test.want)
			}
			if test.want != BinaryTraceFormatUnknown {
				if err := ValidateText(KindTrace, test.name, test.data, false); err == nil || !strings.Contains(err.Error(), string(test.want)) {
					t.Fatalf("known binary format was not rejected with its typed label: %v", err)
				}
			}
		})
	}
}

type trackingReaderAt struct {
	reader  *bytes.Reader
	offsets []int64
	lengths []int
}

func (r *trackingReaderAt) ReadAt(p []byte, off int64) (int, error) {
	r.offsets = append(r.offsets, off)
	r.lengths = append(r.lengths, len(p))
	return r.reader.ReadAt(p, off)
}

func TestValidateTextReaderAtIsBoundedAndDoesNotMoveOffset(t *testing.T) {
	data := bytes.Repeat([]byte("x"), textProbeBytes+128)
	base := bytes.NewReader(data)
	if _, err := base.Seek(11, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	reader := &trackingReaderAt{reader: base}
	if err := ValidateTextReaderAt(KindTrace, "large.systrace", reader, int64(len(data))); err != nil {
		t.Fatalf("bounded text probe should pass: %v", err)
	}
	if len(reader.offsets) != 1 || reader.offsets[0] != 0 || reader.lengths[0] != textProbeBytes {
		t.Fatalf("ReadAt calls offsets=%v lengths=%v", reader.offsets, reader.lengths)
	}
	position, err := base.Seek(0, io.SeekCurrent)
	if err != nil {
		t.Fatal(err)
	}
	if position != 11 {
		t.Fatalf("ReaderAt probe moved stream offset to %d", position)
	}
}

func TestValidateTextReaderAtFullRejectsLateBinarySignalsBeyondFirstChunk(t *testing.T) {
	tests := []struct {
		name   string
		value  byte
		reason string
	}{
		{name: "nul", value: 0, reason: "NUL"},
		{name: "invalid-utf8", value: 0xff, reason: "UTF-8"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := bytes.Repeat([]byte{'x'}, textProbeBytes+33)
			data[textProbeBytes+17] = test.value
			err := ValidateTextReaderAtFull(context.Background(), KindTrace, "late.systrace", bytes.NewReader(data), int64(len(data)), 0)
			if err == nil || !strings.Contains(err.Error(), test.reason) {
				t.Fatalf("late %s beyond the first chunk was not rejected: %v", test.name, err)
			}
		})
	}
}

func TestValidateTextReaderAtFullCarriesUTF8AcrossChunkBoundary(t *testing.T) {
	prefix := bytes.Repeat([]byte{'x'}, textProbeBytes-1)
	valid := append(append(append([]byte(nil), prefix...), []byte("中")...), '\n')
	if err := ValidateTextReaderAtFull(context.Background(), KindTrace, "valid-boundary.systrace", bytes.NewReader(valid), int64(len(valid)), 0); err != nil {
		t.Fatalf("valid UTF-8 rune spanning two chunks was rejected: %v", err)
	}

	invalid := append(append(append([]byte(nil), prefix...), 0xe4), ' ')
	err := ValidateTextReaderAtFull(context.Background(), KindTrace, "invalid-boundary.systrace", bytes.NewReader(invalid), int64(len(invalid)), 0)
	if err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("invalid UTF-8 spanning two chunks was not rejected: %v", err)
	}
}

func TestValidateTextReaderAtFullCountsUnicodeC1ControlRatio(t *testing.T) {
	belowLimit := []byte(strings.Repeat("\u0080", 3) + strings.Repeat("x", 29))
	if err := ValidateTextReaderAtFull(context.Background(), KindTrace, "three-controls.systrace", bytes.NewReader(belowLimit), int64(len(belowLimit)), 0); err != nil {
		t.Fatalf("C1 controls below the ratio threshold were rejected: %v", err)
	}

	aboveLimit := []byte(strings.Repeat("\u0080", 4) + strings.Repeat("x", 28))
	err := ValidateTextReaderAtFull(context.Background(), KindTrace, "four-controls.systrace", bytes.NewReader(aboveLimit), int64(len(aboveLimit)), 0)
	if err == nil || !strings.Contains(err.Error(), "control") {
		t.Fatalf("C1 controls above the ratio threshold were not rejected: %v", err)
	}
}

func TestValidateTextReaderAtFullEnforcesExactPhysicalLineLimitAcrossChunks(t *testing.T) {
	limit := textProbeBytes + 7
	exact := append(bytes.Repeat([]byte{'x'}, limit), '\n')
	if err := ValidateTextReaderAtFull(context.Background(), KindTrace, "exact-line.systrace", bytes.NewReader(exact), int64(len(exact)), limit); err != nil {
		t.Fatalf("physical line exactly at the cross-chunk limit was rejected: %v", err)
	}

	tooLong := append(bytes.Repeat([]byte{'x'}, limit+1), '\n')
	err := ValidateTextReaderAtFull(context.Background(), KindTrace, "long-line.systrace", bytes.NewReader(tooLong), int64(len(tooLong)), limit)
	if err == nil || !strings.Contains(err.Error(), "physical line exceeds "+strconv.Itoa(limit)+" bytes") {
		t.Fatalf("physical line one byte over the cross-chunk limit was not rejected: %v", err)
	}
}

type shortReadReaderAt struct {
	reader *bytes.Reader
}

func (r shortReadReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return r.reader.ReadAt(p[:len(p)-1], off)
}

func TestValidateTextReaderAtFullRejectsShortRead(t *testing.T) {
	data := bytes.Repeat([]byte{'x'}, 128)
	err := ValidateTextReaderAtFull(context.Background(), KindTrace, "short.systrace", shortReadReaderAt{reader: bytes.NewReader(data)}, int64(len(data)), 0)
	if err == nil || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("short ReaderAt read should fail with unexpected EOF: %v", err)
	}
}

func TestValidateTextReaderAtFullHonorsPreCanceledContextWithoutReading(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reader := &trackingReaderAt{reader: bytes.NewReader([]byte("trace row\n"))}
	err := ValidateTextReaderAtFull(ctx, KindTrace, "canceled.systrace", reader, 10, 0)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled validation returned %v", err)
	}
	if len(reader.offsets) != 0 {
		t.Fatalf("pre-canceled validation performed reads at offsets %v", reader.offsets)
	}
}

type cancelingReaderAt struct {
	reader io.ReaderAt
	cancel context.CancelFunc
	reads  int
}

func (r *cancelingReaderAt) ReadAt(p []byte, off int64) (int, error) {
	r.reads++
	n, err := r.reader.ReadAt(p, off)
	// The first read is the exact-format prefix; cancel after the first full
	// scanning chunk so validation must observe cancellation before chunk two.
	if r.reads == 2 {
		r.cancel()
	}
	return n, err
}

func TestValidateTextReaderAtFullHonorsCancellationDuringScan(t *testing.T) {
	data := bytes.Repeat([]byte{'x'}, textProbeBytes+128)
	ctx, cancel := context.WithCancel(context.Background())
	reader := &cancelingReaderAt{reader: bytes.NewReader(data), cancel: cancel}
	err := ValidateTextReaderAtFull(ctx, KindTrace, "cancel-during-scan.systrace", reader, int64(len(data)), 0)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("mid-scan cancellation returned %v", err)
	}
	if reader.reads != 2 {
		t.Fatalf("validation continued reading after cancellation: reads=%d", reader.reads)
	}
}

func TestValidateTextReaderAtFullDoesNotMoveExistingOffset(t *testing.T) {
	data := bytes.Repeat([]byte("trace row\n"), textProbeBytes/10+3)
	base := bytes.NewReader(data)
	if _, err := base.Seek(17, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if err := ValidateTextReaderAtFull(context.Background(), KindTrace, "offset.systrace", base, int64(len(data)), TracePhysicalLineMaxBytes); err != nil {
		t.Fatalf("valid full trace was rejected: %v", err)
	}
	position, err := base.Seek(0, io.SeekCurrent)
	if err != nil {
		t.Fatal(err)
	}
	if position != 17 {
		t.Fatalf("full ReaderAt validation moved stream offset to %d", position)
	}
}

func TestReadTextFileLimitedProbeIsIndependentOfPayloadCap(t *testing.T) {
	dir := t.TempDir()
	binaryPath := filepath.Join(dir, "binary.htrace")
	if err := os.WriteFile(binaryPath, append([]byte("PERFILE2"), make([]byte, 128)...), 0o644); err != nil {
		t.Fatal(err)
	}
	if data, truncated, err := ReadTextFileLimited(KindTrace, binaryPath, 1); err == nil {
		t.Fatalf("one-byte payload cap admitted binary source: data=%q truncated=%v", data, truncated)
	}

	textPath := filepath.Join(dir, "text.systrace")
	if err := os.WriteFile(textPath, []byte("abcdef"), 0o644); err != nil {
		t.Fatal(err)
	}
	data, truncated, err := ReadTextFileLimited(KindTrace, textPath, 1)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "a" || !truncated {
		t.Fatalf("limited text payload data=%q truncated=%v", data, truncated)
	}
}

func TestAuthoritativeTextReadErrorDoesNotPublishMixedGenerationTextIssue(t *testing.T) {
	provisional := TextIssue{Kind: KindTrace, Path: "changed.trace", Reason: "contains NUL bytes"}
	err := authoritativeTextReadError(errors.New("held generation changed"), nil, provisional)
	var issue TextIssue
	if errors.As(err, &issue) {
		t.Fatalf("mixed-generation provisional format verdict remained discoverable: %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), "generation changed") {
		t.Fatalf("identity verdict was not preserved: %v", err)
	}
}

func TestValidateTextRejectsEmptyUntruncated(t *testing.T) {
	err := ValidateText(KindLog, "empty.log", nil, false)
	if err == nil {
		t.Fatal("empty attachment should be rejected")
	}
	if !strings.Contains(err.Error(), "no readable content") {
		t.Fatalf("empty message should be transparent: %v", err)
	}
}

func TestValidateSourceLabelRejectsControlCharacters(t *testing.T) {
	for _, source := range []string{"a\nb.trace", "a\rb.trace", "a\tb.trace", "a\x00b.trace"} {
		if err := ValidateSourceLabel(source); err == nil {
			t.Fatalf("control-bearing source label was accepted: %q", source)
		}
	}
	if err := ValidateSourceLabel("/tmp/customer trace.systrace"); err != nil {
		t.Fatalf("ordinary source label was rejected: %v", err)
	}
}

func TestValidateSingleTraceAttachmentProvenance(t *testing.T) {
	one := "# codrax-source: one.systrace\nsched_switch: prev_pid=1 next_pid=2\n"
	if err := ValidateSingleTraceAttachmentProvenance(one); err != nil {
		t.Fatalf("single-source attachment was rejected: %v", err)
	}
	two := one + "# codrax-source: two.systrace\nsched_switch: prev_pid=3 next_pid=4\n"
	if err := ValidateSingleTraceAttachmentProvenance(two); err == nil {
		t.Fatal("flattened multi-source trace was accepted")
	}
}
