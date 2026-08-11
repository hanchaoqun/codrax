# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T22:31:35Z
- sweep_start_ts: 20260811-153133
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_go_typo | PASS | eval/results/patch_go_typo-20260811-153135 | write_apply,write_patch_oracle,answer_contains | none | 96s | 22 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 计划为单个 `kind=patch`，diff 只把 `main.go:25` 的 `retrun` 改为 `return`；git apply 成功，post-apply 树其余内容未变。验证器从 typed TestSurface 选择 `go test -json ./...`，exit=0、1/1 测试通过，changed_path_coverage=`main.go/covered/project_runner/target_behavior`。这是单批 plan→apply→verify 正证，不覆盖 replan 后累计验证域恢复，不能据此替 T7-1 类场景签绿。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260811-153135 | answer_regex,answer_contains | none | 165s | 27 | read=3,repo_map=2,list=0,trace=0,source_lens=1 | midloop=8,inv=4/1,fin_reject=0,unavail=0,prune=0 | pass | B576 production positive：最终 sequenceDiagram 只有一个 `gate.RunWith` participant，分别承接 `buildAnalysisIR -> gate.RunWith` 与 `gate.Run -> RunWith` 两条 typed 入边；正文明确目标方向不可达，没有把共享汇点写成串行链。零 finalizer reject，未发生系统补边、改图或代写结论。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
