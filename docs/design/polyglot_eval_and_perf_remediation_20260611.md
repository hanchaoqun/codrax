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
- [ ] 批 2:tracequery 韧性三件(行级 recover+typed 计数 / 截断与时钟回退 caveat / 窗口派生共享底层)。
- [ ] 批 3:multigraph 三件(EnsureMany 上限+singleflight / LRU 体积权重+逐出清 oracle / thrashing 切片修剪)。
- [ ] 批 4:repomap 三件(scanWalk 防环 / 缓存目录上界 / stripTypeWrappers+populateImplementers 常数优化)。
- [ ] 批 5:读管线热路(指纹增量化 / 不变段渲染缓存)+ 基准测试基线落盘。

## 4. 进度

- Fixture + 4 case 提交推送;实测 4/4 PASS。
- 性能审计完成;批 1 交付(本文件同 commit)。
