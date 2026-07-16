package tracequery

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

const (
	// RawPerfCaptureCompletenessCaveatToken is the single wire-token authority
	// shared by tracequery and its bounded tool/tracediag renderers.
	RawPerfCaptureCompletenessCaveatToken = "tracebundle_raw_perf_capture_completeness"

	traceBundleRawPerfCaptureProfile = "raw_perf_record_census_v1"
	traceBundleRawPerfCaptureSource  = "linux_perf_data_record_stream"

	traceBundleRawPerfResidualProfile     = "raw_perf_record_header_residual_v1"
	traceBundleRawPerfResidualSource      = "linux_perf_data_record_headers"
	traceBundleRawPerfResidualCaveatToken = "raw_perf_capture_residual"

	traceBundleRawPerfAggregateNotReported = "not_reported"
	traceBundleRawPerfAggregateExact       = "exact"
	traceBundleRawPerfAggregateUnknown     = "unknown"

	traceBundleRawPerfUnknownOverflow             = "aggregate_overflow"
	traceBundleRawPerfUnknownMalformed            = "malformed_aggregate"
	traceBundleRawPerfUnknownMalformedAndOverflow = "malformed_aggregate_and_overflow"

	// The current owned perftrace wire prefix is systraceHeader in
	// hitraceconv/render.go. A structural parity pin below prevents this
	// disclosure consumer from drifting when that closed producer prefix moves.
	traceBundleOwnedPerftraceHeaderLines = 11
)

// IsRawPerfCaptureCompletenessCaveat identifies exactly the typed advisory
// lane; fuzzy prefixes and an unseparated token do not acquire its semantics.
func IsRawPerfCaptureCompletenessCaveat(caveat string) bool {
	return strings.HasPrefix(strings.TrimSpace(caveat), RawPerfCaptureCompletenessCaveatToken+" ")
}

// RawPerfCaptureCompletenessCaveats returns the source-ordered, exact deduped
// advisory roster. Consumers use this helper so hoisting and ordinary-roster
// filtering cannot drift apart.
func RawPerfCaptureCompletenessCaveats(caveats []string) []string {
	var out []string
	seen := make(map[string]struct{})
	for _, caveat := range caveats {
		caveat = strings.TrimSpace(caveat)
		if !IsRawPerfCaptureCompletenessCaveat(caveat) {
			continue
		}
		if _, duplicate := seen[caveat]; duplicate {
			continue
		}
		seen[caveat] = struct{}{}
		out = append(out, caveat)
	}
	return out
}

// These private wire mirrors deliberately do not import hitraceconv. The
// converter imports tracequery for publication validation, so sharing its Go
// type here would create a package cycle. The JSON schema and validator below
// instead form a disclosure-only consumer: they can withdraw manifest numeric
// authority, but they never participate in perf admission or child parsing.
type traceBundleRawPerfCaptureCompleteness struct {
	Profile           string                           `json:"profile"`
	Source            string                           `json:"source"`
	SampleRecords     traceBundleRawPerfRecordCensus   `json:"sample_records"`
	LostRecords       traceBundleRawPerfRecordCensus   `json:"lost_records"`
	LostSampleRecords traceBundleRawPerfRecordCensus   `json:"lost_sample_records"`
	AuxRecords        traceBundleRawPerfRecordCensus   `json:"aux_records"`
	LostEvents        traceBundleRawPerfAggregateTotal `json:"lost_events"`
	LostSamples       traceBundleRawPerfAggregateTotal `json:"lost_samples"`
	AuxBytes          traceBundleRawPerfAggregateTotal `json:"aux_bytes"`
}

type traceBundleRawPerfRecordCensus struct {
	Physical uint64 `json:"physical"`
	Accepted uint64 `json:"accepted"`
	Rejected uint64 `json:"rejected"`
}

type traceBundleRawPerfAggregateTotal struct {
	State  string `json:"state"`
	Value  uint64 `json:"value"`
	Reason string `json:"reason,omitempty"`
}

