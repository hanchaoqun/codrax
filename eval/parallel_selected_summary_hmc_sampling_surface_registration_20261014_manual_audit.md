# HMC243 双例真实评测人工审计

- date: 2026-10-15T03:45:11Z
- sweep_start_ts: 20261014-204511
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_sampling_surface_registration_20261014

机器判定保留原件；人工结论依据完整实际取数、阶段上下文、答案或代码/测试/控制器原件。两例均只执行一次，未追加第三例。实现和后续确定性回归不倒签本次答案。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_filtered_sampling_hotspots | PASS | eval/results/hmc_sampling_surface_registration_20261014/trace_filtered_sampling_hotspots-20261014-204511 | successful_tool_result,trace_attachment,primary_answer | perf_triage+trace_query | 164s | 42 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 原生35/7/100正确，但省略目标查询未自动继承线程，新事实选项未进入成文上下文；正文跨总体加权并误算32/132，附录不能抵销。 |
| 2 | native_registration_commandless | PASS | eval/results/hmc_sampling_surface_registration_20261014/native_registration_commandless-20261014-204511 | write_apply,write_patch_oracle,answer_contains | none | 186s | 31 | read=4,repo_map=2,list=0,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | PASS（普通源码交付） | 3个原生测试和1次探针成功；只读登记NOT_TRIGGERED。持久化finish reason误写4个测试，说明一致性仍FAIL，不算该例零缺陷。 |

## 原始失败与未覆盖分支

- A完整人工FAIL：真实perf_stats只填source/path/flavor，系统补窗[1,2)，没有补pid；event_search先显式选择pid11。actual finalizer只有事件库存选择器，原生采样数字未进入新事实组，错误调查摘要仍投递。首轮分析错误TID自然修复不等于采样query自然重试；合法DSO过滤、新采样selector、完整树和图未触发。primary/principal、Markdown和HTML正文均有相同错误，空根因sidecar不能为正文背书。详见原件归档中的`codrax-hmc243-filtered-manual-audit-v1.md`。
- B普通源码交付PASS不等于只读登记通过：源码隔离修改、真实测试/探针与结构化verified成立；没有登记proposal，也没有新进程登记恢复。最终卡片“4条验证结果”不错误；实际controller工具参数与持久化progress_ledger写“全部4个测试通过”则与原生3方法不符。`/workflow show`会消费该文本，但本次未调用该命令，不声称最终卡片显示了错误数字。另旧summary.md显示plan_written=no/materialization不可用，与实际write-apply.json和applied-tree矛盾，作为验收摘要缺陷留原18.5，不改机器原件。
- 两例使用冻结`b66fa1370072`构建；后续同批补救仅由确定性公开回归和完整测试验证，不用末版全绿倒签这两个较早模型结果。图没有真实模型或浏览器像素验收。
