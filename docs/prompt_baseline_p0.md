# Baseline snapshot — batch 1 (HEAD pre-cleanup)

Comparison target for batches 2/3/4. Any batch that drops PASS count
below this baseline, OR shifts median-of-N run time by more than the
batch's documented tolerance, must revert.

- HEAD at snapshot: `d24088f` (main, clean)
- Date: 2026-04-22
- Runtime: Opus 4.7 (claude-opus-4-7[1m])

## Per-case pass rate

| case | runs | pass rate |
|---|---|---|
| s7a | 3 | 2 / 3 |
| s1a | 3 | 3 / 3 |
| u3a | 5 | 5 / 5 |
| logtri_go | 3 | 3 / 3 |
| logtri_java | 3 | 3 / 3 |

## Per-case verdict detail

### s7a — `s7a-20260422-153838`

## Verdicts

| run | result | reasons |
|----:|--------|---------|
| 1 | FAIL | no_regex_match:[0-9]{4,} |
| 2 | PASS | — |
| 3 | PASS | — |

**pass rate: 2 / 3**


### s1a — `s1a-20260422-153847`

## Verdicts

| run | result | reasons |
|----:|--------|---------|
| 1 | PASS | — |
| 2 | PASS | — |
| 3 | PASS | — |

**pass rate: 3 / 3**


### u3a — `u3a-20260422-153926`

## Verdicts

| run | result | reasons |
|----:|--------|---------|
| 1 | PASS | — |
| 2 | PASS | — |
| 3 | PASS | — |
| 4 | PASS | — |
| 5 | PASS | — |

**pass rate: 5 / 5**


### logtri_go — `logtri_go-20260422-153933`

## Verdicts

| run | result | reasons |
|----:|--------|---------|
| 1 | PASS | — |
| 2 | PASS | — |
| 3 | PASS | — |

**pass rate: 3 / 3**


### logtri_java — `logtri_java-20260422-153941`

## Verdicts

| run | result | reasons |
|----:|--------|---------|
| 1 | PASS | — |
| 2 | PASS | — |
| 3 | PASS | — |

**pass rate: 3 / 3**


## Per-case median metrics

### s7a

## Mechanism trace metrics

| metric | run 1|run 2|run 3 | median |
|--------|---|---|---|------|
| tool_read_file | 7 | 10 | 0 | 7 |
| concrete_values | 7 | 18 | 7 | 7 |
| synthesis_runs | 0 | 0 | 0 | 0 |
| function_boundary_push | 0 | 0 | 0 | 0 |
| enumeration_push | 0 | 0 | 0 | 0 |
| focus_warning | 0 | 0 | 0 | 0 |
| t11_gate_skip | 1 | 2 | 1 | 1 |
| t11_gate_run | 0 | 0 | 0 | 0 |
| dataflow_intent_lookup | 1 | 2 | 1 | 1 |
| dataflow_intent_propagate | 0 | 0 | 0 | 0 |
| midloop_inject | 4 | 3 | 0 | 3 |
| answer_chain_lines | 7 | 20 | 7 | 7 |


### s1a

## Mechanism trace metrics

| metric | run 1|run 2|run 3 | median |
|--------|---|---|---|------|
| tool_read_file | 14 | 20 | 20 | 20 |
| concrete_values | 16 | 16 | 16 | 16 |
| synthesis_runs | 0 | 0 | 0 | 0 |
| function_boundary_push | 0 | 0 | 0 | 0 |
| enumeration_push | 0 | 0 | 0 | 0 |
| focus_warning | 0 | 0 | 0 | 0 |
| t11_gate_skip | 0 | 0 | 0 | 0 |
| t11_gate_run | 2 | 2 | 2 | 2 |
| dataflow_intent_lookup | 2 | 2 | 2 | 2 |
| dataflow_intent_propagate | 0 | 0 | 0 | 0 |
| midloop_inject | 5 | 6 | 8 | 6 |
| answer_chain_lines | 5 | 9 | 5 | 5 |