type traceBundleRawPerfCaptureResidual struct {
	Profile           string `json:"profile"`
	Source            string `json:"source"`
	ThrottleRecords   uint64 `json:"throttle_records"`
	UnthrottleRecords uint64 `json:"unthrottle_records"`
}

type traceBundleRawPerfArtifactClaim struct {
	path       string
	queryReady bool
	capture    traceBundleRawPerfCaptureCompleteness
	residual   *traceBundleRawPerfCaptureResidual
}

// traceBundleRawPerfCaptureCompletenessCaveats is called only after the
// selection layer has decoded a currently held manifest generation and bound
// every V2 child to its declared size+digest. It keeps the census global (not
// query-window scoped) and advisory. Any malformed or inconsistent raw face
// invalidates the whole census projection so a forged sibling cannot coexist
// beside apparently authoritative declared numbers.
func traceBundleRawPerfCaptureCompletenessCaveats(bundle traceBundleFile) []string {
	hasPayload := traceBundleHasRawPerfCapturePayload(bundle)
	if !hasPayload {
		return nil
	}
	invalid := func(reason string) []string {
		return []string{traceBundleRawPerfInvalidCaveat(reason)}
	}
	if bundle.schemaMode != traceBundleSchemaV2 {
		return invalid("untrusted_manifest_schema")
	}
	if traceBundleRawPerfResidualWrongLaneCaveat(bundle) {
		return invalid("raw_residual_wrong_caveat_lane")
	}

	claims := make([]traceBundleRawPerfArtifactClaim, 0)
	claimByPath := make(map[string]int)
	for _, artifact := range bundle.Artifacts {
		hasResidualCaveat := traceBundleRawPerfResidualCaveatCount(artifact.Caveats) > 0
		if artifact.Perf == nil {
			if hasResidualCaveat {
				return invalid("cross_type_or_profile_residual")
			}
			continue
		}
		rawIdentity := traceBundleArtifactClaimsRawPerfProfile(artifact)
		if artifact.Perf.RawCaptureCompleteness == nil && artifact.Perf.RawCaptureResidual == nil && !hasResidualCaveat && !rawIdentity {
			continue
		}
		if artifact.Perf.RawCaptureCompleteness == nil {
			return invalid("raw_artifact_missing_census")
		}
		if !rawIdentity || !traceBundleRawPerfArtifactProfileValid(artifact) {
			return invalid("cross_type_or_profile_payload")
		}
		if reason := validateTraceBundleRawPerfCaptureCompleteness(*artifact.Perf.RawCaptureCompleteness); reason != "" {
			return invalid(reason)
		}
		var residual *traceBundleRawPerfCaptureResidual
		if artifact.Perf.RawCaptureResidual != nil {
			value := *artifact.Perf.RawCaptureResidual
			if reason := validateTraceBundleRawPerfCaptureResidual(value); reason != "" {
				return invalid(reason)
			}
			residual = &value
		}
		if reason := validateTraceBundleRawPerfResidualArtifactCaveats(artifact.Caveats, residual); reason != "" {
			return invalid(reason)
		}
		if _, duplicate := claimByPath[artifact.Path]; duplicate {
			return invalid("duplicate_artifact_claim")
		}
		claimByPath[artifact.Path] = len(claims)
		claims = append(claims, traceBundleRawPerfArtifactClaim{
			path: artifact.Path, queryReady: artifact.Perf.TraceQueryReady,
			capture: *artifact.Perf.RawCaptureCompleteness, residual: residual,
		})
	}
	if len(claims) == 0 {
		return invalid("raw_receipt_without_artifact_claim")
	}
	decisionCount := make(map[string]int, len(claims))
	for _, decision := range bundle.ProviderDecisions {
		isRawLooking := decision.ProviderKind == "raw_fallback" || decision.ProviderName == "codrax_raw_perfdata"
		isRawDecision := decision.ProviderKind == "raw_fallback" && decision.ProviderName == "codrax_raw_perfdata"
		if isRawLooking && !isRawDecision {
			return invalid("raw_provider_identity_mismatch")
		}
		participates := decision.Succeeded || decision.TraceQueryReady || strings.TrimSpace(decision.ArtifactPath) != ""
		if isRawDecision && !participates {
			if !traceBundleRawPerfNonSuccessDecisionRouteValid(decision) {
				return invalid("raw_non_success_decision_mismatch")
			}
			if _, outputClaim := claimByPath[decision.OutputPath]; outputClaim {
				return invalid("raw_non_success_decision_conflict")
			}
			continue
		}
		if !participates {
			// Failed official/provider probes describe the route but own no raw
			// artifact generation. The normal failed official probe may retain
			// OutputPath for its subsequent raw fallback.
			_, artifactClaim := claimByPath[decision.ArtifactPath]
			if artifactClaim {
				return invalid("raw_non_success_decision_conflict")
			}
			continue
		}
		claimIndex, claimsPath := claimByPath[decision.ArtifactPath]
		if !claimsPath {
			if isRawDecision {
				return invalid("orphan_raw_provider_decision")
			}
			continue
		}
		claim := claims[claimIndex]
		if !isRawDecision || !traceBundleRawPerfSuccessDecisionRouteValid(decision, claim) {
			return invalid("raw_provider_decision_mismatch")
		}
		decisionCount[claim.path]++
		if decisionCount[claim.path] > 1 {
			return invalid("duplicate_raw_provider_decision")
		}
	}
	for _, claim := range claims {
		if decisionCount[claim.path] != 1 {
			return invalid("missing_raw_provider_decision")
		}
	}
	for _, alignment := range bundle.PerfClockAlignments {
		claimIndex, ok := claimByPath[alignment.ArtifactPath]
		if ok && !claims[claimIndex].queryReady {
			return invalid("inventory_clock_alignment_conflict")
		}
	}

	// Raw capture completeness is a perf receipt projection. The DB coverage
	// lane cannot carry it, and a fuzzy/future trace-coverage row cannot acquire
	// receipt authority merely by copying the JSON field.
	for _, coverage := range bundle.TraceDBCoverage {
		if coverage.RawCaptureCompleteness != nil || coverage.RawCaptureResidual != nil || traceBundlePerfReceiptNamespaceTable(coverage.Table) {
			return invalid("wrong_coverage_lane")
		}
	}
	coverageSeen := make(map[string]bool, len(claims))
	for _, coverage := range bundle.TraceCoverage {
		_, pointsToRawClaim := claimByPath[coverage.ArtifactPath]
		isClosedPerfReceipt := tracebundle.IsPerfReceiptCoverage(
			coverage.Family, coverage.Table, coverage.Role, coverage.ArtifactPath,
		)
		isRawReceipt := traceBundleIsRawPerfReceiptCoverage(coverage)
		if traceBundlePerfReceiptNamespaceTable(coverage.Table) && !isClosedPerfReceipt {
			return invalid("malformed_or_future_perf_receipt")
		}
		if pointsToRawClaim && isClosedPerfReceipt && !isRawReceipt {
			return invalid("competing_nonraw_receipt_for_raw_claim")
		}
		if coverage.RawCaptureCompleteness == nil {
			if coverage.RawCaptureResidual != nil {
				return invalid("raw_receipt_residual_without_census")
			}
			if isRawReceipt {
				return invalid("raw_receipt_missing_census")
			}
			continue
		}
		if !isRawReceipt {
			return invalid("wrong_coverage_lane")
		}
		claimIndex, ok := claimByPath[coverage.ArtifactPath]
		if !ok {
			return invalid("orphan_coverage_claim")
		}
		if coverageSeen[coverage.ArtifactPath] {
			return invalid("duplicate_coverage_claim")
		}
		coverageSeen[coverage.ArtifactPath] = true
		claim := claims[claimIndex]
		capture := *coverage.RawCaptureCompleteness
		if reason := validateTraceBundleRawPerfCaptureCompleteness(capture); reason != "" {
			return invalid(reason)
		}
		if capture != claim.capture {
			return invalid("artifact_coverage_mismatch")
		}
		if (coverage.RawCaptureResidual == nil) != (claim.residual == nil) {
			return invalid("artifact_coverage_residual_presence_mismatch")
		}
		if coverage.RawCaptureResidual != nil {
			residual := *coverage.RawCaptureResidual
			if reason := validateTraceBundleRawPerfCaptureResidual(residual); reason != "" {
				return invalid(reason)
			}
			if residual != *claim.residual {
				return invalid("artifact_coverage_residual_mismatch")
			}
		}
		if !coverage.Found || strings.TrimSpace(coverage.Error) != "" || strings.TrimSpace(coverage.Skipped) != "" ||
			coverage.RowsRead < traceBundleOwnedPerftraceHeaderLines ||
			uint64(coverage.RowsRead-traceBundleOwnedPerftraceHeaderLines) != capture.SampleRecords.Accepted ||
			coverage.RowsEmitted < 0 || uint64(coverage.RowsEmitted) != capture.SampleRecords.Accepted {
			return invalid("receipt_row_account_mismatch")
		}
		if claim.queryReady != (capture.SampleRecords.Accepted > 0) {
			return invalid("readiness_sample_account_mismatch")
		}
	}
	for _, claim := range claims {
		if !coverageSeen[claim.path] {
			return invalid("missing_coverage_claim")
		}
	}

	out := make([]string, 0, len(claims))
	for _, claim := range claims {
		out = append(out, traceBundleRawPerfValidCaveat(claim))
	}
	return out
}

