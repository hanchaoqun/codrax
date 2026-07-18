# donghu.ftrace CPU 簇判定解剖报告(trace 侧解剖官,2026-07-18)

Witness: `/Users/han/opt/donghu/donghu.ftrace`(3.46MB, 27,844 行, 无 ftrace 头, 首行即事件)
对照: `/Users/han/opt/donghu/xxx_all.systrace`(tieba 历史 witness, 1.92MB, 15,000+ 行)
引擎: main=fec890839 副本实放(`scratchpad/cluster_audit_repro/`, 探针=`internal/tracequery/zz_cluster_audit_probe_test.go`, 只存在于副本)

## 1. 信号盘点(donghu.ftrace)

**时间跨度**: 13762.791708 .. 13763.024898(≈233ms,极短窗采集)。freq 样本跨度 13762.798125..13763.024710。

**CPU 总数 = 14**(sched_switch `[xxx]` 列全谱 000–013;交叉证据:sched_switch `next_info=3fff,…` 亲和位掩码 0x3fff=14 bit)。CPU5/6 调度事件仅 6/5 条(几乎全 idle),但 freq lane 齐全。

**cpu_frequency:874 行,cpu_id 0..13 全 lane 覆盖,三簇结构字面干净**:

| 簇 | 成员 | 每 lane 样本 | 值域 kHz | burst 内散布(实测 max) | 首 burst 行号 |
|---|---|---|---|---|---|
| 小 | cpu0-3 | 31 | 558000–1530000 | 3µs(4 行/burst) | L686-689 (@13762.798125, 840000) |
| 中 | cpu4-11 | 67 | 558000–2270000 | 8µs(8 行/burst) | L3299-3306 (@13762.824115, 640000) |
| 大 | cpu12-13 | 85 | 1200000–2750000 | 2µs(2 行/burst) | L7384 (@13762.858077, 1675000) |

- burst 结构完美:30×4 行 + 66×8 行 + 85×2 行,全部成员行齐、值相等、时间散布 ≤8µs,全部在 15µs 界内(`clusterFreqDeriveMaxSkewSec`)。零缺行、零 lane 内时间回退(`durationOrderFailures=0`,integrity 零毒核)。
- 同簇 lane 值序列全等(每 lane 变化点数 25/55/63,三簇互不相等 → 不会误并)。
- **边缘发现:大簇 lane 上存在相距仅 14µs / 17µs 的两次真实变频**(cpu12: 1675000→1200000 @13762.858659,距上一变频 14µs;@13762.858599 距上一 17µs)。`cluster_freq_share.go` 注释里 "tightest distinct transitions 46µs/61µs apart, 15µs ≈3× below" 的量测基(tieba systrace)在此 trace 上失效——**真实相邻变频可以落进 15µs skew 界内**。当前靠第二判据(全序列值相等)兜底未出事,但 "双因子必须同时失效才误并" 的安全边际叙述需要修订。

**cpu_frequency_limits:44 行,仅 cpu_id=0 和 cpu_id=4 两个簇长 lane**(L8036 首行):c0 max 在 1720000/1530000 间振荡、c1 在 2270000/2100000 间振荡(热压帽动作),cpu12/13 无 limits lane。→ limits 阶梯把 c0 fmax 抬到 1720000(高于其采样最大值 1530000)。

**cpu_idle:2265 行,cpu_id 0..13 全覆盖**(1..445 条/核不等)。

