# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T15:32:16Z
- sweep_start_ts: 20260805-083215
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260805-083216 | answer_regex | none | 150s | 20 | read=4,repo_map=4,list=0,trace=0,source_lens=0 | midloop=6,inv=2/0,fin_reject=3,unavail=0,prune=0 | fail | B117/B118 均获生产正证：无重复成员集，模型 typed 删除可选图后 rejected diagram 未再以系统附件复活。剩余失败是跨语言 endpoint 身份：答案把 PyO3 wrapper `#[pyfunction] tokenize_bytes` 与 Rust core `super::tokenize_bytes` 合并成一个“core”，遗漏 wrapper→core 的显式 landing/citation；3 次 reject 中一次为 provider 把 `replace_blocks` 编成 JSON 字符串，其余仍是无证 diagram 边。升级 `EVAL-B107-ENDPOINTAMBIG1`，需统一语言/FFI binding identity，不放宽 call-edge gate。 |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260805-083216 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 196s | 39 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=1,inv=2/1,fin_reject=0,unavail=0,prune=0 | fail | B119 生效：不再把同方向 10.433/7.386/6.673ms 相加，按方向发布单席 leader，且 direct-blocking authority 明确为未提供。深层系统 gap 是用户显式 114.94ms 窗已有自动补齐全窗 bundle，却被较早 50ms frame drilldown 抢走 projection 主窗；TargetState、wakeup path 与 compact leader 跨窗混池，答案遂把 50ms sleep 34.307ms 写进 114.94ms 总结，并继续把 pre-wakeup dependency 叙述为 post-wakeup CPU 延迟。立案 `EVAL-B119-REQWIN1/MULTIWINLEDGER1`。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
