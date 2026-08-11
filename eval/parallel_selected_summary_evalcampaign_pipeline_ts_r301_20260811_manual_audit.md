# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T08:01:33Z
- sweep_start_ts: 20260811-010131
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_ts_workspace_chain | PASS | eval/results/sr_ts_workspace_chain-20260811-010133 | answer_regex,answer_contains | none | 195s | 24 | read=6,repo_map=2,list=0,trace=0,source_lens=0 | midloop=3,inv=2/0,fin_reject=1,unavail=0,prune=0 | pass | `run -> ApiClient.fetchUser -> HttpTransport.send -> dispatchOnce -> fetch` 六条 exact call 边完整；`maxAttempts/nextDelay/sleep` 重试语义和 `@app/core` 的 tsconfig 定义、import 消费均有源码锚。A4 对异构 TS 调用链获得生产正证。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260811-010133 | answer_regex,answer_contains | none | 375s | 38 | read=11,repo_map=4,list=0,trace=0,source_lens=0 | midloop=7,inv=3/0,fin_reject=5,unavail=0,prune=1 | fail | 第一稿声明 `columns=[Stage,输入,输出,主要状态载体]` 但 `items=[]`；required-block 只计 kind，空壳表被当作完成，渲染时静默消失。五次图修复后只剩两条内部 reset call，四阶段时序、输入输出交接和用户要求的表均缺失；后三次 retry 围绕 `BusContext` 的 disconnected node/boundary row 往返。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human audit conclusion

- Runner：2/2；人工：1/2。浅表 `answer_contains/regex` 没有识别 required table 的零行载体，也没有验证 sequence 的业务时序关系是否仍在。
- `B510-A4` 可关闭生产验收：TS 关系链完整；pipeline 的 call mismatch 也已进入 typed repair，不再显示 actionable target=none。
- 新确认 `B510-E-EMPTYTABLE1/P1-high`：`has_per_member_table=true` 且四个 requested dimensions 均 required 时，只有列头、没有 markdown table、没有任何结构化 row 的 `kind=table` 仍通过载体/required-block 校验。renderer 对零 row 返回空串，因此用户明确要求的整张表静默丢失。最优修复是 JSON 载体级 fail-closed：模型一旦选择 table carrier，必须至少有一条可见 row 或一张有效 markdown table；系统不扫描请求/答案文本，不填值，不改结论。
- `B510-F-BOUNDARYREPAIR1/P2`：精确 participant boundary 合同本身没有矛盾，但当前修复说明要求模型同时维持“节点可见 + unproven row”，模型连续三轮在二者间摆动。应把每个缺口投影成单一 copy-ready recipe（exact participant id、可见 node 声明、boundary JSON），仍由模型 patch，系统不补节点、不造边。
- `B510-D-BUSINESSLAYER/P1` 继续开放：pipeline 图只剩内部函数/状态名，对用户缺少“请求分析、证据探索、证据提炼、答案组织”等业务角色/动作。正确形是业务 subgraph/显示 label 与组内 exact endpoint 双层表达；业务 label 永远不是 relation authority。
- Trace intent、root-cause Trace family、显式窗、因果投影、自动补齐和链上根因合同在本轮均未触发、未改动。
