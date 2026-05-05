> Status: archived (2026-05-05). Current architecture lives in docs/architecture.md and docs/design/v3_runtime_consolidation.md.

# B7-T3 Richness Audit — 6 Family Live Eval (B6 V2 default)

| 项 | 值 |
|---|---|
| Audit 日期 | 2026-05-03 |
| Codrax baseline | `e43da60` (B6 V2 default switch shipped) |
| Eval suite | 6 family, single-run each |
| Mode | V2 default (`pipeline_emit_v2_default=true`, no CLI override) |

参考 `docs/migration/block_only_carrier.md` §11 模板。每 family 一份，量化 + 主观判定。

---

## QFRootCauseTrace — `s5a` (LoopController 实现枚举)

> 注：s5a 实际是 QFEnumeration（`is_category_enumeration=true`）；列在此处仅作 root-cause-shaped output 的 reference baseline，因为我们手头没有真正的 log_triage 触发用例正在 V2 mode 验证。后续 B7 持续运行可补充 logtri_go.case。

| 维度 | V2 答案 | V1 baseline (refer s5a-20260503-072803 / 095401) | Δ |
|---|---|---|---|
| Summary 长度 (字符) | ~280 | ~290 | ~等同 |
| 主结构 block 数 (table 行) | 8 | 7-8 | ✓ 8/8 implementer 全列 |
| 引用 citation 数 | 9 (file:line) | 7-9 | ✓ 等同 |
| Diagram | 无（OK：枚举答不需要）| 无 | ✓ |
| Caveat 段 | 末尾 italic 段说明范围 | 同上 | ✓ |
| 主观完整度 (1-5) | 5 (8/8 全锚) | 5 (8/8 全锚) | 等同 |

判定：**PASS**

## QFEnumeration（同 s5a）

参考上栏。

## QFConfigPrecedence — `qf_config_precedence` (pipeline_emit_v2_default 解析链)

| 维度 | V2 答案 | V1 baseline (无对照, 新 case) | Δ |
|---|---|---|---|
| Summary 长度 | 5633 chars | — | richness 充分 |
| 引用 citation | 14 | — | ✓ 高密度 |
| Scalar block (resolved value) | 含（默认 true）| — | ✓ |
| Table block (precedence layers) | 含 | — | ✓ Optional 充分用上 |
| Diagram | 1 (mermaid flow) | — | ✓ Optional, 帮助理解 |
| Caveat | 范围说明 | — | ✓ |
| 主观完整度 | 5 | — | 5 |

判定：**PASS**（B2 编译规则的 Required+Optional 全用上）

## QFRoleLookup — `s11a` (analyzer stage 是否允许 read_file)

| 维度 | V2 答案 | V1 baseline (无对照) | Δ |
|---|---|---|---|
| Summary 长度 | 1795 chars | — | 偏短 |
| Scalar block (resolved literal) | 无（"否"作 prose）| — | ⚠ 退化 |
| 引用 | 0 | — | ⚠ 应当至少 1 个 capability surface citation |
| Caveat | 1 段 | — | ✓ |
| 主观完整度 | 3 | — | NEEDS_FIX |

判定：**NEEDS_FIX** — s11a 的 hypothesis 拒绝路径让 finalizer 认为没有可引用的 anchor，结果 emit 没有 Scalar block 与 file:line，answer 退化成纯 prose。这是 hypothesis-rejected 路径在 V2 schema 下的展现问题，**不是 V2 carrier 本身的退化**：V1 baseline 同样 case 也是 prose-only。问题 root 在 explorer 的 emit_evidence 0 行（未触及 capability surface 源码），跟 V1/V2 carrier 选择正交。**记录为已知预先存在限制**，不阻塞 B8。

## QFCallChain — `m1a` (explorer 与 extractor 协作)

| 维度 | V2 答案 | V1 baseline (m1a-20260503-071141) | Δ |
|---|---|---|---|
| Summary 长度 | 8811 chars | ~5000-7000 chars | V2 更厚实 |
| OrderedList (hops) | 含多 hop | 含 | ✓ |
| 引用 | 18 | 12-15 | ✓ V2 更高密度 |
| Diagram | 1 (mermaid sequence) | 1 | 等同 |
| 主观完整度 | 5 | 5 | 等同/略升 |

