# r1056 人工审计：显式窗 Binder 与读模式阶段时序/表格

- date: 2026-09-10T15:16:38Z
- sweep_start_ts: 20260910-081636
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

本轮两例各一次，未改问题、oracle、provider 或模型预算。机器 summary 原样保留；人工判定不由关键词评分替代。2026-09-10T15:36:40Z 两路全部结束，无第三个 live，未重跑追绿。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h1_binder_true_false_attribution | PASS | eval/results/real_trace_h1_binder_true_false_attribution-20260910-081638 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 273s | 59 | read=1,repo_map=0,list=0,trace=7,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=1 | FAIL | 五段时间正确，计数/范围/机理有错；投影保留；系统复算附注误配分母，旁路明确 unavailable |
| 2 | read_combo_pipeline_sequence_table | TIMEOUT | eval/results/read_combo_pipeline_sequence_table-20260910-081638 | answer_regex,answer_contains | none | 1200s | 69 | read=44,repo_map=2,list=0,trace=0,source_lens=0 | midloop=54,inv=19/2,fin_reject=15,unavail=0,prune=7 | FAIL | 外层截止前未交最终答案；图修补持续，首稿 ordered_list 不是要求的表格；多陈腐锚修补不可达已独立公共RED确认 |

## 1. 基线、排序与工程收据

243 个 case：默认 read 215、apply 25、plan 3。按影响风险、距上次回放、模式/关系覆盖、可独立复算排序：H1 上次 r1036，检查完整 Binder 库存与长睡眠分类、链内/外边界；阶段题检查显式 sequenceDiagram 与独立输入/输出表。写模式刚经 r1055 与本轮持久 proof 接线覆盖，不因这次两例均读而宣布写模式已全面验证。下一轮仍轮转异构写场景，不对单例加词面门。

四个修复批均已先提交推送：`264ab73b6`（B54 四种启发式判定退出答案附注）、`c3020f17f`（B1652 区分 suite 未执行与退出成功）、`eecd6ebdd`（B1651a Maven/CTest 当前执行报告来源）、`1d9ab6e49`（B1653 CTest disabled/notrun 范围）。本轮干净 binary revision `1d9ab6e49320`、built `2026-09-10T15:16:02Z`，快照 `.codrax/tmp/codrax-selected-20260910-081636`，运行期间未修改源码。

两轮冻结 `go test ./...` 均 exit0，各 86 个有测试包通过，未变包部分缓存：`.codrax/tmp/20260910-b1651a-final-full.log`、`20260910-b1653-final-full.log`；最后 tool 319.313s、agent 63.480s、types 45.192s、tracequery 105.472s、orchestrator 18.375s。最终 B1651a count3/race 17.475/6.753s；B1653 组合 count3/race 26.726/9.519s。各批有效公共 RED 与边界详见统一账 §123.1747–1750。Maven/CTest 使用真实子进程模拟官方报告协议，不冒称本机已执行 native Maven/CTest；Gradle/Meson/Hvigor/旧 JSON 来源仍开放。

活跃流专项 `20260910-b1651a-active-stream.log` 通过（llm 18.113s、agent 1.777s），覆盖 4ms 部分帧、隐藏推理、工具/heartbeat、旧总 cap、真实静默及显式 deadline/cancel。此次读题 exit124 来自操作者预设的 **1200s 外层 eval 上限**，日志末行 08:36:38 `received terminated`；不是产品在活跃流中按 4ms 或旧 4 分钟无可见答案自动降级。不能拿测试进程的强制截止冒充完整产品回退验收。

## 2. H1：原始证据、答案与上下文

最终工件 `.codrax/output/20260910-082109.501-74211.{md,html,root-causes.json}`；MD 113238 字节、HTML 3235725 字节，旁路 130 字节。日志引用均为该 case 的 `run-1.logs.all.log`；原始 Trace 为 `eval/fixtures/real_traces/donghu.ftrace`。物理原文件行与附件查询行有一行偏移，下面明确使用物理原文件行，不互换。

| 已验证等待区间 | 墙钟 ms | 对端 TID / TGID | sleep→wakeup 原文件行 |
|---|---:|---|---|
| 13762.835861–13762.837270 | 1.409 | 10961 / 9743 | 4356→4443 |
| 13762.894496–13762.895420 | 0.924 | 10961 / 9743 | 12562→12618 |
| 13762.937527–13762.937595 | 0.068 | 10625 / 9432 | 16946→16976 |
| 13762.950638–13762.950758 | 0.120 | 10961 / 9743 | 18524→18549 |
| 13762.951138–13762.951711 | 0.573 | 11354 / 9743 | 18611→18668 |

