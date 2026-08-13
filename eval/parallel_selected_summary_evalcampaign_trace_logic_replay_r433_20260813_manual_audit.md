# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T11:46:54Z
- sweep_start_ts: 20260813-044652
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260813-044654 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 219s | 38 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 四次 typed 查询均保持显式窗；链上-only 主因、running 65.912ms、D-state 36.757ms、反转/调度/算力/IO、业务线索及实际占用/规则可消双轴保留；邻近与背景仅作支持。零成文拒绝、零降级。 |
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260813-044655 | answer_regex,answer_contains,mermaid_edge_count | none | 448s | 40 | read=11,repo_map=2,list=0,trace=0,source_lens=0 | midloop=6,inv=4/0,fin_reject=5,unavail=0,prune=1 | fail | B721 schema parity 生效：5 个 patch 都保留 diagram/edge_anchors/from_identity/to_identity/participant_boundaries，且最终保留 7 条局部 typed 边，没有再因 patch schema 缩水删边。但模型持续发出非法 `-..-` 操作符；共享 normalize 将其错误别名化为 `codraxNode1[\"-..-\"]`，最终 PASS 仍出厂非法 Mermaid。新立 B722。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工结论

- B721 的“full/patch 块 schema 漂移”已获生产正证：从第一轮到第五轮 patch，结构字段均完整，最终 7 条 typed 局部关系仍在，不再出现 r432 的删边闭环。5 次拒绝来自参与者 requested-scope 边界修形，成本仍高，继续作为图合同效率债观察，不用答案词面硬门处理。
- B722 是独立 P0 出厂语法 GAP：Mermaid 合法 dotted line 是 `-.-`，模型写成 `-..-`；旧 unsafe-node normalizer 不认识该操作符，把它变成 `PreStages codraxNode1[\"-..-\"]|条件触发| MainPipeline`。结构/evidence 合同只解析关系边，没有在 normalized final source 上做浏览器语法验收，所以 runner PASS 掩盖了不可渲染图。
- 根修不是匹配节点名或该题文本：在共享 Mermaid 兼容层、且在 unsafe endpoint alias 前，quote/shape-aware 地把重复点号操作符 `-..-/-..->` 归一为 `-.-/-.->`；规范化、ParseEdges、unsafe alias 共用 dotted operator 认识。降级 last-mile 对安全修复后的源码重新 dry-run，成功即发布修复后的 Mermaid，失败才转 text fence。标签内相同字节保持不变。
- r433 的 Trace 用例为负向保护：显式时间窗、Trace 因果投影与自动补齐未回归；根因只在 typed 链上选举，邻近/背景没有晋升。活跃输出 219s/448s 均未因 4ms 总年龄降级；终止权仍仅属于 cancel/deadline、首字节/byte stall、transport/decode/retry exhaustion。
