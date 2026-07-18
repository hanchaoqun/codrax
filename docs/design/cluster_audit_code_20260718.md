# CPU 簇结构判定链 — 拒绝/降级路径全谱审计(代码侧)

日期 2026-07-18。基线 main=fec890839(活树只读)。witness=/Users/han/opt/donghu/donghu.ftrace(鸿蒙底座全量 ftrace)+ 同目录 xxx_all.systrace(容器侧 systrace)。探针全部跑在 scratchpad/repo 源码副本(rsync 去 eval/.git),probe 文件 zz_cluster_audit_probe*.go(不入库)。

## 0. 引擎实测(定盘)

| trace | 采样核 | 推导 | capability |
|---|---|---|---|
| donghu.ftrace(14 核) | cpu0-13 全部有 cpu_frequency(31/67/85 样本) | 3 簇 {0-3}{4-11}{12,13},零回滚,burst 内偏斜 ≤8µs | **default_table**,small/middle/big,fmax 1720000/2270000/2750000(limits 仅 cpu_id∈{0,4} 但经成员归并覆盖前两簇)|
| xxx_all.systrace(容器侧 6 核) | 仅 cpu3-5 有样本(各 30) | 1 簇 derived_c0={0..5}(规则1 首簇把 0-2 收编) | **freq_only**(<2 簇臂),rail 无可采族(thermal/ddr 名筛除,l3c 单成员),显示词=「簇结构不可判,按纯频率比折算」|

结论:客户「经常判断失败」的第一机制不在全量底座 trace(判得干净),而在**容器侧/系统单 policy 采集形**——只有一个簇有 cpu_frequency lane → <2 簇 → freq_only。第二机制是大 trace(>MaxEvents)结构性丢失规则4全文件基(见 S2)。

## 1. 拒绝/降级出口全谱

### 1.1 推导层 deriveClusterFreqDomains(cluster_freq_share.go)

| 出口 | file:line | 触发条件 | 用户可见后果 |
|---|---|---|---|
| 零采样 | cluster_freq_share.go:301-303 | 无任何 cpu_frequency 样本(或全被毒化/未采) | domains unknown → freq_only「簇结构不可判」+ 无 donor + 窗口面无核类词 |
| 共动 split:no_samples | :606 | 一侧空(推导入口已滤,实际不可达) | —(防御臂)|
| 共动 split:head_junction_state_mismatch | :636-641 | 头部裁剪后接续态不一致 | 该对分簇 → 可能引发碎裂(>4)或 fmax 撞车 → freq_only |
| 共动 split:mid_alignment_mismatch | :647-651 | 中段任一变化点 kHz 不等或偏斜 >15µs(clusterFreqDeriveMaxSkewSec=15e-6, :141) | 同上 |
| 共动 split:co_witness_floor | :655-662 | 对齐变化点 <2(clusterFreqTrimmedMinAligned=2, :693)——恒频对只要一侧被边界削一行即永不可并 | 同上;splitAudit 可定位 |
| 共动 split:tail_exemption_unmet | :670-679 | 尾部未配对变化 >1 个 / 不在全局流尾 15µs 内 | 同上 |
| 向上不外推 | :354(注释)+ :737-761 | 最高采样核之上的核:≥3 簇→derived_prime(声明不给 donor);<3 簇→无归属 | 该核 无频点数据口径、无核类、无 donor(诚实)|
| 簇间豁缺(R6 撤销向下继承) | :277-284 + :737-742 | 两簇成员区间之间的无样本核 | 同上(诚实无归属)|
| 显式拓扑零解析 | :193(entry 丢弃)+ :231-241 | core_topology 非空但无可识别标签 | explicitInputIgnored 精确旗 + caveat 披露,换道推导 |

### 1.2 capability 层 resolveCoreCapabilityEvidence(core_capability.go)

