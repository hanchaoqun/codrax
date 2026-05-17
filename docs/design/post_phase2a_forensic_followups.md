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

### 2.1 Explorer dispatch count + retry deep forensic (EXHAUSTIVE)

**Symptom**: User reported "探索阶段返工次数变多了" + "模型有
抱怨被迫重试". The 09:09 log shows 3 explorer dispatches across
56 iterations for a 2-sub_topic question.

Initial pass missed the depth. Below is the exhaustive ground-truth
forensic — every dispatch, every iter symptom.

#### 2.1.A Dispatch summary table

| | D1 | D2 | D3 |
|---|---|---|---|
| Lines | 780–3909 | 4652–6269 | 7333–7655 (incomplete in excerpt) |
| Window after E' trim | probe + evidence_t0 + validate | retry-on-t0 (SuccessCriteria fail) | evidence_t1 |
| Wall time | 206 s | 237 s | ~29 s sampled |
| Iterations | 26 | 26 | 4+ |
| `emit_investigation_complete DOWNGRADED` | **6×** | **4×** | ? |
| Per-key midloop cap saturated | YES (closure-repair 5/5 at iter 23) | likely | ? |
| Tool failures (read_file/grep/list_files/exec) | 5 | 2 | ? |

#### 2.1.B The phantom forced-read path bug (load-bearing root)

**This is the SINGLE root cause for 10× DOWNGRADED + 56-iter
dispatch storm**, not the user's "retry mechanism" frustration.

The pre-scanner at D1 iter=8 (line 2882–2885) queued these
forced-reads as bare paths:

```
phase1_unread: queued forced-read file=packages/opencode/src/tool/apply_patch.ts score=1.3
phase1_unread: queued forced-read file=packages/opencode/src/tool/shell.ts       score=1.3
```

The actual files live at:

```
opencode/packages/opencode/src/tool/apply_patch.ts
opencode/packages/opencode/src/tool/shell.ts
```

The forced-read queue's path-canonicalisation prepends the
alphabetically-first active sub_repo prefix (`codrax/...`), yielding
the **non-existent path** `codrax/packages/opencode/src/tool/...`.
The LLM reads the REAL files (multiple times across iters 9, 12,
14, 19), but the queue does NOT match them and refuses to clear
the entry.

D1 iter=24 (line 3878), the LLM ran `list_files codrax/packages`
explicitly and got:

```
readdir failed: open /Users/han/opt/codrax/packages: no such file or directory
```

This proves the LLM correctly diagnosed the bug. The next iter
(iter=25, line 3889) the LLM `<think>` block says verbatim:

> "`codrax/packages` doesn't exist! So
> `codrax/packages/opencode/src/tool/apply_patch.ts` is not a real
> file… the queue's block seems to be a bug. My evidence is solid
> and the paths are legitimate — I just need to try completing the
> investigation again."

The LLM understood the system was broken. Iter=25 then re-emitted
`emit_investigation_complete` and was DOWNGRADED a 6th time for
the same phantom-path reason. End of dispatch.

#### 2.1.C LLM thinking complaints (verbatim, sampled)

The user asked specifically about model `<think>` complaints. The
three most explicit are:

**D1 iter=23 (09:14:02, line 3812–3820)** — first acknowledgment
that something is wrong:

> "There's clearly some path overlap happening here that I'm trying
> to untangle… The forced-read queue is looking for
> `packages/opencode/src/tool/apply_patch.ts`, and when I read that
> path without the full prefix, the tool says it resolves to both
> `codrax/packages/opencode/…` and `opencode/packages/opencode/…`."

**D1 iter=24 (09:14:08, line 3853–3869)** — explicit "queue keeps
requiring the same files":

> "The forced-read queue keeps requiring the same files with bare
> `packages/opencode/...` paths. But every time I read them, the
> queue doesn't acknowledge them… The queue tracks bare paths and
> doesn't recognize my full-path reads."

**D1 iter=25 (09:14:20, line 3889–3897)** — diagnostic conclusion:

> "the queue's block seems to be a bug."

These are the model "complaints" the user observed. They are NOT
generic retry frustration — they are diagnostic notes about a real
system bug.

#### 2.1.D MidLoop hint saturation

D1 fired `explorer.mid-loop.closure-repair` **≥8 times** with
identical body. Each time the LLM followed the instruction (read
files, re-emit) but the hint was never marked satisfied because the
underlying forced-read queue was deadlocked on phantom paths.