func traceBundleRawPerfSuccessDecisionRouteValid(decision traceBundleProviderDecision, claim traceBundleRawPerfArtifactClaim) bool {
	if !traceBundleRawPerfDecisionRouteBaseValid(decision) ||
		decision.ProviderKind != "raw_fallback" || decision.ProviderName != "codrax_raw_perfdata" ||
		decision.InputFormat != "linux_perf_data" ||
		!decision.Selected || !decision.Attempted || !decision.Succeeded ||
		decision.TraceQueryReady != claim.queryReady || decision.ArtifactPath != claim.path ||
		decision.OutputPath != claim.path ||
		strings.TrimSpace(decision.Reason) != "" || strings.TrimSpace(decision.Caveat) != "" {
		return false
	}
	// Mirror the producer's closed raw route: auto reaches raw only as a
	// fallback; an explicit raw/fallback parser selection is not a fallback.
	switch {
	case decision.Fallback:
		return decision.ParserMode == "auto"
	default:
		return decision.ParserMode == "raw" || decision.ParserMode == "fallback"
	}
}

func traceBundleRawPerfDecisionRouteBaseValid(decision traceBundleProviderDecision) bool {
	if decision.InputPath == "" || decision.InputPath != strings.TrimSpace(decision.InputPath) ||
		decision.OutputPath == "" || decision.OutputPath != strings.TrimSpace(decision.OutputPath) ||
		decision.InputFormat != strings.TrimSpace(decision.InputFormat) ||
		decision.Stage != "direct_input" && decision.Stage != "standalone_hiperf" {
		return false
	}
	switch decision.ParserMode {
	case "auto", "official", "raw", "fallback":
		return true
	default:
		return false
	}
}

