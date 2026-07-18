package tracewire

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// PerfSampleLayout selects the closed late-field shape of a Codrax-owned
// perf_sample row. The common prefix and quality fields have one fixed order;
// only raw perf and the two SQL identity lanes have additional fields.
type PerfSampleLayout uint8

const (
	PerfSampleLayoutBase PerfSampleLayout = iota + 1
	PerfSampleLayoutRawExtended
	PerfSampleLayoutResolvedIdentity
	PerfSampleLayoutSourceOnlyIdentity
)

type PerfSampleSource string

const (
	PerfSampleSourceSimpleperfReportSample PerfSampleSource = "simpleperf_report_sample"
	PerfSampleSourceSimpleperfReportProto  PerfSampleSource = "simpleperf_report_proto"
	PerfSampleSourceHiperfProto            PerfSampleSource = "hiperf_proto"
	PerfSampleSourceRawPerfDataFallback    PerfSampleSource = "raw_perfdata_fallback"
	PerfSampleSourceTraceStreamerDB        PerfSampleSource = "trace_streamer_db"
)

type PerfSampleKind string

const (
	PerfSampleKindOnCPU   PerfSampleKind = "on_cpu"
	PerfSampleKindOffCPU  PerfSampleKind = "off_cpu"
	PerfSampleKindUnknown PerfSampleKind = "unknown"
)

type PerfSymbolizationStatus string

const (
	PerfSymbolizationSymbolized   PerfSymbolizationStatus = "symbolized"
	PerfSymbolizationUnsymbolized PerfSymbolizationStatus = "unsymbolized"
	PerfSymbolizationPartial      PerfSymbolizationStatus = "partial"
	PerfSymbolizationUnknown      PerfSymbolizationStatus = "unknown"
)

type PerfCallchainStatus string

const (
	PerfCallchainStatusSymbolized PerfCallchainStatus = "symbolized"
	PerfCallchainStatusPartial    PerfCallchainStatus = "partial"
	PerfCallchainStatusIPOnly     PerfCallchainStatus = "ip_only"
	PerfCallchainStatusMissing    PerfCallchainStatus = "missing"
)

type PerfSampleClock string

const (
	PerfSampleClockRecord           PerfSampleClock = "record"
	PerfSampleClockSimpleperfRecord PerfSampleClock = "simpleperf_record"
	PerfSampleClockMonotonicRaw     PerfSampleClock = "monotonic_raw"
	PerfSampleClockPerfData         PerfSampleClock = "perf_data"
	PerfSampleClockTraceStreamerDB  PerfSampleClock = "trace_streamer_db"
)

type PerfClockConfidence string

const (
	PerfClockConfidenceAssumed    PerfClockConfidence = "assumed"
	PerfClockConfidenceCalibrated PerfClockConfidence = "calibrated"
)

type PerfSampleKindSource string

const PerfSampleKindSourceSchedulerRunning PerfSampleKindSource = "scheduler_running"

type PerfIdentitySource string

const PerfIdentitySourceTraceThread PerfIdentitySource = "trace_thread"

// PerfRawSampleFields is the closed optional tail emitted by the raw perf.data
// fallback. Zero values are absent, matching the historical wire contract.
type PerfRawSampleFields struct {
	Addr          uint64
	SampleID      uint64
	StreamID      uint64
	PerfWeight    uint64
	DataSource    uint64
	Transaction   uint64
	PhysicalAddr  uint64
	CGroupID      uint64
	DataPageSize  uint64
	CodePageSize  uint64
	RawSize       uint64
	BranchCount   uint64
	UserRegsABI   uint64
	UserRegsCount int64
	UserStackSize uint64
	AuxSize       uint64
}

func (f PerfRawSampleFields) empty() bool {
	return f == (PerfRawSampleFields{})
}

