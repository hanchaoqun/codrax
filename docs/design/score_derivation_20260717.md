# Score derivation — composite scores, count equivalents, over-limit clamp (2026-07-17)

Status: SHIPPED with DISPLAY-HYG 第一轮 件3 (§29.104.22 立案, §29.104.22.1 用户裁定).
This is the WEIGHT-CONSTANT authority the report face deliberately hides: the
「阅读参考」/各列口径 formula entries (`internal/tool/answer_document_mutation_runtime.go`,
SCORE-DERIV gated lines) name the terms and mark weighted ones `(加权)`, and the
constants live ONLY here and in the code. 权重全部只活 advisory/背景软引导车道 —
不参与汇排、不铸序数、不进硬门 (§29.104.22 边界确认; 精确信号红线合规).

User ruling verbatim (§29.104.22.1): 「报告里只需要在"阅读参考"里面提及即可,让客户
大概知道公式即可,无需列举详细计算具体值,报告里可以体现公式,但是可以隐去权重常量
的具体数值。」

## 1. 综合评分 (io_pressure) — published value

Mint: `computeIOPressureSummary`, `internal/tracequery/query.go:11891-11896`
(`IOPressureSummary.Score`; wire = the M18 `*_score` key family + typed
`Unit=composite_score`, §29.104.17 裁定②).

```
score = firstPositive(块层最大单事件延迟ms, 存储层最大单事件延迟ms)
      + iowait 阻塞次数            × 5
      + D态墙钟 ms                 × 1
      + iowait 墙钟 ms             × 1
      + 页缓存事件数(churn)        × 0.2
      + 文件IO事件数               × 0.1
      + 文件IO字节 / MiB           × 2
```

Precision note: the first term is `firstPositiveFloat(blockMax, storageMax)` —
the BLOCK-layer max when positive, else the storage-layer max; it is NOT
`max(block, storage)` (the §29.104.22 ledger sketch's `max()` shorthand is
imprecise on traces where both layers are positive and storage > block).

对账 (§29.104.22, donghu witness): io_pressure `score=699.293` reproduced
term-by-term from the mint at filing time.

## 2. 综合评分 (block_io) — published value vs. internal sort score

Two sites share the vocabulary; ONLY the first publishes:

- **Published rank value** — `buildRootCauseRankFromWithCache`,
  `internal/tracequery/query.go:14351`:

  ```
  impact = 块层最大单事件延迟ms + 存储层最大单事件延迟ms + 文件IO字节/MiB × 1
  ```

  This is the value the report faces render as `X(综合评分,非墙钟)` (witness
  E33 `2.694`), then subject to the §4 off-chain cap like every background
  impact.

- **Internal sort score** — `computeBlockIOByInode`,
  `internal/tracequery/query.go:12064`:

  ```
  sort_score = 块最大延迟 + 存储最大延迟 + 文件IO字节/MiB + 页缓存事件 × 0.2
  ```

  Ordering of the `BlockIOByInode` summary list ONLY; never published as a
  value, never rendered.

**Report-entry deviation (委托默认处置 §29.104.19, 待人工追认):** the ruling's
quoted block_io formula (from the §29.104.22 ledger sketch) carried a fourth
term 「页缓存事件(加权)」, which exists only in the internal sort score above —
the PUBLISHED value has no page-cache term. The report entry follows the
published value (three terms); echoing the sort score's fourth term on the
promise surface would over-claim (图例=承诺面).

## 3. 计数当量 — count-equivalent advisory values

Registry class `CausalCaliberSideCount` (`Additivity==count`,
`internal/tracequery/causal_token_registry.go`). Producers:

- **page_cache_churn** — `internal/tracequery/query.go:14290`:

  ```
  impact = churn 事件数 × 0.3
  ```

  对账 (§29.104.22): `84.300 = 281×0.3`, `34.800 = 116×0.3`, `119.100 = 397×0.3`.

- **file_io_hot_inode** — `internal/tracequery/query.go:14265-14267`: uses the
  measured `TotalLatencyMs` when positive; otherwise the advisory count form
  `fileIOAdvisoryImpactMs` (`internal/tracequery/query.go:18140`):

  ```
  impact = 有效事件数 × 0.25 + 字节 / MiB × 2
  ```

  (有效事件数 = `max(Count, CompletionCount)`.)

Display word single sources: `rootCauseCountEquivalentValue` (engine roster
face, `internal/tracequery/rank_family_fold.go`) and
`runtimeTraceProjCountEquivalentValueText` (report face,
`internal/tool/answer_document_mutation_runtime_rcm.go`) — both spell
`计数当量X(非墙钟)`, never an ms suit.

## 4. 超上限截断 — off-chain window cap

`backgroundImpactMs`, `internal/tracequery/query.go:18032-18048` (cap constant
at `:18040`):

```
cap = 窗长ms × 0.35        (floor 0.1ms)
```

Applies to every OFF-CHAIN impact minted while a causal chain exists
(`hasCausalChain && !onChain`); an impact above the cap publishes the cap. 对账
(§29.104.22): donghu E46 `81.616 = 233.190 × 0.35` exact; member Σ `119.100`
stays on the 原始和 comparison face.

Display gate: `runtimeTraceProjFamilyCountSumClamped`
(`internal/tool/answer_document_mutation_runtime_rcm.go:96`) — typed comparison
of the published seat value against `FamilyMemberSumMS` beyond the `%.3f`
print tolerance; produces the `计数当量(超上限截断;共N项,同线程)` word and the
SCORE-DERIV clamp legend entry (same predicate, entry and word can never fork).

## 5. Report-face contract (件3 pins)

- Entries render ON DEMAND: each `阅读参考` formula entry renders exactly when
  its word face is on the render — flags in
  `runtimeTraceProjDetailTableLegendFlagsFor`
  (`internal/tool/answer_document_mutation_runtime_tree.go`) read the SAME
  typed predicates the value word faces read (承诺面双向 by shared gate).
- Weight constants NEVER on the report face: the entry lines carry zero ASCII
  digits by construction; pins additionally grep `0.35 / ×5 / ×0.2 / ×0.3 /
  ×0.1 / 0.25` as absent (`answer_document_projection_scorederiv_test.go`).
- 全部标注「非墙钟,不参与汇排」 (ruling verbatim).
