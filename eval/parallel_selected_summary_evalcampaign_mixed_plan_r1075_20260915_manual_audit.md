# r1075 人工审计：混合 Trace/源码读与 C++ 仅计划（2026-09-15）

## 执行及结论

- 代码：`6859fe81192c`，此前 B1698 `477025af9`、B1697 `6859fe811` 均已推送。提交后清洁构建 `0.1.20260915 / 2026-09-15T08:09:07Z`。
- 2026-09-15T08:10:14Z 启动，`CAP=5 / PARALLEL=2 / TIMEOUT=1200`，两份既有 case 各一次，沿用各自 15 步；无第三路、无重跑、无改预算或 oracle。
- 243 例为 215 read / 25 apply / 3 plan；近期 r1074 已覆盖纯 Trace 与 Java read，r1073 有 C apply。此次补久未回放的混合来源 read（最近 20260604-204422）与 C++ plan-only（最近 r528/20260815），不是按失败反复刷同一例。
- 机器原汇总：[r1075](parallel_selected_summary_evalcampaign_mixed_plan_r1075_20260915.md)。runner 退出 0 仅表示跑批完成，**机器 1 PASS / 1 FAIL，不是两例通过**。

| case | 机器 | 人工 | 范围和实际过程 |
|---|---|---|---|
| patch_cpp_typo | PASS，63s，ctx 28% | 核心 PASS；计划质量 P2 | 仅修复计划；读 2 / 列目录 1；一次计划工具精确拒绝后成功；未 apply、verify、编译或运行 |
| read_combo_trace_current_code_dimensions | FAIL，274s，ctx 33% | FAIL；数值和五维保留 | read 1 / trace_query 3 / repo_map 0；5 次调查、6 次中途检查；成文拒绝 1 / patch 1 / unavailable 0 |

B1697 原 capture 有界回读、B1698 自动注释范围在本对均未自然触发，生产验收为 **N/A**，不以机器结果代替专项正证。两例都没有图，图关系/时序/逻辑与 Mermaid 渲染均 **N/A**。

## C++：计划正确，不等于已修复和验证

结果目录 `eval/results/patch_cpp_typo-20260915-011014`。日志简称 P：`run-1.logs/codrax-20260915-011016-000-52290.log`。

1. `run-1.plan.json:5–19` 的 patch 与结构化 edit 一致，只把 `main.cpp:19` 的 `retrun` 改为 `return`。`run-1.plan-1.json` 与其内容相同。夹具与 scratch 的源码均未改变，不触碰主仓源码。
2. P:953 已明确教 C++ 不用 Python 包装编译证明；P:1152 模型首稿仍提交包装探测，P:1160 精确拒绝，P:1193 接受改正后的计划。这是一次合理的工具级纠正，不是 JSON 畸形或本轮“成文重试烧尽”。
3. 正式状态 `run-1.repo/.codrax/plans/plan-1789459876061947000-52290.final.json` 保留 `planned / pending_approval`；patch/verification 凭证为空，证明 `unknown / runner=none`。`loop complete` 只代表计划流程结束。工具启动时存在编译器不等于已执行验证，计划中的“修复后可通过”也不冒称已经编译。
4. 保留 P2：`run-1.plan.json:38` 起的 placement 同时含 `expected=return` 与 `line_local_not_contains`，意图矛盾但已标为 `planning_only_ungrounded`，没有变成验证权威。验收文字 `./main` 同时预期 world 与 Alice，却没给 Alice 参数；无参数实际只应 world。后续改进结构化计划/执行证明，不从自由文字做新硬门。旧 case 注释“无 g++”不能当当前环境事实。
5. 原夹具与 scratch 的 `main.cpp` SHA256 均为 `e33da0ac80809024484627628026c0f488dd636441b844d6c98e1fa0d5f1cfea`。scratch 只有运行时辅助 `.gitignore` 未跟踪，不宣称整个 scratch 零文件变化。没有为了人审再执行 apply 或编译。

