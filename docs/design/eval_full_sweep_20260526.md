# 2026-05-26 Full Eval Sweep

Status: stopped after Batch 09 per operator request; Batches 10-12 were not
started.

## Run Contract

- Workspace: `/Users/han/opt/small/codrax-small`
- Branch / commit at start: `main @ 660cbb7d`
- Sleep guard: `caffeinate -dimsu` was active during the sweep and stopped
  after Batch 09 completed.
- Case count: 118 `.case` files.
- Order: deterministic randomized order, seed `20260526`.
- Batch size: 10 cases, final batch 8 cases.
- Batch parallelism: 2 cases at a time.
- Per-case timeout: 2400 seconds.
- Results root: `eval/results/full-sweep-20260526-022850`.
- Case order file: `eval/results/full-sweep-20260526-022850/cases_order.txt`.
- Batch mapping file: `eval/results/full-sweep-20260526-022850/batches.txt`.

## Batch Log

| Batch | Cases | Verdict | Notes |
|---|---:|---|---|
| 01 | 10 | 9 PASS / 1 FAIL | FAIL: `u7h`. Slow/high-cost passes: `qf_multi_member_set_count_caveat` 1157s, `read_combo_answer_document_tools` 675s, `u7o` 495s, `u3b` 478s. |
| 02 | 10 | 9 PASS / 1 FAIL | FAIL: `read_combo_config_absent_present_mix`. Slow/high-cost passes: `read_combo_analyze_retry_anchor` 1429s, `read_combo_log_current_code_boundary` 613s, `u6a` 538s, `logtri_line_current_code` 447s. |
| 03 | 10 | 9 PASS / 1 FAIL | FAIL: `read_combo_member_set_closure_scope`. Slow/high-cost passes: `read_combo_trace_current_code_boundary` 558s, `s3d` 257s. |
| 04 | 10 | 9 PASS / 1 FAIL | FAIL: `u7i`. Slow/high-cost passes: `qf_type_relation_loop_controller` 1086s, `qf_called_by_typed_relation_query` 1078s, `read_combo_git_two_diffs_current_code` 689s. |
| 05 | 10 | 9 PASS / 1 FAIL | FAIL: `u7f`. Slow/high-cost passes: `read_combo_pipeline_sequence_table` 1262s, `qf_architecture` 826s, `cangjie_repomap` 404s, `u7k` 356s. |
| 06 | 10 | 9 PASS / 1 FAIL | FAIL: `s11a`. Slow/high-cost passes: `s5a` 693s, `u4b` 649s, `s1a` 485s, `m2a` 442s, `principal_span_inlined_helper` 383s, `u3a` 378s. |
| 07 | 10 | 6 PASS / 4 FAIL | FAIL: `read_combo_source_locations_required_false`, `u7e`, `patch_c_typo`, `qf_sequence_analyzer_gate`. Slow/high-cost passes: `read_combo_loose_multi_question_units` 811s, `u1b` 749s, `principal_span_adjacent_dispatch` 580s, `qf_diagram_pipeline` 266s. |
| 08 | 10 | 9 PASS / 1 FAIL | FAIL: `patch_go_typo`. Slow/high-cost passes: `u7g` 2108s, `qf_logic_view_read_pipeline` 1394s, `read_combo_git_current_source_explanation` 1109s, `logtri_degraded` 991s, `u5b` 867s, `u8a` 656s, `read_combo_multirepo_negative_interaction` 608s. |
| 09 | 10 | 7 PASS / 3 FAIL | FAIL/TIMEOUT: `logtri_warn_no_fatal`, `u1a`, `s5b`. Slow/high-cost passes: `m1a` 744s, `u11b` 707s, `u4a` 394s, `s7b` 291s. |
| 10 | 10 | not run | Stopped by operator after current Batch 09 completed. |
| 11 | 10 | not run | Stopped by operator after current Batch 09 completed. |
| 12 | 8 | not run | Stopped by operator after current Batch 09 completed. |

## Findings

Findings are appended after each batch. The first pass records symptoms,
metrics, and failure surfaces; after the full sweep, related symptoms are
clustered into deeper system causes and generalized optimization directions.

### Batch 01

- `u7h` failed on a current-source diff explanation surface check. The model
  answered the latest commit and cited changed code, but the final visible
  answer did not preserve an explicit "changed files / impacted paths" surface.
  This looks like a generic VCS/current-source answer-contract gap rather than
  missing evidence: diff answers need a stable, visible path/change/impact lane
  when the user asks for code-difference impact, without hard-rewriting model
  content when the relation is only advisory.
- `qf_multi_member_set_count_caveat` passed but took 1157s, with 23 explorer
  iterations, 17 `read_file` calls, 2 `repo_map` calls, and 10 midloop
  injections. The model confused grouped aggregate/member-set data with
  `emit_evidence.items[]`, causing schema errors such as aggregate-shaped
  objects carrying `label`/`members` inside evidence rows. This is a generic
  schema-boundary gap between evidence facts and completion aggregate facts.
- `read_combo_answer_document_tools` passed but took 675s, with 26 explorer
  iterations and 9 midloop injections. The model guessed registry/source paths
  before landing on the paired tool files. This suggests repo navigation results
  are not yet consistently turning into a low-noise "read these verified
  anchors next" handoff for tool/API definition questions.
- `read_combo_command_current_source_explanation` passed after the recent
  command-measurement fixes, but still needed 4 finalizer rejects and 3
  finalizer iterations. The visible failure was fixed, but the answer-document
  contract remains cognitively expensive for measurement-style answers.
- `u7o` passed but was another current-source/diff explanation case with high
  context and multiple repair hints. Its success compared with `u7h` supports
  the same root cluster: VCS/diff answers need a generalized path/change/impact
  surface lane, not case-specific wording.
- `u3b` passed but still spent 478s comparing source templates and numeric
  constraints. It should remain in the performance cluster for evidence
  selection and finalizer burden analysis.

### Batch 02

