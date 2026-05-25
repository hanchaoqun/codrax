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

## Implementation Design - P0 Batch 1

This batch deliberately reuses the existing source-inventory and evidence
repair code instead of creating a second inventory lane.

Existing code anchors:

- `internal/tool/source_inventory_reconcile.go` already owns
  source-inventory reconciliation. It consumes `SourceInventoryProfile`,
  resolves request scopes, builds graph-backed candidate sets, preserves
  member notes, and refuses to rewrite typed relation member sets via
  `sourceInventoryMustNotRewriteRelationMemberSet`.
- `internal/types/source_inventory_profile.go` is the typed analyzer lane for
  inventory roles, requested fields, visibility, and enum-like qualifiers.
- `internal/tool/repomap/types/types.go` is the language-neutral source model:
  symbols carry `Kind`, `File`, `Line`, `Language`, `Exported`, `Doc`,
  receiver/parent metadata, and relation fields across Go, Python,
  JavaScript/TypeScript, Java/Kotlin, Rust, C/C++, Ruby, Swift, Lua, Proto,
  ArkTS, Cangjie, and mixed repositories.
- `internal/tool/emit_evidence.go` already performs local-model compatibility
  repairs before strict validation, including missing `source` recovery from
  exact read windows and anchor validation.
- `internal/tool/emit_investigation_complete.go` already exposes
  `aggregateSupportToolResults`, which is the canonical way for aggregate
  repair to inspect prior `grep`, `read_file`, `repo_map`, `list_files`, and
  command outputs without reparsing prompt prose.

Design choices:

- Source inventory remains advisory unless the analyzer/model emitted a typed
  `SourceInventoryProfile`. This avoids the system deciding that a user wanted
  a source inventory solely from raw request words or model prose.
- Scope recovery may consume verified tool observation banners, such as
  `list_files(path=...)`, only as scope evidence. It does not infer intent,
  target roles, or entry-point semantics. Graph membership must corroborate the
  scope before it is used.
- Candidate membership continues to come from repomap graph symbols and
  existing role/visibility filters. No Go-only filename, exported-name, or
  `func` regex rule is introduced.
- Evidence field repair is structural only. When a small model puts a file path
  in `subject` or `object` and omits `source`, the system may copy that path
  into `source` only if the path has an already-read line window and the
  provided line/anchor is unambiguous, or the line window lets the existing
  anchor-symbol repair run next. The semantic subject/object is not invented or
  rewritten.
- Relation-shaped principal member sets remain protected. Inventory repair must
  not append all symbols from a scoped file into questions whose typed shape is
  implementer/caller/callee/relationship lookup.

Batch 1 implementation:

- [x] `emit_evidence` path-slot repair:
  `repairMissingEmitEvidenceSource` now checks path-valued `subject` and
  `object` before failing on missing `source`. It only accepts a unique
  already-read path and then lets the existing anchor-symbol and grounder
  pipeline validate the row.
- [x] Source-inventory scope recovery from structured tool observation:
  `sourceInventoryRequestedScopes` now falls back to successful `list_files`
  banners when analyzer scope lanes are empty. The recovered path is still
  validated by `sourceInventoryScopeForSurface` against repomap graph files.
- [x] Cross-language test coverage for the scope path:
  the new source-inventory test uses Python and Java graph symbols under the
  recovered scope and proves an out-of-scope Go function is not promoted.
- [x] Ambiguity guard:
  evidence path-slot repair refuses to choose between two candidate source
  paths, preserving the existing fail-loud validator behavior.

Remaining P0 work:

- [ ] Add a shared advisory source-inventory candidate artifact for broad
  attribute-bearing inventories. It should carry principal member candidates,
  per-member attribute candidates, completeness, ambiguity, language, and
  provenance across explorer lanes. It must not hard-trigger inventory flow
  from raw keywords.
- [ ] Feed that artifact into explorer/extractor/finalizer as structured
  context so the model can respect user intent and choose the final answer
  shape without repeating broad searches.
- [ ] Add convergence evals where the analyzer omits
  `source_inventory_profile`, ensuring the system still exposes bounded
  verified candidates as advisory context without overriding the LLM.

## Implementation Design - P0 Batch 2

Batch 2 closes the "shared advisory candidate artifact" gap without adding a
new prompt-only heuristic lane.

Code anchors reviewed before implementation:

- `internal/types/context.go` owns `MutableState`, `TurnAArtifacts`, fork/merge,
  and defensive-copy semantics. This is the existing cross-stage contract and
  is the right place to carry a source-inventory artifact.
- `internal/tool/source_inventory_reconcile.go` already builds graph-backed
  source-inventory candidate sets from `SourceInventoryProfile`,
  `AnswerVisibilityProfile`, repomap graph symbols, requested scopes, and
  successful tool observations such as `list_files`.
- `internal/types/request_traits.go` already exposes typed-only shapes such as
  `HasAttributeBearingEnumeration` and
  `RequiresExhaustiveEnumerationMemberSetHandoff`. These are safe activation
  inputs because they do not scan localized user text or model prose.
- `internal/types/answer_candidate_role.go` already normalizes roles and defines
  `AnswerRoleProfile`, the typed positive role-binding lane.
- `internal/agent/explorer.go` already snapshots Turn A handoff after accepted
  exploration, and `internal/agent/extractor.go` already renders accepted
  closure/aggregate facts from that snapshot.

Design:

- Add `SourceInventoryAdvisory` as a typed, language-neutral data carrier. It
  stores scopes, candidate sets by `AnswerCandidateRole`, candidates with
  member/support/location/language/note/exported fields, completeness, and
  provenance.
- Build the artifact by reusing `sourceInventoryCandidateSets`,
  `sourceInventoryScopes`, and repomap graph data. No new scanner, regex
  parser, or language-specific inventory code is introduced.
- Activation stays typed-only:
  - Authoritative-compatible mode: active `SourceInventoryProfile` with
    confidence above the existing reconciliation threshold.
  - Advisory-only profile mode: active `SourceInventoryProfile` below that
    threshold still publishes graph candidates, but cannot reconcile/replace
    aggregate facts.
  - Advisory-only mode: no source-inventory profile, but typed request shape
    says attribute-bearing or exhaustive enumeration, and typed
    `AnswerRoleProfile` names the requested role. This exposes candidates to
    downstream agents without rewriting aggregate facts.
  - No typed role/profile means no artifact, even if `list_files` or keyword
    search found many files.
- The artifact is not an answer and does not replace model conclusions. It is
  structured context/provenance for downstream stages. Only the existing
  high-confidence `SourceInventoryProfile` path may continue to reconcile
  accepted aggregate member sets.
- Store the artifact on `MutableState` and copy it into `TurnAArtifacts` so
  downstream stages can consume one shared candidate set instead of each lane
  repeating wide searches. Batch 2 renders it to extractor as advisory context;
  finalizer continues to consume accepted aggregate/display rows unless later
  eval data shows a direct finalizer advisory is needed.
- Render a compact extractor advisory section only when the artifact is active.
  The wording is deliberately advisory and model-respecting: use the candidates
  when they match the answer shape; disclose ambiguity; do not treat this as a
  citation by itself. This is scoped dynamic context, not a global prompt rule.

Batch 2 task list:

- [x] Add `SourceInventoryAdvisory` types and clone helpers.
- [x] Add `MutableState` setters/getters plus defensive-copy tests.
- [x] Add `TurnAArtifacts.SourceInventoryAdvisory` copy/merge support.
- [x] Add tool-side builder that reuses existing source-inventory candidate
  code and publishes the artifact during `emit_investigation_complete`
  validation.
- [x] Render a compact extractor advisory handoff from Turn A.
- [x] Add tests for active profile, typed advisory-only fallback,
  no-typed-role silence, and cross-language candidate preservation.

## Eval Observation - 2026-05-24 Local Small Model After Batch 2

Run:

- Command:
  `EVAL_RESULTS_ROOT=eval/results/local-small-20260524 CODRAX_BIN=./codrax bash eval/run.sh eval/cases/s5b.case 1`
- Result directory:
  `eval/results/local-small-20260524/s5b-20260524-190315`
- Model/provider:
  local OpenAI-compatible model from `providers.yaml`,
  `recover_text_tool_calls=true`.

Outcome:

- `s5b` failed before extractor/finalizer: `read_exit:130`, missing required
  tail anchors for `subject.Score`, `sourcemix.FromTemplateMix`, and
  `stopcond.ShouldStop`.
- Mechanism metrics: `tool_read_file=24`, `explorer_iters=39`,
  `midloop_inject=7`, `extractor_iters=0`, `finalizer_iters=0`.
- The first explore window reached around 58k estimated tokens and pruned tool
  history. The local model then hit stream stalls twice and the orchestrator
  retried exploration as transient.
- During the first window, one lane discovered the 25
  `internal/analysis/*` subdirectories from `list_files`, but continued
  reading files in 6-file batches instead of emitting a complete
  `aggregate_facts.member_set`.
- A sibling lane repeated the same inventory work (`grep x25`, then
  `read_file x6`). After transient retry, exploration restarted from the same
  broad directory enumeration again.

Root-cause hypothesis to validate in code:

- Batch 2 advisory is produced only after a successful
  `emit_investigation_complete`/handoff. It helps downstream, but cannot prevent
  explorer from doing broad inventory repeatedly before closure.
- Multi-lane exploration and transient retry do not share a bounded,
  graph-backed inventory candidate artifact early enough. They preserve some
  accumulated evidence on salvage, but not a reusable "scoped directory +
  principal member candidates + missing attribute rows" work plan that can stop
  sibling lanes from redoing the same list/grep/read loop.
- Mid-loop hints correctly detect "read without emit_evidence", but they are
  textual nudges. Local models may ignore them and continue navigation; the
  system needs a deterministic, typed retry/fork handoff rather than more prose.

High-priority follow-up:

- [x] Locate the DAG explore transient retry and fork merge paths that discard
  or underuse inventory-progress state.
- [ ] Promote source-inventory advisory construction earlier, from successful
  structured tool observations such as `list_files`/`repo_map`, not only after
  accepted completion.
- [ ] Make sibling explore lanes and transient retries see the same advisory
  candidate artifact so they can avoid repeating broad directory enumeration.
- [ ] Preserve model authority: the artifact must remain advisory and typed;
  it can guide tool choice / missing-member coverage, but must not decide the
  user's answer or hard rewrite model prose.

2026-05-25 update:

- Deep root cause confirmed in `retryReadStageDispatchError`: read-mode
  stream-level failures requeued the whole explore window without first
  checking whether the explorer had already passed the typed
  `emit_investigation_complete` closure contract. The same class exists across
  dispatch stages whenever an LLM call fails after a structured tool result has
  already landed.
- Commercial boundary implemented for every read-stage dispatch handoff:
  analyzer preserves a usable `AnalysisIR`, explorer preserves accepted
  closure/evidence, extractor preserves reusable Turn-B symbols/verdicts, and
  finalizer preserves a non-degraded structured answer document before any
  retry is attempted. No user-question keyword matching, no model-prose
  parsing, and no deterministic answer supplementation are involved.
- No-progress transport failures still use the existing transient retry budget
  and requeue path. This preserves the intended recovery behavior for genuine
  network/model blips before the model produced durable structured progress.
- The stream-level family covered here is `io.EOF`, `io.ErrUnexpectedEOF`,
  `ErrStreamStalled`, `ErrStreamFirstByteTimeout`, `context.DeadlineExceeded`,
  and `net/url` transport errors. HTTP 429/5xx remain excluded because adapter
  L1 already owns that retry budget; malformed tool JSON and validator rejects
  are tool/content feedback, not dispatch failures.
- Guarded by `TestRunTaskGraph_RetryableAnalyzeErrorAfterUsableIRDoesNotReanalyze`,
  `TestRunTaskGraph_RetryableExploreErrorRequeuesWindow`,
  `TestRunTaskGraph_RetryableExploreErrorAfterAcceptedClosureDoesNotReexplore`,
  `TestRunTaskGraph_RetryableExtractErrorAfterTurnBSlateDoesNotReextract`,
  and `TestRunTaskGraph_RetryableFinalizeErrorAfterUsableAnswerDoesNotRewrite`.

### P0 Design - Pre-completion Source Inventory Progress

Root cause from the `s5b` run:

- The analyzer already emitted a high-confidence typed
  `source_inventory_profile` with package/function target roles and a bounded
  `internal/analysis` scope.
- The graph-backed `SourceInventoryAdvisory` carrier existed, and parallel
  fork/merge already preserved it, but the carrier was only published after a
  successful `emit_investigation_complete`.
- The local model failed before closure, so both parallel lanes and the
  transient retry had no shared structured candidate checklist and repeated
  directory enumeration.

Commercial-grade boundary:

- Publish a pre-completion advisory from typed request data and validated tool
  observations (`repo_map` / `list_files`) as soon as the current run has
  `AnalysisIR` plus a repo-map graph.
- Keep this early artifact `advisory_only=true`. It is not answer text, not a
  citation, not a completion signal, and must not trigger deterministic system
  supplement tables.
- Use the existing source-inventory reconciler and `SourceInventoryAdvisory`
  clone/merge path. Do not introduce a second inventory carrier.
- Support all repo-map languages by using graph files/symbol kinds rather than
  Go-specific parsing. Go-only enum proof remains the existing narrow adapter.
- Preserve model authority: explorer may use the checklist to avoid re-listing
  the same scope or to decide which candidates still need reading, but the model
  still decides when to emit evidence and how to close the investigation.

Implementation tasks:

- [x] Add `PublishSourceInventoryAdvisoryFromTypedRequest` for the post-analyze,
  pre-explore point. It builds the same advisory as the completion path, forces
  `advisory_only=true`, and stores it on `MutableState`.
- [x] Add `PublishSourceInventoryAdvisoryFromToolObservation` for successful
  `repo_map` / `list_files` observations inside explore. It only publishes once
  per run unless the stored advisory was inactive, and returns a compact
  model-visible hint for the same ReAct dispatch.
- [x] Add language-neutral package/directory candidates for
  `AnswerCandidateRolePackage`, derived from repo-map `FileInfo` scope groups,
  so path-scoped package inventories do not depend on Go package symbols.
- [x] Render the active advisory in explorer retry/fresh instructions as
  "structured candidate checklist", explicitly not final answer text.
- [x] Regression tests:
  - typed request publication is advisory-only and cross-language;
  - tool observation emits the compact hint only on first activation;
  - explorer prompt shows advisory context without changing answer authority.
- [x] Keep typed source-inventory dimensions in one explorer dispatch instead
  of splitting them into sibling lanes. The scheduler change is gated only by
  `source_inventory_profile.Active()` plus a multi-evidence window; it avoids
  duplicate enumeration but does not decide the answer.

Related guardrail still open:

- System supplement tables must not become a competing answer surface when
  upstream evidence / accepted aggregate facts already carried a complete
  model-collected set. Future work should prefer omission over an over-broad
  supplement for ambiguous enumeration carriers; any system-added text must be
  localized, append-only, and clearly marked as system-provided support.

Validation note:

- A post-advisory `s5b` run (`s5b-20260524-192902`) confirmed the first half of
  the fix: `pre-explore source-inventory advisory published` appeared before
  explore, and read volume dropped from 24 files to 7 before the run was
  interrupted for further code work.
- The same run showed both sibling lanes still repeated `repo_map` /
  `list_files` against `internal/analysis`. That exposed the second half of the
  root cause: source-inventory dimensions must be scheduled as one shared
  inventory dispatch, not as independent parallel lanes.

### Eval Observation - Profile-Omitted Source Inventory Shape

Run:

- Command:
  `EVAL_RESULTS_ROOT=eval/results/local-small-20260524-post-unified CODRAX_BIN=./codrax bash eval/run.sh eval/cases/s5b.case 1`
- Result directory:
  `eval/results/local-small-20260524-post-unified/s5b-20260524-193249`
- Result:
  interrupted after collecting enough scheduling evidence (`read_exit:130`).

Observed improvement:

- Read volume stayed much lower than the original run:
  `tool_read_file=6` before interrupt versus 24 in the first run.

New root cause:

- This analyzer emission omitted the optional `source_inventory_profile` field
  even though the rest of the structured IR still described a bounded category
  enumeration:
  - `intent=enumerate`
  - `predicates.is_category_enumeration=true`
  - 25 suggested source files under `internal/analysis/*`
  - two evidence siblings representing inventory dimensions
- Because the scheduler guard was keyed only to
  `source_inventory_profile.Active()`, the protection did not fire. The
  explorer still split into sibling lanes and both lanes repeated directory /
  symbol discovery.
- This is a structural small-model compatibility gap, not a prompt problem:
  the system must degrade gracefully when a helpful optional field is missing.

Batch 3 design:

- Add a scheduler-only fallback for typed bounded source enumerations:
  category enumeration + a sizeable set of source/config required files under
  one non-root directory + a multi-evidence ready window.
- Keep this fallback narrower than source-inventory profile activation:
  it only coalesces explore lanes; it does not publish candidate rows, does not
  mark completion, does not synthesize final-answer members, and does not relax
  validation.
- Use existing language-neutral path utilities:
  `types.CanonicalRequiredFileHintPath`,
  `types.HasCodeOrConfigPathSuffix`, `types.ClassifySourcePathRole`, and
  `types.SourceScopeAllowsPathRole`.
- Do not inspect raw user text or model prose. The activation source is typed
  IR plus deterministic path roles.
