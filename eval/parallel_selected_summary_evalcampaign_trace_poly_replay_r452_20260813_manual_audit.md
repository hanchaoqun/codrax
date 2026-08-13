# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T19:30:15Z
- sweep_start_ts: 20260813-123014
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260813-123015 | answer_regex | none | 181s | 24 | read=4,repo_map=2,list=5,trace=0,source_lens=1 | midloop=4,inv=2/0,fin_reject=1,unavail=0,prune=0 | pass | Python `_HAVE_NATIVE` 分流、`_fastlex.tokenize_bytes`、PyO3 wrapper、Rust core、`best_merge` 与纯 Python fallback 的正文关系完整且引用准确。首图把 guard/模块/显示节点误画成 call 后被 typed gate 正确拒绝，模型删除 optional 图；请求未强制图，所以这是允许的表达选择，不由系统代画。 |
| 1 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260813-123015 | log_regex,trace_attachment,answer_contains,principal_answer | perf_triage+trace_query | 294s | 41 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail / typed-context conflict | B735-4 生产闭环：Analyzer 首次保持三个维度，把 causal_attribution 与 causal_diagnosis 配对；显式窗完整因果投影与自动补齐恢复。157.248/5.604/70.338/0 四态、双轴、链上席、业务 span、D/IO 均保留。失败集中在频率判定：模型正确看到 CPU0/CPU4 policy ceiling 和 58.320ms supply-fold，却把“上限存在”升级成目标实际受限/热控绑定。深审确认错误 completion reason 已从 finalizer authority/narrative lane 省略，模型 aggregate 也只留下四态；真正冲突来自 typed feed 一处称“明确热控轨上限”而另一处才说 policy/thermal ceiling 不单独证明 binding。B737 应统一为：上限来源记录、目标 slice 是否命中/绑定、供给相对理想基准 headroom 三个独立权限；不给系统结论权。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