// PerfSampleRow is the only writer input accepted by BuildPerfSampleBody.
// It deliberately exposes no arbitrary KV list or raw suffix: every field and
// its position is part of the converter/tracequery wire contract.
type PerfSampleRow struct {
	Layout              PerfSampleLayout
	CPU                 int64
	CPUKnown            bool
	PID                 int64
	TID                 int64
	ThreadComm          string
	SampleWeight        int64
	Event               string
	Symbol              string
	DSO                 string
	IP                  string
	Callchain           string
	Source              PerfSampleSource
	SampleKind          PerfSampleKind
	SymbolizationStatus PerfSymbolizationStatus
	Clock               PerfSampleClock
	ClockConfidence     PerfClockConfidence
	CallchainStatus     PerfCallchainStatus

	// SQL late fields. Source-only rows always publish all three source
	// identity fields, including a zero PID or an empty comm.
	PerfSourceTID    int64
	PerfSourcePID    int64
	PerfSourceComm   string
	SampleKindSource PerfSampleKindSource
	PerfThreadComm   string
	CommSource       PerfIdentitySource
	ProcessIDSource  PerfIdentitySource

	// Raw fallback late fields.
	Raw           PerfRawSampleFields
	ParserCaveats string
}

// BuildPerfSampleBody returns the complete event body beginning with
// "perf_sample:". The ftrace envelope remains owned by each adapter.
func BuildPerfSampleBody(row PerfSampleRow) (string, error) {
	if err := validatePerfSampleRow(row); err != nil {
		return "", err
	}
	b := newPerfSampleBodyBuilder()
	appendBare := func(key, value string) error { return b.appendField(key, value) }
	appendQuoted := func(key, value string, limit uint64) error {
		encoded, err := encodePerfMetadata(key, value, limit)
		if err != nil {
			return err
		}
		return b.appendField(key, encoded)
	}

	base := []struct {
		key   string
		value string
	}{
		{"cpu", strconv.FormatInt(row.CPU, 10)},
		{"cpu_known", strconv.FormatBool(row.CPUKnown)},
		{"pid", strconv.FormatInt(row.PID, 10)},
		{"tid", strconv.FormatInt(row.TID, 10)},
	}
	for _, field := range base {
		if err := appendBare(field.key, field.value); err != nil {
			return "", err
		}
	}
	if err := appendQuoted("thread_comm", row.ThreadComm, MaxPerfMetadataBytes); err != nil {
		return "", err
	}
	if err := appendBare("sample_weight", strconv.FormatInt(row.SampleWeight, 10)); err != nil {
		return "", err
	}
	for _, field := range []struct {
		key   string
		value string
	}{
		{"event", row.Event},
		{"symbol", row.Symbol},
		{"dso", row.DSO},
		{"ip", row.IP},
	} {
		if err := appendQuoted(field.key, field.value, MaxPerfMetadataBytes); err != nil {
			return "", err
		}
	}
	if err := appendQuoted("callchain", row.Callchain, MaxPerfCallchainBytes); err != nil {
		return "", err
	}
	if err := appendBare("source", string(row.Source)); err != nil {
		return "", err
	}
	if row.SampleKind != "" {
		if err := appendBare("sample_kind", string(row.SampleKind)); err != nil {
			return "", err
		}
	}
	for _, field := range []struct {
		key   string
		value string
	}{
		{"symbolization_status", string(row.SymbolizationStatus)},
		{"clock", string(row.Clock)},
		{"clock_confidence", string(row.ClockConfidence)},
		{"callchain_status", string(row.CallchainStatus)},
	} {
		if err := appendBare(field.key, field.value); err != nil {
			return "", err
		}
	}

	switch row.Layout {
	case PerfSampleLayoutBase:
	case PerfSampleLayoutRawExtended:
		if err := appendRawPerfFields(b, row.Raw); err != nil {
			return "", err
		}
		if row.ParserCaveats != "" {
			// Capture-level caveats are bounded disclosure, not a sample
			// identity. They have one shared writer/reader disclosure budget
			// without inheriting the 512-byte identity limit.
			if err := appendQuoted("parser_caveats", row.ParserCaveats, MaxPerfParserCaveatsBytes); err != nil {
				return "", err
			}
		}
	case PerfSampleLayoutResolvedIdentity:
		if err := appendBare("thread_identity_known", "true"); err != nil {
			return "", err
		}
		if err := appendBare("resolution", "resolved"); err != nil {
			return "", err
		}
		if err := appendBare("lifecycle_unverified", "false"); err != nil {
			return "", err
		}
		if row.SampleKindSource != "" {
			if err := appendBare("sample_kind_source", string(row.SampleKindSource)); err != nil {
				return "", err
			}
		}
		if row.PerfThreadComm != "" {
			if err := appendQuoted("perf_thread_comm", row.PerfThreadComm, MaxPerfMetadataBytes); err != nil {
				return "", err
			}
			if err := appendBare("comm_source", string(row.CommSource)); err != nil {
				return "", err
			}
		}
		if row.ProcessIDSource != "" {
			if err := appendBare("process_id_source", string(row.ProcessIDSource)); err != nil {
				return "", err
			}
		}
	case PerfSampleLayoutSourceOnlyIdentity:
		if err := appendBare("thread_identity_known", "false"); err != nil {
			return "", err
		}
		if err := appendBare("resolution", "perf_source_only"); err != nil {
			return "", err
		}
		if err := appendBare("lifecycle_unverified", "true"); err != nil {
			return "", err
		}
		if err := appendBare("perf_source_tid", strconv.FormatInt(row.PerfSourceTID, 10)); err != nil {
			return "", err
		}
		if err := appendBare("perf_source_pid", strconv.FormatInt(row.PerfSourcePID, 10)); err != nil {
			return "", err
		}
		if err := appendQuoted("perf_source_comm", row.PerfSourceComm, MaxPerfMetadataBytes); err != nil {
			return "", err
		}
		if row.SampleKindSource != "" {
			if err := appendBare("sample_kind_source", string(row.SampleKindSource)); err != nil {
				return "", err
			}
		}
	}
	return b.String(), nil
}

