# r1025 人工审计：日志来源边界与 Java 条件调用链

- date: 2026-09-07T01:58:41Z
- sweep_start_ts: 20260906-185838
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

基线 `main@1af9114112ce`，全部 Go/build 输入提交后重新 make；使用同一不可变二进制快照，严格并行 2 个 read case。机器 1/2 PASS；人工两份答案均未通过语义验收。没有改原答案、原日志或 oracle；格式/引用存在不等于机制与结论正确。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_java_call_chain | FAIL | eval/results/sr_java_call_chain-20260906-185841 | primary_answer | none | 153s | 44 | read=8,repo_map=1,list=0,trace=0,source_lens=0 | midloop=6,inv=3/0,fin_reject=2,unavail=0,prune=0 | fail | 调用边保留且可渲染；stdout 被称落库，图分支/校验位置不正确 |
| 1 | read_combo_log_current_code_boundary | PASS | eval/results/read_combo_log_current_code_boundary-20260906-185841 | log_attachment,answer_regex | log_triage | 359s | 40 | read=8,repo_map=2,list=0,trace=0,source_lens=1 | midloop=2,inv=4/0,fin_reject=1,unavail=0,prune=0 | fail | 读了源码但把日志解码器当消息生产者；inv_reject=0不表示无降级，日志有3次DOWNGRADED |

## 日志回放：有源码引用，但机制解释错误

- 原答案 `.codrax/output/20260906-190438.211-56996.md` / 同名 HTML，完整轨迹 `eval/results/read_combo_log_current_code_boundary-20260906-185841/run-1.logs.all.log`。无 Mermaid、无 Trace 投影是适用域正确，不是丢图/丢投影。
- 最终答案正确保留第3行的 `LLM stream timeout` 与第4行阶段状态，但错误声称 `DetectLogOperationalSemantics` / `decodeRendererOperationalSemantic` 解析日志后“自行生成第4行”。实际这两个函数消费已有文本、生成 typed 解释字段，不能据此证明它们生产该用户状态消息。答案又把匹配失败返回 false 称为系统校验，没找到实际成文拒绝与传输重试入口；末尾披露未读到调度写入路径，仍不能修复前文过度主张。
- 人工反证入口：`internal/render/log_operational_semantics.go` 是解码器；`internal/render/status_messages.go:233` 是 retry 文案表；`internal/orchestrator/read_stage_retry.go:94` 从 typed stream error 决定重试，后段发出通知；`internal/render/structured_tool_summary.go:654` 独立格式化成文拒绝。仅日志行相邻不能补造调度因果。日志第1/2行时间戳间隔20s而文字声称40s，只能注明原日志自报，不能把40s当已复算。
- 当前源码请求与artifact citation限制分开保留：入口 `current_source=required`；analyzer最终 `current_source_explanation_profile` 有明确引用，`external_observation_policy=allow/artifact_citation=external_only`。后续实际读8次，不是B1576把mixed源码关闭。模型偏向读取decoder而非重试producer；已有 protocol 提示也说明 observed_line_order 不证明跨事件transition。此轮尚不足以证明新系统接线错误，记录为“观察/生产者角色混淆与语义核查不足”，不加检测模型结论关键词的硬门。
- 耗时账：analyzer 4轮，3次字段自矛盾拒绝；explorer17轮，4次completion其中3次DOWNGRADED（ok=true，现有metrics拒绝计数仍为0）；finalizer2轮，补1个summary块后成功，未删原正文。context峰值79,651/200,000=40%，无工具不可用、无历史裁剪，并非预算不足。
- 元数据修补不是字段不存在：首批iter7已经带requested_dimension_indices，工具在同轮结果前置说明这些索引只落在definition/context，不能证明operation（日志1970）。模型仍误读成索引丢失，重复提交/same-ID supersession失败，直到iter15才改为实际call锚。不能据模型的解释宣称“教学必须带而工具必拒”已重现；后续仅审是否值得把已有精确提示在duplicate/completion表面复用，不放宽证据资格。

## Java：没有丢关系；可解析不代表条件和业务结论正确

- 原答案 `.codrax/output/20260906-190112.390-57023.md` / 同名 HTML；工作副本 `eval/results/sr_java_call_chain-20260906-185841/run-1.repo` git status为空，无源码修改。8次read、62行grounded证据；这是静态读case，不需要JDK/Maven。
- 正确主链为 create→schedule→insert→record→println；配置读取/已占用计数是schedule内的旁支，`if count>=max`是条件，不是新调用跳。答案把两旁支和条件合为“7跳”，虽然逐项位置基本正确，标题对主链长度的表达不精确。
- 六条已证调用边从首稿保留到最终；第一个patch给有序清单补6条精确关系凭证，修函数引用；图未删边。最后advisory补丁同一block同时用了receipt编辑与replace，被事务性拒绝；保留上一个模型草稿，不是系统删除/代写模型结论。
- 语义错误有3项：图在service被调用后才注明“入参非空校验”，实际在controller调用前；`alt 超限`结束后无条件继续insert/audit，应表达通过条件才可继续；正文把`System.out.println`说成“实现审计落库”，但fixture只标准输出，Repository另有内存ArrayList，并无数据库持久化。
- 次要准确性：正文把实际`application.properties`称`clinic.properties`；函数清单的schedule项引用controller调用点，虽然提示已给schedule精确定义。均不靠系统改正文纠正，保持模型质量观察。
- 精确终点及边界实际给到模型：`AuditLog.java:6`、`effect_scope=exact_call_only`与不能凭命名推持久化的教学在上下文；finalizer首轮推理甚至识别了“rather than persisting to a database”，成文仍升级措辞。当前证据支持模型语义波动/未守边界，不能以增加“必须出现未落库”等case词法门代替模型判断，也不为可解析图强制改写else/throw位置。
- 浏览器使用本仓 `internal/preview/assets/mermaid.min.js`，对最终原文不改字直接parse+render成功，1张sequence，SVG27,525字节；结果 `.codrax/tmp/r1025-java-mermaid.json`。模型参与者引号的既有安全修复生效，语法与语义单列判定。

## 后续优先级

1. 先收B1581异构fallback活跃流误取消，真实SSE 4ms确定性先红，影响整个适用阶段而非单个答案。
2. B1580完整动态关系组的冲突人口修复，避免展示/工作池cap充当唯一性证明；B1575先诚实披露probe证明粒度，不新造逐方法通过凭证。
3. 两份答案的语义偏差、图条件、术语及JSON编辑冲突作为异构质量观察保留，不强行重跑到绿、不下调case标准。继续高优先级Cangjie/ArkTS/其他write覆盖前，先完成本批代码与全套验收。
4. B1582确认低优先级反馈退化：纯duplicate被过滤后不再进入operation advisory，下一步可仅对本次命中的原有行复用同一软提示。它不是索引丢失/矛盾硬门，不改覆盖谓词，不新增模型字段，不保证一次修复就消除推理波动。
