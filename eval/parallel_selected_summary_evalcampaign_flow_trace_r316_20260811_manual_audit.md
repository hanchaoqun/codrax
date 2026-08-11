# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T14:08:02Z
- sweep_start_ts: 20260811-070800
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260811-070802 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 187s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式窗、4 次 windowed 查询、wakeup path、链上排序、frame absent/unproven、实际占用/规则可消双轴及自动投影均在；但模型正文把同一个 ThreadPoolForeg 链上 D/IO 席和主线程供给席先列为链上原因、又放进“背景”，并把 sleep 段称 wakeup latency、把“频点非最高”扩写成可能热节流。系统投影没有把背景加冕，故这是模型叙述波动/上下文心智负担，不以 prose 关键词加硬门；既有 B528 跨查询物理席去重继续开放。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260811-070802 | answer_regex,answer_contains,mermaid_edge_count | none | 193s | 30 | read=5,repo_map=2,list=0,trace=0,source_lens=0 | midloop=6,inv=2/0,fin_reject=1,unavail=0,prune=0 | fail | B534 的 same-turn soft navigation 已出现，但候选文件只被当作 declaration 文件直接 read；模型未先用 typed stem repo_map/grep 定位真实 writer/reader。第二次相同 completion 随即由 flow_participant_coverage low-delta 放行，BusContext/Mutable 仍无 operation row；Finalizer 正确拒绝无证边后只保留四阶段 precedence 和 BusContext 孤点，正文却继续声称经 BusContext/Mutable 传递。runner 的 edge-count/token oracle 误绿。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
