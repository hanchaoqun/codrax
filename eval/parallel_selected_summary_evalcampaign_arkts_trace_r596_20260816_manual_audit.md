# r596 ArkTS / Trace selected eval manual audit

- date: 2026-08-16
- binary baseline: one rebuilt snapshot from the same `main` checkout
- total cases: exactly 2
- parallelism: exactly 2
- runner result: 0 PASS / 2 FAIL

| case | runner | human | result | audit conclusion |
|---|---|---|---|---|
| `arkts_repomap` | FAIL | partial | `eval/results/arkts_repomap-20260816-170847` | The source inventory, exploration handoff, and final answer all retained the correct four `@Entry` members and two `@Builder` members with their ArkTS locations. B946 is production-positive: the requested member dimension was not rejected or patched after the first finalizer emit. The visible answer nevertheless rendered the same roster twice, once as bucket `section.items[]` and again as the required per-member table, so the runner correctly counted eight Entry rows instead of four. One JSON-encoded `blocks` string was recovered losslessly and did not cause the duplication. |
| `real_trace_h7_self_seat_full_spectrum` | FAIL | fail | `eval/results/real_trace_h7_self_seat_full_spectrum-20260816-170847` | All six trace queries retained the explicit 13762.791708..13763.024898 window and the answer preserved basic target facts (running 74.915ms, D-state 36.757ms/11 intervals, `dma_fence_default_w`, and 65.912ms supply-fold deficit). The analyzer, however, emitted `intent=explain`, `scenario=generic`, `scope=bounded_fact_set`, finite fact families, and generic `member_set` dimensions despite the request explicitly asking for a ranked root-cause/contributor roster. Consequently `trace_query_final_projection_blocks=0`: the deterministic Trace causal projection, complete on-chain roster, compaction disclosure, 49.623/0.033 same-source split, and unpriced-occupancy direction were absent. |

## Generalized gap decision

### B947 — one typed principal member set, two model-visible structured carriers

`HasPerMemberTable=true` already makes a principal table mandatory, but three prompt surfaces still allowed or recommended the same principal rows in `section.items[]`: the optional section rationale, Principal Enumeration Rows handoff, and typed support-lane member obligation. The model followed all three literally and duplicated a correct roster. This is a context-contract conflict, not an ArkTS extractor defect and not a runner false positive.

The fix is typed and language-neutral: the required table is the single structured principal-member carrier; optional sections may retain business/category headings and short explanations, with the bucket kept as a visible table column, but may not repeat member rows. This is soft answer-writing guidance only. It does not scan user/model/final prose, add a rejection gate, delete a valid block, or let the system author answer content. Ordinary enumerations without `HasPerMemberTable` keep their existing section/list/table choice.

### Trace r596 — analyzer semantic variance, no new hard gate

The analyzer prompt already contains the exact generalized rule: ranked causes, competing contributors, or all major/minor causal sources require `causal_contributor_set` and `causal_diagnosis`; `bounded_fact_set` is only for finite observed facts. The tool schema then correctly accepts any internally coherent tuple and cannot prove the omitted causal intent without re-reading the user prose. Many heterogeneous executions of this same case have produced the full causal projection, including r577/r580/r584/r593.

Therefore this run is recorded as model semantic variance with sufficient system teaching, not as authority to add a keyword scanner, auto-widen every trace query, or system-write a causal conclusion. A future generalized change would require repeated heterogeneous misses and an independent typed classifier/reviewer signal. The explicit-window projection, automatic supplement, on-chain-only root authority, actual-occupancy/business clues versus rule-eliminable dual axes, and adjacent/background support-only boundary remain unchanged.

## Runtime and recovery checks

- ArkTS: one lossless structured-string recovery, zero finalizer rejects, no empty/degraded answer.
- Trace: zero JSON recovery, zero finalizer rejects, six explicit-window queries.
- Neither run degraded because of 4ms, 4s, or total stream age. Active byte streams remain governed only by caller cancellation/deadline, no-first-byte, byte-stall, transport, or decode failure.
