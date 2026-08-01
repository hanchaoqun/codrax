package types

import (
	"sort"
	"strconv"
	"strings"
)

// HistoricalTransitionStatus is the typed ceiling on claims that connect an
// observation from a historical/external lane to the current checkout.  A
// historical fact and a current-source fact can both be true without proving
// that one revision transitioned into the other.
type HistoricalTransitionStatus string

const (
	HistoricalTransitionNotApplicable HistoricalTransitionStatus = "not_applicable"
	HistoricalTransitionUnproven      HistoricalTransitionStatus = "unproven"
)

// HistoricalCurrentDefinition is one exact current-checkout source binding.
// HistoricalPathMatch only says that the same canonical path appears in a
// typed VCS changed-path roster.  It deliberately does not prove symbol
// identity, behavioural continuity, or that an attached runtime artifact was
// produced by an ancestor of the checkout.
type HistoricalCurrentDefinition struct {
	Symbol              string `json:"symbol,omitempty"`
	Path                string `json:"path"`
	LineStart           int    `json:"line_start"`
	LineEnd             int    `json:"line_end,omitempty"`
	HistoricalPathMatch bool   `json:"historical_path_match,omitempty"`
}

// HistoricalCurrentSourceAuthority is a prompt-facing, read-only boundary.
// It is compiled solely from accepted ObservationLedger records and never
// scans the request, model rationale, or rendered answer prose.
type HistoricalCurrentSourceAuthority struct {
	Active                   bool                          `json:"active,omitempty"`
	TransitionStatus         HistoricalTransitionStatus    `json:"transition_status,omitempty"`
	HistoricalOrigins        []AnswerEvidenceOrigin        `json:"historical_origins,omitempty"`
	HistoricalChangedPaths   []string                      `json:"historical_changed_paths,omitempty"`
	CurrentDefinitions       []HistoricalCurrentDefinition `json:"current_definitions,omitempty"`
	CurrentDefinitionTotal   int                           `json:"current_definition_total,omitempty"`
	CurrentDefinitionsCapped bool                          `json:"current_definitions_capped,omitempty"`
	Reason                   string                        `json:"reason,omitempty"`
}

const historicalCurrentDefinitionPromptLimit = 16

// BuildHistoricalCurrentSourceAuthority keeps historical/external
// observations and exact current-checkout definitions in separate seats.  No
// existing producer carries an artifact revision map or a complete behavioural
// transition witness, so the transition ceiling is intentionally unproven.
// A future proven state must be added only alongside a typed producer; it must
// not be inferred from matching prose, filenames, line numbers, or symbols.
func BuildHistoricalCurrentSourceAuthority(ledger ObservationLedger) HistoricalCurrentSourceAuthority {
	var out HistoricalCurrentSourceAuthority
	originSeen := map[AnswerEvidenceOrigin]bool{}
	changedPathSet := map[string]string{}

	for _, record := range ledger.Records {
		switch record.Origin {
		case AnswerEvidenceOriginVCSMetadata, AnswerEvidenceOriginVCSDiff, AnswerEvidenceOriginRuntimeArtifact:
			originSeen[record.Origin] = true
		}
		if record.Origin != AnswerEvidenceOriginVCSDiff ||
			strings.TrimSpace(record.Predicate) != "changed_paths" {
			continue
		}
		for _, raw := range record.SurfaceTerms {
			canonical := canonicalHistoricalCurrentPath(raw)
			if canonical == "" {
				continue
			}
			if _, exists := changedPathSet[canonical]; !exists {
				changedPathSet[canonical] = strings.TrimSpace(raw)
			}
		}
	}
	for _, origin := range []AnswerEvidenceOrigin{
		AnswerEvidenceOriginRuntimeArtifact,
		AnswerEvidenceOriginVCSMetadata,
		AnswerEvidenceOriginVCSDiff,
	} {
		if originSeen[origin] {
			out.HistoricalOrigins = append(out.HistoricalOrigins, origin)
		}
	}
	for _, display := range changedPathSet {
		out.HistoricalChangedPaths = append(out.HistoricalChangedPaths, display)
	}
	sort.Strings(out.HistoricalChangedPaths)

	definitionSeen := map[string]bool{}
	var definitions []HistoricalCurrentDefinition
	for _, record := range ledger.Records {
		if !ObservationRecordHasCurrentSourceLineSpan(record) {
			continue
		}
		path := strings.TrimSpace(record.SourceRef.Path)
		canonicalPath := canonicalHistoricalCurrentPath(path)
		if canonicalPath == "" {
			continue
		}
		// When an exact VCS changed-path universe is available, definitions
		// outside it are current-source context but cannot align this
		// historical change.  Excluding them keeps the authority compact and
		// prevents unrelated repository-map rows from crowding out the changed
		// files the user asked about.
		_, historicalPathMatch := changedPathSet[canonicalPath]
		if len(changedPathSet) > 0 && !historicalPathMatch {
			continue
		}
		symbol := strings.TrimSpace(record.ClaimKey)
		if symbol == "" {
			symbol = strings.TrimSpace(record.Subject)
		}
		lineEnd := record.Span.LineEnd
		if lineEnd < record.Span.LineStart {
			lineEnd = record.Span.LineStart
		}
		key := canonicalPath + "\x00" + symbol + "\x00" + strconv.Itoa(record.Span.LineStart)
		if definitionSeen[key] {
			continue
		}
		definitionSeen[key] = true
		definitions = append(definitions, HistoricalCurrentDefinition{
			Symbol:              symbol,
			Path:                path,
			LineStart:           record.Span.LineStart,
			LineEnd:             lineEnd,
			HistoricalPathMatch: historicalPathMatch,
		})
	}
	sort.SliceStable(definitions, func(i, j int) bool {
		if definitions[i].HistoricalPathMatch != definitions[j].HistoricalPathMatch {
			return definitions[i].HistoricalPathMatch
		}
		if definitions[i].Path != definitions[j].Path {
			return definitions[i].Path < definitions[j].Path
		}
		if definitions[i].LineStart != definitions[j].LineStart {
			return definitions[i].LineStart < definitions[j].LineStart
		}
		return definitions[i].Symbol < definitions[j].Symbol
	})
	out.CurrentDefinitionTotal = len(definitions)
	if len(definitions) > historicalCurrentDefinitionPromptLimit {
		out.CurrentDefinitionsCapped = true
		definitions = definitions[:historicalCurrentDefinitionPromptLimit]
	}
	out.CurrentDefinitions = definitions
	out.Active = len(out.HistoricalOrigins) > 0 && out.CurrentDefinitionTotal > 0
	if !out.Active {
		out.TransitionStatus = HistoricalTransitionNotApplicable
		return out
	}
	out.TransitionStatus = HistoricalTransitionUnproven
	out.Reason = "no_typed_revision_mapping_or_behavioral_transition_witness"
	return out
}

func canonicalHistoricalCurrentPath(raw string) string {
	p := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	for strings.HasPrefix(p, "./") {
		p = strings.TrimPrefix(p, "./")
	}
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	return strings.TrimPrefix(p, "/")
}