func validatePerfSampleRow(row PerfSampleRow) error {
	if row.SampleWeight <= 0 {
		return &PerfWireBuildError{Field: "sample_weight", Reason: "not_positive"}
	}
	if row.CPU < -1 || row.CPU > 4095 {
		return &PerfWireBuildError{Field: "cpu", Reason: "out_of_range", Limit: 4095}
	}
	if row.CPUKnown && row.CPU < 0 {
		return &PerfWireBuildError{Field: "cpu", Reason: "known_cpu_is_negative"}
	}
	if !row.CPUKnown && row.CPU != -1 {
		return &PerfWireBuildError{Field: "cpu", Reason: "unknown_cpu_not_minus_one"}
	}
	if !validPerfSampleKind(row.SampleKind, true) {
		return invalidPerfEnum("sample_kind")
	}
	if !validPerfSymbolizationStatus(row.SymbolizationStatus) {
		return invalidPerfEnum("symbolization_status")
	}
	if !validPerfCallchainStatus(row.CallchainStatus) {
		return invalidPerfEnum("callchain_status")
	}
	if !validPerfClockConfidence(row.ClockConfidence) {
		return invalidPerfEnum("clock_confidence")
	}
	if row.SampleKindSource != "" && row.SampleKindSource != PerfSampleKindSourceSchedulerRunning {
		return invalidPerfEnum("sample_kind_source")
	}
	if row.CommSource != "" && row.CommSource != PerfIdentitySourceTraceThread {
		return invalidPerfEnum("comm_source")
	}
	if row.ProcessIDSource != "" && row.ProcessIDSource != PerfIdentitySourceTraceThread {
		return invalidPerfEnum("process_id_source")
	}

	switch row.Source {
	case PerfSampleSourceSimpleperfReportSample:
		if row.Layout != PerfSampleLayoutBase || row.Clock != PerfSampleClockRecord ||
			row.ClockConfidence != PerfClockConfidenceAssumed || row.SampleKind == PerfSampleKindUnknown || !row.CPUKnown {
			return &PerfWireBuildError{Field: "layout", Reason: "profile_mismatch"}
		}
	case PerfSampleSourceSimpleperfReportProto:
		if row.Layout != PerfSampleLayoutBase || row.Clock != PerfSampleClockSimpleperfRecord ||
			row.ClockConfidence != PerfClockConfidenceAssumed || row.SampleKind == "" || row.CPUKnown {
			return &PerfWireBuildError{Field: "layout", Reason: "profile_mismatch"}
		}
	case PerfSampleSourceHiperfProto:
		if row.Layout != PerfSampleLayoutBase || row.Clock != PerfSampleClockMonotonicRaw ||
			row.ClockConfidence != PerfClockConfidenceAssumed || row.SampleKind != "" || row.CPUKnown {
			return &PerfWireBuildError{Field: "layout", Reason: "profile_mismatch"}
		}
	case PerfSampleSourceRawPerfDataFallback:
		if row.Layout != PerfSampleLayoutRawExtended || row.Clock != PerfSampleClockPerfData ||
			row.ClockConfidence != PerfClockConfidenceAssumed ||
			row.SampleKind != "" && row.SampleKind != PerfSampleKindOffCPU {
			return &PerfWireBuildError{Field: "layout", Reason: "profile_mismatch"}
		}
	case PerfSampleSourceTraceStreamerDB:
		if row.Layout != PerfSampleLayoutResolvedIdentity && row.Layout != PerfSampleLayoutSourceOnlyIdentity ||
			row.Clock != PerfSampleClockTraceStreamerDB || row.ClockConfidence != PerfClockConfidenceCalibrated ||
			row.SampleKind == "" || !row.CPUKnown {
			return &PerfWireBuildError{Field: "layout", Reason: "profile_mismatch"}
		}
	default:
		return invalidPerfEnum("source")
	}

	hasRaw := !row.Raw.empty() || row.ParserCaveats != ""
	hasSQL := row.PerfSourceTID != 0 || row.PerfSourcePID != 0 || row.PerfSourceComm != "" ||
		row.SampleKindSource != "" || row.PerfThreadComm != "" || row.CommSource != "" || row.ProcessIDSource != ""
	switch row.Layout {
	case PerfSampleLayoutBase:
		// Android simpleperf's report_sample.py prints the producer's uint32
		// SampleStruct pid/tid directly. Zero is therefore a present idle/pseudo
		// coordinate on this one profile, not a missing identity. Other base
		// profiles keep their existing positive public-thread contract.
		zeroAllowed := row.Source == PerfSampleSourceSimpleperfReportSample
		if err := validatePerfID("pid", row.PID, zeroAllowed); err != nil {
			return err
		}
		if err := validatePerfID("tid", row.TID, zeroAllowed); err != nil {
			return err
		}
		if hasRaw || hasSQL {
			return &PerfWireBuildError{Field: "layout", Reason: "late_field_conflict"}
		}
	case PerfSampleLayoutRawExtended:
		// Linux PERF_SAMPLE_TID uses unsigned pid/tid coordinates; zero is a
		// present producer value (for example the idle task), not absence.
		if err := validatePerfID("pid", row.PID, true); err != nil {
			return err
		}
		if err := validatePerfID("tid", row.TID, true); err != nil {
			return err
		}
		if hasSQL {
			return &PerfWireBuildError{Field: "layout", Reason: "late_field_conflict"}
		}
		if row.Raw.UserRegsCount < 0 {
			return &PerfWireBuildError{Field: "user_regs_count", Reason: "out_of_range"}
		}
		if err := validatePerfRawInt64Fields(row.Raw); err != nil {
			return err
		}
	case PerfSampleLayoutResolvedIdentity:
		if err := validatePerfID("pid", row.PID, false); err != nil {
			return err
		}
		if err := validatePerfID("tid", row.TID, false); err != nil {
			return err
		}
		if hasRaw || row.PerfSourceTID != 0 || row.PerfSourcePID != 0 || row.PerfSourceComm != "" {
			return &PerfWireBuildError{Field: "layout", Reason: "late_field_conflict"}
		}
		if (row.PerfThreadComm == "") != (row.CommSource == "") {
			return &PerfWireBuildError{Field: "comm_source", Reason: "incomplete_note"}
		}
		if row.SampleKindSource != "" && row.SampleKind != PerfSampleKindOnCPU {
			return &PerfWireBuildError{Field: "sample_kind_source", Reason: "provenance_mismatch"}
		}
	case PerfSampleLayoutSourceOnlyIdentity:
		if row.PID != 0 || row.TID != 0 || strings.TrimSpace(row.ThreadComm) != "" {
			return &PerfWireBuildError{Field: "thread_identity", Reason: "source_only_common_identity_present"}
		}
		if hasRaw || row.PerfThreadComm != "" || row.CommSource != "" || row.ProcessIDSource != "" || row.SampleKindSource != "" {
			return &PerfWireBuildError{Field: "layout", Reason: "late_field_conflict"}
		}
		if err := validatePerfID("perf_source_tid", row.PerfSourceTID, false); err != nil {
			return err
		}
		if err := validatePerfID("perf_source_pid", row.PerfSourcePID, true); err != nil {
			return err
		}
	default:
		return invalidPerfEnum("layout")
	}
	return nil
}

