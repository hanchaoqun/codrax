# Selected Eval Manual Audit Scaffold

- date: 2026-09-20T06:14:03Z
- sweep_start_ts: 20260919-231403
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_parallel_handoff_replay_20260920

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_business_marker_io_chain | FAIL | eval/results/hmc_parallel_handoff_replay_20260920/trace_query_business_marker_io_chain-20260919-231403 | log_regex,typed_trace_projection_count,trace_attachment,primary_answer | perf_triage+trace_query | 332s | 38 | read=0,repo_map=0,list=0,trace=13,source_lens=0 | midloop=1,inv=2/0,fin_reject=1,unavail=0,prune=0 | FAIL | 分析将因果请求窄化为有限事实；51ms状态账户套50ms业务窗，44ms睡眠混成已证IO等待；图被source-call门拒绝后模型删除 |
| 1 | trace_query_io_request_latency_distribution | PASS | eval/results/hmc_parallel_handoff_replay_20260920/trace_query_io_request_latency_distribution-20260919-231403 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 367s | 33 | read=0,repo_map=0,list=0,trace=7,source_lens=0 | midloop=3,inv=5/1,fin_reject=0,unavail=1,prune=0 | FAIL | 三组八项数值正确；正文混淆读组与全RQ总体、缺完成与歧义，并将请求驻留/压力背景误写为设备物理延迟/已证竞争 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 固定快照与分布例人工审计

本批构建固定`04a6cd0a0a28`，2并行×1于2026-09-20T06:14:03Z启动。自动summary原样保留；不改fixture/oracle，不追加第三例追绿。此次不包含正在施工的源码解释维度适用域修复，不能拿本轮效果验收后续代码。

分布最终报告：`.codrax/output/20260919-232007.599-26749.md`及同名HTML/旁路；日志为结果目录`run-1.logs.all.log`。原始`window_stats`仍是`trace-query-result-8ee5e57a.json`，位于blob session `20260919-231405-000-26749`。最终指令日志3665–3666等明确含各组分位数/身份/窗口。

- 数值改善是真实的：报告29–30、38行三组八项全部符合独立expected.json，读11、写3、BIO读3；写P99=24.8、BIO P99=5.96，表头可读，不再是“列1…8”。
- 交接修复实际触发：日志3000明确从非胜出分支保留4份已完成typed工具结果，3001记录parallelism=2。相关事实到达最终指令；不是仅依赖单测或本轮没有触发代码路径。仍不将一次随机回放当匹配基线A/B效果量证明。
- 人工仍FAIL：报告25行把RQ读组14次提交/11成功配对说成全部RQ；完整RQ实际还有3写，即17提交/14成功配对。44行把1缺完成＋2配对歧义统称3条无complete，69行再混写；原payload明确分开。
- 报告17行把实际`block_bio_queue`写成`block_bio_issue`；62行把请求issue→complete驻留叫存储设备物理延迟；65行由压力背景值推出存在资源竞争，证据仅支持活动/请求耗时而不是已证竞争。这些不靠系统改写结论修正。
- 内部字段直接出现：`layer=block`、`event=block_rq`、`unpaired_start`、`pairing_suppressed`、`activity_index`等。数值和边界准确性优先，统一业务语言教学仍留账，不对模型正文作关键词硬拒或替换。
- `.root-causes.json`确实生成：schema_version=2，空数组，`trace_root_cause_contract_not_active`。本例只问分布和证据边界，旁路不应虚构链上根因；无图按用户要求，不代签Mermaid验证。

## 新回放中的关系成员集与工具提示

日志887的被接受分析同时带`is_relational_lookup=true`、category enumeration、required member_set维度/completeness，以及bounded runtime事实、`runtime_work_relation_requested=false`。1441关系成员集缺失降级忠实消费前者；“不需要图”或后者false不等于没有任何成员集义务，不能据此直接撤门。1452通用修复提示却要求grounded evidence，1477模型尝试当轮不提供的`emit_evidence`并被拒；1500补runtime成员集后1508成功完成，无需源码。已确认的是提示选错证据通道，是否调整“分组统计被分类成relation”的规则仍需独立反例，不能对所有runtime关系或成员集一刀切豁免。该问题与旧回放的源码operation硬门冲突分别跟踪。

## 业务例：问题分类、业务窗口与图分别审计

最终报告`.codrax/output/20260919-231932.957-26750.md`及同名HTML/旁路，blob session `20260919-231405-000-26750`。机器FAIL，人工也FAIL，但不是所有机器失败理由都代表内容缺失：正文明确写了35/31/1、后台47和“不重复相加/不能直接当根因”；背景匹配规则仍有词形假阴性，原oracle不改。真正失败如下。

- 接受的`emit_analysis`参数`0011c574.json`把本轮设成`bounded_fact_set`，诊断/关系/work-relation/frame-causality均false，只引用问题末尾的计量要求，遗漏前面的“哪条依赖真正拖住响应”因果任务。日志2194自动补齐因`families_present`跳过；不是上轮`no_typed_target`，不能沿用旧因果解释。
- 业务span确实查询并被正确呈现为1.000..1.050=50ms；线程与window_stats调用却用1.000..1.051（日志1326–1328、1933–1936），报告33行将51ms的6+1+44状态账户套成50ms业务分解。报告21行又只算恢复后的4msCPU而漏掉开头1ms；23行把LoadDocumentIndex前的运行计入其40ms内部。
- 原请求驻留35ms、提交者S态已证等待31ms、1ms调度延迟均保住，业务名称未丢；但报告39行将app-main的44ms普通睡眠写成`completion_closed`，47行以目标等待症状定为瓶颈，混淆两层等待与具体阻塞机制。报告41行以缺少备份唤醒凭证断言“没有发出唤醒信号”，只能说未观察/未证。
- Trace因果投影确实为0，旁路schema2空数组、`trace_root_cause_contract_not_active`；系统按被接受的有限事实声明执行，不能用扫用户原文补开根因合同。应补全通用多维意图教学/分类验收，同时保留真正有限事实问题的窄通道。
- 模型另画了时序Mermaid；日志3129记录格式安全修复，3133及3180附近却以非runtime source-call关系要求12条箭头的源码call anchors，3223模型通过patch移除整图，3229接受。模型图含未经证明的main→worker派发边，不能一律保图/放宽全部边；但runtime事实图与源码关系图的类型/适用域需单独核查，不能把本次图丢失说成纯语法失败。
- 内部词`caliber`、`residence`、`completion_closed`、`ohos_rt`等仍出现在正文。系统事实对照明确给出51ms测量窗，未替模型改写成50ms；这是可核对的披露，不能拿它倒签正文正确。

结论：本批机器1/2、人工0/2通过。交接修复有实际执行与数据到达见证，但答案质量、业务焦点/窗口、意图分类和关系修复提示尚未闭环；后续按统一账本§24推进，不增加本批第三条模型回放。
