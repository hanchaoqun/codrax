package types

import (
	"sort"
	"strconv"
	"strings"
)

const readRunReplayAuditSampleLimit = 20

// ReadRunReplayAuditMatchState is a closed comparison result for precise
// snapshot identity fields. Unknown means the field was not comparable; it is
// audit metadata only and must not be treated as a scheduler hard gate.
type ReadRunReplayAuditMatchState string

const (
	ReadRunReplayAuditMatchUnknown  ReadRunReplayAuditMatchState = "unknown"
	ReadRunReplayAuditMatchEqual    ReadRunReplayAuditMatchState = "match"
	ReadRunReplayAuditMatchMismatch ReadRunReplayAuditMatchState = "mismatch"
)

// ReadRunReplayAudit compares typed read-run snapshots and replay transition
// rows. It intentionally stores only structured counts, deltas, and reason
// codes; raw prompt text, model rationale, final-answer prose, rendered retry
// hints, and user wording stay out of this surface.
type ReadRunReplayAudit struct {
	BaseRunID                   string                                 `json:"base_run_id,omitempty"`
	CurrentRunID                string                                 `json:"current_run_id,omitempty"`
	BaseSchemaVersion           int                                    `json:"base_schema_version,omitempty"`
	CurrentSchemaVersion        int                                    `json:"current_schema_version,omitempty"`
	TaskGraphHashMatch          ReadRunReplayAuditMatchState           `json:"task_graph_hash_match,omitempty"`
	RequestHashMatch            ReadRunReplayAuditMatchState           `json:"request_hash_match,omitempty"`
	RepoFingerprintMatch        ReadRunReplayAuditMatchState           `json:"repo_fingerprint_match,omitempty"`
	NodeStatusDeltas            []ReadRunReplayAuditCountDelta         `json:"node_status_deltas,omitempty"`
	ReplayReasonCounts          []ReadRunReplayAuditReasonCount        `json:"replay_reason_counts,omitempty"`
	RequeuedFromRunning         int                                    `json:"requeued_from_running,omitempty"`
	DonePreserved               int                                    `json:"done_preserved,omitempty"`
	DoneRequeuedMissingArtifact int                                    `json:"done_requeued_missing_artifact,omitempty"`
	FailedRequeued              int                                    `json:"failed_requeued,omitempty"`
	FailedExhausted             int                                    `json:"failed_exhausted,omitempty"`
	AcceptedEvidenceDelta       ReadRunReplayAuditSetDelta             `json:"accepted_evidence_delta,omitempty"`
	ReadSetDelta                ReadRunReplayAuditSetDelta             `json:"read_set_delta,omitempty"`
	ReadRangeDelta              ReadRunReplayAuditSetDelta             `json:"read_range_delta,omitempty"`
	FileTotalDelta              ReadRunReplayAuditSetDelta             `json:"file_total_delta,omitempty"`
	NodeArtifactDelta           ReadRunReplayAuditSetDelta             `json:"node_artifact_delta,omitempty"`
	NodeArtifactKindDeltas      []ReadRunReplayAuditCountDelta         `json:"node_artifact_kind_deltas,omitempty"`
	NodeArtifactDirectionDeltas []ReadRunReplayAuditCountDelta         `json:"node_artifact_direction_deltas,omitempty"`
	SourceInventoryDelta        ReadRunReplayAuditSourceInventoryDelta `json:"source_inventory_delta,omitempty"`
	ActiveStateBefore           bool                                   `json:"active_state_before,omitempty"`
	ActiveStateAfter            bool                                   `json:"active_state_after,omitempty"`
}

type ReadRunReplayAuditSetDelta struct {
	BeforeCount    int      `json:"before_count,omitempty"`
	AfterCount     int      `json:"after_count,omitempty"`
	AddedCount     int      `json:"added_count,omitempty"`
	RemovedCount   int      `json:"removed_count,omitempty"`
	AddedSamples   []string `json:"added_samples,omitempty"`
	RemovedSamples []string `json:"removed_samples,omitempty"`
}

