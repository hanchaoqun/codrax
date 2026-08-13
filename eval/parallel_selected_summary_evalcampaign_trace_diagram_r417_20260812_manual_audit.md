# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T03:04:00Z
- sweep_start_ts: 20260812-200359
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_type_relation_loop_controller | FAIL | eval/results/qf_type_relation_loop_controller-20260812-200400 | answer_regex,answer_contains | none | 181s | 27 | read=13,repo_map=2,list=0,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | 模型首稿已给出 12 个 production implementer、文件位置和合法 classDiagram；但 UML `Base <\|.. Impl` 的语义方向是 Impl→Base，模型 sibling edge_anchors 却按字面左右顺序写 Base→Impl。载体转换后正文边与元数据相反，系统一次拒绝后又把明确要求的图当 optional 删除。Analyzer 的 typed requested dimension 已是 role=diagram/required=true，但 router 的 requires_diagram 波动为 false，且 predicate_axis=implement 被误配 call_dag。属于结构合同 GAP，不是知识/关系缺失或模型最终能力波动。 |
| 1 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260812-200400 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 229s | 40 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 34579.490..34579.500 窗保留；目标 running=1.156/runnable=.183/sleep=8.661/D=0/IO=0。NetworkService→CookieMonsterCl→目标链、T7→目标链及链上 priority inversion、VerifyClass 0.285ms、调度/算力供给席位均在；实际占用/业务排查与规则计价可消除量双轴齐全。邻近/背景没有晋升主因，missing wakeup 与 frame absent 边界诚实。无 4ms 活跃流降级。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Gap disposition

- `B698-CLASSUMLANCHORDIRECTION1`：模型已经在 UML body 中给出唯一、明确的有向 type relation；structured normalizer 应仅在 `classDiagram + type_relation + 唯一精确无序端点对` 时，将反向 sibling anchor 对齐到 ParseEdges 的 UML 语义方向。不得新增边、不得读取 prose/evidence 猜方向；重复边、调用/时序/流程边、单边 identity、大小写不一致均 fail-open。
- `B699-DIAGRAMAUTHORITYDUALCARRIER1`：通过当前请求 provenance 归一后仍存活的 `requested_answer_dimensions(role=diagram, required=true)` 是第二个 schema-valid typed 当前轮展示信号，可以授权“必须有图”，但不能铸造参与者、事实、边或结论。它补 router boolean 的模型波动，不扫描 raw request / final prose。
- `B700-IMPLEMENTVISUALFAMILY1`：`predicate_axis=implement + diagram_hint.kind=call_dag` 是 typed 自冲突。仅将这一精确组合归一为 architecture；不改变 sequence/flow 的显式展示语义。Turn-policy Mermaid 教学示例同时显式写 `requires_diagram=true`，减少 JSON 心智和自身错误示范。
- 活跃流边界不变：4ms 不是完成期限。只要有字节持续到达，禁止因 evaluator budget/请求年龄降级；仅 cancel/deadline、首字节超时、真实 byte stall、transport/decode terminal 有终止或有界恢复权。
