# Selected Eval Manual Audit Scaffold

- date: 2026-08-29T07:35:05Z
- sweep_start_ts: 20260829-003503
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260829-003505 | answer_regex,answer_contains | none | 223s | 29 | read=6,repo_map=1,list=1,trace=0,source_lens=0 | midloop=5,inv=3/0,fin_reject=1,unavail=0,prune=0 | pass | `Logger::log -> Sink::write` 与 `ConsoleSink::write -> fputs/fputc` 两段实证链、虚分派边界、`SinkRegistry::create` 三种后端选择均准确；明确披露当前源码没有证明 factory 返回值到 Logger 构造调用者的完整绑定。唯一拒绝是两个列表声明 call_edge 但漏 edge_anchors，模型一次局部补齐通过；无系统代写或关系补造。 |
| 1 | sr_ts_workspace_chain | PASS | eval/results/sr_ts_workspace_chain-20260829-003505 | answer_regex,answer_contains | none | 263s | 32 | read=9,repo_map=3,list=0,trace=0,source_lens=0 | midloop=4,inv=2/0,fin_reject=4,unavail=0,prune=0 | pass | 最终准确给出 `run -> ApiClient.fetchUser -> HttpTransport.send -> dispatchOnce -> fetch`、retry guard/delay 与 `@app/core` 路径映射；可选图在修补中被模型删除，事实答案仍完整。B1443 已把同 case 从 r929 的 8 拒/393s 降到 4 拒/263s，但首轮同时缺必需 summary，local diagram lease 又关闭 add_blocks；随后模型同时选删整图与改图边，仍产生两次确定性合同拒绝，确认为 B1444。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Findings

1. 两路 runner 与人工答案均通过；无运行时降级、旧稿恢复、JSON salvage、系统结论替写或关系补造。
2. B1443 获得生产正证但只是 partial：混合图/非图 failure 不再退回 legacy `block_id/match/body_occurrence`，无关 block 删除也已进入精确权限；同一 TypeScript case 的拒绝数和耗时显著下降。
3. 新 P1 `B1444-MIXEDREPAIRREQUIREDADDITION1`：同一 typed validation generation 同时存在可选图局部关系失败与“缺少必需 summary”时，局部图 lease 无条件删除 `add_blocks`，执行器无条件拒绝任何 whole add。模型已经按必需块错误补 summary，却被权限合同拒绝；这不是关系证据不足或模型波动。
4. 第二个同根事务冲突：模型显式选择删除 typed optional diagram 时，还携带了该图的局部 edge edit。删除整个可选载体已经覆盖局部改边，但执行器把两者当冲突再拒一次。最优修复不是猜模型意图，而是仅在 typed `AllowTargetDiagramRemoval` 且模型明确列出 target remove 时，吸收同 target 上被删除载体的冗余原子操作。
5. B1444 已施工：schema 仅按 `AnswerSemanticView.RequiredBlocks` 与不可变拒绝草稿的实际 deficit 开放缺少的 kind/count；执行器要求每一个 add 严格降低 typed deficit，拒绝任何可选额外块。显式可选图删除仅吸收同 target 的冗余 edge/boundary/participant/scope edit，其他 block/diagram 操作不受影响。模型继续作者化 id、正文、证据绑定、关系、动作、节点、标签、布局与结论。
