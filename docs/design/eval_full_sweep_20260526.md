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
- [x] Batch D: strengthen artifact-first finalizer tests for observation-only
  runtime/log/trace answers and mixed artifact/current-source answers.
- [x] Batch E: verify source-inventory/member_set/count handoff with tests that
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

## Live Representative Eval Notes: 2026-05-26 10:04 CST

The representative sweep started after commit `6db40b20` with six cases and
parallelism 2:

- `read_combo_log_current_source_explanation`
- `read_combo_trace_current_source_explanation`
- `read_combo_command_current_source_explanation`
- `qf_diagram_pipeline`
- `s5b`
- `harmony/cangjie_repomap`

The first attempted run was invalid: the temporary `codrax` snapshot was placed
under the eval result directory, and provider discovery is anchored to the
binary directory. The run therefore failed in ~2s with "providers.yaml not
found" and all pipeline metrics at zero. This is an eval-harness hygiene issue,
not a model/runtime result. The valid rerun keeps the binary snapshot beside the
repo-root `providers.yaml`.

Live runner status has a separate formatting issue: when completed cases wrote
their status rows, stdout printed awk syntax errors and `DONE =>` without the
verdict token. The per-run `run-1.verdict` files and status rows still record
`PASS`, so this is a monitoring/reporting bug, not an answer failure.

### Observed Gaps While Run Is Still In Progress

1. **Mixed-origin artifact + current-source cases are still the slow path.**
   Both `read_combo_log_current_source_explanation` and
   `read_combo_trace_current_source_explanation` hit the representative sweep's
   1800s per-case timeout. The new default-disabled post-finalize reviewers are
   not the cost center; the bottleneck remains upstream exploration / evidence
   closure.
2. **Origin-specific current-source nudge can still over-repeat.** The trace
   case repeatedly hit `explorer.mid-loop.read-without-emit` /
   `read-without-emit-closure-only` while the prompt correctly says logs/traces
   must not be forced into `emit_evidence`. The hint is schema-aware, but it can
   still keep a mixed-origin case in a "read or close" loop instead of turning
   the artifact lane into a stable closure/caveat path sooner.
3. **Log case shows repeated upstream restarts / DAG-window reopening.**
   The log case ran log triage, analyzer, and explorer, then later entered a
   DAG-scheduled investigation window for the same timeout distinction. This
   suggests the current-source verification nudge can re-open broad exploration
   even after the runtime log observation is already clear.
4. **Emit-only repair hint is now aligned with tool surface, but may not be
   sufficient.** The trace case correctly injected an emit-only repair hint when
   tools were limited to `emit_evidence` / `emit_investigation_complete`; this
   fixes the old "tell model to reread while no read tool is exposed" bug. The
   remaining issue is convergence: after the hint, the model can still continue
   several turns instead of closing from accepted evidence.
5. **Transient stream-stall retry still risks broad re-entry.** The trace case
   hit `stream stalled mid-stream`, logged that durable progress was preserved,
   and retried with a checkpoint (`structured evidence rows=151`,
   `read files=9`, `successful tool results=65`). The next explore window was
   marked as a continuation, but it still restarted at a fresh explorer iter and
   soon re-entered `read-without-emit` nudges. The remaining gap is not whether
   artifacts are saved; it is whether the retry lane consumes the checkpoint as
   a closure-biased state instead of allowing another broad navigation cycle.
6. **Eval/log tooling must treat run logs as text even when NUL bytes appear.**
   Local `rg` over the trace run log reported `binary file matches (found "\0"
   byte around offset ...)`. Any metric extraction or forensic grep that does
   not force text mode can silently miss later lines. This is separate from the
   answer pipeline but can hide regressions in future sweeps.
7. **Reviewer opt-in change appears effective so far.** No
   `semantic_quality_reviewer` or `self_consistency_reviewer` dispatch has
   appeared in the active representative run. If the final summary confirms
   `sem=0` and `self=0`, token savings landed; remaining latency should be
   attributed to analyzer/explorer/finalizer proper rather than post-finalize
   reviewers.
8. **Close-ready after verification is still not a hard convergence boundary.**
   The log case reached `explorer.mid-loop.completion-ready-closure-only` at
   explorer iter 10 with ~67k estimated context, but still advanced to another
   LLM request. The hint correctly says the verification branch is consumed
   unless a concrete contradiction was found, yet the scheduler still allows
   another broad turn. The next fix should make this a state-machine transition:
   after close-ready + one consumed verification branch, only a contradiction
   evidence batch or `emit_investigation_complete` should be useful; otherwise
   the system should close with a bounded caveat rather than spend another
   navigation round.
9. **Analyzer scalar/source-quote validation may be too source-shaped for
   command measurements.** In `read_combo_command_current_source_explanation`,
   `emit_analysis` was rejected with
   `field_value_profile.source_quote must include both target and literal`.
   The question asks for a command measurement plus current-source explanation,
   so an early analyzer field-value profile should distinguish command/runtime
   literal carriers from source-code quote carriers. This should be solved by
   origin-aware structured validation, not by loosening every scalar question.
10. **Repo Lens wording is still inconsistent in at least one explorer prompt
    lane.** The representative logs still contain the old teaching:
    `do not treat repo_map output as evidence — it is a cached navigation index`.
    That conflicts with the newer boundary that repo_map/source_inventory rows
    are verified navigation / candidate-universe / count facts but not semantic
    source citations. This inconsistency can scare the model away from using
    Repo Lens for candidate universes or make it over-read files to re-prove
    path/count facts. The fix should update the shared explorer workflow
    snippet, not just tool descriptions.
