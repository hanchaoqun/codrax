# 2026-05-20 Full Eval Sweep Gap Tracking

Status: complete

This ledger records findings from the 2026-05-20 full eval sweep. The sweep is
evidence-only: new eval coverage was added and pushed before the run, but product
code is intentionally not modified while logs are being collected and audited.

Hard rule for follow-up fixes: hard gates may consume only precise typed
signals. Soft reviewers, model prose, grep/ranker noise, and rendered answer text
must not become hidden hard control flow. When a run passes but ships a visibly
thin or misleading answer, record it here before choosing a fix.

## Sweep Snapshot

- Workspace: `/Users/han/opt/codrax`
- Sweep start: `2026-05-20T03:03:26+08:00`
- Results root: `eval/results/full-20260520-030326`
- Runner snapshot: `./.codrax-sweep-20260520-030326`
- Cases: 82 (`eval/cases/*.case` plus `eval/cases/harmony/*.case`)
- Parallelism: 2
- Timeout: 1800s per case
- Randomized order: false
- Sweep complete: `2026-05-20T05:33:02+08:00`
- Result: 80 PASS / 2 FAIL of 82
- Failing cases: `u7b`, `arkts_repomap`
- Sweep discipline: no product-code changes were made during log collection / audit after the eval-case commit.

## Added Eval Coverage Before Sweep

- `read_combo_criterion_rich_functions`
- `read_combo_source_locations_required_false`
- `read_combo_pipeline_sequence_table`
- `read_combo_multirepo_negative_interaction`
- `read_combo_analyze_retry_anchor`
- `read_combo_answer_document_tools`
- `read_combo_config_two_knobs_precedence`

## Post-Sweep Eval Coverage Additions

- `read_combo_config_absent_present_mix` — medium-priority guard for mixed
  exact-resolution answers where one config target is absent and another target
  is present.
- `u7h` / `u7i` / `u7j` — history/diff shape guards for single-commit diff
  explanation, fixed multi-commit diff rollup, and all-history topic search.
- `u7m` / `logtri_no_fatal` / `hitrace_no_long_gc` — non-repo negative
  observation guards for git-history no-hit, log no-fatal, and trace no-long
  span questions.
- `u7n` / `logtri_warn_no_fatal` / `hitrace_gc_present_no_long` — mixed
  positive+negative observation guards, ensuring a bounded absent target does
  not erase an observed present target in the same answer.
- `u7l` expectation was tightened after the 2026-05-23 replay to require at
  least one commit-like hash in the final answer, so a recent-N history answer
  cannot pass with only a compressed overview.

## 2026-05-23 Targeted Principal-Ledger Replay

- Results root:
  `eval/results/principal-ledger-u7l-fix-20260523-213328`
- Case: `u7l` — "最近 10 次提交都做了哪些事情？请逐个说明每次提交的作用和影响。"
- Result: PASS, `analyzer_iters=1`, `explorer_iters=5`, `extractor_iters=1`,
  `finalizer_iters=1`, `midloop_inject=0`.
- Root cause of the earlier thin answer: the accepted VCS
  `aggregate_facts.member_set` used decorated commit labels and no current
  source `support_refs`. The old optional-handoff filter only understood
  current-source decorated members, so it dropped the structured recent-10
  commit list and left finalizer with prose context only.
- Fix class: origin-specific principal member sets. A model-authored
  `member_set` with `role=principal_answer` and an explicit non-current-source
  origin is a principal answer payload without current-source citation pressure.
  The same contract now covers VCS/diff, observation-only log/trace runtime
  artifacts, command measurements, cross-repo index observations, external
  documents, web pages, MCP resources, and connector resources.
- Safety boundary: unknown-role external member sets remain soft unless a typed
  pure-history VCS list shape applies; current-source decorated members and
  VCS code identifiers still need support refs, while real commit-hash labels
  can rely on VCS provenance.
- Second replay after short-hash coverage:
  `eval/results/principal-ledger-short-hash-20260523-215117/u7l-20260523-215120`
  PASSed with `finalizer_iters=1`, `midloop_inject=0`, and no visible
  system-generated "符号名称" / verified-member supplement table. This closes
  the immediate UX regression where a rich commit-by-commit answer was followed
  by a dry full-hash table.
- Answer-surface safety replay:
  `eval/results/u7l-20260523-222442` and
  `eval/results/logtri_artifact_line_anchor-20260523-222442` both PASSed with
  one finalizer round, no finalizer rejection, and no first-draft duplication.
  The replay confirms the current system supplement policy no longer replaces a
  rich VCS/history answer with a dry code-symbol table, and runtime artifact
  line anchors stay artifact-scoped. Residual `E20260520-G153` remains open:
  the VCS/history final answer can still miscopy path/date literals inside a
  narrative even when the commit hashes and high-level summaries are preserved.

## Active Finding Ledger

