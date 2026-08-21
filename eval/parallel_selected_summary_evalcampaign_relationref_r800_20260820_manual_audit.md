# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T06:25:22Z
- sweep_start_ts: 20260820-232522
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_go_typo | PASS | eval/results/patch_go_typo-20260820-232522 | write_apply,write_patch_oracle,answer_contains | none | 99s | 26 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 精确单行把 `retrun` 改为 `return`；应用树无额外改动，`go test -json ./...` 实际执行且 1/1 通过，plan/report/final fingerprint、recovery ref、changed-path coverage 与 clean worktree audit 均闭合。低优先观察：controller handoff 重复了一组 approval/action/reason，analyzer 首轮把明确写请求标成 explain，但 write analyzer/controller 没有因此改变范围或跳过验证。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260820-232522 | answer_regex,answer_contains | none | 634s | 48 | read=26,repo_map=3,list=0,trace=0,source_lens=0 | midloop=16,inv=6/0,fin_reject=3,unavail=0,prune=3 | pass | 最终答案准确列出 analyze→explore→extract→finalize，表格五列完整，sequenceDiagram 合法且三条 precedence 均通过 typed 关系校验；系统未代写结论。过程存在确定性合同 gap：概念型 Principal Enumeration Rows 被 evidence_ids 教学与 source_inventory_row_id 行权同时覆盖，模型按第一次修补提示提交两套互斥 owner 后必然再拒一次。另有独立长尾：26 次 read、46 次 explorer iteration、16 次 midloop、3 次 history prune；在 completion-ready 后仍进行超宽 concrete-value/dataflow 扫描，需下一批单独审计。B1279 的冗余 relation selector 生产形本轮未触发。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
