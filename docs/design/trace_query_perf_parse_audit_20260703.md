# trace_query 性能/内存 + 确定性解析覆盖 + 投影展示审计(2026-07-03)

输入:客户 trace ×2(a.systrace 11.7KB 摘录、xxx_all.systrace 1.9MB @ ../../customlogs)、客户报告 lock_001.txt(锁竞争案因果投影)、berlin 1104MiB 事故背景;参考源(hmtrace/hiviewdfx_hiview)按需——本轮实测无未覆盖事件族,未拉取,留作未来新 kind 流程。

## A. 性能/内存审计

**已良好(勿动,防 ping-pong)**:流式 parse(bufio 256KB)+ windowed gate 在完整 ParseLine 之前预过滤 + 过窗尾 early-stop;byte 预算 LRU index cache(512MiB,parseCacheKey 含 windowKey)+ indexBuilds singleflight;字符串 interning 73 处(comm/FieldText 等);FieldText clamp 300B;250K 事件 cap + scoped 512MiB 阶梯(Gap3/C3)+ relation-scope 剪枝;bundle JSON 才整读,主 parse 无整读。

| # | 级别 | 发现 | 修复方向(确定性,无损) |
|---|---|---|---|
| P1 | 高 | **窗口前缀重扫**:cache key 含 windowKey,同文件每个新窗口都从字节 0 重新 ReadString 到窗口尾(R5 递归下钻=标准形态,berlin 案 15 调用≈8 窗口 × GiB 级前缀扫描) | per-file 稀疏锚点索引(首扫时每 N 行记录 {lineNo, ts, byteOffset},cache key 不含 windowKey);后续窗口二分定位 ≤start 锚点直接 seek,行号从锚点延续。纯延迟优化,不进任何门 |
| P2 | 中 | **cache 成本核算漏字符串**:eventSizeBytes=unsafe.Sizeof 只算 struct(string 只算 header);FieldText ≤300B/事件 + interner 内容未计,真实内存可达核算 1.5-2× | 构建时累计真实驻留字节(interner 总字节 + 每事件独有 string 长度)入 Index.MemCost;LRU 按真实成本收费 |
| P3 | 中 | **interner 无界**(plain map,per-build):病态 trace(海量独特 ≤300B payload)可撑爆单次构建 | 条目数上限(如 512K),超限直通不驻留——interning 是优化非语义 |
| P4 | 低 | **Event 结构过胖**:140 字段/70 string 字段,sizeof≈2KB;250K×2KB=单 index 可吃满整个 512MiB 预算;拖慢 parse(零值/拷贝) | measure-first:先落 benchmark+内存基准测试(本批);kind-specific side-table 瘦身是大重构,**单独裁定,本计划不做** |

## B. 确定性解析覆盖审计(逐条)

两客户 trace 事件类型全谱(21 种)对照 classifyEventType + payload 消费,**全部已覆盖**:sched_switch(含 next_info affinity/load/restricted 解析)/ sched_wakeup+_new / sched_blocked_reason(iowait+caller 符号)/ print B|E|S|F|C| 五型 / irq_handler_entry|exit→IRQActivity / softirq_entry|exit→SoftIRQActivity / cpu_idle→算力供给 / cpu_frequency+clock_set_rate(CPU 时钟判别,非 CPU 归 ClockSetRate 且被消费)/ binder_transaction(+received;dest_proc/thread/reply/flags/code)→ipc_graph / mm_filemap_add|delete→page_cache_churn / block_bio_remap+rq_issue|complete→inode IO / workqueue_execute_start|end→spans / 锁竞争 print 链(D1)。thermal/regulator/sched_migrate 辅助族已有分类。

| # | 级别 | 增强项(payload 级) | 说明 |
|---|---|---|---|
| C1 | 中 | **C\| counter 无聚合视图**:counter 已典型解析(name,value)但只可 event_search 逐条查 | window_stats 补 counter_stats:per counter name 的 first/last/min/max/delta,按 \|delta\| Top-N;客户 trace 的 Heap size(KB)/JNI Weak Global Refs/page 计数即可确定性入答案面 |
| C2 | 低 | **transact[Interface:code] span 与 binder_transaction 未 join**:接口名只在 trace_mark span,ipc_graph 边无接口语义 | 确定性 join(同线程、binder_transaction 发射时刻的开启 span 栈顶)→ ipc_graph 边带 interface 注记;需先裁定展示位 |

## C. 因果投影展示审计(lock_001 报告)

