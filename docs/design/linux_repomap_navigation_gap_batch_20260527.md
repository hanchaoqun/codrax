# Linux RepoMap Navigation Gap Batch (2026-05-27)

## Context

真实验证场景：

```text
repo: ../linux
question: 梳理 Linux 内核中 eBPF map 更新路径：从 bpf 系统调用入口到具体 map ops 的 update_elem 分发，再到 hash map 和 array map 的实现函数，说明关键结构体、注册/分发表和调用关系，并输出流程图。
log: /Users/han/opt/codrax/.codrax/logs/linux-3cd4b8d9/codrax-20260527-014956-000-45493.log
output: /Users/han/opt/codrax/.codrax/output/20260527-015357.225-45493.md
```

这次运行证明 typed repo_map navigation policy 已进入 analyzer/explorer prompt，但“模型自然、连续、高效地使用 repo_map 两阶段导航”还没有闭环。

最新复测补充：

```text
log: /Users/han/opt/codrax/.codrax/logs/linux-3cd4b8d9/codrax-20260527-033642-000-61838.log
output: /Users/han/opt/codrax/.codrax/output/20260527-034602.643-61838.md
```

该轮进一步证明 repo_map policy 已能触达 explorer 的第一轮工具调用，但还有四个更高优先级的系统契约问题：

- `source_inventory_profile` 是对象字段，通用 strict-decode 修复却提示模型改成数组，导致 analyzer 两轮返工并显式抱怨 schema 矛盾。
- `principal_span_waiver` 已被模型用 typed enum 正确声明且系统接受后，旧的 `pre_complete.call_chain_principal_span` pending read 仍阻塞 closure，继续强迫模型读取无关辅助区间。
- Mermaid 兼容层或模型 payload 里的 `@[hidden]` 仍可能在最终图里渲染为 `codraxNode1[hidden]`，把系统内部修复痕迹泄漏给用户。
- call-chain 答案仍出现通用“枚举类条目证据稍弱” caveat，说明 enumeration 软告警仍有路径压过 typed call-chain family。

## Red Lines

- 系统不能用自己的结构偏好覆盖用户意图或模型已经写好的答案。
- repo_map、evidence repair、finalizer caveat 都只能提供确定性修复或软指导，不能因为启发式判断硬卡模型。
- 证据池中已落地、已读、定义锚点、符合 scope 的高价值证据及其 summary 不得被下游压扁、丢失或无理由降级。
- 外部观察、日志、trace、VCS、命令结果、MCP/connector 文档等证据必须通过统一证据/observation 通道传递，不能强行伪装成 repo file:line。

## Findings And ROI Order

### P0. strict-decode JSON-string repair must distinguish object vs array carriers

Symptom:

- Analyzer emitted `source_inventory_profile` as a JSON-encoded string.
- Tool rejected it with: `source_inventory_profile field must be a native JSON array of objects`.
- The model followed that instruction and re-emitted an array, then Go rejected the array because the schema field is actually a single object.
- The model explicitly reported the two errors as contradictory and dropped the optional typed profile.

Root cause:

- `RemapStrictDecodeError` only recognized "cannot unmarshal string into Go struct field ..." as an array-carrier artifact.
- The helper was originally designed for fields like `blocks[]`, but later object carriers such as `source_inventory_profile` reused the same remap path.
- This is a system schema/prompt inconsistency, not a model error.

Design:

- Parse the expected Go JSON target shape from the decode error.
- If the target type is `[]...`, render native JSON array guidance.
- If the target type is a struct pointer / map / object carrier, render native JSON object guidance.
- Keep the structured `ToolRepair` generic (`native JSON array/object`) so existing repair consumers still work.
- Add tests for both `blocks` array and `source_inventory_profile` object.

### P0. principal-span waiver must retire stale call-chain span pending reads

Symptom:

- The model correctly identified that `kernel/bpf/hashtab.c:2059-2354` and `kernel/bpf/arraymap.c:693-802` were iterator/batch auxiliary ranges, not the user's syscall-to-ops direct path.
- It emitted `principal_span_waiver.reason=no_intermediate_user_code`; the tool accepted the waiver.
- The same tool result still downgraded closure with a forced-read list for `pre_complete.call_chain_principal_span`.

Root cause:

- `callChainPrincipalSpanDowngrade` checks the typed waiver and stands down, but the earlier repair had already been mirrored into `PendingReads`.
- The pending-read check runs before the later call-chain span gate and did not reclassify `pre_complete.call_chain_principal_span` when a valid waiver is active.

Design:

- Keep the typed waiver validation strict.
- When a valid principal-span waiver is active, treat pending reads whose normalized origin is `pre_complete.call_chain_principal_span` as non-blocking advisory debt.
- Do not globally clear unrelated pending reads; other deterministic file obligations must still block.
- Add a pre-complete test proving an accepted waiver does not loop on stale span pending reads.

### P0. call-chain principal span gate must not infer impossible file-local spans from loose endpoint matches

Symptom:

- The span gate inferred a late range inside `hashtab.c` "between bpf_map_ops and update_elem" even though the actual implementation is at line 1171 and the registration table is at line 2355.
- The model had already emitted the meaningful relation: registration table points to implementation. Forcing the intervening file-local range inflated the answer with auxiliary iterator/batch code.

Root cause:

- `callChainPrincipalSpanDemandForEvidence` groups evidence by source and line order, then finds endpoint matches by loose code-term containment.
- That is safe only for actual source-to-sink spans where the same file contains a sequential call path. It is unsafe for registration/dispatch tables where the relation is pointer/metadata based and the implementation can appear before the table.

Design:

- Span-demand hard gates may only use precise call-edge/path evidence, or a source file span where ordered evidence items already demonstrate a path inside the range.
- Registration/implements evidence should satisfy call-chain boundary through relation/member-set support, not create a synthetic "read everything between definition and table" demand.
- For ambiguous same-file registration spans, emit at most advisory guidance; never hard-block closure.

### P0. Mermaid hidden synthetic nodes must not leak to rendered answers

Symptom:

- The final answer rendered:
  `syscall_entry codraxNode1[hidden]`.
- The original finalizer payload contained `syscall_entry @[hidden]`, which the unsafe-node normalizer aliased into a visible synthetic node.

Root cause:

- The Mermaid repair pipeline correctly handles many syntax slips, but does not treat `@[hidden]` / hidden marker lines as non-renderable layout hints.
- Unsafe-node aliasing turned an invalid hidden marker into a visible node definition.

Design:

- Add a flowchart-level normalization pass before unsafe-node aliasing that removes standalone hidden-marker lines (`node @[hidden]`, `node @hidden`, and generated `codraxNodeN[hidden]` shapes).
- Do not remove valid edges or user-visible node labels; only standalone hidden layout markers are affected.
- Add tests for model-emitted `@[hidden]` and for not touching legitimate labels containing the word "hidden".

### P1. Call-chain answers must not surface generic enumeration caveats

Symptom:

- The accepted call-chain answer still ended with an enumeration-support caveat.

Root cause:

- Some ordered path surfaces share list/table carriers with enumeration answers.
- The caveat materializer still has a path where soft enumeration oracle findings are rendered without rechecking typed call-chain/trace family.

Design:

- Preserve telemetry and soft oracle records.
- Suppress user-visible enumeration caveats for typed `ReqCallChain`/trace path answers unless the current request also has explicit source-inventory/member-set enumeration obligations.
- Add a regression test using a call-chain answer with ordered-list blocks and a soft enumeration oracle.

### P0. In-range `citation_ref` auto-repair can overwrite model intent incorrectly

New symptom from the focused rerun:

- The finalizer produced a complete one-shot answer, but `emit_answer_document` logged
  `repaired 10 item citation_ref value(s) by typed label/citation corroboration`.
- The rendered "关键符号锚点" list then attached visibly wrong citations, for example
  `map_update_elem` was shown with `include/linux/bpf.h:83`, and `bpf_map_ops` was shown
  with `kernel/bpf/syscall.c:6359`.

Root cause:

- The repair layer was allowed to rewrite already-in-range `citation_ref` values through
  broad label/citation corroboration. In a call-chain answer, several nearby symbols share
  surfaces such as `bpf`, `map`, `update_elem`, `ops`, and the system's fuzzy candidate
  search can choose a structurally plausible but semantically wrong citation.
- This violates the red line: system repair must not replace model-visible answer support
  unless the replacement is 100% deterministic.

Design:

- Keep deterministic repair for out-of-range `citation_ref` and for exact, unique
  source-location/member support refs.
- For already-in-range refs, only rewrite when the item surface itself names an exact
  file:line that uniquely maps to a citation, or when a unique exact endpoint-definition
  evidence row exists. Do not fall back to first fuzzy candidates.
- If ambiguity remains, leave the model payload intact and let pre-emit diagnostics ask for
  a repair rather than silently accepting a wrong support link.

### P0. Typed repo_map policy is visible but grep teaching still wins the first move

New symptom from the focused rerun:

- Analyzer classified the request as `question_kind=call_chain`, `predicate_axis=call`,
  `diagram_hint=call_dag`.
- The assistant prose said "Let me use repo_map", but the actual analyzer/explorer tool
  calls used `grep` / `read_file`; no `repo_map` call happened in the rerun.

Root cause:

- The prompt already renders `Typed Repo Map Route Hints`, but the later generic
  non-English search instruction still says to batch translated terms as parallel `grep`
  calls. That sentence is more concrete than the typed repo_map policy, so the model follows
  it even for relation/call-chain shapes where repo_map should be the cheap structural
  navigation pass.

Design:

- Keep the policy soft, typed, and language-neutral.
- Add an explicit "typed first hop" inside the repo_map primer: for call/relation/flow
  shapes, use `repo_map(view="task_map", query=typed terms)` before broad grep when exact
  files are not already pinned, then `relation_map` around chosen sources/scopes.
- Rewrite the non-English instruction so bilingual term expansion applies to text search
  (`grep`) and does not override typed repo_map navigation.

### P1. Dotted / member initializer anchor tokenization still creates repair loops

New symptom from the focused rerun:

- The model emitted `anchor_symbol="map_update_elem"` for the visible line
  `.map_update_elem = array_map_update_elem,`.
- The grounder rejected it because the current tokenizer treated `.map_update_elem` as not
  containing the whole-word token `map_update_elem`.

Root cause:

- The compatibility layer can normalize semantic registration evidence to initializer
  evidence, but the line-text anchor matcher still fails on punctuation-prefixed member
  names in some language surfaces.

Design:

- Fix the shared grounder, not the Linux case: identifier extraction from visible source
  lines should accept punctuation-prefixed member/designator names when the extracted
  identifier token is exact.
- Cover C/C++ designated initializers plus Go / TypeScript / ArkTS / Cangjie / Kotlin style
  member initializers through the same visible-line rule.

### P0. Failed `emit_hypothesis_verdict` can be masked by an older auto-verdict

Symptom:

- Extractor round emitted `emit_hypothesis_verdict(status=confirmed)` without citation and was rejected.
- Later log still printed `applied 1/1 hypothesis verdicts to IR`.

Root cause:

- `emit_hypothesis_verdict` itself does not append rejected payloads.
- The buffer already had deterministic/auto verdicts. `hasPendingHypotheses(ctx)` returned false, so extractor stopped even though the model attempted a stronger verdict and that attempt failed.
- The downstream drain log is technically applying the older auto verdict, but the user/developer reading the log can reasonably interpret it as “the rejected verdict was applied”.

Design:

- Treat a failed `emit_hypothesis_verdict` as an attempted override. If the same turn has no successful verdict call and retry budget remains, extractor must request a bounded repair even if auto verdicts already cover the hypothesis.
- The repair hint should ask for a concrete citation/evidence id, or an explicit inconclusive verdict when no anchor exists.
- Do not discard auto verdict fallback; only prevent it from silently masking a rejected model improvement.

### P0. Generic acceptance caveat leaks internal soft checks to users

Symptom:

- Final answer had no finalizer retry, but two soft oracle violations produced:
  `答案在部分验收检查上未达到预期标准，结果中的相应部分可能需要补充验证。`

Root cause:

- Multiple legacy/soft violation kinds map to `CaveatFamilyAcceptance`.
- The accept path materializes this generic family even when the violation is only an advisory and has no user-actionable localized detail.

Design:

- Suppress generic acceptance-family caveats on the soft accept path unless a more specific actionable family/detail is available.
- Keep hard-cap residual concern disclosure unchanged.
- Keep operator telemetry and strict-promotion behavior unchanged.

### P1. Evidence grounding for designated initializer registrations is not smooth enough

Symptom:

- `.map_update_elem = htab_map_update_elem` / `.map_update_elem = array_map_update_elem` registration evidence needed extra repair loops.

Root cause:

- Model sometimes uses semantic `anchor_kind=registration`, but the visible source line is a field/designated initializer.
- The grounding layer already supports initializer-like anchors, but the compatibility bridge from registration evidence to initializer surface anchor is incomplete.

Design:

- Reuse the existing grounding/read-history path.
- Only when the referenced source line is already read and visibly contains a field/designated initializer or assignment, normalize semantic registration anchor attempts into an initializer-compatible anchor.
- Do not guess line numbers or fabricate source text.
- Add language-agnostic tests around C/C++ designated initializer shape; keep the rule based on visible source syntax, not Linux/eBPF-specific names.

### P1. `repo_map(task_map)` result should nudge the concrete second hop

Symptom:

- For a call-chain/call-dag request, prompt recommended `task_map -> relation_map`, but after `task_map` the model went back to grep/read_file.

Root cause:

- Navigation policy was visible only in prompt. The `task_map` tool result did not carry a concrete next-step reminder tied to the current query/result.

Design:

- After query-bearing `task_map` results, when typed policy includes relation/call path routes, append a short advisory naming the concrete next views to try (`relation_map`, `call_path`) and how to scope them.
- Keep advisory soft and model-visible, not a hard gate.
- Hide internal implementation details.

### P2. `repo_map(task_map)` ranking is too noisy on large multi-term queries

Symptom:

- Query `bpf syscall map update_elem dispatch` ranked unrelated files such as `drivers/pinctrl/...` and `arch/alpha/kernel/io.c`.

Root cause:

- Large-codebase terms like `map`, `hash`, `array` can dominate or pollute ranking without enough query-term specificity.

Design:

- Improve ranking with generic term salience/IDF-style weighting or exact symbol affinity.
- Do not hardcode eBPF/Linux names.
- Do not remove recall: low-salience terms should contribute less, not be banned.
- Add an eBPF call-chain eval to prevent regressions.

### P0. Mermaid split edge-label repair can turn a bad label into a worse graph

New symptom from the latest focused rerun:

- The model emitted a malformed but understandable Mermaid edge label:
  `bpf_map_update_value -->|calls @ :261|:297| ops_dispatch`.
- `mermaidcompat.NormalizeSourceForMarkdown` then ran unsafe-node aliasing before
  handling the split label, producing
  `bpf_map_update_value -->|calls @ :261|codraxNode1[":297"]| ops_dispatch`.
- The final answer therefore kept an invalid edge and a synthetic `codraxNode`
  artifact in the user-visible diagram.

Root cause:

- The Mermaid compatibility layer already repairs unsafe endpoints, quoted labels,
  dangling punctuation, and shape closer mismatches, but it did not first repair
  the common split-pipe edge-label slip `A -->|label|extra| B`.
- Because `:297` is not a valid Mermaid node id, the unsafe-node repair treated it
  as a node endpoint instead of the continuation of the edge label.

Design:

- Add a source-level, meaning-preserving Mermaid repair before unsafe-node aliasing:
  when a flowchart/graph edge has multiple adjacent pipe-delimited label fragments
  and no intervening arrow, merge those fragments into one quoted edge label.
- Preserve valid chained edges such as `A -->|x| B -->|y| C`.
- Do not infer code semantics or file/line citations; this is purely a Mermaid
  syntax compatibility repair and benefits every language/domain that emits
  Mermaid diagrams.

### P1. Call-chain answers still receive generic enumeration caveats

New symptom from the latest focused rerun:

- The accepted call-chain answer ended with:
  `枚举类条目中部分项的证据支持稍弱，请按需对该类别的列表进一步核实完整性。`
- The user did not ask for an exhaustive enumeration; the ordered list was the
  requested path/hop surface.

Root cause:

- `principalEnumerationSurfaceRequested` can fall through to
  `contract.HasOutput(AnswerRequestedOutputEnumeration)`.
- Some trace/call-chain contracts use ordered-list-like outputs, so a soft
  enumeration-label oracle can be materialized even though the typed request is
  a call path, not a principal member inventory.

Design:

- Treat typed `ReqCallChain` / trace path answers as non-enumeration surfaces unless
  the analyzer also emits explicit enumeration intent/category flags or a source
  inventory/relation member-set obligation.
