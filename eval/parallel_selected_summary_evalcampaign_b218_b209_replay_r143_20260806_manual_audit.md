# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T02:38:46Z
- sweep_start_ts: 20260806-193844
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260806-193846 | answer_regex,answer_contains | none | 108s | 20 | read=2,repo_map=0,list=0,trace=0,source_lens=0 | midloop=4,inv=2/0,fin_reject=2,unavail=0,prune=0 | pass | 主链、工厂选择及 write/guard/flush 先后均正确；但短 owner=`log` 与限定 owner=`Logger.log` 未归并，typed lexical-order 胶囊没有发射。两次拒绝均来自模型主动添加缺少 typed ownership 的可选 sequence 图，最终诚实删图。 |
| 2 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260806-193846 | answer_regex,answer_contains | none | 166s | 21 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=5,inv=2/0,fin_reject=2,unavail=0,prune=0 | fail | 主调用链和 cooperative-super 顺序基本正确，但把精确注册键 `json` 错写成 `content_type()` 返回值 `application/json`，并错误声称以 MIME 类型注册/查找；typed cooperative delegation 已到 finalizer，方法定义未到，故 roster 未形成。两次拒绝来自把 lookup/return/MRO 候选关系画成 direct call，最终删图。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
