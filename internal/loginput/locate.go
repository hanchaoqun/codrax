package loginput

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

// ExcerptSelector selects literal attached text, optionally within one actual
// source identity. Model line numbers and filenames are not identity witnesses.
type ExcerptSelector struct{ Text, SourceID string }

// ExcerptLocation describes the excerpt, not the whole logical record. Lines
// are inclusive decoded physical lines and bytes are a half-open decoded span.
// RecordID is empty when the excerpt crosses logical records. A serialized
// location is diagnostic data: it cannot restore the private read receipt.
type ExcerptLocation struct {
	Status    string  `json:"status"`
	Matches   int64   `json:"matches"`
	Source    *Source `json:"source,omitempty"`
	RecordID  string  `json:"record_id,omitempty"`
	FirstLine int64   `json:"first_line,omitempty"`
	LastLine  int64   `json:"last_line,omitempty"`
	ByteStart int64   `json:"byte_start,omitempty"`
	ByteEnd   int64   `json:"byte_end,omitempty"`
	proof     [32]byte
	// Only the owning scan can populate these historical source witnesses.
	matchedExcerpt string
	matchedLine    string
}

func (l ExcerptLocation) Verified() bool {
	return l.Status == "unique" && l.Source != nil && l.proof != [32]byte{} && l.proof == l.digest()
}

// SupportsLineLiteral proves only that this exact excerpt and literal occurred
// on the same physical source line, not exception semantics or a second event.
func (l ExcerptLocation) SupportsLineLiteral(literal, excerpt string) bool {
	literal, excerpt = normalizeExcerpt(strings.TrimSpace(literal)), normalizeExcerpt(strings.TrimSpace(excerpt))
	return l.Verified() && literal != "" && excerpt != "" && !strings.Contains(excerpt, "\n") &&
		!strings.Contains(literal, "\n") && l.matchedExcerpt == excerpt && strings.Contains(l.matchedLine, literal)
}

func (l ExcerptLocation) digest() [32]byte { raw, _ := json.Marshal(l); return sha256.Sum256(raw) }