| ID | Case / Source | Severity | Status | Symptom | Systemic Gap | Follow-up Direction |
| --- | --- | --- | --- | --- | --- | --- |
| E20260520-G1 | `logtri_go`, `logtri_degraded`, `logtri_node` | P1 | Partially mitigated | `semantic_quality_reviewer` can log `sufficient=true` while confidence is below the configured floor, e.g. `confidence=0.88 (floor=0.92)` and in `logtri_node` even `confidence=0.50 (floor=0.92)`. | Reviewer result semantics were ambiguous: either the floor is advisory but rendered as a threshold, or low-confidence sufficiency can silently bless questionable answers. | Implemented: low-confidence `sufficient=true` is no longer treated as strong enough to suppress system caveats; only `confidence >= floor` can suppress low-precision soft coverage/prose/facet caveats. Remaining: make telemetry wording clearly distinguish low-confidence sufficiency from strong sufficiency in logs/metrics. |
| E20260520-G2 | `logtri_degraded` | P1 | Open | Placeholder / degraded log input passed, but took 266s with `explorer_iters=27`, `midloop_inject=14`, and multiple irrelevant source routes before answering. | Degraded or non-extractable log triage can over-investigate current code after the artifact itself already proves the boundary. The system analyzes its own degraded StageReport instead of converging on an artifact-quality answer. | Add an artifact-quality fast path or tighter task graph for placeholder / non-extractable logs. The answer should explain the input limitation and any current-code boundary without broad source exploration. |
| E20260520-G3 | `logtri_oversized` | P2 | Open | PASS required 14 explorer iterations, 12 file reads, and 5 mid-loop injections. The model kept repairing non-defining line-local anchors after the tool explicitly said `Do NOT repair this item`. | Explorer repair guidance is visible but not decisive enough for non-actionable ungrounded rows. Non-defining statement anchors can still consume budget after being classified as diagnostic-only. | Convert non-actionable repair rows into a typed “do not retry” convergence signal consumed by explorer scheduling, not just prose in the tool result. |
| E20260520-G4 | `logtri_node`, `logtri_rust` | P2 | Mitigated for observation-only artifacts | Self-consistency reviewer noticed an external path wording tension: summary says `/app/src/*.js` is not in repo, body treats those paths as concrete trace locations. Confidence was below floor, so no rewrite. A follow-up `logtri_rust-20260520-135944` reproduced the same class at high confidence: the reviewer treated artifact frame locations (`src/config.rs:42`) plus `resolved_files=0` as a contradiction, even though that is the intended provenance distinction. | External runtime artifact paths need a clearer rendered vocabulary: they are concrete log frame locations but not current-repo source citations. Generic self-consistency reviewers are prone to misread this typed provenance boundary. | For pure observation-only runtime artifact answers (typed artifact carrier, no current-repo citations, no current-code facets, no requested diagram), skip both self-consistency and semantic-quality reviewers. This preserves the artifact answer, avoids false “正在重写答案” UI events, and keeps current-code / diagram / current-status routes reviewable. Remaining broader work: first-class artifact-frame rendering vocabulary. |
| E20260520-G5 | `logtri_partial` | P1 | Open | Analyzer repeatedly rejected `emit_analysis` because `is_role_locate_lookup=true` lacked `answer_subject.kind`; the first analyzer dispatch consumed multiple failed turns before a second dispatch succeeded. | Simple external-log scalar/role-locate questions still depend on the model remembering a required typed field. The missing field is often safely inferable from typed predicates and question kind. | Add schema-aware analyzer normalization for `is_role_locate_lookup=true` when the located literal kind is structurally inferable, or make the retry hint a typed patch rather than a full reclassification loop. |
| E20260520-G6 | `logtri_python`, `logtri_rust`, `logtri_node`, `logtri_ruby` | P2 | Partially mitigated | External-source logs with `resolved_files=0` still trigger analyzer/explorer thoughts and sometimes actual grep/list_files or `emit_evidence` attempts against non-repo frames before using `evidence_floor_waiver`. | The external-only artifact contract was present but not early / strong enough across all agents. A specific analyzer false-positive path marked plain artifact explanation as `current_risk=true`, which disabled the existing observation-only explorer tool boundary. A second typed mismatch (`is_diagnostic_question=true` with generic `intent=explain` / `scenario=architecture_explain`) also caused avoidable analyzer retries. A third path let accepted external-only answers inherit generic source-code soft caveats such as “coverage may be insufficient.” | The unified runtime-artifact contract now keeps external-only artifacts observation-only unless a separate typed current-checkout verification anchor exists (`exact_targets`, `required_files`, or resolved frames). This absorbs analyzer mirror drift such as spurious `current_risk` / `current_version_check` on plain artifact questions while preserving true current-version questions that name a verifiable current target. `emit_analysis` schema-normalizes diagnostic route drift before self-consistency validation, and explanation-shaped diagnostic routes now clear stale scalar `answer_subject.kind` before the analyzer quality gate instead of relying on late orchestrator auto-correction. Accepted-path soft caveats for observation-only artifacts now drop implicit summary/caveat coverage noise and render only a precise localized artifact-boundary caveat when needed. Covered by `TestBuildAnalysisIR_ExternalOnlyCurrentRiskWithoutCurrentVersionStaysObservationOnly`, `TestBuildAnalysisIR_ExternalOnlyCurrentVersionCheckKeepsCurrentStatus`, `TestEmitAnalysis_NormalizesDiagnosticExplainRoute`, `TestBuildAnalysisIR_DiagnosticExplanationClearsScalarAnswerSubjectBeforeGate`, and `TestAppendSoftContractCaveatsToAnswerForBus_ObservationOnlyUsesBoundaryCaveat`. Reruns: `logtri_rust-20260520-132850` converged in one analyzer / explorer / extractor / finalizer round with no repo reads; `logtri_rust-20260520-134313` has no analyzer hard-reject and no generic supplement, with remaining cost in reviewer dispatch / prompt weight (G10). Remaining work: artifact-scoped `emit_evidence` / aggregate metadata ergonomics (G7), first-class artifact route (G126-G141), and prompt/reviewer cost reduction (G10). |
| E20260520-G7 | `logtri_ruby` | P2 | Open | First `emit_investigation_complete` used `aggregate_facts.kind=negative_search` for external-only metadata and was rejected; the model then retried with `scalar_value`. | The new negative-evidence channel is useful, but the schema boundary is not obvious enough to the model: external-no-intersection metadata is not the same as a verified repo query with `result_count=0`. | Provide a typed aggregate kind for artifact/no-repo-intersection metadata or auto-normalize safe external-only `negative_search` attempts into the correct runtime-disposition fact. |
| E20260520-G8 | `logtri_rust` | P0 | Guarded; avoidance open | Finalizer emitted `blocks` as a JSON-encoded string. The compatibility path accepted it, but only recovered one `summary` block; ordered-list and caveat blocks were dropped. Contract oracles reported missing required blocks as advisory, semantic reviewers did not run, and the case still PASSed with a thin answer plus generic supplement. | Tool-param compatibility and advisory contract softening interacted badly: a malformed recoverable payload silently lost model-authored blocks, then the system accepted an answer below the required surface. | Brace-balance recovery now fails the tool call if a visible block marker was present but not recovered as either a structured block or a preserved display attachment. `logtri_rust-20260520-135200` reproduced a malformed JSON-string `blocks[]`; the guard correctly rejected the partial recovery and the model re-emitted a complete native array. Remaining work: reduce the first malformed emit without publishing lossy recovered payloads, likely via tighter tool-call compatibility hints or a provably lossless unstringify path only when every visible block is preserved. |
| E20260520-G9 | `logtri_rust` | P1 | Mitigated by unit guard | Extractor emitted hypothesis citations to external log paths (`src/config.rs:42`) and got rejected; parse-output then injected auto-verdicts from criteria and proceeded. | External artifact citations were not consistently isolated from repo-grounded citation fields in extractor tools. The system could recover, but the model still spent a failed tool call. | `emit_hypothesis_verdict` now structurally matches citations against external-only log / trace frames and moves exact matches into the verdict rationale as artifact-frame context, leaving the repo `Citation` field empty. Ordinary repo citations are unchanged. Covered by `TestEmitHypothesisVerdict_NormalizesExternalLogFrameCitation` and `TestEmitHypothesisVerdict_DoesNotNormalizeRepoCitationWithoutExternalArtifact`; rerun `logtri_rust` after the next eval tranche to see whether the live model still attempts the now-compatible shape. |
| E20260520-G10 | `logtri_*` tranche | P3 | Partially mitigated | Many artifact/logtri cases show `enumeration_push=1` and large answer-document schema/diagram instructions even when the answer is simple artifact explanation. `logtri_rust-20260520-135944` spent 53s in post-emit LLM reviewers despite zero deterministic contract violations. | Generic finalizer/evaluator prompt scaffolding remains heavy for artifact-only scalar or short diagnostic answers; reviewer dispatch was also over-broad for pure observation-only artifacts. | Observation-only artifact answers now skip both post-emit LLM reviewers when they have no repo citation/current-code surface, removing the 53s reviewer tail for the `logtri_rust` shape. Remaining work: profile prompt sections by answer family and suppress irrelevant diagram/enumeration instructions for artifact-only simple diagnostics. |
| E20260520-G11 | `m1a` | P0 | Open | PASS answer is materially wrong / polluted: it assigns `emit_analysis` to Explorer Turn A, leaves `emit_answer_document` / `emit_answer_document_patch` as orphaned finalizer-tool rows, has blank bullet citation rows, and ends with generic caveats. Self-consistency reported the Turn A count contradiction and orphaned tools; semantic reviewer reported `sufficient=false`, but both were below confidence floor and no rewrite happened. | A combination of deterministic row normalization, optional anchor skeleton rendering, and low-confidence reviewer gating can ship an internally inconsistent answer. This directly risks system-added rows overriding or polluting the model's intended scoped answer. | Treat reviewer `sufficient=false` / `consistent=false` as structured telemetry requiring either a visible caveat tied to the exact concern or a retry when the concern is about principal answer identity. Also audit row normalization so it cannot import out-of-scope finalizer tools into an Explorer/Extractor question. |
| E20260520-G12 | `m1a` | P1 | Open | Explorer consumed 31 iterations, 33 file reads, and 18 mid-loop injections; one `emit_evidence` call failed on `unknown field "line_start_for_summary"`. | Complex two-agent architecture questions still have a heavy explorer tail and schema-compat gaps for near-miss evidence fields. | Separate safe schema repair for evidence near-miss fields from true invalid evidence, and use accepted principal closure boundaries earlier for architecture narratives. |
| E20260520-G13 | `m2a` | P1 | Open | Final answer is rich enough, but system normalization inserted duplicate / questionable surfaces: a second `Key anchors` block with only `AnalysisIR`, generic caveat text, and `HypothesisStatus` enum rows all citing only the enum type declaration line. Pre-emit named the citation mismatch as advisory and semantic reviewer accepted prose equivalence for a promoted diagram facet. | Deterministic normalization and advisory softening can create visible clutter or citation drift even when the model's core prose is good. The row compiler may satisfy shape pressure by adding low-value anchors instead of preserving the clean answer. | Row compiler must prefer the model's already sufficient prose/table unless a typed required block is truly missing. Citation alignment advisories caused by compiler-added rows should either be fixed deterministically with exact lines or omitted, not rendered as low-quality duplicates. |
| E20260520-G14 | `m1b` / `m2a` / `qf_imports` | P2 | Open | Decorated aggregate members without `support_refs` trigger completion rejection; in `qf_imports`, alias-decorated import members like `.../context (别名 promptctx)` caused a rejected `emit_investigation_complete` even though the corresponding import evidence was already accepted. | The member-set contract is structurally sound, but prompt/schema ergonomics make the model discover the `support_refs` rule late. For import/reference literals, alias decorators are especially common and safely groundable from the same import line. | Consider deterministic support-ref filling from already accepted evidence for decorated members, or provide a compact typed template before `emit_investigation_complete` for common `Symbol (qualifier)` / import alias members. |
| E20260520-G15 | `mr_focus_single` / `mr_cross_repo_compare` | P2 | Open | Semantic quality reviewer can first report `sufficient=false` because it sees only a scalar/table body and misses summary/citation context, then after deterministic metadata repair reruns and passes. | Reviewer input surfaces are not always aligned with the renderer's final visible answer. This creates latency and potentially contradictory telemetry for already-correct small answers. | Reviewer prompts should consume the exact final rendered surface plus typed block metadata after deterministic repair, not a partial body view. |
| E20260520-G16 | `mr_cross_repo_compare` | P0 | Open | Final answer says `repo-tools-py` exports 2 functions in prose/table, but a later system/aggregate table titled `repo-tools-py 核心导出标识符（3）` includes `NewGreetServiceImpl` from `repo-greet-go`. A caveat tries to explain the contamination instead of preventing it. | Multi-repo member grouping can cross-contaminate bucket-local principal rows. The compiler/normalizer may treat a cross-repo search hit as a member of the wrong bucket, then render a contradictory count and row set. | Bucket identity must be part of the principal member key. A row whose source repo differs from the bucket may be support/caveat context, but must not be counted/rendered as that bucket's principal member. |
| E20260520-G17 | `mr_cross_repo_compare`, `mr_focus_single` | P1 | Open | Logs still show `repo_map: multi-repo agent requested parent graph; using primary sub-repo compatibility fallback`, and analyzer can hit malformed JSON before succeeding. | Multi-repo graph fallback is safer than the earlier duplicate-parent-graph bug, but it remains a compatibility path that can hide whether an agent asked for the correct sub-repo graph. | Add telemetry and possibly a soft prompt correction for parent-graph requests in multi-repo mode; the correct behavior is explicit per-sub-repo graph access unless a typed aggregate parent view is intentionally requested. |
| E20260520-G18 | `mr_implementers` | P1 | Open | PASS answer names `GreetServiceImpl` correctly but also renders `UserService` as item 2 in the principal list even while saying it is the interface, then adds a system补表 for the single real implementer. | Principal member semantics can mix “requested result members” with explanatory non-members. The reviewer treated the local correction as sufficient, but the visible list still makes the answer look like two implementers. | Add a typed non-member / excluded-candidate lane for interfaces, false positives, or boundary examples. Compiler-generated principal lists must not contain entries whose own text says they are not members of the requested set. |
| E20260520-G19 | `mr_inactive_path` | P1 | Open | PASS answer is honest about inactive `repo-tools-py`, but semantic reviewer still reports one concern because facet coverage marks current-code / resolved-literal expectations in a negative-search answer. It also appends a generic “coverage may be insufficient” supplement. | Negative evidence and inactive-scope answers still inherit positive-answer facet expectations. The system then has to explain a non-gap as a caveat. | Separate positive-result facets from negative-search facets. For a typed zero-result search, required coverage should be searched scope, query/pattern, result_count=0, and inactive-scope disclosure, not live definition file:line. |
| E20260520-G20 | `mr_keyword` | P1 | Open | Correct scalar answer is followed by a system table “包含 process_request 的子仓（1）” whose row cites `repo-tools-py/pyproject.toml:2` instead of the function definition already present at `repo-tools-py/tools/processor.py:4`. | Row compiler can satisfy a repository/member bucket with a package metadata anchor even when a stronger exact symbol anchor exists. This downgrades evidence quality and adds confusing duplicate surface. | Evidence selection must rank exact symbol/function anchors above package metadata for symbol-location questions. Compiler补表 should reuse the strongest existing principal citation instead of introducing weaker adjacent metadata. |
| E20260520-G21 | `mr_pin_isolation` | P2 | Open | PASS answer correctly says the relevant Go repo is inactive, but renders two overlapping scope caveats plus generic supplements; semantic reviewer says `sufficient=true confidence=0.50 (floor=0.92)`. | Inactive-scope disclosure is duplicated between model text, deterministic caveat, and generic supplement. Low-confidence sufficiency remains under-specified in telemetry. | Deduplicate scope disclosures by semantic role and suppress generic supplement when a typed inactive-scope caveat already fully covers the limitation. Tie this to the reviewer-confidence clarification in G1. |
| E20260520-G22 | `principal_span_adjacent_dispatch` | P1 | Open | User asked for a prose source→sink trace and did not request a diagram; analyzer emitted no `diagram_hint`, yet finalizer/reviewer treated `Diagram facet (covered=false)` as a real shortfall and appended generic补充说明. | Call-chain family defaults can promote presentation obligations that the user did not ask for. This violates the user-intent principle by making an optional presentation shape look like a missing answer facet. | Diagram / visual facets should be required only when the current request or typed presentation directive explicitly asks for a diagram, or when a deterministic contract proves prose cannot carry the relationship. Otherwise keep it advisory and do not append generic caveats. |
| E20260520-G23 | `principal_span_inlined_helper` | P1 | Open | Analyzer treated local surfaces `bySource` and `EvidenceItem.Source` as independent focus points / exact targets and suggested `internal/analysis/gate/coherence.go`, although the actual flow is wholly inside `internal/tool/emit_investigation_complete.go`. The run took 19 explorer iterations and 9 mid-loop injections before PASS. | Local variable / field surfaces named in the user's mechanism question can be promoted to multi-topic exploration targets and unrelated required files. This inflates search, creates extra parallel evidence, and risks steering later stages away from the actual enclosing function body. | Exact-target normalization should preserve the enclosing-function context: local variables and field references are subordinate surfaces unless they resolve to a true top-level declaration needed for the answer. RequiredFiles must prefer the file that contains the requested flow over unrelated same-name search hits. |
| E20260520-G24 | `principal_span_inlined_helper` | P1 | Open | Finalizer prompt contained rich evidence for `canonicalCallChainSource` at definition line 5438, but the final citation pool kept only lines 5287/5292/5296; the answer claims the helper is at 5438 while the ordered-list row cites the call site 5292. A deterministic repair also added low-value `Key anchors`. | Evidence selection / citation compaction can drop the strongest definition anchor and leave a weaker call-site citation for the same symbol. System repair then patches shape rather than preserving the best available evidence. | Citation selection should keep at least one definition/mechanism anchor for every principal helper whose implementation behavior is described. Shape repair must not introduce weaker duplicate anchor carriers when the original prose already contains the richer cited fact. |
| E20260520-G25 | `qf_config_precedence` | P1 | Open | PASS body correctly explains `pipeline_max_steps`, but deterministic normalization added `pipeline_max_steps 解析链成员（6）` and `配置优先级链路（1）` tables with blank cells / duplicate prose. Pre-emit also misread scalar default `50` as a visible count mismatch for aggregate member lists. | Enumeration row compiler is still being applied to configuration precedence answers where the model already supplied clear prose and tables. It can create empty rows and count diagnostics from scalar values, then only soft-advises instead of preventing the low-value supplement. | For config-precedence families, aggregate member lists should be support-only unless the user asked for an inventory. Scalar default values must be excluded from member-count validation. Compiler补表 should never add blank rows; if required data is absent, use a separate localized supplement rather than a malformed table. |
| E20260520-G26 | `qf_architecture`; latest rerun `qf_architecture-20260521-184408` | P0 | Partially mitigated; residual scope/citation gap tracked as G160 | Original PASS answer was visibly polluted: after a correct model-authored 4-main-stage + 2-conditional-pre-stage explanation, deterministic normalization added multiple overlapping tables/lists titled `all 6`, `main pipeline (unconditional 4 stages)（6）`, `pipeline stages（12）`, and several “系统按已验证证据补充缺失成员” blocks. Latest rerun no longer renders those system补表 blocks and `enumeration_push=0`, but the extractor/finalizer prompt still receives `Principal Enumeration Rows` that mix 6 `Stage*` rows with 6 `Agent*` rows; visible stage rows can cite `AgentName` lines. | The row compiler can duplicate and cross-merge aggregate member sets with different scopes (main stages vs conditional pre-stages vs agent constants), then publish contradictory counts or weak citations. Some visible pollution is gone, but the underlying principal-member scope key is still too coarse. | Principal member sets need a scope key (`pipeline_stage`, `agent_binding`, `conditional_pre_stage`, etc.) so agent implementation rows cannot satisfy or cite stage rows. Compiler output should remain suppressed when the model's own stage table covers the requested scope, and any system-generated visible count/citation contradiction should be treated as deterministic post-processing failure, not merely reviewer telemetry. |
| E20260520-G27 | `qf_architecture`; latest rerun `qf_architecture-20260521-184408` | P1 | Mitigated for latest replay; scope-key follow-up remains | The original run took 302s with 41 explorer iterations, 30 file reads, 19 mid-loop injections, and `evidence collected=1157 cited=17`. The explorer prompt treated “由哪几个 stage 组成” as a demand to read or explicitly exclude every grep/repo_map/list_files candidate before completion. Latest rerun completed with `explorer_iters=6`, `tool_read_file=5`, `midloop_inject=2`, `finalizer_iters=1`. | Completeness obligations for architecture enumeration were too broad when derived from ordinary “which stages” wording, forcing exhaustive candidate-file draining instead of proving the finite authoritative enum/topology sources. Recent closure/typed-evidence changes reduce this cost, but scope separation is still needed so stage inventory does not import agent rows as principal members. | Keep the finite-inventory strategy: define the authoritative closure source first (enum/topology/registry) and stop once it proves the set; grep candidates outside that closure stay soft context. Remaining work is covered by G160: preserve principal scope identity through aggregate facts, extractor, and finalizer. |
| E20260520-G28 | `qf_diagram_pipeline` | P1 | Open | PASS kept the requested Mermaid flowchart and the 4 stage explanations, but deterministic normalization appended “系统按已验证证据补充缺失成员：codrax read-mode 4 main pipeline stages（4）” whose rows are `AgentAnalyzer` / `AgentExplorer` / `AgentExtractor` / `AgentFinalizer`, not the requested stage members. It also added generic补充说明. | Even when the user explicitly asks for a diagram and the model satisfies it, the compiler can add a mismatched member table by confusing stage members with agent-name support anchors. | For diagram + stage enumeration answers, stage rows and agent rows must be separate typed roles. Support anchors may enrich the stage text but must not be rendered as missing stage members. |
| E20260520-G29 | `qf_imports` | P1 | Open | After reading the single target file and grounding all 14 imports, explorer received `MID-LOOP CHECK: ... read only 1 of 25 discovered files (4%). Read these next...`; the model correctly objected that the question scope is only `internal/agent/explorer.go`. | Exhaustive-enumeration guidance is using discovered candidate files instead of the user's explicit file scope. This can force useless reads and directly contradict the task boundary. | When the current request names an explicit file, enumeration completeness should be measured against that file's parsed/read content, not repository-wide candidate files. Scope-bound closure must override generic “read every candidate” hints. |
| E20260520-G30 | `qf_multi_member_set_count_caveat` | P2 | Open | PASS answer preserved rich Chinese summaries and correct counts, but `answer_contract_check_auto_repair` still spent about 30s after reviewer already reported `sufficient=true`; rerun reviewers then surfaced low-confidence internal metric disagreements rather than user-visible defects. | Low-confidence reviewer / coverage telemetry can trigger expensive deterministic repair even when the visible answer is complete and no precise contradiction exists. This adds latency without improving the answer. | Gate auto-repair on precise post-processing defects or deterministic data loss. Low-confidence reviewer concerns should be recorded as telemetry or localized supplements, not trigger heavyweight repair when the principal answer is sufficient. |
| E20260520-G31 | `qf_multi_member_set_count_caveat` | P1 | Open | Extractor prompt showed misassigned anchor skeleton candidates: the type bucket was paired with `Eval`, the function bucket with `Kind`, and the Kind-constant bucket with `EvalAll`. The final model recovered, but the prompt was misleading. | Sub-topic anchor skeletons can be selected from noisy “best anchor” candidates instead of typed bucket membership. In harder cases this could steer finalizer away from the authoritative member set. | Build anchor skeletons from accepted aggregate members and their support refs first. Ranker/best-anchor candidates may be support context only, never the principal bucket identity. |
| E20260520-G32 | `qf_multi_member_set_count_caveat` | P2 | Open | Explorer first used an invalid field (`context_role_hint=exhaust_listed`) for exhaustive Kind members, then retried with the accepted `salience=exhaust_listed` shape. Final answer recovered, but one failed tool turn was avoidable. | The evidence schema has an ergonomic near-miss for exhaustive listed members. The model can express the correct intent but with the wrong field name. | Add schema-aware normalization for safe `context_role_hint=exhaust_listed` -> `salience=exhaust_listed`, or make the prompt template show the accepted field for exhaustive enumerations. |
| E20260520-G33 | `qf_multi_member_set_count_caveat` | P2 | Open | Focused package inventory collected 2741 evidence records but cited 33; multiple explorer routes disagreed on Kind count before converging to 25. | Even successful focused enumerations can inflate evidence massively when parallel routes duplicate reads / evidence and convergence is delayed. | Deduplicate accepted evidence by `(source,line,anchor,scope,summary)` earlier and stop route expansion once an authoritative closure source proves the finite set. Keep rich summaries, but merge them rather than multiplying lanes. |
| E20260520-G34 | `qf_relation_subagent_registry` | P1 | Mitigated and replay-verified | PASS answer correctly listed the single default subagent `explorer`, but semantic reviewer/facet telemetry originally treated support anchors (`SubAgentRegistry`, `Register`, `Names`, helper methods, concrete-value anchors) as if they were principal enum members. Later replays also showed a worse accepted-path issue: a stale singleton aggregate member `SubExplorer (Name() 返回 "explorer")` could cause a system补表 to override the finalizer's cleaner `explorer` row. | Reviewer/facet telemetry and deterministic补表 needed the same boundary: principal member rows are the accepted request-aware `member_set`/support-ref rows; support/context anchors enrich explanation but must not create extra principal rows or system supplement tables when the finalizer has already rendered a singleton category. | Implemented: aggregate reconciliation no longer expands self-consistent exact member sets with support methods such as `Names`; singleton category blocks authored by the finalizer are treated as authoritative for system补表 suppression; citation-backed principal list blocks can receive missing claim metadata from their own typed citations; uncertainty caveats are materialized even when sibling soft hints exist. Rerun `qf_relation_subagent_registry-20260521-205455` PASSed with `v2_block_oracles=0`, `lane_block_kind=0`, `finalizer_iters=1`, no generic补充说明, and no `系统按已验证证据补充成员` table. Residual low-priority telemetry: one non-user-visible CGEC `CitationReq` remains for follow-up. |
| E20260520-G35 | `qf_relation_subagent_registry` | P1 | Mitigated and replay-verified | The same correct one-member answer failed `citation_count_ge 3 — 1 citations < 3`, then shipped a generic “答案在部分验收检查上未达到预期标准” supplement. | Fixed citation floors are not semantically valid for small exact answers. A one-member enumeration with one exact `Name()` return citation can be complete with one citation; accepted-path post-oracle noise must not become a scary generic user caveat. | Implemented: citation_count_ge now caps the effective floor to the request-aware principal `aggregate_facts.member_set` cardinality when that set is answer-grade and smaller than the scenario template floor. This is typed-only and does not apply to support/narrative member_sets. Wired through exploration pre-complete, criterion evaluation, and env-shape progress tracking. Guarded by `TestEffectiveCitationFloorForPrincipalMemberSets_*`, `TestEval_CitationCountGE_CapsToPrincipalMemberSetCardinality`, `TestEmitInvestigationComplete_PreCompleteCheck_CitationFloorCapsToSingletonMemberSet`, and replay `qf_relation_subagent_registry-20260521-205455` with no accepted-path generic supplement. |
| E20260520-G36 | `qf_relation_subagent_registry` | P2 | Mitigated by schema normalizer | Analyzer first emitted `enumeration_boundary.declared_count=0` to mean “count requested but unknown,” was hard-rejected, then retried without the field. | The analyzer schema uses `declared_count` only for user-declared numbers, but the model naturally uses 0 as unknown/to-discover. This is safely inferable and should not consume a full retry. | Implemented: `declared_count<=0` now soft-strips the optional `enumeration_boundary` with a warning, preserving the rest of the analysis and emitted question kind. True positive boundaries with `declared_count>0` still require a grounded source quote. Guarded by `TestEmitAnalysis_Execute_WarnAndStripsUnknownEnumerationBoundaryWithQuote`. |
| E20260520-G37 | `qf_logic_view_read_pipeline` | P0 | Open | PASS took 836s with 144 explorer iterations, 136 `read_file` calls, and 72 mid-loop injections. Render logs show 8 parallel routes, with one route completing at iter 14 and later routes continuing through iter 23 before global convergence. | Parallel exploration lacks an effective enough global convergence/cancellation boundary. Once a route has produced high-confidence closure for a logic-view architecture question, sibling routes keep satisfying local forced-read / repair obligations rather than being merged or cancelled. | Add cross-route convergence arbitration: authoritative closure from any route should trigger route merge/cancel unless another route owns a distinct unresolved typed objective. Forced-read repairs must be scoped to the route that introduced them and should not keep global exploration alive when principal coverage is already complete. |
| E20260520-G38 | `qf_logic_view_read_pipeline` | P1 | Open | `emit_investigation_complete` was repeatedly downgraded by pending forced reads such as `internal/agent/explorer.go` symbol `mutable` and `internal/agent/extractor.go` grep-anchor ranges, even though the user asked for a logical pipeline view and the answerable architecture was already grounded. | Forced-read completion blockers can be driven by local fields, partial function slices, or grep matches that are support context rather than principal architecture members. This turns precision guards into over-broad exploration gates. | Classify forced reads by principal/support role. For architecture logic-view questions, principal stage/agent/dataflow anchors should close the route; local field/range repairs should be advisory unless the final answer explicitly relies on that line. |
| E20260520-G39 | `qf_logic_view_read_pipeline` | P1 | Open | Finalizer accepted a rich Mermaid + prose answer, but `v2_block_oracles` soft violations triggered `answer_contract_check_auto_repair`; semantic reviewer first returned prose without the required tool call, then a low-confidence sufficient verdict after 43s, and the final answer still rendered a generic “覆盖度可能不充分” supplement. | Reviewer/tool-call failures and soft oracle gaps can add heavy latency without improving or precisely qualifying the answer. The generic supplement remains even when the reviewer says the gap is “thin at best.” | If reviewer tool-call parsing fails, record telemetry and continue only with deterministic checks. Auto-repair should require a precise fixable defect. Generic supplements should be replaced by specific localized notes or suppressed when all principal blocks, diagram, and citations are present. |
| E20260520-G40 | `qf_logic_view_read_pipeline` | P2 | Open | Final answer is broadly useful, but deterministic principal-row tables duplicate the same stage-binding information already explained in prose and diagram. Some rows have very long semicolon-joined descriptions, making the answer heavier than needed. | Row compiler can preserve rich summaries but still over-render them into duplicate tabular surfaces for explanation/diagram questions. | For mechanism + requested-visual answers, prefer model-authored prose/diagram as the primary surface. Use deterministic row tables only for missing exact member carriers, and compress merged summaries into readable narrative cells rather than long concatenations. |
| E20260520-G41 | `qf_type_relation_loop_controller` | P1 | Open | PASS rendered a usable Mermaid flowchart and all 8 implementation types, but semantic reviewer still reported `sufficient=false confidence=0.88` because prose lacked inline code-path spans and the diagram had only interface→implementation tree edges, not “horizontal” implementation edges. The final answer therefore appended generic补充说明. | Reviewer criteria are misaligned with type-relation diagrams: interface-to-implementer edges are the relationship, and citations/items can ground code paths even when prose does not repeat every file path inline. | Diagram/reviewer contracts must understand diagram family semantics. For type/interface relation diagrams, vertical implementation edges are sufficient; current-code path coverage can be satisfied by cited item rows and diagram node labels, not only prose backticks. |
| E20260520-G42 | `qf_type_relation_loop_controller` | P2 | Open | The user explicitly allowed “Mermaid 类图或类型关系图,” but analyzer collapsed the presentation to `diagram_hint.kind=architecture`, and finalizer emitted `flowchart TD` rather than `classDiagram`. The output is arguably acceptable because “类型关系图” was allowed, but the exact requested class-diagram affordance was lost. | Diagram intent has a coarse enum that cannot distinguish class/type relation from generic architecture. This makes finalizer choose a generic flowchart even when Mermaid has a better native representation. | Add a typed diagram subtype / presentation preference for `classDiagram` or `type_relation`, while still allowing flowchart fallback when the renderer subset lacks support. This should remain presentation-only and must not become a hard gate against valid model diagrams. |
| E20260520-G43 | `qf_type_relation_loop_controller` | P1 | Open | Summary says the 8 implementation types are “分布在 5 个不同的 agent 包中,” but the listed files are all under `internal/agent`; the body groups them into components, not packages. Reviewers did not flag the factual wording because it was not contradicted elsewhere. | Semantic review catches summary/body contradictions better than unsupported summary-only quantitative/location claims. A standalone package/file count can be wrong yet pass if the body omits the same claim. | For numeric/location claims in summaries, require either direct evidence support or a deterministic derivation from cited rows. If unsupported, downgrade wording (“多个文件/组件分组”) rather than shipping a precise but unverified count. |
| E20260520-G44 | `qf_sequence_analyzer_gate`; replay `read_combo_analyze_retry_anchor-20260521-153458` | P0 | Partially mitigated | Self-consistency can emit high-confidence contradictions (`confidence >= 0.92`) while the answer still ships with a caveat. Older UI text said “正在重写答案” even when `ViolSelfContradiction` was default-soft and no finalizer re-dispatch occurred. | Two issues were intertwined: (1) status wording was driven by the yaml intent flag instead of the effective soft/strict retry policy; (2) high-confidence contradiction telemetry can still be diagnostic rather than corrective when the default commercial policy chooses caveat-over-retry. | Fixed status truthfulness: default-soft contradictions now show “仅记录，未重写,” and only strict-promoted `self_contradiction` can show “正在重写.” Remaining design work: for strict-promoted contradictions, require one of deterministic local correction, a successful LLM rewrite with changed offending fields, or fail-loud; do not publish the same contradiction with only a supplement when the fix is count/label-local. |
| E20260520-G45 | `qf_sequence_analyzer_gate` | P1 | Open | Deterministic table `buildAnalysisIR 调用链中的关键中间函数（21）` rendered rows such as `analyzerGraphForNormalize:1377` with empty `符号名称 / 定义位置 / 说明` columns, even though the model-authored ordered list above had rich descriptions and citations. | Row compiler is still allowed to add malformed duplicate tables with blank semantic columns. This harms the final answer after the model already produced a good list. | Enforce a compiler invariant: never render a table row with required display columns empty. If row compilation cannot fill the table cleanly, omit the table and keep the model-authored rich carrier. |
| E20260520-G46 | `qf_sequence_analyzer_gate` | P1 | Open | Extra `Key anchors` block introduced `gate.Run` and `analyzer.go` as separate numbered anchors after the main chain, confusing the terminal call (`gate.RunWith`) and contributing to the reviewer contradiction. | System-added anchor skeletons can leak into the user-visible answer as principal-looking content even when they are only navigation/support anchors. | Keep anchor skeletons as citation/support metadata unless the user requested key anchors. They must not be counted as answer members or placed next to principal lists in a way that changes the chain semantics. |
| E20260520-G47 | `read_combo_analyze_retry_anchor` | P1 | Open | PASS answer correctly says read-mode analyze is fail-loud and does not silently use zero-value `AnalysisIR`, but several item-level citations drift: `planPath != ""` is shown against `analyzer.go:46`, `analyze stage exhausted` against `agent.go:22`, and `storm.exhausted()` against the success guard line instead of the storm-check line. | Citation compaction / item citation assignment can attach a correct claim to a nearby or generic anchor while the citation pool contains stronger exact lines elsewhere. Reviewers focus on prose sufficiency and miss item-level file:line drift. | For mechanism answers, validate that each item citation's quoted line/source semantically contains the item label or predicate. Prefer exact support refs already in the citation pool; do not reuse generic carrier lines for distinct mechanisms. |
| E20260520-G48 | `read_combo_answer_document_tools` | P0 | Open | PASS answer's `Key anchors` block lists `emit_answer_document_patch` but cites `internal/tool/emit_answer_document.go:35` and the `"emit_answer_document"` literal. The main body also discusses `FilterToolSchemas` and dispatch flags without a final citation to `answer_document_evaluator.go:6020`. Semantic reviewer still returned sufficient=true. | System-added anchor/skeleton rows and citation pool pruning can silently corrupt exact literal comparisons. This is dangerous because the user explicitly asked for `Name()` source-string literals. | Exact-literal comparison answers must preserve one citation per literal and per dispatch mechanism. Reviewer/pre-emit should inspect all visible blocks, including system-added `Key anchors`, and fail or deterministically remove wrong anchors when a literal/citation mismatch is precise. |
| E20260520-G49 | `read_combo_answer_document_tools` | P1 | Open | Finalizer emitted `blocks[]` as a JSON-encoded string; tolerance recovered the payload, then repaired mechanism anchor carriers, normalized 6 principal enumeration blocks, accepted a pre-emit advisory, and still appended a generic coverage supplement. | Compatibility recovery can be lossless, but the subsequent deterministic normalization/repaired carriers still add visible noise and may introduce wrong anchors. | Keep JSON-encoded block recovery as a compatibility path only if recovered blocks are byte/field complete. After recovery, system-added carriers must pass the same citation/literal checks as model blocks, and generic supplements should be suppressed when reviewers find no concrete shortfall. |
| E20260520-G50 | `read_combo_criterion_rich_functions` | P2 | Open | PASS preserved rich Chinese function descriptions and exact file:line citations, but semantic reviewer still claimed the BODY “omits” file:line citations entirely, with `sufficient=true confidence=0.78`. | Reviewer input still differs from the final rendered surface: item citations rendered by the answer panel can be invisible or underrepresented to reviewer checks. | Feed reviewers the exact post-render principal surface, including inline item citations, before claiming missing file:line coverage. Low-confidence false positives should remain telemetry only and must not add supplements or trigger repair. |
| E20260520-G51 | `read_combo_config_two_knobs_precedence` | P0 | Open | PASS main comparison table is correct, but deterministic normalization appended multiple duplicate config tables (`三层配置键`, `配置链路`) with empty `定义位置/说明` cells and even a visible `[illustrative]` marker. Semantic reviewer still passed and a generic supplement was appended. | Config precedence answers are still over-compiled as enumeration member sets. System-added tables can be visibly malformed even when the model-authored summary/table already fully answers the request. | For `config_trace/config_precedence`, suppress principal enumeration compiler output when a complete comparison table exists. Enforce no-empty-display-cell and no-internal-marker invariants for any system-added table. Generic uncertainty supplements should be suppressed when reviewer explicitly says the uncertainty gap is unwarranted. |
| E20260520-G52 | `read_combo_multirepo_negative_interaction` | P1 | Open | PASS no longer used fake `repo:0` anchors and gave a clear search-scope caveat, but deterministic normalization rendered `(无任何直接依赖)` as a principal table row with empty cells, then added two overlapping “跨仓依赖关系” tables. | Negative-search / no-interaction conclusions are being coerced into positive member-set table shapes. The structured negative evidence channel works, but the renderer still treats “nothing found” like an enumerable member. | Negative-search facts should render as a localized conclusion + search-scope ledger, not as member rows. Suppress duplicate dependency tables when the model's caveat/table already covers repo/query/scope/result_count. |
| E20260520-G53 | `read_combo_multirepo_negative_interaction` | P2 | Open | Logs still show `repo_map: multi-repo agent requested parent graph; using primary sub-repo compatibility fallback`. The run recovered, but this remains a hidden compatibility path in a multi-repo question. | Multi-repo graph access still allows parent-graph requests to fall back to a primary sub-repo, which can bias later search or explanations if the fallback result is treated as complete. | Keep fallback telemetry visible and add a soft correction to request explicit per-sub-repo graph views. Parent graph fallback should never become principal evidence for cross-repo absence or comparison claims. |
| E20260520-G54 | `read_combo_source_locations_required_false` | P1 | Open | PASS answer correctly found all 4 production `CitationReq.Required=false` locations, but `blocks[]` arrived as a JSON-encoded string, was recovered, and deterministic normalization appended duplicate category tables (`直接赋值位点`, `结构体初始化位点`) after the already-good model-authored sections/table. | Compatibility recovery plus enum-row normalization can still add redundant system surfaces to an otherwise complete exact-location answer. This is not data loss in this case, but it increases answer noise and risks later drift. | Exact field-value/location enumerations should prefer the model's existing complete carrier. Only add deterministic tables when a typed required surface is absent, and dedupe category tables against already-visible section/table coverage. |
| E20260520-G55 | `read_combo_source_locations_required_false` | P1 | Open | Semantic reviewer first reported `sufficient=true confidence=0.92`, explicitly saying the `Current code path` gap was false because file:line citations were present, yet `answer_contract_check_auto_repair` still ran for 19.7s before returning another sufficient verdict. | Auto-repair can still launch after reviewers have already identified a typed coverage gap as a false positive. This wastes finalizer time and can mutate good answers. | Gate auto-repair on precise, fixable structural defects. A reviewer-sufficient verdict that names the oracle gap as false-positive should suppress repair and record telemetry instead. |
| E20260520-G56 | `read_combo_source_locations_required_false` | P2 | Open | The run collected 1022 evidence items but cited only 4 for a bounded exact-location question. Explorer cost was moderate here, but the collection/citation ratio shows severe evidence inflation for a four-row enumeration. | Search and evidence lanes are still too broad for exact field/value inventory tasks. Large support pools increase prompt pressure and make finalizer/reviewer drift more likely. | Add intent-aware evidence budgeting for exact-location enumerations: once all exact assignments/initializers are grounded and bucketed, demote broad support evidence and keep only high-value summaries. |
| E20260520-G57 | `s11a` | P0 | Open | PASS answer correctly says analyzer cannot call `read_file`, but also falsely says analyzer “仅允许三种工具” (`emit_analysis`, `repo_map`, `grep`). Current source includes a fourth analyzer tool, `list_files`, in `AnalysisToolSuggestions`. Reviewers missed the false allowed-tool count. | Evidence summaries and citation quotes can paraphrase/compress a source line and drop a real member (`list_files`), then finalizer treats the compressed summary as authoritative. Exact tool allowlist answers need literal source-list preservation, not paraphrased summaries. | For allowlist/config/list answers, preserve exact source members from the cited declaration and compare visible list count against the declaration. Reviewer/pre-emit should flag “only N” claims when cited source contains additional members. |
| E20260520-G58 | `s11a` | P1 | Open | Explorer first found the key `analysis_contract.go` lines via `grep` with context, but `emit_evidence` rejected two rows as “not present in read_file history,” causing read-after-grep repair loops and 10 mid-loop injections for a simple yes/no policy question. | Prompt says line-level grep belongs to exploration evidence, but the evidence validator still requires `read_file` history for grounding. This mismatch turns valid grep line evidence into avoidable repair work. | Either allow grep-with-line-context to ground exact line evidence, or make the prompt consistently say grep is only a locator and cannot ground `emit_evidence`. The chosen contract should be typed so models do not discover it through retries. |
| E20260520-G59 | `read_combo_pipeline_sequence_table` | P0 | Open | PASS took 708s, 132 explorer iterations, 112 `read_file` calls, and 63 mid-loop injections for one requested sequence diagram plus one stage I/O table. | The parallel exploration convergence problem is still present for explicit diagram+table architecture questions. The system can deliver the answer, but the route convergence / forced-read budget is commercially too expensive. | Extend cross-route convergence arbitration: once the requested diagram spine and stage I/O table anchors are grounded, sibling/support routes should merge or cancel instead of continuing local repair tails. |
| E20260520-G60 | `read_combo_pipeline_sequence_table` | P1 | Open | Main answer includes the requested Mermaid `sequenceDiagram` and a useful stage table, but deterministic normalization appended malformed/low-value tables: `Stage→Agent mapping（4）` has blank `符号名称/定义位置/说明` cells, and conditional pre-stage tables are extra noise. | Row compiler still adds principal-looking supplemental tables even when the user-requested presentation is already satisfied. The no-empty-display-cell invariant is not enforced across all generated table paths. | For explicit diagram+table answers, treat model-authored requested surfaces as primary. Only add deterministic tables for missing required carriers, and never render generated rows with empty semantic columns. |
| E20260520-G61 | `read_combo_pipeline_sequence_table` | P1 | Mitigated for telemetry-only richness | Semantic reviewer said system-detected gaps were false positives, but returned `confidence=0.80 (floor=0.92)` because of a low-confidence “uncertainty boundary” richness candidate, and the final answer appended a generic “覆盖度可能不充分” supplement. | Richness candidates could become user-visible generic caveats after all explicit user obligations were satisfied. This made a successful answer look suspect without naming a concrete defect. | `ViolRichnessRegression` is now registry-typed as operator telemetry with no caveat family, so optional-richness omissions no longer append the generic coverage supplement. Concrete limitations still need typed localized caveats through stricter/promotable signals. |
| E20260520-G62 | `s11b` | P1 | Open | Analyzer spent all 4 pre-scan rounds on broad `grep(files_only=true)` searches for retry-budget terms before emitting analysis; the must-emit hint had to force convergence. | Specific scalar/parameter lookup questions can overuse analyzer pre-scan instead of quickly classifying and leaving line-level evidence to explorer. This increases latency and risks analyzer failure before the real search stage begins. | Add analyzer-side pre-scan stopping rules for exact parameter/name questions: after candidate config/retry files are found, emit analysis with role profile instead of continuing broad grep variants. |
| E20260520-G63 | `s11b` | P1 | Open | Final answer correctly names `MaxRetriesPerStage`, but the rendered references include `internal/types/context.go:14` before the actual `internal/types/config.go:14` definition. | Citation pool rendering can still include stale or adjacent context citations for scalar identifier answers, making the evidence trail look less precise than the answer. | For scalar identifier answers, citation pruning should keep the exact definition/config line first and drop unrelated context citations unless they support a separate visible claim. |
| E20260520-G64 | `s1a` | P1 | Open | User asked for gate check order and retryable behavior, not a diagram. Semantic reviewer nevertheless treated `Diagram facet promoted=true covered=false` as a system gap, then said prose was equivalent; the final answer still appended a generic “覆盖度可能不充分” supplement. | Optional diagram pressure remains in non-visual mechanism questions. Even when reviewers classify the gap as prose-covered, the generic supplement path can still leak a false uncertainty signal. | Diagram facets must only be required when explicitly requested or when a typed visual contract is generated. If a reviewer says a diagram gap is prose-covered, no generic supplement should be emitted. |
| E20260520-G65 | `s1a` | P2 | Open | Eval metrics report `explorer_dispatches=0` while the same run has `explorer_iters=13`, `tool_read_file=5`, and many explorer mid-loop checks. | Telemetry counters are internally inconsistent for at least one successful run, which makes later eval trend analysis less reliable. | Audit metrics aggregation for parallel/scheduler explorer dispatches so dispatch count, iterations, and tool usage share the same source of truth. |
| E20260520-G66 | `s1b` | P1 | Open | PASS answer correctly says validate failure only requeues `EdgeValidationFeedback` upstream nodes plus the validate node itself, but the item “validate 节点自身” cites `scheduler.go:518` (`forceCloseExploreWindow`) instead of the actual `requeueValidationTargets` assignment around `scheduler.go:513`. | Item-level citation drift persists for mechanism answers: a nearby support function can be attached to a principal claim even when the correct citation exists in the same file. | Principal item citations should be validated against the item predicate/label, not merely file proximity. If the claim is about requeueing the validate node, cite the exact `s.status[validateID] = nodeRequeued` line. |
| E20260520-G67 | `s1b` | P1 | Open | `blocks[]` again arrived as a JSON-encoded string, was recovered, and 4 principal enumeration blocks were normalized. The answer passed, but the same run appended a generic low-confidence uncertainty supplement (`semantic_quality confidence=0.65`). | JSON-string recovery, row normalization, and low-confidence richness supplements remain coupled in normal successful answers. This increases surface churn and makes it harder to tell whether the LLM or system authored a visible section. | Track recovered JSON payloads separately from answer-quality concerns. Recovery should be lossless and invisible; supplements should require precise localized defects, not generic richness candidates. |
| E20260520-G68 | `s3a` | P1 | Open | First finalizer emit was rejected because `candidate_role="default"` is not in the code-candidate enum. The model was trying to express config precedence layers (`default/config/runtime`), not code symbol roles. | Config-precedence layer roles are being squeezed through `candidate_role`, whose enum is for code candidate classes. This causes avoidable finalizer retries and teaches the model the wrong field semantics. | Add a separate typed field for config/precedence layer role, or normalize invalid config-layer `candidate_role` values out of table rows before hard rejection when the row is otherwise well formed. |
| E20260520-G69 | `s3a` | P1 | Open | Final table rendered headers as `项目 / 列 2 / 列 3 / 列 4 / 列 5` because the model supplied item `cells` without explicit `columns`. The row content is useful but the user-facing table is unprofessional. | Table rendering still lacks safe header inference for structured cells when columns are absent. The system accepts the table but exposes generic column labels. | Infer stable localized headers from row labels/cell roles for config-precedence rows, or render as an independent supplement with explicit “层 / 依据 / 结论 / 生效值” headers. Never ship `列 N` in final answers. |
| E20260520-G70 | `s3a` | P1 | Open | Final answer mentions `internal/skill/glossary.go:35` as proof the key is an eval-only blocklist identifier, but the final citation pool has only `config.go`, `codrax.yaml.example`, and negative `cmd/root.go`; the glossary citation was dropped. | Summary prose can still keep a source path that is not represented in `citations[]`, while reviewers treat it as anchored. | Any visible file:line in principal summary/caveat text must resolve to a citation entry or be removed. Reviewer input should use the final citation pool, not upstream evidence availability alone. |
| E20260520-G71 | `s3a` / `s3d` | P1 | Mitigated | Semantic reviewer returned `sufficient=true` with no actionable gaps, yet the final answer still appended the generic “覆盖度可能不充分” supplement. `s3d-20260520-143037` showed the residual violation was `answer_richness_facet_coverage` / `ViolRichnessRegression`, even though the reviewer explicitly said the BODY was substantively complete. | The optional-richness telemetry kind still had `CaveatFamilyAnswerCoverage`, so accepted-path caveat materialization treated a trend signal as a user-facing defect. | `ViolRichnessRegression` now carries no caveat family and is pinned as operator-only telemetry. Generic coverage supplements require a concrete non-telemetry violation such as `ViolFacetUncovered`, `ViolRichnessGlaringGap`, or a semantic reviewer concern with localized observation. |
| E20260520-G72 | `s3d` | P1 | Mitigated | Analyzer emitted `answer_role_profile.is_role_binding_requested=true` without `source_quotes[]` and was rejected, consuming a retry turn. | Analyzer role-binding schema remains fragile: source quotes are required when the model infers a positive answer role, but they are often mechanically recoverable from the current request. | Optional positive `answer_role_profile` now softens instead of hard-rejecting when `source_quotes[]` is missing or not copied verbatim from the current request. Anchored role profiles still persist and invalid anchored roles still fail. Re-run `s3d` or equivalent config-absence eval to verify the analyzer retry tail is closed. |
| E20260520-G73 | `s3d` | P1 | Open | Final answer correctly treats `explore_xyz_phantom_unique_budget` as absent across config/default/CLI layers, but reviewer demanded positive line anchors like `codrax.yaml.example:543`, `cmd/root.go:2396`, and `config.go:160`; auto-repair then spent 27.6s and still ended with a generic supplement. | Absence/config-precedence review is still biased toward positive layer anchors even when the exact target is absent and negative citations are the right evidence shape. | Reviewer criteria for absent config keys should accept negative-scope citations for the missing target plus optional nearby-layer context. Do not require unrelated positive config-key lines as principal anchors for an honest-zero answer. |
| E20260520-G74 | `s3d` | P2 | Open | `emit_investigation_complete` params were normalized via `$.aggregate_facts string->array`, and several parallel completion reasons described slightly different absence boundaries before finalizer. | Aggregate-fact compatibility recovery works, but parallel absence conclusions can still merge noisy historical/design-doc details into the final context. | Keep recovered aggregate facts only when they pass the stable evidence contract, and dedupe parallel absence summaries by target/scope/query before finalizer. |
| E20260520-G75 | `s5a` | P0 | Open | Final answer visibly includes a table with all 8 `LoopController` implementation types and file locations, but semantic reviewer said the requested “所在文件” part was “完全缺失” and reported `sufficient=false confidence=0.88`. | Reviewer surface still does not reliably parse final table text/cells after renderer/normalizer repair. It can claim missing file locations that are visibly present to the user. | Reviewer input must be the exact post-normalization rendered answer surface, including Markdown table cells and repaired citation carriers. Table text should be converted to structured rows before reviewer checks. |
| E20260520-G76 | `s5a` | P1 | Open | The summary says `LoopController` is defined at `internal/agent/agent.go:429`, but the final citations include only the 8 implementation files. | Interface/location context can remain visible without citation when the answer's principal citation pool is focused on implementer rows. | If a summary names an interface definition file:line, include that citation or omit the file:line. Principal member citations can stay separate, but visible context anchors must still resolve. |
| E20260520-G77 | `s7a` | P0 | Open | PASS answer says `internal/tool` non-test Go LOC is `68892`, but an external deterministic check (`find internal/tool -type f -name '*.go' ! -name '*_test.go' -print0 | xargs -0 wc -l`) returns `70421`. | Measurement-scalar questions can still ship wrong values when the safe shell policy rejects common exact-count commands (`xargs`, `find -exec`, redirection), forcing the model into manual directory splitting and arithmetic. Reviewers were skipped (`semantic_quality_dispatches=0`), so no one caught the wrong scalar. | Add a dedicated safe deterministic counting path for recursive file/LOC queries, or allow a constrained read-only `find -print0 | xargs -0 wc -l` pattern. Scalar answers must preserve the exact command output and should not be derived from model arithmetic. |
| E20260520-G78 | `s7a` | P1 | Open | Explorer emitted the computed total as `emit_evidence scope=file source=internal/tool`, but validation rejected it because `scope=file` requires a file-role label. The model then completed without evidence/citations (`cited=0`). | Deterministic command outputs do not have a first-class evidence channel. The model tries to force them into file evidence, which fails, and the finalizer receives a citation-free scalar. | Add a typed `tool_result` / `measurement` evidence shape carrying command, scope, value, and timestamp. It should be citation-grade for scalar/count answers without pretending to be a source file. |
| E20260520-G79 | `s5b` | P0 | Open | PASS answer contains contradictory member sets for `internal/analysis`: opening table says 25 subpackages, system-added tables render 25 and 26 entries, and self-consistency reviewer emitted 4 high-confidence contradictions (`confidence=0.95`) covering `contract`, `dataflow`, `logtriage`, and `normalizer`. UI logged “检测到 4 处前后不一致，正在重写答案,” but the final answer still ships the contradictions plus a generic inconsistency supplement. | High-confidence contradiction handling is still diagnostic rather than corrective in some paths. The system can detect the exact conflicting rows but fail to mutate or suppress the offending system-added sections. | For confidence >= floor contradictions, require deterministic removal/repair of conflicting generated tables or a real rewrite that changes the offending rows. If no repair is applied, fail the case instead of publishing with only a supplement. |
| E20260520-G80 | `s5b` | P0 | Open | Deterministic normalization added two malformed duplicate tables (`internal/analysis/ 子包及其入口函数（25）` and `各子包单一入口函数（26）`) with blank columns and inconsistent members such as `perftriage/merge`, `normalizer/canonicalize`, and `logtriage/ResolveJavaFile`. | Row compiler can still override a good primary table with conflicting aggregate-derived surfaces. This is the same systemic risk as earlier count/list collapses, now on package-entry enumeration. | Treat compiler tables as subordinate to a complete model-authored table unless a required member is provably absent. Compiler output must pass no-empty-cell, count consistency, and same-member-same-citation invariants before rendering. |
| E20260520-G81 | `s5b` | P1 | Open | Eval metrics again show `explorer_dispatches=0` despite `explorer_iters=26`, `tool_read_file=74`, and 15 mid-loop injections. | Explorer telemetry inconsistency recurs under heavy enumeration, not just `s1a`. | Same as G65: unify metrics source of truth for dispatch/iteration/tool counters, especially under parallel or scheduler-managed exploration. |
| E20260520-G82 | `s7b` | P1 | Mitigated | The correct count `25` was available from several deterministic commands, but `emit_investigation_complete` was downgraded five times with `deterministic count proof is missing`; the model explicitly complained that `scalar_value` / provenance were not being accepted. Initial run needed 18 explorer iterations for a single-file scalar question. After the scalar aggregate fix, `s7b-20260520-122126` PASSed with no deterministic-count downgrade, 8 explorer iterations, and one mid-loop hint. | Scalar/count completion accepted `total_count` / `unique_count` / `member_set` and system-enriched `scalar_value(answer_axis=count)`, but not the natural model-authored `scalar_value: value=<integer>` form advertised by the prompt. This leaked schema internals to the model and made broad exploratory counts poison later scoped proof recognition. | Count-question closure now accepts an unambiguous model-authored integer `scalar_value` as the exact count handoff when it is not explicitly support/audit and no sibling principal scalar conflicts. Conflicting scalar candidates still require a clearer aggregate or deterministic proof. |
| E20260520-G83 | `s7b` | P1 | Open | Final answer correctly says `25`, but deterministic normalization appended a `distinct Kind constants（25）` table where every row has blank `符号名称 / 定义位置 / 说明` cells except the first column. The user asked for the exact count, not a list. | Row compiler still renders member-set surfaces for scalar answers even when no row details are available. This creates a low-quality, mostly empty table after a good scalar answer. | For scalar/count-only requests, suppress member-list tables unless the user asked for the list or the compiler has complete non-empty row cells. If a proof set is useful, keep it hidden/support-only or summarize it in one localized sentence. |
| E20260520-G84 | `u10a` | P0 | Open | Exploration established that `ShapeValue` is not an active Go constant and appears only in comments/docs/tests, but final answer says `internal/types/analysis_ir.go` is the “常量定义处 / 唯一根源” and should rename the constant there. | Upstream negative evidence and premise-invalid findings are not binding enough downstream. Analyzer’s early “definition” assumption and aggregate member-set labels can override later, stronger absence evidence. | Negative/absence facts for the exact target must dominate stale analyzer assumptions and positive member labels. If the premise is invalid, finalizer must lead with that correction, then list optional comment/doc/test text updates separately. |
| E20260520-G85 | `u10a` | P0 | Open | System-generated supplement adds `internal/types/answer_surface_plan.go:295/312` as missing affected files, but those lines mention `ShapeStepList`, not `ShapeValue`. The table title claims “生产代码受影响文件（含注释引用）,” making unrelated same-family legacy-shape comments look required. | Same-family / regex context evidence (`ShapeStepList`, `ShapeNone`, etc.) can be promoted into the principal affected-file member set for a specific rename target. This is a system-authored false positive. | Principal affected members for rename/change-impact must match the exact target surface or a proven alias. Same-family context may remain background only and must never be rendered as “missing members” for the requested target. |
| E20260520-G86 | `u10a` | P1 | Open | Test and documentation rows cite `internal/types/analysis_ir.go:1433` for every file instead of citing each file’s own grep/read location; the caveat admits those files were not individually read. | When members come from files-only grep or partially verified search results, the renderer borrows a nearby target citation rather than representing the weaker evidence shape honestly. | Add citation semantics for grep/list-derived file members: either cite the actual grep hit when available, or mark rows uncited with a localized caveat. Do not attach an unrelated source line to unrelated file rows. |
| E20260520-G87 | `u10a` | P1 | Open | The run took 50 explorer iterations and 17 mid-loop injections, including repeated `change-impact principal member_set handoff is missing` / `support_refs` repairs, then still shipped the wrong principal framing. Metrics again say `explorer_dispatches=0`. | Change-impact completion is too schema-fragile around file-vs-site member sets and support refs. The repair loop consumes time but does not guarantee that final principal rows preserve exact-target semantics. | Provide a first-class change-impact aggregate shape (`target`, `site`, `file`, `change_type`, `confidence`, `verified_by`) so exploration closure does not depend on ad hoc member strings. Metrics fix follows G65/G81. |
| E20260520-G88 | `u11a` | P1 | Open | User asked only “入口函数叫什么、在哪个文件,” but analyzer invented a second focus “emit_analysis 与 Explorer 的调用关系,” and final answer rendered an extra section for that relationship. | Role-locate / scalar symbol questions can be widened by related architecture context discovered during pre-scan. This violates the direct-answer shape and adds user-unrequested explanation. | For exact role-locate lookups, related call-chain context should be optional support after the scalar answer, not a promoted sub-topic. Analyzer subtopics must be derived from explicit user asks, not nearby pipeline terms. |
| E20260520-G89 | `u11a` | P1 | Mitigated by renderer guard | The extra section contains three empty bullet rows that render only citations: `-  (internal/tool/emit_analysis.go:20)`, etc. | Renderer/finalizer accepted item entries with citation refs but no visible label/text, producing blank list items. | Renderer now skips citation-only list/section items instead of rendering blank bullets; the citation pool remains available and visible items still render their citations. Guard: `TestRenderV2_SkipsCitationOnlyListItems`. This is display-only and does not mutate model-authored prose or citation data. |
| E20260520-G90 | `u11a` | P2 | Open | Direct function lookup needed 17 explorer iterations and received repeated partial-read pressure to continue reading `explorerEvaluator.BuildInitialInstruction` (955 remaining lines) and `buildAnalysisIR` (634 remaining lines), even after the exact function/file answer was grounded. | Function-span coverage hints are over-applied to direct role-locate questions. The task needs the definition and maybe one call-site, not full function-body coverage. | For role-locate/scalar symbol answers, closure should require the exact definition/call-site anchor only. Function-body partial-read hints should be advisory unless the answer describes internals of that function body. |
| E20260520-G91 | `u10b` | P1 | Open | User asked for production code sites; evidence found `internal/orchestrator/orchestrator.go` has two constructor sites (`7558` and `7690`), but the final table collapses that file to only line `7558`. `templates.go` correctly keeps six lines in one row, so site aggregation is inconsistent. | Change-impact rendering mixes “files” and “sites”: the principal member set is file-level, while the user wording and evidence are site-level. Multiple sites in one file can be silently dropped. | Preserve both dimensions for change-impact answers: file rows may group sites, but every verified site line for that file must remain visible in the row or in a nested localized detail. Do not collapse site-level evidence to a single line. |
| E20260520-G92 | `u10b` | P1 | Open | Semantic reviewer spent 26s and emitted a confidence-0.92 concern claiming the BODY “only explicitly names one file,” even though the final Markdown table visibly lists 10 files. The final answer then still includes a generic coverage supplement. Metrics also report `semantic_quality_dispatches=0` while `semantic_quality_concerns=1`. | Reviewer/post-render surface extraction is still not reliably table-aware, and metrics disagree about reviewer dispatch. This repeats G75 in a change-impact table setting. | Feed semantic review the exact post-normalized rendered table rows as structured rows, and suppress generic supplements when the reviewer concern contradicts the visible final surface. Fix dispatch/concern counter source-of-truth as in G65/G81. |
| E20260520-G93 | `u11b` | P1 | Open | Final answer correctly lists the 4 `CitationReq.Required = false` production sites, but begins with “未找到完全一致的精确目标，但已验证等价/别名锚点：CitationReq.Required.” Logs also show a mid-loop hint saying the requested exact symbol “already looks absent,” despite exact positive assignment hits. | Exact-resolution / alias messaging is wrong for field-value lookups. The system can classify an exact owner-qualified field target as absent/alias even when concrete positive hits are present. | Field-value profiles should treat `Owner.Field = literal` grep/read hits as exact target satisfaction. Do not render absence/alias banners or absence repair hints after exact positive assignments/initializers have been grounded. |
| E20260520-G94 | `u1a` | P0 | Open | Security answer identifies the `exec_command` taint source/sink in prose, but the final citation list omits the central source/sink lines (`builtin.go` parameter/execute/wrap and `command_detect.go:28`) and keeps only read-only guard and Windows supervisor citations. It also states allowlist counts as `29` commands + `13` git subcommands, while current source contains `30` commands + `12` git subcommands. | Security / taint-flow questions need exact source-to-sink citation binding and deterministic counts. Current finalizer can cite only mitigation anchors and let unsupported numeric allowlist claims ship. | Add a security-flow contract: every visible source/sink claim must cite the concrete flow line, and exact allowlist counts must be derived from source or deterministic tooling. Reviewer/pre-emit should not bless taint answers whose citation pool lacks the sink/source lines named in prose. |
| E20260520-G95 | `u1a` | P1 | Open | The answer renders a useless `Key anchors` block containing only `exec_command`, plus a generic coverage supplement, after a successful first-turn finalizer. | System-added anchor skeletons and generic supplements still leak into successful security answers and add no user value. | Same as G46/G61: keep navigation anchors support-only unless requested, and suppress generic coverage notes unless a precise localized defect is known. |
| E20260520-G96 | `u1b` | P0 | Open | The final answer says `baselineShow` passes raw user input directly into `os.ReadFile`, but source shows `match` comes from `os.ReadDir(dir)` entries and user `sha` only participates in prefix selection. The risk may still deserve discussion, but the source-to-sink semantics are overstated. | Taint reasoning collapses “user controls path bytes” and “user selects from existing filenames” into the same unsanitized filesystem-flow category. This can create a false-positive security conclusion even when the code has a different risk shape. | Represent filesystem taint flows with typed roles: `path_payload`, `selector`, `directory_scope`, and `enumerated_filename`. Only `path_payload` should satisfy “raw user input passed into filesystem operation”; selector flows should render as a nuanced caveat or separate risk class. |
| E20260520-G97 | `u1b` | P1 | Open | Deterministic supplement table for protected fs-ops includes a blank `handlePlanArg` row, and self-consistency detected a low-confidence “two filepath.Join” vs one described mismatch. The final answer still appends the generic supplement and repeats a duplicate final surface in the log. | Row compiler accepts empty protected-member rows and the contradiction path does not make a localized correction when confidence is below the rewrite floor. Security inventory answers should never show blank rows. | Enforce no-empty-row invariant for protected/control rows. For low-confidence count contradictions, prefer deterministic local wording downgrade (“confirmed sites listed below”) over generic supplements. |
| E20260520-G98 | `u3a` | P0 | Open | Final answer claims extractor `ShouldStop` is effectively “one-shot” and “iteration >= 1 即可能终止,” but the source path is `iterationCapShouldStop(resp, iteration, 3, 5, ...)`, where `iteration < softCap` returns false and hard stop is `iteration >= 5`. The self-consistency reviewer accepted this as a valid inference. | Reviewers validate internal consistency but can miss direct code-semantics falsity when the model infers behavior from counters. Exact control-flow questions need source-condition evaluation, not only prose consistency. | For control-flow/comparison answers, derive stop conditions from the cited boolean predicates and constants. If a visible statement simplifies a predicate (`>=1`, `one-shot`, “only”), verify it against the exact cited condition before publishing. |
| E20260520-G99 | `u3b` | P0 | Open | Summary and early tables say both templates have 7 TaskGraph nodes, while later system-normalized tables say `TaskGraph 节点数: 5`. Self-consistency raised a confidence-0.95 contradiction twice, auto-repair ran for ~30s, yet the contradictory 5-row tables remained in the final answer. | High-confidence contradiction detection still does not force an actual mutation, and deterministic row normalization can introduce the contradiction after the model-authored comparison is correct. | A high-confidence numeric contradiction must either be deterministically repaired by removing/updating the system-added table or fail-loud. System-normalized tables need provenance priority lower than model-authored grounded tables and must not override them. |
| E20260520-G100 | `u3a`, `u3b` | P1 | Open | Both comparison answers append deterministic tables whose semantic columns are empty (`符号名称 / 定义位置 / 说明` blank), even when the prose already provides a rich comparison. | The row compiler still treats comparison dimensions as enumerable members without requiring source locations or explanations, producing blank user-visible tables. | Apply the no-empty-display-cell invariant across comparison/template/control-flow compiler paths. If a dimension row has no location or rationale, keep it in prose/support metadata rather than rendering it as a table row. |
| E20260520-G101 | `u4a`, `u4b` | P1 | Open | Simple import/dependency enumerations produced correct short answers, but collected very large evidence pools (`u4a`: 2809 evidence for 5 imports; `u4b`: 1263 evidence for 9 imports) and cited only the final 5/9 rows. | Exact import/dependency inventory still lacks an early convergence budget. Broad evidence harvesting increases prompt pressure and downstream reviewer noise without changing the bounded answer. | Add an import/dependency enumeration fast path: once deterministic import matches are grouped by package/file and exact counts are established, stop broad exploration and keep auxiliary evidence as support-only. |
| E20260520-G102 | `u4a` | P1 | Open | Final answer lists 5 importing packages, but semantic reviewer telemetry says the promoted enumeration facet has `DeclaredCount=4, AnchoredCount=4` and still reports no concern. | Reviewer/facet counters can diverge from the final visible member set, so telemetry may certify the wrong cardinality. | Make reviewer count attestations consume the same post-normalized principal row set rendered to the answer panel. Count mismatch between visible rows and reviewer/coverage counters should be telemetry at minimum and a hard diagnostic in eval. |
| E20260520-G103 | `u4b` | P2 | Open | The body is useful, but a high-confidence semantic concern about missing search-scope disclosure only results in the generic “coverage may be insufficient” supplement. | Concrete reviewer concerns are still collapsed into generic caveat prose, losing the actionable reason and making a good answer look vaguely untrusted. | When a reviewer concern is accepted as user-visible, render the specific localized limitation (“搜索范围：生产文件直接 import ...；未覆盖测试/生成代码”) instead of the generic supplement. |
| E20260520-G104 | `u5a` | P0 | Open | Final answer says exported functions `Compile`, `InferScenario`, and `RecomputeBudget` have no `_test.go` coverage, but current tests contain many `TestCompile_*` functions and a direct `TestInferScenario`. Self-consistency noticed the `InferScenario` evidence conflict at confidence 0.85, below the rewrite floor, so the wrong answer shipped. | Test-coverage enumeration is not using a precise definition/use pairing. The system can treat definition anchors as principal missing members while ignoring matching test symbols already present in the evidence anchors. Low-confidence reviewer contradiction is insufficient for a claim that can be deterministically checked. | Add a coverage-pair contract: exported symbol `X` is “untested” only after a deterministic search over `_test.go` for `TestX`, subtests, or direct calls under tests. If evidence contains positive test coverage for a listed symbol, remove it deterministically or fail eval. |
| E20260520-G105 | `u5a` | P1 | Open | Semantic reviewer reports promoted facet depth `declared=4, anchored=4` while the final answer lists 3 untested functions and evidence anchors include additional test identifiers. | Coverage counters for enumeration answers can include related test symbols or caveat carriers instead of only principal members, hiding a principal/evidence mismatch. | Same counter-source fix as G102, plus exclude related/test-support anchors from principal member cardinality unless the user asked to list them. |
| E20260520-G106 | `u6a` | P1 | Open | Self-consistency reviewer emitted two confidence-0.95 contradictions claiming `ErrAllRetriesExhausted` was fabricated / absent from evidence anchors, but the cited source line `internal/llm/openai.go:456` does contain `fmt.Errorf("%w: %w", ErrAllRetriesExhausted, lastErr)`. Final answer was marked with a generic inconsistency supplement. | Reviewer anchor extraction can miss identifiers present on cited source lines, producing high-confidence false positives that degrade good answers. | Reviewer contradiction checks must consume the same source-line quote/citation payload visible in the answer. A fabricated-identifier contradiction should be suppressed if the identifier is present in any cited quote or equivalent typed anchor. |
| E20260520-G107 | `u6a` | P2 | Open | Absence evidence for `github.com/pkg/errors` renders in the citation list as `go.mod — github.com/pkg/errors`, which looks like a positive citation even though the fact is zero matches / no dependency. | Negative evidence still has an awkward citation surface when forced through positive file:line references. | Render negative-search facts through the structured negative evidence channel (`repo/query/scope/result_count=0`) rather than a pseudo-positive citation line. |
| E20260520-G108 | `u7a` | P1 | Open | History lookup answer is correct and citation-free by design, but semantic review first emitted a concrete concern that live current-code file:line coverage was missing, then later marked sufficient. The run still collected 2000 code evidence rows for a simple git-log scalar with zero citations. | History/scalar questions are not cleanly separated from current-code coverage facets. Current-code anchors and evidence pools can dominate a VCS metadata question even when the authoritative answer is command output. | For `is_history_lookup=true` + scalar commit answers, use a history-metadata lane with command provenance, skip current-code citation facets after entity-existence proof, and keep code evidence out of the finalizer prompt unless needed for disambiguation. |
| E20260520-G109 | `u7a` | P2 | Open | Final answer renders two unlabeled scalar blocks as repeated `**值：**` lines: one for the commit hash and one for the subject. The user asked for two named fields, but the panel gives both the same generic label. | Scalar block rendering has no per-field label support, so multi-scalar answers become ambiguous even when the payload is correct. | Add a labeled scalar/table presentation for small key-value answers (`短哈希`, `subject 首行`) instead of multiple anonymous scalar blocks. |
| E20260520-G110 | `u5b` | P0 | Open | Exploration preserved an important nuance: `TestStopCondFired_TerminatesInsteadOfHotLooping` covers the stop-condition hot-loop / force-finalize behavior, while the exact `contractFailureBreak` `lastFinalize == nil` fallback branch is not directly tested. Final answer collapses this to a crisp “是的，有专门单元测试,” losing the boundary. | Closure prose nuance is not carried as a binding uncertainty/coverage boundary when aggregate member sets are present. Finalizer prefers the positive member set and drops the “closest match vs exact branch” distinction. | For “does X have a dedicated test?” answers, represent coverage as `exact`, `closest_behavioral`, or `not_direct` typed verdict. If the exact branch is not directly exercised, the final answer must say so before listing nearby regression tests. |
| E20260520-G111 | `u5b` | P1 | Open | Final answer appends a `stuck_injection_test.go` table with four auxiliary tests and even a visible `[illustrative]` marker, although the user asked for the dedicated unit test file/function for a specific fallback path. | Support/illustrative evidence can be normalized into principal-looking tables, and internal display markers still leak to users. | Keep `illustrative_only` / related-context evidence out of the principal answer surface unless explicitly requested. Internal markers such as `[illustrative]` must never render in final answers. |
| E20260520-G112 | `u5b` | P1 | Open | A direct yes/no test-lookup question took 50 explorer iterations, 30 `read_file` calls, 23 mid-loop injections, and parallel route churn before producing an answer. | Exact test-coverage lookup lacks convergence arbitration; multiple routes keep reading surrounding orchestrator/test files after the core candidate tests are known. | Add a test-lookup fast path: once candidate test functions and the target branch/function evidence are read, force a single verdict synthesis rather than opening broad route tails. |
| E20260520-G113 | `u8a` | P2 | Open | Rich API inventory answer is mostly good, but the rendered surface shows empty category headings (`导出的类型/函数/Kind 常量/变量`) followed by duplicate same-named tables. A generic “部分项证据支持稍弱” supplement appears even though reviewers marked the answer sufficient. | Section/table title dedupe is not clean for compiler-normalized inventory answers, and generic supplements still attach to otherwise complete exhaustive enumerations. | Collapse empty heading-only blocks into the following table title, and suppress generic weak-support supplements when every listed member has a citation and semantic review is sufficient. |
| E20260520-G114 | `u7b` | P0 | Mitigated | Original FAIL: user asked for how many of the last 20 `internal/orchestrator/` commits directly involved `runTaskGraph`; final answer said `5`, but an independent check yielded `0`. After the VCS deterministic lane (`u7b-20260520-125017`), the answer is `0` from `git_history_search`, with no manual commit-list comparison. | History/count questions were solved by model comparison over truncated command output instead of deterministic set operations. Read-mode command restrictions rejected safe shell forms, so the model approximated a VCS intersection and shipped the wrong scalar. | `git_history_search` now provides a bounded fixed-string VCS history count with provenance (`window_path`, `window_count`, `diff_path`, `contains`, `answer_count`, matched commits). Residual future work: richer semantic history queries such as “changed inside this function body even when the symbol text is absent” may need a function-range/diff-hunk aware variant. |
| E20260520-G115 | `u7b` | P1 | Mitigated | Original scalar history question consumed 55 explorer iterations, 3 explorer dispatches, 2142 evidence rows, and still produced a wrong count. After scalar-boundary fixes (`u7b-20260520-123245`) it still used 22 explorer iterations / 2 dispatches. After `git_history_search` (`u7b-20260520-125017`), it converges in 3 explorer iterations / 1 dispatch, 0 mid-loop injections, 0 `read_file`, and finalizer 1 turn. | No convergence fast path existed for exact VCS metadata questions. Current-code evidence and broad source anchors could dominate even when authoritative data was git metadata. | For `is_history_lookup=true` count/scalar tasks, exploration can close on bounded VCS tool results. Current-code reads are no longer required for proving the scalar; if the model chooses to disambiguate the symbol, that remains support-only. |
| E20260520-G116 | `u7b` | P1 | Mitigated | Initial final answer appended a system-normalized table titled `直接涉及 runTaskGraph 的 commit（与目录修改列表的交集）（5）` with blank `符号名称/定义位置/说明` cells. Current rerun after row-compiler and scalar-boundary fixes (`u7b-20260520-123245`) renders a scalar/prose answer with no blank commit table. | Row compiler rendered commit/history members through code-symbol table templates, producing empty columns and giving false authority to an inferred list. The later root was analyzer/extractor treating the `最近 20 次` scope window as a principal answer-member boundary. | System-generated table guards now refuse blank generated cells, and scalar count scope windows are stripped at analysis time so commit windows do not become code-symbol slates. Residual VCS convergence remains under G115. |
| E20260520-G142 | customer report + local repro `u7c` / `u7g` | P0 | Mitigated; perf follow-up open | 用户问“最近一次合入的是什么特性？”时，探索阶段已经得到完整特性说明，但分析阶段硬要求 `is_history_lookup=true` 必须同时 `is_scalar_answer=true`，下游把答案压成 `**值：** commit/subject` 式单值。修复后 `u7c` feature-summary 与 `u7g` “最近合入→对应代码→详细解释→逻辑图”均 PASS，且 finalizer 1 轮收敛、不再出现 `**值：**` 压缩；但 `u7g` 仍有 63 轮 explorer 与 30 次 mid-loop inject，属于性能/收敛尾巴。 | 系统把 history lookup 当成答案形态，而不是证据来源。既有 eval 只覆盖“commit hash/subject”和“历史计数”，导致实现拟合 scalar history，用例多样性不足；同时 architecture principal-support gate 和 aggregate member-set renderer 曾把 VCS 元数据当源码成员处理。 | 已将 `is_history_lookup` 重定义为 VCS/history 元数据来源信号，答案形态仍由 scalar/count/list/comparison/diagnostic typed 字段决定。非标量叙述/诊断型 history 跳过源码 file:line 主证据强制 gate，commit/count aggregate facts 降为 support；已补充 `u7c` feature-summary、`u7d` recent-N list、`u7e` commit comparison、`u7f` history+log diagnostic、`u7g` commit-to-code diagram eval。下一步收敛 VCS+代码解释类探索轮数。 |
| E20260520-G143 | `u7g` | P1 | Mitigated | `u7g` 首轮 analyzer 已经识别用户要求“最近合入→代码解释→逻辑图”，但 `subtopic_coherence` 因 `git log/merge commit/flow diagram` 等抽象任务实体不能解析为 repo symbol 而硬拒，重新分析后才进入探索。修复后 `u7g` PASS，analyzer 从 5 轮 / 2 dispatch 降到 2 轮 / 1 dispatch，且没有 `subtopic_coherence` 硬拒。 | Analyzer 的子问题一致性 gate 把历史来源、任务步骤、图形输出要求当成源码实体集合校验。对 history-backed explain/trace/diagram 请求，这些是用户意图/输出形态，不是 hallucinated repo symbols。 | 已将 R1.5 resolver asymmetry 调整为 typed-aware：非标量 `is_history_lookup`、非枚举/非标量的显式 `diagram_hint` 只产生 advisory，不硬拒；真实实体枚举、关系查询、标量定位仍保留硬 gate。剩余：u7g 仍有 54 explorer iterations / 21 mid-loop inject，归入 explorer convergence 后续批次。 |
| E20260520-G144 | `read_combo_config_absent_present_mix` | P2 | Tracking; short-term guard added | Exact-absence scalar suppression can be safe for a single absent target, but it would be too broad for a mixed answer such as “target A absent, target B present.” In that shape, a document-level `exact_resolution.status=absent` must not delete or relabel the present target's scalar/table value. | `exact_resolution` is currently document-level in several paths, while visible blocks and scalar values may refer to only one target. Without a per-target binding, a generic “absent” verdict can be confused with unrelated exact values in the same answer. The same principle applies to VCS/log/trace: typed provenance lanes must bind a negative or positive claim to its own target/scope, not to the whole document. | Short-term mitigation narrows scalar-block suppression to `len(exact_resolution.targets)==1`, so mixed target answers keep unrelated present values. Long-term fix: add per-target `exact_resolution[]` or block/claim-level `target_ref` and validate scalar/absence claims against that target binding. Added medium-priority eval case `read_combo_config_absent_present_mix` to keep this risk visible without blocking higher-ROI P0 work. |
| E20260520-G151 | unified evidence audit | P1 | Mitigated by Batch F.12; eval coverage added | Git results, logs, traces, command output, and repo-map/index output can all prove a bounded absence, but the only structured negative lane was `negative_search`, whose schema correctly requires `repo`. If used for non-repo absence, the model must invent repo dimensions or gets rejected. | Negative evidence was conflated with repository grep. The architecture needs origin-specific zero-result observations: git history can prove no commit matched, a diff can prove no path/hunk changed, a log can prove no error signature appeared, a trace can prove no long span exists, and command output can prove a deterministic zero count. None of these are current-source file:line facts. | Batch F.12 adds `aggregate_facts.kind=negative_observation` for non-repo zero-result facts with `origin + target/query/pattern/predicate + scope + result_count=0 + searched_at`. `negative_search` stays strict for repo searches, and legacy non-repo `negative_search` with explicit non-repo origin is normalized to `negative_observation`. Added eval guards `u7m`, `logtri_no_fatal`, `hitrace_no_long_gc`, plus mixed positive/negative guards `u7n`, `logtri_warn_no_fatal`, and `hitrace_gc_present_no_long`; remaining guard ideas are diff no-change and command zero-count questions. |
| E20260520-G152 | runtime artifact line/order questions | P1 | Mitigated by Batch F.13; eval coverage added | Attached logs/traces sometimes need their own anchors: users ask "第几行", "第几个事件", or "哪一段 span". Existing `LogFrame.File/Line` and `PerfFrame.FrameNo` describe source frames or typed spans, not raw artifact row numbers. Small inline artifacts were rendered without gutters, so the model could only count lines itself; large artifacts relied on blob `read_file` after the model guessed it needed pagination. | Runtime-artifact coordinates were missing a first-class prompt surface. If we reuse repo citations, logs/trace lines pollute current-source proof; if we do nothing, answers can have off-by-one artifact line numbers or force unnecessary repo reads. | Batch F.13 reuses the read-file gutter contract via `internal/textfmt.LineGutter` and renders attached log/trace lines as `N│ text` with an explicit artifact-vs-repo boundary note. Oversized artifacts keep blob offload and preview gutters for shown lines, while exact middle anchors require `read_file` on the blob. Added eval guards `logtri_artifact_line_anchor` and `hitrace_artifact_line_anchor`. Longer-term first-class span IDs remain under the runtime observation ledger work. |
| E20260520-G153 | `u7l-20260523-222442` | P2 | Open | The answer preserved a rich recent-10-commit narrative and avoided the dry system supplement table, but it miscopied design document paths mentioned in VCS history: examples with actual `20260523` dates appeared as `20250523`. | VCS metadata is now a first-class principal source for commit hashes and summaries, but file paths / touched docs inside commit narratives are still ordinary prose. The finalizer can paraphrase or miscopy date-like path literals because they are not carried as typed row details with copy-exact pressure. | Extend the VCS observation lane to preserve exact changed-file paths / path-like literals when they are relevant to the answer, and teach finalizer to either copy those exact typed literals or avoid inventing path details. Add a targeted expectation for recent-N commit summaries that mention changed docs/files without date drift. |
| E20260520-G145 | customer snippet `../customlogs/git_diff_001.log` | P1 | Mitigated by Batch 14 | 用户问“根据最近一次提交代码差异做详细分析”，analyzer stream stall 后 UI 同时出现正确的“模型响应出错”与错误的“验证还不够稳”；explorer 首轮用 `cd /home/mindie && git ... 2>/dev/null`，随后又用 `git -C /home/mindie` 和 `--git-dir=/home/mindie/.git`；`2>/dev/null` 被当成写文件拒绝，`git -C` 被误解析成子命令 `/home/mindie`。最后答案审阅 consistent/sufficient 后仍进入一次本地 auto-repair contract recheck。 | Read-mode shell guard was simultaneously too strict and too loose: it rejected safe stderr-to-null redirection, but did not parse git global cwd options well enough to produce actionable repo-bound guidance. Explorer prompt/tool surface also under-emphasized that `exec_command` already runs from the active repo root and should not `cd`/`git -C` to arbitrary absolute paths. Analyze transient retry reused a generic validation copy at one call site. VCS history/diff tools existed but were not prominent enough for commit-diff explanation questions, so the model fell back to fragile shell. | Implemented: analyze transient retry now uses the stage-aware “模型响应出错” copy; read-mode shell grammar accepts only provably read-only redirections such as `2>/dev/null` / fd duplication and still rejects `2>1` / `1>2`; git global path options (`-C`, `--git-dir`, `--work-tree`, `GIT_DIR`, `GIT_WORK_TREE`) must stay repo-relative and refusals name the active repo command root; structured `git_log` / `git_diff` / `git_history_search` are exposed/preferred and their `path` / `repo_path` parameters are repo-root scoped; `git_diff` supports patch/stat/name-only modes with blob pagination for large outputs. Finalizer metadata auto-repair remains local and does not mean semantic inconsistency or LLM rewrite. |
| E20260520-G146 | `u7a` rerun while adding `u7h/u7i/u7j` | P0 | Mitigated; rerun required | Existing “first commit introducing EvidenceClosure” case failed: `git_history_search(window_count=1)` returned the most recent matching file-window commit (`58881dba`) and the model treated it as the first introduction, while direct `git log --reverse -S 'type EvidenceClosure'` proves the correct first commit is `01e0864e`. | `git_history_search` encoded only “bounded recent window” semantics, but its name/schema did not let the model express earliest/first-introduced history searches. A precise VCS question was answered with a deterministically wrong recent-window result, not with a model hallucination. | `git_history_search` now exposes explicit `order=recent|oldest`; recent remains default for latest/last-N windows, while oldest scans from the earliest commit and returns up to `window_count` matching commits. Explorer/default skill prompts now tell models which order to use. Added a unit test for symbols introduced after an existing file already existed, plus eval guards `u7h/u7i/u7j` for richer diff/history shapes. |
| E20260520-G147 | `u7j-20260520-163512` | P1 | Mitigated by Batch 15; rerun required | 全历史 topic search 最终没有压成标量，但 explorer 花了 159 轮、52 次 read_file、37 次 midloop 注入。多个并行探索路各自反复围绕同一 scalar 历史主题读源码、修 line anchor、补 principal span，远超用户问题所需。 | VCS history topic-search 仍被后续 evidence/dataflow gate 拉回当前源码机制链，导致“提交演进归纳”被当成源码 principal span 调查。对于这类问题，commit metadata + diff/stat/name-only 摘要才是 principal evidence；当前源码只应作为解释补充。 | Batch 15 已新增 typed-only VCS-history narrative boundary：纯历史叙事不再把当前源码 RequiredFiles 当成强制必读，并允许已完成的 VCS 结论提前取消慢源代码分路；history+diagram/current-code/diagnostic/change-impact 仍保留 mixed evidence lane。新增 `u7k` 覆盖全历史 topic + 当前实现解释组合，后续需重跑 `u7j/u7k` 验证成本下降和答案丰富度。 |
| E20260520-G148 | customer render snippet `git show <hash> --no-stat` | P1 | Mitigated by structured tool + compatibility shim | 小模型在探索阶段并行执行 `git show <hash> --no-stat` 三次，客户 git 返回 `fatal: unrecognized argument: --no-stat`，界面连续展示三条失败工具结果。 | `git show` 并不支持 `--no-stat`；模型把“不要 stat”当成一个选项，但默认 `git show` 本来就不显示 stat。系统只有 `git_log/git_diff/git_history_search`，缺少针对“查看某个 commit/ref 的 metadata/patch/stat/name-only”的结构化工具，导致模型退回 shell 并撞上跨 git 版本/命令语义差异。 | 新增 `git_show` 结构化工具，支持 `no_patch`/`stat`/`name_only`/默认 patch 四种安全形态；explorer/default skill prompt 推荐 commit/ref 详情使用 `git_show`。同时 read-mode `exec_command` 兼容层会删除确定安全的 `git show ... --no-stat` no-op 并在工具输出中说明，避免旧提示或小模型继续产生可见失败。 |
| E20260520-G149 | `u7k-20260520-193504` | P1 | Partially mitigated; evidence-origin lane open | diff+当前代码解释没有再压成 commit-id 标量，但探索阶段把历史 diff 中出现过、当前已改名的 `emitEvidenceCommandScalarSoftSkipReason` 当成当前源码锚点去落地，随后 forced-read / evidence repair 反复追逐当前文件里不存在的旧符号。 | 系统缺少一等 VCS/diff evidence origin。历史 hunk 中的旧符号、旧文件名、旧行号是有效历史事实，但不能自动成为 current_source file:line obligation；当前源码 claims 必须由当前 checkout 的 read_file 证据证明。 | 短期已做：history-backed current-code explanation 跳过 generic forced-read gates，保留显式端点 call_chain 强 gate；narrative closure 的可选 aggregate_facts 可以按项丢弃无效支持事实。长期：新增 `vcs_diff` / `vcs_metadata` origin 或 aggregate lane，把旧符号作为历史事实传到 finalizer，并在当前源码 lane 单独绑定“现在叫/现在在”的符号。 |
| E20260520-G150 | `u7j-20260520-194845` | P1 | Mitigated; rerun required | 全历史 topic search 中，explorer 的 VCS member_set 使用 `916511... (最早)` / `e4f15... (最近)` 这类 decorated commit hash。通用 aggregate gate 把它误当成 `Gate.Run (说明)` 这种代码成员，要求 `support_refs=file:line`，导致一次不必要的 completion reject。 | `support_refs` gate 的设计目标是代码成员 citation 对齐，但 commit hash 是 VCS metadata，不应该被 current-source file:line 证明。缺少 typed provenance 时，系统把所有 decorated ASCII 成员都当成 code identity。 | 已将豁免收窄为 typed history + 结构化 commit hash：只有 `is_history_lookup=true` 且 decorated member base 是 7-40 位 hex commit id 时才不要求 file:line；普通 decorated code members 继续硬要求 `support_refs`。长期仍归入 G149 的一等 `vcs_metadata` evidence origin。 |
| E20260520-G117 | `u8b` | P2 | Open | PASS and independently verified: `internal/types` currently has 104 exported `type X string` declarations and all 104 have typed const-set evidence by heuristic scan. Residual issue: the run still emitted `证据锚点偏弱 ... line-text=12%` for a large exact enumeration and produced a very large citation list. | Large exact inventories can be correct while line-text grounding telemetry looks weak because many rows cite type declarations rather than every const member line. The system needs to distinguish “verified inventory summary” from low-quality grounding. | Keep this as a positive baseline for rich enumeration, but add telemetry that separates exact compiler-backed inventory completeness from per-row line-text density. Do not convert this into a hard retry signal. |
| E20260520-G118 | `u9a`, `u9b` | P1 | Open | Analyzer took 5 rounds for both direct mechanism/error-granularity questions. In `u9b`, it first emitted `intent=root_cause` for a non-diagnostic error-granularity question and was rejected, then corrected to `intent=explain`. | Analyzer still tries to perform line-level investigation during prescan, and root-cause/error-granularity taxonomy is too easy to confuse. The hard rejection is correct by schema, but the route wastes user-visible time. | Add a schema-aware intent normalizer or stronger classifier contract: error-granularity questions about code behavior should default to `explain` unless a diagnostic typed signal is present. Prescan should stop after file existence for these mechanism questions. |
| E20260520-G119 | `u9b` | P1 | Mitigated and replay-verified | Explorer had the right conclusion by round 7, but two `emit_investigation_complete` attempts were rejected: `negative_search` required repo dimension, then `bucket_count` rejected categorical values like `item rejected, batch succeeds`. The model eventually removed aggregate facts entirely. | Aggregate fact schema was too narrow for categorical behavioral outcomes. It forced models to misuse count/negative-search shapes, then lose structured closure data after rejection. | Added first-class aggregate kinds `behavior_outcome` and `error_granularity_verdict`. They preserve stable categorical values plus dimensions/support_refs without numeric count validation and compile into the observation ledger as categorical records, not fake count rows. Replay `u9b-20260521-212100` PASSed with `finalizer_iters=1`, no aggregate schema rejection, and no semantic-quality concern. The same batch also dedupes typed decision rendering so a model body starting with the same verdict no longer shows `per_item_rejection — per_item_rejection...`. |
| E20260520-G120 | `u9b` | P1 | Open | Extractor rejected `emit_hypothesis_verdict` because citation `internal/tool/emit_evidence.go:560` did not corroborate nearby identifiers, even though that line is the exact `if perr != nil` branch central to the rationale. The pipeline proceeded without the verdict. | Corroboration heuristics can reject semantically central control-flow lines when the visible identifier set is local (`perr`) rather than the high-level concept in the rationale. | Hypothesis citation validation should accept already-grounded evidence IDs or control-flow anchors from exploration instead of re-matching rationale prose against a single line. Local-variable lines can be valid for branch semantics. |
| E20260520-G121 | `u9b` | P2 | Open | Semantic reviewer reported `sufficient=true confidence=0.92`, but the final answer still appended generic “覆盖度可能不充分 / 枚举类条目支持稍弱” supplements. | Generic supplements are still attached from low-level coverage/facet telemetry even when semantic review marks the visible answer sufficient and the user asked for a behavior verdict, not an enumeration. | Suppress generic supplements when a sufficient semantic verdict exists and no precise user-visible defect is accepted. If a caveat remains needed, render the exact localized limitation. |
| E20260520-G122 | `arkts_repomap` | P1 | Mitigated; rerun required | Analyzer called `grep` with `file_type=arkts`; ripgrep rejected it as `unrecognized file type: arkts`, even though repomap recognizes 6 ArkTS files and the repository has `.ets` corpus files. | Repo-map language detection and grep/search file-type routing were not unified. A language supported by repomap could still be unusable as a search file type, forcing fallback loops. | `grep` now uses the repository's shared `file_type -> glob` adapter for every backend, including ripgrep, so repo-map-only languages such as ArkTS (`*.ets`) and Cangjie (`*.cj`) do not depend on `rg --type` support. Guards prove these file types search successfully without broadening ArkTS into ordinary `.ts` without a project probe. Rerun `arkts_repomap` next. |
| E20260520-G123 | `arkts_repomap` | P1 | Open | First analyzer attempt discovered the right ArkTS files, but the quality gate hard-rejected `subtopic_coherence` because sub-topic file entities did not overlap primary decorator entities `@Entry/@Builder`. A second analyzer dispatch was needed. | Subtopic coherence is too strict for decorator/search-target inventory questions: the primary entities are marker tokens, while valid subtopics are discovered file/function buckets. | For enumeration by marker/decorator/annotation/literal, allow subtopics anchored by discovered file/function members when they are supported by required_files/buckets. Hard coherence should apply only to structurally precise comparable entity sets. |
| E20260520-G124 | `arkts_repomap` | P1 | Open | Case failed `missing:@Component`. Exploration evidence preserved adjacent decorator stack terms (`@Entry`, `@Component`, `@State`, etc.), but the final answer simplified @Entry rows to function/path/description and omitted `@Component`. | Finalizer/table rendering can drop language-significant adjacent annotations/decorators even when they are in `surface_terms`. For ArkTS, `@Entry` page entry is conventionally paired with `@Component`, so omitting the decorator stack loses useful evidence and can fail eval expectations. | Add a generic “marker/decorator stack” lane for language inventories. When the queried member is a decorated declaration, preserve all verified adjacent decorator surface terms in a column such as `标记/装饰器`, without making them hard gates for unrelated languages. |
| E20260520-G125 | `arkts_repomap` | P1 | Open | Self-consistency reviewer falsely reported that the body listed all 6 names under the @Entry section and 0 under @Builder, while the final rendered tables clearly have 4 @Entry rows and 2 @Builder rows. Confidence was below floor, so no rewrite, but telemetry is wrong. | Reviewer table/section parsing is still not aligned with the actual rendered Markdown. It can misattribute rows across adjacent sections and invent contradictions. | Feed reviewers structured rendered blocks with section ownership and table rows, not only flattened body text. Section-scoped row counts should be computed deterministically before reviewer reasoning. |
| E20260520-G126 | `hilog_arkts_panic` | P1 | Open | PASS answer is useful, but an external-only log with `resolved_files=0` still spawned multiple explorer routes (visible up to 第 6 路), attempted repo searches, attempted `emit_evidence` with non-repo artifact paths, and needed 17 explorer iterations plus 6 mid-loop injections. | External-only runtime artifact handling is still advisory rather than a decisive route. The system already has enough structured `emit_log_triage` facts, yet downstream exploration replays repo-grounding instincts. | Add a direct log-triage-to-finalize fast path for `external_only_log` when the user asks what the artifact shows. Skip repo exploration and evidence tools unless the user explicitly asks to verify current code. |
| E20260520-G127 | `hilog_arkts_panic` | P1 | Partially mitigated by Batch F.11 | Extractor carried hypotheses such as `h1: caused by ArkTS or a module it depends on` and rejected h1 because there was no `AnchorKind=call` evidence. That criterion is irrelevant to an external-only runtime artifact and appears only as internal workflow noise. | Code-grounded hypothesis criteria leak into artifact-only diagnostics. Missing repo call evidence can be interpreted as hypothesis falsification even when the artifact question should be answered from log frames. | Batch F.11 skips orchestrator auto-verdicts and extractor criterion auto-verdict injection for observation-only runtime artifacts, so missing current-source call evidence no longer falsifies an artifact-only answer. Rerun `hilog_cangjie_panic-20260521-024522` converged with `extractor_iters=1` and no rejected hypothesis verdict. Remaining: add first-class `artifact_frame_set` / `runtime_call_chain` aggregates so artifact hypotheses do not need code-call criteria at all. |
| E20260520-G128 | `hilog_arkts_panic` | P2 | Partially mitigated by Batch F.11 | Finalizer prompt contained impossible/incorrect validation telemetry such as `|evidence|=2 >= 3`, followed by `richness facet_softened ... observed_artifact_fact ... no surface evidence`, even though runtime observations were present and the answer was sufficient. | Artifact evidence counters and richness facets do not understand log-origin observations as first-class evidence. They can emit contradictory diagnostics and softening warnings that are harmless in this run but risky for gating. | Batch F.11 lets observation-only runtime artifacts satisfy the evidence-count floor from structured artifact observations instead of repo-line evidence counts. Rerun `hilog_cangjie_panic-20260521-024522` had no mid-loop injection or rewrite. Remaining: perf-trace facet/citation accounting and a unified observation ledger, tracked under G137-G140 and the F.11 design note. |
| E20260520-G129 | `hilog_arkts_panic` | P2 | Open | The summary names the innermost crash site `UserCard.build` at `UserCard.ets:42`, but the principal ordered list only includes `UserCard.__updateChildElement` and `IndexPage.__updateRoot`. | Runtime observation lane can drop the innermost trigger frame from the structured visible list even when the summary relies on it. | For crash/root-cause artifacts, the observed frame list should include at least the innermost failing frame, the immediate caller, and the top relevant caller, unless the user asks for a shorter summary. |
| E20260520-G130 | `cangjie_repomap` | P1 | Open | PASS answer gives the right declaration set (1 extend, 1 foreign func, 5 public class) but repeatedly says package declarations are on “同文件第 1 行.” The source and exploration evidence show every package declaration at line 4. | Finalizer invented a nearby source-line detail for package paths instead of reusing the grounded package evidence lane. This is a small but concrete file:line falsehood in a language-inventory answer. | Package/module path details must be carried as typed attributes with their own citation or rendered without line-number claims. Do not allow finalizer to invent line numbers for secondary attributes. |
| E20260520-G131 | `cangjie_repomap` | P1 | Open | The final surface contains the model-authored rich grouped answer, then deterministic `extend/foreign/public class` tables, then the same deterministic tables a second time, plus a generic supplement. | Enumeration row compiler still duplicates already-sufficient grouped answers and can repeat the same补表 twice. The duplication greatly degrades an otherwise correct answer. | Add a single-source-of-truth rendering contract: if model-authored principal tables cover every required member with citations, compiler rows should merge missing attributes only, not append full duplicate tables. Enforce one compiler block per category. |
| E20260520-G132 | `cangjie_repomap` | P1 | Open | Semantic reviewer spent ~65s across review and auto-repair, twice claiming the BODY omitted file paths entirely, even though every list item and table row visibly includes file paths. It still added a generic coverage supplement. | Reviewer input/parser is not table/prose-aware enough and can contradict the visible answer. Auto-repair then costs time without fixing a real defect. | Feed semantic review structured post-render rows and suppress reviewer concerns that contradict deterministic visible-surface extraction. This repeats G75/G92 in a Cangjie language-inventory setting. |
| E20260520-G133 | `cangjie_repomap` | P2 | Open | Cangjie corpus inventory took 29 explorer iterations, 29 `read_file` calls, and 9 mid-loop injections. Multiple explorer routes explored parser implementation and corpus fixtures concurrently before converging. | Language-specific exhaustive inventory lacks a deterministic source-file enumeration route. The system re-discovers fixture coverage through LLM loops instead of using repomap/source-extension knowledge to bound the candidate set. | For language fixture/source inventories, seed exploration with the exact language file set and typed pattern extractors where repomap already supports the language. Keep parser implementation files as support-only unless the user asks how parsing works. |
| E20260520-G134 | `hilog_cangjie_panic` | P1 | Mitigated by Batch F.9/F.10 | Finalizer emitted `citations[]` with external runtime paths (`src/main.cj`, `src/cart/cart.cj`) even though the prompt/tool warned observation-only answers must omit current-repo citations and use `citation_ref=-1`. The tool accepted and rendered these external paths in the citation list. | External artifact citation discipline was advisory in emit-time validation. External stack-frame paths could leak through the repo citation channel and look like current-checkout source citations. | Batch F.9 added pre-emit normalization: observation-only runtime answers drop repo-style citation pools and set artifact items to `citation_ref=-1`; mixed current/runtime answers drop only typed observed-frame citations and remap remaining current-source indexes. Rerun `hilog_cangjie_panic-20260521-021900` confirmed no runtime citation pool and no current-repo citation surface for the external stack frames. |
| E20260520-G135 | `hilog_cangjie_panic` | P1 | Mitigated by Batch F.10 | Deterministic normalization appended `系统按已验证证据补充缺失成员：调用链（1）` with a single `<native>@runtime:0` row and blank columns. | Member-set normalization was treating runtime/native frames as code-symbol enumeration members, producing empty table rows. | Batch F.10 applies the observation-only runtime boundary to deterministic row compilers, aggregate member carriers, aggregate coverage, and cardinality gates. Runtime call-chain/member facts are preserved as artifact observations and no longer trigger current-source system补表 unless a later first-class artifact presentation asks for one explicitly. Rerun `hilog_cangjie_panic-20260521-021900` confirmed no `系统按已验证证据补充成员` table, no reviewer dispatch, and no finalizer rewrite. |
| E20260520-G136 | `hilog_mixed_arkts_cangjie` | P2 | Partially mitigated by Batch F.11 | PASS answer is good, but explorer first tried to encode language-labelled external frames as decorated aggregate members and was rejected because they had no repo `support_refs`; the model then stripped qualifiers to proceed. | External runtime frame aggregates were forced through repo-member support-ref ergonomics. Cross-language frame role / language labels are valuable user-facing structure, but the only available schema made them look like repo symbols needing support refs. | Batch F.11 exempts decorated runtime aggregate members from repo `support_refs` only when the request is typed as observation-only runtime artifact; decorated current-source members still require grounding. Remaining: add an `artifact_frame_set` / `runtime_call_chain` aggregate kind with fields such as language, role, artifact_path, line, message, and frame_order so this does not depend on member-set compatibility. |
| E20260520-G137 | `hitrace_jank` | P1 | Open | `log_triager` prompt told the model that HiTrace/atrace/perfetto traces should be handled by the separate `perf_triager` and to emit an empty errors/signals bundle. The model obeyed, but `emit_log_triage` rejected the empty payload (`errors[], observations[], and unknown_chunks[] all empty`), forcing the model to stuff trace spans into `unknown_chunks`. | Prompt and tool contract contradict each other. A correct model action is rejected, then a lossy workaround becomes the downstream evidence source. | Either skip `log_triager` entirely for detected perf trace blobs or let `emit_log_triage` emit a typed `handoff_to_perf_triager` / empty-pass bundle. Never require the model to put known perf spans in `unknown_chunks`. |
| E20260520-G138 | `hitrace_jank` | P1 | Open | Dedicated `perf_triager` / `emit_perf_trace` did not appear to own the run. The pipeline proceeded through normal analyze/explore, hit analyzer `shape_subject_coherence`, then searched current repo files such as `internal/skill/defaults.go`, `internal/agent/analyzer.go`, and `internal/tool/emit_perf_trace.go` to answer a runtime trace question. | Performance trace dispatch is advisory rather than decisive. Runtime trace questions can fall back to current-code exploration even when the attached artifact alone proves the frame timings and stack. | Add a perf-trace fast path from detected `tracing_mark_write` spans to `perf_triager` output and then to finalizer. Current-code exploration should run only when the user explicitly asks to verify implementation or when artifact frames resolve to repo files. |
| E20260520-G139 | `hitrace_jank` | P1 | Open | Finalizer prompt required HARD `current_code_path` and the reviewer complained about `Current code path declared=1 anchored=0`, while the correct evidence was runtime observations (`H:RenderService:DoFrame`, `H:Layout:measure`, `H:DataLoader:fetchSync`). The run also logged `citation_count_ge 2 — 0 citations < 2` even though runtime-observation citations were emitted. | Artifact evidence and current-code evidence are still partially conflated. Runtime observations are accepted enough to render an answer, but facet/citation accounting still treats them as missing repo citations. | Give perf/runtime observations first-class facet and citation accounting (`observed_artifact_fact`, frame_order, duration_ms, trigger_span). Do not require `current_code_path` or global repo citation floors for external-only trace answers. |
| E20260520-G140 | `hitrace_jank` | P2 | Open | The final answer correctly identified the 86ms `RenderService:DoFrame` jank and 84ms `DataLoader:fetchSync` blocker, but still appended generic “未达到预期标准 / 需要补充验证” text due to the mismatched citation/facet checks. | Generic supplements continue to surface internal validation noise even when the user-visible artifact answer is complete and the gap is in system accounting. | Suppress generic supplements caused by known artifact-accounting mismatches; render a precise localized boundary only when the answer truly lacks a user-relevant fact. |
| E20260520-G141 | `logtri_goroutine_dump` | P1 | Mitigated by Batch F.8/F.9 | PASS answer identified goroutines 15/87/120, but then verified current checkout despite `resolved_files=0`, cited external-binary stack frames through the repo citation pool (`internal/agent/analyzer.go:100`), rendered a system table `并发崩溃的 goroutine（3）` with blank location/description cells, and appended a generic “未达到预期标准” supplement. | External-only log triage let artifact frames become code-symbol member sets and repo citations. The system also added empty member补表 rows to a simple artifact answer instead of preserving the log-frame answer shape. | Batch F.8 kept external-source runtime artifacts observation-only unless an explicit current-checkout verification anchor exists; Batch F.9 removed artifact frame citations from repo citation pools. Rerun `logtri_goroutine_dump-20260521-013104` confirmed no analyzer pre-scan, no explorer repo reads, no duplicate system supplement, and no finalizer rewrite. |