type ReadRunReplayAuditCountDelta struct {
	Name   string `json:"name,omitempty"`
	Before int    `json:"before,omitempty"`
	After  int    `json:"after,omitempty"`
	Delta  int    `json:"delta,omitempty"`
}

type ReadRunReplayAuditReasonCount struct {
	ReasonCode ReadRunReplayReasonCode `json:"reason_code,omitempty"`
	Count      int                     `json:"count,omitempty"`
}

type ReadRunReplayAuditSourceInventoryDelta struct {
	ActiveBefore           bool                       `json:"active_before,omitempty"`
	ActiveAfter            bool                       `json:"active_after,omitempty"`
	CompleteBefore         bool                       `json:"complete_before,omitempty"`
	CompleteAfter          bool                       `json:"complete_after,omitempty"`
	SetCountBefore         int                        `json:"set_count_before,omitempty"`
	SetCountAfter          int                        `json:"set_count_after,omitempty"`
	MemberCountBefore      int                        `json:"member_count_before,omitempty"`
	MemberCountAfter       int                        `json:"member_count_after,omitempty"`
	SourceClassCountBefore int                        `json:"source_class_count_before,omitempty"`
	SourceClassCountAfter  int                        `json:"source_class_count_after,omitempty"`
	SourceClassTotalBefore int                        `json:"source_class_total_before,omitempty"`
	SourceClassTotalAfter  int                        `json:"source_class_total_after,omitempty"`
	ScopeDelta             ReadRunReplayAuditSetDelta `json:"scope_delta,omitempty"`
}

func BuildReadRunReplayAudit(base, current *ReadRunSnapshot, transitions []ReadRunReplayTransition) ReadRunReplayAudit {
	var before, after ReadRunSnapshot
	if base != nil {
		before = NormalizeReadRunSnapshot(*base)
	}
	if current != nil {
		after = NormalizeReadRunSnapshot(*current)
	}
	audit := ReadRunReplayAudit{
		BaseRunID:                   before.RunID,
		CurrentRunID:                after.RunID,
		BaseSchemaVersion:           before.SchemaVersion,
		CurrentSchemaVersion:        after.SchemaVersion,
		TaskGraphHashMatch:          readRunReplayAuditStringMatch(before.TaskGraphHash, after.TaskGraphHash),
		RequestHashMatch:            readRunReplayAuditStringMatch(before.RequestHash, after.RequestHash),
		RepoFingerprintMatch:        readRunReplayAuditRepoFingerprintMatch(before.RepoFingerprint, after.RepoFingerprint),
		NodeStatusDeltas:            readRunReplayAuditNodeStatusDeltas(before.NodeStatuses, after.NodeStatuses),
		ReplayReasonCounts:          readRunReplayAuditReasonCounts(transitions),
		AcceptedEvidenceDelta:       readRunReplayAuditSetDelta(readRunReplayAuditAcceptedEvidenceKeys(before.AcceptedEvidence), readRunReplayAuditAcceptedEvidenceKeys(after.AcceptedEvidence)),
		ReadSetDelta:                readRunReplayAuditSetDelta(before.ReadSet, after.ReadSet),
		ReadRangeDelta:              readRunReplayAuditSetDelta(readRunReplayAuditRangeKeys(before.ReadRanges), readRunReplayAuditRangeKeys(after.ReadRanges)),
		FileTotalDelta:              readRunReplayAuditSetDelta(readRunReplayAuditFileTotalKeys(before.FileTotals), readRunReplayAuditFileTotalKeys(after.FileTotals)),
		NodeArtifactDelta:           readRunReplayAuditSetDelta(readRunReplayAuditNodeArtifactKeys(before.NodeArtifacts), readRunReplayAuditNodeArtifactKeys(after.NodeArtifacts)),
		NodeArtifactKindDeltas:      readRunReplayAuditNamedCountDeltas(readRunReplayAuditNodeArtifactKindCounts(before.NodeArtifacts), readRunReplayAuditNodeArtifactKindCounts(after.NodeArtifacts)),
		NodeArtifactDirectionDeltas: readRunReplayAuditNamedCountDeltas(readRunReplayAuditNodeArtifactDirectionCounts(before.NodeArtifacts), readRunReplayAuditNodeArtifactDirectionCounts(after.NodeArtifacts)),
		SourceInventoryDelta:        readRunReplayAuditSourceInventoryDelta(before.SourceInventory, after.SourceInventory),
		ActiveStateBefore:           NormalizeReadRunActiveState(before.ActiveState).IsActive(),
		ActiveStateAfter:            NormalizeReadRunActiveState(after.ActiveState).IsActive(),
	}
	for _, transition := range transitions {
		transition = normalizeReadRunReplayTransition(transition)
		switch {
		case transition.FromStatus == NodeExecRunning && transition.ToStatus == NodeExecRequeued:
			audit.RequeuedFromRunning++
		case transition.FromStatus == NodeExecDone && transition.ToStatus == NodeExecDone:
			audit.DonePreserved++
		case transition.FromStatus == NodeExecDone && transition.ToStatus == NodeExecRequeued && transition.ReasonCode == ReadRunReplayReasonDoneArtifactMissing:
			audit.DoneRequeuedMissingArtifact++
		case transition.FromStatus == NodeExecFailed && transition.ToStatus == NodeExecRequeued:
			audit.FailedRequeued++
		case transition.FromStatus == NodeExecFailed && transition.ToStatus == NodeExecFailed && transition.ReasonCode == ReadRunReplayReasonFailedRetryExhausted:
			audit.FailedExhausted++
		}
	}
	return NormalizeReadRunReplayAudit(audit)
}

