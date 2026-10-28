# HMC303人工审计：业务范围、31ms与PCAP完整入口

日期：2026-10-28。只读正式冻结后的原始材料，未额外调用模型/查询/测试/构建；未改源码、测试、注释或helper。两个独立审计与全部原件在`eval/results/hmc_business_pcap_20261028_v1/sealed-index-v1.json`按原来源路径可查。

## 1. 总结

机器0/2，完整人工0/2。A数字局部改善但图未交付且正文仍冲突；B在真实编排准入失败，无原生查询。54开放不减；完整能力新增0、候选子能力2组。原四根因、Trace投影、自动补齐的确定性保护通过，不等于本次模型图和解释通过。

两例runner均exit0，不是产品成功：A原verdict `FAIL degraded_answer_checks_skipped:1`；B产品exit1，verdict `FAIL read_exit:1 no_successful_tool_result:trace_query`。实际可配对请求A19/B1、1个A歧义响应保留，至少5.103秒重叠。未以初始化日志或进程并存凑并发。

独立完整报告：

- A：`/private/tmp/codrax-hmc303-A-audit-v1.md`，SHA `d2be43673e65b7b133621c5af7e70d05e566e9984953bf4e69581b58af6bf9ee`。
- B：`/private/tmp/codrax-hmc303-B-audit-v1.md`，SHA `1cb9264c6aa26c3d37d426deddf7ddd74edc04c7985a1efe919c9aa6dbeaf0b2`。

## 2. A：事实到场与正文有限改善

正式run为`A/trace_business_io_sequence-20261028-003259`。实际17次trace_query、13份不同payload、3份Finalizer INIT、27份工具参数；调用数、不同落盘原件数不能混为分母。附件是已有合成ftrace，不冒称实机；本例未读项目源码。

| 同源事实 | 正式答案 |
|---|---|
| OpenDocument [1.000,1.050) 50ms；主线程running5/runnable1/S44 | 正文及原生业务表正确 |
| 查询[0.999,1.051) 52ms；主线程running7/runnable1/S44 | 与业务表分开，没有再套成50=7+1+44 |
| worker业务[1.0045,1.0445) 40ms；running8/runnable1/S31 | 正确 |
| worker闭合S [1.009010,1.040010) 实测31ms；RQ[1.005,1.040)驻留35ms | 实测与估计潜力分开，31与35未相加，未升为设备服务时长 |
| 80→200@1.040010，200上CPU@1.041010；200→100@1.045，100上CPU@1.046 | 正文/原生事实保留两次1ms醒后等待；请求关系图未交付 |
| backup47ms不在目标闭合关系上 | 仅作背景/条件排查，没有升为主因 |
| 原4席#1 IO/#2 main scheduler/#3 main runnable/#4 worker runnable | 正文:326误说只有#1/#2/#4、缺#3，自身附录:580却列#3，FAIL |

本次局部正确不倒签§302旧失败，也不证明新引用字段有效。真实INIT仍有全部正确原生数值/范围，不能将成文冲突归成“系统没有提供事实”。

## 3. A：引用未触发，调查解释仍有冲突

三次complete：第一次有7units但缺runtime_measurements，真实拒绝empty_selection；重试选择10个view、7units全部native_refs=[]，accepted；第二探索分支没有handoff，reason+7aggregates accepted。**非空精确行引用NOT_TRIGGERED。** 被实际接受的较早查询不能因模型thinking怀疑就判错代；没有独立完整wire schema dump证明该轮schema stale，不臆测原因。

上游仍出现“CPU1不缺CPU”套用全范围other_cpu_idle37.5ms、以未知priority声称显式排除反转、将main scheduler与runnable重复相加成三席3ms等自由解释。最终正文没有全部沿用，不能认定所有错误都传播或是唯一成因；也不能称新增行地址验证已校验它们的文字语义。

INIT大小121009/29721/351793B，估计max_context_tokens=189464/200000，不是服务端计费tokens。原302内容字典已触发，仍有7份完全相同main全窗表体在HTML重复；必须保各查询身份而复用显示，不能相同值就合并事实/分母。来源标题“端点包含性未知”与紧邻半开表头/Notes冲突，端点事实并未缺失，继续留账。

