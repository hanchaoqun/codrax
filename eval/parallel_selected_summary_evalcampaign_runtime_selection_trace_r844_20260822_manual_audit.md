# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T05:56:46Z
- sweep_start_ts: 20260821-225644
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-225646 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 255s | 36 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000000..2.020000 用户窗、四线程三条 wakeup edge、11.000ms 链上 IO 第一席、三个独立 1.000ms runnable/优先级候选、实际占时/规则可消双账户、业务下钻、邻近/背景隔离和完整 Trace 因果投影均在；0 次成文拒绝。模型导语把彼此重叠的 14ms/17ms sleep 写成“逐跳叠加”，而系统投影稍后明确不可相加，属模型措辞自相矛盾；typed 数值、排名和守恒没有据此改变，不使用原文扫描硬改。模型也把四个线程称作“四跳”，严格说是四节点三边。 |
| 1 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260821-225646 | answer_regex,answer_contains | none | 380s | 35 | read=6,repo_map=2,list=0,trace=0,source_lens=1 | midloop=7,inv=3/1,fin_reject=3,unavail=0,prune=0 | pass | 事实正文正确回答 JsonPlugin、run_pipeline→resolve、REGISTRY 查表、cls() 实例化、executor callback、协作式 handle 链和 @register 导入期绑定。B1328 新教学逐字到达 analyzer prompt，但 analyzer 仍发 runtime_selection_profile=false，并把仓库发现的 JsonPlugin 填入 discover sink，随后被归一成 discover_terminal；故 B1328 只有测试闭环、无生产正证。Typed 注册/selector/return/callback/MRO 仍以断开组件出现，模型三轮尝试连续图均被正确证据门拒绝，最后删除可选图；终稿关系列表还把 resolve→cls() 标成“查找并返回类”，且系统源码定位补充只列无关 CsvPlugin，视觉与补充质量仍 partial。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
