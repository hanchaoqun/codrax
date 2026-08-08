# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T06:51:33Z
- sweep_start_ts: 20260807-235132
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_python_typo | PASS | eval/results/patch_python_typo-20260807-235133 | write_plan,write_patch_oracle | none | 92s | 21 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | PASS | plan-only 结果准确：仅改 `main.py` 的 `retrun -> return`，dry-build 与 acceptance test 合理，未触碰仓库文件。但 read analyzer 连续两次误发只适用于读查询的 `field_value_profile`，第三轮才接受；跨 Go/Python write 已形成重复信号，记 B328。 |
| 1 | qf_diagram_pipeline | PASS | eval/results/qf_diagram_pipeline-20260807-235133 | answer_regex,answer_contains | none | 130s | 25 | read=2,repo_map=2,list=0,trace=0,source_lens=0 | midloop=3,inv=1/0,fin_reject=1,unavail=0,prune=0 | PARTIAL | 最终四阶段、职责、引用与可见 Mermaid 均正确，`\\n` 也被 source repair 安全改为 `<br/>`。但合法 `A --> E --> X --> F` 只被 AST 识别最后一跳，前三个 typed precedence 锚中前两条被误报 metadata-only，模型被迫删掉全部 edge authority 才通过；另有 `diagram.body` fence / `subgraph` 的静态教学与 schema/renderer 相反。记 B329/B330。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

### `patch_python_typo`

- 最终 `run-1.plan.json` 只有一个 Python patch，目标、旧/新 token、验证命令和验收条件均与请求一致；plan mode 未应用代码。
- analyzer 第 1 轮把明确写计划误分成 `intent/question_kind=return_value`，并把 typo token 当成 source field-value lookup；第 1 次因 quote 不含 target 被拒，第 2 次因 `greet` 不是 owner-qualified field/member/config 被拒，第 3 次删除该 profile 后成功。
- 当前 analysis skill 已明确写着“edit/replacement/typo 的 before/after token 不是 field-value lookup”，仍未降低模型心智。该信号已在 Go 与 Python 异构 write 复现，不能继续只记模型波动。最优方向是让既有 structured write route 给 analyzer 提供模式范围，软抑制 read-only optional profiles；不得扫描用户原始关键词，也不能改变 read scheduler L1。

### `qf_diagram_pipeline`

- 模型首稿正确发出三条 `relation_kind=precedence`：`A -> E`、`E -> X`、`X -> F`，可见 body 使用 Mermaid 合法链式语法 `A --> E --> X --> F`。
- `mermaidcompat.ParseEdges` 的旧实现递归覆盖前一跳，最终只留下 `X -> F`。因此 validator 将前两条锚误判为 `typed_anchor_without_visible_edge`；patch 没有修图，而是删掉全部 typed edge metadata。用户看到的图没错，但机器可验证关系权威被静默降级。
- `mermaid_source_repair_applied=1` 本身是正向信号：节点标签中的 `\\n` 被窄修为 `<br/>`，没有改节点/边/结论。问题在证据 AST，而不是 repair。
- finalizer 静态教学一处要求 `diagram.body` 自带 Markdown fence，并宣称 `subgraph` 不受支持；同一轮 projected tool schema 又要求 raw body、renderer 已有 subgraph flatten shim。模型本次恰好服从 schema 而非静态旧教学，不能把偶然成功当作合同一致。

## Ruling

- runner：2/2 PASS；人工：1 PASS / 1 PARTIAL。
- `EVAL-B328-WRITEROUTEANALYZERSCOPE1=P1-filed`：异构 write 请求被 read-only optional profile 过度建模，增加 analyzer correction；先审计既有 typed route handoff，不做关键词 hard gate。
- `EVAL-B329-MERMAIDCHAINEDGEAST1=P1-confirmed`：合法 chained flow edge 必须拆成逐跳 AST，保留 hidden-metadata fail-closed 反例。
- `EVAL-B330-DIAGRAMBODYTEACHINGDRIFT1=P1-confirmed`：JSON/diagram 教学须与 projected schema 和 renderer 单源一致，structured body 不带 fence，subgraph 支持不得反向教学。
- 本批没有 Trace 输入。后续必须用显式时间窗 Trace 隔离回放确认：仅 `on_chain` typed 席位有根因/主因资格；adjacent/background 永远只能提供背景与额外排查方向，不能因邻近或数值大被加冕。
