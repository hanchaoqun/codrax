package hitraceconv

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestReleaseBuiltinSegmentHeaderTailMatrix(t *testing.T) {
	base := syntheticBinaryHitrace(t)
	completeHeader := make([]byte, segmentHdrSize)
	binary.LittleEndian.PutUint32(completeHeader[0:4], segmentCmdlines)
	binary.LittleEndian.PutUint32(completeHeader[4:8], 0)

	for tailBytes := 0; tailBytes <= segmentHdrSize; tailBytes++ {
		t.Run(strconv.Itoa(tailBytes), func(t *testing.T) {
			body := append(append([]byte(nil), base...), completeHeader[:tailBytes]...)
			path := filepath.Join(t.TempDir(), "tail.sys")
			if err := os.WriteFile(path, body, 0o600); err != nil {
				t.Fatal(err)
			}

			meta, err := scanMetadataAtPathForTest(context.Background(), path)
			if tailBytes == 0 || tailBytes == segmentHdrSize {
				if err != nil {
					t.Fatalf("complete segment boundary with tail=%d failed: %v", tailBytes, err)
				}
				if meta == nil || len(meta.formats) == 0 {
					t.Fatalf("complete segment boundary lost metadata: %+v", meta)
				}
				return
			}

			if meta != nil {
				t.Fatalf("partial segment header returned metadata: %+v", meta)
			}
			var decodeErr *BuiltinSysDecodeError
			if !errors.As(err, &decodeErr) {
				t.Fatalf("partial segment header error is not typed: %T %v", err, err)
			}
			if decodeErr.Code != builtinSysDecodeTruncatedSegmentHeader ||
				decodeErr.HeaderBytes != tailBytes || decodeErr.Offset != int64(len(base)) {
				t.Fatalf("partial segment header evidence drifted: %+v", decodeErr)
			}
			if !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("partial segment header lost io.ErrUnexpectedEOF cause: %v", err)
			}
			for _, token := range []string{"code=truncated_segment_header", "header_bytes=" + strconv.Itoa(tailBytes), "offset=" + strconv.Itoa(len(base))} {
				if !strings.Contains(err.Error(), token) {
					t.Fatalf("partial segment diagnostic missing %q: %v", token, err)
				}
			}
		})
	}
}

func TestReleaseBuiltinPartialSegmentHeaderPublishesNothing(t *testing.T) {
	base := syntheticBinaryHitrace(t)
	header := make([]byte, segmentHdrSize)
	binary.LittleEndian.PutUint32(header[0:4], segmentCmdlines)

	for tailBytes := 1; tailBytes < segmentHdrSize; tailBytes++ {
		t.Run(strconv.Itoa(tailBytes), func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "capture.sys")
			output := filepath.Join(dir, "capture.systrace")
			body := append(append([]byte(nil), base...), header[:tailBytes]...)
			if err := os.WriteFile(input, body, 0o600); err != nil {
				t.Fatal(err)
			}

			result, err := ConvertFile(context.Background(), Options{
				InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin,
			})
			var decodeErr *BuiltinSysDecodeError
			if !errors.As(err, &decodeErr) || decodeErr.Code != builtinSysDecodeTruncatedSegmentHeader {
				t.Fatalf("partial tail=%d must fail closed with typed error: result=%+v err=%T %v", tailBytes, result, err, err)
			}
			if !reflect.DeepEqual(result, Result{}) {
				t.Fatalf("partial tail=%d leaked a result: %+v", tailBytes, result)
			}
			entries, readErr := os.ReadDir(dir)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if len(entries) != 1 || entries[0].Name() != filepath.Base(input) {
				t.Fatalf("partial tail=%d retained output/temp artifacts: %v", tailBytes, entries)
			}
		})
	}
}

func TestReleaseBuiltinSegmentTailUsesSingleRemainingAuthority(t *testing.T) {
	body := sourceGenerationFunctionBody(t, "convert.go", "scanMetadata")
	for _, want := range []string{
		"remaining := size - pos",
		"remaining == 0",
		"remaining < segmentHdrSize",
		"builtinSysDecodeTruncatedSegmentHeader",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("scanMetadata lost segment-tail authority %q", want)
		}
	}
	if strings.Contains(body, "errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)") {
		t.Fatal("scanMetadata still aliases a partial segment header to a clean EOF")
	}
	if bytes.Count([]byte(body), []byte("remaining := size - pos")) != 1 {
		t.Fatal("scanMetadata must have exactly one segment-tail remaining authority")
	}
}
