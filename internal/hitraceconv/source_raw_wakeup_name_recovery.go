package hitraceconv

import (
	"fmt"
	"math"
	"strings"
)

const traceDBDisplayNameSourceRawWakeupNew = "official_raw_sched_wakeup_new_exact_lifecycle"

func traceDBApplyRawWakeupNewDisplayNames(
	inventory *traceDBSourceNameInventory,
	authority *traceDBSchedulerAuthority,
) TraceDBCoverage {
	coverage := TraceDBCoverage{
		Family: "resolver.source_raw_wakeup_name",
		Table:  "__raw_sched_wakeup_new_names__",
		Role:   "display_name_companion",
		FieldSources: map[string]string{
			"authority": "exact comm/pname and payload pid from strict official raw sched_wakeup_new records in the same immutable input generation",
			"identity":  "payload public TID resolves to exactly one canonical DB thread/process incarnation at the exact raw timestamp through the shared lifecycle authority",
			"effect":    "fills only an otherwise-unresolved display name; never changes TID, TGID, ITID, owner, namespace, lifecycle, CPU, event admission or causality",
			"conflict":  "all lifecycle-admitted raw names for one canonical ITID must be byte-identical; conflicts fail closed for that ITID",
		},
		Metadata: map[string]string{"recovery_state": "unavailable"},
	}
	if inventory == nil || inventory.RawDecode.Role == "" {
		coverage.Skipped = "raw wakeup-new display-name recovery unavailable: source raw decode ledger absent"
		return coverage
	}
	coverage.Found = len(inventory.RawWakeupNames) > 0
	coverage.RowsRead = len(inventory.RawWakeupNames)
	if inventory.RawDecode.Metadata["decode_state"] != "strict_target_ledger_complete" ||
		inventory.RawDecode.Metrics["target_sched_wakeup_new_name_record_capture_failed"] != 0 ||
		inventory.RawDecode.Metrics["target_sched_wakeup_new_name_records_retained"] !=
			int64(len(inventory.RawWakeupNames)) {
		coverage.Metadata["recovery_state"] = "withheld_source_raw_decode_incomplete"
		coverage.Skipped = "raw wakeup-new display-name recovery withheld: source raw decode census incomplete"
		return coverage
	}
	if authority == nil || !authority.initialized || !authority.complete {
		coverage.Metadata["recovery_state"] = "withheld_lifecycle_authority_incomplete"
		coverage.Skipped = "raw wakeup-new display-name recovery withheld: lifecycle authority incomplete"
		return coverage
	}
	if len(inventory.RawWakeupNames) == 0 {
		coverage.Metadata["recovery_state"] = "complete_no_source_names"
		return coverage
	}

	namesByITID := map[int64]map[string]int64{}
	for _, raw := range inventory.RawWakeupNames {
		if raw.TimestampNS > math.MaxInt64 || raw.TargetTID <= 0 ||
			raw.TargetTID > math.MaxInt32 || strings.TrimSpace(raw.Name) == "" ||
			!traceDBSinglePhysicalLine(raw.Name, true) {
			traceDBAddCoverageMetric(&coverage, "raw_name_rows_rejected_invalid", 1)
			continue
		}
		thread, _, reason := traceDBResolveRawPublicTID(
			*authority, raw.TargetTID, int64(raw.TimestampNS))
		if reason != "" {
			traceDBAddCoverageMetric(
				&coverage, "raw_name_rows_rejected_"+reason, 1)
			continue
		}
		current := strings.TrimSpace(authority.threadDisplayName(thread))
		if current != "" {
			if current == raw.Name {
				traceDBAddCoverageMetric(&coverage, "raw_name_rows_existing_same", 1)
			} else {
				traceDBAddCoverageMetric(&coverage, "raw_name_rows_existing_different", 1)
			}
			continue
		}
		if namesByITID[thread.ITID] == nil {
			namesByITID[thread.ITID] = map[string]int64{}
		}
		namesByITID[thread.ITID][raw.Name]++
		traceDBAddCoverageMetric(&coverage, "raw_name_rows_identity_admitted", 1)
	}

	for itid, names := range namesByITID {
		if len(names) != 1 {
			traceDBAddCoverageMetric(&coverage, "canonical_itids_with_name_conflict", 1)
			continue
		}
		var name string
		for candidate := range names {
			name = candidate
		}
		authority.identities.DisplayNameByITID[itid] = name
		authority.identities.DisplayNameSourceByITID[itid] =
			traceDBDisplayNameSourceRawWakeupNew
		coverage.RowsEmitted++
	}
	traceDBAddCoverageMetric(
		&coverage, "thread_names_recovered_source_wakeup_new", int64(coverage.RowsEmitted))
	if coverage.RowsEmitted > 0 {
		coverage.Metadata["recovery_state"] = "published_display_names"
		coverage.Metadata["publication_contract"] = fmt.Sprintf(
			"canonical_itids_named=%d; identity_or_event_changes=0", coverage.RowsEmitted)
	} else {
		coverage.Metadata["recovery_state"] = "complete_no_unresolved_unique_name"
	}
	return coverage
}

func traceDBApplyRawWakeupNameRecoveryToThreadCoverage(
	coverage []TraceDBCoverage,
	authority traceDBSchedulerAuthority,
	recovered int,
) {
	if recovered <= 0 {
		return
	}
	for index := range coverage {
		item := &coverage[index]
		if item.Family != "resolver" || item.Table != "thread" {
			continue
		}
		traceDBAddCoverageMetric(
			item, "thread_names_recovered_source_wakeup_new", int64(recovered))
		traceDBSetThreadUnresolvedNameCoverage(item, authority.identities)
		return
	}
}