// Locate scans all selected sources to EOF once for this batch, with the same
// generation and content revalidation as Query. Exact newline normalization
// matches the displayed text but coordinates always describe original bytes.
// Any unreadable source prevents a global uniqueness claim; explicit healthy
// source selection remains usable. Matching never infers actor or clock data.
func (c *Catalog) Locate(ctx context.Context, selectors []ExcerptSelector) ([]ExcerptLocation, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c == nil {
		return nil, fmt.Errorf("no attached log catalog")
	}
	if len(selectors) > 256 {
		return nil, fmt.Errorf("too many excerpt selectors")
	}
	results := make([]ExcerptLocation, len(selectors))
	needles := make([]string, len(selectors))
	maxNeedle := 0
	for i, selector := range selectors {
		if len(selector.Text) > 4096 || !utf8.ValidString(selector.Text) {
			return nil, fmt.Errorf("excerpt must be valid UTF-8 and at most 4096 bytes")
		}
		if selector.SourceID != "" {
			if err := validateQuery(Query{SourceIDs: []string{selector.SourceID}}, c); err != nil {
				return nil, err
			}
		}
		needles[i] = normalizeExcerpt(selector.Text)
		if len(needles[i]) > maxNeedle {
			maxNeedle = len(needles[i])
		}
		results[i].Status = "not_found"
		if needles[i] == "" {
			results[i].Status = "no_evidence"
		}
	}
	failed := make([]bool, len(selectors))
	var accepted []preparedSource
	for _, prepared := range c.sources {
		indices := []int{}
		for i, selector := range selectors {
			if needles[i] != "" && (selector.SourceID == "" || selector.SourceID == prepared.summary.ID) {
				indices = append(indices, i)
			}
		}
		if len(indices) == 0 {
			continue
		}
		local := prepared
		local.summary.Aliases = append([]string(nil), prepared.summary.Aliases...)
		matches := make([]ExcerptLocation, len(selectors))
		var tail []byte
		var err error
		if !prepared.summary.Complete {
			err = fmt.Errorf("source unavailable")
		} else {
			err = scanSource(ctx, &local, c.opts, func(record Record) {
				window := append(append([]byte(nil), tail...), record.RawBytes...)
				start := record.ByteStart - int64(len(tail))
				line := record.FirstLine - int64(bytes.Count(tail, []byte{'\n'}))
				normalized, offsets := normalizedExcerptOffsets(window)
				for _, i := range indices {
					for from := 0; from <= len(normalized)-len(needles[i]); {
						at := strings.Index(normalized[from:], needles[i])
						if at < 0 {
							break
						}
						at += from
						lo, hi := offsets[at], offsets[at+len(needles[i])]
						from = at + 1
						if start+int64(hi) <= record.ByteStart {
							continue
						}
						matches[i].Matches++
						if matches[i].Matches != 1 {
							continue
						}
						matches[i].FirstLine = line + int64(bytes.Count(window[:lo], []byte{'\n'}))
						matches[i].LastLine = line + int64(bytes.Count(window[:hi-1], []byte{'\n'}))
						matches[i].ByteStart, matches[i].ByteEnd = start+int64(lo), start+int64(hi)
						matches[i].matchedExcerpt = needles[i]
						lineStart := strings.LastIndex(normalized[:at], "\n") + 1
						lineEnd := strings.IndexByte(normalized[at:], '\n')
						if lineEnd < 0 {
							lineEnd = len(normalized)
						} else {
							lineEnd += at
						}
						// Do not retain a giant source line through a short excerpt.
						// Over-budget lines keep their location but not literal proof.
						if lineEnd-lineStart <= 4096 {
							matches[i].matchedLine = strings.Clone(normalized[lineStart:lineEnd])
						}
						if matches[i].ByteStart >= record.ByteStart {
							matches[i].RecordID = record.ID
						}
					}
				}
				keep := 2 * maxNeedle
				if keep > len(window) {
					keep = len(window)
				}
				tail = append(tail[:0], window[len(window)-keep:]...)
			}, true)
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		if err != nil {
			for _, i := range indices {
				failed[i] = true
			}
			continue
		}
		accepted = append(accepted, local)
		for _, i := range indices {
			count := results[i].Matches + matches[i].Matches
			if results[i].Matches == 0 && matches[i].Matches > 0 {
				results[i] = matches[i]
				source := local.summary
				source.Aliases = append([]string(nil), source.Aliases...)
				results[i].Source = &source
			}
			results[i].Matches = count
		}
	}
	// A prior member may change while a later member is being read.
	for _, source := range accepted {
		if source.summary.Path == "" {
			continue
		}
		for _, path := range append([]string{source.summary.Path}, source.summary.Aliases...) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			current, err := filegeneration.FromPath(path)
			if err != nil || !source.identity.SameVersion(current) {
				for i, selector := range selectors {
					if selector.SourceID == "" || selector.SourceID == source.summary.ID {
						failed[i] = true
					}
				}
			}
		}
	}
	for i := range results {
		r := &results[i]
		switch {
		case needles[i] == "":
			r.Status = "no_evidence"
		case failed[i]:
			r.Status = "source_unavailable"
		case r.Matches == 0:
			r.Status = "not_found"
		case r.Matches > 1:
			r.Status = "ambiguous"
		default:
			r.Status = "unique"
			r.proof = r.digest()
			continue
		}
		*r = ExcerptLocation{Status: r.Status, Matches: r.Matches}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func normalizeExcerpt(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}

func normalizedExcerptOffsets(raw []byte) (string, []int) {
	out := make([]byte, 0, len(raw))
	offsets := make([]int, 0, len(raw)+1)
	for i := 0; i < len(raw); i++ {
		offsets = append(offsets, i)
		if raw[i] == '\r' {
			out = append(out, '\n')
			if i+1 < len(raw) && raw[i+1] == '\n' {
				i++
			}
		} else {
			out = append(out, raw[i])
		}
	}
	return string(out), append(offsets, len(raw))
}
