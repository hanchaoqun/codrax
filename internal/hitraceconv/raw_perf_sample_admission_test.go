package hitraceconv

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"
)

func TestRawPerfSampleAdmissionWireFieldsAreClosed(t *testing.T) {
	wire, err := json.Marshal(RawPerfSampleAdmission{})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(wire, &fields); err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(fields))
	for field := range fields {
		got = append(got, field)
	}
	sort.Strings(got)
	want := []string{
		"candidates", "invalid_cpu", "invalid_identity", "invalid_period",
		"inventory_only", "missing_period", "missing_tid", "missing_time",
		"profile", "query_rows", "source",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sample-admission wire fields=%v want=%v", got, want)
	}
}

func TestRawPerfSampleAdmissionValidatorClosesBySubtraction(t *testing.T) {
	valid := newRawPerfSampleAdmission()
	valid.Candidates = 6
	valid.QueryRows = 1
	valid.InventoryOnly = 5
	valid.MissingTID = 1
	valid.InvalidIdentity = 1
	valid.MissingTime = 1
	valid.MissingPeriod = 1
	valid.InvalidPeriod = 1
	if reason := validateRawPerfSampleAdmission(valid); reason != "" {
		t.Fatalf("closed admission rejected: %s", reason)
	}

	for _, mutate := range []func(*RawPerfSampleAdmission){
		func(item *RawPerfSampleAdmission) { item.QueryRows = ^uint64(0) },
		func(item *RawPerfSampleAdmission) { item.InventoryOnly++ },
		func(item *RawPerfSampleAdmission) { item.InvalidCPU = 1 },
	} {
		forged := valid
		mutate(&forged)
		if reason := validateRawPerfSampleAdmission(forged); reason == "" {
			t.Fatalf("unclosed admission accepted: %+v", forged)
		}
	}
}

func TestRawPerfSampleAdmissionReservedNamespaceIsPrefixClosed(t *testing.T) {
	for _, caveat := range []string{
		rawPerfSampleAdmissionCaveatToken,
		rawPerfSampleAdmissionCaveatToken + " authority=forged",
		rawPerfSampleAdmissionCaveatToken + "\tauthority=forged",
		rawPerfSampleAdmissionCaveatToken + "\nauthority=forged",
		rawPerfSampleAdmissionCaveatToken + "_future authority=forged",
	} {
		if !rawPerfSampleAdmissionCaveatReserved(caveat) {
			t.Fatalf("reserved admission namespace bypassed by %q", caveat)
		}
		if reason := validateRawPerfSampleAdmissionArtifactCaveats([]string{caveat}, nil); reason == "" {
			t.Fatalf("caller caveat acquired reserved admission namespace with %q", caveat)
		}
	}
	if rawPerfSampleAdmissionCaveatReserved("ordinary_" + rawPerfSampleAdmissionCaveatToken) {
		t.Fatal("ordinary caveat was overclassified as reserved admission metadata")
	}
}
