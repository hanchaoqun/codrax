package tracequery

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

const officialEBPFIntervalPrefix = "# codrax_ebpf_interval/v1"

const (
	EBPFFamilyFilesystem  = "filesystem"
	EBPFFamilyPagedMemory = "paged_memory"
	EBPFFamilyBIOLatency  = "bio_latency"
)

type OfficialEBPFInterval struct {
	Family            string `json:"family"`
	TimestampNS       uint64 `json:"timestamp_ns"`
	EndTimestampNS    uint64 `json:"end_timestamp_ns"`
	DurationNS        uint64 `json:"duration_ns"`
	SourceRow         uint32 `json:"source_row"`
	TypeID            uint64 `json:"type_id"`
	InternalProcessID uint32 `json:"internal_process_id"`
	InternalThreadID  uint32 `json:"internal_thread_id"`
	PID               int    `json:"pid"`
	TID               int    `json:"tid"`
	IdentityStatus    string `json:"identity_status"`
	CallchainID       int64  `json:"callchain_id"`
	CallchainStatus   string `json:"callchain_status"`
	DetailsJSON       string `json:"details_json"`
}

type OfficialEBPFFilesystemDetails struct {
	ReturnValue       string    `json:"return_value"`
	ReturnValueKnown  bool      `json:"return_value_known"`
	ErrorCode         string    `json:"error_code"`
	ErrorCodeKnown    bool      `json:"error_code_known"`
	FD                int64     `json:"fd"`
	FDKnown           bool      `json:"fd_known"`
	FileID            uint64    `json:"file_id"`
	FileIDKnown       bool      `json:"file_id_known"`
	SizeBytes         uint64    `json:"size_bytes"`
	SizeKnown         bool      `json:"size_known"`
	Arguments         [4]string `json:"arguments"`
	ArgumentKnownMask uint8     `json:"argument_known_mask"`
}

type OfficialEBPFPagedMemoryDetails struct {
	SizeBytes    uint64 `json:"size_bytes"`
	SizeKnown    bool   `json:"size_known"`
	Address      string `json:"address"`
	AddressKnown bool   `json:"address_known"`
}

type OfficialEBPFBIOLatencyDetails struct {
	Tier            uint32 `json:"tier"`
	TierKnown       bool   `json:"tier_known"`
	SizeBytes       uint64 `json:"size_bytes"`
	SizeKnown       bool   `json:"size_known"`
	BlockNumber     string `json:"block_number"`
	BlockKnown      bool   `json:"block_number_known"`
	PathID          uint64 `json:"path_id"`
	PathIDKnown     bool   `json:"path_id_known"`
	DurationPer4K   uint64 `json:"duration_per_4k_ns"`
	Duration4KKnown bool   `json:"duration_per_4k_known"`
}

func FormatOfficialEBPFInterval(interval OfficialEBPFInterval) (string, error) {
	if err := validateOfficialEBPFInterval(interval); err != nil {
		return "", err
	}
	payload, err := json.Marshal(interval)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		officialEBPFIntervalPrefix,
		"ts_ns=" + strconv.FormatUint(interval.TimestampNS, 10),
		"payload_b64=" + base64.RawURLEncoding.EncodeToString(payload),
	}, " "), nil
}

func parseOfficialEBPFInterval(line string) (OfficialEBPFInterval, bool) {
	if !strings.HasPrefix(line, officialEBPFIntervalPrefix+" ") {
		return OfficialEBPFInterval{}, false
	}
	parts := strings.Split(line, " ")
	if len(parts) != 4 || parts[0] != "#" || parts[1] != "codrax_ebpf_interval/v1" {
		return OfficialEBPFInterval{}, false
	}
	ts, ok := relationUint(parts[2], "ts_ns=", 64)
	if !ok || !strings.HasPrefix(parts[3], "payload_b64=") {
		return OfficialEBPFInterval{}, false
	}
	raw := strings.TrimPrefix(parts[3], "payload_b64=")
	if raw == "" {
		return OfficialEBPFInterval{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return OfficialEBPFInterval{}, false
	}
	var interval OfficialEBPFInterval
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&interval); err != nil {
		return OfficialEBPFInterval{}, false
	}
	if err := requireJSONEOF(decoder); err != nil || interval.TimestampNS != ts {
		return OfficialEBPFInterval{}, false
	}
	roundTrip, err := FormatOfficialEBPFInterval(interval)
	return interval, err == nil && roundTrip == line
}