- `read_combo_config_absent_present_mix` failed despite collecting enough
  evidence. The final answer did distinguish the missing key from the existing
  key, but not in a stable surface form the eval expected. More importantly,
  the rendered answer opened with a global "exact target not found" warning
  even though this was a mixed-target question where one key was absent and one
  key existed. Root cluster: exact-target resolution needs per-target status
  for mixed present/absent questions, and final surfaces need to preserve an
  explicit "A is not B / do not use B as A" relation when the request asks not
  to confuse two nearby config keys.
- The same config case took 879s, with 29 explorer iterations, 18 `read_file`
  calls, 10 midloop injections, and 62k max estimated context tokens. The logs
  show repeated parallel exploration lanes re-checking absence and example
  values. Evidence already existed early; the remaining cost came from
  grounding/absence schema friction and repeated verification of the same
  three-layer absence boundary.
- `read_combo_analyze_retry_anchor` passed but took 1429s, with 30 explorer
  iterations, 17 `read_file` calls, 15 midloop injections, and 3 tool-history
  prunes. One concrete schema miss was `anchorSymbol` inside `emit_evidence`,
  rejected as an unknown field even though the canonical `anchor_symbol` was
  also present. Root cluster: schema-aware repair should cover safe field alias
  normalization for evidence rows, and prune checkpoints must keep accepted
  evidence stable before raw tool history is trimmed.
- `read_combo_log_current_code_boundary` passed but exposed multiple typed
  negative-search compatibility gaps. The model correctly realized grep
  zero-results should be `aggregate_facts.kind=negative_search`, but the tool
  rejected several minor structural variants: missing `repo`, nested
  `query.pattern`, and omitted `label`. These are safe structural-repair
  candidates because they do not change the negative result value; they only
  normalize dimensions around an already observed zero-result search.
- `u9a` passed but showed a local CPU hotspot rather than model wait:
  `build_agent_context section=typed_relation_probe` spent about 56s in
  explore, about 60s in extract, and about 60s in finalize for a small
  call-branch question. Root cluster: typed relation probing is doing repeated
  graph-carrier scans across stages; it needs caching, narrower carrier
  selection, or a budgeted top-k projection so relation hints do not dominate
  small questions.
- `u9a` also showed extractor prose using an incorrect background path while
  still passing because the final answer used grounded evidence. This is not a
  hard failure, but it reinforces that prose-only intermediate answers should
  be treated as advisory unless structured evidence supports them.
- `u6a`, `logtri_no_fatal`, and `logtri_line_current_code` passed but took
  429-538s on relatively bounded questions. They should be grouped under
  "short artifact/current-source questions still pay full multi-stage cost";
  later analysis should decide whether a safe fast path exists without
  bypassing evidence contracts.

### Batch 03

- `read_combo_member_set_closure_scope` failed after 1815s. The root symptom
  was not lack of evidence: the final answer listed the 11 matching enum types.
  It failed because visible text contained the literal placeholder
  `[excluded]` inside item descriptions. This is a generic surface-leak gap:
  exclusion/negative-role scaffolding must not leak machine placeholders into
  user-visible answers, especially when the user asks for "excluded
  non-target types" as a natural-language explanation.
- The same case exposed a severe request-duration gap. Analyzer iter 0 entered
  `llm_request` at 03:58:43 with `timeout=4m0s first_byte_timeout=40s`, then
  returned at 04:23:56 with a 184k-character no-tool prose response consisting
  largely of repeated "One small detail ... I'm ready. I'll emit." text. This
  suggests the streaming/request layer lacks an effective total stream duration
  or generated-content guard after the first byte arrives. The outer 2400s eval
  timeout eventually protected the batch, but the interactive UX would appear
  stuck for many minutes.
- After the first exploration had already read the full target file and emitted
  all 11 enum types, the reconcile path re-read `internal/types/evidence.go`
  and then 10 additional windows before re-emitting the same evidence. This is
  the same close-ready/reconcile re-exploration cluster seen in earlier member
  set tasks: accepted closure plus complete member_set should become a stable
  handoff, and later lanes should not restart broad or redundant reads unless a
  principal-blocking gap is machine-detected.
- `read_combo_trace_current_code_boundary` passed but took 558s and exposed an
  external-observation origin normalization issue. The model emitted
  `negative_observation` with `origin=attached_perf_trace`; the validator only
  accepted a smaller enum such as `runtime_artifact`, causing a retry. This is
  a safe normalization candidate: attached perf traces, logs, command output,
  VCS output, web/MCP/connector artifacts all need canonical external-origin
  mapping without changing the observation value.
- `s3d` passed but still had 9 answer-contract violations and 5 midloop
  injections. It belongs with the mixed present/absent config-key cluster from
  Batch 02: exact-target and absence lanes should preserve per-layer status
  without forcing repeated evidence repair.
- Fast log triage samples (`logtri_node`, `logtri_goroutine_dump`,
  `logtri_python`, `logtri_ruby`, `hilog_arkts_panic`) all passed quickly
  (67-93s, except 76s for ArkTS). These are useful baselines showing the
  pipeline can stay light when no current-source expansion is required.

### Batch 04

- `u7i` failed on the same VCS/current-source surface cluster as Batch 01
  `u7h`. The answer was semantically rich: it named the three commit hashes,
  summarized each commit, listed changed files, and described their relation.
  The visible surface still did not preserve one stable sentence tying "three
  commits" to "diffs/changes", which caused the regex check to miss it. This
  reinforces that VCS diff answers need a generic, explicit commit/change/path
  lane rather than case-specific wording.
- The `u7i` answer also showed VCS citation hygiene issues: duplicated answer
  sections, `git_show [layer: ]` with a blank layer label, and a soft-accepted
  member-set coverage warning where some changed-file members were omitted from
  the rendered list. These are not evidence-collection failures; they are
  final-surface and citation-format stability gaps for history/diff answers.
- `qf_type_relation_loop_controller` passed but took 1086s, with 32
  `read_file` calls, 24 explorer iterations, 10 midloop injections, and a
  46k-token context peak. The relation/implementation inventory expanded into
  many definitions and appeared to include test-only helper implementations.
  Root cluster: relation inventory needs scoped projection and production/test
  boundary metadata so implementation questions do not over-read or mix
  verification-only fixtures into principal answer candidates.
