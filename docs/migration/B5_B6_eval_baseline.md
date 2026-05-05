> Status: archived (2026-05-05). Current architecture lives in docs/architecture.md and docs/design/v3_runtime_consolidation.md.

# B5 + B6 真 eval baseline (2026-05-04)

**Cases**: s1a (mechanism enumeration — gate.Run) × 2, m1a (architecture — explorer/extractor 协作) × 2
**Binary**: post-3edba3f main (B5+B6 全 ship)
**Provider**: MiniMax-M2.7-highspeed (200K context)

---

## 1. Verdict / 通过率

| case | runs | pass | fail |
|---|---|---|---|
| s1a | 2 | 1 | 1 (run-2 missing literal `gate.Run` in answer) |
| m1a | 2 | 2 | 0 |
| **total** | **4** | **3 (75%)** | **1** |

s1a run-2 fail 是答案没把 `gate.Run` 这个字面 token 写进去 — 不是 B5/B6 的回归,pre-existing s1a 的 `EXPECT_CONTAINS="gate.Run coverage"` 严格度问题。

---

## 2. B5/B6 telemetry 命中

### 2.1 B5-F2 facet softening — 4/4 runs 命中

```
s1a r1 [richness] facet_softened family=enumeration   facet_kind=enumeration_item
s1a r2 [richness] facet_softened family=enumeration   facet_kind=enumeration_item
m1a r1 [richness] facet_softened family=architecture  facet_kind=current_code_path
m1a r2 [richness] facet_softened family=architecture  facet_kind=current_code_path
```

**结论**:每个 Run 都 silently softened 一个 HARD facet。这是 B5-F2 之前完全不可见的信号 — 现在每条都打 WARN 行。
**含义**:
- enumeration family 的 `enumeration_item` HARD facet 在 mechanism-enumeration 类问题里**永远**没 ClaimForm-acceptable 的 evidence;说明 family 模板和实际 emit_evidence 输出之间有结构性 gap
- architecture family 的 `current_code_path` HARD facet 同样问题;两者都需要后续审计 `compile_enumeration.go` / `compile_architecture.go` 的 AcceptableForms 列表是否过严

### 2.2 B5-F3 family_underrepresented — 0/4 命中

4 个 case 都没触发 `Buckets >= 2 落 QFCallChain/QFEnumeration/QFGeneric`,正常 — s1a/m1a 都不是比较类。
**待补充 case**:用户问"compare X vs Y"类问题专门跑一次验证 telemetry 触发。

### 2.3 B5-F1 flat-mode tolerance — 0/4 命中

LLM 没把 blocks 写成 escaped JSON string。预期 — 这是边缘错误,真发生时才会救一下。无回归。

### 2.4 B6-F1 cross_citation_conflict — 0/4 命中

s1a run-1 答案有 15 个引用都指向 `gate.go:128`(注意:这是同一行重复!);本来很可能触发 oracle,但**没有触发**因为:V2 doc.Items[].Label 不是 symbol name 而是 step description (e.g. "checkCoverage — 覆盖度检查");oracle 是按 Label 分组的。
**根因**:oracle 的 Label 假设错位 — 应当用 `block.ClaimUses[i].EntityID` 或 `block.Items[i].ID` 的 symbol name 字段,不是 Label 字段。Label 是渲染文本,跨条 citation 几乎不会重名。
**Action**:**B6-F1 V2 选位错** — 需要修 oracle,改读 ClaimUses 里的 entity id,不读 Label。这是真发现 — eval baseline 价值。

### 2.5 B6-F4 finalizer retry think dim

| run | finalizer iter=0 | finalizer iter≥1 | finalizer total |
|---|---|---|---|
| s1a r1 | 2 | 11 | 13 |
| s1a r2 | 3 | 9  | 12 |
| m1a r1 | 2 | 11 | 13 |
| m1a r2 | 1 | 3  | 4  |

`iter≥1` 的 think_aloud 应该 dim — 共 34 次 iter≥1 dispatch 应该都没带 `<think>` 段。**无法直接 grep 验证**(MiniMax 可能压根不发 think 段),但 dispatch 计数对得上 retry 次数 (CGEC perStage `finalize.retries=1/2`)。

