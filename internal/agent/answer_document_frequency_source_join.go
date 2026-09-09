package agent

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// This is a display join, not target-slice/policy binding. Both finalizer
// surfaces consume these same rows rather than independently picking a first
// policy or a last target measurement. Unaddressed facts remain available in
// their original standalone fact surfaces, but cannot corroborate one another.
type answerDocFrequencySourceJoinRow struct {
	subject, source, window, running, unit, roster string
	cpu                                            int
	targetObserved, rosterComplete                 bool
	frequencies                                    map[int64]bool
	representative                                 *answerDocTargetFrequencyRepresentative
	policy                                         *types.TraceFrequencyLimitAuthority
	policyStatus, binding                          string
}

type answerDocFrequencySourceJoinGroup struct {
	key, subject string
	record       types.ObservationRecord
	scope        types.TraceRuntimeAccountScope
	targets      map[int][]types.ObservationRecord
	frequencies  map[int]map[int64]bool
	seen         map[string]bool
}

func answerDocFrequencyRecordSourceWitness(r types.ObservationRecord) types.TraceFrequencyLimitAuthority {
	scope := types.TraceRuntimeAccountRecordScope(r)
	return types.TraceFrequencyLimitAuthority{SourceRef: &r.SourceRef, ObservedAt: r.ObservedAt,
		WindowStartTs: scope.WindowStartTs, WindowEndTs: scope.WindowEndTs}
}

func answerDocFrequencyWitnessSource(w types.TraceFrequencyLimitAuthority, lang ...string) string {
	if w.SourceRef != nil && strings.TrimSpace(w.SourceRef.Path) != "" {
		return strings.TrimSpace(w.SourceRef.Path)
	}
	if len(lang) > 0 && strings.HasPrefix(strings.ToLower(lang[0]), "zh") {
		return "来源未明确"
	}
	return "source not stated"
}

func answerDocFrequencyJoinCPU(r types.ObservationRecord) (int, bool) {
	text := ""
	switch strings.TrimSpace(r.Predicate) {
	case "target_cpu_running":
		text = strings.TrimSpace(traceDecisionRichNoteValue(r.RichNotes, types.TraceNoteKeyTargetCPURunningCPU))
		if text == "" && strings.HasPrefix(strings.TrimSpace(r.Object), "cpu=") {
			text = strings.TrimPrefix(strings.TrimSpace(r.Object), "cpu=")
		}
	case "running_time":
		text = strings.TrimSpace(traceDecisionRichNoteValue(r.RichNotes, "cpu"))
	default:
		return 0, false
	}
	cpu, err := strconv.Atoi(text)
	return cpu, err == nil && cpu >= 0
}

