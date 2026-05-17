# Post-Phase-2A forensic follow-ups

**Status**: tracking design doc. Each item is independently
schedulable in a future session.

**Baseline forensic anchor**: 2026-05-17 09:09 production run
(`/Users/han/opt/.codrax/logs/opt-1145e7c4/codrax-20260517-090936-000-66725.log`).
First production run after E' Phase 1 (`e62aabfb`) + Phase 2A
(`86d8c84d`) landed.

## 1 ✓ FIXED in this session

### 1.1 关注点 N/M 状态行位置 + 颜色

**Symptom**: User reported the trailing `· 关注点 2/2` tag at the end
of completed-stage rows was confusing — different colour (cyan) from
the rest of the row (gray), positioned far from its semantic sibling
(`2/4` stage progress), and looked like a 7th unrelated metadata
field.

**Forensic line**:

```
09:19:08.587 INFO [render]   ✓ 2/4 已完成证据收集 · 第 5 轮 · 7 次工具调用 · 本 43s · 总 9m0s · 关注点 2/2
```

**Fix**: `internal/render/renderer_dock.go` `formatStageDoneLine`
moved the topic tag from row tail to immediately AFTER the stage
progress, and switched its colour from `statusObjective` (cyan) to
`statusMeta` (gray) to match every other meta segment.

**New row layout**:

```
✓ 2/4 关注点 2/2 已完成证据收集 · 第 5 轮 · 7 次工具调用 · 本 43s · 总 9m0s
```

**Pinned by**: 3 tests in
`internal/render/topic_progress_position_test.go`
(`TestFormatStageDoneLine_TopicProgressPositionAndColor`,
`TestFormatStageDoneLine_TopicTagOnlyForMultiSubTopicEvidence`,
`TestFormatStageDoneLine_TopicTagEnglish`). Test stripAnsi helper
asserts position by character offsets, and zh/EN mode parity.

## 2 ⏸ DEFERRED to Phase 2B session (architecture-blocked)

### 2.1 Explorer dispatch count increase

**Symptom**: User reported "探索阶段返工次数变多了". After E'
Phase 1, the 09:09 run shows 3 explorer dispatches for a 2-sub_topic
question; pre-E' would have shown 1 merged dispatch.

**Forensic timeline**:

```
09:10:54  DISPATCH stage=explore attempt=0  (window: probe + evidence_t0 + validate, trimmed to first evidence)
09:14:20  DISPATCH stage=explore attempt=0  (retry on t0: "Previous attempt collected only 11 entries; need ≥2", forced reads)
09:18:25  DISPATCH stage=explore attempt=0  (window: evidence_t1 — opencode sub_topic)
```

**Analysis**:

1. Dispatch 1 = t0 (codrax sub_topic) — E' Phase 1 working as
   designed
2. Dispatch 2 = t0 RETRY because SuccessCriteria failed (`need ≥2
   [DIRECT] entries; forced reads`) — this is the EXISTING retry
   mechanism, not new in E'
3. Dispatch 3 = t1 (opencode sub_topic) — E' Phase 1 picks up next
   sibling after t0 finally markDone

**The retry (dispatch 2) is NOT introduced by E'**. Pre-E', the same
SuccessCriteria failure would have triggered the same retry; it just
would have run inside the merged window so it wasn't visible as a
separate dispatch.

**The "increase" is largely cosmetic** — total LLM iterations are
comparable to pre-E', but the dispatch boundary is now drawn
per-sub_topic, making it look like more work.

**However**: there IS a real concern lurking under this. The
`explorerEvaluator` is a process-lifetime singleton (see
`docs/design/phase_2b_explorer_parallel_dispatch.md` §2). Three
sequential dispatches against the SAME evaluator means
`investigationNotes` / `structuredEvidence` / `midLoop*` flags are
NOT reset between dispatches, even though each dispatch is on a
different sub_topic. The pre-2026-05-17 code path had this same
issue (it always reused the singleton across DAG rounds), but E'
Phase 1 makes it more visible because the "rounds" now align with
"sub_topics" instead of "validate retries". The evaluator refactor
in Phase 2B Approach A (per-Run evaluator construction) fixes both
problems simultaneously.

**Schedulable as**: Phase 2B Session A (evaluator refactor) —
already documented in `phase_2b_explorer_parallel_dispatch.md` §7.

### 2.2 Model "complaining about forced retries"

**Symptom**: User reported assistant text expressing frustration
about being forced to retry. The closest evidence in the 09:09 log:

```
emit_investigation_complete DOWNGRADED — pending forced reads block the closure.
```

(this pattern repeats ~9 times across the 3 dispatches)

And one explicit assistant note:

