# PR #23 structured Trace root-cause sidecar audit

Date: 2026-08-31  
PR: `https://github.com/hanchaoqun/codrax/pull/23`  
Original commit: `ea525ad937f73c32aed197b5050a8faac1717816`  
Original author: `keruoya <keruoya@users.noreply.github.com>`  
Merge commit: `80b6e0425` (two-parent, non-squash merge)

## 1. Scope and conclusion

The change adds a structured Trace root-cause sidecar, typed candidate compilation,
final-answer artifact plumbing, and `.root-causes.json` retention. The direction is
useful: the compact machine-readable result is separate from the full Markdown/HTML
answer, and output cleanup understands the sibling file.

The PR could not be enabled unchanged. Four architecture gaps survived review:

| ID | Severity | Finding | Resolution |
|---|---|---|---|
| PR23-A | P0 | A missing or semantically invalid `trace_root_causes` field rejected the whole `emit_answer_document` transaction, so an optional sidecar could erase a useful full answer. | Sidecar is optional. Absence is accepted; sidecar-only decode/binding failure is logged and ignored while the model answer remains eligible. Patch failure preserves the last accepted sidecar. |
| PR23-B | P0 | The model could freely author category, thread/resource/phase identity, positive impact and evidence prose without binding any typed candidate or evidence receipt. A background or fabricated number could therefore become a structured root cause. | Input now accepts only an ordered list of exact candidate IDs from a schema enum. Runtime binds every semantic field, magnitude and evidence from the frozen contract. Only typed `on_chain` candidates with representable per-thread wall-clock duration are selectable. |
| PR23-C | P0 | `BuildDeterministicFinding` selected the first eligible ranked candidate as `PrimaryCause` after model completion. That made system ranking a conclusion authority. | Automatic primary-cause selection was removed from finalizer execution. The legacy builder is fail-closed unresolved; model candidate choice and order own the conclusion. |
| PR23-D | P1 | The new atomic artifact setter duplicated the answer-success epilogue and omitted cleanup of the pending patch base and relation-repair lease. Three existing tests reproduced stale retry state after a successful emit. | Full and multi-artifact setters now share one locked success epilogue that clears all retry-local state. |
| PR23-E | P1 | The PR added a `TraceFindingRequired` field and orchestrator setter described as a CLI switch, but no CLI or other caller invoked it and the default activation path did not read it. The eight dead lines also failed the orchestrator hot-file ratchet. | Removed the unused pseudo-configuration surface instead of raising the ratchet budget. Runtime activation remains derived from typed Trace request shape and candidate availability. |

## 2. Preserved behavior

- The full answer remains model-authored and is never replaced, summarized, or
  rewritten by the sidecar path.
- Explicit-window Trace causal projection and system evidence supplementation are
  unchanged.
- Root-cause admission reads typed projection fields only. It does not scan user
  input, model reasoning, final prose, Markdown, or Mermaid labels.
- Adjacent/background rows remain available to the long answer as support, but are
  absent from the root selector.
- Model owns how many candidates to select and their strongest-to-weakest order.
  The system owns only exact fact binding and stable serialization.
- Count, composite-score, and cross-thread CPU-ms values are not relabeled as
  `impact_seconds`.

## 3. Output contract after remediation

The optional model input is deliberately small:

```json
{
  "schema_version": 2,
  "root_causes": [
    {"candidate_id": "candidate-..."}
  ]
}
```

Each candidate ID is exposed by the per-dispatch schema from the exact typed
on-chain roster. The persisted `.root-causes.json` retains the PR's public v2 form:
rank, category, subject/resource/phase identity, impact seconds, stable summary and
evidence. Internal candidate IDs are cleared before persistence.

## 4. Remaining bounded gaps

1. The public category vocabulary still folds semantic CPU-work families without a
   dedicated category (for example class verification, runtime compilation and
   texture upload) into `phase_high_load`. The long answer and Trace projection keep
   the exact semantic class, so no evidence is lost, but the compact sidecar taxonomy
   is less specific. Extending the public schema requires a versioned compatibility
   decision rather than silently changing v2 labels.
2. Cross-thread compute-delivery aggregates are deliberately excluded because their
   unit is CPU-ms rather than wall-clock seconds. Per-thread low-frequency wall-clock
   candidates remain representable. A future aggregate field needs an explicit unit,
   not conversion into `impact_seconds`.
