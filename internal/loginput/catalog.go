// Package loginput preserves attached log sources independently of their
// bounded model previews. Parsed fields describe recorded values only: this
// package never assigns a timezone, maps a clock to a trace, or infers causes.
package loginput

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

const (
	KindHilog = "hilog"
	KindKmsg  = "kmsg"
	KindText  = "text"
)

// Input selects exactly one file path or a complete in-memory payload. Name is
// a display identity for memory input; matching names alone never merge files.
type Input struct {
	Path string
	Name string
	Data []byte
}

type Options struct {
	MaxInputBytes   int64
	MaxDecodedBytes int64
	MaxLineBytes    int
	MaxRecordBytes  int
	MaxResultBytes  int
}

// Source describes a complete scan or an explicit source failure. Byte offsets
// and physical lines refer to decoded bytes, not compressed-file positions.
type Source struct {
	ID                  string   `json:"source_id"`
	Name                string   `json:"name"`
	Path                string   `json:"path,omitempty"`
	Aliases             []string `json:"aliases,omitempty"`
	Generation          string   `json:"generation"`
	OriginalSHA256      string   `json:"original_sha256,omitempty"`
	DecodedSHA256       string   `json:"decoded_sha256,omitempty"`
	Compression         string   `json:"compression"`
	OriginalBytes       int64    `json:"original_bytes"`
	DecodedBytes        int64    `json:"decoded_bytes"`
	PhysicalLines       int64    `json:"physical_lines"`
	Records             int64    `json:"records"`
	ParsedRecords       int64    `json:"parsed_records"`
	UnknownRecords      int64    `json:"unknown_records"`
	MalformedRecords    int64    `json:"malformed_records"`
	OrphanContinuations int64    `json:"orphan_continuations"`
	Complete            bool     `json:"complete"`
	Error               string   `json:"error,omitempty"`
}

type Record struct {
	ID              string `json:"record_id"`
	SourceID        string `json:"source_id"`
	Kind            string `json:"kind"`
	Status          string `json:"status"`
	FirstLine       int64  `json:"first_line"`
	LastLine        int64  `json:"last_line"`
	ByteStart       int64  `json:"byte_start"`
	ByteEnd         int64  `json:"byte_end"`
	RawBytes        []byte `json:"raw_base64"`
	RawText         string `json:"raw_text,omitempty"`
	Message         string `json:"message,omitempty"`
	PID             *int64 `json:"pid,omitempty"`
	TID             *int64 `json:"tid,omitempty"`
	CPU             *int64 `json:"cpu,omitempty"`
	Level           string `json:"level,omitempty"`
	Tag             string `json:"tag,omitempty"`
	Comm            string `json:"comm,omitempty"`
	WallTimestamp   string `json:"wall_timestamp_raw,omitempty"`
	BootTimestamp   string `json:"boot_timestamp_raw,omitempty"`
	BootTimestampNS string `json:"boot_timestamp_ns,omitempty"`
	ClockDomain     string `json:"clock_domain,omitempty"`
	ParseError      string `json:"parse_error,omitempty"`
}

// Query line bounds are inclusive and select every whole record whose physical
// line range overlaps them. Returned FirstLine/LastLine remain the whole record
// range, including a header before the requested first line. Offset is applied
// after filters, in source input order then physical line order, not clock order.
type Query struct {
	SourceIDs []string `json:"source_ids,omitempty"`
	Kinds     []string `json:"kinds,omitempty"`
	PID       *int64   `json:"pid,omitempty"`
	TID       *int64   `json:"tid,omitempty"`
	Contains  string   `json:"contains,omitempty"`
	FirstLine int64    `json:"first_line,omitempty"`
	LastLine  int64    `json:"last_line,omitempty"`
	Offset    int64    `json:"offset,omitempty"`
	Limit     int      `json:"limit,omitempty"`
}

type SourceError struct {
	SourceID string `json:"source_id"`
	Error    string `json:"error"`
}

