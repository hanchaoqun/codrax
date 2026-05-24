# Local Small Model Eval Campaign - 2026-05-24

Status: current measurement batch captured; `s5b` timed out before final
answer.

Commit under test: `426d1022`.

## Goal

Run a focused, serial eval pass on the current `main` code, collect all
observed failures and costly-but-passing behavior, record deep root causes, and
cluster the remaining system gaps before making more code changes.

This campaign intentionally avoids concurrent evals so logs and results can be
attributed to the current run. It also avoids prompt edits during measurement.

## Eval Matrix

| Order | Case | Purpose | Result dir | Verdict |
| ---: | --- | --- | --- | --- |
| 1 | `qf_architecture` | Architecture explanation with required stage roles and diagram contract pressure. | `eval/results/qf_architecture-20260524-141008` | PASS |
| 2 | `qf_diagram_pipeline` | Explicit Mermaid pipeline diagram and final diagram rendering. | `eval/results/qf_diagram_pipeline-20260524-142301` | PASS |
| 3 | `qf_type_relation_loop_controller` | Typed relation/member coverage with diagram output. | `eval/results/qf_type_relation_loop_controller-20260524-143806` | PASS |
| 4 | `read_combo_log_current_code_boundary` | Runtime log + current source boundary separation. | `eval/results/read_combo_log_current_code_boundary-20260524-145726` | PASS |
| 5 | `read_combo_git_two_diffs_current_code` | VCS diff + current source dimension preservation. | `eval/results/read_combo_git_two_diffs_current_code-20260524-151154` | FAIL |
| 6 | `s5b` | Broad enumeration completeness and tail-package preservation. | `eval/results/s5b-20260524-152240` | FAIL (timeout) |

## Metrics To Capture

- Verdict and regex/substr failure reason.
- `analyzer_iters`, `explorer_iters`, `extractor_iters`, `finalizer_iters`.
- `tool_read_file`, `concrete_values`, `midloop_inject`.
- Answer-document reject count and reason family.
- JSON/tool-call recovery events.
- Stream/timeout/stall events.
- Whether the final answer preserved the user's requested surface
  dimensions, diagram type, and source-origin boundaries.

## Findings Log

### Run 1 - `qf_architecture`

Result: PASS.

Metrics:

- `analyzer_iters=4`, `explorer_iters=13`, `extractor_iters=1`,
  `finalizer_iters=1`.
- `tool_read_file=21`, `concrete_values=16`, `midloop_inject=7`.
- `strict_decode_remap_events=0`, `semantic_quality_concerns=0`.

Observations:

- The answer was correct and shipped in one finalizer pass.
- Exploration still paid a noticeable cost: 13 explorer iterations, 21
  `read_file` calls, and 7 mid-loop injections for a small architecture
  question.
- A sibling explorer stream stalled after convergence and was salvaged:
  `upstream LLM stream stalled (no bytes for 2m4.997852s)`. Winner-aware
  convergence prevented this from failing the run, but the stall still consumed
  wall time.
- The model emitted `pipelineTopology` with line `104` even though the file has
  only 84 lines. The system recovered it to line `24`, then forced a localized
  read-and-re-emit repair. This is correct behavior, but it added two turns.
- After the repair, the mid-loop partial-read detector built a hint for the
  giant `explorerEvaluator.BuildInitialInstruction` function even though the
  accepted evidence was already sufficient. It was throttled and did not derail
  this run, but it is a reusable cost-risk signal.
- Finalizer context contained many weakly related value-fact enrichment rows
  from helper functions (`readSetContains`,
  `tracePathEndpointCoveredByReadWindow`, UI/render helpers, etc.). The final
  answer remained clean, but the prompt carried unnecessary cognitive load.

## Root Cause Notes

Classify observations as:

- System over-control: the system changes or prematurely narrows the model's
  intended investigation.
- Missing structural compatibility: model output is semantically usable but
  violates recoverable JSON/schema syntax.
