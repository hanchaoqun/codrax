package types

import (
	"reflect"
	"testing"
)

func TestEvidenceSalienceEnumBasics(t *testing.T) {
	if got := EvidenceSalienceStrings(); !reflect.DeepEqual(got, []string{"load_bearing", "exhaust_listed", "supporting", "context"}) {
		t.Fatalf("EvidenceSalienceStrings = %v", got)
	}
	if !SalienceUnset.IsValid() {
		t.Fatal("unset salience should be valid")
	}
	if EvidenceSalience("urgent").IsValid() {
		t.Fatal("unknown salience should be invalid")
	}
	if SalienceUnset.Resolve() != SalienceSupporting {
		t.Fatalf("unset resolves to %q, want supporting", SalienceUnset.Resolve())
	}
	if SalienceUnset.IsSet() {
		t.Fatal("unset must not be treated as an explicit salience lane")
	}
	if SalienceUnset.IsLocked() {
		t.Fatal("unset must not be locked")
	}
	if !SalienceLoadBearing.IsLocked() || !SalienceExhaustListed.IsLocked() {
		t.Fatal("load_bearing and exhaust_listed must be locked")
	}
	if SalienceSupporting.IsLocked() || SalienceContext.IsLocked() {
		t.Fatal("supporting and context must not be locked")
	}
}

func TestParseEvidenceSalience(t *testing.T) {
	tests := []struct {
		raw  string
		want EvidenceSalience
		ok   bool
	}{
		{"", SalienceUnset, true},
		{" load_bearing ", SalienceLoadBearing, true},
		{"EXHAUST_LISTED", SalienceExhaustListed, true},
		{"supporting", SalienceSupporting, true},
		{"context", SalienceContext, true},
		{"LOAD-BEARING", SalienceUnset, false},
		{"primary", SalienceUnset, false},
	}
	for _, tt := range tests {
		got, ok := ParseEvidenceSalience(tt.raw)
		if got != tt.want || ok != tt.ok {
			t.Fatalf("ParseEvidenceSalience(%q) = (%q,%v), want (%q,%v)", tt.raw, got, ok, tt.want, tt.ok)
		}
	}
}

func TestMergeEvidenceSalience(t *testing.T) {
	if got := MergeEvidenceSalience(SalienceUnset, SalienceContext); got != SalienceContext {
		t.Fatalf("unset + context = %q", got)
	}
	if got := MergeEvidenceSalience(SalienceSupporting, SalienceLoadBearing); got != SalienceLoadBearing {
		t.Fatalf("supporting + load_bearing = %q", got)
	}
	if got := MergeEvidenceSalience(SalienceLoadBearing, SalienceExhaustListed); got != SalienceLoadBearing {
		t.Fatalf("load_bearing should outrank exhaust_listed, got %q", got)
	}
	if got := MergeEvidenceSalience(SalienceContext, SalienceUnset); got != SalienceContext {
		t.Fatalf("unset source must not clear explicit salience, got %q", got)
	}
}
