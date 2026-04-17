package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// FlexInt is an int that accepts both JSON numbers (42) and numeric
// strings ("42"). LLMs routinely stringify integer fields when
// generating tool-call JSON — especially line_start, line_end, and
// citation_ref — because their training data is mixed. A strict
// json.Unmarshal rejects those and surfaces as "json: cannot
// unmarshal string into Go struct field", which then forces a retry
// just to re-type the field. FlexInt absorbs the impedance mismatch
// at the schema boundary so the retry is spent on real correctness
// issues, not format pedantry.
//
// Accepted inputs:
//   42        → 42
//   "42"      → 42
//   "   42  " → 42 (leading/trailing whitespace trimmed)
//   ""        → 0  (empty string = unset)
//   null      → 0  (JSON null = unset)
//   "abc"     → unmarshal error with a readable message
//   42.5      → 42 (float truncated to int — LLMs sometimes emit 0.0)
//
// Use it via json tag `json:"line_start"` on tool-param structs so
// json.Decoder picks up the UnmarshalJSON method automatically.
type FlexInt int

// Int returns the underlying int — convenience so call sites do not
// have to cast `int(fi)` everywhere.
func (fi FlexInt) Int() int { return int(fi) }

// UnmarshalJSON implements json.Unmarshaler. The decision tree is:
//
//  1. null → 0
//  2. starts with '"' → treat as JSON string, trim, empty = 0, else
//     parse with strconv.Atoi (or ParseFloat → truncate for
//     unusual "42.5"-in-a-string encodings)
//  3. otherwise treat as a JSON number; parse int directly, or if
//     that fails, parse float and truncate.
func (fi *FlexInt) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*fi = 0
		return nil
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return fmt.Errorf("flex-int: %w", err)
		}
		s = strings.TrimSpace(s)
		if s == "" {
			*fi = 0
			return nil
		}
		if v, err := strconv.Atoi(s); err == nil {
			*fi = FlexInt(v)
			return nil
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			*fi = FlexInt(int(f))
			return nil
		}
		return fmt.Errorf("flex-int: cannot parse %q as integer", s)
	}
	// JSON number path.
	var v int
	if err := json.Unmarshal(trimmed, &v); err == nil {
		*fi = FlexInt(v)
		return nil
	}
	var f float64
	if err := json.Unmarshal(trimmed, &f); err == nil {
		*fi = FlexInt(int(f))
		return nil
	}
	return fmt.Errorf("flex-int: cannot parse %q as integer", string(trimmed))
}
