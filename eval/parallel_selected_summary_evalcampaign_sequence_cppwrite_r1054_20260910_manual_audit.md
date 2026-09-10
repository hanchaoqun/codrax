# r1054 人工审计：时序修补复验 × C++ 写模式持久交付

- date: 2026-09-10T12:04:43Z
- sweep_start_ts: 20260910-050443
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

生产二进制 `1f0ead712a60`，干净构建于 `2026-09-10T12:04:02Z`；快照 `.codrax/tmp/codrax-selected-20260910-050443`。原 case、oracle、模型步骤/重试预算未改，1200s 是本轮显式外层上限，不是活跃流年龄门。两题于 12:10:28Z 全部结束，未追加第三个 live。机器 1 PASS / 1 FAIL 不等于人工全绿。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_fmt_tm_year_overflow_symptom | FAIL | eval/results/github_issue_fmt_tm_year_overflow_symptom-20260910-050443 | write_apply,answer_regex | none | 213s | 28 | read=6,repo_map=3,list=0,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | 交付代码 PASS；正式证明未闭合 | 两行宽化、原生测试及独立7边界通过；已知 B1561 compiled assertion receipt 缺口，系统诚实 unverified，不回填为绿 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260910-050443 | answer_regex,answer_contains | none | 345s | 58 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=10,inv=2/0,fin_reject=3,unavail=0,prune=0 | FAIL | 图原文可渲染、分阶段修补能推进；正文仍倒置 Run/RunWith、函数职责失真、清单次序不符；另确认 B1649 精确证据已在而短显示名修补候选缺席 |

## 1. 选择与排序

库存仍为 243 个 `.case`。r1053 显式窗 H6 与时序题暴露的 B1645–1648 已逐批提交，本轮优先复验同一修补机制；第二席跨模式选距 r1039 约14轮的 C++ 症状写题，当前 clang/SDK 可实际编译，优于重复已知缺 Java/Rust 原生环境的条目。排序依据是证据/修补风险、覆盖面、距上次、原生可验证性，不为刷机评挑题。

## 2. 时序题：机器 PASS，人工 FAIL

- 工件：`.codrax/output/20260910-051026.423-17843.{md,html}`。
- 过程：`eval/results/qf_sequence_analyzer_gate-20260910-050443/run-1.logs/codrax-20260910-050445-000-17843.log`。
- 使用随仓 `internal/preview/assets/mermaid.min.js` 离线对原图 parse/render 成功，SVG 25,543 bytes；收据 `.codrax/tmp/20260910-r1054-qf-mermaid-render.json`。本轮是关系/答案质量问题，不是 Mermaid 语法失败，不改图去迎合渲染。

### 2.1 成文过程逐轮核对

| 轮次 | 日志位置 | 结果与归因 |
|---|---|---|
| 1 | 3168–3182；3286–3297 | 13 条真实调用缺图内引用锚、列表 call 声明缺锚、错误 RunWith→gate.Run 关系，拒绝有据；缺锚不等于缺调用证据 |
| 2 | 3361–3367 | 模型删除13边、又新增一条已存在的 build→RunWith，关系修补暂存成功；要求处置13个孤立节点是分阶段协议 |
| 3 | 3398–3427 | 孤立节点操作通过租约；随后重复调用发生次数无第二凭证被正常拒绝，没有 r1053 的未动边 removed/added 自冲突 |
| 4 | 3459–3470 | 模型删重复边，接受 |
| 5 | 3512–3523 | 模型补概念终点说明的结构化元数据，接受；没有改正首稿正文 |

比 r1053 的20次拒绝，本轮只有3次；但模型路径不同，不能把差值全部归功于 B1647。当前使用 p0/p14，不重撞 capsule n1/n2 域冲突，也未出现 mixed-identity 恢复日志。只记实际分阶段流程能推进，不冒签两个新分支均已自然命中。

### 2.2 人工答案与入模内容

实际源码是 `buildAnalysisIR→gate.RunWith`（analyzer.go:2725）与 `gate.Run→RunWith`（gate.go:135）汇聚，**不是 RunWith→Run**。首段却称 RunWith 分发/返回 gate.Run 结果；系统概念目标附注不能抵消此错误，也不应替换模型原文。清单放在图前，与用户明确的“图后列出”相反。部分函数职责从名称猜出：slash-pair 补全写成斜杠命令、人名地点实体、Oracle 外部知识，均超出引用所证。gate 覆盖检查的具体口径也混淆。

入模确有方向边界、不得将 fanout 串成互调的教学（2838–2842）及 buildAnalysisIR 32条调用清单（2878–2909）。真实中部还有 Normalize:2323、Amplify:2348、Compile:2530、AmplifyPostCompile:2551、BindByRelevance:2560；模型偏取早期辅助函数，不据此加固定函数必写门。Run 包装器正文有源码，但本轮精确调用 capsule 未含 Run→RunWith，不能从文字把未登记的关系自动授予图。

### 2.3 新确认 P1 B1649：已证短显示名没有保留式补锚通道

