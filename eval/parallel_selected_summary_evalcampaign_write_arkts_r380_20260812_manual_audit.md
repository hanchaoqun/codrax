# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T10:37:12Z
- sweep_start_ts: 20260812-033710
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | patch_go_typo | PASS | eval/results/patch_go_typo-20260812-033712 | write_apply,write_patch_oracle,answer_contains | none | 91s | 23 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Auto Pilot 只把 `retrun` 改为 `return`；最终 diff 仅一行。计划内 `greet_compiles`、`TestGreet`、`go test -json ./...` 全通过，`main.go` 被 project runner 覆盖，四项 verification obligation 均 closed。日志中的 `verification_probe_missing_plan_contract_ref` 是 advisory，未替代独立项目验证，也未放宽交付。 |
| 2 | arkts_repomap | FAIL | eval/results/arkts_repomap-20260812-033712 | typed_inventory_rowset,answer_contains | none | 205s | 25 | read=5,repo_map=2,list=0,trace=0,source_lens=2 | midloop=6,inv=1/0,fin_reject=6,unavail=1,prune=0 | fail | 探索与最终事实均找全 4 个 Entry（Index/ParentComponent/StyledPage/ListPage）和 2 个 Builder（defaultHeader/GlobalCard）。失败有两层系统原因：① typed row 的 member=`Index (struct)`，模型用 reader-facing `Index` 并照抄精确 row_id；validator 仍以单向装饰别名判为“row_id unknown/different member”，同因连续拒绝 6 次，修补提示未披露真实 label 差异；模型最终删除 row_id 才通过。② typed `requested_fields=[name,location]` 已存在，但教学允许 citation 代替可见 location，最终文件路径只留在引用池，行正文仅写“第 N 行”，因此 inventory oracle 正确失败。立案 B635/B636；不是 ArkTS 抽取失败或单纯模型波动。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
