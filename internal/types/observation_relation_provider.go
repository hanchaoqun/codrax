package types

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// ObservationRelationCandidateSource adapts the accepted ObservationLedger into
// the shared typed-relation provider boundary. It links exact external
// observations to exact current-checkout source anchors without turning the
// external observation into a current-source citation.
type ObservationRelationCandidateSource struct {
	Ledger ObservationLedger
}

func (s ObservationRelationCandidateSource) TypedRelationCandidates(q TypedRelationQuery) []TypedRelationCandidate {
	if !q.AllowsKind(TypedRelationSourceAnchor) || len(s.Ledger.Records) == 0 {
		return nil
	}
	current, external := observationRelationPartitionRecords(s.Ledger.Records)
	if len(current) == 0 || len(external) == 0 {
		return nil
	}
	var out []TypedRelationCandidate
	seen := map[string]bool{}
	for _, ext := range external {
		if ext.Negative {
			continue
		}
		for _, cur := range current {
			precision, ok := observationSourceAnchorPrecision(ext, cur)
			if !ok {
				continue
			}
			if q.Purpose == TypedRelationPurposeCoverageGate && !precision.CoverageGateEligible() {
				continue
			}
			if !observationRelationMatchesQuerySources(q.Sources, ext, cur) {
				continue
			}
			member := observationRelationCurrentMember(cur)
			if strings.TrimSpace(member.Name) == "" || strings.TrimSpace(member.File) == "" || member.Line <= 0 {
				continue
			}
			source := observationRelationSourceName(ext)
			if source == "" {
				continue
			}
			key := strings.ToLower(source) + "|" + strings.ToLower(member.Name) + "|" + member.File + "|" + fmt.Sprint(member.Line)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, TypedRelationCandidate{
				Relation:   TypedRelationSourceAnchor,
				SourceName: source,
				SourceKind: string(ext.Origin),
				SourceFile: FormatObservationSourceRef(ext.SourceRef, 120),
				SourceLine: observationRelationExternalLine(ext),
				Member:     member,
				Carrier:    TypedRelationCarrierExternalObservation,
				Precision:  precision,
			})
		}
	}
	if len(out) == 0 {
		return nil
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SourceName != out[j].SourceName {
			return out[i].SourceName < out[j].SourceName
		}
		if out[i].Member.File != out[j].Member.File {
			return out[i].Member.File < out[j].Member.File
		}
		if out[i].Member.Line != out[j].Member.Line {
			return out[i].Member.Line < out[j].Member.Line
		}
		return out[i].Member.Name < out[j].Member.Name
	})
	if q.MaxMembers > 0 && len(out) > q.MaxMembers {
		out = out[:q.MaxMembers]
	}
	return out
}

func observationRelationPartitionRecords(records []ObservationRecord) (current []ObservationRecord, external []ObservationRecord) {
	for _, record := range records {
		if record.Origin == AnswerEvidenceOriginCurrentSource {
			if ObservationRecordHasCurrentSourceLineSpan(record) {
				current = append(current, record)
			}
			continue
		}
		if AnswerEvidenceOriginCarriesOriginSpecificSupport(record.Origin) {
			external = append(external, record)
		}
	}
	return current, external
}

func observationSourceAnchorPrecision(ext, cur ObservationRecord) (TypedRelationPrecision, bool) {
	if observationExternalExplicitlyPointsAtCurrentSource(ext, cur) {
		return TypedRelationPrecisionExactEvidence, true
	}
	if observationRecordsHaveSameStableClaim(ext, cur) && observationExternalHasAddress(ext) {
		return TypedRelationPrecisionNameOnly, true
	}
	return "", false
}

func observationExternalExplicitlyPointsAtCurrentSource(ext, cur ObservationRecord) bool {
	if !ObservationRecordHasCurrentSourceLineSpan(cur) {
		return false
	}
	for _, file := range []string{ext.SourceRef.Path, ext.SourceRef.Pathspec, ext.SourceRef.ResourceURI, ext.SourceRef.RawRef} {
		if observationRefPathLineMatchesCurrent(file, ext.Span.LineStart, cur) ||
			observationRefPathLineMatchesCurrent(file, ext.Span.NewLine, cur) {
			return true
		}
	}
	for _, ref := range observationRelationSupportSurfaces(ext) {
		if observationSupportSurfaceMatchesCurrent(ref, cur) {
			return true
		}
	}
	return false
}

func observationRefPathLineMatchesCurrent(file string, line int, cur ObservationRecord) bool {
	if line <= 0 || strings.TrimSpace(file) == "" {
		return false
	}
	if !observationPathMatches(file, cur.SourceRef.Path) {
		return false
	}
	end := cur.Span.LineEnd
	if end <= 0 {
		end = cur.Span.LineStart
	}
	return line >= cur.Span.LineStart && line <= end
}

func observationSupportSurfaceMatchesCurrent(raw string, cur ObservationRecord) bool {
	_, loc, ok := ParseAnswerSupportRefMemberLocation(raw)
	if !ok || loc.File == "" || loc.LineStart <= 0 {
		return false
	}
	if !observationPathMatches(loc.File, cur.SourceRef.Path) {
		return false
	}
	end := loc.LineEnd
	if end <= 0 {
		end = loc.LineStart
	}
	curEnd := cur.Span.LineEnd
	if curEnd <= 0 {
		curEnd = cur.Span.LineStart
	}
	return end >= cur.Span.LineStart && loc.LineStart <= curEnd
}

