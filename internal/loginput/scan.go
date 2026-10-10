package loginput

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

type countedReader struct {
	reader io.Reader
	count  int64
}

func (r *countedReader) Read(b []byte) (int, error) {
	n, err := r.reader.Read(b)
	r.count += int64(n)
	return n, err
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(b []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(b)
}

func scanSource(ctx context.Context, s *preparedSource, opts Options, emit func(Record), verify bool) (err error) {
	prior := s.summary
	s.summary = Source{ID: prior.ID, Name: prior.Name, Path: prior.Path, Aliases: prior.Aliases, Compression: "none"}
	var reader io.Reader
	if prior.Path == "" {
		reader = bytes.NewReader(s.data)
	} else {
		file, opened, openErr := filegeneration.OpenRegularReadOnly(prior.Path)
		if openErr != nil {
			return openErr
		}
		defer func() {
			final, finalErr := filegeneration.FromFile(file)
			if finalErr == nil && !opened.SameVersion(final) {
				finalErr = fmt.Errorf("log source changed while reading")
			}
			for _, path := range append([]string{prior.Path}, prior.Aliases...) {
				bound, bindErr := filegeneration.FromPath(path)
				if bindErr == nil && !opened.SameVersion(bound) {
					bindErr = fmt.Errorf("log source path changed while reading: %s", path)
				}
				finalErr = errors.Join(finalErr, bindErr)
			}
			closeErr := file.Close()
			if finalErr != nil || closeErr != nil {
				err = errors.Join(finalErr, closeErr)
				s.summary.Complete = false
			}
		}()
		if verify && !s.identity.SameVersion(opened) {
			return fmt.Errorf("log source generation changed since attachment")
		}
		if opened.Size() > opts.MaxInputBytes {
			return fmt.Errorf("input exceeds %d-byte limit", opts.MaxInputBytes)
		}
		s.identity = opened
		s.summary.Generation = opened.CacheToken()
		reader = io.NewSectionReader(file, 0, opened.Size())
	}
	physicalHash := sha256.New()
	physical := &countedReader{reader: io.TeeReader(io.LimitReader(contextReader{ctx, reader}, opts.MaxInputBytes+1), physicalHash)}
	buffered := bufio.NewReaderSize(physical, 32*1024)
	decoded := io.Reader(buffered)
	magic, _ := buffered.Peek(2)
	if len(magic) == 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		s.summary.Compression = "gzip"
		zipped, gzipErr := gzip.NewReader(buffered)
		if gzipErr != nil {
			return fmt.Errorf("gzip header: %w", gzipErr)
		}
		defer func() {
			err = errors.Join(err, zipped.Close())
			if err != nil {
				s.summary.Complete = false
			}
		}()
		decoded = zipped
	}
	decodedHash := sha256.New()
	full := &countedReader{reader: io.TeeReader(io.LimitReader(contextReader{ctx, decoded}, opts.MaxDecodedBytes+1), decodedHash)}
	decodedBuffer := bufio.NewReaderSize(full, 32*1024)
	prefix, _ := decodedBuffer.Peek(32)
	if format := attachment.KnownBinaryTraceFormat(prefix); format != attachment.BinaryTraceFormatUnknown {
		return fmt.Errorf("known binary container %s is not a log text source", format)
	}
	var preview strings.Builder
	err = scanRecords(ctx, decodedBuffer, opts, func(record Record) {
		s.summary.Records++
		s.summary.PhysicalLines = record.LastLine
		switch record.Status {
		case "parsed":
			s.summary.ParsedRecords++
		case "malformed":
			s.summary.MalformedRecords++
		case "orphan_continuation":
			s.summary.OrphanContinuations++
			s.summary.UnknownRecords++
		default:
			s.summary.UnknownRecords++
		}
		if preview.Len() < 16*1024 {
			if previewSafe(record.RawBytes) {
				remaining := 16*1024 - preview.Len()
				value := record.RawBytes
				if len(value) > remaining {
					value = value[:remaining]
					for !utf8.Valid(value) && len(value) > 0 {
						value = value[:len(value)-1]
					}
				}
				preview.Write(value)
			} else {
				preview.WriteString("[log record contains non-printable or non-UTF-8 bytes; exact bytes available through log_query]\n")
			}
		}
		record.SourceID = prior.ID
		record.ID = fmt.Sprintf("%s:L%d-%d", prior.ID, record.FirstLine, record.LastLine)
		if emit != nil {
			emit(record)
		}
	})
	s.summary.OriginalBytes = physical.count
	s.summary.DecodedBytes = full.count
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if err != nil {
		return fmt.Errorf("decode log source: %w", err)
	}
	if physical.count > opts.MaxInputBytes {
		return fmt.Errorf("input exceeds %d-byte limit", opts.MaxInputBytes)
	}
	if full.count > opts.MaxDecodedBytes {
		return fmt.Errorf("decoded input exceeds %d-byte limit", opts.MaxDecodedBytes)
	}
	// A successful decoder must consume all physical bytes. gzip's multistream
	// reader checks every member checksum and reports trailing non-gzip garbage.
	if _, err = io.Copy(io.Discard, buffered); err != nil {
		return err
	}
	s.summary.OriginalBytes = physical.count
	if physical.count > opts.MaxInputBytes {
		return fmt.Errorf("input exceeds %d-byte limit", opts.MaxInputBytes)
	}
	s.summary.OriginalSHA256 = hex.EncodeToString(physicalHash.Sum(nil))
	s.summary.DecodedSHA256 = hex.EncodeToString(decodedHash.Sum(nil))
	if prior.Path == "" {
		s.summary.Generation = "memory-sha256:" + s.summary.OriginalSHA256
	}
	if verify && (prior.OriginalSHA256 != s.summary.OriginalSHA256 || prior.DecodedSHA256 != s.summary.DecodedSHA256) {
		return fmt.Errorf("log source content changed since attachment")
	}
	s.summary.Complete = true
	s.preview = preview.String()
	return nil
}

