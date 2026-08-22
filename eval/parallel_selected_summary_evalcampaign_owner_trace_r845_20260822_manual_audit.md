# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T06:18:48Z
- sweep_start_ts: 20260821-231847
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-231848 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 140s | 33 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | uncertain | 系统精确面完整：显式 20ms 用户窗、四线程三条 wakeup edge、11.000ms 链上 IO 第一席、三个独立 1.000ms runnable/优先级候选、实际占时/规则可消双账户、完整 Trace 因果投影及邻近/背景隔离均在。模型先正确披露 wait-for-work/work-completion/direct-blocking authority 均未建立，后文却又称“主要阻塞原因”“间接受到上游 IO 阻塞传导”“候选叠加”，并从 fscache_page_wait_on_page_bit 标识符猜出页面缓存、网络文件系统层面；同页因果口径矛盾，故人工不签 pass。typed 投影未随之越权，不扫描/重写模型原文。 |
| 1 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260821-231848 | answer_regex,answer_contains | none | 259s | 29 | read=8,repo_map=1,list=1,trace=0,source_lens=0 | midloop=6,inv=3/0,fin_reject=2,unavail=0,prune=0 | pass | 事实正文正确回答 JsonPlugin、run_pipeline→resolve、REGISTRY 查表、cls() 实例化、executor callback 和 @register 导入期绑定；JsonPlugin@plugins.py:18 已由模型自身引用，r844 的无关 CsvPlugin 系统补充不再出现。B1244 本轮无自然歧义触发，记 no-regression 而非生产命中。Analyzer 正确发 source=run_pipeline/sink=""/discover，但 runtime_selection_profile 仍为 false，B1328 仅部分遵循。模型两轮修图后仍删除可选 diagram，终稿只保留 run_pipeline→resolve 与 run_pipeline→plugin.handle 两条结构化关系；动态 selector/register/lookup/factory/MRO 关系岛未闭，B1329 再确认。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
