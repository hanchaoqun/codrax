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
	SourceTable string         `json:"source_table"`
	Name        HiSysEventName `json:"name"`
}

const MaxMarkerNameOriginBytes = 8192

func EncodeMarkerNameOrigin(origin MarkerNameOrigin) (string, error) {
	if origin.SourceTable != "app_startup" || !validHiSysName(origin.Name) ||
		(origin.Name.Name != nil && len(*origin.Name.Name) > 4096) {
		return "", fmt.Errorf("invalid marker name origin")
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