- `qf_called_by_typed_relation_query` passed but took 1078s, with 31 explorer
  iterations, 17 midloop injections, 12 `read_file` calls, and 3 tool-history
  prunes. A supporting wrapper evidence row (`PreCompleteContractCheck`) was
  repeatedly treated as repair-worthy even after the core caller relation was
  known. This belongs to the repair-debt grading cluster: principal-blocking
  relation gaps should block, but supporting-wrapper or advisory rows should
  not pull the investigation into long repair loops.
- The same typed-relation case exercised the new pre-prune checkpoint path:
  checkpoint handoff was injected before history trimming. That is a positive
  signal, but the surrounding loop still shows that pruning is a symptom of
  earlier over-expansion rather than a root cause by itself.
- `read_combo_git_two_diffs_current_code` passed but still took 689s with 7
  answer-contract violations and 6 midloop injections. Along with `u7h` and
  `u7i`, this makes VCS diff/current-source answers a clear optimization
  cluster: the system needs to preserve changed-file, current-source, and
  impact surfaces without asking the model to rediscover formatting.
- Patch and log triage cases in this batch (`patch_cpp_typo`,
  `patch_python_typo`, `hilog_cangjie_panic`, `hilog_mixed_arkts_cangjie`)
  passed quickly enough to serve as control cases while the heavier
  VCS/relation clusters are optimized.

### Batch 05

- `u7f` failed on a history/log/current-source correlation question. The final
  answer did identify the recent merge commit `aa27be488e` and discussed the
  relevant `emit_evidence` / `emit_investigation_complete` code path, but the
  visible answer did not keep a stable "judgment + basis + merge/commit"
  surface that the case expected. This extends the VCS/current-source answer
  surface cluster from `u7h` and `u7i`.
- `u7f` also exposed a more important semantic boundary risk: the user asked
  whether the runtime log rework was "possibly related" to the recent merge,
  while the final answer over-strengthened the verdict into a direct causal
  statement. The evidence showed a plausible relation between a merge touching
  `emit_evidence.go` and a current downgrade path in
  `emit_investigation_complete.go`, but not a machine-proven direct cause from
  the external log to the merge. Generalized fix direction: VCS+log diagnostic
  answers need a bounded verdict lane (`related`, `possibly_related`,
  `not_enough_evidence`, `not_related`) that preserves uncertainty and cites
  both history evidence and current-source evidence without upgrading
  correlation to causality.
- The same `u7f` run had 9 answer-contract violations, 7 midloop injections,
  and a 65k-token context peak despite only 5 `read_file` calls. It also began
  with an analyzer-resolved commit `31d4f32213` that was not present in the
  current repo history, then corrected to `aa27be488e`. This is a generic
  "history target normalization" gap: commit identifiers derived from helper
  scans or logs should be verified against actual git history before they enter
  the answer contract as strong targets.
- `read_combo_pipeline_sequence_table` passed but was the heaviest sample in
  the batch: 1262s, 21 `read_file` calls, 4 `repo_map` calls, 3
  `source_inventory_lens` observations, 6 tool-history prunes, 11 midloop
  injections, 5 analyzer iterations, 26 explorer iterations, 2 finalizer
  iterations, and 2 finalizer rejects. It also showed analyzer-stage prose
  answering before `emit_analysis`, then later recovered. Root clusters:
  stage-boundary discipline for analyzer, budgeted architecture/relation
  exploration, and finalizer contract cost for sequence/table requests.
- The same pipeline-sequence run gave a concrete local CPU hotspot: finalizer
  `build_agent_context section=typed_relation_probe` took about 16s, then
  `prompt_dynamic_instruction section=observation_ledger` emitted still-running
  diagnostics before the LLM request. This is a post-evidence local processing
  cost, not model latency. It belongs with the typed-relation/projection and
  observation-ledger algorithmic optimization cluster.
- `qf_architecture` passed but still took 826s, with 9 reads, 2 `repo_map`
  calls, 13 explorer iterations, 7 midloop injections, and a 65k-token context
  peak. Its final `emit_answer_document` was accepted, but answer-contract
  checking then performed an extra LLM-backed check that ran for more than a
  minute. General direction: accepted answer documents should avoid expensive
  semantic/reviewer passes unless a high-confidence structural risk remains.
- `cangjie_repomap` passed in 404s, proving the Cangjie cross-language path can
  answer correctly, but the model mostly used `grep` and `read_file` despite
  the repo index being warm and the case explicitly being a cross-language
  symbol enumeration. Metrics show `tool_repo_map=0`, `source_inventory_lens=0`,
  14 reads, 4 midloop injections, and 2 semantic-quality dispatches. This is a
  strong data point for the Repo Lens discovery problem: the index exists, but
  the model still defaults to raw search for broad symbol inventories.
- `u7k` passed but showed the same history/current-source fusion cost pattern:
  356s, 13 reads, 1 `repo_map` call, 15 explorer iterations, 8 midloop
  injections, 66k context peak, and 6 answer-contract violations. Analyzer
  emitted a very long visible self-classification before the actual
  `emit_analysis` call. This is a UX and stage-boundary issue: verbose
  reasoning about schema fields should not dominate the REPL scrollback when a
  compact tool call is the real stage output.
- `mr_focus_single`, `logtri_go`, `logtri_rust`, and `s1b` passed with moderate
  cost. They are useful controls: precise symbol/log questions are generally
  healthy compared with architecture, cross-language enumeration, and
  VCS/current-source fusion questions.

### Batch 06

- `s11a` failed on a boolean capability question: "before Explorer, may the
  analyzer stage call `read_file`?" The visible answer correctly said "no" and
  cited runtime guards (`isAnalyzerStageAllowedTool`,
  `FilterToolSchemas`, `analyzerToolNotAllowedGuidance`,
  `analyzerTerminalEmitOnlyHint`). It failed because it did not preserve the
  static skill/tool-surface layer expected by the eval:
  `AnalysisToolSuggestions`, `ToolSuggestions`, `buildToolSchemas`, or
  `validateAnalyzerPrescanToolCall`. Generalized root: capability/tool-surface
  answers must cover both layers: (1) skill config/tool suggestions and schema
  construction, and (2) runtime validator/filter rejection paths.