11. **Evidence-nudge backlog is not limited to mixed-origin cases.**
    `qf_diagram_pipeline` repeatedly hit read-without-emit and emit-only
    evidence nudges even after collecting stage evidence, including an
    `emit_evidence` schema-normalization success for `surfaceTerms` →
    `surface_terms`. This suggests the evidence materialization loop can still
    dominate normal current-source diagram questions, not only artifact/source
    hybrids.
12. **Command-measurement mixed-origin runs can prune before closure.**
    In `read_combo_command_current_source_explanation`, explorer iter 5 reached
    `context_tokens_est=68335` and triggered `TOOL HISTORY PRUNED` while the
    active hint still said the current-source evidence nudge was unresolved.
    This shows the pre-prune checkpoint work needs a closure-oriented consumer:
    after command/runtime facts and any relevant source facts are already in the
    stable ledger, pruning should bias the next turn toward incremental close or
    bounded caveat instead of asking the model to keep reconciling raw history.
13. **Retry/repair can re-enter the initial broad-search workflow.** During the
    same representative run, both `qf_diagram_pipeline` and
    `read_combo_command_current_source_explanation` later logged a fresh
    explorer `iter=0` prompt that again said to "Search broadly" and produce a
    3-6 file list. This happened after earlier evidence collection, repair
    nudges, and/or pruning. That is a state-machine bug class: once a run has a
    stable evidence ledger or accepted closure candidate, transient retry and
    repair continuation should render from that checkpoint and should not fall
    back to the generic initial breadth-scan template unless the prior state is
    genuinely empty. The later `s5b` run repeated the same pattern after
    accepted evidence and prune checkpoints: it returned to explorer `iter=0`
    with a fresh pre-scan ranking for the same question. This confirms the
    issue is systemic across source-only enumeration, diagram, and mixed-origin
    cases.
14. **Presentation-format terms can pollute source discovery.** In
    `qf_diagram_pipeline`, the pre-scanned file ranking promoted
    Mermaid-rendering implementation files because the user asked the answer to
    be a Mermaid diagram. That term describes the requested output format, not
    necessarily the source behavior being investigated. Analyzer/explorer
    should keep answer-surface format tokens separate from domain/source
    retrieval tokens, otherwise diagram/table/JSON/Markdown requests can steer
    search toward renderer code instead of the requested system mechanism. The
    cross-language Cangjie case showed the same class: because the answer must
    report package declarations, the pre-scan promoted many Go files whose only
    match was the generic token `package`. Output-column labels and requested
    reporting fields need separate retrieval weighting from actual domain
    symbols.
15. **Parallel/restarted lanes can duplicate deterministic measurements.**
    The command-measurement case later opened multiple current-source/command
    explorer lanes, and one restarted lane re-ran
    `find internal/tool -name '*.go' ! -name '*_test.go' | wc -l`, producing the
    same deterministic count (`140`). Re-running is not semantically wrong, but
    it is unnecessary once the command result has been typed as
    `command_measurement`. Deterministic artifact/runtime measurements should be
    promoted into a stable, lane-shared ledger so later lanes can cite or
    reconcile against them without re-executing the same shell command unless
    the model explicitly needs a fresh measurement.
16. **Emit-only duplicate evidence is not treated as a close signal.** In
    `qf_diagram_pipeline`, after the tool surface was narrowed to
    `emit_evidence` / `emit_investigation_complete`, the model emitted a
    duplicate `PipelineStage` row. The tool correctly reported "No new evidence
    was recorded", but the scheduler continued into another LLM request instead
    of steering to close from the existing ledger or asking for only genuinely
    missing non-duplicate anchors. Duplicate-only emit results should be a
    convergence signal in emit-only repair mode, not a reason to keep spending
    full exploration turns.
17. **Slow PASS cases confirm reviewers are not the cost center.**
    `qf_diagram_pipeline` passed but took 1528s with
    `explorer_dispatches=4`, `transient_retry_checkpoints=3`,
    `midloop_inject=11`, `tool_read_file=17`, and
    `semantic_quality_dispatches=0`. `read_combo_command_current_source_explanation`
    passed but took 1547s with `tool_history_prunes=2`, `midloop_inject=10`,
    `tool_read_file=22`, `finalizer_iters=2`, and
    `semantic_quality_dispatches=0`. This supports the diagnosis that
    post-finalize LLM reviewers are no longer burning tokens by default; the
    remaining representative latency is in exploration convergence, retry
    restart, evidence materialization, and JSON-as-prose finalizer correction.
18. **Repo Map invocation alone is not enough; the view must match the task.**
    In the `s5b` representative run, the model did call `repo_map` on
    `internal/analysis`, but used the broad `task_map` view and received
    high-fan-in generic files such as context/render/logging/builtin surfaces.
    For source-inventory / entry-point enumeration, this is still noisy. The
    remaining Repo Lens gap is discovery of the right structured view and
    cascaded refinement path, not merely encouraging any `repo_map` call.
19. **Cascaded Source Inventory can arrive too late.** The same `s5b` run later
    reached `repo_map(view="source_inventory")` and produced the expected
    Cascaded Repo Lens Guide for a scoped package. That is the right interface,
    but it appeared after many prior messages and immediately preceded a
    `TOOL HISTORY PRUNED` event at ~62k context. Source-inventory discovery
    should fire earlier when broad repo_map/list_files/grep produces many
    scope candidates; otherwise the right lens helps only after the context has
    already been inflated. The same late-hint pattern appeared in the Cangjie
    cross-language case: the discovery hint arrived after six files had already
    been read, confirming this is not Go-specific.
    Once it did recover, the model expanded only one scope at a time even though
    the tool supports `scopes[]`. For broad but bounded inventories, the guide
    should make batched multi-scope expansion visible so 25 packages do not turn
    into 25 LLM turns.