- Preserve the model's suggested order by leaving the ready window intact,
  instead of splitting sibling evidence nodes into competing lanes.

Batch 3 tasks:

- [x] Add `broadSourceEnumerationScopeActive` as a conservative scheduler
  fallback when `source_inventory_profile` is absent.
- [x] Gate it on typed enumerate/category flags, bounded required-file scope,
  path-role scope, and a multi-evidence window.
- [x] Add regression coverage for active profile, profile-omitted bounded
  enumeration, non-enumeration silence, root-level unbounded silence, and
  out-of-scope auxiliary silence.
- [x] Move production explore scheduling to `exploreWindowDispatchGroups(ctx,
  window)` and test that the final dispatch grouping, not just the lower-level
  predicate, keeps profile-omitted bounded enumerations unified while ordinary
  multi-topic evidence still splits.

Validation:

- `s5b-20260524-194057` was interrupted after the scheduling signal was
  confirmed. The run used a profile-omitted analyzer shape and produced one
  explorer dispatch (`explorer_dispatches=1`) with no `第 1 路 / 第 2 路`
  sibling-lane fan-out in the REPL log.
- Read volume before interrupt was `tool_read_file=5`, down from 24 in the
  original run and 6 in the previous profile-omitted run. This does not prove
  the full answer is solved, but it verifies the high-risk duplicate-lane
  failure mode is guarded.

### P0 Design - Cross-language Inventory Attribute Candidates

Problem:

- The current advisory carrier is role-flat: it can say "these are package
  candidates" and "these are function candidates", but it cannot say "this
  package candidate has these possible callable/entry attributes".
- `s5b` is one visible sample, but the gap is broader: directory/package/module
  inventories often ask for a per-member attribute (entry function, purpose,
  registration point, handler, config anchor, etc.). If the system hands the
  model only a flat set, local models still explore one package at a time with
  grep/read loops.
- A real "entrypoint" is not a universal language concept. The system must not
  pick a single answer by name-pattern or localized request keywords. It can
  safely expose graph-backed callable candidates and preserve ambiguity.

Design:

- Extend `SourceInventoryAdvisoryCandidate` with an `Attributes` side lane.
  Each attribute is another graph-backed candidate row with role, member,
  file, line, language, support_ref, exported flag, and note. The attribute is
  context for its parent member, not an answer row by itself.
- Populate package/directory/module candidates with bounded callable
  attributes from repomap symbols inside the same directory scope:
  `function`, `method`, and callable-like language adapter kinds such as
  `ctor`, `foreign-func`, `operator`, `ui-entry`, `builder`, and `rpc`.
- Do not infer a unique entrypoint. For each package member, keep a small,
  source-order candidate list. Multiple attributes intentionally mean
  "ambiguous candidates; model must verify or disclose".
- Activation stays inside the existing source-inventory advisory path. No raw
  user-question or model-prose keyword matching is introduced. The attribute
  lane is only emitted when a source-inventory advisory is already active from
  typed profile / advisory profile / tool observation.
- Profile-omitted fallback uses the shared typed
  `HasBoundedSourceEnumerationScope` guard. The same helper drives explore
  scheduling and advisory activation, so a local model that omits optional
  `source_inventory_profile` but emits a bounded category-enumeration IR still
  receives a checklist without reopening duplicate lanes.
- If a typed `answer_role_profile` is present, bounded fallback reuses those
  roles instead of substituting a package view. Package candidates are only the
  no-role fallback.
- The no-role fallback renders the existing package role as
  `package/directory/module scope candidates` so operators and future
  developers do not mistake it for a language-package answer decision.
- Keep validation unchanged: attributes are not citations by themselves and do
  not authorize deterministic final-answer supplements.
- Support every repomap language by consuming `repomap/types.Symbol` and
  `FileInfo.Language`, not language-specific parsers. Existing Go-only enum
  proof remains a separate narrow adapter.

Task list:

- [x] Add an attribute slice to `SourceInventoryAdvisoryCandidate` with clone
  and merge support.
- [x] Build bounded callable attributes for package/directory candidates from
  existing graph symbols.
- [x] Render attributes in explorer/extractor advisory blocks without making
  them look mandatory or final.
- [x] Add cross-language tests covering Python/Java/ArkTS/Proto/Cangjie-style
  callable kinds and ambiguity preservation.
- [x] Move bounded source-enumeration scope detection into `internal/types` and
  reuse it from both explore-window scheduling and source-inventory advisory
  activation.
- [x] Add profile-omitted fallback coverage where only
  `EvidencePlan.RequiredFiles` carries the bounded source scope.
- [x] Add coverage that bounded fallback respects model-supplied typed roles
  when `answer_role_profile` is present.
- [x] Add display-level guardrails so package-role advisory rows are presented
  as scope carriers rather than final answer type decisions.
- [x] Attach the compact advisory checklist to the first eligible navigation
  tool result even when the advisory was already published before explore
  dispatch. This keeps the hint adjacent to the model's actual repo_map /
  list_files observation without repeating it on every tool call.
- [x] Add a graph-resolved source-scope fallback for role/profile/required-file
  omissions: typed category enumeration plus a source entity that resolves to
  a repo-map scope now activates advisory-only scope candidates.
- [x] Keep that source-scope-only fallback silent when the typed model already
  declares an explicit completeness obligation but omits the target role; in
  that shape a package/directory fallback could become misleading noise.
- [ ] Re-run targeted source-inventory tests and a short `s5b` observation.

Validation so far:

- `go test ./internal/types -run 'TestSourceInventory|TestHasBoundedSourceEnumerationScope'`
- `go test ./internal/tool -run 'TestBuildSourceInventoryAdvisory|TestPublishSourceInventoryAdvisory|TestNormalizePrincipalEnumeration|TestReconcileCompletionAggregateFactsWithSourceInventory'`
- `go test ./internal/agent -run 'TestExplorer_BuildInitialInstruction_RendersSourceInventoryAdvisory|TestRenderExtractorSourceInventoryAdvisory_RendersCandidateAttributes'`
- `go test ./internal/orchestrator -run 'TestShouldKeepSourceInventoryExploreWindowUnified|TestExploreWindowDispatchGroups'`

2026-05-24 short `s5b` observation:

- Run: `eval/results/local-small-20260524-attribute-handoff/s5b-20260524-200839`
  interrupted with SIGINT after explore iteration 12.
- Positive: duplicate sibling lanes stayed fixed (`explorer_dispatches=1`), and
  pre-explore advisory publication fired.
- Gap found: because the advisory was already active, the tool-observation path
  suppressed the compact checklist; the local model proceeded with repeated
  repo_map/list_files/grep/read_file loops. This was fixed by adding a
  once-per-advisory lifecycle hint claim in `MutableState`.
- Follow-up run `eval/results/local-small-20260524-attribute-handoff3/s5b-20260524-201657`
  found a second upstream omission: analyzer retry kept only
  `entities=["internal/analysis"]` with no role/profile/required files. The
  source-scope fallback covers this by using graph-resolved entities, not raw
  request keywords.
- Follow-up run `eval/results/local-small-20260524-attribute-handoff4/s5b-20260524-202124`
  did not reach the explore advisory verification point. The analyzer used the
  terminal emit-only budget turn for more `list_files` calls, received tool
  rejections, and the run was interrupted. Remaining gap: analyzer terminal
  pre-scan recovery should avoid spending another LLM turn on rejected
  navigation calls when enough typed pre-scan observations already exist.

### P0 Design - Analyzer Terminal Pre-scan Violation

Root cause:

- The tool schema was already narrowed to `emit_analysis`, but local providers
  can still return stale or text-recovered `repo_map` / `grep` / `list_files`
  calls from prior context. Runtime correctly rejects those calls, so the
  repository boundary is safe.
- `analyzerEvaluator.Observe` treated the rejection as an ordinary repairable
  tool error and injected another emit-only hint. For small models that cannot
  obey the terminal tool choice, this spends an extra LLM turn without adding
  any information.
- `MutableState.PrescanSummaryBlob()` is not a replacement for
  `emit_analysis`. It does not contain the complete required schema:
  confidence fields, all explicit semantic predicates, answer subject,
  profiles, completeness obligations, requested dimensions, and optional
  routing lanes. Therefore the system must not synthesize a full analyzer IR
  from pre-scan summaries.

Commercial-grade contract:

- Preserve the last-legal-round grace turn. When the analyzer reaches its
  pre-scan wall, it still emits the existing must-call-`emit_analysis` hint so a
  compliant model can provide the full typed schema.
- After that terminal instruction has been issued, any subsequent response in
  the same analyze dispatch that contains only rejected pre-scan/navigation
  calls is treated as a failed attempt, not as a reason to inject another
  identical hint.
- The failed attempt flows into `runAnalyzePhase`'s existing retry path. A retry
  still asks the model for the real `emit_analysis` payload with named
  tool-choice forcing. Only after the configured analyzer retry budget is
  exhausted does the orchestrator use the existing degraded recovery path.
- No prompt code changes, no user-prose keyword matching, no model-prose
  intent inference, and no deterministic answer synthesis are introduced.
  Pre-scan summaries remain advisory search leads for validation and later
  exploration, never final answer facts.

Task list:

- [x] Track whether a terminal emit-only instruction was already issued in the
  current analyzer dispatch.
- [x] On `analyzer_prescan_budget_reached` /
  `analyzer_terminal_emit_mode` repair results, inject at most one terminal
  correction hint before forcing the current attempt to stop.
- [x] Keep the first-hint behavior for rare states where a rejected terminal
  pre-scan arrives without a prior terminal instruction.
- [x] Add analyzer loop-control tests for both paths: first rejected terminal
  pre-scan gets one correction when no terminal instruction was sent; rejected
  pre-scan after the must-emit hint stops the attempt.

Validation:

- `go test ./internal/agent -run 'TestAnalyzer_Observe_(BudgetRejected|TerminalRejected|RetryTerminal)|TestAnalyzer_PrescanBudget_MustEmitHintOnLastLegalRound'`
- `go test ./internal/agent ./internal/orchestrator`
- `make build`
- Targeted `s5b` eval with the local model:
  `eval/results/local-small-20260524-analyzer-terminal/s5b-20260524-204201`
  returned PASS.

Post-validation observation:

- Analyzer is no longer the terminal blocker. The successful run emitted
  `emit_analysis` on the third analyzer iteration after the one-time terminal
  hint.
- The run still paid a high exploration cost: `read_file=39`,
  `explorer_iters=44`, and `explorer_dispatches=3`.
- `SourceInventoryAdvisory` was present in the explorer prompt and carried a
  rich checklist for `internal/analysis`, including all package/directory
  candidates and related callable attributes.
- The advisory is explicitly "not final answer text and not citations by
  itself", so the evidence contract still pushed the model to re-enumerate and
  read many files before closure. This is correct for safety, but it exposes
  the next P0: advisory context is not enough for broad source-inventory
  questions; the system needs a structured, auditable observation artifact.
- The model also claimed `26` subpackages while the member list held `25`
  entries; aggregate normalization corrected `member_set.value 26->25`. A
  source-inventory observation must make `count == len(members)` explicit and
  machine-checkable.

### P0 Design - Source Inventory Observation Lens

Root cause:

- The current `SourceInventoryAdvisory` is a useful navigation carrier, but it
  intentionally has no evidence authority. Explorer therefore cannot safely
  close an exhaustive source-inventory answer from the advisory alone.
- Prompt-visible advisory rows compete with broader workflow instructions,
  pre-scanned file rankings, and scheduler hints. Even when the advisory is
  rich and correct, local models often fall back to repeated `repo_map`,
  `list_files`, `grep`, `exec_command find`, and one-file-at-a-time
  `read_file` loops to satisfy the evidence contract.
- Re-dispatch after downgraded closure replays the same broad enumeration
  unless the completed candidate coverage is carried as a structured artifact
  across retries. This is not a Go-only problem; it applies to any language
  covered by repo-map symbols or filesystem inventory.

Commercial-grade contract:

- Add a generic source-inventory observation lens rather than a case-specific
  "entry-point function" shortcut. The lens consumes typed scope and model
  declared roles from existing IR (`source_inventory_profile`,
  `answer_role_profile`, bounded source enumeration traits, requested scopes),
  never raw user-prose keywords or model prose.
- The lens returns a compact, language-neutral candidate table:
  members, symbols, attributes, coverage state, ambiguity, count, and
  provenance. It should preserve model-provided scope/order when available and
  use stable graph order only as a tie-break.
- The lens is advisory for semantic choices but authoritative for mechanical
  observations it directly obtains: directory/file membership from filesystem
  scope, parser-backed symbol location from repo-map graph, and fallback search
  output when used. Downstream validators can treat those rows as auditable
  observations without pretending that the model has read each full file.
- Ambiguous rows remain explicit. If a package has multiple plausible
  entry/public/handler candidates, the system marks ambiguity and offers
  bounded `next_reads`; it does not pick a winner on behalf of the model.
- The artifact must carry `count`, `members`, and per-member support refs so
  `count/list` drift is caught before finalizer. A corrected count may be
  appended as system observation, but the answer text must still distinguish
  model-authored conclusions from system-added caveats.
- Existing `SourceInventoryAdvisory` remains the prompt-visible hint. The new
  observation should reuse the same candidate builder and merge logic, not
  duplicate source scanning or invent another ranking subsystem.

Proposed schema:

```json
{
  "scope": ["internal/analysis"],
  "lens": ["members", "symbols", "attributes"],
  "sets": [
    {
      "role": "package",
      "complete": true,
      "count": 25,
      "members": [
        {
          "name": "aggregator",
          "support_ref": "internal/analysis/aggregator",
          "language": "go",
          "attributes": [
            {
              "name": "New",
              "role": "function",
              "file": "internal/analysis/aggregator/aggregator.go",
              "line": 112,
              "ambiguity": "one_of_many_callable_attributes"
            }
          ]
        }
      ]
    }
  ],
  "provenance": ["filesystem_scope", "repomap_graph"],
  "next_reads": [
    {
      "file": "internal/analysis/aggregator/aggregator.go",
      "line_window": "108-132",
      "why": "confirm semantic role among multiple callable attributes"
    }
  ]
}
```

Task list:

- [x] Promote `SourceInventoryAdvisory` rows into a reusable
  `SourceInventoryObservation` artifact with count/member/support-ref fields.
- [x] Register the artifact in `MutableState` / `TurnAArtifacts` so it survives
  explorer retries and extractor/finalizer handoff.
- [x] Teach `emit_investigation_complete` member-set validation to accept
  observation-backed rows for mechanical source-inventory facts, while still
  requiring model evidence or explicit ambiguity for semantic choices.
- [x] Render the observation compactly in explorer and extractor prompts,
  preserving model order and showing ambiguity/next-read rows without forcing
  final-answer text.
- [x] Add cross-language tests using Go, Python, Java, TypeScript/ArkTS, Proto,
  Kotlin/Cangjie-style symbols to ensure the lens consumes repo-map symbols,
  not language-specific heuristics.
- [ ] Add an eval guard for `s5b` that tracks not only PASS/FAIL but also
  `read_file`, `explorer_iters`, and `explorer_dispatches`, so future changes
  cannot silently regress to high-cost enumeration.

### P0 Implementation Design - Repo Lens / Source Inventory View

Current code anchors:

- `internal/tool/repomap/types` already owns the cross-language graph contract:
  `Graph`, `FileInfo`, `Symbol`, `RankIndex`, `LineFeatures`, and the
  authoritative `SupportedReadLanguages()` matrix. The new lens must consume
  those typed fields directly and must not introduce language-specific scanners
  outside the existing repo-map parser/index layer.
- `internal/tool/source_inventory_reconcile.go` already builds
  `SourceInventoryAdvisory` from typed lanes:
  `SourceInventoryProfile`, `AnswerRoleProfile`, bounded source enumeration
  traits, and resolved source scopes. This is the single source of truth for
  candidate construction; the lens should project this carrier into a stronger
  observation shape instead of rebuilding candidates from scratch.
- `internal/types/observation_ledger.go` already provides the shared fact bus
  for current source, VCS, runtime artifacts, command output, web/MCP/connector
  data, and row-set refs. Source inventory should join this ledger as another
  structured observation input, not as ad-hoc prompt markdown.
- `internal/agent/explorer.go` and `internal/agent/extractor.go` already render
  compact source-inventory advisory hints. The new view should reuse that UX
  surface while making the stronger `count == len(members)` and
  `support_ref` contract explicit.

Non-negotiable semantics:

- Activation is typed-only. The lens may activate from
  `source_inventory_profile`, `answer_role_profile`, typed bounded source
  enumeration traits, and graph-resolved scope entities. It must never parse
  raw user prose or model free text to decide that a question is an inventory.
- Model order wins. If the model supplied typed target roles or scopes, preserve
  that order. Graph sort/rank is only a deterministic tie-break inside an
  otherwise model-declared lane.
- Mechanical observations and semantic choices are separated. The system may
  observe "directory `x` is inside scope", "symbol `Foo` exists at
  `a/b.py:17`", and "there are N listed members". It must not decide that
  `Foo` is the user's requested entrypoint, owner, or causal mechanism unless
  that role is already typed/unambiguous and later validated.
- Repo-map navigation must not become hidden final-answer authorship. Any
  system-added correction, count normalization, or ambiguity note must remain
  visibly system-authored and localized; normal model-authored evidence and
  answer text remain the primary output.
