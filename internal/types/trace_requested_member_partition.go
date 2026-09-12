package types

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// A member partition retains the whole native result, including recursive
// local windows. Measurement domains and local spans never select its member.
type traceRequestedMemberResult struct {
	records     []ObservationRecord
	start, end  float64
	windowKnown bool
	principal   bool
	ordinal     int
}

func traceRequestedMemberTargetMatches(subject string, entities []traceCausalProjectionAnchorEntity) bool {
	for _, entity := range entities {
		if entity.typedLane && traceCausalProjectionAnchorLabelMatchesEntity(subject, entity) {
			return true
		}
	}
	return false
}

// The entire source tuple is a partition key, not an equality/additivity
// proof. Unknown origins stay independent rather than joining by record ID.
func traceRequestedMemberResultKey(record ObservationRecord, position int) string {
	ref := record.SourceRef
	if !traceCausalProjectionTraceQueryRecord(record) || ref.Kind != ObservationSourceRuntimeArtifact ||
		TraceCausalProjectionRecordArtifactIdentity(record) == "" || strings.TrimSpace(ref.QueryScopeID) == "" ||
		(strings.TrimSpace(ref.PayloadRef) == "" && strings.TrimSpace(ref.RawRef) == "") {
		return fmt.Sprintf("unknown:%d", position)
	}
	encoded, err := json.Marshal(ref)
	if err != nil {
		return fmt.Sprintf("unknown:%d", position)
	}
	return string(encoded) + "\x00" + record.Producer + "\x00" + record.ObservedAt
}

func traceRequestedMemberResults(records []ObservationRecord, entities []traceCausalProjectionAnchorEntity, profile *RuntimeArtifactScopeProfile) []traceRequestedMemberResult {
	var groups []traceRequestedMemberResult
	indices := map[string]int{}
	for i, record := range records {
		if !traceCausalProjectionTraceQueryRecord(record) {
			continue
		}
		key := traceRequestedMemberResultKey(record, i)
		index, ok := indices[key]
		if !ok {
			index = len(groups)
			indices[key] = index
			groups = append(groups, traceRequestedMemberResult{ordinal: -1})
		}
		groups[index].records = append(groups[index].records, record)
	}
	for i := range groups {
		group := &groups[i]
		ref := group.records[0].SourceRef
		strongSource := !strings.HasPrefix(traceRequestedMemberResultKey(group.records[0], 0), "unknown:")
		target := ref.QueryTargetThread
		if ref.QueryTargetPID > 0 {
			target = strconv.Itoa(ref.QueryTargetPID)
		}
		targetOK := ref.QueryTargetScope == "thread" && traceRequestedMemberTargetMatches(target, entities)
		fallbackTargetAllowed := targetOK || (ref.QueryTargetPID == 0 && strings.TrimSpace(ref.QueryTargetThread) == "" && (ref.QueryTargetScope == "" || ref.QueryTargetScope == "thread"))
		if ref.QueryWindowKnown {
			group.start, group.end = ref.QueryWindowStartTs, ref.QueryWindowEndTs
			group.windowKnown = traceQueryScopeWindowPresent(group.start, group.end)
		} else if strongSource {
			// Legacy rows may provide the parent ruler only through an exact,
			// same-result target account/chain/frame. Conflicting parent rows do
			// not elect the first or the last, and never borrow a native leaf.
			conflict := false
			for _, row := range group.records {
				if !traceRequestedMemberTargetMatches(row.Subject, entities) {
					continue
				}
				var start, end float64
				var ok bool
				switch strings.TrimSpace(row.Predicate) {
				case "target_window_states", "wakeup_chain":
					start, end, ok = traceCausalProjectionSelectedWindowNote(row.RichNotes)
				case "frame_target_resolution":
					if traceObservationRichNoteValue(row.RichNotes, "window_source") == "query_window" {
						start, end, ok = TraceCausalProjectionParseWindowValue(traceObservationRichNoteValue(row.RichNotes, "window"))
					}
				}
				if !ok {
					continue
				}
				if group.windowKnown && !TraceCausalProjectionPrincipalValueSameWindow(group.start, group.end, start, end) {
					conflict = true
				}
				group.start, group.end, group.windowKnown = start, end, true
				targetOK = fallbackTargetAllowed
			}
			if conflict {
				group.start, group.end, group.windowKnown = 0, 0, false
				targetOK = false
			}
		}
		if group.windowKnown {
			group.ordinal, _ = profile.MatchExplicitTimeWindow(group.start, group.end)
			group.principal = group.ordinal >= 0 && strongSource && targetOK && ref.QueryLineRangeKnown && ref.QueryLineStart == 0 && ref.QueryLineEnd == 0
		}
	}
	// Only uniquely qualified request members get a requested-order seat.
	// Supplementary results keep their own source and stable publication order.
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].principal != groups[j].principal {
			return groups[i].principal
		}
		return groups[i].principal && groups[i].ordinal < groups[j].ordinal
	})
	return groups
}

