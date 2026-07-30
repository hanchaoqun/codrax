# EVALFIX-2F — read_combo_log_current_code_boundary PASS→FAIL 判案（2026-07-30）

Case: `eval/cases/read_combo_log_current_code_boundary.case`
Bar (3 regex arms): 第4行↔重写词面 / 网络-校验区分词面 / **`internal/(orchestrator|agent|llm|render)/…\.go:[0-9]+` 当前源码引用**。
Fail shape: 两次 FAIL 均为 `no_regex_match` 第三臂（答案零源码引用）。

Runs adjudicated:
- OLD PASS: `eval/results/read_combo_log_current_code_boundary-20260730-040930`（binary rev 81ccf22b0）
- NEW FAIL ×2: `…-20260730-075705`、`…-20260730-080152`（post EVALFIX-1 + 2A..2E + SWEEPFIX-R6 工作树）
- 历史 PASS ×3: `…-20260612-005644`（read_file=5）、`…-20260612-024958`（read_file=3，引 `internal/orchestrator/user_messages.go:250` + `internal/render/status_messages.go:222` ≈ 真 owner）、`…-20260613-003255`（read_file=17）

---

## 1. Analyzer emit_analysis IR 对比（问题 1）

| 字段 | OLD PASS 040930 (iter=1, log:690) | FAIL 075705 (log:695) | FAIL 080152 (iter=1, log:696) |
|---|---|---|---|
| keywords | 14 | 17 | **0** |
| entities | 5（含 `render` → 预扫验证锚 "render (symbol)" 进入 explorer prompt） | 5（无 `render`，无验证锚节） | 4 |
| current_source_explanation_profile | Active, modes=[explain_current_mechanism, locate_current_code] | Active, modes=[locate/verify/explain] | **整体缺失** |
| external_observation_policy | `external_only` + `current_source_mode=allow` | **缺失** | **缺失** |
| requested_answer_dimensions | 2 维含 role=`current_key_code`（"结合当前源码说明"） | 缺失 | 缺失 |
| is_cross_component / predicate_axis | true / condition | false / "" | true / — |
| diagnostic_profile.current_version_check | false | true | true |

**判定**：三份 IR 形状差异全部是 LLM 输出方差，不是代码。证据：

- analyzer system prompt 三 run 字节一致（len=71503，diff 后 "ANALYZER SYSTEM PROMPT IDENTICAL"）；analyzer user msg 差异仅为 log_triager 的 LLM 输出（triage bundle 行文/置信度/`log_line` 字段），log_triager prompt 三 run 同长（system=10191 / user=1513）。
- explorer system prompt 字节一致（len=33212）；explorer user msg 差异全部由 IR 决定性投影而来（验证锚节有无、probe 任务措辞、lane plan dimensions）。
- **EVALFIX-1 的 exact-targets keep 臂未参与**：三份 emit 均未携带 `exact_targets`，`validateExactTargets` 的新 verbatim-keep 分支（`internal/tool/emit_analysis.go:3848+`）从未执行。
- **EVALFIX-1 的 coherence stat 臂未参与**：该臂只对 file-shaped sub-topic entity 增加放行路径（`internal/analysis/gate/coherence.go` `subTopicEntityGroundedOutsideSymbolUniverse`），本案 sub_topics 实体均为 prose；三 run 均无 coherence 拒绝。OLD run 唯一一次 analyzer 重试是 strict-decode 未知字段（log:669 `json: unknown field "required_answer_dimensions"`），与 coherence 无关。
- 窗口内 diff（`git diff 81ccf22b0..HEAD`）**不含** `internal/tool/emit_investigation_complete.go`、`internal/orchestrator/tier1_floor.go`；触及的 `emit_analysis.go`（字段名教学 + misplaced-hint `requested_files→required_files`）、`log_triager.go`（2B 段级 floor，本案单段小日志零命中，已排除）在本案执行路径上惰性。

结论：**IR 形状确有劣化（080152 连 typed 源码请求 profile 都没发），但劣化不是 EVALFIX 批次造成的**——同一 prompt 下的采样方差。explore pressure 的下降由 IR 方差经由决定性投影传导，且旧 run 的 "更好 IR" 同样没有触发任何硬门。

## 2. 完成门与 pre-finalize floor（问题 2）：三层守卫全部看见、全部放行

**(a) CGEC forced-read 门 —— 看见 `decision=required`，但 skip。** 两个 FAIL 与 OLD PASS 打出完全相同的四行（PASS log:1328-1331 / 080152 log:1222-1225 / 075705 log:1161-1164）：

```
INFO [CGEC] primary_anchor_unread: skipped current-source forced read decision=required
INFO [CGEC] phase1_unread: skipped current-source forced read decision=required
INFO [emit_investigation_complete] multi-topic anchor backbone bypassed by <system-detected external-source log | evidence_floor_waiver=external_only_log>
INFO [emit_investigation_complete] citation-floor bypassed by <同上>
```