- Evidence ranking/search width: relevant accepted evidence exists but is
  buried, delayed, or out-ranked by broader context.
- Surface preservation: the answer is factually grounded but drops requested
  diagram type, labels, dimensions, or origin boundaries.
- Model/runtime transport: stalls, timeouts, or local model latency not caused
  by validation/control logic.

Cross-language red line for enumeration fixes:

- Do not solve `s5b` by hard-coding Go-specific file names, export casing, or
  `func` regexes. This repository's repomap read layer supports Go, Python,
  JavaScript, TypeScript, Java, Kotlin, Rust, C, C++, Ruby, Swift, Lua, Proto,
  ArkTS, and Cangjie through `internal/tool/repomap/types/lang.go`.
- The general fix must build a language-neutral candidate set from repomap
  `FileInfo.Language`, symbol `Kind`, package/module/directory ownership,
  relation data, and existing entrypoint/rank helpers. Per-language adapters may
  supply conventions such as `main.go`, `__main__.py`, `src/main.rs`,
  `Index.ets`, or `main.cj`, but those conventions must remain behind the
  repomap abstraction.
- When a user asks for a source inventory under a path/package/module, the
  deterministic candidate carrier should preserve: discovered member set,
  candidate files, symbol candidates, excluded tests/generated artifacts, and
  uncertainty/ambiguity. The LLM should choose among candidates and explain
  judgment, not rediscover the filesystem by guessing language-specific names.

System-detection safety rule for source-inventory assistance:

- Do not introduce a new hard intent classifier that decides "this is an
  enumeration / entrypoint / directory-inventory question" from user prose or
  model prose and then forces a different flow. That repeats the same
  system-over-control risk observed in finalizer rewrites and diagram-shape
  drift.
- Instead, source-inventory assistance should be an advisory retrieval lane
  produced only from verifiable structured signals: scoped paths or modules
  resolved by existing path/package parsing, analyzer-emitted typed IR when
  present, and repomap/file-index facts. The output should be a compact
  candidate artifact with provenance and confidence, not a system verdict.
- The model must see the artifact as candidates with explicit status, for
  example `inventory_complete=true/false`, `candidate_source=repomap|filetree`,
  `confidence`, `ambiguities`, and `excluded`. The model decides whether those
  candidates answer the user's request, whether the inventory is exhaustive, and
  how to phrase uncertainty.
- Misclassification/noise control: when confidence is low, scope is too broad,
  or the candidate set is large, include at most a bounded summary plus a pointer
  to the structured candidate carrier. Do not promote it to principal answer
  obligations unless downstream evidence or the model's own structured emit
  confirms it. This keeps false positives as low-cost context instead of
  forcing rewrites or overriding user intent.

Run 1 provisional root causes:

- Evidence ranking/search width: mechanism-kind enrichment still lets a large
  set of weak helper facts flow to finalizer context. The current typed support
  lanes keep the final answer bounded, but not the prompt size/cognitive load.
- System over-control risk: partial-read hints are structurally useful, but for
  architecture/mechanism questions they can point at huge functions after an
  accepted sufficient evidence set exists. The current throttle avoided a loop;
  the deeper fix should make partial-read debt load-bearing-aware.
- Missing structural compatibility: line-number recovery behaved correctly and
  did not mutate model semantics. The remaining cost is extra turns, not
  correctness.
- Model/runtime transport: local stream stall after convergence is tolerated by
  current orchestration, but remains a latency cluster to track across runs.

### Run 2 - `qf_diagram_pipeline`

Result: PASS, with significant cost and answer-quality observations.

Metrics:

- `analyzer_iters=4`, `explorer_iters=24`, `extractor_iters=1`,
  `finalizer_iters=1`.
- `tool_read_file=19`, `concrete_values=16`, `midloop_inject=11`.
- `explorer_dispatches=2`, caused by a stream stall and retry.
- `strict_decode_remap_events=0`, `semantic_quality_concerns=0`.

Observations:

- The final output contained a Mermaid `flowchart TD` block and satisfied the
  eval's literal expectations.
- Analyze stage attempted two `read_file` calls and one `grep` after entering
  terminal emit mode. The system rejected them and normalized the grep
  `files_only` parameter, then the model emitted `emit_analysis` on retry.
- The first `emit_analysis` attempt had a schema compatibility issue:
  `requested_answer_dimensions` lacked `confidence`; the second emit succeeded.
  `diagram_hint` shape normalization worked.
- The first exploration dispatch collected useful evidence but then hit
  `upstream LLM stream stalled (no bytes for 2m4.997983s)`. The orchestrator
  retried the explore phase, but the second dispatch repeated broad search and
  file reads instead of closing from the already accepted evidence.
- The second exploration dispatch re-read `enums.go`, `analyzer.go`,
  `explorer.go`, `extractor.go`, and `finalize_preview.go`, adding many
  mid-loop reminders and ungrounded/repaired evidence rows before closure.
- Extractor emitted a full prose answer with a Mermaid diagram in its text. It
  did not break the pipeline, but it is another reproduction of "intermediate
  stages eager-answer instead of only doing their stage contract".
- The final answer passed, but it contained quality smells not caught by the
  case:
  - It stated or implied analyzer uses deep file-reading tools, despite the
    analyzer-stage contract rejecting `read_file`.
  - It rendered a system supplement table with an extra `列 5` column.
  - The user asked for a concise stage diagram, but the final answer expanded
    into internal implementation detail and did not fully balance the four
    stage role sections.

Run 2 provisional root causes:

- Recovery after model/stream transport failures is too coarse. The retry path
  preserved enough state to continue eventually, but it did not exploit the
  first dispatch's accepted evidence as a convergence boundary, causing repeated
  exploration.
- Analyzer-stage boundaries are correct but still cognitively expensive for
  small models: rejected deep-reading calls and repeated `emit_analysis` schema
  repair add latency before exploration even starts.
- Stage-role requested dimensions are preserved enough to pass, but answer
  surface quality is weak: system supplements and model-authored sections can
  add unrelated columns or uneven detail without triggering a reviewer concern.
- Extractor eager-answer text is useful for transparency, but it can leak
  partially grounded diagrams/prose into context and may bias the finalizer
  toward verbose implementation detail.

### Run 3 - `qf_type_relation_loop_controller`

Result: PASS, but this is the strongest gap sample so far.

Metrics:

- `analyzer_iters=3`, `explorer_iters=31`, `extractor_iters=1`,
  `finalizer_iters=1`.
- `tool_read_file=34`, `concrete_values=26`, `midloop_inject=19`.
- `finalizer_dispatches=2` because the first finalizer stream stalled and was
  retried.
- `strict_decode_remap_events=0`, `semantic_quality_concerns=0`.

Observations:

- Analyze again attempted deep `read_file` calls in the classification stage.
  The rejected paths were absolute `.codrax/blob/...` snapshot paths, not repo
  relative source paths. The system rejected the calls correctly.
- Analyzer sub-topic text hallucinated generic implementation names
  (`StructuralIterator`, `ConstrainedIterator`, `ResultIterator`) even though
  the concrete repo matches were evaluator structs. These names later survived
  into the finalizer prompt's Answer Structure heading.
- Exploration quickly found the real `LoopController` interface and many
  implementation anchors, but then kept widening:
  - multiple lanes repeatedly read `internal/agent/agent.go`;
  - one lane read `internal/agent/agent_test.go`;
  - test-only implementations
    `isolatedPromptEvaluator`, `protocolSoftStopEvaluator`, and
    `protocolSoftStopAcceptEvaluator` became grounded evidence and final answer
    members.
- One exploration lane emitted `emit_investigation_complete`, but sibling lanes
  kept reading and emitting. This is probably because relation/member-set +
  diagram requests trigger sibling handoff waiting, but the current wait policy
  does not distinguish "required production members" from "auxiliary/test
  expansion".
