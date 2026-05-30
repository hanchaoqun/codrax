package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// FlexBool is a bool that absorbs common LLM string encodings such as "true",
// "yes", "1", "false", "no", and "0" at tool-parameter boundaries.
type FlexBool bool

func (fb FlexBool) Bool() bool { return bool(fb) }

func (fb *FlexBool) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*fb = false
		return nil
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return fmt.Errorf("flex-bool: %w", err)
		}
		v, err := parseFlexBoolLiteral(s)
		if err != nil {
			return err
		}
		*fb = FlexBool(v)
		return nil
	}
	var v bool
	if err := json.Unmarshal(trimmed, &v); err == nil {
		*fb = FlexBool(v)
		return nil
	}
	var n float64
	if err := json.Unmarshal(trimmed, &n); err == nil {
		*fb = FlexBool(n != 0)
		return nil
	}
	return fmt.Errorf("flex-bool: cannot parse %q as boolean", string(trimmed))
}

func parseFlexBoolLiteral(raw string) (bool, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	switch s {
	case "", "false", "f", "no", "n", "off", "0":
		return false, nil
	case "true", "t", "yes", "y", "on", "1":
		return true, nil
	}
	if n, err := strconv.ParseFloat(s, 64); err == nil {
		return n != 0, nil
	}
	return false, fmt.Errorf("flex-bool: cannot parse %q as boolean", raw)
}
