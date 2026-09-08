# r1036 人工审计：Binder 完整等待 / C++ 虚调用

- date: 2026-09-08T01:54:19Z
- sweep_start_ts: 20260907-185419
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

快照：干净已提交 `34f5dcf0a6ec`，未包含随后生产者统一入口收尾 `1eb085367`。两例同时运行，未启动第三例。机器表原样保留；人工结论依据真实过程、最终文件和原始fixture，不以关键词PASS代替正确性。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260907-185419 | answer_regex,answer_contains | none | 131s | 31 | read=6,repo_map=2,list=0,trace=0,source_lens=0 | midloop=7,inv=2/0,fin_reject=3,unavail=0,prune=0 | fail | 关键路径和工厂大方向对；时间戳幻觉、3个分支错引用，发现旧越界索引被新引用池复活的系统gap B1617 |
| 1 | real_trace_h1_binder_true_false_attribution | PASS | eval/results/real_trace_h1_binder_true_false_attribution-20260907-185419 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 249s | 57 | read=0,repo_map=0,list=0,trace=14,source_lens=0 | midloop=2,inv=1/0,fin_reject=2,unavail=0,prune=2 | fail (inventory fixed) | 5段3.094ms生产正证、投影和5项JSON存在；模型错称同进程/全部长睡眠pacing，内部键泄漏，不能收答案全对 |

## H1：B1607b 真实生效，但答案仍有残余

最终文件：`.codrax/output/20260907-185826.349-75065.{md,html,root-causes.json}`。machine wall249s，子进程247s；analyzer3/explorer6/finalizer3轮；14次trace_query全部带窗，无repo/read_file；4类trace维度、峰值114900/200000（57%）。不需要为了此例提高预算。

1. 实際producer/full finalizer日志2619/2882行清晰给出5条、union3.094ms、unresolved0/unassociated60，所有5条闭合端点进最终上下文。前4条preview明确4/5；完整清单不再受旧链1ms门筛掉。最终md17–25表给1.409/0.924/0.573/0.120/0.068，和原fixture五段一致。探索期曾手算2.621并猜遗漏第五条的时间，最终模型自行回到3.094；系统未把原稿总量替换成期望答案。
2. 最终md15仍声称“对端全部同进程496”，与其表中的binder:227_4-10625矛盾；三个peer的实际TGID分别为9743（10961/11354）和9432（10625），目标TGID17267。md31又把TGID1864的CompThread说成目标同进程。typed数据与原trace并未给这些同进程关系，属于本轮模型错误，不新造按线程名或prose扫词硬门。
3. md15将全部十几毫秒长睡眠判为pacing，并泄漏`pacing_idle`；原投影只对15.758ms那段给pacing上下文，14.302ms仍为普通S/证据不足，不可把它或其余60段同义化。md986的模型caveat又承认60段机制未知，内部自相矛盾。原长段未被系统改造成Binder根因，case正负oracle仍过，但人工不接受“全部pacing”。
4. 两轴、链上根因和业务线索保留：md34起实际占时/业务span，与md64起Trace投影、可消除榜分别存在；供给58.320、IO12.658、反转7.405/4.710、调度3.956，原Running157.248、DoFrame/traversal/measure/Repaint等业务线索也在；背景不因新增Binder清单获得根因资格。JSON schema2/status=available/root_causes5，来自模型显式选择，不是系统代选。
5. 补采入口日志1535–1536明确`no_typed_target`，本次没有补采engine调用，不能称r1036再次验证自动补齐。分析器最终（日志645）把1个用户目标及3个附件中邻居均写成user_explicit，形成4目标歧义；已有教学110/316/338明示只收当前用户身份，但模型没有遵守。待审每条目标的来源声明/焦点绑定机制，不得按原文关键词擅自删掉模型实体或挑第一个PID；现有显式窗/补采结构回归仍通过。
6. JSON重试：首稿blocks为JSON字符串，系统无损恢复并隔离顶层schema_version；恢复后`trace_causal_claim_caliber`不符合当轮schema而拒，trace_root_causes裸数组作为可选字段被诚实忽略并提示正确对象形。第二稿full4block已接纳并成功导出5项根因；最后可选展示修补声称unchanged s4，但s4不存在于刚接纳稿，精确拒绝后保留有效稿。两次拒绝并非同一字段“必带必拒”，没有系统删除模型正确答案。原model主段与可见表头仍有“列1…”及`verified_wait_union`机器键泄漏，列为软教学/显示数据词面改进，不修写用户旧产物。

## C++：机器覆盖不足，引用代次是可修系统gap

最终文件：`.codrax/output/20260907-185628.440-75066.{md,html}`。machine131s、子进程129s，31%上下文；6次read/2次repo_map，analyzer3/explorer10/finalizer4，三次成文拒绝，无Mermaid源及修复，首稿就没图，不是系统删图。

代码真值：logger.cpp29–39先null/min_level guard，level_label/空格/message三次append，虚调用write；Error才flush，Console继承Sink空flush；console_sink.hpp10–12 fputs+fputc输出stderr。registry.cpp15–26根据kind选择类型，make_sink30–32调用create；fixture没有factory结果实际传入Logger构造器的调用点，不能编造初始化接线。README把format_value写到Logger::log路线是陈腐的，实际只在log_latency45；本轮没有read README正文，不能归因该陈腐内容污染模型。原源码日志983–988和finalizer2852已有正确append/flush，模型却仍错说/遗漏。独立native clang++见证在`.codrax/tmp/r1036-cpp-native-1y9htB`，选择/null/level/Errorflush/latency格式及console精确18字节含换行通过；仅验证原fixture，不改变case或冒作正式模型验证。

最终答对主要虚分发、kind选择和stderr，却自增“时间戳”且漏换行/条件flush与初始化接线边界。三个kind分支引用全部错位：console→registry20(file分支)，file→registry23(rotating分支)，rotating→logger30(guard)。初始模型参数本来就提交了错误pool索引6/7/8（8越界），前两错不归罪系统改写正确引用。

**B1617（P1，确认，另批修）**：日志2877首稿8越界，2881被隔离；后续只改其它block、unchanged继承的section，却被每次merged重算CitationRefsModelSubmittedValues的资格；其它修复添加logger30为新pool8后，3004“stabilized”及3010变成引用logger30。共享patch机制允许旧非法索引随pool增长复活成未选过的新事实。应冻结原提交代次资格/来源，明确新提交refs才按新pool登记，旧unchanged不可重新确权；不凭label或源码内容帮模型选“正确引用”。三次拒绝分别是缺typed identity/evidence IDs、同block replacement与atomic mutation冲突、replacement丢旧visible_label；没有发现与教学相反的必拒合同。

完成度仍须诚实：日志1488的investigation_complete曾被DOWNGRADED（工具ok=true，所以机器reject计数0），1570补返回fact后1605仍是low-delta force-complete，1609还披露selection unproven。不能把它当“补证之后全部合同满足”。本轮先留真实过程边界，不凭结果不佳再造强制关系门；后续审选择/构造/调用不同关系族的完成判定是否同源。

## 排序与后续

优先修B1617引用代次，不为两条模型叙述错误加关键词门或强行改文。并列记录不同capture旧账户物理身份审计（B1618）及B1616b/B1561验证履约余项；新Binder上下文直接暴露机器键可做中性词面修复，但不是改写最终正文。补采歧义与模型错误来源声明保留为明确观测，先查精确来源合同再设计。下一轮继续高优先级异构读/写，不为本题再次反复求绿；仍恰两路。