```
系统对 packages/opencode/ 路径的解析指向错误的基础目录
```

**Analysis**: The "complaint" is the assistant noting that the
system's forced-read path resolution is pointing to a non-existent
base directory (`packages/opencode/...` instead of
`opencode/packages/opencode/...`). This is a **path resolution
bug** in the multi-repo forced-read seed, NOT a retry-mechanism
flaw. The retries themselves are correct; the file paths the
retries target are wrong.

**Fix**: Identify the path canonicalisation that emits seeded
forced reads. Check whether the multi-repo sub_repo prefix is
correctly applied. May require auditing
`internal/tool/repomap/multigraph` and
`internal/agent/explorer.go`'s exact-anchor selection path. **Not
in this session's scope**.

### 2.3 Finalizer not converging, falling back to raw

**Symptom**: User reported finalizer retries increased and
"没收敛答案".

**Forensic timeline**:

```
09:20:05  iter=0  emit_answer_document  ok=false   (rejection: missing 11 member_set members in visible answer)
09:20:40  iter=1  emit_answer_document  ok=false   (same rejection)
09:20:58  iter=2  emit_answer_document_patch  ok=false  (same rejection)
09:21:28  iter=3  emit_answer_document  ok=false  → midloop_force_stop (same error class 3 times → quota burn)
09:21:28  WARN  emit_answer_document missing after retries; falling back to raw content (len=2156)
```

**Root cause identified** — **P1A strong-contract typographic
strictness**. The investigator emitted member surfaces with
EN-paren + no inner spaces:

```
SelfConsistencyReviewer (摘要vs正文独立LLM)
```

The finalizer LLM, writing zh prose, rendered the same identity with
zh-paren + spaces around `vs`:

```
SelfConsistencyReviewer（摘要 vs 正文独立 LLM）
```

The pre-emit oracle `preEmitAggregateMemberAppearsInDocument` uses
`preEmitDisplaySurfaceAppears` which normalises whitespace via
`strings.Fields(...) + Join(" ")` but does NOT normalise:

- EN `(` `)` ↔ zh `（` `）`
- Adjacent ASCII-CJK boundary spacing (`vs正文` ↔ `vs 正文`)

So three iterations of perfectly-equivalent prose got rejected for
typographic differences, the patch retry hit the same wall, and the
finalizer hit `same error class 3 times` quota burn → raw fallback.

**P1A was the right shape** (strong-contract pre-emit hint surface);
**the oracle's surface comparator is too strict**.

**Proposed fix** (out of scope for this session):

Add a `preEmitNormalizeSurfaceForMixedCJKComparison(s) string` helper
that, IN ADDITION to whitespace fold:

- maps `（` → `(`, `）` → `)`, `！` → `!`, `：` → `:`, full-width →
  half-width for ASCII punctuation
- collapses ASCII-letter-or-digit ↔ CJK boundary whitespace to no
  space (`vs正文` ≡ `vs 正文`)

and call it on BOTH value and surface inside
`preEmitDisplaySurfaceAppears`. This is a 50-line localised oracle
upgrade with bounded risk; should be done with 4-5 unit-test pairs
covering zh-paren / EN-paren / mixed spacing / surface integrity (do
NOT change behavior on pure-ASCII identifiers like `gate.RunWith`).

**Schedulable as**: 1-session focused fix when this run's
forensic re-test confirms the typographic mismatch is the dominant
finalizer-retry source.

### 2.4 Multi-repo forced-read path resolution bug

**Symptom**: As noted in §2.2, the assistant noted incorrect base
directory in forced-read seeds:

```
系统对 packages/opencode/ 路径的解析指向错误的基础目录
```

**Likely fix surface**: `internal/tool/repomap/multigraph` or
`internal/agent/explorer.go` exact-anchor injection — the sub-repo
prefix is being stripped/added incorrectly when the multi-repo
active set is constructed.

**Schedulable as**: 1-session bug fix, independent of Phase 2B.

## 3 Notes for Phase 2B Session A planner

When the evaluator refactor lands:

- Spec-test for §2.1: a 3-sub_topic IR with deterministic LLM stub
  should produce exactly 3 explorer dispatches (NOT 3+retry); each
  dispatch's evaluator state at entry must equal the zero value.

- Spec-test for §2.3 oracle robustness fix (if done separately):
  build a fixture where investigator emits `Foo (bar)` and finalizer
  writes `Foo（bar）` in prose; oracle must accept.

## 4 Sign-off

The §1 fix in this session is the smallest user-visible improvement
that didn't risk regression. Everything else is parked behind the
forensic doc + the Phase 2B design doc so the next session has a
prioritised list to draw from without re-running the audit.
