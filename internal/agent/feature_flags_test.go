package agent

// This file previously hosted runtime feature flag accessor tests
// for evidence_tool_mode, two_turn_explorer_mode, and
// answer_document_mode. All three flags were deleted in the
// 2026-04-14 simplification pass, so there is nothing to test at
// the flag layer anymore.
//
// The tests that exercised NewFinalizerAgent's dual-evaluator
// selection logic are gone too — NewFinalizerAgent now always
// constructs answerDocumentEvaluator, and the test that covered
// the legacy finalizerEvaluator branch would be a tautology.
//
// The file is intentionally left in place as a landing spot for
// future flag accessor tests if any flag is ever added again.