13条错误均为 `missing_grounded_call_anchor`；`answer_document_diagram_evidence.go` 只在 `hasTypedCallEvidence=true` 时铸造该错误，调用已由原严格 matcher 确认。图显示 `buildAnalysisIR`，候选真实身份是 `agent.buildAnalysisIR`。修补器 `preEmitStandaloneRelationCandidateMismatchSelection` 再用全限定身份等价比较两者，合法短显示名不等于全身份，导致 attach 候选根本未生成，模型被引向删真边。

需纠正粗归因：日志 `additions=1/9` 的另外8条是列表 claim-only 候选，因原 claim_uses 无 EvidenceID 而不能直接绑定租约；列表已通过 whole-block 修补补齐。这8条不是上面的13条图边，不说“9选1吞13”。

通用修向：复用原校验器已确认的精确调用/来源匹配，把候选交给模型选择；不全局放宽短尾等价、不从 prose 猜身份、不自动加边。公开实际 Read→EmitEvidence→缺锚诊断→修补候选→模型选择的先红后绿，以及同名多owner/方向/文件/来源负控，是施工前提。r1054 本身仍 FAIL，不因后续单测修复倒签。

## 3. C++ 写题：交付正确，正式证明未闭合

- 计划 ID：`plan-1789042033391417000-17858`；plan-2 是同 ID 状态镜像，不是第二轮改码。
- 持久引用：`refs/codrax/applied/plan-1789042033391417000-17858` → `6cfe3303923bf64e9e0dac1ba9469f49333c43af`。
- 保留树：`eval/results/github_issue_fmt_tm_year_overflow_symptom-20260910-050443/run-1.repo/.codrax/worktrees/trace-1789042034124246000-18605`；另有 runner 的 `run-1.applied-tree`。
- fixture main/base `84671723ea808834a07cc160137cbac29f2f76e4` 未变；测试/Makefile/README 未改，`-fwrapv` 保留。
- 真实补丁仅两行：`render_year(int)` 改 `render_year(long long)`；在 `parts.year_offset + 1900` 前先转 long long，且 calendar_year 用 long long，不让结果回窄。

应用日志 `run-1.logs/codrax-20260910-050713-000-18605.log` 的665及1039行两次真实 `make check` 成功。root 另在持久交付树独立运行原项目测试（`.codrax/tmp/20260910-r1054-fmt-durable-native.log`）并编译7边界黑盒：

| 输入 tm_year | 期望/实得 calendar_year |
|---:|---:|
| 121 | 2021 |
| 2147483647 | 2147485547 |
| -2147483648 | -2147481748 |
| -1900 | 0 |
| 0 | 1900 |
| 2147481747 | 2147483647 |
| 2147481748 | 2147483648 |

七项均通过，日志 `.codrax/tmp/20260910-r1054-fmt-independent.log`；原 baseline 相同黑盒有两个上界 wrap 失败，基线日志 `20260910-fmt-independent-baseline.log`。负历年对 INT_MIN 合法，不能把模型泛称“无负数”当全输入的有效要求。

正式 report 只有 aggregate `make-test`，`no-wrap-or-neg` 仍缺 `project_test_assertion_not_observed` / `behavior_contract_observation_missing`。应用日志789–794、1163–1168已完整向控制器披露 required=1、covered=0 与具体 why，最终输出明确“未完全验证”。重复跑同一测试不能补足精确断言收据；root 的独立验证不回填官方 report。此为既有 **B1561-NATIVEPROOFRECOVERY1**（同 fmt r1020/r1021/r1039 已有见证），不是 B1616b 跨计划丢证明，不新开同根 P1。

模型首次要求不支持的 C++ inline probe 被 schema 正确拒绝，后改项目测试；一次错误 observed literal 被降为 planning_only，未偷渡为证明。上下文峰值55,356/200,000，未耗预算；两次字符串数组兼容恢复未改最终补丁。未新增编译器猜测或题目专用断言。

待证观察另列：`run-1.plan.json:193–194` 的 provenance 称没有 declaration-line change，而 header 第18行参数声明已变；尚不能据此认定 medium/auto_execute 风险判断错误，不与本次正式 proof 失败混因。

## 4. 后续与保留边界

1. B1649 先补真实公共失败针，再统一证据匹配与修补候选来源；禁止扩大硬门和系统代写。
2. B1561 原生精确断言收据是持续高ROI待办：先统一现有编译测试执行证据域，不为 C++ 单题制造解释器式 probe。
3. 回到异构 read/write/Trace 轮转；本轮模型方向/职责/排序失真保持人工 FAIL，不为某个题补关键词硬合同。
4. B1645–1648 冻结全仓 86 个有测试的包通过（`.codrax/tmp/20260910-b1645-b1648-final-full.log`）；这不等于本轮两个答案全正确。活跃流专项 count3 及完整 llm/agent 通过；没有按4ms或旧4分钟“无可见答案”提前降级，显式 deadline/cancel、真实断流/停滞仍按原边界处理。