type Result struct {
	Records                []Record      `json:"records"`
	Sources                []Source      `json:"sources"`
	Matched                int64         `json:"matched"`
	Returned               int           `json:"returned"`
	Omitted                int64         `json:"omitted"`
	Offset                 int64         `json:"offset"`
	Limit                  int           `json:"limit"`
	SourceErrors           []SourceError `json:"source_errors,omitempty"`
	Complete               bool          `json:"complete"`
	ResultByteLimitReached bool          `json:"result_byte_limit_reached"`
}

type preparedSource struct {
	summary  Source
	identity filegeneration.Identity
	data     []byte
	preview  string
}

type Catalog struct {
	sources []preparedSource
	opts    Options
}

func normalizedOptions(opts Options) (Options, error) {
	if opts.MaxInputBytes < 0 || opts.MaxDecodedBytes < 0 || opts.MaxLineBytes < 0 || opts.MaxRecordBytes < 0 || opts.MaxResultBytes < 0 || opts.MaxInputBytes == math.MaxInt64 || opts.MaxDecodedBytes == math.MaxInt64 {
		return opts, fmt.Errorf("log input limits must not be negative")
	}
	if opts.MaxInputBytes == 0 {
		opts.MaxInputBytes = 1 << 30
	}
	if opts.MaxDecodedBytes == 0 {
		opts.MaxDecodedBytes = 1 << 30
	}
	if opts.MaxLineBytes == 0 {
		opts.MaxLineBytes = 1 << 20
	}
	if opts.MaxRecordBytes == 0 {
		opts.MaxRecordBytes = 1 << 20
	}
	if opts.MaxResultBytes == 0 {
		opts.MaxResultBytes = 4 << 20
	}
	return opts, nil
}

// Prepare reads every selected source to EOF and records its generation and
// content digests. Individual failures remain visible alongside healthy sources.
// When no source can be read completely, the non-nil catalog is returned with an
// error so callers can display diagnostics without treating it as query success.
func Prepare(ctx context.Context, inputs []Input, opts Options) (*Catalog, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	opts, err := normalizedOptions(opts)
	if err != nil {
		return nil, err
	}
	c := &Catalog{opts: opts}
	healthy := 0
	for i, input := range inputs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		s := preparedSource{summary: Source{Name: input.Name, Compression: "none"}}
		if input.Path != "" {
			s.summary.Path, err = filepath.Abs(input.Path)
			if err == nil && input.Data != nil {
				err = fmt.Errorf("file path and memory data are mutually exclusive")
			}
			if s.summary.Name == "" {
				s.summary.Name = input.Path
			}
		} else {
			if s.summary.Name == "" {
				s.summary.Name = fmt.Sprintf("inline-%d", i+1)
			}
			if int64(len(input.Data)) > opts.MaxInputBytes {
				err = fmt.Errorf("input exceeds %d-byte limit", opts.MaxInputBytes)
			} else {
				s.data = append([]byte{}, input.Data...)
			}
		}
		if err == nil {
			err = attachment.ValidateSourceLabel(s.summary.Name)
		}
		if err == nil && s.summary.Path != "" {
			err = attachment.ValidateSourceLabel(s.summary.Path)
		}
		if err == nil {
			err = scanSource(ctx, &s, opts, nil, false)
		}
		if err != nil {
			s.summary.Error = err.Error()
			s.summary.Complete = false
		}
		key := s.summary.Path + "\x00" + s.summary.Name + "\x00" + s.summary.Generation + "\x00" + s.summary.OriginalSHA256
		digest := sha256.Sum256([]byte(key))
		s.summary.ID = "log-" + hex.EncodeToString(digest[:12])
		duplicate := false
		if s.summary.Complete {
			for j := range c.sources {
				old := &c.sources[j]
				if !old.summary.Complete {
					continue
				}
				sameFile := s.summary.Path != "" && old.summary.Path != "" && s.identity.Strong() && s.identity.SameVersion(old.identity)
				sameMemory := s.summary.Path == "" && old.summary.Path == "" && s.summary.Name == old.summary.Name && s.summary.OriginalSHA256 == old.summary.OriginalSHA256
				samePath := s.summary.Path != "" && s.summary.Path == old.summary.Path && s.identity.SameVersion(old.identity)
				if (sameFile || samePath || sameMemory) && s.summary.OriginalSHA256 == old.summary.OriginalSHA256 {
					if s.summary.Path != "" && s.summary.Path != old.summary.Path && !contains(old.summary.Aliases, s.summary.Path) {
						old.summary.Aliases = append(old.summary.Aliases, s.summary.Path)
					}
					duplicate = true
					break
				}
			}
		}
		if !duplicate {
			c.sources = append(c.sources, s)
			if s.summary.Complete {
				healthy++
			}
		}
		err = nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(inputs) > 0 && healthy == 0 {
		var failures []string
		for _, source := range c.sources {
			if len(failures) == 3 {
				break
			}
			failures = append(failures, fmt.Sprintf("%q: %s", boundedDiagnostic(source.summary.Name, 128), boundedDiagnostic(source.summary.Error, 512)))
		}
		if len(c.sources) > len(failures) {
			failures = append(failures, fmt.Sprintf("and %d more failed sources", len(c.sources)-len(failures)))
		}
		return c, fmt.Errorf("no attached log source was read completely: %s", strings.Join(failures, "; "))
	}
	return c, nil
}