func traceBundleRawPerfNonSuccessDecisionRouteValid(decision traceBundleProviderDecision) bool {
	return traceBundleRawPerfDecisionRouteBaseValid(decision) &&
		decision.ProviderKind == "raw_fallback" && decision.ProviderName == "codrax_raw_perfdata" &&
		!decision.Succeeded && !decision.TraceQueryReady && decision.ArtifactPath == "" &&
		decision.Reason != "" && decision.Reason == strings.TrimSpace(decision.Reason) &&
		(!decision.Attempted || decision.Selected)
}

func traceBundleArtifactClaimsRawPerfProfile(artifact traceBundleArtifact) bool {
	return artifact.Type == "perftrace" && artifact.Perf != nil &&
		artifact.Perf.ProviderKind == "raw_fallback" &&
		artifact.Perf.ProviderName == "codrax_raw_perfdata" &&
		artifact.Perf.InputFormat == "linux_perf_data" &&
		artifact.Perf.OutputFormat == "codrax_perftrace"
}

func traceBundleRawPerfArtifactProfileValid(artifact traceBundleArtifact) bool {
	perf := artifact.Perf
	return traceBundleArtifactClaimsRawPerfProfile(artifact) &&
		artifact.Converter == "hitraceconv-v1+raw-perfdata" &&
		perf.TimeDomain == "perf_data_time_ns" && perf.TimeAlignment == "assumed" &&
		perf.ThreadIdentity == "pid_tid_from_sample_or_comm" &&
		perf.CPUIdentity == "sample_cpu_when_recorded" && perf.EventWeight == "period_or_1" &&
		perf.Symbolization == "hiperf_saved_symbols_or_unsymbolized_ip" &&
		perf.Callchain == "symbolized_when_hiperf_files_symbol_present_else_ip_only" &&
		perf.DSOLabel == "mmap_best_effort" && perf.BuildID == "feature_build_id_when_present" &&
		perf.OffCPU == "hiperf_cpu_off_sched_switch_when_event_desc_present" &&
		perf.Confidence == "degraded" && perf.Degraded &&
		traceBundleRawPerfFixedCaveatsValid(perf.Caveats)
}

