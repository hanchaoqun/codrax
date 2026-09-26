package tracewire

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

const HiSysEventObservationPrefix = "# codrax_hisysevent/v1"
const MaxHiSysEventObservationBytes = 1 << 20

// HiSysEvent is an observation of one SQL row, not a scheduler emitter, span,
// or causal relationship. Its timestamp is the row's trace timestamp, never
// the timestamp of a SQL-fidelity carrier. Zero and NULL remain distinct.
type HiSysEvent struct {
	TimestampNS int64              `json:"timestamp_ns,string"`
	SourceTID   *int64             `json:"source_tid,string"`
	Domain      HiSysEventName     `json:"domain"`
	Event       HiSysEventName     `json:"event"`
	Contents    HiSysEventContents `json:"contents"`
}

type HiSysEventName struct {
	Name      *string `json:"name"`
	Status    string  `json:"status"`
	Reference *int64  `json:"reference,string"`
}

// Text is the exact TEXT value, or the canonical decimal value for INTEGER /
// REAL. BytesBase64 preserves BLOB bytes without pretending they are UTF-8.
type HiSysEventContents struct {
	StorageClass string  `json:"storage_class"`
	Text         *string `json:"text,omitempty"`
	BytesBase64  string  `json:"bytes_base64,omitempty"`
}

func validHiSysName(n HiSysEventName) bool {
	if n.Name != nil && (!utf8.ValidString(*n.Name) || len(*n.Name) > MaxHiSysEventObservationBytes) {
		return false
	}
	switch n.Status {
	case "resolved":
		return n.Name != nil && n.Reference != nil
	case "unresolved_reference":
		return n.Name == nil && n.Reference != nil
	case "null_reference", "invalid_reference_storage_class":
		return n.Name == nil && n.Reference == nil
	}
	return false
}

func validHiSysContents(c HiSysEventContents) bool {
	if c.Text != nil && (!utf8.ValidString(*c.Text) || len(*c.Text) > MaxHiSysEventObservationBytes) {
		return false
	}
	if len(c.BytesBase64) > MaxHiSysEventObservationBytes {
		return false
	}
	switch c.StorageClass {
	case "null":
		return c.Text == nil && c.BytesBase64 == ""
	case "text":
		return c.Text != nil && c.BytesBase64 == ""
	case "integer":
		if c.Text == nil || c.BytesBase64 != "" {
			return false
		}
		v, e := strconv.ParseInt(*c.Text, 10, 64)
		return e == nil && strconv.FormatInt(v, 10) == *c.Text
	case "real":
		if c.Text == nil || c.BytesBase64 != "" {
			return false
		}
		v, e := strconv.ParseFloat(*c.Text, 64)
		return e == nil && !math.IsInf(v, 0) && !math.IsNaN(v) && strconv.FormatFloat(v, 'g', -1, 64) == *c.Text
	case "blob":
		if c.Text != nil {
			return false
		}
		b, e := base64.StdEncoding.Strict().DecodeString(c.BytesBase64)
		return e == nil && base64.StdEncoding.EncodeToString(b) == c.BytesBase64
	}
	return false
}

func FormatHiSysEventObservation(e HiSysEvent) (string, error) {
	if e.TimestampNS < 0 || (e.SourceTID != nil && (*e.SourceTID < 0 || *e.SourceTID > math.MaxInt32)) || !validHiSysName(e.Domain) || !validHiSysName(e.Event) || !validHiSysContents(e.Contents) {
		return "", fmt.Errorf("invalid HiSys event observation")
	}
	rawBytes := len(e.Contents.BytesBase64)
	for _, value := range []*string{e.Domain.Name, e.Event.Name, e.Contents.Text} {
		if value != nil {
			rawBytes += len(*value)
		}
	}
	if rawBytes > MaxHiSysEventObservationBytes {
		return "", fmt.Errorf("HiSys event fields exceed line budget")
	}
	b, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	// Guard before the base64 allocation as well as the final physical line.
	if len(b) > MaxHiSysEventObservationBytes*3/4 {
		return "", fmt.Errorf("HiSys event observation exceeds line budget")
	}
	line := HiSysEventObservationPrefix + " ts_ns=" + strconv.FormatInt(e.TimestampNS, 10) + " payload=" + base64.RawURLEncoding.EncodeToString(b)
	if len(line) > MaxHiSysEventObservationBytes {
		return "", fmt.Errorf("HiSys event observation exceeds line budget")
	}
	return line, nil
}

func ParseHiSysEventObservation(line string) (HiSysEvent, bool) {
	if !strings.HasPrefix(line, HiSysEventObservationPrefix+" ts_ns=") || len(line) > MaxHiSysEventObservationBytes {
		return HiSysEvent{}, false
	}
	ts, payload, ok := strings.Cut(strings.TrimPrefix(line, HiSysEventObservationPrefix+" ts_ns="), " payload=")
	if !ok || payload == "" {
		return HiSysEvent{}, false
	}
	ns, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || ns < 0 || strconv.FormatInt(ns, 10) != ts {
		return HiSysEvent{}, false
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(payload)
	if err != nil || !utf8.Valid(b) {
		return HiSysEvent{}, false
	}
	// Allocate decoder state only for this format, never on the hot path
	// used to reject each ordinary ftrace row.
	var e HiSysEvent
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(&e) != nil || e.TimestampNS != ns {
		return HiSysEvent{}, false
	}
	// Canonical re-encoding also rejects duplicate fields, trailing JSON,
	// numeric aliases and lossy unpaired-surrogate replacement by encoding/json.
	canonical, err := FormatHiSysEventObservation(e)
	return e, err == nil && canonical == line
}