func validatePerfRawInt64Fields(raw PerfRawSampleFields) error {
	fields := []struct {
		name  string
		value uint64
	}{
		{"perf_weight", raw.PerfWeight},
		{"data_page_size", raw.DataPageSize},
		{"code_page_size", raw.CodePageSize},
		{"raw_size", raw.RawSize},
		{"branch_count", raw.BranchCount},
		{"user_stack_size", raw.UserStackSize},
		{"aux_size", raw.AuxSize},
	}
	for _, field := range fields {
		if field.value > math.MaxInt64 {
			return &PerfWireBuildError{
				Field: field.name, Reason: "out_of_range", Limit: math.MaxInt64, Actual: field.value,
			}
		}
	}
	return nil
}

func validatePerfID(field string, value int64, zeroAllowed bool) error {
	if value < 0 || value == 0 && !zeroAllowed || value > math.MaxInt32 {
		actual := uint64(0)
		if value > 0 {
			actual = uint64(value)
		}
		return &PerfWireBuildError{Field: field, Reason: "out_of_range", Limit: math.MaxInt32, Actual: actual}
	}
	return nil
}

func invalidPerfEnum(field string) error {
	return &PerfWireBuildError{Field: field, Reason: "invalid_enum"}
}

func validPerfSampleKind(value PerfSampleKind, optional bool) bool {
	return optional && value == "" || value == PerfSampleKindOnCPU || value == PerfSampleKindOffCPU || value == PerfSampleKindUnknown
}

