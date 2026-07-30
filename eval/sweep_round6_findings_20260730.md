# Round-6 sweep survivors over EVALFIX-2A..2E (2026-07-30)

All confirmed by two independent refuters with live reproductions.


## R6-0 [medium] internal/tool/trace_query.go:514 (teaching-memo)

CLAIM: EVALFIX-2B memo key incompleteness: traceQueryMemoKey hashes only the POST-inheritance params, but the memoized result bakes in callCaveat (traceQueryAppendCallCaveats at line 451 runs inside the memoized pure core), and callCaveat's targetCaveat component is a function of the PRE-inheritance params + run context (traceQueryApplyRequestModelTarget). An explicit-target call and a target-inheriting call with identical post-inheritance params therefore collide onto one key, and the memo hit serves the wrong target-provenance caveat in the published result Caveats.

SCENARIO: Task whose analyzer RequestModel carries exactly one typed runtime target pid=123. Call 1: trace_query view=wakeup_chain pid=123 time_start=X time_end=Y (explicit) -> targetCaveat="", result stored under key K with no inheritance caveat. Call 2 in the same task: trace_query view=wakeup_chain time_start=X time_end=Y with pid OMITTED -> traceQueryApp

vote0 refuted=False: CONFIRMED by code reading AND live reproduction (temp test in internal/tool, since removed). Structural chain: (1) traceQueryApplyRequestModelTarget (trace_query.go:14024) runs at line 329, BEFORE the memo boundary, and mints targetCaveat as a function of PRE-inheritance explicit

vote1 refuted=False: CONFIRMED — the finding survives all refutation attempts; every load-bearing claim checks out against the code and the design ledger records no such deviation.

Code verification (/Users/han/opt/claude/codrax/internal/tool/trace_query.go):
1. The fork is real. `traceQueryApplyReq

## R6-1 [medium] internal/tool/strict_decode_params.go:65 (teaching-memo)

CLAIM: EVALFIX-2A made the emit_analysis requested_files rejection self-contradictory: the new CanonicalName remap arm (strict_decode_remap.go:111-117) tells the model to 'rename the key and keep the value unchanged', but the unknown-field census note appended by appendStrictDecodeUnknownFieldCensus is NOT gated on hint match (unlike the schema-list append in strictDecodeFailure, which is gated via strictDecodeHintMatchesField) and still appends 'all unknown fields in this payload (remove every one of them in a single retry)' to the same Summary/error - two conflicting imperatives (rename vs remove) on the exact lane 2A was built to fix.

SCENARIO: Analyzer emits emit_analysis with top-level key requested_files (the EVALRUN-1 F3 2/12 near-miss 2A targets). decodeStrictNormalizedToolParams fails strict decode; RemapStrictDecodeErrorWithRaw matches emitAnalysisMisplacedHints and produces '... the schema has no field "requested_files"; the field is named "required_files" - rename the key and kee

vote0 refuted=False: CONFIRMED by live reproduction. A temp probe test invoking decodeStrictNormalizedToolParams with emit_analysis params containing top-level "requested_files" produced the exact contradictory message: '... the field is named "required_files" — rename the key and keep the value unch

vote1 refuted=False: NOT REFUTED — the contradiction is real and I reproduced it live. A throwaway test in internal/tool calling decodeStrictNormalizedToolParams("emit_analysis", {"requested_files":[...]}, &p, emitAnalysisMisplacedHints) produced exactly the composite message the finding predicts: `i

## R6-2 [low] internal/tool/emit_analysis.go:1018 (teaching-memo)

