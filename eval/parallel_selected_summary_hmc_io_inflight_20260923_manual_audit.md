# IO 在途统计与写模式：固定双例人工审计

- date: 2026-09-23T13:31:41Z
- sweep_start_ts: 20260923-063141
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

18909正式exit0，恰好2并行×1，无第三例。机器1/2、完整人工1/2；runner耗时包括启动收尾，因此表中726/144秒与单例业务计时724/141秒不同。固定二进制revision=a4dd149470c1，不包含后续B1或并发崩溃修复。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | empty_python_module_apply | PASS | eval/results/empty_python_module_apply-20260923-063141 | write_apply,write_patch_oracle,answer_contains | none | 144s | 28 | read=6,repo_map=1,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | PASS | 本次功能/修改范围/真实测试/最终说明正确；不销历史原生断言登记及source-free债 |
| 1 | trace_query_io_inflight | FAIL | eval/results/trace_query_io_inflight-20260923-063141 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 726s | 48 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 确定性四组指标正确，但并行上下文生成崩溃，无最终报告/旁路；非超时降级 |

## IO：测量正确，进程崩溃，完整交付失败

- `run-1.out:84`为`fatal error: concurrent map writes`；89–104定位`NormalizeToolRefinementHint`→`AttachToolHandoffCarrier`/TurnA→`toolDocumentationCarriers`→`BuildAgentContext`→并行explore fork。实际退出2、finalizer_dispatches=0，不是1200秒评测超时、600秒首响应或300秒静默降级。没有final/answer-surface/root-causes，missing_primary等机器项不能另算成已成文的事实错误。上下文峰值95,965/200,000（48%）。
- 日志`run-1.logs/codrax-20260923-063144-000-62516.log:1370–1389`真实window_stats成功；blob `.codrax/blob/20260923-063144-000-62516/trace-query-result-889b3c3b.json:243–418`四组与fixture expected一致：RQ8,0R峰值2/均值1.4/忙10ms/面积14请求·ms/发起6；RQ8,0W为1/.6/6/6/2；BIO8,0R为1/1/10/10/1；RQ8,1R为1/.4/4/4/1。block诊断1未完成、1歧义组、2受抑制配对，非完整采集保证，不授链上根因。日志工具结果仅截显2000字，不等于工具14941字输出缺失。
- 预阶段实际结构提交（231）错误FIFO配9000、给10000虚构完成，并算错读写；但Explorer INIT1113–1129将这些降为candidate locators、withheld subject/summary/evidence，1133明示原生查询优先。不能把预阶段错误自动归为已污染事实上下文。
- 后续实际完成交接1600、1607已接受：aggregate四组主要指标正确，但reason把sector4000窗后完成说未配对/不计面积，把10000未完成错归9000，把1000完整6ms误说全部窗内（实际交集4ms），W后段4ms写成2ms，又混用4pairs/6issues为覆盖率。合格完整配对数不是窗口内完成次数。上述是结构化模型交接错误，非思考文本，亦非最终用户答案；崩溃前后不可假设Finalizer已纠正。原生证据已提供，暂记18.4模型解释及上下文量化债，不添加原文关键词硬门或用一次样本宣称已证波动。
- 广影响确定问题挂01.3/16.4/18.4：共享载体normalization原位写map，须先修输入隔离/并发安全并补公开回归。异常退出缺必选旁路仍归18.4，不以未进入成文宣称已满足，也不生成伪根因。

## 写模式：本次交付通过，旧证明债保留

- `run-1.applied-tree/totals.py:1–2`为`def total(values): return sum(values, 0)`，满足整数精度、空输入、单次迭代。交付提交`86354578d3884b7fd0d36efa734c30d741800f7c`仅totals.py新增2行；seed/main仍原3d5ccc93…，原测试/__init__/README字节不变。
- apply日志`codrax-20260923-063330-000-68926.log:658–665`在plain probe后继续实际原生`python3 -m unittest tests/test_totals.py -v`，四断言通过。report四断言同执行ID `native:10d3e:18d7f68618500b38:1`、命令exit0，probe另1条且源码SHA匹配交付。`run-1.out:136–152`最终“5条验证结果”及未逐项独立证明的限定准确。
- 四条PTO把suite写为`unittest@tests::TotalTest`，实际为`tests.test_totals.TotalTest`；五行为合同均planning_only，未成为required凭证，不证明补登记成功。未走source-free，不能销§165/168旧FAIL。首次合法空文件patch已接收但path-only定位仍重派、模型随后有干运行/动作类型错误，作为旧效率和空源定位债保留，不改变本次功能PASS。

两位独立审查均未改写原始结果、未重跑。后续确定性修复收据写统一账本，不回填本轮FAIL为PASS。