- `s11a` also showed the recurring analyzer-stage verbosity issue: before
  `emit_analysis`, the model printed a long schema self-check and even copied a
  raw JSON sketch. This is now seen across several batches and should be
  treated as a UX/stage-boundary issue, not a one-off model habit.
- `s5a` passed but took 693s, with 21 `read_file` calls, 27 explorer
  iterations, 12 midloop injections, and 18 concrete-value items. The first
  exploration reached `emit_investigation_complete`, then the system pulled it
  back into a second exploration of the same `LoopController` implementer set.
  This is a direct example of "accepted closure / member-set not becoming a
  stable handoff"; downstream checks should not trigger broad re-exploration
  when the model has already submitted a complete, machine-checkable set.
- `s5a` also highlights production/test scope ambiguity. The model eventually
  returned 8 production implementations plus 3 test stubs, which may be right
  for "all concrete types in the repository", but the system should surface
  production/test grouping clearly and avoid silently treating test helpers as
  principal production implementations.
- `u4b` passed but took 649s. The model found 11 true production importers of
  `internal/tool/ground`, but its first `emit_evidence` batch for import lines
  was ungrounded, forcing it to read 12 files just to make import evidence
  citable. Generalized root: import/package dependency facts need a structured
  import-edge evidence path from grep/repo_map/source_inventory so change-impact
  questions do not have to read every importer file to ground a simple import
  line.
- `mr_implementers` passed quickly enough (170s), but initially called tools
  with paths outside the active sub-repos and recovered by listing the run
  parent. Multi-repo tool guidance is working but still has avoidable path
  friction: tool results should make active repo roots and relative paths more
  discoverable without forcing the model to `ls` the harness directory.
- `s1a` passed but analyzer first hit a structural rejection by treating the
  user-stated "9 checks" as a count/scalar question. The model corrected this
  by treating "9" as an enumeration boundary, not the principal answer. This is
  a generalized numeric-boundary issue: a number in the request is not
  automatically a count question, especially when the user asks for order,
  behavior, or mechanism.
- `u9b` passed and correctly found the current behavior: one invalid
  `emit_evidence` item can be skipped while the rest of the batch succeeds,
  unless all items fail or the rejection-ratio gate trips. It still required 13
  explorer iterations and 7 midloop injections, showing that tool error-grain
  questions would benefit from a compact structured view of validator behavior
  and thresholds.
- `m2a` passed but had a serious transient task-contamination symptom: during
  exploration it suddenly emitted an unrelated `PF-INTERNAL-001/startTask`
  task description, read/listed unrelated paths, then recovered. It also
  repeatedly told itself `read_file` was unavailable while subsequently calling
  `read_file` successfully. This is a high-risk context hygiene/tool-surface
  consistency gap: no prompt or carry-forward ledger should leak unrelated eval
  task text, and repair/tool-availability hints must not contradict the actual
  tool schema.
- `principal_span_inlined_helper` passed but exposed the largest local
  algorithmic hotspot in this batch. Logs show `typed_relation_probe` taking
  about 40s in explore, 35s in extract, and 33s in finalize. The same run also
  expanded a small principal-span question into `evidence_in=2440` during
  `applyStageOutput`, with 638 concrete-value resolution chains before cap.
  Generalized root: concrete-values / typed-relation expansion needs stronger
  subject gating, caps, and stage-local caching so a narrow helper question does
  not flood downstream context with unrelated exact-resolution and artifact
  observation chains.
- `u3a` passed but took 378s and 18 explorer iterations for a bounded
  two-file comparison. It did correctly find that `iterationCapShouldStop` is
  shared, but the path required many windows. This belongs with the targeted
  comparison optimization cluster: when analysis has exact files and exact
  method names, source windows should be narrower and more directly centered on
  candidate methods.

### Batch 07

- `read_combo_source_locations_required_false` failed with
  `missing:orchestrator.go`. The model and analyzer initially identified a
  broad production candidate set for `CitationReq.Required=false`, including
  `internal/orchestrator/orchestrator.go`, but the final answer listed only two
  locations (`analyzer.go` and `contract_check.go`). This is not a wording
  problem; it is an enumeration-candidate closure problem. Grep/source
  discovery found a candidate universe, but exploration did not promote that
  universe into a stable checklist that must be resolved as present, absent, or
  excluded before close. Generalized fix direction: field-value lookup and
  assignment enumeration need a machine-checkable candidate universe from
  structured grep/repo_map/source_inventory results, with production/test/doc
  scope retained, but hard blocking only when the model itself claims complete
  coverage or the universe is exact and in-scope.
- The same `CitationReq.Required=false` run shows a classification/surface
  risk: final prose inverted or blurred "direct assignment" vs "struct
  initialization" in places, even though both cited snippets are struct literal
  initializations. The system should not invent a category label when the code
  shape is mechanically knowable. Safe generalized approach: derive code-shape
  metadata from parsed/line-level evidence (`assignment`, `literal field`,
  `comment/doc`, `test`) and pass it as advisory/citable structure; only hard
  reject if a visible category contradicts the cited code line.
- `u7e` failed on the history compare surface even though the answer contained
  rich sections for each commit, common points, and differences. The eval
  missed it because the final surface did not preserve a compact "共同点和不同点"
  line/table heading in one stable place. This extends the VCS compare cluster:
  two-or-more commit comparisons need a generic compare surface with
  `per-target summary`, `shared/common`, `differences`, and `relationship`
  lanes. This should be a presentation contract, not a semantic hard rewrite.
- `patch_c_typo` failed in write/apply mode. The plan JSON contained a correct
  single-line unified diff, but the apply step model emitted only
  `{ "path": "main.c", "kind": "patch" }`, so coder reported `apply
  incomplete (1 missing)` and the worktree kept `retrun buf;`. This is a
  write-mode handoff gap: once a valid ChangePlan with patch content is
  persisted, apply should be able to hydrate the patch from the plan ledger, or
  at least give a schema-aware repair prompt that asks for the missing `patch`
  field while preserving the stored plan. The model should not have to
  reconstruct byte-identical patch content from memory.