func validPerfSymbolizationStatus(value PerfSymbolizationStatus) bool {
	switch value {
	case PerfSymbolizationSymbolized, PerfSymbolizationUnsymbolized, PerfSymbolizationPartial, PerfSymbolizationUnknown:
		return true
	default:
		return false
	}
}

func validPerfCallchainStatus(value PerfCallchainStatus) bool {
	switch value {
	case PerfCallchainStatusSymbolized, PerfCallchainStatusPartial, PerfCallchainStatusIPOnly, PerfCallchainStatusMissing:
		return true
	default:
		return false
	}
}

func validPerfClockConfidence(value PerfClockConfidence) bool {
	return value == PerfClockConfidenceAssumed || value == PerfClockConfidenceCalibrated
}

func encodePerfMetadata(field, raw string, limit uint64) (string, error) {
	if !utf8.ValidString(raw) {
		return "", &PerfWireBuildError{Field: field, Reason: "invalid_utf8", Actual: uint64(len(raw))}
	}
	normalized := strings.TrimSpace(raw)
	if uint64(len(normalized)) > limit {
		return "", &PerfWireBuildError{
			Field: field, Reason: "decoded_value_too_long", Limit: limit, Actual: uint64(len(normalized)),
		}
	}
	encoded := QuotePerfKVValue(normalized)
	if len(encoded) > MaxPerfKVEncodedValueBytes {
		return "", &PerfWireBuildError{
			Field: field, Reason: "encoded_value_too_long", Limit: MaxPerfKVEncodedValueBytes, Actual: uint64(len(encoded)),
		}
	}
	return encoded, nil
}