20. **Structured support metadata leaks across tool schemas.** In `s5b`, after
    source-inventory guidance, the model emitted `emit_evidence.items[]` with
    `support_refs`. The `emit_evidence` schema rejected it as an unknown field.
    This is a generic schema-alignment gap: fields such as `support_refs` are
    meaningful in other evidence/answer lanes, so small and large models may
    naturally carry them into adjacent emit tools. The repair path should either
    safely strip non-semantic advisory metadata before validation or produce a
    concise schema-aware correction that does not restart broad exploration.
    The immediate retry then replaced it with another cross-lane advisory field
    (`summary_is_scalar`), which was also rejected. This should be handled as a
    class of harmless metadata fields, not one field at a time.
    A third retry then omitted `line_start` even though the earlier
    `support_refs` strings contained parseable `file:line` anchors. When the
    anchor can be recovered from the same item without changing semantics, the
    schema-aware repair layer should use that deterministic metadata rather
    than forcing another model round.
21. **Broad enumeration coverage can override exact command matches.** In the
    Cangjie case, the model executed precise grep commands and found the exact
    files/lines for `public class`, `extend`, and `foreign func`. The next
    system hint still said "read only 0 of 14 discovered files" and suggested
    several broad ArkTS corpus files that were not the exact grep hits. This is
    the same contract risk as earlier member-set issues: when a structured tool
    result has exact positive matches for the requested axis, broad discovered
    files must not become a hard or high-priority read requirement.
22. **Model-authored regex verification is brittle for cross-language modifiers.**
    The Cangjie run's follow-up grep used `public class|^extend|foreign func`,
    which confirms some rows but misses modifier forms such as
    `public sealed class` and `public abstract class`. The model had already
    discovered `Animal` and `Service` through `read_file`, but the verification
    command itself under-approximates the language grammar. Cross-language
    enumerations benefit from repo-map parser roles / source-inventory
    candidate universes because those can represent language-specific modifier
    variants without relying on ad hoc regexes written mid-run.
23. **Positive evidence coverage does not stop adjacent-category drift.** After
    the Cangjie run had already grounded the requested extend / foreign func /
    public class rows, it continued into `public interface` because a later
    broad check surfaced an adjacent declaration category. Adjacent categories
    can be useful caveats, but they should not extend the principal search
    axis unless the model explicitly promotes them as relevant to the user's
    requested buckets.

### Follow-Up Direction

- Treat mixed-origin closure as a first-class state: once artifact observations
  are typed and the current-source lane has either grounded a relevant mechanism
  or disclosed absence, repeated read-without-emit nudges should degrade to a
  closure/caveat instruction rather than reopen broad exploration.
- Treat close-ready verification as a bounded state, not a reusable hint. One
  verification branch may refine the answer; repeated no-new-evidence turns
  should converge to closure or a caveat.
- Add a metric for repeated mixed-origin evidence nudges:
  `mixed_origin_nudge_repeats` keyed by stage/run, so future eval summaries can
  show this bottleneck without manual log reading.
- Harden eval harness scripts to snapshot binaries beside repo-root config or
  pass `--providers "$ROOT/providers.yaml"` explicitly, and make log scanning
  use text mode for NUL-tolerant telemetry.
- Audit analyzer scalar/value validators for origin-specific carriers
  (`command_measurement`, runtime log/trace, VCS metadata, external docs).
  Source-quote requirements should apply only when the structured carrier says
  the literal is expected from current source text.
- Replace all remaining "repo_map is not evidence" prompt snippets with the
  precise two-layer boundary: verified navigation/candidate-universe/count
  facts are usable as such; semantic source-code behavior still needs
  read_file/grep/evidence grounding.
- Add a convergence metric for evidence-nudge loops in ordinary source-only
  questions, not just mixed-origin cases.
- Add a prune-before-closure metric that records whether the active hint at
  prune time was principal-blocking, surgical, or advisory. This should make it
  visible when pruning is happening because of useful investigation breadth
  versus because the system kept a non-closing hint alive for too long.
- Split the explorer breadth-scan template from continuation/retry prompts.
  Continuation prompts should start from the durable checkpoint and explicitly
  ask for one of: emit the missing structured rows, close with the accepted
  ledger, or record a bounded caveat. They should not re-teach broad search as
  the default next action.
- Separate answer-format constraints from retrieval intent. Terms such as
  Mermaid, table, JSON, markdown, or sequence diagram should influence final
  rendering and diagram-shape guidance, but should not automatically become
  high-priority source-search keywords unless the question is explicitly about
  the renderer/parser for that format.
- Make deterministic measurement facts lane-shared. The first successful typed
  command/runtime/VCS measurement should be available to later exploration
  lanes as a verified observation, with re-execution treated as optional
  verification rather than the default way to recover after retry.
- In emit-only repair/closure mode, treat duplicate-only `emit_evidence` results
  as a bounded no-progress state. The next action should be to close from the
  existing ledger, request a specific missing anchor if one is machine-known, or
  record a caveat; it should not loop through another broad LLM turn.
- Make Repo Lens discovery view-specific: broad `task_map` is useful for
  mechanism orientation, but scoped enumeration/entry-point questions should be
  nudged toward `source_inventory` / grouped scoped views as advisory
  navigation. The hint must stay structural (based on tool shape and result
  cardinality), not keyed to user wording.
- Trigger cascaded Repo Lens discovery before the first large read/prune risk:
  when structured tool results show many scopes/candidate files, surface the
  advisory `source_inventory` expansion path immediately instead of waiting
  until after multiple read_file rounds.