### u3a

## Mechanism trace metrics

| metric | run 1|run 2|run 3|run 4|run 5 | median |
|--------|---|---|---|---|---|------|
| tool_read_file | 8 | 18 | 11 | 25 | 9 | 11 |
| concrete_values | 16 | 17 | 17 | 29 | 17 | 17 |
| synthesis_runs | 0 | 0 | 0 | 0 | 0 | 0 |
| function_boundary_push | 0 | 0 | 0 | 0 | 0 | 0 |
| enumeration_push | 0 | 0 | 0 | 0 | 0 | 0 |
| focus_warning | 0 | 0 | 0 | 0 | 0 | 0 |
| t11_gate_skip | 0 | 0 | 0 | 0 | 2 | 0 |
| t11_gate_run | 2 | 2 | 2 | 3 | 0 | 2 |
| dataflow_intent_lookup | 2 | 2 | 2 | 3 | 2 | 2 |
| dataflow_intent_propagate | 0 | 0 | 0 | 0 | 0 | 0 |
| midloop_inject | 4 | 5 | 6 | 12 | 3 | 5 |
| answer_chain_lines | 5 | 6 | 7 | 13 | 15 | 7 |


### logtri_go

## Mechanism trace metrics

| metric | run 1|run 2|run 3 | median |
|--------|---|---|---|------|
| tool_read_file | 16 | 20 | 16 | 16 |
| concrete_values | 7 | 7 | 7 | 7 |
| synthesis_runs | 0 | 0 | 0 | 0 |
| function_boundary_push | 0 | 0 | 0 | 0 |
| enumeration_push | 0 | 0 | 0 | 0 |
| focus_warning | 0 | 0 | 0 | 0 |
| t11_gate_skip | 0 | 0 | 1 | 0 |
| t11_gate_run | 0 | 0 | 0 | 0 |
| dataflow_intent_lookup | 0 | 0 | 1 | 0 |
| dataflow_intent_propagate | 0 | 0 | 0 | 0 |
| midloop_inject | 5 | 4 | 6 | 5 |
| answer_chain_lines | 2 | 3 | 5 | 3 |


### logtri_java

## Mechanism trace metrics

| metric | run 1|run 2|run 3 | median |
|--------|---|---|---|------|
| tool_read_file | 17 | 13 | 9 | 13 |
| concrete_values | 8 | 8 | 8 | 8 |
| synthesis_runs | 0 | 0 | 0 | 0 |
| function_boundary_push | 0 | 0 | 0 | 0 |
| enumeration_push | 0 | 0 | 0 | 0 |
| focus_warning | 0 | 0 | 0 | 0 |
| t11_gate_skip | 0 | 0 | 0 | 0 |
| t11_gate_run | 0 | 1 | 1 | 1 |
| dataflow_intent_lookup | 0 | 1 | 1 | 1 |
| dataflow_intent_propagate | 0 | 0 | 0 | 0 |
| midloop_inject | 5 | 3 | 4 | 4 |
| answer_chain_lines | 2 | 4 | 2 | 2 |



## Regression gates by batch

| Batch | Gate | Notes |
|---|---|---|
| 2A | PASS count ≥ baseline; u3a MUST hold 5/5 | pure mechanical rename |
| 2B | PASS count ≥ baseline; parser-contract headers stable | hint/schema desc rename |
| 3A | PASS count ≥ baseline; prescan_rounds median ±0; prescan_budget_exhausted does not rise | analyzer structure move |
| 3B | PASS count ≥ baseline; logtri coverage/diagram gates unchanged | skill dedup |
| 4A | PASS count ≥ baseline; s7a = baseline; new enum/count/zh cases added | keyword-example purge |
| 4B | All `TestNoInternalTermsInX` lints green in `t.Fatal` mode | hard gate |