func (group traceRequestedMemberResult) scope(profile *RuntimeArtifactScopeProfile, start, end float64) TraceQueryWindowScope {
	scope := ResolveTraceQueryWindowScope(profile, start, end)
	if !group.principal || !TraceCausalProjectionPrincipalValueSameWindow(group.start, group.end, start, end) {
		scope.RequestedWindowKnown = false
		scope.RequestedWindowStartTs, scope.RequestedWindowEndTs = 0, 0
		scope.RequestedWindowOrdinal = 0
		if traceQueryScopeWindowPresent(start, end) {
			scope.Role = TraceQueryWindowScopeSupportingExploration
		}
	}
	return scope
}

func traceRequestedMemberProjections(records []ObservationRecord, entities []traceCausalProjectionAnchorEntity, profile *RuntimeArtifactScopeProfile) []TraceCausalProjection {
	var out []TraceCausalProjection
	for _, group := range traceRequestedMemberResults(records, entities, profile) {
		var memberProfile *RuntimeArtifactScopeProfile
		var parentWindow *TraceCausalProjectionQueryWindow
		if group.windowKnown {
			start, end := group.start, group.end
			memberProfile = &RuntimeArtifactScopeProfile{RequestedScope: RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "producer-owned query window"}
			parentWindow = &TraceCausalProjectionQueryWindow{StartTs: start, EndTs: end}
		}
		projection := traceCausalProjectionFromParentResult(group.records, entities, memberProfile, parentWindow)
		if !strings.HasPrefix(traceRequestedMemberResultKey(group.records[0], 0), "unknown:") {
			ref := cloneTraceSchedulerMeasurementOrigin(TraceSchedulerMeasurementOrigin{SourceRef: group.records[0].SourceRef}).SourceRef
			projection.QuerySourceRef = &ref
		}
		// Parent coordinates describe this result; recursive local selected
		// windows do not move the board to a different requested member.
		if group.windowKnown {
			projection.WindowStartTs, projection.WindowEndTs = group.start, group.end
		}
		projection.TargetStateAccount = nil
		var candidates []traceCausalProjectionTargetStateCandidate
		for _, row := range group.records {
			if strings.TrimSpace(row.Predicate) != "target_window_states" || !traceRequestedMemberTargetMatches(row.Subject, entities) {
				continue
			}
			if candidate, ok := traceCausalProjectionTargetStateCandidateFromRecord(row); ok {
				candidates = append(candidates, candidate)
			}
		}
		traceCausalProjectionAttachTargetStateAccount(&projection, candidates)
		projection.WindowScope = group.scope(profile, projection.WindowStartTs, projection.WindowEndTs)
		out = append(out, projection)
	}
	return out
}