func NormalizeReadRunReplayAudit(audit ReadRunReplayAudit) ReadRunReplayAudit {
	audit.BaseRunID = strings.TrimSpace(audit.BaseRunID)
	audit.CurrentRunID = strings.TrimSpace(audit.CurrentRunID)
	audit.TaskGraphHashMatch = normalizeReadRunReplayAuditMatchState(audit.TaskGraphHashMatch)
	audit.RequestHashMatch = normalizeReadRunReplayAuditMatchState(audit.RequestHashMatch)
	audit.RepoFingerprintMatch = normalizeReadRunReplayAuditMatchState(audit.RepoFingerprintMatch)
	audit.NodeStatusDeltas = normalizeReadRunReplayAuditCountDeltas(audit.NodeStatusDeltas)
	audit.ReplayReasonCounts = normalizeReadRunReplayAuditReasonCounts(audit.ReplayReasonCounts)
	audit.AcceptedEvidenceDelta = normalizeReadRunReplayAuditSetDelta(audit.AcceptedEvidenceDelta)
	audit.ReadSetDelta = normalizeReadRunReplayAuditSetDelta(audit.ReadSetDelta)
	audit.ReadRangeDelta = normalizeReadRunReplayAuditSetDelta(audit.ReadRangeDelta)
	audit.FileTotalDelta = normalizeReadRunReplayAuditSetDelta(audit.FileTotalDelta)
	audit.NodeArtifactDelta = normalizeReadRunReplayAuditSetDelta(audit.NodeArtifactDelta)
	audit.NodeArtifactKindDeltas = normalizeReadRunReplayAuditCountDeltas(audit.NodeArtifactKindDeltas)
	audit.NodeArtifactDirectionDeltas = normalizeReadRunReplayAuditCountDeltas(audit.NodeArtifactDirectionDeltas)
	audit.SourceInventoryDelta = normalizeReadRunReplayAuditSourceInventoryDelta(audit.SourceInventoryDelta)
	if audit.BaseSchemaVersion < 0 {
		audit.BaseSchemaVersion = 0
	}
	if audit.CurrentSchemaVersion < 0 {
		audit.CurrentSchemaVersion = 0
	}
	if audit.RequeuedFromRunning < 0 {
		audit.RequeuedFromRunning = 0
	}
	if audit.DonePreserved < 0 {
		audit.DonePreserved = 0
	}
	if audit.DoneRequeuedMissingArtifact < 0 {
		audit.DoneRequeuedMissingArtifact = 0
	}
	if audit.FailedRequeued < 0 {
		audit.FailedRequeued = 0
	}
	if audit.FailedExhausted < 0 {
		audit.FailedExhausted = 0
	}
	return audit
}

