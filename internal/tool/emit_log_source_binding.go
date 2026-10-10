package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/loginput"
	"github.com/hanchaoqun/codrax/internal/types"
)

// logTriageSourceProof is produced for one emission only. It never uses model
// line numbers as lookup authority and is not stored as a replayable permission.
type logTriageSourceProof map[loginput.ExcerptSelector]loginput.ExcerptLocation

func prepareLogTriageSourceProof(ctx *types.BusContext, p *emitLogTriageParams) (logTriageSourceProof, error) {
	if err := ctx.Context().Err(); err != nil {
		return nil, err
	}
	proof := logTriageSourceProof{}
	var selectors []loginput.ExcerptSelector
	add := func(text, source string) {
		s := loginput.ExcerptSelector{Text: strings.TrimSpace(text), SourceID: source}
		if _, exists := proof[s]; !exists {
			proof[s] = loginput.ExcerptLocation{Status: "preview_only"}
			selectors = append(selectors, s)
		}
	}
	var walk func([]emitLogTriageError)
	walk = func(errors []emitLogTriageError) {
		for i := range errors {
			e := &errors[i]
			add(e.Message, e.SourceID)
			// Whole-material cardinality also prevents mixing selected and
			// unselected references to count the same error occurrence twice.
			if e.SourceID != "" {
				add(e.Message, "")
			}
			if e.CauseRelation != nil {
				add(e.CauseRelation.Marker, e.SourceID)
			}
			if e.Cause != nil {
				walk([]emitLogTriageError{*e.Cause})
			}
		}
	}
	walk(p.Errors)
	for _, obs := range p.Observations {
		add(obs.Evidence, obs.SourceID)
	}
	if ctx.AttachedLogCatalog != nil {
		locations, err := ctx.AttachedLogCatalog.Locate(ctx.Context(), selectors)
		if err != nil {
			return nil, err
		}
		for i, s := range selectors {
			proof[s] = locations[i]
		}
	} else {
		for _, selector := range selectors {
			if selector.SourceID != "" {
				return nil, &logSourceSelectionError{}
			}
		}
	}
	return proof, nil
}

type logSourceSelectionError struct{}

func (*logSourceSelectionError) Error() string {
	return "source_id requires an attached complete log catalog"
}

func (proof logTriageSourceProof) location(text, source string) loginput.ExcerptLocation {
	if l, ok := proof[loginput.ExcerptSelector{Text: strings.TrimSpace(text), SourceID: source}]; ok {
		return l
	}
	return loginput.ExcerptLocation{Status: "preview_only"}
}

// Excerpt presence and occurrence counts use the complete selected material
// when available. A failed source may retain exact historical preview text,
// but its location never gains original-source authority.
func logTriageExcerptCount(text, source, preview string, proofs []logTriageSourceProof) int {
	if len(proofs) > 0 {
		location := proofs[0].location(text, source)
		if location.Status != "preview_only" && location.Status != "source_unavailable" {
			return int(location.Matches)
		}
		if location.Status == "source_unavailable" {
			// Healthy matches remain a lower bound, never complete coverage.
			return max(int(location.Matches), strings.Count(preview, strings.TrimSpace(text)))
		}
	}
	return strings.Count(preview, strings.TrimSpace(text))
}

func bindLogTriageSources(p *emitLogTriageParams, proof logTriageSourceProof) {
	var walk func(*emitLogTriageError)
	walk = func(e *emitLogTriageError) {
		e.sourceBinding = types.NewLogSourceBinding(proof.location(e.Message, e.SourceID))
		if e.Cause != nil {
			walk(e.Cause)
		}
	}
	for i := range p.Errors {
		walk(&p.Errors[i])
	}
	for i := range p.Observations {
		o := &p.Observations[i]
		o.sourceBinding = types.NewLogSourceBinding(proof.location(o.Evidence, o.SourceID))
		o.LineStart, o.LineEnd = 0, 0
		if o.sourceBinding.IsVerified() {
			o.LineStart, o.LineEnd = int(o.sourceBinding.FirstLine), int(o.sourceBinding.LastLine)
		}
	}
}
