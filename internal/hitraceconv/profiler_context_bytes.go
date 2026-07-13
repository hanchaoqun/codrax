package hitraceconv

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"
)

// profilerContextByteCheckpointBytes is the common byte-work cancellation
// interval for ProfilerPluginData metadata and strict-text scans. Callers that
// use profilerByteContextCheckpoint must keep each delta at or below this
// value; the helper accounts cumulative bytes across adjacent fields/chunks.
const profilerContextByteCheckpointBytes = 64 << 10

// profilerByteContextCheckpoint registers the next delta bytes before a
// caller processes them. It checks the request on the first call and whenever
// the cumulative byte count crosses a 64 KiB boundary. Callers must also check
// ctx.Err immediately before returning a completed result so cancellation
// cannot escape through a short final chunk.
func profilerByteContextCheckpoint(ctx context.Context, processed *uint64, delta uint64) error {
	if processed == nil {
		return &traceDBOutputInvariantError{Reason: "profiler_byte_checkpoint_nil"}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if delta > profilerContextByteCheckpointBytes {
		return &traceDBOutputInvariantError{Reason: "profiler_byte_checkpoint_delta"}
	}
	previous := *processed
	if delta > ^uint64(0)-previous {
		return &traceDBOutputInvariantError{Reason: "profiler_byte_checkpoint_overflow"}
	}
	next := previous + delta
	if previous == 0 || previous/uint64(profilerContextByteCheckpointBytes) != next/uint64(profilerContextByteCheckpointBytes) {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	*processed = next
	return nil
}

type profilerPhysicalRuneFacts struct {
	allSpace  bool
	tokenSafe bool
}

// profilerPhysicalRunesSafeBytesContext is the uncapped byte authority for
// the physical-rune grammar. It intentionally makes no size or blankness
// decision: oversized strict-text provenance needs to distinguish a physical
// row from one containing invalid UTF-8, controls, Zl, or Zp.
func profilerPhysicalRunesSafeBytesContext(ctx context.Context, raw []byte) (bool, error) {
	_, valid, err := profilerPhysicalRuneFactsBytesContext(ctx, raw)
	return valid, err
}

// profilerSinglePhysicalLineBytesContext is byte-for-byte equivalent to
// traceDBSinglePhysicalLine without allocating an intermediate string.
func profilerSinglePhysicalLineBytesContext(ctx context.Context, raw []byte, allowBlank bool) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if len(raw) > maxTraceDBSystraceLineBytes {
		return false, nil
	}
	facts, valid, err := profilerPhysicalRuneFactsBytesContext(ctx, raw)
	if err != nil || !valid {
		return false, err
	}
	return allowBlank || !facts.allSpace, nil
}

// profilerSinglePhysicalLineStringContext is the zero-copy string adapter for
// the same physical-rune authority used by byte-backed protobuf fields.
func profilerSinglePhysicalLineStringContext(ctx context.Context, raw string, allowBlank bool) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if len(raw) > maxTraceDBSystraceLineBytes {
		return false, nil
	}
	facts, valid, err := profilerPhysicalRuneFactsStringContext(ctx, raw)
	if err != nil || !valid {
		return false, err
	}
	return allowBlank || !facts.allSpace, nil
}

// profilerSingleTokenBytesContext is byte-for-byte equivalent to
// traceDBSingleToken. In particular, every Unicode whitespace rune and the
// public key/value separators '=' and '|' are forbidden.
func profilerSingleTokenBytesContext(ctx context.Context, raw []byte) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if len(raw) == 0 || len(raw) > maxTraceDBSystraceLineBytes {
		return false, nil
	}
	facts, valid, err := profilerPhysicalRuneFactsBytesContext(ctx, raw)
	if err != nil || !valid {
		return false, err
	}
	return !facts.allSpace && facts.tokenSafe, nil
}

// profilerSingleTokenStringContext is the zero-copy string adapter for the
// same token authority used by byte-backed protobuf fields.
func profilerSingleTokenStringContext(ctx context.Context, raw string) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if len(raw) == 0 || len(raw) > maxTraceDBSystraceLineBytes {
		return false, nil
	}
	facts, valid, err := profilerPhysicalRuneFactsStringContext(ctx, raw)
	if err != nil || !valid {
		return false, err
	}
	return !facts.allSpace && facts.tokenSafe, nil
}

