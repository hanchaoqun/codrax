package tool

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFlexInt_Unmarshal(t *testing.T) {
	type box struct {
		N FlexInt `json:"n"`
	}
	cases := []struct {
		name    string
		in      string
		want    int
		wantErr bool
	}{
		{"plain int", `{"n": 42}`, 42, false},
		{"zero", `{"n": 0}`, 0, false},
		{"negative int", `{"n": -7}`, -7, false},
		{"quoted int", `{"n": "42"}`, 42, false},
		{"quoted int with whitespace", `{"n": "  42 "}`, 42, false},
		{"empty string → 0", `{"n": ""}`, 0, false},
		{"null → 0", `{"n": null}`, 0, false},
		{"missing → 0", `{}`, 0, false},
		{"float truncated", `{"n": 42.5}`, 42, false},
		{"quoted float truncated", `{"n": "42.9"}`, 42, false},
		{"non-numeric string errors", `{"n": "abc"}`, 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var b box
			err := json.Unmarshal([]byte(c.in), &b)
			if c.wantErr {
				if err == nil {
					t.Fatalf("want error, got %v", b.N)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if b.N.Int() != c.want {
				t.Errorf("got %d, want %d", b.N.Int(), c.want)
			}
		})
	}
}

// Strict-decoder compatibility: FlexInt must work with
// json.Decoder.DisallowUnknownFields() so emit_evidence's strict
// unmarshal stays intact.
func TestFlexInt_WithDisallowUnknownFields(t *testing.T) {
	type box struct {
		N FlexInt `json:"n"`
	}
	dec := json.NewDecoder(strings.NewReader(`{"n": "42"}`))
	dec.DisallowUnknownFields()
	var b box
	if err := dec.Decode(&b); err != nil {
		t.Fatalf("decoder: %v", err)
	}
	if b.N.Int() != 42 {
		t.Errorf("got %d, want 42", b.N.Int())
	}
}
