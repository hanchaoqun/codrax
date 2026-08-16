# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T17:51:41Z
- sweep_start_ts: 20260816-105139
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260816-105141 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 151s | 38 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 233.190ms 窗、Trace 因果投影和自动补齐均在。模型先给出自身 running 74.915ms / 规则计价 65.912ms、D-state 36.757ms / 11 次 dma_fence_default_w，并把真实占时与可消除量分成两轴。logd.writer 全窗 49.656ms 被严格拆为链上锚定 0.033ms 与无链凭证背景 49.623ms，正文也明确背景不入根因排序。优先级反转、调度供给、算力、D/IO、业务 span 和确定性语义方向均未丢；零成文拒绝。确定性补充很长且仍有内部合同词，属于展示债，不改变本轮正确性。 |
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260816-105141 | answer_regex | none | 152s | 27 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=3,inv=2/0,fin_reject=1,unavail=0,prune=0 | partial | 文字列出了 Python guard/native/fallback、_fastlex、PyO3 wrapper、Rust core 与 best_merge；但模型把 _HAVE_NATIVE 的设置位置误写成 __init__.py（实际就在 tokenizer.py:2-6），并在已读 _tokenize_slow:24-36 后仍称其内部细节超出范围。第一稿图因注册行使用 registration+callback 被降为 ungrounded，typed 池只剩两段断开的真实 call component；validator 正确拒绝未证桥，模型随后删图。日志证明不是纯波动：系统看见同一行唯一 add_function(wrap_pyfunction!(...)) 绑定，却没有把错误 anchor 分类转成 completion-blocking 精确重发义务。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
