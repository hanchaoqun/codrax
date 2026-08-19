# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T23:37:03Z
- sweep_start_ts: 20260819-163702
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_tokenizers_newline_run_multirepo_py | FAIL | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260819-163703 | log_regex,write_apply,answer_regex,answer_contains | none | 248s | 26 | read=5,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | partial | 实现与 exact unittest 均正确；required outcome refs 未绑定，proof 批 ready_to_plan 后被模型 finish 跳过，终态诚实 unverified |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260819-163703 | answer_regex,answer_contains,mermaid_edge_count | none | 391s | 36 | read=9,repo_map=3,list=0,trace=0,source_lens=0 | midloop=10,inv=7/0,fin_reject=2,unavail=1,prune=0 | partial | B1197 禁止单 tuple 克隆生效；终图只剩三条阶段顺序与一条 BusContext 局部参数边，Mutable/各阶段共享状态流仍未证 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

### qf_logic_view_read_pipeline

- B1197 获得生产正证。r746 曾把同一
  `argument_flow / o.busCtx -> ctxbuilder.BuildAgentContext` 复制到四个不同可见 endpoint pair；
  r747 的两次成文拒绝后，终稿只保留一条该 tuple 对应的
  `BusContext -> ctxbuilder.BuildAgentContext` 边。系统没有选边、删边、造边或代写图。
- Runner PASS 仍是弱 oracle。用户要求的核心是四阶段与 `BusContext/Mutable` 之间的数据流；
  终图仅有 `Analyzer -> Explorer -> Extractor -> Finalizer` 三条 ordering 和上述一条局部参数边，
  `Mutable` 以 unproven boundary 呈现。这是诚实 partial，不应为让 case 变绿而补画。
- Explorer 用 20 轮才完成，多次将定义/局部读取误叙为阶段间写入，说明上游
  relation recipe 和 completion grounding 仍有 P1 缺口；后续应补 parser-owned 真实读写/参数边，不放宽发射门。

### github_issue_tokenizers_newline_run_multirepo_py

- B1194/B1195 获得生产正证。Planner 正确发出
  `assertion_suite=TokenizerTest` 和 framework method ids；pytest 缺失后候选替换保留
  `suite="tests/test_tokenizer.py"`，实际执行
  `python3 -m unittest "tests/test_tokenizer.py" -v`，两条断言都产生 assertion-level 通过回执，没有退化为 broad discover。
- 补丁实现正确，`make check` 与 exact unittest 均通过，changed production path 为
  `target_behavior/covered`。Runner FAIL 不是代码失败。
- 新确认 B1198：两个 project-test observation 只引用了 `planning_only` 的 `c1..c4`，
  没引用任一会关闭证明义务的 `soft_required outcome-1..4`。这不能靠系统语义猜测自动合并。
  施工为 typed soft guidance：单独列出可关闭 verified 的 required ids 和仅作规划上下文的 ids；
  允许部分映射，但明确未映射 id 仍为 unverified，不新增一次必须全覆盖的硬门。
- 新确认 B1199：系统已根据未闭合 typed ledger 创建 proof-probe batch，且完成精确路径探索后
  批状态为 `ready_to_plan`、`plan_id` 为空；模型却被普通测试绿误导而 `finish/all_verified`。
  旧状态机接受 finish，最后只能由总账改成 `missing_terminal_verify_verdict`。根修只读 controller-owned
  batch purpose/status/empty plan id 与 exact `verification_probe_required=true` criterion，将该 finish 收窄为
  `plan_batch`；不从模型理由、命令、源码或答案文本确权。
- B1196 本轮没有走到第二份 proof-only plan，因此保持实现+单测，不虚报生产关闭。

## Cross-cutting invariants

- r747 两案没有 JSON 畸形恢复、空答案或系统代写结论。读案持续 391s 且没有按总时长降级；
  active byte stream 不得因 4ms/4m/fixed age 降级的合同保持。
- 本批不修改 Trace 查询、显式时间窗、因果投影或自动补齐。Trace 主因仍只能来自 typed
  on-chain 证据，邻近/背景仅支撑额外排查；真实占用/业务线索与规则计价可消除量双轴未改。