**clock_set_rate 767 条 name 全谱**(逐名计数):thermal_inte1×131(cpu_id=7 键,值=中簇频点 2270000/1880000…,L142 首行)、thermal_inte2×104(cpu_id=12 键,值=大簇频点 2750000/2340000,L7380 附近)、l3c_cluster2_freq×110(cpu_id=2 键,L479)、ddr_cluster{0,1,2}_freq×330、gpu 族×84、杂项×8。
- **鸿蒙 "clock_set_rate 发簇时钟(cpu_c*/cluster*/*_cpu_clk)" 假设在此 trace 上不成立**:没有任何 CPU 簇时钟名。带 "cluster" 字样的都是 DDR/L3C(内存侧)。
- **真正携带簇频信号的是 thermal_inte1/2**(热管理镜像中/大簇频点,cpu_id 键与簇长对齐),但被 rail 六门的 ⑥ 排除词表 "thermal" 正确排除(§28.5-P3-2 裁定词)。Tier-2 keyed rail 在此 trace 上:l3c_cluster2_freq 不在排除表,但族掩码 `l3c_cluster#_freq` 仅 1 成员,过不了 ① ≥2 成员门 → 零候选族,`rail adoption: NONE, rejected={}`。
- 容器痕迹:host 侧 comm(tppmgr-*/sysmgr-*/OS_IPC_*/hilogd)与 Android 侧(.ugc.aweme.lite/binder:*/app)共存;`next_info=` 扩展列=鸿蒙底座 sched 扩展;无显式 ns 标记行。

**对照 xxx_all.systrace**:6 CPU(000–005),cpu_frequency 仅 cpu3/4/5(各 30 样本)——即 CFR #75 文件头记载的历史 donghu witness 形;无 cpu_frequency_limits;clock_set_rate 323 条(thermal_inte1×248、ddr/l3c_cluster1×45、*_temp 族),同样零 rail 候选族。

## 2. 引擎实放 verdict(副本内真实入口,production wiring = `newChainQueryCache(idx,nil).coreCapability("")`)

| 构建形 | fullFreqCurves | 域判定 | capability verdict |
|---|---|---|---|
| donghu.ftrace 全文件 | collected ✓ 14 lane | **3 域** c0=[0-3] c1=[4-11] c2=[12,13],全核入册 | **default_table** / freq_comovement;class=small/middle/big;fmax=1720000/2270000/2750000;cap=1/2.3/2.53;零降级词 |
| donghu 时间窗 100ms/10ms/20ms(AllowWindowedParse) | collected ✓(R6 规则4 旁路采集) | 同上 3 域 | 同上,**全部判定成功** |
| donghu **行窗 line 1..5000** | **NOT collected**(lineEnd 硬停扫,`windowGate.decide` 第一臂 → `buildReachedEOF=false`) | **2 域**(cpu12/13 整簇不可见) | default_table 但 **class=small/big:中簇 [4-11] 冒充 big,cap 2.53(真值 2.3),全域 fmax 1280000(真值 2750000,低估 2.15×)** |
| donghu 行窗 8000..13000 | NOT collected | 3 域(侥幸) | 判出但 fmax 低估(c2=2340000 vs 2750000) |
| donghu MaxEvents=3000 窗 | — | — | `IndexEventLimitError` 整个 build 失败(不是簇降级,是密度切窗指引) |
| xxx_all.systrace 全文件 | collected ✓ 3 lane | **1 域** [0-5](规则1 首簇闭包吞 0-2) | **freq_only(簇数 1<2)→「簇结构不可判,按纯频率比折算」**;rail 无候选,无救 |

**裁剪基失败率量化**(pre-R6 窗口面形 = 今日 fullFreqCurves 不可用时的活回退,真实引擎函数 `deriveClusterFreqDomains`+`resolveCoreCapabilityEvidence` 扫窗):

| 窗长 | 窗数 | 判出3簇 | 判出但簇数错(错类!) | freq_only 失败(其中 floor 门) | 总失败率 |
|---|---|---|---|---|---|
| 10ms | 45 | 1 | 7 | 37 (27) | **98%** |
| 20ms | 22 | 6 | 7 | 9 (7) | **73%** |
| 50ms | 8 | 7 | 1 | 0 | 12% |
| 100ms | 3 | 3 | 0 | 0 | 0% |

