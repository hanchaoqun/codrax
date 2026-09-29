package tracewire

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"unicode/utf8"
)

// MarkerNameOrigin preserves the business name separately from a synthesized
// trace-marker label. This is display provenance, never interval/CPU authority.
type MarkerNameOrigin struct {
	SourceTable string              `json:"source_table"`
	Name        HiSysEventName      `json:"name"`
	Record      *MarkerSourceRecord `json:"record,omitempty"`
}

// MarkerSourceRecord identifies one source-table interval, not a complete
// launch instance. RowID and OwnerIPID are local to this source capture; neither
// is a public PID, lifecycle generation or cross-capture join key.
type MarkerSourceRecord struct {
	RowID     int64 `json:"row_id,string"`
	OwnerIPID int64 `json:"owner_ipid,string"`
	// Empty means a known internal process reference (including 0). When the
	// source has NULL or an invalid reference, OwnerIPID is unused, not PID 0.
	OwnerIssue string `json:"owner_issue,omitempty"`
	OwnerPID   *int64 `json:"owner_pid,string,omitempty"`
	StartNS    int64  `json:"start_ns,string"`
	EndNS      int64  `json:"end_ns,string"`
}

const maxMarkerNameOriginLegacyBytes = 8192

// Reserve record identity plus nullable-owner status separately; additional
// source metadata must not consume the legacy name budget.
const MaxMarkerNameOriginBytes = maxMarkerNameOriginLegacyBytes + 384

func EncodeMarkerNameOrigin(origin MarkerNameOrigin) (string, error) {
	if origin.SourceTable != "app_startup" || !validHiSysName(origin.Name) ||
		(origin.Name.Name != nil && len(*origin.Name.Name) > 4096) {
		return "", fmt.Errorf("invalid marker name origin")
	}
	if r := origin.Record; r != nil && (r.OwnerIPID < 0 || r.StartNS < 0 || r.EndNS <= r.StartNS) {
		return "", fmt.Errorf("invalid marker source record")
	}
	if r := origin.Record; r != nil && r.OwnerIssue != "" &&
		(r.OwnerIPID != 0 || r.OwnerIssue != "null_reference" && r.OwnerIssue != "invalid_reference") {
		return "", fmt.Errorf("invalid marker owner status")
	}
	nameOnly := origin
	if r := origin.Record; r != nil && r.OwnerPID != nil && (r.OwnerIssue != "" || *r.OwnerPID <= 0 || *r.OwnerPID > 2147483647) {
		return "", fmt.Errorf("invalid source process PID")
	}
	nameOnly.Record = nil
	nameBytes, err := json.Marshal(nameOnly)
	if err != nil || len(nameBytes) > maxMarkerNameOriginLegacyBytes*3/4 {
		return "", fmt.Errorf("marker name origin exceeds name budget")
	}
	b, err := json.Marshal(origin)
	if err != nil || len(b) > MaxMarkerNameOriginBytes*3/4 {
		return "", fmt.Errorf("marker name origin exceeds budget")
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func DecodeMarkerNameOrigin(payload string) (MarkerNameOrigin, bool) {
	if payload == "" || len(payload) > MaxMarkerNameOriginBytes {
		return MarkerNameOrigin{}, false
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(payload)
	if err != nil || !utf8.Valid(b) {
		return MarkerNameOrigin{}, false
	}
	var origin MarkerNameOrigin
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(&origin) != nil {
		return MarkerNameOrigin{}, false
	}
	canonical, err := EncodeMarkerNameOrigin(origin)
	return origin, err == nil && canonical == payload
}