- Keep true enumeration/source-inventory/member-set questions unchanged.

### P0. Invalid in-range citations with no safe replacement should not render as proof

New symptom from the post-T16/T17 focused rerun:

- `pre-emit structural accepted as soft advisory` correctly detected
  `block="hops" item="h1" label="sys_bpf (BPF_MAP_UPDATE_ELEM)"`
  had `current_citation=kernel/bpf/syscall.c:1805 candidate_citations=[]`.
- The accepted answer still rendered that invalid citation beside the first hop, making
  line 1805 look like proof for the syscall switch entry.

Root cause:

- T9 stopped unsafe fuzzy replacement, which is correct, but the no-candidate branch still
  leaves the invalid citation attached.
- A retry would be too expensive for this low-level support-link issue, but rendering a
  known-invalid citation is worse than leaving the row uncited.

Design:

- Do not alter model prose, labels, ordering, or diagram.
- If the current citation is in-range and the cited evidence is present, but the label/text
  fails exact endpoint/source-location/aggregate alignment and there is no safe replacement
  candidate, detach only that `citation_ref` (`-1`).
- This is a conservative local repair: it removes a false proof link instead of guessing a
  new one. If a candidate exists, the existing deterministic repair/hint path still owns it.

### P1. `task_map` needs a clustered navigation surface, not only a flat file list

Question raised during the rerun review:

- The problem has two independent dimensions:
  1. The `repo_map(task_map)` user/model-facing result currently exposes a flat candidate
     list. In large repos this can look noisy even when the underlying graph has useful
     structure.
  2. The candidate ordering itself still needs a clearer, typed ranking policy.

Root cause:

- A flat list makes each noisy file look equally important, so the model falls back to
  grep/list_files when the first page contains broad-token matches.
- Ranking improvements help, but they do not teach the model how to turn a broad hit set
  into a subsystem-level navigation plan.

Design:

- Add a `task_map` top section that groups results by directory/subsystem/package/language
  cluster and reports why the cluster is relevant, representative anchors, and the concrete
  next hop (`relation_map`, `file_map`, or `read_file`).
- Keep detailed ranked files below the clustered summary for auditability and direct access.
- Ranking should combine typed request shape, exact symbol/query token salience, graph role
  (entrypoint/registration/implementation/caller/callee/import/export), read/grounding
  coverage, path/language affinity, and cluster diversity. It must remain language-neutral
  and work across the languages supported by repomap.
- This stays soft guidance; true empty results must not be disguised as a tool misuse.

### P1. `source_inventory` misuse needs tool-result guidance without exposing internals

Symptom from the latest focused rerun:

- Explorer called several repo_map views, then said "Source inventory didn't work" and
  reverted to grep/read_file.

Root cause:

- `source_inventory` is useful for inventory/enumeration/member-set surfaces, but call-chain
  questions usually need `task_map -> relation_map/call_path -> read_file`.
- The tool result does not yet clearly explain when a view is valid-but-low-signal versus
  when the model should switch to the typed route.

Design:

- Reuse the shared RepoMapNavigationPolicy in tool-result hints.
- If a legal view is low-signal for the current typed request, return a concise "try next"
  hint with exact parameter shape, without mentioning cache internals or implementation
  details.
- If the result is genuinely empty for correctly scoped parameters, say so plainly and do
  not push the model to search elsewhere by default.

### P1. Line-scope evidence needs clearer read-before-emit teaching

Symptom:

- In the eBPF run the model sometimes emitted line-scope evidence after navigation output
  but before reading the exact `arraymap.c` / `hashtab.c` gutter lines, causing repair loops.

Root cause:

- The evidence tool already validates exact `read_file` gutter lines, but the general
  exploration skill described repo_map/grep/read_file as a sequence without spelling out
  that repo_map/grep findings are navigation facts, not sufficient line-scope evidence.

Design:

- Keep the strict validator unchanged.
- Add model-visible teaching at the exploration skill layer: line-scope `emit_evidence`
  must come after `read_file` on the selected file/range, copying the exact gutter line.
- This benefits all languages because it is about evidence provenance, not source syntax.

### P2. Hypothesis verdicts should prefer accepted `evidence_id`

Symptom:

- Extractor first tried `emit_hypothesis_verdict(status=confirmed)` with a handwritten
  comma-separated citation string and only succeeded on the second turn with evidence ids.

Root cause:

- The schema supports `evidence_id`, and the prompt mentions it, but the preferred path was
  not prominent enough when the accepted investigation snapshot already carries grounded
  evidence rows.

Design:

- Make the extractor skill explicitly prefer `evidence_id` for grounded evidence rows.
- Keep handwritten citations for a single exact repo file:line or exact artifact-local
  anchor. Do not add a new verdict mechanism.

### P1. Navigation benefit evaluation should choose between narrow grep and relation_map

Symptom:

- The Linux focused run proved that `repo_map` is now visible to analyzer/explorer, but
  the model sometimes uses a productive narrow `grep` instead of continuing into
  `relation_map`.
- That is not automatically wrong: for some languages, macro-heavy code, generated code,
  runtime wiring, or sparse relation graphs, grep/read_file can be the cheaper and more
  reliable route. The system must not force relation_map just because the request shape is
  structural.

Root cause:

- Current teaching says typed relation/call-flow shapes can use
  `task_map -> relation_map`, but it does not compare that route with the *current tool
  result*. A narrow grep with a few exact production lines should naturally lead to
  `read_file`; a broad/compacted grep with many candidates should suggest a structural
  narrowing hop if the typed policy already contains relation/call-path routes.
- This is a navigation-efficiency decision, not an answer contract. It must remain soft
  guidance and cannot create forced reads, retries, or absence proof.

Design:

- Reuse the existing `RepoMapNavigationPolicy`; do not add prose/keyword matching.
- When a grep result is narrow, keep the existing advice: read the selected file/range for
  exact line evidence.
- When grep is broad enough to trigger the retrieval governor and the typed policy has
  `relation_map` or `call_path`, add one concise `relation_navigation_hint` with concrete
  `sources=[...]` from the top production paths and candidate `relation_kinds=[...]` when
  available.
- When `relation_map` returns no rows, say this is not proof of absence and suggest
  targeted grep/read_file fallback for sparse graphs, macro/dynamic/runtime wiring, or
  missing structural extraction.
- These hints are language-neutral and multi-repo-safe because they use repo-relative
  paths already returned by the active tool result and the existing typed policy. They do
  not score correctness and do not override model judgment.

## Task List

- [x] T1: Add extractor repair path for rejected `emit_hypothesis_verdict` attempted overrides.
- [x] T2: Suppress generic acceptance caveat on soft accept path unless user-actionable detail exists.
- [x] T3: Add safe designated-initializer evidence anchor compatibility tests and implementation.
- [x] T4: Add query-bearing `task_map` follow-up advisory for typed relation/call-chain routes.
- [x] T5: Improve task_map ranking salience without Linux-specific matching.
- [x] T6: Add deterministic coverage for eBPF-style large-repo call-chain navigation/ranking and relation-map next-hop usage.
- [x] T7: Re-run focused eval and refresh this document with observed tool sequence, retries, and answer quality.
- [x] T8: Make typed call/relation repo_map first-hop guidance stronger than generic grep teaching, without turning it into a hard gate.
- [x] T9: Constrain in-range `citation_ref` repair to exact model-stated source locations or unique exact endpoint evidence.
- [x] T10: Make dotted/member initializer anchor matching language-neutral in the shared grounder.
- [x] T11: Re-run the Linux focused scenario after T8-T10 and record tool sequence, citation quality, evidence repairs, and answer caveats.
- [x] T12: Let grounded model-owned call-chain/mechanism completion boundaries demote generic `phase1_unread` ranker debt to advisory instead of forced reads.
- [x] T13: Soften the explorer "coverage before completion" prompt so only typed structural obligations require every candidate file to be read or excluded.
- [x] T14: Re-run the Linux focused scenario after T12-T13 and record whether explorer still reads ranker-collateral `topology` files, whether citation repair stays deterministic, and whether caveats remain actionable.
- [x] T15: Strengthen analyzer pre-scan teaching so structural location questions start with repo_map before files-only grep.
- [x] T16: Repair Mermaid split-pipe edge labels before unsafe-node aliasing and add regression tests.
- [x] T17: Suppress generic enumeration caveats for typed call-chain / trace-path answers unless the request is explicitly enumerative.
- [x] T18: Re-run the Linux focused scenario after T16-T17 and record diagram syntax, caveats, repo_map use, and citation repair count.
- [x] T19: Detach known-invalid in-range item citations when no safe replacement candidate exists; add regression tests.
- [x] T20: Add clustered `task_map` navigation output and record the ranking policy.
- [x] T21: Add low-signal repo_map view guidance using the shared navigation policy.
- [x] T22: Teach read-before-emit for line-scope evidence in the shared exploration skill.
- [x] T23: Teach extractor to prefer accepted `evidence_id` for hypothesis verdicts.
- [x] T24: Make strict-decode JSON-string repair shape-aware so object carriers are not mis-taught as arrays.
- [x] T25: Reclassify stale `pre_complete.call_chain_principal_span` pending reads as advisory after a valid principal-span waiver.
- [x] T26: Prevent call-chain span hard gates from inventing file-local ranges across static registration / initializer endpoints.
- [x] T27: Remove standalone Mermaid hidden-marker lines before unsafe-node aliasing.
- [x] T28: Tighten call-chain accepted-path caveat filtering so generic acceptance / enumeration telemetry does not surface as user-facing noise.
- [x] T29: Re-run the Linux focused scenario after T24-T28 and record analyzer retries, closure pending reads, Mermaid output, and caveats.
- [x] T30: Make query-bearing `task_map` ranking query-primary and structural-score-capped so high-centrality generic files cannot outrank precise request terms.
- [x] T31: Make line-scope evidence repair hints tool-history-aware: only say a line came from navigation/search rather than `read_file` gutter when `ObservedLineIndex` proves it.
- [x] T32: Suppress non-actionable call-chain accepted-path soft telemetry for optional `branch_guard` / diagram-edge fidelity, while preserving concrete citation caveats.
- [x] T33: Re-run the Linux focused scenario after T30-T32 and record repo_map ranking, evidence repair count, and final caveats.
- [x] T34: Make source-inventory target-role parsing alias/skip small role-name mistakes before relation-flow profile dropping, so ignored soft profiles do not force analyzer retries.
- [x] T35: Extend accepted call-chain caveat filtering to the actual root violation families observed in T33 (`enumeration_label_hallucinated`, diagram relation/endpoint fidelity).
- [x] T36: Re-run the Linux focused scenario after T34-T35 and record analyzer retries, final caveats, and whether any remaining finalizer JSON-string recovery retry is genuinely unsafe to auto-accept.
- [x] T37: Record navigation-benefit evaluation gap and design around broad grep versus relation_map.
- [x] T38: Add broad-grep-only relation navigation hint using the shared typed repo_map policy.
- [x] T39: Make relation_map empty/no-source guidance explicitly fall back to targeted grep/read_file without treating zero rows as absence proof.
- [x] T40: Add regression tests for broad grep relation hint, no hint on non-relation broad grep, and relation_map zero-row fallback wording.

## Progress

