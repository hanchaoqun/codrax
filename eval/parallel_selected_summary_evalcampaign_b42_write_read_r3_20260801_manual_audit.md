# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T13:28:23Z
- sweep_start_ts: 20260802-062822
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | read_combo_log_current_source_explanation | PASS | eval/results/read_combo_log_current_source_explanation-20260802-062823 | log_attachment,answer_regex | log_triage | 161s | 28 | read=4,repo_map=1,list=0,trace=0,source_lens=0 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | `member_set` 已分出两条路径，但第二项只引用 `NoticeAnswerCheckRetry` 的 enum 声明，未读 emit/check/requeue 控制路径；又把 `first_byte_floor_warn` 的 soft predicate 实例化为本次 `model=demo` 的 40s 超时根因，而 `demo` 不满足 reasoning-family operand。声明只证明 carrier identity，规则存在也不证明本次实例命中。 |
| 1 | github_issue_napi_force_wasi_env_symptom | PASS | eval/results/github_issue_napi_force_wasi_env_symptom-20260802-062823 | write_apply,answer_regex | none | 315s | 19 | read=9,repo_map=2,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 生成 loader 的生产代码修复正确，但模型把回归测试改成检查生成源码是否含 `wasiBinding = require`；该语句对所有 env 值都存在，false/0/undefined 的 `doesNotMatch` 在 Node 可用时必败。Node/npm 缺失，fallback Python oracle 只查 token，局部 report passed；typed final report 已正确给出 `unverified/verification_proof_incomplete`，runner 却只读局部 report 而误报 PASS。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch verdict

- Runner: 2/2 PASS；人工：0/2 PASS。
- `EVAL-B42-PRODUCERROLE1/P1`：carrier 声明不能证明 producer、trigger、consumer 或后续控制动作。
- `EVAL-B42-RULEINSTANCE1/P1`：源码 predicate/threshold 只证明规则存在；必须从 typed runtime artifact 绑定全部关键 operand 后才能归因本次事件。
- `EVAL-B42-WRITETEST1/P0`：生成物回归必须执行/解析实际 branch 语义，不能用生成源码 token presence 代替行为。
- `EVAL-B42-EVALSTATUS1/P0`：归并到 B20 typed truth 闭环；eval apply verdict 必须同时消费 current-plan ChangeReport 与 same-plan WriteFinalReport，局部绿不得覆盖最终 unverified。

修复均不扫描用户原始输入、模型 thinking 或最终答案 prose；产品侧只加 soft guidance，eval 侧只消费 typed artifacts。
