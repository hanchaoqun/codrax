package types

import (
	"fmt"
	"strings"
)

const (
	writeExplorationMaxListItems    = 32
	writeExplorationMaxEvidenceRefs = 24
	writeExplorationMaxTextLen      = 240
)

// WriteExplorationRequest is the typed, read-only request a write workflow can
// hand to the explorer lane before planning a bounded code-change batch.
type WriteExplorationRequest struct {
	BatchID              string   `json:"batch_id,omitempty"`
	Goal                 string   `json:"goal,omitempty"`
	ExplorationQuestions []string `json:"exploration_questions,omitempty"`
	CandidatePaths       []string `json:"candidate_paths,omitempty"`
	Constraints          []string `json:"constraints,omitempty"`
	MaxRounds            int      `json:"max_rounds,omitempty"`
	EvidenceRequirements []string `json:"evidence_requirements,omitempty"`
}

// WriteExplorationEvidenceRef is a compact, line-backed pointer into evidence
// found during read-only exploration. It is planning context, not a final-answer
// citation channel.
type WriteExplorationEvidenceRef struct {
	ID           string `json:"id,omitempty"`
	Kind         string `json:"kind,omitempty"`
	Source       string `json:"source,omitempty"`
	LineStart    int    `json:"line_start,omitempty"`
	LineEnd      int    `json:"line_end,omitempty"`
	Subject      string `json:"subject,omitempty"`
	OwnerSymbol  string `json:"owner_symbol,omitempty"`
	AnchorSymbol string `json:"anchor_symbol,omitempty"`
	Summary      string `json:"summary,omitempty"`
}

// WriteExplorationHandoff is the compact planner-facing projection of
// read-mode TurnAArtifacts. It deliberately omits raw tool output.
type WriteExplorationHandoff struct {
	BatchID              string                        `json:"batch_id,omitempty"`
	Goal                 string                        `json:"goal,omitempty"`
	ExplorationQuestions []string                      `json:"exploration_questions,omitempty"`
	TargetFiles          []string                      `json:"target_files,omitempty"`
	RelevantSymbols      []string                      `json:"relevant_symbols,omitempty"`
	ExistingPatterns     []string                      `json:"existing_patterns,omitempty"`
	Invariants           []string                      `json:"invariants,omitempty"`
	TestSurface          []string                      `json:"test_surface,omitempty"`
	RiskNotes            []string                      `json:"risk_notes,omitempty"`
	Unknowns             []string                      `json:"unknowns,omitempty"`
	EvidenceRefs         []WriteExplorationEvidenceRef `json:"evidence_refs,omitempty"`
	SourceLocalization   *SourceLocalizationReview     `json:"source_localization,omitempty"`
	ConventionGraph      *ConventionGraph              `json:"convention_graph,omitempty"`
	Confidence           string                        `json:"confidence,omitempty"`
}

// NormalizeWriteExplorationRequest trims and bounds a request without
// inventing missing goals or questions.
func NormalizeWriteExplorationRequest(in WriteExplorationRequest) WriteExplorationRequest {
	in.BatchID = trimWriteExplorationText(in.BatchID)
	in.Goal = trimWriteExplorationText(in.Goal)
	in.ExplorationQuestions = dedupTrimWriteExplorationStrings(in.ExplorationQuestions, writeExplorationMaxListItems)
	in.CandidatePaths = dedupTrimWriteExplorationStrings(in.CandidatePaths, writeExplorationMaxListItems)
	in.Constraints = dedupTrimWriteExplorationStrings(in.Constraints, writeExplorationMaxListItems)
	in.EvidenceRequirements = dedupTrimWriteExplorationStrings(in.EvidenceRequirements, writeExplorationMaxListItems)
	if in.MaxRounds < 0 {
		in.MaxRounds = 0
	}
	return in
}