错类样例(比 freq_only 更糟——出错不出声):`[13762.817..13762.827] classes={c0:big, c1:small}`——窗内小簇跑 840-1040MHz、中簇恰降到 558-640MHz,**in-window fmax 倒挂 → 小簇被封大核**。`[13762.882..13762.902] {c0=[0-11]:small, c1=[12,13]:big}` —— 小中两簇窗内未变频被闭包并成一簇。

## 3. 失败机械成因排序(「经常判断失败」在此 trace 族上的候选,按证据强度)

1. **裁剪采样基(首要,量化 98%/73%)**:窗口越窄,簇的变频事件越稀(c0 全 trace 才 30 次变频/233ms;10ms 窗内多数簇 0–1 个变化点)。三条失败臂全部实测到:(a) 窗内仅一簇可见 → 簇数 1<2 → freq_only;(b) `clusterFreqComoveMinSamples`/trimmed floor(<2 对齐变迁)→ comoveFloorTripped;(c) 窗内两簇可见 → §26 二簇表 small/big 错类(中簇 cap 2.53 冒充、或 in-window fmax 倒挂封小为大)。**当前 HEAD 上此基仍活的入口**:行窗构建(lineEnd 硬停 → `finalize(!seeked && buildReachedEOF)`=false,今日实放已复现,L1..5000 即错类);anchor-seek 构建且 FullFreq 未盖章(样本 >`fullFreqCurveAnchorSampleCap`=128Ki 的大 trace 永不盖章,跨窗多次查询必踩);全文件曲线样本 >1Mi 溢出;composite bundle 中被窗口交集跳过的 artifact(parse.go:3472 显式 `collected=false`)。客户真实采集是 GB 级长窗,133 样本/233ms 外推 ≈57 万 freq 样本/16min ——**超 128Ki anchor 帽,seek 窗查询全落裁剪基**,这与「经常失败」的频度语义吻合。
2. **单簇 lane 覆盖形(xxx_all.systrace 实证)**:采集配置只让一个簇发 cpu_frequency(cpu3-5)→ 簇数恒 1 → 结构性 freq_only,频点证据面无解;此形只能靠 Tier-2/显式拓扑救,而 rail 六门在两份 witness 上都是**零候选族**(见 3)。
3. **rail 车道在鸿蒙底座上空转**:此 SoC 的 clock_set_rate 没有 CPU 簇时钟名;真携带簇频的 thermal_inte1/2(L142/L7380,值与中/大簇频点逐点镜像)被 ⑥ "thermal" 排除词正确挡掉(裁定如此,不是 bug;但意味着 Tier-2 在此机型族上恒空,单簇覆盖形无兜底)。
4. **skew 界安全边际失效(边缘,未出事)**:大簇 lane 实测相邻真实变频最近 14µs < 15µs 界(L~7440 区,@13762.858659);`clusterFreqDeriveMaxSkewSec` 注释的 "≈3× below tightest transition gap" 量测基在此 trace 被推翻,现仅靠值相等第二判据独木支撑。若两簇出现短暂同值序列 + 高频抖动变频,存在理论误并窗。
5. **非成因排除项**(盘点否证):burst 散布超界(实测 max 8µs,否);成员行缺失(867/874 行全齐,否);lane 内时间回退(0,否);≥2 样本门(全文件基 31/67/85,否);MaxEvents 预算(默认 250k ≫ 27,843,否;触限走 IndexEventLimitError 不走降级)。

## 4. 复核提示

- 全文件基上 donghu.ftrace 判定**成功**(3 簇 small/middle/big)——「经常失败」不发生在这个 233ms witness 的全文件形上;失败形集中在:老 binary(R6 规则4 是 2026-07-14 才落地)、行窗/anchor-cap/composite 裁剪基、以及单簇覆盖采集(systrace 形)。
- 探针文件只存在于副本 `scratchpad/cluster_audit_repro/internal/tracequery/zz_cluster_audit_probe_test.go`;活树未动。
