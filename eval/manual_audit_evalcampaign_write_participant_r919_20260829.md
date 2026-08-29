# r919 人工审计：写恢复与图关系显示身份

- 基线：`main@d802a1354`
- 并发：严格 2 路，`PARALLEL=2`
- runner：2/2 PASS；write 181s，read 310s
- 汇总：`eval/parallel_selected_summary_evalcampaign_write_participant_r919_20260829.md`

## write：人工 PASS，但 B1431 未触发生产正证

`github_issue_dateutil_relativedelta_float_symptom` 正确定位 root-level `relativedelta.py`，生成并安装单文件 patch 计划，应用后执行独立行为 probe 与完整 unittest；5 项验证全绿，最终应用树只修改目标实现，未修改测试期望。

本轮 planner 先通过 repo map 得到真实路径，后续 6 次读取均成功，没有产生“不存在路径读取”。因此 B1431 的“失败读取不消耗成功内容预算”和唯一同名 relocation candidate 只得到无回归证据，不能记为生产转正。

## read：人工 PARTIAL

`qf_logic_view_read_pipeline` 最终 Markdown 与 Mermaid 语法合法；四阶段 precedence、BusContext/Mutable 的已证局部数据流及组件责任均保留。没有字面 `\\n\\n` 泄漏，活动流也未按固定 4ms/4m 或上下文比例降级。

本轮没有出现 `boundary_participant_not_visible`，因此 B1432 的 `participant_ref + ensure_visible` 也只得到无回归证据。B1433 的输入没有携带双转义段落符，同样仍待生产触发。

首稿校验一次联合披露 participant 与 relation delta；模型在同一原子 patch 中完成关系删除、重命名和 addition，随后仅因其自行增加的 stale boundaries 再修一次，最终 2 次 reject 后通过。联合修补能力本身正常。

## 新确认 GAP

### B1434-IMPLICITENDPOINTDISPLAYIDENTITY1（P1）

关系 addition 允许模型选择一个原图未显式声明的端点，但当前 schema 只要求 `edge.from_node/to_node/visible_label`，没有能力让模型在同一操作中给新端点写可见名称。executor 直接追加边，Mermaid 因而隐式创建节点。

生产结果把 `ctxbuilderBuildAgentContext_860bba75bb1a60fb` 直接显示在图里。该值是系统为技术身份生成的稳定 node id，不是用户可理解的组件名称；关系本身有 typed 证据，但图的显示身份合同不完整。

根修冻结：

1. 对 add/replace 引入的每个“当前图中尚无显式声明”的端点，要求模型同时提供该端点的 reader-facing visible label；已显式声明端点继续复用原声明。
2. executor 只把模型选择的 node id 与模型文字编码为对应 Mermaid family 的独立声明，然后追加/替换模型选择的边；不得从技术 identity、请求、thinking、edge message 或答案 prose 自动造名称。
3. flowchart/graph、sequenceDiagram、classDiagram 使用各自语法适配器并做 parser round-trip；不支持的 family fail-closed，不输出隐式内部端点。
4. from/to 相同端点只允许一个一致声明；重复、歧义、unsafe id、空/多行 label、与已有声明冲突均 fail-closed。
5. 新 pin 覆盖：r919 flow 新端点、sequence 新 participant、class 新 class、既有声明无需重写、缺 label 拒绝、边/anchor/关系不变、系统不生成可见名称。

## 观察项

read 在 15 个 explorer iteration 内调用 5 次 `emit_investigation_complete`：前三次依次补 extract_work、builder.Mutable 与 `ac` consumer，第四次一次披露 9 个 member support_ref 错误，第五次完成。前三步具有新证据依赖，本轮先作为串行补证成本观察，不按单例直接立硬门或固定轮数截断。

## 状态

`r919=runner-pass-2/2,human-write-pass+read-partial`；
`B1431=no-regression/no-production-failed-read-trigger`；
`B1432=no-regression/no-production-visibility-trigger`；
`B1433=no-regression/no-production-escape-trigger`；
`B1434=confirmed/P1-next`；
`system-relation/action/node-id/visible-label/conclusion-selection=none`；
`request/model/final-prose/mermaid-message-fact-scan=none`；
`Trace explicit-window/causal projection/auto-supplement=unchanged`；
Trace root=`typed-on-chain-only`；adjacent/background=`support-only`；
`active-stream-4ms-or-4m-degrade=forbidden/production-positive`。
