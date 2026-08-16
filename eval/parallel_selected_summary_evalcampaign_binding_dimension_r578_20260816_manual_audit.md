# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T18:07:03Z
- sweep_start_ts: 20260816-110702
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260816-110703 | answer_regex | none | 140s | 26 | read=2,repo_map=2,list=0,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=1,unavail=0,prune=0 | partial | B919 获生产正证：错误的 `registration+callback` 被要求按已读 line 47 重发为 `call/add_function/wrap_pyfunction!`，随后产生 exact registered-export handoff，原错误行没有被系统晋升。终稿保留 Python→native wrapper→Rust core 与 fallback，但删掉了可选图，并错误声称已读的 `_tokenize_slow` 实现未核。不是纯模型波动：Finalizer 上下文先发布 `verified_relation_component_count=3; inter_component_bridge_status=unproven_between_components` 和“fragments disconnected”，后面又发布 exact registered-export handoff；粗组件断言与精确 binding authority 自相矛盾，增加了模型错误组图和删图概率，冻结为 B921。 |
| 2 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260816-110703 | answer_regex,answer_contains | none | 483s | 41 | read=24,repo_map=3,list=0,trace=0,source_lens=1 | midloop=10,inv=4/0,fin_reject=5,unavail=0,prune=1 | partial | 正文与表格基本覆盖 read-mode 四阶段、职责及主要载体；前四次关系拒绝能阻止虚构调用。最终图却把两个不同的 typed value endpoints 分别声明成同一个 `IRflow`/`EIflow` alias 后画成 `IRflow->>IRflow`、`EIflow->>EIflow` 自环，validator 因 anchor 的 from/to identity 各自有证据而放行。该图把跨对象数据流伪装成对象自循环，是精确 structured alias-collapse GAP B922；应按 typed identity 与 node alias 的矛盾拒绝，不能扫描消息文字或放宽关系证据门。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