机制（代码未在窗口内变动）：`raisePrimaryAnchorPendingRead`（`internal/tool/emit_investigation_complete.go:10528`）→ `currentSourceForcedReadGatesApply`（:10345）返回 false：附件是 external-log carrier，且 `RuntimeSourceRequestCurrentSourceRequirementPrecision` 判 **Soft** —— 因为 `runtimeSourceAuthorityPreciseCurrentSourceRequirement`（`internal/types/runtime_source_answer_authority_view.go:403`）只认 file-shaped quote：`currentSourceExplanationHasPreciseCurrentSourceQuote`（`internal/types/request_traits.go:1896`）要求 SourceQuotes 是代码路径或 file:line 面。题面 verbatim 引文「请结合当前源码说明」是纯 prose → 不 precise → 硬门不适用。`CurrentSourceLaneDecision()` 明明返回 "required"，被降为软车道。

**(b) citation floor / anchor backbone —— 被 typed escape 放行。** 075705：explorer 的 emit_evidence 因 `runtime_artifact:…` 非 repo 路径被逐项拒（log:1132-1136，拒绝文案为旧有 per-item validation，非新教学），随后模型自选 `evidence_floor_waiver`（系统 prompt 第 21 条教学，新旧两 run 字节相同），log:1159 `evidence_floor_waiver accepted: reason=external_only_log`。080152：无需 waiver，`runtimeArtifactGroundingBypassAllowed`（:4782）经 system-detected external-source log 放行。软义务仅折为 completion caveat（`recordSoftCurrentSourceCompletionCaveat` :10410）。

**(c) pre-finalize grounding floor —— 看见 gap，只披露。** 两个 FAIL（075705 log:1221 / 080152 log:1281）：

```
WARN [orchestrator] pre-finalize grounding floor detected a gap; continuing to finalize with disclosure
(arm=followup_coverage exhausted=false): Source localization is not yet narrow enough to finish safely. Missing repo_map lenses: …
```

`internal/orchestrator/tier1_floor.go:105` `discloseTier1FloorGap` —— §29.60（2026-07-13）裁定：ACCEPTED 模型完成对质量类臂是 terminal，只披露不 requeue（endless_loop.txt / donghu 4/4 见证）。披露确实到达答案面（FAIL 答案尾注「系统建议的部分补充定位/钻取步骤未执行…按未验证对待」）。OLD PASS 无此行（有 read 覆盖，floor 未触发）。

**判定**：按 完成门权属模型 红线，本案是 **零见证（fatal class）+ 题面显式要求源码** 的合取，本可拦；但现行谓词把「显式要求源码」的 precise 判定收窄到了 "quote 必须 file-shaped"（§29.146 UPSTREAM-3 反 prose-mint 裁定的过度延伸），致使 fatal-class 臂在这个形状上**结构性不可表达**——三层守卫各自正确地执行了自己的（过窄）契约。

## 3. 归因提交（问题 3）

窗口 `81ccf22b0..HEAD` 共 8 个提交（EVALFIX-1 4fe5e8af8 / 2A bde3aa7cd / 2B 6831cfc83 / 2C a8369acbf / 2D 6f1716639 / 2E 11e6a2444 / docs 2904fd63c / R6 39fcbb77d）。**没有任何提交改动本案执行路径实际消费的表面**：

- 全部 LLM-facing prompt 字节一致（log_triager / analyzer / explorer 三级实测 diff）；
- 完成门与 tier1 floor 文件不在窗口 diff 中，且 PASS/FAIL 打出逐字相同的 skip/bypass 行；
- EVALFIX-1 两臂（exact-targets keep、coherence stat）经实证未执行；2A gate-teaching 只改 retry hint（本案唯一 hint 是未变动的 emit_evidence per-item 拒绝文案）；2B 段级 floor 零命中（题面已排除）；2C/2D/2E 位于 finalize 后置检查/披露面；R6 strict-decode 改动未被触发（新 run 无 near-miss 字段名）。

## 4. 诚实分类（问题 4）：**(b) 既有答案不稳定，旧 PASS 是错锚掩盖的运气 PASS**

