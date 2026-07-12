package hitraceconv

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	segmentEventsFormat = 1
	segmentCmdlines     = 2
	segmentTGIDs        = 3
	segmentRawTrace     = 4
	segmentHeaderPage   = 30
	segmentPrintk       = 31
	segmentKallsyms     = 32

	fileHeaderSize  = 12 // Python struct "@HBHI" on little-endian hosts.
	segmentHdrSize  = 8  // Python struct "@II".
	pageHeaderSize  = 17 // Python struct "@QQB".
	eventHeaderSize = 6  // Python struct "@IH".
	tracePageSize   = 4096

	// The built-in sys decoder implements the Harmony microkernel RMQ page
	// layout (RmqConsumerData/RmqEntry). OpenHarmony/Linux raw captures use a
	// different ring-buffer page and event-header layout and must be handled by
	// trace_streamer until a dedicated built-in decoder exists.
	harmonyRMQFileType = 1
)

const (
	builtinSysDecodeUnsupportedFileType  = "unsupported_file_type"
	builtinSysDecodePartialPageSegment   = "partial_page_segment"
	builtinSysDecodePageLength           = "page_length_out_of_range"
	builtinSysDecodeTruncatedEventHeader = "truncated_event_header"
	builtinSysDecodeEventBounds          = "event_out_of_bounds"
	builtinSysDecodeTimestampOverflow    = "timestamp_overflow"
)

// BuiltinSysDecodeError is a fail-closed, machine-readable rejection from the
// built-in Harmony RMQ decoder. Code is stable for tests and callers; Detail is
// diagnostic text and must not be parsed for control flow.
type BuiltinSysDecodeError struct {
	Code        string
	FileType    uint8
	SegmentType uint32
	Offset      int64
	Detail      string
}

func (e *BuiltinSysDecodeError) Error() string {
	if e == nil {
		return "built-in sys decoder rejected input"
	}
	return fmt.Sprintf("built-in sys decoder rejected input: code=%s file_type=%d segment_type=%d offset=%d: %s",
		e.Code, e.FileType, e.SegmentType, e.Offset, e.Detail)
}

type fileHeader struct {
	Magic    uint16
	FileType uint8
	Version  uint16
	Reserved uint32
	CPUNum   int
}

type segmentMeta struct {
	Type   uint32
	Size   uint32
	Offset int64
}

type pageHeader struct {
	TimestampNS uint64
	Length      uint64
	CPU         int
}

type eventHeader struct {
	TimestampOffsetNS uint32
	Size              uint16
	AlignedSize       int
}

func readFileHeader(r io.Reader) (fileHeader, error) {
	var buf [fileHeaderSize]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return fileHeader{}, fmt.Errorf("read hitrace file header: %w", err)
	}
	reserved := binary.LittleEndian.Uint32(buf[8:12])
	cpuNum := int((reserved >> 1) & 0x1f)
	if cpuNum <= 0 {
		cpuNum = 1
	}
	return fileHeader{
		Magic:    binary.LittleEndian.Uint16(buf[0:2]),
		FileType: buf[2],
		Version:  binary.LittleEndian.Uint16(buf[4:6]),
		Reserved: reserved,
		CPUNum:   cpuNum,
	}, nil
}

func readSegmentHeader(r io.Reader) (typ uint32, size uint32, err error) {
	var buf [segmentHdrSize]byte
	if _, err = io.ReadFull(r, buf[:]); err != nil {
		return 0, 0, err
	}
	return binary.LittleEndian.Uint32(buf[0:4]), binary.LittleEndian.Uint32(buf[4:8]), nil
}

func parsePageHeader(data []byte) (pageHeader, bool) {
	if len(data) < pageHeaderSize {
		return pageHeader{}, false
	}
	return pageHeader{
		TimestampNS: binary.LittleEndian.Uint64(data[0:8]),
		Length:      binary.LittleEndian.Uint64(data[8:16]),
		CPU:         int(data[16]),
	}, true
}

func parseEventHeader(data []byte) (eventHeader, bool) {
	if len(data) < eventHeaderSize {
		return eventHeader{}, false
	}
	size := binary.LittleEndian.Uint16(data[4:6])
	if size == 0 {
		return eventHeader{}, false
	}
	aligned := int((uint32(size) + 3) &^ 3)
	return eventHeader{
		TimestampOffsetNS: binary.LittleEndian.Uint32(data[0:4]),
		Size:              size,
		AlignedSize:       aligned,
	}, true
}

func isRawTraceSegment(typ uint32, cpuNum int) bool {
	if typ == segmentRawTrace {
		return true
	}
	// Metadata segment IDs are fixed protocol authorities. On captures with
	// 27+ CPUs their numeric IDs overlap the legacy per-CPU raw range; metadata
	// must win or printk/header/kallsyms bytes are mis-decoded as RMQ pages.
	if isIgnoredSegment(typ) {
		return false
	}
	return typ > segmentRawTrace && typ <= segmentRawTrace+uint32(cpuNum)
}

func isIgnoredSegment(typ uint32) bool {
	return typ == segmentHeaderPage || typ == segmentPrintk || typ == segmentKallsyms
}