func validateOfficialEBPFInterval(interval OfficialEBPFInterval) error {
	if interval.Family != EBPFFamilyFilesystem &&
		interval.Family != EBPFFamilyPagedMemory &&
		interval.Family != EBPFFamilyBIOLatency {
		return fmt.Errorf("invalid eBPF family")
	}
	if interval.EndTimestampNS < interval.TimestampNS ||
		interval.EndTimestampNS-interval.TimestampNS != interval.DurationNS {
		return fmt.Errorf("invalid eBPF interval")
	}
	switch interval.IdentityStatus {
	case "resolved":
		if interval.PID < 0 || interval.PID > math.MaxInt32 ||
			interval.TID <= 0 || interval.TID > math.MaxInt32 {
			return fmt.Errorf("invalid resolved eBPF identity")
		}
	case "unavailable", "mismatch", "lifecycle_rejected":
		if interval.PID != 0 || interval.TID != 0 {
			return fmt.Errorf("unproven eBPF identity carried public IDs")
		}
	default:
		return fmt.Errorf("invalid eBPF identity status")
	}
	switch interval.CallchainStatus {
	case "available", "unavailable":
		if interval.CallchainID < 0 || interval.CallchainID > math.MaxUint32 {
			return fmt.Errorf("invalid eBPF callchain")
		}
	case "absent":
		if interval.CallchainID != -1 {
			return fmt.Errorf("invalid absent eBPF callchain")
		}
	default:
		return fmt.Errorf("invalid eBPF callchain status")
	}
	if interval.DetailsJSON == "" {
		return fmt.Errorf("missing eBPF details")
	}
	switch interval.Family {
	case EBPFFamilyFilesystem:
		var details OfficialEBPFFilesystemDetails
		if !decodeCanonicalDetails(interval.DetailsJSON, &details) {
			return fmt.Errorf("invalid filesystem details")
		}
	case EBPFFamilyPagedMemory:
		var details OfficialEBPFPagedMemoryDetails
		if !decodeCanonicalDetails(interval.DetailsJSON, &details) {
			return fmt.Errorf("invalid paged-memory details")
		}
	case EBPFFamilyBIOLatency:
		var details OfficialEBPFBIOLatencyDetails
		if !decodeCanonicalDetails(interval.DetailsJSON, &details) {
			return fmt.Errorf("invalid bio-latency details")
		}
	}
	return nil
}

func decodeCanonicalDetails(raw string, destination any) bool {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return false
	}
	if err := requireJSONEOF(decoder); err != nil {
		return false
	}
	canonical, err := json.Marshal(destination)
	return err == nil && string(canonical) == raw
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON value")
		}
		return err
	}
	return nil
}

func officialEBPFIntervalEvent(lineNo int, interval OfficialEBPFInterval, intern *stringInterner) Event {
	event := Event{
		Line: lineNo, Ts: float64(interval.TimestampNS) / 1e9, CPU: -1,
		Type: EventEBPFInterval, Name: intern.intern("codrax_ebpf_" + interval.Family),
		PluginFields: &PluginFields{EBPFInterval: &EBPFIntervalFields{
			Family: interval.Family, TimestampNS: interval.TimestampNS,
			EndTimestampNS: interval.EndTimestampNS, DurationNS: interval.DurationNS,
			SourceRow: interval.SourceRow, TypeID: interval.TypeID,
			InternalProcessID: interval.InternalProcessID,
			InternalThreadID:  interval.InternalThreadID,
			PID:               interval.PID, TID: interval.TID, IdentityStatus: interval.IdentityStatus,
			CallchainID: interval.CallchainID, CallchainStatus: interval.CallchainStatus,
			DetailsJSON: intern.intern(interval.DetailsJSON),
		}},
		FieldText: intern.intern(fmt.Sprintf(
			"family=%s source_row=%d type_id=%d duration_ns=%d identity_status=%s callchain_id=%d callchain_status=%s details_json=%s",
			interval.Family, interval.SourceRow, interval.TypeID, interval.DurationNS,
			interval.IdentityStatus, interval.CallchainID, interval.CallchainStatus, interval.DetailsJSON,
		)),
	}
	if interval.IdentityStatus == "resolved" {
		event.PID = interval.TID
		event.TGID = interval.PID
	}
	return event
}