## Post-Contract Targeted Eval Findings

| ID | Case / Source | Severity | Status | Symptom | Systemic Gap | Follow-up Direction |
| --- | --- | --- | --- | --- | --- | --- |
| E20260521-G142 | `u7n-20260521-112114` / `u7n-20260521-113334` | P0 | Mitigated and guarded | A two-target history existence query was classified as `return_value + is_scalar_answer=true`; the final answer contained useful prose and member tables, but deterministic rendering also surfaced two generic `**值：**` scalar blocks. The eval's absent marker was also unsafe because the full marker string had entered git history through an earlier committed case. | Repository-history evidence source was correctly typed, but answer shape still treated multiple per-target results as one scalar lane. Eval negative markers can self-poison when committed as a full literal. Titled scalar rendering also made per-target values look like anonymous system-compressed values. | Implemented typed analyzer reconciliation: `history_lookup + scalar_answer + multiple typed targets` becomes per-target set/enumeration output unless it is a true count or role-locate scalar; the answer intent contract applies the same rule. Titled scalar blocks now render the model-authored title as the visible label instead of generic `值/Value`. `u7n.case` now constructs the absent marker from split shell fragments so the full search token is not committed. Rerun `u7n-20260521-113334` passed with `requested outputs: summary, enumeration`, `finalizer_iters=1`, and no repair/rewrite. |
| E20260521-G143 | `u7c-20260521-114941` / `u7c-20260521-120536` | P0 | Mitigated and guarded | After the multi-target history fix, the latest-feature answer content was acceptable, but deterministic scenario fallback still converted `intent=explain + scenario=generic + is_history_lookup=true` into `architecture_explain`. That loaded architecture DAG objectives, required `component_relation` sections, and let the semantic reviewer complain about missing current-source proof/diagram. The shipped answer also gained generic “coverage may be insufficient” supplements even though the user asked for VCS history metadata. | `compiler.InferScenario` used architecture as the default for broad `explain`, and `QFGeneric` still attached soft current-code facets to pure VCS narratives. This was a system-origin shape escalation: VCS/history was the principal evidence lane, but downstream contracts asked for current-source architecture evidence. | Pure non-scalar history narratives now keep `ScenarioGeneric`, `QFGeneric` omits current-source/component/diagram facets for that typed boundary, and LLM reviewers / soft caveat materialization skip current-source-oriented pressure unless the request also carries current-code, diagram, diagnostic, change-impact, relation, scalar, or count signals. Rerun `u7c-20260521-120536` passed with `family=generic`, required summary only, `semantic_quality_concerns=0`, `finalizer_iters=1`, and no generic补充说明. |
| E20260521-G145 | `u7o-20260521-121358` / `u7o-20260521-122249` | P1 | Guard refined and passed | The mixed “latest diff + current source impact” eval initially failed only because the assertion banned or under-matched wording around the literal old scalar label `**值：**`. The product answer kept VCS diff provenance and current implementation analysis, and it remained `family=architecture` rather than being swept into the pure-history shortcut. | Mixed VCS+current-code questions are intentionally not pure history narratives. Eval guards must verify both provenance lanes without banning literals that can be legitimate evidence about the bug being analyzed; otherwise the suite can pressure the product toward deleting useful explanation. | `u7o.case` now allows quoted scalar-label text while still requiring both commit/diff provenance and current implementation / impact language. Rerun `u7o-20260521-122249` passed with `finalizer_iters=1`, `semantic_quality_concerns=0`, and no repair/rewrite. The typed pure-history reviewer skip remains guarded so mixed `change-impact` questions still receive current-source checks. A residual performance note remains: this class can still be expensive because it legitimately combines VCS observations with current-source reads (`explorer_iters=32`, `midloop_inject=9` in the rerun). |
| E20260521-G146 | Image diagnostic `1st question processing flow failure root cause` | P1 | Mitigated; targeted eval rerun pending | The attached diagnostic image shows an analyzer/root-cause query cascading through `emit_analysis` field reconciliation, CGEC/gate warnings, ERM unsatisfied state, and a DAG `has_enough_facts` blockage. Current code has already softened several old coherence failures, but two structural risks remained: analyzer gate failures defaulted to hard unless explicitly excluded, and explorer readiness could request another evidence round when only ERM breadth bookkeeping was unsatisfied. | This is the same architectural family as earlier finalizer/explorer loops: noisy or support-tier signals can accidentally become retry gates. Coverage/hint completeness and ERM breadth are useful guidance, but they are not proof that the model/user intent is wrong. | Implemented Batch 7: analyzer hard rejection now uses an explicit typed allowlist, `coverage` is soft telemetry, and ERM-only unsatisfied breadth gaps no longer demote `HasEnoughFacts` or build explorer fact-retry hints. Targeted tests pin soft coverage, still-hard structural gates, and ERM-only non-retry; rerun analyzer/explorer evals before closing this row. |
| E20260521-G147 | `qf_sequence_analyzer_gate-20260521-132150`, `read_combo_analyze_retry_anchor-20260521-132615` / rerun `read_combo_analyze_retry_anchor-20260521-134015` | P1 | Mitigated; residual performance tracked separately | Post-Batch-7 targeted evals passed with one finalizer round and no analyzer hard retry, proving the image-class hard-gate loop is mitigated. However, `read_combo_analyze_retry_anchor` still spent `explorer_iters=52` and `midloop_inject=28`; one parallel lane emitted enumeration/partial-read and repair hints for a mechanism explanation after another lane had already produced enough grounded reasoning. | This is not a final-answer correctness failure, but it is the same "support-tier signal becomes too loud" family at the mid-loop guidance layer. Mechanism exact-anchor questions can be internally multi-topic without requiring exhaustive enumeration coverage across every discovered file, and parallel lanes should not keep widening once an accepted closure/evidence set already covers the typed anchors. | Implemented typed mid-loop gating: enumeration-coverage hints now require a principal member-set enumeration (`IsCategoryEnumerationAnswerShape`, explicit enumeration boundary/completeness obligation, or required member_set handoff). A regression test pins `intent=explain + scenario=architecture_explain + question_kind=mechanism + multiple subtopics + predicate_axis=condition` with stale `isEnumerationQuery=true` and verifies it does not emit the enumeration coverage hint. Rerun passed; the erroneous `question asks for an enumeration` hint disappeared, `explorer_iters` improved `52→42`, and `midloop_inject` improved `28→21`. |
| E20260521-G148 | `read_combo_analyze_retry_anchor-20260521-134015` / rerun `read_combo_analyze_retry_anchor-20260521-135342` | P1 | Mitigated for diagram concern; broader semantic reviewer noise remains | The rerun still shows `semantic_quality_concerns=1`: semantic reviewer emitted a `Diagram facet` concern for a mechanism explanation where the user did not request a diagram. It did not force a finalizer rewrite, but it is noisy telemetry and can later become user-visible pressure if promotion rules change. | Architecture/mechanism explanation defaults were still too eager to treat diagram coverage as a reviewer concern. The correct hard/soft source is the typed `EffectiveDiagramContract` / explicit presentation directive, not broad `architecture_explain` family membership. | Implemented typed semantic-quality input filtering: non-hard `diagram_spine` facets are withheld from reviewer required-facet/depth audits unless the answer semantic view carries a required `DiagramPlan`. Hard diagram requirements still pass through. Added a regression test proving a mechanism question without required diagram contract does not send soft diagram coverage to the reviewer. Rerun verified the diagram concern disappeared; the run still emitted other semantic-quality concerns, recorded separately below. |
| E20260521-G149 | `read_combo_analyze_retry_anchor-20260521-134015` / reruns through `read_combo_analyze_retry_anchor-20260521-170606` | P2 | Mitigated for stale forced-read debt and prompt duplication; convergence follow-up open | Even after enumeration hint suppression, the passing reruns still had heavy exploration: `explorer_iters=42/midloop_inject=21`, `55/26`, later `55/25`, plus prior `emit_investigation_complete DOWNGRADED — pending forced reads block the closure`. After accepted-boundary cleanup and aggregate schema repair, replay improved to `explorer_iters=24`, `midloop_inject=13`, `tool_read_file=26`, `finalizer_iters=1`, and no aggregate schema reject. The latest replay stayed PASS with `explorer_iters=25`, `midloop_inject=12`, `tool_read_file=24`, and no duplicate forced-read / empty search-gap hint sections. | Parallel exploration already has early convergence, but fork merging could still import support-tier `PendingReads` from a non-converged sibling before the accepted closure fork was merged. Accepted completion cleared repair directives, but not pending forced-read debt, so later windows could treat stale support reads as live principal blockers. After stale-debt cleanup, remaining cost is mostly legitimate exact-anchor completion plus prompt/reconciliation overhead. Prompt rendering then had a separate UX gap: several same-kind repair directives produced repeated or empty sections. | Added the missing boundary cleanup: accepted `emit_investigation_complete` and `MutableState.MergeExploreFork` now clear both repair directives and pending forced reads. This preserves read/evidence summaries, only drops the strong "must read" debt after a typed closure has passed pre-complete gates. Existing wait rules still prevent early convergence for explicit buckets, exhaustive enumerations, required diagrams, diagnostics, change-impact, and field-value questions. Guards cover direct completion and parallel sibling merge. Closure-repair prompts now merge same-kind read repairs into one `Forced Read List` and suppress empty `Search Coverage Gap` sections, without weakening explicitly requested anchor reads. |
| E20260521-G150 | `read_combo_analyze_retry_anchor-20260521-135342` / reruns through `read_combo_analyze_retry_anchor-20260521-172824` | P2 | Mitigated for structured-item false positives; residual telemetry open | After G148 removed the diagram false positive, semantic reviewer still emitted concerns about missing inline current-code anchors and `MaxRetriesPerStage` default-upper-bound support refs. The final answer passed and did not rewrite, but reviewer cost/telemetry remained noisy. Latest reruns pass in one finalizer round with `semantic_quality_concerns=0`; the reviewer still may mention a small label-only declared/anchored delta in reasoning, but it no longer turns into a concern/retry/supplement. | Semantic reviewer depth accounting was too narrow: block-level facet declarations were considered anchored only by claim_uses or list/diagram surfaces, so citation-backed `items[]` in sections/summaries/tables could look label-only even though the visible answer was grounded. | Depth audit now treats citation-backed structured items in summary/section/scalar/decision/list/table blocks as anchored when the item label/text/cells or block text carry identifier/code surface. This is structural, not prose-keyword matching. Guards cover section and table surfaces. Remaining: reviewer LLM reasoning can still report tiny label-only deltas as low-impact telemetry; keep it out of control flow unless a precise deterministic citation defect exists. |
| E20260521-G151 | `read_combo_analyze_retry_anchor-20260521-154724` / reruns `read_combo_analyze_retry_anchor-20260521-155947`, `read_combo_analyze_retry_anchor-20260521-161955`, `read_combo_analyze_retry_anchor-20260521-164441` | P1 | Partially mitigated; residual support-block leakage tracked in G154 | The earlier passing answer still rendered a system-added `关键锚点` block because required mechanism anchors were matched only by exact structured label. A model-authored label such as `StageOutput.AnalysisIR` did not satisfy the separate required anchors `StageOutput` and `AnalysisIR`, and `EmitAnalysis` did not satisfy tool-name anchor `emit_analysis`. Two reruns removed the extra `关键锚点` surface, but the latest replay shows a localized `关键锚点` block can still be inserted by a different path after deterministic mechanism-anchor repair / scalar aggregate advisory. | Required-anchor completion was too literal and turned already-covered typed structured labels into visible support补块. The first fix correctly avoided one literal-matching path, but system-added anchor carriers still have more than one entry point; closing only one path is fragile. The fix must keep operating on typed structured labels/items and must not inspect user/model prose or use keywords. | Implemented structured anchor equivalence: typed labels/titles/endpoints now contribute exact keys, compact code-identifier keys (`emit_analysis` ~= `EmitAnalysis`), and one-level qualified owner/member keys (`StageOutput.AnalysisIR` covers both parts). Added unit guards for qualified labels and tool-name variants. Remaining work is separated as G154: make all system-added key-anchor carriers support-only unless the user explicitly requested them or a precise typed visible anchor block is required and exact citations are available. |
| E20260521-G152 | `read_combo_analyze_retry_anchor-20260521-155947` user-visible render / rerun `read_combo_analyze_retry_anchor-20260521-161955` | P1 | Mitigated and replay-verified | The REPL showed duplicate reviewer progress lines: `检查答案是否前后一致`, `正在审阅答案完整性`, then the same two lines again. The finalizer did not rewrite; the duplicates came from deterministic answer-document auto-repair re-running the full `runContractCheck`, which re-dispatched LLM reviewers after the first pass had already reviewed the same answer surface. | Deterministic carrier/metadata repair and LLM answer review were coupled inside `runContractCheck`. A cheap structural re-check after auto-repair inherited expensive reviewer side effects and repeated UI status, even when no strict deterministic gate had blocked reviewers on the first pass. | Added a `contractCheckSkipLLMReview` option. Auto-repair re-checks skip LLM reviewers only when the first pass already passed strict policy (meaning reviewers were eligible to run on the first surface); if the first pass failed a strict deterministic gate, reviewers still run after repair. Unit guard pins that the skip path emits no reviewer start notices. Rerun `read_combo_analyze_retry_anchor-20260521-161955` PASSed and showed exactly one `检查答案是否前后一致` plus one `正在审阅答案完整性`, with `finalizer_iters=1` and no成文打回. |
| E20260521-G153 | `read_combo_analyze_retry_anchor-20260521-163646` / rerun `read_combo_analyze_retry_anchor-20260521-164441` | P1 | Mitigated and replay-verified | The pre-fix run had two explorer completion rejects: `emit_investigation_complete rejected: aggregate_facts[0]: value is required`. The model emitted `bucket_count` facts with complete `members[]` and `unit`, but omitted `value`; this is a small structured JSON omission, not a semantic failure. The rerun had no `value is required` / `aggregate_facts[...]` rejection and still completed in one finalizer round. | `member_set.value` already had schema-aware repair from `members`, but `grouped_count` / `bucket_count` were treated like `total_count` even when their own `members[]` clearly carried the exact bucket/group set. This forced the model to spend another turn discovering an internal schema invariant. | Added a narrow cardinality repair: `grouped_count` and `bucket_count` may derive missing `value` from their own model-authored `members[]`. `total_count` and `unique_count` still require explicit values because their members can be examples rather than the complete set. Updated tool schema text and added unit/tool guards. Rerun `read_combo_analyze_retry_anchor-20260521-164441` passed with `tool_read_file=26`, `midloop_inject=13`, `explorer_iters=24`, `finalizer_iters=1`, and no schema reject. |
| E20260521-G154 | `read_combo_analyze_retry_anchor-20260521-164441` / rerun `read_combo_analyze_retry_anchor-20260521-170606` | P1 | Mitigated and replay-verified | The pre-fix final answer was correct and the English title was gone (`关键锚点` localized), but a system-added key-anchor block still appeared after the main answer. Some rows reused weak or drifted citations, e.g. `dynamicAnalyzeRetries` shared the `runAnalyzePhase` budget-call line and `AnalysisIR` shared the `runTaskGraph` nil-IR line. The rerun has no `关键锚点` / `Key anchors` surface and still finishes in one finalizer round. | There were still multiple deterministic support-block insertion paths. Localizing the title solved UX language, but not the deeper issue: a support scaffold could become visible principal-looking content and add citation drift even when the model's main prose already answered the question. | Required-mechanism-anchor normalization now only creates a new visible key-anchor block for the typed generic multi-topic skeleton case where `RequiresAnchorSkeleton` is true. Architecture/mechanism answers keep missing anchors as soft telemetry unless the model already created an explicit anchor block, in which case the system can still fill that model-owned carrier. This keeps support anchors out of the user-visible surface without using user/model prose keyword scans. Guard: `TestNormalizeRequiredMechanismAnchorCarriers_DoesNotCreateBlockForArchitecture`. |
| E20260521-G155 | `read_combo_analyze_retry_anchor-20260521-172008` / rerun `read_combo_analyze_retry_anchor-20260521-172824` | P2 | Mitigated and guarded; convergence follow-up open | Evidence repair feedback could collapse a recovered citation onto the adjusted definition line and hide the model's originally claimed line. Example: a row claimed one line as the proof while grounding recovered the symbol definition at another line; the tool then made the repaired target look like only the adjusted line mattered, encouraging duplicate or confusing re-emits. | The grounder was doing the right thing by recovering a stronger symbol definition, but the repair contract was under-specified for line drift. In mixed mechanism questions, the intended proof can legitimately be either the claimed statement line or the recovered definition line. Showing only one side makes the model fight the validator. | Recovered-line feedback now explicitly tells the model to choose: if the claimed line is intended, re-emit with a line-local anchor visible there; if the recovered definition is intended, cite the adjusted line. Structured repair targets now preserve both original and adjusted lines. Guards pin both the user-visible repair text and the typed target list. Rerun passed with `finalizer_iters=1`, `semantic_quality_concerns=0`, no成文返工. Remaining cost (`explorer_iters=30`, `midloop_inject=15`) is mostly exact-anchor exploration and should feed the broader parallel-convergence track rather than weakening grounding safety. |
| E20260521-G156 | `read_combo_analyze_retry_anchor-20260521-172824` / replay `read_combo_analyze_retry_anchor-20260521-174144` | P2 | Mitigated and guarded; replay exposed G157 | The same replay showed avoidable repair pressure for line-local statement anchors: the model cited the correct `for attempt := 0; attempt < max; {` line and pasted the exact snippet / condition, but used a descriptive `anchor_symbol` such as `for loop`. Tier-1 refused the row, snippet_fuzzy recovered it, and mid-loop forced another read/re-emit cycle. | For condition / return / assignment / initializer anchors, the proof is often the statement text itself rather than a durable symbol. Requiring only `anchor_symbol` as the Tier-1 carrier makes the system over-focus on model metadata and ignore already-structured source truth (`snippet`, `condition`). | Grounding now lets line-local anchors Tier-1 ground when exact snippet or structured condition text matches the already-read source line; free-form `summary` is deliberately ignored. Tool docs now tell models to use a visible token for `anchor_symbol` and provide exact snippet / structured fields when no durable symbol exists. Guards prove exact snippet succeeds and summary-only does not bypass grounding. Replay passed but did not close the case because it exposed the broader bucket-inference issue tracked as G157. |
| E20260521-G157 | `read_combo_analyze_retry_anchor-20260521-174144` / rerun `read_combo_analyze_retry_anchor-20260521-175636` | P1 | Mitigated and replay-verified; follow-up G158 | After G156, the replay still passed but the final surface degraded: analyzer emitted a mechanism explanation, while `QuestionStructure().Buckets` inferred six comparison buckets from verbatim mechanism participants (`runTaskGraph`, `MaxRetriesPerStage`, `dynamicAnalyzeRetries`, `StageOutput`, `Error`, `AnalysisIR`). The view became `family=comparison`, the prompt required six section blocks, deterministic row normalization appended multiple “系统按已验证证据补充成员” tables, and a generic coverage caveat appeared despite no high-confidence semantic-quality concern. | The entity-bucket fallback was too broad. It treated `is_cross_component + multiple sub_topics + verbatim MentionedEntities` as enough to prove an A/B comparison, even though `SubTopics` are investigator decomposition and mechanism participants are not user-named answer partitions. | Entity-derived comparison buckets now require a one-to-one topic shape: every inferred entity bucket maps to exactly one sub-topic and every sub-topic maps to exactly one candidate. Multi-entity mechanism explanations with `condition/call/register` axes stay `QFGeneric` unless the analyzer explicitly emits buckets. Guards cover the regression and preserve the real codrax/opencode two-side comparison fallback. Rerun stayed `family=generic` with no comparison sections. |
| E20260521-G158 | `read_combo_analyze_retry_anchor-20260521-175636` / replay `read_combo_analyze_retry_anchor-20260521-180505` | P1 | Mitigated for member_set demotion; replay exposed G159 | After G157, the run no longer became comparison, but finalization still received `Principal Enumeration Rows` from a model-emitted `aggregate_facts.member_set` describing four retry exits. The final answer already explained those exits in prose, then rendered the same set again as a second ordered list, including drifted citations such as an auto-correct row cited to the runtime default test. Eval PASS hid the UX/citation defect. | `member_set` had only two coarse roles: principal answer vs support. In single-topic mechanism explanations, explorers can legitimately use `member_set` as a structured outline so rich closure facts are preserved, but the deterministic enum compiler/prompt then treated it as a closed user-requested answer set and added hard row-surface pressure. | Single-topic mechanism explanations now demote `member_set` aggregate facts to `supporting_coverage` unless the typed request declares a real principal set boundary (`RequiresExhaustiveEnumerationMemberSetHandoff`, relation handoff, change impact, or explicit declared count). This preserves the structured handoff for finalizer context while preventing that aggregate from becoming `Principal Enumeration Rows`. Guards cover source-level role normalization and answer-document row compilation. The first replay still routed `family=enumeration` due a broader completeness-obligation issue, now tracked as G159. |
| E20260521-G159 | `read_combo_analyze_retry_anchor-20260521-181609` / rerun `read_combo_analyze_retry_anchor-20260521-183444` | P1 | Mitigated and replay-verified | Even after G158 demoted mechanism `member_set`, the analyzer's verbatim `CompletenessObligation` for "必须说明 MaxRetriesPerStage、dynamicAnalyzeRetries、StageOutput/Error 的关系" still made `QuestionStructure().HasAnyObligation()` true. `ResolveQuestionFamily` therefore routed a single mechanism explanation as `family=enumeration`; extractor saw an answer-symbol obligation and finalizer got `Principal Enumeration Rows` / system补表 pressure. | The system conflated two typed meanings of completeness: (1) a closed principal answer set ("list all X") and (2) coverage requirements inside one mechanism narrative ("explain A, B, and their relationship"). Treating both as hard set boundaries violates the "precise signal for hard gates" red line and lets system intent override the user's explanation intent. The extractor static skill also still said `sub_topics ≥ 1` activates `emit_answer_symbol`, contradicting the dynamic typed gate. | Added `CompletenessObligationIsMechanismCoverageOnly` and `HasPrincipalAnswerSetObligation`. Family routing now uses the latter, so coverage-only mechanism obligations preserve the obligation for finalizer context but do not activate enumeration family, answer-symbol slates, or deterministic row compilers. The extract skill now says `emit_answer_symbol` is required only for true enumerations, explicit `Anchor skeleton` blocks, or requested set boundaries; analyzer `sub_topics` alone are guidance. Guard tests cover request traits, family routing, extractor prompt/needsAnswerSymbols, and extract-skill wording. Rerun `read_combo_analyze_retry_anchor-20260521-183444` PASSed with `family=generic`, no `emit_answer_symbol`, no `Principal Enumeration Rows`, no system补表, `finalizer_iters=1`, and `enumeration_push=0` after fixing the eval metric to count visible Principal rows instead of a fixed finalizer prompt heading. |
| E20260521-G160 | `qf_architecture-20260521-184408`; reruns through `qf_architecture-20260521-194551` | P1 | Mitigated and replay-verified; schema-lane follow-up remains | Architecture replays originally mixed 6 `Stage*` constants with 6 `Agent*` constants and rendered system补表 blocks after an otherwise good stage explanation. The first fix removed Stage/Agent cross-expansion but exposed grouped-fact duplicates; later replays exposed sidecar `agent` / `orchestrator function` member_sets trying to become visible principal tables. Latest replay PASSes with `finalizer_iters=1`, `semantic_quality_concerns=0`, `enumeration_push=0`, and no visible `系统按已验证证据补充成员` block. | Root had three layers: (1) aggregate reconciliation expanded a `PipelineStage` member_set using only coarse repomap role (`const`) + exported status + required-file scope, so `Stage*` and `Agent*` looked like one class; (2) visible-coverage treated enum constant names and enum values as unrelated surfaces; (3) architecture/logical-view sidecar member_sets were still trusted as closed principal answer sets when they should remain rich support context. | Implemented declaration-family gating, visible-surface coverage for decorated/code-token aliases, and source-level role normalization for architecture narrative member_sets. Architecture/logical-view aggregate member_sets now stay `supporting_coverage` unless the typed request declares a real closed-set obligation; pre-complete can still use raw principal sets to prove exploration sufficiency, but finalizer/row compiler no longer turns sidecar agents/functions into dry补表. Guards cover stage-vs-agent constants, same-family stage expansion, visible architecture coverage, architecture narrative demotion, and import enumeration preservation (`qf_imports-20260521-192240`). Follow-up remains: explicit `member_scope` / `answer_entity_type` schema lanes for cross-language non-declaration evidence. |
| E20260521-G161 | `qf_architecture-20260521-191530`; reruns `qf_architecture-20260521-194228`, `qf_architecture-20260521-194551` | P1 | Mitigated and replay-verified | After G160, the answer no longer rendered the worst system补表, but some visible list items still cited adjacent support anchors: e.g. `StageAnalyze` could render with `internal/agent/explorer.go:68` while exact `StageAnalyze` evidence existed at `internal/types/enums.go:26`. A later replay also showed sidecar aggregate member_sets trying to reintroduce agent/function supplements. | Citation repair was too permissive for code-identity list labels after compat recovery, and aggregate role normalization still lacked an architecture-narrative support boundary. A support/implementation citation could remain attached to a principal item even when a grounded exact-definition evidence row for that same label existed; sidecar member_sets could still become deterministic visible tables. | Added exact-definition citation preference for visible definition/enumeration/code-path items: a unique grounded exact endpoint definition/direct anchor outranks adjacent support citations; ambiguous exact anchors are left untouched. Added architecture-narrative member_set demotion using typed signals (diagram hint, multiple subtopics, or cross-component), so sidecar sets enrich finalizer context without creating hard Principal Enumeration Rows. Guards cover exact-definition rebinding, ambiguous same-name definitions, architecture member_set demotion, and qf replay. Rerun `qf_architecture-20260521-194551` PASSed with exact `internal/types/enums.go:{15,24,26,27,28,29}` citations, no成文返工, and no system补表. |

