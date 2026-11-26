package skill

import (
	"crypto/sha256"
	"encoding/json"

	"github.com/hanchaoqun/codrax/internal/types"
)

func ownAnswerDocumentDraftTeaching(c *Config) *Config {
	digest := answerDocumentDraftTeachingDigest(c)
	c.answerDocumentDraftTeaching = &digest
	return c
}

func answerDocumentDraftTeachingDigest(c *Config) [32]byte {
	static := *c
	// Tool visibility is projected independently before prompt assembly.
	static.ToolSuggestions = nil
	raw, _ := json.Marshal(static) // Config's teaching fields are JSON values.
	return sha256.Sum256(raw)
}

// AnswerDocumentPatchTeaching is a shorter representation of the built-in
// skill for one already-routed, necessary patch. It is not a second JSON
// schema, a rewrite license, or an override of customer skill instructions.
func (c *Config) AnswerDocumentPatchTeaching() *Config {
	if c == nil || c.answerDocumentDraftTeaching == nil || *c.answerDocumentDraftTeaching != answerDocumentDraftTeachingDigest(c) {
		return nil
	}
	out := *c
	out.Goal = "Repair the current structured answer draft in one scoped tool call. The draft is not accepted until the tool validates the merged document."
	out.Workflow = []string{
		"Read the latest validator feedback and the projected patch tool schema. The current schema is the sole authority for field names, types, enum values, and available operations; the latest feedback identifies what still needs repair. Do not reopen evidence or start a new answer.",
		types.AnswerDocumentPatchOperationTeaching,
		"Keep all unreported content, identities, citation ownership, and block order in the inherited draft. A replace_blocks entry is a complete replacement of that block; copy its unchanged fields and edit only the reported defect. Preserve already-applied changes in the live patch base. Do not attach raw JSON as a string or write a parallel answer in prose.",
		"Keep displayed graph identities, relation directions and exact evidence anchors aligned. Use business-readable labels without changing the selected native identities. Do not manufacture a relation, a missing endpoint, or closure by relabeling. Keep the reported gap when the evidence does not establish it.",
		"Use only current typed facts and compatible derived arithmetic. Preserve native source, target, time bounds and endpoint inclusion, state, unit, and denominator. A preview, background activity, or nearby span cannot become a complete population or a Trace root cause. Preserve the on-chain evidence and order, applicable uncertainty, and the independent source and observation boundaries in the prompt.",
	}
	out.OutputFormat = "Submit one actual `emit_answer_document_patch` function call, using its projected patch tool schema and the current repair instructions. The following generic answer-emission prohibitions also apply to the merged document and to this patch; they do not request another full submission."
	return &out
}

// AnswerDocumentTraceDraftTeaching delegates emission shape to the current
// projected schema for an artifact-only Trace answer under input pressure.
// It replaces built-in teaching only; custom skill prose remains authoritative.
func (c *Config) AnswerDocumentTraceDraftTeaching() *Config {
	if c == nil || c.answerDocumentDraftTeaching == nil || *c.answerDocumentDraftTeaching != answerDocumentDraftTeachingDigest(c) {
		return nil
	}
	out := *c
	out.Goal = "Answer the requested Trace diagnosis from its typed evidence and declared scope."
	out.Workflow = []string{
		"Use the current answer-document tool schema for fields, block kinds, required receipts, exact evidence IDs, cardinality and permitted mutation. Cover every required answer dimension and applicable deliverable in the current request; preserve its language and scope.",
		"Use the typed trace evidence to distinguish target identity, measured occupancy, ranked candidates and established causality. Keep saved and recomputed accounts separate; keep wakeup edges distinct from independently proved blocking or waiting for work completion. Retain discontinuities, coverage limits, and unknowns in the conclusion itself.",
		"Bind each important claim to its exact source, subject, query scope and measured value. Preserve causal roles and order. Separate observation, inference and next verification; leave unproved ownership, causality and cross-frame implications open. A preview or adjacent load does not establish a complete roster or root cause.",
		"Follow the current typed presentation and evidence-linking requirements. If the schema requires a work binding, name its principal block and select one exact observation and conclusion pair from the schema. Keep system metadata out of visible prose.",
	}
	out.OutputFormat = "Submit one complete native JSON object through emit_answer_document. Include required answer blocks, evidence bindings and uncertainty. Omit optional notes and repeated explanations. Do not also emit the answer as chat text."
	return &out
}