CLAIM: The new CanonicalName hint row {requested_files -> required_files} matches by bare field name only (Go's 'json: unknown field' error carries no JSON path, and CanonicalName rows have no ContainerNames scoping), so a NESTED occurrence of requested_files - the natural files/fields near-miss of source_inventory_profile.requested_fields (edit distance 2) - is now taught 'rename the key to required_files and keep the value unchanged', which is wrong in both container and intent and overrides the census's correct did-you-mean "requested_fields" suggestion that was the sole teaching pre-2A.

SCENARIO: For a source-inventory question the model emits emit_analysis with source_inventory_profile: { is_source_inventory: true, requested_files: ["name","location"] } (meant requested_fields). Strict decode fails with json: unknown field "requested_files" (no path); the 2A remap arm fires the rename-to-required_files instruction and the ToolRepair channe

vote0 refuted=False: Live reproduction against the real code confirms every element of the finding. (1) The hint match is bare-field-name only: RemapStrictDecodeErrorWithRaw and strictDecodeToolRepair compare h.Field==field with no container scoping, and Go's DisallowUnknownFields error carries no pa

vote1 refuted=False: NOT REFUTED. Every mechanical claim verifies against source: (1) the CanonicalName arm in RemapStrictDecodeErrorWithRaw (internal/tool/strict_decode_remap.go:107-117) and strictDecodeToolRepair (strict_decode_repair.go:143-152) matches by bare field name only — ContainerNames is 

## R6-3 [high] internal/orchestrator/mechanical_claim_check.go:573 (claims-lineage)

CLAIM: EVALFIX-2C numeric_direction negation detection over-matches non-negating words and flips the comparator on correct sentences: the zh arm substring-matches single-char negations (不/未) inside the 4-rune window so compounds 不仅/不但/不过 read as negation, and the en arm strips '()' from the last-3-words window so parenthetical 'not counting/not only' lands as negation of the comparator.

SCENARIO: Empirically reproduced against the shipped scanner: 「耗时 80ms 不仅超过了 16.67ms 的帧预算，还阻塞了后续输入。」, "The 80ms frame time not only exceeds the 16.67ms budget but also blocks input.", 「该帧总耗时 20ms，不过超过 16.67ms 预算的部分主要来自 GC。」, and "The 80ms cost (not counting GC) exceeds the 16.67ms budget." each raise mechanical_claim_contradiction claiming the sentence asser

vote0 refuted=False: NOT refuted — empirically confirmed against the shipped scanner. Code reading (/Users/han/opt/claude/codrax/internal/orchestrator/mechanical_claim_check.go): (1) zh arm, mechanicalClaimNegatedComparator (line 573-587) does strings.Contains over the ≤4-rune pre-comparator window w

vote1 refuted=False: NOT REFUTED — independently reproduced and consistent with recorded design intent being violated, not sanctioned.

Empirical verification (temp test in-package against the shipped scanner, since deleted): all four scenario sentences raise a numeric_direction finding claiming "A ≤

## R6-4 [high] internal/orchestrator/mechanical_claim_check.go:404 (claims-lineage)

CLAIM: EVALFIX-2C comparator-operand binding has no clause/punctuation barrier inside the 30-rune leash, so attributive ≤-family comparator phrases (低于/小于 X 的帧, 'frames below X') bind a preceding same-dimension quantity from a DIFFERENT clause (typically a window/trace duration) as the left operand, producing an arithmetic 'contradiction' on a correct sentence.

SCENARIO: Empirically reproduced: 「在 10s 的窗口内，低于 16.67ms 的帧占比 95%。」 raises a violation claiming the sentence asserts 10000ms ≤ 16.67ms; "During the 10s trace, frames below the 16.67ms budget account for 95% of all frames." likewise. The duration-scoped-clause followed by an under-threshold-share statement is a routine shape in trace/perf answers (exactly thi

vote0 refuted=False: Empirically CONFIRMED, not refuted. I read /Users/han/opt/claude/codrax/internal/orchestrator/mechanical_claim_check.go and verified: (1) the binding loop (lines 391-421) picks the nearest same-sentence token strictly left/right of each comparator with only the 30-rune leash chec

vote1 refuted=False: CONFIRMED, not refuted. (1) Empirical reproduction verified in-repo: a temporary test invoking scanNumericDirectionClaims (the exact production scanner, via proseSentenceSpans) on both cited sentences raised exactly the claimed findings — 「在 10s 的窗口内，低于 16.67ms 的帧占比 95%。」 → decis

## R6-5 [low] internal/tool/emit_answer_document_v2.go:259 (disclosure)

CLAIM: recordCitationQuoteRewriteDegradation books rewrites BEFORE the emit-time pre-validation hard reject (failEmitWithRepair at ~line 300+), so a fully rejected draft's rewrites remain on the per-Run ledger; when the Run then ships via the degraded recovery lane, the answer can carry BOTH the degraded caveat's '引用摘录回填' entry (from MaterializeDeterministicAnswerSectionsForDegradedDoc's own backfill) AND the 2E footer's identical label '引用摘录回填 ×N' (from the rejected attempts' bookings) — the design §9 double-disclosure mitigation (degraded_export 不入账) only prevents same-event double booking, not the same lane disclosing twice with the identical ZH wording on one degraded answer, and the footer's count refers to rewrites in a draft that never shipped.

SCENARIO: Emit attempt 1: model-authored doc has 2 quoteless current-source citations; normalizeCurrentSourceCitationQuotes fixes them (books ×2 at line 259), then runPreEmitChecksWithContext hard-rejects the draft on other hints; retries exhaust; the text/lossy recovery lane rebuilds a doc from the raw payload (quoteless as authored), MaterializeDeterminist

vote0 refuted=False: NOT REFUTED — every mechanical claim checks out, and a runtime probe through the production render chokepoint reproduced the co-surface double disclosure.

Verified chain (all paths absolute):

1. Booking-before-reject ordering is real. In /Users/han/opt/claude/codrax/internal/to

vote1 refuted=True: REFUTED under the design-intent lens: every element of the scenario is a composition of recorded rulings, and the one lane where the scenario's premise ("rewrites in a draft that never shipped") is most natural actually contradicts it.

1. "Books BEFORE the hard reject, so reject