## High-ROI Remediation Queue

This queue groups the 141 findings into fixes that should retire whole classes
of failures. Prefer these over isolated case patches. If a later product commit
already addresses part of an item, keep the item as a verification target until
the eval cases prove the class is closed.

| Priority | Track | Covers | Why high ROI | Done When |
| --- | --- | --- | --- | --- |
| P0 | Deterministic scalar / measurement / VCS lanes | G77-G78, G82-G83, G114-G116, G142, plus scalar fallout in G71/G89/G92 | Wrong scalar answers are high-trust failures and often waste many explorer turns. A typed `{command, scope, value, provenance}` lane plus bounded VCS set-intersection support should fix LOC/count/history cases and remove model arithmetic from the critical path, while keeping VCS metadata from collapsing non-scalar history answers. | Count/history evals (`s7a`, `s7b`, `u7b`, `u7c`) produce command-backed answers with no manual arithmetic, no blank member tables, no fake code-symbol citations, and no generic `值：` scalar compression when the user asked for a feature summary/list/comparison. |
| P0 | Row compiler / system补表 safety contract | G11, G13, G16, G18, G20, G25-G28, G35, G79-G80, G83, G116, G131, G135, G141 | The compiler can visibly pollute otherwise good answers, create contradictory counts, empty tables, wrong bucket members, and duplicate surfaces. One renderer/compiler contract can eliminate a large share of unacceptable PASS cases. | System-generated blocks are explicitly marked, never empty, never contradict model-authored principal rows, never override a complete model table, and are bucket/scope keyed. Existing model-authored Markdown/tables stay intact. |
| P0 | Reviewer/render surface alignment and contradiction enforcement | G1, G11, G15, G21-G22, G71, G75-G76, G79, G92, G121, G125, G132 | Reviewer false negatives/positives currently cause both bad publishes and wasted retries. Feeding reviewers the exact post-normalized rendered surface and making high-confidence contradiction repair deterministic should improve many families at once. | Reviewers consume structured rendered blocks/tables; high-confidence contradictions either mutate/suppress the offending system block or fail the run; generic supplements are suppressed when semantic sufficiency is above floor and no precise visible defect remains. |
| P0 | External artifact / runtime trace first-class route | G2, G4, G6-G10, G126-G129, G134-G141 | Log/Hilog/HiTrace cases repeatedly conflate artifact frames with repo source, over-explore current code, and leak artifact paths into repo citations. A first-class artifact evidence lane is broad, user-visible, and reduces runtime. | `resolved_files=0` answers use artifact-frame citations/lanes, skip repo exploration unless requested, reject or normalize repo citation leakage, preserve innermost frames, and do not require current-code facets/citation floors. |
| P1 | Schema-aware compatibility repair without lossy recovery | G5, G8-G9, G12, G14, G67-G68, G72, G74, G87, G119-G120, G136-G137 | Many retries are caused by near-miss fields or schema shapes that are safely repairable; the dangerous part is lossy recovery that silently drops model content. | Safe repairs are typed and lossless; unknown/near-miss fields with obvious mapping are normalized before retry; unrecoverable JSON-string payloads fail instead of publishing thin recovered answers. |
| P1 | Explorer convergence and authoritative-closure routing | G2-G3, G12, G23, G27, G65/G81/G87, G90, G115, G133 | Long-running cases dominate latency. Most tails come from requiring every grep/repo_map candidate after an authoritative source was already found. | Exploration closes on typed authoritative sources, uses do-not-retry convergence signals, records reliable dispatch/iteration metrics, and keeps broad grep candidates soft unless the answer family requires exhaustive verification. |
| P1 | Multi-repo bucket / scope isolation | G16-G21, G93, plus `read_combo_multirepo_negative_interaction` notes | Cross-repo contamination creates contradictory, highly visible answers. The fix is structural: principal keys must include repo/bucket/scope identity. | A row from repo A cannot count/render as repo B's principal member; inactive/negative scope answers use negative-search facets instead of positive definition facets; parent-graph fallback is explicit telemetry only. |
| P1 | Exact target / citation binding and stronger-evidence selection | G24, G66, G70, G76, G84-G86, G91, G93, G97-G100, G130, G144 | Many answers are mostly right but cite adjacent or weaker lines, drop the best definition anchor, promote same-family false positives, or bind a document-level absence verdict to an unrelated present value. This is a cross-family trust issue. | Principal claims cite matching exact anchors; visible file:line text resolves to a citation; definition/assignment hits outrank package metadata or nearby call sites; same-family context cannot become principal for exact-target asks; mixed absent/present answers bind every scalar or absence claim to the exact target it describes. |
| P1 | Analyzer intent/schema normalization for direct questions | G5, G68, G72, G88, G90, G118, G122-G123 | Analyzer retries and over-expanded subtopics slow simple direct questions and sometimes alter user intent. Typed normalization is cheaper and safer than reclassification loops. | Role-locate/error-granularity/direct mechanism questions classify in one pass where fields are inferable; subtopics/focuses come from explicit user asks; decorator/marker inventories do not fail subtopic coherence because marker tokens differ from file buckets. |
| P2 | Language inventory adapters and decorator/package metadata lanes | G122-G125, G130-G133 | ArkTS/Cangjie failures show supported repomap languages are not fully supported end-to-end in search, decorator preservation, and package-line rendering. | Every repomap-supported language maps to valid search extensions; decorator stacks and package/module attributes are typed table columns with citations or no line-number claims. |

