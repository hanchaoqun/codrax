package types

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// TargetWaitOccurrenceAuthority is the answer-side projection of one
// producer-paired, complete, bounded target D/IO-wait roster. This is not a
// census of ordinary S or every wait/blocking mechanism. It is derived only
// from registered trace note keys on a hard-grounded trace_query observation
// that matches a typed user runtime target. Free-form investigation prose and
// aggregate facts never enter this authority.
type TargetWaitOccurrenceAuthority struct {
	RecordID string
	Subject  string
	Count    int
	SumMS    float64
	Rows     []TargetWaitOccurrenceAuthorityRow
}

type TargetWaitOccurrenceAuthorityRow struct {
	Ordinal   int
	State     string
	StartTs   float64
	EndTs     float64
	DurationM float64
	IOWait    string
	Caller    string
}

func (r TargetWaitOccurrenceAuthorityRow) StartToken() string {
	return fmt.Sprintf("%.6f", r.StartTs)
}

func (r TargetWaitOccurrenceAuthorityRow) EndToken() string {
	return fmt.Sprintf("%.6f", r.EndTs)
}

func (r TargetWaitOccurrenceAuthorityRow) DurationToken() string {
	return fmt.Sprintf("%.3fms", r.DurationM)
}

func (r TargetWaitOccurrenceAuthorityRow) CanonicalLine() string {
	return fmt.Sprintf(
		"#%d state=%s %s..%s duration=%s iowait=%s caller=%s",
		r.Ordinal,
		r.State,
		r.StartToken(),
		r.EndToken(),
		r.DurationToken(),
		r.IOWait,
		r.Caller,
	)
}

// BuildTargetWaitOccurrenceAuthorities returns only unambiguous complete
// rosters. Multiple identical observations are deduplicated. If the same
// target subject carries conflicting complete rosters, that subject is
// omitted entirely: ambiguous runtime evidence may guide investigation but
// must never become a hard answer gate.
func BuildTargetWaitOccurrenceAuthorities(ledger ObservationLedger, rm *RequestModel) []TargetWaitOccurrenceAuthority {
	if rm == nil || len(rm.RuntimeTargets) == 0 {
		return nil
	}
	bySubject := map[string]TargetWaitOccurrenceAuthority{}
	fingerprints := map[string]string{}
	conflicted := map[string]bool{}
	for _, record := range ledger.Records {
		if record.Origin != AnswerEvidenceOriginRuntimeArtifact ||
			!RuntimeObservationProducerIsDeterministicQuery(record.Producer) ||
			strings.TrimSpace(record.Predicate) != "target_window_wait_occurrences" ||
			record.GroundingPolicy != ClaimGroundingHard ||
			!ObservationRecordMatchesUserRuntimeTarget(record, rm) {
			continue
		}
		authority, ok := targetWaitOccurrenceAuthorityFromRecord(record)
		if !ok {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(authority.Subject))
		if key == "" {
			continue
		}
		fingerprint := targetWaitOccurrenceAuthorityFingerprint(authority)
		if prior, exists := fingerprints[key]; exists && prior != fingerprint {
			conflicted[key] = true
			continue
		}
		fingerprints[key] = fingerprint
		if _, exists := bySubject[key]; !exists {
			bySubject[key] = authority
		}
	}
	keys := make([]string, 0, len(bySubject))
	for key := range bySubject {
		if !conflicted[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	out := make([]TargetWaitOccurrenceAuthority, 0, len(keys))
	for _, key := range keys {
		out = append(out, bySubject[key])
	}
	return out
}

func targetWaitOccurrenceAuthorityFromRecord(record ObservationRecord) (TargetWaitOccurrenceAuthority, bool) {
	var (
		status  string
		emitted int
		total   int
		sumMS   float64
		hasMeta bool
		hasSum  bool
		rows    []TargetWaitOccurrenceAuthorityRow
	)
	for _, raw := range record.RichNotes {
		note := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(note, TraceNoteKeyTargetWaitOccurrencePrompt+"="):
			payload := strings.TrimPrefix(note, TraceNoteKeyTargetWaitOccurrencePrompt+"=")
			parts := strings.Split(payload, ",")
			if len(parts) != 3 {
				return TargetWaitOccurrenceAuthority{}, false
			}
			values := map[string]string{}
			for _, part := range parts {
				pair := strings.SplitN(strings.TrimSpace(part), "=", 2)
				if len(pair) != 2 {
					return TargetWaitOccurrenceAuthority{}, false
				}
				values[pair[0]] = pair[1]
			}
			var err error
			status = values["status"]
			emitted, err = strconv.Atoi(values["emitted"])
			if err != nil {
				return TargetWaitOccurrenceAuthority{}, false
			}
			total, err = strconv.Atoi(values["total"])
			if err != nil {
				return TargetWaitOccurrenceAuthority{}, false
			}
			hasMeta = true
		case strings.HasPrefix(note, TraceNoteKeyTargetWaitOccurrencePromptSum+"="):
			var err error
			sumMS, err = strconv.ParseFloat(strings.TrimPrefix(note, TraceNoteKeyTargetWaitOccurrencePromptSum+"="), 64)
			if err != nil {
				return TargetWaitOccurrenceAuthority{}, false
			}
			hasSum = true
		case strings.HasPrefix(note, TraceNoteKeyTargetWaitOccurrence+"="):
			row, ok := parseTargetWaitOccurrenceAuthorityRow(strings.TrimPrefix(note, TraceNoteKeyTargetWaitOccurrence+"="))
			if !ok {
				return TargetWaitOccurrenceAuthority{}, false
			}
			rows = append(rows, row)
		}
	}
	if !hasMeta || !hasSum || status != "complete" || emitted != total ||
		total < 0 || total > 8 || len(rows) != total ||
		strings.TrimSpace(record.Object) != "complete" ||
		record.ResultCount == nil || *record.ResultCount != total {
		return TargetWaitOccurrenceAuthority{}, false
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Ordinal < rows[j].Ordinal })
	for i, row := range rows {
		if row.Ordinal != i+1 {
			return TargetWaitOccurrenceAuthority{}, false
		}
	}
	return TargetWaitOccurrenceAuthority{
		RecordID: strings.TrimSpace(record.ID),
		Subject:  strings.TrimSpace(record.Subject),
		Count:    total,
		SumMS:    sumMS,
		Rows:     rows,
	}, true
}

