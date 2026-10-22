# HMC279 双例完整人工审计

机器2/2通过，完整人工0/2通过；55项开放不减少。两例各一次、并行、同一冻结生产版本。确定性测试、旧稿离线重渲染、自然评测分别记证；不靠正确原生表抵消错误正文，不靠新测试倒签原失败。

## A：自然资源申请/释放统计

原问题只提供10.000–10.050秒窗口、按线程/资源类型统计次数/大小及调用位置的业务目标，不把系统护栏和JSON要求塞给用户。未改SQLite原件SHA`a938230f8ec8bceaafd8038d16058057f11407e8ce1846ee43f4e307f6d42bf5`；readonly SQL独立oracle4事件/3组。该SQLite是既有可控夹具，不冒称实机二进制。

真实入口、查询、原生表及最终HTML正确：gallery-main/TID101两次申请合计768B、一次释放512B，background/TID202一次申请128B；10.050秒右边界事件不计。Finalizer显式选择distribution原生表，summary的4事件和members的11帧由已受理展示计划自动补齐。事件—帧绑定、原深度及未知状态保留通过，不包含调用顺序解释；无图，不借表格验收代签自然图。

整答FAIL：把depth0说成入口/最深已记录帧，把depth2说成最深业务帧；释放事件缺depth1仍把depth2称“上一层”，超出记录深度提供的证据。Finalizer已有原深度、缺口、重复、unknown和非有序调用链边界，末尾限制不能抵消正文。属于已有解释越权，不再重写正确事件统计器。

过程另有两处交接欠项：4次complete提交的真实状态为DOWNGRADED1、成员缺字段拒绝2、accepted1，摘要计数不能统称4次reject；最后units-only受理，但七个解释单元未见实际Finalizer INIT，不能宣称unit端到端自然通过。原生三个视图到答案成功是独立路径，不混记。两者留16.4/18.3，并优先验证共享接缝而非case特例。

## B：真实sample-only及HTML身份

完整PERFILE2原件30,062,585B、SHA`da713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632`未改。实际目标窗164记录/259cycles权重；内核IP与OnDataEvent各82，权重174/85、CPU1/0；完整包络、独立分母和原件→准备成员来源均正确。原生表明确“样本类型未知、执行状态未确立”。

整答FAIL：开头和解释段把CPU事件推成“只在线程真正占用CPU运行时产生”，把停顿定义为off-CPU；尾部把200ms分析窗口写作200ms已证停顿。缺调用者帧/采集告警也不能直接解释内核符号缺失，未作源码调查不等于已证明源码不存在。原生事实已经充分到达Finalizer，继续留18.3，不第三次追跑，不增加问答原文硬门。本轮没有再写旧124ms间隔，不误记未出现的旧问题。

共享HTML修复本轮自然命中：同一callback既出现在代码样式，也出现在自由粗体列表及系统Perf事实段，完整`OHOS::sptr<OHOS::IRemoteObject>`保留，不再产生伪自动链接。MD原字节未重写，XSS/普通链接/代码/Mermaid边界由独立公共回归证明。只检查生成HTML正文源，未冒称浏览器截图视觉验收；本轮无Mermaid图，不扩签自然图修复。

Trace因果投影仍使用用户指定窗，显示缺链而不把背景当主因，自动补齐运行。原生事实对照和设备元数据保留，未知sample_kind、假定时钟和权重非耗时边界未丢。多次同源采样总体重复展示另留去噪欠项，本轮未累加为新总体。

## 收据与审计完整性

原v1全仓唯一旧两视图断言失败，先封存，再做test-only精确三视图集合迁移，生产/用例/CLI不变；后继测试不回签本批两份整答。A正式进程收据中`original_sha256`继承公共runner的B二进制参照字段，不能误称A原件；实际A路径/SHA以上述日志与独立oracle为准，原收据保留并披露字段含义缺陷。

独立审查曾误占实现报告的`qualified-identifier-v2.md`路径，已另存独立报告并恢复实现报告，碰撞说明保留，不假称从未覆盖；不涉及源代码、评测输出或原件。所有日志、实际emit参数、恢复状态、准备产物、最终MD/HTML与原始失败按SHA封存。

- [A逐份审计](results/hmc_resource_surfaces_identity_20261021_v1/verification-receipts-v1/process/codrax-hmc279-A-manual-v1.md)
- [B逐份审计](results/hmc_resource_surfaces_identity_20261021_v1/verification-receipts-v1/process/codrax-hmc279-B-manual-v1.md)
- [B真实HTML核验](results/hmc_resource_surfaces_identity_20261021_v1/verification-receipts-v1/process/codrax-hmc279-B-html-manual-v1.md)
- [实现、验证与后续ROI](parallel_selected_summary_hmc_resource_surfaces_identity_20261021.md)
