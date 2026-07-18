# CPU 簇结构参考解析器研究 — hmtrace / hiview 对照审计

日期: 2026-07-18。研究官产出。witness=/Users/han/opt/donghu/donghu.ftrace(鸿蒙底座+容器 Android,14 核)与同目录 xxx_all.systrace(6 核可见)。
克隆位置: scratchpad/hmtrace(gitcode diting/hmtrace,Rust)与 scratchpad/hiviewdfx_hiview(openharmony hiview,C++)。
我方现状权威: internal/tracequery/cluster_freq_share.go(Tier-1 换点共动,15µs skew+全序列值相等+≥2 样本门)/cluster_rail_evidence.go(Tier-2 keyed rail 六门)/cluster_ceilings.go(limits 只做 ceiling,不做成员)/core_capability.go(§26 校准工件车道)。

## ① 两项目认的簇相关 event/lane 全谱

### hmtrace(trace_streamer SQLite DB → systrace/perfetto 转换器)

| lane | 来源表/形 | file:line |
|---|---|---|
| `cpu_frequency` | `measure ⋈ cpu_measure_filter`,per-cpu,转写为 `cpu_frequency: state=V cpu_id=N` | src/native/extractors/counters.rs:9-38(名单在 :15-16) |
| `cpu_idle` | 同上 | 同上 |
| `cpu_frequency_limits_min` / `cpu_frequency_limits_max` | 同上(trace_streamer 把 ftrace 的 `cpu_frequency_limits: min=..max=..` 拆成两条 per-cpu 计数 lane) | counters.rs:15-16;python 参考 reference/python/db2systrace.py:473-486 |
| `clock_set_rate` | `measure ⋈ measure_filter WHERE type='clock_rate_filter'`,**任意时钟名原样透传**(测试样例 `ddr_freq`,tests/golden_diff.rs:99) | counters.rs:40-69 |
| perfetto 导出 | 只导 `cpu_frequency` → `GenericKernelCpuFrequencyEvent{cpu,freq_hz}` | src/export/perfetto.rs:740-777, 3141 |
| CPU 普查 | `infer_cpu_count` = max(cpu) over sched_slice/thread_state/raw/cpu_measure_filter + irq.callid,+1 | src/native/util.rs:199-229 |

**簇判定: 零。** hmtrace 全仓无任何把 cpu 分簇的代码("cluster" 零命中于逻辑代码);它信任 trace_streamer 的 per-cpu 归属,原样搬运。价值在于确认了鸿蒙工具链的**规范 lane 词表**: cpu_frequency / cpu_idle / cpu_frequency_limits_{min,max} / clock_set_rate 五形即全部官方 CPU 频率面。

### hiview(设备端活体采集器,读 sysfs/proc,不解析 trace)

| 信号 | 路径 | file:line |
|---|---|---|
| 核数 | `/sys/devices/system/cpu/possible`(解析 '0-N' 形) | framework/native/unified_collection/collector/impl/cpu/utils/cpu_util.cpp:60-78 |
| per-cpu 频率三元组 | `cpuN/cpufreq/scaling_{cur,min,max}_freq` | cpu_util.cpp:119-141 |
| **per-cpu 算力(dmips)** | `cpuN/cpu_capacity` → `cpuDmipses_`,直接做负载折算分母 | .../calculator/cpu_calculator.cpp:52-65 |
| per-cpu 时间片 | `/proc/stat`(11 字段),`/proc/loadavg` | cpu_util.cpp:36-57, 80-117 |
| **硬编码① 12 核=SMT** | `constexpr numOfCpuCoresWithSMT = 12; return numOfCpuCores_ == 12;` | cpu_calculator.cpp:73-77(账本钉死的反例本体) |
| **硬编码② 小核索引≤3 + SMT 折算 65%** | `maxIndexOfSmallCore = 3`、`capDiscount = 65` | cpu_calculator.cpp:100-107 |
| **硬编码③ policy0/1/2 = LIT/MID/BIG** | `TWELVE_{LIT,MID,BIG}_CPU_{CUR,MAX}_FREQ` = `/sys/devices/system/cpu/cpufreq/policy{0,1,2}/scaling_{cur,max}_freq`,+`thermal_zone1/sustainable_power`(IPA) | plugins/eventlogger/log_catcher/cpu_core_info_catcher.cpp:31-43, 106-119 |

**簇判定: 全硬编码。** hiview 从不读 `policyN/related_cpus`、不读 topology sysfs、不解析任何 ftrace event;簇形状(3 簇、边界 3/4、SMT 折扣)是产品线常量。唯一可迁移的通用机制 = per-cpu `cpu_capacity`(dmips)作为能力折算基准 + cpufreq **policy 目录本身就是簇的内核权威表述**(它硬编码 policy 编号,但 policy 抽象是对的)。

## ② 鸿蒙底座+容器场景特有 lane(witness 实测)