- `qf_sequence_analyzer_gate` failed after 1343s. The model discovered the
  key intermediate functions (`normalizer.Normalize`, `compiler.Compile`,
  `hdp.Plan`, `compiler.RecomputeBudget`, `binder.BindByRelevance`) and even
  wrote them in exploration prose, but the final answer's ordered list and
  Mermaid sequence kept only a sparse subset of earlier reconcile functions and
  `gate.RunWith`. Root: ordered call-chain evidence is not becoming a stable
  principal relation surface for finalizer. The system has typed evidence rows,
  but it still loses model-authored ordered chain summaries when rows are
  partially grounded or later forced reads perturb the slate.
- The same sequence case exposed the most severe Batch 07 performance pattern:
  after an already rich answer attempt, forced-read/keyword coverage pulled the
  model into unrelated files such as `registry.go`, `erm_completeness.go`,
  `explorer.go`, and `explorer_erm.go`. The model explicitly noted these files
  were not part of the `buildAnalysisIR -> gate.Run` call chain, but the system
  restarted exploration and eventually pruned history. Generalized fix:
  forced-read lists must be relation-aware and scope-aware. Once an accepted
  closure contains the requested source/sink path, "top keyword file not read"
  should downgrade to advisory unless the unread file is on a typed path,
  exact target, or unresolved principal candidate.
- `principal_span_adjacent_dispatch` passed but took 580s for a direct adjacent
  dispatch. It first completed the investigation, then reconcile forced a read
  of `facet_plan.go` because a generated resolution chain anchored outside the
  fetched slices. This is another instance of broad relation/concrete-value
  chains hijacking a narrow source-to-sink question. Chain generation should be
  gated by subject confidence and requested hop boundary, and out-of-scope
  support chains should enrich context rather than schedule mandatory reads.
- `read_combo_loose_multi_question_units` passed but took 811s, with 13
  `read_file` calls, 14 explorer iterations, 11 midloop injections, two
  tool-history prunes, and a 69k-token context peak. The two independent lanes
  (runtime config and Mermaid fallback) both accumulated verbose evidence and
  then hit prune. Multi-question decomposition works functionally, but needs
  earlier per-lane compaction: each lane should checkpoint accepted evidence
  and a short lane summary before raw tool results from sibling lanes flood the
  shared prompt.
- That multi-question run also showed tool-surface confusion inside parallel
  lanes: one lane was in an emit-only repair posture while the model still
  wrote that it needed to call `read_file`; another lane could still read. This
  is the same schema-aware repair gap seen elsewhere, but in multi-lane form.
  Hints must be lane-local and schema-aware so the model sees exactly which
  actions are available for that lane, without contradictory global wording.
- `u1b` passed but took 749s and hit a finalizer stream stall before retrying.
  The retry recovered, which is a positive signal for transport robustness.
  However, the generated principal step sequence included a `WriteFile` row
  that the same context described as "not directly user-controlled". This is a
  principal-boundary risk: self-excluded or advisory candidates should not be
  promoted into the authoritative principal ordered list unless another typed
  support lane proves they satisfy the user's requested risk predicate.
- `qf_diagram_pipeline` passed but extractor produced an invalid prose/JSON
  fragment (`{ "messages": [...] }`) before the pipeline recovered. This did
  not fail the eval, but it is a generic structured-output compatibility gap:
  extractor no-tool or wrong-tool-shaped JSON should either be recovered through
  the existing schema-aware path or surfaced as a low-noise retry reason, not
  silently become extra loop cost.
- Repo-map behavior in this batch was mixed. Initial full-repo scans in the
  first case rebuilt the root cache, while later calls usually showed cache
  hits. Some cases still made duplicate same-scope `repo_map` calls (`u1b`,
  `qf_sequence_analyzer_gate`) and then fell back to grep/read_file. This
  reinforces the Repo Lens discovery problem: the index is available, but the
  model is not consistently using a scoped source-inventory view to reduce
  broad grep and forced reads.

### Batch 08

- `patch_go_typo` failed with the same write-mode handoff pattern as Batch 07
  `patch_c_typo`: the plan artifact existed and included a `patch` change for
  `main.go`, but the post-apply worktree still contained `retrun` and lacked
  the expected `return fmt.Sprintf(...)` line. This strengthens the write-mode
  P0 cluster: once a validated ChangePlan contains a patch, apply must hydrate
  from the persisted plan ledger or issue a schema-aware missing-field repair
  for the existing plan. The apply stage should not depend on the model
  re-inventing patch bytes from memory.
- `read_combo_multirepo_negative_interaction` passed but exposed a hard
  relation-classification risk. The user asked whether there is a direct code
  dependency/interface between two repos and the correct answer is negative.
  Exploration collected enough absence/negative-search evidence, but
  `emit_investigation_complete` was repeatedly downgraded because the system
  treated the task like a positive typed-relation member-set handoff and
  complained that members lacked `support_ref`. General direction: negative
  relation questions should be represented as absence/negative-search plus
  scoped coverage, not forced into a positive relation member_set. Hard
  downgrade is only safe when the requested relation requires positive
  qualifying members.
- `read_combo_git_current_source_explanation` passed but took 1109s and
  showed history/current-source principal pollution. Finalizer obligations
  included helper/tool symbols such as `EmitAnswerDocument.Parameters`,
  `Execute`, `Description`, and `ParametersFor`, while expected principal
  items were commit/current-source artifacts such as `Commit`,
  `currentIterCommitSHA`, `bestAppliedCommitSHA`, and changed paths. This
  extends the VCS/current-source cluster: tool/helper symbols may support the
  explanation, but they should not become principal obligations for a history
  + current-implementation answer.
- The same VCS/current-source case rebuilt the root repo cache at the start of
  the batch, while later repo_map calls mostly hit cache. This suggests cache
  reuse is functionally present, but sweep-level runs can still pay a full root
  scan after binary/session changes. Later clustering should distinguish
  acceptable cold-start scans from avoidable repeated same-scope projections.
- `u7g` passed but took 2108s. Analyzer first hard-rejected a natural user
  dimension (`影响流程`) as if it had to resolve to a repo symbol, then retried
  broadly. This is a hard-refusal misuse: answer dimensions and natural
  language facets are not necessarily code entities. It also hit stream-stall
  retry paths that resumed by rediscovering git history/source evidence
  multiple times, indicating transient retry checkpoints are not yet strong
  enough to prevent broad re-exploration after a provider failure.