func readRunReplayAuditStringMatch(before, after string) ReadRunReplayAuditMatchState {
	before = strings.TrimSpace(before)
	after = strings.TrimSpace(after)
	if before == "" || after == "" {
		return ReadRunReplayAuditMatchUnknown
	}
	if before == after {
		return ReadRunReplayAuditMatchEqual
	}
	return ReadRunReplayAuditMatchMismatch
}

func readRunReplayAuditRepoFingerprintMatch(before, after ReadRunRepoFingerprint) ReadRunReplayAuditMatchState {
	if !ReadRunFingerprintsComparable(before, after) {
		return ReadRunReplayAuditMatchUnknown
	}
	if ReadRunRepoFingerprintsEqual(before, after) {
		return ReadRunReplayAuditMatchEqual
	}
	return ReadRunReplayAuditMatchMismatch
}

func normalizeReadRunReplayAuditMatchState(state ReadRunReplayAuditMatchState) ReadRunReplayAuditMatchState {
	switch state {
	case ReadRunReplayAuditMatchEqual, ReadRunReplayAuditMatchMismatch:
		return state
	default:
		return ReadRunReplayAuditMatchUnknown
	}
}

func readRunReplayAuditNodeStatusDeltas(before, after map[string]NodeExecStatus) []ReadRunReplayAuditCountDelta {
	beforeCounts := map[string]int{}
	for _, status := range cloneNodeExecStatusMap(before) {
		beforeCounts[string(NormalizeNodeExecStatus(status))]++
	}
	afterCounts := map[string]int{}
	for _, status := range cloneNodeExecStatusMap(after) {
		afterCounts[string(NormalizeNodeExecStatus(status))]++
	}
	return readRunReplayAuditNamedCountDeltas(beforeCounts, afterCounts)
}

func readRunReplayAuditReasonCounts(transitions []ReadRunReplayTransition) []ReadRunReplayAuditReasonCount {
	counts := map[ReadRunReplayReasonCode]int{}
	for _, transition := range transitions {
		transition = normalizeReadRunReplayTransition(transition)
		if transition.ReasonCode == "" {
			continue
		}
		counts[transition.ReasonCode]++
	}
	out := make([]ReadRunReplayAuditReasonCount, 0, len(counts))
	for reason, count := range counts {
		if count <= 0 {
			continue
		}
		out = append(out, ReadRunReplayAuditReasonCount{ReasonCode: reason, Count: count})
	}
	return normalizeReadRunReplayAuditReasonCounts(out)
}

func normalizeReadRunReplayAuditReasonCounts(in []ReadRunReplayAuditReasonCount) []ReadRunReplayAuditReasonCount {
	merged := map[ReadRunReplayReasonCode]int{}
	for _, item := range in {
		reason := ReadRunReplayReasonCode(strings.TrimSpace(string(item.ReasonCode)))
		if reason == "" || item.Count <= 0 {
			continue
		}
		merged[reason] += item.Count
	}
	out := make([]ReadRunReplayAuditReasonCount, 0, len(merged))
	for reason, count := range merged {
		out = append(out, ReadRunReplayAuditReasonCount{ReasonCode: reason, Count: count})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].ReasonCode < out[j].ReasonCode
	})
	return out
}

func readRunReplayAuditSetDelta(before, after []string) ReadRunReplayAuditSetDelta {
	before = compactReadRunStrings(before...)
	after = compactReadRunStrings(after...)
	beforeSet := readRunReplayAuditStringSet(before)
	afterSet := readRunReplayAuditStringSet(after)
	var added, removed []string
	for _, value := range after {
		if !beforeSet[value] {
			added = append(added, value)
		}
	}
	for _, value := range before {
		if !afterSet[value] {
			removed = append(removed, value)
		}
	}
	return ReadRunReplayAuditSetDelta{
		BeforeCount:    len(before),
		AfterCount:     len(after),
		AddedCount:     len(added),
		RemovedCount:   len(removed),
		AddedSamples:   limitReadRunReplayAuditSamples(added),
		RemovedSamples: limitReadRunReplayAuditSamples(removed),
	}
}

