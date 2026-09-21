# 504883389 已有测试绑定教学：写入与明确窗 Trace 双例人工审计

日期：2026-09-21。固定干净二进制 revision `50488338970c`，buildTime `2026-09-21T07:45:56Z`；2 并行 × 1，单例外层限时1800秒，runner正式exit0。没有改case/oracle、没有同版本第三次重跑。机器摘要见同名非manual文件。

| 场景 | 机器 | 功能/核心证据 | 完整人工结论 |
|---|---|---|---|
| Python relativedelta ordinary apply | PASS，235秒 | 源码修复、4个既有断言、probe及交付树正确 | FAIL：过程/上下文；不否认功能已通过 |
| 明确窗多跳唤醒链及IO | PASS，192秒 | 20ms窗、11ms IO、三条用户线程边、侧车正确 | FAIL：系统投影解释越权与模型措辞 |

结果根目录：`eval/results/hmc_existing_test_binding_20260921/`。以下文件均在各自场景的`*-20260921-004639/`下；原始日志/产物本地保留，不把中间推理复制进答案。

## 1. Python：功能通过不等于旧 required 补证失败已闭环

审计了`run-1.plan.json`、`plan-1789976993028821000-48539.report.json`、`.final.json`、`run-1.materialization.json`、`run-1.write-apply.json`、交付树及两份实际日志。最终仅修改`relativedelta.py`：整数浮点years/months规范化为int，非整数浮点拒绝；日期运算保持。`test_relativedelta.py`前后cmp一致，4条原生unittest逐断言PASS，附带probe也真实执行PASS；最终状态complete/verified，交付树解析为resolved。不是计划通过冒充apply或verify。

最终模型声明4条`project_test_observations`，绑定原有测试文件、suite、assertion和合同，未把测试列为文件修改。确实命中本批教学涉及的普通通道，但不是单变量A/B，不能断言全由新教学带来。本次所有行为合同都被降为`quality_repaired:planning_only_ungrounded`，report的VerificationConfidence只有`source_compile_ok`，没有`project_test_contract_ref_observed`。故不能据此抵销旧§30 required `float_type_check`缺绑定FAIL，也未触及source-free B2–B6。

实际上下文/过程仍有两项债：

- 初轮planner在同一工具批对错误的`dateutil/relativedelta.py`执行read_file和grep，两项失败就耗尽2/2预算（初始日志1199–1211）；工具错误建议repo_map，下一轮该工具却被移出可用集合（1228–1235）。之后发生错误路径/旧文本等发射拒绝，已有结构化修复窗口及跨轮重新规划最终恢复，并非永久无恢复通道。确定性缺口是一个并行失败批耗尽本应留给纠正的机会；应按普通失败观察轮计费，成功读取仍逐调用、专用补证仍逐read_file计费，不增加源码权限。
- 分析器已读真实测试，但内部`int-months-still-works`的comparator.subject把months=12与预期2020-02-15并置；合同主subject与实际测试都是months=1。因无证据引用它只是规划信息，未铸成精确行为证明；最终补丁和测试正确。此上下文错配继续留账，不能扫描自然语言expected或替模型补造合同证据。

初始日志`codrax-20260921-004642-000-48539.log`可核5次emit_change_plan、4次拒绝（1288/1312/1339/1683），1714首次成功。后一次是verification_probes被误放为独立changes成员、缺path；结构化参数兼容处理没有擅自重新归属语义字段。机器摘要中的rejects=0是成文维度，不代表写规划零拒绝。已有明确JSON形态教学，不能因一例重复扩大硬约束或把未改测试强制加入changes。

## 2. Trace：数值/链/侧车保住，状态覆盖不能冒充原因闭合

审计`run-1.principal.md`、`run-1.answer-transcript.md`、实际finalizer输入日志、3份原生查询结果及mandatory sidecar `.codrax/output/20260921-004949.839-48519.root-causes.json`。请求、三次查询及侧车均为2.000–2.020秒；app sleep20ms，pool IO11ms，三个runnable各1ms，未把2.020020的窗外运行混入或相加嵌套睡眠量。pool→network→cookie→app方向及跨CPU位置正确。图是闭合text树，无Mermaid格式错误；第二轮模型自主选择唯一11ms IO项，schema v2/status available，未擅自加入优先级反转候选。

确定系统缺口：

1. `answer-transcript.md:62–63`把20ms称“链上已归因”“已由链上解释”，但实际final输入日志2464明确原因分解未闭合。生成函数取TargetImpactMS为覆盖分子，数据不是算错，错误是将链路状态覆盖的度量升级成原因解释闭合。应修统一覆盖文案，保数字、分母、时间窗、计量和链资格，不新增散文硬门。
2. 正文第15行把cookie17ms/network14ms睡眠称“链上有效工作贡献”；最终typed输入2447/2449已分开占用与1ms attribution，不是数值缺失。系统投影第96行也给两条sleep套上“杠杆=自身工作量”，出自unpriced通用分支。应按typed state区分等待与工作方向，不能把完整睡眠都当可优化工作，也不能丢失链上业务线索。

模型呈现债：正文还泄漏`lock_priority`、`scheduler_iowait`、`kernel_callsite`，虽实际输入已教读者语言；“扩展到2.014秒之后”也未准确承认当前窗已到2.020。继续留账，不新增原文替换或关键词硬拒。

两项排除误判：case HTRACE经runner `--htrace-text`明确注入Harmony来源，typed priority说明与最终教学一致，不能用generic Linux优先级解释反判本例；但此例也不证明所有真实trace来源识别正确。D/IO终端停递归为现有明确设计，irq-2→pool-400@2.014仍在census中，未画IRQ且未否认唤醒不等于丢链；不为凑图增加未证根因。

## 3. 处置顺序与销账边界

先完成§69普通已有测试教学子片收据；并行修普通写规划失败批计数、Trace覆盖/等待方向两项确定性系统缺口，分别公开红绿、相邻/race及末版全仓，分批提交推送。旧live人工FAIL不回写，新测试通过不倒签答案。HMC父账仍13/79已交付、66开放；B2–B6、用户明确窗来源权威、模型上下文/读者语言等继续保持原状态。
