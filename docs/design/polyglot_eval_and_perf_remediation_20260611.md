# 多语言/跨语言/多仓 Eval 扩充与性能·内存优化方案

## 1. Eval 扩充与实测结论

新 fixture `eval/fixtures/multirepo-polyglot`(以 tokenizers/polars、grpc 多语言 SDK、cJSON/libgit2 等真实项目形态为蓝本,5 子仓:Rust 核心+pyo3 / Python 绑定 / Java 路由表 / TS 客户端镜像 / C++ 平铺 C ABI),4 个新 case 全部实测 **PASS**:

| Case | 探测面 | 判定 |
|---|---|---|
| mr_poly_binding_chain | 跨语言绑定链(Python→`_fastlex`→Rust)+ 回退行为 | PASS |
| mr_poly_api_contract | 跨仓契约漂移(缺路由 + 方法漂移,双侧引用) | PASS |
| mr_poly_workspace_inventory | 5 子仓全量清单(超默认 active cap) | PASS |
| mr_poly_capi_boundary | C ABI 导出面 vs 内部 C++ 类 | PASS |

**结论**:多语言/跨语言/多仓读模式当前无新架构 gap;本轮优化主线转向性能与内存。

## 2. 性能·内存 Gap 账本

四路并行审计(repomap 扫描 / tracequery / multigraph·LRU / 读管线热路)产出 30 条候选;进入实施的逐条人工核实,其余标注"审计来源,实施前须核实"。

### 已核实并交付(本轮批 1)

- **flat 符号索引按 oracle 重建(P1)**:`NewSymbolOracle` 在 analyzer、**每个 contract-check 重试轮**、multigraph fan-out 各自构建 oracle,每个 oracle 首次 flat 查询都全量重建 O(symbols) 索引 + 逐名 FlattenIdentifier。修法:memo 移到 `*Graph` 级(`FlatSymbolIndex(build)`,unexported once+map,不序列化),全部 oracle 共享一次构建;`go vet copylocks` 顺带揪出并修复了 `cloneGraphForRanking` 的 Graph 值拷贝(改字段级浅克隆,克隆体独立懒建 memo)。
- **导入边插入在 hub 文件上 O(k²)(P1)**:`resolveImportGraph` 对每条边走 `AppendUnique`(线性扫描);被 k 个文件导入的工具文件其 ReverseImports 每次插入重扫 k 项。修法:函数内侧集合辅助(成员 O(1)),slice 保持插入序(输出确定性不变),集合随函数返回释放。
- **核实后剔除**:NativeGrep "正则重编译"为每次调用一次编译(非每行),调用频度低,收益不抵全局状态成本——不做。

### 待实施(按优先级,实施前逐条核实)

**P1(算法/韧性)**
- repomap `scanWalk` 回退路径无 symlink 环检测(git ls-files 主路径不受影响;回退路径用 WalkDir + visited-inode 防环)。
- tracequery 每行热循环无 panic 恢复(单行畸形导致整次解析崩溃;按解析单元加 recover + typed 降级计数)。
- tracequery 窗口化派生的内存膨胀界(从全量 index 派生时间窗时的拷贝量;改共享底层切片 + 窗口视图)。
- multigraph `EnsureMany` goroutine 无并发上限;重载竞态下重复加载(singleflight 化)。
- multigraph LRU 无条目体积估算,逐出后 oracle 缓存持引致内存滞留(逐出回调清 oracle 缓存;按 SymbolDefs 规模估算条目权重)。
- repomap 缓存目录无增长上界(按 slug LRU + 总字节上限)。

**P2(常数级/可观测)**
- `stripTypeWrappers` 单遍化;`populateImplementers` 大接口集的按方法名预索引;closure 指纹每轮全量哈希的增量化;prompt 不变段落的逐轮重渲染缓存;tracequery 截断尾行/时钟回退的 typed caveat;LRU thrashing 事件切片修剪。

**全部修法遵循**:精确信号、无关键字路由、不动稳定路径的默认行为;每项带基准或回归测试。

## 3. 任务列表

