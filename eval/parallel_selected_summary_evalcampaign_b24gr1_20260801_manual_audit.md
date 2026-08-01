# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T20:25:15Z
- sweep_start_ts: 20260801-132514
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | patch_c_typo | PASS | eval/results/patch_c_typo-20260801-132515 | write_apply,write_patch_oracle,answer_contains | none | 129s | 19 | read=2,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | plan/apply/verify 通过；applied tree 仍只有 `main.c` 的 `retrun`→`return` 单行变更。 |
| 2 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260801-132515 | answer_regex,answer_contains | none | 293s | 31 | read=6,repo_map=1,list=1,trace=0,source_lens=0 | midloop=8,inv=2/1,fin_reject=8,unavail=1,prune=0 | fail | B24-g 生效：required endpoint 仅 2 个，`analyzer.go` 未再升级，endpoint/pruning reject 消失；但最终摘要仍反向声称 `gate.RunWith` 调用 `gate.Run`。typed 图最终只保留 `buildAnalysisIR → gate.RunWith` 等已证边，未到达 `gate.Run`，说明“端点在场”尚未升级为“端点间有向可达”。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- `EVAL-B24-ENDPPRUNE1` 与 `EVAL-B24-ENDPOINTSCOPE1` 的目标症状已覆盖：同 case 从 472s/10 rejects 降到 293s/8 rejects，剩余 8 次全部是 diagram typed-edge 修复；`analyzer.go` 不再进入 required endpoint set。
- 新登记 `EVAL-B24-REACH1/P1`：exact endpoint presence 不能证明 source→sink 可达。最终 diagram validator 正确删除反向/无证边，但 summary 与 principal path list 仍可宣称“完整链”，形成结构化图与正文结论矛盾。
- 最优方案是从 `QFCallChain + exactly-two typed required endpoints + accepted citable ClaimCallEdge` 编译 directed reachability；未证明时由 system authority 把 summary/principal-path carrier 收敛成“unproven”，而不是扫描 raw request/final prose，也不把 sibling definition 当边。
- diagram 仍有 8 次 repair，主要是 finalizer 把定义/同函数内部 check 行误画成边；这是 `EVAL-B24-EVALDIR1/P1` 的剩余效率问题，与本批 reachability 正确性分开处理。
