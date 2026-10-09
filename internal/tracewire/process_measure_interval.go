package tracewire

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

const ProcessMeasureIntervalPrefix = "# codrax_process_measure_interval/v1"

// ProcessMeasureScalar preserves SQLite storage class. Only INTEGER is a
// known integer; an integral REAL or numeric TEXT does not inherit authority.
type ProcessMeasureScalar struct {
	Status       string `json:"status"`
	StorageClass string `json:"storage_class"`
	Value        string `json:"value,omitempty"`
}

func (s ProcessMeasureScalar) Integer() (int64, bool) {
	if s.Status != "known" || s.StorageClass != "integer" {
		return 0, false
	}
	v, err := strconv.ParseInt(s.Value, 10, 64)
	return v, err == nil && strconv.FormatInt(v, 10) == s.Value
}

func (s ProcessMeasureScalar) Valid() bool {
	switch s.Status {
	case "known":
		_, ok := s.Integer()
		return ok
	case "null":
		return s.StorageClass == "null" && s.Value == ""
	case "unavailable":
		return s.StorageClass == "absent" && s.Value == ""
	case "invalid_storage":
		switch s.StorageClass {
		case "text":
			return utf8.ValidString(s.Value)
		case "blob":
			_, err := base64.RawStdEncoding.DecodeString(s.Value)
			return err == nil
		case "real":
			_, err := strconv.ParseFloat(s.Value, 64)
			return err == nil
		}
	}
	return false
}

// This record has process ownership, not an emitting thread or CPU. Interval
// endpoints and values remain exact strings even above JSON's safe integer.
type ProcessMeasureInterval struct {
	RowID       int64                `json:"row_id,string"`
	FilterID    ProcessMeasureScalar `json:"filter_id"`
	StartNS     ProcessMeasureScalar `json:"start_ns"`
	DurationNS  ProcessMeasureScalar `json:"duration_ns"`
	Value       ProcessMeasureScalar `json:"value"`
	IPID        ProcessMeasureScalar `json:"ipid"`
	Name        string               `json:"name,omitempty"`
	NameKnown   bool                 `json:"name_known"`
	MeasureType string               `json:"measure_type,omitempty"`
	TypeKnown   bool                 `json:"type_known"`
	PID         *int                 `json:"pid,omitempty"`
	ProcessName string               `json:"process_name,omitempty"`
	OwnerStatus string               `json:"owner_status"`
}

func (r ProcessMeasureInterval) TimestampNS() int64 {
	v, ok := r.StartNS.Integer()
	if !ok || v < 0 {
		return 0
	}
	return v
}

func (r ProcessMeasureInterval) EndNS() (int64, bool) {
	s, sok := r.StartNS.Integer()
	d, dok := r.DurationNS.Integer()
	if !sok || !dok || d < 0 || s > math.MaxInt64-d {
		return 0, false
	}
	return s + d, true
}

func (r ProcessMeasureInterval) Valid() bool {
	for _, s := range []ProcessMeasureScalar{r.FilterID, r.StartNS, r.DurationNS, r.Value, r.IPID} {
		if !s.Valid() {
			return false
		}
	}
	if !r.NameKnown && r.Name != "" || !r.TypeKnown && r.MeasureType != "" {
		return false
	}
	if !utf8.ValidString(r.Name) || !utf8.ValidString(r.MeasureType) || !utf8.ValidString(r.ProcessName) {
		return false
	}
	switch r.OwnerStatus {
	case "known":
		id, ok := r.IPID.Integer()
		filter, filterOK := r.FilterID.Integer()
		return ok && id >= 0 && id < math.MaxUint32 && filterOK && filter >= 0 && r.PID != nil && *r.PID > 0 && int64(*r.PID) <= math.MaxInt32
	case "unknown", "ambiguous", "outside_lifetime":
		return r.PID == nil && r.ProcessName == ""
	}
	return false
}

func FormatProcessMeasureInterval(r ProcessMeasureInterval) (string, error) {
	if !r.Valid() {
		return "", fmt.Errorf("invalid process measure interval")
	}
	b, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	line := ProcessMeasureIntervalPrefix + " record=" + base64.RawURLEncoding.EncodeToString(b)
	if len(line) > 1<<20 {
		return "", fmt.Errorf("process measure interval exceeds carrier byte limit")
	}
	return line, nil
}

func ParseProcessMeasureInterval(line string) (ProcessMeasureInterval, bool) {
	const prefix = ProcessMeasureIntervalPrefix + " record="
	if !strings.HasPrefix(line, prefix) || len(line) > 1<<20 {
		return ProcessMeasureInterval{}, false
	}
	var r ProcessMeasureInterval
	b, err := base64.RawURLEncoding.Strict().DecodeString(strings.TrimPrefix(line, prefix))
	if err != nil || json.Unmarshal(b, &r) != nil || !r.Valid() {
		return ProcessMeasureInterval{}, false
	}
	canonical, err := FormatProcessMeasureInterval(r)
	return r, err == nil && canonical == line
}
