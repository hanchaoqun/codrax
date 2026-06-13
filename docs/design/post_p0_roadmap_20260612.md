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
| 1 | **全家族 eval 回归 + 效率测量** | 完成 | 167 案 2 并发全扫:154 PASS+13 FAIL→复跑分类后**实质 161/167**(7 jitter 复跑全 PASS);3 pre-existing(data-planner 残余,Tier-3 #8 在档);3 repeat-FAIL 定性列专项(见下);**6 个历史 FAIL 翻 PASS**。效率对比(tool_usage_compare.py,P0 前配对基线):repo_map +20%/trace_query +60%/read_file −6.3%——steering 方向性生效 |
| 3 | **CodeGraph 调用次数预算引导** | 完成 | 代码 ec9f74c8;eval gate sr_cpp_sink_impls PASS(新二进制) |
| 2 | **Route resolver 批 2** | 完成 | mux(逐 verb+ANY/PathPrefix 子路由)/Flask(methods kwarg 逐 verb+Blueprint url_prefix)/Kotlin Spring(新 annotation reader+**文法恢复怪癖修复**:多 controller 文件首类注解被拆为游离 prefix_expression,配对收割)/Express+NestJS(require AST 扫描补 CommonJS gate;JS 文法 decorator 形态差异处理;ArkTS .ets no-op 钉死);extractor bump Go5/Py4/Kt4/JS3/TS3;eval gate sr_java_annotation_route PASS(负向仍零产出,新二进制) |

## 第二梯队（短期，1-2 个 session）

| # | 任务 | 状态 | 备注 |
|---|---|---|---|
| 4 | **source-bearing `repo_node`/outline 视图**（设计文档 §14.3）：container 节点返回结构 outline、普通节点返回有界源码窗口，砍 repo_map→read_file 往返——剩余最大效率杠杆。**必须先出 design doc**：与"导航非引用"citation 契约的边界（outline 可否作 citation anchor）需先裁定 | 待设计 | CodeGraph 核心差异化；勿直接动工 |
| 5 | **修复层豁免清单消化**：strict_decode_registry_test 的 18 工具 backlog 分 2-3 批接入（strict decode + x-codrax 注解），每批顺修 1-2 个 schema 措辞歧义 | 已完成 | 见 `docs/design/post_p0_tier2_5_6_delivery_plan_20260613.md`；`strictDecodeExemptTools` 已清空 |
| 6 | **残差三小件打包**：(a) tracequery 4 新重型视图带 pattern 时接入 auto-window（OOM 路径最后一角）；(b) `mergeTurnAArtifactsForMutable` fork 合并点补界（A2 同款 count/byte 界）；(c) tokenizer 后缀剥离-lite（extractor→extract，修 hit@k 派生词形残差；不碰 CJK 路径，先加 bench 防误伤） | 已完成 | 见 `docs/design/post_p0_tier2_5_6_delivery_plan_20260613.md` |

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
  补发 agent 完成）。
- 2026-06-13: #1 sweep 收口(154+7复跑=161/167 实质);效率对比落账;#3/#2
  eval gate 新二进制双 PASS。**第一梯队全部完成**。
- 2026-06-13: repeat-FAIL 专项三案交付——libgit2(planner 上游同形+保守重建
  教学)/operation_web(operation 答案 prompt+answer-document 双表面用户词
  复用教学)/s3a(探针 case 定性反转,词表第三次扩充)。libgit2/opweb gate
  PASS 且内容逐项核绿;s3a+qf_architecture 回归 gate 批 2 进行中。
- 2026-06-13: 第二梯队 #5/#6 启动设计账本
  `docs/design/post_p0_tier2_5_6_delivery_plan_20260613.md`。确认复用现有
  strict decode repair、tracequery windowed index、TurnA handoff、normalizer/repomap
  resolver；任务拆为 6A/6B/6C + strict decode 5A/5B/5C，按批提交推送。
- 2026-06-13: 第二梯队 #5/#6 交付完成。6A/6B/6C 与 5A/5B/5C 已分批提交推送；#5 strict decode 豁免清单清空；#6 tracequery auto-window、TurnA fork merge bounds、ASCII suffix peel-lite 均落地并通过 focused tests。

## repeat-FAIL 专项（2026-06-13 交付完毕,全部 prompt/case 级,过 BLOCKING 流程）

- `github_issue_libgit2_foreach_worktree` **已修,gate PASS**:
  - 定性升级:复盘第三次 run 发现模型在两站点间"统一化"比较算子(上次统一
    `!= 0`,本次统一 `< 0`),而上游 PR 逐站点保形(cb 站 `!= 0`/lookup 站
    `< 0`);且 planner 无网络,引用的 PR URL 不可读——教学必须含"引用不可达
    时的保守重建"分支。
  - 落点:change-plan-skill 新增 UPSTREAM-REFERENCED FIXES 通用条目:参照
    已知上游修复保持同形;引用不可达时保守重建(最小语义差、保留各触点既有
    算子/操作数、禁止把相邻不同形站点统一化)。无任何 prose 关键字门控;R6
    审计不写案例答案形态进 prompt。
  - gate:worktree 字节逐形复刻上游双站点,PASS(新二进制)。
- `operation_web_manual_summary` **已修,gate PASS**:
  - 定性升级:该案走 route=operation 单发回合,最终答案由
    `commandOperationAnswerSystemPrompt`(command_operation_planner.go)驱动,
    **不经过 answer-document-skill**——首轮教学加错表面后复盘改正。
  - 落点:operation 答案 prompt + answer-document-skill prose-voice 双表面
    各加一条通用"复用用户对命名事物的原词指代,不同义替换"教学(后者覆盖
    repo 路由全局)。
  - gate:答案自然复用"用户使用手册"原词,PASS。
- `s3a` **已修,gate 待批 2 确认**:
  - 定性彻底反转:这是有意探针 case——问题键 shaped like explore_* 但被
    glossary ProjectSpecificIdentifierBlocklist 以字符串拼接形式刻意排除在
    配置表面外。两次 FAIL 的答案实质都正确(其一甚至挖出探针本体),是断言
    词表第三次撞上正确答案新措辞:(a)"无任何绑定"差一词没接上 `无绑定`;
    (b) 答案用真实同族邻居(ExploreMidLoopMinIteration/MaxMidLoopInjects)
    锚定而非 ExploreHeuristics 结构体名。
  - 落点:按该 case 在案两次维护先例做第三次词表扩充——no-CLI 正则收
    `无(任何)?绑定`;正向锚点改为真实同族标识符择一正则(全部存在于代码、
    无法靠回声探针键命中;探针键 camel 形刻意不收)。载荷断言(precedence
    链/yaml 文件名/no-CLI 裁定/env 负向)一条不动;三个历史答案回放全 PASS
    (含旧 PASS 不回归)。系统侧无缺陷,不属 bar 降级。
