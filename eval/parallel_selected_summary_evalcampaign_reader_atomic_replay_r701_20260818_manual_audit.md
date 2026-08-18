# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T19:58:03Z
- sweep_start_ts: 20260818-125801
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260818-125803 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 246s | 39 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | 核心因果/排序正确；B1105a 可见索引生效，但模型正文及另一系统补充块仍泄漏内部字段词。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260818-125803 | answer_regex,answer_contains | none | 265s | 29 | read=5,repo_map=0,list=0,trace=0,source_lens=0 | midloop=10,inv=4/0,fin_reject=1,unavail=0,prune=0 | partial | 最终 shared-callee 关系正确；B1104 原子载体未进入真实 finalizer 提示，首稿漏边界后被正确拒绝。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工审计结论

### 1. Trace IO 唤醒链

- 精确窗口保持为 `2.000000..2.020000s`。目标 `app-100` 在窗内 S 态 20ms，running/runnable/D/IO
  均为 0；答案没有把这个症状时长冒充可消除根因。
- typed 链完整保留为 `threadpool-400 -> network-300 -> cookie-200 -> app-100`。链上第 1 位是
  `threadpool-400` 的 fscache IO 等待 11ms；后续三席各 1ms runnable，作为调度供给方向。邻近
  sleep 和背景 IO pressure 没有晋升主因。
- `Trace 因果投影`、系统确定性补采、实际占用与规则可消除双轴均在；没有因 4ms 内尚未形成完整
  answer 而降级，也没有系统替换模型结论。
- B1105a 生效：系统证据索引已改成「因果位置：链上」「根因排序第 N 位」「置信度：高」等读者
  语言，不再显示 `tier/causality/predicate/origin/member/same_value` 原字段。
- 仍有两条独立展示债。模型正文复制了 `resource_identity_authority=not_provided`、
  `holder_identity_authority=not_provided`、`sleep_cause_authority=not_provided`、`on_wakeup_chain`、
  `caliber` 等内部词；不能靠扫描/改写最终 prose 解决。系统生成的「系统补充：trace_query 关键观测
  核对」仍直接显示 `source/causality/chain_depth/recommended_views`，属于确定可修的第二系统面。
- 模型结构字段把 `trace_causal_claim_caliber` 选成 `no_causal_conclusion`，而它自己的可见正文又排序并
  选择了链上根因。这是模型 structured-decision 自相矛盾；当前系统不得反向扫描正文替模型改 enum。
  后续只可在模型成文前把 closed typed 选项及影响讲得更近、更省心智，或基于结构字段间的精确矛盾
  做 fail-loud，不能从文字推断结论。

### 2. QFCallChain 关系图

- 最终关系事实正确：`buildAnalysisIR -> gate.RunWith <- gate.Run`，七个关键中间函数和源码引用保留，
  Mermaid 语法/方向正确。
- 首稿只有两条已证 call 边，没有 `buildAnalysisIR`、`gate.Run` 的请求级 unproven boundary；硬门
  正确拒绝，后续两次 patch 补回 boundary 与 member-set carrier。最终仍把「未证关系边界」显示给
  客户，属于模型没有遵循 display 指引，不授权系统删除。
- B1104 的 focused test 与真实链路不一致。日志的 Finalizer 提示包含 copy-ready body 和
  `edge_anchors_json`，也包含 `typed_named_participant_relation_coverage`，但完全没有
  `diagram_block_sibling_fields_json`。根因是前者从 RequestModel/semantic view 取参与者，原子载体却
  从 `AnswerSurfacePlan.Diagram` 取；真实计划只保留展示形，`DiagramContract.Participants` 为空，导致
  第二消费者把同一 typed 义务误判为空。这不是模型波动。
- B1106 将边界载体的参与者来源收敛到 hard gate 使用的
  `AnswerSemanticView.DiagramParticipantObligations`；该 slate 已经过 `SourceQuote` 来源校验。展示合同
  参与者只作兼容测试回退，不能覆盖生产 semantic view。系统仍不造节点、边、文字或结论。

## 判定与后续顺序

1. `B1106-DIAGRAMPARTICIPANTSLATESINGLEOWNER1`：P0/P1，立即施工并提交；真实生产形 pin 必须令
   AnswerContract 图参与者为空时仍一次发布 anchors + boundaries。
2. `B1105b-TRACESYSTEMAPPENDIXREADERPROJECTION1`：P1，单独施工；把系统补充块改成 typed→reader
   投影或转入诊断面，保留全部 raw 诊断，不扫描模型正文。
3. `B1105c-TRACEMODELCOPYMINDLOAD1`：P2，先做上下文位置/重复度审计与异构回放；若只证实模型波动，
   暂留，不做关键词硬门。

状态：runner=`2/2 PASS`；manual=`2 partial`；
`Trace root=typed-on-chain-only; adjacent/background=support-only`；
`Trace explicit-window/query/projection/auto-supplement=unchanged`；
`active-stream-fixed-4ms-degrade=forbidden/not-observed`；
`system-answer/relation/diagram/conclusion-authorship=none`。