func normalizeReadRunReplayAuditSetDelta(delta ReadRunReplayAuditSetDelta) ReadRunReplayAuditSetDelta {
	if delta.BeforeCount < 0 {
		delta.BeforeCount = 0
	}
	if delta.AfterCount < 0 {
		delta.AfterCount = 0
	}
	if delta.AddedCount < 0 {
		delta.AddedCount = 0
	}
	if delta.RemovedCount < 0 {
		delta.RemovedCount = 0
	}
	delta.AddedSamples = limitReadRunReplayAuditSamples(delta.AddedSamples)
	delta.RemovedSamples = limitReadRunReplayAuditSamples(delta.RemovedSamples)
	return delta
}

func readRunReplayAuditNamedCountDeltas(before, after map[string]int) []ReadRunReplayAuditCountDelta {
	names := map[string]bool{}
	for name := range before {
		name = strings.TrimSpace(name)
		if name != "" {
			names[name] = true
		}
	}
	for name := range after {
		name = strings.TrimSpace(name)
		if name != "" {
			names[name] = true
		}
	}
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	out := make([]ReadRunReplayAuditCountDelta, 0, len(ordered))
	for _, name := range ordered {
		b := before[name]
		a := after[name]
		out = append(out, ReadRunReplayAuditCountDelta{Name: name, Before: b, After: a, Delta: a - b})
	}
	return normalizeReadRunReplayAuditCountDeltas(out)
}

func normalizeReadRunReplayAuditCountDeltas(in []ReadRunReplayAuditCountDelta) []ReadRunReplayAuditCountDelta {
	merged := map[string]ReadRunReplayAuditCountDelta{}
	for _, item := range in {
		item.Name = strings.TrimSpace(item.Name)
		if item.Name == "" {
			continue
		}
		if item.Before < 0 {
			item.Before = 0
		}
		if item.After < 0 {
			item.After = 0
		}
		item.Delta = item.After - item.Before
		merged[item.Name] = item
	}
	names := make([]string, 0, len(merged))
	for name := range merged {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]ReadRunReplayAuditCountDelta, 0, len(names))
	for _, name := range names {
		out = append(out, merged[name])
	}
	return out
}

func readRunReplayAuditAcceptedEvidenceKeys(refs []AcceptedEvidenceRef) []string {
	refs = cloneAcceptedEvidenceRefs(refs)
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		key := strings.Join([]string{
			ref.ID,
			string(ref.Kind),
			ref.Source,
			strconv.Itoa(ref.LineStart),
			strconv.Itoa(ref.LineEnd),
			ref.Subject,
			ref.OwnerSymbol,
			ref.AnchorSymbol,
			string(ref.SourcePathRole),
		}, "\x00")
		if strings.Trim(key, "\x00") != "" {
			out = append(out, key)
		}
	}
	return compactReadRunStrings(out...)
}

func readRunReplayAuditRangeKeys(ranges map[string][]LineRange) []string {
	ranges = cloneLineRangeMap(ranges)
	var out []string
	for path, items := range ranges {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		for _, item := range mergeLineRanges(items) {
			out = append(out, path+":"+strconv.Itoa(item.Start)+"-"+strconv.Itoa(item.End))
		}
	}
	return compactReadRunStrings(out...)
}

func readRunReplayAuditFileTotalKeys(totals map[string]int) []string {
	totals = cloneIntMap(totals)
	out := make([]string, 0, len(totals))
	for path, total := range totals {
		path = strings.TrimSpace(path)
		if path == "" || total <= 0 {
			continue
		}
		out = append(out, path+":"+strconv.Itoa(total))
	}
	return compactReadRunStrings(out...)
}

