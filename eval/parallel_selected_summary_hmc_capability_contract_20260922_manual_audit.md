# Selected Eval Manual Audit

- date: 2026-09-23T04:11:21Z
- sweep_start_ts: 20260922-211120
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_capability_contract_20260922

本批固定 f230ac8b3a4a 构建，2并行、每例1次；机器分数不替代完整日志、最终答案和交付树审计。主审与独立审计完成后，完整人工1/2通过；旧失败未回写。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | nested_python_increment | PASS | eval/results/hmc_capability_contract_20260922/nested_python_increment-20260922-211121 | write_apply,write_patch_oracle,answer_contains | none | 113s | 28 | read=6,repo_map=1,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | PASS（实现/既有测试执行） | 仅实现一行变更；3个原生方法、7个值检查；测试/配置字节未变。当前plan/commit/patch/test哈希收据真实。错误PTO未授合同证明，原补证债不销；新path/scope修复未命中。 |
| 1 | trace_capability_discovery | FAIL | eval/results/hmc_capability_contract_20260922/trace_capability_discovery-20260922-211121 | log_regex,primary_answer | none | 115s | 28 | read=0,repo_map=1,list=0,trace=0,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | FAIL | 实际目录成功且零runtime权威；终答漏可调用view、41误说34、调度/缺测解释错误，并保留伪文件引用。目录跨阶段保真/文档来源识别列新系统接缝，不用提高证据权重修复。 |

## 完整答案与过程结论

能力例先被Analyzer错当目标仓库实现问题，空stub被误述为程序无Trace实现；Explorer仍成功调用`trace_capabilities({"detail":true})`，21view/41family和正确前提、单位、缺失策略完整到达，未查询、转换或制造运行时观察。其后模型把工具文档写成`trace_capabilities.metrics:1`等虚构源位置，6条发射仅4条被降为system_inference接受。终答未提供window_stats/scheduler_latency_stats/perf_stats，写34族，误把唤醒事件列为全部调度等待的必要条件（遗漏抢占产生runnable）；还称缺完成可参与部分延迟计算、缺测分位数可能全0，违背目录完整配对/缺失不等于0的合同。采样不全来自硬件性能计数器，权重不是统一时间；归档条件被过度简化，二进制自动准备回答不够直接。正文有支持/实测区分、无编造具体测量或根因，但这些正确部分不足以整体PASS。

最后一轮撤掉未接地evidence_ids后，`trace_capabilities:1`四条自由引用仍被接受，系统附录又将伪路径列成源码定位状态并泄漏内部枚举。目录正确进入Explorer不等于完整目录进入成文：目前零权重输出不会成为RepoFact，成文主要消费模型改写的完成理由/推断，不能据此归纯模型波动；后续需独立静态工具文档来源与跨阶段保真，不将其冒充文件或runtime evidence，不扫描散文硬门。实际工具结果45902字节，debug只打印头2000字节不代表Explorer收到截断。没有图/实际Trace附件/查询/因果合同，故不声称本例验证图关系、Trace因果投影或根因旁路生成。

写例交付commit `ceaba9251ab61bc8e694b02da6b37201f0bbd56e`，唯一差异`packages/widget/widget.py`第二行`return value`→`return value + 1`。3个unittest方法覆盖负数3、零1、正数3；计数3是方法级原生断言，不是7/8个方法。根目录另外一次discovery为zero_tests，未冒充成功。最终报告说明3条验证通过和自然语言验收不等于逐合同独立证明，与实际收据一致。公开计划和约束注释把7个值误写8，终稿未重复，留低优先级模型算数缺陷。

写例还有真实系统教学误导：planner的Test surface从setup.py推出`python (pytest)`（planner.go:1193），真实测试import unittest且验证器选择python/unittest@packages/widget；模型因此提交错误的pytest/参数化PTO身份。8项行为合同已明确planning_only，错误PTO没有授per-contract confidence，故不是required义务误销账，但不能签首轮JSON/PTO质量通过。后续应把manifest项目标记与精确测试协议分开供证。当前一次正确micro/patch没有触发create_path_exists/modify拒绝，§151只保公开回归收据；本例也没有source-free补证，§149身份债不销。

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