2026-05-20 implementation note:

- First reviewer/render alignment slice is implemented in `contract_check.go`:
  post-emit reviewers now see V2 block titles, structured table columns, and
  structured item cells after deterministic normalization. This directly
  reduces the G15/G125/G132 class where reviewers judged a partial body view
  instead of the final visible answer. It is not the whole P0 track:
  section-scoped table ownership, high-confidence contradiction enforcement,
  and generic-supplement suppression remain open.
- First generic-supplement suppression slice is also implemented: if semantic
  quality review returns `sufficient=true` at or above the configured
  confidence floor, only low-precision soft coverage/prose/facet caveats from
  the same accepted draft are suppressed. Precise obligations (`must_include`,
  citation failures, contradictions, success criteria, and operator-promoted
  strict kinds) remain untouched. This targets G30/G71/G121-style “reviewer
  says sufficient but final panel still adds generic补充说明” without weakening
  hard safety gates.
- The existing deterministic补表 safety invariants have been reverse-audited
  against code and unit tests:补表 is separate/localized, model-authored
  Markdown is preserved, empty generated cells are skipped/omitted, scalar
  history/count support ledgers do not become principal tables, and singleton
  count-basis metadata is not rendered as an answer row. Remaining row-compiler
  work in this ledger is eval-driven scoping/ranking (multi-repo buckets,
  config precedence, runtime artifacts), not lack of the base safety primitive.