// NormalizeWriteExplorationHandoff trims and bounds a handoff without changing
// its meaning.
func NormalizeWriteExplorationHandoff(in WriteExplorationHandoff) WriteExplorationHandoff {
	in = normalizeWriteExplorationHandoffCore(in)
	if in.ConventionGraph != nil {
		graph := NormalizeConventionGraph(*in.ConventionGraph)
		if len(graph.Nodes) > 0 {
			if graph.BatchID == "" {
				graph.BatchID = in.BatchID
			}
			if graph.Goal == "" {
				graph.Goal = in.Goal
			}
			in.ConventionGraph = &graph
		} else {
			in.ConventionGraph = nil
		}
	} else {
		graph := conventionGraphFromNormalizedExplorationHandoff(in)
		if len(graph.Nodes) > 0 {
			in.ConventionGraph = &graph
		}
	}
	return in
}

func normalizeWriteExplorationHandoffCore(in WriteExplorationHandoff) WriteExplorationHandoff {
	in.BatchID = trimWriteExplorationText(in.BatchID)
	in.Goal = trimWriteExplorationText(in.Goal)
	in.ExplorationQuestions = dedupTrimWriteExplorationStrings(in.ExplorationQuestions, writeExplorationMaxListItems)
	in.TargetFiles = dedupTrimWriteExplorationStrings(in.TargetFiles, writeExplorationMaxListItems)
	in.RelevantSymbols = dedupTrimWriteExplorationStrings(in.RelevantSymbols, writeExplorationMaxListItems)
	in.ExistingPatterns = dedupTrimWriteExplorationStrings(in.ExistingPatterns, writeExplorationMaxListItems)
	in.Invariants = dedupTrimWriteExplorationStrings(in.Invariants, writeExplorationMaxListItems)
	in.TestSurface = dedupTrimWriteExplorationStrings(in.TestSurface, writeExplorationMaxListItems)
	in.RiskNotes = dedupTrimWriteExplorationStrings(in.RiskNotes, writeExplorationMaxListItems)
	in.Unknowns = dedupTrimWriteExplorationStrings(in.Unknowns, writeExplorationMaxListItems)
	if len(in.EvidenceRefs) > writeExplorationMaxEvidenceRefs {
		in.EvidenceRefs = in.EvidenceRefs[:writeExplorationMaxEvidenceRefs]
	}
	for i := range in.EvidenceRefs {
		in.EvidenceRefs[i] = normalizeWriteExplorationEvidenceRef(in.EvidenceRefs[i])
	}
	in.SourceLocalization = CloneSourceLocalizationReviewPtr(in.SourceLocalization)
	in.Confidence = normalizeWriteExplorationConfidence(in.Confidence)
	return in
}