- Teach batched `source_inventory` expansion for bounded scope lists. When a
  prior lens/list_files result already has a finite set of scopes, suggest a
  single `repo_map(view="source_inventory", scopes=[...])` call for the next
  manageable page instead of only showing one-scope examples.
- Audit cross-tool advisory metadata fields (`support_refs`, candidate refs,
  provenance hints, source-inventory support rows). Where they do not change
  evidence semantics, make the parser strip or relocate them consistently;
  where they do change semantics, keep validation fail-loud but make the repair
  hint tool-schema specific and bounded.
- Rework enumeration progress hints to prefer exact positive match sets over
  broad discovered-file universes. If the model already has exact grep/repo-lens
  hits for the requested axis, broad coverage should be advisory context only,
  not a "read these next" instruction that can derail the narrowed path.
- Prefer parser-backed role/candidate views over model-authored regexes for
  cross-language declarations when the repo-map index supports the language.
  Regex remains useful as verification, but its result should not silently
  override already-grounded parser/read evidence when language modifiers or
  decorators create expected surface variation.
- Add an "adjacent category" boundary to enumeration closure: once requested
  buckets have exact grounded rows, new nearby symbol kinds should be recorded
  as optional caveat/advisory only, not as a reason to keep broad exploration
  open or mutate the principal answer axis.

### Post-Stop Code-Level Root Cause Breakdown

After the representative run was stopped, the valid result root was
`eval/results/representative-20260526-100433`. The run produced two timeouts
(`read_combo_log_current_source_explanation`,
`read_combo_trace_current_source_explanation`), two slow passes
(`read_combo_command_current_source_explanation`, `qf_diagram_pipeline`), and
two intentionally killed in-flight cases (`s5b`, `cangjie_repomap`). The code
inspection below treats killed cases only as partial telemetry, not verdicts.

1. **Continuation/retry can still re-enter the fresh breadth-scan prompt.**
   The explorer initial prompt unconditionally teaches "produce a FILE LIST"
   and "Search broadly" (`internal/agent/explorer.go:1373-1379`). Transient
   retry requeues the graph node (`internal/orchestrator/read_stage_retry.go:
   106-113`), and the checkpoint text is advisory only
   (`internal/orchestrator/read_stage_retry.go:209-214`). The agent loop also
   resets per-dispatch tool-result buffers at the start of every dispatch
   (`internal/agent/agent.go:1354-1362`). There is durable evidence handoff, but
   the prompt mode is still "fresh investigation" unless a later hint wins the
   wording battle. This explains the observed fresh `iter=0` broad-search
   restarts in mixed-origin, diagram, and source-inventory cases.

   General fix direction: introduce an explicit continuation prompt mode keyed
   off durable ledger/checkpoint state. Once any accepted evidence, aggregate
   fact, closure reason, deterministic measurement, or prune checkpoint exists,
   retries should start from "close / emit missing structured row / one narrow
   verification" rather than the generic breadth-scan template. This is a
   state-machine boundary, not a prompt patch for one question.

2. **Close-ready is implemented as a hint, not a convergence state.**
   `postCompletionReadySignal` tells the model to prefer
   `emit_investigation_complete` when readiness passes
   (`internal/agent/explorer.go:6952-7041`), but later evidence/coverage hints
   can still reopen work. Duplicate-only `emit_evidence` is correctly marked as
   no-progress by the tool (`internal/tool/emit_evidence.go:3183-3190`,
   `3240-3251`) and progress accounting ignores it
   (`internal/agent/explorer.go:6022-6035`), yet there is no terminal state that
   says "emit-only repair produced only duplicates; close from the existing
   ledger or name one exact missing anchor". This drove slow passes where the
   model kept spending turns after no new evidence was recorded.

   General fix direction: grade repair debt into `principal-blocking`,
   `surgical-grounding`, and `advisory`. After close-ready, only
   principal-blocking gaps may reopen the investigation. Surgical grounding gets
   one local attempt. Duplicate-only/no-progress repair in emit-only mode should
   converge to closure or a bounded caveat.

3. **Enumeration completeness still falls back to broad discovered-file
   universes.** Mid-loop and soft-stop enumeration gates compute coverage from
   grep/list/exec discovered files (`internal/agent/explorer.go:9265-9345`,
   `9990-10049`, `10366-10406`; path extraction at
   `internal/agent/explorer.go:15942-16036`). That broad universe is useful as
   a warning, but it can outrank exact positive matches or source-inventory
   candidates. The Cangjie partial run demonstrated this: exact grep/read
   evidence existed for requested buckets, but the system still pushed broad
   "read discovered files" coverage. The `s5b` partial run exposed another
   edge: a valid `source_inventory_profile` was dropped when the analyzer also
   set a relation-shaped predicate (`internal/tool/emit_analysis.go:2150-2158`;
   the relation predicate comes from `internal/types/request_traits.go:
   119-136`), so a package->entrypoint inventory fell back to relation-style
   handling.

   General fix direction: keep relation axes and source-inventory axes
   orthogonal. A request may ask for members and each member's related
   attribute; that should not erase the source-inventory candidate universe.
   Coverage gates should prefer the most specific machine-known universe in
   this order: exact source-inventory/checklist, exact positive match set,
   model-authored member_set, then broad discovered files as advisory only.
   Hard blocking remains allowed only when the universe is exact, scoped, and
   same-axis with the model's completeness claim.

