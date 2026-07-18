package hitraceconv

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

const simpleperfTextProfileMaxMetaEntries = 256

type simpleperfTextProfileMode uint8

const (
	simpleperfTextProfileUnknown simpleperfTextProfileMode = iota
	simpleperfTextProfileOrdinary
	simpleperfTextProfileTraceOffCPU
)

type simpleperfTextProfile struct {
	mode         simpleperfTextProfileMode
	onCPUEvent   string
	offCPUEvents map[string]struct{}
	reason       string
}

func inspectSimpleperfTextProfile(ctx context.Context, input directPerfInputBinding) (profile simpleperfTextProfile, err error) {
	profile = simpleperfTextProfile{mode: simpleperfTextProfileUnknown, reason: "profile_metadata_unreadable"}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := input.validate(); err != nil {
		return profile, err
	}
	if err := completeConversionInputStage(ctx, input.input, input.stage, nil); err != nil {
		return profile, err
	}
	defer func() {
		err = completeConversionInputStage(ctx, input.input, input.stage, err)
	}()
	if input.inputFormat != perfInputLinuxPerfData {
		profile.reason = "profile_input_format_unsupported"
		return profile, nil
	}

	header, readErr := readRawPerfHeader(input.input, input.inputSize)
	if readErr != nil {
		profile.reason = "profile_header_malformed"
		return profile, nil
	}
	if err := ctx.Err(); err != nil {
		return profile, err
	}
	meta, present, readErr := readSimpleperfMetaInfoSection(ctx, input.input, header, input.inputSize)
	if readErr != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return profile, ctxErr
		}
		profile.reason = "profile_meta_info_malformed"
		return profile, nil
	}
	if !present {
		return simpleperfTextProfile{mode: simpleperfTextProfileOrdinary, reason: "trace_offcpu_absent"}, nil
	}
	values, parseErr := parseSimpleperfMetaInfoStrict(meta)
	if parseErr != nil {
		profile.reason = "profile_meta_info_malformed"
		return profile, nil
	}
	switch values["trace_offcpu"] {
	case "", "false":
		return simpleperfTextProfile{mode: simpleperfTextProfileOrdinary, reason: "trace_offcpu_absent_or_false"}, nil
	case "true":
	default:
		profile.reason = "trace_offcpu_value_unknown"
		return profile, nil
	}
	onCPUEvent, offCPUEvents, parseErr := parseSimpleperfEventTypeInfoStrict(values["event_type_info"])
	if parseErr != nil {
		profile.reason = "event_type_info_malformed"
		return profile, nil
	}
	return simpleperfTextProfile{
		mode:         simpleperfTextProfileTraceOffCPU,
		onCPUEvent:   onCPUEvent,
		offCPUEvents: offCPUEvents,
		reason:       "trace_offcpu_event_order_proved",
	}, nil
}

