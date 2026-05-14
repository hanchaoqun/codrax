package types

import "strings"

// ErrorGranularityVerdict is the canonical answer-surface enum for questions
// about how failures propagate across a batch, call, transaction, or record
// set. It is evidence-derived by the finalizer and carried on a decision block;
// validators must not infer this from rendered prose.
type ErrorGranularityVerdict string

const (
	ErrorGranularityUnknown           ErrorGranularityVerdict = ""
	ErrorGranularityPerItemRejection  ErrorGranularityVerdict = "per_item_rejection"
	ErrorGranularityWholeBatch        ErrorGranularityVerdict = "whole_batch_failure"
	ErrorGranularityPartialSuccess    ErrorGranularityVerdict = "partial_success"
	ErrorGranularityFailFast          ErrorGranularityVerdict = "fail_fast"
	ErrorGranularityCollectErrors     ErrorGranularityVerdict = "collect_errors"
	ErrorGranularityNotEnoughEvidence ErrorGranularityVerdict = "not_enough_evidence"
)

func AllErrorGranularityVerdicts() []ErrorGranularityVerdict {
	return []ErrorGranularityVerdict{
		ErrorGranularityPerItemRejection,
		ErrorGranularityWholeBatch,
		ErrorGranularityPartialSuccess,
		ErrorGranularityFailFast,
		ErrorGranularityCollectErrors,
		ErrorGranularityNotEnoughEvidence,
	}
}

func (v ErrorGranularityVerdict) IsValid() bool {
	if v == ErrorGranularityUnknown {
		return true
	}
	for _, declared := range AllErrorGranularityVerdicts() {
		if v == declared {
			return true
		}
	}
	return false
}

func NormalizeErrorGranularityVerdict(raw string) (ErrorGranularityVerdict, bool) {
	verdict := ErrorGranularityVerdict(strings.TrimSpace(raw))
	if !verdict.IsValid() {
		return ErrorGranularityUnknown, false
	}
	return verdict, true
}

// ErrorGranularityProfile is the analyzer-emitted typed lane for requests that
// ask whether a failure is scoped to one item/record, the whole batch/call, a
// fail-fast stop, a partial-success path, or an error-collection path. The
// profile says a canonical verdict is required; it does not predict the verdict
// before evidence is gathered.
type ErrorGranularityProfile struct {
	IsGranularityQuestion   bool                      `json:"is_granularity_question"`
	RequestedVerdictOptions []ErrorGranularityVerdict `json:"requested_verdict_options,omitempty"`
	SourceQuotes            []string                  `json:"source_quotes,omitempty"`
	Confidence              float64                   `json:"confidence,omitempty"`
	Rationale               string                    `json:"rationale,omitempty"`
}

func (p *ErrorGranularityProfile) Active() bool {
	return p != nil && p.IsGranularityQuestion
}

// MissingErrorGranularityVerdict reports whether a required error-granularity
// answer lacks a typed decision verdict. It reads only typed block fields.
func MissingErrorGranularityVerdict(doc *AnswerDocumentV2, profile *ErrorGranularityProfile) bool {
	if doc == nil || profile == nil || !profile.Active() {
		return false
	}
	for _, block := range doc.Blocks {
		if block.Kind != BlockDecision || block.SurfaceRole != SurfacePrincipal {
			continue
		}
		if block.ErrorGranularityVerdict.IsValid() && block.ErrorGranularityVerdict != ErrorGranularityUnknown {
			return false
		}
	}
	return true
}

func ErrorGranularityVerdictOptionMismatch(doc *AnswerDocumentV2, profile *ErrorGranularityProfile) (ErrorGranularityVerdict, bool) {
	if doc == nil || profile == nil || !profile.Active() || len(profile.RequestedVerdictOptions) == 0 {
		return ErrorGranularityUnknown, false
	}
	for _, block := range doc.Blocks {
		if block.Kind != BlockDecision || block.SurfaceRole != SurfacePrincipal {
			continue
		}
		verdict := block.ErrorGranularityVerdict
		if !verdict.IsValid() || verdict == ErrorGranularityUnknown {
			continue
		}
		if verdict == ErrorGranularityNotEnoughEvidence {
			return ErrorGranularityUnknown, false
		}
		for _, allowed := range profile.RequestedVerdictOptions {
			if verdict == allowed {
				return ErrorGranularityUnknown, false
			}
		}
		return verdict, true
	}
	return ErrorGranularityUnknown, false
}
