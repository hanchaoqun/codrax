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

// MeasureScalar preserves SQLite storage, not an inferred physical quantity.
// Decimal strings avoid lossy JSON-number conversion; blobs use raw base64.
type MeasureScalar struct {
	StorageClass string `json:"storage_class"`
	Value        string `json:"value"`
	Encoding     string `json:"encoding,omitempty"`
}

func (s MeasureScalar) Integer() (int64, bool) {
	if s.StorageClass != "integer" || s.Encoding != "" {
		return 0, false
	}
	v, err := strconv.ParseInt(s.Value, 10, 64)
	return v, err == nil && strconv.FormatInt(v, 10) == s.Value
}

func (s MeasureScalar) Valid() bool {
	if s.Encoding != "" {
		if s.StorageClass != "text" || s.Encoding != "base64" {
			return false
		}
		v, err := base64.RawStdEncoding.DecodeString(s.Value)
		return err == nil && !utf8.Valid(v) && base64.RawStdEncoding.EncodeToString(v) == s.Value
	}
	switch s.StorageClass {
	case "integer":
		_, ok := s.Integer()
		return ok
	case "real":
		v, err := strconv.ParseFloat(s.Value, 64)
		return err == nil && !math.IsNaN(v) && strconv.FormatFloat(v, 'g', -1, 64) == s.Value
	case "text":
		return utf8.ValidString(s.Value)
	case "blob":
		v, err := base64.RawStdEncoding.DecodeString(s.Value)
		return err == nil && base64.RawStdEncoding.EncodeToString(v) == s.Value
	case "null", "absent":
		return s.Value == ""
	}
	return false
}

type MeasureFilter struct {
	ID             MeasureScalar `json:"id"`
	Name           MeasureScalar `json:"name"`
	Type           MeasureScalar `json:"type"`
	SourceArgSetID MeasureScalar `json:"source_arg_set_id"`
}

// Observed_unique proves only one strictly identified filter registry record.
// In particular source_arg_set_id is NOT a hardware, process or thread ID.
type MeasureInterval struct {
	RowID        int64          `json:"row_id,string"`
	StartNS      MeasureScalar  `json:"start_ns"`
	DurationNS   MeasureScalar  `json:"duration_ns"`
	Value        MeasureScalar  `json:"value"`
	FilterID     MeasureScalar  `json:"filter_id"`
	MeasureType  MeasureScalar  `json:"type"`
	FilterStatus string         `json:"filter_status"`
	Filter       *MeasureFilter `json:"filter,omitempty"`
}

func (r MeasureInterval) TimestampNS() int64 {
	v, ok := r.StartNS.Integer()
	if !ok || v < 0 {
		return 0
	}
	return v
}
func (r MeasureInterval) EndNS() (int64, bool) {
	ts, sok := r.StartNS.Integer()
	dur, dok := r.DurationNS.Integer()
	if !sok || !dok || dur < 0 || ts > math.MaxInt64-dur {
		return 0, false
	}
	return ts + dur, true
}
func (r MeasureInterval) Valid() bool {
	for _, s := range []MeasureScalar{r.StartNS, r.DurationNS, r.Value, r.FilterID, r.MeasureType} {
		if !s.Valid() {
			return false
		}
	}
	if r.FilterStatus == "unknown" || r.FilterStatus == "ambiguous" {
		return r.Filter == nil
	}
	if r.FilterStatus != "observed_unique" || r.Filter == nil {
		return false
	}
	id, ok := r.FilterID.Integer()
	fid, fok := r.Filter.ID.Integer()
	return ok && fok && id == fid && r.Filter.Name.Valid() && r.Filter.Type.Valid() && r.Filter.SourceArgSetID.Valid()
}

const MeasureIntervalPrefix = "# codrax_measure_interval/v1 record="

func FormatMeasureInterval(r MeasureInterval) (string, error) {
	if !r.Valid() {
		return "", fmt.Errorf("invalid raw measure record")
	}
	b, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	if len(b) > 1<<20 {
		return "", fmt.Errorf("raw measure record exceeds carrier limit")
	}
	return MeasureIntervalPrefix + base64.RawURLEncoding.EncodeToString(b), nil
}
func ParseMeasureInterval(line string) (MeasureInterval, bool) {
	raw, ok := strings.CutPrefix(line, MeasureIntervalPrefix)
	if !ok || len(raw) > (1<<20)*4/3+4 {
		return MeasureInterval{}, false
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return MeasureInterval{}, false
	}
	var r MeasureInterval
	if json.Unmarshal(b, &r) != nil || !r.Valid() {
		return MeasureInterval{}, false
	}
	canonical, err := FormatMeasureInterval(r)
	return r, err == nil && canonical == line
}