func scanRecords(ctx context.Context, input io.Reader, opts Options, emit func(Record)) error {
	reader := bufio.NewReaderSize(input, 32*1024)
	var current *Record
	var messages []string
	var line, offset int64
	flush := func() {
		if current != nil {
			if len(messages) > 0 {
				current.Message = strings.Join(messages, "\n")
			}
			if utf8.Valid(current.RawBytes) {
				current.RawText = string(current.RawBytes)
			}
			emit(*current)
			current = nil
			messages = nil
		}
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		raw, err := readPhysicalLine(reader, opts.MaxLineBytes)
		if err != nil && err != io.EOF {
			return err
		}
		if len(raw) > 0 {
			line++
			text := strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
			record, recognized := parseRecord(text)
			indented := len(text) > 0 && (text[0] == ' ' || text[0] == '\t')
			if !recognized && indented {
				if header, ok := parseRecord(strings.TrimLeft(text, " \t")); ok {
					record, recognized = header, true
				}
			}
			if utf8.Valid(raw) && !recognized && indented && current != nil && current.Status == "parsed" {
				if len(current.RawBytes)+len(raw) > opts.MaxRecordBytes {
					return fmt.Errorf("logical record exceeds %d-byte limit at line %d", opts.MaxRecordBytes, line)
				}
				current.RawBytes = append(current.RawBytes, raw...)
				current.LastLine = line
				current.ByteEnd = offset + int64(len(raw))
				messages = append(messages, text)
			} else {
				flush()
				if len(raw) > opts.MaxRecordBytes {
					return fmt.Errorf("logical record exceeds %d-byte limit at line %d", opts.MaxRecordBytes, line)
				}
				if !recognized && indented {
					record.Status = "orphan_continuation"
				}
				if !utf8.Valid(raw) {
					record = Record{Kind: KindText, Status: "unknown", ParseError: "invalid_utf8"}
				}
				record.FirstLine, record.LastLine = line, line
				record.ByteStart, record.ByteEnd = offset, offset+int64(len(raw))
				record.RawBytes = append([]byte(nil), raw...)
				current = &record
				messages = []string{record.Message}
			}
			offset += int64(len(raw))
		}
		if err == io.EOF {
			flush()
			return nil
		}
	}
}

func readPhysicalLine(reader *bufio.Reader, max int) ([]byte, error) {
	var raw []byte
	for {
		fragment, err := reader.ReadSlice('\n')
		if len(fragment) > max-len(raw) {
			return nil, fmt.Errorf("physical line exceeds %d-byte limit", max)
		}
		raw = append(raw, fragment...)
		if err != bufio.ErrBufferFull {
			return raw, err
		}
	}
}

func validUTF8(value string) bool { return utf8.ValidString(value) }

func previewSafe(raw []byte) bool {
	if !utf8.Valid(raw) {
		return false
	}
	for _, r := range string(raw) {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return false
		}
	}
	return true
}