func traceBundleRawPerfFixedCaveatsValid(caveats []string) bool {
	return len(caveats) == 2 &&
		caveats[0] == "raw fallback resolves function names only from saved hiperf symbol sections; without those sections it remains IP/DSO-level" &&
		caveats[1] == "raw fallback can label hiperf --offcpu sched_switch samples when official EVENT_DESC and HIPERF_CPU_OFF features are present, but full off-CPU stack expansion still needs official hiperf report flow"
}

func traceBundleIsRawPerfReceiptCoverage(coverage traceBundleCoverage) bool {
	return tracebundle.IsPerfReceiptCoverage(
		coverage.Family, coverage.Table, coverage.Role, coverage.ArtifactPath,
	) && coverage.Table == tracebundle.PerfReceiptTableRawPerf
}

func traceBundlePerfReceiptNamespaceTable(table string) bool {
	table = strings.TrimSpace(table)
	return tracebundle.IsClosedPerfReceiptTable(table) || strings.HasPrefix(table, "perftrace_")
}

func traceBundleHasRawPerfCapturePayload(bundle traceBundleFile) bool {
	if traceBundleRawPerfResidualWrongLaneCaveat(bundle) {
		return true
	}
	for _, artifact := range bundle.Artifacts {
		if traceBundleRawPerfResidualCaveatCount(artifact.Caveats) > 0 ||
			artifact.Perf != nil && (artifact.Perf.RawCaptureCompleteness != nil ||
				artifact.Perf.RawCaptureResidual != nil ||
				traceBundleRawPerfResidualCaveatCount(artifact.Perf.Caveats) > 0 ||
				traceBundleArtifactClaimsRawPerfProfile(artifact)) {
			return true
		}
	}
	for _, coverage := range bundle.TraceDBCoverage {
		if coverage.RawCaptureCompleteness != nil || coverage.RawCaptureResidual != nil ||
			traceBundlePerfReceiptNamespaceTable(coverage.Table) {
			return true
		}
	}
	for _, coverage := range bundle.TraceCoverage {
		isClosedPerfReceipt := tracebundle.IsPerfReceiptCoverage(
			coverage.Family, coverage.Table, coverage.Role, coverage.ArtifactPath,
		)
		if coverage.RawCaptureCompleteness != nil || coverage.RawCaptureResidual != nil ||
			traceBundleIsRawPerfReceiptCoverage(coverage) ||
			traceBundlePerfReceiptNamespaceTable(coverage.Table) && !isClosedPerfReceipt {
			return true
		}
	}
	return false
}

