# 回访验收包(2026-07-09,交客户;新构建=main 最新)

请用新构建对原有 trace 重跑以下场景,**全量转录输出**(含末尾"系统补充：trace_query 关键观测核对"块)回传。每场景后列出预期可见变化,便于双方逐格对账。

## 场景A 滑动卡顿(berlin.systrace,原 79 系同一提问)
预期变化:
1. "IO延迟 ×8 合计11.506ms"家族行**不再**与多条"IO等待(对端 udk-irq-*)"行并列——单席,明细新增「链上并入: 链上车道 N 条同源观测已并入本行(E#…)」。
2. "数据盲区"行**不再**携带"根因排序#N"榜位;每窗榜位序数连续无洞;同线程"running 榜位行+窗内无调度数据"并存的自相矛盾消失(盲区措辞分两形:「窗内无调度数据·链止」/「窗内无≥阈值等待区间·链止」)。
3. hmfs_discard 不再以"唤醒"边挂在 OS_mmi_EventHdr(触控链)之下——跨窗假边消失,落"未接入树"诚实席。
4. 同线程两段 sleep 不再渲染为"自己成因自己"子行——合并 ×2(a–b) 单行。
5. 目标 sleep 症状行在指标表"有效归因"列为"—";"runnable 4.115ms"类词值错配消失(词随值走);"(全额)"不再孤行、"小核"不再拆行。
6. 目标自身 binder_wait 等待行在关注线程自身状态区可见(top-4+溢出披露)。
7. 全零值"×9(0.000–0.000ms)取最大"折叠行退役为一行注。

## 场景B OpenDir(东湖 record_trace_…ftrace,原 79 系同一提问)
预期变化:
1. "IO延迟 ×6"与四条"IO等待(对端 udk-irq-*)"行合并单席+「链上并入」注。
2. "页缓存抖动"行三面同值(树/指标表/明细=同一发布值),成员带"计数当量"记号并入图例;指标表邻近行"链上累计"列为"—"。
3. 持有者归因撤回披露改中文;"持有者移交链(线程):"标签消歧;锁行明细新增「等待点: <span 原文 blocking from 签名>」行。
4. 成文不再出现依常识推测的等待点(如"enqueueMessage");判定降级为"未评估"时括注含"正文…请以趋势性描述解读"。
5. 进度行"调查单元"不再出现乱码(如"事件``…")。
6. window_stats/系统补充出现 **top_io_inode** 段(events= 排序+尾行 total/shown 总组数)——该段若含文件系统层内容,即证明 hmfs 事件解析生效。

## 场景C 对比场景(原 cmp 提问)
预期变化:语义族(VerifyClass/JIT/Shader)合并参赛保持;同值背景观测双行折叠为一行+"另 N 条同值"注;窗基/±10% 披露保持。

## 场景D texture 场景(含 "Texture upload" span 的 trace)
预期变化:出现 "Texture upload" 语义类行(与 VerifyClass 同待遇:合并参赛/背景榜位/提及义务/✦ 影响形态);`H:Texture upload(…)` 形同样命中。

## 通用
- HTML 报告:中文字体与行距改善(不再回落宋体密排)。
- 所有报告:调查单元/正文无 UTF-8 乱码。

---

# 定向复查指令包(两悬案,输出片段即可定案,无需提供 trace 文件)

新构建内置 **tracediag 确定性采集模式**(§28.12):零 LLM、纯只读、不需要仓库与模型凭据,直接对 trace 跑采集脚本并生成单文件文本报告(每步行数帽默认 800/硬帽 1000,截断诚实披露;任一步骤失败退出码非零但报告仍全量)。采集脚本随构建在 `examples/tracediag/` 出货。

## G12(滑动场景,两条 14.272ms 折叠成员疑同段双归属)
```
codrax --tracediag examples/tracediag/collect_g12.yaml --trace berlin.systrace --out g12_report.txt
```
回传:`g12_report.txt` 单文件。判定点=hmfs_discard-26-562 与 oney.hmn.berlin-42591 两线程各自最长 D-state/IO 等待段的行号区间是否重合。脚本五步:两成员各一步 `pattern="prev_state=D"` 的 D 段起点行采集(**同一全区间 1017021–1625582,按 thread 过滤区分成员**——判定"是否重合"不预设区间拆分;起点行数量少,帽内必全,两成员起点行号相同⟺同一段)+ 两成员原始行上下文头部采样 + root_cause_rank(窗 6793224.9..6793225.0)对照面。若行号区间与上一轮报告有偏移,可编辑脚本中 `line_start`/`line_end` 后重跑。

## D-10(OpenDir 场景,actual 口径两面互斥)
```
codrax --tracediag examples/tracediag/collect_d10.yaml --trace record_trace_20260606064820@33863-826532969.sys.ftrace --out d10_report.txt
```
回传:`d10_report.txt` 单文件。判定点=running 窗口内时长与实际(跨窗)总时长两口径的来源与差值——window_stats 步骤输出状态统计行族(含 actual_* token 原文),thread_timeline 步骤输出逐区间双账本(`duration_ms` 与 `actual_duration_ms` 并列)。目标线程只用 `thread: "#RxComputationT-16816"` 选择子采集(引擎 pid 优先会无视 thread,故脚本不设 pid;pid+thread 同设会被 tracediag 拒绝执行)。

## 格式盲点普查(通用,建议每类新 trace 各跑一次)
```
codrax --tracediag examples/tracediag/collect_format_census.yaml --trace <你的trace文件> --out format_census.txt
```
输出全部为聚合统计+有界样本(top-N 帽+总数披露),不含业务负载明文:事件名全谱+**未识别事件名单(格式盲点清单)**、标记形普查、clock_set_rate 轨谱、调度域(prio 直方/prev_state token)、FS/IO 前缀谱与 kv 覆盖率、电源事件 CPU 集、span 普查、行级质量。这份报告直接支撑我们后续的解析扩展方向(hmfs_ 类静默漏采、键控簇轨证据均由此类普查发现)。

## 通用验收快照(可选,任意场景可复用)
```
codrax --tracediag examples/tracediag/collect_acceptance_snapshot.yaml --trace <你的trace文件> --out acceptance_report.txt
```
使用前按脚本头部注释改三处占位参数(pid/thread/window),即得 window_stats+root_cause_rank+wakeup_chain 三步快照。

## 回退方式(无新构建时)

以下自然语言命令走 LLM 管线(路由不确定、耗 token、需模型凭据),仅在拿不到含 tracediag 的新构建时使用:

### G12 回退
```
codrax -r "分析 berlin.systrace 中 hmfs_discard-26-562 与 oney.hmn.berlin-42591 两个线程在 6793222.031s 至 6793225.370s 期间各自最长的一段 D-state/IO 等待:逐条列出原始事件与行号区间,不要分析代码"
```
回传:完整输出。判定点=两线程的 14.272ms 段行号区间是否重合。

### D-10 回退
```
codrax -r "查询 record_trace_20260606064820@33863-826532969.sys.ftrace 中 #RxComputationT-16816 在 33872.289161s 至 33872.408222s 的线程状态切换统计:分别给出 running、runnable、sleep 的窗口内时长与实际(跨窗)总时长,不要分析代码"
```
回传:完整输出。判定点=running 实际总时长与线程级 actual_total 两个口径的来源与差值。
