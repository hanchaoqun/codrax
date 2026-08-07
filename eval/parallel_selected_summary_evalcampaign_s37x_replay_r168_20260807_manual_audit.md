# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T14:20:33Z
- sweep_start_ts: 20260807-072032
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260807-072033 | log_regex,answer_regex | none | 46s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | `route=data`，单批 `custom_transform`，零 repair/拒绝；最终字节为 `{"ids":["u1","u3"]}`，没有代码围栏或解释。JSON 教学、schema、执行与终态投影一致，未发现新合同冲突。 |
| 1 | qf_sequence_analyzer_gate | FAIL | eval/results/qf_sequence_analyzer_gate-20260807-072033 | answer_regex,answer_contains | none | 196s | 25 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=7,inv=4/0,fin_reject=1,unavail=0,prune=0 | fail | `typed direct-call frontier source="buildAnalysisIR" sink="gate.Run" emitted=24 total=235` 已在生产日志发射，且模型主动搜索了 `analyzerGraphForNormalize`，证明 B262 接线有效；最终未选该 helper 是模型选材，不应由单一 helper regex 硬判。真实缺口是 `gate.go:135` 从未进入 read closure，因而答案漏掉 `gate.Run -> RunWith` 反向/共享前沿边，只能说“需要进一步探索”。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 审计结论

1. runner 的首个失败原因是 `analyzerGraphForNormalize` 缺失，但该断言把 235 条直接调用中的一个 helper 固化为唯一正确选材，属于过硬 oracle。frontier 已把它提供给模型，模型也消费到搜索动作；系统不应为了让 case 通过而强迫答案列出它。
2. `buildAnalysisIR -> gate.RunWith @ analyzer.go:2667` 已正确进入 typed evidence，`no_directed_path` 也没有伪造 `buildAnalysisIR -> gate.Run`。不过 exact sink `gate.Run` 的函数体未被 `read_file`，所以现有“已读 AST 关系必须转成 EvidenceItem”硬门依法旁路；最终缺少真实的 `gate.Run -> RunWith @ gate.go:135`。
3. 最优方案是两阶段权限链：唯一 source/sink 的 AST 图只软提示 reverse/shared-frontier 邻接行；模型读到精确行后，现有 typed relation handoff 才允许阻止遗漏。未读图关系不得直接成为证据或系统结论。
4. JSON 案人工与 runner 均通过，且没有畸形 JSON、自愈、降级或教学矛盾信号，本批不改 JSON 路径。
5. 本轮不涉及 runtime Trace。显式时间窗、因果投影、系统自动补齐、根因排序、唤醒链、窗内可消除量及“真实耗时贡献 / 规则内可消除量”双维根因分析保持原样。
