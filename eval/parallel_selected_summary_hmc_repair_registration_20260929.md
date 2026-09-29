# HMC §191：固定双例原始判定

- date: 2026-09-29
- parallel: 2
- runs_per_case: 1
- trace_session: 30338，正式exit0
- registration_session: 2362，正式exit0
- full_suite_session: 93230，正式exit0，87测试包PASS/13无测试/零FAIL（独立于两例模型评测）
- production_binary_sha256: f77829b0dfa180888f9c9425931833c3d89d4cb79f758ab210b9bbfbe9a7d1a2

两例各运行一次，没有第三例追绿。构建版本d2ebc4c1c776-dirty，包含随后按同字节提交的303bd55e6生产修复；登记入口6d73a22d1在同一冻结输入集内。此组合包含一例完整CLI和一例明确标注的真实planner阶段验收，不伪称两例完整CLI。

| case | 范围 | 机器原判 | 秒 | 结果目录 |
| --- | --- | --- | ---: | --- |
| trace_existing_sqlite_dictionary | 完整只读CLI，原自然问句与原oracle不变 | FAIL：5ms字面正则未命中 | 157 | eval/results/trace_existing_sqlite_dictionary-20260929-013126 |
| native_registration_restored | 真实planner＋公开controller/工具＋文件恢复＋新执行 | PASS | 16.07（测试体） | eval/results/native_registration_restored-20260929-hmc191 |

机器合计1/2。启动完整人工FAIL；登记仅该阶段人工PASS，最终用户答案/整个工作流完成不在覆盖范围。详见[人工审计](parallel_selected_summary_hmc_repair_registration_20260929_manual_audit.md)。原启动FAIL不因识别到单位在表头而回写。