## 4. A：图身份失败、修补失败和恢复隔离

首图有Main/Worker/Storage三个节点，两箭头方向及时间顺序正确，**不是倒序**。但第一跳worker接收端原生lane为TGID未记录的`rt_thread_c63f…`，第二跳worker发起端为TGID100的`rt_thread_e4d2…`；模型把Worker显式绑后者又承担前者事件。不能凭同TID/名称授权合并。

原日志4283行pre-emit advisory后，真实发射拒绝两条`edge_anchor_node_identity_conflict`及三条业务角色`missing_unproven_boundary`。补丁加三未证边界并删两条边，却未声明孤立Storage应删除还是保作背景；关系改动仅暂存在未发布retry base，再次拒绝，没有下一成功修补。被拒图还把completion1.040与wake1.040010合成同一标签，非最终泄漏。

最终MD:310声明未证图暂不展示。HTML主区域Mermaid/SVG/canvas/iframe/img均0，MD无Mermaid；未证图没有经恢复/附件泄漏。`.answer-surfaces.json` unavailable/final_render_ownership_unavailable，`.root-causes.json` unavailable/valid_model_root_cause_selection_unavailable；空root_causes不等于已查无根因。

隔离安全有限通过，用户要求的时序图和完整答案FAIL。HTML只做原件/主区域结构审计，未做像素浏览器验收；不存在可签成功的图。后续须公开复现精确身份与原子修补，不放松原边门或系统补造业务关系。

## 5. B：真实PCAP准备成功，但完整编排失败

正式run为`B/pcap_packet_inventory-20261028-003259`。自然题仅问包数/时间/协议/几条包长与来源，不把时钟和因果约束塞给用户。真实原件3,499,182B；全78行日志显示准备生成542B导航、查询材料仍是完整PCAP，唯一分类请求正确为artifact/optional-source、answer+measurement。

随后`Orchestrator.Run→validateAttachedTraceInputs`对`material.QueryPath()`无条件调用文本`ValidateTraceInputPath`，以NUL要求转换Trace。没有prestage、trace_query或Finalizer，blob目录空；只写132B unavailable根因状态，没有MD/HTML。不是零包结果，也不是模型未选择工具。collector的旧payload文件名过滤局限不是本次原件缺失原因，因为查询根本没执行。

`cmd/attached_pcap_hmc303_test.go`在prepare后手工建Bus直接调用工具，绕过Run门；局部PASS不支持完整CLI宣称。命名材料另有无条件文本准入的静态同类风险，本例没有触发。下一批以受信材料的格式/查询能力分派准入，保regular readonly/代次/manifest及坏源边界，不将所有binary免检，不按后缀或原文关键词放行。

独立Ruby框架oracle和系统tcpdump提供6101条记录、IPv46101/TCP4336/UDP1627/ICMP138、epoch首末及捕获/原长等基准，但本轮没有可对照的原生查询。原件SHA冻结后相等，仅证输入字节不变；CLI正常缓存清理/日志输出存在，不能宣称全局零副作用，更不能给WAL放行。

## 6. 留账与封存

唯一全仓两FAIL按原断言保留：新透明转发依赖未进严格typecheck；专用view缺真实metric_refs。冻结后无生产/测试/helper修补、无第二次全仓/第三模型例。

优先级：先补PCAP受信查询域跨Run准入及专用输出/指标/epoch合同，接着修图lane—事件—业务角色的可验证绑定和孤立节点原子修补；非空行引用自然覆盖另验。原4席正文、显示重复、未知优先级/CPU范围解释及旧采样/IO/代表/权限欠项继续原账。原四席位可保留，不能因时长等值机械相加。

封存索引保存原始失败字节、所有过程/输出/旁路及两份完整人工报告，源码快照使用非Go后缀；原始非法UTF8日志不被派生文本替换。所有结论只覆盖实际触发路径，不签未触发分支或完整父能力。
