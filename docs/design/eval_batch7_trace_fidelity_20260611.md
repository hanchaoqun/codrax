# Eval 批 7 — trace 终答原生字段保真 + 修复回归验证(5/6 收口)

## 1. 批 7(6 案)结论

`operation_web_manual_summary`(批 6 A1 第二个 stale-spec 修复验证)/ `mr_inactive_path`(批 6 C1 多仓改动的 L1 拒绝路径,零回归)/ `patch_cpp_typo`(批 5 structured-edit 修复 C++ 覆盖)/ `qf_called_by_typed_relation_query` **4 PASS**;两 FAIL:

- **trace_query_smartperf_resources** → 真软引导 gap,修后 PASS(见 §2)。
- **data_text_filter_count** → data-lane planner 脆弱家族又一形态(deferred-rank 续航循环至 contributions missing;relation-stall guard 不适用——无关系物化)。归既有 planner 引导专项,不点补。

附:一轮 `no_result` 为本地代理瞬断(127.0.0.1:1082 拒连),基础设施抖动,重试即过。

## 2. trace 终答压缩丢事件原生识别字段(第二次出现,软引导收口)

smartperf 案:模型**拿到了** `operation=major address=0x1234`(日志 31 次出现),终答甚至点名"page_fault_user 事件原生字段为 operation/FaultHandler",却只渲染问题列举的通用维度(path/latency/bytes),把事件压缩进请求维度列表,丢掉事件自身的识别字段。与批 4 inode 案(绝对路径/措辞抖动)同属 trace 终答稳定性类,**第二次复现**,达到软引导阈值。

**修**(skill 软引导,允许带):TRACE QUERY skill 块增加一句——终答报告具体 trace 事件时,事件的**原生识别 key=value 字段**(如 page-fault 行的 operation=/address=、事件的 code/flag 值)必须随通用请求维度一起原样携带,不得把事件压缩为请求维度列表。修后重跑 PASS。

## 3. 任务列表

- [x] TRACE QUERY skill 原生字段保真软引导;smartperf 重跑 PASS。
- [x] 批 5/6 修复回归验证(operation spec #2 / mr_inactive_path / patch_cpp)。
- [ ] 残余:data-lane planner 路径稳健性(专项,形态清单又 +1)。
