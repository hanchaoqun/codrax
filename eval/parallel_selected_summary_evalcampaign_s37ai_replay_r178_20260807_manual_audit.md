# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T17:49:08Z
- sweep_start_ts: 20260807-104907
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_python_typo | PASS | eval/results/patch_python_typo-20260807-104908 | write_plan,write_patch_oracle | none | 102s | 21 | read=2,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | ChangePlan 只对 `main.py:20` 做 `retrun -> return` 的 structured replace，范围、旧值和验收项均正确，Planner 首次 emit 即通过。不过本轮模型没有生成 verification probe，因此没有实际执行 S37ah 的单行 `import main; ...` 生产路径；该修复仍由工具包完整测试见证，不能把本轮零拒绝虚记为 semicolon parser 的 production positive。Analyzer 为一个单行 typo 连续 4 次修正可选 field-value profile，仍有上游心智负担，但没有污染最终计划。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260807-104908 | answer_regex,answer_contains | none | 430s | 28 | read=6,repo_map=1,list=0,trace=0,source_lens=0 | midloop=11,inv=5/0,fin_reject=2,unavail=0,prune=0 | partial | S37ai 两项获得 production witness：Finalizer 收到 `unique_endpoint_relations=18 / weak_components=2 / max_out_degree=17 / single_linear=false`，Explorer 发出 `Run -> RunWith @ gate.go:135` 独立 call row；最终答案正确说明 `buildAnalysisIR` 与 `gate.Run` 无有向路径、两者分别汇入 `gate.RunWith`。但首稿 16 条可见边未同步 edge anchors，被拒；patch 又丢失精确 `gate.Run` 标签，再次被拒，随后模型流式连接异常并整轮重启才成功，总耗时 430s。答案仍把 17 条同一函数体内按源码顺序执行的 sibling calls 称为“中间函数”，虽未再伪称 17-hop 链，表达仍偏重。记新的过程/补丁保持 gap，不能由 runner PASS 关闭。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