## 混合读：核心测量正确，但来源与当前实现未闭合

结果目录 `eval/results/read_combo_trace_current_code_dimensions-20260915-011014`。

- 日志 L：`run-1.logs/codrax-20260915-011016-000-52275.log`。
- 答案 A：`.codrax/output/20260915-011446.411-52275.md`，同名 HTML 与 root-causes.json 一并保留。
- 查询 Q：`.codrax/blob/20260915-011016-000-52275/trace-query-result-07958df0.json`。

Q:65–67 的两条 B/E 给出 1000.100000s 至 1000.186111s，即 **86.111ms > 50ms**，超过 36.111ms / 72.222%。A:15–30 保留五个用户维度，测量和比较正确。这是 span 墙钟，不是执行 CPU 时长、完整帧截止时间或链上根因；291B 夹具没有调度、IO、Binder、帧截止时间证据。

机器失败条件是未匹配 `internal/(analysis|tool|agent|orchestrator|types)/...go:行号`；不能仅由正则失败判整份答案错误。独立人工失败依据为：

- L:3290 唯一源码读取是 `internal/skill/defaults.go` 提示文本；没有查到当前工具的实际计算实现。A:21 的“仓库唯一相关/没有实现”超出一次读取可证明范围。A:30/40 坦承不是设备 RenderService 源码是正确边界，仍不能填补用户要求的“当前关键代码”。
- A:27 把 DoFrame 说成完整 measure/layout/Drawing 全流程，证据只是列举多个 span 标签，不能证明包含/时序关系。A:18 把 B/E 扩称帧开始/结束也过强。
- A:18 的覆盖度 1.00 被扩写成“无解析残余”；Q:46/95 实际有 4 条未解析头部行。不能混用覆盖口径，也不能反过来把这些头部当缺事件。
- A:32 的 B 时间戳被附上格式教学源码引用，A:42–46 系统补表把 `86.111ms 50ms` 放进“符号名称／定义位置 defaults.go:1238”。引用与事实不相称，具体所有权链如下。
- A:30/38/40 有内部 lane/validator-owned 等术语及中英混用。记作模型教学与系统事实展示的 P2，不改原答案或做输出关键词硬门。

### B1699，P1：同一源码义务的收束提示与完成条件分叉

L:48 路由 `current_source=required`；L:1019/1475/1929/2408/2838 的实际快照持续为 `req=soft:lane=required:required=true:satisfied=false`。与此同时 L:1383/1853/2284/3199 提示源码 optional、优先收束，L:1477 等又因 `missing_origin_lanes=current_source` 不能自动完成。这会重复派调查，并未帮助模型定位真实实现。

代码核实：`internal/types/external_observation_sufficiency.go:81–85` 仅把 required 且 precise 算为阻断外部单独充分；`runtime_source_answer_authority_view.go:145–158` 仍保留 required lane。`internal/agent/explorer.go:731–732` 每次供给当前 AnalysisIR/TurnRouteHint，不是已证的陈旧 IR 或模型取消义务。

**下一片先做公共先红后绿**：区分“运行时问题已足够回答”与“源码部分仍需调查”的范围，统一 typed 义务来源和指导/完成语义。不把 soft 简单硬化，不把源码义务一刀删除，不根据用户原话判门；要求纯外部可收束、混合来源不被误教为全题完成、同一个状态跨 dispatch 一致。

### B1700，P1：混合聚合获源码清单资格，并触发错误引用扩充

所有权不能简单归为“系统凭空造数”或“全是模型波动”：