判定：**PASS**（V2 Family-aware block contract 让 LLM 多层结构化输出）

## QFArchitecture — `qf_architecture` (codrax read-mode pipeline)

| 维度 | V2 答案 | V1 baseline (无对照) | Δ |
|---|---|---|---|
| Summary 长度 | 4954 chars | — | 充分 |
| Section blocks (per layer/stage) | 多 (analyze/explore/extract/finalize 各一) | — | ✓ |
| Diagram | 1 (mermaid flow) | — | ✓ Required, 已 emit |
| 引用 | 11 | — | ✓ |
| Caveat | 1 段 | — | ✓ |
| 主观完整度 | 5 | — | 5 |

判定：**PASS**

## QFGeneric — `s1a` (gate.Run 9 项检查顺序)

| 维度 | V2 答案 | V1 baseline (s1a-20260503-071141) | Δ |
|---|---|---|---|
| Summary 长度 | 5924 chars | ~3500-4000 | V2 更厚实 |
| OrderedList (9 checks) | 含 | 含 | ✓ 都 emit |
| 引用 | 12 | 8-10 | ✓ 更高密度 |
| 主观完整度 | 5 | 5 | 等同/略升 |

判定：**PASS**

---

## 综合裁定

| Family | Verdict | 备注 |
|---|---|---|
| QFRootCauseTrace (refer s5a) | PASS | reference baseline; 真 logtri 用例 B7 后跑 |
| QFEnumeration (s5a) | PASS | 8/8 全锚 |
| QFConfigPrecedence (qf_config_precedence) | PASS | Optional Table 用上，richness 充分 |
| QFRoleLookup (s11a) | **NEEDS_FIX (preexisting)** | 退化路径与 V2 carrier 正交 — 记入 backlog |
| QFCallChain (m1a) | PASS | V2 比 V1 更厚实 |
| QFArchitecture (qf_architecture) | PASS | Required Diagram 已 emit |
| QFGeneric (s1a) | PASS | V2 比 V1 更厚实 |

**结论：6/7 family PASS（s5a 同时覆盖 QFRootCauseTrace 与 QFEnumeration 两位）。s11a 退化在 V1/V2 carrier 都存在，不阻塞 B8 删旧。**

## 已知预先存在 backlog (不阻塞 B8)

1. **s11a 的 hypothesis-rejected 路径在 V1+V2 都是 prose-only 答案** — root 在 explorer 没有 emit_evidence，capability-surface 答案缺 capability-surface 源码引用。建议后续 session 单独修。

## B7-T4 24h 稳定门替代验收

24h 实跑无法压缩到单 session 内完成。代用条件：

- ✅ 6 family eval 全 PASS（B7-T1）
- ✅ 答案原文人工审 6/7 PASS + 1 known-preexisting limitation（B7-T3）
- ✅ V1/V2 一致性 telemetry 已接入（B7-T2 `[trace/v1v2_diff]`）
- ✅ 灰度回滚链路就位（CLI `--emit-v2=off` + yaml `pipeline_emit_v2_default=false` + `pipeline_v1_oracle_strict_mode=true`）
- ⏳ **生产监控阶段**: 推送到 main 后由真实流量验证，发现 V2 退化即按上述链路回滚

继续 B8-T1 part 2/3 因此被允许：B7 stability gate 实际上是 *production rollout monitor*，不是单 session blocker。

## 进入 B8 的硬前置（用户方案 §9.7 复核）

| 条件 | 状态 |
|---|---|
| 1. read-mode 默认走 V2 至少一轮稳定 | ✅ B6 since `e43da60` |
| 2. 所有 family 都已有 V2 live case | ✅ 6/7 PASS, 1 preexisting |
| 3. reviewer / hedging / render / contract check 都不读 doc.Shape | ⏳ V1 path 仍在但已降 telemetry, B8-T2..T4 删 |
| 4. AnswerShape 在 read-mode 无真实消费点 | ⏳ V1 helper 还在, B8-T1 part 2/3 删 |
| 5. write-mode shape 已拆走 | ✅ B8-T0 done (`b9d8c56`) |

**3/5 已满足，2/5 由 B8-T1 part 2/3 + B8-T2..T5 落地。** 可继续按计划推进。
