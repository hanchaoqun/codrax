# Post-P0 高 ROI 路线图与进度跟踪 (2026-06-12)

承接 `repo_map_p0_low_risk_reanalysis_20260612.md` + `repo_map_p0_delivery_plan_20260612.md`
全量交付（41d06242→1210fe73，终审收口完毕）。本文是后续工作的唯一跟踪账本：
按 ROI =（影响 × 确定性）/ 成本 分三梯队；每批交付后更新状态列。

执行约束（沿用 P0 线红线）：精确信号进硬门禁/嘈声只软引导；禁关键字匹配用户
散文与模型输出；prompt 改动过 ATOMIC 7 + prompt_hygiene；不拟合单 case；
eval bar 不降级，FAIL 修系统；分批 commit+push。

## 第一梯队（本周，执行顺序 #1 → #3 → #2）

| # | 任务 | 状态 | 备注 |
|---|---|---|---|
| 1 | **全家族 eval 回归 + 工具调用效率测量**：`parallel_all.sh` 167 案全扫（P0 后首次全量；改了生产排序/banner/strict decode/教学/handoff）；FAIL 按红线修系统；用 run 指标（`tool_repo_map` 等计数）对比 P0 前历史 results，量化 steering 投资回报（§14.8 benchmark 第一步） | 进行中 | sweep 后台运行；快照二进制免受并行开发干扰 |
| 3 | **CodeGraph 调用次数预算引导** | 代码完成 | medium≥2/large 3/very_large 4;仅探索类视图;broad_fallback 连带抑制;措辞保住"引用前必读"契约(对 CodeGraph 原文的适配而非照搬);9 测试;eval gate 待 #1 sweep 结束后补 |
| 2 | **Route resolver 批 2** | 完成 | mux(逐 verb+ANY/PathPrefix 子路由)/Flask(methods kwarg 逐 verb+Blueprint url_prefix)/Kotlin Spring(新 annotation reader+**文法恢复怪癖修复**:多 controller 文件首类注解被拆为游离 prefix_expression,配对收割)/Express+NestJS(require AST 扫描补 CommonJS gate;JS 文法 decorator 形态差异处理;ArkTS .ets no-op 钉死);extractor bump Go5/Py4/Kt4/JS3/TS3 |

## 第二梯队（短期，1-2 个 session）

| # | 任务 | 状态 | 备注 |
|---|---|---|---|
| 4 | **source-bearing `repo_node`/outline 视图**（设计文档 §14.3）：container 节点返回结构 outline、普通节点返回有界源码窗口，砍 repo_map→read_file 往返——剩余最大效率杠杆。**必须先出 design doc**：与"导航非引用"citation 契约的边界（outline 可否作 citation anchor）需先裁定 | 待设计 | CodeGraph 核心差异化；勿直接动工 |
| 5 | **修复层豁免清单消化**：strict_decode_registry_test 的 18 工具 backlog 分 2-3 批接入（strict decode + x-codrax 注解），每批顺修 1-2 个 schema 措辞歧义 | 待做 | 机械；每批独立可交付 |
| 6 | **残差三小件打包**：(a) tracequery 4 新重型视图带 pattern 时接入 auto-window（OOM 路径最后一角）；(b) `mergeTurnAArtifactsForMutable` fork 合并点补界（A2 同款 count/byte 界）；(c) tokenizer 后缀剥离-lite（extractor→extract，修 hit@k 派生词形残差；不碰 CJK 路径，先加 bench 防误伤） | 待做 | |

## 第三梯队（中期 / 需用户拍板）

| # | 任务 | 状态 | 备注 |
|---|---|---|---|
| 7 | **Phase A finalizer rule bisection**（design doc 已审计落盘 `79e1b90`）：删 ~11 条 machine-checkable rules + hint composer 3 case；gate qf_arch 4/4 | 排队 | 与 repomap 线正交，memory 🔴 候选 |
| 8 | **eval 积压扫尾**：read_combo 21 案 + trace_query 6 案 + data-planner 3 案（形态清单在档） | 排队 | |
| 9 | **SQLite sidecar / watcher**（§14.1/14.2） | 明确不做 | 图规模/跨进程共享成为实际瓶颈前 ROI 为负；CodeGraph 行号敏感 id 教训在档 |
| 10 | **Survey mode** | 等 greenlight | §10 四个 open question 待用户决策，不自行启动 |

## 进度日志

- 2026-06-12: 路线图落盘；#1 sweep 启动（HEAD 1210fe73，167 案 6 并发）。
- 2026-06-12: #3 调用预算引导代码落地（repoMapNavigationCallBudget + 视图门控
  + 抑制联动 + 9 测试）；eval gate 排队等 sweep。
- 2026-06-12: 按用户改令 sweep 并发 6→2 重启（167 案全量）。
- 2026-06-12: #2 route 批 2 交付（4 框架;首轮 fan-out 被中断击杀后 mux/kotlin
  半成品验收续修——kotlin 多 controller 文法恢复怪癖由我修复;flask/express
  补发 agent 完成）。#3/#2 的 eval gate 待 sweep 结束统一补。