- Fallbacks must be loss-preserving. If a set is too large, write a row-set ref
  and a visible compact summary; do not silently truncate the underlying
  member list. If a row is ambiguous, carry the ambiguity and next-read hint
  rather than dropping it or picking a winner.

Artifact schema:

- `SourceInventoryObservation`
  - `active`, `advisory_only`, `complete`
  - `scopes`, `provenance`, `lens`
  - `sets []SourceInventoryObservationSet`
- `SourceInventoryObservationSet`
  - `role`, `complete`, `count`
  - `members []SourceInventoryObservationMember`
  - invariant: `count == len(members)` after normalization
- `SourceInventoryObservationMember`
  - `name`, `key`, `support_ref`, `note`
  - `role`, `file`, `line`, `language`, `exported`
  - `coverage_state`: `observed`, `ambiguous`, `needs_read`, `no_index`
  - `attributes []SourceInventoryObservationAttribute`
- `SourceInventoryObservationAttribute`
  - same location/role fields as a member
  - `ambiguity` and `reason` fields for "one of several plausible callables"

Ledger projection:

- Member rows with an exact `file:line` become `current_source` observation
  records with `GroundingPolicy=repairable`, `EvidenceScope=line`, and
  `ClaimKey=name`. These are eligible as mechanical support refs but are not
  automatically rendered as final answer claims.
- Directory/file/package rows without exact line spans become path-scoped
  `current_source` observations with `GroundingPolicy=soft`; they can support
  bounded member-set/count checks but cannot satisfy line-citation-only gates.
- Large sets should use the existing row-set writer pattern from
  `ObservationLedgerInput.RowSetWriter` so the prompt can stay compact while
  downstream checks still have the full list.

View/tool strategy:

- P0 does not add a new model-facing tool. It strengthens the existing
  `SourceInventoryAdvisory` lifecycle into a typed observation artifact and
  renders it where advisory hints already appear. This avoids adding schema
  burden while immediately reducing repeated broad enumeration.
- P1 adds model-driven querying via `repo_map(view="source_inventory")`. This is
  important for commercial use because the model must be able to ask for the
  scope and role slice it wants instead of being locked to the analyzer's first
  guess. The view remains a thin projection over the same observation builder,
  not a second implementation.
- The active query parameters should stay small and typed:
  `path`/scope, `query` as an optional ranking hint, `roles` as a list of
  `AnswerCandidateRole` values, `include_attributes`, `include_counts`,
  `top_n`, and later `cursor` for large row sets. When `roles` is omitted, the
  view uses the current typed request roles. When `path` is omitted, it uses the
  already-resolved request scopes. No raw prose classifier is added.
- Tool output must label itself as "repo lens / source-inventory observation
  checklist", carry `count == len(members)`, preserve model-specified role/scope
  order, expose ambiguity and bounded next-read suggestions, and keep the
  existing `repo_map` warning that semantic facts still require source evidence.

Batch plan:

- [x] P0-A: add `SourceInventoryObservation` types, clone/merge/count-normalize
  helpers, `MutableState`/`TurnAArtifacts` preservation, and unit tests. This
  is behavior-neutral except for storing a stronger artifact.
- [x] P0-B: project `SourceInventoryAdvisory` into `SourceInventoryObservation` at
  the same publication points (`pre_explore_typed_request`, `repo_map`,
  `list_files`, accepted completion), then render compact count/support rows in
  explorer/extractor prompts.
- [x] P0-C1: compile source-inventory observations into `ObservationLedger`
  with count/member/support-ref rows and exact file:line spans when available.
- [x] P0-C2: add validation support for mechanical member-set/count coverage.
  This must only accept rows that exactly match the observation name/support-ref
  and must not auto-pick semantic attributes.
- [x] P1-A: add optional `repo_map(view="source_inventory")` as a thin view over the
  same artifact, with tests proving it respects supported languages and scoped
  path safety.
- [x] P1-B: add cursor/offset paging for very large model-driven source
  inventory views. Tool results may render a bounded page while the structured
  observation in run state and the ledger row-set ref preserve the full member
  list.
- [x] P2: add eval/cost guardrails for broad inventory samples (`s5b` and at least
  one non-Go repo-map-supported language fixture) measuring final answer
  quality plus `read_file`, `repo_map/list_files`, `explorer_iters`, and
  redispatch count.

Implementation notes:

- 2026-05-24: landed P0-A/P0-B/P0-C1/P1-A. `repo_map` now accepts
  `view="source_inventory"` with typed `scope`/`scopes`, `roles`,
  `include_attributes`, `include_counts`, and `top_n` parameters. The view is
  model-driven and advisory-only; it stores the same observation in mutable run
  state and renders a compact checklist with count invariants.
- 2026-05-24: landed P0-C2. `emit_investigation_complete` member-set support
  validation now accepts source-inventory observation rows only when the model's
  emitted `support_refs` exactly match the observation support ref and the
  member label matches the observed row. This supports path-only package rows
  and exact file:line symbol rows without letting the system auto-pick semantic
  attributes.
- 2026-05-24: landed P1-B and cross-language role coverage hardening.
  `repo_map(view="source_inventory")` now accepts `cursor`/`offset` for paged
  checklist output; the underlying `SourceInventoryObservation` remains full
  fidelity. Symbol-kind projection was widened over the existing repomap parser
  kinds (`rpc`, `ui-entry`, `builder`, `operator`, `foreign-func`, `ctor`,
  ArkTS state fields, Proto service/message, Swift actor/protocol, Kotlin
  object/data/sealed/annotation, import symbols, etc.) so direct role queries
  and package attributes share one language-neutral mapping.
- 2026-05-24: multi-repo and configuration-file handling are guarded in the
  same source-inventory lens path. The view runs after the existing active-set
  hard gate, so a model can query only an active sub-repo path and cannot use
  the lens to scan a parent workspace or inactive sibling. Explicit `scope="."`
  is treated as "the active repo root" rather than an answer evidence path, and
  every returned row still passes per-file source-scope checks. Config files
  are first-class navigation rows via `config_file`; ordinary `file` rows also
  include repomap `IsSpecial` files. `config_key` remains conservative: it is
  returned only when the repomap graph already exposes a structured symbol row,
  never by guessing YAML/TOML/XML semantics from file names.
- 2026-05-24: path-role classification now treats common project descriptors
  (`CMakeLists.txt`, `Makefile`, `Dockerfile`, `Jenkinsfile`, `meson.build`,
  plus extension-based config files) as production config surfaces unless a
  higher-priority structural directory role such as `tests/`, `fixtures/`,
  `examples/`, or `docs/` applies. This prevents build manifests from being
  filtered as documentation merely because their basename ends in `.txt`.
- 2026-05-24: eval guardrails now count control-plane tool calls with
  `eval_count_tool_calls`, exposing `tool_repo_map`, `tool_list_files`, and
  `source_inventory_lens` metrics without being polluted by model prose. The
  convergence audit includes `s5b` plus non-Go Harmony ArkTS and Cangjie cases
  and flags wide-search regressions when read/list/repo-map calls exceed the
  configured thresholds.
- 2026-05-24 post-push real eval (`s5b`, local small model) exposed an
  ergonomics gap: the model naturally called
  `repo_map(path="internal/analysis/aggregator", view="source_inventory",
  roles=["function"])` without an explicit `scope`, but the lens fell back to
  analyzer/profile scopes and returned the broad `internal/analysis/*` function
  table. This is a tool-contract issue, not a prompt issue. Fixed by deriving a
  default lens scope from the model-supplied `repo_map.path` only when
  `scope/scopes` are absent; explicit model scopes still win. The regression
  test `TestRepoMapSourceInventoryViewPathDefaultsToScope` pins that subdir
  path queries do not leak sibling package members.
- 2026-05-24 scoped rerun exposed the sibling root cause for non-lens views:
  in single-repo MultiGraph posture, `GraphFromBusContextOrLoad` always returned
  `mg.Single()` before considering the requested repo-map path, so
  `repo_map(path="internal/analysis", view="file_map")` still rendered the
  whole repository. The fix keeps byte-equivalent `mg.Single()` reuse for repo
  root requests, but routes subdirectory requests through the scoped graph
  loader. This is a general repo-map path contract fix, not a question-specific
  special case; it reduces analyzer/explorer context noise before the model ever
  reaches the source-inventory lens.
- 2026-05-24 post-fix eval (`s5b`, local small model, 900s cap) confirmed the
  subdirectory root fix: `repo_map(path="internal/analysis", view="overview")`
  and `view="file_map"` built the `analysis` graph only (95 files, 94 source
  files), not the full repo. The run still timed out during exploration before
  finalization. The remaining bottleneck is now one layer deeper: the model did
  not select `repo_map(view="source_inventory")`; it used `overview`/`file_map`,
  then repeatedly guessed per-package entry filenames, issued whole-file
  `read_file` calls, hit path misses, and triggered a second exploration pass.
  This is not a JSON-recovery issue. It shows the source-inventory lens must be
  made easier to discover and must expose the right shape for "member + candidate
  entry symbols" inventories without requiring the model to learn a separate
  strategy. Any fix must stay advisory-only: it can offer a compact checklist
  and exact count invariants, but must not turn navigation candidates into final
  answer facts without `read_file`/grounded evidence.
- Same eval also showed that analyzer emitted
  `source_inventory_profile.target_roles=["file"]` for a directory-plus-entry
  request. That preserved the bounded-inventory shape but not the nested
  "directory member -> entry function" relation the explorer needed. The next
  design batch should extend the typed inventory contract with composable
  relation slots (for example member role + requested attribute role) instead of
  hard-coding Go packages or a particular eval question. It must work over all
  repomap languages, mixed-language subtrees, config-file surfaces, and active
  multi-repo boundaries.

Next batch design: scoped repo-map projection + composable source-inventory
relations.

Problem A: scoped repo-map calls currently avoid the earlier full-repo noise
bug, but they do so by loading a separate subdirectory graph/cache. On a cache
hit this is cheap, yet it still repeats file listing, hash validation, graph
build, and ranking when an in-memory whole-repo graph already exists. The safe
commercial fix is not to blindly reuse the whole graph. Add a projection path
that runs only after the existing active-set / symlink / parent-escape gates,
filters the already-loaded graph to the requested subtree, rebases file paths
to the scoped root, keeps only in-scope symbols/imports/relations, rebuilds the
graph indexes with the existing `index.BuildGraph`, ranks the clone with the
current query, and caches the projection for the current run under
`canonical(parent_root) + canonical(scope_root) + query`. This preserves the
boundary fix while avoiding repeated disk-cache graph construction. The
projection must not replace the full `Mutable.SearchGraph`; downstream
validators often need the full graph. It should be a repo-map execution cache,
not a global semantic authority.

Problem B: `source_inventory` already carries row-local attributes, but the
model has to discover the exact `view="source_inventory"` shape and the current
attributes are too generic for questions that ask for a two-level inventory
such as "directory members plus each member's entry function". The next lens
extension should add model-controlled, typed relation slots rather than
case-specific prompts: `member_roles`/`roles` continue to define principal
rows, while `attribute_roles` requests row-local related candidates such as
function/method/type/config_key. If omitted, existing behavior remains intact.
For directory-like members (`package`, `file`, `config_file`), attributes are
selected from the same scope and ranked by structural signals already available
in repomap: exported/public first when visibility asks for it, parser rank
quality, file locality, symbol kind, line order, and graph score. The tool must
return enough rows to be useful but label them as candidates/ambiguous when
there are multiple possible entries. It must never assert "the entry point is X"
without grounded evidence; it only gives a compact checklist of where the model
should verify next.

Batch tasks:

- [x] B2-A: add run-scoped repo-map projection cache on `MutableState`, keyed by
  parent root, scoped root, and query, returning defensive graph clones.
- [x] B2-B: implement `ProjectGraphToRoot(parent, scope)` by reusing
  `index.BuildGraph` so all graph indexes, import resolution, implementer
  metadata, and ranking inputs stay single-source. Add tests that the projection
  rebases paths, excludes sibling files/relations, and does not mutate the
  full graph.
- [x] B2-C: wire single-repo subdirectory `repo_map` calls through the projection
  cache when a full in-memory graph is present; fall back to the current scoped
  disk cache path when no full graph exists.
- [x] B2-D: add source-inventory lens query fields for row-local
  `attribute_roles` while keeping existing `roles` behavior and model learning
  burden small. Render attributes as advisory "candidate attributes" with
  support refs and ambiguity notes.
- [ ] B2-E: update eval guardrails so `source_inventory_lens` use and
  full-file `read_file` count can distinguish "subdir graph reuse works" from
  "model still did manual broad reads".
- [ ] B2-F: rerun `s5b` and record whether the eval now enters finalizer, how
  many `read_file`/`list_files`/`repo_map` calls remain, and whether the model
  naturally uses the lens or still needs discoverability work.

Implementation notes for B2:

- 2026-05-24: landed run-scoped graph projection without replacing the full
  `Mutable.SearchGraph`. Subdirectory `repo_map` calls first try a cached
  projection derived from an already-loaded full graph; only cache misses fall
  back to the existing safe scoped loader. Projection filters and rebases files
  and source-local symbol/relationship fields, then calls the existing
  `index.BuildGraph` and `retrieve.RankGraph`, so graph indexes and language
  behavior remain single-source.
- 2026-05-24: landed `attribute_roles` for `repo_map(view="source_inventory")`.
  The field is typed and model-driven: rows still come from `roles`, while
  row-local candidate attributes can request functions, methods, types,
  config keys, and other existing candidate roles. File, config-file, and
  package/module rows can now expose bounded candidate attributes without
  turning them into final facts.
- Tests added/updated:
  `TestSourceInventoryObservation*`,
  `TestCompileObservationLedger_SourceInventoryObservation`,
  `TestPublishSourceInventoryObservationFromLens_ModelDrivenRolesAndScopes`,
  `TestPublishSourceInventoryObservationFromLens_UsesCrossLanguageRepoMapKinds`,
  `TestAggregateMemberSetMemberUsableWithSourceInventoryObservationSupportRef`,
  `TestRepoMapSourceInventoryViewModelDrivenQuery`,
  `TestRepoMapSourceInventoryViewSameRepoCrossLanguage`,
  `TestRepoMapSourceInventoryViewConfigFilesAreNavigationRows`,
  `TestRepoMapSourceInventoryViewPathDefaultsToScope`,
  `TestRepoMapSourceInventoryViewMultiRepoHonorsActiveSubRepoScope`,
  `TestRepoMapSourceInventoryViewMultiRepoConfigFilesStayScoped`,
  `TestRepoMapSourceInventoryViewMultiRepoRejectsInactiveSubRepo`,
  `TestGraphFromBusContextOrLoadSingleRepoHonorsSubdirRoot`,
  control-plane eval counter tests, and prompt rendering tests for
  source-inventory count invariants.

Follow-up finding from B2 eval, 2026-05-24 22:59 CST:

- The scoped graph projection and run-level cache worked: `repo_map` for
  `internal/analysis` projected from the in-memory full graph and reused the
  scoped projection on the next call. The remaining failure mode shifted to the
  shape of the source-inventory output. The model called
  `repo_map(view="source_inventory", roles=["function"], attribute_roles=["function"])`
  over many scopes. The tool returned a mechanically correct flat set with
  hundreds of functions, but the first visible shape did not guide the model
  toward "one scope -> candidate entries -> verify the right file". The model
  therefore fell back to whole-file reads. This is not an enumeration-only
  issue; the same broad flat-list problem affects mechanism questions,
  architecture/component explanations, entry/handler/route discovery, config
  exploration, mixed-language trees, and multi-repo scoped work.

Design B2-G: source-inventory grouped navigation projection.

- Keep the existing `SourceInventoryObservation` as the complete, auditable
  carrier. Do not replace or truncate it. `count == len(members)` remains the
  machine-checkable invariant and downstream validators still require normal
  grounded evidence before user-visible claims.
- Add a render-time, advisory-only grouping layer for `repo_map` source
  inventory results. It is triggered only from structured tool parameters and
  structured rows: active source-inventory observation, broad/multi-scope
  candidate set, symbol/config/route/function/type-like roles or explicit
  `attribute_roles`, and first page rendering. It must not inspect the user's
  raw question or the model's prose.
- Group candidates by the model-provided `scopes[]` order when present. If the
  model provides one broad scope, derive child groups from repo-relative file
  paths under that scope. This preserves model-driven search order and avoids a
  system-authored intent decision.
- Render each group as navigation, not answer text:
  scope, candidate_count, role/language distribution, and a bounded list of
  top candidate rows with file:line. Labels must say "advisory" / "verify next"
  and must not say "entry point is".
- Preserve the original flat member rows with pagination. When grouped rows are
  rendered and the model did not request a custom `top_n`, reduce the flat
  preview to a smaller page so the grouped view is visible first without
  deleting the full structured set.
- Generality requirements:
  - mechanism/call-chain: use grouped function/method/type candidates to choose
    which files to read next;
  - architecture/component: group package/file/type candidates by component
    boundary before evidence collection;
  - entry/handler/route: show per-scope candidate handlers/routes, explicitly
    ambiguous when multiple candidates exist;
  - config: group config files and config keys by directory/file without
    assuming a language;
  - multi-language and same-repo cross-language: use repomap language/kind
    metadata, not Go-specific naming;
  - multi-repo: grouping happens only after active-set scoped repo-map gates,
    so inactive sibling repositories cannot leak rows.

Tasks:

- [x] B2-G1: implement a render-time grouped source-inventory projection that
  groups broad candidate rows by structured `scopes[]` or derived child scope.
- [x] B2-G2: render grouped rows before the flat list with explicit
  advisory-only wording, role/language counts, and bounded top candidates.
- [x] B2-G3: keep original flat rows paged and intact in run state; reduce only
  default first-page preview when grouped rows are present.
- [x] B2-G4: add tests for multi-scope symbol roles, broad single-scope child
  grouping, cross-language roles, and config/config-key grouping.
- [x] B2-G5: rerun focused `s5b` eval and compare `read_file` count,
  `source_inventory` usage, finalizer entry, and answer completeness.

Implementation notes for B2-G:

- Added a render-time `Scope-grouped Candidate View (advisory)` to
  `repo_map(view="source_inventory")`. It uses only typed tool parameters and
  structured observation rows. It preserves model-provided `scopes[]` order; for
  a single broad scope it derives child scopes from repo-relative file paths.
- The grouped view is intentionally generic. It applies to functions, methods,
  types, constants, variables, fields, packages, files, config files,
  config keys, routes, import paths, and literal values. It is useful for
  enumeration, mechanism/call-chain navigation, architecture/component mapping,
  handler/route discovery, config inspection, and mixed-language scoped
  exploration.
- The first page now shows grouped rows before the flat list and reduces only
  the default flat preview when grouping is active. The full structured
  observation remains in run state and cursor paging still exposes flat rows.
- Fixed a related information-loss bug: source-inventory symbol candidate
  construction no longer deduplicates by symbol name alone. It now keeps
  same-named functions/types/routes/config keys at distinct file:line locations.
- Added route symbol kind normalization (`route`, `handler_route`,
  `handler-route`) so route inventories can use the same typed role path as
  config keys and callables.
- The pre-B2-G eval (`s5b`, local small model) reached finalizer with
  `source_lens=1`, `read_file=9`, `repo_map=4`, and no finalizer rejects, but
  failed semantic review because the model flattened ambiguous entry candidates
  and treated unresolved rows inconsistently. This is the direct validation
  target for the B2-G rerun.
- The B2-G rerun with `Qwen3.5-9B-OptiQ-4bit` confirmed that the grouped view is
  visible to the model and remains model-driven (`source_lens=2`, grouped rows
  rendered from the model's structured `roles` / `attribute_roles` / `scopes`
  request). It still failed the focused semantic verdict: the model produced a
  partial entry-function inventory and missed packages such as `normalizer`,
  `criterion`, `subject`, `sourcemix`, and `stopcond`; `read_file` increased to
  17. The failure path shows the next bottleneck is not JSON recovery or
  finalizer retry: after seeing grouped candidates, the model still guessed
  conventional files such as `<scope>/<scope>.go` and then repaired by grepping
  broadly. This is a generic navigation-shape gap.

Design B2-H: verified candidate file manifest and path-miss guidance.

- Keep the grouped source-inventory view advisory-only. Do not convert it into a
  hard read whitelist and do not block arbitrary `read_file` calls. The model
  remains free to inspect files outside the manifest when it has a reason.
- Add a visually prominent "Suggested files to verify" section derived only
  from structured source-inventory rows and their row-local attributes:
  `scope`, `role`, `attribute_role`, `file`, `line`, `language`, and
  `support_ref`. This is a verified candidate file manifest, not an answer
  slate. It should say "read these first when verifying this scope" rather than
  "these are the only valid files".
- Render the manifest for every repo-lens source-inventory shape, not just
  enumeration:
  - enumeration: scope -> verified files -> candidate members, preserving
    `count == len(members)` as the mechanical completeness invariant;
  - mechanism / call-chain: function/method/type files become the first
    verification windows before widening to callers/callees;
  - architecture / component: package/file/type rows provide real component
    boundaries and the files that describe them;
  - route / handler / config: route and config-key rows point to their owning
    manifest/source files without assuming a programming language;
  - external / artifact-assisted questions: repo-lens files remain repo-only
    navigation and must not replace log/trace/web/MCP evidence lanes.
- Preserve model intent and order: when the model supplies `scopes[]`, render
  files in that order; when a single broad scope is supplied, derive child
  scopes from repo-relative paths. Do not read raw user text or model prose.
- Make it language-neutral and multi-repo safe: files come from repomap rows
  after active-set, symlink, and parent-escape boundaries. The manifest must
  include Go, Java, JavaScript/TypeScript, Python, Rust, C/C++, Swift, Ruby,
  config files, routes, imports, and literals through existing
  `AnswerCandidateRole` metadata rather than filename heuristics.
- Attach bounded ambiguity: if a scope has multiple candidate files or multiple
  candidate symbols in one file, render counts and a small sample. Ambiguity is
  for the model to verify or disclose; the system must not pick the winner.
- Keep the section small and stable: cap groups, files per group, and candidate
  labels per file; rely on the stored `SourceInventoryObservation` for the full
  set. The manifest is a UX/navigation projection, not a new source of truth.
- Add a soft `read_file` path-miss hint using the same stored observation:
  when a requested path is missing and there are known source-inventory
  candidate files in the same structured scope, append a concise suggestion.
  This is recovery guidance only; it must not mark the attempted path forbidden
  and must not fire outside the active repository scope.
- Tests should guard the contract:
  candidate manifests render for function/method/type, route/config, and
  cross-language rows; model-supplied scope order is preserved; no suggestions
  are emitted for sibling/inactive repo paths; path-miss suggestions are bounded
  and do not change tool success/failure semantics.

Tasks:

- [x] B2-H1: render a bounded verified candidate file manifest from
  source-inventory grouped rows without truncating the underlying observation.
- [x] B2-H2: add same-scope path-miss suggestions to `read_file` using the
  stored source-inventory observation, preserving failure semantics.
- [x] B2-H3: add tests across functions/methods/types, route/config rows,
  same-repo mixed language, and multi-repo active-scope isolation.
- [ ] B2-H4: rerun focused `s5b` eval and compare `read_file` count,
  guessed-path misses, `source_inventory` usage, and final answer completeness.

Implementation notes for B2-H:

- Added the candidate-file projection to
  `RenderSourceInventoryObservationView`. It is derived from the same stored
  `SourceInventoryObservation` rows and row-local attributes used by the
  grouped view, so the full observation remains the single source of truth.
- Initially reused the same projection in the compact source-inventory advisory
  hint, because a model may follow the pre-explore checklist without explicitly
  calling `repo_map(view="source_inventory")` again. B2-I supersedes this
  ordering: compact hints now lead with a cascade guide and no longer put the
  file manifest first.
- The manifest groups by model-provided scopes first, or derived child scopes
  for a single broad scope. Each row shows verified candidate files, role and
  language counts, and a bounded candidate label sample. It stays advisory and
  explicitly says it is not a hard whitelist or final-answer evidence.
- The first B2-H implementation rendered the manifest before grouped candidate
  rows. Follow-up eval observation showed that this can over-emphasize file
  names and drown the more important self-service navigation shape. B2-I changes
  the primary UI contract to summary-first cascaded navigation while keeping the
  file manifest as bounded samples and path-miss recovery.
- `read_file` now appends a same-scope path-miss suggestion only when the active
  source-inventory observation contains verified candidate files under that
  exact structured scope. The tool result remains `Success=false`; sibling
  scopes and outside-repo paths do not receive suggestions.
- Tests cover source-inventory render output for function/method/type-like
  rows, config keys, routes, cross-language rows, and `read_file` same-scope
  path misses without sibling leakage. Existing multi-repo source-inventory
  active-set tests continue to guard inactive-repo isolation.

Design B2-I: summary-first cascaded source-inventory navigation.

- Problem: a prominent "Suggested files" block is safe as an advisory manifest,
  but it still encourages the model to jump straight to files. In broad source
  inventory, mechanism tracing, architecture, route/config, mixed-language, and
  multi-repo questions, that can drown the model's own intent-driven choice of
  which branch to expand. The first source-inventory call should teach the
  model how to ask the next narrower question, not pre-feed a large file list.
- New contract:
  - first render a compact, machine-checkable summary:
    `member_rows`, `scope_groups`, `candidate_files`, `candidate_items`,
    `ambiguous_groups`, role counts, attribute-role counts, language counts;
  - then render concrete follow-up `repo_map(view="source_inventory")` call
    shapes for narrowing one scope, paging the current checklist, and verifying
    after a narrower lens result;
  - then render bounded "suggested cascade expansions" per structured scope:
    scope, file/candidate counts, role/language distributions, and the next
    repo_map call to expand that scope;
  - only after the cascade guide and grouped view, render a small
    `Candidate File Samples to Verify` appendix. This preserves useful
    path/file breadcrumbs without letting file lists dominate the first screen.
- The guide is deliberately model-driven and advisory-only:
  - it uses only structured `repo_map` parameters (`path`, `scope(s)`,
    `roles`, `attribute_roles`, cursor/top_n) and structured observation rows;
  - it does not inspect raw user text or model prose;
  - it respects model-provided scope order and role order;
  - when role fields are omitted, it derives follow-up call roles from the
    already structured observation sets, not from keywords;
  - it never marks candidates as final answer members and never blocks reading
    other files.
- Generality:
  - enumeration: model expands the scopes it cares about and checks count/list
    invariants without reading every file;
  - mechanism/call-chain: model narrows to likely function/method/type scopes,
    then verifies with read/grep before citing;
  - architecture/component: model expands component scopes by package/file/type
    boundaries;
  - route/handler/config: model expands route/config-key owning scopes without
    language-specific filename guesses;
  - mixed-language and multi-repo: all rows come from repomap roles, languages,
    active-set boundaries, and source-inventory observations.

Tasks:

- [x] B2-I1: add `Cascaded Repo Lens Guide (advisory)` before grouped rows in
  `RenderSourceInventoryObservationView`.
- [x] B2-I2: remove the file manifest from compact advisory hints and replace
  it with the same summary-first cascade guide.
- [x] B2-I3: keep candidate files as a bounded `Candidate File Samples to
  Verify` appendix after grouped rows, plus the existing `read_file` path-miss
  recovery hint.
- [x] B2-I4: carry the structured repo_map `path` into
  `SourceInventoryLensQuery` so suggested follow-up calls can reuse the same
  repo/scope boundary without guessing.
- [x] B2-I5: update tests to guard cascade-before-grouped-before-file-samples
  ordering and compact-hint behavior.
- [ ] B2-I6: rerun focused eval and compare whether the model performs narrower
  source-inventory expansions before broad reads.

B2-I eval observation, 2026-05-24 23:49 CST:

- Focused `s5b` was intentionally stopped after enough signal to avoid wasting
  local eval time. The run did not reach final answer.
- The new cascade renderer was not exercised, because the model never called
  `repo_map(view="source_inventory")`.
- Instead, the analyzer/explorer repeated the older broad-search pattern:
  `repo_map(view=file_map)` → many `list_files` / `grep` calls → batches of
  `read_file`. By explorer round 8, tool history had already pruned and the
  run had read many large files while still missing structured evidence emits.
- The model also attempted shell loops for directory/function extraction. The
  read-mode shell safety gate correctly refused command substitution, but the
  recovery path fell back to even more broad grep/read_file work.
- Root cause: B2-I fixed the shape once `source_inventory` is used, but not the
  discovery path that gets the model to use it. The current tool description
  and generic exhaustive-enumeration guidance still make `file_map`,
  `list_files`, `grep`, and shell feel like the obvious first tools.

Design B2-J: structure-triggered Repo Lens discovery hints.

- Goal: make the source-inventory lens discoverable during broad navigation
  without changing user intent, without inspecting user/model prose, and without
  forcing a different control flow.
- Trigger only from structured runtime facts:
  - a successful `repo_map`, `list_files`, or `grep(files_only=true)` result is
    scoped to a repo-relative directory/package/module;
  - the result exposes multiple child scopes, many candidate files, or repeated
    navigation over the same scope;
  - the active typed request / advisory already has source-inventory-shaped
    roles, or the model's current tool parameters ask for broad symbol/file
    discovery;
  - the model is consuming read_file budget without emitting evidence.
- Do not trigger from raw current-request keywords or model prose. Chinese and
  English phrases such as "所有", "entry point", "handler", "目录" are not control
  signals here; only parsed tool/result structure is.
- Hint shape:
  - append a short advisory to the next tool result or mid-loop hint:
    "This result is broad. To inspect it incrementally, consider
    `repo_map(path=<same path>, view=\"source_inventory\", scope=<child>,
    roles=[...], attribute_roles=[...])`."
  - include only a summary plus 2-5 model-selectable next calls; no big file
    table in the hint.
  - keep wording advisory: "consider" / "if this matches the user's intent";
    never "must" unless the existing phase budget already requires stopping
    and emitting the current stage tool.
- Generality:
  - works for enumeration, mechanism/call-chain, architecture/component,
    route/handler, config, import/literal inventories, mixed-language trees,
    and multi-repo scoped work;
  - roles come from structured `AnswerCandidateRole`, graph symbol kinds,
    config/route metadata, or already active typed profiles;
  - path/scope comes from repo-map/list-files/grep tool parameters and active
    scope boundaries, not guessed names.
- Safety:
  - source-inventory remains an advisory navigation index, not final evidence;
  - `read_file` / `grep` remain available and model-chosen;
  - hints are bounded and per-scope deduped to avoid prompt spam;
  - active-set and parent-escape checks continue to run before any suggested
    path can be used by `repo_map`.

Tasks:

- [x] B2-J1: add a reusable structured "broad navigation observation" detector
  that consumes tool name, params, success status, and result shape.
- [x] B2-J2: attach a bounded source-inventory discovery hint after broad
  `repo_map(file_map|overview)`, `list_files`, and `grep(files_only=true)`
  results when the detector fires.
- [x] B2-J3: integrate with existing mid-loop read-without-emit hints so the
  second escalation can suggest source-inventory narrowing before more broad
  reads, while still prioritizing `emit_evidence` when evidence has already
  been read.
- [x] B2-J4: add tests proving no raw user/model-prose keyword dependency,
  dedupe across repeated tool calls, active-set safety, and non-Go route/config
  coverage.
- [x] B2-J5: rerun `s5b` plus one mechanism and one config/route case to verify
  the hint is generic and not enumeration-only.

B2-J implementation note, 2026-05-24 CST:

- Discovery hints now attach after successful broad `repo_map` non-inventory
  views, `list_files`, and `grep(files_only=true)` results. The trigger uses
  only structured tool parameters plus result shape (candidate file count /
  child scope groups); it never reads the raw user question or model prose.
- The hint is intentionally small: a broad member/attribute source-inventory
  call, up to four branch-expansion calls, and a reminder that repo lens output
  is navigation only and must be verified with source reads before citation.
- Path handling follows tool-layer semantics: banner paths already normalized by
  active-set gates are preferred, repo_map paths are re-run through the active
  multi-repo gate when present, parent escapes are refused, and absolute paths
  are accepted only when they normalize under `BusContext.RepoRoot` and can be
  rendered repo-relative. This prevents the hint from propagating unverified or
  parent-wide paths.
- Mid-loop read-without-emit recovery can surface the same discovery hint only
  after telling the model to first emit already-read evidence, avoiding a
  system-driven detour that would discard gathered evidence.

B2-J smoke observation, 2026-05-25 00:10 CST:

- Ran one focused `s5b` sample with
  `EVAL_RESULTS_ROOT=eval/results/b2j-smoke bash eval/run.sh
  eval/cases/s5b.case 1` and stopped after enough signal rather than waiting
  for the slow local model to finish the full case.
- The new discovery hint fired correctly after a broad `list_files` result:
  `scope_groups=25 candidate_files=25`, with normalized
  `repo_map {"path": "internal/analysis", "view": "source_inventory", ...}`
  follow-up calls.
- The model did not use the hint in analyze; it tried forbidden `read_file`
  calls, then recovered to `emit_analysis`. This confirms B2-J is advisory and
  does not override stage policy, but also shows the analyzer does not preserve
  the source-inventory opportunity as a structured handoff.
- In explore, the model first used older shell/list/grep patterns, then adopted
  `repo_map(view="source_inventory")` in round 6 and a scoped expansion
  (`scope="aggregator"`) in round 7. The discovery → cascade path is therefore
  learnable by the local model.
- Remaining gap: source-inventory output is still treated as navigation, not as
  a structured coverage/verification plan. The model kept returning to shell
  loops and one-file-at-a-time reads, and later the retry path complained that
  the exhaustive principal enumeration was not closed through
  `aggregate_facts.member_set`.

B2-J5 focused eval note, 2026-05-25 CST:

- `s5b` passed with no finalizer reject/rewrite. Metrics still showed high
  navigation cost (`read_file=45`, `list_files=12`, `source_inventory_lens=3`).
  Root cause: the analyzer did discover `repo_map(view="source_inventory")`,
  but the first suggested call used a package/file scope-summary shape before
  `emit_analysis` had produced `AnalysisIR`; the lens returned "no
  source-inventory observation" and the model fell back to broad listing.
- `qf_config_precedence` passed with no finalizer churn and exercised the same
  discovery hint generically (`source_inventory_lens=2`).
- `qf_architecture` had no finalizer churn but failed its expected surface
  regex and produced one semantic concern about inline source identifiers. This
  is a separate answer-richness/current-code-path gap, not a source-inventory
  discovery safety issue.

B2-J follow-up implementation note, 2026-05-25 CST:

- Discovery hints can appear during analyzer pre-scan, before `AnalysisIR`
  exists. If the lens implementation requires `AnalysisIR`, the model sees a
  tool that is advertised but returns an empty observation.
- Contract: a `repo_map(view="source_inventory")` call with explicit typed
  `roles` and explicit/bounded `scope` or `scopes` is sufficient typed input for
  navigation. The tool may render an advisory observation before analysis IR;
  it still does not become final-answer evidence.
- Hint shape now leads with a broad member/attribute checklist using
  language-neutral roles (`function`, `method`, `type`, `config_key`, `route`)
  or the structured roles already present in the tool params. It no longer
  leads with `roles=["package","file"]` scope-summary calls, which can be useful
  later but were a poor first hop for entry/member discovery.
- Safety: no raw user/model prose is inspected; active-set and parent-escape
  gates stay in the tool layer; the output remains explicitly advisory and must
  be verified by `read_file`/`grep` before citation.

Tasks:

- [x] B2-J6: allow model-driven source-inventory lens calls with explicit
  typed roles/scopes to work before `AnalysisIR` exists.
- [x] B2-J7: change discovery hints to lead with member/attribute checklist
  calls rather than package/file summary calls.
- [x] B2-J8: add regression tests for pre-analysis source-inventory and for
  discovery hints not emitting package/file summary calls as the first route.
- [x] B2-J9: rerun `s5b` after the shape fix and compare navigation cost.

B2-J9 focused eval note, 2026-05-25 CST:

- Ran `CODRAX_BIN=./codrax eval/run.sh eval/cases/s5b.case 1` after the
  pre-analysis source-inventory shape fix. Result directory:
  `eval/results/s5b-20260525-002621`. The case reported PASS with
  `finalizer_rejects=0` and `finalizer_rewrites=0`.
- The PASS is not acceptable as a quality signal by itself. Metrics regressed
  badly: `tool_read_file=150`, `tool_repo_map=29`,
  `tool_list_files=19`, `source_inventory_lens=71`,
  `explorer_iters=64`, `parallel_sibling_skips=0`.
- Root causes observed in the log:
  - analyzer accepted deep `repo_map(view="source_inventory")` expansion and
    spent an analyze round issuing 25 scoped source-inventory calls. This
    violates the evidence-lite analyze boundary even though the tool params are
    typed. Source-inventory row expansion belongs to explore after
    `emit_analysis` preserves the request shape.
  - parallel explorer lanes all worked the same `current_source` topic. One
    lane emitted a high-quality 25-member `principal_answer` member_set, but
    sibling lanes continued emitting conflicting 47/21/25 member-set variants.
  - final answer row citations were corrupted after the model's first emit:
    the first JSON payload cited `aggregator` at
    `internal/analysis/aggregator/aggregator.go:112`, but deterministic
    pre-emit normalization later displayed it as
    `internal/analysis/hint/composer.go:142`; `gate` similarly displayed
    `internal/analysis/aggregator/aggregator.go:112`. The pre-emit checker
    logged the mismatch only as a soft advisory, so eval passed with a wrong
    answer.
- Follow-up closure:
  - analyze-stage source-inventory discovery hints are now suppressed, and
    `repo_map(view="source_inventory")` is rejected during analyze with a
    precise stage-boundary repair hint to call `emit_analysis`;
  - package/directory enumeration rows now keep an already same-directory
    citation instead of letting generic symbol-citation repair move the row to
    another package;
  - remaining B2-K/T20 work still needs to make source-inventory observations a
    verification checklist and prevent sibling lanes from preserving conflicting
    principal member sets.

Design B2-K: source-inventory observation handoff without pretending it is
evidence.

- Goal: once a source-inventory lens has produced a bounded candidate set, carry
  it forward as a typed verification checklist so explorer/extractor/finalizer
  can preserve count/list coverage without forcing the model to rediscover the
  same rows.
- Boundary:
  - source-inventory rows remain navigation / mechanical inventory candidates,
    not final source evidence;
  - final answer citations still require `read_file`, `grep`, real directory
    listing evidence, or other source/artifact evidence according to the
    existing contract;
  - the system may preserve candidate set shape, counts, languages, files, and
    provenance, but must not select the final "entry point" semantics for the
    model.
- Generic mechanism:
  - when `SourceInventoryObservation` is active and the stage later records
    matching grounded evidence, reconcile the verified evidence back to the
    candidate rows by role/file/symbol support refs;
  - expose an advisory "verification checklist" to the next stage: confirmed
    rows, unverified rows, ambiguous rows, and `count == len(candidates)`;
  - when the model calls `emit_investigation_complete` without a member_set but
    verified source-inventory coverage is already complete, offer a typed repair
    hint that references the checklist instead of asking it to reread files;
  - if coverage is incomplete, surface only the missing candidate rows and their
    suggested verification files, not a broad reread instruction.
- Generality:
  - applies to package/file/function/type inventories, route/config inventories,
    mixed-language trees, multi-repo active-set scoped inventories, and external
    artifact backed inventories when the candidate rows carry typed provenance;
  - does not depend on raw user keywords or model prose.

Tasks:

- [x] B2-K1: inspect existing `SourceInventoryObservation` persistence and
  Turn-A handoff paths; reuse them rather than introducing a parallel carrier.
- [x] B2-K2: add a source-inventory verification checklist view that separates
  candidate rows from verified evidence rows and keeps `count == len(rows)`.
- [x] B2-K3: teach the explorer close/retry path to point at missing checklist
  rows when a member_set handoff is incomplete.
- [x] B2-K4: add tests covering function/package, route/config, multi-language,
  and active-set scoped checklists.
- [ ] B2-K5: rerun `s5b` plus one mechanism and one config/route case.

B2-K implementation design, 2026-05-25 CST:

- Reuse existing carriers:
  - `MutableState.SourceInventoryObservation()` is already populated by
    `SetSourceInventoryAdvisory` and by model-driven
    `repo_map(view="source_inventory")` calls.
  - Turn-A snapshot and `ObservationLedgerInput*` already merge the observation,
    so no new handoff type is required.
  - `buildAggregateMemberSupportIndex` already accepts source-inventory support
    refs for later `aggregate_facts.member_set` validation. This remains a
    validation aid, not final evidence.
- Current gap:
  - explorer prompt rendering requires an active `SourceInventoryAdvisory`; an
    observation-only artifact can be present but invisible.
  - extractor prompt rendering derives visible rows from `advisory.Sets`; when
    only `SourceInventoryObservation` exists it shows invariants but not the
    actual rows.
  - the exhaustive enumeration close/retry downgrade still says "reread or emit
    a member_set" generically, even when a bounded source-inventory observation
    already contains the candidate checklist and suggested verification files.
- Implementation contract:
  - render observation-only rows from `SourceInventoryObservation` directly,
    preserving `count == len(members)`, role, language, support_ref, file:line,
    coverage_state, and bounded row-local attributes;
  - classify rows for repair hints as `verified_source_window` only when the
    current evidence/tool history has a matching read/grounded file:line, and as
    `candidate_needs_verification` otherwise. Repo-map rows alone are not
    promoted to final evidence;
  - when `emit_investigation_complete` is rejected for missing exhaustive
    `member_set`, attach a bounded source-inventory verification checklist and
    candidate files to the existing repair directive. The model still decides
    whether these rows match the user-intended principal set;
  - do not read raw user text or model prose; use only typed observation rows,
    support refs, evidence items, and tool-history source windows.
- Safety:
  - no automatic final-answer/member_set synthesis;
  - no hard whitelist for reads;
  - no language-specific path heuristics;
  - active-set, symlink, parent-escape, and multi-repo boundaries remain in the
    existing repo_map/read_file/list_files/grep gates.

B2-K implementation note, 2026-05-25 CST:

- `publishSourceInventoryAdvisory` now preserves an active model-driven
  `SourceInventoryObservation` when the current completion pass has no fresh
  request-derived advisory. This prevents `emit_investigation_complete` from
  accidentally erasing the exact repo-lens checklist it should use for repair
  guidance.
- Explorer and extractor prompt rendering now handle observation-only artifacts.
  The rendered rows include role, file:line, language, support_ref,
  coverage_state, row-local attributes, and `count == len(members)` invariants.
- The exhaustive member-set pre-complete downgrade now includes a bounded
  source-inventory verification checklist:
  - `verified_source_window` means the row has a matching emitted evidence item
    or read/grep source window at the same file:line;
  - `candidate_needs_verification` means the row is still only repo-map
    navigation context and must be verified or explicitly supported before the
    model cites it;
  - `ambiguous_candidate` preserves graph ambiguity for the model to resolve or
    disclose.
- Existing repair directives receive bounded candidate files from the
  source-inventory observation, but only as read suggestions. They are not a hard
  whitelist and they do not change tool success/failure semantics.
- Tests added/updated:
  - observation-only explorer and extractor rendering;
  - exhaustive close rejection with one verified source row and one unverified
    source-inventory row;
  - existing source_inventory lens tests continue to cover cross-language
    functions/methods/types/config keys/imports and multi-repo active-scope
    isolation.

B2-K post-pull audit note, 2026-05-25 CST:

- Remote update `cd6c54c9` closes the largest B2-J9 regression before this
  batch continues: analyze now rejects `repo_map(view="source_inventory")`
  row expansion and source-inventory discovery hints are suppressed in analyze.
  The typed request shape should be captured through `emit_analysis`; row
  expansion and verification belong to explore.
- The source-inventory path surface is normalized before hint rendering:
  active-set aliases are resolved, absolute in-repo paths are rendered
  repo-relative, parent escapes are refused, and repo-root containment is
  rechecked before result paths become scoped branch suggestions.
- Clarified member-set boundary:
  - the system can guarantee mechanical invariants for repo-lens observations:
    row provenance, normalized repo/scope boundary, role/language/file/line
    shape, `count == len(members)`, and whether a row has a matching
    read/grep/evidence source window;
  - the system cannot guarantee that a repo-lens row is the user's intended
    final principal member. It must not synthesize `aggregate_facts.member_set`
    or present the checklist as final evidence;
  - therefore the repair hint explicitly says the checklist is not a
    system-authored member_set and asks the model to re-emit the member_set
    only after its own verification. This preserves user/model intent while
    still reducing redundant rediscovery.
- Focused `s5b` run started from the pre-pull B2-K build was interrupted for
  the remote update and produced a useful fail sample:
  `eval/results/b2k-source-inventory/s5b-20260525-004339`.
  It saw source-inventory lens hints but still used older `exec_command` /
  `list_files` / `read_file` patterns and exited early with missing
  `normalizer`, `compiler`, `criterion`, `gate`, `subject`, `sourcemix`, and
  `stopcond`.
- Post-`cd6c54c9` verification completed with
  `eval/results/s5b-20260525-004645`: PASS, `tool_repo_map=1`,
  `tool_read_file=29`, `source_inventory_lens=3`, `explorer_iters=11`,
  `finalizer_rejects=0`, `finalizer_rewrites=0`. The final answer kept the
  model's package rows and preserved directory-scoped citations for
  `aggregator`, `gate`, `logtriage`, and `perftriage`; no citation-repair log
  rewrote those rows. B2-J9 is therefore closed for the analyze-boundary and
  directory-citation regression. Remaining B2-K/T20 work is broader lane
  ownership and verification-checklist convergence, not this regression.
- A separate post-pull local run on the pre-B2-L patch stack still produced a
  broader failure sample below. It is retained because it exposed systemic gaps
  outside the closed B2-J9 regression: source-inventory scope aliasing,
  finalizer row/citation safety, language-following, and cascade-guidance
  prominence.

B2-K live eval findings, 2026-05-25 00:51 CST:

- Run: `EVAL_RESULTS_ROOT=eval/results/b2k-postpull CODRAX_BIN=./codrax
  bash eval/run.sh eval/cases/s5b.case 1`, result directory
  `eval/results/b2k-postpull/s5b-20260525-005119`.
- Final verdict: `FAIL missing:gate`.
- Trace metrics: `tool_read_file=24`, `tool_repo_map=4`, `tool_list_files=9`,
  `source_inventory_lens=9`, `midloop_inject=8`, `analyzer_iters=5`,
  `explorer_iters=20`, `finalizer_rejects=0`, `finalizer_rewrites=0`,
  `semantic_quality_dispatches=1`.
- Confirmed improvement: analyze no longer accepts deep
  `repo_map(view="source_inventory")` expansion. The model used `file_map` /
  `list_files`, then explore used `repo_map(view="source_inventory")`.
- New P0/P1 gaps observed:
  - `subtopic_coherence` falsely rejected a successful `emit_analysis` because
    primary entities were path-shaped (`internal/analysis/aggregator`) while
    subtopic entities were basename/symbol-shaped (`aggregator`, `New`,
    `Aggregate`, etc.). This is a typed normalization gap, not a model
    hallucination. Path/package entities and their basenames should be compared
    through canonical identity aliases for coherence gates.
  - The analyzer still spent a full pre-scan round on seven parallel
    `list_files` calls under child directories after `list_files
    internal/analysis` had already established the 25 child scopes. This is not
    source-inventory-specific; broad directory enumeration needs an
    evidence-lite stop once path existence and child-scope shape are known.
  - After analyze degraded, recovery explore still used the source-inventory
    lens but quickly fell back to grep/read_file. The lens guidance is visible
    and usable, but the recovery path loses typed profile authority and makes
    old broad-search habits more likely.
  - Source-inventory handoff duplicated scoped rows: the final extractor digest
    carried both `internal/analysis/aggregator` and basename `aggregator`
    scopes, reporting package scope `count=50` for a 25-directory target and
    showing `40 of 544` candidates. This is a canonical scope merge problem,
    not an answer-shape decision. The handoff must dedupe aliases while
    preserving provenance and ambiguity.
  - `emit_evidence` near-miss: the model tried to record package/directory
    role summaries as `scope=file` with positive `line_start`; validation first
    rejected `scope=file must have line_start=0`, then rejected file rows for
    missing config-style `file_role_label`. This is a generic schema ergonomics
    issue: package/module role summaries should have a clearer accepted carrier
    or a repair that points to `scope=line` for function entries and to a
    package/directory role carrier for package summaries. Do not solve this by
    matching the s5b answer text.
  - After a failed `emit_evidence(items=[...])`, the local model next emitted
    `{}` for `emit_evidence`. The recovery hint should preserve enough
    structured shape from the failed call to make “fix and re-emit the same
    batch” easier, rather than forcing the model to reconstruct JSON from
    memory.
  - Path-miss help worked but came late: the model guessed
    `internal/analysis/prescan/scan.go`, `internal/analysis/priority/priority.go`,
    and `internal/analysis/axis/axis.go`; read failures included same-scope
    candidate files and the model recovered for two of them. The repo-lens
    "known candidate files" guidance should be visible before the wrong
    `read_file`, not only after a miss.
  - Finalizer accepted a string-wrapped `blocks` document and repaired several
    `citation_ref` carriers, which is the desired compatibility behavior.
    However the same accepted draft still contained visible citation mismatches
    (`amplifier` cited `budget`, `declarative` cited `amplifier`, `patcher`
    cited `stopcond`, `subject` cited `declarative`) and omitted `gate`.
    The answer contract recorded one inconsistency but did not rewrite. This is
    an end-to-end contract gap: soft advisory acceptance must not allow a
    visibly wrong evidence binding to ship for an enumeration row.
  - The final prior slate mixed directory labels and entry-function labels
    (`Compute`, `Compile`, `Check`, etc.), so downstream rendered an uneven
    principal set. Enumeration handoff needs a stable row identity of
    `directory/package member -> selected entry symbol` instead of letting the
    entry function replace the requested member label.
- Candidate fix order after the run finishes:
  1. P0: canonical path/basename entity aliasing for analyzer coherence gates,
     covered by cross-language path/package/module tests.
  2. P0: source-inventory canonical scope dedupe and row identity preservation
     (`requested member label` separate from `candidate entry symbol`), covered
     by multi-language and multi-repo alias tests.
  3. P0: finalizer enumeration citation safety: if a row label is the requested
     member and the cited line is only the entry symbol, require a row-local
     support relation or render both fields without pretending the line defines
     the directory label. Do not auto-synthesize the answer set; only validate
     visible bindings.
  4. P0: analyzer broad-child-list stop/fence after a parent directory listing
     establishes enough source-inventory shape; keep it typed and advisory,
     with no raw keyword control-flow.
  5. P1: `emit_evidence` package/module role-summary near-miss repair or
     normalization, shared across languages and config/route/file inventories.
  6. P1: failed-tool-call repair memory for empty retry payloads, reusing the
     existing text-tool-call recovery and repair-directive carriers.

B2-L batch 1 implementation note, 2026-05-25 CST:

- Implemented P0 analyzer coherence aliasing:
  - R1.3 now compares typed analyzer entities through structural repo-scope
    aliases in addition to exact strings. A path/package/module surface such as
    `internal/analysis/gate` and its basename `gate` are treated as the same
    coherence identity for the analyzer gate.
  - The aliasing is deliberately structural and language-neutral: repo-relative
    paths, package/module paths, file names, and final path segments only. It
    does not read user-question keywords or model prose.
  - Added regression tests proving that source-inventory basename anchors pass
    while unrelated basenames still fail.
- Implemented P0 source-inventory scope alias dedupe:
  - source-inventory advisory construction now canonicalizes scope lists after
    typed-request and model-driven lens scope resolution.
  - If a full scope and its basename alias both appear, the basename alias is
    dropped and the full repo-relative scope is preserved. This prevents
    package/member rows from doubling (`count=50` for 25 directories) while
    keeping provenance and the model's advisory-only choice surface intact.
  - Added regression coverage for package/directory inventories with mixed
    full-path and basename scopes.