D1 iter=23 (line 3846):

```
continue (per-key mid-loop inject cap reached (key="explorer.mid-loop.closure-repair", count=5/5))
```

The cap saturation is correct safety behaviour; the design
assumption was that 5 hint reinjects should suffice if the LLM is
following instructions. **In phantom-path cases the assumption
breaks** — the LLM IS following but the system never accepts.

#### 2.1.E Evaluator singleton state leakage (Phase 2B preview)

D2 iter=0 (line 4657) has `context_tokens=26626` — a FRESH context
window, not the ~70k carryover from D1's final iter. Yet the LLM's
opening `<think>` block (line 4657-4678) reads:

> "The previous attempt collected 13 evidence items but apparently
> only 11 were tagged as [DIRECT]/[REGISTRATION]. I need to collect
> more… The core issue is that I need to shift my evidence emission
> strategy."

The LLM has analyzed D1's failure pathology without seeing D1's
conversation history. This is `explorerEvaluator` singleton state
(or `BaseAgent.Execute` retry-hint synthesis) leaking the analysis
across dispatches without going through the explicit message
channel.

This is **direct forensic evidence** for the architectural
observation in
`docs/design/phase_2b_explorer_parallel_dispatch.md` §2 (the
80-field singleton evaluator). The doc had been written from
code-reading alone; this log confirms the leak fires in production.

The 2B Approach A refactor (per-Run evaluator construction) closes
this leak alongside the parallel-dispatch foundation.

#### 2.1.F Real schema rejection mixed in (NOT phantom-path)

D1 iter=7 (line 2859) hit a different, legitimate, rejection:

```
emit_investigation_complete = emit_investigation_complete REJECTED
member_set "codrax防幻觉核心组件" has 4 member(s) shaped as
"<code identifier> (<qualifier>)" but support_refs is empty
```

This is the analyzer's decorated-member-set contract (P1A
companion path): when a model emits members of the form
`Foo (qualifier)`, they cannot auto-resolve against evidence
anchors so `support_refs` must be populated. The LLM had emitted
`SelfConsistencyReviewer (摘要vs正文独立LLM)` etc. without
`support_refs`. This is a real prompt-side opportunity (the
LLM didn't know it needed `support_refs`). After this reject the
LLM retried with members + got tangled in the phantom-path loop
above.

#### 2.1.G Summary of all problems found in this run

| # | Problem | Source | Fix surface | Severity |
|---|---|---|---|---|
| 1 | Forced-read queue prepends wrong sub_repo prefix → phantom path | path canonicalisation in multi-repo forced-read seed | `internal/tool/repomap/multigraph` + forced-read seed code path | **CRITICAL** — caused 10× DOWNGRADED + ~5min wasted |
| 2 | Queue does not match full-path read against bare-path entry | forced-read queue dequeue logic | same surface as #1 | CRITICAL (same root) |
| 3 | LLM diagnoses bug in `<think>` block but cannot escape | system-side path bug, LLM behaviour correct | fixing #1+#2 makes #3 moot | observable symptom |
| 4 | `explorer.mid-loop.closure-repair` reinjected ≥8× identical body when underlying gate cannot satisfy | midloop policy doesn't detect "LLM is following but gate refuses" | add detection: if same hint body + LLM same response shape 3×, escalate to fallback instead of reinjecting | medium |
| 5 | Evaluator singleton leaks D1 analysis into D2 via non-message channel | `explorerEvaluator` 80-field singleton | Phase 2B Session A (per-Run evaluator construction) | medium (latent until parallel) |
| 6 | Decorated-member-set without `support_refs` not surfaced as prompt-side requirement | P1A prompt covers the principal contract but not the support_refs requirement for decorator-shape members | extend P1A renderer or its companion section | medium |
| 7 | Per-key midloop cap saturates at 5/5 but caller continues without re-routing | mid-loop policy | escalate to "report unable to satisfy" caveat instead of silent continue | medium |

**Schedulable as**:
- #1, #2, #3 — single focused session, called out in §2.4 below as
  the highest-ROI fix
- #4, #7 — together as a midloop-policy session
- #5 — Phase 2B Session A
- #6 — P1A follow-up

### 2.2 Model "complaining about forced retries" — RESOLVED INTO §2.1.C

See §2.1.C above for verbatim `<think>` quotes. The "complaints"
are LLM diagnostic notes about the forced-read phantom-path bug,
not generic retry frustration. The fix in §2.4 makes the entire
complaint pattern moot.

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