### 2.6 B6-F5 per-agent iter counters — 4/4 写入 metrics.txt

```
metric           | s1a r1 | s1a r2 | m1a r1 | m1a r2
analyzer_iters   | 3      | 4      | 4      | 6
explorer_iters   | 9      | 14     | 19     | 20
extractor_iters  | 1      | 2      | 1      | 1
finalizer_iters  | 13     | 12     | 13     | 4
```

但 **eval/run.sh::summary.md 渲染表格里没显示**新加的 4 列。`write_metrics` 写了文件,`summary.md` 的聚合块还是老 12 列。**Action**:summary.md 渲染同步加 4 列。

---

## 3. F6/F7 设计的 baseline 数据

### 3.1 analyze 阶段时长 (F6 prompt cache 评估输入)

| run | analyze first | analyze last | duration | 注 |
|---|---|---|---|---|
| s1a r1 | 01:11:19.854 | 01:12:01.312 | **41.5s** | 单 round |
| s1a r2 | 01:20:09.116 | 01:21:19.788 | **70.7s** | 多 round |
| m1a r1 | 01:11:26.266 | 01:12:00.702 | **34.4s** | 单 round |
| m1a r2 | 01:21:41.341 | 01:23:18.490 | **97.1s** | 多 round |

均值 60.9 秒。F6 设计文档里推断的"analyze 1.5-3.5 min"在多 round + 多 sub-topic 情况下成立,但单 round 情况下只有 ~35 秒,F6 收益占比就小了。**调整 F6 收益预期**:多 sub-topic / 多 round 场景才是主收益场;简单 case 收益 < 20%。

### 3.2 总 Run 时长

| run | 总时长 | finalizer 占比估 |
|---|---|---|
| s1a r1 | **8min 54s** | 大头 (13 iter × ~30s) |
| s1a r2 | **9min 41s** | 大头 |
| m1a r1 | **10min 22s** | 大头 |
| m1a r2 | **8min 6s** | 极大头 (4 iter,但 explorer 20 iter) |

每 Run 5-10min;符合 audit P26。F7 (retry token 优化) 影响面集中在 finalizer iter≥1 那部分 — s1a/m1a 平均 ~10 次 retry iter × ~20s = ~3.5min/Run,**占总时长 35-40%**。

**F7 收益 vs F6 收益**:
- F6: 节省 ~30-50% on analyze (~20-50s/Run × 25-50% off ≈ 10-25s/Run)
- F7: 节省 ~50% on finalizer retry (~3.5min/Run × 50% off ≈ 1-2min/Run)

**F7 的实际收益 5-10× 于 F6**。这翻转了 F6/F7 的优先级 — 应该**优先 F7 (而不是先 F6)**,但 F7 风险更高,所以仍然推荐先 F6 试水后跟 F7。

---

## 4. 真发现的 bug

### 4.1 B6-F1 oracle 选位错(P0,小修)
oracle 按 V2 `Block.Items[].Label` 分组判断"同 symbol 多 citation",但 Label 是渲染文本(如 "checkCoverage — 覆盖度检查"),不是 symbol name。需要改读 `Items[].ID` 或 `ClaimUses[].EntityID`。

### 4.2 eval/run.sh summary.md 渲染漏(P1,小修)
`write_metrics` 写了 4 个新 per-agent iter 字段,但 `summary.md` 的聚合表格没加 4 列。

### 4.3 enumeration / architecture facet 模板过严(P1,审计)
两个 family 都 4/4 fired facet_softened — 不是边缘 case,是模板和实际 evidence 之间的结构性 gap。审计 `compile_enumeration.go` 和 `compile_architecture.go` 的 `AcceptableForms`。

---

## 5. 下一步建议

1. **立即修 B6-F1 oracle 选位错** (1 commit, 小)
2. **立即修 eval summary.md 渲染** (1 commit, 极小)
3. **审计 4.3 enumeration/architecture facet 模板** (中,可能 1-2 commit)
4. **F6/F7 单独 session**(根据本 baseline 重排优先级:先 F6 试水 → F7 主菜)
5. **比较类 case 跑一次** (验证 B5-F3 family_underrepresented telemetry)