- 2026-05-27: Findings recorded from `linux-3cd4b8d9/codrax-20260527-014956-000-45493.log`; root-cause analysis started with extractor verdict masking and soft caveat materialization.
- 2026-05-27: T1 implemented in `extractorEvaluator.Observe`: a failed model-authored `emit_hypothesis_verdict` attempted override now gets one bounded repair turn even when older auto-verdicts already cover the hypothesis. Test added for the masking case.
- 2026-05-27: T2 implemented in the soft accept caveat filter: generic acceptance-family telemetry is suppressed on accepted answers, while concrete citation/coverage caveats still surface when actionable. Test added for call-chain answers.
- 2026-05-27: T3 implemented in `emit_evidence`: semantic `anchor_kind=registration` on an already-read designated/member initializer line is normalized to `anchor_kind=initializer` and re-grounded. The compatibility path requires visible source text and does not guess paths or line numbers.
- 2026-05-27: T3 cross-language guard expanded: the same compatibility test now covers C/C++ designated initializers, Go composite literal fields, TypeScript/ArkTS object members, Cangjie named members, and Kotlin named-argument style. The implementation remains syntax/visibility based, not language-name based.
- 2026-05-27: T4 implemented in `repo_map`: query-bearing `task_map` results now render the shared typed navigation policy when the request shape contains relation/call-path routes, so the tool result itself nudges `relation_map` as the next hop.
- 2026-05-27: T5 implemented in `retrieve.RankGraphScores`: query token salience downweights high-document-frequency generic terms while preserving recall. This is IDF-style and language-neutral; no Linux/eBPF token is special-cased.
- 2026-05-27: T6 covered by deterministic tests for eBPF-style noisy query ranking and query-bearing task_map relation next-hop rendering. Full LLM eval remains T7 because it depends on remote model behavior and wall-clock budget.
- 2026-05-27: T7 focused rerun completed at `/Users/han/opt/codrax/.codrax/logs/linux-3cd4b8d9/codrax-20260527-022438-000-49987.log`. It confirmed T1/T2 regressions were gone and the answer was broadly useful, but exposed three remaining system gaps: repo_map policy did not change the first tool choice, dotted initializer fields still caused an evidence repair loop, and broad in-range citation repair produced wrong user-visible citations.
- 2026-05-27: T8 implemented in `explorer` prompt construction. A typed "Repo Map First Hop" section now renders only when the structural request policy contains both `task_map` and relation/call-path routes. It tells explorer to use `repo_map(view="task_map")` before broad grep for typed relation/call-flow shapes, then `relation_map` around chosen sources/scopes. This is soft policy guidance, not a hard read obligation, and the generic bilingual grep instruction now explicitly says it applies only when using text search.
- 2026-05-27: T9 implemented in `answer_document_pre_emit_check`: in-range `citation_ref` repair no longer falls back to the first fuzzy label candidate. It can rewrite only when the visible item surface names one exact source location that uniquely maps to the document citation pool, or when existing deterministic exact-endpoint/aggregate alignment proves a unique match. Ambiguous explicit locations and ambiguous endpoint candidates remain model-authored rather than being silently changed by the system.
- 2026-05-27: T10 covered in the shared grounder with a cross-language tokenizer guard. The existing token extractor already accepts punctuation-prefixed member/designator tokens from visible read_file lines; tests now lock C/C++ designated initializers, Go composite literals, TypeScript/ArkTS object members, Cangjie named members, and Kotlin named arguments. The strict read_file requirement remains unchanged: grep-observed lines are navigation clues, not citation grounding.
- 2026-05-27: T11 focused rerun completed at `/Users/han/opt/codrax/.codrax/logs/linux-3cd4b8d9/codrax-20260527-024522-000-53699.log`. Analyzer/explorer now used `repo_map` in the first turns, and broad in-range citation rewrite dropped from the earlier 10-item repair to a single deterministic repair. However, the run exposed a higher-ROI forced-read gap: after a complete grounded eBPF call-chain handoff, `phase1_unread` forced the model to read unrelated ranker-collateral `topology` files (`drivers/platform/surface/...`, `arch/*/topology.*`). The model then emitted a rejected negative observation to justify the collateral. Root cause: `genericForcedReadBoundaryCanUseModelPrincipalSet` did not include typed `call_chain`, so already-grounded call-chain completions could not bypass generic ranker debt; and the explorer prompt still says every candidate file from grep/repo_map/list_files must be read or excluded for structural obligations without clearly limiting that to exhaustive/count/grouped coverage. T12-T13 track the generic fix; no Linux/eBPF term will be special-cased.
- 2026-05-27: T12 implemented by extending the existing grounded model-owned completion boundary to typed `call_chain` questions, including analyzer outputs that classify the answer as `intent=trace`. This does not disable exact required-file, explicit endpoint, history lookup, or current-source source-to-sink gates; it only prevents generic ranker debt (`phase1_unread`) from overriding already-grounded call-chain/mechanism evidence. Regression test `TestEmitInvestigationComplete_PreCompleteCheck_CallChainGroundedEvidenceBypassesGenericForcedReads` mirrors the Linux eBPF shape with unrelated ranker-collateral files and no Linux-specific branch.
- 2026-05-27: T13 implemented in `explore-skill`: the "coverage before completion" rule now explicitly applies to structural coverage obligations (exhaustive coverage, declared count, named-group partitions). Mechanism, architecture, and call-chain explanations are taught to read load-bearing files and treat collateral candidates as optional navigation hints unless such a coverage obligation is also declared. Test `TestExploreSkill_CoverageBeforeCompletionIsLimitedToStructuralCoverageObligations` guards the wording.
- 2026-05-27: T15 implemented in `analysis-skill`: mechanism/architecture/call-chain/handler/route/config/current-source pre-scan now says to start with `repo_map` for structural path discovery, and to use `grep(files_only=true)` only when repo_map is unavailable, empty/ambiguous, or an exact literal/path needs direct confirmation. This is still model-visible guidance, not a hard gate, and it does not inspect user/model prose. Test `TestAnalysisSkill_PromptTeachesRepoMapFirstForStructuralLocationPrescan` locks the wording.
- 2026-05-27: First T14 rerun with the pre-T15 binary at `/Users/han/opt/codrax/.codrax/logs/linux-3cd4b8d9/codrax-20260527-025940-000-55542.log` showed the remaining T12 gap precisely: analyzer emitted `intent=trace` + `question_kind=call_chain`, so the previous `intent=explain` boundary still failed to bypass generic `phase1_unread`; the model then spent extra turns proving `registry/topology` files were collateral. T12 was tightened to include typed `trace + call_chain` while still excluding history lookup / exact required-file gates. The rerun also confirmed T15 is necessary: analyzer still started with grep before repo_map under the older prompt.
- 2026-05-27: T14 rerun with the T12/T13/T15 binary completed at `/Users/han/opt/codrax/.codrax/logs/linux-3cd4b8d9/codrax-20260527-030818-000-57173.log`; output `/Users/han/opt/codrax/.codrax/output/20260527-031201.635-57173.md`. The forced-read regression is fixed: explorer no longer reopened `topology`/registry collateral after the grounded chain was complete. Citation repair dropped to 3 deterministic, visible file:line-driven corrections, and the rendered citations now align with the prose. Two new defects were exposed and promoted to T16/T17: the Mermaid repair layer converted a split edge label into a synthetic `codraxNode` artifact, and a generic enumeration caveat leaked into a non-enumeration call-chain answer. Analyzer still used grep/list_files in classification after seeing a broad precomputed task map; this is a P1 task-map quality/teaching issue, not a hard gate candidate, because explorer did use repo_map in its first navigation turns.
- 2026-05-27: T16 implemented in `mermaidcompat`: split-pipe flowchart edge labels are merged before unsafe-node aliasing, preserving valid chained edges. Targeted tests cover both repair and non-repair shapes.
- 2026-05-27: T17 implemented in the caveat materializer: typed call-chain / trace-path answers no longer receive generic enumeration-depth caveats unless the request is explicitly enumerative or carries a true member-set/source-inventory obligation.
- 2026-05-27: T18 rerun completed at `/Users/han/opt/codrax/.codrax/logs/linux-3cd4b8d9/codrax-20260527-032004-000-59337.log`; output `/Users/han/opt/codrax/.codrax/output/20260527-032451.976-59337.md`. The Mermaid `codraxNode` artifact is gone and the enumeration caveat is gone. Remaining issues promoted to T19-T21: one known-invalid item citation still rendered because there was no safe candidate, `task_map` still exposes too much flat-list noise for the model, and `source_inventory` low-signal behavior needs clearer next-step guidance.
- 2026-05-27: T19 implemented in `answer_document_pre_emit_check`: if an in-range item citation is proven misaligned, has cited evidence present, and has no exact safe replacement candidate, the system now detaches only that `citation_ref` instead of rendering a false proof link. Candidate-backed mismatches still go through the existing deterministic repair/hint path.
- 2026-05-27: T20 implemented in `repo_map(task_map)`: query-bearing task maps now render a top-level `Navigation Clusters` section before the detailed ranked files. Clusters group by directory/subsystem, list representative files and matched anchors, and show a concrete `relation_map` next hop. The original detailed candidate list remains unchanged below the cluster summary.
- 2026-05-27: T21 implemented for `source_inventory`: when a structural relation/call-flow request uses source_inventory without an active inventory profile, the result prepends a `Repo Map View Fit Hint` that points back to `task_map -> relation_map`. Analyzer-stage output remains compact and true inventory requests remain unchanged.
- 2026-05-27: T22/T23 implemented in shared skill prompts: exploration now explicitly says line-scope evidence must follow `read_file` gutter inspection, and extraction now says to prefer `evidence_id` when accepted grounded evidence already exists. Targeted tests passed for tool, repomap render/tool, and skill packages.
- 2026-05-27: T24 implemented in `RemapStrictDecodeError`: the repair now parses the decoder target shape and emits native JSON object guidance for fields like `source_inventory_profile`, while preserving native JSON array guidance for fields like `blocks`. The LLM-facing message does not expose Go type names. Regression tests cover both object and array carriers.
- 2026-05-27: T25 implemented in `partitionPendingReadsForAcceptedClosure`: once a typed `principal_span_waiver` is valid and active, stale pending reads from `pre_complete.call_chain_principal_span` become advisory debt instead of blocking closure. Other pending-read origins remain unchanged.
- 2026-05-27: T26 implemented in the call-chain principal-span demand builder: static binding endpoints (`registration`, initializer/assignment anchors, or typed register/implements predicates) can satisfy the relation boundary without creating a synthetic "read every line between definition and table" hard gate. This is language-neutral because it keys off structured evidence roles, not Linux tokens.
- 2026-05-27: T27 implemented in `mermaidcompat`: standalone hidden marker lines such as `node @[hidden]`, `node @hidden`, and generated `node codraxNodeN[hidden]` are removed before unsafe-node aliasing. Valid edges and real labels containing "hidden" are preserved.
- 2026-05-27: T28 tightened accepted-path caveat filtering: call-chain / trace-path answers still keep concrete citation grounding caveats when needed, but generic acceptance-family telemetry and generic enumeration caveats are suppressed unless the request is truly enumerative. Regression tests cover the mixed citation + acceptance + enumeration soft-violation case.
- 2026-05-27: T22 clarification after review: model-visible teaching remains generic ("repo_map/grep are navigation; line-scope evidence needs read_file gutter"). The system does not tell the model "you used grep/repo_map, not read_file" unless a future tool-history-aware repair can prove that exact provenance. This avoids misleading the model when it already read an overlapping range.
- 2026-05-27: T29 focused rerun completed at `/Users/han/opt/codrax/.codrax/logs/linux-3cd4b8d9/codrax-20260527-035940-000-64463.log`; output `/Users/han/opt/codrax/.codrax/output/20260527-040513.000-64463.md`. T24-T28 held: no analyzer schema contradiction, no stale principal-span pending-read loop, no Mermaid hidden marker, and no generic enumeration caveat. Remaining gaps were promoted to T30-T32: query-bearing `task_map` still ranked high-centrality generic files above `kernel/bpf`-like precise request surfaces, explorer still emitted some line-scope evidence from navigation/search lines before `read_file` gutter verification, and final rendering surfaced generic branch/diagram soft caveats even though the call-chain answer was accepted and useful.
- 2026-05-27: T30 design: for non-empty query, `repo_map` ranking must treat query relevance as the primary score and use structural centrality only as a bounded tie-breaker. This is language-neutral because it uses token salience and graph structure already present in the ranker, not Linux/eBPF-specific terms.
- 2026-05-27: T31 design: use the existing `ObservedLineIndex` as precise provenance. If a cited source:line is visible only in navigation/search output and absent from strict `read_file` gutter history, the repair note can say that. If the line was not observed there either, keep the generic read-file-history guidance to avoid misleading the model.
- 2026-05-27: T32 design: optional branch-guard and diagram-edge telemetry on an accepted typed call-chain / trace answer should not become generic user-facing caveats. Concrete grounding problems such as bad citations still surface; this keeps user-visible notes actionable rather than making good answers look unstable.
- 2026-05-27: T30 implemented in `retrieve.RankGraphScores`: query-bearing ranking now uses query match as the primary score and applies structural centrality only as a logarithmic tie-breaker. The prior multiplicative boost of the full structural score was removed for query mode. Regression coverage adds a high-centrality broad-token noise file to prove precise request terms still win without Linux-specific matching.
- 2026-05-27: T31 implemented in `ground`: ungrounded line-scope repair notes now distinguish "navigation/search output but not read_file gutter" only when the source:line is present in `ObservedLineIndex` and absent from strict `LineIndex`. Otherwise the note remains generic. Regression coverage proves grep-observed lines stay ungrounded and get the precise hint.
- 2026-05-27: T32 implemented in `repair_caveat_materializer`: accepted typed call-chain / trace-path answers suppress generic diagram-edge fidelity and optional branch-guard coverage caveats, while still retaining concrete citation-grounding caveats. The suppression is keyed off structured request shape and facet/violation types, not user/model prose.
- 2026-05-27: T33 focused rerun completed at `/Users/han/opt/codrax/.codrax/logs/linux-3cd4b8d9/codrax-20260527-041356-000-66680.log`; output `/Users/han/opt/codrax/.codrax/output/20260527-041937.018-66680.md`. Improvements: explorer used `repo_map` in the first two turns and followed with read-before-reemit repairs; stale principal-span forced reads did not return. Remaining gaps promoted to T34-T36: analyzer still retried once because `source_inventory_profile.target_roles` used `struct_field` even though that profile was later ignored for a call-chain relation flow; final caveats still showed generic diagram/enumeration notes because the actual root violations were `enumeration_label_hallucinated` and diagram relation/endpoint family members not covered by T32; finalizer first emitted `blocks` as a JSON-encoded string and the existing recovery correctly refused lossy acceptance.
- 2026-05-27: T34 implemented in the shared candidate-role parser and `emit_analysis`: `struct_field` / object-field aliases normalize to `field`, and unknown source-inventory target roles are skipped with warnings instead of causing a hard analyzer retry. Relation-flow profile dropping still clears the whole `source_inventory_profile` after parsing, so call-chain answers are not flattened into inventories.
- 2026-05-27: T35 implemented in the soft-caveat filter: typed call-chain / trace answers now suppress generic enumeration-depth caveats for `enumeration_label_hallucinated` and suppress diagram relation/endpoint telemetry when the answer has already been accepted. Concrete citation caveats still surface. Regression coverage now includes an active but ignored source-inventory profile to guard the exact T33 shape.
- 2026-05-27: T36 focused rerun completed at `/Users/han/opt/codrax/.codrax/logs/linux-3cd4b8d9/codrax-20260527-042551-000-68631.log`; output `/Users/han/opt/codrax/.codrax/output/20260527-043043.849-68631.md`. The analyzer completed without schema retry, finalizer emitted a native `blocks[]` payload on the first attempt, there were no user-visible generic "图示中..." / "枚举类..." system caveats, and no forced reads were raised. One internal `diagram_edges` soft oracle remains as operator telemetry only (`finalize={retries:0, violations:2}` with no user-facing caveat); this is acceptable for this batch because the requested Mermaid diagram rendered and the accepted prose/ordered list carried the principal call path.
- 2026-05-27: T37-T40 implemented as a soft navigation-benefit evaluator. Broad/compacted grep results now consult the shared typed repo_map policy and, only for relation/call-flow shapes, append a concrete `relation_navigation_hint` with top production `sources` and candidate `relation_kinds`. Narrow grep results keep the existing read_file next step, and non-relation broad grep does not mention relation_map. Empty relation_map results now explicitly say zero rows are not absence proof and recommend targeted grep/read_file fallback for sparse graph, macro/dynamic/runtime wiring, or missing extraction. Regression tests cover relation-shaped broad grep, non-relation broad grep, and empty relation_map fallback wording.