func (c *Catalog) Sources() []Source {
	if c == nil {
		return nil
	}
	out := make([]Source, len(c.sources))
	for i := range c.sources {
		out[i] = c.sources[i].summary
		out[i].Aliases = append([]string(nil), out[i].Aliases...)
	}
	return out
}

// Preview is explicitly non-authoritative. It never constructs a new Catalog
// and does not change complete source query coverage.
func (c *Catalog) Preview(maxBytes int) string {
	if c == nil || maxBytes <= 0 {
		return ""
	}
	var b strings.Builder
	for _, s := range c.sources {
		if len(c.sources) > 1 {
			if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
				b.WriteByte('\n')
			}
			fmt.Fprintf(&b, "# codrax-source: %s (%s)\n", s.summary.Name, s.summary.ID)
		}
		if !s.summary.Complete {
			fmt.Fprintf(&b, "# source unavailable: %s\n", s.summary.Error)
		} else {
			b.WriteString(s.preview)
		}
		if b.Len() >= maxBytes {
			break
		}
	}
	value := b.String()
	if len(value) > maxBytes {
		value = value[:maxBytes]
		for !validUTF8(value) && len(value) > 0 {
			value = value[:len(value)-1]
		}
	}
	return value
}

func boundedDiagnostic(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for !validUTF8(value) && len(value) > 0 {
		value = value[:len(value)-1]
	}
	return value + "…"
}

// Append retains prior complete memory payloads and generation receipts; it
// never reconstructs a source from Preview. The receiver remains immutable.
func (c *Catalog) Append(ctx context.Context, inputs []Input) (*Catalog, error) {
	opts := Options{}
	if c != nil {
		opts = c.opts
	}
	added, err := Prepare(ctx, inputs, opts)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return added, nil
	}
	merged := &Catalog{opts: c.opts, sources: make([]preparedSource, len(c.sources))}
	copy(merged.sources, c.sources)
	for i := range merged.sources {
		merged.sources[i].summary.Aliases = append([]string(nil), merged.sources[i].summary.Aliases...)
	}
	for _, s := range added.sources {
		duplicate := false
		for i := range merged.sources {
			old := &merged.sources[i]
			if !s.summary.Complete || !old.summary.Complete {
				continue
			}
			sameFile := s.summary.Path != "" && old.summary.Path != "" && s.identity.SameVersion(old.identity) && (s.identity.Strong() || s.summary.Path == old.summary.Path)
			sameMemory := s.summary.Path == "" && old.summary.Path == "" && s.summary.Name == old.summary.Name
			if (sameFile || sameMemory) && s.summary.OriginalSHA256 == old.summary.OriginalSHA256 {
				for _, path := range append([]string{s.summary.Path}, s.summary.Aliases...) {
					if path != "" && path != old.summary.Path && !contains(old.summary.Aliases, path) {
						old.summary.Aliases = append(old.summary.Aliases, path)
					}
				}
				duplicate = true
				break
			}
		}
		if !duplicate {
			merged.sources = append(merged.sources, s)
		}
	}
	return merged, nil
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