- The model produced a complete class diagram and member table in exploration
  prose, then continued reading more files. This repeats the eager-answer +
  failure-to-stop pattern.
- The finalizer first stream stalled; retry succeeded.
- The successful `emit_answer_document` still needed compatibility repair:
  `blocks[]` arrived as a JSON-encoded string, a recovered diagram attachment
  was promoted, and current-source citation rows were materialized.
- Final answer passed but included 11 implementations, including 3 test-only
  types. It did explicitly label them "测试专用", so it is not strictly wrong,
  but the user's phrase "主要实现类型" arguably should have prioritized
  production implementations and kept tests separate or caveated.
- The final answer said "前 6 个实现是生产环境使用的，后 3 个是仅在测试中使用的",
  but also listed `logTriagerEvaluator` and `perfTriagerEvaluator` after the
  test-only rows. That visible grouping/count prose is internally inconsistent.

Run 3 provisional root causes:

- Analyzer can inject wrong example members into typed sub-topic headings. Even
  when downstream evidence corrects the actual member set, the stale heading can
  remain in finalizer instructions.
- Relation/member-set exploration lacks a strong production-vs-auxiliary
  principal boundary. Typed graph results include test implementations as real
  code facts, but the answer surface needs "principal production members" and
  "auxiliary/test-only members" as separate roles.
- Sibling handoff waiting is too broad for relation+diagram requests: after one
  lane has a complete production set, support/test lanes can keep expanding and
  eventually become principal.
- Finalizer JSON compatibility repairs are working, but this run proves the
  stringified `blocks[]` path is not only a small-model issue; it appears under
  a large, noisy relation prompt too.
- Semantic review accepted an answer with minor internal grouping/count
  inconsistency, suggesting reviewer criteria focus on required literal
  coverage and not enough on visible self-consistency of grouped member prose.

### Run 4 - `read_combo_log_current_code_boundary`

Result: PASS.

Metrics:

- `analyzer_iters=1`, `explorer_iters=11`, `extractor_iters=1`,
  `finalizer_iters=1`.
- `tool_read_file=15`, `concrete_values=8`, `midloop_inject=6`.
- `strict_decode_remap_events=0`, `semantic_quality_concerns=0`.

Observations:

- Log triage took two rounds: the first `emit_log_triage` was rejected because
  `errors[]`, `observations[]`, and `unknown_chunks[]` were all empty. The
  second emit correctly represented the attached log as a timeout/retry
  observation.
- Analyze succeeded in one round and correctly emitted
  `current_source_explanation_profile` with both
  `compare_with_current_source` and `explain_current_mechanism`. This preserved
  the user's boundary: do not treat attached log line numbers as current-source
  citations.
- Exploration started with a broad pre-scan list for the word family around
  "finalizer" and read several generic files before producing evidence:
  `violation_registry.go`, `defaults.go`, `logtriage/validate.go`,
  `status_classify.go`, `env/recommend/llm.go`, `status_messages.go`,
  `finalizer.go`, `finalizer_auto_repair.go`, and `orchestrator.go`. The final
  answer was still correct, but the first half of exploration was wider than
  the user surface required.
- Mid-loop had to remind the model after 9 read files without an
  `emit_evidence` call. The reminder worked, but it is another sign that broad
  current-source questions can drift into "read many finalizer-adjacent files"
  before emitting usable facts.
- `emit_evidence` needed anchor repairs:
  - the model used a full Go signature
    `func (o *Orchestrator) recoverRejectedFinalizerDraftAfterTransientFailure`
    as `anchor_symbol`; the line only grounds the identifier
    `recoverRejectedFinalizerDraftAfterTransientFailure`;
  - the model used `dispatchFinalizer` on a call-site line whose local snippet
    actually grounded `emit`.
  The repair loop recovered both, but it added two exploration turns.
- Extractor emitted a useful prose answer and clear historical/current-source
  distinction. Its prose slightly over-simplified validation behavior
  ("system validation usually does not retry"), but the finalizer's accepted
  answer restored a bounded distinction between transient stream failures and
  contract validation failures.