func traceBundleRawPerfInvalidCaveat(reason string) string {
	return strings.Join([]string{
		RawPerfCaptureCompletenessCaveatToken,
		"authority=manifest_advisory",
		"capture_hard_gate=false",
		"positive_evidence=preserve",
		"absence_policy=require_quality_caveat",
		"valid=false",
		"applicability=ignored",
		"reason=" + traceBundleControlSafeToken(reason),
	}, " ")
}

func traceBundleRawPerfValidCaveat(claim traceBundleRawPerfArtifactClaim) string {
	capture := claim.capture
	artifactPathDigest := sha256.Sum256([]byte(claim.path))
	captureState := "query_ready"
	analysisParts := []string{
		"effective_clock_evidence=unchanged_by_capture_advisory",
		"census_participation=capture_quality_only",
	}
	if !claim.queryReady {
		captureState = "inventory_only"
		analysisParts = []string{
			"effective_clock_evidence=none",
			"sample_aggregation=none",
			"clock_alignment=none",
			"thread_attribution=none",
			"root_cause_rank=none",
			"census_participation=capture_quality_only",
		}
	}
	parts := []string{
		RawPerfCaptureCompletenessCaveatToken,
		"authority=manifest_advisory",
		"capture_hard_gate=false",
		"positive_evidence=preserve",
		"absence_policy=require_quality_caveat",
		"census_scope=observed_perf_record_stream",
		"device_capture_completeness=not_claimed",
		"valid=true",
		"applicability=raw_perf_artifact_and_receipt",
		"artifact=" + traceBundleControlSafeToken(claim.path),
		fmt.Sprintf("artifact_path_sha256=%x", artifactPathDigest),
		fmt.Sprintf("query_ready=%t", claim.queryReady),
		"capture_state=" + captureState,
		"capture_quality_issue=" + fmt.Sprintf("%t", traceBundleRawPerfCaptureHasQualityIssue(capture, claim.residual)),
	}
	parts = append(parts, analysisParts...)
	parts = append(parts,
		traceBundleRawPerfRecordToken("sample_records", capture.SampleRecords),
		traceBundleRawPerfRecordToken("lost_records", capture.LostRecords),
		traceBundleRawPerfRecordToken("lost_sample_records", capture.LostSampleRecords),
		traceBundleRawPerfRecordToken("aux_records", capture.AuxRecords),
		traceBundleRawPerfAggregateToken("lost_events", capture.LostEvents),
		traceBundleRawPerfAggregateToken("lost_samples", capture.LostSamples),
		traceBundleRawPerfAggregateToken("aux_bytes", capture.AuxBytes),
	)
	if claim.residual == nil {
		parts = append(parts,
			"perf_sampler_throttle_records=not_reported",
			"perf_sampler_unthrottle_records=not_reported",
		)
	} else {
		parts = append(parts,
			fmt.Sprintf("perf_sampler_throttle_records=exact:%d", claim.residual.ThrottleRecords),
			fmt.Sprintf("perf_sampler_unthrottle_records=exact:%d", claim.residual.UnthrottleRecords),
		)
	}
	parts = append(parts,
		"perf_sampler_throttle_scope=observed_perf_record_type_headers",
		"perf_sampler_throttle_payload_validation=not_claimed",
		"perf_sampler_throttle_semantics=capture_quality_not_cpu_thermal",
		"perf_sampler_throttle_duration=not_reported",
		"perf_sampler_throttle_lost_samples=not_reported",
	)
	return strings.Join(parts, " ")
}

func traceBundleRawPerfRecordToken(name string, record traceBundleRawPerfRecordCensus) string {
	return fmt.Sprintf("%s=physical:%d,accepted:%d,rejected:%d", name, record.Physical, record.Accepted, record.Rejected)
}

func traceBundleRawPerfAggregateToken(name string, total traceBundleRawPerfAggregateTotal) string {
	switch total.State {
	case traceBundleRawPerfAggregateNotReported:
		return name + "=not_reported"
	case traceBundleRawPerfAggregateExact:
		return fmt.Sprintf("%s=exact:%d", name, total.Value)
	case traceBundleRawPerfAggregateUnknown:
		return name + "=unknown:" + total.Reason
	default:
		// Production reaches this formatter only after strict validation.
		return name + "=unknown:invalid_state"
	}
}