func answerDocFrequencySourceJoinRows(ctx *types.AgentContext, published []types.TraceFrequencyLimitAuthority) []answerDocFrequencySourceJoinRow {
	if ctx == nil {
		return nil
	}
	// Completeness/uniqueness is checked before the existing policy display
	// cap. An unshown conflicting row must not make the shown row unique.
	all := append([]types.TraceFrequencyLimitAuthority(nil), published...)
	input := types.ObservationLedgerInputFromAgentContext(ctx, 1)
	for _, result := range append(append([]types.ToolResult(nil), input.ToolResults...), input.SystemTraceSupplementResults...) {
		if result.TraceEvidenceAuthority != nil {
			all = append(all, result.TraceEvidenceAuthority.FrequencyLimitWitnesses...)
		}
	}
	all = types.DedupTraceFrequencyLimitAuthorities(all, len(all))
	ledger := answerDocObservationLedger(ctx)
	groups := map[string]*answerDocFrequencySourceJoinGroup{}
	for _, r := range ledger.Records {
		if r.Origin != types.AnswerEvidenceOriginRuntimeArtifact || !types.RuntimeObservationProducerIsDeterministicQuery(r.Producer) || r.Predicate != "target_cpu_running" || strings.TrimSpace(r.Subject) == "" {
			continue
		}
		cpu, ok := answerDocFrequencyJoinCPU(r)
		if !ok {
			continue
		}
		sourceKey := types.TraceFrequencyLimitSourceKey(answerDocFrequencyRecordSourceWitness(r))
		if sourceKey == "" {
			continue
		}
		key := sourceKey + "\x00" + strings.TrimSpace(r.Subject)
		g := groups[key]
		if g == nil {
			g = &answerDocFrequencySourceJoinGroup{key: key, subject: strings.TrimSpace(r.Subject), record: r,
				scope: types.TraceRuntimeAccountRecordScope(r), targets: map[int][]types.ObservationRecord{}, frequencies: map[int]map[int64]bool{}, seen: map[string]bool{}}
			groups[key] = g
		}
		// Different values/statuses remain independent, even when a producer
		// reused a record ID. Equal display facts within one complete receipt
		// may repeat through multiple handoff lanes without doubling the row.
		fact, _ := json.Marshal([]any{cpu, r.Value, r.Unit, r.Object, r.RichNotes})
		if g.seen[string(fact)] {
			continue
		}
		g.seen[string(fact)] = true
		g.targets[cpu] = append(g.targets[cpu], r)
	}
	for _, r := range ledger.Records {
		if r.Origin != types.AnswerEvidenceOriginRuntimeArtifact || !types.RuntimeObservationProducerIsDeterministicQuery(r.Producer) || r.Predicate != "running_time" {
			continue
		}
		cpu, ok := answerDocFrequencyJoinCPU(r)
		if !ok {
			continue
		}
		freq, err := strconv.ParseInt(strings.TrimSpace(traceDecisionRichNoteValue(r.RichNotes, "freq")), 10, 64)
		if err != nil || freq <= 0 {
			continue
		}
		key := types.TraceFrequencyLimitSourceKey(answerDocFrequencyRecordSourceWitness(r))
		if key == "" {
			continue
		}
		g := groups[key+"\x00"+strings.TrimSpace(r.Subject)]
		if g == nil || !types.TraceFrequencyLimitMatchesRecord(answerDocFrequencyRecordSourceWitness(g.record), r) {
			continue
		}
		if g.frequencies[cpu] == nil {
			g.frequencies[cpu] = map[int64]bool{}
		}
		g.frequencies[cpu][freq] = true
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	// Optional call/capture enrichment may let a source witness match more
	// than one fully identified ledger group. Preserve that compatibility
	// for a single consistent result, but never use it to choose among
	// contradictory target values or two proven physical captures.
	ambiguous := map[string]map[string]bool{}
	for _, w := range all {
		wkey := types.TraceFrequencyLimitSourceKey(w) + fmt.Sprintf("\x00%d", w.CPU)
		if _, done := ambiguous[wkey]; done {
			continue
		}
		bad, first, captures := map[string]bool{}, map[string]string{}, map[string]bool{}
		for _, g := range groups {
			if !types.TraceFrequencyLimitMatchesRecord(w, g.record) {
				continue
			}
			if g.record.SourceRef.CaptureIdentityPath != "" {
				captures[g.scope.ArtifactKey] = true
			}
			for _, r := range g.targets[w.CPU] {
				fact, _ := json.Marshal([]any{r.Value, r.Unit, g.scope.WindowStartTs, g.scope.WindowEndTs,
					strings.TrimSpace(traceDecisionRichNoteValue(r.RichNotes, types.TraceNoteKeyTargetCPURunningRosterStatus))})
				if prev, seen := first[g.subject]; seen && prev != string(fact) {
					bad[g.subject] = true
				}
				first[g.subject] = string(fact)
			}
		}
		if len(captures) > 1 {
			bad[""] = true
		}
		ambiguous[wkey] = bad
	}
	var rows []answerDocFrequencySourceJoinRow
	for _, key := range keys {
		g := groups[key]
		visibleCPU := map[int]bool{}
		for _, w := range published {
			if w.CPU >= 0 && types.TraceFrequencyLimitMatchesRecord(w, g.record) {
				visibleCPU[w.CPU] = true
			}
		}
		// Full-target measurements do not require policy-limit events. Keep
		// the legacy policy-only entry unchanged, but let a valid new carrier
		// publish its own roster without borrowing another result's policy.
		hasTargetFrequency := false
		for cpu, targets := range g.targets {
			for _, target := range targets {
				hasTargetFrequency = hasTargetFrequency || answerDocTargetFrequencyFromRecord(target, cpu) != nil
			}
		}
		if len(visibleCPU) == 0 && !hasTargetFrequency {
			continue
		}
		policies := map[int][]types.TraceFrequencyLimitAuthority{}
		ambiguousCPU := map[int]bool{}
		for _, w := range all {
			if w.CPU >= 0 && types.TraceFrequencyLimitMatchesRecord(w, g.record) {
				policies[w.CPU] = append(policies[w.CPU], w)
				bad := ambiguous[types.TraceFrequencyLimitSourceKey(w)+fmt.Sprintf("\x00%d", w.CPU)]
				ambiguousCPU[w.CPU] = ambiguousCPU[w.CPU] || bad[""] || bad[g.subject]
			}
		}
		complete := true
		cpuset := map[int]bool{}
		for cpu := range visibleCPU {
			cpuset[cpu] = true
		}
		for cpu, targets := range g.targets {
			cpuset[cpu] = true
			if len(targets) != 1 || !strings.EqualFold(strings.TrimSpace(traceDecisionRichNoteValue(targets[0].RichNotes, types.TraceNoteKeyTargetCPURunningRosterStatus)), "complete") {
				complete = false
			}
		}
		cpus := make([]int, 0, len(cpuset))
		for cpu := range cpuset {
			cpus = append(cpus, cpu)
		}
		sort.Ints(cpus)
		for _, cpu := range cpus {
			targets := g.targets[cpu]
			// Sort conflicting facts for stable disclosure, never to select a
			// winner. Every different original measurement is still displayed.
			sort.SliceStable(targets, func(i, j int) bool {
				a, _ := json.Marshal(targets[i])
				b, _ := json.Marshal(targets[j])
				return string(a) < string(b)
			})
			if len(targets) == 0 {
				targets = []types.ObservationRecord{{}}
			}
			for _, target := range targets {
				row := answerDocFrequencySourceJoinRow{subject: g.subject, source: g.record.SourceRef.Path,
					window: types.FormatTraceRuntimeAccountWindow(g.scope.WindowStartTs, g.scope.WindowEndTs, "en"), cpu: cpu,
					targetObserved: len(g.targets[cpu]) > 0, rosterComplete: complete, frequencies: g.frequencies[cpu],
					running: strings.TrimSpace(target.Value), unit: strings.TrimSpace(target.Unit),
					roster:       strings.TrimSpace(traceDecisionRichNoteValue(target.RichNotes, types.TraceNoteKeyTargetCPURunningRosterStatus)),
					policyStatus: "not_observed_in_same_result", binding: "not_comparable_missing_same_cpu_pair"}
				row.representative = answerDocTargetFrequencyFromRecord(target, cpu)
				switch {
				case len(g.targets[cpu]) > 1 || len(policies[cpu]) > 1 || ambiguousCPU[cpu]:
					row.policyStatus, row.binding = "ambiguous_same_result_records", "not_comparable_ambiguous_source_rows"
				case len(policies[cpu]) == 1 && !visibleCPU[cpu]:
					row.policyStatus = "not_emitted_in_bounded_policy_preview"
				case len(policies[cpu]) == 1:
					p := policies[cpu][0]
					row.policy = &p
					row.policyStatus = "present"
					if row.targetObserved {
						row.binding = "target_effect_unproven_no_slice_binding"
					}
				}
				rows = append(rows, row)
			}
		}
	}
	return rows
}
