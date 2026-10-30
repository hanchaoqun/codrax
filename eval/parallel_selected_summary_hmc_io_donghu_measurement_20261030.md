# HMC321：IO 与真实 Donghu 启动实例——失败代次封存

持续 main，基线 `d14d2c24fb343e685c282f3e70da982e14c431cc`。**本代未单独提交/发布，不能作为通过版本。** 11项源变更保留为失败冻结快照，由[HMC322](parallel_selected_summary_hmc_endpoint_sample_registration_20261030.md)修复回归后以最终版本发布，不回签本代结果。

79唯一任务＝27已交付＋52开放，18类；完整能力新增0，无稳定ID销账。机器smoke不替代人工验收，两个进程exit0不覆盖全仓失败。

| 侧 | 固定输入/预算 | 进程与原机器 | 完整人工 |
| --- | --- | --- | --- |
| A `trace_io_inflight_overview` | 原1839B authored ftrace；15步、n1 | exit0 / PASS | FAIL，08.3开放 |
| B `donghu_v105_yuanbao_activity_instances` | 真实105版元宝原件；原case、各一次 | exit0 / PASS | FAIL，04.3开放 |

两侧实际请求11/59对，无配对歧义，最大可证重叠22.855秒；没有第三例或失败后的追绿重跑。B是真实设备ftrace文本，不是二进制格式验收；另一个100版抖音极速版新增case仍NOT_RUN。

## 实现意图与失败原因

参考仓 `io_ops.py`/`io_latency.yaml` 的桶统计用于定位活跃区间，不是精确在途深度积分；保留本项目半开配对sweep、全窗分母及request·ms。`marker_ops.py` 的同线程实例、父子和self启发本项目直接子区间并集与状态交集，不能照搬缺值补零或同前缀取最后。参考关键词归并不迁移为用户/模型原文硬门。

本代统一三处系统算术教学：只允许有来源、同量尺、可证集合关系的派生值，不加根因席位或物理对应权限。业务区间、IO在途、线程/CPU频率时序的原生producer将真实查询端点交给共享标题，不由notes、成员包络或用户问句猜端点。raw权限allowlist不扩大。

定向检查通过后，唯一冻结全仓暴露两个顶层失败：

- `TestRuntimeMeasurementHandoffShortProviderNotes` 三子例：合法删除可选表说明后，整记录端点凭证失效，共享标题退回未知。**这是实际新回归，阻止本代发布。**
- `TestHMC283StatisticsSourceWindowUnchanged`：旧字符串要求端点未知，而原生IO已真实发布 `[1,1.01) s`；数值/分母/成员无变化。不能把这个过时文字pin与前项真实回归混为一类，后继须保原数值断言并准确更新协议期望。

冻结后没有修本代代码或重跑本代full。后继HMC322独立公开RED、删除式投影修复及新冻结，见对应文档。

## 真实材料与成文发现

用户指定目录的真实原件已实际使用：
`/Users/han/opt/donghu/基线数据/105版本/元宝/容器重启后冷启动/1、LOG_ftrace_20261019_17.17.44/ftrace-20261019_17.17.44.ftrace`。
63,659,046B，412546行，SHA `be59c0648ac7e1d8a2d117f5dc36efa0c92b5f3686671719ada435c16713a20c`。operator独立整数微秒标尺不送模型；原件不复制进Git。

A四组在途summary、成员与时序正确，九张来源标题均保半开窗，漏选成员表由自动补齐保留。正文仍把浅在途解释为无积压/正常，把完整驻留4–6ms与窗内贡献2ms混用，把窗内6次发起与相交4个完整配对说成子集；旧selector还绝对禁止跨组小计，与新系统许可冲突。一次错误JSON patch未staged，不能说修补成功。

B自然取到两个activityStart和主要子阶段，header TID/TGID46571不混marker payload PID2227。完整inclusive为59.695/111.487ms，self为6.770/3.938ms；后者已在原生结果却未进入三份成文INIT，窄窗合法self0被配到完整111.487ms。12条已证IO共1.350ms却被配上前6条的预览包络；另0.006ms D的IO属性未知被共享标签写成非IO。重复两张836行全窗表、caller/等待概括越权继续留账。图均未触发，不新增图退出门。

## 冻结与唯一全仓

CLI只构建一次，SHA `d4838ed02c867a5eee582fab6c14168fb1f02fd624b20a006b46abd5a9cce496`。冻结7529仓库/185外部/23仓外快照，SHA `eecfc0614266bcb36ddc6447e4dc2ad706b32c3423ea717f1ebe54a91c0ed241`，后置完整性通过。

唯一full实际exit1：97包PASS、1包FAIL、12无测试；28592顶层PASS/2FAIL/43SKIP，33984子例PASS/3FAIL/14SKIP，零非JSON/重复或未终态。日志SHA `a7a09ca49dd14743b0ad1655d2ff7eb9cf7a20ab8348a70d3363b6773f4ea966`。不能以定向绿或A/B进程成功消掉该失败。

528件[封存索引](results/hmc_io_donghu_measurement_20261030_v1/sealed-index-v1.json)，SHA `bc54772053efb86af5024269041959f7af35f189b0f3094a9079b84de0877dc9`。原始失败、INIT、参数、完整payload、正文表格和独立审计均保留；原件及同字节全文副本只引用登记，派生材料不按文件大小删减。HTML仅源审，不冒像素QA；凭据不读/哈希/复制。

详见[完整人工汇总](parallel_selected_summary_hmc_io_donghu_measurement_20261030_manual_audit.md)。本代没有publication；最终main实现与7份文档的提交/推送收据归HMC322，WAL仍生产NO-GO。
