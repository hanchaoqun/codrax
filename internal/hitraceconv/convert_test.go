package hitraceconv

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestDefaultOutputPathAppendsSystraceSuffix(t *testing.T) {
	if got, want := DefaultOutputPath("foo.htrace.bin"), "foo.htrace.bin.systrace"; got != want {
		t.Fatalf("default output: got %q, want %q", got, want)
	}
}

func TestConvertFileWritesTextSystraceAndRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "sample.htrace.bin")
	if err := os.WriteFile(input, syntheticBinaryHitrace(t), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ConvertFile(context.Background(), Options{InputPath: input})
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if result.OutputPath != input+".systrace" {
		t.Fatalf("output path: got %q", result.OutputPath)
	}
	if result.EventsWritten != 1 {
		t.Fatalf("events written: got %d", result.EventsWritten)
	}
	body, err := os.ReadFile(result.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{
		"sched_wakeup: comm=com.tencent.mm pid=36379 prio=53 target_cpu=000",
		"2942.124416",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("converted trace missing %q:\n%s", want, text)
		}
	}

	idx, err := tracequery.BuildIndex(context.Background(), result.OutputPath)
	if err != nil {
		t.Fatalf("tracequery parse converted output: %v", err)
	}
	if len(idx.Events) != 1 || idx.Events[0].WakeePID != 36379 {
		t.Fatalf("converted output did not round-trip through tracequery: %+v", idx.Events)
	}

	if _, err := ConvertFile(context.Background(), Options{InputPath: input}); err == nil ||
		!strings.Contains(err.Error(), "output file already exists") {
		t.Fatalf("expected existing output refusal, got %v", err)
	}
}

func TestConvertFilePreservesMissingFormatAsUnknownRow(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "missing-format.htrace")
	var b bytes.Buffer
	writeFileHeader(&b, 1)
	writeSegment(&b, segmentEventsFormat, []byte(syntheticEventFormat()))
	writeSegment(&b, segmentCmdlines, []byte("36379 com.tencent.mm\n"))
	writeSegment(&b, segmentRawTrace, syntheticRawPageForEventID(99))
	if err := os.WriteFile(input, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(dir, "out.systrace")
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output})
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if result.MissingFormatCount != 1 || result.UnknownEventCount != 1 {
		t.Fatalf("missing/unknown counts: %+v", result)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{
		"unknown_event_99: event_id=99 payload_len=36 payload_hex=6300",
		"payload_truncated=true",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing-format row missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "raw_event=unparsed") {
		t.Fatalf("missing-format row not preserved:\n%s", string(body))
	}
}

func syntheticBinaryHitrace(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	writeFileHeader(&b, 1)
	writeSegment(&b, segmentEventsFormat, []byte(syntheticEventFormat()))
	writeSegment(&b, segmentCmdlines, []byte("36379 com.tencent.mm\n"))
	writeSegment(&b, segmentTGIDs, []byte("36379 36379\n"))
	writeSegment(&b, segmentRawTrace, syntheticRawPage())
	return b.Bytes()
}

func syntheticEventFormat() string {
	return strings.Join([]string{
		"name: sched_wakeup",
		"ID: 10",
		"format:",
		"\tfield:unsigned short common_type;\toffset:0;\tsize:2;\tsigned:0;",
		"\tfield:unsigned char common_flags;\toffset:2;\tsize:1;\tsigned:0;",
		"\tfield:unsigned char common_preempt_count;\toffset:3;\tsize:1;\tsigned:0;",
		"\tfield:int common_pid;\toffset:4;\tsize:4;\tsigned:1;",
		"\tfield:char comm[16];\toffset:8;\tsize:16;\tsigned:0;",
		"\tfield:int pid;\toffset:24;\tsize:4;\tsigned:1;",
		"\tfield:int prio;\toffset:28;\tsize:4;\tsigned:1;",
		"\tfield:int target_cpu;\toffset:32;\tsize:4;\tsigned:1;",
		`print fmt: "comm=%s pid=%d prio=%d target_cpu=%03d"`,
		"",
	}, "\n")
}

func syntheticRawPage() []byte {
	return syntheticRawPageForEventID(10)
}

func syntheticRawPageForEventID(eventID uint16) []byte {
	page := make([]byte, tracePageSize)
	binary.LittleEndian.PutUint64(page[0:8], 2942124416000) // ns, renders 2942.124416
	binary.LittleEndian.PutUint64(page[8:16], tracePageSize)
	page[16] = 4

	content := make([]byte, 36)
	binary.LittleEndian.PutUint16(content[0:2], eventID)
	content[2] = 0
	content[3] = 0
	binary.LittleEndian.PutUint32(content[4:8], uint32(36379))
	copy(content[8:24], []byte("com.tencent.mm"))
	binary.LittleEndian.PutUint32(content[24:28], uint32(36379))
	binary.LittleEndian.PutUint32(content[28:32], uint32(53))
	binary.LittleEndian.PutUint32(content[32:36], uint32(0))

	off := pageHeaderSize
	binary.LittleEndian.PutUint32(page[off:off+4], 0)
	binary.LittleEndian.PutUint16(page[off+4:off+6], uint16(len(content)))
	copy(page[off+eventHeaderSize:], content)
	return page
}

func writeFileHeader(b *bytes.Buffer, cpuNum int) {
	buf := make([]byte, fileHeaderSize)
	binary.LittleEndian.PutUint16(buf[0:2], 0xace)
	buf[2] = 1
	binary.LittleEndian.PutUint16(buf[4:6], 1)
	binary.LittleEndian.PutUint32(buf[8:12], uint32(cpuNum<<1))
	b.Write(buf)
}

func writeSegment(b *bytes.Buffer, typ int, payload []byte) {
	var hdr [segmentHdrSize]byte
	binary.LittleEndian.PutUint32(hdr[0:4], uint32(typ))
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(len(payload)))
	b.Write(hdr[:])
	b.Write(payload)
}