3. Syntactically invalid JSON for the entire tool call is handled by the existing
   answer-document recovery path. This batch makes any syntactically valid but
   malformed sidecar shape non-blocking; it cannot recover bytes the provider never
   delivered as a parseable tool call.

## 4.1 Versioned decision — v2 additive qualifiers (2026-09-02, user ruling colleague_merge_audit §40.28 ②)

The public v2 item gains two ALWAYS-EXPLICIT closed-set fields, append-only
(`schema_version` stays 2; consumers ignore unknown keys, the same
compatibility posture as the `next_info` tail-append ruling):

- `impact_caliber` ∈ {`effective_attribution`, `window_projection`} — the ruler
  behind `impact_seconds`; the evidence sentence speaks the same caliber
  (「链上有效归因为」 vs 「窗内投影占用为…（未发布有效归因）」), so a raw window
  projection is never called 有效.
- `causal_qualifier` ∈ {`proven`, `frame_unproven`} — SEAT-LEVEL, bound from
  the same evidence-ID authority index the Markdown headline consults
  (`tracefinding.SeatFrameCausalityIndex`); a `frame_unproven` item's summary
  carries the headline's exact qualifier words 「（帧因果未证）」. The
  session-wide ANY causal signal is advisory-only
  (`TestSessionAnyCausalSignalFeedsAdvisoryLanesOnly`).

`status=available` on the default artifact remains a DELIVERY status (a valid
model selection was persisted), never a causal-proof assertion.

- 2026-09-02 QUALGATE-1(colleague_merge_audit §40.30 V-QUAL-1 方案 A):`causal_qualifier` 闭集追加第三值 `not_applicable`——analyzer typed 判定 `runtime_question_profile.frame_causality_requested=false`(非帧/卡帧类问题)时,席位级提供者关门,两面均不作帧因果声明(头行无限定注,sidecar 显式 `not_applicable`,summary 无后缀,合同顶棚 `not_applicable` 不封顶 status);append-only,`schema_version` 仍为 2。
- 2026-09-02 SIDECAR-EVID-1(客户反馈 → colleague_merge_audit §40.32):`evidence` 改由候选的系统拥有 typed 事实包渲染为最多四句客户可读证据(量化 / 链路关系与凭证 / 机理与边界 / trace 定位),**不再发布** `.codrax/blob/…` 临时路径与 `trace_query:…json#…` 内部结果 id;wire 形状不变(`evidence: []string`,1–4 条,每条 ≤240 rune)。

## 5. Verification obligations

- candidate compiler: exact on-chain admission; background/adjacent negative arms;
- selector binder: model semantic-field spoofing is ignored; candidate ID/order are
  the only model-owned values;
- full emit: invalid optional selector does not reject or alter the full answer;
- success epilogue: accepted full and patch emits clear relation lease and staged
  patch base;
- hot-file ratchet: no dead CLI switch or budget increase;
- schema: sidecar property is optional and candidate IDs are enum-bound;
- repository-wide Go tests and build must pass before push.

Verification result: `go test ./... -count=1` passed; the focused finalizer
prompt tests passed after the compact roster projection; `make` passed.

## 6. History preservation

The PR was merged with a regular two-parent merge. Commit `ea525ad93` remains
reachable unchanged with the original author identity; remediation is recorded as a
separate descendant commit so review can distinguish imported work from audit fixes.

## 7. Current-main survivorship re-audit (2026-09-01)

The requested PR merge is already complete and must not be repeated:

- `git merge-base --is-ancestor ea525ad937f73c32aed197b5050a8faac1717816 main`
  succeeds;
- merge `80b6e0425a65240eff677446e3407be3635e6cd1` still has two parents,
  `5ee4046b...` and the unchanged PR head `ea525ad93...`;
- the imported commit still records `keruoya <keruoya@users.noreply.github.com>` as
  author and committer. A new cherry-pick or empty merge would only duplicate history
  and is therefore intentionally not created.

All five remediation properties still survive current main:

1. `BindRootCauseReportSelection` accepts only the model-selected ordered candidate
   IDs and binds category, identity, duration and evidence from the frozen typed
   contract.