// WriteExplorationHandoffFromTurnA builds a compact, bounded write-planning
// handoff from the read-mode exploration handoff.
func WriteExplorationHandoffFromTurnA(req WriteExplorationRequest, turnA TurnAArtifacts) WriteExplorationHandoff {
	req = NormalizeWriteExplorationRequest(req)
	out := WriteExplorationHandoff{
		BatchID:              req.BatchID,
		Goal:                 req.Goal,
		ExplorationQuestions: append([]string(nil), req.ExplorationQuestions...),
	}
	addFile := func(path string) {
		out.TargetFiles = append(out.TargetFiles, path)
	}
	addSymbol := func(symbol string) {
		out.RelevantSymbols = append(out.RelevantSymbols, symbol)
	}
	addPattern := func(pattern string) {
		out.ExistingPatterns = append(out.ExistingPatterns, pattern)
	}
	addInvariant := func(inv string) {
		out.Invariants = append(out.Invariants, inv)
	}
	addRisk := func(note string) {
		out.RiskNotes = append(out.RiskNotes, note)
	}
	addUnknown := func(note string) {
		out.Unknowns = append(out.Unknowns, note)
	}

	for _, file := range turnA.ReadFiles {
		addFile(file)
	}
	for _, ev := range turnA.EvidenceItems {
		addFile(ev.Source)
		addSymbol(ev.AnchorSymbol)
		addSymbol(ev.OwnerSymbol)
		addSymbol(ev.Subject)
		if ev.Kind == EvidenceUnresolved || ev.Kind == EvidenceTruncated || ev.GroundingStatus == GroundingUngrounded {
			addUnknown(compactEvidenceSurface(ev))
		}
		if ev.Kind == EvidenceConflict {
			addRisk(compactEvidenceSurface(ev))
		}
		if ev.Kind == EvidenceMechanism || ev.Kind == EvidenceRelationship || ev.Kind == EvidenceRegistration {
			addPattern(compactEvidenceSurface(ev))
		}
		ref := WriteExplorationEvidenceRef{
			ID:           ev.ID,
			Kind:         string(ev.Kind),
			Source:       ev.Source,
			LineStart:    ev.LineStart,
			LineEnd:      ev.LineEnd,
			Subject:      ev.Subject,
			OwnerSymbol:  ev.OwnerSymbol,
			AnchorSymbol: ev.AnchorSymbol,
			Summary:      ev.Summary,
		}
		out.EvidenceRefs = append(out.EvidenceRefs, ref)
	}
	for _, flow := range turnA.FlowFindings {
		for _, p := range flow.Path {
			addSymbol(p)
		}
		for _, source := range flow.Sources {
			addSymbol(source)
		}
		for _, sink := range flow.Sinks {
			addSymbol(sink)
		}
		if flow.UnsupportedReason != "" {
			addUnknown(flow.UnsupportedReason)
		}
	}
	for _, fact := range turnA.AcceptedAggregateFacts {
		if fact.Label != "" || fact.Value != "" {
			addPattern(formatAggregateFactForWriteHandoff(fact))
		}
		for _, ref := range fact.SupportRefs {
			addInvariant(ref)
		}
	}
	for _, note := range turnA.ValidationBoundaryNotes {
		addInvariant(note)
	}
	if turnA.AcceptedClosureReason != "" {
		addInvariant(turnA.AcceptedClosureReason)
	}
	derivedLocalization := SourceLocalizationReviewFromTurnA(turnA.ReadFiles, turnA.EvidenceItems)
	out.SourceLocalization = MergeSourceLocalizationReviews(turnA.SourceLocalization, &derivedLocalization)
	out.TargetFiles = append(out.TargetFiles, req.CandidatePaths...)
	out.Confidence = inferWriteExplorationConfidence(out)
	return NormalizeWriteExplorationHandoff(out)
}

func cloneWriteExplorationRequest(in WriteExplorationRequest) WriteExplorationRequest {
	in.ExplorationQuestions = append([]string(nil), in.ExplorationQuestions...)
	in.CandidatePaths = append([]string(nil), in.CandidatePaths...)
	in.Constraints = append([]string(nil), in.Constraints...)
	in.EvidenceRequirements = append([]string(nil), in.EvidenceRequirements...)
	return in
}

func cloneWriteExplorationHandoff(in WriteExplorationHandoff) WriteExplorationHandoff {
	in.ExplorationQuestions = append([]string(nil), in.ExplorationQuestions...)
	in.TargetFiles = append([]string(nil), in.TargetFiles...)
	in.RelevantSymbols = append([]string(nil), in.RelevantSymbols...)
	in.ExistingPatterns = append([]string(nil), in.ExistingPatterns...)
	in.Invariants = append([]string(nil), in.Invariants...)
	in.TestSurface = append([]string(nil), in.TestSurface...)
	in.RiskNotes = append([]string(nil), in.RiskNotes...)
	in.Unknowns = append([]string(nil), in.Unknowns...)
	in.EvidenceRefs = append([]WriteExplorationEvidenceRef(nil), in.EvidenceRefs...)
	in.SourceLocalization = CloneSourceLocalizationReviewPtr(in.SourceLocalization)
	in.ConventionGraph = cloneConventionGraph(in.ConventionGraph)
	return in
}

func normalizeWriteExplorationEvidenceRef(in WriteExplorationEvidenceRef) WriteExplorationEvidenceRef {
	in.ID = trimWriteExplorationText(in.ID)
	in.Kind = trimWriteExplorationText(in.Kind)
	in.Source = trimWriteExplorationText(in.Source)
	in.Subject = trimWriteExplorationText(in.Subject)
	in.OwnerSymbol = trimWriteExplorationText(in.OwnerSymbol)
	in.AnchorSymbol = trimWriteExplorationText(in.AnchorSymbol)
	in.Summary = trimWriteExplorationText(in.Summary)
	if in.LineStart < 0 {
		in.LineStart = 0
	}
	if in.LineEnd < 0 {
		in.LineEnd = 0
	}
	if in.LineEnd > 0 && in.LineStart > 0 && in.LineEnd < in.LineStart {
		in.LineEnd = in.LineStart
	}
	return in
}

