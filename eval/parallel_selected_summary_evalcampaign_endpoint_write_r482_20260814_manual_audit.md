# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T11:24:12Z
- sweep_start_ts: 20260814-042411
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_go_typo | PASS | eval/results/patch_go_typo-20260814-042413 | write_apply,write_patch_oracle,answer_contains | none | 91s | 24 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 只把 main.go 第 25 行 `retrun` 改为 `return`，其余字节不变；计划、apply、verify、finish 全链闭环，无 replan。verify 恰好执行一次 `go test -json ./...`，main.go 获 target_behavior/project_runner 覆盖，1/1 test 通过，终态 verified，累计终验域非空。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260814-042413 | answer_regex,answer_contains | none | 214s | 29 | read=4,repo_map=0,list=0,trace=0,source_lens=0 | midloop=7,inv=4/0,fin_reject=1,unavail=0,prune=0 | partial | B787 已生效：typed 状态为 shared_callee_boundary，facet/机制载体/First-Pass 图只给 `buildAnalysisIR -> gate.RunWith` 与 `gate.Run -> gate.RunWith` 两条真实边，最终图也精确保留双入边。残余是 ordered_list 的块级 `principal_path_edge` 仍包住 19 个同 caller 本地调用；系统只验 facet 存在，未验每个 citation 是否属于两条主边。登记 B789 结构化 facet-candidate 所有权断层。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human audit conclusion

- Runner：`2 PASS / 2`；人工：`1 pass / 1 partial`。
- 写模式正证：变更面精确到一个 token、一行、一个文件；apply 后行为验证覆盖 changed path，验证域没有被 stamp/replan 清空，且没有重复测试或重复补丁。
- Sequence 证明 B787 的统一 typed 投影已在生产生效，图关系与端点边界正确；同时暴露 `B789-PRINCIPALFACETCANDIDATEOWNERSHIP1/P1`：块级 facet 可以借一条合法主边，为同块其余支持 citation 冒充 principal intermediate hop 授权。
- B789 只消费 typed no-directed-path disposition、endpoint capsule EvidenceID、结构化 facet 与 citation_ref。主关系块只允许 endpoint-boundary 候选；其他真实本地调用可留在独立支持块。系统不删除模型内容、不扫描答案文字、不代写结论或图。
- 本轮无 Trace 路径改动；显式时间窗、因果投影、自动补齐、链上-only 主因和双耗时轴保持。活跃连接没有 4ms/4m/固定累计年龄降级。