- Finalizer accepted in one round. The answer-document tool auto-repaired two
  observed-artifact claim-use carriers, which is a good low-risk compatibility
  path because it preserves the model's content and only fixes structural
  metadata.

Run 4 provisional root causes:

- Current-source boundary handling is conceptually correct, but retrieval
  ranking is still too broad for log+current-code questions. Generic finalizer
  matches can outrank the few load-bearing current-code symbols needed to
  explain the attached runtime observation.
- Anchor-symbol compatibility is still incomplete for language-native
  declarations. A model can paste an entire function signature, receiver, or
  call-chain phrase where the grounder expects the minimal identifier. This is
  recoverable across languages if the system normalizes to parser/repomap
  identifiers before rejecting.
- The "read many files before first evidence" pattern recurred, suggesting the
  evidence emission cadence guard is valuable but reactive. A deeper fix should
  let retrieval produce smaller, better-ranked first batches and encourage
  early evidence on already-read load-bearing files.
- Observed-artifact/current-source citation separation worked and should remain
  a protected contract for future changes.

### Run 5 - `read_combo_git_two_diffs_current_code`

Result: FAIL.

Failure reason:

- Eval regex did not find a visible binding between commit/diff evidence and
  current implementation: `no_regex_match:(commit|提交|diff|差异).*(当前|源码|现在|实现)|...`.

Metrics:

- `analyzer_iters=6`, `explorer_iters=16`, `extractor_iters=1`,
  `finalizer_iters=1`.
- `analyzer_dispatches=2`, `tool_read_file=8`, `concrete_values=8`,
  `midloop_inject=7`.
- `semantic_quality_dispatches=1`, `semantic_quality_concerns=0`.
- `strict_decode_remap_events=0`.

Observations:

- The first analyze dispatch went down the wrong shape:
  `question_kind=call_chain`, `scenario=root_cause`, and generated generic
  sub-topic entities like `current source code`, `implementation chain`,
  `function call`, and `data flow`. The quality gate rejected it for
  `subtopic_coherence`. This was a correct rejection, but cost a full dispatch.
- The second analyze dispatch preserved the user's requested visible
  dimensions: `diff 线索`, `当前关键代码`, `作用`, `影响`, and correctly set
  `is_history_lookup=true` plus current-source explanation modes. This is good
  intent capture.
- Exploration used the right VCS tools first:
  `git_log(count=2)`, `git_diff(ref=426d1022)`, and `git_show` for both
  `426d1022` and `92c4edfa`.
- After VCS collection, current-source exploration widened into implementation
  files and produced useful but broad evidence:
  `explorer.go` endpoint-coverage helpers, `agent.go` salvage/compat helpers,
  and `orchestrator.go` hypothesis-verdict drain behavior.
- A `git_diff(ref=426d1022)` call returned only the banner
  (`[git_diff: ref=426d1022 ...]`) rather than useful patch content. The run
  still had `git_show` output, but the explicit `git_diff` lane was effectively
  empty. This is a tool-use / result-shape risk for diff-first questions.
- `emit_evidence.items` arrived as a JSON-encoded string and was normalized:
  `$.items string->array via json_string_array_quote_escape`. This compatibility
  path worked.
- Evidence shape repair also worked: missing `line_end` was safely downgraded
  to `scope=line`, and missing `anchor_symbol` was inferred from typed fields
  corroborated by already-read lines.
- The repair lane cost was high: after narrowing to exact repair tools, the
  model took about 68 seconds to re-emit two grounded rows for
  `salvagePartialDispatch`.
- Extractor produced a richer answer than finalizer. It had two commit
  sections, listed diff-derived clues, current code, purpose, impact, and a
  comparison table. This was much closer to the user's requested shape.
- Finalizer compressed the answer into three implementation sections and one
  ordered list of functions. It mentioned both commits only in the summary, but
  did not render a per-commit/per-dimension table or section structure.
