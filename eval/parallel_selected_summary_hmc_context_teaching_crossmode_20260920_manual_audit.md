# 上下文与成文教学修复后的跨模式双例人工审计（2026-09-20）

固定干净构建 `02cea94edaa4`，批次 `20260920-033511`，2并行×1、每例1800秒；没有修改case/oracle，没有追加第三次追绿。结果根为 `eval/results/hmc_context_teaching_crossmode_20260920`。**机器1/2，人工0/2**。读例的正则PASS只说明源码与Trace内容同时出现，不代表机制解释或风险结论正确；写例的原4测试与plain probe通过也不等于整个写工作流完成。

| case | 批次耗时 / 最大上下文 | 过程 | 人工结论 |
|---|---|---|---|
| read_combo_trace_current_source_explanation | 383秒 / 42% | 2次trace_query、11次read_file；completion 4次＝1次DOWNGRADED＋1次硬拒绝＋2次接受；成文一次接受，零成文重试 | FAIL：正确86.111ms观测被错误源码链路解释；从解析能力推出当前业务风险及历史行为一致，无充分证据 |
| github_issue_dateutil_relativedelta_float_symptom | 576秒 / 29% | 原补丁应用成功；原4测试及Python探测通过；累计验证缺行为凭证，补证规划反复失败；最终blocked | FAIL：补丁方向正确且可恢复，但补证批身份丢失及证明能力不匹配使自动闭环失败 |

耗时采用批次汇总的端到端口径；读例 `run-1.metrics.txt` 的pipeline耗时为380秒，不与383秒混用。工具/拒绝次数按实际调用核对，不把重复日志行当额外拒绝。日志、原始附件、模型完整输出仅留本机，不提交客户数据。

## 1. 审计材料与定位约定

- 读例目录：`eval/results/hmc_context_teaching_crossmode_20260920/read_combo_trace_current_source_explanation-20260920-033511`；下文“读日志”指其 `run-1.logs/codrax-20260920-033513-000-91700.log`。
- 读报告：`.codrax/output/20260920-034131.756-91700.md`、同stem `.html` 和 `.root-causes.json`。
- 读原生查询：`.codrax/blob/20260920-033513-000-91700/trace-query-result-b636bda9.json`。
- 写例目录：`eval/results/hmc_context_teaching_crossmode_20260920/github_issue_dateutil_relativedelta_float_symptom-20260920-033511`；下文“初始规划日志”指其 `run-1.logs/codrax-20260920-033513-000-91719.log`，“写执行日志”指其 `run-1.logs/codrax-20260920-033703-000-98515.log`。
- 写工件：`run-1.plan.json`、`plan-1789900623764734000-91719.report.json`、同stem `.final.json`、`run-1.write-apply.json`、`run-1.materialization.json`、`run-1.applied-tree/`。
- 本文源码行号均对应固定revision `02cea94edaa4`，后续修复不得反过来改写本批失败收据。

## 2. 混合Trace与当前源码解释：观测正确，解释未通过

### 2.1 保住的能力

- 原生查询明确给出来源、TID 2000、已知CPU 2、B/E端点 `1000.100000..1000.186111`、86.111ms和原始第5–6行（blob第50–67行）；该条观测仍在成文上下文中（读日志4470附近），最终报告第15行保留端点与耗时。没有证据表明本例丢失时间窗或CPU信息。
- 回答承认附件仅两条marker，不能进一步分解调度、唤醒链或IO阻塞根因。最终文本**没有直接宣称已经证明丢帧**，不能扩大为“伪造丢帧数”事故。
- 必选旁路正常生成：`schema_version=2`、`root_causes=[]`、`status=unavailable`、`reason_code=no_selectable_typed_on_chain_candidates`。本例无调度链证据，空根因是正确边界，不是根因文件缺失。
- 最终5个区块为summary、3个ordered_list和decision（读日志4942/4952）；没有Mermaid图，HTML第308–321行是正常列表。本例既不是图语法故障，也不构成所有图表关系已验收。

### 2.2 实质错误

