# extractAnswerSymbols enumeration completeness gap

**Status**: **UNRESOLVED** — not blocking 5e695d7 ship, surfaces as a
stochastic t2 quality floor
**Discovered**: 2026-04-14, during post-P1.1 grid validation (Option D)
**First observed HEAD**: `5e695d7` (but pre-existing — see §Historical
baseline)
**Related code**:
- `internal/agent/erm.go:1677 extractAnswerSymbols`
- `internal/agent/explorer.go:1649` (call site)
- `internal/context/builder.go:213-232` (rendering + authoritative
  directive)
**Related memory**: `project_answer_symbol_extraction_audit.md`,
`project_df3_push_strategy_enumeration_gap.md`,
`project_fake_green_audit_2026_04_14.md`
**Related roadmap item**: `docs/architecture-root-cause-remediation.md`
§6 P1.3 (AnalysisIR → QualityGate) — closest existing slot; may also
warrant a dedicated P2.x entry

---

## TL;DR (EN)

For enumeration / list-of-symbols questions (e.g. t2 "项目中所有实现了
Evaluator接口的具体类型有哪些?"), `extractAnswerSymbols` commits the
finalizer to a **partial allowlist** whenever the LLM-extracted
evidence happens to contain terminal literals for a *subset* of the
true answer. There is no completeness check. The pipeline then renders
that partial allowlist with a hard directive — "You MUST NOT add or
remove symbols; your answer lists EXACTLY these symbols, no others" —
and the finalizer faithfully outputs k of N items.

On 2026-04-14 07:50 HEAD `5e695d7` this produced a **2/9** answer for
t2 despite the explorer finding all 9 Evaluator types. This is not a
regression introduced by P0/P1.1 — it is a structural flaw whose trip
rate depends entirely on LLM-side variance in Phase-1 evidence
extraction, and the pre-ship runs were only "9/9" when the gate happened
to stay open by luck (L0-2=0 case).

## TL;DR (中文)

枚举类 / list_of_symbols 问题（例如 t2 "项目中所有实现了 Evaluator 接口
的具体类型有哪些?"），当 LLM 恰好为 N 个答案中的 k 个提取到 terminal
literal 证据时，`extractAnswerSymbols` 会把这个 k 项子集当作
"deterministic, authoritative" allowlist 交给 finalizer，附带强指令
"You MUST NOT add or remove symbols"。finalizer 于是忠实输出 k 项，
丢弃 Prior Stage Findings 里已经完整存在的 N 项。整个过程没有任何
enumeration 完备性 gate。

2026-04-14 07:50 HEAD `5e695d7` 的 t2 run 答 **2/9**。不是 P0/P1.1
回归 — 代码路径未被 5 个相关 commit 任何一个触碰。审计期 01:32 的
"9/9" 是 `extractAnswerSymbols` 恰好返回 0 的运气，**不是确定性修复**。

---

## Evidence chain (e2e traced)

### Step 1 — Explorer finds all 9 types (correct)

Current 07:50 finalizer user message (`eval/results/t2-20260414-075035/
run-1.logs/codrax-*.log` around the `[diag finalizer] INIT msg role=user`
block) contains a complete Prior Stage Findings section listing:

```
1. plannerEvaluator          — internal/agent/planner.go:20
2. codeReviewerEvaluator     — internal/agent/code_reviewer.go:19
3. implementerEvaluator      — internal/agent/implementer.go:18
4. subExplorerEvaluator      — internal/agent/sub_explorer.go:148
5. explorerEvaluator         — internal/agent/explorer.go:665
6. analyzerEvaluator         — internal/agent/analyzer.go:132
7. designReviewerEvaluator   — internal/agent/design_reviewer.go:19
8. finalizerEvaluator        — internal/agent/finalizer.go:152
9. verifierEvaluator         — internal/agent/verifier.go:18
```

Each with its correct ShouldStop condition. Grounding is complete.

### Step 2 — extractAnswerSymbols prunes to 2

At `internal/agent/explorer.go:1649`, after the Phase 1 evidence-
collection finishes, the pipeline calls:

```go
answerSymbols := extractAnswerSymbols(
    strictAnswerItems,
    irQuestionKind(ctx),   // "enumeration"
    questionText,
    irAnswerShape(ctx),    // "list_of_symbols"
    ermGraph,
)
```

The log records:

```
[explorer] L0-2 extracted 2 answer symbols
  answer_symbol[0]: subExplorerEvaluator (internal/agent/sub_explorer.go:148)
  answer_symbol[1]: finalizerEvaluator   (internal/agent/finalizer.go:169)
```

The function (`internal/agent/erm.go:1677`) has the following gate
order for a list_of_symbols enumeration:

1. If `questionKind == "mechanism"`: return nil (the df3 fix)
2. If `!hasTerminalEvidence(items)`: return nil
3. Phase 4 gate: if RoleTerminal + list_of_symbols + hasTerminalLiteral,
   promote to RoleAnchor
4. For each item, call `answerSymbolFromEvidence`, deduplicate by name,
   return the list

**None of these gates asks "is the resulting list plausibly complete?"**
If the strict evidence subset happens to contain exactly 2 items with
a terminal-literal shape matching the Anchor pattern, the function
returns a 2-item allowlist — regardless of whether the true answer
has 9 items.

### Step 3 — Authoritative directive forces 2 into the finalizer

`internal/context/builder.go:213-232`:

```go
if len(ac.AnswerSymbols) > 0 {
    var symContent strings.Builder
    symContent.WriteString("The deterministic pipeline has already " +
        "identified the answer to this question. Your task is to " +
        "render these symbols as prose. You MUST NOT add or remove " +
        "symbols; your training-data recall is irrelevant here.\n\n")
    for _, s := range ac.AnswerSymbols {
        fmt.Fprintf(&symContent, "- **%s** (%s:%d)\n", s.Name, s.File, s.Line)
    }
    symContent.WriteString("\nStrict rules:\n")
    symContent.WriteString("1. Your answer lists EXACTLY these symbols, no others.\n")
    symContent.WriteString("2. For each symbol, cite its file:line if provided.\n")
    symContent.WriteString("3. If a plausible-looking name is not in the list above, it is NOT part of the answer.\n")
    pc.UserSections = append(pc.UserSections, types.PromptSection{
        Title:   "Extracted Answer Symbols (deterministic, authoritative)",
        Content: symContent.String(),
    })
}
```

This block is unconditionally appended whenever `AnswerSymbols` is
non-empty. The section header calls itself "deterministic,
authoritative" — the LLM has no way to know that the 2-item list is
actually a partial slice. The "MUST NOT add or remove symbols" clause
is a hard constraint.

### Step 4 — Finalizer obeys

Current 07:50 run:

```
iter=0: 在项目中实现了 Evaluator 接口的具体类型有两个：
        subExplorerEvaluator 和 finalizerEvaluator. (478 chars)
[finalizer] S3 correction #1: out-of-list symbols: [Evaluator Evidence sub_explorer ShouldStop]
iter=1: 在项目中实现了的具体类型有两个：subExplorerEvaluator 和 finalizerEvaluator. (314 chars)
[finalizer] S3 correction #2: out-of-list symbols: [Evidence sub_explorer]
iter=2: 在项目中实现了的具体类型有两个：subExplorerEvaluator 和 finalizerEvaluator. (110 chars)
[finalizer] S3 retries exhausted (2), accepting response
```

The S3 retry loop (the `c92068b` symbol-allowlist validator, NOT
P0.2) strips every out-of-list token across iterations, converging
to the bare 2-item answer.

---

## Historical baseline

Across 8 historical single-run t2 executions (all at different HEADs,
most pre-dating P0/P1.1), the L0-2 extracted symbol count and final
quality distribute as:

| Timestamp | HEAD epoch | L0-2 count | Extracted symbols | Final |
|:---|:---|:---:|:---|:---:|
| 2026-04-13 08:31 | pre-audit | 0 | nil | 0/9 |
| 2026-04-13 08:45 | pre-audit | 0 | nil | **9/9** |
| 2026-04-13 09:57 | pre-audit | 3 | BuildInitialPrompt/ContinuationPrompt/ShouldStop | 0/9 |
| 2026-04-13 13:52 | pre-audit | 4 | ShouldStop/finalizer/subExplorer/explorer | 3/9 |
| 2026-04-13 15:43 | pre-audit | 3 | BuildInitialPrompt/ContinuationPrompt/ShouldStop | 0/9 |
| 2026-04-14 00:13 | pre-audit | 4 | ShouldStop/finalizer/subExplorer/BuildInitialPrompt | 2/9 |
| 2026-04-14 01:32 | audit `133973d` | 0 | nil | **9/9** |
| 2026-04-14 07:50 | post-P1.1 `5e695d7` | 2 | subExplorer/finalizer | 2/9 |

**Key observations**:

1. `extractAnswerSymbols` output is highly stochastic across runs —
   0, 2, 3, or 4 items with different symbol sets each time, including
   method names (`BuildInitialPrompt`, `ContinuationPrompt`,
   `ShouldStop`) that are not even the answer.
2. All **9/9** runs correspond to `L0-2 count = 0` — the gate
   happened to stay open. The finalizer then freely enumerated all
   9 types from the Prior Stage Findings block.
3. The `133973d` "t2 2/9→9/9" claim from the fake-green audit
   (`memory/project_fake_green_audit_2026_04_14.md`) is an N=1
   observation that coincided with the L0-2=0 lucky variance, not
   a deterministic fix. The grounder change in `14e2c07` helped
   remove some wrong-symbol injections but did not close the
   completeness gap.
4. The 2026-04-14 07:50 `2/9` under HEAD `5e695d7` is **well within**
   the pre-existing distribution (2/9 also occurred at 00:13
   pre-audit). It is not a new regression.

## Commit-level proof of non-regression

`git log --oneline 133973d..5e695d7`:

```
5e695d7 agent/tool: P1.1 emit_evidence structured tool (default off)
652ee70 docs: P0.3 filtering pipeline DAG snapshot
7e4ecad context/builder: P0.1 visible /ungrounded tag in formatEvidenceItems
41f4b61 agent/finalizer: P0.2 runtime validators for step_list/value/boolean/config_value
6d8cb49 docs: architecture root-cause & remediation roadmap
```

None of these touch `internal/agent/erm.go:extractAnswerSymbols` or
any of its inputs:

- `6d8cb49`, `652ee70` — docs only
- `7e4ecad` (P0.1) — `formatEvidenceItems` in `builder.go` (the
  Structured Evidence section at line 560, not the Answer Symbols
  section at line 213); adds the `[UNGROUNDED: ...]` tag. Zero impact
  on `extractAnswerSymbols` or its gate.
- `41f4b61` (P0.2) — new validators in `finalizer.go` +
  `finalizer_validators.go`; does not touch `erm.go`. Runs *after*
  the allowlist is already baked into the prompt.
- `5e695d7` (P1.1) — `evidence_tool_mode` defaults to `off`;
  `cmd/root.go` does not register `emit_evidence` at boot;
  `ensureStructuredEvidence` is a no-op; zero behavior change from
  `652ee70`.

Therefore the 07:50 quality floor is NOT introduced by any of the
P0.1 / P0.2 / P0.3 / P0.4 / P1.1 ships. It is a pre-existing
structural flaw whose trip rate is governed by LLM-side variance in
Phase-1 evidence collection.

## Root cause statement

`extractAnswerSymbols` treats a partial, LLM-derived set of
terminal-literal evidence items as an authoritative *complete*
allowlist, without any completeness check for `enumeration` /
`list_of_symbols` questions. Downstream, `context/builder.go:213`
renders this allowlist with a hard "MUST NOT add or remove symbols"
directive plus a "Strict rules" block, giving the finalizer no way
to recognise that the list is a subset of the true answer.

In short: **the selection layer has no "maybe we missed some" escape
hatch, and the rendering layer sells an uncertain partial as a
verified complete.**

## Why this is cross-cutting, not just a t2 bug

Any enumeration question whose Phase-1 evidence happens to cover a
strict subset of the true answer is exposed:

- t2 (Evaluator types × ShouldStop conditions) — this memo
- df3 (ContinuationPrompt push strategies) — listed in
  `memory/project_df3_push_strategy_enumeration_gap.md` as
  "UNRESOLVED, cross-session"; the symptom is identical (5/9 push
  strategies answered, remaining 4 dropped)
- any future "list all handlers / all middlewares / all
  implementations of X" question

The unifying condition is:

```
questionKind == "enumeration"
  && answerShape == "list_of_symbols"
  && LLM Phase-1 evidence produces terminal literals for a
     proper subset of the true answer
```

## Non-fix — why this memo is NOT a patch

Per the roadmap guardrails in
`docs/architecture-root-cause-remediation.md` §7:

1. **Over-fitting audit required before any design.** A naive "just
   require N extracted symbols to match the Prior Stage Findings
   count" gate is a one-sided heuristic; it would trip on every
   legitimately short list.
2. **No hardcoded word lists.** Predicates must be structural.
3. **No grid PASS as ship gate.** The fix design must survive
   manual inspection, not just flip a verdict.
4. **N=2 threshold for generalisation.** t2 + df3 both exhibit
   the pattern, so the class is already established.

The right shape of the fix likely involves **one or more** of:

- A completeness check that compares `len(AnswerSymbols)` against the
  number of distinct `strictAnswerItems` that pass `hasTerminalEvidence`
  (or against the count inferred from AnalysisIR `EvidencePlan`), and
  falls back to nil when the ratio is suspiciously low.
- A softer rendering in `builder.go:213` when the allowlist is
  flagged "partial" — e.g., dropping the "MUST NOT add or remove"
  clause and treating it as a floor rather than a ceiling.
- A pre-check in `erm.go` that for `answerShape=list_of_symbols`
  cross-references the allowlist against the Prior Stage Findings
  enumeration in the explorer Answer text, and refuses to commit
  if the Prior Stage Findings names strictly more distinct subjects
  than the allowlist.
- Wiring this into **P1.3 QualityGate** once AnalysisIR carries an
  explicit `ExpectedCardinality` field.

Selecting between these requires:
1. A multi-run grid measurement of the enumeration-completeness hit
   rate across t2 + df3 + synthetic "list all X" probes,
2. Confirmation that none of the options re-introduces the df3
   mechanism-mode symptom that the 1709-line mechanism gate was
   specifically added to prevent, and
3. An over-fitting audit on the proposed gate predicate.

None of that work belongs inside a grid-validation session.

## Action items

- [ ] Schedule a dedicated session to design the completeness gate.
      Likely bundled with **P1.3** (AnalysisIR → QualityGate) since
      `ExpectedCardinality` is the cleanest data source.
- [ ] Add a synthetic "list all N" probe case to `eval/cases/` so
      the completeness regression has direct coverage, not just
      implicit t2/df3 signal.
- [ ] Record the L0-2 count in the eval harness metrics so future
      grid runs can distinguish "lucky 9/9" from "deterministic
      9/9".
- [ ] Link this memo from
      `docs/architecture-root-cause-remediation.md` §6 P1.3 as a
      concrete motivator.

## Do NOT

- **Do NOT flip `extractAnswerSymbols` to return nil for
  `enumeration` kind.** That would revert the original L0-2 extract-
  then-express design for questions where the 2-item allowlist is
  actually correct (e.g., "which two agents can invoke a subagent").
  The gate needs to be data-driven, not kind-gated.
- **Do NOT soften the `MUST NOT add or remove` directive
  globally.** On questions where the allowlist IS complete
  (list_of_symbols with exactly one true answer), the directive
  is the reason the finalizer stops hallucinating. Any softening
  must be conditional on a "partial" flag set by the completeness
  check.
- **Do NOT treat the audit-era 9/9 as the target baseline.** It is
  a stochastic 25% quartile of a 0/2/3/9 distribution, not a
  regression benchmark.

## Historical references

- `memory/project_fake_green_audit_2026_04_14.md` — the source of
  the "t2 2/9→9/9" claim that this memo downgrades to "t2 stochastic
  0-to-9/9 baseline"
- `memory/project_df3_push_strategy_enumeration_gap.md` — the
  cross-question sibling case
- `memory/project_answer_symbol_extraction_audit.md` — the Phase
  1-3 audit that established `extractAnswerSymbols` as the
  selection layer; completeness was out of scope there
- `docs/s1-s2-s3-fix-journey.md` — where the S3 symbol-allowlist
  validator (`c92068b`) was introduced; it assumes the allowlist
  is complete
- `docs/architecture-root-cause-remediation.md` §6 P1.3, §10 — the
  target fix window
