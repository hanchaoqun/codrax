# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T11:31:54Z
- sweep_start_ts: 20260817-043153
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_zod_prefault | FAIL | eval/results/github_issue_zod_prefault-20260817-043154 | write_apply,answer_regex | none | 192s | 25 | read=4,repo_map=3,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 两个补丁正确：truthiness 改为 `_prefault` 字段存在性判断，并新增 false/0/空串三条测试；未重复修改。Node 不可用，`make check` 只提供 Python/source-static 覆盖，因此 deterministic final 诚实降为 `unverified/proof_weak`，runner FAIL 不是代码失败。Controller 虽收到 `source_static 不是 behavior` 的精确上下文仍选择 `all_verified`，但后置 typed verdict 正确兜底；属既有模型服从观察，不降低验证杆。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260817-043154 | answer_regex,answer_contains,mermaid_edge_count | none | 371s | 41 | read=16,repo_map=4,list=0,trace=0,source_lens=1 | midloop=8,inv=6/0,fin_reject=1,unavail=1,prune=0 | partial | B968 的 read/extract 状态分离未触发：首次和第二次 exact repair 分别选中 `answer_document_pre_emit_check.go:5107/6071`，模型把校验器读取 Mutable 的局部事实误述为 extractor→finalizer 交接，随后又引用测试 mock。终图被 validator 正确收窄为三条 stage precedence，Mutable/BusContext 保留 unproven，但正文仍越界。真实根因是 repomap call 的 FromEP 为空；精确 enclosing method range 在图中却未进入导航 owner，故通用 carrier use 抢占。新立 B969，以 parser-owned enclosing callable 补 owner，不按文件名/语言/答案词面拟合。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
