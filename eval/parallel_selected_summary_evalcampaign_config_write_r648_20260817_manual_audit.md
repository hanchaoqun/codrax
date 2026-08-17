# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T22:46:25Z
- sweep_start_ts: 20260817-154624
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_zod_prefault | FAIL | eval/results/github_issue_zod_prefault-20260817-154625 | write_apply,answer_regex | none | 129s | 25 | read=2,repo_map=2,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=1,prune=0 | pass | 源码把 truthiness 改为结构化存在性判断，false/0/空串及已有 default 回归均落地；`make check` 通过。两条 changed path 仅有 typed `source_static`，当前执行环境没有 Node，系统按既定验证红线降为 `accept_unverified`，这是诚实能力边界而非代码失败；不得为 runner 变绿把静态检查冒充目标行为执行。 |
| 1 | read_combo_config_two_knobs_precedence | FAIL | eval/results/read_combo_config_two_knobs_precedence-20260817-154625 | answer_regex,answer_contains | none | 204s | 34 | read=10,repo_map=0,list=0,trace=0,source_lens=0 | fail | B1019 修复获生产正证：不再出现 alias/not-found 伪头行，也没有 resolved→absence。新失败是代码默认值取错：Explorer 未读 `cmd/root.go:3147` 的 `MaxRetriesPerStage: 3`，把下游 `dynamicAnalyzeRetries(base<1)=1` 当成默认值。Analyzer 给出两个用户键及四个展示维度，却未发 exact_targets/context roles；多配置键的隐式 exact contract 又被“多实体歧义”整体关闭，导致精确补齐缺席。三个 completion 尝试均用 member_set 代替逐键 code-default scalar，终稿错误写 1。另有重复/空配置清单补充，维持 B1020 P2 观察。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Runner: `0/2 PASS`; human: one correct delivery with honest unverified boundary, one substantive config-value failure.
- B1019 is production-closed by this replay.
- B1021 is a typed carrier/navigation gap, not a finalizer wording fluctuation: a finite multi-key config comparison lost exact-resolution before exploration, so the system never supplied the source-completion pressure that exists for a single key.
- The write case does not justify a production change on this host. Direct-runtime proof follow-up already requires an actually available runtime; preserving `unverified` is the correct result.
