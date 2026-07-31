# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T15:28:41Z
- sweep_start_ts: 20260731-082841
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_e2_cross_trace_asymmetry | PASS | eval/results/real_trace_e2_cross_trace_asymmetry-20260731-082841 | log_regex,answer_regex,answer_contains | none | 119s | 34 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=0,prune=0 | fail | T11 已覆盖：普通双工件比较不再出现因果投影或 last-mile 观测墙，且没有源码读取。144.557ms/0.556ms、90 条 cpu_frequency、VSync 单边采样和 31637s≈8.8h 均正确；但答案无证据声称两份工件“来自同一台设备（com.baidu.tieba 进程）”，并把两个 artifact-local identity 组合成共享时基关系，还把第一份称为“完整帧渲染链路”。跨工件设备/session/clock relation 未经 typed calibration 证明，人工判 fail。 |
| 2 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260731-082841 | typed_inventory_rowset,dimension_substring,answer_contains | none | 219s | 21 | read=9,repo_map=5,list=0,trace=0,source_lens=5 | midloop=5,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | S9 已覆盖：analyzer 正确保留 name/location/package、构造 source quotes，且未再由 guessed required_file 缩窄。但首个确定性 source_inventory 把 ArkTS `@Extend(Text) function highlight` 的 parser row 铸成裸 `extend` family；完整性门随后要求补入该行，最终错误输出 3 个 extend，而 case 的 Cangjie ground truth 为 2。runner 只核对两条 expected row 是否出现，未拒绝额外第三行，属于 false PASS。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
