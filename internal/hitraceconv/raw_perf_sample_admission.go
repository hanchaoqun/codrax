package hitraceconv

import (
	"fmt"
	"math"
	"strings"
)

const (
	rawPerfSampleAdmissionProfile     = "raw_perf_sample_admission_v1"
	rawPerfSampleAdmissionSource      = "linux_perf_data_sample_payloads"
	rawPerfSampleAdmissionCaveatToken = "raw_perf_sample_admission"
)

type rawPerfSampleAdmissionReason uint8

const (
	rawPerfSampleAdmissionQuery rawPerfSampleAdmissionReason = iota
	rawPerfSampleAdmissionMissingTID
	rawPerfSampleAdmissionInvalidIdentity
	rawPerfSampleAdmissionMissingTime
	rawPerfSampleAdmissionMissingPeriod
	rawPerfSampleAdmissionInvalidPeriod
	rawPerfSampleAdmissionInvalidCPU
)

func newRawPerfSampleAdmission() RawPerfSampleAdmission {
	return RawPerfSampleAdmission{
		Profile: rawPerfSampleAdmissionProfile,
		Source:  rawPerfSampleAdmissionSource,
	}
}

func cloneRawPerfSampleAdmission(admission RawPerfSampleAdmission) *RawPerfSampleAdmission {
	cloned := admission
	return &cloned
}

func rawPerfSampleAdmissionPointerEqual(left, right *RawPerfSampleAdmission) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func validateRawPerfSampleAdmission(admission RawPerfSampleAdmission) string {
	if admission.Profile != rawPerfSampleAdmissionProfile {
		return "profile must be raw_perf_sample_admission_v1"
	}
	if admission.Source != rawPerfSampleAdmissionSource {
		return "source must be linux_perf_data_sample_payloads"
	}
	if admission.QueryRows > admission.Candidates ||
		admission.InventoryOnly != admission.Candidates-admission.QueryRows {
		return "candidate census does not close"
	}
	remaining := admission.InventoryOnly
	for _, item := range []struct {
		name  string
		count uint64
	}{
		{name: "missing_tid", count: admission.MissingTID},
		{name: "invalid_identity", count: admission.InvalidIdentity},
		{name: "missing_time", count: admission.MissingTime},
		{name: "missing_period", count: admission.MissingPeriod},
		{name: "invalid_period", count: admission.InvalidPeriod},
		{name: "invalid_cpu", count: admission.InvalidCPU},
	} {
		if item.count > remaining {
			return item.name + " exceeds inventory-only census"
		}
		remaining -= item.count
	}
	if remaining != 0 {
		return "inventory-only reason census does not close"
	}
	return ""
}

func rawPerfSampleAdmissionHasIssue(admission RawPerfSampleAdmission) bool {
	return validateRawPerfSampleAdmission(admission) == "" && admission.InventoryOnly > 0
}

func classifyRawPerfSample(sample rawPerfSample) rawPerfSampleAdmissionReason {
	// The order is part of the v1 schema: every candidate receives one primary
	// verdict so untrusted manifest counters can be closed exactly.
	if !sample.TIDPresent {
		return rawPerfSampleAdmissionMissingTID
	}
	if sample.PID < 0 || int64(sample.PID) > math.MaxInt32 ||
		sample.TID < 0 || int64(sample.TID) > math.MaxInt32 {
		return rawPerfSampleAdmissionInvalidIdentity
	}
	if !sample.TimePresent {
		return rawPerfSampleAdmissionMissingTime
	}
	if !sample.PeriodPresent {
		return rawPerfSampleAdmissionMissingPeriod
	}
	if sample.Period > math.MaxInt64 {
		return rawPerfSampleAdmissionInvalidPeriod
	}
	if sample.CPUValid && (sample.CPU < 0 || sample.CPU > 4095) {
		return rawPerfSampleAdmissionInvalidCPU
	}
	return rawPerfSampleAdmissionQuery
}

func observeRawPerfSampleAdmission(admission *RawPerfSampleAdmission, sample rawPerfSample) bool {
	admission.Candidates++
	switch classifyRawPerfSample(sample) {
	case rawPerfSampleAdmissionQuery:
		admission.QueryRows++
		return true
	case rawPerfSampleAdmissionMissingTID:
		admission.MissingTID++
	case rawPerfSampleAdmissionInvalidIdentity:
		admission.InvalidIdentity++
	case rawPerfSampleAdmissionMissingTime:
		admission.MissingTime++
	case rawPerfSampleAdmissionMissingPeriod:
		admission.MissingPeriod++
	case rawPerfSampleAdmissionInvalidPeriod:
		admission.InvalidPeriod++
	case rawPerfSampleAdmissionInvalidCPU:
		admission.InvalidCPU++
	}
	admission.InventoryOnly++
	return false
}

func rawPerfSampleAdmissionCaveat(admission RawPerfSampleAdmission) (string, bool) {
	if !rawPerfSampleAdmissionHasIssue(admission) {
		return "", false
	}
	return fmt.Sprintf(
		"%s authority=artifact_receipt_advisory capture_hard_gate=false profile=%s source=%s candidates=%d query_rows=%d inventory_only=%d missing_tid=%d invalid_identity=%d missing_time=%d missing_period=%d invalid_period=%d invalid_cpu=%d no_thread_cpu_clock_or_weight_claim_for_inventory=true",
		rawPerfSampleAdmissionCaveatToken,
		admission.Profile,
		admission.Source,
		admission.Candidates,
		admission.QueryRows,
		admission.InventoryOnly,
		admission.MissingTID,
		admission.InvalidIdentity,
		admission.MissingTime,
		admission.MissingPeriod,
		admission.InvalidPeriod,
		admission.InvalidCPU,
	), true
}

func rawPerfSampleAdmissionCaveatReserved(caveat string) bool {
	return strings.HasPrefix(strings.TrimSpace(caveat), rawPerfSampleAdmissionCaveatToken)
}

func validateRawPerfSampleAdmissionArtifactCaveats(caveats []string, admission *RawPerfSampleAdmission) string {
	want := ""
	if admission != nil {
		if reason := validateRawPerfSampleAdmission(*admission); reason != "" {
			return reason
		}
		want, _ = rawPerfSampleAdmissionCaveat(*admission)
	}
	seen := 0
	for _, caveat := range caveats {
		if !rawPerfSampleAdmissionCaveatReserved(caveat) {
			continue
		}
		seen++
		if want == "" || caveat != want {
			return "artifact sample admission caveat is not canonical"
		}
	}
	if want == "" && seen != 0 || want != "" && seen != 1 {
		return "artifact sample admission caveat count does not match receipt"
	}
	return ""
}