func readSimpleperfMetaInfoSection(ctx context.Context, r io.ReaderAt, header rawPerfFileHeader, fileSize int64) ([]byte, bool, error) {
	ids := rawPerfFeatureIDs(header.Features)
	metaIndex := -1
	for index, id := range ids {
		if id == perfFeatureMetaInfo {
			metaIndex = index
			break
		}
	}
	if metaIndex < 0 {
		return nil, false, nil
	}
	if len(ids) == 0 || len(ids) > 128 || fileSize < 0 {
		return nil, true, fmt.Errorf("invalid feature descriptor inventory")
	}
	descriptorOffset, ok := checkedSimpleperfProfileAdd(header.DataOffset, header.DataSize)
	if !ok || descriptorOffset == 0 {
		return nil, true, fmt.Errorf("invalid feature descriptor offset")
	}
	descriptorBytes := uint64(len(ids)) * 16
	descriptorEnd, ok := checkedSimpleperfProfileAdd(descriptorOffset, descriptorBytes)
	if !ok || descriptorEnd > uint64(fileSize) {
		return nil, true, fmt.Errorf("feature descriptor table exceeds fixed input")
	}
	entryOffset, ok := checkedSimpleperfProfileAdd(descriptorOffset, uint64(metaIndex)*16)
	if !ok || entryOffset > uint64(fileSize) || uint64(fileSize)-entryOffset < 16 {
		return nil, true, fmt.Errorf("META_INFO descriptor exceeds fixed input")
	}
	var descriptor [16]byte
	if err := ctx.Err(); err != nil {
		return nil, true, err
	}
	if _, err := r.ReadAt(descriptor[:], int64(entryOffset)); err != nil {
		return nil, true, fmt.Errorf("read META_INFO descriptor: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, true, err
	}
	sectionOffset := binary.LittleEndian.Uint64(descriptor[0:8])
	sectionSize := binary.LittleEndian.Uint64(descriptor[8:16])
	if sectionSize == 0 || sectionSize > rawPerfFeatureMaxSectionSize(perfFeatureMetaInfo) ||
		sectionOffset < descriptorEnd || sectionOffset > uint64(fileSize) || sectionSize > uint64(fileSize)-sectionOffset {
		return nil, true, fmt.Errorf("invalid META_INFO section range")
	}
	section := make([]byte, int(sectionSize))
	if _, err := r.ReadAt(section, int64(sectionOffset)); err != nil {
		return nil, true, fmt.Errorf("read META_INFO section: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, true, err
	}
	return section, true, nil
}

func checkedSimpleperfProfileAdd(left, right uint64) (uint64, bool) {
	if right > ^uint64(0)-left {
		return 0, false
	}
	return left + right, true
}

func parseSimpleperfMetaInfoStrict(data []byte) (map[string]string, error) {
	values := make(map[string]string)
	offset := 0
	for offset < len(data) {
		if len(values) >= simpleperfTextProfileMaxMetaEntries {
			return nil, fmt.Errorf("META_INFO entry count exceeds %d", simpleperfTextProfileMaxMetaEntries)
		}
		keyEnd := indexByte(data[offset:], 0)
		if keyEnd <= 0 {
			return nil, fmt.Errorf("META_INFO key is empty or unterminated")
		}
		keyBytes := data[offset : offset+keyEnd]
		offset += keyEnd + 1
		if offset >= len(data) {
			return nil, fmt.Errorf("META_INFO value is missing")
		}
		valueEnd := indexByte(data[offset:], 0)
		if valueEnd <= 0 {
			return nil, fmt.Errorf("META_INFO value is empty or unterminated")
		}
		valueBytes := data[offset : offset+valueEnd]
		offset += valueEnd + 1
		if !utf8.Valid(keyBytes) || !utf8.Valid(valueBytes) {
			return nil, fmt.Errorf("META_INFO contains invalid UTF-8")
		}
		key := string(keyBytes)
		if _, duplicate := values[key]; duplicate {
			return nil, fmt.Errorf("META_INFO contains duplicate key %q", key)
		}
		values[key] = string(valueBytes)
	}
	if offset != len(data) {
		return nil, fmt.Errorf("META_INFO has trailing bytes")
	}
	return values, nil
}

func parseSimpleperfEventTypeInfoStrict(raw string) (string, map[string]struct{}, error) {
	if raw == "" || strings.HasSuffix(raw, "\n") || strings.Contains(raw, "\r") {
		return "", nil, fmt.Errorf("event_type_info is empty or has a non-canonical boundary")
	}
	rows := strings.Split(raw, "\n")
	if len(rows) < 2 {
		return "", nil, fmt.Errorf("event_type_info has no off-CPU event")
	}
	events := make([]string, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		fields := strings.Split(row, ",")
		if len(fields) != 3 || fields[0] == "" || !utf8.ValidString(fields[0]) {
			return "", nil, fmt.Errorf("event_type_info row is malformed")
		}
		for _, scalar := range fields[1:] {
			value, err := strconv.ParseUint(scalar, 10, 64)
			if err != nil || strconv.FormatUint(value, 10) != scalar {
				return "", nil, fmt.Errorf("event_type_info scalar is non-canonical")
			}
		}
		if _, duplicate := seen[fields[0]]; duplicate {
			return "", nil, fmt.Errorf("event_type_info contains duplicate event")
		}
		seen[fields[0]] = struct{}{}
		events = append(events, fields[0])
	}
	offCPUEvents := make(map[string]struct{}, len(events)-1)
	for _, event := range events[1:] {
		offCPUEvents[event] = struct{}{}
	}
	return events[0], offCPUEvents, nil
}

func prepareSimpleperfTextProfile(ctx context.Context, opts Options, input directPerfInputBinding, resolution simpleperfToolResolution) (simpleperfTextProfile, error) {
	profile, err := inspectSimpleperfTextProfile(ctx, input)
	if err != nil || profile.mode != simpleperfTextProfileTraceOffCPU {
		return profile, err
	}
	output, runErr := runSimpleperfTextProfileHelp(ctx, opts, resolution)
	if runErr != nil {
		if errors.Is(runErr, errExternalToolSupervisorAuthority) || (ctx != nil && ctx.Err() != nil) {
			return profile, runErr
		}
		profile.mode = simpleperfTextProfileUnknown
		profile.reason = "trace_offcpu_help_unavailable"
		return profile, nil
	}
	for _, token := range []string{"--trace-offcpu", "on-off-cpu", "mixed-on-off-cpu"} {
		if !simpleperfHelpHasExactToken(string(output), token) {
			profile.mode = simpleperfTextProfileUnknown
			profile.reason = "trace_offcpu_help_incomplete"
			return profile, nil
		}
	}
	profile.reason = "trace_offcpu_on_off_mode_proved"
	return profile, nil
}

func runSimpleperfTextProfileHelp(ctx context.Context, opts Options, resolution simpleperfToolResolution) ([]byte, error) {
	name := resolution.Tool
	args := []string{"--help"}
	if resolution.Python != "" {
		name = resolution.Python
		args = []string{resolution.Tool, "--help"}
	}
	command, err := newExternalToolCommand(ctx, name, args...)
	if err != nil {
		return nil, err
	}
	output, runErr, _, _ := runCommandWithProgressUntilExit(opts, command, "simpleperf_profile_help", "checking official simpleperf profile support")
	return output, runErr
}

func simpleperfHelpHasExactToken(help, token string) bool {
	for start := 0; start < len(help); {
		for start < len(help) && !simpleperfHelpTokenByte(help[start]) {
			start++
		}
		end := start
		for end < len(help) && simpleperfHelpTokenByte(help[end]) {
			end++
		}
		if end > start && help[start:end] == token {
			return true
		}
		start = end + 1
	}
	return false
}

func simpleperfHelpTokenByte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9' || value == '_' || value == '-'
}

func applySimpleperfTextProfile(samples []simpleperfSample, profile simpleperfTextProfile) error {
	for index := range samples {
		switch profile.mode {
		case simpleperfTextProfileUnknown:
			samples[index].SampleKind = ""
		case simpleperfTextProfileOrdinary:
			samples[index].SampleKind = tracewire.PerfSampleKindOnCPU
		case simpleperfTextProfileTraceOffCPU:
			switch samples[index].Event {
			case profile.onCPUEvent:
				samples[index].SampleKind = tracewire.PerfSampleKindOnCPU
			default:
				if _, ok := profile.offCPUEvents[samples[index].Event]; !ok {
					return fmt.Errorf("simpleperf report event %q is outside proved event_type_info", samples[index].Event)
				}
				samples[index].SampleKind = tracewire.PerfSampleKindOffCPU
			}
		default:
			return fmt.Errorf("simpleperf text profile mode is invalid")
		}
	}
	return nil
}

func (profile simpleperfTextProfile) caveat() string {
	switch profile.mode {
	case simpleperfTextProfileOrdinary:
		return "simpleperf text samples are marked on_cpu only because immutable perf metadata proves trace_offcpu absent or exactly false"
	case simpleperfTextProfileTraceOffCPU:
		return "simpleperf trace-offcpu sample kinds are mapped from exact ordered event_type_info after explicit on-off-cpu capability negotiation"
	default:
		return "simpleperf text sample kind remains unknown because the exact capture profile or on-off-cpu adapter capability was not proved (" + firstNonEmpty(profile.reason, "unknown") + ")"
	}
}
