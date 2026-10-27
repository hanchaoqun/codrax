# HMC301 两例人工审计

结论：机器2/2 PASS，**完整人工0/2**。A原4项子合同保持，但新宽窗资格分支未触发；B真实二进制取数及自然树查询正确，旧unknown→执行状态解释仍FAIL。54开放保持，不能用定向测试或原生事实表正确抵消成文误述。

## 1. 方法与并行证据

两名审计者分别检查每次真实工具参数、完整payload、日志中实际Finalizer INIT、提交文档、最终MD/HTML可见内容、原生表/图与sidecar；主控复核完整A 451行、B 82行及决定性差异。未跑额外模型/测试/原生查询，冻结collector每侧终态后一次；不新建或改执行helper。HTML是完整源码/表格内容审计，不签浏览器像素渲染。

实际请求区间：A local02:26:52.907–02:27:14.313；B local02:26:54.213–02:26:57.385，UTC均加7小时，**重叠3.172秒**。原run n1、budget15，没有第三例求绿；内部合法重试不算另一次eval。

结果根 `eval/results/hmc_scoped_rank_sampling_20261027_v1`。完整审计原件`/private/tmp/codrax-hmc301-A-audit-v1.md`、`/private/tmp/codrax-hmc301-B-audit-v1.md`和每侧materials/extra manifest经最终sealed-index定位，临时路径不作为唯一保存处。两正式进程exit0且冻结前后校验true。

## 2. A：链上候选和背景隔离

原case `trace_query_wakeup_background_demotion`；15事件构造附件，不冒称新实机trace。5个实际query都对app-100、2–2.02秒：window_stats、thread_timeline、wakeup_chain、root_cause_rank、critical_blocking_calls。没有宽查询，因此本批新增异窗候选排除**NOT_TRIGGERED**；不能用同窗输出4条反推新分支命中。

原生目标20ms sleep，链上IO threadpool-400 `[2.003,2.014)`11ms；三个priority候选各1ms。最终root-causes旁路确4条，保11/1/1/1ms，logger19.5ms没有升主因。物理wake链及关键信息保持，未把跨线程等待相加。原生表和投影保原量尺，闭合睡眠的更宽实际组合由确定性公共回归证明，而非本题自然签收。

**完整FAIL**：sidecar第3项把threadpool的1ms供给候选描述为“在同一IO等待区间上”，实际上就绪段`[2.014,2.015)`紧接IO结束，不是IO区间内状态。实际Finalizer INIT03:287含IO 2.003–2.014，INIT03:339/383/416明确就绪2.014–2.015/1ms/行9–10，最终MD E14:378也保留；这是说明错绑，不是上游漏传。真实数值/顺序不能替错误说明背书。占用表cookie17ms/network14ms各重复且没有区别标签，是既有量测→展示复用欠项，不据此删除独立量测。旧OpenDocument图6主体/3孤立角色没有在本题复验，不销账。

实际调查解释交接与B不同：A的3个interpretation units只投递前2个，明确披露2/3及572字节遗漏。已投unit[0]称“链上唯一有排名席位”，与原生四席位冲突；unit[1]把logger链相关统计说成“与目标重叠0ms”，与实际同窗时间易混。更强CPU解释unit[2]未投，不可声称已传播。最终正文又说logger几乎贯穿全窗，所以“没有重叠关系记录”只记结构化关联/时间概念歧义，不另断言其声称物理不重叠。不得推断调查冲突就是sidecar错句唯一原因。

最终原件前缀`.codrax/output/20261027-023106.109-95194`，MD55,464字节、HTML2,911,087字节、root-causes7,851字节；完整副本与SHA在sealed-index。A上下文峰值估计119,895 tokens，仅诊断估算不是账单用量。2,178字节/15事件原件产生至少三份各带查询身份的相同就绪成员表及重复唤醒事实；可证存在同源复用/预算审计空间，不证明这些记录均可删除或重复必然造成错句。两文本投影图保链上/背景及原生定位，实际无Mermaid；HTML5表逐值正确但重复行照样留账。

## 3. B：sample-only真实原件与解释权限

原case `trace_binary_sample_only`，原停顿问题不加系统护栏到用户问句。真实完整PERFILE2为30,062,585字节，SHA `da713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632`。同源HMC296独立oracle只读复用，没有重新转换或注入答案。

6次实际trace_query：目标perf_stats、目标event_search、显式perf_sampling_tree、2.2秒宽窗perf_stats、地址event_search自动继承目标窗、邻窗event_search。完整准备12000条；目标tid1609/tgid1514 `[486489.4,486489.6)`有164条/259权重；CPU1未符号化内核IP82/174，CPU0 OnDataEvent_82/85；记录份额各50%，权重67.18%/32.82%。宽窗515/1156独立，不混用分母。树1cohort/2节点、depth0、无children；只证明记录IP，不证明完整栈或最外caller。

自然显式树查询是在初统计选择器不支持完整树之后进行，成功而非零值伪装。三次event预览40+15+15条存在重叠，不能称70个独立样本；原生总体仍164。自动补齐查询范围实际命中。约122ms记录空白本次未当作阻塞或idle，权重未直接换时长。这些有限正向保持，但不关闭18.3或17.6。

**完整FAIL**：最终MD L15明确“这些是on-CPU的CPU周期采样”“最外一帧”；原生sample_kind100% unknown，depth0仅记录IP。L40/L56排除off-CPU也未被unknown支持。准确计数、正确unknown caveat、不能解释停顿的结论都不能抵消同答矛盾。

来源诊断：调查reason确写“CPU上跑什么”，但完整三个实际Finalizer INIT均未含该reason；不能臆证它传播导致终稿。INIT3 local174/175/180及426–430已交付unknown、精确CPU/权重/深度及不代表执行覆盖的边界，非字段漏传。成文估计92,801 tokens，未见决定性字段被截断；不因模型犯错就增加重复prompt或扩预算。

最终MD SHA `5db5e8d7b7df6e30029ee9e1ae84efa4d47dc4610037e85f76017f57bf7180d6`，HTML SHA `17cccd8b517b869c1ef2284293466da0c473191fd42880dcb9b5c601b40200e1`。4张HTML表内容正确，热点表重复两次；原始比较false源于Markdown反引号，不是HTML身份损失。无Mermaid/SVG/canvas图，不签图修复覆盖；原题无需图，不以缺图判失败。根因合同inactive/旁路为空，未伪造根因。

## 4. 状态和下一批

本批明确分开：2组候选共享修复；0完整能力新增；宽窗自然未触发；A区间/重复展示和B采样解释FAIL。代码保持冻结，失败原件不修写；main不合入。优先修已证图身份和同源呈现交接，再核解释载体与状态操作数；旧IO、采样、栈关系、代表与权限欠项保原。WAL生产NO-GO。全仓/归档/候选发布终态见[批汇总](parallel_selected_summary_hmc_scoped_rank_sampling_20261027.md)。
