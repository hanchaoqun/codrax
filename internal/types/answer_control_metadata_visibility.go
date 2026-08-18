package types

// AnswerControlMetadataVisibilityGuide is shared prompt guidance for
// model-authored structured answer fields. Enum literals and field names are
// transport/control metadata: they must steer the structured contract without
// leaking into customer-facing prose. This is deliberately guidance, not a
// prose scanner or answer rewriter; conclusion wording remains model-owned.
const AnswerControlMetadataVisibilityGuide = "JSON field names and raw enum literals are control metadata only. Put them only in their projected JSON fields; never copy them into user-visible text, headings, lists, tables, parenthetical explanations, or diagrams. Do not narrate which machine value you selected (for example, never write 'classified as <enum>' or 'the status is <enum>'); state only its reader-facing meaning in the current answer language (for example, say that something is only a candidate within the selected window and that frame/deadline causality remains unproven). Do not expose internal pipeline terminology merely to explain the schema. This is authoring guidance: the framework does not scan, reject, delete, translate, or rewrite your prose or conclusion."