- Verified:
  - `go test ./internal/analysis/gate`
  - `go test ./internal/tool -run 'TestBuildSourceInventoryAdvisory|TestPublishSourceInventory|TestAggregateMemberSetMemberUsableWithSourceInventory|TestSourceInventory'`
  - `go test ./internal/agent -run 'Test.*Analyzer|Test.*SourceInventory'`
  - `make build`
- Remaining P0 after this batch:
  - finalizer enumeration row/citation safety so soft advisory acceptance cannot
    ship visibly wrong bindings;
  - analyzer broad-child-list stop/fence after a parent directory listing has
    established enough typed source-inventory shape.

B2-L language-following gap, 2026-05-25 CST:

- Same `s5b` run exposed a separate UX/quality regression: the user request was
  Chinese, but the final answer body started in English ("Following the
  directory structure..."). Only the system-added caveat localized to Chinese.
- This is not a model capability assumption; earlier REPL behavior generally
  followed the user's language. The likely failure is a lost language contract
  between analyzer/explorer evidence (mostly English structural text) and
  finalizer answer-document rendering.
- Required investigation:
  - locate the current language-detection / response-locale carriers;
  - verify whether they are present in `AnalysisIR`, finalizer prompt assembly,
    answer-document validator, reviewer, fallback/raw-output renderer, markdown
    output, and HTTP preview;
  - fix at the typed contract layer if missing. Do not infer control flow from
    raw user keywords beyond an existing locale/language detector, and do not
    rewrite semantic content. The system may add localized repair guidance or
    require the finalizer to re-emit in the detected response language.
- Candidate acceptance criterion:
  - for Chinese REPL questions, principal final answer prose and system-added
    notices/caveats are Chinese unless the user explicitly asks for another
    language or the answer is a code/API identifier;
  - source quotes, identifiers, file paths, symbols, and citations remain
    verbatim.

B2-L language-following implementation, 2026-05-25 CST:

- Root cause confirmed in code: finalizer renderer locale was captured only from
  `AgentContext.Language`; the analyzer's structured `emit_analysis.language`
  was not a fallback for call paths that failed to carry the configured
  language into finalizer context. The huge answer-document prompt also did not
  restate the response-language contract near the finalizer's dynamic data, so
  local models could follow evidence/prose language instead of the configured
  answer language.
- Fix:
  - concrete project/CLI language remains highest priority (`zh`, `en`, etc.);
  - `off` / `none` remains a hard disable and does not get overridden by the
    analyzer;
  - when no concrete configured language reaches finalizer, structured
    `AnalysisIR.AnswerContract.Language` then `AnalysisIR.RequestModel.Language`
    provide a fallback renderer locale;
  - finalizer now emits a small `Response Language` dynamic section near the
    top of its prompt, explicitly preserving identifiers/file paths/source
    quotes verbatim.
- This is a typed language-carrier fix, not a user-intent classifier: it does
  not inspect raw model prose and does not use question keywords for workflow
  routing.

B2-L finalizer citation safety implementation, 2026-05-25 CST:

- Root cause confirmed from `s5b` logs: answer-document pre-emit already knew
  many `ordered_list` rows had invalid `citation_ref` bindings, but the generic
  citation violation is soft-by-default. Before that soft advisory, deterministic
  repair missed a safe class: package/directory member labels whose correct
  support citation was already present in the same document citation pool.
- Fix:
  - for typed enumeration rows (`facet_id=enumeration_item`), a member label
    such as `amplifier` may align to a citation inside
    `.../amplifier/...` even when the cited line names the entry function
    rather than the directory itself;
  - the same scoped-directory alignment now participates in citation-ref
    rebind, not just in "current citation already OK" checks;
  - this is still advisory-only with respect to answer semantics: it never
    creates a member, never chooses an entry function, and never reads the user
    question or model prose. It only repairs an already-emitted row to an
    already-emitted citation whose path structurally matches the row label.
- Added regression coverage for a wrong `budget` citation being rebound to an
  already-present `internal/analysis/amplifier/...` citation for the
  `amplifier` package row.

B2-M source-inventory cascade prompt gap, 2026-05-25 CST:

- Root cause from the broad search eval cluster: `repo_map(view="source_inventory")`
  already renders a good "summary first, cascade next" lens, and the discovery
  hint can point models toward that tool. However, the explorer/extractor
  handoff prompt still rendered source-inventory context mostly as flat rows.
  Local models therefore saw real candidates but still tended to continue broad
  `list_files` / `grep` / large `read_file` loops instead of narrowing with
  another structured repo lens call.
- Fix:
  - exported the existing cascade guide renderer instead of creating a second
    prompt-only implementation;
  - explorer and extractor now place the same advisory `Cascaded Repo Lens
    Guide` immediately after count/list invariants and before flat candidate
    rows;
  - the guide is language-neutral and role-neutral: it uses structured
    source-inventory rows (`scope`, `role`, `attribute_role`, `file`, `line`,
    `language`, `support_ref`) and never reads user keywords or model prose;
  - the guide remains navigation-only. It suggests narrower `repo_map` calls
    and reminds the model to verify with `read_file` / `grep` before citing.
- Acceptance criteria:
  - supports directories/packages/modules, functions, methods, types,
    routes, config keys, config files, and mixed-language scopes;
  - supports multi-repo active-set boundaries through the existing normalized
    source-inventory observation;
  - does not turn suggested files into a whitelist or final answer membership.

B2-N analyzer broad child-list pre-scan gap, 2026-05-25 CST:

- Root cause: analyzer prescan-ready detection only recognized
  `grep(files_only=true)` summaries. A successful `list_files` parent-scope
  directory listing could already show the bounded child-scope shape, but the
  analyzer still exposed `repo_map` / `grep` / `list_files` in the next round.
  Local models then spent another round listing child directories one by one,
  even though analyze only needed routing/classification hints and explore
  should own line-level evidence and full source-inventory expansion.
- Commercial-grade contract:
  - use only structured tool result shape: tool name, success flag,
    `list_files` banner fields, and repo-relative result paths;
  - never inspect user-question keywords or model prose;
  - never synthesize `emit_analysis` or final answer members from a directory
    listing;
  - close only the analyzer pre-scan surface. The model still emits the real
    `emit_analysis` payload, and explore remains responsible for verification.
- Implementation:
  - classify scoped `list_files` results as analyzer-ready when the path is
    non-root, non-escaping, and the visible output contains child entries under
    the same scope;
  - treat larger child-scope listings as strong ready and switch immediately to
    emit-only even when the global prescan budget is larger for multi-topic
    questions;
  - keep small scoped listings as soft ready so the existing budget policy can
    decide whether to allow one more lightweight check;
  - added tests for path-shaped directories with spaces, root and parent-escape
    guards, soft small-scope listings, and the emit-only schema transition.
- This remains cross-language: entries are treated as repo-relative path
  surfaces only. No Go parser, extension, package naming convention, user
  keyword, or model-prose rule participates in the control flow.
- Partial eval observation:
  - `eval/results/b2n-analyzer-child-list/s5b-20260525-012859` was started
    from the B2-N build and interrupted after B2-O was diagnosed.
  - The analyzer path changed as intended: `repo_map internal/analysis` ->
    `list_files internal/analysis` -> `emit_analysis`; it did not spend another
    analyzer round listing each child directory.
  - The first `emit_analysis` then hit the B2-O decorated-entity coherence gap
    below, so this run is retained as a partial diagnostic rather than a final
    post-fix eval.

B2-O source-inventory decorated entity coherence gap, 2026-05-25 CST:

- The B2-N eval showed the child-list loop was fixed in analyze: the first
  attempt used `repo_map internal/analysis`, `list_files internal/analysis`,
  then `emit_analysis`. The remaining analyzer failure was a different
  coherence false positive: sub-topic entities such as `gate entry function`
  combine a verified source-inventory member alias (`gate`) with a typed role
  suffix (`function`). R1.5 treated the whole decorated surface as an invented
  unresolved symbol.
- Fix:
  - only when `source_inventory_profile` is active, split structured sub-topic
    entity surfaces into `member alias + role suffix`;
  - the member alias must match existing source-inventory primary scope/entity
    aliases such as `internal/analysis/gate` -> `gate`;
  - the suffix must contain a normalized `AnswerCandidateRole` such as
    function, method, type, config_key, route, etc.;
  - the result is used only to avoid a coherence hard-fail. It is not evidence,
    not an answer member, and not a parser/language decision.
- Guard:
  - unrelated decorated surfaces like `scheduler entry function` still fail
    when `scheduler` is not a source-inventory scope alias.

B2-P same-batch analyzer pre-scan fan-out gap, 2026-05-25 CST:

- Post B2-O eval `eval/results/b2o-postfix/s5b-20260525-013651` exposed a
  deeper analyzer loophole: the first analyze attempt emitted
  `list_files internal/analysis` and 25 child `list_files` calls in the same
  assistant response. B2-N only closed the pre-scan surface after the tool batch
  completed, so it protected the next round but could not prevent same-batch
  redundant fan-out.
- Root cause:
  - read-only batches are normally parallelized for latency;
  - analyzer pre-scan readiness was observed only by `analyzerEvaluator.Observe`
    after the whole batch;
  - therefore later calls in the same batch did not see `Mutable.PrescanReady`
    and bypassed the existing terminal emit-only boundary.
- Commercial-grade fix:
  - analyze-stage tool batches now execute sequentially even when every tool is
    read-only, because the analyzer pre-scan boundary is intentionally
    stateful;
  - after each successful lightweight pre-scan result, the runtime reuses the
    existing `analyzerPrescanResultReadiness` logic and marks prescan ready
    immediately when the result is a strong close signal;
  - subsequent pre-scan calls in the same assistant response are rejected by the
    existing `validateAnalyzerToolBoundary` terminal emit-only path, while
    `emit_analysis` remains available.
- Contract:
  - no user-question keyword matching;
  - no model-prose inspection;
  - no synthetic `emit_analysis` payload;
  - no semantic answer/member inference from directory listings;
  - non-analyze stages keep read-only parallel execution.
- Additional navigation wording audit:
  - the log line "根据 `repo_map` 的概述，我可以看到每个包导出的函数" was model
    prose, not a fixed system decision;
  - repo_map overview does expose language-neutral public/exported symbol
    counts, which can be useful but may lead small models to over-assume
    exported-only semantics;
  - the repo_map tool description was widened from files/packages/symbols to
    directories/modules/packages/symbols/routes/config surfaces, and
    `attribute_roles` examples now mention functions, methods, types, routes,
    and config keys under directory/module/package/file scopes. This keeps the
    lens general and avoids teaching a Go/package-only mental model.

B2-P config/root scope regression caught during verification:

- While running the B2-P test batch, existing config-file coverage exposed a
  related source-inventory scope bug: explicit lens scopes `[".", "src"]`
  were normalized through `normalizeSourceInventoryScopeSurface`, which turns
  `"."` into empty. The dedupe pass then silently dropped the repo-root scope,
  so root-level configuration files such as `package.json` disappeared when a
  sibling scope like `src` was also present.
- Fix:
  - preserve `"."` as a first-class repo-root lens selector inside
    `sourceInventoryDedupeScopeAliases`;
  - keep the existing candidate-time source-scope filters, so root scope does
    not bypass production/test/config policy;
  - reuse the existing `TestRepoMapSourceInventoryViewConfigFilesAreNavigationRows`
    test to lock root config + nested source files together.
- This is not s5b-specific: it protects config, route, file, package/module,
  and mixed-language repo-lens calls where the model asks for multiple scopes
  including the repo root.

B2-Q explorer entry-attribute search fallback gap, 2026-05-25 CST:

- Post B2-P eval `eval/results/b2p-samebatch/s5b-20260525-014621` confirmed the
  analyzer same-batch fan-out fix: analyze no longer runs `list_files x25`; it
  used `repo_map x2`, one scoped `list_files`, then `emit_analysis`.
- New downstream gap: once in explore, the model still reverted from the Repo
  Lens path to language-specific grep patterns such as `func New` and
  `func init`, then exhausted the grep sourcemix budget. This is not caused by
  prompt keyword matching; it reflects a missing structured carrier for
  "member -> candidate entry-like attribute" priority.
- Generalized root cause hypothesis:
  - source_inventory can list functions/methods/types/config/routes, but the
    currently rendered rows do not make "candidate attribute ordering under a
    member" strong enough for models to choose a small verification set;
  - broad entry-like tasks therefore fall back to model-learned language
    heuristics (`New`, `init`, exported Go functions) instead of using the
    language-neutral graph candidate rows already available;
  - when the model chooses grep, sourcemix protects the run but the user sees
    wasted retries and lower answer completeness.
- Design constraints for the follow-up fix:
  - no user-question keyword routing and no model-prose inspection;
  - do not infer the final entry point automatically;
  - use only structured source_inventory roles/attributes, graph location,
    visibility, doc/note, and model-provided role order;
  - support non-Go languages, config/route inventories, multi-repo active-set
    boundaries, and same-repo mixed-language scopes;
  - present the result as advisory ordering/checklist, never final evidence.

B2-R citation-floor repair should prefer structured read targets, 2026-05-25 CST:

- Same B2-P eval exposed a lower-level repair inversion after the model finally
  used `repo_map(view="source_inventory")`: the accepted
  `emit_investigation_complete.aggregate_facts.member_set.support_refs` named
  concrete candidate files such as `aggregator/aggregator.go:112`, but the
  pre-complete citation-floor gate still emitted `RepairExpandSearch` with
  analyzer keyword stems. The next hint told the model to broaden grep even
  though the current structured handoff already identified exact file:line
  anchors that merely needed verification reads.
- Historical source:
  - `RepairExpandSearch` was wired in commit `62e9728e4` as CGEC B1c, before
    source-inventory observations and member-specific `support_refs` became the
    main handoff lane. The old root cause was "keywords found too few files",
    so keyword broadening was useful when no structural targets existed.
- Updated contract:
  - keep `RepairExpandSearch` for true no-target cases, unverified-path
    rediscovery, phase-0 zero-hit search, field/value syntax-family expansion,
    and stall advisories;
  - for `pre_complete.citation_floor_low`, first try to derive bounded
    `RepairReadFile` targets from current aggregate `support_refs` and
    source-inventory observation rows;
  - resolve support-ref files against typed request scopes such as
    `AnalyzerHints.ExactTargets` and `list_files` scopes, so scoped references
    like `aggregator/aggregator.go:112` become
    `internal/analysis/aggregator/aggregator.go`;
  - preserve active-set / repo-root safety and cap the read set with the
    existing required-file coverage bound;
  - only fall back to keyword stems when no verified source file target can be
    resolved.
- This is language-neutral and not s5b-specific: support refs may point at Go,
  Python, TS, Java, YAML, route/config, or mixed-language files. The system does
  not infer the final member set; it only converts model-provided/file-index
  anchors into the next verification read.

B2-S analyzer terminal emit-only correction budget, 2026-05-25 CST:

- Post B2-R focused eval failed before exploration: analyzer successfully ran a
  scoped `list_files internal/analysis`, marked the pre-scan as ready, then the
  local model hallucinated unavailable `repo_map` / `grep` calls after the tool
  schema had narrowed to `emit_analysis`. The runtime correctly rejected those
  calls, but `analyzerEvaluator.Observe` immediately stopped the current
  analyze attempt because a terminal emit-only hint had already been issued.
  Three analyze attempts repeated this pattern and the pipeline degraded into an
  empty recovery IR, so extractor/finalizer never received investigation data.
- Root cause:
  - B2-P made analyze-stage pre-scan boundaries stateful and stricter, but the
    correction loop was too brittle for providers that emit one or more stale
    tool calls after a schema narrowing;
  - the failure is purely control-flow. The tool boundary itself is correct and
    must remain firm: analyze cannot execute `repo_map` / `grep` / `list_files`
    once terminal emit-only mode is active.
- Fix contract:
  - keep rejecting unavailable prescan tools in terminal emit-only mode;
  - do not synthesize `emit_analysis` and do not infer the request semantics;
  - give a bounded number of extra correction turns that expose only
    `emit_analysis`, because analyze is critical and failure prevents the rest
    of the pipeline from doing useful work;
  - after the correction budget is exhausted, fail loud as before to avoid an
    infinite loop.
- Runtime/config contract:
  - reuse the existing `AnalysisLimits` path instead of adding a provider-level
    special case;
  - export `analysis_emit_only_correction_retries` in `codrax.yaml.example`,
    default 3, 0 = fail immediately after terminal stale-tool rejection;
  - keep provider/model config (`providers.yaml`) scoped to model connectivity
    and text-tool-call recovery, not analyzer control-flow budgets.
- This is model- and language-neutral. It reacts only to structured tool repair
  codes and stage state, not to user keywords or model prose.

B2-T enumeration completeness handoff must dominate close-ready, 2026-05-25 CST:

- Verification run:
  - command: `EVAL_RESULTS_ROOT=eval/results/b2s-analyzer-emit-correction CODRAX_BIN=./codrax bash eval/run.sh eval/cases/s5b.case 1`;
  - result dir: `eval/results/b2s-analyzer-emit-correction/s5b-20260525-020710`;
  - verdict: FAIL `missing:subject no_regex_match:(subject.*Score|Score.*subject)`.
- Good news from the run:
  - analyzer no longer degraded early; a single analyze dispatch reached
    `emit_analysis` after 4 iterations;
  - `analysis_limits` startup log showed
    `emit_only_correction_retries=3`, proving the new yaml-backed runtime knob
    is active;
  - explorer used the cascaded `source_inventory` path (`source_inventory_lens=14`)
    and scoped graph projection reused the active in-memory graph.
- New root-cause cluster:
  - `list_files internal/analysis` observed 25 direct directories, including
    `subject`, but the close payload claimed `value="24"` and omitted
    `subject (Score)`;
  - the model had only read/grounded 4 source files when
    `explorer.mid-loop.completion-ready` fired, because current evidence
    requirements were satisfied for the local branch even though the exhaustive
    enumeration candidate set was not covered;
  - the rejected first `emit_investigation_complete` correctly demanded
    `support_refs` for decorated members, but this is downstream shape repair;
    it did not catch the stronger upstream completeness mismatch against the
    already-observed source-inventory/list-files candidate universe;
  - finalizer rendered the 24-member aggregate slate, and the eval failed on
    the missing `subject` row.
- Contract gap:
  - for exhaustive enumeration answers, known structured candidate universes
    from `list_files` / source_inventory count rows must suppress or qualify
    close-ready until the model either covers every candidate, explicitly
    excludes candidates with a reason, or marks the result as a lower bound /
    incomplete set;
  - the system must not invent the missing member or force a final answer
    rewrite. It should surface a structured completion obligation upstream:
    "candidate universe has N, current member_set has M, missing candidates are
    advisory verification targets";
  - source_inventory remains navigation, not final evidence. The guard should
    compare structured counts/members and ask the model to verify or disclose,
    not auto-fill answers.
- Generality:
  - this is not Go-specific. The same failure can happen for packages,
    directories, modules, routes, config files/keys, classes/types, handlers,
    tests, cross-repo active-set members, or external artifact sections;
  - the guard should operate over structured candidate universes and typed
    roles/provenance, not user-question keywords or model prose.

Design B2-T1: candidate universe registry on existing source-inventory carriers.

- Do not create a second fact system. Reuse `SourceInventoryObservation` as the
  auditable candidate-universe carrier and keep `SourceInventoryAdvisory` as the
  prompt/navigation hint. The observation already has `count == len(members)`,
  role, support-ref, file, line, language, merge, ledger, and Turn-A handoff
  semantics.
- Publish exact candidate universes from structured tool results, starting with
  non-recursive `list_files`. A non-recursive list over an active-set-safe path
  is a mechanical direct-child universe: directories become
  `AnswerCandidateRolePackage` (package/directory/module scope), regular files
  become `file`, and recognizable config/manifest files become `config_file`.
  This is language-neutral and multi-repo safe because it runs after the
  existing path resolver, active-set gate, symlink/parent escape checks, and
  shared exclude filters.
- Keep source-inventory graph/lens rows advisory unless their exact universe is
  backed by such a structured exact observation. Function/method/type/config-key
  rows remain navigation candidates by default. The system must not broaden a
  model-authored answer by appending every graph symbol in scope.
- Coverage contract:
  - close-ready must be suppressed or qualified when an exact candidate
    universe is active and no model-authored principal `member_set` covers,
    excludes, or honestly discloses the universe;
  - `emit_investigation_complete` must reject an exact-universe mismatch when
    the model emits a principal `member_set` that partially covers the universe
    and claims completeness through `value == len(members)`;
  - finalizer must not silently render an exhaustive answer that omits known
    exact-universe candidates. If a legacy/bad handoff reaches finalization, the
    model can either include the missing candidates itself or add a structured
    caveat/lower-bound disclosure; the system still does not fill the answer.
- Matching rules must be structural and language-neutral:
  - compare observation member identities against model-authored members,
    relation left axes, decorated-label bases, support-ref labels, and explicit
    exclusions;
  - never read the user question or model prose to decide whether a universe is
    relevant;
  - only hard-block when the request is already a typed exhaustive enumeration
    and the model has strongly aligned its member_set/count facts with the exact
    universe. Current guardrail: an incomplete universe must have at least two
    aligned members for tiny sets, or at least three aligned members and >=60%
    coverage/exclusion for larger sets. Purely unrelated list/file inventories,
    one-off overlaps, mechanism/architecture/trace questions, and graph/lens
    advisory rows stay advisory.
- Task split:
  - [x] B2-T1-A: publish non-recursive `list_files` results into
    `SourceInventoryObservation` without overwriting existing advisory/lens
    observations.
  - [x] B2-T1-B: add a shared candidate-universe coverage helper over
    `SourceInventoryObservation` + aggregate facts.
  - [x] B2-T1-C: use the helper to suppress generic close-ready and emit a
    candidate-universe reconciliation hint.
  - [x] B2-T1-D: use the helper in `emit_investigation_complete` pre-complete
    downgrade, preserving the existing support-ref and missing-member-set
    repair flow.
  - [x] B2-T1-E: add finalizer pre-emit protection for legacy/bad handoffs.
  - [x] B2-T1-F: add tests for list-files publication, incomplete universe
    rejection, explicit exclusion allowance, close-ready suppression, and
    finalizer protection.
  - [x] B2-T1-G: publish exact direct-child universes from explicit
    `repo_map(view="source_inventory")` lens requests for mechanical direct
    child roles (`package`, `file`, `config_file`). This is not Go-specific:
    it scans the active-set-safe repo-relative direct children under the model's
    typed `path`/`scope` query, reuses the shared search exclusion policy, and
    stores the result as internal exact provenance
    `repo_lens:direct_children` without duplicating rows in the visible lens
    table.

Follow-up observation during B2-T1 verification, 2026-05-25 CST:

- The running s5b eval showed `建议文件 23 个` in the analyzer summary while
  `repo_map(view="source_inventory")` actually returned
  `scope_groups=25` and `roles=package:25` for `internal/analysis`.
  Filesystem reality is also 25 direct directories. Therefore "23" was not a
  repo-map count; it came from the model-authored `emit_analysis.required_files`
  advisory list and was later confused with the source-inventory universe.
- Root cause: analyzer `required_files` is a file-reading hint, not an
  exhaustive candidate universe. It can be incomplete by design, especially
  when derived from grep/repo-map prescan. It must not outrank an exact
  source-inventory/list-files universe for exhaustive enumeration closure.
- Implemented mitigation: explicit source-inventory lens direct children now
  publish the same kind of exact universe as direct `list_files`, but with a
  separate provenance marker. The visible lens keeps its compact summary and
  cascade guidance, while close-ready / investigation-complete / finalizer read
  the exact internal carrier. This avoids both user-facing row duplication and
  downstream "23 files == 23 members" confusion.
- Remaining watch point: analyzer summaries may still display the raw
  `required_files` count as "suggested files". That is correct UX for reading
  hints, but future wording should keep the "not an exhaustive inventory"
  boundary obvious when an exact source-inventory universe is also active.

Stabilization matrix added after repeated s5b probes:

- The repeated fixes were not answer-content fitting. They exposed two generic
  repo-lens contract holes:
  - scoped-root aliases: a model can express the same bounded scope as
    `path="pkg"`, `scope="."`, `scope="pkg"`, or `scope="pkg/child"`. In a
    projected/scoped repo-map graph, these must normalize to the graph-local
    root/child instead of being treated as extra sibling scopes;
  - carrier isolation: exact candidate universes are internal audit carriers
    for close/finalizer coverage checks. They must not be merged into the
    visible `repo_map(view="source_inventory")` result for a later, narrower
    role query, otherwise the model sees a broad union and mistakes navigation
    rows for the current answer set.
- These are cross-language/multi-repo/config-shape issues, not Go or
  `internal/analysis` issues. The code-level guardrail is now a test matrix:
  - path-only, dot, path-identical, child-relative, child-full, and duplicate
    scope aliases;
  - mixed direct children across directories, config/manifest files, and
    ordinary source files;
  - consecutive source-inventory lens calls with different roles must render
    only the current query role set;
  - exact direct-child observations remain stored for coverage checks but do
    not appear in the visible lens rows unless the current query itself asks
    for that role;
  - advisory graph rows without exact member-level provenance never become a
    blocking candidate-universe contract.
- Commercial boundary:
  - hard blocks remain allowed only for exact provenance + typed exhaustive
    enumeration + strong alignment with model-authored principal member data +
    no model-authored exclusion/disclosure;
  - all broader repo-map graph/source-inventory rows stay advisory, so the
    system does not replace user/model intent or auto-fill answers.

Hard-gate audit after B2-T1, 2026-05-25 CST:

- Existing hard gates are not all equivalent. The safe class is "preserve
  model-authored structured data": if the model already emitted a principal
  `member_set`, finalizer must not shrink it. That is `preCheckAggregateMemberSetCoverage`
  and cardinality consistency. It can still be wrong only if an upstream
  supporting/advisory set was misclassified as principal, so the prevention
  point remains `AnswerAggregateFactRoleForRequest` demotion and typed request
  shape, not finalizer prose matching.
- Riskier class is "system observed candidates, model did not author them".
  B2-T1 deliberately makes this class advisory by default. It hard-blocks only
  when all of these typed conditions hold: exact non-recursive direct-child
  universe provenance, exhaustive enumeration request, model-authored
  principal `member_set`, strong alignment with that same universe, and no
  explicit exclusion/disclosure. One-off overlaps and advisory repo-map graph
  rows do not block.
- Other current hard member-set gates to keep auditing:
  - relation lookup handoff: only fires when `RequiresRelationMemberSetHandoff`
    is true and the model/evidence already selected qualifying relation members;
  - change-impact handoff: only fires when a typed `ChangeImpactProfile` requests
    files/sites as the principal output;
  - principal required-term handoff: only fires for answer-contract
    `must_include` terms;
  - enumeration label grounding: hard only for typed enumeration/member lanes
    and long identifier-shaped labels with a SymbolOracle result. During this
    audit, the no-typed-context path was explicitly changed to advisory-only so
    a local SymbolOracle cannot reject external-artifact/runtime labels when the
    request shape is unknown. This remains a possible future audit point for
    non-code external-artifact labels, but it does not consume raw user/model
    prose.
- Red line for future changes: any new hard gate that starts from system
  candidates rather than model-authored principal data must include a strong
  alignment check and a disclosure/exclusion escape hatch. Otherwise it belongs
  in advisory UI/hints, not in a blocking validator.

New-model s5b eval observation, 2026-05-25 CST:

- Run:
  `eval/results/b2t-candidate-universe/s5b-20260525-031319`, model
  `Qwen3.6-35B-A3B-4bit`, verdict PASS.
- Positive validation:
  - the direct `list_files internal/analysis` universe remained 25 directories;
  - the model used `repo_map(view="source_inventory")` early instead of only
    guessing files;
  - the final extraction/final answer preserved all 25 package rows, including
    `subject`;
  - string-wrapped `emit_evidence.items` was repaired by the shared structured
    payload compatibility path;
  - a guessed missing path (`internal/analysis/gate/registrations.go`) received
    an advisory same-scope candidate-file hint and recovered without hard
    blocking the model.
- Remaining generic gaps:
  - `explorer_iters=35`, `tool_read_file=40`, `midloop_inject=12`. After the
    first explorer lane had already emitted a complete 25-member investigation
    closure, the reconcile / summary exploration lane reopened the same
    bounded inventory and repeated reads for many already-grounded packages.
    Evidence de-duplication prevented answer corruption, but the workflow paid
    unnecessary model and tool cost.
  - The finalizer's first turn returned an empty end-turn with no tool call and
    no recoverable draft. The existing missing-document correction retry
    recovered and did not count as a rejection, but the behavior should remain
    visible in eval metrics because it affects latency.
- Design implication:
  - Candidate-universe correctness is now guarded well enough for this sample;
    the next high-ROI work is lane-level reuse. A downstream reconcile / summary
    lane should treat a prior exact-universe `emit_investigation_complete`
    closure plus accepted evidence/member_set as the default source of truth,
    and should ask for only missing/conflicting members instead of replaying the
    full source-inventory workflow.
  - This must stay generic: apply to any exact typed candidate universe
    (directories, packages/modules, config files, route/config/member
    inventories, multi-repo scopes, and non-Go languages). It should not infer
    from raw user/model prose. It should key off structured provenance,
    exact-universe coverage status, accepted evidence IDs, and model-authored
    closure facts.
- Candidate follow-up tasks:
  - [ ] B2-T2-A: expose prior exact-universe closure state to reconcile /
    summary lanes as a compact "already verified principal universe" handoff.
  - [ ] B2-T2-B: when a lane has the same universe fingerprint and no missing
    members/conflicts, prefer close/summary synthesis over repeated
    `read_file`.
  - [ ] B2-T2-C: add an eval metric for repeated reads of already-grounded
    source-inventory members across sibling/reconcile lanes.
  - [ ] B2-T2-D: keep finalizer empty-response retry telemetry separate from
    validator rejects so PASS runs still surface local-model latency risks.

B2-T2 root-cause refinement, 2026-05-25 CST:

- The repeated "summary/reconcile exploration" was not caused by missing JSON
  recovery or by a bad candidate-universe count. The first explorer lane had
  already emitted a complete model-authored `aggregate_facts.member_set` over
  the exact direct-child universe.
- The deeper fault was a two-axis inventory mismatch:
  - principal axis: exact members such as package/directory/module/config-file
    rows observed by `list_files` / source-inventory direct-child provenance;
  - attribute axis: candidate roles such as function/method/type/route/config
    key attached under each principal member.
- `source_inventory_profile.target_roles=["function"]` is valid guidance for
  the attribute axis, but the current answer-subject fallback promoted it into
  `answer_subject=function_name`. The chain-ranker / pre-complete subject gate
  then raised `RepairRebindSubject(function_name)` even though the accepted
  closure's principal member set was `package -> function`. That repair was
  non-advisory, so reconcile auto-complete refused to skip the summary node and
  the model replayed source-inventory reads.
- Commercial fix boundary:
  - do not globally disable subject constraints; they remain useful for true
    scalar/source-literal lookups such as "which function/config key/handler";
  - add a positive, typed-only proof helper:
    `SourceInventoryAcceptedClosureCoversExactUniverse(ctx, facts)`;
  - only when that proof is true and the blocking repair is the
    source-inventory-derived attribute subject, demote `RepairRebindSubject` to
    advisory for accepted-closure/reconcile auto-complete;
  - suppress reconcile-only fact retries after the same proof, so meaningless
    `0 of 0 discovered relevant files` coverage hints cannot reopen an already
    closed exact universe;
  - never synthesize missing members, never change model-authored facts, never
    parse user/model prose to decide this.
- Generality:
  - applies to package -> exported function, route -> handler, config file ->
    config key, module -> public type, class -> method, multi-repo service ->
    entrypoint, and mixed-language inventories;
  - the proof consumes only exact member-level provenance plus model-authored
    aggregate facts/exclusions, so advisory repo-map graph rows and one-off
    overlaps cannot become hard blockers or auto-complete proofs.
- Hard-retry audit rule carried forward:
  - if a system check starts from system-observed candidates, it may become a
    hard retry only after exact provenance, strong typed alignment, and an
    explicit model-authored answer/exclusion carrier are all present;
  - when the model has already provided a complete structured closure, later
    summary/finalization stages should prefer explaining/disclosing residual
    uncertainty over forcing another exploration round.

B2-U repo-map prompt and path-feedback hygiene, 2026-05-25 CST:

- New customer-observed gap:
  - the analyzer Workflow still described "repo_map and grep for each
    candidate entity", while runtime correctly rejects
    `repo_map(view="source_inventory")` during classification;
  - the rejection message used internal phase names and only pushed the model
    toward `emit_analysis`, without positively explaining which repo_map views
    are legal in classification and how to preserve a desired inventory request
    through `source_inventory_profile`;
  - the explorer Breadth Scan / explore-skill Workflow named repo_map only as a
    rough task_map/file discovery tool, so models missed the cascaded
    source-inventory lens that is intended to reduce broad search and repeated
    reads;
  - `repo_map(path=<bad path>)` reached the index loader before discovering the
    path problem, causing REPL scan-progress noise such as "indexing files" and
    "scan failed" for a model path typo.
- Root-cause classification:
  - prompt/tool-boundary surfaces drifted after `source_inventory` became a
    model-driven lens. The runtime boundary was correct, but the adjacent
    instruction and error surfaces did not explain the safe replacement path;
  - path legality and scope legality were separated: scope escape was blocked
    before scan, but "inside scope yet missing/file path" still reached scan.
- Contract update:
  - analyzer may use only lightweight location views:
    `repo_map(view=overview/task_map/file_map)`, `grep(files_only=true)`, and
    `list_files`;
  - analyzer must not expand `source_inventory`; it should encode the user's
    desired inventory shape in `emit_analysis.source_inventory_profile` so
    evidence gathering can expand and verify the rows;
  - explorer receives an explicit Repo Map Navigation primer: source_inventory
    is a navigation summary/checklist with roles, attribute_roles, scopes,
    counts, grouped scope rows, paging, languages, routes/config keys, and
    candidate files across repomap-supported languages, but every selected row
    still needs read_file/grep verification before evidence/citation;
  - repo_map preflights model-provided path existence and directory-ness before
    index/cache/file-scan work. Missing paths and file paths return a gentle
    tool summary with next-step guidance and emit no scan-notifier events.
- Guardrails added:
  - analyzer Workflow regression test prevents the old "for each candidate
    entity" wording and internal explore/analyze phrasing from returning;
  - analyzer boundary test asserts source_inventory rejection includes positive
    legal repo_map view guidance plus `source_inventory_profile`;
  - explorer initial-instruction test asserts the Breadth Scan contains
    cascaded source_inventory guidance;
  - repo_map scope tests assert missing/file paths fail before scan notices and
    do not surface generic "scan failed" noise.

B2-V repo-map prompt/internal-mechanism leakage audit, 2026-05-25 CST:

- Follow-up audit scope:
  - scanned static skill prompts for analyzer, explorer, extractor, finalizer,
    log/perf triage, write analysis, and change planning;
  - scanned dynamic model-facing hints in explorer and emit_evidence where
    repo-map navigation and evidence repair guidance are injected outside the
    static skill registry;
  - scanned model-facing tool descriptions/parameter schemas for the same
    internal-mechanism terms, because tool schemas are prompt text too.
- Findings:
  - `repo_map/source_inventory` teaching is present in analyzer classification
    and explorer breadth-scan prompts, but surrounding text still used internal
    terms such as "downstream synthesis", "the framework", "mid-loop
    observer", "stage tool allowlist", and "Plan-stage probe results";
  - dynamic explorer hints used the visible prefix `MID-LOOP CHECK`, which is
    an implementation label rather than an instruction the model can act on;
  - log/perf segmentation prompts also described later consumers as
    "downstream per-segment extractor", creating the same leakage pattern
    outside read-mode code questions.
- Fix strategy:
  - keep field/tool names that are part of the actual schema
    (`source_inventory_profile`, `emit_evidence`, `repo_map`) because models
    must call them correctly;
  - replace implementation topology words with neutral, task-facing wording:
    "answer synthesis", "final rendering", "later evidence handling",
    "later per-segment extraction", and "Progress check";
  - preserve all gating and retry semantics; these are prompt/hint hygiene
    changes only.
- Guardrails added:
  - extended `InternalTermsBlocklist` with the high-risk phrases found in this
    audit, so static skill prompt regressions fail `TestNoInternalTermsInPrompts`;
  - added an explore-skill regression test that pins cascaded Repo Lens teaching
    while banning the newly cleaned internal mechanism phrases;
  - added a runtime-hint source test so `MID-LOOP CHECK:` cannot re-enter the
    model-facing explorer / emit_evidence hint surfaces;
  - added a tool-schema hygiene test covering registered built-in and emit tools
    so descriptions/parameters cannot reintroduce the same internal phrases.

B2-W repo_map scoped projection / malformed tool-argument repair, 2026-05-25 CST:

- Customer-observed gaps:
  - `repo_map(path="CodeAgent/packages/cli/src", view="task_map")` rebuilt or
    loaded a subdirectory index even though an earlier analyzer overview had
    already warmed the `CodeAgent` graph;
  - local-model tool calls sometimes arrived as `}{"path":"..."}` or truncated
    `{"files_only":true,...` argument bytes. The generic repair covered trailing
    garbage and missing closing delimiters, but not safe leading delimiter
    carry-over, so `grep` / `repo_map` were rejected before execution.
- Root cause:
  - scoped projection assumed `Mutable.SearchGraph().Root == ctx.RepoRoot`.
    That is false in multi-repo / active-set posture: the warmed graph may be a
    sub-repo root (`CodeAgent`) while `ctx.RepoRoot` is the parent workspace.
    The requested path can still be a child of the warmed graph and should be
    projected from it instead of scanning the child directory again;
  - structural JSON repair ran during execution, but not before assistant
    history sanitization. Also, leading delimiter carry-over was intentionally
    treated as invalid before this batch.
- Fix contract:
  - scoped projection now tries both the supplied parent root and the cached
    graph's own root, but only when the requested scope is provably inside that
    graph root. The projection cache key uses the actual parent root that
    produced the projection, so sibling sub-repos cannot collide;
  - tool-call argument syntax repair now strips only safe leading delimiter
    garbage (`}`, `]`, `,`, whitespace) before a JSON object/array and then
    requires the remainder to parse as exactly one JSON value. It still refuses
    arbitrary text prefixes and double-object payloads;
  - structural repair runs before logging/history sanitization, so repaired
    tool calls remain visible as their real arguments instead of being replaced
    with `{}` in the next model turn.
- Guardrails added:
  - agent tests cover leading-delimiter repair, double-object refusal, and
    pre-history repair preservation for `repo_map`;
  - repo_map tests cover projecting a subdirectory from a warmed sub-repo graph
    while the session root is the parent workspace. This is cross-language and
    not tied to Go: the regression fixture uses TypeScript paths/symbols.

B2-X analyzer required_files path normalization, 2026-05-25 CST:

- Customer-observed gap:
  - analyzer emitted `required_files` such as
    `packages/core/src/mcp/token-storage/types.ts` while the active workspace
    path was `CodeAgent/packages/core/src/mcp/token-storage/types.ts`;
  - explorer trusted the stale suggestion, tried `read_file` on the missing
    path, then had to recover with `grep`, wasting a turn and making the
    navigation trace look unreliable.
- Root cause:
  - `emit_analysis.required_files` only performed string canonicalization
    (trim, slash normalization, `./` removal). It did not use the active-set
    path resolver or a file-existence check at the analyzer boundary;
  - REPL analysis summaries rendered the raw tool arguments, so even if a
    later consumer could repair a path, the visible "suggested files" list
    still showed the model's stale path.
- Fix contract:
  - required-file hints are now resolved through the shared active-set seed
    path gate when a `BusContext` is available. A unique active sub-repo match
    may auto-prefix the hint; a redundant current-repo label may be stripped;
    unresolvable or directory paths are dropped as advisory misses instead of
    causing a hard analyzer retry;
  - the accepted `emit_analysis` summary carries normalized required-file
    paths, and REPL rendering prefers those normalized paths over raw argument
    bytes. This keeps the user-visible navigation list aligned with what the
    system persisted;
  - the behavior is path/schema based only. It does not inspect user keywords
    or model prose, and it is language-agnostic across TypeScript, Java, Go,
    Python, config files, and multi-repo layouts.
- Guardrails added:
  - unit tests cover active-set auto-prefix, redundant repo-label stripping,
    unresolvable path drop, full `emit_analysis` persistence/summary behavior,
    and REPL summary rendering of normalized required files.

B2-Y finalizer no-tool recovery follow-up, evidence repair hygiene, 2026-05-25 CST:

- Eval batch signal:
  - `read_combo_log_current_source_explanation` previously failed because the
    finalizer's first rich `{blocks,citations,...}` draft was plain assistant
    text, then an isolated fallback dropped that draft from the final
    `ParseOutput` candidate set;
  - after preserving finalizer no-tool answer-document drafts, the focused case
    passes. The run also exposed two non-fatal long-tail costs: an
    `emit_evidence.items[].field_constraints` sidecar caused a strict-decode
    reject, and external artifact source `/dev/stdin` briefly appeared in an
    evidence repair target list.
- Root cause:
  - the answer-document recovery parser already existed, but no-tool draft
    candidates were not durable across isolated fallback message reset;
  - `emit_evidence` had compatibility repair for known scalar/key aliases, but
    not schema-neutral constraint sidecars that local/smaller models often add
    around otherwise valid fields;
  - evidence repair target construction treated every non-grounded source as a
    repo-readable path, even when the item represented an external runtime log
    lane rather than current checkout source.
- Fix contract:
  - finalizer no-tool answer-document-shaped content is stored in a bounded
    ledger before fallback isolation and recovered through the existing
    `RecoverAnswerDocumentV2FromText` path. Recovery is rendered as model draft
    recovery, not a fake successful tool call;
  - `emit_evidence.items[].field_constraints` / `fieldConstraints` are treated
    as schema sidecars only. Known fields are promoted only when the canonical
    field is absent; otherwise the sidecar is ignored. Ordinary unknown fields
    such as `note` remain fail-loud;
  - `ToolRepair.Targets` for evidence grounding now include only repo-local,
    file-shaped paths. Absolute paths are admitted only when provably inside
    `ctx.RepoRoot` and are normalized to repo-relative paths. External artifact
    lanes (`/dev/stdin`, command output, runtime logs) stay as evidence context
    but cannot drive `read_file` repair.
- Observability added:
  - eval summaries now include tool-history prune count, max context-token
    estimate, max context window, and max context-window percentage. The parser
    runs in byte mode and requires real control-plane log prefixes so quoted
    customer logs or model prose cannot contaminate metrics.
- Remaining follow-up:
  - `emit_investigation_complete.aggregate_facts` stringification is mostly
    recoverable when the string contains a JSON array, but prose strings still
    require a model correction;
  - `negative_observation` over current repo source should usually be expressed
    as source-backed absence / negative-search structure, not as a non-repo
    aggregate origin. The latest focused eval self-corrected this and passed,
    but it remains a useful future compatibility target if it recurs.

B2-Z random-serial eval follow-up and hard-gate boundary, 2026-05-25 CST:

- Eval batch:
  - result directory:
    `eval/results/random-serial-10-20260525-151839-provider-fixed`;
  - result summary: 9 PASS / 1 FAIL;
  - only failure: `read_combo_pipeline_sequence_table`.
- Deep root cause:
  - the model did answer the pipeline sequence, but final rendering diluted the
    verified stage/agent relation. The visible answer kept a generic
    `analyze -> explore -> extract -> finalize` loop, while the diagram body
    reused `Analyzer` inside the loop and omitted the concrete `explorer` and
    `extractor` actors required by the task;
  - the repository already contains a canonical typed relation in
    `internal/types/stage_binding.go`:
    `StageAnalyze -> AgentAnalyzer`,
    `StageExplore -> AgentExplorer`,
    `StageExtract -> AgentExtractor`,
    `StageFinalize -> AgentFinalizer`;
  - `answer_document_evaluator.go` renders requested answer dimensions as
    guidance, but there is no structural preservation lane for
    evidence-backed relations such as "stage -> agent -> output carrier".
    This lets the finalizer compress relation-rich evidence into a thinner
    prose/table answer.
- P0 batch plan:
  - add an evidence-backed relation surface for stage/actor bindings. It must
    be advisory-first and append clearly marked system-supplemented context
    only when the relation is already grounded in typed evidence or current
    repository source. It must not inspect user prose keywords or model prose
    to decide intent;
  - split exact targets into primary answer subjects vs context/background
    paths. Only primary structured targets may drive hard
    exact-resolution/absence gates. Background files should remain required
    reading hints or caveat context;
  - make close-ready and retry hints prefer scoped exact candidate universes
    over broad discovered-file pools. When a scoped universe exists, broad
    coverage hints must not ask the model to read unrelated files;
  - limit post-close-ready repair to one surgical grounding/line-validation
    turn unless a principal-blocking structured contract is still unresolved.
- P1 batch plan:
  - continue unifying finalizer JSON-as-prose recovery with the existing
    `RecoverAnswerDocumentV2FromText` and tool-layer JSON repair paths, without
    pretending a recovered draft was a successful tool call;
  - move work out of the semantic reviewer when the same issue can be enforced
    by structure, citation alignment, or candidate-universe contracts.
- P2 observability and regression plan:
  - metrics:
    `close_ready_ignored_count`,
    `broad_hint_after_scoped_universe`,
    `exact_target_context_demoted`,
    `finalizer_no_tool_recovered`,
    `stage_binding_surface_materialized`;
  - eval coverage:
    pipeline stage/agent sequence table with `explorer` and `extractor`,
    background-file exact-target demotion, scoped multi-language/config/route
    source-inventory enumeration, finalizer JSON-as-prose plus Mermaid, and
    close-ready long-tail repair.
- Hard-gate boundary:
  - hard rejection is allowed only when all four are true:
    (1) the primary user intent comes from structured IR, not keyword matching;
    (2) the candidate universe or citation target is machine-verifiable;
    (3) the model output conflicts with that verified fact on the same axis;
    (4) the system can provide a local, precise correction path.
  - otherwise the system must use advisory hints, caveats, reviewer notes, or
    preserved model text. It must not substitute system intent for user intent
    or force a rewrite merely because a heuristic is uncomfortable.
- Batch 1 implementation:
  - final-answer rendering now has a last-mile, source-verified stage-binding
    supplement. It activates only when the run already cited or grounded
    `internal/types/stage_binding.go`, then reads the current repo source and
    verifies all four read-mode bindings before appending a clearly marked
    system supplement;
  - it does not reject or rewrite the model answer, does not inspect user prose
    or model prose for control-flow decisions, and skips silently when the
    source file or any binding line cannot be verified;
  - regression tests cover both activation and the no-evidence no-noise path.
- Batch 2 implementation:
  - `emit_analysis` now demotes path-shaped `exact_targets` that duplicate
    `required_files` when the structured answer subject or validated entities
    show a non-file primary focus. The file stays as a required navigation
    hint, but no longer creates a hard exact-resolution / absence target;
  - pure file-subject questions keep their file exact target, so existence or
    file-specific lookups do not lose quality;
  - the rule is schema-based (`exact_targets`, `required_files`,
    `answer_subject`, `entities`) and language-agnostic. It does not inspect
    model prose or use user-question keyword control flow;
  - regression tests cover both demotion and preservation paths.
- Batch 3 implementation:
  - broad discovered-file enumeration hints now stand down when an exact
    source-inventory candidate universe is active. In that state the scoped
    universe remains the navigation checklist; unrelated broad file counts
    cannot override it;
  - retry hints also prefer the scoped candidate-universe summary over broad
    discovered-file coverage when such a universe exists;
  - close-ready evidence repair now grants only one targeted read branch before
    narrowing to emit-only. This preserves a model's chance to verify a real
    contradiction while preventing post-close-ready repair from reopening broad
    exploration;
  - regression tests cover broad-hint suppression and the close-ready repair
    quota.
- Batch 4 design correction:
  - a focused rerun narrowed the remaining failure to a lost display detail in
    a requested stage/state-carrier table. A first attempted fix appended a
    carrier-specific last-mile note; that was rejected as too case-shaped and
    was reverted before commit;
  - the generic replacement reuses the existing
    `requested_answer_dimensions` soft typed lane. That lane already validates
    each dimension against the current request, preserves model-authored
    labels/source quotes, and is explicitly presentation-only;
  - final rendering now appends a compact, localized "requested output
    dimensions" note only for dimensions whose validated source quote contains
    extra wording beyond the short label. This preserves explicit display
    requirements such as diagram type, table axes, comparison axes, log fields,
    config dimensions, or state-carrier examples without knowing or matching
    any specific term;
  - the note is append-only and advisory: it does not hard reject, does not
    rewrite model content, does not read model prose for control flow, and does
    not treat requested dimensions as evidence.
  - validation note: targeted unit tests and related package tests pass. A
    focused eval rerun was stopped because it spent more than three minutes in
    local `build_agent_context` before the explorer request was sent. That is a
    separate performance gap, not evidence that the dimension-preservation
    change failed, and should be handled by the agent-context assembly budget /
    cache workstream rather than by adding answer-specific logic.

## 2026-05-25 build_agent_context typed-relation prompt-hint stall

Root cause:

- The stopped focused rerun showed the explorer dispatch stuck in local
  `build_agent_context` for more than three minutes before any explorer model
  request was sent.
- The slow path is advisory prompt-hint construction, not evidence validation
  and not LLM/network latency:
  `BuildAgentContext` copies `AnalysisIR`, then probes typed-relation carriers
  to populate `AgentContext.TypedRelationHints`.
- For a request classified as a broad `call_chain`, the central typed selector
  can choose `called-by`. With broad sources such as lifecycle words, stage
  labels, or common function names, graph-backed prompt hints can fan out across
  all sub-repo graphs, source matches, files, and relations.
- This is safe semantically because prompt hints are advisory-only, but unsafe
  operationally because the work currently runs synchronously before the next
  LLM request and has no narrow-source guard or section-level diagnostic.

Commercial-grade boundary:

- Do not change prompts or infer from raw user/model prose.
- Do not disable coverage gates or exact relation checks that already operate
  on machine-verifiable, grounded evidence.
- Do not turn skipped hints into answer facts, caveats, or user-visible
  failures. A skipped prompt hint only means the model receives less navigation
  assistance for that relation family.
- Expensive graph-backed prompt relations (`called-by`, `references`,
  `extends`) may run only for narrow, exact symbol sources. Name-only or
  multi-match sources stay model-driven: the explorer can still use repo_map,
  grep, read_file, source_inventory, and explicit evidence tools to verify the
  path it chooses.

Task list:

1. Add section-level diagnostics inside `BuildAgentContext`, especially around
   typed-relation prompt-hint probing, so future stalls identify the exact local
   section instead of only the outer `build_agent_context` phase.
2. Add a narrow-source gate for prompt-hint relation probes:
   - keep cheap/exact families such as implementers, imports/exports, route or
     config evidence hints unchanged;
   - filter expensive graph-backed prompt families to sources that resolve to
     exactly one coverage-eligible source fact;
   - cap expensive prompt-hint source count per dispatch.
3. Add tests proving:
   - ambiguous `called-by` prompt hints skip without calling an expensive
     candidate provider;
   - exact `called-by` prompt hints still work;
   - implementer and cross-language relation hints still work;
   - coverage-gate provider behavior is not changed by this prompt-hint guard.
4. Re-run focused typed-relation/context tests, then the focused pipeline eval.
