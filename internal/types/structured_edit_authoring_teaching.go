package types

// StructuredEditPythonScopeTeaching is shared authoring and retry guidance,
// not an additional scope or edit-kind admission rule. Keep the complete text
// within NormalizePlanRepairPack's existing 480-byte retry budget.
const StructuredEditPythonScopeTeaching = "Python: use kind=patch for indented blocks: line-range replace (start_line/end_line), or insert_before/insert_after at a current read_file anchor with exact indentation. For existing files, scope=micro requires patch. Suggest kind=modify only if scope is explicitly package, cross, or project and the edit rewrites most of the existing file, not for indentation alone. No insert_before_final_brace; EOF permits unindented top-level or supported class-member additions."