- 该 case 的第三臂从未被任何硬门保障过：external-log carrier + prose 源码请求 = 软车道，PASS 与否完全取决于 explorer 的自愿读源码（软引导地带的 LLM 方差）。历史轨迹：read_file 5→3→17（6月，其中 024958 读到真 owner）→ **1**（0730 旧 PASS）→ **0**（新 FAIL ×2）。
- 旧 PASS 双重运气：(i) explorer 自愿读了一个文件；(ii) 读的还是**错误 owner** —— `internal/orchestrator/finalizer_visible_timeout_fallback.go:14/:16/:20`（`StreamNoVisibleOutputTimeoutError` 兜底面板），而日志第 4 行短语「模型响应出错,正在重新撰写答案」的真 owner 是 **`internal/render/status_messages.go:233`**（finalize 行 retry 面）与 **`internal/orchestrator/user_messages.go:259`**（retryNotice）。regex 第三臂只认目录族，错 owner 也 PASS。
- 零见证的代价实证：FAIL 答案把 `⟳ 4/4` 混构为「重试预算 4 次已全部耗尽」——4/4 实为 4 阶段管线的 finalize 序号徽标（同一 run 自己的进度行 `› 4/4 正在生成最终答案` 可证）。**bar 不能降**：第三臂正是拦这种混构的。
- 不是 (a)：无提交改动任何被消费表面。不是 (c)：窗口内代码对本案行为零参与（(b) 的"掩盖"在窗口前就成立）。

## 5. 修复方向（不实施；尊重完成门权属模型与精确信号红线）

**5.1 新增 fatal-class 精确合取臂（核心修复）**。在 emit_investigation_complete 完成门增加一个由三个 PRECISE 信号构成的合取：
1. `CurrentSourceExplanationProfile.Active()`（schema-validated typed boolean），且其 `SourceQuotes` 中存在**逐字命中原始请求**的引文（verbatim substring —— 与 EVALFIX-1 在 `validateExactTargets` 用过的同款精确判定）；
2. external-observation carrier 在场（现有 typed 判定）；
3. 当前源码见证计数 == 0（proof-lane 红线口径：只数确定性工具见证——read_file 覆盖 / grep / typed observation，即 `CurrentSourceSatisfied` 同源谓词）。

合取成立时**不接受静默 bypass/waiver 放行**，改走**既有**降级车道 `currentSourceLaneCoverageDowngrade`（:10377，RepairExpandSearch + 定位教学，一轮有界重开，受 RetryBudget 约束，无 requeue 循环）。这是 零见证 fatal-class 拦截，不触碰"结论一致性指控"禁区，不违反 §29.60（拦的是零见证结构缺失，不是质量类臂）。

**5.2 收窄 evidence_floor_waiver 的覆盖范围（typed escape 保留，§1.6）**：waiver 语义应是"豁免 artifact 观察的 repo-grounding"，不应连带豁免题面 typed 源码解释义务。当 5.1 合取成立时，waiver 可继续豁免 artifact 车道，但不覆盖 current-source 车道（真正 external-only 且用户未要源码的场景不受影响）。

**5.3 精确谓词补第二臂**：`currentSourceExplanationHasPreciseCurrentSourceQuote` 现把 "precise" 等同于 "file-shaped"；应补 "typed profile + verbatim 请求引文" 臂（boolean + verbatim match 均为红线认可的精确信号）。§29.146 反 prose-mint 裁定针对的是**噪声词面推断**，不是 analyzer 显式发射、逐字可核的 typed 引文。

**5.4 case bar 真锚修正（只升不降）**：三臂全保留。可在 case 注释记录真 owner（`internal/render/status_messages.go:233` / `internal/orchestrator/user_messages.go:259`），并考虑追加一条不降低现有臂的真锚加强（如第三臂命中文件须与日志短语 owner 目录族一致）；6月 024958 run 证明真锚可达。

**5.5 程序纪律**：本修复批按既定教训必须进入下一轮 self-sweep 靶面；落地后本 case 至少 2/2 复跑 + 一条负 pin（外部日志 + 无源码要求的题面仍可 waiver 完成，防 5.1 过杀真 external-only 场景）。

## 附：证据坐标速查

| 证据 | 位置 |
|---|---|
| OLD PASS 工具序列 grep×2→repo_map→read_file→complete | 040930 log:1063-1064,1129,1180,1326 |
| FAIL 075705 序列 repo_map→emit_evidence(拒)→waiver complete | 075705 log:1069,1132,1159 |
| FAIL 080152 序列 grep×2→complete（"从日志本身已经能够完整回答…不需要读取当前源码"） | 080152 log:1104-1105,1215,1220 |
| 三 run 相同的 CGEC skip / citation-floor bypass 行 | 040930:1328-1331 / 075705:1161-1164 / 080152:1222-1225 |
| tier1 floor gap 仅披露（仅 FAIL 触发） | 075705:1221 / 080152:1281；`internal/orchestrator/tier1_floor.go:105` |
| 软化谓词链 | `internal/tool/emit_investigation_complete.go:10345,10377,10528,11187,4782,3458`；`internal/types/runtime_source_answer_authority_view.go:390-421`；`internal/types/request_traits.go:1896` |
| 真 owner | `internal/render/status_messages.go:233`；`internal/orchestrator/user_messages.go:259` |
| 旧 PASS 错锚引用 | 040930 run-1.out：`finalizer_visible_timeout_fallback.go:14/:16/:20` |
