# Native resource identity — fixed-pair manual audit

日期：2026-09-23。代码快照 `bcc498bd00b5`；构建标记 dirty 仅来自文档，Go/依赖/构建输入冻结。84753 正式 exit 0，恰好 2 并行、各 1 次；没有第三例或复跑。机器判定单独保存，不改答案、oracle 或历史失败。

| 用例 | 机器 | 完整人工 | 外层耗时 | 公开工具 | 结论 |
|---|---|---|---|---|---|
| trace_query_native_resource_identity | PASS | FAIL | 248s | trace_query 10；read/repo_map/list/source_lens 均0 | 精确数值及字段完整到场，但地址有效性、位型算术及NULL解释错误 |
| trace_query_smartperf_resources | PASS | FAIL | 130s | trace_query 1；read/repo_map/list/source_lens 均0 | 主要量值正确，但系统把地址补成路径、线程补成领域，并有范围/源码附录越域 |

## 1. 新增资源身份用例

结果目录：`eval/results/hmc_native_resource_identity_20260923/trace_query_native_resource_identity-20260923-023455`。完整日志：`run-1.logs/codrax-20260923-023457-000-54074.log`。完整报告：`.codrax/output/20260923-023901.034-54074.md`；同名HTML、answer-surfaces及root-causes旁路均保留。外层248s，内部summary246s不混记。

主审和独立审查阅读完整正文、原始夹具、过程和实际Finalizer上下文。日志3276的最终清单包含5条完整原始记录，无截断、5行全部展示；9007199254740993、-1、-9223372036854775808及其hex位型、子类ID、显式null/空串/字段缺失都保持原样。这里不是JSON浮点损失，也不是系统把name补成null。

通过范围：指定0.0005–0.0055秒；5条I操作与5条C采样分开计数；HeapSize 8192→4096→4128、MmapSize 4096→8192；窗外FD不混入。表格整数/位型准确，缓存竖线正确转义，未制作图、未把资源记录晋升卡顿根因。这个手写文本fixture只验模型可见查询和解释，不能作为binary/SQLite producer命中证明；producer由公开DB回归单独验证。

失败事实：

- 完整报告53行、principal39行把`0xffffffffffffffff`说成无符号0/NULL，这是算术错误；又无依据断言`0x8000000000000000`必为标记值/无效地址、Free地址0必为空操作、Mmap负值暗示失败。保位型不等于证明地址有效或无效。
- 完整报告63行、principal49行把操作5没有发布的name说成显式name=null；操作3的显式字典NULL又解释成未提供字段。完整输入已区分这些状态。
- 完整报告67行对资源end极值猜占位语义，81行/principal67行笼统说没有sched_switch数据；原fixture第3/16行存在调度包络，资源查询没有发布线程状态账，不等于整个附件没有调度数据。没有据此编造根因，但缺测理由失真。

上述坏句已经出现在首次模型`emit_answer_document`（日志3442）；后续patch（3483）只补表格facet关联，没有注入这些解释。日志3273的原生资源合同已说明end极值不能预设sentinel，模型仍误述；但新地址位型及显式NULL/缺字段的共享reader语义尚未完整接入教学，留16.4，不宣称全是已证模型波动。3094说明Finalizer不再重复整段raw trace，不能由未投递scheduler原行推断附件没有scheduler数据，也不应把有限资源问题强制扩成调度调查。

首次接受后因required多主题调查重开（1410），1440–1442启动两路，之后重复3+4次event_search；并非current_source欠证。1次表格修补、2路调查、共10次Trace查询属于本次过程成本，不要求固定调用序列，不靠关键词硬门纠正模型。

`.root-causes.json`为schema_version=2、root_causes=[]、status=unavailable、reason_code=trace_root_cause_contract_not_active。用户是有限资源事实查询，无活跃根因合同；这不是旁路未生成或模型选择JSON损坏。解释质量挂03.2待验收与16.4/18.2，代码通过不倒签答案PASS。

## 2. 六类资源邻接用例

结果目录：`eval/results/hmc_native_resource_identity_20260923/trace_query_smartperf_resources-20260923-023455`。日志：`run-1.logs/codrax-20260923-023457-000-54062.log`。完整报告：`.codrax/output/20260923-023701.853-54062.md`。外层130s、内部summary127s。

通过范围：六类事件数量；BIO 2.5ms/4096B、FileSystem 3.5ms/1024B、PageFault 0.15ms/4096B；Ability AAFWK/AbilityStart/latency_ms12.5/foreground，XPower CPU/73/foreground，HiSystem POWER/THERMAL_REPORT/STAT/hot/MINOR。IO压力保背景，没有虚构链、根因或图。合法空root-causes旁路与上一例同原因。该例没有native_hook，不是新增metadata命中。

确定性系统缺口及传播：

- `internal/tracequery/query.go`的`accumulateRuntimeResource`把Path、Dev、Address用作同一Path回退。源缺页只有address=0x1234；实际Finalizer日志2083发布path=0x1234，最终报告58–60行按路径列出。
- `accumulateTracePluginEvent`以Comm回填Domain。源XPower没有domain，实际上下文2085和最终报告67行都得到虚构的domain=xpower。
- 最终模型输入2077–2086共10条优先观测，仅io_pressure正确携带查询范围；其余9条显示实际查询窗未知，而2071明示实际查询8.000–9.030秒。不是源trace缺时间。
- 最终报告80–82行系统附录要求从仓库声明核对运行时条目io_pressure，与runtime-only来源边界矛盾；另有内部枚举混入正文。不能把这些全归为模型波动。

## 3. 留账与下一优先级

稳定任务仍79=14已交付+65开放；03.2由待实施改待验收，不提前打勾。源字段/聚合身份/显示标签混用属于当前真实系统P1，挂现有02.4/16.4/18.4，优先于17.7安全SQLite和新统计能力。修复必须保真实地址/设备/路径及不同对象分组，不能只删回退导致信息丢失。范围交接与runtime-only源码附录分别留16.4/§166。全局字典COALESCE/重复覆盖/坏ID错误传播仍留17.7；原生登记和以前人工FAIL保持开放。