- `qf_logic_view_read_pipeline` passed but took 1394s. Analyzer emitted a
  large self-correction/prose JSON sketch before the real `emit_analysis`.
  Exploration later collected rich stage/BusContext/Mutable evidence, but
  forced-read and finalization still produced a heavy run. This joins the
  architecture/data-flow cluster: stage relationship summaries need stable
  handoff surfaces, while analyzer and extractor no-tool prose should be
  recovered or compacted without turning into extra loops.
- `u8a` passed and is a positive Repo Lens data point: the run identified the
  small `criterion` package and produced a large API/member slate. It still
  needed extractor prose-to-structure recovery and two finalizer iterations,
  so large API enumeration could benefit from compact member-set handoff and
  finalizer compression even when navigation is healthy.
- `u5b` passed but took 867s. The model found the dedicated tests
  `TestStopCondFired_TerminatesInsteadOfHotLooping` and
  `TestStopCondFired_RetryAfterFinalizeFailure_UsesExplorer`, plus
  configuration-only force-finalize attempt tests. However, after enough
  answer-changing facts existed, partial-read hints for the giant
  `runReadSchedulerLoop` function and repeated prunes still dragged the
  investigation. Narrow yes/no test-existence questions need an accepted
  closure path that treats large unrelated function coverage as advisory once
  the requested test/function evidence is grounded.
- `logtri_degraded` passed but took 991s on a Lorem Ipsum log. The log triage
  pre-stage recognized the attached text as low information, yet exploration
  turned into a source-code explanation of `log_triager` internals and read
  implementation files after a mid-stream stall. This is a high-value
  generalization gap: low-signal runtime artifacts should be able to answer
  with a bounded "no diagnostic signal / no actionable stack or error" result
  without current-source investigation unless the user asks how the system
  handles such logs. Transport retry checkpoints should preserve the degraded
  artifact conclusion instead of re-expanding into repo internals.
- `logtri_degraded` also showed a timeout-observability mismatch. One explorer
  request was logged with `timeout=4m0s first_byte_timeout=40s
  stall_timeout=2m0s`, but the run continued after a multi-minute wait and
  then resumed broad source reads. Later transport analysis should verify
  whether response-header, first-byte, mid-stream, and total-stream budgets are
  all enforced and logged consistently across retry paths.
- `read_combo_git_diff_hunk_current_code` and `u7d` passed at moderate cost
  and serve as healthy VCS controls: when helper/tool symbols do not pollute
  the principal slate, the history-diff plus current-source lane can produce a
  stable answer. The optimization target is therefore not "remove VCS
  contracts", but keep their principal/support boundaries stable.

### Batch 09

- Batch result: 7 PASS / 3 FAIL. Failures were
  `logtri_warn_no_fatal`, `u1a` (2400s wall-time timeout), and `s5b`.
  This batch was intentionally the last batch run; the remaining queued
  batches were not started after the operator asked to stop after the current
  running evals.
- `logtri_warn_no_fatal` is a small but high-signal artifact-lane failure.
  The log-triage pre-stage answered the user correctly in natural language:
  WARN exists on line 2, and there is no FATAL/crash because the worker
  completed with `status=ok`. The later analysis stage nevertheless
  classified the task as root-cause/current-risk against current code, causing
  exploration to search the repository for `WARN`, read `REPL.warn` /
  `cancel_listener`, and finalization to add a current-code decision surface.
  The final visible answer still said "FATAL/崩溃 不存在", but the eval matcher
  expected the narrower words "没有/未发现/0". Two root issues are present:
  pure attached-artifact presence/absence questions should not be promoted to
  current-source contracts unless the user asks how the current code produces
  the artifact; and artifact absence wording should be rendered with stable
  explicit negative phrasing while preserving the model's direct answer.
- `logtri_warn_no_fatal` also demonstrates that answer recovery can preserve a
  raw JSON-like first draft but still allow irrelevant current-code scaffolding
  into the accepted answer. A safer contract is: when an accepted log/perf
  artifact lane already contains positive/negative observation facts and no
  current-source claim is requested, finalizer should keep the principal
  surface to the artifact facts and put any current-code speculation behind a
  caveat only if explicitly introduced by the model.
- `hitrace_artifact_line_anchor` is the healthy control for the same class. It
  passed in 211s without repo tools, with aggregate scalar facts for line 5,
  8.0ms, and "not over 50ms". This confirms the runtime-artifact scalar lane
  can work when analysis/finalization do not force current-code surfaces.
- `u1a` timed out at the 2400s case limit. Early exploration collected the
  important taint facts for `exec_command`: `command := p.Command` is the
  taint source; shell dispatch through `NewShellCommandContext` is the sink;
  read-mode protections include `validateReadOnlyExecCommand`,
  `normalizeReadOnlyExecCommand`, command-substitution rejection, active-set
  gating, resource caps, and process supervision. The run then drifted into
  many rounds of mixed-origin/current-source nudge handling, stream-stall
  salvage, and helper-symbol expansion. The finalizer prompt's principal
  structure named internal helper topics such as
  `execCommandHasGitHistoryCommand`, `execCommandPayloadWithTypedOrigins`, and
  `execCommandGitOrigins`, which diluted the user's actual taint-source /
  sink / defense request. Root issue: observation-derived subjects and tool
  provenance helpers should enrich a security answer, not become required
  principal sections unless they satisfy the user-requested risk predicate.
- `u1a` also shows the close-ready / repair loop is still too soft for mixed
  origin work. The logs repeatedly injected "current-source evidence nudge"
  guidance after enough source evidence existed. The model kept searching for
  more helper functions instead of closing. The system needs a stateful
  consumption rule: once a source nudge has been satisfied by a bounded set of
  grounded source/sink/guard facts, the same nudge must not re-open broad
  navigation. Any remaining origin-specific observations belong in
  `aggregate_facts` / caveats, not more source reads.
