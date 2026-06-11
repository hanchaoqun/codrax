# Eval 批 5 — structured-edit 行融合(P0 结构 bug)+ data 停滞 guard + patch 行压缩软提示

## 1. 批 5(6 案)结论

mr_pin_isolation / patch_python_typo / qf_imports / logtri_python **4 案 PASS**;两案 FAIL 经取证均为真结构 gap,修后双 PASS(**6/6 收口**)。

## 2. P0:structured-edit 合成器行融合(patch_go_typo 真根因)

**取证翻案过程**:第一轮以为是模型 diff 风格抖动(plan.json 里 `-2/+1` 把 `}` 并入语句行),给 advisory 检测器补了"闭括号合并"形态 + emit 一次性软退回。重跑发现模型走 escape lane **原样重发**——深挖日志才发现:**模型发的是结构化 edit(replace 第 25 行,content 单行),完全正确;压缩 diff 是系统合成的**。

**根因**:`compileStructuredEditsToPatch` 用 `SplitAfter` 保留行尾 `\n`,而模型 content 合理地不带尾 `\n`;splice 后 `strings.Join(newLines, "")` 把无 `\n` 的新行与下一行 `"}\n"` **字节级融合**,合成出模型从未要求的行合并 diff。schema 对 old_text 已明文容忍"末换行是引用变体",content 侧却没有镜像容差。

**修法**(单一 chokepoint,纯结构):join 前规范化——非末元素必须以 `\n` 结尾;末元素镜像原文件 EOF 换行约定。同时修通同类边角:insert_after 无 EOF 换行文件的末行融合。3 个测试钉死(中间行替换不融合 / EOF 约定双向保持 / insert_after 不融合)。

**配套防线**(对真发 unified diff 的模型仍有效):
- advisory 检测器补**闭括号合并**形态(`-stmt` `-}` → `+stmt}`),纯结构读法,排除 `{}` 字面量与含 `{` 行。
- emit_change_plan **一次性行结构软退回**:首次压缩 emission 退回并给重发指引,之后任何 emission(含原样重发)无条件接受——bounded retry-hint,按红线属软引导,永不硬卡。`MutableState.TestAndSetPatchStyleNudge` per-run 一次。

## 3. data-lane 停滞 guard(data_jsonl_filter_count 第一轮死法)

planner 卡 `mapping_candidate` 关系物化 3+ 轮零进展,stall 检测只 warning,模型不改行为,预算耗尽无答案终态。**架构缺口**:`WouldRepeatRelationNoProgress` 精确谓词只防系统自身 fallback scaffold,LLM plan 不受约束。

**修法**:新 `RelationNoProgressGuardResult` 挂入 plan 验收链——停滞状态(贡献阶段 pending + 0 贡献记录 + 近期≥2 个关系物化零进展结果)下重复关系物化的 plan 硬拒,给出全部 productive 动作词汇。信号全精确(typed action kind / typed stage / 整数计数),镜像 custom_transform_disabled guard 形态;非关系动作、有进展、低于阈值均放行(三向测试钉死)。第二轮即走通典型路径输出"2"。

## 4. harness:短标量答案被 20 字符下限拒

工作流修通后输出恰为"2"(1 字符),被 `too_short` 通用检查拒。harness 本就有 `MIN_OUTPUT_CHARS=1` 旋钮给短标量 case——`data_jsonl_filter_count` / `data_text_filter_count` 补设。

## 5. 残余(planner 专项,不点补)

第三轮 data count 又一新死法:planner 选 custom_transform 脚本路线撞 strict-output-contract 死端,终态 missing decisions ledger。注意 decisions ledger 对 filter+count 是**合理**要求(过滤决策即审计轨迹),非契约过触发;这是 data-lane planner 路径选择稳健性问题,归入已立项的 planner 引导专项。

## 6. 任务列表

- [x] structured-edit 行完整性规范化(P0)+ 3 测试。
- [x] advisory 闭括号合并形态 + 一次性 emit 软退回 + 测试。
- [x] RelationNoProgressGuard 挂入 plan 验收链 + 三向测试。
- [x] 两个 count case 补 MIN_OUTPUT_CHARS=1。
- [x] 双案重跑 PASS;patch_python_typo 无回归 PASS;67 包全绿。
- [ ] 残余:data-lane planner 路径选择稳健性(专项)。