4. **Repo Lens discovery exists but often fires too late or with the wrong
   shape.** The broad-result detector can recognize many scope groups from
   `grep`/`list_files` output (`internal/tool/source_inventory_reconcile.go:
   792-810`, broadness at `895-903`) and can render a source-inventory advisory
   (`918-954`). The cascaded guide is also present
   (`internal/tool/source_inventory_reconcile.go:1176-1229`), and the call
   renderer already supports `scopes[]` (`1270-1295`). However, the guide tends
   to appear after large reads or after many one-scope calls. In the partial
   `s5b` run, the model expanded 25 scopes as 25 parallel one-scope
   `repo_map(view="source_inventory")` calls even though the tool can batch
   scopes; in Cangjie it reached the right analysis profile but still first
   attempted a disallowed analyzer-stage source-inventory call.

   General fix direction: fire discovery immediately after a broad structural
   tool result, before the next large `read_file` or prune risk. The advisory
   should default to summary-first and show batched `scopes[]` expansion when a
   finite scope list is already known. It must remain advisory and based only on
   tool shape/result cardinality, not user keyword matching.

5. **Repo Map cache reuse is partial; cache hit does not mean cheap view
   rendering.** On cache hit, `loadFromCache` rebuilds the graph and runs
   `retrieve.RankGraph` for the query (`internal/tool/repomap/tool.go:
   682-712`). In-memory reuse still clones and re-ranks the graph for every
   query (`internal/tool/repomap/multigraph_facade.go:332-340`), and `task_map`
   rendering computes another query ranking (`internal/tool/repomap/render/
   render.go:1254-1262`). Scoped projection reuse works
   (`internal/tool/repomap/scoped_projection.go:62-95`), as seen by
   `projected scoped graph from in-memory graph`, but full-repo startup still
   paid 8-12s rank costs even on warm cache.

   General fix direction: add run-level query-ranking memoization keyed by
   canonical graph identity + query + view-relevant scope. Reuse the ranking in
   `task_map`/`source_inventory` renderers when the graph was already ranked for
   the same query. This is a performance optimization only; it must not change
   graph membership or evidence semantics.

6. **Answer-format/reporting terms still leak into retrieval ranking.**
   `formatKeywordResults` renders a "TOP PRIORITY" ranking and asks the model to
   read those files first (`internal/agent/keyword_search.go:1205-1226`). The
   initial explorer prompt then tells it to search broadly
   (`internal/agent/explorer.go:1373-1379`). Today the ranker cannot reliably
   distinguish domain/source terms from presentation terms such as Mermaid,
   table, package-column labels, or JSON/Markdown output requirements. This
   caused diagram requests to surface rendering files and Cangjie inventory to
   over-weight generic `package` surfaces.

   General fix direction: carry token provenance from analysis into search:
   domain identifiers, structural roles, exact paths/scopes, external-artifact
   terms, and answer-surface/reporting terms. Retrieval should rank by the
   first three categories; answer-surface terms should guide final rendering,
   not source discovery, unless the question is explicitly about the renderer or
   parser itself.

7. **Mixed-origin artifact/current-source closure is not a first-class
   lifecycle.** The current-source profile is intentionally soft
   (`internal/types/current_source_explanation_profile.go:54-67`), but slow
   mixed-origin cases show that once artifact facts and current-source facts are
   both present, the system still lacks a stable "enough to answer this
   mixed-origin contract" state. Prune checkpoints preserve counts
   (`internal/agent/agent.go:1426-1445`), but the next turn can still be led by
   raw-history repair hints instead of a compact closure contract.

   General fix direction: create a mixed-origin closure ledger containing
   artifact observations, deterministic measurements, current-source anchors,
   unresolved boundaries, and whether each requested mode has at least one
   model-approved support item. After the ledger is complete or bounded, future
   hints should ask for closure/caveat, not more broad source work.

8. **Schema-aware compatibility is strong for known structures but narrow for
   harmless cross-lane metadata.** The strict-decode layer already rewrites
   unknown-field and string-carrier errors into model-friendly repairs
   (`internal/tool/strict_decode_repair.go:43-81`,
   `internal/tool/strict_decode_remap.go:79-108`). It deliberately keeps
   arbitrary unknown fields fail-loud, and tests pin that behavior
   (`internal/tool/emit_evidence_test.go:1261-1265`). The partial `s5b` run
   showed that fields like `support_refs` and `summary_is_scalar` can be
   harmless advisory metadata copied from adjacent schemas, while line anchors
   embedded in `support_refs` can sometimes deterministically recover a missing
   `line_start`.

   General fix direction: keep strict validation for semantic fields, but add a
   small schema-owned "safe advisory metadata" class per tool. Safe fields may
   be stripped, relocated, or used to fill deterministic missing anchors only
   when parsing succeeds and the target value is unambiguous. Anything that
   changes evidence meaning remains fail-loud.

9. **Finalizer JSON-as-prose recovery works but still costs avoidable model
   turns.** The agent now captures answer-document-shaped no-tool drafts
   (`internal/agent/agent.go:1381-1390`), the finalizer can recover preserved
   drafts (`internal/agent/answer_document_evaluator.go:8481`,
   `8709-8744`), and text recovery is schema-driven
   (`internal/tool/answer_document_text_recovery.go:30-51`). The slow command
   PASS still paid an extra finalizer iteration because recovery is downstream
   fallback after nudging for the tool call. That is safer than losing content,
   but it remains latency-heavy for providers that often emit JSON-as-prose.

   General fix direction: when a no-tool draft is losslessly
   answer-document-shaped and no semantic validator would need model judgment,
   the finalizer can render recovered content immediately with a localized
   disclosure instead of spending another LLM round. Non-lossless recovery should
   keep the existing "preserve visible draft, do not pretend validation
   succeeded" behavior.