| # | 级别 | 发现 | 修复 |
|---|---|---|---|
| D-tree1 | 高 | **目标自身状态行缺"影响形态"**:树图例声称"无主导状态的行沿用影响形态",链行已实现 state-else-shape 回退(tree.go:1040-1046),但 self-row builder(runtimeTraceProjSelfRowText)从未渲染——E1/E2 锁竞争行只有 名称+112.223ms+[E1(+1)],一眼看不到"锁竞争·阻塞" | self 行接入同一回退(runtimeTraceCausalProjectionImpactShapeCell 单源);💤 sleep 行已有专属措辞,保持。per-class 排查确认:链/成因/邻近/背景/合并行均已带标签,self 行是唯一漏点 |
| D-tree2 | 裁定 | self 行是否补"关系▸影响点"(表有"自身状态▸影响点 trace_span") | **不加**:self 行紧贴 🎯 目标行,关系自明;加列徒增行宽(v3 行宽 120 预算) |

## D. eval 工件路径 + 复跑验证

| # | 级别 | 发现 | 修复 |
|---|---|---|---|
| E-path | 高 | 3 个 case 用 `../customlogs`(=repo 同级,不存在);实际在 `../../customlogs`;昨日 donghu_real LAUNCH_FAIL 即此因 | 修 2 个 HTRACE_FILE + 1 个 relative-path QUESTION → `../../customlogs`;修后复跑 donghu_real_frame_multicausal(兼作 O-7 R5d 门控呈现验证 + O 批防线真机回归) |

## E. gap 状态核对(全量开放项 → 跟踪确认)

- O-7(R5d 呈现复跑)→ **本计划 D 批完成**;O-8(write 侧读取收编 bounded helper)→ **本计划 T1 批实施**(便宜,顺带);F5-T2 行为验证 → 留代表性 eval 阶段(已 tracked);anchor obligation lane(qf_arch/s1a 失败类)→ 已 tracked(arch_stability 批计划 eval 段),**大项单独批次,非本计划**;primary-model 复跑两失败案 → 已 tracked,依赖 provider 环境。无失联 gap。

## 任务批次

- **T1(展示+路径+收编,快)**:D-tree1 self 行影响形态;E-path 3 case 修正;O-8 write 侧 os.ReadFile 面收编 ReadFileBounded(apply_patch/change_plan_validate/emit_change_plan/structured_edit/feedback/emit_investigation_complete fieldValue 扫描);D 批复跑。
- **T2(性能)**:P1 稀疏锚点索引;P2 真实内存成本核算;P3 interner 有界;P4 基准测试落盘(优化本体不做)。
- **T3(解析覆盖)**:C1 counter_stats;C2 transact join(先在本文档记录展示位裁定再实现)。

每批:实现→测试看护→`go test ./...` 全绿→commit/push→本文档进展刷新。

## 进展

