# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T05:43:50Z
- sweep_start_ts: 20260817-224350
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260817-224350 | answer_regex | none | 184s | 29 | read=1,repo_map=2,list=0,trace=0,source_lens=1 | midloop=5,inv=1/0,fin_reject=3,unavail=0,prune=0 | fail | B1045/B1046 正向生效：不再发生“删锚后又必带锚”的合同循环，最终维度补充也不再泄漏 role 枚举；184s 相比上一轮 356s 明显下降。三次拒绝分别是确实缺锚、注册锚端点错误、可选图含未证边，均为精确信号。但最终答案仍有三项系统级问题：① 模型原选 `FastTokenizer._tokenize_slow` 定义行 24，系统机械改成原生调用点行 21，形成错引；② 探索证据只覆盖 `_HAVE_NATIVE=True`，没有把 `except ImportError` 与 `_HAVE_NATIVE=False` 作为分支状态生产者闭合，模型遂错误断言“未显式捕获、标志不会更新”，与源码及答案前文矛盾；③ 精确 `_fastlex.tokenize_bytes -> py.tokenize_bytes` 注册关系通过隐藏 edge_anchors 验证，但模型删除可选图后，最终可见列表没有明确显示这条关系。runner PASS 只命中浅层 answer_regex，人工判失败。 |
| 2 | github_issue_zod_prefault | FAIL | eval/results/github_issue_zod_prefault-20260817-224350 | write_apply,answer_regex | none | 241s | 25 | read=4,repo_map=1,list=4,trace=0,source_lens=0 | midloop=2,inv=0/0,fin_reject=0,unavail=1,prune=0 | pass | 代码修改正确覆盖 `_prefault=false/0/\"\"`，项目 `make check` 通过；但该命令只运行静态源码形状校验，changed-path capability 因而是 `source_static`。controller 将模型的 `all_verified` 精确收窄为 `accept_unverified / production_verification_source_static_only`，最终中文明确说明“未完全验证、不是已确认代码失败”。runner FAIL 是预期 verifier/oracle 边界，不是产品失败，不应为了 eval 变绿而放宽生产验证合同。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Confirmed generalized gaps

1. `B1047-CITATIONDEFINITIONDOWNGRADE1 / P0-redline`：标签引用机械归一化会把模型已经选择的唯一精确定义引用降级为同名调用点。最优形是单调保护：typed evidence 能唯一证明当前引用就是该标签定义时，任何较弱的标签/上下文匹配不得覆盖。
2. `B1048-REQUESTEDBRANCHREACHABILITY1 / P1-high`：用户显式要求某分支行为时，当前闭包只保证目标 callable/效果，不保证 guard、被选效果以及 guard 状态生产者/异常处理器同时有证据。应在 typed requested-dimension/evidence 上构造通用分支可达性闭包，不按 Python、fallback 或原始问题关键词拟合。
3. `B1049-STANDALONERELATIONVISIBILITY1 / P1-high`：非 diagram 主载体可用隐藏 `edge_anchors` 通过精确关系门，但删除可选图后关系可能不再出现在用户可见结构里。应让模型在结构化关系载体中同时提供可见关系表面与 typed identity，renderer 只机械呈现模型字段；禁止系统从隐藏 identity 自行代写关系结论。
4. `WRITE-STATIC-VERIFY / not-a-gap`：静态测试不能签发生产验证是诚实边界；此次不修改。