2. `SelectableRootCauseCandidates` and `boundRootCauseItem` still require
   `PrimaryEligible`, a positive typed magnitude, `ms`, and
   `wall_clock_per_thread`; background, score/count and cross-thread CPU-ms rows do
   not enter the compact root selector.
3. No current finalizer call automatically chooses `PrimaryCause`; the legacy
   `SetTraceFinding` path remains documented as non-conclusion-selecting, while the
   sidecar stores only the bound model selection.
4. The long `AnswerDocumentV2` and optional root-cause report remain sibling
   artifacts. The report setter does not render, replace, summarize or mutate the
   long answer.
5. Current dynamic-schema, binding, state, output-dump and mutation tests pass. The
   focused current-main command covered `internal/analysis/tracefinding`,
   `internal/types`, `internal/tool`, `internal/outputdump` and
   `internal/orchestrator` with Trace root-cause/finding/final-artifact test families.

The bounded taxonomy note in §4.1 has partially evolved since the original audit:
v2 now has dedicated `jit_compilation`, `shader_compilation`, `gc_long_pause` and
`compute_supply_shortage` categories. Other deterministic semantic work without a
versioned public category can still fall back to `phase_high_load`; changing that
remaining public vocabulary still requires an explicit compatibility decision.
The CPU-ms unit boundary and syntactically unparseable whole-tool-call boundary in
§4.2–§4.3 remain intentional and unchanged.

Re-audit result: no new PR #23 gap was found, no code change was required, original
author history is preserved, and the existing Trace explicit-window causal
projection/automatic supplementation path remains untouched.

## 8. r1018 production follow-up (2026-09-02)

The September 1 result above is historical, not a claim that all sidecar behavior
is gap-free. The next heterogeneous replay exposed three additional gaps; their
single task/status ledger is `eval_priority_campaign_audit_20260730.md`
§123.1630–§123.1633:

- B1555 (`1cee3bc3d`): distinct model-selected receipts collided on a short
  display summary and invalidated the complete report. Duplicate checks now use
  the frozen candidate identity; public JSON still omits that internal ID.
- B1557 (`24d696fc2`): the broad token lane erased the meaning of the selected
  magnitude. The candidate now retains producer supply-fold and D/IO accounting.
  An effective running deficit is not labeled phase workload, and a mixed or
  proven non-IO D wait is not labeled entirely IO. Existing v2 categories are
  retained; `sleep_blocking` is the broad thread-blocking summary, not proof of
  scheduler state S. Model context and bound evidence share the same qualifier.
- B1556 (`b3b122619`): full/patch field teaching differed. The two native
  object forms are now taught together; an unambiguous full-field selector at the
  patch entry is moved without changing order, IDs or model answer. Conflicts and
  incomplete carriers are not guessed; invalid optional selectors retain the last
  accepted report on patch.

B1555/B1557 passed full-repository tests and build before push. B1556 verification
and the next paired replay are tracked in the unified ledger. The remaining
versioned taxonomy boundary in §4.1 is not declared solved by these changes.
The model selection remains optional; the default sibling file is mandatory and
uses the existing typed unavailable envelope when no valid selection can be bound.

## 9. r1019/r1020 evidence packing and remaining claim ceiling (2026-09-02)

B1557's new caliber text exposed a publication regression: full source and row
references plus the qualifier exceeded the existing 240-character evidence-entry
limit. B1558 (`a9b1e8b5e`) packs complete facts/references at semantic boundaries
into the existing four-entry capacity. It neither truncates evidence nor widens
the public v2 limits. Full-repository tests and build passed. r1020 produced an
available five-item report matching the model's five selected candidates; the
first item's complete evidence occupies two entries of 194 and 65 characters.
The long answer and causal projection remained present; no conclusion was selected
or rewritten by this repair. The earlier unavailable artifact was not overwritten.

A separate P1, B1562, remains open: the priority-inversion candidate's source-specific
mechanism ceiling and composite runnable/full + running/discounted caliber do not
yet accompany its compact public category/summary. The existing category vocabulary
must not silently be treated as proof of an occurrence that the source only offers
as a candidate. Design the certainty/caliber carrier and v2 compatibility before
changing public semantics; do not infer it from model prose. Unified status and
next batches are in §123.1637 of the campaign ledger. These positive replay results
do not assert that all sidecar semantic gaps are closed.