func parseTargetWaitOccurrenceAuthorityRow(raw string) (TargetWaitOccurrenceAuthorityRow, bool) {
	fields := strings.Fields(strings.TrimSpace(raw))
	if len(fields) < 6 || !strings.HasPrefix(fields[0], "#") ||
		!strings.HasPrefix(fields[1], "state=") ||
		!strings.Contains(fields[2], "..") ||
		!strings.HasPrefix(fields[3], "duration=") ||
		!strings.HasPrefix(fields[4], "iowait=") ||
		!strings.HasPrefix(fields[5], "caller=") {
		return TargetWaitOccurrenceAuthorityRow{}, false
	}
	ordinal, err := strconv.Atoi(strings.TrimPrefix(fields[0], "#"))
	if err != nil || ordinal <= 0 {
		return TargetWaitOccurrenceAuthorityRow{}, false
	}
	bounds := strings.SplitN(fields[2], "..", 2)
	if len(bounds) != 2 {
		return TargetWaitOccurrenceAuthorityRow{}, false
	}
	start, err := strconv.ParseFloat(bounds[0], 64)
	if err != nil {
		return TargetWaitOccurrenceAuthorityRow{}, false
	}
	end, err := strconv.ParseFloat(bounds[1], 64)
	if err != nil || end < start {
		return TargetWaitOccurrenceAuthorityRow{}, false
	}
	duration, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimPrefix(fields[3], "duration="), "ms"), 64)
	if err != nil || duration < 0 {
		return TargetWaitOccurrenceAuthorityRow{}, false
	}
	state := strings.TrimPrefix(fields[1], "state=")
	iowait := strings.TrimPrefix(fields[4], "iowait=")
	caller := strings.TrimPrefix(fields[5], "caller=")
	if state == "" || caller == "" || (iowait != "0" && iowait != "1" && iowait != "unknown") {
		return TargetWaitOccurrenceAuthorityRow{}, false
	}
	return TargetWaitOccurrenceAuthorityRow{
		Ordinal:   ordinal,
		State:     state,
		StartTs:   start,
		EndTs:     end,
		DurationM: duration,
		IOWait:    iowait,
		Caller:    caller,
	}, true
}

func targetWaitOccurrenceAuthorityFingerprint(authority TargetWaitOccurrenceAuthority) string {
	parts := []string{strconv.Itoa(authority.Count), fmt.Sprintf("%.3f", authority.SumMS)}
	for _, row := range authority.Rows {
		parts = append(parts, row.CanonicalLine())
	}
	return strings.Join(parts, "\n")
}

type TargetWaitOccurrenceConsistencyIssue struct {
	Authority TargetWaitOccurrenceAuthority
	Missing   []string
	Conflicts []string
}

