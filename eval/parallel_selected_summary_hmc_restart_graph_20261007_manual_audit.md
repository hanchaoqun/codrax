# HMC §203：多查询图与只读补证恢复人工审计

- date: 2026-10-08T01:12:58Z
- sweep_start_ts: 20261007-181257
- total cases: 1
- parallel: 1
- timeout: 1200s per case
- results_root: eval/results/hmc_restart_graph_20261007

上面的runner元数据只统计CLI一例。实际批次在冻结`91aaa9be3`上恰好两例并行×1：自然Trace CLI 18:12:58–18:15:28，真实native planner＋新进程生产恢复18:13:03–18:13:14（America/Los_Angeles）。机器退出2/2，完整人工0/2，没有第三例。子能力结果与整个答案/流程验收分开。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_sleep_dependencies | PASS | eval/results/hmc_restart_graph_20261007/trace_query_sleep_dependencies-20261007-181258 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 150s | 42 | read=0,repo_map=0,list=0,trace=7,source_lens=0 | midloop=2,inv=1/0,fin_reject=1,unavail=0,prune=0 | FAIL | 四段数值正确，但用户所求图被删；边界/优先级排除误述 |
| 2 | native_registration_restart | PASS | eval/results/hmc_restart_graph_20261007/native_registration_restart | exact_native_receipt,fresh_process,readonly | real planner + production scheduler/store + scripted decisions | 11.21s | — | read=2,emit=1,new_execution=1 | rejects=0 | 整体FAIL；子能力PASS | 登记/新进程/新执行正确；proof批次仍unverified，非完整CLI |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 1. Trace：最终缺图，不以语法通过冒充交付

自然问题保持不变，没有向用户加配对/凭证/JSON规则。app 40ms、dep2四段10/6/5/0.8ms正确，第三段不再借后面的dep5；但5.027状态边界被填进“唤醒时间”，实际没有wakeup记录；正文无充分执行/供给证据就排除优先级反转，仍FAIL。

7次trace_query、0次源码读取、峰值42%、1次emit拒绝/2次patch。第一次wakeup_chain缺目标，实际`trace-query-result-eb25736b.json`为target={}、nodes=null、target thread not found；后续只有interaction_stats与thread_timeline。查询窗全部4.999..5.042，宽于用户5.000..5.041；系统附注披露了差异，不能替代指定范围的独立账户。

实际日志2447首稿有5参与者/4箭头但无运行时锚；2449–2495拒绝守住关系真值，提示偏源码call；2511–2520模型误解运行时不能画，2528删diagram，2533接受。此次组合没有图凭证供给，本批去重算法未命中live。需要修typed目标/关系补齐与诚实缺证教学，不能放开无锚边或按名称猜身份。

最终完整答案经仓库浏览器审计入口核对，`diagram_count=0`，无被接受图可截图，不是渲染PASS。独立**公开回归**的发射图与日志fence逐字diff一致，终端/浏览器均通过：1 sequence、SVG24382字节、5生命线/4箭头；截图`/tmp/codrax-hmc203-public-graph.png`。此图只证明多query→actual finalizer→emit/patch子能力，不代替live答案。

completion_lane_fired=0、degraded_read_answer_check_skips=0；没有活跃流等待降级，日志仍显示10分钟首响应/5分钟静默/10分钟非流式。

## 2. 只读登记：真实模型、新进程及收尾缺陷

模型MiniMax-M2.7-highspeed；自然目标“实现已修好，请运行保留的现有测试，确认返回值正确。不要再修改源码、测试、配置或依赖。”顺序read_file(value.py)→read_file(test_value.py完整4行)→emit_change_plan(changes=[], project_test_observations一条)。3回合、无拒绝，无改文件/合同或同响应read+emit。

- source plan `plan-1791421983192551000-78912`；registration `plan-1791421993462153000-78912`。
- 父PID78912→子PID79796，子进程不注入IR/私有授权/报告，生产load/hydrate与默认store恢复。
- 旧invocation `native:13440:18dc68cd6f255608:1`→新`native:137b4:18dc68cfe5851188:1`。
- 精确断言`test_value.ValueTest::test_increment`，原合同increment-result；文件SHA `ec7d5c37376fa9c583ba402a196e94c9fa3d1c12c734195ccd18e8e7386a7864`。
- HEAD `c435cdf736de69fdee602132d55eb1b1ca8a8cef`未变，新命令exit0，源交付/登记/测试/命令/断言摘要由同一invocation绑定，授权撤销。

原receipt.passed=true只说明旧入口断言通过，workflow.status=complete不等于verified。人工发现新proof批次为verification_proof_incomplete。公开新进程回归加严后复现：RunTests账本strong/verified，调度器附加计划上下文时却把只读TargetPaths当修改源码目标，产生source_localization_weak；内存判定与较早保存plan也不一致。`db54efcd2`修复这一义务归属，合法native/probe-only持久化形态共同适用，非法形态/状态与真正修改仍要求定位。

修后确定性planned/verifying两崩溃点通过，proof批次verified；fixture原source批次没有终态凭证，整轮诚实保留unverified/missing_terminal_verify_verdict。新入口分列proof_batch_verified与workflow_completion。**未追加真实模型重跑，也未改本目录原workflow。** 完整CLI、真实controller决策、来源终态累计消费、多来源/其他框架仍开放。

上下文：system第52行和user第130行重复长行为教学；system空changes禁令只列旧两种例外，与user授权登记不一致。本例模型做对了，但仍是16.4通用教学债，不归咎于模型波动；后续按typed形态统一教学来源，不能继续把约束放进用户问题。

## 3. 原始证据指纹

以下相对`eval/results/hmc_restart_graph_20261007/`；旧§200–202原始失败不覆盖。

| 文件 | SHA-256 |
| --- | --- |
| trace_query_sleep_dependencies-20261007-181258/run-1.answer-transcript.md | `317922981dace4881b90316e1fba127c490a63bc3445c3179eaccb4e0dde92d4` |
| trace_query_sleep_dependencies-20261007-181258/run-1.logs.all.log | `095f6809a14d76ae68a9c0996dcc5b98d59d8c265d6b3a55790a8cde60350f61` |
| native_registration_restart/final-plan.json | `d268da5b8fe9f8f374745809d1ca28c533ddc04e6ce05de5491a2671d164e835` |
| native_registration_restart/final-report.json | `99f452d3cae244ffb590324172a87199ac0e3fc6c0bd139d35cf2d367da58df3` |
| native_registration_restart/workflow.json | `d930c01631bf2f6a66c7d249fa91fcf6c3592e8350314b4da68747dc6a2d76c2` |
| native_registration_restart/logs/codrax-20261007-181303-000-78912.log | `f1f5f7724637b0c7bac018b67144ffbcf9947a361f39c653f138c77dfb8462d1` |
| /tmp/codrax-hmc203-public-graph.png（公开回归，非live） | `66d1c10a01c487713f324436e55b6ee77069d5860f04e5757cf0e0605e5f255f` |

验证及发布见统一账本§203；本审计不关闭父任务，不用确定性回归倒签旧live。