10. **Eval harness/telemetry issues hid some facts during live monitoring.**
    The custom representative runner's markdown table put `PASS` under the
    reason column for completed cases, while the per-case `run-1.verdict` files
    were correct. Several logs contain NUL bytes, so plain `grep`/`rg` without
    text-mode handling reports "binary file matches" and hides nearby telemetry.
    This is not product behavior, but it slows root-cause analysis.

    General fix direction: make eval summaries source verdict/reason from the
    same normalized fields and make log-mining scripts use NUL-tolerant text
    mode. These fixes should live in the eval harness, not runtime flow.

### Selected Observation Audit Matrix

This matrix cross-checks the live observations from the interrupted run against
runtime code. It intentionally groups repeated observations by system behavior:
the goal is to confirm classes of risk, not to fit a single eval case.

1. **Reviewer is not the dominant latency source.**
   Covers selections 1-4, 6, and 17. Confirmed. The representative command and
   diagram passes reported `semantic_quality_dispatches=0`, and
   `codrax.yaml.example` keeps `pipeline_semantic_quality_review_enabled` and
   `pipeline_self_consistency_review_enabled` off by default. The slow paths
   still spent most time in explorer iterations, read/evidence nudges, retry, and
   prune. This confirms the bottleneck is upstream evidence convergence rather
   than post-finalizer review.

   Follow-up: keep reviewer defaults off unless explicitly configured, and focus
   optimization on explorer convergence, mixed-origin closure, and finalizer
   recovery.

2. **The selected "misc issue bundle" is real, but spans several layers.**
   Covers selection 5. Confirmed and decomposed:
   - provider harness / valid-provider UX is a setup/runtime issue, not answer
     semantics;
   - mixed-origin slow path is a lifecycle issue (`current_source_explanation`
     plus log/trace/command artifact facts);
   - repeated current-source nudges come from read-without-emit and coverage
     hints;
   - DAG window reopen comes from transient retry requeue plus fresh explorer
     prompt mode;
   - emit-only repair still lacks a terminal duplicate/no-progress state;
   - NUL log bytes affect eval mining and telemetry, not model reasoning;
   - reviewer-off metrics confirm the above are upstream.

   Follow-up: keep these as separate backlog entries so one fix cannot be
   mistaken for resolving the entire bundle.

3. **Mid-stream stall checkpoints exist, but continuation can still behave like
   a fresh investigation.**
   Covers selections 7-8 and 18. Confirmed. Transient retry can requeue stage
   nodes (`internal/orchestrator/read_stage_retry.go`), the checkpoint hint is
   advisory, and explorer's initial prompt still teaches broad file-list search.
   The agent loop also resets transient tool-result buffers per dispatch, so a
   retry may be semantically "continuation" in state but "fresh search" in prompt
   framing.

   Follow-up: add an explicit continuation mode after durable progress,
   checkpoint, prune, or accepted closure. This mode should ask for closure,
   one narrow repair, or a caveat; it must not re-enter the broad search
   workflow unless no durable progress exists.

4. **Close-ready plus one verification branch is not currently terminal.**
   Covers selections 9-11. Confirmed. `postCompletionReadySignal` and
   `completion-ready-closure-only` are hints, not convergence states. Later
   read-without-emit, evidence repair, or coverage hints can still ask for more
   work after readiness. Duplicate-only evidence is recognized as no progress,
   but that no-progress status does not yet force closure.

   Follow-up: implement close-ready debt classes. After close-ready, only
   principal-blocking debt may reopen exploration; surgical grounding gets one
   local attempt; advisory/duplicate-only debt must become caveat or closure.

5. **`field_value_profile` currently overfits source-field lookup shape and can
   reject command/runtime scalar cases too early.**
   Covers selections 12-13. Confirmed. The analysis contract says
   `field_value_profile` is for named field/member/config literal values, while
   the validator requires target, literal, source_quote, quote containment, and
   owner-qualified field/member/config shape. The command measurement case used
   it for "count internal/tool non-test Go files", causing rejection before the
   investigation could use the more appropriate command-measurement lane.

   Follow-up: keep `field_value_profile` strict for source-field lookups, but add
   origin-aware scalar validation/routing so command, trace, log, VCS, and other
   runtime artifact measurements use their own typed scalar contract instead of
   being forced through source-field semantics.

6. **Repo Lens wording is internally inconsistent.**
   Covers selections 14-15 and the user's later hypothesis that the old warning
   may discourage repo_map use. Confirmed. `internal/skill/defaults.go` still
   says "do not treat repo_map output as evidence", while the newer explorer
   primer correctly distinguishes navigation/candidate-universe/count facts from
   source-code semantic proof. This contradiction can make models avoid the tool
   or treat its verified candidate counts as unusable.

   Follow-up: centralize a shared Repo Lens teaching snippet. The wording should
   say: repo_map/source_inventory is verified navigation and candidate-universe
   evidence, but source-code behavior claims still require read/grep evidence.
   This is a semantics clarification, not a prompt trick.

7. **Read-without-emit nudge is firing across source-only and mixed-origin
   cases.**
   Covers selection 16 and part of 17. Confirmed. The explorer has a dedicated
   `postReadWithoutEmitSignal`, soft-stop read-without-emit signal, and
   escalation paths. These are useful when the model is hoarding evidence, but
   the live run shows the same mechanism can keep nudging after the model has
   already accumulated enough context or after closure is the safer next action.

   Follow-up: make read-without-emit nudge state-aware. If close-ready,
   checkpoint, accepted evidence, deterministic measurement, or exact candidate
   coverage is already present, the nudge must ask for structured closure or a
   single named missing support item, not generic more reading.