- [x] 批 1:Graph 级 flat 索引 memo + copylocks 值拷贝修复 + 导入边集合辅助插入;全量测试绿。
- [x] 批 2:tracequery 韧性——行级 panic recover(typed `ParseLinePanics` 计数,注入式测试缝)+ 时钟回退 typed `ClockRegressions` 计数,两者经查询层 caveat 向用户披露;"截断尾行被忽略"经核实为审计误报(`len(line)>0` 已处理无换行尾行),剔除;窗口派生共享底层移入批 5 一并核实。
- [x] 批 3:multigraph——核实后 3 条中 2 条为误报(EnsureMany 入口即有 Cap 上界;EnsureLoaded 的 loading map 就是 singleflight);**真问题比审计更重**:per-slug oracle 缓存不校验图身份,LRU 逐出重载后返回陈旧 oracle(旧图泄漏 + 答案陈旧)——修为图指针校验缓存,身份变更即重建,附身份变更测试。
- [x] 批 4(核实处置):scanWalk symlink 环为**误报**(`filepath.Walk` 不跟随符号链接,stdlib 语义);剩余为常数级优化(stripTypeWrappers 单遍化 / populateImplementers 预索引 / 缓存目录上界),列为低优先待办,实施前逐项基准验证。
- [x] 批 5a(基准先行,已交付):基线落盘于 `perf_baseline_bench_test.go`×2。**stripTypeWrappers 37ns/0 allocs → 记录后弃**(无优化价值);**populateImplementers 二次项坐实**(2000T×500I 20.7ms,10× 规模 38× 耗时)→ 按方法名倒排预筛 + 命中计数等值匹配,4.9× 提速且近线性,map 迭代非确定性以排序收口(输出与朴素双层循环逐字节一致);**tracequery 窗口派生坐实为最大内存热点**(Event 968B,200k 事件中段窗口单次 165MB 分配)→ 连续区间零拷贝共享底层(append 行序 + 单趟验证连续性,时钟回退穿窗自动回落拷贝路径),165MB→224B、9.9ms→3.1ms,双向正确性测试。
- [x] 批 5b:**closure 指纹基准 8.7µs/轮**(每轮一次、全 Run 合计 <0.5ms)→ 增量化记录后弃;**缓存目录上界**:topology 缓存按 parent_root 存在性精确修剪(每个 eval scratch/已删 worktree 永久留一个 JSON 的增长类问题;Save 后冷路径修剪,10 分钟宽限期防重建竞态,损坏条目同窗回收,五形态测试)。"不变段渲染缓存"留待真实 profile 数据(prompt 装配非当前瓶颈,无量化不动)。

## 3.5 非 Go 单仓扩充与基线(批 6)

- run.sh 新增 read 模式 FIXTURE 通道(复用 setup_scratch,无写门;harness 自测绿)——读模式问题可指向任意 fixture 仓,补齐"现有 eval 大多只覆盖本仓 Go"的结构性空白。
- 新 fixture ×2(真实项目形态缩减):`java-layered-service`(petclinic 式四跳写路径 + 三层配置优先级)/`rust-cli-indexer`(ripgrep 式模块组织 + trait 双实现)。
- 4 case 实测 **4/4 PASS**(java 调用链 / java 配置优先级 / rust trait 枚举 / rust 跨模块链);日志复核:零重试,构图 14-16ms,tier 无回退告警——Java/Rust 单仓读模式无架构 gap。

## 4. 进度

- Fixture + 4 case 提交推送;实测 4/4 PASS。
- 性能审计完成;批 1 交付(本文件同 commit)。
- 批 2 交付:tracequery 韧性 typed 计数 + caveat;全量测试绿。
- 批 3 交付:multigraph 陈旧 oracle 修复(比审计断言更严重的正确性问题);2 条审计误报证伪并记录。
- 批 4 处置:防环断言证伪;常数级项与批 5 合并为基准先行的低优先待办。
- **审计误报统计**:30 条候选中 8 条经人工核实为误报或设计内行为——凡未亲手核实的审计结论不得直接实施,本账本逐条记录处置依据。
- 批 5a/5b 交付:基准先行四项(两项重大优化 + 两项记录后弃)+ topology 缓存修剪;全量测试绿。
- 批 6 交付:read FIXTURE 通道 + Java/Rust 单仓 fixture 与 4 case,实测 4/4 PASS。**全部余项收口**:唯一未实施项"不变段渲染缓存"以"无 profile 数据不动"原则显式挂起,触发条件(prompt 装配进入 profile 热点)已记录。