- The answer-document tool repaired 5 `citation_ref` values by typed
  label/citation corroboration and accepted the document. That reduced model
  burden, but it also shows citation repair alone cannot guarantee requested
  dimension preservation.
- Contract check recorded 2 v2 block-oracle violations and CGEC reported
  `answer_facet_coverage` / `answer_richness_facet_coverage` violations, but
  semantic quality still returned no concern and the pipeline shipped a failing
  answer.

Run 5 root causes:

- Dimension preservation gap: `requested_answer_dimensions` are captured in
  analysis, but finalizer contracts do not force a visible row/section/table
  that binds each dimension to each requested bucket (here: each recent commit).
- Origin-lane preservation gap: VCS/diff observations are available in the
  Observation Ledger, but only current-source lanes become principal support.
  Finalizer therefore drifts toward current-source implementation sections and
  treats diff evidence as summary context.
- Extractor-to-finalizer richness loss: extractor generated the desired
  user-facing structure, but downstream synthesis rebuilt a thinner answer
  instead of preserving the typed dimensions and bucket grouping.
- Review gap: self-consistency checks compared summary/body non-contradiction,
  not whether the answer satisfied the user's requested dimensions. CGEC saw
  facet-coverage violations, but they were not promoted into a rewrite or a
  deterministic repair.
- Analyze fallback gap: the first analyze run tried deep tools and bad generic
  sub-topics for a history+current-code question. The fallback succeeded, but
  this class should avoid the first expensive failed shape.

### Run 6 - `s5b`

Result: FAIL (terminated by eval harness timeout before final answer).

Result dir: `eval/results/s5b-20260524-152240`.

User request:

> 列出 internal/analysis/ 下所有子包的目录名，以及每个子包的单一入口函数（entry-point function）。

Observed so far:

- The request is an intentionally broad source-inventory problem. It is not a
  Go-only problem class even though this repository's `internal/analysis`
  happens to contain Go packages. Any product fix must use repomap's
  cross-language file/symbol model and language adapters, not Go regexes or
  `main.go`/`func` assumptions.
- Analyzer did exploration-like work: mass `list_files` over
  `internal/analysis/*` and then 25 `grep` calls with the Go-shaped pattern
  `func\s+(Run|Execute|Analyze|Dispatch|Process)\(`. The existing
  compatibility layer correctly forced analyzer greps to `files_only=true`, but
  the plan was still too language- and verb-specific.
- Exploration guessed likely filenames such as `axis/axis.go`,
  `compiler/compiler.go`, `prescan/prescan.go`, and `priority/priority.go`
  even after directory listing had enough information to constrain candidates.
  The actual files include paths like `axis/matrix.go`,
  `compiler/compile.go`, `prescan/token_classifier.go`, and
  `priority/score.go`.
- Multi-lane exploration caused repeated broad enumeration. Different lanes
  restarted from partial views and re-read overlapping packages instead of
  sharing one authoritative inventory candidate set.
- Context reached roughly 60k estimated tokens and triggered tool-history prune
  while the run was still trying to enumerate packages.
- A first `emit_investigation_complete` attempt was rejected by structural
  count checks: `member_set.value` and `members` length disagreed, and one
  grouped count omitted at least one discovered package. The guard is valuable,
  but the upstream inventory should have prevented the mismatch before the
  model tried to close.
- A later exploration lane used `exec_command ls -la internal/analysis/` to get
  directory inventory. The result was useful, but this information should be
  available through bounded repository tools (`list_files` / repomap inventory)
  so enumeration does not depend on shell behavior.
- The pre-scanned file ranking mixed scoped files under `internal/analysis/`
  with many unrelated `internal/types`, `internal/tool`, and orchestrator files.
  It also showed non-monotonic priority ordering. This reinforces that keyword
  pre-scan cannot be the authority for scoped source inventory.