func observationRecordsHaveSameStableClaim(ext, cur ObservationRecord) bool {
	for _, a := range []string{ext.ClaimKey, ext.Subject, ext.Object, ext.Value} {
		a = observationRelationStableToken(a)
		if a == "" {
			continue
		}
		for _, b := range []string{cur.ClaimKey, cur.Subject, cur.Object, cur.Value} {
			if a == observationRelationStableToken(b) {
				return true
			}
		}
	}
	return false
}

func observationExternalHasAddress(record ObservationRecord) bool {
	ref := record.SourceRef
	return strings.TrimSpace(ref.RawRef) != "" ||
		strings.TrimSpace(ref.PayloadRef) != "" ||
		strings.TrimSpace(ref.RowSetRef) != "" ||
		strings.TrimSpace(ref.PageRef) != "" ||
		strings.TrimSpace(ref.ArtifactID) != "" ||
		strings.TrimSpace(ref.URL) != "" ||
		strings.TrimSpace(ref.ResourceURI) != "" ||
		strings.TrimSpace(ref.Commit) != "" ||
		strings.TrimSpace(ref.Command) != "" ||
		strings.TrimSpace(ref.Path) != ""
}

func observationRelationMatchesQuerySources(sources []string, ext, cur ObservationRecord) bool {
	if len(sources) == 0 {
		return true
	}
	for _, source := range sources {
		source = strings.TrimSpace(source)
		if source == "" {
			continue
		}
		if observationRecordSurfaceMatchesSource(cur, source) || observationRecordSurfaceMatchesSource(ext, source) {
			return true
		}
	}
	return false
}

func observationRecordSurfaceMatchesSource(record ObservationRecord, source string) bool {
	sourceKey := observationRelationStableToken(source)
	if sourceKey == "" {
		return false
	}
	if observationPathMatches(source, record.SourceRef.Path) ||
		observationPathMatches(source, record.SourceRef.Pathspec) ||
		observationPathMatches(source, record.SourceRef.ResourceURI) ||
		observationPathMatches(source, record.SourceRef.URL) {
		return true
	}
	for _, value := range []string{
		record.ClaimKey,
		record.Subject,
		record.Object,
		record.Value,
		record.SourceRef.Commit,
		record.SourceRef.Command,
		record.SourceRef.ArtifactID,
		record.SourceRef.RawRef,
		record.SourceRef.PayloadRef,
		record.SourceRef.RowSetRef,
		record.SourceRef.PageRef,
	} {
		if sourceKey == observationRelationStableToken(value) {
			return true
		}
	}
	return false
}

func observationRelationCurrentMember(record ObservationRecord) TypedRelationMember {
	name := firstNonEmptyString(record.ClaimKey, record.Subject, record.Object, record.SourceRef.Path)
	return TypedRelationMember{
		Name: strings.TrimSpace(name),
		File: strings.TrimSpace(record.SourceRef.Path),
		Line: record.Span.LineStart,
		Kind: firstNonEmptyString(string(record.AnchorKind), string(record.EvidenceKind), "source_anchor"),
	}
}

func observationRelationSourceName(record ObservationRecord) string {
	for _, value := range []string{
		record.Subject,
		record.ClaimKey,
		record.Value,
		record.SourceRef.Commit,
		record.SourceRef.ArtifactID,
		record.SourceRef.ResourceURI,
		record.SourceRef.URL,
		record.SourceRef.Command,
		record.ID,
	} {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func observationRelationExternalLine(record ObservationRecord) int {
	if record.Span.LineStart > 0 {
		return record.Span.LineStart
	}
	if record.Span.NewLine > 0 {
		return record.Span.NewLine
	}
	if record.Span.OldLine > 0 {
		return record.Span.OldLine
	}
	return 0
}

func observationRelationSupportSurfaces(record ObservationRecord) []string {
	var out []string
	out = appendUniqueObservationStrings(out, record.SupportRefs...)
	out = appendUniqueObservationString(out, record.RawExcerpt)
	for _, note := range record.RichNotes {
		out = appendUniqueObservationString(out, note)
	}
	return out
}

func observationPathMatches(a, b string) bool {
	a = observationCleanPathLike(a)
	b = observationCleanPathLike(b)
	if a == "" || b == "" {
		return false
	}
	return a == b || strings.HasSuffix(a, "/"+b) || strings.HasSuffix(b, "/"+a)
}

func observationCleanPathLike(raw string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`))
	raw = strings.Trim(raw, "`'\" ")
	if raw == "" || strings.Contains(raw, "\n") {
		return ""
	}
	raw = strings.TrimPrefix(raw, "file://")
	if idx := strings.LastIndex(raw, "#"); idx > 0 {
		raw = raw[:idx]
	}
	if idx := strings.LastIndex(raw, "?"); idx > 0 {
		raw = raw[:idx]
	}
	if raw == "" || raw == "." || raw == "/" {
		return ""
	}
	cleaned := path.Clean(raw)
	if cleaned == "." {
		return ""
	}
	return strings.TrimPrefix(cleaned, "./")
}

func observationRelationStableToken(raw string) string {
	raw = strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	if raw == "" {
		return ""
	}
	return strings.ToLower(raw)
}
