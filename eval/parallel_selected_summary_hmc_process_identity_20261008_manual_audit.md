# 进程量测与成员身份：完整人工审计（§212）

- date: 2026-10-09T02:39:26Z
- sweep_start_ts: 20261008-193923
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_process_identity_20261008

固定两例并行、各一次，使用 `2c76ffd425c7-dirty` 冻结构建（当时仅文档未提交）。机器2/2、完整人工0/2；自动检查只证明指定工具路径与运行完成，不证明答案正确。两个自然问句不带守护答案，独立oracle未作为模型输入。两例均未读取仓库源码、未修改业务文件；没有追加第三例求绿。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_process_measurements | PASS | eval/results/hmc_process_identity_20261008/trace_process_measurements-20261008-193926 | log_regex,trace_attachment | perf_triage+trace_query | 230s | 46 | read=2,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | FAIL | 默认SQLite转换、精确原值/缺值/区间/进程与成文三表已命中；终稿弃用来源表并猜存量/特殊标记语义，用单点零排除原因，不能销整答。 |
| 2 | trace_rendering_candidates | PASS | eval/results/hmc_process_identity_20261008/trace_rendering_candidates-20261008-193926 | log_regex,trace_attachment | perf_triage+trace_query | 293s | 43 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | FAIL | 本次analyzer保对用户窗，但查询/终稿扩到1.051并纳入右界；跨owner流水线、框架和执行语义仍无证。身份修复路径命中，确定性回归证明限定身份不再等价；无live归一化后逐成员快照。 |

## 进程量测：数据接入通过，业务解释未通过

目录 `results/hmc_process_identity_20261008/trace_process_measurements-20261008-193926/`；下列日志行号指 `run-1.logs.all.log`。

- 原始附件为20KiB合成SQLite，不是预先手工转换文本。运行时默认完成只读快照、导出、bundle准备；`process_measurements`四次调用，含用户 `[1,2)` 与补充 `[0,2)`，后者在交接中明确不能替代用户窗口。`read_file`两次只读完整query blob，不是源码调查。
- 用户窗结果保11条入选行、1条起点未知另计；2秒的9900行未混入。Resident两owner、NULL、TEXT unavailable、真实0、`9007199254740993`原字串、窗前起点0.9及持续/裁剪区间均保真。NULL时长的Allocation只为时间点，未知起点123不被定位到用户窗口。
- 实际finalizer日志2819–2839包含全部4组序列的summary和11行可选members/timeline。summary预览覆盖两应用、Page faults和Allocation；精确大整数未截断。明确携带“单位及存量/累计/增量未有协议”、NULL不等于0、空洞不填满、记录数不证明采集完整。完整三表来源、窗口选择器已提供；模型未在emit中选用，不可称作新接口未投递。
- `run-1.primary.md`仍将未知聚合语义解释为驻留内存时点快照及净增长80%/10%，以Page faults单点0断言“并非缺页触发”，并猜Allocation大整数是特殊标记/分配失败编码。大整数实际为2^53+1，答案的近似2^53-1及MAX_SAFE_INTEGER解释没有来源依据；不能因此改原始数值或加数值特例。
- Beta明确2000→1900→2100→2200，不是单向递增；“变化”列又混用相邻差值和基线差值。Alpha区间洞与NULL/非数值记录是不同覆盖状态，终稿未完整区分。末尾单位/因果限定不能抵消正文越权。
- 预检与analyzer只看到一条未知起点123的显著预览，产生“只有一个应用/一条记录”的先验；探索实际已补齐。调查closure日志1436、2078把推断写入resolved reason及自由scalar，随后在finalizer closure/narrative重复出现（2850以后），与原生表的语义上限竞争。这属于上下文组织及派生证据分层的通用债，不应简单归为随机波动。

## 渲染线索：身份修复路径命中，范围与无证关系仍失败

目录 `results/hmc_process_identity_20261008/trace_rendering_candidates-20261008-193926/`。

- 五份analyzer原始提交均保 `time_start=1,time_end=1.05` 与原句引用。这与§211不同：本次不能再记为analyzer数值漂移。后续六查询使用1.051结束（事件查找还标记其lookup-only边界容差），终稿沿用1.000–1.051并列入右界PID900/TID901。
- 两次completion输入各含9个有owner限定的框架成员，日志1650–1657、2213证实归一化修复路径实际执行成功；确定性公开回归证明这些身份不再等价。live没有输出归一化后逐成员快照，不能夸大为最终9→9逐行见证。另一“6个排查点”及最终“6类”是不同模型表述，不能用数字巧合重开已修身份缺陷；同时9是扩展查询的组合口径，不等于用户窗口总体或9个进程。
- 完整候选专栏因实际查询窗不匹配而未投递，这是正确的范围保护，不应放宽。日志2927明确22条越窗观测从主视图排除，2971仅剩带补充范围说明的截短generic记录；3035仍明确用户范围1..1.05。整轮从未生成对应用户窗的候选查询，需补“主范围证据仍缺失”的typed补查衔接，不能把广查总数改名为用户窗总数。
- 终稿把独立进程标记串为主线程→UI→光栅化→合成器主流水线，把KMP/Compose归ArkUI，并根据GPURasterizer名称断言GPU执行。Unity与普通Draw/FlushBuffer的关系也未由同owner/帧连接证明。结尾“缺源码”限定不能替代Trace内所需的精确关系证明。
- 最终一轮patch只是补facet归属，还把进程线程清单重复插到框架节；没有纠正范围或业务语义。没有Mermaid图，不代签旧图格式/时序/关系失败。显示内部view与resolved_files也违背业务化表达目标，暂不围绕该句另开修辞施工线。

## 挂回稳定任务与下一验收边界

- 01.3/16.4/18.5：统一请求范围、实际查询范围、展示子集和查询总数的来源坐标；补充查询仍允许，不能通过禁止扩窗破坏自动补证。§211的引用与端点未绑定缺陷也独立保留；本次正确IR表明只修analyzer不能包办下游范围失真。
- 16.4/17.7：优先改善原始量测与模型派生值的证据分层、精确来源表选择和预览代表性；未知单位/聚合语义不能被自由closure自动升级，旧叙述不应重复压过当前原生表。没有充分证据证明这是偶发波动，不按“模型波动”销账；不扫问答原文设关键词门。
- 17.7/01.3：独立审计另确认 `internal/context/attached_trace_semantics.go` 仍用载体排序时间显示未知起点为0。日志213外层 `timestamp_ns=0` 与内层 `source.start_ns` 不可用矛盾；公开event_search本批已修，但预览消费者未复用。该确定性接缝在live之后补修，保留本次原始FAIL；后续确定性回归不倒签模型效果。
- 12.1实现交付保持，12.2帧实例/跨线程精确连接器与12.3明确协议帧率观测继续开放。五稳定验收父项、只读登记来源终态及旧失败保留；本批为两组子能力/修复，不新增完整父能力销账。
