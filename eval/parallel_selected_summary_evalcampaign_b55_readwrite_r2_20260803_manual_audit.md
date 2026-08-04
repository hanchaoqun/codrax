# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T01:35:04Z
- sweep_start_ts: 20260803-183503
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260803-183504 | answer_regex,answer_contains | none | 183s | 29 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=9,inv=3/1,fin_reject=2,unavail=0,prune=0 | fail-system | Runner 的正则只证明有图和目标名。源码实际是 buildAnalysisIR→gate.RunWith，gate.Run→RunWith（反向 wrapper）；不存在 RunWith→Run。Explorer 把 Run 的定义当终点可达并错误签绿，Finalizer 图表门虽删掉伪边，summary/第 15 项仍声称 RunWith 提交给 Run，人工判错。首稿还把同一 caller 内按源码顺序发生的 fan-out 画成 helper 串联，精确图表门正确拦截；两次 final reject 中第一次有实质价值，第二次暴露探索 authority 不完整。 |
| 2 | github_issue_commons_lang_random_ascii_symptom | FAIL | eval/results/github_issue_commons_lang_random_ascii_symptom-20260803-183504 | write_apply,answer_regex | none | 393s | 19 | read=11,repo_map=3,list=1,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | uncertain-safe-fail | Patch 去掉 ASCII 上界并补 Unicode 测试，replan 后 make check 与 Python source probe 均通过；但 Maven 缺失，Java 行为未编译/执行。最终 fail-closed 正确。系统 GAP 是 typed changed_path_verification_uncovered 后的补证计划仍可缺 contract_refs/changed_symbol_refs，且 Python 静态源码探针无法拥有 Java behavior authority，导致又走完一轮 apply/verify 才发现同一缺口。不能把该探针升格成 Java 行为验证。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- `EVAL-B55-CALLSINK1`（P0）：精确调用链终点目前只要求“出现/定义”，没有要求 typed call-edge 的有向可达；support/explorer 的前缀兼容还把 `RunWith` 当成 `Run`。这使探索错误闭环，最终图表与正文 authority 分裂。
- 泛化修向：在语言无关 typed call-edge 图上验证 source→sink；定义只证明身份；exact short/qualified alias 仅允许无歧义等价，不允许 prefix sibling。若源码证明没有该方向，由模型通过 typed `no_directed_path` 声明终止，系统只把声明和精确边界送入成文上下文，不替模型写结论。运行时 Trace causal projection 明确排除。
- `EVAL-B55-VERIFYREF1`（P1）：写模式的 proof-followup 缺少精确目标/合同引用时应在计划边界修复，不能等昂贵验证末端再失败；跨语言 driver 只能取得 source_static 能力，不能冒充目标语言行为验证。保持开放，下一小批施工。
