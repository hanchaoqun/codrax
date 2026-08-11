# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T17:04:34Z
- sweep_start_ts: 20260811-100433
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260811-100434 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 157s | 40 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass-with-caveat | 显式 114.940ms 窗、4 次 typed trace_query 覆盖 5 个维度族、自动补齐和 Trace 因果投影均在；链上 CookieMonsterCl 23.994ms、NetworkService 19.041ms、ThreadPoolForeg D/IO、目标 running 算力缺口、VerifyClass 语义点与实际占用/规则可消双轴均保留，adjacent/background 未升入链上榜。模型自行把互相唤醒描述为“链上阻塞传导”、把可重叠 IO 行相加为 24.492/27.942ms，并称主线程卡顿“完全来自 S 状态”，超过 typed 口径；记 B544 软上下文债，不以扫描终稿或系统改写结论处理。Finalizer 首稿即通过。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260811-100435 | answer_regex,answer_contains,mermaid_edge_count | none | 508s | 37 | read=15,repo_map=2,list=0,trace=0,source_lens=0 | midloop=8,inv=3/0,fin_reject=4,unavail=0,prune=0 | fail | B543 获生产正证：同一 Finalizer prompt 只发布 `incident=[analyzer explorer extractor finalizer Mutable BusContext]`，不再同时发布 BusContext/Mutable boundary recipe。B545 的 `firstFinalizeDraft/strings.TrimSpace(...)` 自冲突本轮未复现，但原 witness 未再次被模型选择，记无回归而非完全生产闭环。新 GAP：最终 Mermaid 虽通过，却没有 BusContext 节点，也没有四 Agent 与 Mutable/BusContext 间的数据流；validator 把 exact internal operation 经 declared type 对齐到 participant 当成“参与者已可见”，使用户点名的业务参与者可被技术端点替代（B548）。同一 `available_typed_incident_edge_not_rendered` 六席提示连续拒绝 3 次，只给大段全局 capsule、未给每席 bounded candidate recipe，造成 4 次 Finalizer 拒绝（B549）。508s 全程有模型/工具进展，系统等待模型答案，未触发四分钟降级或系统代答。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
