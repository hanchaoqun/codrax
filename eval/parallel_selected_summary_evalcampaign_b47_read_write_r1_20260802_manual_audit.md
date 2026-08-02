# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T22:12:21Z
- sweep_start_ts: 20260802-151219
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_log_current_code_dimensions | PASS | eval/results/read_combo_log_current_code_dimensions-20260802-151221 | log_attachment,answer_regex | log_triage | 258s | 28 | read=6,repo_map=0,list=0,trace=0,source_lens=0 | midloop=5,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | `4/4` 是 read pipeline 的 finalize stage ordinal/total，不是“四个模型”；Log Triage 已先把它误铸成 `subject=全部模型失败`，Explorer/Finalizer 又把 attempt、identical-error streak、local fallback budget、repair hard cap 四个不同计数域串成历史实证链。日志/源码通道分离正确，6 个源码文件也真实读取，但上下文缺少 renderer progress 与 retry counter ownership 的 typed 语义，最终 still_present 结论过强。analyzer 另有 3 次 schema 修复，非本轮主 GAP。 |
| 2 | github_issue_commons_lang_random_ascii_symptom | FAIL | eval/results/github_issue_commons_lang_random_ascii_symptom-20260802-151221 | write_apply,answer_regex | none | 426s | 19 | read=10,repo_map=2,list=0,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 第一版实现/测试被 `make check` 正确打红，replan 后 source 用 `end <= 0x7f`、测试用 0x400/0x370/0x660，第二次 `make check` exit=0；最终却因 TestSurface 的 Python driver family 与两个精确 Java target family 不相交而判 `changed_path_verification_uncovered`。最终 patch 大概率正确但交付为 unverified，属于 verification authority 轴混淆。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Gap decisions

- `EVAL-B47-XCOV1/P1`：meta-runner 的 driver language 与 exact checked target language 被当成同一轴。修复必须只授权成功执行的精确 repository-declared target roster member，并禁止扩到同目录/同语言 sibling；单独记录 `declared_project_check` caliber，不伪装成 Java/Rust project runner。
- `EVAL-B47-RUNTIMESEM1/P1`：日志预处理会把用户可见 `K/N` progress 自由解释成对象数量，并把当前源码中的独立 retry/cap 定义当成历史日志已走过的连续链。需要在模型判断前提供 typed operational-event semantics/counter-domain ownership；不得扫描或重写最终答案。
- 本轮没有 Trace 输入；显式时间窗 causal projection、系统补采和 relation-claim 链未进入改动面，保持不变量。