// profilerCloneBytesStringContext clones raw in bounded chunks. Cancellation
// never exposes a partial string, including when it arrives after the final
// copy but before publication.
func profilerCloneBytesStringContext(ctx context.Context, raw []byte) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var clone strings.Builder
	clone.Grow(len(raw))
	processed := uint64(0)
	for start := 0; start < len(raw); {
		end := min(start+profilerContextByteCheckpointBytes, len(raw))
		if err := profilerByteContextCheckpoint(ctx, &processed, uint64(end-start)); err != nil {
			return "", err
		}
		_, _ = clone.Write(raw[start:end])
		start = end
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return clone.String(), nil
}

// profilerTrimASCIISpacesBytesContext mirrors bytes.Trim(raw, " ") without
// allowing a single all-space physical row to bypass the shared byte-work
// cancellation bound. The returned view always aliases raw; no evidence bytes
// are copied or normalized.
func profilerTrimASCIISpacesBytesContext(ctx context.Context, raw []byte) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	left, right := 0, len(raw)
	processed := uint64(0)
	pending := uint64(0)
	for left < right && raw[left] == ' ' {
		left++
		pending++
		if pending == profilerContextByteCheckpointBytes {
			if err := profilerByteContextCheckpoint(ctx, &processed, pending); err != nil {
				return nil, err
			}
			pending = 0
		}
	}
	for right > left && raw[right-1] == ' ' {
		right--
		pending++
		if pending == profilerContextByteCheckpointBytes {
			if err := profilerByteContextCheckpoint(ctx, &processed, pending); err != nil {
				return nil, err
			}
			pending = 0
		}
	}
	if err := profilerByteContextCheckpoint(ctx, &processed, pending); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return raw[left:right], nil
}

func profilerPhysicalRuneFactsBytesContext(ctx context.Context, raw []byte) (profilerPhysicalRuneFacts, bool, error) {
	return profilerPhysicalRuneFactsContext(ctx, len(raw), func(offset int) (rune, int) {
		return utf8.DecodeRune(raw[offset:])
	})
}

func profilerPhysicalRuneFactsStringContext(ctx context.Context, raw string) (profilerPhysicalRuneFacts, bool, error) {
	return profilerPhysicalRuneFactsContext(ctx, len(raw), func(offset int) (rune, int) {
		return utf8.DecodeRuneInString(raw[offset:])
	})
}

func profilerPhysicalRuneFactsContext(ctx context.Context, length int, decode func(int) (rune, int)) (profilerPhysicalRuneFacts, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return profilerPhysicalRuneFacts{}, false, err
	}
	if length < 0 || decode == nil {
		return profilerPhysicalRuneFacts{}, false, &traceDBOutputInvariantError{Reason: "profiler_physical_rune_source_invalid"}
	}
	facts := profilerPhysicalRuneFacts{allSpace: true, tokenSafe: true}
	nextCheckpoint := profilerContextByteCheckpointBytes
	for offset := 0; offset < length; {
		r, width := decode(offset)
		if r == utf8.RuneError && width == 1 {
			return profilerPhysicalRuneFacts{}, false, nil
		}
		if width <= 0 || width > length-offset {
			return profilerPhysicalRuneFacts{}, false, &traceDBOutputInvariantError{Reason: "profiler_physical_rune_width_invalid"}
		}
		offset += width
		if offset >= nextCheckpoint {
			if err := ctx.Err(); err != nil {
				return profilerPhysicalRuneFacts{}, false, err
			}
			nextCheckpoint = (offset/profilerContextByteCheckpointBytes + 1) * profilerContextByteCheckpointBytes
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) {
			return profilerPhysicalRuneFacts{}, false, nil
		}
		space := unicode.IsSpace(r)
		if !space {
			facts.allSpace = false
		}
		if space || r == '=' || r == '|' {
			facts.tokenSafe = false
		}
	}
	if err := ctx.Err(); err != nil {
		return profilerPhysicalRuneFacts{}, false, err
	}
	return facts, true, nil
}