func validateTraceBundleRawPerfCaptureResidual(residual traceBundleRawPerfCaptureResidual) string {
	if residual.Profile != traceBundleRawPerfResidualProfile {
		return "invalid_residual_profile"
	}
	if residual.Source != traceBundleRawPerfResidualSource {
		return "invalid_residual_source"
	}
	return ""
}

func traceBundleRawPerfResidualCaveat(residual traceBundleRawPerfCaptureResidual) (string, bool) {
	if validateTraceBundleRawPerfCaptureResidual(residual) != "" ||
		residual.ThrottleRecords == 0 && residual.UnthrottleRecords == 0 {
		return "", false
	}
	return fmt.Sprintf(
		"%s authority=artifact_receipt_advisory capture_hard_gate=false scope=observed_perf_record_type_headers payload_validation=not_claimed interpretation=perf_sampling_control_not_cpu_thermal no_duration_or_lost_sample_count=true perf_sampler_throttle_records=%d perf_sampler_unthrottle_records=%d",
		traceBundleRawPerfResidualCaveatToken, residual.ThrottleRecords, residual.UnthrottleRecords,
	), true
}

func traceBundleRawPerfResidualCaveatReserved(caveat string) bool {
	return strings.HasPrefix(strings.TrimSpace(caveat), traceBundleRawPerfResidualCaveatToken)
}

func traceBundleRawPerfResidualWrongLaneCaveat(bundle traceBundleFile) bool {
	contains := func(caveats []string) bool {
		return traceBundleRawPerfResidualCaveatCount(caveats) > 0
	}
	if contains(bundle.Caveats) {
		return true
	}
	for _, artifact := range bundle.Artifacts {
		if artifact.Perf != nil && contains(artifact.Perf.Caveats) {
			return true
		}
	}
	for _, decision := range bundle.ProviderDecisions {
		if traceBundleRawPerfResidualCaveatReserved(decision.Caveat) {
			return true
		}
	}
	for _, decision := range bundle.TraceDecisions {
		if traceBundleRawPerfResidualCaveatReserved(decision.Caveat) {
			return true
		}
	}
	for _, gate := range bundle.TraceToolGates {
		if contains(gate.Caveats) {
			return true
		}
	}
	for _, alignment := range bundle.PerfClockAlignments {
		if contains(alignment.Caveats) {
			return true
		}
	}
	return false
}

func traceBundleRawPerfResidualCaveatCount(caveats []string) int {
	count := 0
	for _, caveat := range caveats {
		if traceBundleRawPerfResidualCaveatReserved(caveat) {
			count++
		}
	}
	return count
}

func validateTraceBundleRawPerfResidualArtifactCaveats(caveats []string, residual *traceBundleRawPerfCaptureResidual) string {
	want := ""
	if residual != nil {
		if reason := validateTraceBundleRawPerfCaptureResidual(*residual); reason != "" {
			return reason
		}
		want, _ = traceBundleRawPerfResidualCaveat(*residual)
	}
	seen := 0
	for _, caveat := range caveats {
		if !traceBundleRawPerfResidualCaveatReserved(caveat) {
			continue
		}
		seen++
		if want == "" || caveat != want {
			return "raw_residual_artifact_caveat_mismatch"
		}
	}
	if want == "" && seen != 0 || want != "" && seen != 1 {
		return "raw_residual_artifact_caveat_count_mismatch"
	}
	return ""
}

func traceBundleRawPerfCaptureHasQualityIssue(capture traceBundleRawPerfCaptureCompleteness, residual *traceBundleRawPerfCaptureResidual) bool {
	if capture.SampleRecords.Rejected > 0 || capture.LostRecords.Rejected > 0 ||
		capture.LostSampleRecords.Rejected > 0 || capture.AuxRecords.Rejected > 0 {
		return true
	}
	return traceBundleRawPerfLossTotalIsIssue(capture.LostEvents) ||
		traceBundleRawPerfLossTotalIsIssue(capture.LostSamples) ||
		capture.AuxBytes.State == traceBundleRawPerfAggregateUnknown ||
		residual != nil && (residual.ThrottleRecords > 0 || residual.UnthrottleRecords > 0)
}

