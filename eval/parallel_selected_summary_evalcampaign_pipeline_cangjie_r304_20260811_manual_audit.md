# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T10:11:48Z
- sweep_start_ts: 20260811-031146
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260811-031148 | typed_inventory_rowset,dimension_substring,answer_contains | none | 162s | 23 | read=8,repo_map=2,list=0,trace=0,source_lens=2 | midloop=3,inv=1/0,fin_reject=1,unavail=0,prune=0 | pass | B516 生产正证：最终答案一致列出 2 个 extend、2 个 foreign func 与 8 个 public class，且明确 8 个 class 包含 public sealed/public abstract，不再出现“标题排除、typed roster 又强制列出”的三面冲突。唯一 Finalizer reject 是模型首稿 table 声明 4 列但每行只给 label+text；既有 JSON row-width 教学精确指出结构，下一轮即修正，未进入草稿恢复或降级，归为正常模型波动。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260811-031148 | answer_regex,answer_contains | none | 233s | 38 | read=14,repo_map=3,list=0,trace=0,source_lens=0 | midloop=4,inv=2/0,fin_reject=1,unavail=0,prune=1 | fail | 四阶段表与 5 条 call + 3 条 precedence 的 exact typed 关系大体正确，且修补后保住两个不连通分量，没有伪造跨分量桥；但最终图仍以 `calls`、`precedence`、Go 函数名和源码行号作为主要可见文案，业务含义很弱。初始 prompt 已有业务显示指导，局部 relation-boundary repair 却要求 exact identity 为第一可见标签并紧贴重复 relation/location，形成确定性教学自冲突；另有 3 个 item citation_ref 被安全剥离后正文断言仍保留，形成独立 citation-detach 观察项。runner PASS 不能覆盖这两项人工质量失败。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