func appendRawPerfFields(b *perfSampleBodyBuilder, raw PerfRawSampleFields) error {
	fields := []struct {
		key   string
		value uint64
		hex   bool
	}{
		{"addr", raw.Addr, true},
		{"sample_id", raw.SampleID, false},
		{"stream_id", raw.StreamID, false},
		{"perf_weight", raw.PerfWeight, false},
		{"data_src", raw.DataSource, true},
		{"transaction", raw.Transaction, true},
		{"phys_addr", raw.PhysicalAddr, true},
		{"cgroup_id", raw.CGroupID, false},
		{"data_page_size", raw.DataPageSize, false},
		{"code_page_size", raw.CodePageSize, false},
		{"raw_size", raw.RawSize, false},
		{"branch_count", raw.BranchCount, false},
		{"user_regs_abi", raw.UserRegsABI, false},
	}
	for _, field := range fields {
		if field.value == 0 {
			continue
		}
		value := strconv.FormatUint(field.value, 10)
		if field.hex {
			value = "0x" + strconv.FormatUint(field.value, 16)
		}
		if err := b.appendField(field.key, value); err != nil {
			return err
		}
	}
	if raw.UserRegsCount > 0 {
		if err := b.appendField("user_regs_count", strconv.FormatInt(raw.UserRegsCount, 10)); err != nil {
			return err
		}
	}
	for _, field := range []struct {
		key   string
		value uint64
	}{
		{"user_stack_size", raw.UserStackSize},
		{"aux_size", raw.AuxSize},
	} {
		if field.value != 0 {
			if err := b.appendField(field.key, strconv.FormatUint(field.value, 10)); err != nil {
				return err
			}
		}
	}
	return nil
}

type perfSampleBodyBuilder struct {
	body    strings.Builder
	fields  int
	kvBytes int
}

func newPerfSampleBodyBuilder() *perfSampleBodyBuilder {
	b := &perfSampleBodyBuilder{}
	b.body.WriteString("perf_sample:")
	return b
}

func (b *perfSampleBodyBuilder) appendField(key, encodedValue string) error {
	if len(key) > MaxPerfKVKeyBytes {
		return &PerfWireBuildError{Field: "key", Reason: "key_too_long", Limit: MaxPerfKVKeyBytes, Actual: uint64(len(key))}
	}
	if !validPerfKey(key) {
		return &PerfWireBuildError{Field: "key", Reason: "invalid_key"}
	}
	if b.fields >= MaxPerfKVFields {
		return &PerfWireBuildError{Field: "fields", Reason: "field_count_exceeded", Limit: MaxPerfKVFields, Actual: uint64(b.fields + 1)}
	}
	if !utf8.ValidString(encodedValue) {
		return &PerfWireBuildError{Field: key, Reason: "invalid_utf8", Actual: uint64(len(encodedValue))}
	}
	if encodedValue == "" {
		return &PerfWireBuildError{Field: key, Reason: "missing_value"}
	}
	if len(encodedValue) > MaxPerfKVEncodedValueBytes {
		return &PerfWireBuildError{Field: key, Reason: "encoded_value_too_long", Limit: MaxPerfKVEncodedValueBytes, Actual: uint64(len(encodedValue))}
	}
	extra := 1 + len(key) + 1 + len(encodedValue)
	if b.kvBytes+extra > MaxPerfKVBodyBytes {
		return &PerfWireBuildError{Field: "body", Reason: "body_too_long", Limit: MaxPerfKVBodyBytes, Actual: uint64(b.kvBytes + extra)}
	}
	b.body.WriteByte(' ')
	b.body.WriteString(key)
	b.body.WriteByte('=')
	b.body.WriteString(encodedValue)
	b.fields++
	b.kvBytes += extra
	return nil
}

func (b *perfSampleBodyBuilder) String() string { return b.body.String() }

func validPerfKey(key string) bool {
	if key == "" || !isKeyStart(key[0]) {
		return false
	}
	for pos := 1; pos < len(key); pos++ {
		if !isKeyContinue(key[pos]) {
			return false
		}
	}
	return true
}