func mergeWriteExplorationEvidenceRef(existing, incoming WriteExplorationEvidenceRef) WriteExplorationEvidenceRef {
	out := normalizeWriteExplorationEvidenceRef(existing)
	incoming = normalizeWriteExplorationEvidenceRef(incoming)
	if out.ID == "" {
		out.ID = incoming.ID
	}
	if out.Kind == "" {
		out.Kind = incoming.Kind
	}
	if out.Source == "" {
		out.Source = incoming.Source
	}
	if out.LineStart == 0 {
		out.LineStart = incoming.LineStart
	}
	if out.LineEnd == 0 {
		out.LineEnd = incoming.LineEnd
	}
	if out.Subject == "" {
		out.Subject = incoming.Subject
	}
	if out.OwnerSymbol == "" {
		out.OwnerSymbol = incoming.OwnerSymbol
	}
	if out.AnchorSymbol == "" {
		out.AnchorSymbol = incoming.AnchorSymbol
	}
	if out.Summary == "" {
		out.Summary = incoming.Summary
	}
	return normalizeWriteExplorationEvidenceRef(out)
}

func normalizeWriteExplorationConfidence(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "high", "medium", "low":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return ""
	}
}

func inferWriteExplorationConfidence(h WriteExplorationHandoff) string {
	switch {
	case len(h.EvidenceRefs) >= 4 && len(h.TargetFiles) > 0:
		return "high"
	case len(h.EvidenceRefs) > 0 || len(h.TargetFiles) > 0 || len(h.ExistingPatterns) > 0:
		return "medium"
	default:
		return "low"
	}
}

func compactEvidenceSurface(ev EvidenceItem) string {
	parts := []string{}
	if ev.Subject != "" {
		parts = append(parts, ev.Subject)
	}
	if ev.Predicate != "" {
		parts = append(parts, ev.Predicate)
	}
	if ev.Object != "" {
		parts = append(parts, ev.Object)
	}
	if ev.AnchorSymbol != "" {
		parts = append(parts, "anchor="+ev.AnchorSymbol)
	}
	if ev.Source != "" {
		loc := ev.Source
		if ev.LineStart > 0 {
			loc += fmt.Sprintf(":%d", ev.LineStart)
		}
		parts = append(parts, loc)
	}
	if ev.Summary != "" {
		parts = append(parts, ev.Summary)
	}
	return trimWriteExplorationText(strings.Join(parts, " — "))
}

func formatAggregateFactForWriteHandoff(f AnswerAggregateFact) string {
	parts := []string{}
	if f.Label != "" {
		parts = append(parts, f.Label)
	}
	if f.Value != "" {
		parts = append(parts, "value="+f.Value)
	}
	if f.Unit != "" {
		parts = append(parts, "unit="+f.Unit)
	}
	if len(f.Members) > 0 {
		members := append([]string(nil), f.Members...)
		if len(members) > 5 {
			members = append(members[:5], fmt.Sprintf("+%d more", len(f.Members)-5))
		}
		parts = append(parts, "members="+strings.Join(members, ", "))
	}
	return trimWriteExplorationText(strings.Join(parts, " — "))
}

func trimWriteExplorationText(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	raw = strings.Join(strings.Fields(raw), " ")
	runes := []rune(raw)
	if len(runes) > writeExplorationMaxTextLen {
		return string(runes[:writeExplorationMaxTextLen]) + "..."
	}
	return raw
}

func dedupTrimWriteExplorationStrings(in []string, limit int) []string {
	if limit <= 0 {
		limit = len(in)
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, value := range in {
		value = trimWriteExplorationText(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
		if len(out) >= limit {
			break
		}
	}
	return out
}