1. **普通marker解析与精确注释旁路被拼成一条处理链。** 报告第15/19/21行把本例普通 `tracing_mark_write` 接到 `ExactTraceMark → exactTraceMarkEvent → durationOrderObservations`，并称最后一项负责span配对和计时。源码实际普通路径在 `internal/tracequery/parse.go:4541–4553` 调用 `parseTraceMarkValidated`；`ExactTraceMark` 是独立精确注释表示（`exact_trace_mark.go:9–14/46–48`，`parse.go:4327`），不是每条普通B/E的必经阶段。查询的同步B/E栈配对在 `query.go:10229–10245`，持续时间在 `query.go:10616` 计算。若干局部源码描述属实，仍不能推出它们在本例串行相连。
2. **当前解析能力被当作当前业务风险及历史版本一致性的证明。** 报告第23行称“当前代码仍存在同类风险”，并断言与抓取时的行为一致。本次研究的是分析器如何解析外部Trace，不是客户RenderService当前源码或历史版本比对；能量到慢span不证明当前客户代码保留同一业务缺陷。
3. **60fps参考缺少条件限定。** 报告第15行直接以约16.67ms帧预算评判86.111ms；附件没有实际刷新率/帧预算见证。作为“若按60Hz参考”的条件性线索可以保留，不能把参照预算写成已确认的采集事实。无须删除真实耗时，也不能因此补造帧因果。

### 2.3 上下文与教学审计：未证明新的互斥合同

- 正确教学已到：读日志708明确机制解释不等于诊断；841/843限制“当前风险/当前版本”不等同泛指“读当前源码”；1207对错误组合给出两条合法修复路由，而模型1251自己把相关维度全部置真，1259接受。不是系统要求同一合法声明同时必带、必拒的复现。
- 成文上下文4393禁止把替代实现拼成顺序，4394说明定义不能证明函数体；4651–4658强调历史/当前边界且允许 `not_enough_evidence`。模型最后仍误拼和越界。优先归为“已给正确证据/教学但本次未遵循”的质量失败；仅此一次不能签成“已证纯模型波动、可销账”，也不应立即再加原文关键词硬门。
- 3300已排除不受支持的member_set；4520–4525把旧closure reason明确标为advisory且不可引用。不能称这些旧推测被系统重新授予证据权威。
- 过程有3次分析发射（1175/1207两次硬拒绝，1258接受）、1次不可用repo_map尝试（1060）；completion实际4次（2192 DOWNGRADED、2223硬拒绝、2248/3300接受），不是4次硬拒绝。成文仅一次接受，未出现成文无限重试或降级回退。
- perf_triage首轮把JSON工具形写进普通文本，没有真实工具调用，随后进入既有回退。本次最终时长有独立原生查询支撑，不能把未提交的秒/毫秒草稿直接认定为最终测量污染。

## 3. Python写模式：补丁正确不代表流程交付成功

### 3.1 已完成且未丢失的部分

- 应用树 `run-1.applied-tree/relativedelta.py` 在构造时通过 `_normalize` 保留整数、将整数值float转换为int、对非整数float抛出ValueError；不再把whole-float带到日期运算。`__radd__`未被更改。该修复与本例现象一致。
- `test_relativedelta.py` 与原fixture逐字相同，未改弱测试期望。验证报告有4条原生unittest成功记录与1条plain Python probe成功记录，source compile亦成功；报告的 `passed=true/verification_status=passed` 是执行结果，不能替代累计行为合同覆盖。
- 最终报告明确 `run_status=blocked`，活动批为 `batch-1-float-type-check-verification-probe`；`run-1.write-apply.json` 同时明确 `verify_authoritative=false`、`allow_unverified=false`。机器FAIL为 `write_final_run_status:blocked` 和缺少终验verdict，不是补丁正则不匹配。
- 已应用提交 **`3347c62584fea1c932a80f47608465c70e3895af`** 真实保存在评测fixture仓的 **`refs/codrax/applied/plan-1789900623764734000-91719`**，已用 `show-ref`核验；`run-1.materialization.json` 状态为resolved并保留该plan。工作树已清理，但提交和应用树仍在。不得写成“修复丢失”或“没有任何改动”。这里的fixture仓不是Codrax开发主仓，未自动合入开发主干。

### 3.2 首轮缺证：要求本身合法，模型漏交绑定

硬要求 `float_type_check` 需要 `relativedelta(months=1.5)` 的异常行为证明。初始plan虽列 `contract_refs` 并执行了plain Python探测，却没交 `project_test_observations`（下称PTO，原生测试断言与行为合同的结构化绑定）。报告明确 `verification_probe_missing_required_contract_ref`：仅声明refs和进程成功不能证明每项断言。

`internal/tool/run_tests.go:3541–3554` 区分Python plain probe的目标执行见证与逐合同观察；3740–3742附近需要准确PTO才能将原生测试断言绑定合同。初始规划日志1095/1186已教这一差别，1336也已读取现成 `assertRaises`，模型仍漏交PTO。因此最初补证不是误拒，不能用“所有测试绿”豁免硬合同。

### 3.3 系统gap A：补证批的控制器身份被模型改写

