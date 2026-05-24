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

- [ ] Locate the DAG explore transient retry and fork merge paths that discard
  or underuse inventory-progress state.
- [ ] Promote source-inventory advisory construction earlier, from successful
  structured tool observations such as `list_files`/`repo_map`, not only after
  accepted completion.
- [ ] Make sibling explore lanes and transient retries see the same advisory
  candidate artifact so they can avoid repeating broad directory enumeration.
- [ ] Preserve model authority: the artifact must remain advisory and typed;
  it can guide tool choice / missing-member coverage, but must not decide the
  user's answer or hard rewrite model prose.

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