- Reviewer/retry routing now consumes the same claim-binding surface as the
  finalizer prompt. Semantic quality review receives origin/policy/output rows,
  and strict-promoted generic coverage/support-lane signals are suppressed from
  finalizer retry when the active principal claim is non-current-source,
  non-exact-output narrative support. This directly targets VCS feature-summary
  and artifact/measurement support cases where system gates previously tried to
  re-prove a narrative answer with current-source shape. Precise defects
  (citations, must-include, contradictions, success criteria, exact scalar/count
  outputs, and requested diagram/table/list omissions) remain retry-eligible.

### Batch 2 — Row Compiler / System Supplement Safety Contract

Status: first bottom-layer guard in progress after G83/G116/G131/G135/G141
showed that system-generated tables can visibly degrade otherwise usable
answers.

Safety contract:

- System-generated supplement tables must never introduce blank generated
  cells. If a generated column such as location or notes is required by the
  table shape, every generated row that remains in that table must have that
  value.
- If repairing a model-emitted empty structured table would create blank cells,
  the repair is skipped and the model-authored surface stays untouched.
- This guard is intentionally local and conservative: it does not parse user
  prose, does not override model-authored Markdown tables, and does not promote
  support metadata into the principal answer. It only prevents unsafe
  deterministic additions.

2026-05-20 validation:

- Added unit coverage for mixed complete/incomplete deterministic rows in both
  `normalizePrincipalEnumerationRowBlocks` and
  `compileEnumerationDisplayTableRows`.
- `go test ./internal/tool ./internal/render ./internal/types` PASS.
- `s7b` PASS in `eval/results/s7b-20260520-121256`: final answer renders the
  scalar `25` without appending the previous mostly-empty Kind-member table.
  Remaining unrelated gap: exploration still spends 19 iterations because the
  deterministic count-proof closure is not recognized early enough.

### Batch 3 — Scalar Count Aggregate Handoff Compatibility

Status: completed for G82; committed in the current high-ROI batch after the
`s7b` rerun proved that the answer was correct but the explorer was still being
misled by the closure contract.

Safety contract:

- For questions typed by the analyzer as `is_count_question`, a model-authored
  `aggregate_facts[].kind="scalar_value"` with an integer `value` is a valid
  exact count handoff. The model does not need to know the system-internal
  `dimensions.answer_axis=count` marker.
- This compatibility is deliberately narrow: supporting/audit scalar facts do
  not close the investigation, and multiple principal/unknown scalar facts with
  different integer values remain ambiguous and still require a clearer typed
  handoff.
- The fix does not read user prose or model free-form text. It only consumes
  schema-validated aggregate facts after the analyzer has already marked the
  request as a count question.

2026-05-20 validation:

- Added regression coverage for `scalar_value` clearing the count downgrade
  even when earlier broad `exec_command` counts conflict with the scoped answer.
- Added the inverse guard: conflicting scalar aggregate candidates do not clear
  the deterministic-count gate.
- `go test ./internal/tool` PASS.
- `s7b` PASS in `eval/results/s7b-20260520-122126`: no
  `deterministic count proof is missing` downgrade, explorer iterations
  improved from 19 to 8, mid-loop injections from 21 to 1, and the final answer
  stayed scalar/prose without system-added empty member tables.

### Batch 4 — Scalar Count Scope Windows Are Not Answer Member Sets

Status: completed for the u7b extraction/finalizer loop. VCS convergence was
still open at this checkpoint and is addressed by Batch 5 below.

Safety contract:

- If analysis has already typed the request as `is_count_question=true` and
  `is_scalar_answer=true`, an emitted `enumeration_boundary` is treated as a
  contextual scope/window count, not as the principal answer member set. The
  boundary is soft-stripped before the RequestModel is persisted.
- True bounded-list/mechanism questions keep their boundary. For example, a
  non-scalar mechanism request such as “the 7 checks” still persists
  `declared_count=7` and can drive downstream member/step obligations.
- The rule is typed-only: it reads the schema predicates, not localized prose or
  model thinking. This prevents “last 20 commits”, “top 5 files considered”,
  and similar scope windows from forcing `emit_answer_symbol` or finalizer
  ordered-list/table requirements when the user explicitly asked for a number.

2026-05-20 validation:

- Added unit coverage proving a scalar history count strips
  `enumeration_boundary`, while the existing bounded mechanism test still
  persists `boundary=7`.
- `go test ./internal/tool` PASS.
- `u7b` PASS in `eval/results/u7b-20260520-123245`: no extractor retry for
  missing `emit_answer_symbol`, no 20-item commit slate request, no
  “枚举完整性不足” supplement, and no blank system commit table. Remaining
  issue at this checkpoint: explorer still spent 22 iterations on manual VCS
  intersection; Batch 5 addresses that G115 tail.

### Batch 5 — Deterministic VCS History Count Tool

Status: completed for the high-ROI G114/G115 path. This batch is deliberately
scoped to history-count questions, not to narrative history explanations such as
“最近一次合入的是什么特性？” or “找到合入并解释相关代码/画图”.

Safety contract:

- `git_history_search` is a read-only exploration tool for bounded VCS count
  questions. It collects the recent commit window from `window_path`, inspects
  each commit diff under `diff_path` (defaulting to the same path), and returns
  `answer_count=<n>` plus matched commit subjects for a fixed `contains`
  surface.
- The tool accepts `./...` and absolute paths under the repo root by
  normalizing them to repo-relative pathspecs. It fails loudly if
  `window_count` exceeds the configured maximum instead of silently truncating
  the user's requested window.
- Closure enrichment preserves provenance as
  `proof_source=git_history_search`, `answer_axis=count`, and
  `measurement_kind=vcs_history_count`. This lets extractor/finalizer consume a
  scalar history count without pretending it came from repo file:line evidence
  or ordinary shell arithmetic.
- The analyzer/history-shape fix from B1.7 still owns non-scalar history
  questions. `git_history_search` is only a deterministic proof lane for
  count/scalar history tasks; it does not compress feature summaries,
  commit-list comparisons, diagnostics, or commit-to-code diagrams into
  `**值：**`.

2026-05-20 validation:

- Added unit coverage for the tool's bounded diff search, absolute/`./`
  pathspec normalization, oversize-window fail-loud behavior, and closure
  acceptance with `proof_source=git_history_search`.
- `go test ./internal/tool ./internal/skill` PASS.
- `go test ./internal/agent -run 'Test.*(ToolSchemas|ToolSuggestions|Parallel|RuntimeBlocks|Capability)'` PASS.
- `make` PASS.
- `u7b` PASS in `eval/results/u7b-20260520-125017`: the explorer used
  `git_history_search` in turn 1, converged in 3 explorer iterations / 1
  dispatch, had 0 mid-loop injections, 0 `read_file`, finalizer 1 iteration,
  and rendered the correct scalar `0` with VCS provenance in the summary.

### Batch 6 — Lossy Answer-Document Recovery Must Fail Loud

Status: completed as a guard for G8/G49/G54/G67-style JSON-string payload
compatibility. This batch protects the answer surface from silent data loss; it
does not try to force a model rewrite for cosmetic or advisory defects.

Safety contract:

- Lossless JSON-string recovery remains accepted. If the model stringifies
  `blocks[]` but every block and sibling field can be parsed as native JSON,
  the tool repairs it locally and proceeds.
- Heuristic brace-balanced recovery may still preserve malformed Mermaid/diagram
  payloads as display attachments, because REPL rendering can show them without
  feeding altered content back into model memory.
- If a visible answer block marker is present but cannot be recovered as either
  a structured block or a preserved display attachment, the tool call fails
  before pre-emit advisory softening or renderer normalization. The model must
  re-emit a complete native JSON `blocks[]` array; the partial recovered
  document is not published.
- This is a structural JSON/field-preservation guard. It does not inspect user
  prose or infer answer quality from model free text.

2026-05-20 validation:

- Added regression coverage where brace-balanced recovery can recover a summary
  block but drops a malformed ordered-list block. The tool now rejects that
  partial recovery and does not persist the partial document.
- Existing recovered-diagram behavior remains covered: a malformed Mermaid
  diagram can still be preserved as a display attachment when the visible
  content is not lost.
- `go test ./internal/tool` PASS.
- `logtri_rust` PASS in `eval/results/logtri_rust-20260520-130133`: no
  finalizer retry, no JSON-string recovery event in this run, and no semantic
  concerns. Because the latest model output did not reproduce the original
  malformed payload, the G8 closure is guarded primarily by the targeted unit
  test.

### Batch 7 — Observation-Only Runtime Caveats Stay Artifact-Scoped

Status: completed for the generic-supplement leak found after the first
external-log fixes. This batch is intentionally narrow: it changes accepted-path
soft-caveat materialization and analyzer typed reconciliation, not the broader
artifact fast path.

Safety contract:

- Observation-only runtime answers must not inherit generic source-code caveats
  such as “coverage may be insufficient” or “check against source code” when a
  typed runtime disposition already says `resolved_files=0`.
- If a real user-facing limitation remains, render a precise localized boundary
  that distinguishes artifact frame locations from current-repo source
  citations. Do not expose internal violation keys or generic reviewer text.
- Explanation-shaped diagnostic requests may clear stale scalar
  `answer_subject.kind` before the analyzer quality gate only when typed
  predicates already prove the request is not scalar/count/role-locate. Scalar
  and return-value routes remain untouched.
- The late orchestrator R2.2 auto-correction remains as a fallback, but the
  normal path should not show a red analyzer failure for a schema-consistent
  diagnostic explanation.

2026-05-20 validation:

- Added `TestAppendSoftContractCaveatsToAnswerForBus_ObservationOnlyUsesBoundaryCaveat`.
- Added `TestBuildAnalysisIR_DiagnosticExplanationClearsScalarAnswerSubjectBeforeGate`.
- `go test ./internal/orchestrator ./internal/agent ./internal/tool ./internal/types` PASS.
- `make` PASS.
- `logtri_rust` PASS in `eval/results/logtri_rust-20260520-133532`: the final
  answer no longer includes the generic “结合源码进一步核对” supplement; the
  only remaining issue in that run was late R2.2 auto-correction after an
  analyzer hard failure.
- `logtri_rust` PASS in `eval/results/logtri_rust-20260520-134313`: no analyzer
  hard failure, no `shape_subject_coherence` retry, no repo reads in explorer,
  no generic supplement. Residual tail: one advisory mid-loop nudge and the
  semantic/self-consistency reviewers still run even though the structural
  contract is quiet; that cost stays under G10 / artifact fast-path work.
- Follow-up `logtri_rust` PASS in `eval/results/logtri_rust-20260520-135200`:
  semantic-quality dispatch dropped to 0 while self-consistency still ran as
  the contradiction guard. The run retained no repo reads and no generic
  supplement. Residual tail: the model again emitted malformed JSON-string
  `blocks[]`; the G8 lossless-recovery guard rejected it and the second emit
  succeeded.

### Batch 8 — Prevent String-Wrapped Answer Blocks at the Tool Contract

Status: implemented as a prevention layer for G8/G49/G54/G67-style first-emit
retries. This batch deliberately does not loosen recovery or publish partially
recovered answer surfaces.

Safety contract:

- `emit_answer_document.blocks` is now described in both the tool description
  and JSON schema as a native JSON array, not a JSON-encoded string containing
  escaped block JSON.
- `answer-document-skill` repeats the same requirement in the finalizer's
  workflow and block-contract sections, so the model sees the rule before the
  first emit rather than only after a rejection.
- The existing lossy-recovery guard remains authoritative: lossless
  string-array recovery can still proceed; any recovery that cannot preserve
  every visible block must fail before the partial document reaches the
  renderer.

2026-05-20 validation:

- Added schema/prompt regression tests so future prompt edits cannot drop the
  native-array guidance.

### Batch 9 — Skip LLM Reviewers for Pure Observation-Only Artifact Answers

Status: implemented after `logtri_rust-20260520-135944` exposed a reviewer
false positive and a 53s post-emit tail.

Safety contract:

- Applies only when the typed request model says the attached runtime artifact
  is observation-only for the current checkout: external-only log/trace and no
  current-status diagnostic request.
- The answer must already carry the `observed_artifact_fact` +
  `external_observation` typed carrier, declare no facet outside
  `observed_artifact_fact` / `uncertainty_boundary`, have zero current-repo
  citations, and have no diagram request.
- If the answer cites current repo code, declares current-code facets, or the
  user requested a diagram, the LLM reviewers remain available.
- Rationale: artifact frame paths are concrete runtime observations but not
  current-repo source citations. Generic self-consistency review can misread
  that intended provenance distinction as a contradiction; semantic review can
  spend a full LLM round chasing artifact-anchor cardinality that deterministic
  gates already accepted.

2026-05-20 validation:

- Extended the observation-only artifact reviewer skip unit test to cover the
  shared gate for both semantic-quality and self-consistency reviewers, including
  a normal self-consistency-eligible answer surface.
- The same test pins the non-skip cases: repo-style citations, current-code
  facets, and explicit diagram requests.
- `logtri_rust` PASS in `eval/results/logtri_rust-20260520-140801`:
  `finalizer_iters=1`, `semantic_quality_dispatches=0`,
  `strict_decode_remap_events=0`, no “检测到前后不一致，正在重写答案” render line.
  Logs show both reviewers skipped via the shared observation-only artifact gate.

### Batch 10 — Optional Analyzer Role Binding Softening

Status: implemented after G72 showed an analyzer retry caused by an optional
positive `answer_role_profile` that lacked grounded `source_quotes[]`.

Safety contract:

- Positive role binding can change the answer shape, so it remains authoritative
  only when at least one `source_quote` is copied verbatim from the current user
  request.
- Missing or unanchored `source_quotes[]` now softens the optional profile:
  analysis still succeeds, a warning is surfaced in the tool summary/logs, and
  no downstream hard role gate is installed from that profile.
- This is intentionally not a prose keyword fallback. The system does not infer
  a replacement role from model prose or user text; it only decides whether the
  model's optional typed profile is grounded enough to persist.
- Anchored profiles are preserved, and invalid anchored role enum values remain
  hard rejects because they are precise schema errors that can otherwise corrupt
  downstream role-specific prompts.

2026-05-20 validation:

- Added analyzer unit coverage for the two softening cases: missing
  `source_quotes[]` and `source_quotes[]` not present verbatim in the current
  request.
- Existing anchored-profile coverage still proves grounded `answer_role_profile`
  values are persisted into the request model.
- `s3d` PASS in `eval/results/s3d-20260520-141918`: `analyzer_dispatches=1`,
  `analyzer_iters=3`, no analyzer retry / `answer_role_profile.source_quotes`
  rejection, and `finalizer_iters=1`.
- The same replay intentionally leaves G73/G74/G71 open: exact absence closure
  still required a second `emit_investigation_complete` because
  `absence_justification` was missing, a supporting `member_set` without
  `support_refs` caused another explorer retry, and a generic coverage
  supplement was still appended despite the answer passing. Those are next-batch
  absence and supplement-gating targets, not regressions from this softening.

### Batch 11 — Exact Absence Closure Compatibility

Status: implemented after the `s3d` Batch 10 replay showed two avoidable
explorer retries in an otherwise valid exact-absence answer.

Safety contract:

- `aggregate_facts.kind=negative_search` remains a repo-scoped zero-result
  search fact. It must keep `repo`, `query` or `pattern`, `scope`, and
  `searched_at` dimensions so multi-repo negative evidence cannot be mixed
  across active repositories.
- The `repo` requirement is not a universal rule for every negative conclusion.
  File-local negative evidence belongs in typed evidence (`scope=negative` /
  `negative_query.file`), external log/trace no-intersection facts need a
  runtime-artifact lane, VCS/history zero-intersection facts need a VCS typed
  lane, and behavioral outcomes need a `behavior_outcome` /
  `error_granularity_verdict` style aggregate rather than `negative_search`.
- When a repo-scoped `negative_search(value=0)` names every exact target and the
  exact-resolution contract allows absence, the tool can synthesize the
  deterministic `absence_justification` instead of forcing the model to repeat
  the same conclusion in another `emit_investigation_complete` turn.
- For exact-absence closures, a decorated `member_set` without `support_refs`
  is dropped only when the model explicitly marks it as `supporting_coverage` or
  `audit_ledger`. Principal and unknown-role member sets still obey the existing
  hard `support_refs` contract, so the system does not silently discard a
  possible user-facing answer set.

2026-05-20 validation:

- Added unit coverage for negative-search-driven absence justification
  synthesis, supporting member-set drop, and the guard that unknown-role
  decorated member sets still reject without `support_refs`.
- Re-ran `s3d` in `eval/results/s3d-20260520-143037`: PASS,
  `finalizer_iters=1`, `strict_decode_remap_events=0`, and no
  `emit_investigation_complete rejected` / `support_refs is empty` lines.
- Residual open issue: the final answer still appends the generic “覆盖度可能不充分”
  supplement after a substantively sufficient semantic review with confidence
  below floor. That remains G71 / reviewer-supplement gating, not an absence
  closure retry.

### Batch 12 — Optional Richness Telemetry Must Not Render Generic Caveats

Status: implemented after the Batch 11 `s3d` replay proved the remaining
generic supplement came from `ViolRichnessRegression`, not from a concrete
semantic reviewer defect.

Safety contract:

- `ViolRichnessRegression` is pure telemetry for CGEC / eval trend analysis.
  Its own producer says “telemetry only — answer ships unchanged,” and the
  retry profile already treats it as permanently soft and non-promotable.
- Optional-richness misses must therefore not append the generic
  answer-coverage caveat to accepted answers. If the system has a concrete
  user-facing limitation, it must come from a more precise signal:
  `ViolFacetUncovered`, `ViolRichnessGlaringGap`, `ViolAnswerSemanticUnderfilled`
  with a localized concern, artifact-boundary caveats, inactive-scope
  disclosures, etc.
- This fix does not suppress git/log/trace boundaries. Those are separate typed
  negative/provenance lanes: VCS history uses `git_history_search` /
  history-scoped aggregates, runtime logs/traces use observation-only artifact
  lanes, and repo search zero-results use `negative_search(repo, pattern/query,
  scope, searched_at)`.

2026-05-20 validation plan:

- Unit guard: `AppendSoftContractCaveatsToAnswer` leaves an accepted answer
  unchanged when the only violation is `ViolRichnessRegression`.
- Registry guard: `ViolRichnessRegression` is included in the operator-only
  no-caveat-family set.
- Replay `s3d` after tests: expected finalizer 1 turn, no
  `覆盖度可能不充分` supplement from optional-richness telemetry.

### Batch 13 — Mixed Exact-Target Absence Needs Target Binding

Status: short-term guard added; long-term schema work tracked as P2 (G144).

Safety contract:

- Single-target exact absence may suppress scalar blocks that would otherwise
  render a nearby/sibling value as the absent target's value.
- Multi-target answers are different: if the run asks about both an absent
  target and a present target, the system must keep the present target's value
  and provenance visible. A document-level absent verdict is not enough to
  delete every scalar block.
- Long-term architecture target: exact-resolution, VCS history facts,
  negative-search facts, and runtime artifact facts should all bind positive or
  negative claims to their own typed target/scope. The preferred shape is
  per-target `exact_resolution[]` or block/claim-level `target_ref`, not prose
  matching and not whole-document inference.

2026-05-20 validation:

- Unit coverage:
  `TestNormalizeViewCompatibleAnswerDocument_DropsScalarWhenExactResolutionAbsent`
  guards the single-target exact-absence repair, while
  `TestNormalizeViewCompatibleAnswerDocument_KeepsScalarWhenAbsentExactResolutionHasMultipleTargets`
  guards the mixed-target boundary.
- Added medium-priority eval case
  `read_combo_config_absent_present_mix` so future broad scalar/absence
  normalizers cannot silently regress this shape.

### Batch 14 — Read-Mode Git Command Ergonomics And Repo Boundary

Status: implemented from customer snippet `../customlogs/git_diff_001.log`.

Safety contract:

- Read-mode `exec_command` runs from the active repository root. The model
  should not need absolute `cd`, `git -C`, `--git-dir`, or `--work-tree`
  guesses to inspect the current repo.
- Shell redirection is not all equally dangerous. Discarding output to the
  null device or duplicating descriptors is read-only; redirecting to arbitrary
  names such as `2>1` / `1>2` creates files and remains forbidden.
- Git global path options are command-scope path changes and must stay
  repo-relative / active-scope. If rejected, the tool result must tell the
  model what the current command root is and how to retry safely.
- VCS explanation questions should prefer structured git tools before free-form
  shell. The structured git tools' command-root parameters are repo-root scoped
  too. Free-form shell remains a deterministic fallback and already uses the
  blob pagination path for large output.
- Transient model/provider failures and semantic validation retries have
  different user-facing meanings. UI copy must not call a stream stall
  “validation not stable.”

Validation:

- Unit tests for `2>/dev/null`, `>/dev/null`, and `2>&1` allowed; `2>1` and
  `1>2` rejected.
- Unit tests for `git -C . log`, `git -C subdir log`, `git -C /tmp log`,
  `git --git-dir=/tmp/.git log`, and `GIT_DIR=/tmp/.git git log` path-scope
  behavior.
- Unit / tool tests for `git_diff(stat=true)` and `git_diff(name_only=true)`.
- Message test proving analyze transient retry uses the
  stage-aware “模型响应出错” copy.

### Batch 15 — VCS History Narrative Convergence And Mixed-Code Boundary

Status: implemented after `u7j-20260520-163512` showed a pure history topic
search spending 159 explorer rounds, 52 `read_file` calls, and 37 mid-loop
injections after a valid VCS/history conclusion was already available.

Safety contract:

- `is_history_lookup` is an evidence-source lane, not an answer-shape
  compressor. For non-scalar/non-count history narrative questions, commit/ref
  metadata, history search results, and diff/stat/name-only output are the
  principal evidence. Current source files are optional support unless typed
  analyzer/contract fields require current-code, diagram, diagnostic,
  comparison, relation, change-impact, or enumerated-list evidence.
- Pure history narratives may converge after a successful
  `emit_investigation_complete` even when the analyzer emitted multiple source
  subtopics. Those subtopics are search hints, not mandatory current-code
  coverage.
- Mixed history + current-source questions are protected explicitly:
  `DiagramHint`, required `AnswerContract.Diagram`, diagnostic/current-status
  profile, change-impact profile, relation/comparison/enumeration predicates,
  or structured user buckets keep the mixed lane active. In that lane VCS tools
  explain what changed and source evidence explains the current implementation;
  neither lane may erase the other.
- Explorer prompt now tells the model not to encode commit metadata as fake
  file:line `emit_evidence` rows. The narrative conclusion belongs in
  `emit_investigation_complete.reason`; aggregate facts remain supporting VCS
  metadata unless the typed request really asks for a scalar/count/list.

Validation:

- Added typed helper tests for pure history narrative, history scalar,
  history+diagram, history+change-impact, and history+current-diagnostic
  boundaries.
- Added parallel explorer tests proving pure history subtopics can cancel slow
  source siblings after convergence, while history+diagram/current-flow keeps
  sibling handoffs active.
- Added explorer prompt tests proving pure VCS narrative drops mandatory
  current-source RequiredFiles, and history+diagram preserves the mixed
  handoff.
- Added eval case `u7k` for “all-history topic search + current implementation
  explanation” so future fixes cannot solve pure history by breaking
  diff/current-code analysis.

2026-05-20 mixed diff/current follow-up:

- Design decision for “diff + 基于当前代码分析” questions: treat them as a
  two-lane answer, not as `history scalar`. VCS/diff tools prove what changed
  and when; current-source `read_file`/grounded evidence proves how the current
  implementation behaves now. A commit hash, file count, or subject line may be
  supporting metadata, but it cannot replace the requested explanation,
  comparison, trace, diagram, or log/diagnostic analysis.
- Boundary guard is typed-only. History-backed `trace/call_chain` requests
  preserve strong current-code gates only when the analyzer has explicit source
  and sink endpoints (`exact_targets` / exactly-bound mentioned entities). A
  loose phrase such as “解释这条链路怎么工作” remains an architecture/mechanism
  narrative and must not be forced into source-to-sink endpoint proof.
- Generic forced-read gates (`primary_anchor_unread`, `phase1_unread`,
  `multi_path_anchor`) now skip for history-backed current-code explanations.
  Those gates were designed for ordinary breadth mechanism questions; on
  diff/current questions they repeatedly pulled the explorer back into adjacent
  current-source files after VCS + current evidence was already sufficient.
  Explicit endpoint call-chain questions are not skipped.
- Optional aggregate facts from narrative explain/trace closures are now
  normalized item-by-item: invalid support-only facts are dropped with a
  localized normalization note instead of rejecting the whole closure. Hard
  aggregate contracts remain hard for count/scalar/enumeration/relation/
  diagnostic/change-impact shapes.
- VCS member sets now preserve decorated commit identifiers such as
  `916511... (最早)` without requiring source `file:line` support refs. The
  guard is intentionally narrow: it only applies when the analyzer typed the
  request as `is_history_lookup=true` and the decorated member base is a
  structural commit hash. Ordinary decorated code members still require
  `support_refs`, so the architectural-comparison citation alignment fix is not
  weakened.
- `u7k-20260520-193504` exposed a still-open architecture gap: a historical
  diff can mention a symbol name that no longer exists in the current checkout.
  The model correctly recognized the rename later, but the current-source
  evidence gate first tried to line-ground the old diff symbol. Long-term fix:
  add first-class VCS/diff evidence origin (`current_source` vs `vcs_diff` /
  `vcs_metadata`) so old hunk symbols can be preserved as historical facts
  without becoming current file:line obligations. This is recorded as a P1
  follow-up under the deterministic VCS lane rather than patched by keyword
  matching.

