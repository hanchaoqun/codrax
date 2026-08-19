# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T17:24:09Z
- sweep_start_ts: 20260819-102407
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_tokenizers_newline_run_multirepo_py | FAIL | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260819-102409 | log_regex,write_apply,answer_regex,answer_contains | none | 602s | 26 | read=7,repo_map=3,list=0,trace=0,source_lens=2 | midloop=2,inv=0/0,fin_reject=0,unavail=3,prune=0 | fail（诚实未洗绿） | B1174 第一层生产正证：controller 接管普通 append，形成 `changes=[] + verification_probes[] + DependsOn` 的纯补证批，没有再制造无意义源码 patch；第二层也正确拒绝用 2/5 通过洗绿。新 B1176：该 planner batch 被铸成 `NeedsCodeExploration=false/ready_to_plan`，`read_file` 确定性不可用，且 planner context 没携带上一代两个已通过 probe 的真实构造器/方法形状；模型猜出 `FastTokenizer()`、模块级 `_tokenize_slow` 和 `inspect.getsource` 字符串检查，3 个 probe 真实失败。根修是给 proof materialization 一次 bounded exact-path 源码探索，并把 typed prior passing probe/API receipt 作为模板上下文；不降低 exact proof ledger。 |
| 1 | qf_logic_view_read_pipeline | TIMEOUT | eval/results/qf_logic_view_read_pipeline-20260819-102409 | answer_regex,answer_contains,mermaid_edge_count | none | 1200s | 47 | read=24,repo_map=4,list=0,trace=0,source_lens=0 | midloop=18,inv=10/0,fin_reject=6,unavail=5,prune=0 | fail（无终稿） | B1171 快路径扩展生效但未闭环：证据已推进到 `o.busCtx -> BuildAgentContext -> ac -> ExtractStageHasRequiredWork`，不再只停 builder；随后关系完成/成文合同发生优先级反转。typed evidence scope 已有连接候选、模型图仍分岛时，per-participant `available_typed_incident_edge_not_rendered` 先占 mismatch，使 ComponentSplit join 候选不发布，只给 `Mutable -> len/Emitted*` 局部边；错误正文允许“局部边+未证边界”共存，repair_action 却要求 `omit_unproven_boundary`。模型六次 patch 在局部边、边界和精确 endpoint 间循环，最终超时。B1175 应让可执行 join frontier 优先于局部 incident 修复，并统一 boundary action；系统仍不画边。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工结论与下一批

1. `B1171` 的 parser-owned continuation quality 已跨早期 fast path 生效：本轮首次得到调用方完整值传递行，且 read closure 未冒充 relation evidence。它仍只能记 production-partial，因为最终图被下游参与者修复合同拖死。
2. 新 P0/P1 `B1175-DIAGRAMJOINCANDIDATEPRIORITY1`：当 evidence participant graph 已完整而 reader-visible graph 分裂时，先发布能跨当前可见 component 的 typed join candidate；不能因同一参与者同时有 per-participant mismatch 而屏蔽 ComponentSplit。局部 incident candidate 只能作备选，不能代替未证的请求关系，也不能与 repair_action 的 boundary 指令互相冲突。模型选择、业务词、布局、边和结论继续由模型负责。
3. 新 P1 `B1176-PROOFFOLLOWUPSOURCECONTEXT1`：proof-only probe planner 需要精确 API 上下文。最优方案是 controller-owned exact `ExpectedPaths` 的一次 bounded read/explore，加上上一代计划中已执行通过的 probe 代码/working_dir/changed_symbol_refs 作为只读模板。planner 仍须自行决定哪些 missing contract 真被新断言覆盖；系统不扩写 contract_ref，不把源码 token 扫描当行为证明。
4. B1174 没有回归：后继只在 exact ledger 全绿时才结清直接 proof 前驱，本轮坏探针被如实报告 `parser_error/unverified`。这次失败不能通过放宽 proof ledger 或把 project suite 一次通过 blanket-sign 全部行为合同来修。
5. QF 首次 `blocks` string carrier 被安全恢复，后续一次 patch string carrier 因内层 JSON 畸形 fail-closed；这是模型/JSON 教学观察项，不是本轮超时主因。活跃流全程没有 4ms/固定 age 降级，超时来自外层 eval 1200s 上限。
6. 本轮无 Trace case，施工继续以专项回归钉住显式窗因果投影、自动补齐、链上主因、实际占用/业务线索与规则计价可消除量双轴；邻近/背景不得升主因。