var targetWaitOccurrenceAnswerIntervalRe = regexp.MustCompile(`([0-9]+\.[0-9]{6})\s*\.\.\s*([0-9]+\.[0-9]{6})`)

// CheckTargetWaitOccurrencePrincipalConsistency activates only when the model
// has begun publishing an authority row (one exact start token is present on
// a model-authored visible surface). Once active, every complete row must be
// present as an exact start/end/duration relation, and an item that pairs an
// authoritative duration with a different interval is a deterministic
// conflict. SurfaceRole is not an authority boundary here: model-authored
// section/table/list blocks render to the user even when the model omits the
// optional role annotation. The unforgeable SystemGeneratedKind marker keeps
// deterministic supplements outside this model-consistency gate.
func CheckTargetWaitOccurrencePrincipalConsistency(authorities []TargetWaitOccurrenceAuthority, doc *AnswerDocumentV2) []TargetWaitOccurrenceConsistencyIssue {
	segments := targetWaitOccurrenceModelClaimSegments(doc)
	if len(segments) == 0 {
		return nil
	}
	var issues []TargetWaitOccurrenceConsistencyIssue
	for _, authority := range authorities {
		if len(authority.Rows) == 0 || !targetWaitOccurrenceAuthorityActivated(authority, segments) {
			continue
		}
		var issue TargetWaitOccurrenceConsistencyIssue
		issue.Authority = authority
		for _, row := range authority.Rows {
			if !targetWaitOccurrenceRowPresent(row, segments) {
				issue.Missing = append(issue.Missing, row.CanonicalLine())
			}
		}
		for _, segment := range segments {
			if conflict := targetWaitOccurrenceSegmentConflict(authority, segment); conflict != "" {
				issue.Conflicts = append(issue.Conflicts, conflict)
			}
		}
		if len(issue.Missing) > 0 || len(issue.Conflicts) > 0 {
			issues = append(issues, issue)
		}
	}
	return issues
}

func targetWaitOccurrenceModelClaimSegments(doc *AnswerDocumentV2) []string {
	if doc == nil {
		return nil
	}
	var out []string
	appendValue := func(value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	for _, block := range doc.Blocks {
		if block.SystemGeneratedKind != AnswerSystemGeneratedBlockUnknown {
			continue
		}
		appendValue(block.Title)
		appendValue(block.Text)
		for _, item := range block.Items {
			var b strings.Builder
			b.WriteString(item.Label)
			b.WriteByte(' ')
			b.WriteString(item.Text)
			for _, cell := range item.Cells {
				b.WriteByte(' ')
				b.WriteString(cell)
			}
			appendValue(b.String())
		}
	}
	return out
}

func targetWaitOccurrenceAuthorityActivated(authority TargetWaitOccurrenceAuthority, segments []string) bool {
	for _, segment := range segments {
		for _, row := range authority.Rows {
			if strings.Contains(segment, row.StartToken()) &&
				strings.Contains(segment, row.DurationToken()) {
				return true
			}
		}
		if targetWaitOccurrenceSegmentHasAuthorityRelationCandidate(authority, segment) {
			return true
		}
	}
	return false
}

func targetWaitOccurrenceRowPresent(row TargetWaitOccurrenceAuthorityRow, segments []string) bool {
	for _, segment := range segments {
		if strings.Contains(segment, row.StartToken()) &&
			strings.Contains(segment, row.EndToken()) &&
			strings.Contains(segment, row.DurationToken()) {
			return true
		}
	}
	return false
}

func targetWaitOccurrenceSegmentConflict(authority TargetWaitOccurrenceAuthority, segment string) string {
	matches := targetWaitOccurrenceAnswerIntervalRe.FindAllStringSubmatch(segment, -1)
	for _, match := range matches {
		if len(match) != 3 {
			continue
		}
		durationMatched := false
		for _, row := range authority.Rows {
			if !strings.Contains(segment, row.DurationToken()) {
				continue
			}
			durationMatched = true
			if match[1] == row.StartToken() && match[2] == row.EndToken() {
				return ""
			}
		}
		if durationMatched {
			return match[1] + ".." + match[2]
		}
	}
	return ""
}

func targetWaitOccurrenceSegmentHasAuthorityRelationCandidate(authority TargetWaitOccurrenceAuthority, segment string) bool {
	if !targetWaitOccurrenceAnswerIntervalRe.MatchString(segment) {
		return false
	}
	for _, row := range authority.Rows {
		if strings.Contains(segment, row.DurationToken()) {
			return true
		}
	}
	return false
}