- The model repeatedly put the repo file path in `subject` while omitting
  `source` for `emit_evidence(scope="line")`. Existing repair inferred `source`
  for some rows when exact line/anchor corroboration was unambiguous, but a
  later all-path-as-subject batch was rejected wholesale. This is a safe
  compatibility opportunity: when `subject` is an already-read repo-relative
  path and the requested line/anchor validates, it can be treated as `source`
  while leaving semantic subject inference to the model or validator.
- The run timed out while still in exploration. No extractor or finalizer stage
  ran, so this is a pipeline-convergence failure rather than a final answer
  formatting failure.

Metrics:

- `analyzer_iters=5` by unique iter id; analyzer output showed four visible
  rounds.
- `explorer_iters_unique=15`, but visible output had five concurrent routes
  (`第 1 路` ... `第 5 路`) with route-local rounds up to at least 19.
- Tool result counts: `read_file=28`, `list_files=30`, `grep=29`,
  `exec_command=5`, `emit_evidence=5`, `emit_investigation_complete=1`.
- `midloop_inject=9`, `TOOL HISTORY PRUNED=1`, `received terminated=1`.
- The deterministic directory count is 25 direct child directories under
  `internal/analysis`; one exploration route nevertheless stated "all 26
  subpackages" and another close attempt had mismatched aggregate counts.

Run 6 provisional root causes:

- Missing source-inventory carrier: directory member sets, candidate files, and
  per-member candidate symbols exist in tool outputs, but are not preserved as a
  shared structured artifact across analyzer/explorer lanes.
- System detection must be advisory, not decisive. The system can compute a
  candidate inventory from verified repo facts, but should not decide the user's
  answer shape or force an enumeration flow from prose classification.
- Cross-language entrypoint abstraction gap: "entry-point function" requires a
  language-neutral candidate model. Candidate generation should use repomap
  language, symbol kind, ownership, visibility, call graph/rank hints, and
  per-language adapters behind repomap, then let the model choose and explain
  ambiguity.
- Search-width governance gap: broad grep and guessed filenames remain cheaper
  for the model than consulting a structured candidate set. That incentives
  slow, lossy, duplicated exploration.
- Count/member-set integrity is currently enforced downstream. It should also
  be supported upstream by deterministic inventory facts so the model has a
  reliable source of truth for complete lists and exclusions.
- Field-shape repair is still incomplete for evidence rows. Path-valued
  `subject`, path-valued `object`, and omitted `source` are a recoverable family
  when corroborated by already-read source windows. The repair must remain
  structural only: never infer the missing domain subject unless the model also
  supplied a validated anchor symbol or surface term.

## Gap Clusters

Provisional clusters after Run 1:

1. Context noise after accepted evidence: typed enrichment can carry many
   technically grounded but non-principal helper facts downstream.
2. Load-bearing vs advisory repair/read debt: partial-read and recovered-line
   repairs should stay precise, but not reopen broad exploration when accepted
   evidence already satisfies the user surface.
3. Local model transport latency: stream stalls are now recoverable, but still
   affect wall time and perceived responsiveness.
4. Transport retry state reuse: after a stream stall, rerunning a whole
   exploration dispatch can duplicate broad search instead of resuming from or
   closing over already accepted evidence.
5. Intermediate-stage eager answering: explorer/extractor prose can be valuable
   to display, but stage contracts and downstream prompts need to prevent it
   from becoming noisy or misleading principal context.
6. Accepted-answer surface polish: passing answers can still carry odd system
   supplement columns or uneven section depth; current eval regexes and
   semantic review do not flag this class.
7. Production-vs-auxiliary principal boundaries: test fixtures and helper
   implementations are valid code facts, but they should not automatically
   become principal members for "main/primary/production" relation answers.
8. Analyzer stale sub-topic contamination: wrong illustrative entities in
   analyzer sub-topics can survive as downstream structure even after evidence
   disproves them.
9. Grouped member self-consistency: final answers can preserve required member
   names while emitting inconsistent count/group prose; semantic review did not
   catch it in Run 3.