8. **Presentation/output-format terms can pollute source retrieval.**
   Covers selection 19. Confirmed. The keyword search renderer elevates ranked
   files as "TOP PRIORITY", but current token handling does not reliably separate
   domain/source terms from answer-surface terms such as Mermaid, table, diagram,
   JSON, or Markdown. The diagram case showed ranking pressure toward Mermaid
   rendering files even when the user wanted a Mermaid-shaped answer about a
   different source subject.

   Follow-up: add token provenance to analysis/search handoff. Domain symbols,
   exact paths, scopes, and structural roles should rank source retrieval.
   Output-surface terms should guide final rendering unless the user is asking
   about the renderer/parser itself.

9. **Command measurements are reusable facts, but retry/parallel lanes can still
   re-execute them.**
   Covers selection 20. Confirmed. The system has typed origins for
   `command_measurement` and an observation ledger, and the command case
   successfully measured 140. It also re-ran the same count in a later branch and
   hit tool-history prune, which shows the measurement was not promoted into a
   lane-shared closure fact strongly enough.

   Follow-up: promote deterministic command/runtime measurements into the mixed
   origin closure ledger. Reuse them after retry/prune unless the model or tool
   output gives a concrete reason to invalidate them.

10. **Repo Lens/source-inventory exists but is not yet early or compact enough
    for broad searches.**
    Additional confirmation related to the selected repomap observations.
    Confirmed. Source-inventory discovery can parse broad `grep`/`list_files`
    scopes, render a cascaded guide, and accept batched `scopes[]`, but the model
    still often reaches it after large reads or expands many scopes as one call
    per scope. In `s5b`, a relation-shaped analysis path also dropped
    `source_inventory_profile`, which erased a useful candidate universe.

    Follow-up: preserve relation and inventory axes simultaneously, trigger Repo
    Lens discovery immediately after broad structural results, and prefer batched
    scoped expansion when finite scopes are already known.

11. **Repomap cache hits can still spend CPU on graph ranking/rendering.**
    Additional confirmation related to the selected repomap/cache observations.
    Confirmed. Warm cache still rebuilds/ranks graph views; scoped projection can
    reuse in-memory graph state, but query ranking and task/source-inventory
    rendering can repeat. This explains why "cache hit" in the UI does not always
    mean "near-free" for user-perceived latency.

    Follow-up: add run-level ranking memoization keyed by canonical graph identity,
    query, view, and scope. This must be performance-only and must not change
    membership semantics.

12. **Eval/log tooling itself needs hardening to avoid hiding root-cause data.**
    Covers the NUL/log-mining part of selection 5. Confirmed. Per-case verdicts
    are reliable, but the custom representative summary misplaced PASS/reason
    values, and binary/NUL logs can hide surrounding telemetry in default grep
    modes.

    Follow-up: normalize eval summary fields from the same verdict source and
    make log mining NUL-tolerant by default.

### Consolidated Backlog From This Representative Run

### Generalized Delivery Plan

This plan turns the audited gaps into implementation batches. The batches are
ordered to reduce model confusion first, then bound retry/closure loops, then
optimize latency. Every runtime change must obey the same red lines:

- Do not infer user intent from keyword matching over the user's question or
  model prose. Use structured request profiles, tool schemas, tool results,
  accepted evidence, and durable ledgers.
- Do not make system-authored facts replace model conclusions. System additions
  are advisory, caveats, or verified navigation facts unless a validator can
  machine-prove a direct conflict.
- Hard rejection requires all four conditions: structured user/contract intent,
  exact machine-verifiable candidate or citation set, model output in direct
  conflict with that set, and a local repair path. Otherwise prefer warning,
  caveat, preservation, or bounded continuation.
- Preserve rich upstream summaries, runtime artifacts, command/VCS results,
  external documents, web/MCP/connector observations, source-inventory rows, and
  accepted closure notes through the same observation-ledger path whenever
  possible. Do not invent parallel evidence stacks.

Batch P0-A — contract wording and analyzer scalar safety:

- Replace the remaining "repo_map is not evidence" wording with the precise
  Repo Lens boundary: repo_map/source_inventory can prove verified navigation,
  candidate universes, counts, scopes, languages, and existing files/symbol
  candidates; semantic source-code behavior still needs read/grep evidence.
- Keep analyzer field/value lookup strict for real source field/member/config
  literals, but drop optional invalid `field_value_profile` when the model has
  clearly used it for a generic scalar/count/current-source bridge rather than a
  parseable owner-qualified field target. This avoids early analyzer retries
  without weakening true `Owner.Field = literal` checks.
- Add prompt and tool tests so future edits cannot reintroduce the wording
  conflict or over-broaden field-value compatibility.

Batch P0-B — continuation after durable progress:

- Add a continuation-mode render path for transient retry / prune / accepted
  evidence checkpoints. Once durable progress exists, retries should start from
  closure, one narrow missing support item, or caveat, not from the fresh breadth
  scan template.
- Record metrics for `fresh_breadth_after_checkpoint` and
  `checkpoint_continuation_rendered`.

Delivery status:

- Implemented in `explorer.BuildInitialInstruction`: system-generated
  transient stream retry / tool-history prune checkpoint directives now render a
  checkpoint-continuation instruction instead of the fresh breadth-scan
  workflow. The continuation path is intentionally narrow: close from the
  accepted evidence, verify one exact missing anchor, or preserve a caveat.

Batch P0-C — close-ready and read-without-emit convergence:

- Classify repair/read debt as `principal-blocking`, `surgical-grounding`, or
  `advisory`.
- After close-ready, allow only principal-blocking debt to reopen exploration.
  Surgical debt gets one local attempt; duplicate-only/no-progress repair and
  advisory debt must converge to closure or caveat.