2026-05-20 validation / follow-up:

- `u7j-20260520-195708` PASS on the current branch with finalizer 1 turn and no
  decorated commit-hash `support_refs` rejection. Cost is still high
  (41 explorer iterations, 16 mid-loop injections, extractor 3 iterations);
  this confirms the commit metadata gate is fixed but pure history narrative
  convergence remains a P1 optimization target.
- `u7k-20260520-200331` was stopped after ~6 minutes because multiple explore
  forks had already called `emit_investigation_complete` and the dispatcher
  still launched / continued later sibling routes. This is a convergence
  policy problem, not a finalizer answer-shape problem. Short-term fix:
  distinguish pure history evolution (`kind=history`) from mixed
  history/current-code mechanism (`kind=mechanism`) and allow early convergence
  for the latter once one fork passes the `emit_investigation_complete`
  pre-complete checks. Cross-component, diagram, category enumeration, relation,
  diagnostic, and change-impact shapes still wait for sibling handoffs.

### Batch 1 — Deterministic Scalar / Measurement / VCS Guardrails

Status: first implementation committed as `91651164`; follow-up hardening in
progress after `s7a` exposed a residual scalar-support role leak.

Scope for this batch:

- B1.1 Safe read-mode counting pipelines: allow `xargs` only as a wrapper around
  already allowlisted read-only commands, so large-file count/measurement
  questions can use `find ... -print0 | xargs -0 wc -l` without falling back to
  model arithmetic. Mutating nested commands (`rm`, `git clean`, `sed -i`) and
  interactive `xargs -p` remain rejected.
- B1.2 Scalar/history count support separation: a `member_set` attached to a
  scalar count answer is corroborating evidence, not a principal answer table,
  even when the model marked it `principal_answer`. This prevents commit/file
  support ledgers from being rendered as system-generated member tables.
- B1.3 Strict VCS scalar lane: history/count questions may be completed from a
  deterministic `exec_command` only when the command provenance is VCS-shaped
  (`git log` / `git rev-list`) and the output explicitly labels the final
  result as `answer_count`, `intersection_count`, `filtered_count`,
  `result_count`, `vcs_count`, or `history_count`. Broad unlabeled
  `git log | wc -l` counts remain support-only and still require a structured
  handoff.
- B1.4 Command-scalar evidence compatibility: if a model mistakenly sends a
  command-derived scalar such as `70693 total` through `emit_evidence` without
  a real file:line anchor, the system should not force a fake source line or a
  full retry. The item is skipped as an advisory no-op and the tool result
  routes the model to `emit_investigation_complete.aggregate_facts`, where
  command-backed scalars belong.
- B1.5 Source-of-truth role normalization: request-aware aggregate role repair
  must happen before stable aggregate facts enter downstream prompt/render
  surfaces. Renderers also re-apply the same typed normalizer defensively so a
  future caller cannot reintroduce explicit `principal_answer` support ledgers
  into scalar-count finalizer gates.
- B1.6 Count-support leniency: `excluded_count` may be a count-only support
  fact. When the model supplies a partial/prose `excluded[]` placeholder whose
  length does not match the numeric value, normalize by preserving the count
  and omitting the non-exact list instead of forcing a full excluded-file
  inventory. Directory/file-set measurement attempts through `scope=file`
  without a real file-role label are likewise routed to aggregate facts as
  advisory no-ops.
- B1.7 History evidence-source separation: `is_history_lookup` is a VCS/history
  metadata source signal, not a scalar-answer shape. Non-scalar history
  questions such as latest-merged-feature summaries, recent-merge lists,
  commit comparisons, and history-backed diagnostics must keep raw VCS tool
  outputs visible to finalizer without forcing `scalar` blocks or generic
  `值：...` rendering. Scalar aggregate facts in non-scalar history runs are
  support metadata unless the typed request says `is_scalar_answer=true` or
  `is_count_question=true`. Narrative/diagnostic history runs also skip the
  architecture/source-code principal materialization gate; explicit history
  lists (`intent=enumerate`) and bucketed/cross-component comparisons keep
  their member_set aggregates as principal answer structure.

Validation target: `s7a` should stop relying on manual line arithmetic; `u7b`
should no longer get a system commit/member table when the user asked for a
scalar count, and can close on a precisely labeled VCS count proof. `s7b/u7b`
still need eval confirmation that the prompts choose the deterministic command
shape consistently. `u7c` guards the non-scalar history path so feature-summary
answers stay explanatory and are not compressed into a commit-id/subject scalar;
`u7g` guards the richer history-backed path where the user asks Codrax to find
the merge, locate corresponding code, explain affected flows, and render a
logic diagram.

2026-05-20 B1.7 validation:

- `u7c` PASS in `eval/results/u7c-20260520-105501`: feature-summary history
  answer stayed explanatory, finalizer converged in one iteration, and no
  generic `**值：** commit` compression was rendered.
- `u7g` PASS in `eval/results/u7g-20260520-110614`: answer included the merge
  commit, corresponding code/process explanation, and a `flowchart` diagram
  with one finalizer iteration. Remaining gap: analyzer first hit
  `subtopic_coherence` once, and exploration still used 63 iterations / 30
  mid-loop injections for a history-backed architecture explanation.

2026-05-20 G143 follow-up:

- R1.5 sub-topic resolver asymmetry now treats non-scalar history-backed
  trace/explain requests and explicit diagram presentation axes as advisory
  rather than hard analyzer failures. This uses typed fields
  (`is_history_lookup`, `is_scalar_answer`, `is_category_enumeration`,
  `diagram_hint`, `intent`) rather than matching user prose or model free text.
- Guard tests pin the intended boundary: history-backed commit-to-code diagram
  requests pass with advisory telemetry; ordinary diagram presentation axes pass
  with advisory telemetry; category enumeration entity asymmetry remains hard
  even when a diagram hint is present.
- `u7g` PASS in `eval/results/u7g-20260520-115916`: analyzer improved from
  5 iterations / 2 dispatches to 2 iterations / 1 dispatch; finalizer stayed at
  1 iteration with 0 semantic concerns. Remaining performance gap: exploration
  still consumed 54 iterations / 21 mid-loop injections.

2026-05-20 follow-up after first `s7a` rerun:

- `s7a` produced the correct scalar (`70693`) with one finalizer iteration, but
  finalizer prompt still received a file-list `member_set` as
  `role=principal_answer`, so deterministic exhaustive-member review added a
  generic weak-support supplement. Root cause: the earlier demotion only
  affected principal-member-set selectors; the stable aggregate pool and
  aggregate prompt renderer still trusted the model's explicit role.
- Explorer also tried to record `70693 total` as `emit_evidence scope=line`
  with `line_start=0`; this is not a valid file:line citation and should be
  treated as command measurement support, not as a hard evidence failure.
- Second `s7a` rerun (`eval/results/s7a-20260520-101348`) fixed the final
  surface but still needed 10 explorer turns: one `scope=file` directory
  measurement emit was hard-rejected, and two `excluded_count` emits were
  rejected because the model provided only a count/prose placeholder instead
  of all excluded test files. These are support-lane ergonomics issues and
  should be normalized rather than forcing inventory work the user did not ask
  for.

## Case Notes So Far

### Healthy Baselines

- `logtri_cpp_asan`: PASS, one finalizer turn, no hard rejects or rewrites.
- `logtri_custom`: PASS, one finalizer turn, no hard rejects or rewrites.
- `logtri_java`: PASS, one finalizer turn, no hard rejects or rewrites.
- `m1_tool_name_literal`: PASS, one finalizer turn; current literal-evidence path looks stable in this run.

### Cost / Quality Outliers

- `logtri_degraded`: PASS but expensive. Main issue is upstream exploration cost on a degraded placeholder artifact.
- `logtri_goroutine_dump`: PASS but polluted by external-frame repo citations, current-code over-verification, an empty system补表 for goroutine IDs, and a generic supplement; tracked as G141.
- `logtri_partial`: PASS but analyzer retry path is noisy and user-visible runtime is high for a simple scalar-from-log answer.
- `logtri_oversized`: PASS but explorer repair tail is avoidable.
- `logtri_rust`: PASS but answer quality is unacceptable because compatibility recovery lost blocks.
- `m1a`: PASS but answer is unacceptable: wrong Turn A tool count, out-of-scope finalizer tools, blank bullets, and low-confidence reviewer warnings that did not stop publication.
- `m2a`: PASS with useful main prose but duplicated anchors / generic supplement from system-side normalization.
- `mr_cross_repo_compare`: PASS but multi-repo table is contradictory and polluted by a cross-bucket symbol.
- `mr_implementers`: PASS but mixes an interface non-member into a principal implementer list, then adds a duplicate system补表.
- `mr_inactive_path`: PASS with honest inactive-scope answer, but positive-answer facets and generic supplements create avoidable caveat noise.
- `mr_keyword`: PASS with correct scalar answer, but compiler補表 cites package metadata instead of the exact function definition.
- `mr_pin_isolation`: PASS with correct inactive-scope answer, but duplicate caveats and low-confidence sufficient telemetry remain.
- `principal_span_adjacent_dispatch`: PASS and the principal-span waiver prevented a retry storm, but optional diagram pressure still leaked into reviewer/supplement output.
- `principal_span_inlined_helper`: PASS but costly; local variables became focus targets, strong definition evidence was downgraded to call-site citations, and generic补充说明 still appeared.
- `qf_config_precedence`: PASS with correct main prose, but row compiler over-applied enumeration surfaces to a precedence/scalar answer and introduced blank/duplicate tables.
- `qf_architecture`: PASS but unacceptable visible answer: row compiler duplicated scoped member sets and introduced 4/6/12 count confusion after a long, over-broad exhaustive exploration.
- `qf_diagram_pipeline`: PASS and Mermaid was preserved, but compiler补表 confused stage members with agent constants.
- `qf_imports`: PASS but system wrongly pushed repository-wide enumeration reads for a single-file import-list question; the model ignored the bad hint and finished.
- `qf_multi_member_set_count_caveat`: PASS and the final answer kept rich descriptions plus correct counts, which is an important positive signal for the richer evidence merge path. Remaining issues are cost / ergonomics: low-value auto-repair, noisy anchor skeletons, invalid exhaustive-member field spelling, and high duplicate evidence volume.
- `qf_relation_subagent_registry`: PASS and the principal answer is likely correct, but the system appended a “未达标准” supplement because support anchors and fixed citation floors were mistaken for answer defects.
- `qf_logic_view_read_pipeline`: PASS and delivered the requested Mermaid logic view with rich prose, but runtime is unacceptable. Parallel explorer convergence, forced-read role scoping, reviewer tool-call resilience, and duplicate table rendering all need follow-up.
- `qf_type_relation_loop_controller`: PASS and the implementation list is useful, but reviewer criteria are misaligned with type-relation diagrams, the diagram subtype collapsed to generic architecture, and one unsupported “5 packages” summary claim slipped through.
- `qf_sequence_analyzer_gate`: PASS but not acceptable as a final answer. Mermaid sequence diagram is present, but high-confidence contradiction review did not actually repair the 21/22 count mismatch, and compiler-added blank tables / key anchors degraded the answer.
- `read_combo_analyze_retry_anchor`: PASS with a strong main answer, but item-level citation drift remains visible in mechanism rows.
- `read_combo_answer_document_tools`: PASS with useful prose/table/flowchart, but exact literal comparison is undermined by a wrong system-added key anchor and missing finalizer dispatch citations.
- `read_combo_criterion_rich_functions`: PASS and a healthy example that rich Chinese evidence summaries can reach final output. The remaining issue is reviewer-surface mismatch, not answer content.
- `read_combo_config_two_knobs_precedence`: PASS but repeats the config-precedence compiler failure mode: good main answer plus malformed duplicate system tables and generic supplement.
- `read_combo_multirepo_negative_interaction`: PASS and improved on fake negative anchors, but negative facts are still rendered through positive member-set tables and multi-repo parent graph fallback remains in telemetry.
- `read_combo_source_locations_required_false`: PASS with a correct and rich 4-location answer. Remaining gaps are system-side: JSON-string block recovery, duplicate deterministic category tables, unnecessary finalizer auto-repair after reviewer sufficiency, and evidence inflation.
- `s11a`: PASS but materially flawed. The yes/no conclusion about `read_file` is right, while the visible allowlist omits `list_files`, so an exact allowlist/list-count statement shipped wrong. The run also exposes a grep-vs-read_file grounding contract mismatch.
- `read_combo_pipeline_sequence_table`: PASS and includes the requested Mermaid sequence diagram plus stage I/O table, but runtime/cost is unacceptable and system-added blank tables/generic supplements degrade a usable answer.
- `s11b`: PASS and the scalar value `MaxRetriesPerStage` is correct, but analyzer pre-scan was too broad and the final citation list kept a stale `context.go:14` reference ahead of the exact `config.go:14` definition.
- `s1a`: PASS and the ordered check list is useful, but optional diagram pressure and generic supplements still leak into a prose-only mechanism question; metrics also show inconsistent explorer dispatch counting.
- `s1b`: PASS and the selective-requeue conclusion is right, but one principal item cites the wrong nearby function and the run again shows JSON-string block recovery plus generic supplement churn.
- `s3a`: PASS and correctly concludes `explore_mid_loop_hint_budget` is absent from code/config/CLI, but finalizer hit an avoidable config-role enum rejection, table headers degraded to `列 N`, a visible glossary file:line lacked citation coverage, and a generic supplement appeared despite semantic confidence 1.00.
- `s3d`: PASS and the absence conclusion is broadly right, but analyzer role-binding schema caused a retry; reviewer then treated an absence answer as if positive layer anchors were mandatory, causing 27.6s of auto-repair and another generic supplement.
- `s5a`: PASS and the visible table answers the user, but semantic reviewer falsely says file locations are missing from the body. This is a strong reviewer-surface mismatch around Markdown tables / repaired citation carriers.
- `s7a`: PASS but wrong. The scalar LOC answer is lower than a deterministic external recount by 1529 lines, caused by rejected safe-count command forms and subsequent model arithmetic. This is a top-priority scalar measurement gap.
- `s5b`: PASS but unacceptable. High-confidence self-consistency found four contradictions, yet the final answer still contains conflicting 25/26 member tables generated by deterministic normalization. This is a direct repeat of “diagnose but do not actually repair.”
- `s7b`: PASS with the correct scalar count, but the route is still unhealthy. Deterministic command output repeatedly failed to satisfy the count-proof closure, and the final renderer appended a mostly empty 25-row member table to a count-only answer.
- `s8a`: PASS with a useful ordered call-chain answer. Remaining problems are recurring system tails: call-chain exploration received generic enumeration/read-more pressure, and a generic “coverage may be insufficient” supplement was rendered even though the semantic reviewer reported `sufficient=true`.
- `u10a`: PASS but unacceptable. Strong exploration evidence said `ShapeValue` is not a live constant, yet the final answer regressed to a “rename the definition” story, borrowed wrong citations for test/doc rows, and rendered unrelated `ShapeStepList` context as system补充缺失成员.
- `u11a`: PASS and the core scalar answer (`buildAnalysisIR` in `internal/agent/analyzer.go`) is correct, but analyzer over-expanded the user ask into an extra `emit_analysis`/Explorer relationship section. The final renderer also exposed citation-only empty bullets and another generic supplement despite semantic sufficiency.
- `u10b`: PASS with a mostly useful change-impact table, but it loses one verified site (`orchestrator.go:7690`) when grouping by file. Semantic review then misread the table as if it contained only one file, adding latency and another generic supplement.
- `u11b`: PASS and the 4-site answer is accurate, but exact-resolution messaging falsely says the exact target was not found and only an alias was verified. The underlying field-value lookup succeeded; the banner is a system-side false uncertainty signal.
- `u1a`: PASS but not acceptable for a security answer. The prose names the right source/sink shape, yet citations omit the source/sink lines and the allowlist counts are wrong (`29/13` in answer vs current `30/12` in source). This exposes a precise-count + citation-binding gap in taint-flow answers.
- `u1b`: PASS but security semantics are too coarse. `baselineShow` uses user `sha` to select among enumerated filenames; it does not pass `sha` itself as a path segment to `os.ReadFile`. The answer should distinguish selector influence from raw path-payload injection, and the generated protected-path table still contains a blank row.
- `u3a`: PASS but includes a factual control-flow error: extractor `ShouldStop` is described as one-shot / `iteration >= 1`, while the cited helper stops below soft cap only after `iteration >= 3` logic and hard-stops at 5. Rich prose is present, but the reviewer did not validate the simplified predicate.
- `u3b`: PASS but unacceptable because final answer contains an explicit 7-vs-5 TaskGraph node contradiction. The system detected it with confidence 0.95 and still shipped it after auto-repair, with additional blank compiler tables.
- `u4a`: PASS and the visible import list is mostly clean, but the run collected 2809 evidence rows for a 5-package import inventory. Reviewer facet telemetry also counted only 4/4 while the visible answer has 5 items.
- `u4b`: PASS and the 9 direct import sites are useful, but evidence volume is still high and a concrete reviewer concern about search scope was collapsed into the generic coverage supplement instead of a localized note.
- `u5a`: PASS but wrong. `Compile` and `InferScenario` are listed as lacking `_test.go` coverage even though `compile_test.go` has many `TestCompile_*` tests and a direct `TestInferScenario`. This is a precise coverage-pairing failure, not just presentation noise.
- `u6a`: PASS with a useful error-wrapping summary, but self-consistency produced a high-confidence false contradiction about `ErrAllRetriesExhausted` even though the cited source line contains that identifier. The answer then received an unnecessary inconsistency supplement.
- `u7a`: PASS and the commit hash/subject are correct, but a simple history scalar still dragged in 2000 current-code evidence rows and rendered two anonymous `值` scalar blocks.
- `u5b`: PASS but unacceptable nuance loss. The closest stop-condition hot-loop tests are real, yet the answer states “yes, dedicated test” without preserving that the exact `contractFailureBreak` / `lastFinalize == nil` fallback is not directly tested. It also leaks unrelated `stuck_injection_test.go` rows and `[illustrative]`.
- `u8a`: PASS and a positive signal for rich enumeration: all 34 exported API members are listed with useful Chinese descriptions. Residual issue is UI noise from empty duplicate category headings and a generic weak-support supplement.
- `u7b`: FAIL and top-priority scalar/history regression. The correct intersection between last 20 `internal/orchestrator/` commits and `runTaskGraph` function-history commits is 0, but the answer said 5 after a long manual-output comparison route.
- `u8b`: PASS and independently verified as correct: `internal/types` has 104 exported string-enum types with typed const-set evidence. Treat the weak line-text telemetry as a presentation/telemetry nuance, not a correctness failure.
- `u9a`: PASS with a correct mechanism answer, but analyzer spent 5 rounds trying to inspect source-context in prescan before classifying.
- `u9b`: PASS with a correct per-item-rejection answer, but analyzer/explorer/extractor all hit avoidable schema or corroboration friction before the finalizer produced a sufficient answer.
- `arkts_repomap`: FAIL only because final output omitted `@Component`; the @Entry/@Builder member list and counts are otherwise correct. The run is still valuable because it exposes ArkTS search file-type mismatch, over-strict analyzer subtopic coherence, decorator-stack loss, and reviewer section/table misparsing.
- `hilog_arkts_panic`: PASS and the root-cause text is useful, but external-only log handling still wastes a full exploration route and leaks code-grounded hypothesis/evidence criteria into an artifact-only diagnostic.
- `cangjie_repomap`: PASS with the correct declaration set, but the answer is cluttered by duplicated deterministic tables, wrong package-line wording, reviewer false concerns, and high exploration cost.
- `hilog_cangjie_panic`: PASS with the correct root-cause text, but external runtime paths leaked into the repo citation list and a blank `<native>@runtime:0`補表 row was rendered.
- `hilog_mixed_arkts_cangjie`: PASS and a healthy cross-language artifact answer: four runtime frames were preserved with no repo citation pool leakage. Residual issue is schema ergonomics for external frame aggregates; the model had to strip language/decorator qualifiers after a support-ref rejection.
- `hitrace_jank`: PASS and the final root-cause text is correct, but the route exposes a serious perf-artifact contract gap: log triage rejected the exact empty handoff it requested, perf triage did not own the run, current-code facets leaked into an external trace answer, and generic “未达标准” supplements appeared despite the answer being artifact-complete.

### Healthy / Mostly Clean Read-Mode Cases

- `qf_import_literal_references`: PASS. Import path literals stayed as import-reference members and were not forced through declaration-symbol gates; only a localized scope note was rendered.

### Write-Mode Baselines

- `patch_c_typo`, `patch_cpp_typo`, `patch_go_typo`, `patch_java_typo`, `patch_python_typo`: PASS. These are not the primary read-mode focus of this sweep, but they provide a sanity baseline that the added read-mode evals did not disturb write-mode apply/verify plumbing.

## Reverse-Audit Coverage Check

2026-05-20 follow-up audit reran a log-pattern cross-check over all 82
`run-1.out` / `run-1.metrics.txt` files under
`eval/results/full-20260520-030326` and compared anomaly-bearing cases against
this ledger. The scan covered finalizer rejects, analyzer retries, schema
compatibility repairs, reviewer negative verdicts, generic supplements,
degraded table headers, citation-only list items, multi-repo fallback telemetry,
external-artifact/current-code conflation, timeout/cost markers, and metric
outliers such as `explorer_dispatches=0` with positive explorer iterations.

Result: every anomaly-bearing case was already represented in this document
except `logtri_goroutine_dump`, now recorded as G141. No additional unrecorded
case-level gaps were found by the reverse-audit pass. The pattern scan also
confirms the high-level systemic clusters already represented above:
external-only artifact routing, row-compiler/system补表 pollution,
reviewer/render surface mismatch, schema-aware repair gaps, telemetry counter
drift, scalar/count deterministic-measurement gaps, and high-cost explorer
convergence tails.

2026-05-20 late contract rerun:

- After Batch D.6, representative contract evals were rerun with the local
  binary: `u7c-20260520-221501`, `u7k-20260520-221645`,
  `s7a-20260520-222122`, `qf_logic_view_read_pipeline-20260520-222241`,
  `qf_multi_member_set_count_caveat-20260520-222601`, and
  `logtri_rust-20260520-223108`. All PASSed with one finalizer iteration and
  without hard成文 rejects / heavy rewrite loops in the scanned logs.
- Manual inspection of `qf_multi_member_set_count_caveat-20260520-222601`
  found a remaining system-side defect despite PASS: deterministic supplements
  duplicated member tables and expanded the read-mode/write-mode `Kind` buckets
  into the same full const universe. Root cause was role-only aggregate
  reconciliation; it treated every same-role definition anchor as missing
  members for every same-role principal member set.
- Batch E.2 fixed that at the source by disabling role-only expansion whenever
  multiple principal complete member sets share the same candidate role. The
  rerun `qf_multi_member_set_count_caveat-20260520-224212` PASSed with one
  finalizer iteration, no “系统按已验证证据补充成员” pollution, no read/write
  bucket over-expansion, and rich Chinese summaries preserved in the `Kind`
  rows.
- Residual telemetry only: the low-confidence self-consistency reviewer noticed
  wording drift between `Kind` const block ranges (`26-65` vs `29-65`) but did
  not trigger a rewrite. This is acceptable under the current red line because
  it is model-authored low-confidence wording, not a precise system-generated
  contradiction. A future localized line-range formatter could tighten this
  without changing model content.

2026-05-20 VCS/non-code evidence follow-up:

- Added eval `u7l` for "最近 10 次提交都做了哪些事情，作用和影响分别是什么".
  This was created after the customer-style failure where a perfect exploration
  summary about the latest merged feature was collapsed by later stages into a
  single commit-id value.
- First local `u7l` replay exposed two system issues:
  - pure history enumeration was treated as mixed current-source work, causing
    unnecessary `read_file` / `emit_evidence` behavior;
  - finalizer could leak internal carrier terminology (`citation_ref=-1`,
    `citations[]`) when asked to explain command/VCS provenance.
- Fixes landed in the unified contract path:
  - pure recent-N history enumeration stays in the VCS lane and may close via
    `emit_investigation_complete(reason, aggregate_facts)`;
  - answer-document item JSON with exactly one unknown non-empty string field is
    repaired losslessly into `text`, avoiding a visible item that keeps only the
    commit hash;
  - analyzer prompt now teaches direct repository-history classification
    without source pre-scan;
  - finalizer and tool-sourced guidance now prohibit exposing internal
    `citation_ref` / `citations[]` carriers to the user.
- `u7l-20260520-234847` passed after the VCS-lane fix with `read_file=0`,
  `midloop_inject=0`, `finalizer_iters=1`, and no semantic-quality concerns.
  It should be rerun after the analyzer direct-history prompt change to verify
  whether the analyzer pre-scan disappears as well.
- Design decision for git diff evidence: diff output is first-class evidence,
  but its origin is `vcs_diff`, not current-source `emit_evidence(file:line)`.
  The short-term carrier is structured git/exec tool output plus
  `emit_investigation_complete.reason/aggregate_facts`; the long-term target is
  a dedicated VCS diff evidence item with commit/ref/path/hunk/old-new side.
  This avoids forcing old/deleted/renamed diff lines through current-checkout
  citation gates.
- New Batch F in `unified_evidence_answer_contract_20260520.md` generalizes
  this beyond git: VCS, logs, traces, command measurements, negative searches,
  and cross-repo index facts all use typed evidence-origin lanes instead of
  fake current-source anchors.
- Batch F.4 added code-level protection for one mixed-origin edge that evals
  should cover next: an answer can have a present current-source anchor and a
  bounded negative-search result at the same time. The negative result remains
  first-class only when `repo/query|pattern/result_count=0/scope/searched_at`
  are preserved. A future eval should ask for a relationship where one repo has
  a concrete implementation and another repo has a verified zero-hit interface
  search.
- `u7l-20260521-001605` passed after the direct-history analyzer prompt and
  unified VCS lane: `read_file=0`, `midloop_inject=0`, `analyzer_iters=1`,
  `explorer_iters=2`, `extractor_iters=1`, `finalizer_iters=1`. It also exposed
  two residual, non-blocking system gaps:
  - semantic-quality review dispatch failed when a model emitted
    `concerns` as a string instead of the schema's array. This is schema drift,
    not a semantic answer failure, so Batch F.5 added a schema-aware repair
    path for string/single-object/null concerns.
  - self-consistency noticed a low-confidence unsupported grouping count in the
    VCS summary. This should not force a rewrite, but it is avoidable; Batch
    F.5 added soft finalizer guidance to avoid module/component/category counts
    unless they are explicitly present in VCS/command output or
    `aggregate_facts`.
- `u7l-20260521-002737` passed structurally, but self-consistency falsely
  emitted a high-confidence row-order contradiction by inferring chronology
  from patch size ("large change usually older"). This is a precise red-line
  violation for reviewer gating: patch size is noisy evidence and must not
  drive a hard rewrite signal. Batch F.6 added a typed VCS row-order suppressor
  that compares visible commit labels against `git_log` order, plus prompt
  guidance forbidding patch-size chronology inference.
- `u7l-20260521-003544` passed without the reviewer false positive, but the
  final answer duplicated the already-rendered commit list with a system
  supplement table. Root cause: aggregate `member_set` rows were decorated as
  `hash: subject (stat)` while the model's visible list labels were bare
  commit hashes, so the compiler thought every row was missing. Batch F.6
  treats 7-40 char commit-hash prefixes as a structural identity match for VCS
  member rows.
- `u7l-20260521-004117` removed the duplicate table but failed the quality
  regex: the explorer closed after one `git_log`, the finalizer prompt showed
  a trimmed VCS output, and later commit rows degraded into path-only generic
  prose. Batch F.6 increased VCS raw-output prompt caps, added finalizer
  guidance to state both change and effect/impact for history lists, and keeps
  history closure prose visible even when a principal VCS member_set exists.
- Runtime artifact follow-up from the same unified contract audit: the
  `logtri_goroutine_dump` family previously showed external frame paths and
  `<native>@runtime:0`-style placeholders leaking into repo-citation/member
  supplement machinery. Batch F.7 moves the fix to the shared enumeration row
  compiler and pre-emit carrier repair: rows inherit aggregate
  `AnswerEvidenceOrigin`, and pure non-current-source origins cannot promote
  member/support-ref `file:line` surfaces into current-checkout citations.
  Coordinate-only runtime placeholders with no useful note/location are skipped
  by system supplements. This is a source-level contract fix; the next eval
  replay should prioritize `logtri_goroutine_dump`, `hilog_cangjie_panic`, and
  `hitrace_jank` to verify artifact-only answers stay artifact-scoped without
  losing useful diagnostic prose.