- `m1a` passed but cost 744s, 10 `read_file` calls, 15 explorer iterations,
  and 7 midloop injections. It used `repo_map` productively for explorer /
  extractor collaboration, then got pulled into a forced read of
  `internal/agent/finalize_preview.go` because the surface term `extractor`
  appeared there. The answer was already supported by `TurnAArtifacts`,
  extractor comments, and actual tool handoff evidence. Root issue:
  same-surface symbol mentions in unrelated support files must not block
  accepted closure when the requested relation already has typed evidence.
- `u11b` passed but still took 707s. Analyzer's path-scoped grep prescan was
  correctly normalized to `files_only=true`, but explorer later fell back to a
  broad `CitationReq` grep with 344 lines, read a tool-documentation example
  in `emit_evidence.go:251`, and made the model explicitly exclude that
  comment as a non-production assignment. This extends the "tool/protocol
  noise" cluster: comments, prompt/tool-schema examples, test fixtures, and
  internal compatibility helpers should be marked as auxiliary candidates in
  source inventories and absence/member-set obligations unless the user asks
  about tool documentation itself.
- `u11b` also showed a REPL presentation issue: duplicate `emit_evidence`
  corrections were rendered as extra "记录" rows after the stable evidence row
  was already updated. This is low risk but noisy. Exact duplicate/no-progress
  feedback should be visible enough to explain progress, but should not make
  the user-facing evidence list look larger than the answer-grade snapshot.
- `s7b` passed in 291s and is a good measurement scalar control. The system
  used a path-scoped directory listing for classification, then grep counted
  25 exact `Kind... Kind =` declarations. This is the desired pattern for
  bounded literal/count questions: deterministic grep/measurement can be the
  authoritative value, with source lines used for audit rather than broad
  code reading.
- `u4a` passed in 394s but finalizer first returned an empty assistant message
  and had to be reminded to call `emit_answer_document`. This is the
  finalizer-no-tool recovery cluster in a benign form: recovery worked, but
  an empty response still costs a retry. The system should continue storing
  all shaped drafts, but also record empty/no-tool rates as provider or prompt
  health metrics.
- `logtri_oversized` passed quickly (128s) but opened by saying it did not see
  the attached log and tried `recall_memory`, which is unavailable in
  single-shot evals. The answer recovered, but the first step is a signal that
  large artifact availability is not always obvious enough in the exploration
  prompt. Runtime artifacts should have a clear "attached artifact available"
  carrier with a minimal preview and a direct tool/source name, so the model
  does not go to memory or repository search first.
- `patch_java_typo` passed and is a useful control against the previous C/Go
  patch failures. The Java plan path read `Main.java:16`, emitted a patch plan,
  and the persisted plan artifact contained `patch,bugfix` for `Main.java`.
  This suggests the write-mode patch handoff bug is not universal, but the C/Go
  failures still require plan-ledger hydration at apply time.
- `s5b` failed after 1273s despite collecting the full answer during
  extraction. It used `source_inventory` five times and correctly discovered
  25 `internal/analysis` subpackages and hundreds of function candidates. The
  model then over-verified by reading 55 files, hit 8 tool-history prunes, and
  still ended with finalizer no-tool fallback showing
  `{{PLACEHOLDER_REASONING_PLACEHOLDER}}`. The extractor prose contained the
  near-complete 25-row table, but finalization did not preserve it. Root
  cluster: source-inventory candidates are too strongly described as "not
  evidence", so the model feels forced to read almost every package before
  answering. The better boundary is: repo_map/source_inventory rows are
  machine-verified navigation and candidate-universe facts (directory names,
  files, symbols, counts, languages); they are not citation lines for semantic
  claims. For enumeration, they can safely establish the candidate universe and
  count, while only disputed/high-risk semantic labels need targeted source
  verification.
- `s5b` also demonstrates that the path-miss fallback works but fires too
  late. When the model guessed `risk/assessment.go`, `sourcemix/mix.go`,
  `stopcond/evaluator.go`, and `subject/candidates.go`, read_file returned
  same-scope source-inventory hints with real files and candidate functions.
  That is useful and generic. But the model had already chosen the "read every
  primary file" strategy. The source-inventory result itself should surface a
  compact "verified candidate file manifest" and entry-candidate confidence
  earlier, so guessed paths are less likely.
- `s5b` finalizer prompt said "Required-member floor is empty" even though the
  exploration/extraction had a 25-package slate. This is unsafe for enumerated
  source-inventory questions: an advisory candidate universe should not force
  the final answer, but once the model emits a complete member_set or extraction
  table with count/member consistency, finalizer should receive that slate as
  a soft principal floor. If the model later omits members, the system should
  prefer preserved extractor/exploration prose or a caveated recovered table
  over falling back to a placeholder.

## Cross-Batch System Clusters After Batch 09

- **Artifact-only questions are being over-promoted to current-code work.**
  `logtri_warn_no_fatal`, `logtri_degraded`, and parts of `logtri_oversized`
  show the same pattern: a runtime/log/trace observation can often answer the
  user directly, but analysis or finalization pulls in repo source because the
  question is labeled diagnostic/root-cause. The generic fix is an
  artifact-first answer contract: if the user asks what is present in the
  artifact, artifact positive/negative facts are principal; current-code
  investigation is advisory unless explicitly requested or required to explain
  provenance.
- **Repo Lens is being used, but its "not evidence" wording causes
  over-verification.** `s5b` used source_inventory successfully, then read
  nearly every package because the prompt frames repo_map as advisory-only.
  The distinction should be refined: directory/file/symbol/count rows from
  repo_map are verified navigation/candidate-universe facts, while semantic
  entry-point interpretation may need targeted verification. This lets the
  model rely on the index for scope, count, and candidate files without
  pretending those rows are final source citations.
- **Candidate universe handoff is not stable enough end-to-end.** Several
  cases have a complete slate in exploration or extraction but an empty
  required-member floor in finalizer (`s5b`) or forced broad reads after
  accepted closure (`m1a`, `u1a`, `u11b`). A stable contract should carry:
  candidate universe, model-selected member_set, count consistency,
  source-inventory provenance, and confidence/ambiguity flags through
  extractor and finalizer. Hard rejection remains unsafe unless the universe
  is exact and the model explicitly claims completeness.