- 写执行日志1326：系统把缺硬合同覆盖路由到 `verification_proof_probe_plan_required`，创建 `batch-1-cumulative-review-proof-probe-plan` 这一补证批。
- 1861：模型的 `plan_batch` 改用 `batch-1-float-type-check-verification-probe`，purpose变成解释性中文。`internal/writeflow/controller.go:314`附近仅在同ID时保留已有身份；`internal/orchestrator/write_controller_scheduler.go:6722`附近当时只保护finish，没有保住该plan路由的控制器补证身份。
- 2374：带PTO的 `changes: []` 首先因“source-free sentinel不可携带PTO”被拒。2406模型按提示删PTO，**仍保留完整 `verification_probes[]`**；2408却被 `changes_empty` 拒绝。不能误诊为模型忘了附探测，实际typed补证授权在交接中丢失。
- 后续进入“源已修复但必须再写”的循环，出现full-modify与micro范围不符、no-op编辑和重复文件编辑。最终拒绝 `duplicate_change_path` 本身合法，但前面的系统交接缺口把正确补丁后的收尾拖成了重新造改动。JSON字符串数组兼容修复已执行，不是最后一次纯JSON解析错误导致停机。

修复方向：控制器拥有的补证身份、purpose与权限边界不随模型新ID/自然语言重述丢失；要有正反回归，不能把所有空changes计划一律放行或授权任意新批继承该权限。

### 3.4 系统gap B：补证要求没有核对运行器的实际证明能力

`internal/orchestrator/write_controller_scheduler.go:9239/9346`附近仅凭可执行runtime就要求补probe，却未验证该运行器能否产生目标合同的逐断言见证。Python plain probe在当前设计中只有目标执行权威；同时 `internal/tool/emit_change_plan.go:386–389` 禁止source-free计划携带PTO。于是即使修好gap A、保住原批身份，普通Python探测仍不能为本例缺失的异常合同提供所需凭证；重复执行不会升级证据等级。

修复应提供受控的、绑定既有应用代次/合同/真实原生断言的补证通道，或选择实际有逐断言能力的验证方式；不能扫描探测源码/输出字符串猜断言，不可把退出码0或模型 `contract_refs` 自动升级为逐合同证明，不应为了补凭证强迫写无意义源代码。A与B分别保留，不能只修身份后宣布整类问题闭环。

### 3.5 可见结果与边界

终态给出恢复ref，避免已应用成果丢失；但主段落仍是通用“本轮没生成改动方案/把目标说具体”，没有清楚区分“首批补丁已应用”和“后续验证补证失败”。这是本例的展示改进观察，不替代两个已确认系统gap，也不把该段表述当作仓库提交事实。

## 4. 本批新修复命中范围

`02cea94edaa4` 修复的是IO原生测量在宽泛profile下的口径说明交付，以及aggregate_facts达到上限时的错误类型转换教学。**本批两例既无原生IO统计，也没有aggregate_facts cap overflow，因此都没有命中这两条新路径。** 本批不能充作它们的live绿证据；也不能因为跨模式FAIL就倒推是两个局部修复造成了上述旧/新失败。对应公开入口、race和整仓测试收据另见统一差距文档，本收据不混淆确定性测试与真实模型回放。

本批没有因4ms总年龄主动终止活跃流的修改或观察；写日志保留首响应600秒、静默300秒、请求600秒配置。1800秒是本次case外层上限，两例均在其内自然结束。读例成文无降级，写例blocked来自补证规划失败，不是流式超时。

## 5. 开放项与下一步，不误销账

1. **P1 WRITE-PROOF-IDENTITY：** 先补可执行红针，修控制器补证批身份跨 `plan_batch` 交接；保持普通写批风险/审批/指纹与空changes约束。
2. **P1 WRITE-PROOF-CAPABILITY：** 独立修能力匹配及既有原生断言的受控补证，证明同一应用代次的硬合同闭合；不能仅靠探测成功签绿。
3. **READ-SEMANTICS：** 保留本次人工FAIL。后续异构源码解释回放检查替代路径误拼、解析器能力与客户业务风险混同、参考帧预算缺条件三类问题；目前正确教学已到，尚无新互斥合同复现，不以原文扫描或无限重试兜底。
4. 前批业务50ms/52ms窗口账户、IO表格缺语义列名、统计解释错误及accepted业务焦点补齐仍开放；本次跨模式回放没有重测这些失败，不得替它们销账。

本批只新增失败收据和可定位系统原因；不提升稳定任务清单的交付数，原13/79交付、66开放状态不因机器1/2变化。修复应分别提交验证，随后再安排固定版本的2并行回放。
