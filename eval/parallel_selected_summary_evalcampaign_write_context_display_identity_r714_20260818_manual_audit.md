# r714 人工审计：写合同权威与参与者显示身份

- 日期：2026-08-18
- 基线：`main@660d47f67`
- 并发：严格恰好 2；同一不可变 `./codrax`
- 案例：`github_issue_tokenizers_newline_run_multirepo_py`、`qf_logic_view_read_pipeline`
- Runner：`2 PASS / 2`
- 人工：写案例 `PASS`；读模式架构图 `PARTIAL`

| 案例 | 人工判定 | 关键结论 |
|---|---|---|
| `github_issue_tokenizers_newline_run_multirepo_py` | PASS | 三条模型自拟精确边界均被标为 `planning_only`，未进入 hard verifier；controller 完整执行 plan/apply/verify/finish，补丁只改 `fastlex/tokenizer.py`，`make check` 两测全绿。 |
| `qf_logic_view_read_pipeline` | PARTIAL | 终稿保留 analyzer→explorer→extractor→finalizer 及 BusContext/Mutable 的 typed 关系，`Mutable` 显示身份不再丢失；但经历 4 次成文拒绝、5 次 finalizer 尝试、87,407 tokens/44% 上下文，说明首轮关系作者上下文仍不够精确。 |

## 写模式审计

1. Write Analyzer 首轮曾发出不合法的 `::` 测试 target，重试后正常；这不是最终合同逃逸。
2. 模型自行提出的“奇数连续换行”等三个精确合同均携
   `authority=planning_only`，controller/planner 也明确显示 `planning_only=true`。它们仍可作为计划设想，
   但不进入 required/hard contract IDs，验证器没有拿模型自拟样例反向确权。
3. 最终补丁按连续换行 run 统一处理：run>1 使用已注册 merge rank，单换行保留 literal；既有五换行回归
   与普通 merge 测试均通过。没有重复写、越界文件或结果伪绿。
4. B1131 的旧 `missing_task_input` 未复现，controller 从计划到 finish 的 typed 任务与 workflow 状态完整。
   本轮首轮 controller 直接发工具，因此“不发工具后重试”专门分支仍以单测为主，不把本次误记为生产正臂。

## 读模式/图关系审计

1. 终稿图合法，保留 4 阶段 precedence，并显示：
   `BusContext -> BuildAgentContext`（argument flow）与
   `BuildAgentContext -> Mutable`（call，技术端点仍保存在 edge anchor）。B1133 获生产正证：用户明确要求的
   `Mutable` 没再被归一成 `MutableState` 后从图中消失。
2. 需要纠正一个初步判断：首轮 Diagram Contract 没有要求 BusContext/Mutable 的 unproven boundary；
   `typed_named_participant_relation_coverage` 已将两者列为 incident。边界是模型在第一稿额外发出的。
3. 真正的 B1134 是提示/校验信息时序 gap：首轮只发布通用 typed relation capsule，未把其中关系映射为
   “哪一侧可用哪个指定参与者 node id、应使用哪个业务箭头标签”的参与者级候选。模型因此自行把 busCtx
   连到 Extractor/Finalizer、把 Analyzer 连到 Mutable，并同时加了边界；硬校验正确拒绝。模型删边后，系统
   才在失败提示里第一次发布 `typed_candidate[BusContext]` / `typed_candidate[Mutable]`，随后仍因端点映射
   反复失败。相同 typed 事实首轮已有、精确映射却只在拒绝后出现，造成 4 次拒绝和明显 token/时延浪费。
4. 最优修向不是放松 hard gate，也不是系统代画图：让首轮作者上下文与 hard gate 复用同一 typed candidate
   解析器，前置发布有界、参与者感知的可选 recipe。它只列既有候选、允许模型择一/不用，不添加可见边、
   不选择结论；无候选参与者仍保留诚实 boundary。不得读取用户或答案原文作硬门。
5. 终稿箭头仍显示 raw `call`，且正文末尾有偏内部化的“系统补充”块，记为 P2 展示债；先通过业务化
   `visible_arrow_label` 的首轮 recipe 降低发生率，不增加答案关键词删除门或系统重写。

## 不变量复核

- 本批未改 Trace 查询、显式时间窗、因果投影或自动补齐。
- Trace 主因仍只可来自 typed on-chain 证据；邻近/背景只能作为支持与额外排查方向。
- 优先级反转、调度供给、算力供给、D 状态、IO 等待、确定性语义开销与链上业务线索的载体未变。
- 活跃字节流跨过 4ms 仍不降级；终止/恢复只由 caller deadline/cancel、首字节或 byte-stall、传输/解码失败控制。
- 系统只提供 typed 事实和结构校验，不代替模型画关系图、写结论或改答案主张。

状态：

`B1131-WRITECONTROLLERRETRYCONTEXT1=production-no-regression/direct-retry-arm-unit-pinned`；
`B1132-WRITECONTRACTAUTHORITY1=production-positive-r714`；
`B1133-DIAGRAMPARTICIPANTDISPLAYIDENTITY1=production-positive-r714`；
`B1134-DIAGRAMPARTICIPANTCANDIDATEFIRSTPASS1=confirmed/P1/next`；
`active-stream-4ms-degrade=forbidden/not-observed`；
`Trace explicit-window/causal projection/auto-supplement=unchanged`；
`system-answer/conclusion-authorship=none`。
