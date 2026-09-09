package types

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// TraceRuntimeAccountScope describes a producer's query, not a member event
// envelope. Missing identity remains unknown and cannot join two accounts.
type TraceRuntimeAccountScope struct {
	ArtifactKey, ArtifactLabel, Subject string
	WindowStartTs, WindowEndTs          float64
	WindowKnown                         bool
}

// FormatTraceRuntimeAccountWindow formats only a producer-owned query ruler;
// absent endpoints must not render as a fictitious zero-length window.
func FormatTraceRuntimeAccountWindow(start, end float64, lang string) string {
	if traceQueryScopeWindowPresent(start, end) {
		return fmt.Sprintf("%.6f..%.6f", start, end)
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(lang)), "zh") {
		return "查询范围未明确"
	}
	return "query window not stated"
}

func TraceRuntimeAccountRecordScope(record ObservationRecord) TraceRuntimeAccountScope {
	key, label := TraceCausalProjectionRecordArtifactIdentityWithLabel(record)
	start, end, known := TraceCausalProjectionSelectedWindowNote(record.RichNotes)
	known = known && traceQueryScopeWindowPresent(start, end)
	if !known {
		start, end = 0, 0
	}
	return TraceRuntimeAccountScope{ArtifactKey: key, ArtifactLabel: label, Subject: strings.TrimSpace(record.Subject), WindowStartTs: start, WindowEndTs: end, WindowKnown: known}
}

func (scope TraceRuntimeAccountScope) Complete() bool {
	return scope.ArtifactKey != "" && scope.Subject != "" && scope.WindowKnown && traceQueryScopeWindowPresent(scope.WindowStartTs, scope.WindowEndTs)
}

func (scope TraceRuntimeAccountScope) SameQuery(other TraceRuntimeAccountScope) bool {
	return scope.Complete() && other.Complete() && scope.SameTarget(other) &&
		TraceCausalProjectionPrincipalValueSameWindow(scope.WindowStartTs, scope.WindowEndTs, other.WindowStartTs, other.WindowEndTs)
}

func (scope TraceRuntimeAccountScope) SameTarget(other TraceRuntimeAccountScope) bool {
	return scope.ArtifactKey != "" && scope.ArtifactKey == other.ArtifactKey &&
		strings.TrimSpace(scope.Subject) != "" && strings.TrimSpace(scope.Subject) == strings.TrimSpace(other.Subject)
}

// TraceRuntimeAccountRecordsSameResult requires actual result provenance.
// A basename, producer lane marker, or a record-ID prefix alone is not enough.
// Query/target equality is a separate condition (a leaf has its own span).
func TraceRuntimeAccountRecordsSameResult(a, b ObservationRecord) bool {
	x, y := a.SourceRef, b.SourceRef
	key := TraceCausalProjectionRecordArtifactIdentity(a)
	if key == "" || key != TraceCausalProjectionRecordArtifactIdentity(b) ||
		x.Kind != ObservationSourceRuntimeArtifact || x.Kind != y.Kind ||
		strings.TrimSpace(a.Producer) != strings.TrimSpace(b.Producer) ||
		strings.TrimSpace(x.Path) != strings.TrimSpace(y.Path) ||
		strings.TrimSpace(x.PayloadRef) != strings.TrimSpace(y.PayloadRef) ||
		strings.TrimSpace(x.RawRef) != strings.TrimSpace(y.RawRef) ||
		x.QueryScopeID != y.QueryScopeID ||
		(strings.TrimSpace(x.PayloadRef) == "" && strings.TrimSpace(x.RawRef) == "") {
		return false
	}
	if strings.TrimSpace(x.ToolCallID) != "" && strings.TrimSpace(y.ToolCallID) != "" && x.ToolCallID != y.ToolCallID {
		return false
	}
	aa, ba := strings.TrimSpace(a.ObservedAt), strings.TrimSpace(b.ObservedAt)
	return aa == "" || ba == "" || aa == ba
}

func traceRuntimeAccountScopeKey(scope TraceRuntimeAccountScope, recordID string, position int) string {
	if !scope.Complete() {
		// Independent unknown records stay readable, but cannot corroborate or
		// invalidate another result merely by sharing a subject or zero value.
		return fmt.Sprintf("unknown\x00%s\x00%d", recordID, position)
	}
	return strings.Join([]string{scope.ArtifactKey, scope.Subject,
		strconv.FormatFloat(scope.WindowStartTs, 'g', -1, 64), strconv.FormatFloat(scope.WindowEndTs, 'g', -1, 64)}, "\x00")
}

func traceRuntimeAccountMergeRecordIDs(a, b []string) []string {
	seen := traceRuntimeAccountRecordIDSet{}
	seen.add(a)
	seen.add(b)
	return seen.sorted()
}

// Accumulate equivalent-source receipts once, then sort at publication. A
// growing duplicate-query group must not repeatedly copy all earlier IDs.
type traceRuntimeAccountRecordIDSet map[string]struct{}

func (seen traceRuntimeAccountRecordIDSet) add(ids []string) {
	for _, id := range ids {
		if id = strings.TrimSpace(id); id != "" {
			seen[id] = struct{}{}
		}
	}
}

func (seen traceRuntimeAccountRecordIDSet) sorted() []string {
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// IDs can collide across exports/captures. Such an ID must never authorize
// reader shadowing; the individual scoped values remain available.
func traceRuntimeAccountUnambiguousRecordIDs(records []ObservationRecord) map[string]bool {
	first := map[string]ObservationRecord{}
	ok := map[string]bool{}
	for _, record := range records {
		id := strings.TrimSpace(record.ID)
		if id == "" {
			continue
		}
		if prior, exists := first[id]; exists {
			if !reflect.DeepEqual(prior, record) {
				ok[id] = false
			}
		} else {
			first[id], ok[id] = record, true
		}
	}
	return ok
}

func traceRuntimeAccountSafeRecordIDs(ids []string, safe map[string]bool) []string {
	var out []string
	for _, id := range traceRuntimeAccountMergeRecordIDs(nil, ids) {
		if safe[id] {
			out = append(out, id)
		}
	}
	return out
}

func traceRuntimeAccountRequestedProfile(ledger ObservationLedger, rm *RequestModel) *RuntimeArtifactScopeProfile {
	if ledger.RuntimeArtifactScopeProfile != nil {
		return ledger.RuntimeArtifactScopeProfile
	}
	if rm != nil {
		return rm.RuntimeArtifactScopeProfile
	}
	return nil
}