func traceBundleRawPerfLossTotalIsIssue(total traceBundleRawPerfAggregateTotal) bool {
	return total.State == traceBundleRawPerfAggregateUnknown ||
		(total.State == traceBundleRawPerfAggregateExact && total.Value > 0)
}

func validateTraceBundleRawPerfCaptureCompleteness(capture traceBundleRawPerfCaptureCompleteness) string {
	if capture.Profile != traceBundleRawPerfCaptureProfile {
		return "invalid_profile"
	}
	if capture.Source != traceBundleRawPerfCaptureSource {
		return "invalid_source"
	}
	records := []struct {
		name   string
		census traceBundleRawPerfRecordCensus
	}{
		{"sample", capture.SampleRecords},
		{"lost", capture.LostRecords},
		{"lost_samples", capture.LostSampleRecords},
		{"aux", capture.AuxRecords},
	}
	for _, record := range records {
		if record.census.Accepted > record.census.Physical ||
			record.census.Rejected != record.census.Physical-record.census.Accepted {
			return record.name + "_record_census_not_closed"
		}
	}
	totals := []struct {
		name    string
		records traceBundleRawPerfRecordCensus
		total   traceBundleRawPerfAggregateTotal
	}{
		{"lost_events", capture.LostRecords, capture.LostEvents},
		{"lost_samples", capture.LostSampleRecords, capture.LostSamples},
		{"aux_bytes", capture.AuxRecords, capture.AuxBytes},
	}
	for _, item := range totals {
		if reason := validateTraceBundleRawPerfAggregateTotal(item.records, item.total); reason != "" {
			return item.name + "_" + reason
		}
	}
	if capture.SampleRecords.Accepted == 0 && !traceBundleRawPerfZeroSamplePublicationIssue(capture) {
		return "inventory_without_zero_sample_publication_issue"
	}
	return ""
}

func traceBundleRawPerfZeroSamplePublicationIssue(capture traceBundleRawPerfCaptureCompleteness) bool {
	if capture.SampleRecords.Rejected > 0 || capture.LostRecords.Rejected > 0 ||
		capture.LostSampleRecords.Rejected > 0 || capture.AuxRecords.Rejected > 0 {
		return true
	}
	return traceBundleRawPerfLossTotalIsIssue(capture.LostEvents) ||
		traceBundleRawPerfLossTotalIsIssue(capture.LostSamples)
}

func validateTraceBundleRawPerfAggregateTotal(records traceBundleRawPerfRecordCensus, total traceBundleRawPerfAggregateTotal) string {
	if records.Physical == 0 {
		if total.State != traceBundleRawPerfAggregateNotReported || total.Value != 0 || total.Reason != "" {
			return "must_be_not_reported_without_records"
		}
		return ""
	}
	switch total.State {
	case traceBundleRawPerfAggregateExact:
		if records.Rejected != 0 || total.Reason != "" {
			return "invalid_exact_total"
		}
	case traceBundleRawPerfAggregateUnknown:
		if total.Value != 0 {
			return "unknown_exposes_numeric_prefix"
		}
		switch total.Reason {
		case traceBundleRawPerfUnknownOverflow:
			if records.Rejected != 0 || records.Accepted < 2 {
				return "invalid_overflow_reason"
			}
		case traceBundleRawPerfUnknownMalformed:
			if records.Rejected == 0 {
				return "invalid_malformed_reason"
			}
		case traceBundleRawPerfUnknownMalformedAndOverflow:
			if records.Rejected == 0 || records.Accepted < 2 {
				return "invalid_malformed_overflow_reason"
			}
		default:
			return "invalid_unknown_reason"
		}
	default:
		return "invalid_state"
	}
	return ""
}