独立复算为 **5 段、3.094ms、3+1+1 个对端发生数**，五段互不重叠。日志3021–3026完整提供这五条；MD27逐段保留正确，但MD15/21/33错误写两对端、10961四次及4+1+1，且虚构不同对端请求重叠。233.190ms 窗的中点13762.908303，后三段在其后，不能说全部前半窗。传输延迟短不单独证明对端处理正常，闭合库存也不自动获得根因席；“体量小所以不列席”是模型解释，不是已核的系统准入规则。

15.758ms（13762.992415–13763.008173）由 app-9511/TGID9432 唤醒，当前 typed pacing 限定与原事件支持其节拍语境。但另有 **14.302ms**（13763.009537–13763.023839，原24820→27467）由 DetectViewRect-17679/TGID17267 唤醒，不能自动标成相同 pacing。日志2121的完整唤醒边、2289的 member_max 已进入成文上下文；MD53“唯一超过10ms”、其他睡眠均属IO或调度的穷尽主张不成立。不能以未关联 Binder 就证明主动休眠。MD41还把实际 RCVHS 写成 RWSB/RS，系统树仍保留正确操作。以上错误已在首稿3286中，不是系统 patch 改写。

七次 trace_query 均原窗13762.791708–13763.024898、目标线程一致，峰值118404/200000≈59%。完整五段、睡眠、TGID、目标/背景、业务及语义工作库存都提供了上下文。发布结果 JSON 经 read_file/grep 成功读取（1177、1275）；`|` 被模型配 fixed_string 导致零匹配，工具1281解释正确。派生结果回原导航没有实际使用，不宣称该分支获得新 live 正证。

投影只生成一份且保留：前五席58.320/12.658/7.405/4.710/3.956ms与本轮发布值一致；实际运行157.248ms、sleep70.338ms、runnable5.604ms、D0与规则折算分开。47条完成闭合IO阻塞12.658ms不能用 marker 状态0否定，也不能与请求驻留/其他IO席直接求和；邻近/背景与业务 span 分栏。46.455ms 自身状态行统计明确不是全窗分母，不判错账。没有 Mermaid；离线提取结果 `20260910-r1056-h1-mermaid-render.json` 为0图，原生text因果树不是解析失败，不能冒签 Mermaid render PASS。

首次全文被接受，顾问补丁一次失败后保留主体；读题15次成文拒绝不应混入H1。runtime_work_relation 两字段齐全，但模型选的 `wakeup_causal_impact:15` 实际是普通 s_sleep 1.805ms，不在 SemanticSpans 允许选择集，不能系统改选JIT/pacing。root-causes首稿直接传六候选数组，patch又把完整report包成单元素数组；2325教学明确同一个原生对象，没有发现合法对象被拒。旁路 `status=unavailable, reason_code=model_root_cause_selection_rejected` 诚实，不是零根因或文件丢失。

## 3. H1 确认的系统残余与下一批

| 工单 | 优先级/判定 | 通用修向与不可越界事项 |
|---|---|---|
| EVAL-B193-ARITHPAIR1 新显示见证 | P1 显示准确性，重新打开未覆盖臂 | MD21的3.094ms在233.190ms窗约1.3%合理；MD1019却选同句分母233.190作分子，报100%/差98.7pp。旧后置括号修复未覆盖跨句主语延续；不得跨句猜值或用数值/关键词拟合。优先限制未获语法/typed关系证明的“算术错误”评价，保原模型正文和确切事实，不新增硬门。源 `answer_document_mutation_runtime_arithmetic.go:309,687`。 |
| B1646 提示出口残余 | P2，确认 | 日志3010清单只含D/io_wait/内核IO标记S，3011却泛称0次目标等待。agent `answer_document_trace_principal_value_authority.go:173,187` 双语限定清单范围，保0和正值，不排除普通S、runnable、独立Binder/完成闭合。旧两tool出口本轮MD63/543正确；MD1013旧footer泛标签是已记未覆盖面，一并排期但不宣称已修回归。 |
| B1654-ROOTCAUSESINGLETON1 | P2，确认结构兼容机会，未实现 | 仅对完整原生report外包一层、恰一个对象做无损解包；内version、候选顺序、description不变，继续原binder/重复/未知候选校验。禁补版本/选择/结论；空、多元素、缺字段、重复key、截断不能猜。full/patch/拒稿暂存/既有选择保留需公共测试。首稿裸六候选数组不在这条无歧义修向内。 |

三者不能作为上述模型计数/机理错误的因果证明。B1651b Gradle 仍是证明来源 P1 主批；具体方案已在§123.1749。B1654公共红转绿后才声明修复，不能把静态六个ID确实可选写成已成功发布的收据。

## 4. 读模式：过程判定与未交付边界