10. Current-source boundary retrieval width: history/log/diff questions can
    preserve citation boundaries but still over-read generic current-source
    files before finding the few load-bearing implementation anchors.
11. Cross-language anchor normalization: evidence anchors should accept
    language-native signatures and reduce them to canonical identifiers when
    parser/repomap data makes the reduction unambiguous; otherwise small
    models pay extra repair turns for recoverable formatting.
12. Dimension-by-bucket contract loss: requested visible dimensions can be
    captured in analyze and even drafted in extract, then disappear in finalizer
    because acceptance focuses on grounded citations rather than per-bucket
    dimension coverage.
13. Origin-lane principal promotion gap: VCS/log/trace/tool-output lanes remain
    "soft" support even when the user explicitly asks for them as principal
    answer dimensions; current-source lanes can dominate the final answer.
14. Review escalation gap: CGEC/v2 block-oracle facet violations may be logged
    without triggering a visible repair when the answer is internally
    consistent but user-dimension incomplete.
15. Source-inventory carrier gap: scoped directory/package/module inventories
    are discovered repeatedly through tools, but not elevated into a shared
    structured candidate artifact with completeness, exclusions, and
    provenance.
16. Cross-language entrypoint candidate gap: broad "entry function" questions
    need a language-neutral candidate model over repomap symbols and relations.
    Go-shaped regexes and exported-name heuristics are only one adapter's
    private implementation detail, not the product contract.
17. Multi-lane duplication gap: parallel exploration lanes do not coordinate on
    a single authoritative inventory candidate set, so each lane repeats broad
    search, guesses filenames, and inflates context.
18. Shell-as-inventory gap: models fall back to `exec_command find/ls` for
    repository enumeration because structured tools do not expose the exact
    inventory artifact they need. Bounded repo tools should make shell use
    unnecessary for normal source inventories.
19. Evidence field transposition gap: small models often place file paths in
    `subject`/`object` instead of `source`. This is safely repairable only when
    the path, line, and anchor are already corroborated by a read window.

## Proposed High-ROI Task Order

P0 - Cross-language source-inventory carrier:

- Add a structured candidate artifact for scoped source inventories. It should
  be built from existing repo/file/repomap data, carry member directories or
  modules, candidate files, candidate symbols, language, exclusions,
  completeness, ambiguity, and provenance.
- Feed it to analyzer/explorer/finalizer as advisory context. It must never
  force a user-intent decision or overwrite model conclusions; the model decides
  how to answer and how to disclose ambiguity.
- Make multi-lane exploration share the artifact so each lane does not repeat
  broad enumeration from scratch.

P0 - Evidence repair widening with strict proof:

- Extend existing schema-shape compatibility for `emit_evidence` so
  path-valued `subject` or `object` can fill missing `source` only when the path
  was already read and the line/anchor validates.
- Keep this structural only. Do not infer a domain subject unless it is already
  present as a validated anchor/surface term.

P1 - Requested-dimension and origin-lane preservation:

- Promote user-requested dimensions and origin lanes such as git/log/trace/tool
  output into answer contracts when they are explicitly captured by typed IR.
- Prevent finalizer from accepting an answer that drops requested buckets or
  dimensions even when citations are internally valid.

P1 - Retrieval ranking/governor cleanup:

- Put scoped path/member inventory above keyword pre-scan ranking.
- Treat keyword pre-scan as fallback advisory ranking, not as the authority for
  source inventory or entrypoint candidates.
- Audit non-monotonic "TOP PRIORITY" ordering and unrelated-file leakage.

P2 - Eval expansion:

- Add cross-language inventory evals using at least Python, TypeScript/JavaScript,
  Java/Kotlin, Rust, C/C++, Proto, ArkTS, and Cangjie-shaped fixtures so the
  source-inventory fix cannot regress into Go-only behavior.
- Add a timeout/convergence eval for broad enumeration that fails if exploration
  exceeds a bounded number of reads or repeats the same inventory across lanes.
