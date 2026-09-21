# 4bcc 固定双例人工审计：0/2，不销旧账

- date: 2026-09-21T04:41:42Z
- sweep_start_ts: 20260920-214141
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_marker_navigation_envelope_20260920

干净二进制 `4bcc34e886e6`，固定快照 `.codrax/tmp/codrax-selected-20260920-214141`，两例各一次、并行2；runner正式exit0，机器0/2、人工0/2。同版无第三次追绿。后续普通账目未知关系修复不在该快照内。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_c_typo | FAIL | eval/results/hmc_marker_navigation_envelope_20260920/patch_c_typo-20260920-214142 | write_apply,write_patch_oracle,answer_contains | none | 124s | 27 | read=1,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | FAIL | 补丁和实际make test通过；required编译合同无精确执行见证，最终unverified，端到端未完成 |
| 1 | trace_query_business_marker_io_chain | FAIL | eval/results/hmc_marker_navigation_envelope_20260920/trace_query_business_marker_io_chain-20260920-214142 | log_regex,typed_trace_projection_count,trace_attachment,primary_answer | perf_triage+trace_query | 287s | 44 | read=0,repo_map=0,list=0,trace=15,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | FAIL | 因果问题误分类bounded事实，投影缺失；完整50ms套裁剪49ms账、请求量错说设备处理量及业务结束错序仍在 |

## 业务Trace：分类/系统显示/正文错误分别归属

已完整读 `run-1.primary.md`、最终 `.codrax/output/20260920-214627.375-11108.md`、根因旁路及相关原始调用/成文日志。原始fixture仍是OpenDocument 1.000..1.050=50ms；子业务LoadDocumentIndex 1.0045..1.0445=40ms；请求1.005..1.040=35ms，闭合issuer S等待1.009010..1.040010=31ms；目标S44ms、唤醒后R1ms，不可重复相加。

1. **投影确实缺失，不是评测标题误计数。** 日志829附近分类器将因果问题的原句“解释时间主要花在哪里、哪条依赖真正拖住了响应”声明为 `runtime_question_profile.scope=bounded_fact_set`，同时带wait/io/reason事实族和runtime_work_relation请求。现有报告权威以bounded声明优先，故未启完整报告/根因合同；不是本片marker导航删除了链，也不是工具没执行。应审分类schema/教学，不能按原句关键词开门或让所有有限事实查询强制变成因果报告。根因JSON必选旁路存在，schema2、空数组、`trace_root_cause_contract_not_active`，这是对本轮合同状态的如实披露，不是根因文件丢失。
2. **正确查询和不完整消费并存。** 日志1237–1238首次无过滤marker发现已取到TID100/TID200；1300模型随后显式PID100查子业务返回零行，这不是新游标例外可越权取消的显式过滤。1919–1920再无PID查询时已是多游标，本来也不会继承单目标。因此本轮没有命中6ebc的新skip分支，不以最终找到业务签该片生产命中。15次Trace调用、两路探索，正确完整marker及多种范围账都在。
3. **正文仍FAIL。** primary第3行完整50ms正确，但9–12行用1.001..1.050的4/1/44=49ms回答完整响应组成；14行把最长S现象直接称根本来源。20行泄漏内部 `completion_woke_issuer=false`；31行先正确排除后台47ms写的已证主因资格，又称其“并发干扰项”并建议错峰避免竞争，尚缺队列因果证据（55行限定不能抵消先前定谳）。35/39行把35ms请求驻留称IO设备处理时间，未排除排队。46行保子业务名和起点，但漏结束/40ms及8/1/31局部账；48行省略唤醒10微秒，49行误把1.045唤醒主线程写成业务完成，真实结束1.050。35/31/44/1不相加的方向正确，不足以抵消上述错误。
4. **独立系统量尺疑点。** 第53行工作关系说明是结构字段渲染，不是普通散文：模型选择`trace_query:trace-query-result-200c14c7.json#trace_business_span:1`，该次是1.001..1.050查询范围内49ms，但生成行仅称OpenDocument“实测49ms”，未显示这是查询内片段。应核原生测量/裁剪字段贯通，不猜成50、不让显示行替模型改结论。已交独立审计，未宣称已修。
5. **成文过程非JSON失败。** 一次完整成功，随后系统展示advisory引导一次metadata patch成功；零成文硬拒绝。最终仅两条唤醒CPU角色事实对照，确实没有Trace因果投影。无空答案、无活跃流请求年龄截断。本轮因未渲染方向板，也不能验证4bcc新段头生产表现。

## C写模式：正确改动和缺失证明分账

已读原计划、实际应用树diff、post-apply report、final JSON和`run-1.write-apply.json`。只改 `main.c:19 retrun→return`，实际测试`make test`退出0，构建输出`cc -Wall -O0 -o main main.c`；原输入仓源码未改，结果保留在隔离工作树/应用树。构建产生的未跟踪main已披露，不混入源码交付。

计划required合同`compiles_ok`却声明subject=`gcc main.c`、kind=`command_result`、operator=`not_raises`，expected为中文编译成功。报告只有make聚合通过，没有精确逐合同assertion回执。累计终验再次执行仍不能补出该证明，最终`unverified/verification_proof_incomplete`，没有误签all_verified。因此补丁、执行与隔离边界PASS；端到端验证FAIL。

独立代码审计：`run_tests_parsers.go`有意只产Make聚合行；PTO join要求exact suite/assertion，当前command_result凭证闭集是probe/project_test。C规划教学又要求不造probe、将编译条件放acceptance_tests。属于既有HMC18 B2–B6下具体化的native-command能力缺口，不是4bcc回归；不能把make等同gcc、从stdout猜命令身份，或把自然语言expected扫描成证明。最窄后续需要typed可执行能力核对，以及受控精确command/argv/cwd、typed结果谓词、源字节/审批/快照与新执行回执的生产者；仅加提示/PTO声明不能创造缺失执行能力。

旧人工失败、B2–B6、业务焦点生产命中与父能力项均保持开放。该双例不回写任何旧审计结果。