1. **模型提交**（L:3387）：`member_set` 的 member 是 `H:RenderService:DoFrame 持续时间 86.111ms > 50ms 阈值`，support_refs 却填 `defaults.go:1238/1239`。这是模型先错误关联实测事实与格式教学。
2. **系统资格提升**（L:4182/4266）：模型聚合被供给为 `current_source / independently_proven`，并编成 `location=defaults.go:1238` 的 PrincipalEnumerationRow。格式文档并没有这个运行时值或源码成员声明。
3. **系统新增显示**：原始模型成文 L:4437 只有 summary、ordered_list、caveat，没有表或引用池；L:4441 的 `appendPrincipalEnumerationTypedSupplements` 追加了最终 A:42–46 的伪源码清单。这条补表资格是本次真正应修的系统缺口。
4. **正确拒绝后仍有扩充缺口**：L:4444 起正确拒绝将 runtime observation ID 填入 current-source `evidence_ids`。模型 L:4511 仅删这些 ID，保原文字；L:4514–4516 的系统 helper 通过正文反引号 `B|<pid>|<tag>` 匹配教学 quote，给原无引用的实测条目新增源码引用。不是模型主动选择这个 citation，也不是系统删除模型正文。

本例是 **member_set，不是 scalar_value**。`aggregateFactHasIndependentTypedAuthority` 仍复用 B1695 的资格判据，两 consumer 没有再次分叉；缺口在普通非关系/工作流聚合的 exact-source 资格不足以证明实际成员/主张。下一片测试并收紧结构化主张与已观察源码证据的资格边界；禁止数值/prose 形状作硬门、禁止恢复两处独立判据或系统代写结论。

自动反引号引用已有相关账 `EVAL-B21-CIT2 / B657`，旧修保护的是已有正确绑定；本次是给**空引用运行时条目新增源码引用**，不能冒称旧修回归。作为 B1700 同源显示/来源子通道连同 full/patch、公用 helper 审计，不另起单例编号。

### 根因文件、图、JSON 与超时边界

同名 root-causes.json 已必出，`schema_version=2 / root_causes=[] / status=unavailable / reason_code=no_selectable_typed_on_chain_candidates`。两条 B/E 不能构建链上根因，这不是文件遗漏，也不能要求系统补出 IO/供给/帧因果或把背景升为根因。无图不记图能力通过。

初始分析 root_cause 与 bounded_effect_verdict 不一致后，模型改为 causal_diagnosis，列后续路由教学观察；本次不据此武断归因投影丢失。最终一次拒绝是来源槽位问题，JSON 并未畸形，也没有观察到活跃流因无正文而被超时降级。

默认首响应 600s / 真正中途无字节 300s / 非流式 600s 未改；心跳、推理、工具等实际字节仍续活，4ms 或旧 4 分钟无答案正文不应触发降级。显式调用方 deadline/cancel 与 eval 1200s 外层边界独立。

## 收据与下一优先级

- 全仓验证属于前两修复的冻结联合代码：`.codrax/tmp/20260915-b1697-b1698-full-v3.log`，86 个有测试包全部强制重跑通过、13 无测试包；中间失败日志、B1694 已知缺口 SKIP 保留。不是每个孤立提交各跑一次全仓，也不是零 SKIP。
- 跑批日志：`.codrax/tmp/20260915-r1075-runner.log`。
- 原 case/夹具 SHA：`.codrax/tmp/20260915-r1075-case-fixtures-before.sha`；原结果与答案 SHA：`20260915-r1075-results-audit.sha`、`20260915-r1075-answer-audit.sha`。审计不改原日志、模型答案、侧车或计划。
- 原机器 summary SHA256：`3c91607bd7c2858e1e397c8abde679ad4399949fb20ab24ea119207769131525`。只填写本人工文档与统一账，不改变机器判定。
- 下一顺序：**B1699 公共复现/统一源码义务 → B1700 聚合资格和新引用来源 → B1694 引用范围与同坐标异语义债**；随后轮转异构 read/write/Trace 图例，每批仍恰好 2。全部图语言/关系/时序/逻辑矩阵、B1319/B1561 原生验证证明等旧债保持 OPEN，不由本对代销。