- Make read-without-emit hints state-aware: after accepted evidence, exact
  candidate coverage, deterministic measurement, checkpoint, or close-ready,
  ask for structured closure / one named missing support item rather than more
  generic reading.

Delivery status:

- First guard implemented: once completion-ready has fired, a navigation-only
  verification branch without structured progress can receive exactly one
  closure-only redirect. The hint uses a stable key and a per-dispatch latch, so
  later same-shape navigation does not keep injecting fresh "continue verifying"
  permission. This remains advisory; it does not hard-stop the model or hide
  any model output.

Batch P0-D — exact candidate universe precedence:

- Preserve relation axes and source-inventory axes together; do not drop
  `source_inventory_profile` solely because the request also asks for a related
  attribute.
- In enumeration/read-hint gates, prefer exact source-inventory/checklist or
  exact positive match sets over broad discovered files. Broad discovered files
  remain advisory unless the model claims exact completeness against the same
  axis and the exact universe is machine-known.

Delivery status:

- Exact candidate-universe gaps now outrank broad discovered-file coverage in
  mid-loop, soft-stop, and ParseOutput retry paths. A partial model-authored
  `member_set` can no longer be promoted complete just because
  `emit_investigation_complete` was called when the same exact universe still
  has uncovered or undisclosed members. The repair remains checklist-only: the
  system asks the model to verify, exclude, or caveat missing members; it does
  not auto-fill the answer set.

Batch P0-E — mixed-origin closure ledger:

- Promote deterministic command/runtime/VCS measurements and artifact facts into
  a lane-shared closure ledger.
- After retry/prune, reuse those facts unless invalidated by a new tool result.
  Do not re-run deterministic measurements by default.

Delivery status:

- Implemented the first durable baseline: ObservationLedger inputs now merge
  same-dispatch tool results from both agent and bus contexts, so a successful
  command/runtime/VCS/external observation can be projected before Turn A
  handoff or ParseOutput. Tool-history prune checkpoints render a compact typed
  observation snapshot, and transient explore retry checkpoints include
  origin-specific observation counts. These snapshots are advisory recovery
  baselines only: they preserve structured tool facts across retry/prune, but
  they do not decide sufficiency, synthesize answer content, or override model
  judgment.

Batch P1:

- Move Repo Lens discovery before large reads when a broad structural tool
  result exposes many scopes. Render summary-first guidance plus batched
  `scopes[]` expansion.
- Add token provenance to search ranking so answer-surface terms such as
  Mermaid/table/JSON/Markdown do not outrank domain/source terms.
- Extend schema-owned safe advisory metadata repair for harmless cross-lane
  fields while keeping semantic fields strict.
- Fast-path lossless finalizer JSON-as-prose recovery with localized disclosure.
- Add run-level repomap ranking memoization keyed by graph identity, query,
  view, and scope.

Delivery status:

- Finalizer no-tool preservation now covers both answer-document-shaped JSON
  payloads and rich visible model drafts (tables, fenced diagrams, box drawings,
  long structured lists) using the existing structural surface detector. If a
  later isolated fallback resets message history, ParseOutput can still recover
  the best structured JSON draft first, or display the preserved visible draft
  with a localized degraded-output disclosure. This does not infer answer
  semantics and does not mark the draft as validator-approved.
- Repo Lens discovery hints now emit an explicit control-plane log metric
  (`repo_lens_discovery_hints`) whenever the system surfaces an advisory
  source-inventory/relation-map cascade after a broad navigation result. This
  lets evals distinguish "no hint emitted", "hint emitted but model ignored it",
  and "model actually called source_inventory" without scraping prompt prose.

Batch P2:

- Harden eval summary/log mining for verdict correctness and NUL-safe scanning.
- Add dashboards/metrics for the P0/P1 convergence and cache signals.

Delivery status:

- Eval metric helpers now treat metric/log files as text even when attached
  traces inject NUL bytes. `eval_metric_field` uses binary-safe grep, and the
  runner contract test covers both metric extraction and pattern counting with
  embedded NUL data. This keeps post-run analysis from silently dropping
  timeout/prune/Repo Lens signals in trace-heavy customer cases.

P0:

- Add continuation prompt mode after durable progress/prune/transient retry so
  retries cannot fall back to the fresh breadth-scan workflow.
- Convert close-ready plus duplicate-only emit repair into bounded convergence:
  close, exact missing-anchor repair, or caveat.
- Preserve source-inventory and relation axes simultaneously; do not drop
  `source_inventory_profile` solely because a relation-like attribute is also
  requested.
- Prefer exact candidate universes / exact positive match sets over broad
  discovered-file coverage in enumeration gates.
- Add mixed-origin closure ledger for log/trace/command/VCS/external artifact
  plus current-source questions.

P1:

- Move Repo Lens discovery earlier and make batched `scopes[]` expansion
  prominent for finite scope lists.
- Add run-level query-ranking memoization for repomap cache hits and in-memory
  graph reuse.
- Add token-provenance-aware retrieval ranking so output-format/reporting terms
  do not pollute source discovery.
- Extend schema-owned safe advisory metadata repair for harmless cross-lane
  fields such as support/provenance hints.
- Fast-path lossless finalizer JSON-as-prose recovery with explicit disclosure.

P2:

- Add metrics for `fresh_breadth_after_checkpoint`,
  `duplicate_emit_after_close_ready`, `broad_coverage_after_exact_universe`,
  `repo_lens_discovery_before_first_large_read`, `repomap_rank_cache_hit`, and
  `mixed_origin_closure_ready`.
- Harden eval summary/log mining for verdict column correctness and NUL-safe
  searching.
