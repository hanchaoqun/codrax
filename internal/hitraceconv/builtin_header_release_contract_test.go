package hitraceconv

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReleaseBuiltinHeaderStrictLengthAndCauseContract pins the byte-exact
// header boundary. A short header must stay typed while retaining the original
// io sentinel for callers which use errors.Is.
func TestReleaseBuiltinHeaderStrictLengthAndCauseContract(t *testing.T) {
	full := releaseBuiltinHeader(harmonyRMQMagic, harmonyRMQFileType, harmonyRMQVersion)
	for length := 0; length < fileHeaderSize; length++ {
		t.Run(string(rune('A'+length)), func(t *testing.T) {
			_, err := readFileHeader(bytes.NewReader(full[:length]))
			if err == nil {
				t.Fatalf("readFileHeader accepted %d/%d header bytes", length, fileHeaderSize)
			}
			var decodeErr *BuiltinSysDecodeError
			if !errors.As(err, &decodeErr) {
				t.Fatalf("short header error is not *BuiltinSysDecodeError: %T %v", err, err)
			}
			if decodeErr.Code != builtinSysDecodeTruncatedHeader || decodeErr.HeaderBytes != length || decodeErr.Offset != int64(length) {
				t.Fatalf("short header metadata mismatch: %+v", decodeErr)
			}
			wantCause := error(io.ErrUnexpectedEOF)
			if length == 0 {
				wantCause = io.EOF
			}
			if !errors.Is(err, wantCause) {
				t.Fatalf("short header len=%d lost io cause %v: %v", length, wantCause, err)
			}
		})
	}
}

// TestReleaseBuiltinHeaderParsesAllFileTypeBytes documents that byte parsing
// is deliberately separate from decoder admission. Admission is tested below.
func TestReleaseBuiltinHeaderParsesAllFileTypeBytes(t *testing.T) {
	for _, fileType := range []uint8{0, 1, 54, 255} {
		t.Run(string(rune(fileType)), func(t *testing.T) {
			header, err := readFileHeader(bytes.NewReader(releaseBuiltinHeader(harmonyRMQMagic, fileType, harmonyRMQVersion)))
			if err != nil {
				t.Fatal(err)
			}
			if header.Magic != harmonyRMQMagic || header.Version != harmonyRMQVersion || header.FileType != fileType {
				t.Fatalf("parsed header mismatch for file_type=%d: %+v", fileType, header)
			}
		})
	}
}

// TestReleaseBuiltinHeaderValidationOrder pins the fail-closed authority order:
// magic, then version, then file type. Invalid lower-priority fields must never
// obscure the first malformed authority.
func TestReleaseBuiltinHeaderValidationOrder(t *testing.T) {
	tests := []struct {
		name       string
		magic      uint16
		fileType   uint8
		version    uint16
		wantCode   string
		wantOffset int64
	}{
		{name: "magic precedes version and type", magic: 0xbeef, fileType: 54, version: 99, wantCode: builtinSysDecodeInvalidMagic, wantOffset: 0},
		{name: "version precedes type", magic: harmonyRMQMagic, fileType: 54, version: 99, wantCode: builtinSysDecodeUnsupportedVersion, wantOffset: 4},
		{name: "openharmony type zero", magic: harmonyRMQMagic, fileType: 0, version: harmonyRMQVersion, wantCode: builtinSysDecodeUnsupportedFileType, wantOffset: 2},
		{name: "observed type fifty four", magic: harmonyRMQMagic, fileType: 54, version: harmonyRMQVersion, wantCode: builtinSysDecodeUnsupportedFileType, wantOffset: 2},
		{name: "unknown type max", magic: harmonyRMQMagic, fileType: 255, version: harmonyRMQVersion, wantCode: builtinSysDecodeUnsupportedFileType, wantOffset: 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "capture.sys")
			body := releaseBuiltinHeader(tc.magic, tc.fileType, tc.version)
			if err := os.WriteFile(path, body, 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := scanMetadata(context.Background(), path, int64(len(body)))
			var decodeErr *BuiltinSysDecodeError
			if !errors.As(err, &decodeErr) {
				t.Fatalf("scanMetadata error is not *BuiltinSysDecodeError: %T %v", err, err)
			}
			if decodeErr.Code != tc.wantCode || decodeErr.Offset != tc.wantOffset {
				t.Fatalf("validation order mismatch: got %+v, want code=%s offset=%d", decodeErr, tc.wantCode, tc.wantOffset)
			}
			if tc.wantCode == builtinSysDecodeUnsupportedFileType && decodeErr.FileType != tc.fileType {
				t.Fatalf("file type evidence mismatch: got=%d want=%d", decodeErr.FileType, tc.fileType)
			}
		})
	}

	path := filepath.Join(t.TempDir(), "valid-header-only.sys")
	body := releaseBuiltinHeader(harmonyRMQMagic, harmonyRMQFileType, harmonyRMQVersion)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := scanMetadata(context.Background(), path, int64(len(body)))
	var decodeErr *BuiltinSysDecodeError
	if errors.As(err, &decodeErr) {
		t.Fatalf("valid admitted header was rejected as a header error: %+v", decodeErr)
	}
	if err == nil || !strings.Contains(err.Error(), "no event format segment found") {
		t.Fatalf("valid header should advance to segment validation, got %v", err)
	}
}

func releaseBuiltinHeader(magic uint16, fileType uint8, version uint16) []byte {
	body := make([]byte, fileHeaderSize)
	binary.LittleEndian.PutUint16(body[0:2], magic)
	body[2] = fileType
	binary.LittleEndian.PutUint16(body[4:6], version)
	return body
}
