# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T12:57:14Z
- sweep_start_ts: 20260802-055713
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | read_combo_log_current_source_explanation | PASS | eval/results/read_combo_log_current_source_explanation-20260802-055714 | log_attachment,answer_regex | log_triage | 197s | 24 | read=3,repo_map=0,list=0,trace=0,source_lens=0 | midloop=2,inv=3/0,fin_reject=0,unavail=0,prune=0 | fail | P1 false green：超时路径正确，但把 IsStreamLevelRetryable 的 false/complement 臆断成内容校验失败；实际 contract.Check/requeue 是另一条控制路径且完全未读。 |
| 1 | github_issue_napi_force_wasi_env_symptom | PASS | eval/results/github_issue_napi_force_wasi_env_symptom-20260802-055714 | write_apply,answer_regex | none | 300s | 20 | read=12,repo_map=4,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=1,prune=0 | fail | P0 false green：forceWasi 声明在 renderNativeBinding 返回模板之外，生成 loader 引用未定义变量；静态 token oracle、Python checker 与 make check 均错误放行。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human audit details

### github_issue_napi_force_wasi_env_symptom

- 应用补丁把 `const forceWasi = ...` 放在 TypeScript generator 函数体内、返回模板字符串之前，但把 `if (!nativeBinding || forceWasi)` 放在生成模板内。生成的 JavaScript 因而没有该声明，运行时会得到未定义标识符。
- verify 在 npm 不可用后只执行 `tests/check_force_wasi.py` / `make check`；旧 checker 只在整个 producer source 中查 token，不能证明生成物词法作用域、解析或行为，因此给出 verified 假绿。
- 上游 PR #3236 的逻辑是 `false/0/其他值` 不强制 WASI、native 不可用时仍允许正常 fallback；首轮答案的布尔条件方向基本正确，错误在生成物边界与验证方法。
- 登记 `EVAL-B42-GENART1/P0`。通用处置不是识别 napi/forceWasi，而是要求 generator/template/emitter 变更在生成物自身范围内解析/执行验证。

### read_combo_log_current_source_explanation

- 日志边界“`phase=llm_request` 的 first-byte timeout 不能证明后续 validation 结果”正确。
- `internal/llm/retryable_error.go:IsStreamLevelRetryable` 只分类 transport/stream error；false 分支还包含 HTTP API error、重试耗尽、auth/schema/config 等非流错误，不能据此证明“内容校验失败”。
- 内容合同失败由 orchestrator 的 contract check、reject/retry budget 与 scheduler requeue 路径处理，是独立机制。explorer 没读该路径，却把超时 classifier 的 complement 当成 validation 机制。
- 登记 `EVAL-B42-CONTRAST1/P1`。通用处置要求机制对比的每一侧分别拥有 producer/control-path 证据；一侧 predicate 的 false 分支不是另一侧的证据。

## Batch decision

- runner：2/2 PASS；人工：0/2 PASS。
- 第一施工批只增强共享 soft guidance 与 eval behavioral oracle，不增加用户原文/模型答案扫描，不做系统结论替换，不影响 Trace 因果投影或自动补齐。
- 同对回放前，新 oracle 必须同时拒绝原始裸 truthiness 和首轮模板外声明补丁。
