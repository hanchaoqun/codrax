# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T22:25:59Z
- sweep_start_ts: 20260805-152558
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_java_typo | PASS | eval/results/patch_java_typo-20260805-152600 | write_plan,write_patch_oracle | none | 60s | 19 | read=1,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Plan-only 权限未越界，最终 ChangePlan 只有 `Main.java:16` 的 `retrun → return` 单行 patch，raw diff、structured edit、target path 和验收项一致，主仓无源码改动。write Analyzer 首轮虚构未取证的 hard `returns` command result，被 exact grounding gate 正确拒绝，第二轮改成 soft `satisfies`；结论正确但浪费一轮，确认 `EVAL-B138-WRITECONTRACTMIND1` 的通用教学降噪机会。 |
| 1 | trace_query_state_churn_root_cause_rank | PASS | eval/results/trace_query_state_churn_root_cause_rank-20260805-152600 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 158s | 30 | read=0,repo_map=0,list=0,trace=1,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 11.000–11.008s 窗、pid=20、root_cause_rank、因果投影与自动补齐均保留。模型正文同时给出两轴：真实主要占用/新修向与规则内可消 5.000ms；正确把 fragmented_runnable_wait 吸收到同一 runnable_wait 席、CPU pressure 7.700 cpu·ms 留在背景，19 switches/20 segments 单一 state 账不再出现 19/20 vs 20/21 双版本。无 frame/deadline 证据的边界也有披露。Harmony `prio=53/ohos_rt` 与“larger numeric higher”经既有 typed 平台合同核对正确。pre-triage 模型曾错算 23/64 switches 并作连续帧预算外推，但 deterministic query 后该 navigation-only observation 未进入 Finalizer；记 `EVAL-B138-PRETRIAGENOISE1=P2/model-variance-contained`。Analyzer 首轮把 `fact_families` 错带到 `causal_diagnosis`，现有初始教学与 reject 同向，单次修复，暂不新增 schema 硬化。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
