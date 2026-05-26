# Explorer Completion Convergence Contract (2026-05-27)

## Status

Batch 5 complete; Batch 6 repo_map-efficiency follow-up remains planned. This document tracks the repo-map navigation convergence gap found in the Linux `io_uring` validation run and the broader forced-read audit required by the red-line rule: only precise, deterministic unread-file obligations may hard-block model completion. Ranker / repo_map / pre-scan / broad graph hints are navigation signals and must remain soft once the model has already established a grounded completion boundary.

## Incident

Validation run:

```text
repo: ../linux
log: .codrax/logs/linux-3cd4b8d9/codrax-20260526-235421-000-24909.log
request: 梳理 io_uring 的网络 SEND/RECV 类请求从 opcode 注册到实际执行函数、关键数据结构，再到 socket 层调用的结构关系，并输出流程图。
```

The explorer used `repo_map(view="task_map")` naturally, without the user naming repo_map. By round 18 the model had already emitted a high-confidence structured completion boundary:

- `aggregate_facts.member_set` for SEND/RECV opcode registration.
- `aggregate_facts.member_set` for io_uring to socket call functions.
- `aggregate_facts.member_set` for socket-layer functions.
- `aggregate_facts.member_set` for core data structures.
- A rich Chinese completion reason describing the full chain.

However `emit_investigation_complete` downgraded completion three times:

1. Round 18: forced reads for `io_uring/io_uring.c` and `io_uring/cmd_net.c`.
2. Round 26: forced reads for `io_uring/rw.c` and `io_uring/cmd_net.c`.
3. Round 32: forced read for `io_uring/uring_cmd.c`.

These files came from generic pre-complete breadth/anchor machinery, while the model-owned answer boundary was already grounded in the principal SEND/RECV evidence. The result was extra exploration rounds, larger prompt history, and risk that tangential support files would be pulled into the final answer as if they were principal.

## Root Cause

The current pre-complete path has a split-brain contract:

- `internal/types/repair.go` already defines repair-debt classes and says only `RepairDebtPrincipalBlocking` should hard-block accepted closure.
- `internal/tool/emit_investigation_complete.go` still renders every live `PendingRead` as a hard forced-read closure blocker.
- Generic forced-read producers (`pre_complete.primary_anchor`, `phase1_unread`, `pre_complete.multi_path_anchor`) are fed by ranker/pre-scan/graph breadth signals. Those signals are useful navigation guidance, but they are not precise enough to override a model-emitted, grounded principal completion boundary.
- For single-topic mechanism explanations, `PrincipalAggregateMemberSetFactRefsForRequest` intentionally demotes `member_set` away from deterministic final-answer tables. That is correct for answer rendering, but the same demotion accidentally prevents completion gating from seeing the model's structured boundary.

This violates the repository red line: precise signals may hard-gate; noisy ranker/pre-scan signals must remain soft guidance once the model has provided a grounded principal boundary.

## Contract

1. Model-owned completion boundary is authoritative for generic breadth reads when all of these typed conditions hold:
   - The completion carries one or more `aggregate_facts` with explicit `role="principal_answer"`.
   - The facts are `member_set` facts with at least two members.
   - Each member is usable through grounded evidence, support refs, read-file line references, source-inventory support, or existing answer-symbol support.
   - The request is an explanation/mechanism narrative shape, not an exhaustive enumeration/source-inventory/relation/count handoff that has its own precise closure contract.

2. Generic forced-read debt becomes advisory under that boundary:
   - `phase1_unread`
   - `pre_complete.phase1_unread`
   - `pre_complete.primary_anchor`
   - `pre_complete.multi_path_anchor`

3. Precise hard blockers still hard-block:
   - exact target absence/positive-substitute guards
   - exhaustive enumeration/source inventory member-set handoff
   - typed relation member-set handoff
   - call-chain principal span/endpoint gates
   - required current-source files in mixed artifact/history + current-source answers
   - unverified path citations
   - citation/grounding floors

4. The fix must not generate or replace final-answer content. It only decides whether generic exploration should continue. The finalizer remains model-owned and evidence-grounded.

## Forced-Read Source Matrix

All producers flow through `EvidenceClosure.PendingRead`, so the contract is enforced at the queue consumer, not by case-by-case prompt wording.

Hard blocking is allowed only for deterministic obligations:

- `required_file_hint_unread` / `pre_dispatch.required_file_hint_unread`: analyzer typed current-source requirement for mixed history/runtime + current-code answers.
- `pre_complete.call_chain_principal_span` and `pre_complete.call_chain_qualified_intermediate`: typed source-to-sink endpoint/span evidence gaps.
- `pre_complete.citation_floor_support_refs`, `emit_investigation_complete.tier1_floor`, and answer-grounder `RepairReadFile`: the model already emitted a citation/support location and the source line must be read or materialized.
- exact absence / config trace / field-value / source-inventory / typed relation / change-impact handoffs when the target set is precise and typed.
- ungrounded evidence repair for a model-emitted evidence row tied to an already named source.

