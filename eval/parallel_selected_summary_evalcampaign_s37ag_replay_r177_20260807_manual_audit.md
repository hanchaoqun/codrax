# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T17:34:02Z
- sweep_start_ts: 20260807-103401
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_python_typo | PASS | eval/results/patch_python_typo-20260807-103403 | write_plan,write_patch_oracle | none | 160s | 21 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 最终 ChangePlan 仅替换 main.py 第 20 行 retrun 为 return，范围与验收正确；但首次 Planner 的合法单行 `import main; ...` 被 probe coupling 解析器连续拒绝 4 次，第二次 dispatch 改成多行才通过。确认 EVAL-B296-PYSEMICOLONIMPORT1；runner 指标未计入 planner 内部 structured rejects。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260807-103403 | answer_regex,answer_contains | none | 234s | 29 | read=5,repo_map=2,list=0,trace=0,source_lens=0 | midloop=7,inv=3/0,fin_reject=1,unavail=0,prune=0 | partial | S37ag 生效：20 条唯一 call edge 全部进入 Mermaid，无重复箭头/anchor，也无八边截断。答案正确披露 buildAnalysisIR 只调用 RunWith，但把 `Run -> RunWith` 已读源码说成“关系需进一步验证”，且仍将 sibling calls 称作中间处理顺序；Explorer 没有把 gate.go:135 另发 typed call row。B291 仍待真实 witness，B294 保持开放。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
