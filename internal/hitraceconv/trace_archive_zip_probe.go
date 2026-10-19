package hitraceconv

import (
	"archive/zip"
	"compress/flate"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	traceArchiveZIPProbeMemberBytes = int64(1 << 20)
	traceArchiveZIPProbeTotalBytes  = int64(8 << 20)
)

type traceArchiveZIPProbeBudget struct {
	remaining int64
	perMember int64
}

// A prefix is only candidate discovery. OpenRaw reads the held archive's
// section, never a member path. Count compressed bytes before flate (including
// its read-ahead); a stream of empty deflate blocks cannot evade the budget.
// The selected member is subsequently read in full through zip.File.Open,
// which retains its CRC, length, generation and immutable receipt checks.
func probeTraceArchiveZIPLeaf(ctx context.Context, file *zip.File, budget *traceArchiveZIPProbeBudget) (known bool, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := validateTraceArchiveZIPMemberSize(file, true); err != nil {
		return false, err
	}
	if budget == nil || budget.perMember <= 0 || budget.remaining <= 0 {
		return false, traceArchiveFailure(traceArchiveCodeResourceLimit, file.Name, errors.New("ZIP candidate compressed-input budget exhausted"))
	}
	if file.Method != zip.Store && file.Method != zip.Deflate {
		return false, traceArchiveFailure(traceArchiveCodeMemberIntegrity, file.Name, zip.ErrAlgorithm)
	}
	raw, err := file.OpenRaw()
	if err != nil {
		return false, traceArchiveFailure(traceArchiveCodeMemberIntegrity, file.Name, err)
	}
	limited := &traceArchiveZIPProbeReader{ctx: ctx, reader: raw, budget: budget, remaining: budget.perMember, member: file.Name}
	var reader io.Reader = limited
	if file.Method == zip.Deflate {
		inflater := flate.NewReader(limited)
		reader = inflater
		defer func() { err = errors.Join(err, inflater.Close()) }()
	}
	defer func() {
		err = errors.Join(err, ctx.Err(), limited.failure)
		if err != nil {
			known = false
		}
	}()
	var probe [conversionInputProbeSize]byte
	n, readErr := io.ReadFull(reader, probe[:])
	if limited.failure != nil {
		return false, limited.failure
	}
	if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		return false, traceArchiveFailure(traceArchiveCodeMemberIntegrity, file.Name, readErr)
	}
	if uint64(n) > file.UncompressedSize64 || (n < len(probe) && uint64(n) != file.UncompressedSize64) {
		return false, traceArchiveFailure(traceArchiveCodeMemberIntegrity, file.Name, errors.New("ZIP candidate prefix disagrees with declared member size"))
	}
	return traceArchiveZIPKnownLeafMagic(probe[:n]), nil
}

func traceArchiveZIPKnownLeafMagic(probe []byte) bool {
	switch detectPerfInputFormatProbe(probe) {
	case perfInputLinuxPerfData, perfInputSimpleperfReportProto:
		return true
	}
	return (len(probe) >= 2 && binary.LittleEndian.Uint16(probe[:2]) == harmonyRMQMagic) ||
		(len(probe) >= 8 && binary.LittleEndian.Uint64(probe[:8]) == profilerTraceHeaderMagic)
}

type traceArchiveZIPProbeReader struct {
	ctx       context.Context
	reader    io.Reader
	budget    *traceArchiveZIPProbeBudget
	remaining int64
	member    string
	failure   error
}

func (reader *traceArchiveZIPProbeReader) Read(out []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		reader.failure = err
		return 0, err
	}
	if len(out) == 0 {
		return 0, nil
	}
	if reader.remaining <= 0 || reader.budget.remaining <= 0 {
		reader.failure = traceArchiveFailure(traceArchiveCodeResourceLimit, reader.member, errors.New("ZIP candidate compressed-input budget exhausted"))
		return 0, reader.failure
	}
	limit := min(int64(len(out)), reader.remaining, reader.budget.remaining)
	n, err := reader.reader.Read(out[:int(limit)])
	reader.remaining -= int64(n)
	reader.budget.remaining -= int64(n)
	return n, err
}

func validateTraceArchiveZIPMemberSize(file *zip.File, allowEmpty bool) error {
	if file == nil {
		return traceArchiveFailure(traceArchiveCodeInvalidMember, "", errors.New("missing ZIP member"))
	}
	compressed, uncompressed := file.CompressedSize64, file.UncompressedSize64
	if (!allowEmpty && uncompressed == 0) || compressed > traceArchiveZIPMaxMemberCompressed || uncompressed > traceArchiveZIPMaxMemberUncompressed ||
		(compressed == 0 && uncompressed != 0) {
		return traceArchiveFailure(traceArchiveCodeResourceLimit, file.Name, fmt.Errorf("ZIP member exceeds size/ratio limits: compressed=%d uncompressed=%d", compressed, uncompressed))
	}
	// Both sizes are bounded before multiplication, independent of host width.
	if compressed != 0 && uncompressed > compressed*traceArchiveZIPMaxCompressionRatio {
		return traceArchiveFailure(traceArchiveCodeResourceLimit, file.Name, fmt.Errorf("ZIP member compression ratio exceeds %d", traceArchiveZIPMaxCompressionRatio))
	}
	return nil
}