- 审计落盘(本节)。
- **T1 交付(2026-07-03,f022aa67)**:D-tree1 self 行接入 state-else-shape 回退(pin:anchored 树中 subject=目标线程的 contention 行必带 锁竞争·阻塞);E-path 3 case 修正 `../../customlogs`;O-8 收编 8 处 write/scan 侧 os.ReadFile → ReadFileBounded(go.mod/package.json 等定名 manifest 保持普通读)。
- **O-7 复跑关闭(2026-07-03)**:donghu_real 双案 **2/2 PASS**(真机 1.9MB trace);输出实证 self 行标签生效(`Binder:43397_19-23088 11.103ms 候选影响`)+ R5d 门控构成如实呈现(`反转影响 · 影响构成: 可运行等待 8.307ms + 运行折算 0.000ms`)。
- **T2 P2/P3/P4 交付(2026-07-03,1a9d5bb9)**:Index.RetainedStringBytes 真实字节核算入 LRU 成本;interner 512K 条目上限(超限直通);BenchmarkBuildIndexWindowed 基准落盘(20K 行合成 ~95ms/窗口构建)。
- **T2 P1 交付(2026-07-03,3d3e8c4c)**:per-file 稀疏锚点索引(64K 行间隔,{lineNo,byteOffset,runningMaxTs}),窗口构建 seek 到严格小于 padded start 的最后锚点;discovery 预扫同享;flavor 随锚点缓存(seek 构建复用 from-0 扫描结果,flavor 变为稳定 per-file 属性);双 pin(seek 与冷构建事件集 deep-equal;时钟回退尖峰禁止越过)。**A/B 实证 3.65×**(36.4ms vs 132.9ms,262K 行晚窗口;前缀越长收益越大)。附带修正:原窗口基准/测试未设 AllowWindowedParse,实际测的是全量构建——已修,identity pin 现在真实覆盖 seek 路径。发现并修复实现期正确性洞:line+time 双窗口构建的 line-skip 分支不产 ts,recorder 无条件补 parseLineTimestamp,否则 runningMax 低报可致未来窗口越过窗内行。P4 Event 瘦身维持"单独裁定,不在本计划"(基准已就位)。
- **T3 交付(2026-07-03,d2b55c08)**:C1=CounterDeltas 加法式数值聚合(per thread+name 的 first/last/min/max/delta,|delta| Top-8;TraceCounters 原样保留零漂移);C2=IPCEdge.Interface verbatim 同线程 span-name join+边行 interface= 渲染(展示位裁定记录于此)。双双 pin 于客户 trace 形态(Heap size 三采样 delta=-1028;transact[android.net.IConnectivityManager:9] 上边)。
- **本计划全部条目交付完毕**;剩余全局开放项(F5-T2 eval 验证/anchor obligation lane/primary-model 复跑)归 arch_stability 批计划跟踪。
- **P4 Event 瘦身交付(2026-07-05,用户裁定执行)**:kind-specific side-table 落地——8 个嵌入指针组(Perf 32 字段/Constraint/Binder/BlockIO/Resource/File/Plugin/SchedStat),组指针非 nil ⇔ 事件 kind 属于该族;Event 核 1736B→688B(−60.4%,250K cap 单 index 结构字节 434MB→172MB,不再单独吃满 512MiB 预算);JSON 面字节不变(嵌入组原位促升+tag 原样,TestEventJSONSurfaceGolden 三 golden pin:全填充/EventView/稀疏 nil 组;TestEventCoreSizeRatchet 688B ratchet)。组内字段名刻意改名(PerfComm→PerfFields.Comm)使全部旧访问点编译期断裂、逐点 nil-aware 改写,禁止经促升裸读组字段(nil panic)。cache 核算:Index.RetainedSideTableBytes 入 LRU 成本;顺带修 bundle 合并不并入 child RetainedStringBytes 的既有欠账。基准(6 次中位,合成 fixture):DeriveWindowedIndex 4.53ms→2.12ms(−53%);BuildIndexWindowed B/op 45.8MB→25.3MB(−45%),时间 −6%;Anchored B/op −19%;parse 吞吐字节churn 475MB→258MB/op(−46%),时间 ±4% 噪音带内;dense fixture allocs/op 持平(无稀疏 kind)。LOC 净增:types.go +94(含复核后扩写注释 +18)/query.go +87/parse.go +71(nil-guard 展开)。全仓 77 包绿。
- **P4 对抗复核 4 项修复(2026-07-05)**:①促升禁令机械化——TestEventSideTablePromotionBan(go/types 结构规则:Event/EventView 上 selection 的 index 路径穿越嵌入指针组即 fatal,非字段名单,未来第 9 组自动覆盖;tracequery 严格 real-check,下游 tool/tool/width/hitraceconv/cmd 用混合 importer(stdlib+tracequery+logging real、其余 fake,fake 边界盲区已注明),0.8s;突变实证:注入 ev.Symbol/ev.TransactionID/view.Callchain/view.Ino 四形态 `go build` 零告警、pin 全部指认 file:line;types.go 注释扩写同名遮蔽(ev.Comm 永远绑核心字段)与双组歧义陷阱)。②稀疏族分配车道——BenchmarkParseFileThroughputSparseKinds(5% perf_sample+5% binder_transaction,print/噪音配比与 dense 对齐使 bench 差分干净)+TestSparseSideTableAllocCost 确定性差分:**7.8 allocs/稀疏事件**(bench 对互证 7.99),分解=perf 行 +18.0(几乎全为既有 perf kv/字符串车道成本,P4 新增按构造恰 +1 组分配)、binder 行 −2.5(含组分配仍比 sched_switch 行便宜)、payload 方差影响 <0.1;Δbytes≈+1113B/稀疏事件(内含组结构 PerfFields 416B/BinderFields 112B);无 alloc 风暴,storm tripwire=16(≈2× 实测)。③golden fill 加固——未处理 kind 一律 fatal([]string 注入双绿漏网已堵,突变实证红)+ 137 可序列化叶子计数 pin;测试头注 overclaim 按实况校准。④LOC 勘误如上(复核时点 types.go +76,+18 为本轮注释扩写)。