// TraceCausalProjectionMatchesRecordSource joins reader context to its own
// compiled parent result. Multi-window input requires the exact source tuple;
// same time, target, capture or native partition alone is not sufficient.
// Legacy projections retain their artifact-only lookup semantics. Callers
// still require one unique matching projection, never first-wins.
func TraceCausalProjectionMatchesRecordSource(projection TraceCausalProjection, record ObservationRecord) bool {
	if projection.WindowScope.RequestedWindowCount > 1 {
		if projection.QuerySourceRef == nil || !traceCausalProjectionTraceQueryRecord(record) {
			return false
		}
		base := record
		base.SourceRef = *projection.QuerySourceRef
		if !TraceRuntimeAccountRecordsSameResult(base, record) {
			return false
		}
		x, err := json.Marshal(projection.QuerySourceRef)
		if err != nil {
			return false
		}
		y, err := json.Marshal(record.SourceRef)
		return err == nil && string(x) == string(y)
	}
	_, _, path := traceCausalProjectionArtifactIdentity(record)
	return path != "" && projection.ArtifactPath != "" && (path == projection.ArtifactPath || traceCausalProjectionSuffixAliasPaths(path, projection.ArtifactPath))
}

// Finite accounts do not need a causal anchor. Enumerate them in their own
// result groups; neither an active A board nor a larger filtered account can
// suppress B. Native local accounts remain supplementary within the parent.
func traceRequestedMemberStateAuthorities(ledger ObservationLedger) []TraceTargetStateScopeAuthority {
	entities := traceCausalProjectionAnchorEntitiesFromLedger(ledger.AnchorUserEntities)
	var projections []TraceCausalProjection
	for _, group := range traceRequestedMemberResults(ledger.Records, entities, ledger.RuntimeArtifactScopeProfile) {
		for _, row := range group.records {
			if strings.TrimSpace(row.Predicate) != "target_window_states" || !traceRequestedMemberTargetMatches(row.Subject, entities) {
				continue
			}
			candidate, ok := traceCausalProjectionTargetStateCandidateFromRecord(row)
			key, label, path := traceCausalProjectionArtifactIdentity(row)
			if !ok || key == "" {
				continue
			}
			account := candidate.Account
			projections = append(projections, TraceCausalProjection{ArtifactPath: path, ArtifactLabel: label, TargetStateAccount: &account,
				WindowScope: group.scope(ledger.RuntimeArtifactScopeProfile, candidate.WindowStart, candidate.WindowEnd)})
		}
	}
	return BuildTraceTargetStateScopeAuthorities(TraceCausalProjectionSet{Projections: projections})
}

func traceRequestedMemberAccountSourceKey(account *TraceCausalProjectionTargetStateAccount) string {
	if account == nil {
		return ""
	}
	encoded, _ := json.Marshal(account.MeasurementOrigins)
	return fmt.Sprintf("%.17g\x00%.17g\x00%s", account.WindowStartTs, account.WindowEndTs, encoded)
}

func traceRequestedMemberEntitiesForRequest(rm *RequestModel) []traceCausalProjectionAnchorEntity {
	if rm == nil {
		return nil
	}
	var entities []traceCausalProjectionAnchorEntity
	for _, target := range rm.RuntimeTargets {
		if target.Kind != RuntimeTargetKindThread {
			continue
		}
		if target.PID > 0 {
			entities = append(entities, traceCausalProjectionAnchorEntity{value: strconv.Itoa(target.PID), typedLane: true})
		} else if strings.TrimSpace(target.Thread) != "" {
			entities = append(entities, traceCausalProjectionAnchorEntity{value: target.Thread, typedLane: true})
		}
	}
	return entities
}

// A reader row must inherit its result's scope, not match an arbitrary leaf
// window against the request list. This private map is reused by role readers.
func traceRequestedMemberRecordScopes(records []ObservationRecord, entities []traceCausalProjectionAnchorEntity, profile *RuntimeArtifactScopeProfile) map[string]TraceQueryWindowScope {
	out := map[string]TraceQueryWindowScope{}
	for _, group := range traceRequestedMemberResults(records, entities, profile) {
		for _, row := range group.records {
			key := traceRequestedMemberResultKey(row, 0)
			if strings.HasPrefix(key, "unknown:") {
				continue
			}
			out[key] = group.scope(profile, group.start, group.end)
		}
	}
	return out
}