func readRunReplayAuditNodeArtifactKeys(records []NodeArtifactRecord) []string {
	records = NormalizeNodeArtifactRecords(records)
	out := make([]string, 0, len(records))
	for _, record := range records {
		if key := NodeArtifactRecordKey(record); key != "" {
			out = append(out, key)
		}
	}
	return compactReadRunStrings(out...)
}

func readRunReplayAuditNodeArtifactKindCounts(records []NodeArtifactRecord) map[string]int {
	counts := map[string]int{}
	for _, record := range NormalizeNodeArtifactRecords(records) {
		kind := string(record.Artifact.Kind)
		if kind != "" {
			counts[kind]++
		}
	}
	return counts
}

func readRunReplayAuditNodeArtifactDirectionCounts(records []NodeArtifactRecord) map[string]int {
	counts := map[string]int{}
	for _, record := range NormalizeNodeArtifactRecords(records) {
		direction := string(record.Direction)
		if direction != "" {
			counts[direction]++
		}
	}
	return counts
}

func readRunReplayAuditSourceInventoryDelta(before, after SourceInventoryObservation) ReadRunReplayAuditSourceInventoryDelta {
	before = CloneSourceInventoryObservation(before)
	after = CloneSourceInventoryObservation(after)
	return normalizeReadRunReplayAuditSourceInventoryDelta(ReadRunReplayAuditSourceInventoryDelta{
		ActiveBefore:           before.IsActive(),
		ActiveAfter:            after.IsActive(),
		CompleteBefore:         before.Complete,
		CompleteAfter:          after.Complete,
		SetCountBefore:         len(before.Sets),
		SetCountAfter:          len(after.Sets),
		MemberCountBefore:      readRunReplayAuditSourceInventoryMemberCount(before),
		MemberCountAfter:       readRunReplayAuditSourceInventoryMemberCount(after),
		SourceClassCountBefore: len(before.SourceClasses),
		SourceClassCountAfter:  len(after.SourceClasses),
		SourceClassTotalBefore: readRunReplayAuditSourceInventoryClassTotal(before),
		SourceClassTotalAfter:  readRunReplayAuditSourceInventoryClassTotal(after),
		ScopeDelta:             readRunReplayAuditSetDelta(before.Scopes, after.Scopes),
	})
}

func normalizeReadRunReplayAuditSourceInventoryDelta(delta ReadRunReplayAuditSourceInventoryDelta) ReadRunReplayAuditSourceInventoryDelta {
	if delta.SetCountBefore < 0 {
		delta.SetCountBefore = 0
	}
	if delta.SetCountAfter < 0 {
		delta.SetCountAfter = 0
	}
	if delta.MemberCountBefore < 0 {
		delta.MemberCountBefore = 0
	}
	if delta.MemberCountAfter < 0 {
		delta.MemberCountAfter = 0
	}
	if delta.SourceClassCountBefore < 0 {
		delta.SourceClassCountBefore = 0
	}
	if delta.SourceClassCountAfter < 0 {
		delta.SourceClassCountAfter = 0
	}
	if delta.SourceClassTotalBefore < 0 {
		delta.SourceClassTotalBefore = 0
	}
	if delta.SourceClassTotalAfter < 0 {
		delta.SourceClassTotalAfter = 0
	}
	delta.ScopeDelta = normalizeReadRunReplayAuditSetDelta(delta.ScopeDelta)
	return delta
}

func readRunReplayAuditSourceInventoryMemberCount(observation SourceInventoryObservation) int {
	total := 0
	for _, set := range observation.Sets {
		total += len(set.Members)
	}
	return total
}

func readRunReplayAuditSourceInventoryClassTotal(observation SourceInventoryObservation) int {
	total := 0
	for _, class := range observation.SourceClasses {
		if class.Count > 0 {
			total += class.Count
		}
	}
	return total
}

func readRunReplayAuditStringSet(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		if value != "" {
			out[value] = true
		}
	}
	return out
}

func limitReadRunReplayAuditSamples(values []string) []string {
	values = compactReadRunStrings(values...)
	if len(values) > readRunReplayAuditSampleLimit {
		values = values[:readRunReplayAuditSampleLimit]
	}
	return values
}