| 出口 | file:line | 触发条件 | 后果 |
|---|---|---|---|
| Tier-1 样本下限门 | core_capability.go:298-312 | 某簇 ≥2 采样成员而代表时间线 <2 样本(单 burst 合并) | freq_only + comoveFloorTripped(typed pin)|
| domains unknown | :352-355 | 推导零采样 / 无可归属 | freq_only |
| 簇数 <2 | :370-374 | **单采样簇**(容器侧常态)或全毒化仅剩一簇 | freq_only,splitAudit 空,与其它臂不可分辨 |
| 簇数 >4 | :370-395 | 碎裂形(边界/偏斜 split)或真 5 域 SoC | freq_only + splitAudit(定位首分裂点)|
| fmax 撞车 | :403-412 | 任意相邻序两簇 fmax 相等(全表死,非仅撞车对) | freq_only + splitAudit |
| rail 交叉验证冲突 | :317-319(cluster_rail_evidence.go:401-440) | rail 样本与成员实测频点 10ms 邻域内无一 10% 相容 | 仅丢 Tier-2,Tier-1 保留(railRejectReason typed)|
| rail 结构冲突 | :320-322(cluster_rail_evidence.go:500-539) | 锚区间横跨两 Tier-1 簇 / 首锚以下+区间内混合 | 同上 |

### 1.3 Tier-2 六门(cluster_rail_evidence.go scanClusterRailEvidence)

