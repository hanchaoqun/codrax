# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T01:49:28Z
- sweep_start_ts: 20260820-184928
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260820-184928 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 297s | 38 | read=0,repo_map=0,list=0,trace=7,source_lens=0 | midloop=0,inv=3/1,fin_reject=0,unavail=0,prune=0 | partial | 指定 2.000..2.020s 主窗、自动补采、四跳 waker→wakee 链、11.000ms 链上 IO 主席、三个独立 1.000ms 调度/优先级候选、实际占时/规则可消双账户及 Trace 因果投影均保留；邻近/背景未升为主因，活动流未按耗时降级。模型再次仅凭 `fscache_page_wait_on_page_bit` 名字扩写为 fscache 后端磁盘/网络机理，并从 cross-CPU 推出 NUMA 排查；typed 输入明确没有资源/子系统/直接竞争权限，故为 B1271 第三次生产复现。中文正文还复制 `kernel_wait_callsite` 内部枚举，B1272 展示语义债确认。 |
| 1 | read_combo_pipeline_sequence_table | FAIL | eval/results/read_combo_pipeline_sequence_table-20260820-184928 | answer_regex,answer_contains | none | 1109s | 73 | read=27,repo_map=3,list=0,trace=0,source_lens=0 | midloop=30,inv=7/0,fin_reject=20,unavail=0,prune=7 | fail | B1270 的 ref 入口被模型正确采用，但 producer 提示中的 ref 来自规范化文档，executor live lease 的 ref 来自原 rejected patch base；同一失败的两个载体不同。模型两轮逐字复制系统给出的 ref，均稳定被拒为 unknown/stale，继而退回全块重写并耗尽 20 次 reject。恢复稿虽有四阶段、表和图，但仍含未充分证明的统一 dispatch 叙述，且是未通过最终结构校验的旧稿，不能视为合格答案。确定性根因记为 B1270A，不是模型波动。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
