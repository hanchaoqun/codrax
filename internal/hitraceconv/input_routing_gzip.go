package hitraceconv

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// Includes the outer-format prefix already consumed by the public probe, gzip
// headers and inflater read-ahead. Empty deflate blocks cannot evade this cap.
const gzipRoutingProbeCompressedBytes int64 = 1 << 20

// This is an output-prefix budget, independent of the actual compressed-input
// budget above. Missing a header within it remains unknown, not unsupported.
const gzipRoutingProbeTextBytes = 16 << 10

var errGzipRoutingProbeBudget = errors.New("gzip routing compressed-input budget exhausted")

// This is navigation, not another decoder: existing binary leaf signatures or
// a physical ftrace header can offer a reader. No extraction, receipt, full CRC,
// trailing-byte validation or sample fact is claimed. PrepareFile still validates
// the entire one-member transport before any native query, including short prefixes.
func probeGzipRoutingLeaf(ctx context.Context, source *conversionInputAuthority, consumed int64) (known bool, err error) {
	limited := &gzipRoutingProbeReader{ctx: ctx, reader: io.NewSectionReader(source, 0, source.Size()), remaining: gzipRoutingProbeCompressedBytes - consumed}
	defer func() {
		err = errors.Join(err, limited.failure, ctx.Err())
		if err != nil {
			known = false
		}
	}()
	reader, err := gzip.NewReader(limited)
	if err != nil {
		return false, err
	}
	defer func() { err = errors.Join(err, reader.Close()) }()
	reader.Multistream(false)
	var prefix [conversionInputProbeSize]byte
	n, err := io.ReadFull(reader, prefix[:])
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return false, err
	}
	if traceArchiveZIPKnownLeafMagic(prefix[:n]) {
		return true, nil
	}
	text := make([]byte, gzipRoutingProbeTextBytes)
	copy(text, prefix[:n])
	if n == len(prefix) {
		more, readErr := io.ReadFull(reader, text[n:])
		n += more
		if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
			return false, readErr
		}
	}
	// Elect only the first physical data row, never a trace-looking fragment
	// embedded after prose, binary metadata or an oversized comment. Reuse the
	// parser's anchored origin grammar, without parsing event payloads or an
	// event-name allowlist. A single header supplies navigation, never facts.
	for _, line := range bytes.Split(text[:n], []byte{'\n'}) {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 || trimmed[0] == '#' {
			continue
		}
		_, ok := tracequery.ProbeLeadingExactEventNamePrefix(string(line))
		return ok, nil
	}
	return false, nil
}

func gzipRoutingProbeFailureReason(err error) string {
	if errors.Is(err, errGzipRoutingProbeBudget) {
		return "gzip_probe_input_budget"
	}
	return "gzip_prefix_probe_unavailable"
}

type gzipRoutingProbeReader struct {
	ctx       context.Context
	reader    io.Reader
	remaining int64
	failure   error
}

func (r *gzipRoutingProbeReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		r.failure = err
		return 0, err
	}
	if len(p) == 0 {
		return 0, nil
	}
	if r.remaining <= 0 {
		r.failure = errGzipRoutingProbeBudget
		return 0, r.failure
	}
	n, err := r.reader.Read(p[:min(int64(len(p)), r.remaining)])
	r.remaining -= int64(n)
	return n, err
}