机器 TIMEOUT、人工 FAIL；退出124、partial_result=1、partial_reason=selected_eval_worker_incomplete。44次read/2次repo_map，峰值137135/200000≈69%，15次成文拒绝、14次patch，源码与原case未变。没有最终 `output_dump` 记录，不能拿首稿或日志thinking冒充已交付MD/HTML；也未宣称最终图成功渲染。以下审计对象是已记录的模型首稿与实际修补调用，不是最终成品。

- analyzer先拒缺 relation_scope_quote，再拒把未出现在引文内的 Orchestrator 当显式图参与者；后改原文 analyze/finalizer。首稿日志9750的声明只有Run/Phase/Dispatch/Graph/Loop/ExReq/Apply/Bus，AgentAnalyzer/AgentFinalizer只在消息中，因此首次参与者缺失不是已证alias大小写误判。
- canonical stage_binding的Responsibility/PrimaryArtifacts、Run/dispatch/applyStageOutput实现已入模；FinalAnswer实际经scheduler消费/UpdateTaskResult，不经applyStageOutput合入Bus。首稿与completion仍误写最终文档写回该入口；标题stage-table实际ordered_list，没有表头/表结构，用户要求尚未满足。
- 不存在的runExplorePhase/runExtractPhase/runFinalizePhase首先出现在模型grep1749，其后模型反复纳入成员；未发现系统先要求这三个虚构函数。completion 8/7/6数组错位、无关支撑锚和错职责拒绝有依据；不能把整段多次补证都称系统自冲突。
- stage roster要求精确provider行47/61/73/85；这些是已验证完整composite内的Stage key位置，不是仅类型声明。模型另以definition/StageBinding为47等literal锚，ground恢复到类型定义7行并披露不证明职责；不能说provider强制正确实现改引类型行。是否过窄排斥完整正确implementation-only清单，仍需独立公共RED。
- 中段原生patch的n2被拒，但完整当轮schema在日志截断，n2属于同侧允许列表只见模型自述且后来自我否定；不据thinking判硬合同冲突。另将AnalysisIR/AnswerDocumentV2作为Stage字段的typed目标被拒有依据。
- 关系阶段staged后只需两人孤儿处置，模型带三人，Bus仍被两个typed carrier引用，因此拒绝；前置可见箭头统计与后置metadata统计不同域是审计线索，不等于已证明当轮schema仍教三个孤儿。
- 多次 Dispatch→Bus 无可见箭头的残余metadata，加上 ExReq→Dispatch 缺锚/重复发生，持续消耗后续补丁。日志10266报告两条陈腐锚，10305实际remove请求在10308被 `carrier=unknown / allowed_actions=[]` 拒绝；这不是仅模型自述。独立公共RED已确认同pair多锚清理不可达，见下段；候选提示截断与模型复用陈腐ref仍单独记，不以删图/放开整图改写/虚造边代替根修。
- 两次 completion 分别14.874s和14.900s，flow_participant与aggregate阶段各约7.4s；归已账B915-COMPLETIONFLOWFULLSCAN1残余观察。仅计时不能证明仍是同一全仓扫描，需profile再优化，不误写成两次总计15s。

**B1655-STALEANCHORMULTI1 公共封证（未修复）。** 临时overlay测试实际经过 ReadFile→EmitEvidence→EmitAnswerDocument→NewLease→Patch.Execute，并保留同块另一个有真实证据的可增加关系，复现 live 的局部租约条件。最终有效收据 `.codrax/tmp/20260910-r1056-stale-anchor-public-executor-red-final.log`（tool1.193s，预期exit1）：原两标签、完全相同重复、顺序反转三格均把两锚合成一失败，`carrier=unknown/actions=[]` 却仍 `locally_executable=true`，实际remove被拒且不暂存。单锚实际删除成功，摘要、正文、图体与无关锚不变；错ref与无证增边两负控通过。前一 `...-verified.log` 是fixture grounding前提失败，不计产品RED。根修须让本代冻结的元数据发生定位贯穿失败去重、租约、引用解析及删除，不能仅放宽 `len==1` 或按标签猜选；当前锚没有source字段，不宣称已覆盖不同来源分支。此测试只在 `.codrax/tmp/r1056-stale-anchor-uMAFsp`，正式源码/测试未改；实施与红转绿仍排下一批。

## 5. 交付范围

本轮四个代码批工程验收完成，不等于本轮live两答案正确，B54/B1652修复分支也未在这两题自然重撞。B1651a/B1653是验证器公共协议验收，不借两道读题冒充Java/CTest live。新发现全部回写统一账，后续优先处理图修补可达性与系统误述，再推进Gradle来源和其余适配器；模型已获准确上下文仍误读的部分保留人工观察，不持续堆硬约束。
