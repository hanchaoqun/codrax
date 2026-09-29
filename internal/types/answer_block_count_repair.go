package types

import (
	"fmt"
	"strings"
)

// AnswerBlockCountRepair describes a count comparison, not permission to
// overwrite a particular carrier. It is produced only from the typed answer
// contract and live document; ids, titles and prose never determine the kind.
type AnswerBlockCountRepair struct {
	Operation     string            `json:"operation"`
	AcceptedKinds []AnswerBlockKind `json:"accepted_kinds"`
	FacetIDs      []string          `json:"facet_ids,omitempty"`
	Minimum       int               `json:"minimum"`
	Maximum       int               `json:"maximum,omitempty"`
	Actual        int               `json:"actual"`
}

func NewAnswerBlockCountRepair(req BlockRequirement, actual int) *AnswerBlockCountRepair {
	if !req.Required || actual < 0 {
		return nil
	}
	r := &AnswerBlockCountRepair{AcceptedKinds: req.AcceptedKinds(), FacetIDs: append([]string(nil), req.FacetIDs...), Minimum: req.MinCount, Maximum: req.MaxCount, Actual: actual}
	switch {
	case actual < req.MinCount:
		r.Operation = "add_blocks"
	case req.MaxCount > 0 && actual > req.MaxCount:
		r.Operation = "reduce_blocks"
	default:
		return nil
	}
	if !r.Valid() {
		return nil
	}
	return r
}

func (r *AnswerBlockCountRepair) Valid() bool {
	if r == nil || r.Actual < 0 || r.Minimum < 0 || r.Maximum < 0 || len(r.AcceptedKinds) == 0 {
		return false
	}
	for _, kind := range r.AcceptedKinds {
		if !IsValidAnswerBlockKind(kind) {
			return false
		}
	}
	return r.Operation == "add_blocks" && r.Actual < r.Minimum ||
		r.Operation == "reduce_blocks" && r.Maximum > 0 && r.Actual > r.Maximum
}

func (r *AnswerBlockCountRepair) Instruction() string {
	if !r.Valid() {
		return ""
	}
	parts := make([]string, len(r.AcceptedKinds))
	for i, kind := range r.AcceptedKinds {
		parts[i] = string(kind)
	}
	scope := "kind=" + strings.Join(parts, "/")
	if len(r.FacetIDs) > 0 {
		scope += ", facet_ids=" + strings.Join(r.FacetIDs, ",")
	}
	if r.Operation == "add_blocks" {
		return fmt.Sprintf("add at least %d additional block(s) of %s (currently %d; minimum %d), using add_blocks with new ids. Preserve existing data-bearing blocks; a missing kind does not mean an existing block has the wrong kind. If an existing suitable block merely lacks the required facet membership, correct that metadata while retaining its payload instead", r.Minimum-r.Actual, scope, r.Actual, r.Minimum)
	}
	return fmt.Sprintf("reduce %s blocks to at most %d (currently %d): consolidate the facts you choose to retain, then explicitly remove surplus ids with remove_block_ids; do not add another block to fix this excess", scope, r.Maximum, r.Actual)
}