- First F.7 replay `logtri_goroutine_dump-20260521-010439` PASSed and kept
  `tool_read_file=0`, but still exposed two system-side tails: finalizer
  rewrote once because optional `candidate_role` used natural labels
  (`goroutine`, `error`), and the compiler appended a duplicate runtime
  artifact function supplement because it did not count prose coverage. The
  follow-up code now repairs invalid optional candidate roles to `other` and
  treats runtime-artifact member prose as coverage. Rerun this case after the
  patch before considering the artifact lane closed.
- Second replay `logtri_goroutine_dump-20260521-011127` removed the finalizer
  retry (`finalizer_iters=1`) but showed a deeper origin-boundary gap:
  analyzer mirror drift set `current_version_check=true`, the prompt exposed
  active origins as `current_source + runtime_artifact`, and explorer read
  current repo files even though the user's question only asked what the log
  showed. Batch F.8 moved the fix to the typed contract: external-source
  runtime artifacts with `resolved_files=0` remain observation-only unless a
  separate current-checkout verification anchor exists (`exact_targets`,
  `required_files`, or resolved frames).
- Third replay `logtri_goroutine_dump-20260521-012600` PASSed with
  `tool_read_file=0`, `midloop_inject=0`, `explorer_iters=1`,
  `finalizer_iters=1`, and prompt origins only `runtime_artifact`; it still
  spent one analyzer files-only grep before emit_analysis. Batch F.8 added
  analysis-skill guidance so external-source log/trace classification follows
  the same direct path as VCS history.
- Fourth replay `logtri_goroutine_dump-20260521-013104` PASSed after that
  prompt fix with `analyzer_iters=1`, `tool_read_file=0`, `midloop_inject=0`,
  `explorer_iters=1`, `finalizer_iters=1`, and no semantic-quality concerns.
  This closes the local `logtri_goroutine_dump` runtime-artifact lane: no
  analyzer pre-scan, no explorer repo reads, no duplicate system supplement,
  and no finalizer rewrite.
- `hilog_cangjie_panic` follow-up confirmed the same boundary needed two more
  source-level protections. First, analyzer/explorer had to skip source tools
  decisively for observation-only artifacts; otherwise the model still tried
  repo searches or fake `emit_evidence(source=runtime_artifact)` before
  converging. Second, deterministic row compilers had to stop treating runtime
  call-chain aggregate members as current-source enumeration rows. The final
  replay `hilog_cangjie_panic-20260521-021900` PASSed with `tool_read_file=0`,
  `midloop_inject=0`, `analyzer_iters=1`, `explorer_iters=2`,
  `finalizer_iters=1`, `semantic_quality_dispatches=0`, no artifact citation
  pool, and no system-generated补表. This closes G134/G135 for the Cangjie
  runtime-log case while leaving the broader first-class artifact-frame schema
  work (G136-G140) open.
- F.11 code audit answered the broader contract question: exploration can hand
  off external information, history, logs, traces, and command output as
  evidence, but not every family should travel through `emit_evidence`.
  `emit_evidence` remains the current-checkout source-line tool; non-code
  facts travel through producer-origin banners, raw `ToolResults`, runtime
  bundles, stable `aggregate_facts`, and `AnswerClaimBinding`.
- F.11 also closed three runtime-artifact validator tails found after
  `hilog_cangjie_panic-20260521-021900`: observation-only runtime observations
  now satisfy the evidence-count floor, orchestrator/extractor skip code-call
  auto-verdicts for artifact-only answers, and decorated runtime aggregate
  members no longer need repo `support_refs`. Wrapped/raw external frame
  citations in `emit_hypothesis_verdict` are normalized into rationale context
  instead of being treated as unresolved repo citations.
- Replay `hilog_cangjie_panic-20260521-024522` PASSed with
  `tool_read_file=0`, `midloop_inject=0`, `analyzer_iters=1`,
  `explorer_iters=2`, `extractor_iters=1`, `finalizer_iters=1`,
  `semantic_quality_dispatches=0`, and no rejected hypothesis/support-ref
  loops. Residual low-priority tail: pre-emit still logged a non-blocking
  summary/caveat block-shape advisory even though the accepted answer included
  summary/caveat content inside one ordered list. Do not harden that advisory
  until a real user-visible defect appears.
- Long-term remaining gap is structural rather than case-specific: introduce a
  single internal observation ledger indexed by `origin`, `target_ref`, and
  `support_ref`, then refactor the existing carriers into it. This would make
  VCS, command, runtime, negative-search, and cross-repo-index facts easier for
  future developers to consume without confusing them with current-source
  `EvidenceItem`s. It should not replace the working producer-origin /
  claim-binding path.
- F.13 added explicit line/order coverage for attached logs and traces. New
  evals `logtri_artifact_line_anchor` and `hitrace_artifact_line_anchor`
  verify that artifact-local `N│` gutters can answer "第几行/第几个事件"
  questions without repo reads or fake current-source citations. Follow-up
  audit found and fixed one hidden contract mismatch: extractor prompts allowed
  artifact anchors, but `emit_hypothesis_verdict` rejected `runtime_artifact:3`
  / `trace:5`. The tool now normalizes `log:N`, `trace:N-M`, and
  `runtime_artifact:N-M` into rationale-only artifact context; the repo
  citation field stays empty.
- Latest replays before committing:
  `logtri_artifact_line_anchor-20260521-085719` PASSed with
  `tool_read_file=0`, `midloop_inject=0`, all pipeline stages at one LLM
  iteration, and no semantic-quality concerns. `hitrace_artifact_line_anchor-
  20260521-085840` PASSed with `tool_read_file=0`, `midloop_inject=0`,
  `semantic_quality_dispatches=0`, and no finalizer retry. Rerun both after
  the artifact-local verdict parser patch; expected difference is removal of
  hidden `emit_hypothesis_verdict` rejects, not a visible answer change.
- The first post-parser trace replay
  `hitrace_artifact_line_anchor-20260521-090750` removed the hidden extractor
  reject but exposed a separate analyzer tail: optional `field_value_profile`
  and derived `exact_targets` caused five `emit_analysis` rejects even though
  the primary classification was already sufficient. The fix keeps the strict
  behavior for normal current-source requests, but in observation-only
  runtime-artifact requests drops invalid optional field-value refinements and
  filters exact targets down to request-verbatim items. This follows the
  contract rule that optional refinements must not make artifact answers
  longer or less stable.
- The next mixed replay showed the same class in log form:
  `logtri_artifact_line_anchor-20260521-091414` still had analyzer retries
  because `is_role_locate_lookup=true` lacked `answer_subject.kind`, and a
  hidden extractor reject because the model provided a rationale but omitted
  the artifact-local citation. Runtime line/event-row observations now default
  the missing role-locate subject to `numeric`, and rationale-only verdicts are
  accepted as artifact context when the request is observation-only. This keeps
  current-source citation strictness intact while removing non-user-visible
  artifact retries.
- One more prompt/contract mismatch surfaced in the accepted log output:
  `intent=explain` + `question_kind=return_value` loaded explanation-style
  finalizer obligations for a scalar log-line question. The analyzer tool now
  normalizes observation-only scalar runtime requests to `intent=return_value`,
  so finalizer shape follows the user's actual line/existence question rather
  than an architecture fallback.
- Final F.13 replay after the compatibility fixes:
  `logtri_artifact_line_anchor-20260521-092414` and
  `hitrace_artifact_line_anchor-20260521-092414` both PASSed with
  `tool_read_file=0`, `midloop_inject=0`, `analyzer_iters=1`,
  `explorer_iters=1`, `extractor_iters=1`, `finalizer_iters=1`, no semantic
  concerns, and no `TOOLRESULT ... ok=false` / finalizer retry markers in the
  logs.
- Observation-ledger Batch 4C added two mixed-origin guards after the unified
  prioritization work: `u7o` for "latest git diff + current source impact" and
  `logtri_line_current_code` for "artifact-local log line + current-code
  explanation". These extend the earlier `u7k/u7l` and artifact-line cases so
  future compact-ledger budgeting cannot hide grounded current-source evidence
  in mixed questions or let external-only observations be swallowed by
  incidental source reads. MCP/web mixed-origin cases remain backlog items
  until those producers are executable in the eval runner.

### Batch 12 — Parallel Explorer Closure Convergence

- Scope: address G149 without turning a single customer log into a bespoke
  branch. The code audit shows existing early-convergence support in
  `dispatchExploreWindowsParallel`, but the wait/continue predicate still
  treats broad analyzer signals (`sub_topics` and `is_cross_component`) as
  hard blockers. That conflicts with the project red line: noisy breadth
  signals may guide exploration, but they must not by themselves force all
  sibling lanes to chase surgical forced reads after a lane has passed
  `emit_investigation_complete` pre-complete gates.
- Contract update: parallel siblings must all finish only when a precise,
  user-structure signal says the answer is partitioned or complete-set shaped:
  explicit analyzer-emitted `QuestionStructure` buckets/count/completeness,
  diagram presentation, relation/enumeration member-set handoff,
  change-impact/file-site outputs, field-value profiles, or diagnostic/current
  status obligations. Bare `is_cross_component=true` and internally expanded
  `sub_topics` remain scheduling breadth signals, not convergence blockers.
- Safety guard: true comparison / multi-repo questions stay protected through
  analyzer-emitted `Buckets`, relation predicates, required diagram, or
  change-impact obligations. System-inferred buckets from required files or
  user-mentioned entities still help rendering/support planning, but are not a
  hard parallel-convergence blocker because they are fallback structure rather
  than a direct model-authored partition. If those typed hard carriers are
  absent, an accepted closure is allowed to converge the parallel window
  because the model still saw the full user request and the tool pre-complete
  checks already accepted its structured conclusion.
- Task list:
  1. Refactor `parallelExploreAllowsEarlyConvergence` into a positive
     "must wait for sibling handoffs" predicate that consumes only precise
     typed obligations.
  2. Update tests to preserve true bucketed/diagram/enumeration wait behavior
     while allowing bare cross-component/mechanism breadth to converge after
     an accepted closure.
  3. Rerun targeted orchestrator tests and the `read_combo_analyze_retry_anchor`
     eval; record whether explorer iterations and mid-loop injections drop
     without losing final-answer completeness.

Progress 2026-05-21:

- Implemented the first predicate refactor and tests. Follow-up replay
  `read_combo_analyze_retry_anchor-20260521-141645` showed the original
  parallel wait condition was only one part of the problem: the eval failed
  with `no_result` because the final answer quoted the literal legacy symptom
  `(no result)`, and exploration still reached 101 model turns.
- Root-cause split:
  - The current source comment in `orchestrator.go` described the avoided
    historical empty-result failure with wording that the model could read as
    current behavior. That polluted the accepted closure reason and final
    answer. Fix: rewrite the comment to explicitly say this is legacy behavior
    avoided by the current recovery path and remove the literal marker that
    eval treats as a real no-result answer.
  - `emit_investigation_complete.aggregate_facts` carried optional mechanism
    scaffolding as a decorated `principal_answer` member_set without
    `support_refs`. For true enumeration / relation / count / bucketed
    structure this must remain hard, but for a narrative mechanism answer it
    is optional support: rejecting the completion forced another explorer turn
    even though grounded prose and evidence were sufficient. Fix: under the
    existing `completionAggregateFactsAreOptional` typed predicate, drop
    unsupported decorated member_sets even when the model marked them
    `principal_answer`, disclose the drop in the tool summary, and let the
    accepted closure proceed. This does not weaken precise structural outputs
    because the optional predicate is false for scalar/count/enumeration/
    relation/diagnostic/change-impact/field-value/question-structure cases.
- New test guard: mechanism narrative completions with optional decorated
  principal member_sets no longer retry, while existing decorated code member
  tests still require `support_refs` for true principal structured outputs.
- Replay `read_combo_analyze_retry_anchor-20260521-144112` PASSed after the
  legacy-comment and optional-member-set fixes: `tool_read_file=17`,
  `midloop_inject=11`, `explorer_iters=24`, `extractor_iters=3`, and
  `finalizer_iters=1`. This confirms the no-result pollution is gone and the
  finalizer no longer retries. Residual observations are now narrower:
  extractor can still choose a call-site line for one answer symbol and repair
  itself, and the answer may include system-supplemented sections; both are
  non-blocking compared with the previous exploration/finalizer loop.
- UX/localization follow-up: the system-injected required-mechanism-anchor
  block was titled `Key anchors`. This is not model output, so the title now
  follows the requested answer language (`关键锚点` for Chinese, unchanged
  English otherwise). Tests cover both languages and keep the change scoped to
  system-added blocks only.
- Forced-read repair follow-up: multipath surgical-read rationale previously
  emitted a copy-pasteable `read_file(...)` command only for the first missing
  range. When a file had multiple missing windows, the model had to infer the
  later `offset` values and could miss a boundary line, producing the familiar
  "I already read it but the system still says unread" loop. The shared
  renderer now emits one exact zero-based `read_file` command per missing
  range (capped for pathological cases) across symbol, keyword, and small-file
  demand paths. Regression tests pin the real two-range pattern
  `2749-2804, 3755-3800` and the keyword analogue.
- Extractor contract follow-up: replay
  `read_combo_analyze_retry_anchor-20260521-144956` PASSed with
  `finalizer_iters=1`, confirming the final answer no longer falls back to
  the legacy empty-result wording. It also exposed a separate prompt/schema
  mismatch: the axis-alignment retry asked the model to add a `condition`
  anchor to `emit_answer_symbol`, but `emit_answer_symbol.kind` deliberately
  has no `condition` kind. The second attempt therefore emitted exactly what
  the hint implied and the tool dropped it. The fix keeps the L3 axis retry
  for real enumeration/symbol slates, but skips it for multi-topic
  explanation anchor skeletons. In those narrative answers, condition /
  return / assignment evidence stays in the normal evidence/support lanes
  where finalizer can cite it without forcing a non-symbol fact into the
  symbol taxonomy.
- Latest replay data after the forced-read renderer change:
  `tool_read_file=30`, `midloop_inject=22`, `explorer_iters=46`,
  `extractor_iters=2`, `finalizer_iters=1`. This is functionally stable but
  shows remaining exploration-budget noise from repeated evidence/partial-read
  hints. Treat that as a follow-up optimization target, not as proof that the
  answer contract failed.
- Replay after the axis-skeleton fix
  `read_combo_analyze_retry_anchor-20260521-150115` also PASSed:
  `tool_read_file=25`, `midloop_inject=18`, `explorer_iters=32`,
  `extractor_iters=2`, `finalizer_iters=1`, and no `axis-anchor mismatch` /
  `unknown kind` marker. The remaining extractor retry was a separate
  symbol-line repair (`buildDegradedSemanticIR` was cited at a fallback
  sibling line), so it should be handled through existing answer-symbol line
  repair work rather than re-opening the axis contract. This replay also
  proved one more source comment still exposed the legacy empty-answer marker;
  source comments/tests now describe it generically as an empty-output symptom
  so model-visible code-reading runs do not accidentally quote the sentinel as
  current behavior.

### Batch 13 — Role-Accurate Anchor Handoff

Status: implemented; targeted replay done; one downstream status-contract fix added.

- Latest high-ROI gap from `read_combo_analyze_retry_anchor-20260521-150115`:
  the finalizer prompt's `Preferred anchors — safe to cite` list advertised
  non-definition evidence as `function` anchors. For example, a call-site line
  inside `buildDegradedSemanticIR` could be rendered as if
  `buildDegradedSemanticIR` itself were safely citeable at that call-site line.
  This is system-generated misinformation: the model is asked to trust a
  "safe" anchor guide whose role does not match what the line proves.
- Contract update: prompt-visible anchors must be role-accurate. Definition
  evidence can expose definition-style `function` / `symbol` anchors. Call,
  condition, return, assignment, initializer, import, and string-literal rows
  stay visible only as those evidence roles. Caller / owner context from a
  call-site row is no longer exposed as a definition-style safe anchor.
- Matching update: aggregate member evidence selection now ranks identity
  matches by proof strength before generic evidence rank: direct
  `anchor_symbol` matches outrank `object`, surface-term, owner, and subject
  context. This preserves rich grounded summaries while preventing a sibling
  call line from becoming the principal citation merely because the requested
  member appeared as caller/subject metadata.
- Scope: this is generic across all supported languages and anchor roles. It
  does not inspect user prose or model prose with keywords; it only consumes
  typed evidence fields (`anchor_kind`, `anchor_symbol`, `subject`, `object`,
  `surface_terms`, source line, grounding tier).
- Guards added:
  - visible-anchor whitelist test proving call-site evidence exposes the
    callee as `call_site` and does not expose the caller as a safe definition
    anchor at that line;
  - required-symbol test proving `ContractTermSymbol` is rendered as `symbol`,
    not `function`;
  - aggregate-member support-plan test proving a member row chooses the line
    whose `anchor_symbol` matches the member over another line where the member
    is only the call subject.
- Next validation: rerun `read_combo_analyze_retry_anchor.case`. Success
  criteria: no `buildDegradedSemanticIR ... 7612 · function` preferred-anchor
  line, no axis-anchor / unknown-kind retry regression, and finalizer remains a
  one-turn answer.

Progress 2026-05-21:

- Rerun `read_combo_analyze_retry_anchor-20260521-152318` PASSed with
  `tool_read_file=23`, `midloop_inject=12`, `explorer_iters=28`,
  `extractor_iters=1`, `finalizer_iters=1`, no semantic-quality concerns, and
  no `axis-anchor mismatch` / `unknown kind` retry. The prompt-side preferred
  anchors now label non-definition rows as role-specific anchors; for example
  `applyStageOutput`, `dispatchStage`, and `dynamicAnalyzeRetries` appear as
  `call_site`, not as definition-style `function` anchors.
- The same replay exposed a higher-level residual: because architecture /
  mechanism multi-topic explanations still treated analyzer sub-topics as a
  hard `emit_answer_symbol` skeleton, a pre-scan-derived write-mode helper
  (`fallbackWriteAnalysisIR`) could become a visible "关键符号锚点" and steer
  the final answer toward the wrong read-mode recovery path. This is not a
  model-only defect; the system turned optional analyzer decomposition into
  principal answer structure.
- Follow-up contract update: `emit_answer_symbol` is now a hard requirement
  only for generic multi-topic explanations, explicit requested bounded sets,
  and symbol-backed enumerations. Architecture/mechanism multi-topic
  sub-topics remain visible as optional structure guidance, but the extractor
  prompt explicitly says not to call `emit_answer_symbol` merely to mirror
  those hints. The answer should be rendered from grounded evidence/support
  lanes plus prose/sections, so unsupported or out-of-scope helper symbols can
  be omitted or disclosed instead of promoted.
- Guard added: `TestExtractor_ArchitectureMultiTopicSubtopicsAreOptionalStructure`
  proves architecture/mechanism sub-topics do not create a hard answer-symbol
  floor and do not instruct the model to emit one anchor per sub-topic.
- Rerun `read_combo_analyze_retry_anchor-20260521-153458` PASSed after the
  optional-structure change. The final answer now correctly follows the
  read-mode degradation path: `Run()` calls `buildDegradedSemanticIR`, then
  falls back to `buildDegradedFallbackIR` only when `partialIR == nil`; the
  pre-scan-only `fallbackWriteAnalysisIR` helper no longer appears as a
  principal answer-symbol block. Metrics stayed one-turn at extraction and
  finalization (`extractor_iters=1`, `finalizer_iters=1`).
- Residual observed in the same replay: the self-consistency reviewer emitted
  three high-confidence contradictions and the UI printed
  `检测到 3 处前后不一致，正在重写答案`, but the default soft violation policy
  accepted the answer with a caveat and did not re-dispatch finalizer. Root
  cause: `runSelfConsistencyReviewV2` rendered the status from the yaml
  `rewrite_on_contradiction` intent flag instead of the effective
  soft/strict retry decision. Fix: the notice now calls the same
  `FilterFinalizerRetryRootViolationsForBus` policy used by the actual
  control flow. Default-soft self-contradiction shows `仅记录，未重写`; only
  strict-promoted deployments can display `正在重写`. Guard added:
  `TestRunSelfConsistencyReviewV2_NoticeMatchesEffectiveRetryPolicy`.
- Rerun after the notice fix: `read_combo_analyze_retry_anchor-20260521-154724`
  PASSed. Self-consistency returned `consistent=true confidence=0.90` (below
  floor but clean), so no contradiction notice fired. The final answer kept the
  corrected read-mode story and no longer exposed an English `Key anchors`
  title; the system-added support block rendered as localized `关键锚点`.
  Remaining gaps recorded for follow-up rather than widened in this batch:
  finalizer first emitted JSON-encoded `blocks[]` that could not be recovered
  without loss, so one light schema retry was necessary (`finalizer_iters=2`);
  and the localized `关键锚点` block is still a system-added support carrier that
  can duplicate the model-authored explanation. This is the same G46/G49 family:
  future work should keep mechanism anchors as support metadata unless the user
  explicitly asks for a key-anchor surface, and schema-aware recovery should
  only accept JSON-string blocks when every visible block is preserved.
- Follow-up fix: required mechanism-anchor coverage now uses only typed
  structured carriers, but accepts safe structural equivalents. A label/title
  like `StageOutput.AnalysisIR` covers both `StageOutput` and `AnalysisIR`;
  code-identifier variants such as `EmitAnalysis` cover the tool-name anchor
  `emit_analysis` through a compact identifier key. This avoids turning already
  covered structured answer content into a separate visible `关键锚点` support
  block. It deliberately does not scan user prose or model prose.
- Guards added:
  - `TestMissingRequiredMechanismAnchors_StructuredQualifiedLabelsSatisfyParts`
    pins qualified typed labels as owner/member carriers;
  - `TestMissingRequiredMechanismAnchors_StructuredIdentifierVariantSatisfiesToolName`
    pins safe code-identifier variants for tool-name anchors.
- Reruns `read_combo_analyze_retry_anchor-20260521-155947` and
  `read_combo_analyze_retry_anchor-20260521-161955` PASSed with
  `finalizer_iters=1`, no `关键锚点` / `Key anchors`, and no finalizer retry.
  They also exposed the next high-ROI convergence gap: `explorer_iters` remains
  high (`112` then `55`), with repeated mid-loop repair/forced-read work after
  other lanes had already accepted grounded closure. Track this under
  E20260521-G149/E20260521-G151 rather than widening required-anchor matching
  further.
- Same replay exposed duplicate reviewer progress notices in the REPL:
  `检查答案是否前后一致` / `正在审阅答案完整性` appeared twice. Root cause was a
  deterministic answer-document auto-repair follow-up calling the full
  contract checker, including LLM reviewers, after the first pass had already
  reviewed the answer. The auto-repair re-check now skips LLM reviewers when
  the first pass already passed strict policy; strict-blocked first passes
  still allow reviewers after repair. Guard:
  `TestRunContractCheck_SkipLLMReviewSuppressesReviewerNotices`. Replay
  `read_combo_analyze_retry_anchor-20260521-161955` confirmed the render now
  emits each reviewer progress line once.
- Convergence boundary follow-up: accepted exploration closure now clears
  stale pending forced-read debt as well as repair directives. The boundary is
  deliberately narrow: it fires only after `emit_investigation_complete` passed
  pre-complete gates, or when an accepted fork is merged into the parent state.
  It does not erase evidence/read coverage, and early convergence is still
  disabled for explicit user buckets, exhaustive enumeration, required diagrams,
  diagnostics, change-impact, and field-value questions. Guards:
  `TestEmitInvestigationComplete_AcceptedCompletionClearsBypassedPendingReads`
  and the expanded
  `TestMergeExploreFork_AcceptedCompletionClearsSiblingRepairs`.
- Schema-aware aggregate follow-up from the same eval: `bucket_count` /
  `grouped_count` now derive a missing `value` from their own exact
  `members[]`, matching the existing `member_set` compatibility lane. This is
  still structural, not semantic: `total_count` / `unique_count` continue to
  require explicit numeric values because their members can be samples. Guards:
  `TestNormalizeAnswerAggregateFacts_DerivesBucketCountValueFromMembers` and
  `TestEmitInvestigationComplete_DerivesBucketCountValueFromMembers`.
- Replay after the schema repair:
  `read_combo_analyze_retry_anchor-20260521-164441` PASSed with
  `tool_read_file=26`, `midloop_inject=13`, `explorer_iters=24`,
  `finalizer_iters=1`, and no `value is required` aggregate rejection. The
  remaining completion downgrade was a legitimate exact-anchor repair: the
  user explicitly asked about `runTaskGraph`, and the route had not yet read
  the entry/empty-IR guard. This should stay a precise forced read. Two
  residual UX/system gaps remain open: the forced-read repair hint renders
  duplicate `Forced Read List` headings, and a localized but system-added
  `关键锚点` support block can still appear with weaker citations. Track these
  under G149/G154; do not weaken exact anchor safety to hide them.
- Follow-up replay after the prompt/render cleanup:
  `read_combo_analyze_retry_anchor-20260521-170606` PASSed with
  `tool_read_file=24`, `midloop_inject=12`, `explorer_iters=25`,
  `finalizer_iters=1`, `semantic_quality_concerns=0`, no `关键锚点` /
  `Key anchors`, no duplicate `Forced Read List`, and no empty
  `Search Coverage Gap` heading. The final answer now reflects the current
  source accurately: read-mode analyze retry exhaustion installs an explicit
  degraded semantic/fallback IR; this is not silent use of an uninitialized
  zero-value `AnalysisIR`. One residual soft telemetry item remains:
  pre-emit still logs a non-blocking mechanism-anchor advisory for
  `runTaskGraph` when the anchor is explained in section text rather than an
  item label. It did not reject, rewrite, or render a system support block, so
  it stays a lower-priority telemetry cleanup rather than a user-visible fix.
- Reviewer-depth and recovered-line repair follow-up:
  `read_combo_analyze_retry_anchor-20260521-172008` PASSed after broadening the
  semantic-quality depth audit from list-only item surfaces to citation-backed
  structured items in summary/section/scalar/decision/list/table blocks. This
  directly addresses the false "label-only" pressure without scanning model
  prose or weakening deterministic citation checks. The rerun
  `read_combo_analyze_retry_anchor-20260521-172824` also PASSed with
  `finalizer_iters=1`, `semantic_quality_concerns=0`, and no成文打回. It still
  spent `explorer_iters=30` / `midloop_inject=15`; log inspection showed the
  remaining cost is concentrated in exact-anchor repair and forced reads rather
  than finalizer/schema churn. During that audit, a recovered-line drift case
  was found: the tool could recover a claimed line to a stronger symbol
  definition line but only expose the adjusted line as the repair target. The
  repair contract now carries both original and adjusted lines and tells the
  model to choose a line-local anchor or the recovered definition explicitly.
  This keeps grounding safety intact while removing a source of validator/model
  confusion.
- Line-local grounding follow-up from the same log: one avoidable repair loop
  came from a correct condition line plus exact snippet, but with a descriptive
  `anchor_symbol` (`for loop`). The tool had enough typed evidence to ground
  the line itself, yet still downgraded through `snippet_fuzzy` and asked the
  model to reread/re-emit. The grounder now treats exact `snippet` or
  structured `condition` text as Tier-1 support for condition/return/
  assignment/initializer anchors when the line is already in read history.
  This deliberately excludes free-form `summary`, so model prose cannot bypass
  source-line grounding. Tool instructions were also tightened to ask for a
  visible line token as `anchor_symbol` and exact snippet/structured fields
  when no durable symbol exists. Targeted guards are in
  `TestGroundItem_LineLocalAnchorUsesExactSnippetWhenSymbolIsDescriptive` and
  `TestGroundItem_LineLocalAnchorDoesNotUseSummaryOnly`.
- Mechanism completeness follow-up: `read_combo_analyze_retry_anchor-20260521-
  181609` showed that demoting mechanism `member_set` alone was insufficient
  because the analyzer's quoted completeness requirement still made
  `ResolveQuestionFamily` choose `QFEnumeration`. The fix is now at the typed
  contract boundary: coverage-only mechanism obligations stay visible to
  finalizer as coverage context, but `HasPrincipalAnswerSetObligation` is false,
  so extractor does not require `emit_answer_symbol` and finalizer does not see
  `Principal Enumeration Rows`. The extractor skill was also corrected so
  `sub_topics` alone are guidance, not a hard anchor slate trigger. Replay
  `read_combo_analyze_retry_anchor-20260521-183444` PASSed with
  `family=generic`, no `emit_answer_symbol`, no visible system补表,
  `finalizer_iters=1`, `semantic_quality_concerns=0`, and
  `enumeration_push=0`. The eval metric was tightened to count visible
  `Principal Enumeration Rows` / system补表 paths rather than the fixed
  finalizer prompt heading `Enumeration completeness`.