- **Finalizer no-tool fallback still loses high-value drafts.** `s5b` had a
  rich extractor table but finalizer ended with a placeholder; `u4a` needed a
  no-tool retry; earlier batches showed JSON-as-prose drafts. The fallback
  order should include preserved extractor/exploration answer-shaped prose and
  rejected/no-tool drafts before displaying placeholders. Recovery must be
  labeled as recovered/caveated, not as a successful tool call.
- **Repair and close-ready hints can keep broadening after the answer is
  effectively known.** `u1a` and `s5b` both demonstrate repeated nudges:
  current-source evidence nudge, read-without-emit, and evidence repair. These
  hints are valuable, but after a close-ready state or a complete slate they
  must become one-shot and lane-local. Repeated same-key nudges should degrade
  to "close with current evidence and caveat unresolved branches" rather than
  keep encouraging more reads.
- **Internal tool/protocol surfaces pollute principal obligations.** Tool
  documentation examples (`emit_evidence.go:251`), helper provenance names
  (`execCommandGitOrigins`, typed origins), and schema/tool names repeatedly
  become answer candidates. The general defense is not a keyword blacklist:
  tag candidates by provenance and role (source implementation, test fixture,
  tool schema/example, prompt/compatibility helper, runtime artifact) and keep
  auxiliary provenance out of principal floors unless requested.
- **Transport and local-budget observability remains incomplete.** `u1a`
  ended by wall-time timeout after a large finalizer prompt; earlier batches
  showed stream stalls and long waits despite per-request timeout labels.
  Transport timeouts should distinguish connect/header/first-byte/mid-stream
  stall/total-stream duration, and retry should restore the last stable
  evidence checkpoint rather than re-expanding the task.

## Generalized Optimization Backlog

1. Add an artifact-first contract for runtime logs/traces/command output:
   preserve positive/negative observations, scalar values, and absence facts as
   principal without forcing current-source verification.
2. Reword Repo Lens teaching and schema text from "not evidence" to "verified
   navigation/candidate-universe facts; source citations still required for
   semantic claims". Include a compact candidate confidence/ambiguity summary.
3. Carry source-inventory candidate universe + model member_set + count
   consistency into finalizer as a soft principal floor. Only hard-block when
   exact universe, same-axis member_set, and explicit completeness claims all
   align.
4. Teach finalizer fallback to recover from preserved extractor/exploration
   answer-shaped prose and rich tables before showing placeholders or generic
   degradation text.
5. Make close-ready and read-without-emit hints one-shot per lane once a
   complete slate exists; subsequent repeats should recommend closure with a
   caveat instead of broad navigation.
6. Add provenance/role tagging for repo candidates so tool-schema examples,
   comments, tests, and compatibility helpers do not become principal answer
   members unless the user's question targets those surfaces.
7. Improve metrics for timeout and local CPU phases: record finalizer prompt
   build time, local contract-check time, provider header/first-byte/stall
   events, and whether a retry reused a stable checkpoint.

## Implementation Plan: Artifact / Repo Lens / Finalizer Handoff

This plan addresses the Batch 09 clusters without fitting any individual eval
case. The shared red line is that the system may preserve and surface
model-authored or tool-verified material, but it must not replace the user's
intent or invent a stronger answer than the model gave.

### Design Contracts

1. **Origin-specific facts are first-class support.** Runtime artifacts,
   command measurements, VCS/diff output, external documents, MCP/connector
   resources, and repo-map index rows each have their own support shape. They
   must not be coerced into current-source `file:line` citations unless a
   separate current-source lane exists and is grounded.
2. **Repo Lens has two layers.** `repo_map` / `source_inventory` can establish
   verified navigation facts: existing scopes, files, candidate symbols/routes/
   config keys, languages, and `count == len(members)` invariants. It is still
   not a semantic source citation for claims such as "this function implements
   the behavior" unless the selected file/symbol is verified with source text or
   another typed support lane.
3. **Candidate universes are advisory until the model closes them.** A
   source-inventory universe must not auto-write the final answer. When the
   model emits a same-axis member_set or extraction table with explicit
   completeness/count consistency, finalizer receives it as a soft floor. A
   hard rejection is allowed only when the universe is exact, the member_set is
   same-axis, the model claims complete, and the missing/extra member is
   machine-checkable.
4. **Fallback preserves user-visible model surfaces before system summaries.**
   If finalization fails to emit `answer_document`, recovery order is:
   accepted answer document, rejected/retry-state document, preserved finalizer
   no-tool JSON, preserved extractor/explorer answer-shaped surfaces, then
   evidence-only fallback. Placeholder/scaffolding text is never a better
   answer than an earlier rich model-authored table or diagram.
5. **Artifact-first does not mean source-blind.** If the request is
   observation-only, artifact facts can directly answer the question. If a
   typed current-source explanation lane exists, both lanes remain visible and
   separated. Ambiguous cases become caveats, not broad forced rewrites.

### Task List

- [x] Batch A: update Repo Lens wording and tests so the tool teaches
  "verified navigation/candidate-universe facts, not semantic citations" rather
  than the over-strong "not evidence" language.
- [x] Batch B: add a bounded model-authored surface handoff for extractor /
  explorer reports in typed-support finalization, with prompt and fallback
  guardrails that mark the surface as advisory and model-authored.
- [x] Batch C: harden fallback sanitization so placeholder-only finalizer text
  is ignored when richer preserved surfaces or evidence exist.
- [ ] Batch D: strengthen artifact-first finalizer tests for observation-only
  runtime/log/trace answers and mixed artifact/current-source answers.
- [ ] Batch E: verify source-inventory/member_set/count handoff with tests that
  use existing `SourceInventoryObservation` and aggregate-fact carriers instead
  of a new parallel schema.

### Risk Controls

- No user-question keyword or model-prose keyword drives control flow. The code
  may inspect structured tool parameters, structured observations, evidence
  origins, schema markers (`blocks`, markdown table/diagram shape), and stage /
  agent metadata.
- No automatic table/member completion from source inventory unless an existing
  structured carrier already says the model selected that set. Otherwise the
  system may only show a clearly labeled recovered/advisory surface or caveat.
- Prompt text changes must be generic tool teaching. They must not mention
  project-specific paths, package names, eval IDs, or Codrax-only relationships.
