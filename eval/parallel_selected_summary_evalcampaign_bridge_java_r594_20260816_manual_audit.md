# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T23:43:50Z
- sweep_start_ts: 20260816-164348
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_java_call_chain | FAIL | eval/results/sr_java_call_chain-20260816-164350 | primary_answer | none | 118s | 26 | read=5,repo_map=2,list=0,trace=0,source_lens=0 | midloop=3,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | 五条普通 call anchors、容量 guard 与源码引用均完整，post-contract 零违例；但模型把已读取的 `AuditLog.record -> System.out.println` 写成“打印到 System.out 完成落库动作”，未明确说明它不是持久化审计存储。Finalizer 上下文已精确提示 storage/durability/completion unproven，因此判模型成文波动，不以答案关键词增加硬门。 |
| 1 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260816-164350 | answer_regex | none | 230s | 28 | read=3,repo_map=1,list=0,trace=0,source_lens=1 | midloop=5,inv=2/0,fin_reject=2,unavail=0,prune=0 | pass | B944/B945 生产闭环：principal 结构 receipt 保留 `_fastlex.tokenize_bytes --register--> py.tokenize_bytes --call--> tokenize_bytes`，普通 call/fallback anchors 均在；一次 Finalizer、post-contract strict=0，无跨阶段重写、无 diagram caveat。墙钟由 656s 降到 230s，拒绝由 9 降到 2。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

- B945 shared validation view 获生产正证；合法 semantic receipt 未被 post-contract 二次拒绝，collapsed-node 负臂由测试守住。
- Java 的普通 call chain 完整通过，证明 semantic-handoff 过滤没有扩域到普通 calls 或 Java 关系图层。
- Java 终稿错误发生在模型结论层。系统已提供 exact operation 与“持久化未证”边界；不扫描模型正文、不代写结论、不为单一术语组合建立硬拒绝。保留为模型波动，后续异构 terminal-semantics 回放再判断是否存在泛化软教学收益。
- Poly 仍有两次同轮结构修补（零锚与 exact requested label），但没有跨阶段矛盾或重试风暴；继续观察，不因本例降低结构证据杆。
