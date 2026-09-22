# b3c0bf 固定双例完整人工审计（2026-09-22）

- date: 2026-09-22T10:59:28Z
- sweep_start_ts: 20260922-035927
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_native_execution_20260922

生产冻结为 `b3c0bf870378`（前一笔原生执行代码 `111b22d23`）。构建9277正式exit0，时间 `2026-09-22T10:58:36Z`；版本后缀 `-dirty` 来自未提交的文档收据，Go/构建输入干净且runner已检查；不冒称当时整个工作区干净。runner98029正式exit0，严格2并行×各1次，没有第三次追跑。

机器2/2 PASS，完整人工按本次用户任务正确性判 **2/2 PASS**。这不是所有修复分支生产验收、全部父项完成或历史失败答案重签；特别是Python本轮没有发射新执行要求。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | nested_python_increment | PASS | eval/results/hmc_native_execution_20260922/nested_python_increment-20260922-035928 | write_apply,write_patch_oracle,answer_contains | none | 127s | 28 | read=6,repo_map=1,list=1,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | PASS | 原测试3方法实际通过；新执行意图/收据未命中 |
| 1 | trace_query_frame_semantic_span_optimization | PASS | eval/results/hmc_native_execution_20260922/trace_query_frame_semantic_span_optimization-20260922-035928 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 151s | 42 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | PASS | 保数值、链上语义与未证帧因果；worker图例命中 |

## Trace：原始事实、完整图与答案分开核对

完整阅读292行answer-transcript、11行primary、原始附件、实际最终上下文及emit/patch；独立审计同核。导出 `.codrax/output/20260922-040157.134-2672.{md,html,root-causes.json}`。

- 目标5.000–5.007秒窗完整，S5.000/runnable0.800/running1.200ms；CPU2→CPU1唤醒在5.005，实际切入运行在5.0058。没有把跨核边当成同核竞争。
- VerifyClass原始5.0004–5.0054，即5.000ms墙钟；链上计入5.0004–5.0050为4.600ms，余0.400ms在唤醒后。worker边前running4.000ms、全窗running4.600ms分开；“部分重叠”指原始类校验片段与目标就绪等待，不能误读为两个有效归因区间相交。
- 正文不再声称类校验完成触发5.005唤醒，也不再说两修复方向独立。明确缺帧/截止期证据，只是该窗口的优先候选；保4.600/0.800ms排名、缺频不可折算、邻近sleep1.400ms和背景累计0.800ms的非根因身份。
- 第70行新worker墙钟图例真实命中，专用text因果树/方向图均保留。self/mixed/mention及§126源码拒绝分支未命中，不能借签。
- 模型首次emit省略根因选择，接受后一次可选patch主动选择两项；`logs.all:2562`保原3正文块。sidecar为schema2、available、两项0.0046/0.0008秒、均frame_unproven且requested/query scope=5..5.007；不是系统强填或导出失败。不得倒签旧§124模型主动空选择为故障。
- 完整上下文可支撑这次正确回答。不把单次改善说成旧错误已证明仅为随机波动。非阻断展示债仍在：同类线程措辞欠精确，正文runnable_wait/wakee/cross_cpu、平台双语说明和占用表“现规则可消/可消除榜”等旧词未全清；不为本例新增禁词或扫描模型原文。

## Python：交付与执行正确，新强收据分支未命中

完整阅读151行输出、固定IR/计划、最终报告、规划与apply日志及交付差异。案例自身125秒、外层127秒分别保留。

- seed `da3061ec76daba7a5dbb64dd06271639a1fb43e4`，交付commit `c27961f556ef4d13a30d62674d0bd2be12d358a4`；唯一改动为widget.py第2行`return value + 1`，原checkout仍在seed，不自动合主干。原测试/setup.py/tests/__init__.py逐字不变；test_widget.py SHA256均`504b2535413cafa883be2802d92a698036735de8883384530caec4fa8375f322`。
- apply日志`040059-000-12025.log:682–683`及report:61–75记录当前交付目录内`packages/widget`的`python3 -m unittest "tests/test_widget.py" -v`真实exit0。三方法覆盖-5/-1/-(2**64)、0、1/7/2**64。根目录discovery零测试另记，未拿它代替；本次无post-apply probe替代这3个测试。因此用户要求的修复与已有测试验证PASS。
- 新教学已真实到分析消息`035939-000-2691.log:560/568/586/599`；但最终固定IR（plan:628–634）只有preserve_regression_test，没有run_existing_test，报告亦无existing_test_executions。实际由旧PTO路径选择精确测试。新intent/强收据和分析补读分支均未live命中，不把功能通过归功于它们。分析省略该要求的可靠性问题仍留观察，不能凭一次运行归为模型波动或用原文硬门代填。
- 三条PTO（plan:47–83）的声明suite/ID不匹配报告完整身份；10个行为合同都是planning-only、required=0，不授独立合同证明。B2–B6、原生声明-观测配对继续开放。
- 规划期所谓先跑测试的临时probe实际只打印三个值且Ran 0 tests（2691日志1821–1837），系统将其标为observation-only；它不是原3断言的基线失败。本次最终验证没有依赖它假绿。
- 另留统计命名债：final.json:268的probe_count=1来自`verification_proof_profile.go:171`的len(VerificationConfidence)，唯一行为source_compile；不能解释为执行过一个post-apply probe。当前所查消费处为汇总/展示字段，不因本例擅改验证权限。

## 记账与后续

仍79个唯一父ID=13已交付+66开放。保留§124等历史人审FAIL；本次通过只表明这一冻结版本的两项用户任务已正确交付。§127新机制由公开真实Run正反/consumer/race证明，真实模型未命中另列，不与生产验收混同。下一确定性高ROI为§128旧pytest产物冒借；之后恢复caller双轴、业务局部补齐/容量、声明-观测pair及B2–B6。下一异构双例建议jank_field_inventory＋原版dateutil_relativedelta_float，各一次并行2；本批不再追加追跑。
