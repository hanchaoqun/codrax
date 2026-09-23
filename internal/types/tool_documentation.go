package types

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

const (
	ToolDocumentationVersion  = 1
	ToolDocumentationMaxBytes = 64 << 10
)

// ToolDocumentationSelection records the successful call's canonical selection.
// Empty View selects a catalog; Detail distinguishes its overview from full
// contracts. It is not a query scope, capture identity, or capability grant.
type ToolDocumentationSelection struct {
	View   string `json:"view,omitempty"`
	Detail bool   `json:"detail"`
}

// ToolDocumentation is static, producer-owned documentation, never source or
// runtime evidence. Content is one complete JSON document, including all of its
// qualifications. Version describes this envelope; Schema identifies Content's
// protocol. ContentHash is a deduplication/integrity key, not authentication.
// Model emit schemas must not accept this carrier.
type ToolDocumentation struct {
	read        *toolDocumentationRead
	Version     int                        `json:"version"`
	Schema      string                     `json:"schema"`
	Selection   ToolDocumentationSelection `json:"selection"`
	ContentHash string                     `json:"content_hash"`
	Content     json.RawMessage            `json:"content"`
}

// NormalizeToolDocumentation returns a defensive copy or rejects the whole
// document. Never shorten JSON or clip conditions/limitations to fit a budget.
// A producer may omit the hash when constructing a document; a supplied hash
// must match. Unknown envelope versions are not silently interpreted as v1.
func NormalizeToolDocumentation(in ToolDocumentation) (ToolDocumentation, bool) {
	if in.Version != ToolDocumentationVersion || len(in.Content) > ToolDocumentationMaxBytes {
		return ToolDocumentation{}, false
	}
	in.Schema = strings.TrimSpace(in.Schema)
	in.Selection.View = strings.TrimSpace(in.Selection.View)
	if in.Schema == "" || len(in.Schema) > 160 || len(in.Selection.View) > 160 {
		return ToolDocumentation{}, false
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, in.Content); err != nil || compact.Len() == 0 || compact.Bytes()[0] != '{' {
		return ToolDocumentation{}, false
	}
	digest := sha256.Sum256(compact.Bytes())
	hash := hex.EncodeToString(digest[:])
	if in.ContentHash != "" && in.ContentHash != hash {
		return ToolDocumentation{}, false
	}
	in.ContentHash = hash
	in.Content = append(json.RawMessage(nil), compact.Bytes()...)
	return in, true
}

func cloneToolDocumentation(in *ToolDocumentation) *ToolDocumentation {
	if in == nil {
		return nil
	}
	out := *in
	out.Content = append(json.RawMessage(nil), in.Content...)
	return &out
}

// CloneToolHandoffCarriers protects the newly mutable documentation bytes while
// preserving the copy semantics of existing evidence/repair fields.
func CloneToolHandoffCarriers(in []ToolHandoffCarrier) []ToolHandoffCarrier {
	if in == nil {
		return nil
	}
	out := append([]ToolHandoffCarrier(nil), in...)
	for i := range out {
		out[i].Documentation = cloneToolDocumentation(in[i].Documentation)
	}
	return out
}

// ToolHandoffCarrierIsDocumentationOnly separates static metadata from the
// ordinary repair/evidence prompt and write-context projections. Mixed carriers
// retain their independently typed fields; documentation never lends authority.
func ToolHandoffCarrierIsDocumentationOnly(c ToolHandoffCarrier) bool {
	if c.Documentation == nil || c.RepairCode != "" || c.Repair != nil || c.Refinement != nil ||
		c.PlanRepairPack != nil || c.SupportedJSON != nil || len(c.AcceptedEvidence) != 0 || len(c.ObservationRefs) != 0 {
		return false
	}
	_, ok := NormalizeToolDocumentation(*c.Documentation)
	return ok
}

func toolResultHasDocumentation(r ToolResult) bool {
	if !r.Success || strings.TrimSpace(r.ToolName) == "" || r.Handoff == nil || r.Handoff.ToolName != r.ToolName || r.Handoff.Documentation == nil {
		return false
	}
	_, ok := NormalizeToolDocumentation(*r.Handoff.Documentation)
	return ok
}
