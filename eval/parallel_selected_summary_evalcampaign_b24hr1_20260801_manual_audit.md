# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T20:43:22Z
- sweep_start_ts: 20260801-134320
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | patch_c_typo | PASS | eval/results/patch_c_typo-20260801-134322 | write_apply,write_patch_oracle,answer_contains | none | 114s | 19 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | plan/apply/verify 通过；applied tree 仍只有 `main.c` 的 `retrun`→`return` 单行修改。 |
| 2 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260801-134322 | answer_regex,answer_contains | none | 158s | 23 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=6,inv=1/0,fin_reject=2,unavail=0,prune=0 | pass | 首段与 principal path 均明确 `buildAnalysisIR → gate.Run` 有向可达性未证明，两个 exact endpoint 保留；图仅含逐边 typed call evidence，未再反向拼接 `RunWith → Run`。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- `EVAL-B24-REACH1` 由真实 replay 覆盖：endpoint identity 与 directed reachability 已分离，model summary/principal list 不能再绕过最终 typed diagram 的方向结论。
- 效率同步改善：同 read case 从 B24-f 的 472s/10 rejects、B24-g 的 293s/8 rejects 收敛为 158s/2 rejects；剩余 reject 是 `ParseOutput` 简称未精确匹配 `analyzerEvaluator.ParseOutput`，一次 patch 后通过。
- write apply 连续三轮稳定，说明 call-chain authority 没有污染写模式 controller/plan/apply/verify。
- `EVAL-B24-EVALDIR1/P1` 由“8 次错误边修复”降为一次端点简称修复，降级为 P2 观察；`EVAL-B24-KEYSET1/P2` 仍开放，最终答案的多个完整清册明显偏宽，但不再影响主结论，避免继续在单 case 上过拟合。
