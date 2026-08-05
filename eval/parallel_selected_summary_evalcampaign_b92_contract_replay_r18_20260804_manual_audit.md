# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T03:20:16Z
- sweep_start_ts: 20260804-202015
- total cases: 2
- parallel: 2
- timeout: 1500s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_multi_member_set_count_caveat | FAIL | eval/results/qf_multi_member_set_count_caveat-20260804-202016 | answer_regex,answer_contains | none | 415s | 25 | read=2,repo_map=5,list=1,trace=0,source_lens=5 | midloop=5,inv=2/1,fin_reject=1,unavail=0,prune=0 | fail | B91-C 已消除 56-function/test 污染：最终 hard roster 只有 production rows，没有 `_test.go` 入口。但 analyzer 在 compound `type,function,constant` profile 上误置 enum-only `type_underlying=string + requires_const_set=true`，而 source lens 又把空 query 自动补为整段 source quote + 六个 entity，五次 lens 均在 163 个已索引符号上返回零 member rows。模型靠两次源码读取手工形成 3/2/30，漏掉 production `IsRegistered`、`RegisteredKinds`、`SetExternalArtifactFloor`，动态真值为 3/5/30。新 `EVAL-B92-INVENTORYSELECT1`：category enumeration 不得被隐式 token query 收窄；enum-only facet 只有在 typed role universe 仅为 type(+supporting constant) 时才可折叠 principal roles。 |
| 1 | qf_sequence_analyzer_gate | FAIL | eval/results/qf_sequence_analyzer_gate-20260804-202016 | answer_regex,answer_contains | none | 681s | 31 | read=14,repo_map=1,list=0,trace=0,source_lens=1 | midloop=15,inv=8/1,fin_reject=6,unavail=0,prune=0 | fail | 源码真值仍是 `buildAnalysisIR -> gate.RunWith` 与独立的 `gate.Run -> gate.RunWith`，不存在到 `gate.Run` 的有向路径；降级答案却再次写成 `buildAnalysisIR -> gate.RunWith -> Run`，人工 FAIL。前几次 validator 正确拒绝反向/虚构边；后期稿已保留两条正确 typed edge，却为同一 `gate.RunWith` 建了 `RW`/`RW2` 两个 participant alias，resolver 把两条正确 edge 统一报成 `missing_call_anchor`，反馈没有点明 duplicate identity，最终 6 次耗尽。登记 `EVAL-B92-DIAGIDENT1=P1`：先以精确 parsed participant identity 检出同 label 多 alias 并给单一可执行修复，不放宽 call evidence gate；另一次 Finalizer 首字节等待约 3 分钟属 provider 波动，不立系统 gap。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