Soft/advisory after a grounded model-owned completion boundary:

- `pre_complete.primary_anchor`: exact-entity ranker/pre-scan first hit. It may be useful, but a same-token match can land on support tools or userspace helpers (`tools/include/io_uring/mini_liburing.h`) instead of the user’s actual answer path.
- `phase1_unread` / `pre_complete.phase1_unread`: top-K breadth files from keyword/ranker navigation.
- generic `pre_complete.multi_path_anchor`: cross-file parity hints unless it is tied to a typed call-chain endpoint or model-emitted evidence surface.
- `chain_promotion.concrete_values_tracer*`: deterministic scan hints that are not model-authored citations.

The system must not parse user prose or model `<think>` to decide this. The completion boundary can be established only by typed request traits plus grounded/citable evidence, or by structured aggregate facts whose members are supported by grounded evidence.

## 2026-05-27 Follow-up Validation

After Batch 1, the same Linux `io_uring` validation run completed with a good final answer and efficient `repo_map(view="task_map")` usage, but still showed a generic forced-read false positive:

- Log: `.codrax/logs/linux-3cd4b8d9/codrax-20260527-002652-000-30624.log`
- Round 18 `emit_investigation_complete` had 38 grounded evidence items and a high-confidence Chinese completion reason.
- The system queued `pre_complete.primary_anchor` for `tools/include/io_uring/mini_liburing.h`.
- The model explicitly complained that it had to read the pre-scanned file only to satisfy the forced-read requirement.

This means explicit `aggregate_facts.role="principal_answer"` is not the only valid completion boundary. For narrative mechanism/architecture explanations, a sufficiently rich grounded evidence boundary must also demote generic forced reads to advisory, while preserving deterministic hard blockers listed above.

After Batch 2, the Linux scenario completed without a generic forced-read downgrade:

- Log: `.codrax/logs/linux-3cd4b8d9/codrax-20260527-004358-000-34163.log`
- `repo_map(view="task_map")` was used naturally for the `io_uring send recv opcode` navigation question.
- `emit_investigation_complete` logged `generic forced-read gates bypassed by grounded model-owned completion boundary`.
- The final answer was good and included the requested flowchart.

However the upstream explorer prompt still rendered a ranker/graph candidate as hard language:

```text
Read tools/include/io_uring/mini_liburing.h first. This is the receiver-aware primary target for the user question.
```

That wording came from `buildPrimaryEntityDepthStartInstruction`, not from the completion gate. The source signal is `uniquePrimaryEntityFile()`, which is a graph navigation shortcut. It can point at support/tool/helper paths and therefore must be phrased as a candidate focus only. The prompt must tell the model to skip the candidate when it is visibly outside the user-requested answer path and navigate with `repo_map` / focused grep instead.

Batch 3 removed that primary-entity hard language. A follow-up run then exposed the same contract violation in the more generic pre-scan/ranker lane:

```text
### Analyzer's Required Files
... identified these files as structurally relevant. Start your investigation here ...
- tools/include/io_uring/mini_liburing.h

### Pre-scanned File Ranking (TOP PRIORITY)
These are the TOP PRIORITY files to investigate — read them first ...
```

The model did the right thing after a second repo_map/grep pass and did not read the userspace helper, but the prompt still framed ranker candidates as first-class obligations. This is a systemic prompt contract bug, not a model bug: `EvidencePlan.RequiredFiles` can contain both precise current-source anchors and soft structural ranker hits. Only validated log frames / typed current-source required-file hints are deterministic read obligations; ranker/repo_map candidates must remain advisory navigation hints.

Batch 4 removed the generic hard-read wording and reran the same Linux scenario:

- Log: `.codrax/logs/linux-3cd4b8d9/codrax-20260527-010319-000-37159.log`
- The explorer prompt rendered `### Pre-scanned Navigation Candidates` and explicitly said the rows are candidate routes, not read obligations.
- `emit_investigation_complete` logged `generic forced-read gates bypassed by grounded model-owned completion boundary`.
- The model did not read `tools/include/io_uring/mini_liburing.h` as a forced-read obligation.
- The final answer was useful and included the requested Mermaid flowchart.

The run exposed a second path of the same class in the extractor/verdict stage:

- The extractor prompt still said `Files the investigation read (authoritative citation source)` and the default skill said current-repo confirmed/rejected verdicts require a repo file:line citation.
- Several accepted deterministic evidence rows were grounded from repo_map/grep and carried concrete file:line anchors (`net/socket.c`, `include/uapi/linux/io_uring.h`) but were not in `read_file` history.
- The extractor model therefore marked the hypothesis `inconclusive`, even though the finalizer had enough grounded evidence and produced a good answer.