- **发射者身份**: 全部 cpu_frequency / cpu_frequency_limits / clock_set_rate 由 `tppmgr-sched-in-NNNN (    2)` 发射 — 鸿蒙底座 TPP 管理器,**pid=2**(宿主内核身份);容器内 Android 框架线程(aweme/app)只发射 print/binder lane。宿主/容器 lane 分界清晰可辨。
- **sched_switch 尾field** `next_info=3fff,89,3,0,2,0` — 鸿蒙独有扩展;首段 `3fff` = 14 位亲和掩码 = **nr_cpus=14 的字面见证**(cpu_idle 普查在稀疏窗漏核时它不漏)。
- **clock_set_rate 名字空间**(donghu.ftrace): `ddr_cluster{0,1,2}_freq`、`l3c_cluster{2}_freq`、`thermal_inte{1,2}`、`gpu*`、`soc_tz_boost`;xxx_all.systrace 另见 `cluster{0,1}_temp`、`l3c_cluster1_freq`。**注意陷阱: 名字含 "cluster" 的轨道全部是 DDR 通道/L3 缓存/热区,不是 CPU 簇时钟**;donghu 两 trace 均无 m3_cN_freq 型 CPU 簇时钟轨 → Tier-2 在该客户设备上结构性空手,判定全压 Tier-1。
- **cpu_frequency_limits 只在 policy 首核发射**: donghu.ftrace 44 条全部 cpu_id∈{0,4}(cpu0=16 条,cpu4=28 条;大簇 12-13 窗内无限频变化故缺席)— 内核 per-policy 发射规约,cpu_id=policy leader。
- **共发射 burst 形**: 一次 policy 换频 = 连续 N 行(每成员一行),同发射者、ts 相邻 1-2µs、同值、cpu_id 升序 — donghu.ftrace 完美三分 {0-3}/{4-11}/{12-13}(31/67/85 次换点);xxx_all.systrace 中 cpu_frequency **只有 cpu 3-5 发射**(cpu0-2 整窗静默)= 客户"经常判断失败"的实锤形状: 静默簇无换点 → Tier-1 无样本 → 不可判。

## ③ 值得补充解析的关键信号候选(排序)

| # | 候选 | 内容 | 精确性评估 | 现状缺口 |
|---|---|---|---|---|
| C1 | **共发射 burst 单次见证** | 把"一次 policy 更新=连续成员行"的 burst 结构本身当成员资格见证: 同发射者 comm/pid + ts∈skew 界 + 同值 + cpu_id 连续升序 → 一个 burst 即证成员集,不必等第二个换点 | **精确,可作硬门**(四条件全是字面逐字段判定,与既有 15µs skew 界同族);严格 burst 准入下单 burst 即可判,直接解掉"窗内只换一次频/短窗 ≥2 样本门拦死"的失败形 | cluster_freq_share.go 要求逐 CPU 时间线全序列相等 + ≥2 样本;单 burst 窗判不可判 |
| C2 | **cpu_frequency_limits 的 cpu_id=policy 锚点** | limits 事件 per-policy 发射且 cpu_id=首核(witness: 锚点集 {0,4})→ 锚点集 ⊆ 簇首集,可做分区边界下界 | **半精确**: 单事件 cpu_id 字面精确,可硬门用作"锚点处必为簇边界"的分割约束;但覆盖不全(窗内无限频变化的簇缺席)→ 只可收紧分区、禁单独定全分区 | cluster_ceilings.go 已消费 limits 做 ceiling,**cpu_id 从未进成员判定**(cluster_freq_share.go 零引用) |
| C3 | **补采 sidecar 拓扑工件**(tdkit) | 采集时随包抓 `/sys/devices/system/cpu/cpufreq/policyN/related_cpus` + `cpuN/cpu_capacity`(hiview 已示范 cpu_capacity 可读,cpu_calculator.cpp:52-65)→ Tier-0 确定性见证 | **精确,可作硬门**(设备运行时自述,非 per-SoC 硬编码,C-7 安全);依赖外部补采动作,走 core_capability.go §26 既有校准工件车道 | 无拓扑工件摄入形;tdkit 补采包已有先例(memory: TDKIT 补采包) |
| C4 | **next_info 掩码宽度核普查** | 鸿蒙 sched_switch 尾 field 首段亲和掩码,位宽=nr_cpus(3fff→14) | 宽度作核数见证 = **精确可硬门**(单字面 hex 字段);per-task 掩码值推簇边界 = 嘈声,只配软引导 | 我方核普查依赖 cpu_idle/sched 出现集,稀疏窗漏核 |
| C5 | **cluster{N}_temp / l3c_clusterN / ddr_clusterN 名字空间** | 轨道名枚举厂商簇编号空间(donghu: cluster0..2 存在 → 簇数≥3 的提示) | **嘈声,只配软引导**(DDR 通道 vs CPU 簇的语义映射是厂商约定);且必须显式反陷阱: "cluster" 子串**禁止**放进 Tier-2 CPU 轨道准入 | Tier-2 keyed rail 若按名匹配可能被 DDR/L3C 轨误喂(需核对六门是否已挡) |
| C6 | **发射者身份 provenance 标注** | tppmgr pid=2 = 宿主鸿蒙 lane;容器 Android 线程 lane 分标 | 嘈声(comm 可截断/撞名),只配软引导/展示层诊断,禁进门 | 无宿主/容器 lane 区分 |

反例确认(账本已钉): hiview 12 核硬编码本体 = cpu_calculator.cpp:75 `numOfCpuCoresWithSMT = 12`,伴生 :103 `maxIndexOfSmallCore = 3`、:105 `capDiscount = 65`,以及 cpu_core_info_catcher.cpp:31-41 的 policy0/1/2=小/中/大 常量路径 — 全部是 C-7 红线禁止的形;我方不学。可学的只有 policy 抽象(C2/C3)与 cpu_capacity(C3)。
