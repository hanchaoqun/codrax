# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T03:48:31Z
- sweep_start_ts: 20260803-204830
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | arkts_repomap | PASS | eval/results/arkts_repomap-20260803-204831 | typed_inventory_rowset,answer_contains | none | 126s | 20 | read=6,repo_map=1,list=1,trace=0,source_lens=1 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 4 个 `@Entry + @Component` struct 与 2 个 `@Builder` 函数完整、文件/行号准确；`EntryAbility` 被正确排除。typed rowset 通过且零成文拒绝，未复现 Cangjie 同名声明 identity 观察项。 |
| 1 | qf_sequence_analyzer_gate | FAIL | eval/results/qf_sequence_analyzer_gate-20260803-204831 | answer_regex,answer_contains | none | 418s | 26 | read=13,repo_map=2,list=0,trace=0,source_lens=0 | midloop=17,inv=7/1,fin_reject=6,unavail=0,prune=0 | fail | Explorer 已正确接受 `no_directed_path`：最近已证路径止于 `gate.RunWith`，`gate.Run -> RunWith` 是独立反向 wrapper。Finalizer 将同一真实 call edge 的短名证据 `Run -> RunWith` 与图中限定名 `gate.Run -> gate.RunWith` 判为不等价，连续 6 次拒绝后 degraded 出厂；runner 新降级闸正确判 FAIL。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