| 门 | file:line | 触发 | 备注 |
|---|---|---|---|
| sched 集空 | :282-284 | 无 sched_switch | 整个 rail 扫描不跑 |
| ① 族形 <2 成员 | :308-310 | 单 rail 或名字无数字段(无 mask) | 无 reason 记录(从未成候选);厂商命名无共同数字位的两簇轨(cpu_little_freq/cpu_big_freq)结构性永不成族 |
| ② 无键/键缺 anchor_key_missing | :318-321 | hmtrace 位置形(cpu_id 无效) | 整族回退 |
| ② 锚撞车 anchor_collision | :327-330 | 族内两 rail 同锚 | 整族回退 |
| ③ 锚漂移 anchor_unstable | :322-325 | cpu_id 非恒定(thermal/heca 形) | 整族回退 |
| ④ 量纲 dimension | :332-335([50MHz,6GHz] :96-99) | 任一样本出带 | 整族回退 |
| ⑤ 锚出集 anchor_outside_cpus | :336-339 | 锚不在 **sched_switch 观测集**(:590-605,取自 idx.Events 即窗口 carve) | 整族回退;窗内全闲簇误杀(见 S3)|
| ⑥ 负向词汇 | :194-202(tokens :113) | 名含 thermal/ddr/gpu/vote/delay/info/load/temp/tsens | 仅收缩候选(donghu 的 ddr_cluster#_freq 族靠它正确杀掉——载荷是内存轨,锚 0/1/2 恒定在带内,若不筛除会被采成假拓扑,⑥ 是承重墙)|
| 多族歧义 ambiguous_families | :381-390 | >1 族全过门 | 全拒(不部分猜测)|

### 1.4 基底层(规则4全文件曲线,full_freq_curves.go + parse.go)

| 出口 | file:line | 触发 | 后果 |
|---|---|---|---|
| 非 EOF 完成 | parse.go:2007 finalize(!seeked && buildReachedEOF) | anchor seek / padding 截断(parse.go:1919-1924 break)/ windowed stop(parse.go:1922 区)/ 未达 EOF | collected=false → 推导基=窗口 carve → CAP-3 病灶族回归(簇数随窗漂、fmax 偏小)|
| 样本溢出 | full_freq_curves.go:150-152(cap=1M) | 病理级频点行数 | 同上 |
| anchor 复用上限 | full_freq_curves.go:71 + parse.go:2008(128Ki) | 大频点集不入 anchor 记录 | 后续 seek 建的 index 拿不到全文件基 |
| 组合 bundle 不可映射 | full_freq_curves.go:194-211 | 任一子样本无法映到公共钟域 | 组合体退回 events 基 |
| 毒化核剔除 | full_freq_curves.go:162-167 + cluster_freq_share.go:425-435 | 同 lane 物理时间戳回滚 | 该核整体退出推导:**若它是某簇唯一采样核,簇数静默降**(3→2 时中簇被按 small 定价、规则1把真小核收编进中簇)——方向是静默错分类而非拒绝,推导面无 caveat |

### 1.5 消费面(判不出的落点)

- supply fold:coreCapability(q.CoreTopology)(supply_fold.go:607)→ freq_only 时基准=全域最高频点、cap=1(supply_fold.go:565-583);显示词单点 internal/tool/answer_document_mutation_runtime_supplyfold.go:148「簇结构不可判,按纯频率比折算」。
- b3「频点数据不全,无法折算」:切片 CPU 无 governed 样本且无 donor(domains unknown 或簇间豁缺/prime)。
- 窗口面核类词:resolveCoreTopology(query.go:8592)→ indexDerivedCoreClassByCPU(core_capability.go:573)→ 非 usable 返回 nil → 全部 unclassified(ClusterFrequencyCeilings 归入 "" 池,cluster_ceilings.go:86)。
- R5a 核档排除:cpuConstraintTierExclusion(core_capability.go:617)!usable() 即整臂静默(双重否定验收,合规)。
- donor:donorFor(cluster_freq_share.go:716)三处 fail-open(自有样本/无归属/无采样同胞)。

## 2. 短板清单(按 常见度×后果 排序)

### S1【最常见】<2 簇臂把「单采样簇」说成「簇结构不可判」,且 freq_only 五个成因下游不可分辨
- core_capability.go:370-374;显示 runtime_supplyfold.go:148。
- 常见度:**高**。容器侧/系统 systrace 只 trace 一个 policy 的 notifier(xxx_all.systrace 引擎实测:仅 cpu3-5 采样 → groups=1 → freq_only)。这就是客户投诉主形。
- 评估:拒绝方向本身正确(单簇无跨类比例证据,cap=1 纯频率比 == 单类定价,数值恰好无损);短板在**披露**:①「不可判」措辞失实——结构判出来了(单簇、连 0-2 都闭包了),缺的是跨簇算力信息;②freq_only 无 typed 成因(no_samples/single_cluster/>4/tie/floor 全部折叠为一个 token,仅碎裂臂有 splitAudit),客户回访与自诊断均无从区分。
- 放宽:加 typed freqOnlyReason 枚举(精确信号:groupCount、sampledAsc 长度,已有)+ 单簇形专用措辞「仅单簇有频点采样,无跨簇算力信息,按纯频率比折算(单簇内等价)」。纯披露车道,无门,红线全合规。

### S2【大 trace 结构性】>MaxEvents(250k events)的 trace 永远拿不到规则4全文件基 → 簇判定退回窗口 carve,簇数随窗漂
- parse.go:2007(finalize 条件)、parse.go:1919-1924(padding 截断 break)、defaultTraceIndexMaxEvents=250000(parse.go:80);全量建 index 直接预算硬拒(probe3 实证),窗口建则 padding 截断/seek → buildReachedEOF=false → collected=false → indexFreqSampleTimelines 落回 idx.Events carve(cluster_freq_share.go:419-437)。
- 常见度:**高**(客户旗舰 trace berlin 1104MiB 级全命中;donghu 3.5MB 才 27843 events 幸免)。
- 后果:CAP-3/R6 已裁的病灶在大 trace 全数回归——窗内零变化点的簇从计数消失(3 簇 SoC 判成 2 簇 → 中簇按 small 定价、规则1把真小核收编进中簇=静默错定价而非拒绝)、恒频+边界削行 → co_witness_floor 碎裂 → >4/撞车 → freq_only、fmax 系统性偏小。TestR6FullFileCurvesSurviveWindowedCarve 只 pin 了 collected=true 的治愈形,collected=false 形零 witness。
- 放宽:collected=false 时补一条**流式频点侧扫**(不留 events,只收 freqSample;频点行稀疏,donghu 830/27843 行;prescreen 已有 fullFreqCurveRawCandidate)——与 completeTimestampOrderProof 同范式的有界二次流扫。精确、有界、不动窗口 events 语义;与「禁二次全文件重扫」的 R6 注释冲突,需裁定(该禁令本意是禁 O(cores×events),流式单遍侧扫不违本意)。

### S3【窗口误杀 Tier-2】门⑤宇宙与成员上界取「窗口 carve 内 sched_switch 观测集」,窗内全闲簇 → 锚被判「非 CPU 键」→ 整族回退
- cluster_rail_evidence.go:336-339(门⑤)+ :283-287(maxCPU 上界)+ :590-605(schedObservedCPUs 读 c.idx.Events)。
- 常见度:中(短窗 + prime/big 簇整窗 idle 是真实采集常态;大 trace 必窗口化,叠加 S2)。
- 评估:拒绝比必要更宽——门⑤本意是杀「锚指向不存在的 CPU」,窗口内没调度 ≠ CPU 不存在。
- 放宽:宇宙改取「任一事件的 CPU 归属集」(cpu_idle 的 cpu_id、事件发射 CPU 字段 [00x] 都是 typed 精确信号,idle 核恰有 cpu_idle lane)。仍是精确信号硬门,红线合规。

### S4【静默错分类】频点 lane 时间戳回滚毒化剔核后,簇计数静默降级,推导面零披露
- full_freq_curves.go:162-167、cluster_freq_share.go:430-433。
- 常见度:低-中(ftrace 多 buffer 汇流可产生;donghu 实测零回滚)。
- 后果方向恶劣:不是拒绝而是**错类**(唯一采样核被剔 → 簇消失 → 3→2 计数 → 类映射整体错位)且无 caveat(fold 的 frequencyLaneUnsafe 只护切片值,不护成员判定)。
- 放宽方向相反——应**收紧披露**:推导时若 integrity 剔除过核,typed 旗上 caveat(「cpuN 频点序损坏已剔除,簇计数可能低估」);若被剔核曾是某簇唯一采样源可考虑整判 freq_only(精确信号:剔除集非空)。零 witness。

### S5【恒频/稀变化窗】trimmed 共动 floor=2 使「恒频对+一侧被削一行」永不可并;单 burst 全 trace 触 comove floor
- cluster_freq_share.go:655-662+:693、core_capability.go:298-312。
- 常见度:全文件基下低(fast path 等长恒频照并);carve 基下(S2 命中面)中-高。
- 评估:floor 本身是 §28.5 复核 P1 裁定(停放恒频异簇假并的真教训),**不建议放宽**;正确解法是修 S2(基底全文件化后 floor 命中面自然收缩)。floor split-arm token 有 witness(cap3),head/tail 臂 token 字面无直接 pin(行为有 pin,M 系列)。

### S6【常数鲁棒性】15µs 偏斜界按单一 donghu 采集测定;成员多/发射慢的平台 burst 展宽可破界 → 同 fmax 双胞 → 撞车/碎裂
- cluster_freq_share.go:141(15e-6;实测注:成员内 ≤5µs、最紧异变迁 46µs)。donghu.ftrace 实测 8 成员簇最大对代表偏斜 8µs——余量仅 ~2×;10+ 成员簇或内核发射节奏更慢(notifier loop 内被抢占)可能超界。
- 常见度:观察项(当前两 witness 未破界)。
- 放宽:值序列全等是并簇第一判据、偏斜只是第二因子,适度加宽(如 30µs,仍低于 46µs 最紧变迁间隔)对假并的暴露增量极小;但常数是精确信号、由实测裁定钉死,改值需新平台实测 witness + 裁定,不可先斩。备选:按 trace 实测 burst 展宽自适应=嘈声信号,禁入硬门(红线),只可作 WARN 软披露(「实测 burst 展宽 Xµs 接近界」)。

### S7【fmax 撞车全表死】两簇撞车杀死全部 N 簇的判定(非仅撞车对)
- core_capability.go:403-412。
- 常见度:中-低(limits/rail 入 max() 阶梯后并列概率降;无 limits 的容器侧采集撞车概率升——恰是 S1 人群)。
- 评估:§26 类映射按簇数取表,局部撞车拆不出部分类表(2 撞车对 + 2 好簇没有合法 4 类分配),整表 fail-loud 是裁定形;splitAudit 已披露。**不建议放宽**(放宽需 §26 表扩展裁定);记录为已知代价。

### S8【Tier-2 盲区】族形门①要求共数字位 mask ≥2 成员:无数字命名(cpu_little_freq/cpu_big_freq)或单轨平台结构性永不成族
- cluster_rail_evidence.go:207-223(railFamilyMasks)+ :308-310。
- 常见度:中(厂商命名自由度大)。
- 放宽:名字语义扩展被 §7.10 终局裁定封死(name-role 红线),mask 之外的成族判据只能来自新的 typed 结构信号——现无安全放宽路径。记录,不立项。

### S9【未消费 typed 信号】cpu_frequency_limits 的 keyed 恒定锚(donghu 实测 cpu_id∈{0,4},per-policy 发射)从不参与簇结构,只喂 fmax
- 消费点仅 core_capability.go:497-519(阶梯)与 supply_fold 基准;成员判定零消费。
- 常见度:该信号在鸿蒙底座 ftrace 常在(donghu 44 行);容器侧 systrace 缺席(0 行)——对 S1 主形帮助有限。
- 评估:limits 锚={0,4} 而真簇为 {0-3}{4-11}{12,13}——锚连续推定会把 {12,13} 误并入 {4..13},**必须**过 Tier-1 结构冲突交叉验证才能用(refineDomainsWithRails 已有该臂)。可作为 Tier-1.5 候选(限「与 Tier-1 无冲突时确认 policy 边界」),但 donghu 形上 Tier-1 已自足、xxx 形上 limits 缺席,杠杆低。裁定候选,非急件。

### S10【显式拓扑部分丢弃静默】core_topology 部分条目标签不可识别时,被丢条目零披露(explicitInputIgnored 仅在「零条目获准」时置位)
- cluster_freq_share.go:193(逐条 continue 丢弃)vs :231-241(旗只护全丢形)。"big=4-7;efficiency=0-3" → 只 big 获准,0-3 无归属且无任何 caveat,违「静默换道禁止」精神。
- 常见度:低(显式拓扑本身少用;normalizeCoreClass 词表 little/small/l·middle/mid/medium/m·big/large/prime/b 覆盖主流写法)。
- 放宽:precise——解析时计数 droppedEntries>0 即披露(与 P3-4 同车道)。低风险小修。

### S11【>4 域 SoC】真 5 频域平台永远 freq_only(§26 表止于 4 类)
- core_capability.go:130-134+:370。常见度:低(现役移动 SoC ≤4 簇)。表扩展属 §26 裁定域,记录即可。

## 3. 测试覆盖图

有 witness:rail 六门+歧义+交叉验证+结构冲突 全 token(cluster_rail_evidence_cap2_test.go);<2/>4/tie/no_domains fail-loud 四臂(core_capability_cap_test.go:122);comove floor(cap3);mid/floor split-arm token(cap3);skew 10/20µs 边界(TestDeriveClusterFreqDomainsSkewBoundary);donghu 真 trace 地面真值(TestR6DonghuClusterGroundTruth / TestDeriveClusterFreqDomainsRealDonghuTrace);向上不外推、prime 披露、区间闭包、carve 存活(collected=true 形)、四窗一致性。

零 witness 的拒绝分支/形:
1. collected=false(padding 截断/seek)大 trace carve 基上的簇判定漂移(S2 核心形);
2. 「单采样簇=容器侧 systrace」真实采集形无 fixture(xxx_all.systrace 未入仓;合成 <2 臂有 pin 但无端到端措辞验收);
3. head/tail split-arm token 字面(行为有 pin,token 无);
4. 毒化剔核 → 簇计数静默降级形(S4);
5. 门⑤ 窗内闲簇误杀形(S3);
6. 显式拓扑部分丢弃静默形(S10);
7. explicitInputIgnored 旗本身(非 caveat 面)无直接单测。

## 4. 探针存档

- scratchpad/repo/internal/tracequery/zz_cluster_audit_probe_test.go — 双 witness 全量判定 dump
- zz_cluster_audit_probe2_test.go — 窗口首触建(donghu 小 trace collected=true,判定四窗一致)
- zz_cluster_audit_probe3_test.go — 预算硬拒形(窗内触顶=IndexEventLimitError 硬拒,padding 截断形需大 trace 复现)
- scratchpad/freq_samples.txt — donghu 频点样本抽取(14 核零回滚、burst 偏斜 ≤8µs 实测)
