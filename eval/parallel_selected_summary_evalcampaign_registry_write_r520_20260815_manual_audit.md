# Selected Eval Manual Audit Scaffold

- date: 2026-08-15T17:58:55Z
- sweep_start_ts: 20260815-105854
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_python_typo | PASS | eval/results/patch_python_typo-20260815-105855 | write_plan,write_patch_oracle | none | 54s | 24 | read=1,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 精确单文件、单行 typo 计划；文件、行号、旧值/新值和验收项一致，无越界改动、无成文重试。 |
| 1 | qf_relation_subagent_registry | PASS | eval/results/qf_relation_subagent_registry-20260815-105855 | answer_regex,answer_contains | none | 337s | 28 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 结论 `1 / explorer` 正确，注册与 `Name()` 两轴也写出；但 aggregate 仍为 `fact_authority=advisory_model_inference/principal_contract=not_authorized`，finalizer 明示无 evidence-authorized principal relation set，最终又追加弱证据 caveat。B847 已使 register 轴进入权威车道，残余是 B849：精确 bridge 候选只以 registrar=`RegisterDefaultSubAgents` 为 source，生产 analyzer 只给被注册对象 `SubAgentRegistry`，同一注册事实无法按 target/receiver 查询。另有 B848：单表行只能放一个 `citation_ref`，注册调用与 `Name()` 返回两份证据不能同时绑定，最终引用池清掉了注册调用。337s 主因是 analyzer 首轮已持续产生字节约 4 分钟；无 fixed-age 降级、无 finalizer reject，判为 provider/model 时延波动而非成文合同重试。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