This is not a model error. It is a contract mismatch: once `emit_evidence` has accepted a row as grounded/citable, downstream extractor/verdict code must not force a redundant read_file just to justify a hypothesis verdict. Batch 5 fixes this by exposing `evidence_id` in the extractor prompt and by letting `emit_hypothesis_verdict` accept citations that are already covered by accepted grounded evidence.

Residual repo_map efficiency gap from the same run:

- Analyzer round 2 batched `list_files, grep, repo_map` after it had effectively reached terminal classification, so `grep` and `repo_map` were rejected with “classification is already in terminal emit mode”.
- Explorer did use `repo_map(view="task_map")` naturally, but it passed no query in round 1, so it behaved as a broad orientation map rather than a tighter two-stage navigator.

These are efficiency / UX gaps, not correctness gates. They should be addressed in the next repo_map-navigation batch by improving analyzer terminal-mode guidance and by encouraging query-bearing `repo_map(task_map/file_map/relation_map)` calls when the model already has named entities. They must remain soft guidance: no hard rejection for choosing grep/list_files when the result is still valid.

## Data Flow

```text
repo_map / grep / read_file
  -> emit_evidence
  -> EvidenceClosure(ReadSet, PendingReads, Repairs)
  -> emit_investigation_complete aggregate_facts + reason
  -> preCompleteContractCheck
       -> precise blockers: hard
       -> generic forced-read blockers: hard only when no grounded model boundary
  -> Mutable.SetInvestigationComplete
  -> extractor/finalizer receives accepted rich aggregate facts and reason
```

## Task List

- [x] Capture Linux log evidence and classify root cause.
- [x] Implement contextual pending-read partitioning in `emit_investigation_complete`.
- [x] Add tests proving grounded explicit principal mechanism boundaries bypass generic forced reads without enabling deterministic补表.
- [x] Add regression tests proving precise blockers still block: required current-source, config/source-inventory, and call-chain endpoint gaps.
- [x] Run focused tests for `internal/tool`, `internal/types`, and `internal/agent`.
- [x] Run `go test ./...` and `make`.
- [x] Run the Linux repo_map navigation scenario again and verify the answer is good and repo_map is used efficiently.
- [x] Batch 2: add a grounded-evidence completion boundary so generic forced reads do not require explicit aggregate role/support_refs.
- [x] Batch 2: centralize generic forced-read origin classification in `internal/types` so future producers do not reintroduce ad-hoc hard gates.
- [x] Batch 2: add regression tests for aggregate-free grounded narrative completion and for required current-source blockers.
- [x] Batch 2: rerun the Linux repo_map navigation scenario and verify `tools/include/io_uring/mini_liburing.h` no longer blocks completion.
- [x] Batch 3: audit upstream primary-entity prompt wording and classify graph/ranker primary entity files as navigation candidates, not hard read obligations.
- [x] Batch 3: update `buildPrimaryEntityDepthStartInstruction` to remove `Read first`, `read_file immediately`, and `Read the primary file now` hard language for non-exact primary entity candidates.
- [x] Batch 3: add prompt regression coverage so primary-entity candidates cannot regress to hard forced-read wording.
- [x] Batch 4: remove hard-read wording from generic analyzer-required/ranker prompt sections; render them as `Analyzer Navigation Candidates`.
- [x] Batch 4: remove `TOP PRIORITY` / `read them first` language from pre-scanned keyword rankings.
- [x] Batch 4: split mid-loop required-file coverage into deterministic log-frame hard anchors vs advisory ranker candidates, and suppress ranker candidate nudges when the first close-ready stop already has grounded evidence.
- [x] Batch 5: remove extractor/verdict wording that treated read_file history as the only current-repo citation source.
- [x] Batch 5: expose accepted grounded deterministic evidence rows as `evidence_id=...` in the extractor prompt.
- [x] Batch 5: make `emit_hypothesis_verdict` accept current-repo citations covered by accepted grounded evidence, even when unrelated read_file history exists.
- [x] Batch 5: add regression tests for evidence_id verdicts that would previously require a redundant read_file.
- [ ] Batch 6: repo_map navigation efficiency follow-up: analyzer terminal-mode guidance and query-bearing repo_map examples without exposing internal implementation details.
- [x] Run focused tests, `go test ./...`, and `make`.
- [x] Commit and push the batch.

## Non-Goals

- Do not lower grounding/citation floors.
- Do not rewrite model answers or system-generate replacement tables.
- Do not parse user text or model prose for completion decisions.
- Do not make Go-specific assumptions; the boundary uses typed evidence/support refs and works for all repomap-supported languages.
