# HMC262 人工审计：ZIP接入与采样解释分别判定

机器2/2；限定人工1/2：A完整答案FAIL、普通命名ZIP接入PASS；B只读登记终态PASS。79唯一＝24已交付＋55开放保持，17.6及18.3不整体销账。审计是原始载荷、上下文、MD/HTML内容和结构化终态复核，不冒称浏览器像素验收。

## A：正常ZIP成员的真实准备与采样答案

原件：[A运行](results/hmc_zip_native_ingestion_20261018/trace_zip_binary_named_member-20261018-232955/)。独立全文归档`verification-receipts-v1/codrax-hmc262-sample-manual-audit-v1.md`，SHA`f3547b6505d0142b5d58c19b40d597ea286d07242097bf15ccb09ead1f7d0de9`。

| 范围 | 人工结论 |
|---|---|
| 普通`.data`成员、共享准备、完整来源 | PASS：完整30,062,585B成员，保外层/成员双重身份，12000记录非预览替代全集 |
| 独立统计 | PASS：200ms窗1393记录，目标164条/259cycles；两热点各82条，权重174/85，共同分母259 |
| CPU和时间 | 最终正确：CPU1内核地址、CPU0传感器回调；约1.5ms是各自记录包络，不是执行时长 |
| 栈帧 | 两个原生exact_frames均frame_count=1；支持有限已记录路径，不证明完整unwind或上游调用关系 |
| 整体正文 | FAIL：两次把未知类型的内核IP升级为确定“执行系统调用” |
| 图及根因投影 | 未触发；没有图，root-causes为`trace_root_cause_contract_not_active`，不冒签图或链筛 |

实际4次trace_query：首次tree错误带pattern，工具typed拒绝；event_search自动补目标和窗得164/预览40，再查完整164；perf_stats得164/259与两个精确单帧路径。首次不可用返回的0不是测得零样本；没有自然重试tree成功。不能仅凭工具名判本题缺交付，也不能签tree自然入口通过。

最终MD L28“仅知线程在内核态执行系统调用”、L30“仅能确认线程在内核态执行系统调用”；HTML可见正文、primary/principal同错。MD SHA`6fed95af6b6c7574859cb28d82917efac02360e3e723befa41f36f9b16d15a77`，HTML SHA`f01e2fb2bfec21f93417b0da8003761a6b81f9aabe184ba33ab356635514d05d`。正确原生表的unknown、执行状态未确立及非因果边界不能抵销正文错误。

过程CPU0误述已被最终答案纠正；本次没有把174/259数值写成实际耗时，也没有4ms执行/196ms停顿，不沿用旧错误。新问题不问停顿，不因未给等待根因扣分，亦不替代旧18.3退出条件。传感器符号可提供业务排查线索，错误是从地址/DSO归属推已发生执行状态，不是禁止业务语言。

上下文已供齐unknown、两CPU、窗口、分母、权重及exact_frames；独立调查解释仍含错误CPU/确定系统调用，作为单份advisory进入finalizer，旧reason没有重复回流。HMC261去重没有退化，但不能保证语义正确。原生视图2/4完全发布边界如实披露；不得把这点泛化成热点事实未到场。最终纠正CPU而保留系统调用，说明解释与事实权限冲突仍在，不足以认定纯随机波动。

最高上下文估算110822 tokens、explorer10轮、finalizer2轮是成本观察，不设新硬门。运行时调用栈仍触发soft源码提示、同源质量/模型表与原生表有重复，均归既有16.4交接降噪，不加重复任务或逐句关键词规则。完整raw.log保真实finalizer prompt，未重建摘要冒充原件。

## B：只读登记的历史原件、真实新执行与完成说明

原件：[B运行](results/hmc_zip_native_ingestion_20261018/native-registration-natural/)。独立全文归档`verification-receipts-v1/codrax-hmc262-registration-manual-audit-v1.md`，SHA`83bac76d10e25b7b758c7da3b826375a8d2377a0573eb30a01b0997a36153d09`。限定PASS：实际父controller/planner/独立恢复子与原生RunTests，operator提供初始实现/合同/历史运行；不是完整CLI或从零修复源码。

父、子留存各7件：3个精确验证报告、2个current alias、2个真实最终说明。逐件类型/计划/路径/字节SHA匹配；3个VerifyRef和2个final链接准确。旧source final仍in_progress/weak，绑定`85d5c7e68b00464a4d44e28307bc5083f58040eddf207bba71635a85654af9be`；当前final为complete/verified，绑定`ba4f37617769b5639514a2fdc4ab21bb625d82e81d7e8fbcc44676b0a2c65099`，不会按同PlanID最新结果偷换历史。中间报告`b91dda4684ca1e474e80c33ad2313ce9541043fb9f7fca896439b3534aca3452`也保留。

| 量尺 | 实际结果 |
|---|---|
| 当前验证报告/最终说明 | 1条断言结果、passed1/failed0，2条命令 |
| 累计证明 | 2份贡献报告、2条断言结果、4条命令；不是2个不同测试 |
| 独立原生执行 | operator准备1次＋工作流3次；14件归档不是14次执行 |

恢复子PID39280确实新执行后完成，不复用历史成功；源码/测试/配置/依赖及HEAD不变，6个上下文包无重复保存引用累积。两次过早accept_unverified均被确定性控制层挡住并返回探索/计划，最终完成不靠放宽证明。18.4本轮限定合同通过，不代签全部框架、GUI或多语言。

## 交付口径

完整能力新增0；生产子能力1组；既有只读终态轮换验收1组；剩余人工缺陷1组含于55。完整原始失败、独立报告和实际查询/最终文件统一归档。后续优先多成员精确选择的共享身份接入，采样语义失败继续留账，不追加第三例追绿。全仓与发布见[批次汇总](parallel_selected_summary_hmc_zip_native_ingestion_20261018.md)和统一账本§262。
